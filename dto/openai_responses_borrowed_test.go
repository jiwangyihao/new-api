package dto

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestUnmarshalOpenAIResponsesRequestBorrowedRawMessagesMatchesJSON(t *testing.T) {
	body := []byte(`{"model":"gpt-5.4","input":[{"role":"user","content":"hello"}],"instructions":"be concise","metadata":{"tenant":"a"},"tools":[{"type":"function","name":"lookup"}],"stream":false,"max_output_tokens":0}`)
	var standard OpenAIResponsesRequest
	require.NoError(t, common.Unmarshal(body, &standard))

	borrowed, err := UnmarshalOpenAIResponsesRequestBorrowed(body)

	require.NoError(t, err)
	require.Equal(t, standard, *borrowed)
	standardJSON, err := common.Marshal(&standard)
	require.NoError(t, err)
	borrowedJSON, err := common.Marshal(borrowed)
	require.NoError(t, err)
	require.JSONEq(t, string(standardJSON), string(borrowedJSON))
}

func TestUnmarshalOpenAIResponsesRequestBorrowedRawMessagesShareInput(t *testing.T) {
	body := []byte(`{"model":"gpt-5.4","input":[1,2,3],"metadata":{"tenant":"a"}}`)

	request, err := UnmarshalOpenAIResponsesRequestBorrowed(body)

	require.NoError(t, err)
	inputStart := bytes.Index(body, request.Input)
	metadataStart := bytes.Index(body, request.Metadata)
	require.GreaterOrEqual(t, inputStart, 0)
	require.GreaterOrEqual(t, metadataStart, 0)
	require.Equal(t, &body[inputStart], &request.Input[0])
	require.Equal(t, &body[metadataStart], &request.Metadata[0])
}

func TestUnmarshalOpenAIResponsesRequestBorrowedRejectsInvalidJSON(t *testing.T) {
	_, err := UnmarshalOpenAIResponsesRequestBorrowed([]byte(`{"model":"gpt-5.4","input":`))
	require.Error(t, err)
}

func TestUnmarshalOpenAIResponsesRequestBorrowedMatchesStandardEdgeCases(t *testing.T) {
	cases := []string{
		`{"MODEL":"gpt-5.4","input":null,"metadata":null,"stream":null}`,
		`{"model":"gpt-5.4","input":{"a":1},"input":{"b":2},"unknown":{"x":true}}`,
		`{"model":"gpt-5.4","input":[{"role":"user","content":"a"}],"tools":[],"prompt":"p"}`,
		`{"model":"gpt-5.4","input":true,"include":42,"text":[1,2],"user":"u"}`,
		`{"model":"gpt-5.4","input":"\\u4f60\\u597d","instructions":"x","previous_response_id":"r"}`,
	}
	for _, bodyText := range cases {
		t.Run(bodyText, func(t *testing.T) {
			body := []byte(bodyText)
			var standard OpenAIResponsesRequest
			require.NoError(t, common.Unmarshal(body, &standard))
			borrowed, err := UnmarshalOpenAIResponsesRequestBorrowed(body)
			require.NoError(t, err)
			require.Equal(t, standard, *borrowed)
			standardJSON, err := common.Marshal(&standard)
			require.NoError(t, err)
			borrowedJSON, err := common.Marshal(borrowed)
			require.NoError(t, err)
			require.JSONEq(t, string(standardJSON), string(borrowedJSON))
		})
	}
}

func TestUnmarshalOpenAIResponsesRequestBorrowedPreservesErrorOffset(t *testing.T) {
	body := []byte(`{"model":"gpt-5.4","input":[}`)
	var standard OpenAIResponsesRequest
	standardErr := common.Unmarshal(body, &standard)
	_, borrowedErr := UnmarshalOpenAIResponsesRequestBorrowed(body)
	require.Error(t, standardErr)
	require.Error(t, borrowedErr)
	require.Equal(t, standardErr.Error(), borrowedErr.Error())
}

var borrowedResponsesRequestSink *OpenAIResponsesRequest

func BenchmarkUnmarshalOpenAIResponsesRequest(b *testing.B) {
	body := []byte(`{"model":"gpt-5.4","input":"` + string(bytes.Repeat([]byte("x"), 1<<20)) + `","metadata":{"tenant":"a"},"stream":true}`)
	b.SetBytes(int64(len(body)))
	b.Run("standard", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			request := &OpenAIResponsesRequest{}
			if err := json.Unmarshal(body, request); err != nil {
				b.Fatal(err)
			}
			borrowedResponsesRequestSink = request
		}
	})
	b.Run("borrowed", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			request, err := UnmarshalOpenAIResponsesRequestBorrowed(body)
			if err != nil {
				b.Fatal(err)
			}
			borrowedResponsesRequestSink = request
		}
	})
}
