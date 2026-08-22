package common

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestCleanupBodyStorageReleasesMemoryBackingArrayAndLegacyCache(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	payload := bytes.Repeat([]byte("x"), 1<<20)
	storage := newMemoryStorage(payload)
	c.Set(KeyBodyStorage, storage)
	c.Set(KeyRequestBody, payload)

	CleanupBodyStorage(c)

	stored, exists := c.Get(KeyBodyStorage)
	require.True(t, exists)
	require.Nil(t, stored)
	legacy, exists := c.Get(KeyRequestBody)
	require.True(t, exists)
	require.Nil(t, legacy)
	require.Nil(t, storage.data)
	require.Nil(t, storage.reader)
	require.Zero(t, storage.Size())
	_, err := storage.Bytes()
	require.ErrorIs(t, err, ErrStorageClosed)
}
