package dto

import (
	"bytes"
	"encoding/json"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func populatedResponsesRequestForWriterTest(t *testing.T) OpenAIResponsesRequest {
	t.Helper()
	request := OpenAIResponsesRequest{}
	value := reflect.ValueOf(&request).Elem()
	for index := 0; index < value.NumField(); index++ {
		field := value.Field(index)
		switch field.Kind() {
		case reflect.String:
			field.SetString("value<&\u2028")
		case reflect.Slice:
			field.SetBytes([]byte(` { "nested" : "<>&\u2028\u2029", "items" : [ 1, 2 ] } `))
		case reflect.Pointer:
			field.Set(reflect.New(field.Type().Elem()))
		default:
			t.Fatalf("unsupported OpenAIResponsesRequest field %s (%s)", value.Type().Field(index).Name, field.Kind())
		}
	}
	return request
}

func TestOpenAIResponsesRequestWriteJSONMatchesMarshalBytes(t *testing.T) {
	request := populatedResponsesRequestForWriterTest(t)
	want, err := common.Marshal(request)
	require.NoError(t, err)
	var output bytes.Buffer

	err = request.WriteJSON(&output)

	require.NoError(t, err)
	require.Equal(t, want, output.Bytes())
}

func TestOpenAIResponsesRequestWriteJSONMatchesRawMessageEncoding(t *testing.T) {
	values := []json.RawMessage{
		json.RawMessage(` [ 1, { "nested" : true } ] `),
		json.RawMessage(`"<>&\u001f"`),
		json.RawMessage("\"line\u2028separator\""),
		json.RawMessage("\"line\u2029separator\""),
		json.RawMessage(`123.45e-6`),
		json.RawMessage(`true`),
		json.RawMessage(`null`),
	}
	for _, raw := range values {
		request := OpenAIResponsesRequest{Model: "gpt-5", Input: raw}
		want, err := common.Marshal(request)
		require.NoError(t, err)
		var output bytes.Buffer

		err = request.WriteJSON(&output)

		require.NoError(t, err)
		require.Equal(t, want, output.Bytes(), "raw=%q", raw)
	}
}

func TestOpenAIResponsesRequestWriteJSONPreservesOmitEmpty(t *testing.T) {
	request := OpenAIResponsesRequest{}
	want, err := common.Marshal(request)
	require.NoError(t, err)
	var output bytes.Buffer

	err = request.WriteJSON(&output)

	require.NoError(t, err)
	require.Equal(t, want, output.Bytes())
}

func TestOpenAIResponsesRequestWriteJSONRejectsInvalidRawMessage(t *testing.T) {
	request := OpenAIResponsesRequest{Model: "gpt-5", Input: json.RawMessage(`{"broken":`)}
	_, marshalErr := common.Marshal(request)
	require.Error(t, marshalErr)
	var output bytes.Buffer

	writeErr := request.WriteJSON(&output)

	require.Error(t, writeErr)
}

func BenchmarkOpenAIResponsesRequestWriteJSON(b *testing.B) {
	input := append([]byte(`[{"role":"user","content":"`), []byte(strings.Repeat("x", 1<<20))...)
	input = append(input, []byte(`"}]`)...)
	request := OpenAIResponsesRequest{Model: "gpt-5.4", Input: input}
	b.Run("marshal", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(input)))
		for b.Loop() {
			encoded, err := common.Marshal(request)
			if err != nil {
				b.Fatal(err)
			}
			_, _ = io.Discard.Write(encoded)
		}
	})
	b.Run("stream", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(input)))
		for b.Loop() {
			if err := request.WriteJSON(io.Discard); err != nil {
				b.Fatal(err)
			}
		}
	})
}
