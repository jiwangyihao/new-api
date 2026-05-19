package controller

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

const (
	configGuideJSONContentType     = "application/json; charset=utf-8"
	configGuideYAMLContentType     = "application/yaml; charset=utf-8"
	configGuideTextContentType     = "text/plain; charset=utf-8"
	configGuideMarkdownContentType = "text/markdown; charset=utf-8"
	configGuideProviderID          = "new-api"
	configGuideImageProviderID     = "new-api-image"
	configGuideProviderToolsPkg    = "omp-openai-provider-tools"
	configGuideDefaultModelID      = "gpt-5"
	configGuideSmallModelID        = "gpt-5-mini"
	configGuideFastModelID         = "gpt-5-fast"
)

type configGuideManifest struct {
	SchemaVersion int               `json:"schema_version"`
	Client        string            `json:"client"`
	Title         string            `json:"title"`
	GeneratedAt   string            `json:"generated_at"`
	BaseURL       string            `json:"base_url"`
	Items         []configGuideItem `json:"items"`
	Notes         []string          `json:"notes,omitempty"`
}

type configGuideItem struct {
	ID          string  `json:"id"`
	Kind        string  `json:"kind"`
	Method      string  `json:"method"`
	URL         string  `json:"url"`
	TargetPath  *string `json:"target_path"`
	ContentType string  `json:"content_type"`
}

type configGuideQueryParams struct {
	apiKey          string
	baseURL         string
	explicitBaseURL string
}

type configGuideOMPModel struct {
	ID            string
	Name          string
	API           string
	Reasoning     bool
	Input         []string
	ContextWindow int
	MaxTokens     int
	Cost          configGuideOMPCost
}

type configGuideOMPCost struct {
	Input      float64
	Output     float64
	CacheRead  float64
	CacheWrite float64
}

func GetOMPConfigGuideManifest(c *gin.Context) {
	setConfigGuideNoStore(c)

	params, ok := configGuideQuery(c)
	if !ok {
		return
	}
	pluginVersion, ok := requireConfigGuideOMPProviderToolsVersion(c)
	if !ok {
		return
	}
	_ = pluginVersion
	models, ok := requireConfigGuideOpenAIModels(c)
	if !ok || !requireConfigGuideModels(c, models, []string{configGuideDefaultModelID, configGuideSmallModelID}) {
		return
	}

	writeConfigGuideJSON(c, configGuideManifest{
		SchemaVersion: 1,
		Client:        "omp",
		Title:         "new-api OpenAI for OMP",
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
		BaseURL:       params.baseURL,
		Items: []configGuideItem{
			{
				ID:          "plugin",
				Kind:        "instructions",
				Method:      http.MethodGet,
				URL:         configGuideItemURL("/config-guides/omp-openai/plugin.txt", params),
				ContentType: configGuideTextContentType,
			},
			{
				ID:          "models",
				Kind:        "file",
				Method:      http.MethodGet,
				URL:         configGuideItemURL("/config-guides/omp-openai/models.yml", params),
				TargetPath:  strPtr("~/.omp/agent/models.yml"),
				ContentType: configGuideYAMLContentType,
			},
			{
				ID:          "config",
				Kind:        "file",
				Method:      http.MethodGet,
				URL:         configGuideItemURL("/config-guides/omp-openai/config.yml", params),
				TargetPath:  strPtr("~/.omp/agent/config.yml"),
				ContentType: configGuideYAMLContentType,
			},
			{
				ID:          "image-generator",
				Kind:        "file",
				Method:      http.MethodGet,
				URL:         configGuideItemURL("/config-guides/omp-openai/image-generator.md", params),
				TargetPath:  strPtr("~/.omp/agent/agents/image-generator.md"),
				ContentType: configGuideMarkdownContentType,
			},
		},
		Notes: []string{
			"Download every item to a local temporary copy before editing existing files; do not transcribe YAML or JSON from chat output.",
			"If a target file is missing, copy the downloaded file to that path. If it exists, compare both files and merge the smaller side into the larger side when that is safer than replacing.",
			"After writing files, compare them with the downloaded copies and run the listed plugin doctor/check commands before reporting completion.",
			"Run plugin.txt commands before using provider-native web_search or image_generation.",
			"Restart OMP after installing or upgrading plugins and writing agent files.",
		},
	})
}

func GetOMPConfigGuidePlugin(c *gin.Context) {
	setConfigGuideNoStore(c)

	if _, ok := configGuideQuery(c); !ok {
		return
	}
	pluginVersion, ok := requireConfigGuideOMPProviderToolsVersion(c)
	if !ok {
		return
	}
	c.Data(http.StatusOK, configGuideTextContentType, []byte(renderConfigGuideOMPPlugin(pluginVersion)))
}

func GetOMPConfigGuideModels(c *gin.Context) {
	setConfigGuideNoStore(c)

	params, ok := configGuideQuery(c)
	if !ok {
		return
	}
	pluginVersion, ok := requireConfigGuideOMPProviderToolsVersion(c)
	if !ok {
		return
	}
	models, ok := requireConfigGuideOpenAIModels(c)
	if !ok || !requireConfigGuideModels(c, models, []string{configGuideDefaultModelID, configGuideSmallModelID}) {
		return
	}
	content, err := renderConfigGuideOMPModels(params.baseURL, params.apiKey, pluginVersion, models)
	if err != nil {
		writeConfigGuideError(c, http.StatusServiceUnavailable, "OpenAI model metadata incomplete")
		return
	}
	c.Data(http.StatusOK, configGuideYAMLContentType, []byte(content))
}

func GetOMPConfigGuideConfig(c *gin.Context) {
	setConfigGuideNoStore(c)

	if _, ok := configGuideQuery(c); !ok {
		return
	}
	c.Data(http.StatusOK, configGuideYAMLContentType, []byte(renderConfigGuideOMPSettings()))
}

func GetOMPConfigGuideImageGenerator(c *gin.Context) {
	setConfigGuideNoStore(c)

	if _, ok := configGuideQuery(c); !ok {
		return
	}
	c.Data(http.StatusOK, configGuideMarkdownContentType, []byte(renderConfigGuideOMPImageGenerator()))
}

func GetOpenCodeConfigGuideManifest(c *gin.Context) {
	setConfigGuideNoStore(c)

	params, ok := configGuideQuery(c)
	if !ok {
		return
	}
	models, ok := requireConfigGuideOpenAIModels(c)
	if !ok || !requireConfigGuideModels(c, models, []string{configGuideDefaultModelID}) {
		return
	}

	writeConfigGuideJSON(c, configGuideManifest{
		SchemaVersion: 1,
		Client:        "opencode",
		Title:         "new-api OpenAI for OpenCode",
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
		BaseURL:       params.baseURL,
		Items: []configGuideItem{
			{
				ID:          "opencode",
				Kind:        "file",
				Method:      http.MethodGet,
				URL:         configGuideItemURL("/config-guides/opencode-openai/opencode.json", params),
				TargetPath:  strPtr("~/.config/opencode/opencode.json"),
				ContentType: configGuideJSONContentType,
			},
		},
		Notes: []string{
			"Download every item to a local temporary copy before editing existing files; do not transcribe JSON from chat output.",
			"If the target file exists, merge the provider, model, and agent sections instead of replacing unrelated user settings.",
			"After writing opencode.json, compare it with the downloaded copy and run an OpenCode configuration parse/check command if available before reporting completion.",
			"This config adds provider new-api and does not replace OpenCode built-in providers.",
		},
	})
}

func GetOpenCodeConfigGuideJSON(c *gin.Context) {
	setConfigGuideNoStore(c)

	params, ok := configGuideQuery(c)
	if !ok {
		return
	}
	models, ok := requireConfigGuideOpenAIModels(c)
	if !ok || !requireConfigGuideModels(c, models, []string{configGuideDefaultModelID}) {
		return
	}
	content, err := renderConfigGuideOpenCode(params.baseURL, params.apiKey, models)
	if err != nil {
		writeConfigGuideError(c, http.StatusServiceUnavailable, "OpenAI model metadata incomplete")
		return
	}
	c.Data(http.StatusOK, configGuideJSONContentType, content)
}

func setConfigGuideNoStore(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
}

func configGuideQuery(c *gin.Context) (configGuideQueryParams, bool) {
	apiKey := strings.TrimSpace(c.Query("api_key"))
	if apiKey == "" || containsControlCharacter(apiKey) {
		writeConfigGuideError(c, http.StatusBadRequest, "api_key is required")
		return configGuideQueryParams{}, false
	}

	baseURL, explicitBaseURL, err := buildConfigGuideBaseURL(c)
	if err != nil {
		writeConfigGuideError(c, http.StatusBadRequest, "invalid base_url")
		return configGuideQueryParams{}, false
	}

	return configGuideQueryParams{
		apiKey:          normalizeConfigGuideAPIKey(apiKey),
		baseURL:         baseURL,
		explicitBaseURL: explicitBaseURL,
	}, true
}

func normalizeConfigGuideAPIKey(apiKey string) string {
	apiKey = strings.TrimSpace(apiKey)
	for strings.HasPrefix(apiKey, "sk-sk-") {
		apiKey = strings.TrimPrefix(apiKey, "sk-")
	}
	if apiKey == "" || strings.HasPrefix(apiKey, "sk-") {
		return apiKey
	}
	return "sk-" + apiKey
}

func buildConfigGuideBaseURL(c *gin.Context) (baseURL string, explicitBaseURL string, err error) {
	if _, ok := c.GetQuery("base_url"); ok {
		raw := strings.TrimSpace(c.Query("base_url"))
		if raw == "" || containsControlCharacter(raw) {
			return "", "", errors.New("invalid explicit base_url")
		}
		parsed, err := url.Parse(raw)
		if err != nil {
			return "", "", err
		}
		scheme := strings.ToLower(parsed.Scheme)
		if scheme != "http" && scheme != "https" {
			return "", "", errors.New("invalid explicit base_url scheme")
		}
		if strings.TrimSpace(parsed.Host) == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
			return "", "", errors.New("invalid explicit base_url components")
		}
		return strings.TrimRight(raw, "/"), raw, nil
	}

	return deriveConfigGuideBaseURL(c)
}

func deriveConfigGuideBaseURL(c *gin.Context) (string, string, error) {
	scheme := ""
	if c.Request != nil {
		firstForwardedProto := strings.ToLower(strings.TrimSpace(firstForwardedValue(c.Request.Header.Get("X-Forwarded-Proto"))))
		if firstForwardedProto == "http" || firstForwardedProto == "https" {
			scheme = firstForwardedProto
		}
	}
	if scheme == "" {
		if c.Request != nil && c.Request.TLS != nil {
			scheme = "https"
		} else {
			scheme = "http"
		}
	}

	host := ""
	if c.Request != nil {
		host = strings.TrimSpace(c.Request.Host)
	}
	if host == "" || containsControlCharacter(host) || strings.ContainsAny(host, "?#") {
		return "", "", errors.New("invalid request host")
	}

	return strings.TrimRight(scheme+"://"+host, "/") + "/v1", "", nil
}

func firstForwardedValue(raw string) string {
	if idx := strings.Index(raw, ","); idx >= 0 {
		return raw[:idx]
	}
	return raw
}

func configGuideItemURL(path string, params configGuideQueryParams) string {
	query := url.Values{}
	query.Set("api_key", params.apiKey)
	if params.explicitBaseURL != "" {
		query.Set("base_url", params.explicitBaseURL)
	}
	return path + "?" + query.Encode()
}

func requireConfigGuideOMPProviderToolsVersion(c *gin.Context) (string, bool) {
	metadata := getOpenCodeMetadataProvider().GetOMPProviderToolsMetadata(c.Request.Context())
	version := strings.TrimSpace(metadata.LatestVersion)
	status := strings.ToLower(strings.TrimSpace(metadata.Status))
	if version == "" || (status != "ok" && status != "cached") {
		writeConfigGuideError(c, http.StatusServiceUnavailable, "OMP provider tools metadata unavailable")
		return "", false
	}
	return version, true
}

func requireConfigGuideOpenAIModels(c *gin.Context) (map[string]service.OpenCodeOpenAIModel, bool) {
	models, err := getOpenCodeMetadataProvider().GetOpenAIModels(c.Request.Context())
	if err != nil || len(models) == 0 {
		writeConfigGuideError(c, http.StatusServiceUnavailable, "OpenCode OpenAI model metadata unavailable")
		return nil, false
	}
	return models, true
}

func requireConfigGuideModels(c *gin.Context, models map[string]service.OpenCodeOpenAIModel, ids []string) bool {
	for _, id := range ids {
		if _, ok := models[id]; !ok {
			writeConfigGuideError(c, http.StatusServiceUnavailable, "OpenAI model metadata incomplete")
			return false
		}
	}
	return true
}

func writeConfigGuideJSON(c *gin.Context, value any) {
	payload, err := common.Marshal(value)
	if err != nil {
		writeConfigGuideError(c, http.StatusInternalServerError, "failed to render config guide")
		return
	}
	c.Data(http.StatusOK, configGuideJSONContentType, payload)
}

func writeConfigGuideError(c *gin.Context, status int, message string) {
	c.JSON(status, gin.H{"success": false, "message": message})
}

func renderConfigGuideOMPPlugin(version string) string {
	return fmt.Sprintf(`# 1. Install or upgrade provider-native tools plugin
omp plugin install npm:%s@%s

# 2. Check plugin health
omp plugin doctor

# 3. Preview the recommended image subagent template
npx %s configure-image-agent --model %s/%s-Sys --dry-run

# 4. After reviewing the preview, write ~/.omp/agent/agents/image-generator.md
npx %s configure-image-agent --model %s/%s-Sys

# If image_generator already exists, the command refuses to overwrite it.
# Use --print to inspect and merge manually; use --force only when you intentionally replace it.
npx %s configure-image-agent --model %s/%s-Sys --print`, configGuideProviderToolsPkg, version, configGuideProviderToolsPkg, configGuideImageProviderID, configGuideDefaultModelID, configGuideProviderToolsPkg, configGuideImageProviderID, configGuideDefaultModelID, configGuideProviderToolsPkg, configGuideImageProviderID, configGuideDefaultModelID)
}

func renderConfigGuideOMPModels(baseURL string, apiKey string, pluginVersion string, models map[string]service.OpenCodeOpenAIModel) (string, error) {
	baseModels := make(map[string]configGuideOMPModel, len(models))
	for id, model := range models {
		baseModels[id] = normalizeConfigGuideOMPModel(model)
	}
	expanded := withConfigGuideOMPSysVariants(baseModels)

	selectedIDs := []string{configGuideDefaultModelID, configGuideDefaultModelID + "-Sys", configGuideSmallModelID, configGuideSmallModelID + "-Sys"}
	selected := make([]string, 0, len(selectedIDs))
	for _, id := range selectedIDs {
		model, ok := expanded[id]
		if !ok {
			return "", fmt.Errorf("required OMP model missing: %s", id)
		}
		selected = append(selected, renderConfigGuideOMPModelYAML(model, "      ", nil))
	}

	imageSource, ok := expanded[configGuideDefaultModelID+"-Sys"]
	if !ok {
		return "", fmt.Errorf("required OMP image source missing")
	}
	imageSource.ID = configGuideDefaultModelID + "-Sys"
	imageSource.Name = models[configGuideDefaultModelID].Name + " Image (Sys)"
	imageYAML := renderConfigGuideOMPModelYAML(imageSource, "      ", []string{
		"        compat:",
		"          openaiProviderTools:",
		"            imageGeneration: true",
	})

	return fmt.Sprintf(`# Image generation and provider-native web_search require this plugin:
#   omp plugin install npm:%s@%s
#   omp plugin doctor
# Recommended image subagent command:
#   npx %s configure-image-agent --model %s/%s-Sys --dry-run
# Restart OMP after installing or upgrading the plugin.
providers:
  %s:
    api: openai-responses
    baseUrl: %s
    apiKey: %s
    compat:
      openaiProviderTools:
        enabled: true
    models:
%s

  %s:
    api: openai-responses
    baseUrl: %s
    apiKey: %s
    compat:
      openaiProviderTools:
        enabled: true
    models:
%s

equivalence:
  overrides:
    %[14]s/%[16]s: %[16]s
    %[14]s/%[16]s-Sys: %[16]s-sys
    %[14]s/%[17]s: %[17]s
    %[14]s/%[17]s-Sys: %[17]s-sys
    %[15]s/%[16]s-Sys: %[16]s-image-sys`, configGuideProviderToolsPkg, pluginVersion, configGuideProviderToolsPkg, configGuideImageProviderID, configGuideDefaultModelID, configGuideProviderID, baseURL, apiKey, strings.Join(selected, "\n"), configGuideImageProviderID, baseURL, apiKey, imageYAML, configGuideProviderID, configGuideImageProviderID, configGuideDefaultModelID, configGuideSmallModelID), nil
}

func normalizeConfigGuideOMPModel(model service.OpenCodeOpenAIModel) configGuideOMPModel {
	input := make([]string, 0, len(model.Modalities.Input))
	for _, item := range model.Modalities.Input {
		if item == "text" || item == "image" {
			input = append(input, item)
		}
	}
	if len(input) == 0 {
		input = []string{"text"}
	}
	return configGuideOMPModel{
		ID:            model.ID,
		Name:          model.Name,
		API:           "openai-responses",
		Reasoning:     model.Reasoning,
		Input:         input,
		ContextWindow: model.Limit.Context,
		MaxTokens:     model.Limit.Output,
		Cost: configGuideOMPCost{
			Input:      model.Cost.Input,
			Output:     model.Cost.Output,
			CacheRead:  model.Cost.CacheRead,
			CacheWrite: model.Cost.CacheWrite,
		},
	}
}

func withConfigGuideOMPSysVariants(models map[string]configGuideOMPModel) map[string]configGuideOMPModel {
	expanded := make(map[string]configGuideOMPModel, len(models)*2)
	for id, model := range models {
		expanded[id] = cloneConfigGuideOMPModel(model)
		sys := cloneConfigGuideOMPModel(model)
		sys.ID = model.ID + "-Sys"
		sys.Name = model.Name + " (Sys)"
		expanded[id+"-Sys"] = sys
	}
	return expanded
}

func cloneConfigGuideOMPModel(model configGuideOMPModel) configGuideOMPModel {
	copy := model
	copy.Input = append([]string(nil), model.Input...)
	return copy
}

func renderConfigGuideOMPModelYAML(model configGuideOMPModel, indent string, extraLines []string) string {
	lines := []string{
		fmt.Sprintf("%s- id: %s", indent, model.ID),
		fmt.Sprintf("%s  name: %s", indent, configGuideYAMLDoubleQuotedScalar(model.Name)),
		fmt.Sprintf("%s  api: %s", indent, model.API),
		fmt.Sprintf("%s  reasoning: %t", indent, model.Reasoning),
		fmt.Sprintf("%s  input:", indent),
	}
	for _, item := range model.Input {
		lines = append(lines, fmt.Sprintf("%s    - %s", indent, item))
	}
	if model.ContextWindow != 0 {
		lines = append(lines, fmt.Sprintf("%s  contextWindow: %d", indent, model.ContextWindow))
	}
	if model.MaxTokens != 0 {
		lines = append(lines, fmt.Sprintf("%s  maxTokens: %d", indent, model.MaxTokens))
	}
	costLines := renderConfigGuideOMPCostYAML(model.Cost, indent+"    ")
	if len(costLines) > 0 {
		lines = append(lines, fmt.Sprintf("%s  cost:", indent))
		lines = append(lines, costLines...)
	}
	lines = append(lines, extraLines...)
	return strings.Join(lines, "\n")
}

func renderConfigGuideOMPCostYAML(cost configGuideOMPCost, indent string) []string {
	var lines []string
	if cost.Input != 0 {
		lines = append(lines, fmt.Sprintf("%sinput: %s", indent, formatConfigGuideFloat(cost.Input)))
	}
	if cost.Output != 0 {
		lines = append(lines, fmt.Sprintf("%soutput: %s", indent, formatConfigGuideFloat(cost.Output)))
	}
	if cost.CacheRead != 0 {
		lines = append(lines, fmt.Sprintf("%scacheRead: %s", indent, formatConfigGuideFloat(cost.CacheRead)))
	}
	lines = append(lines, fmt.Sprintf("%scacheWrite: %s", indent, formatConfigGuideFloat(cost.CacheWrite)))
	return lines
}

func configGuideYAMLDoubleQuotedScalar(value string) string {
	encoded, err := common.Marshal(value)
	if err != nil {
		return "\"\""
	}
	return string(encoded)
}

func renderConfigGuideOMPSettings() string {
	return fmt.Sprintf(`defaultThinkingLevel: xhigh
serviceTier: priority

modelRoles:
  default: %s/%s-Sys
  slow: %s/%s-Sys
  smol: %s/%s-Sys
  plan: %s/%s-Sys
  task: %s/%s-Sys:xhigh
  vision: %s/%s-Sys
  designer: %s/%s-Sys:xhigh
  commit: %s/%s-Sys:xhigh

task:
  agentModelOverrides:
    explore: %s/%s-Sys:xhigh
    librarian: %s/%s-Sys:xhigh
    reviewer: %s/%s-Sys:xhigh
    plan: %s/%s-Sys:xhigh`, configGuideProviderID, configGuideDefaultModelID, configGuideProviderID, configGuideDefaultModelID, configGuideProviderID, configGuideSmallModelID, configGuideProviderID, configGuideDefaultModelID, configGuideProviderID, configGuideDefaultModelID, configGuideProviderID, configGuideDefaultModelID, configGuideProviderID, configGuideDefaultModelID, configGuideProviderID, configGuideDefaultModelID, configGuideProviderID, configGuideSmallModelID, configGuideProviderID, configGuideSmallModelID, configGuideProviderID, configGuideDefaultModelID, configGuideProviderID, configGuideDefaultModelID)
}

func renderConfigGuideOMPImageGenerator() string {
	return fmt.Sprintf(`---
name: image_generator
description: Generate or iterate images only; do not handle ordinary code modification tasks.
model: %s/%s-Sys:xhigh
---

You are a specialized image generation subagent.

Use the provider-native image generation capability to create or refine images when the user explicitly asks for visual output. Do not take over normal coding, refactoring, debugging, or documentation tasks. Return concise status and generated image references to the caller.`, configGuideImageProviderID, configGuideDefaultModelID)
}

func renderConfigGuideOpenCode(baseURL string, apiKey string, models map[string]service.OpenCodeOpenAIModel) ([]byte, error) {
	if _, ok := models[configGuideDefaultModelID]; !ok {
		return nil, fmt.Errorf("%s missing", configGuideDefaultModelID)
	}
	smallModelID := configGuideSmallModelID
	if !isConfigGuideOpenCodeTextModel(models[configGuideSmallModelID]) {
		smallModelID = configGuideDefaultModelID
	}

	openAIModels := buildConfigGuideOpenCodeBaseModels(models)

	cfg := map[string]any{
		"provider": map[string]any{
			configGuideProviderID: map[string]any{
				"npm":  "@ai-sdk/openai",
				"name": configGuideProviderID,
				"options": map[string]any{
					"baseURL": baseURL,
					"apiKey":  apiKey,
				},
				"models": openAIModels,
			},
		},
		"model":       configGuideProviderID + "/" + configGuideDefaultModelID,
		"small_model": configGuideProviderID + "/" + smallModelID,
		"agent": map[string]any{
			"build": map[string]any{
				"options": map[string]any{"store": false},
			},
			"plan": map[string]any{
				"options": map[string]any{"store": false},
			},
		},
		"$schema": "https://opencode.ai/config.json",
	}
	return common.Marshal(cfg)
}

func buildConfigGuideOpenCodeBaseModels(source map[string]service.OpenCodeOpenAIModel) map[string]any {
	ids := make([]string, 0, len(source))
	for id := range source {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	out := make(map[string]any, len(source))
	for _, id := range ids {
		model := source[id]
		if !isConfigGuideOpenCodeTextModel(model) {
			continue
		}
		out[id] = normalizeConfigGuideOpenCodeModel(id, model)
	}
	return out
}

func isConfigGuideOpenCodeTextModel(model service.OpenCodeOpenAIModel) bool {
	return containsConfigGuideString(model.Modalities.Output, "text") && configGuideOpenCodeRequiredPricingComplete(model)
}

func configGuideOpenCodeRequiredPricingComplete(model service.OpenCodeOpenAIModel) bool {
	return model.Cost.Input > 0 && model.Cost.Output > 0 &&
		model.Limit.Context > 0 && model.Limit.Output > 0
}

func containsConfigGuideString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func normalizeConfigGuideOpenCodeModel(id string, model service.OpenCodeOpenAIModel) map[string]any {
	config := map[string]any{
		"id":          strings.TrimSuffix(id, "-fast"),
		"name":        model.Name,
		"attachment":  model.Attachment,
		"reasoning":   model.Reasoning,
		"tool_call":   model.ToolCall,
		"temperature": model.Temperature,
		"options":     mergeConfigGuideOpenCodeModelOptions(model.Options),
		"variants":    buildConfigGuideOpenCodeReasoningVariants(configGuideReasoningLevels(id, model)),
	}
	if model.Family != "" {
		config["family"] = model.Family
	}
	if model.Knowledge != "" {
		config["knowledge"] = model.Knowledge
	}
	if model.Interleaved != nil {
		config["interleaved"] = model.Interleaved
	}
	if len(model.Modalities.Input) > 0 || len(model.Modalities.Output) > 0 {
		modalities := map[string]any{}
		if len(model.Modalities.Input) > 0 {
			modalities["input"] = append([]string(nil), model.Modalities.Input...)
		}
		if len(model.Modalities.Output) > 0 {
			modalities["output"] = append([]string(nil), model.Modalities.Output...)
		}
		config["modalities"] = modalities
	}
	if cost := configGuideOpenCodeCostMap(model.Cost); len(cost) > 0 {
		config["cost"] = cost
	}
	if limit := configGuideOpenCodeLimitMap(model.Limit); len(limit) > 0 {
		config["limit"] = limit
	}
	if model.ReleaseDate != "" {
		config["release_date"] = model.ReleaseDate
	}
	if len(model.Headers) > 0 {
		config["headers"] = cloneStringAnyMapFromString(model.Headers)
	}
	return config
}

func buildConfigGuideOpenCodeReasoningVariants(levels []string) map[string]any {
	variants := make(map[string]any, len(levels))
	for _, level := range levels {
		variants[level] = map[string]any{
			"reasoningEffort":  level,
			"reasoningSummary": "auto",
			"include":          []string{"reasoning.encrypted_content"},
		}
	}
	return variants
}

func configGuideReasoningLevels(id string, model service.OpenCodeOpenAIModel) []string {
	if !model.Reasoning {
		return nil
	}
	lower := strings.ToLower(id)
	if lower == "gpt-5-pro" {
		return nil
	}
	if lower == "gpt-5-codex" || lower == "gpt-5.1-codex" || lower == "gpt-5.1-codex-max" || lower == "gpt-5.1-codex-mini" || lower == "codex-mini-latest" {
		return []string{"low", "medium", "high"}
	}
	if lower == "gpt-5.3-codex-spark" || lower == "gpt-5.3-codex" || lower == "gpt-5.2-codex" {
		return []string{"low", "medium", "high", "xhigh"}
	}
	levels := []string{"low", "medium", "high"}
	if strings.Contains(lower, "gpt-5-") || lower == "gpt-5" {
		levels = append([]string{"minimal"}, levels...)
	}
	if model.ReleaseDate >= "2025-11-13" {
		levels = append([]string{"none"}, levels...)
	}
	if model.ReleaseDate >= "2025-12-04" {
		levels = append(levels, "xhigh")
	}
	return levels
}

func mergeConfigGuideOpenCodeModelOptions(options map[string]any) map[string]any {
	out := deepCloneStringAnyMap(options)
	out["store"] = false
	return out
}

func configGuideOpenCodeCostMap(cost service.OpenCodeOpenAIModelCost) map[string]any {
	if cost.Input <= 0 || cost.Output <= 0 {
		return nil
	}
	out := map[string]any{
		"input":  cost.Input,
		"output": cost.Output,
	}
	if cost.CacheRead != 0 {
		out["cache_read"] = cost.CacheRead
	}
	if cost.CacheWrite != 0 {
		out["cache_write"] = cost.CacheWrite
	}
	if contextOver200K := configGuideOpenCodeContextOver200KCostMap(cost.ContextOver200K); len(contextOver200K) > 0 {
		out["context_over_200k"] = contextOver200K
	}
	return out
}

func configGuideOpenCodeContextOver200KCostMap(cost service.OpenCodeOpenAIModelContextCost) map[string]any {
	if cost.Input <= 0 || cost.Output <= 0 {
		return nil
	}
	out := map[string]any{
		"input":  cost.Input,
		"output": cost.Output,
	}
	if cost.CacheRead != 0 {
		out["cache_read"] = cost.CacheRead
	}
	if cost.CacheWrite != 0 {
		out["cache_write"] = cost.CacheWrite
	}
	return out
}

func configGuideOpenCodeLimitMap(limit service.OpenCodeOpenAIModelLimit) map[string]any {
	if limit.Context <= 0 || limit.Output <= 0 {
		return nil
	}
	out := map[string]any{
		"context": limit.Context,
		"output":  limit.Output,
	}
	if limit.Input != 0 {
		out["input"] = limit.Input
	}
	return out
}

func cloneStringAnyMapFromString(src map[string]string) map[string]any {
	out := make(map[string]any, len(src))
	for key, value := range src {
		out[key] = value
	}
	return out
}

func ensureStringAnyMap(value any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	if mapped, ok := value.(map[string]any); ok {
		return deepCloneStringAnyMap(mapped)
	}
	return map[string]any{}
}

func deepCloneStringAnyMap(src map[string]any) map[string]any {
	if len(src) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(src))
	for key, value := range src {
		switch typed := value.(type) {
		case map[string]any:
			out[key] = deepCloneStringAnyMap(typed)
		case map[string]string:
			out[key] = cloneStringAnyMapFromString(typed)
		case []string:
			out[key] = append([]string(nil), typed...)
		case []any:
			out[key] = append([]any(nil), typed...)
		default:
			out[key] = typed
		}
	}
	return out
}

func containsControlCharacter(value string) bool {
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}

func strPtr(v string) *string {
	return &v
}

func formatConfigGuideFloat(value float64) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.10f", value), "0"), ".")
}
