package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestMain(m *testing.M) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		panic("failed to open test db: " + err.Error())
	}
	sqlDB, err := db.DB()
	if err != nil {
		panic("failed to get sql.DB: " + err.Error())
	}
	sqlDB.SetMaxOpenConns(1)

	model.DB = db
	model.LOG_DB = db

	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false
	common.BatchUpdateEnabled = false
	common.LogConsumeEnabled = true

	if err := db.AutoMigrate(
		&model.Task{},
		&model.User{},
		&model.Token{},
		&model.Log{},
		&model.Channel{},
		&model.TopUp{},
		&model.SubscriptionOrder{},
		&model.UserSubscription{},
		&model.SubscriptionPlan{},
		&model.SubscriptionConversion{},
		&model.SubscriptionPreConsumeRecord{},
		&model.CreditValuationState{},
		&model.CreditValuationMigration{},
		&model.CreditBalanceLedger{},
		&model.TrialCode{},
		&model.TrialRedemption{},
		&model.OAuthProviderLock{},
		&model.Ability{},
		&model.LogAggregationEvent{},
		&model.LogUsageHourly{},
		&model.FreeSubscriptionUsageHourly{},
		&model.InvitationMonthlyEntitlement{},
	); err != nil {
		panic("failed to migrate: " + err.Error())
	}

	os.Exit(m.Run())
}

// ---------------------------------------------------------------------------
// Seed helpers
// ---------------------------------------------------------------------------

func truncate(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		for _, tableName := range []string{
			"tasks",
			"users",
			"tokens",
			"logs",
			"channels",
			"top_ups",
			"subscription_orders",
			"user_subscriptions",
			"subscription_plans",
			"subscription_conversions",
			"subscription_pre_consume_records",
			"credit_valuation_states",
			"credit_valuation_migrations",
			"credit_balance_ledgers",
			"abilities",
			"log_aggregation_events",
			"log_usage_hourly",
			"free_subscription_usage_hourly",
			"trial_codes",
			"trial_redemptions",
			"oauth_provider_locks",
			"invitation_monthly_entitlements",
		} {
			if model.DB.Migrator().HasTable(tableName) {
				model.DB.Exec("DELETE FROM " + tableName)
			}
		}
		model.ClearPrimaryBillableSubscriptionCacheForTest()
		model.ClearSubscriptionPlanCacheForTest()
	})
	model.ClearPrimaryBillableSubscriptionCacheForTest()
	model.ClearSubscriptionPlanCacheForTest()
}

func seedUser(t *testing.T, id int, quota int) {
	t.Helper()
	user := &model.User{Id: id, Username: fmt.Sprintf("test_user_%d", id), AffCode: fmt.Sprintf("aff%d", id), Quota: quota, Status: common.UserStatusEnabled}
	require.NoError(t, model.DB.Create(user).Error)
}

func seedToken(t *testing.T, id int, userId int, key string, remainQuota int) {
	t.Helper()
	token := &model.Token{
		Id:          id,
		UserId:      userId,
		Key:         key,
		Name:        "test_token",
		Status:      common.TokenStatusEnabled,
		RemainQuota: remainQuota,
		UsedQuota:   0,
	}
	require.NoError(t, model.DB.Create(token).Error)
}

func seedSubscription(t *testing.T, id int, userId int, amountTotal int64, amountUsed int64) {
	t.Helper()
	sub := &model.UserSubscription{
		Id:          id,
		UserId:      userId,
		AmountTotal: amountTotal,
		AmountUsed:  amountUsed,
		Status:      "active",
		StartTime:   time.Now().Unix(),
		EndTime:     time.Now().Add(30 * 24 * time.Hour).Unix(),
	}
	require.NoError(t, model.DB.Create(sub).Error)
}

func seedChannel(t *testing.T, id int) {
	t.Helper()
	ch := &model.Channel{Id: id, Name: "test_channel", Key: "sk-test", Status: common.ChannelStatusEnabled}
	require.NoError(t, model.DB.Create(ch).Error)
}

func makeTask(userId, channelId, quota, tokenId int, billingSource string, subscriptionId int) *model.Task {
	return &model.Task{
		TaskID:    "task_" + time.Now().Format("150405.000"),
		UserId:    userId,
		ChannelId: channelId,
		Quota:     quota,
		Status:    model.TaskStatus(model.TaskStatusInProgress),
		Group:     "default",
		Data:      json.RawMessage(`{}`),
		CreatedAt: time.Now().Unix(),
		UpdatedAt: time.Now().Unix(),
		Properties: model.Properties{
			OriginModelName: "test-model",
		},
		PrivateData: model.TaskPrivateData{
			BillingSource:  billingSource,
			SubscriptionId: subscriptionId,
			TokenId:        tokenId,
			BillingContext: &model.TaskBillingContext{
				ModelPrice:      0.02,
				QuotaMultiplier: 1.0,
				OriginModelName: "test-model",
			},
		},
	}
}

// ---------------------------------------------------------------------------
// Read-back helpers
// ---------------------------------------------------------------------------

func getUserQuota(t *testing.T, id int) int {
	t.Helper()
	var user model.User
	require.NoError(t, model.DB.Select("quota").Where("id = ?", id).First(&user).Error)
	return user.Quota
}

func getTokenRemainQuota(t *testing.T, id int) int {
	t.Helper()
	var token model.Token
	require.NoError(t, model.DB.Select("remain_quota").Where("id = ?", id).First(&token).Error)
	return token.RemainQuota
}

func getTokenUsedQuota(t *testing.T, id int) int {
	t.Helper()
	var token model.Token
	require.NoError(t, model.DB.Select("used_quota").Where("id = ?", id).First(&token).Error)
	return token.UsedQuota
}

func getSubscriptionUsed(t *testing.T, id int) int64 {
	t.Helper()
	var sub model.UserSubscription
	require.NoError(t, model.DB.Select("amount_used").Where("id = ?", id).First(&sub).Error)
	return sub.AmountUsed
}

func getLastLog(t *testing.T) *model.Log {
	t.Helper()
	var log model.Log
	err := model.LOG_DB.Order("id desc").First(&log).Error
	if err != nil {
		return nil
	}
	return &log
}

func countLogs(t *testing.T) int64 {
	t.Helper()
	var count int64
	model.LOG_DB.Model(&model.Log{}).Count(&count)
	return count
}

// ===========================================================================
// RefundTaskQuota tests
// ===========================================================================

func TestRefundTaskQuota_Wallet(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID = 1, 1, 1
	const initQuota, preConsumed = 10000, 3000
	const tokenRemain = 5000

	seedUser(t, userID, initQuota)
	seedToken(t, tokenID, userID, "sk-test-key", tokenRemain)
	seedChannel(t, channelID)

	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceWallet, 0)

	RefundTaskQuota(ctx, task, "task failed: upstream error")

	assert.Equal(t, initQuota, getUserQuota(t, userID))
	assert.Equal(t, tokenRemain, getTokenRemainQuota(t, tokenID))
	assert.Equal(t, 0, getTokenUsedQuota(t, tokenID))
	assert.Equal(t, int64(0), countLogs(t))
}

func TestRefundTaskQuota_Subscription(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID, subID = 2, 2, 2, 1
	const preConsumed = 2000
	const subTotal, subUsed int64 = 100000, 50000
	const tokenRemain = 8000

	seedUser(t, userID, 0)
	seedToken(t, tokenID, userID, "sk-sub-key", tokenRemain)
	seedChannel(t, channelID)
	seedSubscription(t, subID, userID, subTotal, subUsed)

	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceSubscription, subID)

	RefundTaskQuota(ctx, task, "subscription task failed")

	// Subscription used should decrease by preConsumed
	assert.Equal(t, subUsed-int64(preConsumed), getSubscriptionUsed(t, subID))

	// Legacy API key quota is compatibility data and must not be adjusted.
	assert.Equal(t, tokenRemain, getTokenRemainQuota(t, tokenID))
	assert.Equal(t, 0, getTokenUsedQuota(t, tokenID))

	log := getLastLog(t)
	require.NotNil(t, log)
	assert.Equal(t, model.LogTypeRefund, log.Type)
}

func TestRefundTaskQuotaDoesNotAdjustLegacyTokenQuota(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	const userID, tokenID, channelID, subID = 97001, 97002, 97003, 97004
	const preConsumed = 10

	seedUser(t, userID, 0)
	seedToken(t, tokenID, userID, "sk-task-refund-no-legacy", 100)
	require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", tokenID).Update("used_quota", 50).Error)
	seedChannel(t, channelID)
	seedSubscription(t, subID, userID, 1000, 500)

	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceSubscription, subID)
	RefundTaskQuota(ctx, task, "task failed")

	require.Equal(t, int64(500-preConsumed), getSubscriptionUsed(t, subID))
	require.Equal(t, 100, getTokenRemainQuota(t, tokenID))
	require.Equal(t, 50, getTokenUsedQuota(t, tokenID))
}

func TestRecalculateTaskQuotaDoesNotAdjustLegacyTokenQuota(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	const userID, tokenID, channelID, subID = 97011, 97012, 97013, 97014
	const preConsumed = 10
	const actualQuota = 15

	seedUser(t, userID, 0)
	seedToken(t, tokenID, userID, "sk-task-recalc-no-legacy", 100)
	require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", tokenID).Update("used_quota", 50).Error)
	seedChannel(t, channelID)
	seedSubscription(t, subID, userID, 1000, 500)

	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceSubscription, subID)
	RecalculateTaskQuota(ctx, task, actualQuota, "task settle")

	require.Equal(t, int64(500+actualQuota-preConsumed), getSubscriptionUsed(t, subID))
	require.Equal(t, 100, getTokenRemainQuota(t, tokenID))
	require.Equal(t, 50, getTokenUsedQuota(t, tokenID))
}

func seedDistributorTaskSubscription(t *testing.T, id int, userId int, tokenLimit int64, tokenUsed int64) {
	t.Helper()
	sub := &model.UserSubscription{
		Id:               id,
		UserId:           userId,
		TokenLimit:       tokenLimit,
		TokenUsed:        tokenUsed,
		ConcurrencyLimit: 1,
		Status:           "active",
		StartTime:        time.Now().Unix(),
		EndTime:          time.Now().Add(30 * 24 * time.Hour).Unix(),
	}
	require.NoError(t, model.DB.Create(sub).Error)
}

func TestTaskBillingDoesNotAdjustDistributorSubscription(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID, subID = 80, 80, 80, 80
	const preConsumed = 2000
	const tokenRemain = 8000
	const tokenUsed int64 = 50

	seedUser(t, userID, 0)
	seedToken(t, tokenID, userID, "sk-dist-task", tokenRemain)
	seedChannel(t, channelID)
	seedDistributorTaskSubscription(t, subID, userID, 100, tokenUsed)

	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceSubscription, subID)

	RefundTaskQuota(ctx, task, "distributor task failed")
	assert.Equal(t, tokenUsed, getSubscriptionTokenUsedForTaskTest(t, subID))
	assert.Equal(t, tokenRemain, getTokenRemainQuota(t, tokenID), "token key quota must not be refunded when distributor task funding is rejected")

	RecalculateTaskQuota(ctx, task, 3000, "distributor task settle")
	assert.Equal(t, tokenUsed, getSubscriptionTokenUsedForTaskTest(t, subID))
}

func getSubscriptionTokenUsedForTaskTest(t *testing.T, id int) int64 {
	t.Helper()
	var sub model.UserSubscription
	require.NoError(t, model.DB.Select("token_used").Where("id = ?", id).First(&sub).Error)
	return sub.TokenUsed
}
func TestTaskBillingDoesNotAdjustBusinessCodedDistributorSubscription(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID, planID, subID = 81, 81, 81, 81, 82
	const preConsumed = 2000
	const tokenRemain = 8000
	const tokenUsed int64 = 50

	seedUser(t, userID, 0)
	seedToken(t, tokenID, userID, "sk-business-task", tokenRemain)
	seedChannel(t, channelID)
	require.NoError(t, model.DB.Migrator().DropTable(&model.SubscriptionPlan{}))
	require.NoError(t, model.DB.AutoMigrate(&model.SubscriptionPlan{}))
	code := "business-task"
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: planID, Title: "Business Task", Enabled: true, BusinessCode: &code}).Error)
	require.NoError(t, model.DB.Create(&model.UserSubscription{Id: subID, UserId: userID, PlanId: planID, TokenUsed: tokenUsed, Status: "active", StartTime: time.Now().Unix(), EndTime: time.Now().Add(30 * 24 * time.Hour).Unix()}).Error)

	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceSubscription, subID)
	RecalculateTaskQuota(ctx, task, 3000, "business-coded distributor task settle")
	assert.Equal(t, tokenUsed, getSubscriptionTokenUsedForTaskTest(t, subID))
}

func TestCreditBalanceTaskBillingAdjustsTokenUsedBothDirections(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	const userID, tokenID, channelID, subID = 83, 83, 83, 83
	seedUser(t, userID, 0)
	seedToken(t, tokenID, userID, "sk-credit-balance-task", 8_000)
	seedChannel(t, channelID)
	require.NoError(t, model.DB.Create(&model.UserSubscription{
		Id:              subID,
		UserId:          userID,
		EntitlementType: model.SubscriptionEntitlementCreditBalance,
		TokenLimit:      1_000,
		TokenUsed:       100,
		AmountUsed:      9,
		Status:          "active",
		EndTime:         0,
	}).Error)

	settleTask := makeTask(userID, channelID, 100, tokenID, BillingSourceSubscription, subID)
	RecalculateTaskQuota(ctx, settleTask, 140, "credit task settle")
	require.Equal(t, int64(140), getSubscriptionTokenUsedForTaskTest(t, subID))
	var sub model.UserSubscription
	require.NoError(t, model.DB.Select("amount_used").Where("id = ?", subID).First(&sub).Error)
	require.Equal(t, int64(9), sub.AmountUsed)

	refundTask := makeTask(userID, channelID, 40, tokenID, BillingSourceSubscription, subID)
	RefundTaskQuota(ctx, refundTask, "credit task failed")
	require.Equal(t, int64(100), getSubscriptionTokenUsedForTaskTest(t, subID))
	require.NoError(t, model.DB.Select("amount_used").Where("id = ?", subID).First(&sub).Error)
	require.Equal(t, int64(9), sub.AmountUsed)
}

func TestCreditTaskPersistsSubscriptionRequestIDAcrossReload(t *testing.T) {
	truncate(t)
	relayInfo := &relaycommon.RelayInfo{
		UserId:         83,
		RequestId:      "req-credit-task-persisted",
		BillingSource:  BillingSourceSubscription,
		SubscriptionId: 83,
	}
	task := model.InitTask("video", relayInfo)
	task.TaskID = "task-credit-request-id-reload"
	require.NoError(t, model.DB.Create(task).Error)

	var reloaded model.Task
	require.NoError(t, model.DB.First(&reloaded, task.ID).Error)
	require.Equal(t, relayInfo.RequestId, reloaded.PrivateData.SubscriptionRequestId)
}

func TestLegacyCreditTaskRequestIDUsesPersistentTaskPrimaryKey(t *testing.T) {
	truncate(t)
	first := makeTask(83, 83, 100, 83, BillingSourceSubscription, 83)
	first.TaskID = "task-credit-legacy-first"
	second := makeTask(83, 83, 100, 83, BillingSourceSubscription, 83)
	second.TaskID = "task-credit-legacy-second"
	require.NoError(t, model.DB.Create(first).Error)
	require.NoError(t, model.DB.Create(second).Error)

	firstRequestID, err := taskSubscriptionRequestID(first)
	require.NoError(t, err)
	secondRequestID, err := taskSubscriptionRequestID(second)
	require.NoError(t, err)
	require.NotEqual(t, firstRequestID, secondRequestID)

	var reloaded model.Task
	require.NoError(t, model.DB.First(&reloaded, first.ID).Error)
	reloadedRequestID, err := taskSubscriptionRequestID(&reloaded)
	require.NoError(t, err)
	require.Equal(t, firstRequestID, reloadedRequestID)
}

func seedCreditTaskRequestLifecycle(t *testing.T, requestID string, taskID string, preConsumed int64) *model.Task {
	t.Helper()
	const userID, tokenID, planID, subID, channelID = 83, 84, 85, 86, 87
	seedCreditBillingRuntime(t, userID, tokenID, planID, subID, channelID, "sk-credit-task-identity", 1_000, 0)
	require.NoError(t, model.DB.Create(&model.CreditValuationMigration{
		Version: 1, Status: model.CreditValuationMigrationReady, ValuationCurrency: "CNY",
	}).Error)
	require.NoError(t, model.DB.Create(&model.CreditValuationState{
		UserSubscriptionId: subID,
		UserId:             userID,
		AvailableCredit:    1_000,
		ExactCostMicros:    40_000_000,
		Currency:           "CNY",
		RuleVersion:        model.CreditValuationRuleVersion,
		StateVersion:       1,
	}).Error)
	_, err := model.PreConsumeUserSubscriptionByUnits(requestID, userID, "video-model", 0, preConsumed, preConsumed)
	require.NoError(t, err)

	task := makeTask(userID, channelID, int(preConsumed), tokenID, BillingSourceSubscription, subID)
	task.TaskID = taskID
	task.PrivateData.SubscriptionRequestId = requestID
	require.NoError(t, model.DB.Create(task).Error)
	var reloaded model.Task
	require.NoError(t, model.DB.First(&reloaded, task.ID).Error)
	return &reloaded
}

func loadCreditTaskRequestRecord(t *testing.T, requestID string) model.SubscriptionPreConsumeRecord {
	t.Helper()
	var record model.SubscriptionPreConsumeRecord
	require.NoError(t, model.DB.Where("request_id = ?", requestID).First(&record).Error)
	return record
}

func TestCreditTaskSuccessFinalAndReplayReusePersistedRequestID(t *testing.T) {
	truncate(t)
	const requestID = "req-credit-task-success-replay"
	task := seedCreditTaskRequestLifecycle(t, requestID, "task-credit-success-replay", 100)
	require.NoError(t, model.SettleUserSubscriptionRequestTarget(requestID, task.PrivateData.SubscriptionId, 140, false))
	task.Quota = 140
	require.NoError(t, task.UpdateQuota())
	require.Equal(t, "consumed", loadCreditTaskRequestRecord(t, requestID).Status)

	RecalculateTaskQuota(context.Background(), task, 140, "credit task success")
	settled := loadCreditTaskRequestRecord(t, requestID)
	require.Equal(t, int64(140), settled.AppliedCredit)
	require.Equal(t, "settled", settled.Status)
	require.Positive(t, settled.FinalizedAt)
	require.Equal(t, int64(140), getSubscriptionTokenUsedForTaskTest(t, task.PrivateData.SubscriptionId))

	var reloaded model.Task
	require.NoError(t, model.DB.First(&reloaded, task.ID).Error)
	RecalculateTaskQuota(context.Background(), &reloaded, 140, "credit task success replay")
	replayed := loadCreditTaskRequestRecord(t, requestID)
	require.Equal(t, settled.SettlementVersion, replayed.SettlementVersion)
	require.Equal(t, settled.FinalizedAt, replayed.FinalizedAt)
	require.Equal(t, int64(140), getSubscriptionTokenUsedForTaskTest(t, task.PrivateData.SubscriptionId))
}

func TestCreditTaskFailureRefundAndReplayReusePersistedRequestID(t *testing.T) {
	truncate(t)
	const requestID = "req-credit-task-failure-replay"
	task := seedCreditTaskRequestLifecycle(t, requestID, "task-credit-failure-replay", 100)

	RefundTaskQuota(context.Background(), task, "credit task failed")
	refunded := loadCreditTaskRequestRecord(t, requestID)
	require.Zero(t, refunded.AppliedCredit)
	require.Equal(t, "refunded", refunded.Status)
	require.Positive(t, refunded.FinalizedAt)
	require.Zero(t, getSubscriptionTokenUsedForTaskTest(t, task.PrivateData.SubscriptionId))

	var reloaded model.Task
	require.NoError(t, model.DB.First(&reloaded, task.ID).Error)
	RefundTaskQuota(context.Background(), &reloaded, "credit task failed replay")
	replayed := loadCreditTaskRequestRecord(t, requestID)
	require.Equal(t, refunded.SettlementVersion, replayed.SettlementVersion)
	require.Equal(t, refunded.FinalizedAt, replayed.FinalizedAt)
	require.Zero(t, getSubscriptionTokenUsedForTaskTest(t, task.PrivateData.SubscriptionId))
}

func TestConvertedTimedTaskSettlementKeepsSourceIdentityAndAdjustsCreditBalance(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	const userID, tokenID, channelID, sourceID, targetID, conversionID = 84, 84, 84, 84, 85, 86
	seedUser(t, userID, 0)
	seedToken(t, tokenID, userID, "sk-converted-task", 8_000)
	seedChannel(t, channelID)
	require.NoError(t, model.DB.Create(&model.UserSubscription{
		Id: sourceID, UserId: userID, EntitlementType: model.SubscriptionEntitlementTimed,
		AmountTotal: 1_000, AmountUsed: 100, TokenLimit: 100, TokenUsed: 10,
		Status:       model.SubscriptionStatusConverted,
		ConversionId: conversionID, ConvertedToSubscriptionId: targetID,
	}).Error)
	require.NoError(t, model.DB.Create(&model.UserSubscription{
		Id: targetID, UserId: userID, EntitlementType: model.SubscriptionEntitlementCreditBalance,
		TokenLimit: 100, TokenUsed: 20, Status: model.SubscriptionStatusActive,
	}).Error)
	require.NoError(t, model.DB.Create(&model.SubscriptionConversion{
		Id: conversionID, UserId: userID, IdempotencyKey: "converted-task",
		SourceSubscriptionId: sourceID, TargetSubscriptionId: targetID,
		ConvertedAt: time.Now().Unix(), CreatedAt: time.Now().Unix(),
	}).Error)

	settleTask := makeTask(userID, channelID, 10, tokenID, BillingSourceSubscription, sourceID)
	RecalculateTaskQuota(ctx, settleTask, 35, "converted task settle")
	require.Equal(t, int64(45), getSubscriptionTokenUsedForTaskTest(t, targetID))
	require.Equal(t, int64(100), getSubscriptionUsed(t, sourceID))
	require.Equal(t, sourceID, settleTask.PrivateData.SubscriptionId)
	settleLog := getLastLog(t)
	require.NotNil(t, settleLog)
	require.NotNil(t, settleLog.SubscriptionID)
	assert.Equal(t, sourceID, *settleLog.SubscriptionID)
	require.NotNil(t, settleLog.BillingSource)
	assert.Equal(t, BillingSourceSubscription, *settleLog.BillingSource)

	refundTask := makeTask(userID, channelID, 15, tokenID, BillingSourceSubscription, sourceID)
	RefundTaskQuota(ctx, refundTask, "converted task refund")
	require.Equal(t, int64(30), getSubscriptionTokenUsedForTaskTest(t, targetID))
	require.Equal(t, int64(100), getSubscriptionUsed(t, sourceID))
	require.Equal(t, sourceID, refundTask.PrivateData.SubscriptionId)
	refundLog := getLastLog(t)
	require.NotNil(t, refundLog)
	require.NotNil(t, refundLog.SubscriptionID)
	assert.Equal(t, sourceID, *refundLog.SubscriptionID)
	require.NotNil(t, refundLog.BillingSource)
	assert.Equal(t, BillingSourceSubscription, *refundLog.BillingSource)
}

func TestRefundTaskQuota_ZeroQuota(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID = 3
	seedUser(t, userID, 5000)

	task := makeTask(userID, 0, 0, 0, BillingSourceWallet, 0)

	RefundTaskQuota(ctx, task, "zero quota task")

	// No change to user quota
	assert.Equal(t, 5000, getUserQuota(t, userID))

	// No log created
	assert.Equal(t, int64(0), countLogs(t))
}

func TestRefundTaskQuota_NoToken(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, channelID = 4, 4
	const initQuota, preConsumed = 10000, 1500

	seedUser(t, userID, initQuota)
	seedChannel(t, channelID)

	task := makeTask(userID, channelID, preConsumed, 0, BillingSourceWallet, 0) // TokenId=0

	RefundTaskQuota(ctx, task, "no token task failed")

	assert.Equal(t, initQuota, getUserQuota(t, userID))
	assert.Equal(t, int64(0), countLogs(t))
}

// ===========================================================================
// RecalculateTaskQuota tests
// ===========================================================================

func TestRecalculate_PositiveDelta(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID = 10, 10, 10
	const initQuota, preConsumed = 10000, 2000
	const actualQuota = 3000
	const tokenRemain = 5000

	seedUser(t, userID, initQuota)
	seedToken(t, tokenID, userID, "sk-recalc-pos", tokenRemain)
	seedChannel(t, channelID)

	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceWallet, 0)

	RecalculateTaskQuota(ctx, task, actualQuota, "adaptor adjustment")

	assert.Equal(t, initQuota, getUserQuota(t, userID))
	assert.Equal(t, tokenRemain, getTokenRemainQuota(t, tokenID))
	assert.Equal(t, preConsumed, task.Quota)
	assert.Equal(t, int64(0), countLogs(t))
}

func TestRecalculate_NegativeDelta(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID = 11, 11, 11
	const initQuota, preConsumed = 10000, 5000
	const actualQuota = 3000
	const tokenRemain = 5000

	seedUser(t, userID, initQuota)
	seedToken(t, tokenID, userID, "sk-recalc-neg", tokenRemain)
	seedChannel(t, channelID)

	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceWallet, 0)

	RecalculateTaskQuota(ctx, task, actualQuota, "adaptor adjustment")

	assert.Equal(t, initQuota, getUserQuota(t, userID))
	assert.Equal(t, tokenRemain, getTokenRemainQuota(t, tokenID))
	assert.Equal(t, preConsumed, task.Quota)
	assert.Equal(t, int64(0), countLogs(t))
}

func TestRecalculate_ZeroDelta(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID = 12
	const initQuota, preConsumed = 10000, 3000

	seedUser(t, userID, initQuota)

	task := makeTask(userID, 0, preConsumed, 0, BillingSourceWallet, 0)

	RecalculateTaskQuota(ctx, task, preConsumed, "exact match")

	// No change to user quota
	assert.Equal(t, initQuota, getUserQuota(t, userID))

	// No log created (delta is zero)
	assert.Equal(t, int64(0), countLogs(t))
}

func TestRecalculate_ActualQuotaZero(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID = 13
	const initQuota = 10000

	seedUser(t, userID, initQuota)

	task := makeTask(userID, 0, 5000, 0, BillingSourceWallet, 0)

	RecalculateTaskQuota(ctx, task, 0, "zero actual")

	// No change (early return)
	assert.Equal(t, initQuota, getUserQuota(t, userID))
	assert.Equal(t, int64(0), countLogs(t))
}

func TestRecalculate_Subscription_NegativeDelta(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID, subID = 14, 14, 14, 2
	const preConsumed = 5000
	const actualQuota = 2000 // over-charged by 3000
	const subTotal, subUsed int64 = 100000, 50000
	const tokenRemain = 8000

	seedUser(t, userID, 0)
	seedToken(t, tokenID, userID, "sk-sub-recalc", tokenRemain)
	seedChannel(t, channelID)
	seedSubscription(t, subID, userID, subTotal, subUsed)

	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceSubscription, subID)

	RecalculateTaskQuota(ctx, task, actualQuota, "subscription over-charge")

	// Subscription used should decrease by delta (refund 3000)
	assert.Equal(t, subUsed-int64(preConsumed-actualQuota), getSubscriptionUsed(t, subID))

	// Legacy API key quota is compatibility data and must not be adjusted.
	assert.Equal(t, tokenRemain, getTokenRemainQuota(t, tokenID))
	assert.Equal(t, 0, getTokenUsedQuota(t, tokenID))

	assert.Equal(t, actualQuota, task.Quota)

	log := getLastLog(t)
	require.NotNil(t, log)
	assert.Equal(t, model.LogTypeRefund, log.Type)
}

// ===========================================================================
// CAS + Billing integration tests
// Simulates the flow in updateVideoSingleTask (service/task_polling.go)
// ===========================================================================

// simulatePollBilling reproduces the CAS + billing logic from updateVideoSingleTask.
// It takes a persisted task (already in DB), applies the new status, and performs
// the conditional update + billing exactly as the polling loop does.
func simulatePollBilling(ctx context.Context, task *model.Task, newStatus model.TaskStatus, actualQuota int) {
	snap := task.Snapshot()

	shouldRefund := false
	shouldSettle := false
	quota := task.Quota

	task.Status = newStatus
	switch string(newStatus) {
	case model.TaskStatusSuccess:
		task.Progress = "100%"
		task.FinishTime = 9999
		shouldSettle = true
	case model.TaskStatusFailure:
		task.Progress = "100%"
		task.FinishTime = 9999
		task.FailReason = "upstream error"
		if quota != 0 {
			shouldRefund = true
		}
	default:
		task.Progress = "50%"
	}

	isDone := task.Status == model.TaskStatus(model.TaskStatusSuccess) || task.Status == model.TaskStatus(model.TaskStatusFailure)
	if isDone && snap.Status != task.Status {
		won, err := task.UpdateWithStatus(snap.Status)
		if err != nil {
			shouldRefund = false
			shouldSettle = false
		} else if !won {
			shouldRefund = false
			shouldSettle = false
		}
	} else if !snap.Equal(task.Snapshot()) {
		_, _ = task.UpdateWithStatus(snap.Status)
	}

	if shouldSettle && actualQuota > 0 {
		RecalculateTaskQuota(ctx, task, actualQuota, "test settle")
	}
	if shouldRefund {
		RefundTaskQuota(ctx, task, task.FailReason)
	}
}

func TestCASGuardedRefund_Win(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID = 20, 20, 20
	const initQuota, preConsumed = 10000, 4000
	const tokenRemain = 6000

	seedUser(t, userID, initQuota)
	seedToken(t, tokenID, userID, "sk-cas-refund-win", tokenRemain)
	seedChannel(t, channelID)

	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceWallet, 0)
	task.Status = model.TaskStatus(model.TaskStatusInProgress)
	require.NoError(t, model.DB.Create(task).Error)

	simulatePollBilling(ctx, task, model.TaskStatus(model.TaskStatusFailure), 0)

	var reloaded model.Task
	require.NoError(t, model.DB.First(&reloaded, task.ID).Error)
	assert.EqualValues(t, model.TaskStatusFailure, reloaded.Status)
	assert.Equal(t, initQuota, getUserQuota(t, userID))
	assert.Equal(t, tokenRemain, getTokenRemainQuota(t, tokenID))
	assert.Equal(t, int64(0), countLogs(t))
}

func TestCASGuardedRefund_Lose(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID = 21, 21, 21
	const initQuota, preConsumed = 10000, 4000
	const tokenRemain = 6000

	seedUser(t, userID, initQuota)
	seedToken(t, tokenID, userID, "sk-cas-refund-lose", tokenRemain)
	seedChannel(t, channelID)

	// Create task with IN_PROGRESS in DB
	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceWallet, 0)
	task.Status = model.TaskStatus(model.TaskStatusInProgress)
	require.NoError(t, model.DB.Create(task).Error)

	// Simulate another process already transitioning to FAILURE
	model.DB.Model(&model.Task{}).Where("id = ?", task.ID).Update("status", model.TaskStatusFailure)

	// Our process still has the old in-memory state (IN_PROGRESS) and tries to transition
	// task.Status is still IN_PROGRESS in the snapshot
	simulatePollBilling(ctx, task, model.TaskStatus(model.TaskStatusFailure), 0)

	// CAS lost: user quota should NOT change (no double refund)
	assert.Equal(t, initQuota, getUserQuota(t, userID))
	assert.Equal(t, tokenRemain, getTokenRemainQuota(t, tokenID))

	// No billing log should be created
	assert.Equal(t, int64(0), countLogs(t))
}

func TestCASGuardedSettle_Win(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID = 22, 22, 22
	const initQuota, preConsumed = 10000, 5000
	const actualQuota = 3000
	const tokenRemain = 8000

	seedUser(t, userID, initQuota)
	seedToken(t, tokenID, userID, "sk-cas-settle-win", tokenRemain)
	seedChannel(t, channelID)

	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceWallet, 0)
	task.Status = model.TaskStatus(model.TaskStatusInProgress)
	require.NoError(t, model.DB.Create(task).Error)

	simulatePollBilling(ctx, task, model.TaskStatus(model.TaskStatusSuccess), actualQuota)

	var reloaded model.Task
	require.NoError(t, model.DB.First(&reloaded, task.ID).Error)
	assert.EqualValues(t, model.TaskStatusSuccess, reloaded.Status)
	assert.Equal(t, initQuota, getUserQuota(t, userID))
	assert.Equal(t, tokenRemain, getTokenRemainQuota(t, tokenID))
	assert.Equal(t, preConsumed, task.Quota)
	assert.Equal(t, int64(0), countLogs(t))
}

func TestNonTerminalUpdate_NoBilling(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, channelID = 23, 23
	const initQuota, preConsumed = 10000, 3000

	seedUser(t, userID, initQuota)
	seedChannel(t, channelID)

	task := makeTask(userID, channelID, preConsumed, 0, BillingSourceWallet, 0)
	task.Status = model.TaskStatus(model.TaskStatusInProgress)
	task.Progress = "20%"
	require.NoError(t, model.DB.Create(task).Error)

	// Simulate a non-terminal poll update (still IN_PROGRESS, progress changed)
	simulatePollBilling(ctx, task, model.TaskStatus(model.TaskStatusInProgress), 0)

	// User quota should NOT change
	assert.Equal(t, initQuota, getUserQuota(t, userID))

	// No billing log
	assert.Equal(t, int64(0), countLogs(t))

	// Task progress should be updated in DB
	var reloaded model.Task
	require.NoError(t, model.DB.First(&reloaded, task.ID).Error)
	assert.Equal(t, "50%", reloaded.Progress)
}

// ===========================================================================
// Mock adaptor for settleTaskBillingOnComplete tests
// ===========================================================================

type mockAdaptor struct {
	adjustReturn int
}

func (m *mockAdaptor) Init(_ *relaycommon.RelayInfo) {}
func (m *mockAdaptor) FetchTask(string, string, map[string]any, string) (*http.Response, error) {
	return nil, nil
}
func (m *mockAdaptor) ParseTaskResult([]byte) (*relaycommon.TaskInfo, error) { return nil, nil }
func (m *mockAdaptor) AdjustBillingOnComplete(_ *model.Task, _ *relaycommon.TaskInfo) int {
	return m.adjustReturn
}

// ===========================================================================
// PerCallBilling tests — settleTaskBillingOnComplete
// ===========================================================================

func TestSettle_PerCallBilling_SkipsAdaptorAdjust(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID = 30, 30, 30
	const initQuota, preConsumed = 10000, 5000
	const tokenRemain = 8000

	seedUser(t, userID, initQuota)
	seedToken(t, tokenID, userID, "sk-percall-adaptor", tokenRemain)
	seedChannel(t, channelID)

	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceWallet, 0)
	task.PrivateData.BillingContext.PerCallBilling = true

	adaptor := &mockAdaptor{adjustReturn: 2000}
	taskResult := &relaycommon.TaskInfo{Status: model.TaskStatusSuccess}

	settleTaskBillingOnComplete(ctx, adaptor, task, taskResult)

	// Per-call: no adjustment despite adaptor returning 2000
	assert.Equal(t, initQuota, getUserQuota(t, userID))
	assert.Equal(t, tokenRemain, getTokenRemainQuota(t, tokenID))
	assert.Equal(t, preConsumed, task.Quota)
	assert.Equal(t, int64(0), countLogs(t))
}

func TestSettle_PerCallBilling_SkipsTotalTokens(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID = 31, 31, 31
	const initQuota, preConsumed = 10000, 4000
	const tokenRemain = 7000

	seedUser(t, userID, initQuota)
	seedToken(t, tokenID, userID, "sk-percall-tokens", tokenRemain)
	seedChannel(t, channelID)

	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceWallet, 0)
	task.PrivateData.BillingContext.PerCallBilling = true

	adaptor := &mockAdaptor{adjustReturn: 0}
	taskResult := &relaycommon.TaskInfo{Status: model.TaskStatusSuccess, TotalTokens: 9999}

	settleTaskBillingOnComplete(ctx, adaptor, task, taskResult)

	// Per-call: no recalculation by tokens
	assert.Equal(t, initQuota, getUserQuota(t, userID))
	assert.Equal(t, tokenRemain, getTokenRemainQuota(t, tokenID))
	assert.Equal(t, preConsumed, task.Quota)
	assert.Equal(t, int64(0), countLogs(t))
}

func TestSettle_NonPerCall_AdaptorAdjustWorks(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID = 32, 32, 32
	const initQuota, preConsumed = 10000, 5000
	const adaptorQuota = 3000
	const tokenRemain = 8000

	seedUser(t, userID, initQuota)
	seedToken(t, tokenID, userID, "sk-nonpercall-adj", tokenRemain)
	seedChannel(t, channelID)

	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceWallet, 0)

	adaptor := &mockAdaptor{adjustReturn: adaptorQuota}
	taskResult := &relaycommon.TaskInfo{Status: model.TaskStatusSuccess}

	settleTaskBillingOnComplete(ctx, adaptor, task, taskResult)

	assert.Equal(t, initQuota, getUserQuota(t, userID))
	assert.Equal(t, tokenRemain, getTokenRemainQuota(t, tokenID))
	assert.Equal(t, preConsumed, task.Quota)
	assert.Equal(t, int64(0), countLogs(t))
}

func TestLegacyTaskRefundAndRecalculateDoNotWriteAccountBalance(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID = 40, 40, 40
	const initQuota, preConsumed = 4000, 500000
	const tokenRemain = 900000

	seedUser(t, userID, initQuota)
	seedToken(t, tokenID, userID, "sk-legacy-task", tokenRemain)
	seedChannel(t, channelID)

	refundTask := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceWallet, 0)
	RefundTaskQuota(ctx, refundTask, "legacy wallet refund")
	assert.Equal(t, initQuota, getUserQuota(t, userID))
	assert.Equal(t, tokenRemain, getTokenRemainQuota(t, tokenID))
	assert.Equal(t, int64(0), countLogs(t))

	recalculateTask := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceWallet, 0)
	RecalculateTaskQuota(ctx, recalculateTask, preConsumed+1000, "legacy wallet recalculate")
	assert.Equal(t, initQuota, getUserQuota(t, userID))
	assert.Equal(t, tokenRemain, getTokenRemainQuota(t, tokenID))
	assert.Equal(t, preConsumed, recalculateTask.Quota)
	assert.Equal(t, int64(0), countLogs(t))
}
