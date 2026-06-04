package aws

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/smithy-go/middleware"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestDoAwsClientRequest_AppliesRuntimeHeaderOverrideToAnthropicBeta(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	info := &relaycommon.RelayInfo{
		OriginModelName:           "claude-3-5-sonnet-20240620",
		IsStream:                  false,
		UseRuntimeHeadersOverride: true,
		RuntimeHeadersOverride: map[string]any{
			"anthropic-beta": "computer-use-2025-01-24",
		},
		ChannelMeta: &relaycommon.ChannelMeta{
			ApiKey:            "access-key|secret-key|us-east-1",
			UpstreamModelName: "claude-3-5-sonnet-20240620",
		},
	}

	requestBody := bytes.NewBufferString(`{"messages":[{"role":"user","content":"hello"}],"max_tokens":128}`)
	adaptor := &Adaptor{}

	_, err := doAwsClientRequest(ctx, info, adaptor, requestBody)
	require.NoError(t, err)

	awsReq, ok := adaptor.AwsReq.(*bedrockruntime.InvokeModelInput)
	require.True(t, ok)

	var payload map[string]any
	require.NoError(t, common.Unmarshal(awsReq.Body, &payload))

	anthropicBeta, exists := payload["anthropic_beta"]
	require.True(t, exists)

	values, ok := anthropicBeta.([]any)
	require.True(t, ok)
	require.Equal(t, []any{"computer-use-2025-01-24"}, values)
}

type captureHTTPClient struct {
	lastRequest *http.Request
}

func (c *captureHTTPClient) Do(req *http.Request) (*http.Response, error) {
	c.lastRequest = req.Clone(req.Context())
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
		Request:    req,
	}, nil
}

func addSpoofedSubscriptionMarkerMiddleware(o *bedrockruntime.Options) {
	o.APIOptions = append(o.APIOptions, func(stack *middleware.Stack) error {
		return stack.Finalize.Add(middleware.FinalizeMiddlewareFunc("spoofSubscriptionMarker", func(ctx context.Context, in middleware.FinalizeInput, next middleware.FinalizeHandler) (middleware.FinalizeOutput, middleware.Metadata, error) {
			if req, ok := in.Request.(*smithyhttp.Request); ok && req != nil {
				req.Header.Set(channel.SubscriptionMarkerHeaderName, "trial")
			}
			return next.HandleFinalize(ctx, in)
		}), middleware.Before)
	})
}

func newAwsClientWithSubscriptionMarker(t *testing.T, info *relaycommon.RelayInfo, httpClient bedrockruntime.HTTPClient) *bedrockruntime.Client {
	t.Helper()

	opts := bedrockruntime.Options{
		Region:       "us-east-1",
		Credentials:  credentials.NewStaticCredentialsProvider("akid", "secret", ""),
		HTTPClient:   httpClient,
		BaseEndpoint: aws.String("https://example.com"),
	}
	addSubscriptionMarkerMiddleware(info)(&opts)

	return bedrockruntime.New(opts)
}

func TestAwsClientRequestAppliesSubscriptionMarkerToInvokeModel(t *testing.T) {
	t.Parallel()

	capture := &captureHTTPClient{}
	client := newAwsClientWithSubscriptionMarker(t, &relaycommon.RelayInfo{SubscriptionTrialMarker: "trial"}, capture)

	_, err := client.InvokeModel(context.Background(), &bedrockruntime.InvokeModelInput{
		ModelId:     aws.String("anthropic.claude-3-sonnet-20240229-v1:0"),
		Accept:      aws.String("application/json"),
		ContentType: aws.String("application/json"),
		Body:        []byte(`{"inputText":"hello"}`),
	})
	require.NoError(t, err)
	require.NotNil(t, capture.lastRequest)
	require.Equal(t, "trial", capture.lastRequest.Header.Get(channel.SubscriptionMarkerHeaderName))
}

func TestAwsClientRequestRemovesSpoofedSubscriptionMarkerWhenNotTrial(t *testing.T) {
	t.Parallel()

	capture := &captureHTTPClient{}
	client := newAwsClientWithSubscriptionMarker(t, &relaycommon.RelayInfo{SubscriptionTrialMarker: "paid"}, capture)

	_, err := client.InvokeModel(context.Background(), &bedrockruntime.InvokeModelInput{
		ModelId:     aws.String("anthropic.claude-3-sonnet-20240229-v1:0"),
		Accept:      aws.String("application/json"),
		ContentType: aws.String("application/json"),
		Body:        []byte(`{"inputText":"hello"}`),
	}, addSpoofedSubscriptionMarkerMiddleware)
	require.NoError(t, err)
	require.NotNil(t, capture.lastRequest)
	require.Empty(t, capture.lastRequest.Header.Get(channel.SubscriptionMarkerHeaderName))
}
