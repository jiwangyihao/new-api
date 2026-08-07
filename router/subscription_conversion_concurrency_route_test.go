package router

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestSubscriptionConversionRouteConcurrentDifferentKeysConvertsSourceOnce(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupSubscriptionConversionConcurrentRouteTestDB(t)

	const userID = 9_991
	const sourceID = 9_992
	const creditPlanID = 9_993
	const timedPlanID = 9_994
	accessToken := "subscription-conversion-concurrent-token"
	settingBytes, err := common.Marshal(map[string]any{
		"active_subscription_id":        sourceID,
		"subscription_billing_strategy": model.SubscriptionBillingStrategySingleActive,
	})
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.User{
		Id: userID, Username: "subscription-conversion-concurrent", Status: common.UserStatusEnabled,
		Role: common.RoleCommonUser, AccessToken: &accessToken, Setting: string(settingBytes),
	}).Error)
	creditCode := "subscription_conversion_concurrent_credit"
	require.NoError(t, db.Create(&model.SubscriptionPlan{
		Id: creditPlanID, Title: "Credit balance", EntitlementType: model.SubscriptionEntitlementCreditBalance,
		Enabled: true, BusinessCode: &creditCode, CreditBalanceConfigured: true, CreditBalanceConversionEnabled: true,
	}).Error)
	timedCode := "subscription_conversion_concurrent_timed"
	require.NoError(t, db.Create(&model.SubscriptionPlan{
		Id: timedPlanID, Title: "Monthly convertible", EntitlementType: model.SubscriptionEntitlementTimed,
		Enabled: true, BusinessCode: &timedCode, DurationUnit: model.SubscriptionDurationMonth, DurationValue: 1,
		QuotaResetPeriod: model.SubscriptionResetMonthly, MonthlyTokenLimit: 100, TimedConversionEnabled: true,
	}).Error)
	now := model.GetDBTimestamp()
	basis := int64(100)
	require.NoError(t, db.Create(&model.UserSubscription{
		Id: sourceID, UserId: userID, PlanId: timedPlanID, EntitlementType: model.SubscriptionEntitlementTimed,
		TokenLimit: 100, GrantReason: model.SubscriptionGrantOrder, Source: model.SubscriptionGrantOrder,
		StartTime: now - 40*24*60*60, EndTime: now + model.TimedSubscriptionConversionBlockSeconds + 60, Status: "active",
		LastGrantedAt: now - 40*24*60*60, LastGrantCreditSnapshot: &basis,
		LastGrantTimeSource: model.SubscriptionGrantTimeSourceLive, LastGrantSource: model.SubscriptionGrantOrder,
	}).Error)

	engine := gin.New()
	engine.Use(sessions.Sessions("session", cookie.NewStore([]byte("secret"))))
	SetApiRouter(engine)

	type outcome struct {
		response subscriptionConversionRouteResponse
		err      error
	}
	start := make(chan struct{})
	outcomes := make(chan outcome, 2)
	var waitGroup sync.WaitGroup
	for _, key := range []string{"concurrent-key-a", "concurrent-key-b"} {
		waitGroup.Add(1)
		go func(idempotencyKey string) {
			defer waitGroup.Done()
			<-start
			body := fmt.Sprintf(`{"subscription_id":"%d","idempotency_key":"%s"}`, sourceID, idempotencyKey)
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/api/subscription/self/conversions", bytes.NewBufferString(body))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Authorization", "Bearer "+accessToken)
			request.Header.Set("New-Api-User", strconv.Itoa(userID))
			engine.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusOK {
				outcomes <- outcome{err: fmt.Errorf("unexpected status %d: %s", recorder.Code, recorder.Body.String())}
				return
			}
			var response subscriptionConversionRouteResponse
			if err := common.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				outcomes <- outcome{err: err}
				return
			}
			outcomes <- outcome{response: response}
		}(key)
	}
	close(start)
	waitGroup.Wait()
	close(outcomes)

	successes := 0
	failures := 0
	for result := range outcomes {
		require.NoError(t, result.err)
		if result.response.Success {
			successes++
			assert.False(t, result.response.Data.Replayed)
			continue
		}
		failures++
		assert.Equal(t, "subscription_conversion_idempotency_conflict", result.response.Code)
	}
	assert.Equal(t, 1, successes)
	assert.Equal(t, 1, failures)

	var conversionCount int64
	require.NoError(t, db.Model(&model.SubscriptionConversion{}).Where("source_subscription_id = ?", sourceID).Count(&conversionCount).Error)
	assert.Equal(t, int64(1), conversionCount)
	var ledgerCount int64
	require.NoError(t, db.Model(&model.CreditBalanceLedger{}).
		Where("source_type = ? AND source_id = ?", model.CreditBalanceLedgerSourceSubscriptionConversion, sourceID).
		Count(&ledgerCount).Error)
	assert.Equal(t, int64(1), ledgerCount)
	var balances []model.UserSubscription
	require.NoError(t, db.Where("user_id = ? AND entitlement_type = ?", userID, model.SubscriptionEntitlementCreditBalance).Find(&balances).Error)
	require.Len(t, balances, 1)
	assert.Equal(t, int64(200), balances[0].TokenLimit)
	assert.Zero(t, balances[0].TokenUsed)
	var source model.UserSubscription
	require.NoError(t, db.First(&source, sourceID).Error)
	assert.Equal(t, model.SubscriptionStatusConverted, source.Status)
	assert.Equal(t, balances[0].Id, source.ConvertedToSubscriptionId)
}

func setupSubscriptionConversionConcurrentRouteTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	oldDB := model.DB
	oldLogDB := model.LOG_DB
	oldUsingSQLite := common.UsingSQLite
	oldUsingMySQL := common.UsingMySQL
	oldUsingPostgreSQL := common.UsingPostgreSQL
	oldRedisEnabled := common.RedisEnabled
	oldGlobalAPIRateLimitEnable := common.GlobalApiRateLimitEnable

	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false
	common.GlobalApiRateLimitEnable = false
	dsn := "file:" + filepath.ToSlash(filepath.Join(t.TempDir(), "subscription-conversion-race.db")) + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(4)
	model.DB = db
	model.LOG_DB = db
	require.NoError(t, db.AutoMigrate(
		&model.SubscriptionPlan{},
		&model.User{},
		&model.Token{},
		&model.UserSubscription{},
		&model.SubscriptionOrder{},
		&model.Redemption{},
		&model.InvitationRewardEvent{},
		&model.CreditBalanceLedger{},
		&model.SubscriptionConversion{},
	))
	model.ClearPrimaryBillableSubscriptionCacheForTest()
	model.ClearSubscriptionPlanCacheForTest()
	t.Cleanup(func() {
		model.ClearPrimaryBillableSubscriptionCacheForTest()
		model.ClearSubscriptionPlanCacheForTest()
		model.DB = oldDB
		model.LOG_DB = oldLogDB
		common.UsingSQLite = oldUsingSQLite
		common.UsingMySQL = oldUsingMySQL
		common.UsingPostgreSQL = oldUsingPostgreSQL
		common.RedisEnabled = oldRedisEnabled
		common.GlobalApiRateLimitEnable = oldGlobalAPIRateLimitEnable
		_ = sqlDB.Close()
	})
	return db
}
