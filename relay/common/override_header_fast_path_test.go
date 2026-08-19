package common

import (
	"bytes"
	"testing"
)

func headerOnlyOverrideForTest() map[string]interface{} {
	return map[string]interface{}{
		"operations": []interface{}{
			map[string]interface{}{
				"mode":  "pass_headers",
				"value": []interface{}{"Session_id", "X-Codex-Beta-Features"},
			},
			map[string]interface{}{
				"mode":  "set_header",
				"path":  "X-Static",
				"value": "enabled",
			},
		},
	}
}

func TestApplyParamOverrideHeaderOnlyReturnsOriginalBackingArray(t *testing.T) {
	input := append([]byte(`{"model":"gpt-5.4","input":"`), bytes.Repeat([]byte("x"), 1<<20)...)
	input = append(input, []byte(`"}`)...)
	ctx := map[string]interface{}{
		paramOverrideContextRequestHeaders: map[string]interface{}{
			"session_id": "sess-123",
		},
	}

	out, err := ApplyParamOverride(input, headerOnlyOverrideForTest(), ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != len(input) || &out[0] != &input[0] {
		t.Fatalf("header-only override copied JSON payload: input=%p output=%p", &input[0], &out[0])
	}
	if !bytes.Equal(out, input) {
		t.Fatal("header-only override changed JSON payload")
	}
	headers, ok := ctx[paramOverrideContextHeaderOverride].(map[string]interface{})
	if !ok {
		t.Fatal("missing header override context")
	}
	if headers["session_id"] != "sess-123" || headers["x-static"] != "enabled" {
		t.Fatalf("unexpected header overrides: %#v", headers)
	}
}

func TestApplyParamOverrideConditionalHeaderDoesNotUseHeaderOnlyFastPath(t *testing.T) {
	input := []byte(`{"model":"gpt-5.4"}`)
	override := map[string]interface{}{
		"operations": []interface{}{
			map[string]interface{}{
				"mode":  "set_header",
				"path":  "X-Conditional",
				"value": "enabled",
				"conditions": []interface{}{
					map[string]interface{}{
						"path":  "model",
						"mode":  "full",
						"value": "gpt-5.4",
					},
				},
			},
		},
	}
	ctx := map[string]interface{}{}

	out, err := ApplyParamOverride(input, override, ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out, input) {
		t.Fatal("conditional header override changed JSON payload")
	}
	headers, ok := ctx[paramOverrideContextHeaderOverride].(map[string]interface{})
	if !ok || headers["x-conditional"] != "enabled" {
		t.Fatalf("conditional header override was not applied: %#v", headers)
	}
}

func TestApplyParamOverrideSyncFieldsStillMutatesJSON(t *testing.T) {
	input := []byte(`{"model":"gpt-5.4"}`)
	override := map[string]interface{}{
		"operations": []interface{}{
			map[string]interface{}{
				"mode": "sync_fields",
				"from": "header:session_id",
				"to":   "json:prompt_cache_key",
			},
		},
	}
	ctx := map[string]interface{}{
		paramOverrideContextRequestHeaders: map[string]interface{}{
			"session_id": "sess-123",
		},
	}

	out, err := ApplyParamOverride(input, override, ctx)
	if err != nil {
		t.Fatal(err)
	}
	assertJSONEqual(t, `{"model":"gpt-5.4","prompt_cache_key":"sess-123"}`, string(out))
}

func BenchmarkApplyParamOverrideHeaderOnlyLargePayload(b *testing.B) {
	input := append([]byte(`{"model":"gpt-5.4","input":"`), bytes.Repeat([]byte("x"), 1<<20)...)
	input = append(input, []byte(`"}`)...)
	override := headerOnlyOverrideForTest()
	ctx := map[string]interface{}{
		paramOverrideContextRequestHeaders: map[string]interface{}{
			"session_id": "sess-123",
		},
	}

	b.ReportAllocs()
	b.SetBytes(int64(len(input)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		out, err := ApplyParamOverride(input, override, ctx)
		if err != nil {
			b.Fatal(err)
		}
		if len(out) == 0 || &out[0] != &input[0] {
			b.Fatal("header-only override copied JSON payload")
		}
	}
}
