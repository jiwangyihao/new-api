package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupTimedHistoricalBackfillDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&UserSubscription{}, &SubscriptionOrder{}, &Redemption{}, &InvitationRewardEvent{}, &TimedSubscriptionValuationGrant{}))
	t.Cleanup(func() {
		if sqlDB, dbErr := db.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func timedHistoricalSnapshot(t *testing.T, planID int, price int64) string {
	t.Helper()
	payload, err := common.Marshal(SubscriptionEntitlementSnapshot{
		PurchaseMode: SubscriptionPurchaseModeTimed, PlanID: planID, PlanEntitlementType: SubscriptionEntitlementTimed,
		ListPriceMicros: &price, ListPriceCurrency: "CNY", MonthlyTokenLimit: 1000,
		DurationUnit: SubscriptionDurationMonth, DurationValue: 1, QuotaResetPeriod: SubscriptionResetNever,
	})
	require.NoError(t, err)
	return string(payload)
}

func TestTimedHistoricalBackfillCreatesEstimatedGrantAndIsIdempotent(t *testing.T) {
	db := setupTimedHistoricalBackfillDB(t)
	sub := UserSubscription{Id: 701, UserId: 801, PlanId: 901, EntitlementType: SubscriptionEntitlementTimed, TokenLimit: 1000, StartTime: 100, EndTime: 200, Status: SubscriptionStatusActive}
	require.NoError(t, db.Create(&sub).Error)
	require.NoError(t, db.Create(&SubscriptionOrder{Id: 1001, UserId: sub.UserId, PlanId: sub.PlanId, TradeNo: "timed-historical-order-1001", Status: common.TopUpStatusSuccess, FulfilledSubscriptionID: sub.Id, EntitlementSnapshot: timedHistoricalSnapshot(t, sub.PlanId, 40_000_000)}).Error)
	require.NoError(t, db.Create(&InvitationRewardEvent{Id: 10001, InviteeId: sub.UserId, SourceType: InvitationRewardEventSourceSubscriptionOrder, SourceId: 1001, SourceOrderId: 1001, SourceSubscriptionId: sub.Id, EventStartTime: sub.StartTime, EventEndTime: sub.EndTime, Status: InvitationRewardEventStatusActive, CreatedAt: 1, UpdatedAt: 1}).Error)
	request := creditHistoricalRequest(true)

	report, err := RunTimedSubscriptionValuationHistoricalBackfill(db, request)
	require.NoError(t, err)
	require.Equal(t, int64(1), report.RowsEstimated)
	var grant TimedSubscriptionValuationGrant
	require.NoError(t, db.First(&grant).Error)
	require.Equal(t, CreditValuationConfidenceEstimated, grant.Confidence)
	require.Equal(t, int64(40_000_000), grant.ValuationAmountMicros)
	require.Equal(t, "CNY", grant.ValuationCurrency)
	require.Equal(t, int64(100), grant.EventStartTime)
	require.Equal(t, int64(200), grant.EventEndTime)

	replay, err := RunTimedSubscriptionValuationHistoricalBackfill(db, request)
	require.NoError(t, err)
	require.Equal(t, int64(1), replay.RowsSkippedExisting)
	var count int64
	require.NoError(t, db.Model(&TimedSubscriptionValuationGrant{}).Count(&count).Error)
	require.Equal(t, int64(1), count)
}

func TestTimedHistoricalBackfillDryRunAndAmbiguityDoNotWrite(t *testing.T) {
	db := setupTimedHistoricalBackfillDB(t)
	sub := UserSubscription{Id: 702, UserId: 802, PlanId: 902, EntitlementType: SubscriptionEntitlementTimed, TokenLimit: 1000, StartTime: 100, EndTime: 200, Status: SubscriptionStatusActive}
	require.NoError(t, db.Create(&sub).Error)
	orders := []SubscriptionOrder{
		{Id: 1101, UserId: sub.UserId, PlanId: sub.PlanId, TradeNo: "timed-historical-order-1101", Status: common.TopUpStatusSuccess, FulfilledSubscriptionID: sub.Id, EntitlementSnapshot: timedHistoricalSnapshot(t, sub.PlanId, 40_000_000)},
		{Id: 1102, UserId: sub.UserId, PlanId: sub.PlanId, TradeNo: "timed-historical-order-1102", Status: common.TopUpStatusSuccess, FulfilledSubscriptionID: sub.Id, EntitlementSnapshot: timedHistoricalSnapshot(t, sub.PlanId, 40_000_000)},
	}
	for _, order := range orders {
		require.NoError(t, db.Create(&order).Error)
		require.NoError(t, db.Create(&InvitationRewardEvent{Id: 11000 + order.Id, InviteeId: sub.UserId, SourceType: InvitationRewardEventSourceSubscriptionOrder, SourceId: order.Id, SourceOrderId: order.Id, SourceSubscriptionId: sub.Id, EventStartTime: sub.StartTime, EventEndTime: sub.EndTime, Status: InvitationRewardEventStatusActive, CreatedAt: 1, UpdatedAt: 1}).Error)
	}
	request := creditHistoricalRequest(false)
	report, err := RunTimedSubscriptionValuationHistoricalBackfill(db, request)
	require.NoError(t, err)
	require.Equal(t, int64(1), report.AmbiguousRows)
	require.Equal(t, int64(1), report.RowsUnknown)
	var count int64
	require.NoError(t, db.Model(&TimedSubscriptionValuationGrant{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestTimedHistoricalBackfillPreservesForwardGrant(t *testing.T) {
	db := setupTimedHistoricalBackfillDB(t)
	sub := UserSubscription{Id: 703, UserId: 803, PlanId: 903, EntitlementType: SubscriptionEntitlementTimed, TokenLimit: 1000, StartTime: 100, EndTime: 200, Status: SubscriptionStatusActive}
	require.NoError(t, db.Create(&sub).Error)
	require.NoError(t, db.Create(&SubscriptionOrder{Id: 1201, UserId: sub.UserId, PlanId: sub.PlanId, TradeNo: "timed-historical-order-1201", Status: common.TopUpStatusSuccess, FulfilledSubscriptionID: sub.Id, EntitlementSnapshot: timedHistoricalSnapshot(t, sub.PlanId, 40_000_000)}).Error)
	require.NoError(t, db.Create(&InvitationRewardEvent{Id: 12001, InviteeId: sub.UserId, SourceType: InvitationRewardEventSourceSubscriptionOrder, SourceId: 1201, SourceOrderId: 1201, SourceSubscriptionId: sub.Id, EventStartTime: sub.StartTime, EventEndTime: sub.EndTime, Status: InvitationRewardEventStatusActive, CreatedAt: 1, UpdatedAt: 1}).Error)
	forward := TimedSubscriptionValuationGrant{IdempotencyKey: "forward:1201", UserSubscriptionId: sub.Id, UserId: sub.UserId, PlanId: sub.PlanId, SourceType: TimedSubscriptionGrantSourceOrder, SourceKey: "subscription_order:1201", SourceId: 1201, EventStartTime: 100, EventEndTime: 200, GrantCredit: 1000, SourcePriceMicros: 80_000_000, SourceCurrency: "CNY", ValuationAmountMicros: 80_000_000, ValuationCurrency: "CNY", Confidence: TimedSubscriptionValuationConfidenceExact, RuleVersion: CreditValuationRuleVersion, FxRateNumerator: 1, FxRateDenominator: 1, FxCapturedAt: 1, CreatedAt: 1}
	forwardSnapshot, err := common.Marshal(timedSubscriptionGrantSourceSnapshot{
		IdempotencyKey: forward.IdempotencyKey, SourceType: forward.SourceType, SourceKey: forward.SourceKey, SourceId: forward.SourceId,
		UserId: forward.UserId, PlanId: forward.PlanId, SourcePriceMicros: forward.SourcePriceMicros, SourceCurrency: forward.SourceCurrency,
		GrantCredit: forward.GrantCredit, DurationUnit: SubscriptionDurationMonth, DurationValue: 1, QuotaResetPeriod: SubscriptionResetNever, ValuationRuleVersion: forward.RuleVersion,
	})
	require.NoError(t, err)
	forward.SourceSnapshot = string(forwardSnapshot)
	require.NoError(t, db.Create(&forward).Error)

	report, err := RunTimedSubscriptionValuationHistoricalBackfill(db, creditHistoricalRequest(true))
	require.NoError(t, err)
	require.Equal(t, int64(1), report.RowsSkippedExisting)
	var got TimedSubscriptionValuationGrant
	require.NoError(t, db.First(&got).Error)
	require.Equal(t, TimedSubscriptionValuationConfidenceExact, got.Confidence)
	require.Equal(t, int64(80_000_000), got.ValuationAmountMicros)
}

func TestTimedHistoricalBackfillMissingInvitationEventRemainsUnknown(t *testing.T) {
	db := setupTimedHistoricalBackfillDB(t)
	sub := UserSubscription{Id: 704, UserId: 804, PlanId: 904, EntitlementType: SubscriptionEntitlementTimed, TokenLimit: 1000, StartTime: 100, EndTime: 200, Status: SubscriptionStatusActive}
	require.NoError(t, db.Create(&sub).Error)
	require.NoError(t, db.Create(&SubscriptionOrder{Id: 1301, UserId: sub.UserId, PlanId: sub.PlanId, TradeNo: "timed-historical-order-1301", Status: common.TopUpStatusSuccess, FulfilledSubscriptionID: sub.Id, EntitlementSnapshot: timedHistoricalSnapshot(t, sub.PlanId, 40_000_000)}).Error)

	report, err := RunTimedSubscriptionValuationHistoricalBackfill(db, creditHistoricalRequest(true))
	require.NoError(t, err)
	require.Zero(t, report.RowsEstimated)
	require.Equal(t, int64(1), report.RowsUnknown)
	require.Equal(t, int64(1), report.InvalidRows)
	require.Equal(t, []CreditValuationMigrationReasonCount{{Reason: timedValuationHistoricalReasonWindow, Count: 1}}, report.Reasons)
	var count int64
	require.NoError(t, db.Model(&TimedSubscriptionValuationGrant{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestTimedHistoricalBackfillLegacyAdminSubscriptionWithoutAuthoritativeWindowRemainsUnknown(t *testing.T) {
	db := setupTimedHistoricalBackfillDB(t)
	sub := UserSubscription{Id: 710, UserId: 810, PlanId: 910, EntitlementType: SubscriptionEntitlementTimed, TokenLimit: 1000, StartTime: 100, EndTime: 200, Status: SubscriptionStatusActive, GrantReason: SubscriptionGrantAdmin, Source: SubscriptionGrantAdmin}
	require.NoError(t, db.Create(&sub).Error)

	report, err := RunTimedSubscriptionValuationHistoricalBackfill(db, creditHistoricalRequest(true))
	require.NoError(t, err)
	require.Zero(t, report.RowsEstimated)
	require.Equal(t, int64(1), report.RowsUnknown)
	require.Equal(t, []CreditValuationMigrationReasonCount{{Reason: timedValuationHistoricalReasonNoSource, Count: 1}}, report.Reasons)
	require.Len(t, report.Diagnostics, 1)
	require.Equal(t, timedValuationHistoricalReasonNoSource, report.Diagnostics[0].Reason)
	var count int64
	require.NoError(t, db.Model(&TimedSubscriptionValuationGrant{}).Count(&count).Error)
	require.Zero(t, count, "UserSubscription dates are not an authoritative historical admin window")
}

func TestTimedHistoricalBackfillRetainsExistingForwardAdminGrantWithoutLegacyWindow(t *testing.T) {
	db := setupTimedHistoricalBackfillDB(t)
	sub := UserSubscription{Id: 711, UserId: 811, PlanId: 911, EntitlementType: SubscriptionEntitlementTimed, TokenLimit: 1000, StartTime: 100, EndTime: 200, Status: SubscriptionStatusActive, GrantReason: SubscriptionGrantAdmin, Source: SubscriptionGrantAdmin}
	require.NoError(t, db.Create(&sub).Error)
	snapshotPayload, err := common.Marshal(timedSubscriptionGrantSourceSnapshot{
		IdempotencyKey: "forward-admin-711", SourceType: TimedSubscriptionGrantSourceAdmin, SourceKey: "admin:forward-admin-711", SourceId: 71101,
		UserId: sub.UserId, PlanId: sub.PlanId, SourcePriceMicros: 40_000_000, SourceCurrency: "CNY", Reason: "legacy admin forward fixture",
		GrantCredit: 1000, DurationUnit: SubscriptionDurationMonth, DurationValue: 1, QuotaResetPeriod: SubscriptionResetNever, ValuationRuleVersion: CreditValuationRuleVersion,
	})
	require.NoError(t, err)
	forward := TimedSubscriptionValuationGrant{IdempotencyKey: "forward-admin-711", UserSubscriptionId: sub.Id, UserId: sub.UserId, PlanId: sub.PlanId, SourceType: TimedSubscriptionGrantSourceAdmin, SourceKey: "admin:forward-admin-711", SourceId: 71101, EventStartTime: 100, EventEndTime: 200, GrantCredit: 1000, SourcePriceMicros: 40_000_000, SourceCurrency: "CNY", ValuationAmountMicros: 40_000_000, ValuationCurrency: "CNY", Confidence: TimedSubscriptionValuationConfidenceExact, RuleVersion: CreditValuationRuleVersion, FxRateNumerator: 1, FxRateDenominator: 1, FxCapturedAt: 1_700_000_000, SourceSnapshot: string(snapshotPayload), CreatedAt: 1_700_000_000}
	require.True(t, validTimedSubscriptionValuationGrant(forward))
	require.NoError(t, db.Create(&forward).Error)

	report, err := RunTimedSubscriptionValuationHistoricalBackfill(db, creditHistoricalRequest(true))
	require.NoError(t, err)
	require.Equal(t, int64(1), report.RowsSkippedExisting)
	require.Zero(t, report.RowsUnknown)
	require.Zero(t, report.RowsEstimated)
	var count int64
	require.NoError(t, db.Model(&TimedSubscriptionValuationGrant{}).Where("user_subscription_id = ?", sub.Id).Count(&count).Error)
	require.Equal(t, int64(1), count)
	var got TimedSubscriptionValuationGrant
	require.NoError(t, db.First(&got, "user_subscription_id = ?", sub.Id).Error)
	require.Equal(t, TimedSubscriptionValuationConfidenceExact, got.Confidence)
	require.Equal(t, int64(100), got.EventStartTime)
	require.Equal(t, int64(200), got.EventEndTime)
}

func TestTimedHistoricalBackfillAddsMissingWindowBesideForwardGrant(t *testing.T) {
	db := setupTimedHistoricalBackfillDB(t)
	sub := UserSubscription{Id: 705, UserId: 805, PlanId: 905, EntitlementType: SubscriptionEntitlementTimed, TokenLimit: 1000, StartTime: 100, EndTime: 300, Status: SubscriptionStatusActive}
	require.NoError(t, db.Create(&sub).Error)
	require.NoError(t, db.Create(&SubscriptionOrder{Id: 1401, UserId: sub.UserId, PlanId: sub.PlanId, TradeNo: "timed-historical-order-1401", Status: common.TopUpStatusSuccess, FulfilledSubscriptionID: sub.Id, EntitlementSnapshot: timedHistoricalSnapshot(t, sub.PlanId, 40_000_000)}).Error)
	require.NoError(t, db.Create(&InvitationRewardEvent{Id: 14001, SourceOrderId: 1401, SourceSubscriptionId: sub.Id, EventStartTime: 100, EventEndTime: 200}).Error)
	forward := TimedSubscriptionValuationGrant{IdempotencyKey: "forward:1402", UserSubscriptionId: sub.Id, UserId: sub.UserId, PlanId: sub.PlanId, SourceType: TimedSubscriptionGrantSourceAdmin, SourceKey: "admin:1402", EventStartTime: 200, EventEndTime: 300, GrantCredit: 1000, SourcePriceMicros: 10_000_000, SourceCurrency: "USD", ValuationAmountMicros: 10_000_000, ValuationCurrency: "USD", Confidence: TimedSubscriptionValuationConfidenceExact, RuleVersion: CreditValuationRuleVersion, FxRateNumerator: 1, FxRateDenominator: 1, FxCapturedAt: 1, CreatedAt: 1}
	forwardSnapshot, err := common.Marshal(timedSubscriptionGrantSourceSnapshot{
		IdempotencyKey: forward.IdempotencyKey, SourceType: forward.SourceType, SourceKey: forward.SourceKey, SourceId: forward.SourceId,
		UserId: forward.UserId, PlanId: forward.PlanId, SourcePriceMicros: forward.SourcePriceMicros, SourceCurrency: forward.SourceCurrency, Reason: "forward admin fixture",
		GrantCredit: forward.GrantCredit, DurationUnit: SubscriptionDurationMonth, DurationValue: 1, QuotaResetPeriod: SubscriptionResetNever, ValuationRuleVersion: forward.RuleVersion,
	})
	require.NoError(t, err)
	forward.SourceSnapshot = string(forwardSnapshot)
	require.NoError(t, db.Create(&forward).Error)

	report, err := RunTimedSubscriptionValuationHistoricalBackfill(db, creditHistoricalRequest(true))
	require.NoError(t, err)
	require.Equal(t, int64(1), report.RowsEstimated)
	require.Zero(t, report.RowsUnknown)
	require.Equal(t, []TimedSubscriptionValuationHistoricalCurrencyAmount{{Currency: "CNY", AmountMicros: 40_000_000}}, report.EstimatedCostMicrosByCurrency)
	var count int64
	require.NoError(t, db.Model(&TimedSubscriptionValuationGrant{}).Where("user_subscription_id = ?", sub.Id).Count(&count).Error)
	require.Equal(t, int64(2), count)
}

func TestTimedHistoricalBackfillRejectsEntireSubscriptionWhenRenewalWindowsOverlap(t *testing.T) {
	db := setupTimedHistoricalBackfillDB(t)
	sub := UserSubscription{Id: 709, UserId: 809, PlanId: 909, EntitlementType: SubscriptionEntitlementTimed, TokenLimit: 1000, StartTime: 100, EndTime: 300, Status: SubscriptionStatusActive}
	require.NoError(t, db.Create(&sub).Error)
	orders := []SubscriptionOrder{
		{Id: 1701, UserId: sub.UserId, PlanId: sub.PlanId, TradeNo: "timed-overlap-1701", Status: common.TopUpStatusSuccess, FulfilledSubscriptionID: sub.Id, EntitlementSnapshot: timedHistoricalSnapshot(t, sub.PlanId, 40_000_000)},
		{Id: 1702, UserId: sub.UserId, PlanId: sub.PlanId, TradeNo: "timed-overlap-1702", Status: common.TopUpStatusSuccess, FulfilledSubscriptionID: sub.Id, EntitlementSnapshot: timedHistoricalSnapshot(t, sub.PlanId, 40_000_000)},
	}
	windows := []InvitationRewardEvent{
		{Id: 27001, InviteeId: sub.UserId, SourceType: InvitationRewardEventSourceSubscriptionOrder, SourceId: 1701, SourceOrderId: 1701, SourceSubscriptionId: sub.Id, EventStartTime: 100, EventEndTime: 250, Status: InvitationRewardEventStatusActive, CreatedAt: 1, UpdatedAt: 1},
		{Id: 27002, InviteeId: sub.UserId, SourceType: InvitationRewardEventSourceSubscriptionOrder, SourceId: 1702, SourceOrderId: 1702, SourceSubscriptionId: sub.Id, EventStartTime: 200, EventEndTime: 300, Status: InvitationRewardEventStatusActive, CreatedAt: 1, UpdatedAt: 1},
	}
	require.NoError(t, db.Create(&orders).Error)
	require.NoError(t, db.Create(&windows).Error)

	report, err := RunTimedSubscriptionValuationHistoricalBackfill(db, creditHistoricalRequest(true))
	require.NoError(t, err)
	require.Zero(t, report.RowsEstimated)
	require.Equal(t, int64(1), report.RowsUnknown)
	require.Equal(t, int64(1), report.AmbiguousRows)
	require.Equal(t, []CreditValuationMigrationReasonCount{{Reason: timedValuationHistoricalReasonAmbiguous, Count: 1}}, report.Reasons)
	require.Empty(t, report.EstimatedCostMicrosByCurrency)
	var count int64
	require.NoError(t, db.Model(&TimedSubscriptionValuationGrant{}).Where("user_subscription_id = ?", sub.Id).Count(&count).Error)
	require.Zero(t, count, "an overlapping renewal must not partially persist or collapse into the enclosing subscription window")
}

func TestTimedHistoricalBackfillReportsCurrenciesSeparately(t *testing.T) {
	db := setupTimedHistoricalBackfillDB(t)
	for index, currency := range []string{"CNY", "USD"} {
		id := 706 + index
		sub := UserSubscription{Id: id, UserId: 806 + index, PlanId: 906 + index, EntitlementType: SubscriptionEntitlementTimed, TokenLimit: 1000, StartTime: 100, EndTime: 200, Status: SubscriptionStatusActive}
		require.NoError(t, db.Create(&sub).Error)
		price := int64(40_000_000)
		snapshot := SubscriptionEntitlementSnapshot{PurchaseMode: SubscriptionPurchaseModeTimed, PlanID: sub.PlanId, PlanEntitlementType: SubscriptionEntitlementTimed, ListPriceMicros: &price, ListPriceCurrency: currency, MonthlyTokenLimit: 1000, DurationUnit: SubscriptionDurationMonth, DurationValue: 1, QuotaResetPeriod: SubscriptionResetNever}
		payload, err := common.Marshal(snapshot)
		require.NoError(t, err)
		require.NoError(t, db.Create(&SubscriptionOrder{Id: 1501 + index, UserId: sub.UserId, PlanId: sub.PlanId, TradeNo: "timed-currency-" + currency, Status: common.TopUpStatusSuccess, FulfilledSubscriptionID: sub.Id, EntitlementSnapshot: string(payload)}).Error)
		require.NoError(t, db.Create(&InvitationRewardEvent{Id: 25001 + index, InviteeId: sub.UserId, SourceType: InvitationRewardEventSourceSubscriptionOrder, SourceId: 1501 + index, SourceOrderId: 1501 + index, SourceSubscriptionId: sub.Id, EventStartTime: sub.StartTime, EventEndTime: sub.EndTime, Status: InvitationRewardEventStatusActive, CreatedAt: 1, UpdatedAt: 1}).Error)
	}

	report, err := RunTimedSubscriptionValuationHistoricalBackfill(db, creditHistoricalRequest(false))
	require.NoError(t, err)
	require.Zero(t, report.EstimatedCostMicros)
	require.Equal(t, []TimedSubscriptionValuationHistoricalCurrencyAmount{{Currency: "CNY", AmountMicros: 40_000_000}, {Currency: "USD", AmountMicros: 40_000_000}}, report.EstimatedCostMicrosByCurrency)
}

func TestTimedHistoricalBackfillPersistsFXSnapshots(t *testing.T) {
	const (
		capturedAt  = int64(1_800_000_000)
		priceMicros = int64(42_000_000)
	)
	tests := []struct {
		name                 string
		sourceCurrency       string
		valuationCurrency    string
		fxSourceCurrency     string
		fxValuationCurrency  string
		requestFXNumerator   int64
		requestFXDenominator int64
		wantFXNumerator      int64
		wantFXDenominator    int64
		wantValuationMicros  int64
		wantDirection        string
	}{
		{name: "CNY identity", sourceCurrency: "CNY", valuationCurrency: "CNY", fxSourceCurrency: "USD", fxValuationCurrency: "CNY", requestFXNumerator: 7, requestFXDenominator: 1, wantFXNumerator: 1, wantFXDenominator: 1, wantValuationMicros: 42_000_000, wantDirection: CreditFXDirectionIdentity},
		{name: "USD to CNY", sourceCurrency: "USD", valuationCurrency: "CNY", fxSourceCurrency: "USD", fxValuationCurrency: "CNY", requestFXNumerator: 7, requestFXDenominator: 1, wantFXNumerator: 7, wantFXDenominator: 1, wantValuationMicros: 294_000_000, wantDirection: CreditFXDirectionUSDtoCNY},
		{name: "CNY to USD", sourceCurrency: "CNY", valuationCurrency: "USD", fxSourceCurrency: "USD", fxValuationCurrency: "CNY", requestFXNumerator: 7, requestFXDenominator: 1, wantFXNumerator: 1, wantFXDenominator: 7, wantValuationMicros: 6_000_000, wantDirection: CreditFXDirectionCNYtoUSD},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db := setupTimedHistoricalBackfillDB(t)
			sub := UserSubscription{Id: 708, UserId: 808, PlanId: 908, EntitlementType: SubscriptionEntitlementTimed, TokenLimit: 1000, StartTime: 100, EndTime: 200, Status: SubscriptionStatusActive}
			require.NoError(t, db.Create(&sub).Error)
			snapshot := SubscriptionEntitlementSnapshot{PurchaseMode: SubscriptionPurchaseModeTimed, PlanID: sub.PlanId, PlanEntitlementType: SubscriptionEntitlementTimed, ListPriceMicros: func() *int64 { value := priceMicros; return &value }(), ListPriceCurrency: test.sourceCurrency, MonthlyTokenLimit: 1000, DurationUnit: SubscriptionDurationMonth, DurationValue: 1, QuotaResetPeriod: SubscriptionResetNever}
			payload, err := common.Marshal(snapshot)
			require.NoError(t, err)
			require.NoError(t, db.Create(&SubscriptionOrder{Id: 1601, UserId: sub.UserId, PlanId: sub.PlanId, TradeNo: "timed-fx-" + test.sourceCurrency + "-" + test.valuationCurrency, Status: common.TopUpStatusSuccess, FulfilledSubscriptionID: sub.Id, EntitlementSnapshot: string(payload)}).Error)
			require.NoError(t, db.Create(&InvitationRewardEvent{Id: 26001, InviteeId: sub.UserId, SourceType: InvitationRewardEventSourceSubscriptionOrder, SourceId: 1601, SourceOrderId: 1601, SourceSubscriptionId: sub.Id, EventStartTime: sub.StartTime, EventEndTime: sub.EndTime, Status: InvitationRewardEventStatusActive, CreatedAt: 1, UpdatedAt: 1}).Error)

			request := CreditValuationHistoricalBackfillRequest{
				Apply: true, MigrationVersion: 1, BatchSize: 10, ValuationCurrency: test.valuationCurrency,
				FX: CreditValuationMigrationFXSnapshot{SourceCurrency: test.fxSourceCurrency, ValuationCurrency: test.fxValuationCurrency, Numerator: test.requestFXNumerator, Denominator: test.requestFXDenominator, CapturedAt: capturedAt},
			}
			report, err := RunTimedSubscriptionValuationHistoricalBackfill(db, request)
			require.NoError(t, err)
			require.Equal(t, []TimedSubscriptionValuationHistoricalCurrencyAmount{{Currency: test.sourceCurrency, AmountMicros: priceMicros}}, report.EstimatedCostMicrosByCurrency)

			var grant TimedSubscriptionValuationGrant
			require.NoError(t, db.First(&grant).Error)
			require.Equal(t, test.sourceCurrency, grant.SourceCurrency)
			require.Equal(t, test.valuationCurrency, grant.ValuationCurrency)
			require.Equal(t, priceMicros, grant.SourcePriceMicros)
			require.Equal(t, test.wantValuationMicros, grant.ValuationAmountMicros)
			require.Equal(t, test.wantFXNumerator, grant.FxRateNumerator)
			require.Equal(t, test.wantFXDenominator, grant.FxRateDenominator)
			require.Equal(t, capturedAt, grant.FxCapturedAt)
			require.Equal(t, test.wantDirection, creditFXDirection(grant.SourceCurrency, grant.ValuationCurrency))
			require.True(t, validTimedSubscriptionValuationGrant(grant))
		})
	}
}
