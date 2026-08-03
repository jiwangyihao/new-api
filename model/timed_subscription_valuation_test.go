package model

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestTimedSubscriptionValuationGrantCreatesTimelineAndReplaysSource(t *testing.T) {
	setupTimedSubscriptionValuationTestDB(t)
	priceMicros := int64(40_000_000)
	user := User{Id: 21_001, Username: "timed-grant", Status: common.UserStatusEnabled, AffCode: "timed-grant-aff"}
	plan := SubscriptionPlan{
		Id: 21_002, Title: "Timed Basic", Enabled: true,
		EntitlementType: SubscriptionEntitlementTimed,
		PriceAmount:     40, PriceAmountMicros: &priceMicros, Currency: "CNY",
		DurationUnit: SubscriptionDurationCustom, CustomSeconds: 3600,
		MonthlyTokenLimit: 1000, QuotaResetPeriod: SubscriptionResetNever,
	}
	require.NoError(t, DB.Create(&user).Error)
	require.NoError(t, DB.Create(&plan).Error)

	request := TimedSubscriptionGrantRequest{
		UserId:            user.Id,
		Plan:              &plan,
		IdempotencyKey:    "subscription-order:21003",
		SourceType:        TimedSubscriptionGrantSourceOrder,
		SourceId:          21_003,
		SourcePriceMicros: priceMicros,
		SourceCurrency:    "CNY",
	}
	var first *UserSubscriptionCreationResult
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		var err error
		first, err = GrantTimedSubscriptionTx(tx, request)
		return err
	}))
	require.NotNil(t, first)
	require.NotNil(t, first.Subscription)
	require.Equal(t, int64(3600), first.EventEndTime-first.EventStartTime)

	var grant TimedSubscriptionValuationGrant
	require.NoError(t, DB.Where("source_type = ? AND source_key = ?", "subscription_order", "subscription_order:21003").First(&grant).Error)
	require.Equal(t, first.Subscription.Id, grant.UserSubscriptionId)
	require.Equal(t, user.Id, grant.UserId)
	require.Equal(t, plan.Id, grant.PlanId)
	require.Equal(t, first.EventStartTime, grant.EventStartTime)
	require.Equal(t, first.EventEndTime, grant.EventEndTime)
	require.Equal(t, int64(1000), grant.GrantCredit)
	require.Equal(t, priceMicros, grant.SourcePriceMicros)
	require.Equal(t, priceMicros, grant.ValuationAmountMicros)
	require.Equal(t, "CNY", grant.SourceCurrency)
	require.Equal(t, "CNY", grant.ValuationCurrency)
	require.Equal(t, "exact", grant.Confidence)
	require.Equal(t, CreditValuationRuleVersion, grant.RuleVersion)
	require.Equal(t, int64(1), grant.FxRateNumerator)
	require.Equal(t, int64(1), grant.FxRateDenominator)
	require.NotZero(t, grant.CreatedAt)
	require.False(t, strings.TrimSpace(grant.SourceSnapshot) == "")

	originalEndTime := first.Subscription.EndTime
	var replay *UserSubscriptionCreationResult
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		var err error
		replay, err = GrantTimedSubscriptionTx(tx, request)
		return err
	}))
	require.NotNil(t, replay)
	require.NotNil(t, replay.Subscription)
	require.Equal(t, first.Subscription.Id, replay.Subscription.Id)
	require.Equal(t, first.EventStartTime, replay.EventStartTime)
	require.Equal(t, first.EventEndTime, replay.EventEndTime)

	var persisted UserSubscription
	require.NoError(t, DB.First(&persisted, first.Subscription.Id).Error)
	require.Equal(t, originalEndTime, persisted.EndTime)
	var grantCount int64
	require.NoError(t, DB.Model(&TimedSubscriptionValuationGrant{}).Count(&grantCount).Error)
	require.Equal(t, int64(1), grantCount)
}

func TestTimedSubscriptionValuationGrantRejectsConflictAndAppendsRenewal(t *testing.T) {
	setupTimedSubscriptionValuationTestDB(t)
	priceMicros := int64(40_000_000)
	plan := SubscriptionPlan{
		Id: 21_102, Title: "Timed Pro", Enabled: true,
		EntitlementType: SubscriptionEntitlementTimed,
		DurationUnit:    SubscriptionDurationCustom, CustomSeconds: 3600,
		MonthlyTokenLimit: 1000, QuotaResetPeriod: SubscriptionResetNever,
	}
	require.NoError(t, DB.Create(&User{Id: 21_101, Username: "timed-renew", Status: common.UserStatusEnabled, AffCode: "timed-renew-aff"}).Error)
	require.NoError(t, DB.Create(&plan).Error)

	firstRequest := TimedSubscriptionGrantRequest{
		UserId: 21_101, Plan: &plan, IdempotencyKey: "subscription-order:21103",
		SourceType: TimedSubscriptionGrantSourceOrder, SourceId: 21_103,
		SourcePriceMicros: priceMicros, SourceCurrency: "CNY",
	}
	var first *UserSubscriptionCreationResult
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		var err error
		first, err = GrantTimedSubscriptionTx(tx, firstRequest)
		return err
	}))

	conflict := firstRequest
	conflict.SourcePriceMicros = 50_000_000
	err := DB.Transaction(func(tx *gorm.DB) error {
		_, grantErr := GrantTimedSubscriptionTx(tx, conflict)
		return grantErr
	})
	require.ErrorIs(t, err, ErrTimedSubscriptionGrantIdempotencyMismatch)
	var afterConflict UserSubscription
	require.NoError(t, DB.First(&afterConflict, first.Subscription.Id).Error)
	require.Equal(t, first.EventEndTime, afterConflict.EndTime)

	renewalRequest := firstRequest
	renewalRequest.IdempotencyKey = "subscription-order:21104"
	renewalRequest.SourceId = 21_104
	renewalRequest.SourcePriceMicros = 50_000_000
	renewalRequest.SourceCurrency = "USD"
	var renewal *UserSubscriptionCreationResult
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		var err error
		renewal, err = GrantTimedSubscriptionTx(tx, renewalRequest)
		return err
	}))
	require.Equal(t, first.Subscription.Id, renewal.Subscription.Id)
	require.Equal(t, first.EventEndTime, renewal.EventStartTime)
	require.Equal(t, int64(3600), renewal.EventEndTime-renewal.EventStartTime)

	var grants []TimedSubscriptionValuationGrant
	require.NoError(t, DB.Order("id asc").Find(&grants).Error)
	require.Len(t, grants, 2)
	require.Equal(t, []string{"CNY", "USD"}, []string{grants[0].ValuationCurrency, grants[1].ValuationCurrency})
	require.Equal(t, []int64{40_000_000, 50_000_000}, []int64{grants[0].ValuationAmountMicros, grants[1].ValuationAmountMicros})
}

func TestTimedSubscriptionValuationGrantOrderCompletionCreatesGrant(t *testing.T) {
	setupTimedSubscriptionValuationTestDB(t)
	priceMicros := int64(40_000_000)
	plan := SubscriptionPlan{
		Id: 21_202, Title: "Timed Order", Enabled: true,
		EntitlementType: SubscriptionEntitlementTimed,
		PriceAmount:     40, PriceAmountMicros: &priceMicros, Currency: "CNY",
		DurationUnit: SubscriptionDurationCustom, CustomSeconds: 3600,
		MonthlyTokenLimit: 1000, QuotaResetPeriod: SubscriptionResetNever,
	}
	require.NoError(t, DB.Create(&User{Id: 21_201, Username: "timed-order", Status: common.UserStatusEnabled, AffCode: "timed-order-aff"}).Error)
	require.NoError(t, DB.Create(&plan).Error)
	snapshot := NewSubscriptionEntitlementSnapshot(&plan, SubscriptionPurchaseModeTimed, 0)
	snapshot.SetPaymentSnapshot(PaymentProviderBalance, "balance", PaymentMethodAccountBalance, 4000, "CNY")
	snapshotJSON, err := MarshalSubscriptionEntitlementSnapshot(snapshot)
	require.NoError(t, err)
	order := SubscriptionOrder{
		Id: 21_203, UserId: 21_201, PlanId: plan.Id, Money: 40, AmountCents: 4000, Currency: "CNY",
		TradeNo: "timed-order-21203", PaymentProvider: PaymentProviderBalance, PaymentMethod: PaymentMethodAccountBalance,
		Status: common.TopUpStatusPending, CreateTime: common.GetTimestamp(), EntitlementSnapshot: snapshotJSON,
	}
	require.NoError(t, DB.Create(&order).Error)

	var result *SubscriptionOrderCompletionResult
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		var locked SubscriptionOrder
		if err := tx.Where("id = ?", order.Id).First(&locked).Error; err != nil {
			return err
		}
		var completeErr error
		result, completeErr = CompleteSubscriptionOrderTx(tx, &locked, "", PaymentMethodAccountBalance)
		return completeErr
	}))
	require.NotNil(t, result)
	require.NotNil(t, result.Subscription)

	var grant TimedSubscriptionValuationGrant
	require.NoError(t, DB.Where("source_type = ? AND source_key = ?", TimedSubscriptionGrantSourceOrder, "subscription_order:21203").First(&grant).Error)
	require.Equal(t, result.Subscription.Id, grant.UserSubscriptionId)
	require.Equal(t, snapshot.ListPriceMicros, &grant.SourcePriceMicros)
	require.Equal(t, snapshot.ListPriceCurrency, grant.SourceCurrency)
	require.Equal(t, result.EventStartTime, grant.EventStartTime)
	require.Equal(t, result.EventEndTime, grant.EventEndTime)
}

func TestTimedSubscriptionValuationGrantPaidOrderWithoutSnapshotRejectsAtomically(t *testing.T) {
	setupTimedSubscriptionValuationTestDB(t)
	priceMicros := int64(40_000_000)
	plan := SubscriptionPlan{
		Id: 21_302, Title: "Legacy Paid Timed", Enabled: true,
		EntitlementType: SubscriptionEntitlementTimed,
		PriceAmount:     40, PriceAmountMicros: &priceMicros, Currency: "CNY",
		DurationUnit: SubscriptionDurationCustom, CustomSeconds: 3600,
		MonthlyTokenLimit: 1000, QuotaResetPeriod: SubscriptionResetNever,
	}
	require.NoError(t, DB.Create(&User{Id: 21_301, Username: "timed-no-snapshot", Status: common.UserStatusEnabled, AffCode: "timed-no-snapshot-aff"}).Error)
	require.NoError(t, DB.Create(&plan).Error)
	order := SubscriptionOrder{
		Id: 21_303, UserId: 21_301, PlanId: plan.Id, Money: 40, AmountCents: 4000, Currency: "CNY",
		TradeNo: "timed-no-snapshot-21303", PaymentProvider: PaymentProviderBalance, PaymentMethod: PaymentMethodAccountBalance,
		Status: common.TopUpStatusPending, CreateTime: common.GetTimestamp(),
	}
	require.NoError(t, DB.Create(&order).Error)

	err := DB.Transaction(func(tx *gorm.DB) error {
		var locked SubscriptionOrder
		if err := tx.Where("id = ?", order.Id).First(&locked).Error; err != nil {
			return err
		}
		_, completeErr := CompleteSubscriptionOrderTx(tx, &locked, "", PaymentMethodAccountBalance)
		return completeErr
	})
	require.ErrorIs(t, err, ErrTimedSubscriptionGrantInvalid)

	var persisted SubscriptionOrder
	require.NoError(t, DB.First(&persisted, order.Id).Error)
	require.Equal(t, common.TopUpStatusPending, persisted.Status)
	var subscriptionCount int64
	require.NoError(t, DB.Model(&UserSubscription{}).Count(&subscriptionCount).Error)
	require.Zero(t, subscriptionCount)
	var grantCount int64
	require.NoError(t, DB.Model(&TimedSubscriptionValuationGrant{}).Count(&grantCount).Error)
	require.Zero(t, grantCount)
}

func TestTimedSubscriptionValuationGrantExplicitTrialOrderCreatesNoGrant(t *testing.T) {
	setupTimedSubscriptionValuationTestDB(t)
	plan := SubscriptionPlan{
		Id: 21_402, Title: "Timed Trial", Enabled: true, IsTrial: true,
		EntitlementType: SubscriptionEntitlementTimed,
		DurationUnit:    SubscriptionDurationHour, DurationValue: 1,
		MonthlyTokenLimit: 100, QuotaResetPeriod: SubscriptionResetNever,
	}
	require.NoError(t, DB.Create(&User{Id: 21_401, Username: "timed-trial", Status: common.UserStatusEnabled, AffCode: "timed-trial-aff"}).Error)
	require.NoError(t, DB.Create(&plan).Error)
	snapshot := NewSubscriptionEntitlementSnapshot(&plan, SubscriptionPurchaseModeTimed, 0)
	snapshotJSON, err := MarshalSubscriptionEntitlementSnapshot(snapshot)
	require.NoError(t, err)
	order := SubscriptionOrder{
		Id: 21_403, UserId: 21_401, PlanId: plan.Id, TradeNo: "timed-trial-21403",
		PaymentProvider: PaymentProviderBalance, PaymentMethod: PaymentMethodAccountBalance,
		Status: common.TopUpStatusPending, CreateTime: common.GetTimestamp(), EntitlementSnapshot: snapshotJSON,
	}
	require.NoError(t, DB.Create(&order).Error)

	var result *SubscriptionOrderCompletionResult
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		var locked SubscriptionOrder
		if err := tx.Where("id = ?", order.Id).First(&locked).Error; err != nil {
			return err
		}
		var completeErr error
		result, completeErr = CompleteSubscriptionOrderTx(tx, &locked, "", PaymentMethodAccountBalance)
		return completeErr
	}))
	require.NotNil(t, result)
	require.NotNil(t, result.Subscription)
	var grantCount int64
	require.NoError(t, DB.Model(&TimedSubscriptionValuationGrant{}).Count(&grantCount).Error)
	require.Zero(t, grantCount)
}

func TestTimedSubscriptionValuationGrantRedemptionCreatesAndReplaysGrant(t *testing.T) {
	setupTimedSubscriptionValuationTestDB(t)
	redemptionPriceMicros := int64(80_000_000)
	currentPlanPriceMicros := int64(50_000_000)
	plan := SubscriptionPlan{
		Id: 21_502, Title: "Timed Redemption", Enabled: true,
		EntitlementType: SubscriptionEntitlementTimed,
		PriceAmount:     80, PriceAmountMicros: &redemptionPriceMicros, Currency: "CNY",
		DurationUnit: SubscriptionDurationCustom, CustomSeconds: 7200,
		MonthlyTokenLimit: 2000, QuotaResetPeriod: SubscriptionResetNever,
	}
	require.NoError(t, DB.Create(&User{Id: 21_501, Username: "timed-redemption", Status: common.UserStatusEnabled, AffCode: "timed-redemption-aff"}).Error)
	require.NoError(t, DB.Create(&plan).Error)
	redemption := Redemption{
		Id: 21_503, Key: "timed-redemption-21503", Name: "timed-redemption", Type: RedemptionTypeSubscription,
		PlanId: plan.Id, Status: common.RedemptionCodeStatusEnabled, AmountCents: 8000, Currency: "CNY", CreatedTime: common.GetTimestamp(),
	}
	require.NoError(t, DB.Create(&redemption).Error)
	require.NoError(t, DB.Model(&SubscriptionPlan{}).Where("id = ?", plan.Id).Updates(map[string]any{
		"price_amount": 50, "price_amount_micros": currentPlanPriceMicros, "currency": "USD",
	}).Error)
	ClearSubscriptionPlanCacheForTest()

	first, err := Redeem(redemption.Key, 21_501, RedemptionModeTimed)
	require.NoError(t, err)
	require.NotNil(t, first)
	require.False(t, first.Replayed)
	require.Greater(t, first.FulfillmentSubscriptionId, 0)

	var grant TimedSubscriptionValuationGrant
	require.NoError(t, DB.Where("source_type = ? AND source_key = ?", TimedSubscriptionGrantSourceRedemption, "redemption:21503").First(&grant).Error)
	require.Equal(t, first.FulfillmentSubscriptionId, grant.UserSubscriptionId)
	require.Equal(t, redemptionPriceMicros, grant.SourcePriceMicros)
	require.Equal(t, "CNY", grant.SourceCurrency)
	firstEnd := grant.EventEndTime

	replay, err := Redeem(redemption.Key, 21_501, RedemptionModeTimed)
	require.NoError(t, err)
	require.True(t, replay.Replayed)
	require.Equal(t, first.FulfillmentSubscriptionId, replay.FulfillmentSubscriptionId)
	var persisted UserSubscription
	require.NoError(t, DB.First(&persisted, first.FulfillmentSubscriptionId).Error)
	require.Equal(t, firstEnd, persisted.EndTime)
	var grantCount int64
	require.NoError(t, DB.Model(&TimedSubscriptionValuationGrant{}).Count(&grantCount).Error)
	require.Equal(t, int64(1), grantCount)
}

func setupTimedSubscriptionValuationTestDB(t *testing.T) {
	t.Helper()
	oldDB := DB
	oldLogDB := LOG_DB
	oldUsingSQLite := common.UsingSQLite
	oldUsingMySQL := common.UsingMySQL
	oldUsingPostgreSQL := common.UsingPostgreSQL
	oldRedisEnabled := common.RedisEnabled

	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false

	safeName := strings.NewReplacer("/", "_", " ", "_", ":", "_").Replace(t.Name())
	db, err := gorm.Open(sqlite.Open("file:"+safeName+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	DB = db
	LOG_DB = db
	require.NoError(t, db.AutoMigrate(&User{}, &SubscriptionPlan{}, &SubscriptionOrder{}, &Redemption{}, &UserSubscription{}, &TimedSubscriptionValuationGrant{}))

	t.Cleanup(func() {
		_ = sqlDB.Close()
		DB = oldDB
		LOG_DB = oldLogDB
		common.UsingSQLite = oldUsingSQLite
		common.UsingMySQL = oldUsingMySQL
		common.UsingPostgreSQL = oldUsingPostgreSQL
		common.RedisEnabled = oldRedisEnabled
		ClearSubscriptionPlanCacheForTest()
		resetDBTimestampCacheForTest()
	})
}
