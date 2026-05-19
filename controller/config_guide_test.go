package controller

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"
	"gopkg.in/yaml.v3"
)

type stubOpenCodeMetadataProvider struct {
	models map[string]service.OpenCodeOpenAIModel
	plugin service.OMPProviderToolsMetadata
	err    error
}

func (p stubOpenCodeMetadataProvider) GetOpenAIModels(context.Context) (map[string]service.OpenCodeOpenAIModel, error) {
	return p.models, p.err
}

func (p stubOpenCodeMetadataProvider) GetOMPProviderToolsMetadata(context.Context) service.OMPProviderToolsMetadata {
	return p.plugin
}

func withStubOpenCodeMetadataProvider(t *testing.T, provider openCodeMetadataProvider) {
	SetOpenCodeMetadataProviderForTest(t, provider)
}

func configGuideTestModels() map[string]service.OpenCodeOpenAIModel {
	return map[string]service.OpenCodeOpenAIModel{
		"gpt-5": {
			ID:               "gpt-5",
			Name:             "GPT-5",
			Attachment:       true,
			Reasoning:        true,
			ToolCall:         true,
			StructuredOutput: true,
			Modalities:       service.OpenCodeOpenAIModelModalities{Input: []string{"text", "image"}, Output: []string{"text"}},
			Cost:             service.OpenCodeOpenAIModelCost{Input: 5, Output: 30, CacheRead: 0.5},
			Limit:            service.OpenCodeOpenAIModelLimit{Context: 400000, Input: 272000, Output: 128000},
			ReleaseDate:      "2026-01-01",
		},
		"gpt-5-fast": {
			ID:               "gpt-5-fast",
			Name:             "GPT-5 Fast",
			Attachment:       true,
			Reasoning:        true,
			ToolCall:         true,
			StructuredOutput: true,
			Modalities:       service.OpenCodeOpenAIModelModalities{Input: []string{"text", "image"}, Output: []string{"text"}},
			Limit:            service.OpenCodeOpenAIModelLimit{Context: 400000, Input: 272000, Output: 128000},
			Cost:             service.OpenCodeOpenAIModelCost{Input: 5, Output: 30},
			Options:          map[string]any{"serviceTier": "priority"},
		},
		"gpt-5-text-incomplete": {
			ID:               "gpt-5-text-incomplete",
			Name:             "GPT-5 Text Incomplete",
			Attachment:       true,
			Reasoning:        true,
			ToolCall:         true,
			StructuredOutput: true,
			Modalities:       service.OpenCodeOpenAIModelModalities{Input: []string{"text"}, Output: []string{"text"}},
			Cost:             service.OpenCodeOpenAIModelCost{Input: 1},
			Limit:            service.OpenCodeOpenAIModelLimit{Input: 128000},
		},
		"gpt-5-image-only": {
			ID:               "gpt-5-image-only",
			Name:             "GPT-5 Image Only",
			Attachment:       true,
			Reasoning:        false,
			ToolCall:         false,
			StructuredOutput: false,
			Modalities:       service.OpenCodeOpenAIModelModalities{Input: []string{"text"}, Output: []string{"image"}},
			Cost:             service.OpenCodeOpenAIModelCost{Input: 1},
			Limit:            service.OpenCodeOpenAIModelLimit{Input: 128000},
		},
		"gpt-5-mini": {
			ID:               "gpt-5-mini",
			Name:             "GPT-5 mini",
			Attachment:       true,
			Reasoning:        true,
			ToolCall:         true,
			StructuredOutput: true,
			Modalities:       service.OpenCodeOpenAIModelModalities{Input: []string{"text"}, Output: []string{"text"}},
			Cost:             service.OpenCodeOpenAIModelCost{Input: 0.75, Output: 4.5, CacheRead: 0.075},
			Limit:            service.OpenCodeOpenAIModelLimit{Context: 272000, Input: 272000, Output: 128000},
		},
	}
}

func TestOpenCodeConfigGuideManifestReturnsJSONNotWebFallback(t *testing.T) {
	withStubOpenCodeMetadataProvider(t, stubOpenCodeMetadataProvider{
		models: configGuideTestModels(),
		plugin: service.OMPProviderToolsMetadata{Package: "omp-openai-provider-tools", LatestVersion: "9.9.9", Status: "ok"},
	})

	ctx, recorder := newAuthenticatedContext(t, http.MethodGet, "/config-guides/opencode-openai/manifest.json?api_key=sk-sk-live&base_url=https://api.example.com/v1", nil, 1)
	GetOpenCodeConfigGuideManifest(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected manifest status 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if contentType := recorder.Header().Get("Content-Type"); contentType != configGuideJSONContentType {
		t.Fatalf("expected JSON content-type, got %q", contentType)
	}
	var manifest configGuideManifest
	if err := common.Unmarshal(recorder.Body.Bytes(), &manifest); err != nil {
		t.Fatalf("manifest should be JSON, got %q: %v", recorder.Body.String(), err)
	}
	if manifest.Client != "opencode" || manifest.BaseURL != "https://api.example.com/v1" || len(manifest.Items) != 1 {
		t.Fatalf("unexpected manifest: %#v", manifest)
	}
	if manifest.Items[0].URL != "/config-guides/opencode-openai/opencode.json?api_key=sk-live&base_url=https%3A%2F%2Fapi.example.com%2Fv1" {
		t.Fatalf("expected normalized item URL, got %q", manifest.Items[0].URL)
	}
}

func TestOpenCodeConfigGuideJSONReturnsRenderableConfig(t *testing.T) {
	withStubOpenCodeMetadataProvider(t, stubOpenCodeMetadataProvider{models: configGuideTestModels()})

	ctx, recorder := newAuthenticatedContext(t, http.MethodGet, "/config-guides/opencode-openai/opencode.json?api_key=live&base_url=https://api.example.com/v1", nil, 1)
	GetOpenCodeConfigGuideJSON(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected opencode config status 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var config map[string]any
	if err := common.Unmarshal(recorder.Body.Bytes(), &config); err != nil {
		t.Fatalf("OpenCode config should be JSON, got %q: %v", recorder.Body.String(), err)
	}
	provider := config["provider"].(map[string]any)["new-api"].(map[string]any)
	if provider["npm"] != "@ai-sdk/openai" {
		t.Fatalf("OpenCode provider must use OpenAI SDK responses support, got %#v", provider["npm"])
	}
	options := provider["options"].(map[string]any)
	if options["apiKey"] != "sk-live" || options["baseURL"] != "https://api.example.com/v1" {
		t.Fatalf("unexpected provider options: %#v", options)
	}
	models := provider["models"].(map[string]any)
	for modelID := range models {
		if strings.HasSuffix(modelID, "-Sys") {
			t.Fatalf("OpenCode config must not generate unmapped -Sys model aliases, got keys %#v", models)
		}
	}
	fast := models["gpt-5-fast"].(map[string]any)
	gpt5 := models["gpt-5"].(map[string]any)
	if options, ok := gpt5["options"].(map[string]any); !ok || options["metadata"] != nil || options["store"] != false {
		t.Fatalf("OpenCode model options should only disable store and must not inject provider-native tools: %#v", gpt5["options"])
	}
	if variants := gpt5["variants"].(map[string]any); variants["image"] != nil {
		t.Fatalf("OpenCode model variants must not inject image_generation: %#v", variants["image"])
	}
	if _, ok := fast["structured_output"]; ok {
		t.Fatalf("OpenCode schema does not accept structured_output: %#v", fast)
	}
	if _, ok := models["gpt-5-text-incomplete"]; ok {
		t.Fatalf("text models with incomplete cost/limit must not be emitted")
	}
	if _, ok := models["gpt-5-image-only"]; ok {
		t.Fatalf("non-text-output OpenCode models must not be emitted")
	}
	if config["model"] != "new-api/gpt-5" {
		t.Fatalf("expected OpenCode default model to target new-api, got %#v", config["model"])
	}
	if config["small_model"] != "new-api/gpt-5-mini" {
		t.Fatalf("expected OpenCode small model to target a real new-api model, got %#v", config["small_model"])
	}
	agents := config["agent"].(map[string]any)
	if imageAgent := agents["image"]; imageAgent != nil {
		t.Fatalf("OpenCode config must not add an image agent that relies on provider-native image_generation: %#v", imageAgent)
	}
}

func TestOpenCodeConfigGuideJSONFallsBackWhenSmallModelMissing(t *testing.T) {
	models := configGuideTestModels()
	delete(models, "gpt-5-mini")
	withStubOpenCodeMetadataProvider(t, stubOpenCodeMetadataProvider{models: models})

	ctx, recorder := newAuthenticatedContext(t, http.MethodGet, "/config-guides/opencode-openai/opencode.json?api_key=live&base_url=https://api.example.com/v1", nil, 1)
	GetOpenCodeConfigGuideJSON(ctx)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected opencode config status 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var config map[string]any
	if err := common.Unmarshal(recorder.Body.Bytes(), &config); err != nil {
		t.Fatalf("OpenCode config should be JSON, got %q: %v", recorder.Body.String(), err)
	}
	provider := config["provider"].(map[string]any)["new-api"].(map[string]any)
	generatedModels := provider["models"].(map[string]any)
	smallModel := strings.TrimPrefix(config["small_model"].(string), "new-api/")
	if _, ok := generatedModels[smallModel]; !ok {
		t.Fatalf("small_model should reference a generated model, got %q with keys %#v", smallModel, generatedModels)
	}
}

func TestOMPConfigGuideManifestAndFiles(t *testing.T) {
	withStubOpenCodeMetadataProvider(t, stubOpenCodeMetadataProvider{
		models: configGuideTestModels(),
		plugin: service.OMPProviderToolsMetadata{Package: "omp-openai-provider-tools", LatestVersion: "9.9.9", Status: "ok"},
	})

	ctx, recorder := newAuthenticatedContext(t, http.MethodGet, "/config-guides/omp-openai/manifest.json?api_key=live&base_url=https://api.example.com/v1", nil, 1)
	GetOMPConfigGuideManifest(ctx)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected OMP manifest status 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var manifest configGuideManifest
	if err := common.Unmarshal(recorder.Body.Bytes(), &manifest); err != nil {
		t.Fatalf("OMP manifest should be JSON: %v", err)
	}
	if manifest.Client != "omp" || len(manifest.Items) != 4 {
		t.Fatalf("unexpected OMP manifest: %#v", manifest)
	}

	ctx, recorder = newAuthenticatedContext(t, http.MethodGet, "/config-guides/omp-openai/models.yml?api_key=live&base_url=https://api.example.com/v1", nil, 1)
	GetOMPConfigGuideModels(ctx)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected OMP models status 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	if !strings.Contains(body, "omp plugin install npm:omp-openai-provider-tools@9.9.9") || !strings.Contains(body, "new-api-image:") || !strings.Contains(body, "apiKey: sk-live") {
		t.Fatalf("unexpected OMP models.yml: %s", body)
	}
	if !strings.Contains(body, "    compat:\n      openaiProviderTools:\n        enabled: true") {
		t.Fatalf("expected provider-level provider tools compat, got: %s", body)
	}
	if !strings.Contains(body, "        compat:\n          openaiProviderTools:\n            imageGeneration: true") {
		t.Fatalf("expected image model to opt in to provider-native image generation, got: %s", body)
	}
}

func TestOMPConfigGuideModelsQuotesYAMLScalars(t *testing.T) {
	models := configGuideTestModels()
	defaultModel := models["gpt-5"]
	defaultModel.Name = "OpenAI: GPT-5"
	models["gpt-5"] = defaultModel
	miniModel := models["gpt-5-mini"]
	miniModel.Name = "OpenAI: GPT-5 Mini"
	models["gpt-5-mini"] = miniModel
	withStubOpenCodeMetadataProvider(t, stubOpenCodeMetadataProvider{
		models: models,
		plugin: service.OMPProviderToolsMetadata{Package: "omp-openai-provider-tools", LatestVersion: "9.9.9", Status: "ok"},
	})

	ctx, recorder := newAuthenticatedContext(t, http.MethodGet, "/config-guides/omp-openai/models.yml?api_key=live&base_url=https://api.example.com/v1", nil, 1)
	GetOMPConfigGuideModels(ctx)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected OMP models status 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	if !strings.Contains(body, `name: "OpenAI: GPT-5"`) {
		t.Fatalf("expected YAML string scalar to be quoted, got %s", body)
	}
	var parsed map[string]any
	if err := yaml.Unmarshal([]byte(body), &parsed); err != nil {
		t.Fatalf("expected generated models.yml to parse as YAML: %v\n%s", err, body)
	}
}

func TestConfigGuideDerivesBaseURLFromRequest(t *testing.T) {
	withStubOpenCodeMetadataProvider(t, stubOpenCodeMetadataProvider{models: configGuideTestModels()})

	ctx, recorder := newAuthenticatedContext(t, http.MethodGet, "/config-guides/opencode-openai/manifest.json?api_key=live", nil, 1)
	ctx.Request.Host = "gateway.example.com"
	ctx.Request.Header.Set("X-Forwarded-Proto", "https")
	GetOpenCodeConfigGuideManifest(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected derived manifest status 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var manifest configGuideManifest
	if err := common.Unmarshal(recorder.Body.Bytes(), &manifest); err != nil {
		t.Fatalf("manifest should be JSON: %v", err)
	}
	if manifest.BaseURL != "https://gateway.example.com/v1" {
		t.Fatalf("expected derived base URL, got %q", manifest.BaseURL)
	}
	if strings.Contains(manifest.Items[0].URL, "base_url=") {
		t.Fatalf("derived manifest item URLs should not redundantly carry base_url, got %q", manifest.Items[0].URL)
	}
}

func TestConfigGuideRejectsInvalidQuery(t *testing.T) {
	withStubOpenCodeMetadataProvider(t, stubOpenCodeMetadataProvider{models: configGuideTestModels()})

	cases := []string{
		"/config-guides/opencode-openai/manifest.json?base_url=https://api.example.com/v1",
		"/config-guides/opencode-openai/manifest.json?api_key=live&base_url=https://user:pass@example.com/v1",
		"/config-guides/opencode-openai/manifest.json?api_key=live&base_url=https://api.example.com/v1?x=1",
		"/config-guides/opencode-openai/manifest.json?api_key=live&base_url=ftp://api.example.com/v1",
	}
	for _, target := range cases {
		ctx, recorder := newAuthenticatedContext(t, http.MethodGet, target, nil, 1)
		GetOpenCodeConfigGuideManifest(ctx)
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for %s, got %d: %s", target, recorder.Code, recorder.Body.String())
		}
	}
}
