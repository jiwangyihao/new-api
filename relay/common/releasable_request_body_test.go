package common

import (
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReleasableRequestBodyDropsBackingDataAfterReadersClose(t *testing.T) {
	body := NewReleasableRequestBody([]byte("request-body"))
	reader := body.Reader()
	replay, err := reader.GetBody()
	require.NoError(t, err)

	body.Release()
	require.NotNil(t, body.data)

	data, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.Equal(t, "request-body", string(data))
	require.NoError(t, reader.Close())
	require.NotNil(t, body.data)
	require.NoError(t, replay.Close())
	require.Nil(t, body.data)
	_, err = reader.GetBody()
	require.ErrorIs(t, err, errReleasableRequestBodyReleased)
}

func TestReleasableRequestBodyReleaseWithoutReaderDropsImmediately(t *testing.T) {
	body := NewReleasableRequestBody([]byte("request-body"))
	body.Release()
	require.Nil(t, body.data)
}
