package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestOpenCodeMetadataServiceExtractsModelsAndProviderTools(t *testing.T) {
	modelsPayload := `{
		"openai": {
			"models": {
				"gpt-5": {
					"id": "gpt-5",
					"name": "GPT-5",
					"attachment": true,
					"reasoning": true,
					"tool_call": true,
					"structured_output": true,
					"temperature": false,
					"release_date": "2026-01-01",
					"modalities": {"input": ["text", "image"], "output": ["text"]},
					"cost": {"input": 5, "output": 30, "cache_read": 0.5},
					"limit": {"context": 400000, "input": 272000, "output": 128000},
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
	fast := models["gpt-5-fast"]
	if fast.ID != "gpt-5-fast" {
		t.Fatalf("expected experimental fast model, got %#v", fast)
	}
	if fast.Options["serviceTier"] != "priority" {
		t.Fatalf("expected provider body to be camel-cased, got %#v", fast.Options)
	}
	if fast.Headers["x-test"] != "fast" {
		t.Fatalf("expected experimental headers, got %#v", fast.Headers)
	}
	if models["gpt-5"].Limit.Input != 272000 {
		t.Fatalf("expected GPT-5 limit override, got %#v", models["gpt-5"].Limit)
	}

	plugin := svc.GetOMPProviderToolsMetadata(context.Background())
	if plugin.Status != "ok" || plugin.LatestVersion != "9.9.9" {
		t.Fatalf("expected provider tools metadata, got %#v", plugin)
	}
}

func TestOpenCodeMetadataServiceExtractsModelsFromProviderCatalog(t *testing.T) {
	modelsPayload := `{
		"helicone": {
			"models": {
				"gpt-5": {
					"id": "gpt-5",
					"name": "OpenAI GPT-5",
					"family": "gpt",
					"attachment": true,
					"reasoning": true,
					"tool_call": true,
					"structured_output": true,
					"temperature": false,
					"release_date": "2026-01-01",
					"modalities": {"input": ["text", "image"], "output": ["text"]},
					"cost": {"input": 5, "output": 30, "cache_read": 0.5},
					"limit": {"context": 400000, "input": 272000, "output": 128000},
					"experimental": {"modes": {"fast": {"provider": {"body": {"service_tier": "priority"}}}}}
				},
				"gpt-4o": {
					"id": "gpt-4o",
					"name": "OpenAI GPT-4o",
					"family": "gpt",
					"attachment": true,
					"reasoning": false,
					"tool_call": true,
					"structured_output": true,
					"temperature": true,
					"modalities": {"input": ["text", "image"], "output": ["text"]},
					"limit": {"context": 128000, "output": 16384}
				},
				"claude-opus": {
					"id": "claude-opus",
					"name": "Claude Opus",
					"attachment": true,
					"reasoning": true,
					"tool_call": true,
					"structured_output": true,
					"temperature": true
				}
			}
		},
		"requesty": {
			"models": {
				"openai/gpt-5-mini": {
					"id": "openai/gpt-5-mini",
					"name": "OpenAI GPT-5 Mini",
					"family": "gpt",
					"attachment": true,
					"reasoning": true,
					"tool_call": true,
					"structured_output": true,
					"temperature": false,
					"modalities": {"input": ["text"], "output": ["text"]},
					"limit": {"context": 272000, "output": 128000}
				}
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
	if _, ok := models["claude-opus"]; ok {
		t.Fatalf("expected non-OpenAI provider model to be ignored")
	}
	if models["gpt-5-mini"].ID != "gpt-5-mini" {
		t.Fatalf("expected prefixed model id to be canonicalized, got %#v", models["gpt-5-mini"])
	}
	if models["gpt-5-fast"].Options["serviceTier"] != "priority" {
		t.Fatalf("expected experimental mode from provider catalog, got %#v", models["gpt-5-fast"])
	}
	if models["gpt-4o"].ID != "gpt-4o" {
		t.Fatalf("expected ordinary OpenAI model to survive filtering, got %#v", models["gpt-4o"])
	}
}
