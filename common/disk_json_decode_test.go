package common

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type diskJSONTestStorage struct {
	reader     *bytes.Reader
	data       []byte
	bytesCalls int
	seekCalls  int
}

func (s *diskJSONTestStorage) Read(p []byte) (int, error) {
	return s.reader.Read(p)
}

func (s *diskJSONTestStorage) Seek(offset int64, whence int) (int64, error) {
	s.seekCalls++
	return s.reader.Seek(offset, whence)
}

func (s *diskJSONTestStorage) Close() error { return nil }

func (s *diskJSONTestStorage) Bytes() ([]byte, error) {
	s.bytesCalls++
	return nil, errors.New("disk JSON path must not call Bytes")
}

func (s *diskJSONTestStorage) Size() int64 { return int64(len(s.data)) }

func (s *diskJSONTestStorage) IsDisk() bool { return true }

func newDiskJSONTestContext(storage BodyStorage) *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(KeyBodyStorage, storage)
	return c
}

func TestUnmarshalBodyReusableDiskJSONStreamsWithoutBytesCopy(t *testing.T) {
	payload := []byte(`{"model":"gpt-5","input":"hello"}`)
	storage := &diskJSONTestStorage{reader: bytes.NewReader(payload), data: payload}
	c := newDiskJSONTestContext(storage)

	var decoded struct {
		Model string `json:"model"`
		Input string `json:"input"`
	}
	err := UnmarshalBodyReusable(c, &decoded)

	require.NoError(t, err)
	require.Zero(t, storage.bytesCalls)
	require.GreaterOrEqual(t, storage.seekCalls, 2)
	require.Equal(t, "gpt-5", decoded.Model)
	require.Equal(t, "hello", decoded.Input)
	require.NotNil(t, c.Request.Body)
	body, err := io.ReadAll(c.Request.Body)
	require.NoError(t, err)
	require.Equal(t, payload, body)
}

func TestUnmarshalBodyReusableDiskJSONResetsAfterDecodeError(t *testing.T) {
	payload := []byte(`{"model":`)
	storage := &diskJSONTestStorage{reader: bytes.NewReader(payload), data: payload}
	c := newDiskJSONTestContext(storage)
	originalBody := c.Request.Body

	var decoded map[string]any
	err := UnmarshalBodyReusable(c, &decoded)

	require.Error(t, err)
	require.Zero(t, storage.bytesCalls)
	position, seekErr := storage.Seek(0, io.SeekCurrent)
	require.NoError(t, seekErr)
	require.Zero(t, position)
	require.Equal(t, originalBody, c.Request.Body)
}

func TestUnmarshalBodyReusableDiskJSONRejectsTrailingValue(t *testing.T) {
	payload := []byte(`{"model":"gpt-5"} {"extra":true}`)
	storage := &diskJSONTestStorage{reader: bytes.NewReader(payload), data: payload}
	c := newDiskJSONTestContext(storage)
	var decoded map[string]any
	err := UnmarshalBodyReusable(c, &decoded)
	require.Error(t, err)
	require.Equal(t, int64(0), func() int64 { position, _ := storage.Seek(0, io.SeekCurrent); return position }())
}

func BenchmarkUnmarshalBodyReusableDiskJSON(b *testing.B) {
	gin.SetMode(gin.TestMode)
	payload := bytes.Repeat([]byte("x"), 1<<20)
	jsonPayload := append([]byte(`{"input":"`), payload...)
	jsonPayload = append(jsonPayload, []byte(`"}`)...)
	b.ReportAllocs()
	b.SetBytes(int64(len(jsonPayload)))
	for i := 0; i < b.N; i++ {
		storage := &diskJSONTestStorage{reader: bytes.NewReader(jsonPayload), data: jsonPayload}
		c := newDiskJSONTestContext(storage)
		var decoded map[string]any
		if err := UnmarshalBodyReusable(c, &decoded); err != nil {
			b.Fatal(err)
		}
	}
}
