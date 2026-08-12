package dto

import (
	"encoding/json"
	"reflect"
	"testing"

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
