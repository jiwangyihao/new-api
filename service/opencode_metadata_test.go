package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"
)

func TestOpenCodeMetadataServiceExtractsModelsAndProviderTools(t *testing.T) {
	modelsPayload := `{
		"openai": {
			"models": {
				"gpt-5.5": {
					"id": "gpt-5.5",
					"name": "GPT-5.5",
					"attachment": true,
					"reasoning": true,
					"tool_call": true,
					"structured_output": true,
					"temperature": false,
					"release_date": "2026-03-05",
					"modalities": {"input": ["text", "image"], "output": ["text"]},
					"cost": {"input": 5, "output": 30, "cache_read": 0.5},
					"limit": {"context": 1050000, "input": 922000, "output": 128000},
					"experimental": {
						"modes": {
							"fast": {
								"provider": {
									"body": {"service_tier": "priority"},
									"headers": {"x-test": "fast"}
								}
							}
						}
					}
				},
				"gpt-5-chat-latest": {
					"id": "gpt-5-chat-latest",
					"name": "GPT-5 Chat",
					"attachment": true,
					"reasoning": false,
					"tool_call": true,
					"structured_output": true,
					"temperature": true
				}
			}
		}
	}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/models":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(modelsPayload))
		case "/npm":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"version":"9.9.9"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	svc := &OpenCodeMetadataService{
		client:       server.Client(),
		url:          server.URL + "/models",
		ttl:          time.Minute,
		npmLatestURL: server.URL + "/npm",
		npmTTL:       time.Minute,
	}
	models, err := svc.GetOpenAIModels(context.Background())
	if err != nil {
		t.Fatalf("expected metadata fetch to succeed: %v", err)
	}
	if _, ok := models["gpt-5-chat-latest"]; ok {
		t.Fatalf("deprecated chat alias should be filtered out")
	}
	fast := models["gpt-5.5-fast"]
	if fast.ID != "gpt-5.5-fast" {
		t.Fatalf("expected experimental fast model, got %#v", fast)
	}
	if fast.Options["serviceTier"] != "priority" {
		t.Fatalf("expected provider body to be camel-cased, got %#v", fast.Options)
	}
	if fast.Headers["x-test"] != "fast" {
		t.Fatalf("expected experimental headers, got %#v", fast.Headers)
	}
	if models["gpt-5.5"].Limit.Input != 272000 {
		t.Fatalf("expected GPT-5.5 codex oauth limit override, got %#v", models["gpt-5.5"].Limit)
	}

	plugin := svc.GetOMPProviderToolsMetadata(context.Background())
	if plugin.Status != "ok" || plugin.LatestVersion != "9.9.9" {
		t.Fatalf("expected provider tools metadata, got %#v", plugin)
	}
}

func TestOpenCodeMetadataServiceIgnoresNonOpenAIProviderCatalog(t *testing.T) {
	modelsPayload := `{
		"helicone": {
			"models": {
				"gpt-5-5": {
					"id": "gpt-5-5",
					"name": "GPT-5.5",
					"family": "gpt",
					"attachment": true,
					"reasoning": true,
					"tool_call": true,
					"structured_output": true,
					"temperature": false
				}
			}
		},
		"digitalocean": {
			"models": {
				"openai-gpt-5.5": {
					"id": "openai-gpt-5.5",
					"name": "GPT-5.5",
					"family": "gpt",
					"attachment": true,
					"reasoning": true,
					"tool_call": true,
					"structured_output": true,
					"temperature": false
				}
			}
		},
		"openai": {
			"models": {
				"gpt-5.4": {"id": "gpt-5.4", "name": "GPT-5.4"}
			}
		}
	}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(modelsPayload))
	}))
	defer server.Close()

	svc := &OpenCodeMetadataService{client: server.Client(), url: server.URL, ttl: time.Minute}
	models, err := svc.GetOpenAIModels(context.Background())
	if err != nil {
		t.Fatalf("expected provider catalog fetch to succeed: %v", err)
	}
	if _, ok := models["gpt-5-5"]; ok {
		t.Fatalf("non-openai provider model gpt-5-5 must not be imported")
	}
	if _, ok := models["openai-gpt-5.5"]; ok {
		t.Fatalf("non-openai provider model openai-gpt-5.5 must not be imported")
	}
	if _, ok := models["gpt-5.4"]; !ok {
		t.Fatalf("expected built-in openai provider model to survive filtering")
	}
}

func TestFilterOpenCodeOpenAIModelsForCodexOAuth(t *testing.T) {
	models := map[string]OpenCodeOpenAIModel{
		"gpt-4o":             {ID: "gpt-4o", Name: "GPT-4o"},
		"gpt-4o-fast":        {ID: "gpt-4o-fast", Name: "GPT-4o Fast"},
		"gpt-5.3":            {ID: "gpt-5.3", Name: "GPT-5.3"},
		"gpt-5.3-fast":       {ID: "gpt-5.3-fast", Name: "GPT-5.3 Fast"},
		"gpt-5.4":            {ID: "gpt-5.4", Name: "GPT-5.4"},
		"gpt-5.4-fast":       {ID: "gpt-5.4-fast", Name: "GPT-5.4 Fast"},
		"gpt-5.4-mini":       {ID: "gpt-5.4-mini", Name: "GPT-5.4 Mini"},
		"gpt-5.5":            {ID: "gpt-5.5", Name: "GPT-5.5"},
		"gpt-5.5-fast":       {ID: "gpt-5.5-fast", Name: "GPT-5.5 Fast"},
		"gpt-5.10":           {ID: "gpt-5.10", Name: "GPT-5.10"},
		"gpt-5.2":            {ID: "gpt-5.2", Name: "GPT-5.2"},
		"gpt-5.1-codex":      {ID: "gpt-5.1-codex", Name: "GPT-5.1 Codex"},
		"gpt-5.3-codex":      {ID: "gpt-5.3-codex", Name: "GPT-5.3 Codex"},
		"codex-mini-latest":  {ID: "codex-mini-latest", Name: "Codex Mini"},
		"gpt-5.2-codex":      {ID: "gpt-5.2-codex", Name: "GPT-5.2 Codex"},
		"gpt-5.1-codex-max":  {ID: "gpt-5.1 Codex Max", Name: "GPT-5.1 Codex Max"},
		"gpt-5.1-codex-mini": {ID: "gpt-5.1 Codex Mini", Name: "GPT-5.1 Codex Mini"},
	}

	filtered := filterOpenCodeOpenAIModelsForCodexOAuth(models)

	for _, id := range []string{"gpt-5.4", "gpt-5.4-fast", "gpt-5.4-mini", "gpt-5.5", "gpt-5.5-fast", "gpt-5.10", "gpt-5.2", "gpt-5.1-codex", "gpt-5.3-codex", "codex-mini-latest"} {
		if _, ok := filtered[id]; !ok {
			t.Fatalf("expected %s to survive filtering, got %#v", id, filtered)
		}
	}
	for _, id := range []string{"gpt-4o", "gpt-4o-fast", "gpt-5.3", "gpt-5.3-fast"} {
		if _, ok := filtered[id]; ok {
			t.Fatalf("expected %s to be filtered out, got %#v", id, filtered)
		}
	}
}

func requireOpenCodeModelOptions(t *testing.T, model OpenCodeOpenAIModel) map[string]any {
	t.Helper()
	field := reflect.ValueOf(model).FieldByName("Options")
	if !field.IsValid() {
		t.Fatalf("OpenCodeOpenAIModel.Options should exist")
	}
	options, ok := field.Interface().(map[string]any)
	if !ok {
		t.Fatalf("OpenCodeOpenAIModel.Options should be map[string]any")
	}
	return options
}

func requireOpenCodeModelHeaders(t *testing.T, model OpenCodeOpenAIModel) map[string]string {
	t.Helper()
	field := reflect.ValueOf(model).FieldByName("Headers")
	if !field.IsValid() {
		t.Fatalf("OpenCodeOpenAIModel.Headers should exist")
	}
	headers, ok := field.Interface().(map[string]string)
	if !ok {
		t.Fatalf("OpenCodeOpenAIModel.Headers should be map[string]string")
	}
	return headers
}
