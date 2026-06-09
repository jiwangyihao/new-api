package controller

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
	"gorm.io/gorm"
)

type stubOpenCodeMetadataProvider struct {
	models                 map[string]service.OpenCodeOpenAIModel
	plugin                 service.OMPProviderToolsMetadata
	err                    error
	failOnOMPProviderTools bool
}

func (p stubOpenCodeMetadataProvider) GetOpenAIModels(context.Context) (map[string]service.OpenCodeOpenAIModel, error) {
	return p.models, p.err
}

func (p stubOpenCodeMetadataProvider) GetOMPProviderToolsMetadata(context.Context) service.OMPProviderToolsMetadata {
	if p.failOnOMPProviderTools {
		panic("OMP provider-tools metadata must not be called")
	}
	return p.plugin
}

func withStubOpenCodeMetadataProvider(t *testing.T, provider openCodeMetadataProvider) {
	SetOpenCodeMetadataProviderForTest(t, provider)
}

func setupConfigGuideTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := setupTokenControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Ability{}))

	originalGroupRatio := ratio_setting.GroupRatio2JSONString()
	originalModelRatio := ratio_setting.ModelRatio2JSONString()
	originalUserUsableGroups := setting.UserUsableGroups2JSONString()
	originalAutoGroups := setting.AutoGroups2JsonString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(originalGroupRatio))
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(originalModelRatio))
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(originalUserUsableGroups))
		require.NoError(t, setting.UpdateAutoGroupsByJsonString(originalAutoGroups))
	})
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"paid":1}`))
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"gpt-5.5":1,"gpt-5.4-mini":1,"gpt-5.5-fast":1}`))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Default","paid":"Paid"}`))
	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["default","paid"]`))
	return db
}

func seedConfigGuideUser(t *testing.T, db *gorm.DB, id int, group string, status int) *model.User {
	t.Helper()
	user := &model.User{Id: id, Username: fmt.Sprintf("cg-user-%d", id), Password: "password123", Group: group, Status: status, AffCode: fmt.Sprintf("cg-aff-%d", id)}
	require.NoError(t, db.Create(user).Error)
	return user
}

func seedConfigGuideToken(t *testing.T, db *gorm.DB, userID int, rawKey string, status int, expiredTime int64, group string, unlimited bool, modelLimits string, allowIps *string) *model.Token {
	t.Helper()
	token := &model.Token{UserId: userID, Name: rawKey, Key: rawKey, Status: status, CreatedTime: 1, AccessedTime: 1, ExpiredTime: expiredTime, RemainQuota: 100, UnlimitedQuota: unlimited, Group: group, ModelLimits: modelLimits, ModelLimitsEnabled: modelLimits != "", AllowIps: allowIps}
	require.NoError(t, db.Create(token).Error)
	return token
}

func seedConfigGuideAbility(t *testing.T, db *gorm.DB, group string, modelName string) {
	t.Helper()
	require.NoError(t, db.Create(&model.Ability{Group: group, Model: modelName, ChannelId: len(modelName) + len(group), Enabled: true}).Error)
}

func seedValidConfigGuideFixture(t *testing.T) *gorm.DB {
	t.Helper()
	db := setupConfigGuideTestDB(t)
	seedConfigGuideUser(t, db, 10, "default", common.UserStatusEnabled)
	seedConfigGuideToken(t, db, 10, "livetoken", common.TokenStatusEnabled, -1, "default", true, "", nil)
	seedConfigGuideAbility(t, db, "default", "gpt-5.5")
	seedConfigGuideAbility(t, db, "default", "gpt-5.4-mini")
	return db
}
func configGuideTestModels() map[string]service.OpenCodeOpenAIModel {
	return map[string]service.OpenCodeOpenAIModel{
		"gpt-5.5": {
			ID:               "gpt-5.5",
			Name:             "GPT-5.5",
			Attachment:       true,
			Reasoning:        true,
			ToolCall:         true,
			StructuredOutput: true,
			Modalities:       service.OpenCodeOpenAIModelModalities{Input: []string{"text", "image"}, Output: []string{"text"}},
			Cost:             service.OpenCodeOpenAIModelCost{Input: 5, Output: 30, CacheRead: 0.5},
			Limit:            service.OpenCodeOpenAIModelLimit{Context: 400000, Input: 272000, Output: 128000},
			ReleaseDate:      "2026-01-01",
			Options: map[string]any{
				"metadata":        map[string]any{"builtin_tools": map[string]any{"web_search": true}},
				"builtin_tools":   map[string]any{"image_generation": true},
				"web_search":      true,
				"imageGeneration": true,
				"serviceTier":     "priority",
				"allowedArray": []any{
					map[string]any{"metadata": map[string]any{"builtin_tools": map[string]any{"web_search": true}}},
					map[string]any{"serviceTier": "priority"},
				},
			},
		},
		"gpt-5.5-fast": {
			ID:               "gpt-5.5-fast",
			Name:             "GPT-5.5 Fast",
			Attachment:       true,
			Reasoning:        true,
			ToolCall:         true,
			StructuredOutput: true,
			Modalities:       service.OpenCodeOpenAIModelModalities{Input: []string{"text", "image"}, Output: []string{"text"}},
			Limit:            service.OpenCodeOpenAIModelLimit{Context: 400000, Input: 272000, Output: 128000},
			Cost:             service.OpenCodeOpenAIModelCost{Input: 5, Output: 30},
			Options:          map[string]any{"serviceTier": "priority"},
		},
		"gpt-5.5-text-incomplete": {
			ID:               "gpt-5.5-text-incomplete",
			Name:             "GPT-5.5 Text Incomplete",
			Attachment:       true,
			Reasoning:        true,
			ToolCall:         true,
			StructuredOutput: true,
			Modalities:       service.OpenCodeOpenAIModelModalities{Input: []string{"text"}, Output: []string{"text"}},
			Cost:             service.OpenCodeOpenAIModelCost{Input: 1},
			Limit:            service.OpenCodeOpenAIModelLimit{Input: 128000},
		},
		"gpt-5.5-image-only": {
			ID:               "gpt-5.5-image-only",
			Name:             "GPT-5.5 Image Only",
			Attachment:       true,
			Reasoning:        false,
			ToolCall:         false,
			StructuredOutput: false,
			Modalities:       service.OpenCodeOpenAIModelModalities{Input: []string{"text"}, Output: []string{"image"}},
			Cost:             service.OpenCodeOpenAIModelCost{Input: 1},
			Limit:            service.OpenCodeOpenAIModelLimit{Input: 128000},
		},
		"gpt-5.4-mini": {
			ID:               "gpt-5.4-mini",
			Name:             "GPT-5.4 Mini",
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
	seedValidConfigGuideFixture(t)

	ctx, recorder := newAuthenticatedContext(t, http.MethodGet, "/config-guides/opencode-openai/manifest.json?api_key=sk-livetoken&base_url=https://api.example.com/v1", nil, 1)
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
	if manifest.Items[0].URL != "/config-guides/opencode-openai/opencode.json?api_key=sk-livetoken&base_url=https%3A%2F%2Fapi.example.com%2Fv1" {
		t.Fatalf("expected normalized item URL, got %q", manifest.Items[0].URL)
	}
}

func TestOpenCodeConfigGuideJSONReturnsRenderableConfig(t *testing.T) {
	withStubOpenCodeMetadataProvider(t, stubOpenCodeMetadataProvider{models: configGuideTestModels()})
	seedValidConfigGuideFixture(t)

	ctx, recorder := newAuthenticatedContext(t, http.MethodGet, "/config-guides/opencode-openai/opencode.json?api_key=sk-livetoken&base_url=https://api.example.com/v1", nil, 1)
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
	if options["apiKey"] != "sk-livetoken" || options["baseURL"] != "https://api.example.com/v1" {
		t.Fatalf("unexpected provider options: %#v", options)
	}
	models := provider["models"].(map[string]any)
	for modelID := range models {
		if strings.HasSuffix(modelID, "-Sys") {
			t.Fatalf("OpenCode config must not generate unmapped -Sys model aliases, got keys %#v", models)
		}
	}
	fast := models["gpt-5.5-fast"].(map[string]any)
	gpt5 := models["gpt-5.5"].(map[string]any)
	if options, ok := gpt5["options"].(map[string]any); !ok || options["metadata"] != nil || options["store"] != false {
		t.Fatalf("OpenCode model options should only disable store and must not inject provider-native tools: %#v", gpt5["options"])
	}
	if variants := gpt5["variants"].(map[string]any); variants["image"] != nil {
		t.Fatalf("OpenCode model variants must not inject image_generation: %#v", variants["image"])
	}
	if _, ok := fast["structured_output"]; ok {
		t.Fatalf("OpenCode schema does not accept structured_output: %#v", fast)
	}
	if _, ok := models["gpt-5.5-text-incomplete"]; ok {
		t.Fatalf("text models with incomplete cost/limit must not be emitted")
	}
	if _, ok := models["gpt-5.5-image-only"]; ok {
		t.Fatalf("non-text-output OpenCode models must not be emitted")
	}
	if config["model"] != "new-api/gpt-5.5" {
		t.Fatalf("expected OpenCode default model to target new-api, got %#v", config["model"])
	}
	if config["small_model"] != "new-api/gpt-5.4-mini" {
		t.Fatalf("expected OpenCode small model to target a real new-api model, got %#v", config["small_model"])
	}
	agents := config["agent"].(map[string]any)
	if imageAgent := agents["image"]; imageAgent != nil {
		t.Fatalf("OpenCode config must not add an image agent that relies on provider-native image_generation: %#v", imageAgent)
	}
}

func TestConfigGuideOpenCodeCodexProAddsIntentHeaderOnlyToModels(t *testing.T) {
	withStubOpenCodeMetadataProvider(t, stubOpenCodeMetadataProvider{models: configGuideTestModels()})
	seedValidConfigGuideFixture(t)

	ctx, recorder := newAuthenticatedContext(t, http.MethodGet, "/config-guides/opencode-openai/opencode.json?api_key=sk-livetoken&base_url=https://api.example.com/v1", nil, 1)
	GetOpenCodeConfigGuideJSON(ctx)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	var config map[string]any
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &config))
	provider := config["provider"].(map[string]any)["new-api"].(map[string]any)
	require.Equal(t, "@ai-sdk/openai", provider["npm"])
	require.Equal(t, "new-api", provider["name"])
	options := provider["options"].(map[string]any)
	require.Equal(t, "https://api.example.com/v1", options["baseURL"])
	require.Equal(t, "sk-livetoken", options["apiKey"])
	require.NotContains(t, options, "headers")

	models := provider["models"].(map[string]any)
	require.NotEmpty(t, models)
	for modelID, rawModel := range models {
		modelConfig := rawModel.(map[string]any)
		headers, ok := modelConfig["headers"].(map[string]any)
		require.True(t, ok, "model %s must carry Codex Pro intent header", modelID)
		require.Equal(t, map[string]any{
			"X-NewAPI-Codex-Pro-Intent": "codex-pro",
		}, headers)
		require.NotContains(t, headers, "X-NewAPI-Pro-Request")
		require.NotContains(t, headers, "X-NewAPI-Pro-Served")
	}
}

func TestConfigGuideOMPCodexProAddsHeadersWithoutChangingProviderShape(t *testing.T) {
	withStubOpenCodeMetadataProvider(t, stubOpenCodeMetadataProvider{models: configGuideTestModels()})
	seedValidConfigGuideFixture(t)

	ctx, recorder := newAuthenticatedContext(t, http.MethodGet, "/config-guides/omp-openai/models.yml?api_key=sk-livetoken&base_url=https://api.example.com/v1", nil, 1)
	GetOMPConfigGuideModels(ctx)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	body := recorder.Body.String()
	require.Contains(t, body, "providers:")
	require.Contains(t, body, "new-api:")
	require.Contains(t, body, "api: openai-responses")
	require.Contains(t, body, `baseUrl: "https://api.example.com/v1"`)
	require.Contains(t, body, `apiKey: "sk-livetoken"`)
	require.Contains(t, body, "headers:")
	require.Contains(t, body, "X-NewAPI-Codex-Pro-Intent")
	require.Contains(t, body, `"codex-pro"`)
	require.NotContains(t, body, "X-NewAPI-Pro-Request")
	require.NotContains(t, body, "X-NewAPI-Pro-Served")

	var parsed map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(body), &parsed), body)
	providers := parsed["providers"].(map[string]any)
	provider := providers["new-api"].(map[string]any)
	require.Equal(t, "openai-responses", provider["api"])
	require.Equal(t, "https://api.example.com/v1", provider["baseUrl"])
	require.Equal(t, "sk-livetoken", provider["apiKey"])
	require.Equal(t, map[string]any{
		"X-NewAPI-Codex-Pro-Intent": "codex-pro",
	}, provider["headers"])
}

func TestOpenCodeConfigGuideJSONDoesNotEmitProviderNativeTools(t *testing.T) {
	withStubOpenCodeMetadataProvider(t, stubOpenCodeMetadataProvider{models: configGuideTestModels()})
	seedValidConfigGuideFixture(t)

	ctx, recorder := newAuthenticatedContext(t, http.MethodGet, "/config-guides/opencode-openai/opencode.json?api_key=sk-livetoken&base_url=https://api.example.com/v1", nil, 0)
	GetOpenCodeConfigGuideJSON(ctx)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	body := recorder.Body.String()
	for _, forbidden := range []string{"builtin_tools", "web_search", "image_generation", "imageGeneration", "metadata", "agent.image", "structured_output"} {
		require.NotContains(t, body, forbidden)
	}
	require.Contains(t, body, `"store":false`)
	require.Contains(t, body, `"serviceTier":"priority"`)
}

func TestOpenCodeConfigGuideJSONFallsBackToDefaultForSmallModel(t *testing.T) {
	models := configGuideTestModels()
	delete(models, "gpt-5.4-mini")
	withStubOpenCodeMetadataProvider(t, stubOpenCodeMetadataProvider{models: models})
	seedValidConfigGuideFixture(t)

	ctx, recorder := newAuthenticatedContext(t, http.MethodGet, "/config-guides/opencode-openai/opencode.json?api_key=sk-livetoken&base_url=https://api.example.com/v1", nil, 1)
	GetOpenCodeConfigGuideJSON(ctx)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	var config map[string]any
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &config))
	require.Equal(t, "new-api/gpt-5.5", config["small_model"])
}

func TestOpenCodeConfigGuideJSONDoesNotRequireDeprecatedGPT5(t *testing.T) {
	models := map[string]service.OpenCodeOpenAIModel{
		"gpt-5.5": {
			ID:               "gpt-5.5",
			Name:             "GPT-5.5",
			Attachment:       true,
			Reasoning:        true,
			ToolCall:         true,
			StructuredOutput: true,
			Modalities:       service.OpenCodeOpenAIModelModalities{Input: []string{"text", "image"}, Output: []string{"text"}},
			Cost:             service.OpenCodeOpenAIModelCost{Input: 6, Output: 36, CacheRead: 0.6},
			Limit:            service.OpenCodeOpenAIModelLimit{Context: 400000, Input: 272000, Output: 128000},
		},
		"gpt-5.4-mini": {
			ID:               "gpt-5.4-mini",
			Name:             "GPT-5.4 Mini",
			Attachment:       true,
			Reasoning:        true,
			ToolCall:         true,
			StructuredOutput: true,
			Modalities:       service.OpenCodeOpenAIModelModalities{Input: []string{"text"}, Output: []string{"text"}},
			Cost:             service.OpenCodeOpenAIModelCost{Input: 0.75, Output: 4.5, CacheRead: 0.075},
			Limit:            service.OpenCodeOpenAIModelLimit{Context: 272000, Input: 272000, Output: 128000},
		},
	}
	withStubOpenCodeMetadataProvider(t, stubOpenCodeMetadataProvider{models: models})
	db := setupConfigGuideTestDB(t)
	seedConfigGuideUser(t, db, 10, "default", common.UserStatusEnabled)
	seedConfigGuideToken(t, db, 10, "livetoken", common.TokenStatusEnabled, -1, "default", true, "", nil)
	seedConfigGuideAbility(t, db, "default", "gpt-5.5")
	seedConfigGuideAbility(t, db, "default", "gpt-5.4-mini")

	ctx, recorder := newAuthenticatedContext(t, http.MethodGet, "/config-guides/opencode-openai/opencode.json?api_key=sk-livetoken&base_url=https://api.example.com/v1", nil, 0)
	GetOpenCodeConfigGuideJSON(ctx)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	var config map[string]any
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &config))
	require.Equal(t, "new-api/gpt-5.5", config["model"])
	require.Equal(t, "new-api/gpt-5.4-mini", config["small_model"])
}

func TestOMPConfigGuideManifestAndFiles(t *testing.T) {
	withStubOpenCodeMetadataProvider(t, stubOpenCodeMetadataProvider{models: configGuideTestModels()})
	seedValidConfigGuideFixture(t)

	ctx, recorder := newAuthenticatedContext(t, http.MethodGet, "/config-guides/omp-openai/manifest.json?api_key=sk-livetoken&base_url=https://api.example.com/v1", nil, 1)
	GetOMPConfigGuideManifest(ctx)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected OMP manifest status 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var manifest configGuideManifest
	if err := common.Unmarshal(recorder.Body.Bytes(), &manifest); err != nil {
		t.Fatalf("OMP manifest should be JSON: %v", err)
	}
	if manifest.Client != "omp" || len(manifest.Items) != 2 {
		t.Fatalf("unexpected OMP manifest: %#v", manifest)
	}
	require.Equal(t, "models", manifest.Items[0].ID)
	require.Equal(t, "config", manifest.Items[1].ID)

	ctx, recorder = newAuthenticatedContext(t, http.MethodGet, "/config-guides/omp-openai/models.yml?api_key=sk-livetoken&base_url=https://api.example.com/v1", nil, 1)
	GetOMPConfigGuideModels(ctx)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected OMP models status 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	for _, expected := range []string{"providers:", "new-api:", `apiKey: "sk-livetoken"`, "gpt-5.5", "gpt-5.4-mini"} {
		require.Contains(t, body, expected)
	}
	for _, forbidden := range []string{"omp-openai-provider-tools", "new-api-image", "openaiProviderTools", "imageGeneration", "-Sys"} {
		require.NotContains(t, body, forbidden)
	}

	ctx, recorder = newAuthenticatedContext(t, http.MethodGet, "/config-guides/omp-openai/config.yml?api_key=sk-livetoken&base_url=https://api.example.com/v1", nil, 1)
	GetOMPConfigGuideConfig(ctx)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.NotContains(t, recorder.Body.String(), "-Sys")
}

func TestOMPConfigGuideDoesNotRequireProviderToolsMetadata(t *testing.T) {
	withStubOpenCodeMetadataProvider(t, stubOpenCodeMetadataProvider{models: configGuideTestModels(), failOnOMPProviderTools: true})
	seedValidConfigGuideFixture(t)

	targets := []string{
		"/config-guides/omp-openai/manifest.json?api_key=sk-livetoken&base_url=https://api.example.com/v1",
		"/config-guides/omp-openai/models.yml?api_key=sk-livetoken&base_url=https://api.example.com/v1",
		"/config-guides/omp-openai/config.yml?api_key=sk-livetoken&base_url=https://api.example.com/v1",
	}
	for _, target := range targets {
		ctx, recorder := newAuthenticatedContext(t, http.MethodGet, target, nil, 0)
		switch {
		case strings.Contains(target, "manifest.json"):
			GetOMPConfigGuideManifest(ctx)
		case strings.Contains(target, "models.yml"):
			GetOMPConfigGuideModels(ctx)
		default:
			GetOMPConfigGuideConfig(ctx)
		}
		require.Equal(t, http.StatusOK, recorder.Code, target+recorder.Body.String())
		for _, forbidden := range []string{"plugin", "image-generator", "openaiProviderTools", "new-api-image", "imageGeneration", "-Sys"} {
			require.NotContains(t, recorder.Body.String(), forbidden)
		}
	}
}

func TestOMPConfigGuideModelsQuotesYAMLScalars(t *testing.T) {
	models := configGuideTestModels()
	defaultModel := models["gpt-5.5"]
	defaultModel.Name = "OpenAI: GPT-5.5"
	models["gpt-5.5"] = defaultModel
	miniModel := models["gpt-5.4-mini"]
	miniModel.Name = "OpenAI: GPT-5.5 Mini"
	models["gpt-5.4-mini"] = miniModel
	withStubOpenCodeMetadataProvider(t, stubOpenCodeMetadataProvider{
		models: models,
		plugin: service.OMPProviderToolsMetadata{Package: "omp-openai-provider-tools", LatestVersion: "9.9.9", Status: "ok"},
	})
	seedValidConfigGuideFixture(t)

	ctx, recorder := newAuthenticatedContext(t, http.MethodGet, "/config-guides/omp-openai/models.yml?api_key=sk-livetoken&base_url=https://api.example.com/v1", nil, 1)
	GetOMPConfigGuideModels(ctx)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected OMP models status 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	if !strings.Contains(body, `name: "OpenAI: GPT-5.5"`) {
		t.Fatalf("expected YAML string scalar to be quoted, got %s", body)
	}
	var parsed map[string]any
	if err := yaml.Unmarshal([]byte(body), &parsed); err != nil {
		t.Fatalf("expected generated models.yml to parse as YAML: %v\n%s", err, body)
	}
}

func TestOMPConfigGuideModelsQuotesDynamicYAMLScalars(t *testing.T) {
	withStubOpenCodeMetadataProvider(t, stubOpenCodeMetadataProvider{models: configGuideTestModels()})
	seedValidConfigGuideFixture(t)

	ctx, recorder := newAuthenticatedContext(t, http.MethodGet, "/config-guides/omp-openai/models.yml?api_key=sk-livetoken&base_url=https%3A%2F%2Fapi.example.com%2Fv1%3A+bad", nil, 1)
	GetOMPConfigGuideModels(ctx)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	body := recorder.Body.String()
	require.Contains(t, body, `baseUrl: "https://api.example.com/v1: bad"`)
	require.Contains(t, body, `apiKey: "sk-livetoken"`)

	var parsed map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(body), &parsed), body)
}

func TestConfigGuideDerivesBaseURLFromRequest(t *testing.T) {
	withStubOpenCodeMetadataProvider(t, stubOpenCodeMetadataProvider{models: configGuideTestModels()})
	seedValidConfigGuideFixture(t)

	ctx, recorder := newAuthenticatedContext(t, http.MethodGet, "/config-guides/opencode-openai/manifest.json?api_key=sk-livetoken", nil, 1)
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
		"/config-guides/opencode-openai/manifest.json?api_key=sk-livetoken&base_url=https://user:pass@example.com/v1",
		"/config-guides/opencode-openai/manifest.json?api_key=sk-livetoken&base_url=https://api.example.com/v1?x=1",
		"/config-guides/opencode-openai/manifest.json?api_key=sk-livetoken&base_url=ftp://api.example.com/v1",
	}
	for _, target := range cases {
		ctx, recorder := newAuthenticatedContext(t, http.MethodGet, target, nil, 1)
		GetOpenCodeConfigGuideManifest(ctx)
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for %s, got %d: %s", target, recorder.Code, recorder.Body.String())
		}
	}
}

func TestConfigGuidePublicAPIKeyValidation(t *testing.T) {
	cases := []struct {
		name        string
		tokenStatus int
		userStatus  int
		expiredTime int64
		group       string
		remainQuota int
		allowIps    *string
		target      string
		wantStatus  int
	}{
		{name: "missing", target: "/config-guides/opencode-openai/manifest.json", wantStatus: http.StatusBadRequest},
		{name: "unknown", target: "/config-guides/opencode-openai/manifest.json?api_key=sk-missing", wantStatus: http.StatusUnauthorized},
		{name: "disabled", tokenStatus: common.TokenStatusDisabled, userStatus: common.UserStatusEnabled, expiredTime: -1, group: "default", target: "/config-guides/opencode-openai/manifest.json?api_key=sk-livetoken", wantStatus: http.StatusForbidden},
		{name: "expired status", tokenStatus: common.TokenStatusExpired, userStatus: common.UserStatusEnabled, expiredTime: -1, group: "default", target: "/config-guides/opencode-openai/manifest.json?api_key=sk-livetoken", wantStatus: http.StatusForbidden},
		{name: "expired time", tokenStatus: common.TokenStatusEnabled, userStatus: common.UserStatusEnabled, expiredTime: 1, group: "default", target: "/config-guides/opencode-openai/manifest.json?api_key=sk-livetoken", wantStatus: http.StatusForbidden},
		{name: "user disabled", tokenStatus: common.TokenStatusEnabled, userStatus: common.UserStatusDisabled, expiredTime: -1, group: "default", target: "/config-guides/opencode-openai/manifest.json?api_key=sk-livetoken", wantStatus: http.StatusForbidden},
		{name: "deprecated token group", tokenStatus: common.TokenStatusEnabled, userStatus: common.UserStatusEnabled, expiredTime: -1, group: "gone", target: "/config-guides/opencode-openai/manifest.json?api_key=sk-livetoken", wantStatus: http.StatusOK},
		{name: "ip denied", tokenStatus: common.TokenStatusEnabled, userStatus: common.UserStatusEnabled, expiredTime: -1, group: "default", allowIps: common.GetPointer("10.0.0.0/8"), target: "/config-guides/opencode-openai/manifest.json?api_key=sk-livetoken", wantStatus: http.StatusForbidden},
		{name: "deprecated user group", tokenStatus: common.TokenStatusEnabled, userStatus: common.UserStatusEnabled, expiredTime: -1, group: "", target: "/config-guides/opencode-openai/manifest.json?api_key=sk-livetoken", wantStatus: http.StatusOK},
		{name: "exhausted", tokenStatus: common.TokenStatusExhausted, userStatus: common.UserStatusEnabled, expiredTime: -1, group: "default", target: "/config-guides/opencode-openai/manifest.json?api_key=sk-livetoken", wantStatus: http.StatusTooManyRequests},
		{name: "enabled zero quota ok", tokenStatus: common.TokenStatusEnabled, userStatus: common.UserStatusEnabled, expiredTime: -1, group: "default", remainQuota: 0, target: "/config-guides/opencode-openai/manifest.json?api_key=sk-livetoken", wantStatus: http.StatusOK},
		{name: "suffix key accepted like TokenAuth", tokenStatus: common.TokenStatusEnabled, userStatus: common.UserStatusEnabled, expiredTime: -1, group: "default", target: "/config-guides/opencode-openai/manifest.json?api_key=sk-livetoken-extra-suffix", wantStatus: http.StatusOK},
		{name: "control character", target: "/config-guides/opencode-openai/manifest.json?api_key=sk-live%0A-token", wantStatus: http.StatusBadRequest},
		{name: "control character around key", tokenStatus: common.TokenStatusEnabled, userStatus: common.UserStatusEnabled, expiredTime: -1, group: "default", target: "/config-guides/opencode-openai/manifest.json?api_key=%0Ask-livetoken%0A", wantStatus: http.StatusBadRequest},
		{name: "unicode control character around key", tokenStatus: common.TokenStatusEnabled, userStatus: common.UserStatusEnabled, expiredTime: -1, group: "default", target: "/config-guides/opencode-openai/manifest.json?api_key=%C2%85sk-livetoken%C2%85", wantStatus: http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := setupConfigGuideTestDB(t)
			if tc.tokenStatus != 0 {
				userGroup := "default"
				if tc.name == "deprecated user group" {
					userGroup = "gone"
				}
				seedConfigGuideUser(t, db, 10, userGroup, tc.userStatus)
				token := seedConfigGuideToken(t, db, 10, "livetoken", tc.tokenStatus, tc.expiredTime, tc.group, true, "", tc.allowIps)
				if tc.name == "enabled zero quota ok" {
					token.RemainQuota = tc.remainQuota
					require.NoError(t, db.Save(token).Error)
				}
				seedConfigGuideAbility(t, db, "default", "gpt-5.5")
				seedConfigGuideAbility(t, db, "default", "gpt-5.4-mini")
			}
			withStubOpenCodeMetadataProvider(t, stubOpenCodeMetadataProvider{models: configGuideTestModels()})
			ctx, recorder := newAuthenticatedContext(t, http.MethodGet, tc.target, nil, 0)
			GetOpenCodeConfigGuideManifest(ctx)
			require.Equal(t, tc.wantStatus, recorder.Code, recorder.Body.String())
			if recorder.Code != http.StatusOK {
				require.NotContains(t, recorder.Body.String(), "livetoken")
			}
		})
	}
}

func TestConfigGuideIgnoresUserTokenAndAbilityGroups(t *testing.T) {
	withStubOpenCodeMetadataProvider(t, stubOpenCodeMetadataProvider{models: configGuideTestModels()})
	db := setupConfigGuideTestDB(t)
	seedConfigGuideUser(t, db, 10, "gone", common.UserStatusEnabled)
	seedConfigGuideToken(t, db, 10, "livetoken", common.TokenStatusEnabled, -1, "gone", true, "", nil)
	seedConfigGuideAbility(t, db, "legacy", "gpt-5.5")
	seedConfigGuideAbility(t, db, "legacy", "gpt-5.4-mini")

	ctx, recorder := newAuthenticatedContext(t, http.MethodGet, "/config-guides/opencode-openai/opencode.json?api_key=sk-livetoken&base_url=https://api.example.com/v1", nil, 0)
	GetOpenCodeConfigGuideJSON(ctx)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Contains(t, recorder.Body.String(), "gpt-5.5")
	require.Contains(t, recorder.Body.String(), "gpt-5.4-mini")
}

func TestConfigGuidePublicAPIKeyUsesEffectiveModels(t *testing.T) {
	db := setupConfigGuideTestDB(t)
	seedConfigGuideUser(t, db, 10, "default", common.UserStatusEnabled)
	seedConfigGuideToken(t, db, 10, "livetoken", common.TokenStatusEnabled, -1, "default", true, "", nil)
	seedConfigGuideAbility(t, db, "default", "gpt-5.5")
	seedConfigGuideAbility(t, db, "default", "gpt-5.4-mini")
	seedConfigGuideAbility(t, db, "default", "gpt-5.5-Sys")
	seedConfigGuideAbility(t, db, "default", "not-in-metadata")
	withStubOpenCodeMetadataProvider(t, stubOpenCodeMetadataProvider{models: configGuideTestModels()})

	ctx, recorder := newAuthenticatedContext(t, http.MethodGet, "/config-guides/opencode-openai/opencode.json?api_key=sk-livetoken&base_url=https://api.example.com/v1", nil, 0)
	GetOpenCodeConfigGuideJSON(ctx)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Equal(t, "no-store", recorder.Header().Get("Cache-Control"))
	require.Equal(t, "no-cache", recorder.Header().Get("Pragma"))
	var cfg map[string]any
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &cfg))
	models := cfg["provider"].(map[string]any)["new-api"].(map[string]any)["models"].(map[string]any)
	require.Contains(t, models, "gpt-5.5")
	require.Contains(t, models, "gpt-5.4-mini")
	require.Contains(t, models, "gpt-5.5-fast")
	require.NotContains(t, models, "not-in-metadata")
	for id := range models {
		require.NotContains(t, id, "-Sys")
	}
}

func TestConfigGuideTokenModelLimitsNormalizeSysBeforeBillingFilter(t *testing.T) {
	db := setupConfigGuideTestDB(t)
	seedConfigGuideUser(t, db, 10, "default", common.UserStatusEnabled)
	seedConfigGuideToken(t, db, 10, "livetoken", common.TokenStatusEnabled, -1, "default", true, "gpt-5.5-Sys,gpt-5.4-mini", nil)
	withStubOpenCodeMetadataProvider(t, stubOpenCodeMetadataProvider{models: configGuideTestModels()})

	ctx, recorder := newAuthenticatedContext(t, http.MethodGet, "/config-guides/opencode-openai/opencode.json?api_key=sk-livetoken&base_url=https://api.example.com/v1", nil, 0)
	GetOpenCodeConfigGuideJSON(ctx)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.NotContains(t, recorder.Body.String(), "-Sys")
	require.Contains(t, recorder.Body.String(), "gpt-5.5")
}

func TestBuildConfigGuideEffectiveModels(t *testing.T) {
	baseModels := configGuideTestModels()
	cases := []struct {
		name            string
		client          configGuideClient
		availableModels []string
		metadata        map[string]service.OpenCodeOpenAIModel
		want            []string
		wantErr         bool
	}{
		{
			name:            "token model limits normalize sys and intersect",
			client:          configGuideClientOpenCode,
			availableModels: []string{" gpt-5.5 ", "gpt-5.4-mini", "gpt-5.5-Sys", "not-in-metadata"},
			metadata:        baseModels,
			want:            []string{"gpt-5.5", "gpt-5.5-fast", "gpt-5.4-mini"},
		},
		{
			name:            "auto group available list",
			client:          configGuideClientOpenCode,
			availableModels: []string{"gpt-5.4-mini", "gpt-5.5", "gpt-5.5", "not-in-metadata"},
			metadata:        baseModels,
			want:            []string{"gpt-5.5", "gpt-5.5-fast", "gpt-5.4-mini"},
		},
		{
			name:            "omp does not synthesize fast",
			client:          configGuideClientOMP,
			availableModels: []string{"gpt-5.5", "gpt-5.4-mini"},
			metadata:        baseModels,
			want:            []string{"gpt-5.5", "gpt-5.4-mini"},
		},
		{
			name:            "single recommended model still renders",
			client:          configGuideClientOpenCode,
			availableModels: []string{"gpt-5.5"},
			metadata:        baseModels,
			want:            []string{"gpt-5.5", "gpt-5.5-fast"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := buildConfigGuideEffectiveModels(configGuideEffectiveModelsInput{Client: tc.client, Metadata: tc.metadata, AvailableModels: tc.availableModels})
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			for _, id := range tc.want {
				require.Contains(t, got, id)
			}
			require.Len(t, got, len(tc.want))
		})
	}
}
