package router

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type subscriptionPlansPublicRouteResponse struct {
	Success bool `json:"success"`
	Data    []struct {
		Plan map[string]any `json:"plan"`
	} `json:"data"`
}

func TestSubscriptionPlansPublicRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupSubscriptionPublicPlansRouteTestDB(t)
	seedSubscriptionPublicPlanRouteTestPlans(t)

	engine := gin.New()
	engine.Use(sessions.Sessions("session", cookie.NewStore([]byte("secret"))))
	SetApiRouter(engine)

	publicRecorder := httptest.NewRecorder()
	publicReq := httptest.NewRequest(http.MethodGet, "/api/subscription/public/plans", nil)
	engine.ServeHTTP(publicRecorder, publicReq)
	require.Equal(t, http.StatusOK, publicRecorder.Code)

	var payload subscriptionPlansPublicRouteResponse
	require.NoError(t, common.Unmarshal(publicRecorder.Body.Bytes(), &payload))
	require.True(t, payload.Success)
	require.Len(t, payload.Data, 2)

	allowedPlanKeys := map[string]struct{}{
		"id":                         {},
		"title":                      {},
		"subtitle":                   {},
		"price_amount":               {},
		"price_amount_micros":        {},
		"currency":                   {},
		"duration_unit":              {},
		"duration_value":             {},
		"custom_seconds":             {},
		"monthly_token_limit":        {},
		"concurrency_limit":          {},
		"queue_capacity":             {},
		"public_visible":             {},
		"gpt_abuse_warning_limit":    {},
		"channel_token_equivalents":  {},
		"channel_credit_equivalents": {},
		"kyren_product_id":           {},
	}

	assert.Equal(t, "Public High", payload.Data[0].Plan["title"])
	assert.Equal(t, "Public Low", payload.Data[1].Plan["title"])
	for _, record := range payload.Data {
		require.Len(t, record.Plan, len(allowedPlanKeys))
		for key := range record.Plan {
			_, ok := allowedPlanKeys[key]
			assert.Truef(t, ok, "unexpected public plan key %q", key)
		}
		for key := range allowedPlanKeys {
			_, ok := record.Plan[key]
			assert.Truef(t, ok, "missing public plan key %q", key)
		}
	}

	kyrenProductByTitle := map[string]string{
		"Public High": "prod_public_high",
		"Public Low":  "prod_public_low",
	}
	for _, record := range payload.Data {
		title, ok := record.Plan["title"].(string)
		require.True(t, ok)
		assert.Equal(t, kyrenProductByTitle[title], record.Plan["kyren_product_id"])

		equivalents, ok := record.Plan["channel_credit_equivalents"].([]any)
		require.True(t, ok, "public plan channel_credit_equivalents must be an array")
		require.Len(t, equivalents, 1)
		equivalent, ok := equivalents[0].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "usage_tokens", equivalent["kind"])
		assert.Equal(t, "single", equivalent["value_type"])
		assert.Equal(t, float64(constant.ChannelTypeOpenAI), equivalent["channel_type"])
		assert.Equal(t, float64(2), equivalent["multiplier"])
	}

	body := publicRecorder.Body.String()
	assert.NotContains(t, body, "Hidden Plan")
	assert.NotContains(t, body, "Disabled Plan")
	assert.NotContains(t, body, "Trial Plan")
	assert.NotContains(t, body, "stripe_price_id")
	assert.NotContains(t, body, "creem_product_id")
	assert.NotContains(t, body, "max_purchase_per_user")
	assert.NotContains(t, body, "upgrade_group")
	assert.NotContains(t, body, "business_code")
	assert.NotContains(t, body, "reward_eligible")
	assert.NotContains(t, body, "total_amount")
	assert.NotContains(t, body, "enabled")
	assert.NotContains(t, body, "sort_order")
	assert.NotContains(t, body, "is_trial")
	assert.NotContains(t, body, "invite_trial")
	assert.NotContains(t, body, "trial_duration_hours")
	assert.NotContains(t, body, "quota_reset_period")
	assert.NotContains(t, body, "quota_reset_custom_seconds")
	assert.NotContains(t, body, "created_at")
	assert.NotContains(t, body, "updated_at")

	protectedPlansRecorder := httptest.NewRecorder()
	protectedPlansReq := httptest.NewRequest(http.MethodGet, "/api/subscription/plans", nil)
	engine.ServeHTTP(protectedPlansRecorder, protectedPlansReq)
	require.Equal(t, http.StatusUnauthorized, protectedPlansRecorder.Code)

	selfRecorder := httptest.NewRecorder()
	selfReq := httptest.NewRequest(http.MethodGet, "/api/subscription/self", nil)
	engine.ServeHTTP(selfRecorder, selfReq)
	require.Equal(t, http.StatusUnauthorized, selfRecorder.Code)
}

func TestSubscriptionPlansProtectedDTOOmitsLegacyBusinessGroupFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupSubscriptionPublicPlansRouteTestDB(t)
	seedSubscriptionPublicPlanRouteTestPlans(t)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/subscription/plans", nil)
	controller.GetSubscriptionPlans(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	body := recorder.Body.String()
	assert.Contains(t, body, "stripe_price_id")
	assert.Contains(t, body, "creem_product_id")
	assert.Contains(t, body, "max_purchase_per_user")
	assert.NotContains(t, body, "upgrade_group")
	assert.Contains(t, body, "business_code")
	assert.Contains(t, body, "reward_eligible")
}

func setupSubscriptionPublicPlansRouteTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	oldDB := model.DB
	oldLogDB := model.LOG_DB
	oldUsingSQLite := common.UsingSQLite
	oldUsingMySQL := common.UsingMySQL
	oldUsingPostgreSQL := common.UsingPostgreSQL
	oldRedisEnabled := common.RedisEnabled
	oldGlobalApiRateLimitEnable := common.GlobalApiRateLimitEnable

	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false
	common.GlobalApiRateLimitEnable = false

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	model.LOG_DB = db
	require.NoError(t, db.AutoMigrate(&model.SubscriptionPlan{}, &model.TimedSubscriptionValuationGrant{}, &model.SubscriptionConversionQuote{}, &model.Channel{}, &model.ChannelGroup{}, &model.ChannelGroupChannel{}))

	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
		model.DB = oldDB
		model.LOG_DB = oldLogDB
		common.UsingSQLite = oldUsingSQLite
		common.UsingMySQL = oldUsingMySQL
		common.UsingPostgreSQL = oldUsingPostgreSQL
		common.RedisEnabled = oldRedisEnabled
		common.GlobalApiRateLimitEnable = oldGlobalApiRateLimitEnable
	})

	return db
}

func seedSubscriptionPublicPlanRouteTestPlans(t *testing.T) {
	t.Helper()

	require.NoError(t, model.DB.Create(&model.Channel{Id: 9181, Type: constant.ChannelTypeOpenAI, Key: "sk-public-plans", Status: common.ChannelStatusEnabled, Name: "public-plans-openai", Models: "gpt-test", TokenBillingMultiplier: 2}).Error)
	require.NoError(t, model.DB.Create(&model.ChannelGroup{Id: constant.ChannelTypeOpenAI, Name: "OpenAI", Enabled: true}).Error)
	require.NoError(t, model.DB.Create(&model.ChannelGroupChannel{ChannelGroupId: constant.ChannelTypeOpenAI, ChannelId: 9181}).Error)
	createSubscriptionPublicPlanRouteTestPlan(t, model.SubscriptionPlan{
		Id:                 9101,
		Title:              "Public Low",
		Subtitle:           "Lower public plan",
		PriceAmount:        9.9,
		Currency:           "CNY",
		DurationUnit:       model.SubscriptionDurationMonth,
		DurationValue:      1,
		MonthlyTokenLimit:  100000,
		ConcurrencyLimit:   2,
		QueueCapacity:      8,
		Enabled:            true,
		PublicVisible:      true,
		SortOrder:          10,
		StripePriceId:      "price_public_low",
		CreemProductId:     "creem_public_low",
		KyrenProductId:     "prod_public_low",
		MaxPurchasePerUser: 3,
		UpgradeGroup:       "vip-low",
		TotalAmount:        1000,
		RewardEligible:     true,
		BusinessCode:       subscriptionPublicPlanRouteTestBusinessCode("public_low"),
	}, true, true, false, true)
	createSubscriptionPublicPlanRouteTestPlan(t, model.SubscriptionPlan{
		Id:                 9102,
		Title:              "Public High",
		Subtitle:           "Higher public plan",
		PriceAmount:        19.9,
		Currency:           "CNY",
		DurationUnit:       model.SubscriptionDurationMonth,
		DurationValue:      1,
		MonthlyTokenLimit:  200000,
		ConcurrencyLimit:   4,
		QueueCapacity:      16,
		Enabled:            true,
		PublicVisible:      true,
		SortOrder:          20,
		StripePriceId:      "price_public_high",
		CreemProductId:     "creem_public_high",
		KyrenProductId:     "prod_public_high",
		MaxPurchasePerUser: 5,
		UpgradeGroup:       "vip-high",
		TotalAmount:        2000,
		RewardEligible:     true,
		BusinessCode:       subscriptionPublicPlanRouteTestBusinessCode("public_high"),
	}, true, true, false, true)
	createSubscriptionPublicPlanRouteTestPlan(t, model.SubscriptionPlan{
		Id:                9103,
		Title:             "Hidden Plan",
		Subtitle:          "Hidden from public listing",
		PriceAmount:       29.9,
		Currency:          "CNY",
		DurationUnit:      model.SubscriptionDurationMonth,
		DurationValue:     1,
		MonthlyTokenLimit: 300000,
		ConcurrencyLimit:  6,
		Enabled:           true,
		PublicVisible:     false,
		SortOrder:         30,
		RewardEligible:    true,
		BusinessCode:      subscriptionPublicPlanRouteTestBusinessCode("hidden"),
	}, true, false, false, true)
	createSubscriptionPublicPlanRouteTestPlan(t, model.SubscriptionPlan{
		Id:                9104,
		Title:             "Disabled Plan",
		Subtitle:          "Disabled from public listing",
		PriceAmount:       39.9,
		Currency:          "CNY",
		DurationUnit:      model.SubscriptionDurationMonth,
		DurationValue:     1,
		MonthlyTokenLimit: 400000,
		ConcurrencyLimit:  8,
		Enabled:           false,
		PublicVisible:     true,
		SortOrder:         40,
		RewardEligible:    true,
		BusinessCode:      subscriptionPublicPlanRouteTestBusinessCode("disabled"),
	}, false, true, false, true)
	createSubscriptionPublicPlanRouteTestPlan(t, model.SubscriptionPlan{
		Id:                9105,
		Title:             "Trial Plan",
		Subtitle:          "Trial from public listing",
		PriceAmount:       0,
		Currency:          "CNY",
		DurationUnit:      model.SubscriptionDurationDay,
		DurationValue:     7,
		MonthlyTokenLimit: 50000,
		ConcurrencyLimit:  1,
		Enabled:           true,
		PublicVisible:     true,
		SortOrder:         50,
		IsTrial:           true,
		RewardEligible:    true,
		BusinessCode:      subscriptionPublicPlanRouteTestBusinessCode("trial"),
	}, true, true, true, true)
}

func createSubscriptionPublicPlanRouteTestPlan(t *testing.T, plan model.SubscriptionPlan, enabled bool, publicVisible bool, isTrial bool, rewardEligible bool) {
	t.Helper()

	require.NoError(t, model.DB.Create(&plan).Error)
	require.NoError(t, model.DB.Model(&plan).Updates(map[string]interface{}{
		"enabled":         enabled,
		"public_visible":  publicVisible,
		"is_trial":        isTrial,
		"reward_eligible": rewardEligible,
	}).Error)
}

func subscriptionPublicPlanRouteTestBusinessCode(code string) *string {
	return &code
}
