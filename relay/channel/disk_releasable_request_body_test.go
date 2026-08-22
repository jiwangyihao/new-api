package channel

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/stretchr/testify/require"
)

func TestNewAPIRequestReplaysDiskBody(t *testing.T) {
	owner, err := relaycommon.NewDiskReleasableRequestBody([]byte("disk-request-body"))
	require.NoError(t, err)
	body, err := owner.Reader()
	require.NoError(t, err)

	req, err := newAPIRequest(http.MethodPost, "https://example.com/v1/responses", body)
	require.NoError(t, err)
	require.Equal(t, int64(len("disk-request-body")), req.ContentLength)
	require.NotNil(t, req.GetBody)

	primaryData, err := io.ReadAll(req.Body)
	require.NoError(t, err)
	replayed, err := req.GetBody()
	require.NoError(t, err)
	replayedData, err := io.ReadAll(replayed)
	require.NoError(t, err)
	require.Equal(t, primaryData, replayedData)
	require.NoError(t, replayed.Close())
	require.NoError(t, req.Body.Close())
	body.Release()
	_, err = req.GetBody()
	require.Error(t, err)
}

func TestHTTPClientRedirectReplaysDiskBody(t *testing.T) {
	const payload = "redirect-disk-request-body"
	received := make(chan string, 1)
	var targetURL string
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		received <- string(body)
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()
	targetURL = target.URL
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", targetURL)
		w.WriteHeader(http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()

	owner, err := relaycommon.NewDiskReleasableRequestBody([]byte(payload))
	require.NoError(t, err)
	body, err := owner.Reader()
	require.NoError(t, err)
	req, err := newAPIRequest(http.MethodPost, redirect.URL, body)
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, payload, <-received)
	owner.Release()
}
