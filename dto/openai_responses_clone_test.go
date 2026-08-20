package dto

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
)

func TestOpenAIResponsesRequestCloneDoesNotShareMutableFields(t *testing.T) {
	original := &OpenAIResponsesRequest{
		Model:           "gpt-5.5-high",
		Input:           json.RawMessage(`[{"role":"user","content":"hello"}]`),
		Instructions:    json.RawMessage(`"be concise"`),
		MaxOutputTokens: lo.ToPtr(uint(0)),
		Reasoning:       &Reasoning{Effort: "high", Summary: "auto"},
		Stream:          lo.ToPtr(false),
		StreamOptions:   &StreamOptions{IncludeUsage: true, IncludeObfuscation: true},
		Temperature:     lo.ToPtr(float64(0)),
		TopP:            lo.ToPtr(float64(0)),
	}

	cloned := original.Clone()
	require.NotSame(t, original, cloned)
	require.Equal(t, original, cloned)

	cloned.Input[0] = '{'
	cloned.Instructions[0] = '['
	*cloned.MaxOutputTokens = 8
	cloned.Reasoning.Effort = "low"
	*cloned.Stream = true
	cloned.StreamOptions.IncludeUsage = false
	*cloned.Temperature = 1
	*cloned.TopP = 1

	require.Equal(t, byte('['), original.Input[0])
	require.Equal(t, byte('"'), original.Instructions[0])
	require.Zero(t, *original.MaxOutputTokens)
	require.Equal(t, "high", original.Reasoning.Effort)
	require.False(t, *original.Stream)
	require.True(t, original.StreamOptions.IncludeUsage)
	require.Zero(t, *original.Temperature)
	require.Zero(t, *original.TopP)
}

func TestOpenAIResponsesRequestCloneCopiesEveryMutableField(t *testing.T) {
	original := &OpenAIResponsesRequest{}
	originalValue := reflect.ValueOf(original).Elem()
	requestType := originalValue.Type()
	for i := 0; i < originalValue.NumField(); i++ {
		field := originalValue.Field(i)
		switch field.Kind() {
		case reflect.Slice:
			value := reflect.MakeSlice(field.Type(), 1, 1)
			value.Index(0).SetUint(uint64(i + 1))
			field.Set(value)
		case reflect.Pointer:
			field.Set(reflect.New(field.Type().Elem()))
		}
	}

	cloned := original.Clone()
	require.Equal(t, original, cloned)
	clonedValue := reflect.ValueOf(cloned).Elem()
	for i := 0; i < originalValue.NumField(); i++ {
		originalField := originalValue.Field(i)
		clonedField := clonedValue.Field(i)
		switch originalField.Kind() {
		case reflect.Slice, reflect.Pointer:
			require.NotEqualf(t, originalField.Pointer(), clonedField.Pointer(), "%s shares mutable storage", requestType.Field(i).Name)
		}
	}
}

func TestOpenAIResponsesRequestClonePreservesNilAndExplicitZero(t *testing.T) {
	original := &OpenAIResponsesRequest{
		MaxOutputTokens: lo.ToPtr(uint(0)),
		MaxToolCalls:    lo.ToPtr(uint(0)),
		Stream:          lo.ToPtr(false),
		Temperature:     lo.ToPtr(float64(0)),
		TopP:            lo.ToPtr(float64(0)),
	}

	cloned := original.Clone()
	require.Nil(t, cloned.Reasoning)
	require.Nil(t, cloned.StreamOptions)
	require.NotNil(t, cloned.MaxOutputTokens)
	require.NotNil(t, cloned.MaxToolCalls)
	require.NotNil(t, cloned.Stream)
	require.NotNil(t, cloned.Temperature)
	require.NotNil(t, cloned.TopP)

	require.Zero(t, *cloned.MaxOutputTokens)
	require.Zero(t, *cloned.MaxToolCalls)
	require.False(t, *cloned.Stream)
	require.Zero(t, *cloned.Temperature)
	require.Zero(t, *cloned.TopP)
}

func TestOpenAIResponsesRequestCloneNil(t *testing.T) {
	var original *OpenAIResponsesRequest
	require.Nil(t, original.Clone())
}

func TestOpenAIResponsesRequestCloneForRelaySharesRawMessageBacking(t *testing.T) {
	original := &OpenAIResponsesRequest{
		Model:            "gpt-5.5-high",
		Input:            json.RawMessage(`[{"role":"user","content":"hello"}]`),
		Include:          json.RawMessage(`[{"type":"reasoning.encrypted_content"}]`),
		Instructions:     json.RawMessage(`"be concise"`),
		Metadata:         json.RawMessage(`{"tenant":"test"}`),
		PromptCacheKey:   json.RawMessage(`"cache-key"`),
		SafetyIdentifier: json.RawMessage(`"user-1"`),
		Tools:            json.RawMessage(`[{"type":"function"}]`),
		MaxOutputTokens:  lo.ToPtr(uint(0)),
		Reasoning:        &Reasoning{Effort: "high", Summary: "auto"},
		Stream:           lo.ToPtr(false),
		StreamOptions:    &StreamOptions{IncludeUsage: true, IncludeObfuscation: true},
		Temperature:      lo.ToPtr(float64(0)),
		TopP:             lo.ToPtr(float64(0)),
		MaxToolCalls:     lo.ToPtr(uint(0)),
	}

	cloned := original.CloneForRelay()
	require.NotSame(t, original, cloned)
	require.Equal(t, original, cloned)

	originalValue := reflect.ValueOf(original).Elem()
	clonedValue := reflect.ValueOf(cloned).Elem()
	for index := 0; index < originalValue.NumField(); index++ {
		originalField := originalValue.Field(index)
		clonedField := clonedValue.Field(index)
		if originalField.Kind() == reflect.Slice && !originalField.IsNil() {
			require.Equal(t, originalField.Pointer(), clonedField.Pointer(), "%s must share immutable raw bytes", originalValue.Type().Field(index).Name)
		}
		if originalField.Kind() == reflect.Pointer && !originalField.IsNil() {
			require.NotEqual(t, originalField.Pointer(), clonedField.Pointer(), "%s must copy mutable pointer", originalValue.Type().Field(index).Name)
		}
	}

	originalJSON, err := common.Marshal(original)
	require.NoError(t, err)
	clonedJSON, err := common.Marshal(cloned)
	require.NoError(t, err)
	require.Equal(t, originalJSON, clonedJSON)
}

var cloneForRelayBenchmarkSink *OpenAIResponsesRequest

func newCloneBenchmarkRequest() *OpenAIResponsesRequest {
	return &OpenAIResponsesRequest{
		Model:           "gpt-5.5-high",
		Input:           bytes.Repeat([]byte("x"), 1<<20),
		Instructions:    json.RawMessage(`"be concise"`),
		Metadata:        json.RawMessage(`{"tenant":"test"}`),
		Tools:           json.RawMessage(`[{"type":"function","name":"lookup"}]`),
		Reasoning:       &Reasoning{Effort: "high", Summary: "auto"},
		Stream:          lo.ToPtr(true),
		StreamOptions:   &StreamOptions{IncludeUsage: true},
		Temperature:     lo.ToPtr(float64(0.2)),
		TopP:            lo.ToPtr(float64(0.9)),
		MaxToolCalls:    lo.ToPtr(uint(4)),
		MaxOutputTokens: lo.ToPtr(uint(2048)),
	}
}

func BenchmarkOpenAIResponsesRequestClone(b *testing.B) {
	original := newCloneBenchmarkRequest()
	b.ReportAllocs()
	b.SetBytes(int64(len(original.Input)))
	for index := 0; index < b.N; index++ {
		cloneForRelayBenchmarkSink = original.Clone()
	}
}

func BenchmarkOpenAIResponsesRequestCloneForRelay(b *testing.B) {
	original := newCloneBenchmarkRequest()
	b.ReportAllocs()
	b.SetBytes(int64(len(original.Input)))
	for index := 0; index < b.N; index++ {
		cloneForRelayBenchmarkSink = original.CloneForRelay()
	}
}
