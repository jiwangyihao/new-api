package model

import (
	"math"
	"sort"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/stretchr/testify/require"
)

const adminPaidTestCurrencyCNY = "CNY"
const adminPaidTestCurrencyUSD = "USD"

func adminPaidTestSnapshot() int64 {
	return time.Date(2026, 1, 30, 0, 0, 0, 0, time.UTC).Unix()
}

func adminPaidTestPlan(id int, price float64, currency string) SubscriptionPlan {
	return SubscriptionPlan{
		Id:                id,
		Title:             "Plan " + string(rune('A'+id)),
		PriceAmount:       price,
		Currency:          currency,
		DurationUnit:      SubscriptionDurationDay,
		DurationValue:     30,
		MonthlyTokenLimit: 1000000000,
		QuotaResetPeriod:  SubscriptionResetMonthly,
		RewardEligible:    true,
		PublicVisible:     true,
		Enabled:           true,
	}
}

func adminPaidTestUser(id int, username string) User {
	return User{Id: id, Username: username, DisplayName: username, Status: common.UserStatusEnabled, Group: "default", AffCode: "aff-" + username, CreatedAt: adminPaidTestSnapshot() - 1000}
}

func adminPaidTestSubscription(id int, userID int, planID int, snapshot int64, source string) UserSubscription {
	return UserSubscription{
		Id:            id,
		UserId:        userID,
		PlanId:        planID,
		Status:        "active",
		StartTime:     snapshot - 27*86400,
		EndTime:       snapshot + 33*86400,
		TokenLimit:    1000000000,
		TokenUsed:     200000000,
		GrantReason:   source,
		Source:        source,
		LastResetTime: snapshot - 29*86400,
		NextResetTime: time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC).Unix(),
	}
}

func adminPaidTestQuery(snapshot int64) AdminAnalyticsQuery {
	return AdminAnalyticsQuery{SnapshotAt: snapshot, EndTimestamp: snapshot, Currency: adminPaidTestCurrencyCNY, RangeMode: AdminAnalyticsRangeModeSnapshot, Limit: 20}
}

func adminPaidMoneyAmount(amounts []dto.AdminAnalyticsMoneyBreakdown, currency string) float64 {
	for _, amount := range amounts {
		if amount.Currency == currency {
			return amount.Amount
		}
	}
	return 0
}

func adminPaidRequireAmount(t *testing.T, amounts []dto.AdminAnalyticsMoneyBreakdown, currency string, expected float64) {
	t.Helper()
	require.InDelta(t, expected, adminPaidMoneyAmount(amounts, currency), 0.0001)
}

func adminPaidRequireNoCurrency(t *testing.T, amounts []dto.AdminAnalyticsMoneyBreakdown, currency string) {
	t.Helper()
	for _, amount := range amounts {
		require.NotEqual(t, currency, amount.Currency)
	}
}

func adminPaidCreatePlanUserSub(t *testing.T, plan SubscriptionPlan, user User, sub UserSubscription) {
	t.Helper()
	require.NoError(t, DB.Create(&plan).Error)
	require.NoError(t, DB.Create(&user).Error)
	require.NoError(t, DB.Create(&sub).Error)
}

func TestPaidSubscriptionValueCalculatesMinTokenAndTimeValue(t *testing.T) {
	setupAdminAnalyticsTestDBs(t)
	snapshot := adminPaidTestSnapshot()
	plan := adminPaidTestPlan(1, 40, adminPaidTestCurrencyCNY)
	user := adminPaidTestUser(1, "paid")
	sub := adminPaidTestSubscription(1, user.Id, plan.Id, snapshot, "order")
	adminPaidCreatePlanUserSub(t, plan, user, sub)

	value, err := adminRecognizedRemainingValue(sub, plan, snapshot)
	require.NoError(t, err)
	require.InDelta(t, 44, value.RecognizedRemainingValue, 0.0001)
	require.InDelta(t, 44, value.TimeBasedValue, 0.0001)
	require.True(t, value.TokenBasedValueAvailable)
	require.Greater(t, value.TokenBasedValue, value.TimeBasedValue)
	require.InDelta(t, 75.8710, value.TokenBasedValue, 0.0001)

	res, err := GetAdminPaidSubscriptionValueSummary(adminPaidTestQuery(snapshot))
	require.NoError(t, err)
	adminPaidRequireAmount(t, res.Data.Summary.RecognizedRemainingValueByCurrency, adminPaidTestCurrencyCNY, 44)
	adminPaidRequireAmount(t, res.Data.Summary.TimeBasedValueByCurrency, adminPaidTestCurrencyCNY, 44)
	adminPaidRequireAmount(t, res.Data.Summary.TokenBasedValueByCurrency, adminPaidTestCurrencyCNY, 75.8710)
}

func TestPaidSubscriptionValueTimeOnlyWhenTokenUnavailable(t *testing.T) {
	setupAdminAnalyticsTestDBs(t)
	snapshot := adminPaidTestSnapshot()
	plan := adminPaidTestPlan(1, 30, adminPaidTestCurrencyCNY)
	user := adminPaidTestUser(1, "time-only")
	sub := adminPaidTestSubscription(1, user.Id, plan.Id, snapshot, "order")
	sub.TokenLimit = 0
	sub.TokenUsed = 0
	adminPaidCreatePlanUserSub(t, plan, user, sub)

	value, err := adminRecognizedRemainingValue(sub, plan, snapshot)
	require.NoError(t, err)
	require.False(t, value.TokenBasedValueAvailable)
	require.Equal(t, "time_only", value.ValuationBasis)
	require.InDelta(t, value.TimeBasedValue, value.RecognizedRemainingValue, 0.0001)

	res, err := GetAdminPaidSubscriptionValueSummary(adminPaidTestQuery(snapshot))
	require.NoError(t, err)
	require.Equal(t, 1, res.Data.Summary.TokenValueUnavailableCount)
}

func TestPaidSubscriptionValueTokenOveruseCurrentCycleIsZero(t *testing.T) {
	snapshot := adminPaidTestSnapshot()
	plan := adminPaidTestPlan(1, 40, adminPaidTestCurrencyCNY)
	sub := adminPaidTestSubscription(1, 1, plan.Id, snapshot, "order")
	sub.TokenUsed = 1200000000

	value, err := adminRecognizedRemainingValue(sub, plan, snapshot)
	require.NoError(t, err)
	require.True(t, value.TokenBasedValueAvailable)
	require.InDelta(t, 43.8710, value.TokenBasedValue, 0.0001)
	require.InDelta(t, 43.8710, value.RecognizedRemainingValue, 0.0001)
}

func TestPaidSubscriptionValueNeverResetDoesNotAddFutureCycles(t *testing.T) {
	snapshot := adminPaidTestSnapshot()
	plan := adminPaidTestPlan(1, 40, adminPaidTestCurrencyCNY)
	plan.QuotaResetPeriod = SubscriptionResetNever
	sub := adminPaidTestSubscription(1, 1, plan.Id, snapshot, "order")
	sub.TokenUsed = 200000000

	value, err := adminRecognizedRemainingValue(sub, plan, snapshot)
	require.NoError(t, err)
	require.True(t, value.TokenBasedValueAvailable)
	require.Equal(t, "token_never_reset", value.ValuationBasis)
	require.InDelta(t, 32, value.TokenBasedValue, 0.0001)
	require.InDelta(t, 32, value.RecognizedRemainingValue, 0.0001)
}

func TestPaidSubscriptionValueUsesPlanDurationForShortenedSubscriptions(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).Unix()
	dayPlan := adminPaidTestPlan(1, 30, adminPaidTestCurrencyCNY)
	dayPlan.QuotaResetPeriod = SubscriptionResetNever
	daySub := UserSubscription{Id: 1, UserId: 1, PlanId: dayPlan.Id, Status: "active", StartTime: start, EndTime: start + 15*86400, TokenLimit: 0, GrantReason: "order", Source: "order"}

	dayValue, err := adminSubscriptionTimeValue(daySub, dayPlan, start)
	require.NoError(t, err)
	require.InDelta(t, 15, dayValue, 0.0001)

	monthPlan := adminPaidTestPlan(2, 31, adminPaidTestCurrencyCNY)
	monthPlan.DurationUnit = SubscriptionDurationMonth
	monthPlan.DurationValue = 1
	monthPlan.QuotaResetPeriod = SubscriptionResetNever
	monthSub := UserSubscription{Id: 2, UserId: 2, PlanId: monthPlan.Id, Status: "active", StartTime: start, EndTime: start + 15*86400, TokenLimit: 0, GrantReason: "order", Source: "order"}

	monthValue, err := adminSubscriptionTimeValue(monthSub, monthPlan, start)
	require.NoError(t, err)
	require.InDelta(t, 15, monthValue, 0.0001)
}

func TestPaidSubscriptionValueUsesCalendarResetBoundariesForFutureTokens(t *testing.T) {
	t.Run("daily aligns to next midnight", func(t *testing.T) {
		snapshot := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC).Unix()
		plan := adminPaidTestPlan(1, 30, adminPaidTestCurrencyCNY)
		plan.QuotaResetPeriod = SubscriptionResetDaily
		sub := adminPaidTestSubscription(1, 1, plan.Id, snapshot, "order")
		sub.StartTime = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).Unix()
		sub.EndTime = time.Date(2026, 1, 17, 6, 0, 0, 0, time.UTC).Unix()
		sub.TokenLimit = 100
		sub.TokenUsed = 0
		sub.LastResetTime = 0
		sub.NextResetTime = 0

		value, err := adminRecognizedRemainingValue(sub, plan, snapshot)
		require.NoError(t, err)
		require.True(t, value.TokenBasedValueAvailable)
		require.InDelta(t, 2.25, value.TokenBasedValue, 0.0001)
	})

	t.Run("weekly aligns to next monday", func(t *testing.T) {
		snapshot := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC).Unix()
		plan := adminPaidTestPlan(1, 30, adminPaidTestCurrencyCNY)
		plan.QuotaResetPeriod = SubscriptionResetWeekly
		sub := adminPaidTestSubscription(1, 1, plan.Id, snapshot, "order")
		sub.StartTime = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).Unix()
		sub.EndTime = time.Date(2026, 1, 20, 12, 0, 0, 0, time.UTC).Unix()
		sub.TokenLimit = 100
		sub.TokenUsed = 0
		sub.LastResetTime = 0
		sub.NextResetTime = 0

		value, err := adminRecognizedRemainingValue(sub, plan, snapshot)
		require.NoError(t, err)
		require.True(t, value.TokenBasedValueAvailable)
		require.InDelta(t, 8.5, value.TokenBasedValue, 0.0001)
	})

	t.Run("monthly prorates each calendar cycle", func(t *testing.T) {
		start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).Unix()
		plan := adminPaidTestPlan(1, 310, adminPaidTestCurrencyCNY)
		plan.DurationUnit = SubscriptionDurationMonth
		plan.DurationValue = 1
		plan.QuotaResetPeriod = SubscriptionResetMonthly
		sub := adminPaidTestSubscription(1, 1, plan.Id, start, "order")
		sub.StartTime = start
		sub.EndTime = time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC).Unix()
		sub.TokenLimit = 100
		sub.TokenUsed = 0
		sub.LastResetTime = 0
		sub.NextResetTime = 0

		value, err := adminRecognizedRemainingValue(sub, plan, start)
		require.NoError(t, err)
		require.True(t, value.TokenBasedValueAvailable)
		require.InDelta(t, 760, value.TokenBasedValue, 0.0001)
	})
}

func TestPaidSubscriptionValueDailyWeeklyCustomResetProratesCycleValue(t *testing.T) {
	snapshot := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC).Unix()
	periods := []struct {
		name          string
		period        string
		customSeconds int64
		wantTokenValue float64
	}{
		{name: "daily", period: SubscriptionResetDaily, wantTokenValue: 42},
		{name: "weekly", period: SubscriptionResetWeekly, wantTokenValue: 54},
		{name: "custom", period: SubscriptionResetCustom, customSeconds: 3 * 86400, wantTokenValue: 40},
	}
	for _, tt := range periods {
		t.Run(tt.name, func(t *testing.T) {
			plan := adminPaidTestPlan(1, 120, adminPaidTestCurrencyCNY)
			plan.QuotaResetPeriod = tt.period
			plan.QuotaResetCustomSeconds = tt.customSeconds
			sub := adminPaidTestSubscription(1, 1, plan.Id, snapshot, "order")
			sub.StartTime = snapshot - 10*86400
			sub.EndTime = snapshot + 10*86400
			sub.TokenUsed = 0

			value, err := adminRecognizedRemainingValue(sub, plan, snapshot)
			require.NoError(t, err)
			require.True(t, value.TokenBasedValueAvailable)
			require.InDelta(t, 40, value.TimeBasedValue, 0.0001)
			require.InDelta(t, tt.wantTokenValue, value.TokenBasedValue, 0.0001)
			require.InDelta(t, 40, value.RecognizedRemainingValue, 0.0001)
		})
	}
}

func TestPaidSubscriptionValueMonthYearUsesCalendarDuration(t *testing.T) {
	monthStart := time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC).Unix()
	monthSnapshot := time.Date(2026, 2, 14, 0, 0, 0, 0, time.UTC).Unix()
	monthPlan := adminPaidTestPlan(1, 28, adminPaidTestCurrencyCNY)
	monthPlan.DurationUnit = SubscriptionDurationMonth
	monthPlan.DurationValue = 1
	monthPlan.QuotaResetPeriod = SubscriptionResetNever
	monthSub := UserSubscription{Id: 1, UserId: 1, PlanId: 1, Status: "active", StartTime: monthStart, EndTime: time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC).Unix(), TokenLimit: 100, TokenUsed: 0, GrantReason: "order", Source: "order"}
	monthValue, err := adminRecognizedRemainingValue(monthSub, monthPlan, monthSnapshot)
	require.NoError(t, err)
	require.InDelta(t, 12.6452, monthValue.TimeBasedValue, 0.0001)

	yearStart := time.Date(2024, 2, 29, 0, 0, 0, 0, time.UTC).Unix()
	yearSnapshot := time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC).Unix()
	yearPlan := adminPaidTestPlan(2, 365, adminPaidTestCurrencyCNY)
	yearPlan.DurationUnit = SubscriptionDurationYear
	yearPlan.DurationValue = 1
	yearPlan.QuotaResetPeriod = SubscriptionResetNever
	yearSub := UserSubscription{Id: 2, UserId: 2, PlanId: 2, Status: "active", StartTime: yearStart, EndTime: time.Date(2025, 2, 28, 0, 0, 0, 0, time.UTC).Unix(), TokenLimit: 365, TokenUsed: 0, GrantReason: "order", Source: "order"}
	yearValue, err := adminRecognizedRemainingValue(yearSub, yearPlan, yearSnapshot)
	require.NoError(t, err)
	require.InDelta(t, 363.0055, yearValue.TimeBasedValue, 0.0001)
}

func TestPaidSubscriptionValueExcludesGiftTrialAndInviteSourcesFromMainAndAudit(t *testing.T) {
	setupAdminAnalyticsTestDBs(t)
	snapshot := adminPaidTestSnapshot()
	plan := adminPaidTestPlan(1, 40, adminPaidTestCurrencyCNY)
	require.NoError(t, DB.Create(&plan).Error)
	giftSources := []string{SubscriptionGrantMonthlyInviteEntitlement, "invite_trial", "trial_code"}
	for i, source := range giftSources {
		userID := i + 1
		require.NoError(t, DB.Create(&User{Id: userID, Username: source, Status: common.UserStatusEnabled, Group: "default", AffCode: "aff-" + source}).Error)
		sub := adminPaidTestSubscription(i+1, userID, plan.Id, snapshot, source)
		require.NoError(t, DB.Create(&sub).Error)
	}

	res, err := GetAdminPaidSubscriptionValueSummary(adminPaidTestQuery(snapshot))
	require.NoError(t, err)
	require.Empty(t, res.Data.Summary.RecognizedRemainingValueByCurrency)
	require.Equal(t, 0, res.Data.Summary.ActivePaidUserCount)
	require.Equal(t, 0, res.Data.Summary.ActivePaidSubscriptionCount)
	require.Empty(t, res.Data.Summary.ExcludedRemainingValueByCurrency)
}

func TestPaidSubscriptionValueIncludesPaidSourcesWithoutOrders(t *testing.T) {
	setupAdminAnalyticsTestDBs(t)
	snapshot := adminPaidTestSnapshot()
	plan := adminPaidTestPlan(1, 30, adminPaidTestCurrencyCNY)
	require.NoError(t, DB.Create(&plan).Error)
	paidSources := []string{"order", "admin", "redemption"}
	for i, source := range paidSources {
		userID := i + 1
		require.NoError(t, DB.Create(&User{Id: userID, Username: source, Status: common.UserStatusEnabled, Group: "default", AffCode: "aff-" + source}).Error)
		sub := adminPaidTestSubscription(i+1, userID, plan.Id, snapshot, source)
		sub.TokenLimit = 0
		require.NoError(t, DB.Create(&sub).Error)
	}

	res, err := GetAdminPaidSubscriptionValueSummary(adminPaidTestQuery(snapshot))
	require.NoError(t, err)
	require.Equal(t, 3, res.Data.Summary.ActivePaidSubscriptionCount)
	require.Equal(t, 3, res.Data.Summary.ActivePaidUserCount)
	adminPaidRequireAmount(t, res.Data.Summary.RecognizedRemainingValueByCurrency, adminPaidTestCurrencyCNY, 99)
}

func TestPaidSubscriptionValueExcludedModeAuditsPaidExcludedUsers(t *testing.T) {
	setupAdminAnalyticsTestDBs(t)
	snapshot := adminPaidTestSnapshot()
	plan := adminPaidTestPlan(1, 30, adminPaidTestCurrencyCNY)
	user := adminPaidTestUser(1, "excluded")
	sub := adminPaidTestSubscription(1, user.Id, plan.Id, snapshot, "admin")
	sub.TokenLimit = 0
	adminPaidCreatePlanUserSub(t, plan, user, sub)
	adminPaidSetExcludedUsersForTest(t, []adminPaidExcludedUserForTest{{UserID: user.Id, Reason: "ops", ExcludedAt: 100, ExcludedBy: 2}})

	included, err := GetAdminPaidSubscriptionValueUsers(adminPaidTestQuery(snapshot))
	require.NoError(t, err)
	require.Empty(t, included.Data.Users.Items)
	adminPaidRequireAmount(t, included.Data.Summary.ExcludedRemainingValueByCurrency, adminPaidTestCurrencyCNY, 33)
	require.Empty(t, included.Data.Summary.RecognizedRemainingValueByCurrency)

	withExcluded, err := GetAdminPaidSubscriptionValueUsers(AdminAnalyticsQuery{SnapshotAt: snapshot, EndTimestamp: snapshot, Currency: adminPaidTestCurrencyCNY, RangeMode: AdminAnalyticsRangeModeSnapshot, ExcludedMode: dto.AdminAnalyticsExcludedModeIncludeExcluded, Limit: 20})
	require.NoError(t, err)
	require.Len(t, withExcluded.Data.Users.Items, 1)
	require.True(t, withExcluded.Data.Users.Items[0].Excluded)
	require.Equal(t, "ops", withExcluded.Data.Users.Items[0].ExcludedReason)
	adminPaidRequireAmount(t, withExcluded.Data.Users.Items[0].WouldHaveRemainingValueByCurrency, adminPaidTestCurrencyCNY, 33)

	onlyExcluded, err := GetAdminPaidSubscriptionValueUsers(AdminAnalyticsQuery{SnapshotAt: snapshot, EndTimestamp: snapshot, Currency: adminPaidTestCurrencyCNY, RangeMode: AdminAnalyticsRangeModeSnapshot, ExcludedMode: dto.AdminAnalyticsExcludedModeExcludedOnly, Limit: 20})
	require.NoError(t, err)
	require.Len(t, onlyExcluded.Data.Users.Items, 1)
	require.Empty(t, onlyExcluded.Data.Summary.RecognizedRemainingValueByCurrency)
	adminPaidRequireAmount(t, onlyExcluded.Data.Summary.ExcludedRemainingValueByCurrency, adminPaidTestCurrencyCNY, 33)
}

func TestPaidSubscriptionValueEmptyExcludedListDoesNotFilterRows(t *testing.T) {
	setupAdminAnalyticsTestDBs(t)
	snapshot := adminPaidTestSnapshot()
	plan := adminPaidTestPlan(1, 30, adminPaidTestCurrencyCNY)
	user := adminPaidTestUser(1, "included")
	sub := adminPaidTestSubscription(1, user.Id, plan.Id, snapshot, "order")
	sub.TokenLimit = 0
	adminPaidCreatePlanUserSub(t, plan, user, sub)
	adminPaidSetExcludedUsersForTest(t, nil)

	res, err := GetAdminPaidSubscriptionValueSummary(adminPaidTestQuery(snapshot))
	require.NoError(t, err)
	require.Equal(t, 1, res.Data.Summary.ActivePaidSubscriptionCount)
	adminPaidRequireAmount(t, res.Data.Summary.RecognizedRemainingValueByCurrency, adminPaidTestCurrencyCNY, 33)
}

func TestPaidSubscriptionValueSubscriptionsFiltersBySubscriptionID(t *testing.T) {
	setupAdminAnalyticsTestDBs(t)
	snapshot := adminPaidTestSnapshot()
	plan := adminPaidTestPlan(1, 30, adminPaidTestCurrencyCNY)
	require.NoError(t, DB.Create(&plan).Error)
	for i := 1; i <= 2; i++ {
		user := adminPaidTestUser(i, "u"+string(rune('0'+i)))
		require.NoError(t, DB.Create(&user).Error)
		sub := adminPaidTestSubscription(i, user.Id, plan.Id, snapshot, "order")
		sub.TokenLimit = 0
		require.NoError(t, DB.Create(&sub).Error)
	}

	query := adminPaidTestQuery(snapshot)
	query.SubscriptionID = 2
	res, err := GetAdminPaidSubscriptionValueSubscriptions(query)
	require.NoError(t, err)
	require.Len(t, res.Data.Subscriptions.Items, 1)
	require.Equal(t, 2, res.Data.Subscriptions.Items[0].SubscriptionID)

	summary, err := GetAdminPaidSubscriptionValueSummary(query)
	require.NoError(t, err)
	require.Equal(t, 2, summary.Data.Summary.ActivePaidSubscriptionCount)
}

func TestPaidSubscriptionValueSortsMoneyBySelectedCurrencyOnly(t *testing.T) {
	setupAdminAnalyticsTestDBs(t)
	snapshot := adminPaidTestSnapshot()
	cnyPlan := adminPaidTestPlan(1, 30, adminPaidTestCurrencyCNY)
	usdPlan := adminPaidTestPlan(2, 1000, adminPaidTestCurrencyUSD)
	require.NoError(t, DB.Create(&cnyPlan).Error)
	require.NoError(t, DB.Create(&usdPlan).Error)
	cnyUser := adminPaidTestUser(1, "cny")
	usdUser := adminPaidTestUser(2, "usd")
	require.NoError(t, DB.Create(&cnyUser).Error)
	require.NoError(t, DB.Create(&usdUser).Error)
	cnySub := adminPaidTestSubscription(1, 1, cnyPlan.Id, snapshot, "order")
	cnySub.TokenLimit = 0
	usdSub := adminPaidTestSubscription(2, 2, usdPlan.Id, snapshot, "order")
	usdSub.TokenLimit = 0
	require.NoError(t, DB.Create(&cnySub).Error)
	require.NoError(t, DB.Create(&usdSub).Error)

	query := adminPaidTestQuery(snapshot)
	query.SortBy = "recognized_remaining_value"
	query.SortOrder = dto.AdminAnalyticsSortDesc
	query.Currency = adminPaidTestCurrencyCNY
	res, err := GetAdminPaidSubscriptionValueUsers(query)
	require.NoError(t, err)
	require.Len(t, res.Data.Users.Items, 2)
	require.Equal(t, 1, res.Data.Users.Items[0].UserID)
	adminPaidRequireNoCurrency(t, res.Data.Users.Items[1].RecognizedRemainingValueByCurrency, adminPaidTestCurrencyCNY)
}

func TestPaidSubscriptionValueSubscriptionsSortsMoneyBySelectedCurrencyOnly(t *testing.T) {
	setupAdminAnalyticsTestDBs(t)
	snapshot := adminPaidTestSnapshot()
	cnyPlan := adminPaidTestPlan(1, 30, adminPaidTestCurrencyCNY)
	usdPlan := adminPaidTestPlan(2, 1000, adminPaidTestCurrencyUSD)
	require.NoError(t, DB.Create(&cnyPlan).Error)
	require.NoError(t, DB.Create(&usdPlan).Error)
	cnyUser := adminPaidTestUser(1, "cny")
	usdUser := adminPaidTestUser(2, "usd")
	require.NoError(t, DB.Create(&cnyUser).Error)
	require.NoError(t, DB.Create(&usdUser).Error)
	cnySub := adminPaidTestSubscription(1, cnyUser.Id, cnyPlan.Id, snapshot, "order")
	cnySub.TokenLimit = 0
	usdSub := adminPaidTestSubscription(2, usdUser.Id, usdPlan.Id, snapshot, "order")
	usdSub.TokenLimit = 0
	require.NoError(t, DB.Create(&cnySub).Error)
	require.NoError(t, DB.Create(&usdSub).Error)

	for _, sortBy := range []string{"recognized_remaining_value", "plan_price"} {
		t.Run(sortBy, func(t *testing.T) {
			query := adminPaidTestQuery(snapshot)
			query.SortBy = sortBy
			query.SortOrder = dto.AdminAnalyticsSortDesc
			query.Currency = adminPaidTestCurrencyCNY

			res, err := GetAdminPaidSubscriptionValueSubscriptions(query)
			require.NoError(t, err)
			require.Len(t, res.Data.Subscriptions.Items, 2)
			require.Equal(t, cnySub.Id, res.Data.Subscriptions.Items[0].SubscriptionID)
			require.Equal(t, usdSub.Id, res.Data.Subscriptions.Items[1].SubscriptionID)
		})
	}
}

func TestPaidSubscriptionValueUsersDescSortUsesUserIDTieBreaker(t *testing.T) {
	items := []dto.AdminPaidSubscriptionValueUser{
		{UserID: 1, RecognizedRemainingValueByCurrency: []dto.AdminAnalyticsMoneyBreakdown{{Currency: adminPaidTestCurrencyCNY, Amount: 10}}},
		{UserID: 2, RecognizedRemainingValueByCurrency: []dto.AdminAnalyticsMoneyBreakdown{{Currency: adminPaidTestCurrencyCNY, Amount: 10}}},
	}

	adminSortPaidSubscriptionUsers(items, AdminAnalyticsQuery{SortBy: "recognized_remaining_value", SortOrder: dto.AdminAnalyticsSortDesc, Currency: adminPaidTestCurrencyCNY})

	require.Equal(t, 1, items[0].UserID)
	require.Equal(t, 2, items[1].UserID)
}

func TestPaidSubscriptionValueSubscriptionsIncludesOrderAuxiliaryAmountWithPlanCurrency(t *testing.T) {
	setupAdminAnalyticsTestDBs(t)
	snapshot := adminPaidTestSnapshot()
	plan := adminPaidTestPlan(1, 40, adminPaidTestCurrencyCNY)
	user := adminPaidTestUser(1, "ordered")
	sub := adminPaidTestSubscription(1, user.Id, plan.Id, snapshot, "order")
	sub.TokenLimit = 0
	adminPaidCreatePlanUserSub(t, plan, user, sub)
	require.NoError(t, DB.Create(&SubscriptionOrder{Id: 7, UserId: user.Id, PlanId: plan.Id, Money: 9.99, TradeNo: "trade-7", PaymentProvider: "stripe", PaymentMethod: "card", Status: common.TopUpStatusSuccess, CompleteTime: snapshot - 10}).Error)

	res, err := GetAdminPaidSubscriptionValueSubscriptions(adminPaidTestQuery(snapshot))
	require.NoError(t, err)
	require.Len(t, res.Data.Subscriptions.Items, 1)
	item := res.Data.Subscriptions.Items[0]
	require.Equal(t, adminPaidTestCurrencyCNY, item.PlanPrice.Currency)
	require.InDelta(t, 40, item.PlanPrice.Amount, 0.0001)
	require.NotNil(t, item.OrderRecordedAmount)
	require.NotNil(t, item.PossibleOrderID)
	require.Equal(t, 7, *item.PossibleOrderID)
	require.Equal(t, "stripe", item.PaymentProvider)
	require.Equal(t, "card", item.PaymentMethod)
	require.Equal(t, adminPaidTestCurrencyCNY, item.OrderRecordedAmount.Currency)
	require.InDelta(t, 9.99, item.OrderRecordedAmount.Amount, 0.0001)
	require.InDelta(t, 44, item.RecognizedRemainingValue.Amount, 0.0001)
}

func TestInvitationPaidSubscriptionsCountsAllHistoryByDefault(t *testing.T) {
	setupAdminAnalyticsTestDBs(t)
	snapshot := adminPaidTestSnapshot()
	plan := adminPaidTestPlan(1, 50, adminPaidTestCurrencyCNY)
	require.NoError(t, DB.Create(&plan).Error)
	inviter := adminPaidTestUser(1, "inviter")
	require.NoError(t, DB.Create(&inviter).Error)
	invitee := adminPaidTestUser(2, "invitee")
	invitee.InviterId = 1
	require.NoError(t, DB.Create(&invitee).Error)
	expired := adminPaidTestSubscription(1, invitee.Id, plan.Id, snapshot-90*86400, "order")
	expired.StartTime = snapshot - 90*86400
	expired.EndTime = snapshot - 60*86400
	expired.Status = "expired"
	active := adminPaidTestSubscription(2, invitee.Id, plan.Id, snapshot, "order")
	active.StartTime = snapshot - 15*86400
	active.EndTime = snapshot + 15*86400
	require.NoError(t, DB.Create(&expired).Error)
	require.NoError(t, DB.Create(&active).Error)

	res, err := GetAdminInvitationPaidSubscriptionsSummary(AdminAnalyticsQuery{SnapshotAt: snapshot, RangeMode: AdminAnalyticsRangeModeAllHistory, Limit: 20})
	require.NoError(t, err)
	require.Equal(t, int64(0), res.Range.StartTimestamp)
	adminPaidRequireAmount(t, res.Data.Summary.RecognizedInvitationPaidAmountByCurrency, adminPaidTestCurrencyCNY, 100)
	require.Equal(t, 1, res.Data.Summary.InviterCount)
	require.Equal(t, 1, res.Data.Summary.InviteeCount)
	require.Equal(t, 1, res.Data.Summary.PaidInviteeCount)
}

func TestInvitationPaidSubscriptionsCountsInviteRelationshipsWithoutPaidRows(t *testing.T) {
	setupAdminAnalyticsTestDBs(t)
	snapshot := adminPaidTestSnapshot()
	plan := adminPaidTestPlan(1, 50, adminPaidTestCurrencyCNY)
	require.NoError(t, DB.Create(&plan).Error)
	inviter := adminPaidTestUser(1, "inviter")
	require.NoError(t, DB.Create(&inviter).Error)
	paidInvitee := adminPaidTestUser(2, "paid")
	paidInvitee.InviterId = inviter.Id
	unpaidInvitee := adminPaidTestUser(3, "unpaid")
	unpaidInvitee.InviterId = inviter.Id
	require.NoError(t, DB.Create(&paidInvitee).Error)
	require.NoError(t, DB.Create(&unpaidInvitee).Error)
	sub := adminPaidTestSubscription(1, paidInvitee.Id, plan.Id, snapshot, "order")
	require.NoError(t, DB.Create(&sub).Error)

	res, err := GetAdminInvitationPaidSubscriptionsInviters(AdminAnalyticsQuery{SnapshotAt: snapshot, RangeMode: AdminAnalyticsRangeModeAllHistory, Limit: 20})
	require.NoError(t, err)
	require.Equal(t, 1, res.Data.Summary.InviterCount)
	require.Equal(t, 2, res.Data.Summary.InviteeCount)
	require.Equal(t, 1, res.Data.Summary.PaidInviteeCount)
	require.Len(t, res.Data.Inviters.Items, 1)
	require.Equal(t, 2, res.Data.Inviters.Items[0].InviteeCount)
	require.Equal(t, 1, res.Data.Inviters.Items[0].PaidInviteeCount)
}

func TestInvitationPaidSubscriptionsAllHistoryStopsAtSnapshot(t *testing.T) {
	setupAdminAnalyticsTestDBs(t)
	snapshot := adminPaidTestSnapshot()
	plan := adminPaidTestPlan(1, 50, adminPaidTestCurrencyCNY)
	require.NoError(t, DB.Create(&plan).Error)
	inviter := adminPaidTestUser(1, "inviter")
	require.NoError(t, DB.Create(&inviter).Error)
	invitee := adminPaidTestUser(2, "invitee")
	invitee.InviterId = inviter.Id
	require.NoError(t, DB.Create(&invitee).Error)
	active := adminPaidTestSubscription(1, invitee.Id, plan.Id, snapshot, "order")
	active.StartTime = snapshot - 15*86400
	active.EndTime = snapshot + 45*86400
	future := adminPaidTestSubscription(2, invitee.Id, plan.Id, snapshot, "order")
	future.StartTime = snapshot + 86400
	future.EndTime = snapshot + 31*86400
	require.NoError(t, DB.Create(&active).Error)
	require.NoError(t, DB.Create(&future).Error)

	res, err := GetAdminInvitationPaidSubscriptionsSummary(AdminAnalyticsQuery{SnapshotAt: snapshot, RangeMode: AdminAnalyticsRangeModeAllHistory, Currency: adminPaidTestCurrencyCNY, Limit: 20})
	require.NoError(t, err)
	adminPaidRequireAmount(t, res.Data.Summary.RecognizedInvitationPaidAmountByCurrency, adminPaidTestCurrencyCNY, 50)
}

func TestInvitationPaidSubscriptionsFiltersConfirmationUnitTimeRange(t *testing.T) {
	setupAdminAnalyticsTestDBs(t)
	snapshot := adminPaidTestSnapshot()
	plan := adminPaidTestPlan(1, 50, adminPaidTestCurrencyCNY)
	require.NoError(t, DB.Create(&plan).Error)
	inviter := adminPaidTestUser(1, "inviter")
	require.NoError(t, DB.Create(&inviter).Error)
	invitee := adminPaidTestUser(2, "invitee")
	invitee.InviterId = 1
	require.NoError(t, DB.Create(&invitee).Error)
	oldSub := adminPaidTestSubscription(1, invitee.Id, plan.Id, snapshot, "order")
	oldSub.StartTime = snapshot - 90*86400
	oldSub.EndTime = snapshot - 60*86400
	oldSub.Status = "expired"
	newSub := adminPaidTestSubscription(2, invitee.Id, plan.Id, snapshot, "order")
	newSub.StartTime = snapshot - 10*86400
	newSub.EndTime = snapshot + 20*86400
	require.NoError(t, DB.Create(&oldSub).Error)
	require.NoError(t, DB.Create(&newSub).Error)

	res, err := GetAdminInvitationPaidSubscriptionsSummary(AdminAnalyticsQuery{SnapshotAt: snapshot, StartTimestamp: snapshot - 20*86400, EndTimestamp: snapshot, TimeRangeExplicit: true, RangeMode: AdminAnalyticsRangeModeAllHistory, Limit: 20})
	require.NoError(t, err)
	adminPaidRequireAmount(t, res.Data.Summary.RecognizedInvitationPaidAmountByCurrency, adminPaidTestCurrencyCNY, 50)
}

func TestInvitationPaidSubscriptionsInfersRepeatedUnitsFromExtendedSnapshot(t *testing.T) {
	setupAdminAnalyticsTestDBs(t)
	snapshot := adminPaidTestSnapshot()
	plan := adminPaidTestPlan(1, 40, adminPaidTestCurrencyCNY)
	require.NoError(t, DB.Create(&plan).Error)
	inviter := adminPaidTestUser(1, "inviter")
	require.NoError(t, DB.Create(&inviter).Error)
	invitee := adminPaidTestUser(2, "invitee")
	invitee.InviterId = 1
	require.NoError(t, DB.Create(&invitee).Error)
	sub := adminPaidTestSubscription(1, invitee.Id, plan.Id, snapshot, "order")
	sub.StartTime = snapshot - 75*86400
	sub.EndTime = snapshot
	sub.Status = "expired"
	require.NoError(t, DB.Create(&sub).Error)

	res, err := GetAdminInvitationPaidSubscriptionsSubscriptions(AdminAnalyticsQuery{SnapshotAt: snapshot, RangeMode: AdminAnalyticsRangeModeAllHistory, Currency: adminPaidTestCurrencyCNY, Limit: 20})
	require.NoError(t, err)
	require.Len(t, res.Data.Subscriptions.Items, 1)
	item := res.Data.Subscriptions.Items[0]
	require.InDelta(t, 2.5, item.RecognizedPaidUnits, 0.0001)
	require.Equal(t, "period_fraction", item.UnitInferenceBasis)
	require.InDelta(t, 100, item.RecognizedPaidAmount.Amount, 0.0001)
}

func TestInvitationPaidSubscriptionsExcludesGiftSourcesFromMainAndAudit(t *testing.T) {
	setupAdminAnalyticsTestDBs(t)
	snapshot := adminPaidTestSnapshot()
	plan := adminPaidTestPlan(1, 50, adminPaidTestCurrencyCNY)
	require.NoError(t, DB.Create(&plan).Error)
	inviter := adminPaidTestUser(1, "inviter")
	require.NoError(t, DB.Create(&inviter).Error)
	giftSources := []string{SubscriptionGrantMonthlyInviteEntitlement, "invite_trial", "trial_code"}
	for i, source := range giftSources {
		invitee := adminPaidTestUser(i+2, source)
		invitee.InviterId = 1
		require.NoError(t, DB.Create(&invitee).Error)
		sub := adminPaidTestSubscription(i+1, invitee.Id, plan.Id, snapshot, source)
		require.NoError(t, DB.Create(&sub).Error)
	}

	res, err := GetAdminInvitationPaidSubscriptionsSummary(AdminAnalyticsQuery{SnapshotAt: snapshot, RangeMode: AdminAnalyticsRangeModeAllHistory, Limit: 20})
	require.NoError(t, err)
	require.Empty(t, res.Data.Summary.RecognizedInvitationPaidAmountByCurrency)
	require.Empty(t, res.Data.Summary.ActiveInvitationPaidAmountByCurrency)
	require.Empty(t, res.Data.Summary.ActiveInvitationRemainingValueByCurrency)
	require.Empty(t, res.Data.Summary.ExcludedInvitationPaidAmountByCurrency)
	require.Equal(t, 0, res.Data.Summary.PaidInviteeCount)
	require.Equal(t, 0, res.Data.Summary.ActivePaidInviteeCount)
}

func TestInvitationPaidSubscriptionsActiveAmountAndRemainingValue(t *testing.T) {
	setupAdminAnalyticsTestDBs(t)
	snapshot := adminPaidTestSnapshot()
	plan := adminPaidTestPlan(1, 40, adminPaidTestCurrencyCNY)
	require.NoError(t, DB.Create(&plan).Error)
	inviter := adminPaidTestUser(1, "inviter")
	require.NoError(t, DB.Create(&inviter).Error)
	invitee := adminPaidTestUser(2, "invitee")
	invitee.InviterId = 1
	require.NoError(t, DB.Create(&invitee).Error)
	sub := adminPaidTestSubscription(1, invitee.Id, plan.Id, snapshot, "order")
	require.NoError(t, DB.Create(&sub).Error)

	res, err := GetAdminInvitationPaidSubscriptionsSummary(AdminAnalyticsQuery{SnapshotAt: snapshot, RangeMode: AdminAnalyticsRangeModeAllHistory, Currency: adminPaidTestCurrencyCNY, Limit: 20})
	require.NoError(t, err)
	adminPaidRequireAmount(t, res.Data.Summary.ActiveInvitationPaidAmountByCurrency, adminPaidTestCurrencyCNY, 40)
	adminPaidRequireAmount(t, res.Data.Summary.ActiveInvitationRemainingValueByCurrency, adminPaidTestCurrencyCNY, 44)
	require.Equal(t, 1, res.Data.Summary.ActivePaidInviteeCount)
}

func TestInvitationPaidSubscriptionsActiveOnlyDoesNotChangeHistorySummary(t *testing.T) {
	setupAdminAnalyticsTestDBs(t)
	snapshot := adminPaidTestSnapshot()
	plan := adminPaidTestPlan(1, 50, adminPaidTestCurrencyCNY)
	require.NoError(t, DB.Create(&plan).Error)
	inviter := adminPaidTestUser(1, "inviter")
	require.NoError(t, DB.Create(&inviter).Error)
	invitee := adminPaidTestUser(2, "invitee")
	invitee.InviterId = 1
	require.NoError(t, DB.Create(&invitee).Error)
	expired := adminPaidTestSubscription(1, invitee.Id, plan.Id, snapshot, "order")
	expired.StartTime = snapshot - 90*86400
	expired.EndTime = snapshot - 60*86400
	expired.Status = "expired"
	active := adminPaidTestSubscription(2, invitee.Id, plan.Id, snapshot, "order")
	active.StartTime = snapshot - 15*86400
	active.EndTime = snapshot + 15*86400
	require.NoError(t, DB.Create(&expired).Error)
	require.NoError(t, DB.Create(&active).Error)

	query := AdminAnalyticsQuery{SnapshotAt: snapshot, RangeMode: AdminAnalyticsRangeModeAllHistory, Currency: adminPaidTestCurrencyCNY, ActiveOnly: true, Limit: 20}
	summary, err := GetAdminInvitationPaidSubscriptionsSummary(query)
	require.NoError(t, err)
	adminPaidRequireAmount(t, summary.Data.Summary.RecognizedInvitationPaidAmountByCurrency, adminPaidTestCurrencyCNY, 100)

	subs, err := GetAdminInvitationPaidSubscriptionsSubscriptions(query)
	require.NoError(t, err)
	require.Len(t, subs.Data.Subscriptions.Items, 1)
	require.Equal(t, 2, subs.Data.Subscriptions.Items[0].SubscriptionID)
}

func TestInvitationPaidSubscriptionsSubscriptionsFiltersBySubscriptionID(t *testing.T) {
	setupAdminAnalyticsTestDBs(t)
	snapshot := adminPaidTestSnapshot()
	plan := adminPaidTestPlan(1, 50, adminPaidTestCurrencyCNY)
	require.NoError(t, DB.Create(&plan).Error)
	inviter := adminPaidTestUser(1, "inviter")
	require.NoError(t, DB.Create(&inviter).Error)
	invitee := adminPaidTestUser(2, "invitee")
	invitee.InviterId = 1
	require.NoError(t, DB.Create(&invitee).Error)
	for i := 1; i <= 2; i++ {
		sub := adminPaidTestSubscription(i, invitee.Id, plan.Id, snapshot, "order")
		sub.StartTime += int64(i) * 100
		sub.EndTime += int64(i) * 100
		require.NoError(t, DB.Create(&sub).Error)
	}

	res, err := GetAdminInvitationPaidSubscriptionsSubscriptions(AdminAnalyticsQuery{SnapshotAt: snapshot, RangeMode: AdminAnalyticsRangeModeAllHistory, SubscriptionID: 2, Currency: adminPaidTestCurrencyCNY, Limit: 20})
	require.NoError(t, err)
	require.Len(t, res.Data.Subscriptions.Items, 1)
	require.Equal(t, 2, res.Data.Subscriptions.Items[0].SubscriptionID)
}

func TestInvitationPaidSubscriptionsSortsMoneyBySelectedCurrencyOnly(t *testing.T) {
	setupAdminAnalyticsTestDBs(t)
	snapshot := adminPaidTestSnapshot()
	cnyPlan := adminPaidTestPlan(1, 30, adminPaidTestCurrencyCNY)
	usdPlan := adminPaidTestPlan(2, 1000, adminPaidTestCurrencyUSD)
	require.NoError(t, DB.Create(&cnyPlan).Error)
	require.NoError(t, DB.Create(&usdPlan).Error)
	inviter := adminPaidTestUser(1, "inviter")
	require.NoError(t, DB.Create(&inviter).Error)
	cnyInvitee := adminPaidTestUser(2, "cny")
	cnyInvitee.InviterId = 1
	usdInvitee := adminPaidTestUser(3, "usd")
	usdInvitee.InviterId = 1
	require.NoError(t, DB.Create(&cnyInvitee).Error)
	require.NoError(t, DB.Create(&usdInvitee).Error)
	cnySub := adminPaidTestSubscription(1, cnyInvitee.Id, cnyPlan.Id, snapshot, "order")
	usdSub := adminPaidTestSubscription(2, usdInvitee.Id, usdPlan.Id, snapshot, "order")
	require.NoError(t, DB.Create(&cnySub).Error)
	require.NoError(t, DB.Create(&usdSub).Error)

	query := AdminAnalyticsQuery{SnapshotAt: snapshot, RangeMode: AdminAnalyticsRangeModeAllHistory, Currency: adminPaidTestCurrencyCNY, SortBy: "recognized_paid_amount", SortOrder: dto.AdminAnalyticsSortDesc, Limit: 20}
	res, err := GetAdminInvitationPaidSubscriptionsInvitees(query)
	require.NoError(t, err)
	require.Len(t, res.Data.Invitees.Items, 2)
	require.Equal(t, cnyInvitee.Id, res.Data.Invitees.Items[0].InviteeUserID)
	adminPaidRequireNoCurrency(t, res.Data.Invitees.Items[1].RecognizedPaidAmountByCurrency, adminPaidTestCurrencyCNY)
}

func TestInvitationPaidSubscriptionsSubscriptionsSortsMoneyBySelectedCurrencyOnly(t *testing.T) {
	setupAdminAnalyticsTestDBs(t)
	snapshot := adminPaidTestSnapshot()
	cnyPlan := adminPaidTestPlan(1, 30, adminPaidTestCurrencyCNY)
	usdPlan := adminPaidTestPlan(2, 1000, adminPaidTestCurrencyUSD)
	require.NoError(t, DB.Create(&cnyPlan).Error)
	require.NoError(t, DB.Create(&usdPlan).Error)
	inviter := adminPaidTestUser(1, "inviter")
	require.NoError(t, DB.Create(&inviter).Error)
	cnyInvitee := adminPaidTestUser(2, "cny")
	cnyInvitee.InviterId = inviter.Id
	usdInvitee := adminPaidTestUser(3, "usd")
	usdInvitee.InviterId = inviter.Id
	require.NoError(t, DB.Create(&cnyInvitee).Error)
	require.NoError(t, DB.Create(&usdInvitee).Error)
	cnySub := adminPaidTestSubscription(1, cnyInvitee.Id, cnyPlan.Id, snapshot, "order")
	usdSub := adminPaidTestSubscription(2, usdInvitee.Id, usdPlan.Id, snapshot, "order")
	require.NoError(t, DB.Create(&cnySub).Error)
	require.NoError(t, DB.Create(&usdSub).Error)

	for _, sortBy := range []string{"recognized_paid_amount", "recognized_remaining_value", "plan_price"} {
		t.Run(sortBy, func(t *testing.T) {
			query := AdminAnalyticsQuery{SnapshotAt: snapshot, RangeMode: AdminAnalyticsRangeModeAllHistory, Currency: adminPaidTestCurrencyCNY, SortBy: sortBy, SortOrder: dto.AdminAnalyticsSortDesc, Limit: 20}

			res, err := GetAdminInvitationPaidSubscriptionsSubscriptions(query)
			require.NoError(t, err)
			require.Len(t, res.Data.Subscriptions.Items, 2)
			require.Equal(t, cnySub.Id, res.Data.Subscriptions.Items[0].SubscriptionID)
			require.Equal(t, usdSub.Id, res.Data.Subscriptions.Items[1].SubscriptionID)
		})
	}
}

func TestInvitationPaidSubscriptionsActiveOnlyReturnsExcludedAuditDetails(t *testing.T) {
	setupAdminAnalyticsTestDBs(t)
	snapshot := adminPaidTestSnapshot()
	plan := adminPaidTestPlan(1, 30, adminPaidTestCurrencyCNY)
	require.NoError(t, DB.Create(&plan).Error)
	inviter := adminPaidTestUser(1, "inviter")
	require.NoError(t, DB.Create(&inviter).Error)
	invitee := adminPaidTestUser(2, "excluded")
	invitee.InviterId = inviter.Id
	require.NoError(t, DB.Create(&invitee).Error)
	sub := adminPaidTestSubscription(1, invitee.Id, plan.Id, snapshot, "redemption")
	sub.TokenLimit = 0
	require.NoError(t, DB.Create(&sub).Error)
	adminPaidSetExcludedUsersForTest(t, []adminPaidExcludedUserForTest{{UserID: invitee.Id, Reason: "ops", ExcludedAt: 100, ExcludedBy: 2}})

	query := AdminAnalyticsQuery{SnapshotAt: snapshot, RangeMode: AdminAnalyticsRangeModeAllHistory, Currency: adminPaidTestCurrencyCNY, ExcludedMode: dto.AdminAnalyticsExcludedModeExcludedOnly, ActiveOnly: true, Limit: 20}
	inviterRes, err := GetAdminInvitationPaidSubscriptionsInviters(query)
	require.NoError(t, err)
	require.Len(t, inviterRes.Data.Inviters.Items, 1)
	require.Equal(t, inviter.Id, inviterRes.Data.Inviters.Items[0].InviterUserID)
	require.Equal(t, 1, inviterRes.Data.Inviters.Items[0].ActivePaidInviteeCount)
	adminPaidRequireAmount(t, inviterRes.Data.Summary.ExcludedActiveRemainingValueByCurrency, adminPaidTestCurrencyCNY, 33)
	adminPaidRequireAmount(t, inviterRes.Data.Inviters.Items[0].ExcludedActiveRemainingValueByCurrency, adminPaidTestCurrencyCNY, 33)

	inviteeRes, err := GetAdminInvitationPaidSubscriptionsInvitees(query)
	require.NoError(t, err)
	require.Len(t, inviteeRes.Data.Invitees.Items, 1)
	require.Equal(t, invitee.Id, inviteeRes.Data.Invitees.Items[0].InviteeUserID)
	require.True(t, inviteeRes.Data.Invitees.Items[0].Excluded)
	require.Equal(t, 1, inviteeRes.Data.Invitees.Items[0].ActivePaidSubscriptionCount)
	adminPaidRequireAmount(t, inviteeRes.Data.Invitees.Items[0].WouldHaveActiveRemainingValueByCurrency, adminPaidTestCurrencyCNY, 33)
}

func TestPaidSubscriptionValueExcludedUsersHelperRestoresExistingConfig(t *testing.T) {
	original := adminPaidExcludedUsersConfigValue(t, setting.GetSubscriptionAnalyticsExcludedUsers())
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{"subscription_analytics.excluded_users": original}))
	})
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{"subscription_analytics.excluded_users": adminPaidExcludedUsersConfigValueFromSlice(t, []setting.SubscriptionAnalyticsExcludedUser{{UserID: 77, Reason: "preexisting", ExcludedAt: 11, ExcludedBy: 3}})}))

	t.Run("temporary excluded users", func(t *testing.T) {
		adminPaidSetExcludedUsersForTest(t, []adminPaidExcludedUserForTest{{UserID: 88, Reason: "temporary", ExcludedAt: 22, ExcludedBy: 4}})
		excluded := setting.GetSubscriptionAnalyticsExcludedUsers()
		require.Contains(t, excluded, 88)
		require.NotContains(t, excluded, 77)
	})

	restored := setting.GetSubscriptionAnalyticsExcludedUsers()
	require.Contains(t, restored, 77)
	require.Equal(t, "preexisting", restored[77].Reason)
	require.NotContains(t, restored, 88)
}

func TestInvitationPaidSubscriptionsExcludedModeAuditsPaidExcludedInvitees(t *testing.T) {
	setupAdminAnalyticsTestDBs(t)
	snapshot := adminPaidTestSnapshot()
	plan := adminPaidTestPlan(1, 30, adminPaidTestCurrencyCNY)
	require.NoError(t, DB.Create(&plan).Error)
	inviter := adminPaidTestUser(1, "inviter")
	require.NoError(t, DB.Create(&inviter).Error)
	invitee := adminPaidTestUser(2, "excluded")
	invitee.InviterId = 1
	require.NoError(t, DB.Create(&invitee).Error)
	sub := adminPaidTestSubscription(1, invitee.Id, plan.Id, snapshot, "redemption")
	sub.TokenLimit = 0
	require.NoError(t, DB.Create(&sub).Error)
	adminPaidSetExcludedUsersForTest(t, []adminPaidExcludedUserForTest{{UserID: invitee.Id, Reason: "ops", ExcludedAt: 100, ExcludedBy: 2}})

	base := AdminAnalyticsQuery{SnapshotAt: snapshot, RangeMode: AdminAnalyticsRangeModeAllHistory, Currency: adminPaidTestCurrencyCNY, Limit: 20}
	included, err := GetAdminInvitationPaidSubscriptionsInvitees(base)
	require.NoError(t, err)
	require.Empty(t, included.Data.Invitees.Items)
	require.Empty(t, included.Data.Summary.RecognizedInvitationPaidAmountByCurrency)
	adminPaidRequireAmount(t, included.Data.Summary.ExcludedInvitationPaidAmountByCurrency, adminPaidTestCurrencyCNY, 60)
	adminPaidRequireAmount(t, included.Data.Summary.ExcludedActiveRemainingValueByCurrency, adminPaidTestCurrencyCNY, 33)

	base.ExcludedMode = dto.AdminAnalyticsExcludedModeIncludeExcluded
	withExcluded, err := GetAdminInvitationPaidSubscriptionsInvitees(base)
	require.NoError(t, err)
	require.Len(t, withExcluded.Data.Invitees.Items, 1)
	require.True(t, withExcluded.Data.Invitees.Items[0].Excluded)
	require.Equal(t, "ops", withExcluded.Data.Invitees.Items[0].ExcludedReason)
	adminPaidRequireAmount(t, withExcluded.Data.Invitees.Items[0].WouldHavePaidAmountByCurrency, adminPaidTestCurrencyCNY, 60)
	adminPaidRequireAmount(t, withExcluded.Data.Invitees.Items[0].WouldHaveActiveRemainingValueByCurrency, adminPaidTestCurrencyCNY, 33)

	base.ExcludedMode = dto.AdminAnalyticsExcludedModeExcludedOnly
	onlyExcluded, err := GetAdminInvitationPaidSubscriptionsInvitees(base)
	require.NoError(t, err)
	require.Len(t, onlyExcluded.Data.Invitees.Items, 1)
	require.Empty(t, onlyExcluded.Data.Summary.RecognizedInvitationPaidAmountByCurrency)
	adminPaidRequireAmount(t, onlyExcluded.Data.Summary.ExcludedInvitationPaidAmountByCurrency, adminPaidTestCurrencyCNY, 60)
}

type adminPaidExcludedUserForTest struct {
	UserID     int
	Reason     string
	ExcludedAt int64
	ExcludedBy int
}

func adminPaidSetExcludedUsersForTest(t *testing.T, users []adminPaidExcludedUserForTest) {
	t.Helper()
	original := adminPaidExcludedUsersConfigValue(t, setting.GetSubscriptionAnalyticsExcludedUsers())
	value := adminPaidExcludedUsersConfigValueFromTestUsers(t, users)
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{"subscription_analytics.excluded_users": value}))
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{"subscription_analytics.excluded_users": original}))
	})
}

func adminPaidExcludedUsersConfigValueFromTestUsers(t *testing.T, users []adminPaidExcludedUserForTest) string {
	t.Helper()
	excluded := make([]setting.SubscriptionAnalyticsExcludedUser, 0, len(users))
	for _, user := range users {
		excluded = append(excluded, setting.SubscriptionAnalyticsExcludedUser{UserID: user.UserID, Reason: user.Reason, ExcludedAt: user.ExcludedAt, ExcludedBy: user.ExcludedBy})
	}
	return adminPaidExcludedUsersConfigValueFromSlice(t, excluded)
}

func adminPaidExcludedUsersConfigValue(t *testing.T, users map[int]setting.SubscriptionAnalyticsExcludedUser) string {
	t.Helper()
	ids := make([]int, 0, len(users))
	for id := range users {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	excluded := make([]setting.SubscriptionAnalyticsExcludedUser, 0, len(ids))
	for _, id := range ids {
		excluded = append(excluded, users[id])
	}
	return adminPaidExcludedUsersConfigValueFromSlice(t, excluded)
}

func adminPaidExcludedUsersConfigValueFromSlice(t *testing.T, users []setting.SubscriptionAnalyticsExcludedUser) string {
	t.Helper()
	payload, err := common.Marshal(users)
	require.NoError(t, err)
	return string(payload)
}

func adminPaidAssertFinite(t *testing.T, value float64) {
	t.Helper()
	require.False(t, math.IsNaN(value))
	require.False(t, math.IsInf(value, 0))
}
