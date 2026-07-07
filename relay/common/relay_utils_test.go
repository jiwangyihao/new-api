package common

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestValidateMultipartDirectRejectsUnsafeTaskDuration(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "duration above max",
			body: fmt.Sprintf(`{"model":"sora-2","prompt":"make video","duration":%d}`, MaxTaskDurationSeconds+1),
		},
		{
			name: "seconds above max",
			body: fmt.Sprintf(`{"model":"sora-2","prompt":"make video","seconds":"%d"}`, MaxTaskDurationSeconds+1),
		},
		{
			name: "negative duration",
			body: `{"model":"sora-2","prompt":"make video","duration":-1}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := newTaskJSONContext(t, tt.body)

			taskErr := ValidateMultipartDirect(ctx, &RelayInfo{TaskRelayInfo: &TaskRelayInfo{}})

			require.NotNil(t, taskErr)
			require.Equal(t, "invalid_seconds", taskErr.Code)
		})
	}
}

func TestValidateMultipartDirectAcceptsNormalTaskDuration(t *testing.T) {
	ctx := newTaskJSONContext(t, `{"model":"sora-2","prompt":"make video","duration":8}`)
	info := &RelayInfo{TaskRelayInfo: &TaskRelayInfo{}}

	taskErr := ValidateMultipartDirect(ctx, info)

	require.Nil(t, taskErr)
	stored, err := GetTaskRequest(ctx)
	require.NoError(t, err)
	require.Equal(t, 8, stored.Duration)
}

func TestValidateBasicTaskRequestRejectsUnsafeTaskDuration(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "duration above max",
			body: fmt.Sprintf(`{"model":"video-model","prompt":"make video","duration":%d}`, MaxTaskDurationSeconds+1),
		},
		{
			name: "seconds above max",
			body: fmt.Sprintf(`{"model":"video-model","prompt":"make video","seconds":"%d"}`, MaxTaskDurationSeconds+1),
		},
		{
			name: "negative duration",
			body: `{"model":"video-model","prompt":"make video","duration":-1}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := newTaskJSONContext(t, tt.body)

			taskErr := ValidateBasicTaskRequest(ctx, &RelayInfo{TaskRelayInfo: &TaskRelayInfo{}}, constant.TaskActionGenerate)

			require.NotNil(t, taskErr)
			require.Equal(t, "invalid_seconds", taskErr.Code)
		})
	}
}

func TestValidateBasicTaskRequestAcceptsNormalTaskDuration(t *testing.T) {
	ctx := newTaskJSONContext(t, `{"model":"video-model","prompt":"make video","duration":8}`)
	info := &RelayInfo{TaskRelayInfo: &TaskRelayInfo{}}

	taskErr := ValidateBasicTaskRequest(ctx, info, constant.TaskActionGenerate)

	require.Nil(t, taskErr)
	stored, err := GetTaskRequest(ctx)
	require.NoError(t, err)
	require.Equal(t, 8, stored.Duration)
}

func newTaskJSONContext(t *testing.T, body string) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/video/generations", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	return ctx
}
