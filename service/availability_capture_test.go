package service

import (
	"context"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
)

func TestAvailabilityStreamDisconnectDoesNotHideServiceFailure(t *testing.T) {
	stream := relaycommon.NewStreamStatus()
	stream.SetEndReason(relaycommon.StreamEndReasonClientGone, context.Canceled)
	outcome, _ := ClassifyAvailabilityResult(nil, stream, true, true)
	assert.Equal(t, model.AvailabilityExcluded, outcome)

	stream.SetEndReason(relaycommon.StreamEndReasonTimeout, context.DeadlineExceeded)
	outcome, _ = ClassifyAvailabilityResult(nil, stream, true, true)
	assert.Equal(t, model.AvailabilityFailure, outcome)

	outcome, _ = ClassifyAvailabilityResult(types.NewError(context.DeadlineExceeded, types.ErrorCodeDoRequestFailed), nil, false, true)
	assert.Equal(t, model.AvailabilityFailure, outcome)
}

func TestAvailabilityCompletedStreamIgnoresLaterCleanupTimeout(t *testing.T) {
	stream := relaycommon.NewStreamStatus()
	stream.SetEndReason(relaycommon.StreamEndReasonDone, nil)
	stream.SetEndReason(relaycommon.StreamEndReasonTimeout, nil)
	outcome, _ := ClassifyAvailabilityResult(nil, stream, true, false)
	assert.Equal(t, model.AvailabilitySuccess, outcome)
}

func TestAvailabilityCompletedStreamKeepsSuccessAfterCleanupError(t *testing.T) {
	stream := relaycommon.NewStreamStatus()
	stream.SetEndReason(relaycommon.StreamEndReasonDone, nil)
	stream.SetEndReason(relaycommon.StreamEndReasonHandlerStop, context.Canceled)
	outcome, _ := ClassifyAvailabilityResult(nil, stream, true, true)
	assert.Equal(t, model.AvailabilitySuccess, outcome)
}

func TestAvailabilityUpstreamRequestErrorIsNotLocalRejection(t *testing.T) {
	upstream := types.WithOpenAIError(types.OpenAIError{Code: "invalid_request", Type: "invalid_request_error", Message: "upstream configuration rejected"}, http.StatusBadRequest)
	outcome, _ := ClassifyAvailabilityResult(upstream, nil, false, false)
	assert.Equal(t, model.AvailabilityFailure, outcome)
	local := types.NewOpenAIError(assert.AnError, types.ErrorCodeSubscriptionTokenExhausted, http.StatusForbidden)
	outcome, _ = ClassifyAvailabilityResult(local, nil, false, false)
	assert.Equal(t, model.AvailabilityExcluded, outcome)
}

func TestAvailabilityEOFWithoutProtocolCompletionFails(t *testing.T) {
	stream := relaycommon.NewStreamStatus()
	stream.FinalizeEOF()
	outcome, _ := ClassifyAvailabilityResult(nil, stream, true, false)
	assert.Equal(t, model.AvailabilityFailure, outcome)
}

func TestAvailabilityQuotaRejectionCannotHideKnownStreamFailure(t *testing.T) {
	stream := relaycommon.NewStreamStatus()
	stream.SetEndReason(relaycommon.StreamEndReasonTimeout, context.DeadlineExceeded)
	quotaErr := types.NewError(assert.AnError, types.ErrorCodeAPIKeyTokenLimitExhausted)
	outcome, _ := ClassifyAvailabilityResult(quotaErr, stream, true, false)
	assert.Equal(t, model.AvailabilityFailure, outcome)
}

func TestAvailabilityClientCancellationBeforeResponseIsExcluded(t *testing.T) {
	apiErr := types.NewError(context.Canceled, types.ErrorCodeDoRequestFailed)
	outcome, _ := ClassifyAvailabilityResult(apiErr, nil, false, true)
	assert.Equal(t, model.AvailabilityExcluded, outcome)
	// A timeout is independent service failure evidence, even if the client
	// also disconnected while the error response was being delivered.
	apiErr = types.NewError(context.DeadlineExceeded, types.ErrorCodeDoRequestFailed)
	outcome, _ = ClassifyAvailabilityResult(apiErr, nil, false, true)
	assert.Equal(t, model.AvailabilityFailure, outcome)
}

func TestAvailabilityCaptureFreezesOwnershipAndCountsFinalOutcome(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "capture.db")), &gorm.Config{})
	require.NoError(t, err)
	oldDB, oldLogs := model.DB, model.LOG_DB
	model.DB, model.LOG_DB = db, db
	t.Cleanup(func() { model.DB, model.LOG_DB = oldDB, oldLogs; conn, _ := db.DB(); _ = conn.Close() })
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.ChannelGroup{}, &model.ChannelGroupChannel{}, &model.Task{}, &model.Midjourney{}))
	require.NoError(t, model.MigrateAvailability(db))
	for _, g := range []model.ChannelGroup{{Id: 2, Name: "Alpha", Enabled: true}, {Id: 3, Name: "Beta", Enabled: true}} {
		require.NoError(t, db.Create(&g).Error)
	}
	for _, ch := range []model.Channel{{Id: 10, Name: "secret-a", Key: "secret", Status: 1, Models: "m", TokenBillingMultiplier: 1}, {Id: 11, Name: "secret-b", Key: "secret", Status: 1, Models: "m", TokenBillingMultiplier: 1}} {
		require.NoError(t, db.Create(&ch).Error)
	}
	require.NoError(t, db.Create(&[]model.ChannelGroupChannel{{ChannelGroupId: 2, ChannelId: 10}, {ChannelGroupId: 3, ChannelId: 11}}).Error)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Set(common.RequestIdKey, "caller-reused-id")
	common.SetContextKey(ctx, constant.ContextKeyRequestStartTime, time.Now())
	capture := BeginAvailability(ctx, "m", []string{"Alpha", "Beta"})
	require.NotNil(t, capture)
	AttemptAvailabilityChannel(ctx, 10)
	AttemptAvailabilityChannel(ctx, 11)
	// Membership changes after request start must not move the final outcome.
	require.NoError(t, db.Model(&model.ChannelGroupChannel{}).Where("channel_id = ?", 11).Update("channel_group_id", 2).Error)
	model.InvalidateAvailabilityCatalog()
	ObserveAvailabilityResult(ctx, &relaycommon.RelayInfo{}, nil)
	capture.Finish(ctx)
	// Keep Beta visible through a different channel after moving the original.
	require.NoError(t, db.Create(&model.Channel{Id: 12, Key: "secret", Status: 1, Models: "m", TokenBillingMultiplier: 1}).Error)
	require.NoError(t, db.Create(&model.ChannelGroupChannel{ChannelGroupId: 3, ChannelId: 12}).Error)
	second := BeginAvailability(ctx, "m", []string{"Alpha"})
	AttemptAvailabilityChannel(ctx, 10)
	ObserveAvailabilityResult(ctx, nil, types.NewError(assert.AnError, types.ErrorCodeDoRequestFailed))
	second.Finish(ctx)
	report, err := model.GetAvailabilityReport(context.Background(), "1h", time.Now().Add(time.Second))
	require.NoError(t, err)
	require.Len(t, report.Groups, 2)
	require.NotNil(t, report.Groups[0].Current.SuccessRate)
	require.NotNil(t, report.Groups[1].Current.SuccessRate)
	assert.Equal(t, 0.0, *report.Groups[0].Current.SuccessRate)
	assert.Equal(t, 100.0, *report.Groups[1].Current.SuccessRate)
	// Async acceptance remains pending until persisted terminal status.
	third := BeginAvailability(ctx, "m", []string{"Alpha"})
	AttemptAvailabilityChannel(ctx, 10)
	data, err := common.Marshal(DeferTaskAvailability(ctx))
	require.NoError(t, err)
	task := &model.Task{TaskID: "async-test", Status: model.TaskStatusNotStart, AvailabilityData: string(data), AvailabilityPending: true}
	require.NoError(t, task.Insert())
	third.Finish(ctx)
	task.Status, task.FinishTime = model.TaskStatusSuccess, time.Now().Unix()
	won, err := task.UpdateWithStatus(model.TaskStatusNotStart)
	require.NoError(t, err)
	require.True(t, won)
	require.NoError(t, model.ReplayTaskAvailability(context.Background()))
	report, err = model.GetAvailabilityReport(context.Background(), "1h", time.Now().Add(62*time.Second))
	require.NoError(t, err)
	assert.Equal(t, 50.0, *report.Groups[0].Current.SuccessRate)
}
