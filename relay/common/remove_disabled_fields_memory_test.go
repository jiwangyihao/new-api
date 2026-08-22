package common

import (
	"bytes"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/require"
)

func TestHasDisabledFieldsHandlesTopLevelAndNestedKeys(t *testing.T) {
	cases := []struct {
		name     string
		jsonData []byte
		settings dto.ChannelOtherSettings
		want     bool
	}{
		{name: "empty", jsonData: []byte{}, want: false},
		{name: "no_targets", jsonData: []byte(`{"model":"gpt-5","input":"hello"}`), want: false},
		{name: "top_level_target", jsonData: []byte(`{"service_tier":"flex","model":"gpt-5"}`), want: true},
		{name: "escaped_target", jsonData: []byte("{\"\\u0073ervice_tier\":\"flex\"}"), want: true},
		{name: "nested_non_target", jsonData: []byte(`{"metadata":{"service_tier":"flex"}}`), want: false},
		{name: "nested_stream_target", jsonData: []byte(`{"stream_options":{"include_obfuscation":false}}`), want: true},
		{name: "stream_options_scalar", jsonData: []byte(`{"stream_options":"include_obfuscation"}`), want: false},
		{name: "malformed", jsonData: []byte(`{"model":`), want: false},
		{name: "allow_all", jsonData: []byte(`{"service_tier":"flex","stream_options":{"include_obfuscation":false}}`), settings: dto.ChannelOtherSettings{AllowServiceTier: true, AllowIncludeObfuscation: true}, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, hasDisabledFields(tc.jsonData, tc.settings))
		})
	}
}

func TestRemoveDisabledFieldsPreservesFilteringSemantics(t *testing.T) {
	input := []byte(`{"model":"gpt-5","stream_options":{"include_obfuscation":false,"other":true}}`)

	output, err := RemoveDisabledFields(input, dto.ChannelOtherSettings{}, false)

	require.NoError(t, err)
	require.JSONEq(t, `{"model":"gpt-5","stream_options":{"other":true}}`, string(output))
}

func TestRemoveDisabledFieldsNoTargetReturnsOriginalBytes(t *testing.T) {
	input := []byte(`{"model":"gpt-5","input":[{"role":"user","content":"hello"}],"stream":true}`)

	output, err := RemoveDisabledFields(input, dto.ChannelOtherSettings{}, false)

	require.NoError(t, err)
	require.Equal(t, input, output)
	require.NotEmpty(t, output)
	require.Equal(t, &input[0], &output[0])
}

func TestRemoveDisabledFieldsIgnoresNestedTargetKeys(t *testing.T) {
	input := []byte(`{"metadata":{"service_tier":"flex","safety_identifier":"user"},"stream_options":{"other":true}}`)

	output, err := RemoveDisabledFields(input, dto.ChannelOtherSettings{}, false)

	require.NoError(t, err)
	require.Equal(t, input, output)
	require.Equal(t, &input[0], &output[0])
}

func TestRemoveDisabledFieldsDetectsEscapedTargetKey(t *testing.T) {
	input := []byte("{\"\\u0073ervice_tier\":\"flex\",\"keep\":true}")

	output, err := RemoveDisabledFields(input, dto.ChannelOtherSettings{}, false)

	require.NoError(t, err)
	require.JSONEq(t, `{"keep":true}`, string(output))
}

func BenchmarkHasDisabledFieldsLargeBody(b *testing.B) {
	input := []byte(`{"model":"gpt-5","input":"` + strings.Repeat("x", 1<<20) + `"}`)
	settings := dto.ChannelOtherSettings{}

	b.ReportAllocs()
	b.SetBytes(int64(len(input)))
	for i := 0; i < b.N; i++ {
		if hasDisabledFields(input, settings) {
			b.Fatal("unexpected target")
		}
	}
}

func BenchmarkRemoveDisabledFieldsLargeBodyWithoutTargets(b *testing.B) {
	input := []byte(`{"model":"gpt-5","input":"` + strings.Repeat("x", 1<<20) + `"}`)
	settings := dto.ChannelOtherSettings{}

	b.ReportAllocs()
	b.SetBytes(int64(len(input)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		output, err := RemoveDisabledFields(input, settings, false)
		if err != nil {
			b.Fatal(err)
		}
		if !bytes.Equal(output, input) {
			b.Fatal("body changed")
		}
	}
}
