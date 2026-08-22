package channel

import (
	"io"
	"net/http"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/stretchr/testify/require"
)

func TestNewAPIRequestReplaysAndReleasesBody(t *testing.T) {
	owner := relaycommon.NewReleasableRequestBody([]byte("request-body"))
	body := owner.Reader()

	req, err := newAPIRequest(http.MethodPost, "https://example.com/v1/responses", body)
	require.NoError(t, err)
	require.Equal(t, int64(len("request-body")), req.ContentLength)
	require.NotNil(t, req.GetBody)

	replayed, err := req.GetBody()
	require.NoError(t, err)
	replayedData, err := io.ReadAll(replayed)
	require.NoError(t, err)
	require.Equal(t, "request-body", string(replayedData))
	require.NoError(t, replayed.Close())

	require.NoError(t, req.Body.Close())
	body.Release()
	_, err = req.GetBody()
	require.Error(t, err)
}
