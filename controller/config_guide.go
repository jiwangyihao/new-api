package controller

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	configGuideJSONContentType = "application/json; charset=utf-8"
	configGuideYAMLContentType = "application/yaml; charset=utf-8"
	configGuideProviderID      = "new-api"
	configGuideDefaultModelID  = "gpt-5.5"
	configGuideSmallModelID    = "gpt-5.4-mini"
)

type configGuideClient string

const (
	configGuideClientOpenCode configGuideClient = "opencode"
	configGuideClientOMP      configGuideClient = "omp"
)

type configGuideEffectiveModelsInput struct {
	Client          configGuideClient
	Metadata        map[string]service.OpenCodeOpenAIModel
	AvailableModels []string
}

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

	params, _, ok := requireConfigGuideEffectiveModels(c, configGuideClientOMP)
	if !ok {
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
		},
		Notes: []string{
			"Download every item to a local temporary copy before editing existing files; do not transcribe YAML from chat output.",
			"If a target file is missing, copy the downloaded file to that path. If it exists, compare both files and merge the smaller side into the larger side when that is safer than replacing.",
			"After writing files, compare them with the downloaded copies before reporting completion.",
		},
	})
}

func GetOMPConfigGuideModels(c *gin.Context) {
	setConfigGuideNoStore(c)

	params, models, ok := requireConfigGuideEffectiveModels(c, configGuideClientOMP)
	if !ok {
		return
	}
	content, err := renderConfigGuideOMPModels(params.baseURL, params.apiKey, models)
	if err != nil {
		writeConfigGuideError(c, http.StatusServiceUnavailable, "OpenAI model metadata incomplete")
		return
	}
	c.Data(http.StatusOK, configGuideYAMLContentType, []byte(content))
}

func GetOMPConfigGuideConfig(c *gin.Context) {
	setConfigGuideNoStore(c)

	_, models, ok := requireConfigGuideEffectiveModels(c, configGuideClientOMP)
	if !ok {
		return
	}
	content, err := renderConfigGuideOMPSettings(models)
	if err != nil {
		writeConfigGuideError(c, http.StatusServiceUnavailable, "OpenAI model metadata incomplete")
		return
	}
	c.Data(http.StatusOK, configGuideYAMLContentType, []byte(content))
}

func GetOpenCodeConfigGuideManifest(c *gin.Context) {
	setConfigGuideNoStore(c)

	params, _, ok := requireConfigGuideEffectiveModels(c, configGuideClientOpenCode)
	if !ok {
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

	params, models, ok := requireConfigGuideEffectiveModels(c, configGuideClientOpenCode)
	if !ok {
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
	rawAPIKey := c.Query("api_key")
	if rawAPIKey == "" || containsControlCharacter(rawAPIKey) {
		writeConfigGuideError(c, http.StatusBadRequest, "api_key is required")
		return configGuideQueryParams{}, false
	}
	apiKey := strings.TrimSpace(rawAPIKey)
	if apiKey == "" {
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
	key, err := parseConfigGuideTokenKey(apiKey)
	if err != nil {
		return ""
	}
	return "sk-" + key
}

func parseConfigGuideTokenKey(raw string) (string, error) {
	if raw == "" || containsControlCharacter(raw) {
		return "", errors.New("invalid api_key")
	}
	key := strings.TrimSpace(raw)
	if key == "" {
		return "", errors.New("invalid api_key")
	}
	if len(key) >= len("Bearer ") && strings.EqualFold(key[:len("Bearer ")], "Bearer ") {
		key = strings.TrimSpace(key[len("Bearer "):])
		if key == "" || containsControlCharacter(key) {
			return "", errors.New("invalid api_key")
		}
	}
	for strings.HasPrefix(key, "sk-sk-") {
		key = strings.TrimPrefix(key, "sk-")
	}
	key = strings.TrimPrefix(key, "sk-")
	if idx := strings.IndexByte(key, '-'); idx >= 0 {
		key = key[:idx]
	}
	if key == "" || containsControlCharacter(key) {
		return "", errors.New("invalid api_key")
	}
	return key, nil
}

func loadConfigGuideTokenByPublicKey(c *gin.Context, apiKey string) (*model.Token, bool) {
	key, err := parseConfigGuideTokenKey(apiKey)
	if err != nil {
		writeConfigGuideError(c, http.StatusBadRequest, "invalid api_key")
		return nil, false
	}
	token, err := model.GetTokenByKey(key, false)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeConfigGuideError(c, http.StatusUnauthorized, "invalid api_key")
		} else {
			common.SysLog("config guide token lookup failed: " + err.Error())
			writeConfigGuideError(c, http.StatusInternalServerError, "failed to validate api_key")
		}
		return nil, false
	}
	return token, true
}

func loadConfigGuideTokenByID(c *gin.Context, tokenID int, userID int) (*model.Token, bool) {
	token, err := model.GetTokenByIds(tokenID, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) || tokenID <= 0 || userID <= 0 {
			writeConfigGuideError(c, http.StatusUnauthorized, "token not found")
		} else {
			common.SysLog("config guide token id lookup failed: " + err.Error())
			writeConfigGuideError(c, http.StatusInternalServerError, "failed to validate token")
		}
		return nil, false
	}
	return token, true
}

func validateConfigGuideTokenUsability(c *gin.Context, token *model.Token) (*model.UserBase, string, bool) {
	if token == nil {
		writeConfigGuideError(c, http.StatusUnauthorized, "invalid api_key")
		return nil, "", false
	}
	if token.Status == common.TokenStatusExhausted {
		writeConfigGuideError(c, http.StatusTooManyRequests, "token quota exhausted")
		return nil, "", false
	}
	if token.Status != common.TokenStatusEnabled || token.ExpiredTime != -1 && token.ExpiredTime < common.GetTimestamp() {
		writeConfigGuideError(c, http.StatusForbidden, "token is not usable")
		return nil, "", false
	}

	userCache, err := model.GetUserCache(token.UserId)
	if err != nil {
		common.SysLog(fmt.Sprintf("config guide GetUserCache error for user %d: %v", token.UserId, err))
		writeConfigGuideError(c, http.StatusInternalServerError, "failed to validate user")
		return nil, "", false
	}
	if userCache.Status != common.UserStatusEnabled {
		writeConfigGuideError(c, http.StatusForbidden, "user is not enabled")
		return nil, "", false
	}

	allowIps := token.GetIpLimits()
	if len(allowIps) > 0 {
		clientIP := net.ParseIP(c.ClientIP())
		if clientIP == nil || !common.IsIpInCIDRList(clientIP, allowIps) {
			writeConfigGuideError(c, http.StatusForbidden, "client ip is not allowed")
			return nil, "", false
		}
	}

	// Legacy user/token group fields are ignored for config guide eligibility.

	c.Set("id", token.UserId)
	c.Set("token_id", token.Id)
	c.Set("token_key", token.Key)
	c.Set("token_name", token.Name)
	c.Set("token_unlimited_quota", token.UnlimitedQuota)
	if !token.UnlimitedQuota {
		c.Set("token_quota", token.RemainQuota)
	}
	if token.ModelLimitsEnabled {
		c.Set("token_model_limit_enabled", true)
		c.Set("token_model_limit", token.GetModelLimitsMap())
	} else {
		c.Set("token_model_limit_enabled", false)
	}

	userCache.WriteContext(c)
	return userCache, "", true
}

func availableConfigGuideModelsForToken(token *model.Token, user *model.UserBase) []string {
	acceptUnsetRatioModel := operation_setting.SelfUseModeEnabled
	if !acceptUnsetRatioModel && user != nil && user.Id > 0 {
		userSettings, _ := model.GetUserSetting(user.Id, false)
		if userSettings.AcceptUnsetRatioModel {
			acceptUnsetRatioModel = true
		}
	}

	if token != nil && token.IsModelLimitsEnabled() {
		limits := token.GetModelLimits()
		models := make([]string, 0, len(limits))
		for _, modelName := range limits {
			models = appendConfigGuideAvailableModel(models, modelName, acceptUnsetRatioModel)
		}
		return models
	}

	models := model.GetEnabledModels()

	filtered := models[:0]
	for _, modelName := range models {
		filtered = appendConfigGuideAvailableModel(filtered, modelName, acceptUnsetRatioModel)
	}
	return filtered
}

func appendConfigGuideAvailableModel(models []string, modelName string, acceptUnsetRatioModel bool) []string {
	normalized := normalizeConfigGuideAvailableModelID(modelName)
	if normalized == "" {
		return models
	}
	if !acceptUnsetRatioModel && !helper.HasModelBillingConfig(normalized) {
		return models
	}
	return append(models, normalized)
}

func buildConfigGuideEffectiveModels(input configGuideEffectiveModelsInput) (map[string]service.OpenCodeOpenAIModel, error) {
	if len(input.Metadata) == 0 {
		return nil, errors.New("OpenAI model metadata unavailable")
	}
	available := make(map[string]struct{}, len(input.AvailableModels))
	for _, id := range input.AvailableModels {
		id = normalizeConfigGuideAvailableModelID(id)
		if id != "" {
			available[id] = struct{}{}
		}
	}

	effective := make(map[string]service.OpenCodeOpenAIModel)
	metadataIDs := make([]string, 0, len(input.Metadata))
	for id := range input.Metadata {
		metadataIDs = append(metadataIDs, id)
	}
	sort.Strings(metadataIDs)
	for _, metadataID := range metadataIDs {
		normalizedID := normalizeConfigGuideAvailableModelID(metadataID)
		if normalizedID == "" {
			continue
		}
		_, directAvailable := available[normalizedID]
		fastBase := strings.TrimSuffix(normalizedID, "-fast")
		_, baseAvailable := available[fastBase]
		if !directAvailable && (input.Client != configGuideClientOpenCode || normalizedID == fastBase || !baseAvailable) {
			continue
		}
		if input.Client != configGuideClientOpenCode && strings.HasSuffix(normalizedID, "-fast") && !directAvailable {
			continue
		}
		modelValue := input.Metadata[metadataID]
		modelValue.ID = normalizeConfigGuideAvailableModelID(modelValue.ID)
		if modelValue.ID == "" {
			modelValue.ID = strings.TrimSuffix(normalizedID, "-fast")
		}
		if _, exists := effective[normalizedID]; !exists {
			effective[normalizedID] = modelValue
		}
	}

	return effective, nil
}

func normalizeConfigGuideAvailableModelID(id string) string {
	id = strings.TrimSpace(id)
	id = strings.TrimSuffix(id, "-Sys")
	return id
}

func requireConfigGuideEffectiveModels(c *gin.Context, client configGuideClient) (configGuideQueryParams, map[string]service.OpenCodeOpenAIModel, bool) {
	params, ok := configGuideQuery(c)
	if !ok {
		return configGuideQueryParams{}, nil, false
	}
	token, ok := loadConfigGuideTokenByPublicKey(c, params.apiKey)
	if !ok {
		return configGuideQueryParams{}, nil, false
	}
	user, _, ok := validateConfigGuideTokenUsability(c, token)
	if !ok {
		return configGuideQueryParams{}, nil, false
	}
	metadata, ok := requireConfigGuideOpenAIModels(c)
	if !ok {
		return configGuideQueryParams{}, nil, false
	}
	effective, err := buildConfigGuideEffectiveModels(configGuideEffectiveModelsInput{
		Client:          client,
		Metadata:        metadata,
		AvailableModels: availableConfigGuideModelsForToken(token, user),
	})
	if err != nil {
		writeConfigGuideError(c, http.StatusServiceUnavailable, "OpenAI model metadata incomplete")
		return configGuideQueryParams{}, nil, false
	}
	return params, effective, true
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

func renderConfigGuideOMPModels(baseURL string, apiKey string, models map[string]service.OpenCodeOpenAIModel) (string, error) {
	ids := make([]string, 0, len(models))
	for id := range models {
		id = normalizeConfigGuideAvailableModelID(id)
		if id == "" || strings.HasSuffix(id, "-fast") {
			continue
		}
		ids = append(ids, id)
	}
	sort.Strings(ids)

	selected := make([]string, 0, len(ids))
	for _, id := range ids {
		model, ok := models[id]
		if !ok {
			continue
		}
		model.ID = id
		selected = append(selected, renderConfigGuideOMPModelYAML(normalizeConfigGuideOMPModel(model), "      ", nil))
	}
	if len(selected) == 0 {
		return "", fmt.Errorf("required OMP models missing")
	}

	return fmt.Sprintf(`providers:
  %s:
    api: openai-responses
    baseUrl: %s
    apiKey: %s
    models:
%s`, configGuideProviderID, configGuideYAMLDoubleQuotedScalar(baseURL), configGuideYAMLDoubleQuotedScalar(apiKey), strings.Join(selected, "\n")), nil
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

func renderConfigGuideOMPModelYAML(model configGuideOMPModel, indent string, extraLines []string) string {
	lines := []string{
		fmt.Sprintf("%s- id: %s", indent, configGuideYAMLDoubleQuotedScalar(model.ID)),
		fmt.Sprintf("%s  name: %s", indent, configGuideYAMLDoubleQuotedScalar(model.Name)),
		fmt.Sprintf("%s  api: %s", indent, configGuideYAMLDoubleQuotedScalar(model.API)),
		fmt.Sprintf("%s  reasoning: %t", indent, model.Reasoning),
		fmt.Sprintf("%s  input:", indent),
	}
	for _, item := range model.Input {
		lines = append(lines, fmt.Sprintf("%s    - %s", indent, configGuideYAMLDoubleQuotedScalar(item)))
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

func renderConfigGuideOMPSettings(models map[string]service.OpenCodeOpenAIModel) (string, error) {
	defaultModelID, smallModelID, err := selectConfigGuideRecommendedModels(models)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(`defaultThinkingLevel: xhigh
serviceTier: priority

modelRoles:
  default: %s/%s
  slow: %s/%s
  smol: %s/%s
  plan: %s/%s
  task: %s/%s:xhigh
  vision: %s/%s
  designer: %s/%s:xhigh
  commit: %s/%s:xhigh

task:
  agentModelOverrides:
    explore: %s/%s:xhigh
    librarian: %s/%s:xhigh
    reviewer: %s/%s:xhigh
    plan: %s/%s:xhigh`, configGuideProviderID, defaultModelID, configGuideProviderID, defaultModelID, configGuideProviderID, smallModelID, configGuideProviderID, defaultModelID, configGuideProviderID, defaultModelID, configGuideProviderID, defaultModelID, configGuideProviderID, defaultModelID, configGuideProviderID, defaultModelID, configGuideProviderID, smallModelID, configGuideProviderID, smallModelID, configGuideProviderID, defaultModelID, configGuideProviderID, defaultModelID), nil
}

func renderConfigGuideOpenCode(baseURL string, apiKey string, models map[string]service.OpenCodeOpenAIModel) ([]byte, error) {
	defaultModelID, smallModelID, err := selectConfigGuideRecommendedModels(models)
	if err != nil {
		return nil, err
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
		"model":       configGuideProviderID + "/" + defaultModelID,
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

func selectConfigGuideRecommendedModels(models map[string]service.OpenCodeOpenAIModel) (string, string, error) {
	defaultModelID := firstConfigGuideOpenCodeTextModel(models, []string{configGuideDefaultModelID, "gpt-5.4", "gpt-5.3-codex", "gpt-5.2-codex"})
	if defaultModelID == "" {
		return "", "", fmt.Errorf("default model missing")
	}
	smallModelID := firstConfigGuideOpenCodeTextModel(models, []string{configGuideSmallModelID, "gpt-5.5", "gpt-5.4", "gpt-5.3-codex-mini", "codex-mini-latest"})
	if smallModelID == "" {
		smallModelID = defaultModelID
	}
	return defaultModelID, smallModelID, nil
}

func firstConfigGuideOpenCodeTextModel(models map[string]service.OpenCodeOpenAIModel, candidates []string) string {
	for _, id := range candidates {
		if isConfigGuideOpenCodeTextModel(models[id]) {
			return id
		}
	}
	return ""
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
	out := sanitizeConfigGuideOpenCodeOptions(deepCloneStringAnyMap(options))
	out["store"] = false
	return out
}

func sanitizeConfigGuideOpenCodeOptions(options map[string]any) map[string]any {
	for key, value := range options {
		if configGuideOpenCodeOptionForbidden(key) {
			delete(options, key)
			continue
		}
		options[key] = sanitizeConfigGuideOpenCodeValue(value)
	}
	return options
}

func sanitizeConfigGuideOpenCodeValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return sanitizeConfigGuideOpenCodeOptions(typed)
	case []any:
		for i, item := range typed {
			typed[i] = sanitizeConfigGuideOpenCodeValue(item)
		}
		return typed
	default:
		return value
	}
}

func configGuideOpenCodeOptionForbidden(key string) bool {
	switch key {
	case "metadata", "builtin_tools", "web_search", "image_generation", "imageGeneration":
		return true
	default:
		return false
	}
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
		if unicode.IsControl(r) {
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
