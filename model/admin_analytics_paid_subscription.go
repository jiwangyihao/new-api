package model

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/setting"
)

const (
	adminPaidSubscriptionValuationTokenAndTime = "token_and_time"
	adminPaidSubscriptionValuationTimeOnly     = "time_only"
	adminPaidSubscriptionValuationNeverReset   = "token_never_reset"
	adminPaidSubscriptionValuationCreditPool   = "credit_moving_weighted_average"

	adminPaidSubscriptionSourceAttributionSnapshot       = "snapshot"
	adminPaidSubscriptionSourceAttributionMixedOrUnknown = "mixed_or_unknown"
	adminPaidSubscriptionSourceAttributionCreditPool     = "moving_weighted_pool"

	adminPaidSubscriptionSnapshotSemanticsSnapshot    = "snapshot"
	adminPaidSubscriptionSnapshotSemanticsCurrentOnly = "current_only"

	adminInvitationPaidUnitPeriodAligned   = "period_aligned"
	adminInvitationPaidUnitPeriodFraction  = "period_fraction"
	adminInvitationPaidUnitEventSnapshot   = "event_snapshot"
	adminInvitationPaidUnitSnapshotMinimum = "snapshot_minimum"
)

type adminMoneyAccumulator struct {
	values   map[string]int64
	overflow bool
}

func (a *adminMoneyAccumulator) add(currency string, amount float64) {
	if amount == 0 {
		return
	}
	if math.IsNaN(amount) || math.IsInf(amount, 0) || amount > float64(math.MaxInt64)/float64(amountMicrosPerUnit) || amount < float64(math.MinInt64)/float64(amountMicrosPerUnit) {
		a.overflow = true
		return
	}
	a.addMicros(currency, int64(math.Round(amount*float64(amountMicrosPerUnit))))
}

func (a *adminMoneyAccumulator) addMicros(currency string, amountMicros int64) {
	if amountMicros == 0 {
		return
	}
	if a.values == nil {
		a.values = map[string]int64{}
	}
	currency = strings.TrimSpace(currency)
	value, ok := checkedAddInt64(a.values[currency], amountMicros)
	if !ok {
		a.overflow = true
		return
	}
	a.values[currency] = value
}

func (a adminMoneyAccumulator) amount(currency string) float64 {
	return float64(a.amountMicros(currency)) / float64(amountMicrosPerUnit)
}

func (a adminMoneyAccumulator) amountMicros(currency string) int64 {
	return a.values[strings.TrimSpace(currency)]
}

func (a adminMoneyAccumulator) breakdown() []dto.AdminAnalyticsMoneyBreakdown {
	return a.breakdownWithPreferredCurrency("")
}

func (a adminMoneyAccumulator) breakdownWithPreferredCurrency(currency string) []dto.AdminAnalyticsMoneyBreakdown {
	if len(a.values) == 0 {
		return []dto.AdminAnalyticsMoneyBreakdown{}
	}
	items := make([]dto.AdminAnalyticsMoneyBreakdown, 0, len(a.values))
	for key, amountMicros := range a.values {
		if amountMicros == 0 {
			continue
		}
		items = append(items, dto.AdminAnalyticsMoneyBreakdown{
			Currency:     key,
			Amount:       float64(amountMicros) / float64(amountMicrosPerUnit),
			AmountMicros: strconv.FormatInt(amountMicros, 10),
		})
	}
	preferred := strings.TrimSpace(currency)
	sort.Slice(items, func(i, j int) bool {
		if preferred != "" {
			leftPreferred := items[i].Currency == preferred
			rightPreferred := items[j].Currency == preferred
			if leftPreferred != rightPreferred {
				return leftPreferred
			}
		}
		return items[i].Currency < items[j].Currency
	})
	return items
}

type adminSubscriptionValue struct {
	RecognizedRemainingValue       float64
	TimeBasedValue                 float64
	TokenBasedValue                float64
	RecognizedRemainingValueMicros int64
	TimeBasedValueMicros           int64
	TokenBasedValueMicros          int64
	ExactCostMicros                int64
	EstimatedCostMicros            int64
	UnknownCredit                  int64
	AvailableCredit                int64
	TokenBasedValueAvailable       bool
	TimeBasedValueAvailable        bool
	ValuationBasis                 string
	ValuationConfidence            string
	StateVersion                   int64
	UpdatedAt                      int64
	SnapshotSemantics              string
	Currency                       string
	RemainingSeconds               int64
}

func adminPlanDurationSeconds(start int64, plan *SubscriptionPlan) (int64, error) {
	if plan == nil {
		return 0, errors.New("plan is nil")
	}
	end, err := calcPlanEndTime(time.Unix(start, 0).UTC(), plan)
	if err != nil {
		return 0, err
	}
	if end <= start {
		return 0, errors.New("plan duration must be positive")
	}
	return end - start, nil
}

func adminSubscriptionPlanDurationSeconds(sub UserSubscription, plan SubscriptionPlan) (int64, error) {
	return adminPlanDurationSeconds(sub.StartTime, &plan)
}

func adminSubscriptionTimeValue(sub UserSubscription, plan SubscriptionPlan, snapshotAt int64) (float64, error) {
	planDurationSeconds, err := adminSubscriptionPlanDurationSeconds(sub, plan)
	if err != nil {
		return 0, err
	}
	remainingSeconds := sub.EndTime - snapshotAt
	if remainingSeconds < 0 {
		remainingSeconds = 0
	}
	return plan.PriceAmount * float64(remainingSeconds) / float64(planDurationSeconds), nil
}

func adminSubscriptionTokenValue(sub UserSubscription, plan SubscriptionPlan, snapshotAt int64, planDurationSeconds int64) (float64, bool, error) {
	if sub.TokenLimit <= 0 {
		return 0, false, nil
	}
	if planDurationSeconds <= 0 {
		return 0, true, errors.New("plan duration must be positive")
	}
	remainingTokens := sub.TokenLimit - sub.TokenUsed
	if remainingTokens < 0 {
		remainingTokens = 0
	}
	remainingTokenRatio := float64(remainingTokens) / float64(sub.TokenLimit)
	period := NormalizeResetPeriod(plan.QuotaResetPeriod)
	if period == SubscriptionResetNever {
		return plan.PriceAmount * remainingTokenRatio, true, nil
	}

	currentCycleSeconds := adminCurrentTokenCycleSeconds(sub, plan, snapshotAt, planDurationSeconds)
	if currentCycleSeconds <= 0 {
		currentCycleSeconds = planDurationSeconds
	}
	currentCycleValue := adminTokenCycleValue(period, plan.PriceAmount, currentCycleSeconds, planDurationSeconds) * remainingTokenRatio
	futureValue := adminFutureTokenCyclesValue(sub, plan, snapshotAt, planDurationSeconds, period)
	return currentCycleValue + futureValue, true, nil
}

func adminCurrentTokenCycleSeconds(sub UserSubscription, plan SubscriptionPlan, snapshotAt int64, planDurationSeconds int64) int64 {
	period := NormalizeResetPeriod(plan.QuotaResetPeriod)
	cycleStart, cycleEnd := adminTokenCycleBounds(sub, plan, snapshotAt, planDurationSeconds, period)
	if cycleEnd <= cycleStart {
		return planDurationSeconds
	}
	return cycleEnd - cycleStart
}

func adminTokenCycleBounds(sub UserSubscription, plan SubscriptionPlan, snapshotAt int64, planDurationSeconds int64, period string) (int64, int64) {
	if adminStoredResetWindowMatchesPeriod(sub, plan, snapshotAt, period) {
		return sub.LastResetTime, sub.NextResetTime
	}
	snapshot := time.Unix(snapshotAt, 0).UTC()
	switch period {
	case SubscriptionResetDaily:
		start := time.Date(snapshot.Year(), snapshot.Month(), snapshot.Day(), 0, 0, 0, 0, time.UTC).Unix()
		return start, adminNextResetAfter(time.Unix(start, 0).UTC(), &plan, 0)
	case SubscriptionResetWeekly:
		weekday := int(snapshot.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		start := time.Date(snapshot.Year(), snapshot.Month(), snapshot.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, 1-weekday).Unix()
		return start, adminNextResetAfter(time.Unix(start, 0).UTC(), &plan, 0)
	case SubscriptionResetMonthly:
		start := time.Date(snapshot.Year(), snapshot.Month(), 1, 0, 0, 0, 0, time.UTC).Unix()
		return start, adminNextResetAfter(time.Unix(start, 0).UTC(), &plan, 0)
	case SubscriptionResetCustom:
		if plan.QuotaResetCustomSeconds > 0 {
			cycleSeconds := plan.QuotaResetCustomSeconds
			base := sub.StartTime
			if sub.LastResetTime > 0 {
				base = sub.LastResetTime
			}
			if snapshotAt >= base {
				elapsedCycles := (snapshotAt - base) / cycleSeconds
				start := base + elapsedCycles*cycleSeconds
				return start, start + cycleSeconds
			}
			return base, base + cycleSeconds
		}
	}
	return sub.StartTime, sub.StartTime + planDurationSeconds
}

func adminStoredResetWindowMatchesPeriod(sub UserSubscription, plan SubscriptionPlan, snapshotAt int64, period string) bool {
	if sub.LastResetTime <= 0 || sub.NextResetTime <= sub.LastResetTime || sub.LastResetTime > snapshotAt || snapshotAt >= sub.NextResetTime {
		return false
	}
	window := sub.NextResetTime - sub.LastResetTime
	switch period {
	case SubscriptionResetDaily:
		return window == 86400
	case SubscriptionResetWeekly:
		return window == 7*86400
	case SubscriptionResetMonthly:
		return true
	case SubscriptionResetCustom:
		return plan.QuotaResetCustomSeconds > 0 && window == plan.QuotaResetCustomSeconds
	default:
		return false
	}
}

func adminTokenCycleValue(period string, price float64, cycleSeconds int64, planDurationSeconds int64) float64 {
	if planDurationSeconds <= 0 || cycleSeconds <= 0 {
		return 0
	}
	if period == SubscriptionResetMonthly {
		return price
	}
	return price * float64(cycleSeconds) / float64(planDurationSeconds)
}

func adminFutureTokenCyclesValue(sub UserSubscription, plan SubscriptionPlan, snapshotAt int64, planDurationSeconds int64, period string) float64 {
	if sub.EndTime <= snapshotAt || planDurationSeconds <= 0 {
		return 0
	}
	nextReset := sub.NextResetTime
	if nextReset <= snapshotAt || nextReset > sub.EndTime {
		nextReset = adminNextResetAfter(time.Unix(snapshotAt, 0).UTC(), &plan, sub.EndTime)
	}
	if nextReset <= snapshotAt || nextReset >= sub.EndTime {
		return 0
	}

	value := 0.0
	for cursor := nextReset; cursor < sub.EndTime; {
		cycleEnd := adminNextTokenCycleEnd(cursor, plan, period, planDurationSeconds)
		if cycleEnd <= cursor {
			cycleEnd = sub.EndTime
		}
		segmentEnd := cycleEnd
		if segmentEnd > sub.EndTime {
			segmentEnd = sub.EndTime
		}
		cycleSeconds := cycleEnd - cursor
		segmentSeconds := segmentEnd - cursor
		if cycleSeconds <= 0 || segmentSeconds <= 0 {
			break
		}
		if segmentEnd < cycleEnd {
			value += plan.PriceAmount * float64(segmentSeconds) / float64(planDurationSeconds)
		} else {
			value += adminTokenCycleValue(period, plan.PriceAmount, cycleSeconds, planDurationSeconds)
		}
		cursor = segmentEnd
	}
	return value
}

func adminNextTokenCycleEnd(cursor int64, plan SubscriptionPlan, period string, planDurationSeconds int64) int64 {
	if cursor <= 0 {
		return 0
	}
	base := time.Unix(cursor, 0).UTC()
	switch period {
	case SubscriptionResetDaily:
		return base.AddDate(0, 0, 1).Unix()
	case SubscriptionResetWeekly:
		return base.AddDate(0, 0, 7).Unix()
	case SubscriptionResetMonthly:
		return adminNextMonthlyTokenCycleEnd(cursor, plan)
	case SubscriptionResetCustom:
		if plan.QuotaResetCustomSeconds > 0 {
			return cursor + plan.QuotaResetCustomSeconds
		}
	}
	if planDurationSeconds > 0 {
		return cursor + planDurationSeconds
	}
	return 0
}

func adminNextMonthlyTokenCycleEnd(cursor int64, plan SubscriptionPlan) int64 {
	baseLocal := time.Unix(cursor, 0).In(time.Local)
	if adminIsMonthResetBoundary(baseLocal) {
		if next := calcNextResetTime(baseLocal, &plan, 0); next > cursor {
			return next
		}
		return 0
	}
	baseUTC := time.Unix(cursor, 0).UTC()
	if adminIsMonthResetBoundary(baseUTC) {
		return baseUTC.AddDate(0, 1, 0).Unix()
	}
	if next := calcNextResetTime(baseLocal, &plan, 0); next > cursor {
		return next
	}
	return 0
}

func adminIsMonthResetBoundary(value time.Time) bool {
	return value.Day() == 1 && value.Hour() == 0 && value.Minute() == 0 && value.Second() == 0 && value.Nanosecond() == 0
}

func adminNextResetAfter(base time.Time, plan *SubscriptionPlan, endUnix int64) int64 {
	return calcNextResetTime(base.UTC(), plan, endUnix)
}

func adminRecognizedRemainingValue(sub UserSubscription, plan SubscriptionPlan, snapshotAt int64) (adminSubscriptionValue, error) {
	planDurationSeconds, err := adminSubscriptionPlanDurationSeconds(sub, plan)
	if err != nil {
		return adminSubscriptionValue{}, err
	}
	remainingSeconds := sub.EndTime - snapshotAt
	if remainingSeconds < 0 {
		remainingSeconds = 0
	}
	timeValue := plan.PriceAmount * float64(remainingSeconds) / float64(planDurationSeconds)
	tokenValue, tokenAvailable, err := adminSubscriptionTokenValue(sub, plan, snapshotAt, planDurationSeconds)
	if err != nil {
		return adminSubscriptionValue{}, err
	}
	result := adminSubscriptionValue{
		TimeBasedValue:           timeValue,
		TokenBasedValue:          tokenValue,
		TokenBasedValueAvailable: tokenAvailable,
		TimeBasedValueAvailable:  true,
		Currency:                 strings.TrimSpace(plan.Currency),
		RemainingSeconds:         remainingSeconds,
		SnapshotSemantics:        adminPaidSubscriptionSnapshotSemanticsSnapshot,
	}
	if !tokenAvailable {
		result.RecognizedRemainingValue = timeValue
		result.ValuationBasis = adminPaidSubscriptionValuationTimeOnly
	} else {
		result.RecognizedRemainingValue = math.Min(timeValue, tokenValue)
		if NormalizeResetPeriod(plan.QuotaResetPeriod) == SubscriptionResetNever {
			result.ValuationBasis = adminPaidSubscriptionValuationNeverReset
		} else {
			result.ValuationBasis = adminPaidSubscriptionValuationTokenAndTime
		}
	}
	result.RecognizedRemainingValueMicros, err = adminPaidFloatAmountMicros(result.RecognizedRemainingValue)
	if err != nil {
		return adminSubscriptionValue{}, err
	}
	result.TimeBasedValueMicros, err = adminPaidFloatAmountMicros(result.TimeBasedValue)
	if err != nil {
		return adminSubscriptionValue{}, err
	}
	result.TokenBasedValueMicros, err = adminPaidFloatAmountMicros(result.TokenBasedValue)
	if err != nil {
		return adminSubscriptionValue{}, err
	}
	return result, nil
}

func adminPaidFloatAmountMicros(amount float64) (int64, error) {
	if math.IsNaN(amount) || math.IsInf(amount, 0) || amount > float64(math.MaxInt64)/float64(amountMicrosPerUnit) || amount < float64(math.MinInt64)/float64(amountMicrosPerUnit) {
		return 0, ErrCreditValuationOverflow
	}
	return int64(math.Round(amount * float64(amountMicrosPerUnit))), nil
}

type adminPaidSubscriptionRow struct {
	Subscription      UserSubscription
	Plan              SubscriptionPlan
	User              User
	Source            dto.AdminAnalyticsSource
	SourceAttribution string
	Value             adminSubscriptionValue
	TimedValue        *adminTimedSubscriptionValue
	Active            bool
	StateMissing      bool
	StateMismatch     bool
	Excluded          bool
	ExcludedReason    string
	ExcludedAt        int64
	ExcludedBy        int
	Order             *SubscriptionOrder
}

func adminPaidRowAccumulateValues(row adminPaidSubscriptionRow, recognized *adminMoneyAccumulator, token *adminMoneyAccumulator, timeBased *adminMoneyAccumulator) {
	if row.TimedValue == nil {
		currency := adminPaidSubscriptionRowCurrency(row)
		recognized.addMicros(currency, row.Value.RecognizedRemainingValueMicros)
		timeBased.addMicros(currency, row.Value.TimeBasedValueMicros)
		if row.Value.TokenBasedValueAvailable {
			token.addMicros(currency, row.Value.TokenBasedValueMicros)
		}
		return
	}
	for currency, value := range row.TimedValue.ByCurrency {
		recognized.addMicros(currency, value.RecognizedMicros)
		timeBased.addMicros(currency, value.TimeMicros)
		if row.TimedValue.TokenAvailable {
			token.addMicros(currency, value.TokenMicros)
		}
	}
}

func adminPaidRowAccumulateRecognized(row adminPaidSubscriptionRow, accumulator *adminMoneyAccumulator) {
	if row.TimedValue == nil {
		accumulator.addMicros(adminPaidSubscriptionRowCurrency(row), row.Value.RecognizedRemainingValueMicros)
		return
	}
	for currency, value := range row.TimedValue.ByCurrency {
		accumulator.addMicros(currency, value.RecognizedMicros)
	}
}

func adminTimedSourcesMatchQuery(value adminTimedSubscriptionValue, sources []dto.AdminAnalyticsSource) bool {
	if len(sources) == 0 {
		return true
	}
	for _, source := range value.Sources {
		if adminSourceInSet(source, sources) {
			return true
		}
	}
	return false
}

func adminTimedSourceProjection(value adminTimedSubscriptionValue) (dto.AdminAnalyticsSource, string) {
	switch len(value.Sources) {
	case 0:
		return dto.AdminAnalyticsSourceUnknown, adminPaidSubscriptionSourceAttributionMixedOrUnknown
	case 1:
		return value.Sources[0], adminPaidSubscriptionSourceAttributionSnapshot
	default:
		return dto.AdminAnalyticsSourceUnknown, adminTimedSourceAttributionMixed
	}
}

func adminTimedGrantsBySubscriptionID(subscriptionIDs []int) (map[int][]TimedSubscriptionValuationGrant, error) {
	result := make(map[int][]TimedSubscriptionValuationGrant)
	ids := adminUniquePositiveInts(subscriptionIDs)
	if len(ids) == 0 {
		return result, nil
	}
	var grants []TimedSubscriptionValuationGrant
	if err := DB.Where("user_subscription_id IN ?", ids).Order("created_at asc, id asc").Find(&grants).Error; err != nil {
		return nil, err
	}
	for i := range grants {
		grant := grants[i]
		result[grant.UserSubscriptionId] = append(result[grant.UserSubscriptionId], grant)
	}
	return result, nil
}

type adminOrderLookupKey struct {
	UserID int
	PlanID int
}

func adminIsNonSalesGiftSubscription(sub UserSubscription) bool {
	return adminIsNonSalesGiftValue(sub.GrantReason) || adminIsNonSalesGiftValue(sub.Source)
}

func adminIsNonSalesGiftValue(value string) bool {
	switch strings.TrimSpace(value) {
	case SubscriptionGrantMonthlyInviteEntitlement, "invite_trial", "trial_code":
		return true
	default:
		return false
	}
}

func adminPaidSourceAttribution(sub UserSubscription) string {
	if adminIsNonSalesGiftSubscription(sub) {
		return adminPaidSubscriptionSourceAttributionMixedOrUnknown
	}
	return adminPaidSubscriptionSourceAttributionSnapshot
}
func adminCreditPaidSubscriptionValue(sub UserSubscription, plan SubscriptionPlan, state CreditValuationState, found bool, snapshotAt int64) (adminSubscriptionValue, bool, bool, error) {
	available := maxInt64(sub.TokenLimit-sub.TokenUsed, 0)
	value := adminSubscriptionValue{
		AvailableCredit:          available,
		TokenBasedValueAvailable: true,
		ValuationBasis:           adminPaidSubscriptionValuationCreditPool,
		ValuationConfidence:      CreditValuationConfidenceUnknown,
		SnapshotSemantics:        adminPaidSubscriptionSnapshotSemanticsSnapshot,
		Currency:                 adminCreditPaidSubscriptionCurrency(plan),
	}
	if !found {
		return value, available > 0, true, nil
	}
	if err := validateCreditValuationState(&sub, &state); err != nil || value.Currency == "" || state.Currency != value.Currency {
		return value, available > 0, true, nil
	}
	recognizedMicros, ok := checkedAddInt64(state.ExactCostMicros, state.EstimatedCostMicros)
	if !ok {
		return adminSubscriptionValue{}, false, false, ErrCreditValuationOverflow
	}
	value.RecognizedRemainingValueMicros = recognizedMicros
	value.TokenBasedValueMicros = recognizedMicros
	value.ExactCostMicros = state.ExactCostMicros
	value.EstimatedCostMicros = state.EstimatedCostMicros
	value.UnknownCredit = state.UnknownCredit
	value.AvailableCredit = state.AvailableCredit
	value.RecognizedRemainingValue = float64(recognizedMicros) / float64(amountMicrosPerUnit)
	value.TokenBasedValue = value.RecognizedRemainingValue
	value.ValuationConfidence = adminCreditPaidSubscriptionConfidence(state)
	value.StateVersion = state.StateVersion
	value.UpdatedAt = state.UpdatedAt
	value.Currency = state.Currency
	if state.UpdatedAt > snapshotAt {
		value.SnapshotSemantics = adminPaidSubscriptionSnapshotSemanticsCurrentOnly
	}
	return value, state.AvailableCredit > 0, false, nil
}

func adminCreditPaidSubscriptionCurrency(plan SubscriptionPlan) string {
	currency := strings.TrimSpace(plan.Currency)
	if plan.ValuationCurrency != nil && strings.TrimSpace(*plan.ValuationCurrency) != "" {
		currency = strings.TrimSpace(*plan.ValuationCurrency)
	}
	normalized, err := NormalizeCreditValuationCurrency(currency)
	if err != nil {
		return ""
	}
	return normalized
}

func adminCreditPaidSubscriptionConfidence(state CreditValuationState) string {
	kinds := 0
	if state.ExactCostMicros > 0 {
		kinds++
	}
	if state.EstimatedCostMicros > 0 {
		kinds++
	}
	if state.UnknownCredit > 0 {
		kinds++
	}
	if kinds > 1 {
		return "mixed"
	}
	if state.ExactCostMicros > 0 {
		return CreditValuationConfidenceExact
	}
	if state.EstimatedCostMicros > 0 {
		return CreditValuationConfidenceEstimated
	}
	return CreditValuationConfidenceUnknown
}

func adminPaidSubscriptionRowCurrency(row adminPaidSubscriptionRow) string {
	if strings.TrimSpace(row.Value.Currency) != "" {
		return strings.TrimSpace(row.Value.Currency)
	}
	return strings.TrimSpace(row.Plan.Currency)
}

func loadAdminPaidSubscriptionValueRows(query AdminAnalyticsQuery, filterSubscriptionID bool) ([]adminPaidSubscriptionRow, error) {
	query = normalizeAdminPaidSubscriptionAnalyticsQuery(query)
	var subs []UserSubscription
	db := applyAdminActiveSubscriptionScope(DB.Model(&UserSubscription{}), query.SnapshotAt)
	if len(query.PlanIDs) > 0 {
		db = db.Where("plan_id IN ?", query.PlanIDs)
	}
	if len(query.UserIDs) > 0 {
		db = db.Where("user_id IN ?", query.UserIDs)
	}
	if len(query.GrantReasons) > 0 {
		db = db.Where("grant_reason IN ?", query.GrantReasons)
	}
	if filterSubscriptionID && query.SubscriptionID > 0 {
		db = db.Where("id = ?", query.SubscriptionID)
	}
	if err := db.Find(&subs).Error; err != nil {
		return nil, err
	}
	return adminBuildPaidRowsFromSubscriptions(subs, query)
}

func adminBuildPaidRowsFromSubscriptions(subs []UserSubscription, query AdminAnalyticsQuery) ([]adminPaidSubscriptionRow, error) {
	userIDs := make([]int, 0, len(subs))
	planIDs := make([]int, 0, len(subs))
	subscriptionIDs := make([]int, 0, len(subs))
	creditSubscriptionIDs := make([]int, 0, len(subs))
	timedUserIDs := make([]int, 0, len(subs))
	timedPlanIDs := make([]int, 0, len(subs))
	for i := range subs {
		userIDs = append(userIDs, subs[i].UserId)
		planIDs = append(planIDs, subs[i].PlanId)
		if subs[i].EntitlementType == SubscriptionEntitlementCreditBalance {
			creditSubscriptionIDs = append(creditSubscriptionIDs, subs[i].Id)
		} else {
			subscriptionIDs = append(subscriptionIDs, subs[i].Id)
			timedUserIDs = append(timedUserIDs, subs[i].UserId)
			timedPlanIDs = append(timedPlanIDs, subs[i].PlanId)
		}
	}
	users, err := adminUsersByID(userIDs)
	if err != nil {
		return nil, err
	}
	plans, err := adminPlansByID(planIDs)
	if err != nil {
		return nil, err
	}
	grantsBySubscriptionID, err := adminTimedGrantsBySubscriptionID(subscriptionIDs)
	if err != nil {
		return nil, err
	}
	valuationReady, err := CreditValuationRuntimeReadyTx(DB)
	if err != nil {
		return nil, err
	}
	states := map[int]CreditValuationState{}
	if valuationReady && len(creditSubscriptionIDs) > 0 {
		var stateRows []CreditValuationState
		if err := DB.Where("user_subscription_id IN ?", adminUniquePositiveInts(creditSubscriptionIDs)).Find(&stateRows).Error; err != nil {
			return nil, err
		}
		for i := range stateRows {
			states[stateRows[i].UserSubscriptionId] = stateRows[i]
		}
	}
	excludedUsers := setting.GetSubscriptionAnalyticsExcludedUsers()
	orders, err := adminBestSubscriptionOrders(timedUserIDs, timedPlanIDs)
	if err != nil {
		return nil, err
	}
	rows := make([]adminPaidSubscriptionRow, 0, len(subs))
	for i := range subs {
		sub := subs[i]
		user, ok := users[sub.UserId]
		if !ok || !adminPaidUserMatchesQuery(user, query) {
			continue
		}
		plan, ok := plans[sub.PlanId]
		if !ok || !adminPaidPlanMatchesQuery(plan, query) {
			continue
		}
		excluded := excludedUsers[sub.UserId]
		if sub.EntitlementType == SubscriptionEntitlementCreditBalance {
			if !valuationReady {
				continue
			}
			source := dto.AdminAnalyticsSourceCreditBalancePool
			if len(query.Sources) > 0 && !adminSourceInSet(source, query.Sources) {
				continue
			}
			state, found := states[sub.Id]
			value, active, stateMissing, err := adminCreditPaidSubscriptionValue(sub, plan, state, found, query.SnapshotAt)
			if err != nil {
				return nil, err
			}
			rows = append(rows, adminPaidSubscriptionRow{
				Subscription:      sub,
				Plan:              plan,
				User:              user,
				Source:            source,
				SourceAttribution: adminPaidSubscriptionSourceAttributionCreditPool,
				Value:             value,
				Active:            active,
				StateMissing:      stateMissing,
				Excluded:          excluded.UserID > 0,
				ExcludedReason:    excluded.Reason,
				ExcludedAt:        excluded.ExcludedAt,
				ExcludedBy:        excluded.ExcludedBy,
			})
			continue
		}
		if sub.EntitlementType != SubscriptionEntitlementTimed || adminIsNonSalesGiftSubscription(sub) {
			continue
		}
		timedValue, calcErr := adminCalculateTimedSubscriptionValue(sub, grantsBySubscriptionID[sub.Id], query.SnapshotAt)
		if calcErr != nil {
			return nil, calcErr
		}
		if !adminTimedSourcesMatchQuery(timedValue, query.Sources) {
			continue
		}
		source, sourceAttribution := adminTimedSourceProjection(timedValue)
		remainingSeconds := sub.EndTime - query.SnapshotAt
		if remainingSeconds < 0 {
			remainingSeconds = 0
		}
		rows = append(rows, adminPaidSubscriptionRow{
			Subscription:      sub,
			Plan:              plan,
			User:              user,
			Source:            source,
			SourceAttribution: sourceAttribution,
			Value: adminSubscriptionValue{
				TokenBasedValueAvailable: timedValue.TokenAvailable,
				TimeBasedValueAvailable:  true,
				ValuationBasis:           adminTimedValuationBasisGrantTimeline,
				RemainingSeconds:         remainingSeconds,
			},
			TimedValue:     &timedValue,
			Active:         true,
			Excluded:       excluded.UserID > 0,
			ExcludedReason: excluded.Reason,
			ExcludedAt:     excluded.ExcludedAt,
			ExcludedBy:     excluded.ExcludedBy,
			Order:          orders[adminOrderLookupKey{UserID: sub.UserId, PlanID: sub.PlanId}],
		})
	}
	return rows, nil
}

func adminPaidUserMatchesQuery(user User, query AdminAnalyticsQuery) bool {
	if len(query.UserStatuses) > 0 && !adminIntInSet(user.Status, query.UserStatuses) {
		return false
	}
	if query.RegisteredStartTimestamp > 0 && user.CreatedAt < query.RegisteredStartTimestamp {
		return false
	}
	if query.RegisteredEndTimestamp > 0 && user.CreatedAt > query.RegisteredEndTimestamp {
		return false
	}
	if query.InviterID > 0 && user.InviterId != query.InviterID {
		return false
	}
	if query.InviteeID > 0 && user.Id != query.InviteeID {
		return false
	}
	if query.Username != "" && user.Username != query.Username {
		return false
	}
	return true
}

func adminPaidPlanMatchesQuery(plan SubscriptionPlan, query AdminAnalyticsQuery) bool {
	if len(query.BusinessCodes) > 0 && !adminStringInSet(subscriptionTierKey(&plan), query.BusinessCodes) {
		return false
	}
	if query.Trial != nil && plan.IsTrial != *query.Trial && plan.InviteTrial != *query.Trial {
		return false
	}
	if query.RewardEligible != nil && plan.RewardEligible != *query.RewardEligible {
		return false
	}
	return true
}

func adminBestSubscriptionOrders(userIDs []int, planIDs []int) (map[adminOrderLookupKey]*SubscriptionOrder, error) {
	result := map[adminOrderLookupKey]*SubscriptionOrder{}
	uniqueUserIDs := adminUniquePositiveInts(userIDs)
	uniquePlanIDs := adminUniquePositiveInts(planIDs)
	if len(uniqueUserIDs) == 0 || len(uniquePlanIDs) == 0 {
		return result, nil
	}
	var orders []SubscriptionOrder
	if err := DB.Where("user_id IN ? AND plan_id IN ?", uniqueUserIDs, uniquePlanIDs).
		Order("complete_time desc, id desc").
		Find(&orders).Error; err != nil {
		return nil, err
	}
	for i := range orders {
		order := orders[i]
		key := adminOrderLookupKey{UserID: order.UserId, PlanID: order.PlanId}
		if _, ok := result[key]; ok {
			continue
		}
		result[key] = &orders[i]
	}
	return result, nil
}

func adminShouldShowExcludedRow(excluded bool, mode dto.AdminAnalyticsExcludedMode) bool {
	switch mode {
	case dto.AdminAnalyticsExcludedModeIncludeExcluded:
		return true
	case dto.AdminAnalyticsExcludedModeExcludedOnly:
		return excluded
	default:
		return !excluded
	}
}

func adminInvitationRelationshipRequiresPaidScope(query AdminAnalyticsQuery) bool {
	return len(query.PlanIDs) > 0 || len(query.Sources) > 0 || len(query.GrantReasons) > 0 || len(query.BusinessCodes) > 0
}

func adminIncludeInMain(excluded bool, mode dto.AdminAnalyticsExcludedMode) bool {
	return !excluded && mode != dto.AdminAnalyticsExcludedModeExcludedOnly
}

type adminPaidSubscriptionValueBuild struct {
	Summary       dto.AdminPaidSubscriptionValueSummary
	Users         []dto.AdminPaidSubscriptionValueUser
	Subscriptions []dto.AdminPaidSubscriptionValueSubscription
	Plans         []dto.AdminPaidSubscriptionValuePlanGroup
	Sources       []dto.AdminPaidSubscriptionValueSourceGroup
	Warnings      []dto.AdminAnalyticsAvailabilityWarning
}

func buildAdminPaidSubscriptionValueData(query AdminAnalyticsQuery) (adminPaidSubscriptionValueBuild, error) {
	query = normalizeAdminPaidSubscriptionAnalyticsQuery(query)
	rows, err := loadAdminPaidSubscriptionValueRows(query, false)
	if err != nil {
		return adminPaidSubscriptionValueBuild{}, err
	}
	return adminBuildPaidSubscriptionValueDataFromRows(query, rows, true)
}

func adminBuildPaidSubscriptionValueDataFromRows(query AdminAnalyticsQuery, rows []adminPaidSubscriptionRow, applySubscriptionIDToList bool) (adminPaidSubscriptionValueBuild, error) {
	recognized := adminMoneyAccumulator{}
	token := adminMoneyAccumulator{}
	timeBased := adminMoneyAccumulator{}
	exact := adminMoneyAccumulator{}
	estimated := adminMoneyAccumulator{}
	excluded := adminMoneyAccumulator{}
	mainUserIDs := map[int]struct{}{}
	activePaidSubscriptionCount := 0
	tokenUnavailableCount := 0
	unknownCostCredit := int64(0)
	stateMissingCount := 0
	currentOnly := false

	userGroups := map[int]*adminPaidUserGroup{}
	planGroups := map[int]*adminPaidPlanGroup{}
	sourceGroups := map[adminPaidSourceKey]*adminPaidSourceGroup{}
	subscriptionItems := make([]dto.AdminPaidSubscriptionValueSubscription, 0, len(rows))

	unknownTimedSubscriptionCount := 0
	for i := range rows {
		row := rows[i]
		if row.Value.SnapshotSemantics == adminPaidSubscriptionSnapshotSemanticsCurrentOnly {
			currentOnly = true
		}
		currency := adminPaidSubscriptionRowCurrency(row)
		if row.Excluded {
			excluded.addMicros(currency, row.Value.RecognizedRemainingValueMicros)
		}
		main := adminIncludeInMain(row.Excluded, query.ExcludedMode) && row.Active
		if main {
			adminPaidRowAccumulateValues(row, &recognized, &token, &timeBased)
			exact.addMicros(currency, row.Value.ExactCostMicros)
			estimated.addMicros(currency, row.Value.EstimatedCostMicros)
			if !row.Value.TokenBasedValueAvailable {
				tokenUnavailableCount++
			}
			var ok bool
			unknownCostCredit, ok = checkedAddInt64(unknownCostCredit, row.Value.UnknownCredit)
			if !ok {
				return adminPaidSubscriptionValueBuild{}, ErrCreditValuationOverflow
			}
			if row.StateMissing {
				stateMissingCount++
			}
			if row.TimedValue != nil && row.TimedValue.Unknown {
				unknownTimedSubscriptionCount++
			}
			activePaidSubscriptionCount++
			mainUserIDs[row.User.Id] = struct{}{}
		}
		if err := adminAccumulatePaidUserGroup(userGroups, row, main); err != nil {
			return adminPaidSubscriptionValueBuild{}, err
		}
		if err := adminAccumulatePaidPlanGroup(planGroups, row, main); err != nil {
			return adminPaidSubscriptionValueBuild{}, err
		}
		if err := adminAccumulatePaidSourceGroup(sourceGroups, row, main); err != nil {
			return adminPaidSubscriptionValueBuild{}, err
		}
		if adminShouldShowExcludedRow(row.Excluded, query.ExcludedMode) && (!applySubscriptionIDToList || query.SubscriptionID <= 0 || row.Subscription.Id == query.SubscriptionID) {
			subscriptionItems = append(subscriptionItems, adminPaidSubscriptionItem(row))
		}
	}
	if recognized.overflow || token.overflow || timeBased.overflow || exact.overflow || estimated.overflow || excluded.overflow {
		return adminPaidSubscriptionValueBuild{}, ErrCreditValuationOverflow
	}

	users := adminPaidUserItems(userGroups, query)
	plans := adminPaidPlanItems(planGroups, query)
	sources := adminPaidSourceItems(sourceGroups, query)
	if err := adminSortPaidSubscriptionUsers(users, query); err != nil {
		return adminPaidSubscriptionValueBuild{}, err
	}
	if err := adminSortPaidSubscriptionPlans(plans, query); err != nil {
		return adminPaidSubscriptionValueBuild{}, err
	}
	if err := adminSortPaidSubscriptionSources(sources, query); err != nil {
		return adminPaidSubscriptionValueBuild{}, err
	}
	if err := adminSortPaidSubscriptionItems(subscriptionItems, query); err != nil {
		return adminPaidSubscriptionValueBuild{}, err
	}

	warnings := make([]dto.AdminAnalyticsAvailabilityWarning, 0, 1)
	if currentOnly {
		warnings = append(warnings, dto.AdminAnalyticsAvailabilityWarning{
			Section: "credit_valuation",
			Reason:  adminPaidSubscriptionSnapshotSemanticsCurrentOnly,
			Message: "credit valuation state is newer than snapshot",
		})
	}

	return adminPaidSubscriptionValueBuild{
		Summary: dto.AdminPaidSubscriptionValueSummary{
			RecognizedRemainingValueByCurrency: recognized.breakdownWithPreferredCurrency(query.Currency),
			TokenBasedValueByCurrency:          token.breakdownWithPreferredCurrency(query.Currency),
			TimeBasedValueByCurrency:           timeBased.breakdownWithPreferredCurrency(query.Currency),
			ExactRemainingValueByCurrency:      exact.breakdownWithPreferredCurrency(query.Currency),
			EstimatedRemainingValueByCurrency:  estimated.breakdownWithPreferredCurrency(query.Currency),
			ExcludedRemainingValueByCurrency:   excluded.breakdownWithPreferredCurrency(query.Currency),
			ActivePaidSubscriptionCount:        activePaidSubscriptionCount,
			ActivePaidUserCount:                len(mainUserIDs),
			TokenValueUnavailableCount:         tokenUnavailableCount,
			UnknownCostCredit:                  unknownCostCredit,
			UnknownTimedSubscriptionCount:      unknownTimedSubscriptionCount,
			CreditValuationStateMissingCount:   stateMissingCount,
		},
		Users:         users,
		Subscriptions: subscriptionItems,
		Plans:         plans,
		Sources:       sources,
		Warnings:      warnings,
	}, nil
}

type adminPaidUserGroup struct {
	User            User
	Excluded        bool
	ExcludedReason  string
	ExcludedAt      int64
	ExcludedBy      int
	MainCount       int
	WouldHaveCount  int
	Recognized      adminMoneyAccumulator
	Token           adminMoneyAccumulator
	TimeBased       adminMoneyAccumulator
	Exact           adminMoneyAccumulator
	Estimated       adminMoneyAccumulator
	UnknownCredit   int64
	WouldHave       adminMoneyAccumulator
	EarliestEndTime int64
}

func adminAccumulatePaidUserGroup(groups map[int]*adminPaidUserGroup, row adminPaidSubscriptionRow, main bool) error {
	group := groups[row.User.Id]
	if group == nil {
		group = &adminPaidUserGroup{
			User:           row.User,
			Excluded:       row.Excluded,
			ExcludedReason: row.ExcludedReason,
			ExcludedAt:     row.ExcludedAt,
			ExcludedBy:     row.ExcludedBy,
		}
		groups[row.User.Id] = group
	}
	currency := adminPaidSubscriptionRowCurrency(row)
	if row.Excluded {
		adminPaidRowAccumulateRecognized(row, &group.WouldHave)
		group.WouldHaveCount++
	} else if main {
		adminPaidRowAccumulateValues(row, &group.Recognized, &group.Token, &group.TimeBased)
		group.Exact.addMicros(currency, row.Value.ExactCostMicros)
		group.Estimated.addMicros(currency, row.Value.EstimatedCostMicros)
		var ok bool
		group.UnknownCredit, ok = checkedAddInt64(group.UnknownCredit, row.Value.UnknownCredit)
		if !ok {
			return ErrCreditValuationOverflow
		}
		group.MainCount++
	}
	if group.EarliestEndTime == 0 || (row.Subscription.EndTime > 0 && row.Subscription.EndTime < group.EarliestEndTime) {
		group.EarliestEndTime = row.Subscription.EndTime
	}
	return nil
}

func adminPaidUserItems(groups map[int]*adminPaidUserGroup, query AdminAnalyticsQuery) []dto.AdminPaidSubscriptionValueUser {
	items := make([]dto.AdminPaidSubscriptionValueUser, 0, len(groups))
	for _, group := range groups {
		if !adminShouldShowExcludedRow(group.Excluded, query.ExcludedMode) {
			continue
		}
		if group.Excluded && group.WouldHaveCount == 0 {
			continue
		}
		if !group.Excluded && group.MainCount == 0 {
			continue
		}
		userID := group.User.Id
		items = append(items, dto.AdminPaidSubscriptionValueUser{
			UserID:                             group.User.Id,
			Username:                           group.User.Username,
			DisplayName:                        group.User.DisplayName,
			ActivePaidPlanCount:                group.MainCount,
			RecognizedRemainingValueByCurrency: group.Recognized.breakdownWithPreferredCurrency(query.Currency),
			TokenBasedValueByCurrency:          group.Token.breakdownWithPreferredCurrency(query.Currency),
			TimeBasedValueByCurrency:           group.TimeBased.breakdownWithPreferredCurrency(query.Currency),
			ExactRemainingValueByCurrency:      group.Exact.breakdownWithPreferredCurrency(query.Currency),
			EstimatedRemainingValueByCurrency:  group.Estimated.breakdownWithPreferredCurrency(query.Currency),
			UnknownCostCredit:                  group.UnknownCredit,
			EarliestEndTime:                    group.EarliestEndTime,
			Excluded:                           group.Excluded,
			ExcludedReason:                     group.ExcludedReason,
			ExcludedAt:                         group.ExcludedAt,
			ExcludedBy:                         group.ExcludedBy,
			WouldHaveRemainingValueByCurrency:  group.WouldHave.breakdownWithPreferredCurrency(query.Currency),
			Drilldown:                          &dto.AdminAnalyticsDrilldownTarget{Kind: "paid_subscription_value_user", UserID: &userID, Tab: "paid-subscription-value"},
		})
	}
	return items
}

type adminPaidPlanGroup struct {
	Plan              SubscriptionPlan
	MainUserIDs       map[int]struct{}
	MainCount         int
	ExcludedCount     int
	Recognized        adminMoneyAccumulator
	Token             adminMoneyAccumulator
	TimeBased         adminMoneyAccumulator
	Exact             adminMoneyAccumulator
	Estimated         adminMoneyAccumulator
	UnknownCredit     int64
	ExcludedValue     adminMoneyAccumulator
	TokenUsageSum     float64
	TokenUsageSamples int
}

func adminAccumulatePaidPlanGroup(groups map[int]*adminPaidPlanGroup, row adminPaidSubscriptionRow, main bool) error {
	group := groups[row.Plan.Id]
	if group == nil {
		group = &adminPaidPlanGroup{Plan: row.Plan, MainUserIDs: map[int]struct{}{}}
		groups[row.Plan.Id] = group
	}
	currency := adminPaidSubscriptionRowCurrency(row)
	if row.Excluded {
		adminPaidRowAccumulateRecognized(row, &group.ExcludedValue)
		group.ExcludedCount++
	} else if main {
		adminPaidRowAccumulateValues(row, &group.Recognized, &group.Token, &group.TimeBased)
		group.Exact.addMicros(currency, row.Value.ExactCostMicros)
		group.Estimated.addMicros(currency, row.Value.EstimatedCostMicros)
		var ok bool
		group.UnknownCredit, ok = checkedAddInt64(group.UnknownCredit, row.Value.UnknownCredit)
		if !ok {
			return ErrCreditValuationOverflow
		}
		group.MainCount++
		group.MainUserIDs[row.User.Id] = struct{}{}
		if row.Subscription.TokenLimit > 0 {
			group.TokenUsageSum += float64(row.Subscription.TokenUsed) / float64(row.Subscription.TokenLimit)
			group.TokenUsageSamples++
		}
	}
	return nil
}

func adminPaidPlanItems(groups map[int]*adminPaidPlanGroup, query AdminAnalyticsQuery) []dto.AdminPaidSubscriptionValuePlanGroup {
	items := make([]dto.AdminPaidSubscriptionValuePlanGroup, 0, len(groups))
	for _, group := range groups {
		if group.MainCount == 0 && group.ExcludedCount == 0 {
			continue
		}
		if query.ExcludedMode == dto.AdminAnalyticsExcludedModeIncludedOnly && group.MainCount == 0 {
			continue
		}
		if query.ExcludedMode == dto.AdminAnalyticsExcludedModeExcludedOnly && group.ExcludedCount == 0 {
			continue
		}
		var average *float64
		if group.TokenUsageSamples > 0 {
			value := group.TokenUsageSum / float64(group.TokenUsageSamples)
			average = &value
		}
		items = append(items, dto.AdminPaidSubscriptionValuePlanGroup{
			PlanID:                             group.Plan.Id,
			PlanName:                           group.Plan.Title,
			PlanBusinessCode:                   subscriptionTierKey(&group.Plan),
			ActiveUserCount:                    len(group.MainUserIDs),
			ActiveSubscriptionCount:            group.MainCount,
			RecognizedRemainingValueByCurrency: group.Recognized.breakdownWithPreferredCurrency(query.Currency),
			TokenBasedValueByCurrency:          group.Token.breakdownWithPreferredCurrency(query.Currency),
			TimeBasedValueByCurrency:           group.TimeBased.breakdownWithPreferredCurrency(query.Currency),
			ExactRemainingValueByCurrency:      group.Exact.breakdownWithPreferredCurrency(query.Currency),
			EstimatedRemainingValueByCurrency:  group.Estimated.breakdownWithPreferredCurrency(query.Currency),
			UnknownCostCredit:                  group.UnknownCredit,
			ExcludedRemainingValueByCurrency:   group.ExcludedValue.breakdownWithPreferredCurrency(query.Currency),
			AverageTokenUsageRatio:             average,
		})
	}
	return items
}

type adminPaidSourceKey struct {
	Source      dto.AdminAnalyticsSource
	GrantReason string
}

type adminPaidSourceGroup struct {
	Key           adminPaidSourceKey
	MainUserIDs   map[int]struct{}
	MainCount     int
	ExcludedCount int
	Recognized    adminMoneyAccumulator
	Exact         adminMoneyAccumulator
	Estimated     adminMoneyAccumulator
	UnknownCredit int64
	ExcludedValue adminMoneyAccumulator
	Attribution   string
}

func adminAccumulatePaidSourceGroup(groups map[adminPaidSourceKey]*adminPaidSourceGroup, row adminPaidSubscriptionRow, main bool) error {
	if row.TimedValue != nil {
		seenSources := map[dto.AdminAnalyticsSource]struct{}{}
		for sourceCurrency, value := range row.TimedValue.BySourceCurrency {
			key := adminPaidSourceKey{Source: sourceCurrency.Source, GrantReason: string(sourceCurrency.Source)}
			group := groups[key]
			if group == nil {
				group = &adminPaidSourceGroup{Key: key, MainUserIDs: map[int]struct{}{}, Attribution: adminPaidSubscriptionSourceAttributionSnapshot}
				groups[key] = group
			}
			recognizedMicros := value.TimeMicros
			total := row.TimedValue.ByCurrency[sourceCurrency.Currency]
			if row.TimedValue.TokenAvailable && total.TokenMicros < total.TimeMicros {
				recognizedMicros = value.TokenMicros
			}
			if row.Excluded {
				group.ExcludedValue.addMicros(sourceCurrency.Currency, recognizedMicros)
			} else if main {
				group.Recognized.addMicros(sourceCurrency.Currency, recognizedMicros)
			}
			seenSources[sourceCurrency.Source] = struct{}{}
		}
		for source := range seenSources {
			group := groups[adminPaidSourceKey{Source: source, GrantReason: string(source)}]
			if row.Excluded {
				group.ExcludedCount++
			} else if main {
				group.MainCount++
				group.MainUserIDs[row.User.Id] = struct{}{}
			}
		}
		return nil
	}
	key := adminPaidSourceKey{Source: row.Source, GrantReason: row.Subscription.GrantReason}
	group := groups[key]
	if group == nil {
		group = &adminPaidSourceGroup{Key: key, MainUserIDs: map[int]struct{}{}, Attribution: row.SourceAttribution}
		groups[key] = group
	}
	currency := adminPaidSubscriptionRowCurrency(row)
	if row.Excluded {
		group.ExcludedValue.addMicros(currency, row.Value.RecognizedRemainingValueMicros)
		group.ExcludedCount++
	} else if main {
		group.Recognized.addMicros(currency, row.Value.RecognizedRemainingValueMicros)
		group.Exact.addMicros(currency, row.Value.ExactCostMicros)
		group.Estimated.addMicros(currency, row.Value.EstimatedCostMicros)
		var ok bool
		group.UnknownCredit, ok = checkedAddInt64(group.UnknownCredit, row.Value.UnknownCredit)
		if !ok {
			return ErrCreditValuationOverflow
		}
		group.MainCount++
		group.MainUserIDs[row.User.Id] = struct{}{}
	}
	return nil
}

func adminPaidSourceItems(groups map[adminPaidSourceKey]*adminPaidSourceGroup, query AdminAnalyticsQuery) []dto.AdminPaidSubscriptionValueSourceGroup {
	items := make([]dto.AdminPaidSubscriptionValueSourceGroup, 0, len(groups))
	for _, group := range groups {
		if query.ExcludedMode == dto.AdminAnalyticsExcludedModeIncludedOnly && group.MainCount == 0 {
			continue
		}
		if query.ExcludedMode == dto.AdminAnalyticsExcludedModeExcludedOnly && group.ExcludedCount == 0 {
			continue
		}
		items = append(items, dto.AdminPaidSubscriptionValueSourceGroup{
			Source:                             group.Key.Source,
			GrantReason:                        group.Key.GrantReason,
			UserCount:                          len(group.MainUserIDs),
			SubscriptionCount:                  group.MainCount,
			RecognizedRemainingValueByCurrency: group.Recognized.breakdownWithPreferredCurrency(query.Currency),
			ExactRemainingValueByCurrency:      group.Exact.breakdownWithPreferredCurrency(query.Currency),
			EstimatedRemainingValueByCurrency:  group.Estimated.breakdownWithPreferredCurrency(query.Currency),
			UnknownCostCredit:                  group.UnknownCredit,
			ExcludedRemainingValueByCurrency:   group.ExcludedValue.breakdownWithPreferredCurrency(query.Currency),
			SourceAttribution:                  group.Attribution,
		})
	}
	return items
}

func adminPaidSubscriptionItem(row adminPaidSubscriptionRow) dto.AdminPaidSubscriptionValueSubscription {
	planID := row.Plan.Id
	userID := row.User.Id
	subscriptionID := row.Subscription.Id
	currency := adminPaidSubscriptionRowCurrency(row)
	item := dto.AdminPaidSubscriptionValueSubscription{
		SubscriptionID:          row.Subscription.Id,
		UserID:                  row.User.Id,
		Username:                row.User.Username,
		PlanID:                  row.Plan.Id,
		PlanName:                row.Plan.Title,
		EntitlementType:         row.Subscription.EntitlementType,
		Source:                  row.Source,
		GrantReason:             row.Subscription.GrantReason,
		PlanPrice:               adminPaidPlanPriceMoneyAmount(row.Plan),
		StartTime:               row.Subscription.StartTime,
		EndTime:                 row.Subscription.EndTime,
		RemainingSeconds:        row.Value.RemainingSeconds,
		TokenLimit:              row.Subscription.TokenLimit,
		TokenUsed:               row.Subscription.TokenUsed,
		AvailableCredit:         row.Value.AvailableCredit,
		UnknownCostCredit:       row.Value.UnknownCredit,
		NextResetTime:           row.Subscription.NextResetTime,
		ExactRemainingValue:     adminPaidMicrosMoneyAmount(currency, row.Value.ExactCostMicros),
		EstimatedRemainingValue: adminPaidMicrosMoneyAmount(currency, row.Value.EstimatedCostMicros),
		ValuationBasis:          row.Value.ValuationBasis,
		ValuationConfidence:     row.Value.ValuationConfidence,
		ValuationStateVersion:   row.Value.StateVersion,
		ValuationUpdatedAt:      row.Value.UpdatedAt,
		SnapshotSemantics:       row.Value.SnapshotSemantics,
		SourceAttribution:       row.SourceAttribution,
		Excluded:                row.Excluded,
		ExcludedReason:          row.ExcludedReason,
		Drilldown:               &dto.AdminAnalyticsDrilldownTarget{Kind: "paid_subscription_value_subscription", UserID: &userID, PlanID: &planID, SubscriptionID: &subscriptionID, Tab: "paid-subscription-value"},
	}
	if row.TimedValue == nil {
		recognized := adminPaidMicrosMoneyAmount(currency, row.Value.RecognizedRemainingValueMicros)
		item.RecognizedRemainingValue = &recognized
		if row.Value.TokenBasedValueAvailable {
			value := adminPaidMicrosMoneyAmount(currency, row.Value.TokenBasedValueMicros)
			item.TokenBasedValue = &value
		}
		if row.Value.TimeBasedValueAvailable {
			value := adminPaidMicrosMoneyAmount(currency, row.Value.TimeBasedValueMicros)
			item.TimeBasedValue = &value
		}
	} else {
		recognized := adminMoneyAccumulator{}
		token := adminMoneyAccumulator{}
		timeBased := adminMoneyAccumulator{}
		adminPaidRowAccumulateValues(row, &recognized, &token, &timeBased)
		item.RecognizedRemainingValueByCurrency = recognized.breakdown()
		item.TimeBasedValueByCurrency = timeBased.breakdown()
		if row.TimedValue.TokenAvailable {
			item.TokenBasedValueByCurrency = token.breakdown()
		}
		if len(row.TimedValue.ByCurrency) == 1 {
			for valueCurrency, value := range row.TimedValue.ByCurrency {
				timeValue := adminPaidMicrosMoneyAmount(valueCurrency, value.TimeMicros)
				item.TimeBasedValue = &timeValue
				recognizedValue := adminPaidMicrosMoneyAmount(valueCurrency, value.RecognizedMicros)
				item.RecognizedRemainingValue = &recognizedValue
				if row.TimedValue.TokenAvailable {
					tokenValue := adminPaidMicrosMoneyAmount(valueCurrency, value.TokenMicros)
					item.TokenBasedValue = &tokenValue
				}
			}
		}
		item.ValuationConfidence = TimedSubscriptionValuationConfidenceExact
		if row.TimedValue.Unknown {
			item.ValuationConfidence = "unknown"
		}
		item.ValuationWarnings = append([]string(nil), row.TimedValue.Warnings...)
	}
	if row.Order != nil {
		orderID := row.Order.Id
		item.PossibleOrderID = &orderID
		item.PaymentProvider = row.Order.PaymentProvider
		item.PaymentMethod = row.Order.PaymentMethod
		amountMicros, _ := adminPaidFloatAmountMicros(row.Order.Money)
		value := adminPaidMicrosMoneyAmount(row.Plan.Currency, amountMicros)
		item.OrderRecordedAmount = &value
	}
	return item
}

func adminPaidMicrosMoneyAmount(currency string, amountMicros int64) dto.AdminAnalyticsMoneyAmount {
	return dto.AdminAnalyticsMoneyAmount{
		Amount:       float64(amountMicros) / float64(amountMicrosPerUnit),
		AmountMicros: strconv.FormatInt(amountMicros, 10),
		Currency:     strings.TrimSpace(currency),
	}
}

func adminPaidPlanPriceMoneyAmount(plan SubscriptionPlan) dto.AdminAnalyticsMoneyAmount {
	amountMicros, _ := adminPaidFloatAmountMicros(plan.PriceAmount)
	if plan.PriceAmountMicros != nil {
		amountMicros = *plan.PriceAmountMicros
	}
	return dto.AdminAnalyticsMoneyAmount{
		Amount:       plan.PriceAmount,
		AmountMicros: strconv.FormatInt(amountMicros, 10),
		Currency:     strings.TrimSpace(plan.Currency),
	}
}

func GetAdminPaidSubscriptionValueSummary(query AdminAnalyticsQuery) (dto.AdminAnalyticsPanelResponse[dto.AdminPaidSubscriptionValueResponse], error) {
	query = normalizeAdminPaidSubscriptionAnalyticsQuery(query)
	data, err := buildAdminPaidSubscriptionValueData(query)
	if err != nil {
		return dto.AdminAnalyticsPanelResponse[dto.AdminPaidSubscriptionValueResponse]{}, err
	}
	return dto.AdminAnalyticsPanelResponse[dto.AdminPaidSubscriptionValueResponse]{Range: adminAnalyticsRangeMeta(query), Data: dto.AdminPaidSubscriptionValueResponse{Summary: data.Summary}, Warnings: data.Warnings}, nil
}

func GetAdminPaidSubscriptionValueUsers(query AdminAnalyticsQuery) (dto.AdminAnalyticsPanelResponse[dto.AdminPaidSubscriptionValueResponse], error) {
	query = normalizeAdminPaidSubscriptionAnalyticsQuery(query)
	data, err := buildAdminPaidSubscriptionValueData(query)
	if err != nil {
		return dto.AdminAnalyticsPanelResponse[dto.AdminPaidSubscriptionValueResponse]{}, err
	}
	paged, page := paginateAdminAnalyticsList(data.Users, query.Limit, query.Offset)
	return dto.AdminAnalyticsPanelResponse[dto.AdminPaidSubscriptionValueResponse]{Range: adminAnalyticsRangeMeta(query), Data: dto.AdminPaidSubscriptionValueResponse{Summary: data.Summary, Users: dto.AdminAnalyticsList[dto.AdminPaidSubscriptionValueUser]{Items: paged, Page: page, SortBy: query.SortBy, SortOrder: query.SortOrder}}, Warnings: data.Warnings}, nil
}

func GetAdminPaidSubscriptionValueSubscriptions(query AdminAnalyticsQuery) (dto.AdminAnalyticsPanelResponse[dto.AdminPaidSubscriptionValueResponse], error) {
	query = normalizeAdminPaidSubscriptionAnalyticsQuery(query)
	rows, err := loadAdminPaidSubscriptionValueRows(query, false)
	if err != nil {
		return dto.AdminAnalyticsPanelResponse[dto.AdminPaidSubscriptionValueResponse]{}, err
	}
	data, err := adminBuildPaidSubscriptionValueDataFromRows(query, rows, true)
	if err != nil {
		return dto.AdminAnalyticsPanelResponse[dto.AdminPaidSubscriptionValueResponse]{}, err
	}
	paged, page := paginateAdminAnalyticsList(data.Subscriptions, query.Limit, query.Offset)
	return dto.AdminAnalyticsPanelResponse[dto.AdminPaidSubscriptionValueResponse]{Range: adminAnalyticsRangeMeta(query), Data: dto.AdminPaidSubscriptionValueResponse{Summary: data.Summary, Subscriptions: dto.AdminAnalyticsList[dto.AdminPaidSubscriptionValueSubscription]{Items: paged, Page: page, SortBy: query.SortBy, SortOrder: query.SortOrder}}, Warnings: data.Warnings}, nil
}

func GetAdminPaidSubscriptionValuePlanBreakdown(query AdminAnalyticsQuery) (dto.AdminAnalyticsPanelResponse[dto.AdminPaidSubscriptionValueResponse], error) {
	query = normalizeAdminPaidSubscriptionAnalyticsQuery(query)
	data, err := buildAdminPaidSubscriptionValueData(query)
	if err != nil {
		return dto.AdminAnalyticsPanelResponse[dto.AdminPaidSubscriptionValueResponse]{}, err
	}
	paged, page := paginateAdminAnalyticsList(data.Plans, query.Limit, query.Offset)
	return dto.AdminAnalyticsPanelResponse[dto.AdminPaidSubscriptionValueResponse]{Range: adminAnalyticsRangeMeta(query), Data: dto.AdminPaidSubscriptionValueResponse{Summary: data.Summary, Plans: dto.AdminAnalyticsList[dto.AdminPaidSubscriptionValuePlanGroup]{Items: paged, Page: page, SortBy: query.SortBy, SortOrder: query.SortOrder}}, Warnings: data.Warnings}, nil
}

func GetAdminPaidSubscriptionValueSourceBreakdown(query AdminAnalyticsQuery) (dto.AdminAnalyticsPanelResponse[dto.AdminPaidSubscriptionValueResponse], error) {
	query = normalizeAdminPaidSubscriptionAnalyticsQuery(query)
	data, err := buildAdminPaidSubscriptionValueData(query)
	if err != nil {
		return dto.AdminAnalyticsPanelResponse[dto.AdminPaidSubscriptionValueResponse]{}, err
	}
	paged, page := paginateAdminAnalyticsList(data.Sources, query.Limit, query.Offset)
	return dto.AdminAnalyticsPanelResponse[dto.AdminPaidSubscriptionValueResponse]{Range: adminAnalyticsRangeMeta(query), Data: dto.AdminPaidSubscriptionValueResponse{Summary: data.Summary, Sources: dto.AdminAnalyticsList[dto.AdminPaidSubscriptionValueSourceGroup]{Items: paged, Page: page, SortBy: query.SortBy, SortOrder: query.SortOrder}}, Warnings: data.Warnings}, nil
}

func adminParseAmountMicros(amountMicros string) (int64, error) {
	parsed, err := strconv.ParseUint(amountMicros, 10, 63)
	if errors.Is(err, strconv.ErrRange) {
		return 0, ErrCreditValuationOverflow
	}
	if err != nil {
		return 0, ErrCreditValuationSourceInvalid
	}
	return int64(parsed), nil
}

func adminAmountMicrosInBreakdown(amounts []dto.AdminAnalyticsMoneyBreakdown, currency string) (int64, error) {
	currency = strings.TrimSpace(currency)
	for _, amount := range amounts {
		if strings.TrimSpace(amount.Currency) == currency {
			return adminParseAmountMicros(amount.AmountMicros)
		}
	}
	return 0, nil
}

func adminMoneyAmountMicrosForCurrency(amount dto.AdminAnalyticsMoneyAmount, currency string) (int64, error) {
	if strings.TrimSpace(amount.Currency) != strings.TrimSpace(currency) {
		return 0, nil
	}
	return adminParseAmountMicros(amount.AmountMicros)
}

func adminAmountInBreakdown(amounts []dto.AdminAnalyticsMoneyBreakdown, currency string) float64 {
	currency = strings.TrimSpace(currency)
	for _, amount := range amounts {
		if amount.Currency == currency {
			return amount.Amount
		}
	}
	return 0
}

func adminMoneyAmountForCurrency(amount dto.AdminAnalyticsMoneyAmount, currency string) float64 {
	if strings.TrimSpace(amount.Currency) != strings.TrimSpace(currency) {
		return 0
	}
	return amount.Amount
}

func adminOptionalMoneyAmountForCurrency(amount *dto.AdminAnalyticsMoneyAmount, currency string) float64 {
	if amount == nil {
		return 0
	}
	return adminMoneyAmountForCurrency(*amount, currency)
}

func adminSortPaidSubscriptionUsers(items []dto.AdminPaidSubscriptionValueUser, query AdminAnalyticsQuery) error {
	recognizedMicros := make(map[int]int64, len(items))
	if query.SortBy == "recognized_remaining_value" {
		for i := range items {
			amount, err := adminAmountMicrosInBreakdown(items[i].RecognizedRemainingValueByCurrency, query.Currency)
			if err != nil {
				return err
			}
			recognizedMicros[items[i].UserID] = amount
		}
	}
	desc := adminSortDesc(query.SortOrder)
	sort.SliceStable(items, func(i, j int) bool {
		left := items[i]
		right := items[j]
		cmp := 0
		switch query.SortBy {
		case "recognized_remaining_value":
			cmp = adminCompareInt64(recognizedMicros[left.UserID], recognizedMicros[right.UserID])
		case "active_paid_plan_count":
			cmp = left.ActivePaidPlanCount - right.ActivePaidPlanCount
		case "earliest_end_time":
			cmp = adminCompareInt64(left.EarliestEndTime, right.EarliestEndTime)
		default:
			cmp = left.UserID - right.UserID
		}
		if cmp == 0 {
			cmp = left.UserID - right.UserID
			return cmp < 0
		}
		if desc {
			return cmp > 0
		}
		return cmp < 0
	})
	return nil
}

func adminPaidSubscriptionItemRecognizedMicros(item dto.AdminPaidSubscriptionValueSubscription, currency string) (int64, error) {
	if item.RecognizedRemainingValue != nil {
		return adminMoneyAmountMicrosForCurrency(*item.RecognizedRemainingValue, currency)
	}
	return adminAmountMicrosInBreakdown(item.RecognizedRemainingValueByCurrency, currency)
}

func adminSortPaidSubscriptionItems(items []dto.AdminPaidSubscriptionValueSubscription, query AdminAnalyticsQuery) error {
	recognizedMicros := make(map[int]int64, len(items))
	if query.SortBy == "recognized_remaining_value" {
		for i := range items {
			amount, err := adminPaidSubscriptionItemRecognizedMicros(items[i], query.Currency)
			if err != nil {
				return err
			}
			recognizedMicros[items[i].SubscriptionID] = amount
		}
	}
	desc := adminSortDesc(query.SortOrder)
	sort.SliceStable(items, func(i, j int) bool {
		left := items[i]
		right := items[j]
		cmp := adminComparePaidSubscriptionItem(left, right, query, recognizedMicros)
		if cmp == 0 {
			cmp = left.SubscriptionID - right.SubscriptionID
		}
		if desc {
			return cmp > 0
		}
		return cmp < 0
	})
	return nil
}

func adminComparePaidSubscriptionItem(left dto.AdminPaidSubscriptionValueSubscription, right dto.AdminPaidSubscriptionValueSubscription, query AdminAnalyticsQuery, recognizedMicros map[int]int64) int {
	switch query.SortBy {
	case "recognized_remaining_value":
		return adminCompareInt64(recognizedMicros[left.SubscriptionID], recognizedMicros[right.SubscriptionID])
	case "end_time":
		return adminCompareInt64(left.EndTime, right.EndTime)
	case "start_time":
		return adminCompareInt64(left.StartTime, right.StartTime)
	case "plan_price":
		return adminCompareFloat(adminMoneyAmountForCurrency(left.PlanPrice, query.Currency), adminMoneyAmountForCurrency(right.PlanPrice, query.Currency))
	default:
		return left.SubscriptionID - right.SubscriptionID
	}
}

func adminSortPaidSubscriptionPlans(items []dto.AdminPaidSubscriptionValuePlanGroup, query AdminAnalyticsQuery) error {
	recognizedMicros := make(map[int]int64, len(items))
	if query.SortBy == "recognized_remaining_value" {
		for i := range items {
			amount, err := adminAmountMicrosInBreakdown(items[i].RecognizedRemainingValueByCurrency, query.Currency)
			if err != nil {
				return err
			}
			recognizedMicros[items[i].PlanID] = amount
		}
	}
	desc := adminSortDesc(query.SortOrder)
	sort.SliceStable(items, func(i, j int) bool {
		left := items[i]
		right := items[j]
		cmp := 0
		switch query.SortBy {
		case "recognized_remaining_value":
			cmp = adminCompareInt64(recognizedMicros[left.PlanID], recognizedMicros[right.PlanID])
		case "subscription_count":
			cmp = left.ActiveSubscriptionCount - right.ActiveSubscriptionCount
		case "user_count":
			cmp = left.ActiveUserCount - right.ActiveUserCount
		default:
			cmp = left.PlanID - right.PlanID
		}
		if cmp == 0 {
			cmp = left.PlanID - right.PlanID
		}
		if desc {
			return cmp > 0
		}
		return cmp < 0
	})
	return nil
}

func adminSortPaidSubscriptionSources(items []dto.AdminPaidSubscriptionValueSourceGroup, query AdminAnalyticsQuery) error {
	recognizedMicros := make(map[adminPaidSourceKey]int64, len(items))
	if query.SortBy == "recognized_remaining_value" {
		for i := range items {
			amount, err := adminAmountMicrosInBreakdown(items[i].RecognizedRemainingValueByCurrency, query.Currency)
			if err != nil {
				return err
			}
			key := adminPaidSourceKey{Source: items[i].Source, GrantReason: items[i].GrantReason}
			recognizedMicros[key] = amount
		}
	}
	desc := adminSortDesc(query.SortOrder)
	sort.SliceStable(items, func(i, j int) bool {
		left := items[i]
		right := items[j]
		cmp := 0
		switch query.SortBy {
		case "recognized_remaining_value":
			leftKey := adminPaidSourceKey{Source: left.Source, GrantReason: left.GrantReason}
			rightKey := adminPaidSourceKey{Source: right.Source, GrantReason: right.GrantReason}
			cmp = adminCompareInt64(recognizedMicros[leftKey], recognizedMicros[rightKey])
		case "subscription_count":
			cmp = left.SubscriptionCount - right.SubscriptionCount
		case "user_count":
			cmp = left.UserCount - right.UserCount
		case "grant_reason":
			cmp = strings.Compare(left.GrantReason, right.GrantReason)
		default:
			cmp = strings.Compare(string(left.Source), string(right.Source))
		}
		if cmp == 0 {
			cmp = strings.Compare(left.GrantReason, right.GrantReason)
		}
		if desc {
			return cmp > 0
		}
		return cmp < 0
	})
	return nil
}

func adminCompareFloat(left float64, right float64) int {
	if left < right {
		return -1
	}
	if left > right {
		return 1
	}
	return 0
}

func adminCompareInt64(left int64, right int64) int {
	if left < right {
		return -1
	}
	if left > right {
		return 1
	}
	return 0
}

type adminInvitationPaidRow struct {
	Subscription          UserSubscription
	Plan                  SubscriptionPlan
	Invitee               User
	Inviter               User
	Source                dto.AdminAnalyticsSource
	SourceAttribution     string
	RecognizedUnits       float64
	RecognizedAmount      float64
	RecognizedCurrency    string
	ExcludedAuditAmount   float64
	ExcludedAuditCurrency string
	UnitBasis             string
	Active                bool
	ActivePaidAmount      float64
	ActivePaidCurrency    string
	ActiveValue           *adminSubscriptionValue
	Excluded              bool
	ExcludedReason        string
	ExcludedAt            int64
	ExcludedBy            int
	Order                 *SubscriptionOrder
}

type adminInvitationRelationshipRow struct {
	Invitee        User
	Inviter        User
	Excluded       bool
	ExcludedReason string
	ExcludedAt     int64
	ExcludedBy     int
}

func loadAdminInvitationRelationshipRows(query AdminAnalyticsQuery) ([]adminInvitationRelationshipRow, error) {
	var invitees []User
	if err := DB.Where("inviter_id > ?", 0).Find(&invitees).Error; err != nil {
		return nil, err
	}
	excludedUsers := setting.GetSubscriptionAnalyticsExcludedUsers()
	filtered := make([]User, 0, len(invitees))
	for i := range invitees {
		invitee := invitees[i]
		if invitee.CreatedAt > query.SnapshotAt {
			continue
		}
		if !adminPaidUserMatchesQuery(invitee, query) {
			continue
		}
		filtered = append(filtered, invitee)
	}
	if len(filtered) == 0 {
		return []adminInvitationRelationshipRow{}, nil
	}
	inviterIDs := make([]int, 0, len(filtered))
	for i := range filtered {
		if filtered[i].InviterId > 0 {
			inviterIDs = append(inviterIDs, filtered[i].InviterId)
		}
	}
	inviters, err := adminUsersByID(inviterIDs)
	if err != nil {
		return nil, err
	}
	rows := make([]adminInvitationRelationshipRow, 0, len(filtered))
	for i := range filtered {
		invitee := filtered[i]
		if query.InviterID > 0 && invitee.InviterId != query.InviterID {
			continue
		}
		if query.InviteeID > 0 && invitee.Id != query.InviteeID {
			continue
		}
		inviter, ok := inviters[invitee.InviterId]
		if !ok {
			continue
		}
		excluded := excludedUsers[invitee.Id]
		rows = append(rows, adminInvitationRelationshipRow{
			Invitee:        invitee,
			Inviter:        inviter,
			Excluded:       excluded.UserID > 0,
			ExcludedReason: excluded.Reason,
			ExcludedAt:     excluded.ExcludedAt,
			ExcludedBy:     excluded.ExcludedBy,
		})
	}
	return rows, nil
}

type adminInvitationPaidUnitSegment struct {
	Units      float64
	AcquiredAt int64
}

type adminInvitationPaidEventSnapshot struct {
	RecognizedUnits  float64
	RecognizedAmount float64
	ActiveAmount     float64
	Currency         string
}

func adminInferInvitationPaidUnits(sub UserSubscription, plan SubscriptionPlan, query AdminAnalyticsQuery) (float64, string) {
	segments, basis := adminInferInvitationPaidUnitSegments(sub, plan)
	units := 0.0
	for _, segment := range segments {
		if segment.AcquiredAt > query.SnapshotAt {
			continue
		}
		if query.TimeRangeExplicit && (segment.AcquiredAt < query.StartTimestamp || segment.AcquiredAt > query.EndTimestamp) {
			continue
		}
		units += segment.Units
	}
	return units, basis
}

func adminInvitationPaidEventSnapshots(subscriptionIDs []int, query AdminAnalyticsQuery) (map[int]adminInvitationPaidEventSnapshot, error) {
	if len(subscriptionIDs) == 0 {
		return map[int]adminInvitationPaidEventSnapshot{}, nil
	}
	var events []InvitationRewardEvent
	if err := DB.Where("source_subscription_id IN ? AND status = ?", subscriptionIDs, InvitationRewardEventStatusActive).Find(&events).Error; err != nil {
		return nil, err
	}
	snapshots := make(map[int]adminInvitationPaidEventSnapshot, len(events))
	for i := range events {
		event := events[i]
		currency := strings.TrimSpace(event.SourceCurrency)
		if event.SourceSubscriptionId <= 0 || event.SourceAmountCents <= 0 || currency == "" {
			continue
		}
		snapshot := snapshots[event.SourceSubscriptionId]
		if snapshot.Currency != "" && snapshot.Currency != currency {
			return nil, fmt.Errorf("invitation reward event currency mismatch for subscription %d", event.SourceSubscriptionId)
		}
		snapshot.Currency = currency
		amount := float64(event.SourceAmountCents) / 100
		if event.EventStartTime <= query.SnapshotAt && (!query.TimeRangeExplicit || (event.EventStartTime >= query.StartTimestamp && event.EventStartTime <= query.EndTimestamp)) {
			snapshot.RecognizedUnits++
			snapshot.RecognizedAmount += amount
		}
		if event.EventStartTime <= query.SnapshotAt && event.EventEndTime > query.SnapshotAt {
			snapshot.ActiveAmount += amount
		}
		snapshots[event.SourceSubscriptionId] = snapshot
	}
	return snapshots, nil
}

func adminInferExcludedInvitationPaidAuditUnits(sub UserSubscription, plan SubscriptionPlan, query AdminAnalyticsQuery) float64 {
	if sub.StartTime > query.SnapshotAt {
		return 0
	}
	segments, _ := adminInferInvitationPaidUnitSegments(sub, plan)
	units := 0.0
	for _, segment := range segments {
		if segment.AcquiredAt > query.SnapshotAt {
			continue
		}
		if query.TimeRangeExplicit && (segment.AcquiredAt < query.StartTimestamp || segment.AcquiredAt > query.EndTimestamp) {
			continue
		}
		units += segment.Units
	}
	return units
}

func adminInferInvitationPaidUnitSegments(sub UserSubscription, plan SubscriptionPlan) ([]adminInvitationPaidUnitSegment, string) {
	if sub.StartTime <= 0 || sub.EndTime <= sub.StartTime {
		return []adminInvitationPaidUnitSegment{{Units: 1, AcquiredAt: sub.StartTime}}, adminInvitationPaidUnitSnapshotMinimum
	}
	segments := []adminInvitationPaidUnitSegment{}
	basis := adminInvitationPaidUnitPeriodAligned
	cursor := sub.StartTime
	for guard := 0; cursor < sub.EndTime && guard < 10000; guard++ {
		next, err := calcPlanEndTime(time.Unix(cursor, 0).UTC(), &plan)
		if err != nil || next <= cursor {
			return []adminInvitationPaidUnitSegment{{Units: 1, AcquiredAt: sub.StartTime}}, adminInvitationPaidUnitSnapshotMinimum
		}
		if next <= sub.EndTime {
			segments = append(segments, adminInvitationPaidUnitSegment{Units: 1, AcquiredAt: cursor})
			cursor = next
			continue
		}
		cycleSeconds := next - cursor
		remainingSeconds := sub.EndTime - cursor
		if cycleSeconds <= 0 || remainingSeconds <= 0 {
			break
		}
		segments = append(segments, adminInvitationPaidUnitSegment{Units: float64(remainingSeconds) / float64(cycleSeconds), AcquiredAt: cursor})
		basis = adminInvitationPaidUnitPeriodFraction
		cursor = sub.EndTime
	}
	if len(segments) == 0 {
		return []adminInvitationPaidUnitSegment{{Units: 1, AcquiredAt: sub.StartTime}}, adminInvitationPaidUnitSnapshotMinimum
	}
	if cursor < sub.EndTime {
		return []adminInvitationPaidUnitSegment{{Units: 1, AcquiredAt: sub.StartTime}}, adminInvitationPaidUnitSnapshotMinimum
	}
	return segments, basis
}

func loadAdminInvitationPaidRows(query AdminAnalyticsQuery, filterSubscriptionID bool) ([]adminInvitationPaidRow, error) {
	query = normalizeAdminPaidSubscriptionAnalyticsQuery(query)
	var subs []UserSubscription
	db := DB.Model(&UserSubscription{})
	if query.TimeRangeExplicit && query.EndTimestamp > 0 {
		db = db.Where("start_time <= ?", query.EndTimestamp)
	}
	if len(query.PlanIDs) > 0 {
		db = db.Where("plan_id IN ?", query.PlanIDs)
	}
	if len(query.UserIDs) > 0 {
		db = db.Where("user_id IN ?", query.UserIDs)
	}
	if len(query.GrantReasons) > 0 {
		db = db.Where("grant_reason IN ?", query.GrantReasons)
	}
	if filterSubscriptionID && query.SubscriptionID > 0 {
		db = db.Where("id = ?", query.SubscriptionID)
	}
	if err := db.Find(&subs).Error; err != nil {
		return nil, err
	}
	userIDs := make([]int, 0, len(subs))
	planIDs := make([]int, 0, len(subs))
	for i := range subs {
		userIDs = append(userIDs, subs[i].UserId)
		planIDs = append(planIDs, subs[i].PlanId)
	}
	invitees, err := adminUsersByID(userIDs)
	if err != nil {
		return nil, err
	}
	inviterIDs := make([]int, 0, len(invitees))
	for _, invitee := range invitees {
		if invitee.InviterId > 0 {
			inviterIDs = append(inviterIDs, invitee.InviterId)
		}
	}
	inviters, err := adminUsersByID(inviterIDs)
	if err != nil {
		return nil, err
	}
	plans, err := adminPlansByID(planIDs)
	if err != nil {
		return nil, err
	}
	excludedUsers := setting.GetSubscriptionAnalyticsExcludedUsers()
	orders, err := adminBestSubscriptionOrders(userIDs, planIDs)
	if err != nil {
		return nil, err
	}
	rows := make([]adminInvitationPaidRow, 0, len(subs))
	for i := range subs {
		sub := subs[i]
		if adminIsNonSalesGiftSubscription(sub) {
			continue
		}
		invitee, ok := invitees[sub.UserId]
		if !ok || invitee.InviterId <= 0 {
			continue
		}
		if !adminPaidUserMatchesQuery(invitee, query) {
			continue
		}
		if query.InviterID > 0 && invitee.InviterId != query.InviterID {
			continue
		}
		if query.InviteeID > 0 && invitee.Id != query.InviteeID {
			continue
		}
		inviter, ok := inviters[invitee.InviterId]
		if !ok {
			continue
		}
		plan, ok := plans[sub.PlanId]
		if !ok || plan.PriceAmount <= 0 {
			continue
		}
		if !adminPaidPlanMatchesQuery(plan, query) {
			continue
		}
		source := normalizeAdminSubscriptionSource(sub.GrantReason, sub.Source)
		if len(query.Sources) > 0 && !adminSourceInSet(source, query.Sources) {
			continue
		}
		excluded := excludedUsers[invitee.Id]
		rows = append(rows, adminInvitationPaidRow{
			Subscription:      sub,
			Plan:              plan,
			Invitee:           invitee,
			Inviter:           inviter,
			Source:            source,
			SourceAttribution: adminPaidSourceAttribution(sub),
			Excluded:          excluded.UserID > 0,
			ExcludedReason:    excluded.Reason,
			ExcludedAt:        excluded.ExcludedAt,
			ExcludedBy:        excluded.ExcludedBy,
			Order:             orders[adminOrderLookupKey{UserID: sub.UserId, PlanID: sub.PlanId}],
		})
	}
	candidateSubscriptionIDs := make([]int, 0, len(rows))
	for i := range rows {
		candidateSubscriptionIDs = append(candidateSubscriptionIDs, rows[i].Subscription.Id)
	}
	eventSnapshots, err := adminInvitationPaidEventSnapshots(candidateSubscriptionIDs, query)
	if err != nil {
		return nil, err
	}
	for i := range rows {
		row := &rows[i]
		units, unitBasis := adminInferInvitationPaidUnits(row.Subscription, row.Plan, query)
		recognizedAmount := row.Plan.PriceAmount * units
		recognizedCurrency := row.Plan.Currency
		eventSnapshot, hasEventSnapshot := eventSnapshots[row.Subscription.Id]
		if hasEventSnapshot {
			units = eventSnapshot.RecognizedUnits
			unitBasis = adminInvitationPaidUnitEventSnapshot
			recognizedAmount = eventSnapshot.RecognizedAmount
			recognizedCurrency = eventSnapshot.Currency
		}
		row.RecognizedUnits = units
		row.RecognizedAmount = recognizedAmount
		row.RecognizedCurrency = recognizedCurrency
		row.ExcludedAuditCurrency = row.Plan.Currency
		if row.Excluded {
			row.ExcludedAuditAmount = recognizedAmount
			row.ExcludedAuditCurrency = recognizedCurrency
		}
		row.UnitBasis = unitBasis
		row.Active = row.Subscription.Status == "active" && row.Subscription.StartTime <= query.SnapshotAt && row.Subscription.EndTime > query.SnapshotAt
		row.ActivePaidAmount = row.Plan.PriceAmount
		row.ActivePaidCurrency = row.Plan.Currency
		if hasEventSnapshot {
			row.ActivePaidAmount = eventSnapshot.ActiveAmount
			row.ActivePaidCurrency = eventSnapshot.Currency
		}
		if row.Active {
			value, err := adminRecognizedRemainingValue(row.Subscription, row.Plan, query.SnapshotAt)
			if err != nil {
				return nil, err
			}
			row.ActiveValue = &value
		}
	}
	return rows, nil
}

type adminInvitationPaidBuild struct {
	Summary       dto.AdminInvitationPaidSubscriptionsSummary
	Inviters      []dto.AdminInvitationPaidInviter
	Invitees      []dto.AdminInvitationPaidInvitee
	Subscriptions []dto.AdminInvitationPaidSubscriptionRecord
}

func buildAdminInvitationPaidSubscriptionsData(query AdminAnalyticsQuery) (adminInvitationPaidBuild, error) {
	query = normalizeAdminPaidSubscriptionAnalyticsQuery(query)
	rows, err := loadAdminInvitationPaidRows(query, false)
	if err != nil {
		return adminInvitationPaidBuild{}, err
	}
	relationships, err := loadAdminInvitationRelationshipRows(query)
	if err != nil {
		return adminInvitationPaidBuild{}, err
	}
	return adminBuildInvitationPaidDataFromRows(query, rows, relationships, true)
}

func adminInvitationPaidScopedInvitees(query AdminAnalyticsQuery, rows []adminInvitationPaidRow) map[int]struct{} {
	if !adminInvitationRelationshipRequiresPaidScope(query) {
		return nil
	}
	invitees := make(map[int]struct{}, len(rows))
	for i := range rows {
		row := rows[i]
		if row.RecognizedUnits <= 0 && !row.Active {
			continue
		}
		if !adminShouldShowExcludedRow(row.Excluded, query.ExcludedMode) {
			continue
		}
		invitees[row.Invitee.Id] = struct{}{}
	}
	return invitees
}

func adminBuildInvitationPaidDataFromRows(query AdminAnalyticsQuery, rows []adminInvitationPaidRow, relationships []adminInvitationRelationshipRow, applySubscriptionIDToList bool) (adminInvitationPaidBuild, error) {
	recognized := adminMoneyAccumulator{}
	activeAmount := adminMoneyAccumulator{}
	activeRemaining := adminMoneyAccumulator{}
	excludedPaid := adminMoneyAccumulator{}
	excludedActiveRemaining := adminMoneyAccumulator{}
	mainInviters := map[int]struct{}{}
	mainInvitees := map[int]struct{}{}
	paidInvitees := map[int]struct{}{}
	activePaidInvitees := map[int]struct{}{}

	inviterGroups := map[int]*adminInvitationInviterGroup{}
	inviteeGroups := map[int]*adminInvitationInviteeGroup{}
	subscriptionItems := make([]dto.AdminInvitationPaidSubscriptionRecord, 0, len(rows))

	paidScopedInvitees := adminInvitationPaidScopedInvitees(query, rows)
	for i := range relationships {
		relationship := relationships[i]
		if !adminShouldShowExcludedRow(relationship.Excluded, query.ExcludedMode) {
			continue
		}
		if paidScopedInvitees != nil {
			if _, ok := paidScopedInvitees[relationship.Invitee.Id]; !ok {
				continue
			}
		}
		adminAccumulateInvitationRelationship(inviterGroups, relationship)
		main := adminIncludeInMain(relationship.Excluded, query.ExcludedMode)
		if main {
			mainInviters[relationship.Inviter.Id] = struct{}{}
			mainInvitees[relationship.Invitee.Id] = struct{}{}
		}
	}

	for i := range rows {
		row := rows[i]
		if row.RecognizedUnits <= 0 && !row.Active {
			continue
		}
		currency := row.RecognizedCurrency
		if row.Excluded {
			excludedPaid.add(row.ExcludedAuditCurrency, row.ExcludedAuditAmount)
			if row.ActiveValue != nil {
				excludedActiveRemaining.add(row.Plan.Currency, row.ActiveValue.RecognizedRemainingValue)
			}
		}
		main := adminIncludeInMain(row.Excluded, query.ExcludedMode)
		if main {
			recognized.add(currency, row.RecognizedAmount)
			if row.RecognizedUnits > 0 {
				paidInvitees[row.Invitee.Id] = struct{}{}
			}
			if row.Active {
				activeAmount.add(row.ActivePaidCurrency, row.ActivePaidAmount)
				activePaidInvitees[row.Invitee.Id] = struct{}{}
				if row.ActiveValue != nil {
					activeRemaining.add(row.Plan.Currency, row.ActiveValue.RecognizedRemainingValue)
				}
			}
		}
		adminAccumulateInvitationInviter(inviterGroups, row, main)
		adminAccumulateInvitationInvitee(inviteeGroups, row, main)
		if adminShouldShowExcludedRow(row.Excluded, query.ExcludedMode) && (!query.ActiveOnly || row.Active) && (!applySubscriptionIDToList || query.SubscriptionID <= 0 || row.Subscription.Id == query.SubscriptionID) {
			subscriptionItems = append(subscriptionItems, adminInvitationSubscriptionItem(row))
		}
	}

	inviters := adminInvitationInviterItems(inviterGroups, query)
	invitees := adminInvitationInviteeItems(inviteeGroups, query)
	adminSortInvitationInviters(inviters, query)
	adminSortInvitationInvitees(invitees, query)
	adminSortInvitationSubscriptions(subscriptionItems, query)

	return adminInvitationPaidBuild{
		Summary: dto.AdminInvitationPaidSubscriptionsSummary{
			RecognizedInvitationPaidAmountByCurrency: recognized.breakdownWithPreferredCurrency(query.Currency),
			ActiveInvitationPaidAmountByCurrency:     activeAmount.breakdownWithPreferredCurrency(query.Currency),
			ActiveInvitationRemainingValueByCurrency: activeRemaining.breakdownWithPreferredCurrency(query.Currency),
			ExcludedInvitationPaidAmountByCurrency:   excludedPaid.breakdownWithPreferredCurrency(query.Currency),
			ExcludedActiveRemainingValueByCurrency:   excludedActiveRemaining.breakdownWithPreferredCurrency(query.Currency),
			InviterCount:                             len(mainInviters),
			InviteeCount:                             len(mainInvitees),
			PaidInviteeCount:                         len(paidInvitees),
			ActivePaidInviteeCount:                   len(activePaidInvitees),
		},
		Inviters:      inviters,
		Invitees:      invitees,
		Subscriptions: subscriptionItems,
	}, nil
}

type adminInvitationInviterGroup struct {
	Inviter                      User
	InviteeIDs                   map[int]struct{}
	PaidInviteeIDs               map[int]struct{}
	ActivePaidInviteeIDs         map[int]struct{}
	ExcludedActivePaidInviteeIDs map[int]struct{}
	Recognized                   adminMoneyAccumulator
	ActiveAmount                 adminMoneyAccumulator
	ActiveRemaining              adminMoneyAccumulator
	ExcludedPaid                 adminMoneyAccumulator
	ExcludedActiveRemaining      adminMoneyAccumulator
	LatestPaidTime               int64
	HasMain                      bool
	HasExcluded                  bool
}

func adminAccumulateInvitationRelationship(groups map[int]*adminInvitationInviterGroup, row adminInvitationRelationshipRow) {
	group := groups[row.Inviter.Id]
	if group == nil {
		group = &adminInvitationInviterGroup{Inviter: row.Inviter, InviteeIDs: map[int]struct{}{}, PaidInviteeIDs: map[int]struct{}{}, ActivePaidInviteeIDs: map[int]struct{}{}, ExcludedActivePaidInviteeIDs: map[int]struct{}{}, Recognized: adminMoneyAccumulator{}, ActiveAmount: adminMoneyAccumulator{}, ActiveRemaining: adminMoneyAccumulator{}, ExcludedPaid: adminMoneyAccumulator{}, ExcludedActiveRemaining: adminMoneyAccumulator{}}
		groups[row.Inviter.Id] = group
	}
	if row.Excluded {
		group.HasExcluded = true
		return
	}
	group.HasMain = true
	group.InviteeIDs[row.Invitee.Id] = struct{}{}
}

func adminAccumulateInvitationInviter(groups map[int]*adminInvitationInviterGroup, row adminInvitationPaidRow, main bool) {
	group := groups[row.Inviter.Id]
	if group == nil {
		group = &adminInvitationInviterGroup{Inviter: row.Inviter, InviteeIDs: map[int]struct{}{}, PaidInviteeIDs: map[int]struct{}{}, ActivePaidInviteeIDs: map[int]struct{}{}, ExcludedActivePaidInviteeIDs: map[int]struct{}{}, Recognized: adminMoneyAccumulator{}, ActiveAmount: adminMoneyAccumulator{}, ActiveRemaining: adminMoneyAccumulator{}, ExcludedPaid: adminMoneyAccumulator{}, ExcludedActiveRemaining: adminMoneyAccumulator{}}
		groups[row.Inviter.Id] = group
	}
	if row.Excluded {
		group.HasExcluded = true
		group.ExcludedPaid.add(row.ExcludedAuditCurrency, row.ExcludedAuditAmount)
		if row.ActiveValue != nil {
			group.ExcludedActiveRemaining.add(row.Plan.Currency, row.ActiveValue.RecognizedRemainingValue)
		}
		if row.Active {
			group.ExcludedActivePaidInviteeIDs[row.Invitee.Id] = struct{}{}
		}
	} else if main {
		group.HasMain = true
		group.InviteeIDs[row.Invitee.Id] = struct{}{}
		if row.RecognizedUnits > 0 {
			group.PaidInviteeIDs[row.Invitee.Id] = struct{}{}
			if row.Subscription.StartTime > group.LatestPaidTime {
				group.LatestPaidTime = row.Subscription.StartTime
			}
		}
		group.Recognized.add(row.RecognizedCurrency, row.RecognizedAmount)
		if row.Active {
			group.ActiveAmount.add(row.ActivePaidCurrency, row.ActivePaidAmount)
			group.ActivePaidInviteeIDs[row.Invitee.Id] = struct{}{}
			if row.ActiveValue != nil {
				group.ActiveRemaining.add(row.Plan.Currency, row.ActiveValue.RecognizedRemainingValue)
			}
		}
	}
}

func adminInvitationInviterItems(groups map[int]*adminInvitationInviterGroup, query AdminAnalyticsQuery) []dto.AdminInvitationPaidInviter {
	items := make([]dto.AdminInvitationPaidInviter, 0, len(groups))
	for _, group := range groups {
		if query.ExcludedMode == dto.AdminAnalyticsExcludedModeIncludedOnly && !group.HasMain {
			continue
		}
		if query.ExcludedMode == dto.AdminAnalyticsExcludedModeExcludedOnly && !group.HasExcluded {
			continue
		}
		activeCount := len(group.ActivePaidInviteeIDs)
		if query.ExcludedMode != dto.AdminAnalyticsExcludedModeIncludedOnly && len(group.ExcludedActivePaidInviteeIDs) > 0 {
			activeCount += len(group.ExcludedActivePaidInviteeIDs)
		}
		if query.ActiveOnly && activeCount == 0 {
			continue
		}
		inviterID := group.Inviter.Id
		items = append(items, dto.AdminInvitationPaidInviter{
			InviterUserID:                            group.Inviter.Id,
			InviterUsername:                          group.Inviter.Username,
			InviteeCount:                             len(group.InviteeIDs),
			PaidInviteeCount:                         len(group.PaidInviteeIDs),
			ActivePaidInviteeCount:                   activeCount,
			RecognizedInvitationPaidAmountByCurrency: group.Recognized.breakdownWithPreferredCurrency(query.Currency),
			ActiveInvitationPaidAmountByCurrency:     group.ActiveAmount.breakdownWithPreferredCurrency(query.Currency),
			ActiveInvitationRemainingValueByCurrency: group.ActiveRemaining.breakdownWithPreferredCurrency(query.Currency),
			ExcludedInvitationPaidAmountByCurrency:   group.ExcludedPaid.breakdownWithPreferredCurrency(query.Currency),
			ExcludedActiveRemainingValueByCurrency:   group.ExcludedActiveRemaining.breakdownWithPreferredCurrency(query.Currency),
			LatestPaidSubscriptionTime:               group.LatestPaidTime,
			Drilldown:                                &dto.AdminAnalyticsDrilldownTarget{Kind: "invitation_paid_inviter", InviterID: &inviterID, Tab: "invitation-paid-subscriptions"},
		})
	}
	return items
}

type adminInvitationInviteeGroup struct {
	Invitee             User
	Excluded            bool
	ExcludedReason      string
	ExcludedAt          int64
	ExcludedBy          int
	SnapshotCount       int
	RecognizedUnits     float64
	ActiveCount         int
	ExcludedActiveCount int
	Recognized          adminMoneyAccumulator
	ActiveRemaining     adminMoneyAccumulator
	ActiveAmount        adminMoneyAccumulator
	WouldHavePaid       adminMoneyAccumulator
	WouldHaveActive     adminMoneyAccumulator
	HasMain             bool
	HasExcluded         bool
}

func adminAccumulateInvitationInvitee(groups map[int]*adminInvitationInviteeGroup, row adminInvitationPaidRow, main bool) {
	group := groups[row.Invitee.Id]
	if group == nil {
		group = &adminInvitationInviteeGroup{Invitee: row.Invitee, Excluded: row.Excluded, ExcludedReason: row.ExcludedReason, ExcludedAt: row.ExcludedAt, ExcludedBy: row.ExcludedBy, Recognized: adminMoneyAccumulator{}, ActiveRemaining: adminMoneyAccumulator{}, ActiveAmount: adminMoneyAccumulator{}, WouldHavePaid: adminMoneyAccumulator{}, WouldHaveActive: adminMoneyAccumulator{}}
		groups[row.Invitee.Id] = group
	}
	if row.Excluded {
		group.HasExcluded = true
		group.WouldHavePaid.add(row.ExcludedAuditCurrency, row.ExcludedAuditAmount)
		if row.ActiveValue != nil {
			group.WouldHaveActive.add(row.Plan.Currency, row.ActiveValue.RecognizedRemainingValue)
		}
		if row.Active {
			group.ExcludedActiveCount++
		}
	} else if main {
		group.HasMain = true
		group.SnapshotCount++
		group.RecognizedUnits += row.RecognizedUnits
		group.Recognized.add(row.RecognizedCurrency, row.RecognizedAmount)
		if row.Active {
			group.ActiveCount++
			group.ActiveAmount.add(row.ActivePaidCurrency, row.ActivePaidAmount)
			if row.ActiveValue != nil {
				group.ActiveRemaining.add(row.Plan.Currency, row.ActiveValue.RecognizedRemainingValue)
			}
		}
	}
}

func adminInvitationInviteeItems(groups map[int]*adminInvitationInviteeGroup, query AdminAnalyticsQuery) []dto.AdminInvitationPaidInvitee {
	items := make([]dto.AdminInvitationPaidInvitee, 0, len(groups))
	for _, group := range groups {
		if query.ExcludedMode == dto.AdminAnalyticsExcludedModeIncludedOnly && !group.HasMain {
			continue
		}
		if query.ExcludedMode == dto.AdminAnalyticsExcludedModeExcludedOnly && !group.HasExcluded {
			continue
		}
		activeCount := group.ActiveCount
		if query.ExcludedMode != dto.AdminAnalyticsExcludedModeIncludedOnly {
			activeCount += group.ExcludedActiveCount
		}
		if query.ActiveOnly && activeCount == 0 {
			continue
		}
		inviteeID := group.Invitee.Id
		inviterID := group.Invitee.InviterId
		items = append(items, dto.AdminInvitationPaidInvitee{
			InviteeUserID:                           group.Invitee.Id,
			InviteeUsername:                         group.Invitee.Username,
			InviterUserID:                           group.Invitee.InviterId,
			RegisteredAt:                            group.Invitee.CreatedAt,
			PaidSubscriptionSnapshotCount:           group.SnapshotCount,
			RecognizedPaidUnits:                     group.RecognizedUnits,
			ActivePaidSubscriptionCount:             activeCount,
			RecognizedPaidAmountByCurrency:          group.Recognized.breakdownWithPreferredCurrency(query.Currency),
			ActiveRemainingValueByCurrency:          group.ActiveRemaining.breakdownWithPreferredCurrency(query.Currency),
			ActivePaidAmountByCurrency:              group.ActiveAmount.breakdownWithPreferredCurrency(query.Currency),
			Excluded:                                group.Excluded,
			ExcludedReason:                          group.ExcludedReason,
			ExcludedAt:                              group.ExcludedAt,
			ExcludedBy:                              group.ExcludedBy,
			WouldHavePaidAmountByCurrency:           group.WouldHavePaid.breakdownWithPreferredCurrency(query.Currency),
			WouldHaveActiveRemainingValueByCurrency: group.WouldHaveActive.breakdownWithPreferredCurrency(query.Currency),
			Drilldown:                               &dto.AdminAnalyticsDrilldownTarget{Kind: "invitation_paid_invitee", InviterID: &inviterID, InviteeID: &inviteeID, Tab: "invitation-paid-subscriptions"},
		})
	}
	return items
}

func adminInvitationSubscriptionItem(row adminInvitationPaidRow) dto.AdminInvitationPaidSubscriptionRecord {
	planID := row.Plan.Id
	inviteeID := row.Invitee.Id
	inviterID := row.Inviter.Id
	subscriptionID := row.Subscription.Id
	item := dto.AdminInvitationPaidSubscriptionRecord{
		SubscriptionID:       row.Subscription.Id,
		InviteeUserID:        row.Invitee.Id,
		InviterUserID:        row.Inviter.Id,
		PlanID:               row.Plan.Id,
		PlanName:             row.Plan.Title,
		PlanPrice:            dto.AdminAnalyticsMoneyAmount{Amount: row.Plan.PriceAmount, Currency: row.Plan.Currency},
		RecognizedPaidUnits:  row.RecognizedUnits,
		RecognizedPaidAmount: dto.AdminAnalyticsMoneyAmount{Amount: row.RecognizedAmount, Currency: row.RecognizedCurrency},
		UnitInferenceBasis:   row.UnitBasis,
		Source:               row.Source,
		GrantReason:          row.Subscription.GrantReason,
		SourceAttribution:    row.SourceAttribution,
		StartTime:            row.Subscription.StartTime,
		EndTime:              row.Subscription.EndTime,
		Status:               row.Subscription.Status,
		Excluded:             row.Excluded,
		ExcludedReason:       row.ExcludedReason,
		Drilldown:            &dto.AdminAnalyticsDrilldownTarget{Kind: "invitation_paid_invitee", UserID: &inviteeID, PlanID: &planID, SubscriptionID: &subscriptionID, InviteeID: &inviteeID, InviterID: &inviterID, Tab: "invitation-paid-subscriptions"},
	}
	if row.ActiveValue != nil {
		item.RecognizedRemainingValue = &dto.AdminAnalyticsMoneyAmount{Amount: row.ActiveValue.RecognizedRemainingValue, Currency: row.Plan.Currency}
	}
	if row.Order != nil {
		orderID := row.Order.Id
		item.PossibleOrderID = &orderID
		item.PaymentProvider = row.Order.PaymentProvider
		item.PaymentMethod = row.Order.PaymentMethod
		item.OrderRecordedAmount = &dto.AdminAnalyticsMoneyAmount{Amount: row.Order.Money, Currency: row.Plan.Currency}
		item.OrderStatus = row.Order.Status
		item.CompleteTime = row.Order.CompleteTime
	}
	return item
}

func GetAdminInvitationPaidSubscriptionsSummary(query AdminAnalyticsQuery) (dto.AdminAnalyticsPanelResponse[dto.AdminInvitationPaidSubscriptionsResponse], error) {
	query = normalizeAdminPaidSubscriptionAnalyticsQuery(query)
	data, err := buildAdminInvitationPaidSubscriptionsData(query)
	if err != nil {
		return dto.AdminAnalyticsPanelResponse[dto.AdminInvitationPaidSubscriptionsResponse]{}, err
	}
	return dto.AdminAnalyticsPanelResponse[dto.AdminInvitationPaidSubscriptionsResponse]{Range: adminAnalyticsRangeMeta(query), Data: dto.AdminInvitationPaidSubscriptionsResponse{Summary: data.Summary}}, nil
}

func GetAdminInvitationPaidSubscriptionsInviters(query AdminAnalyticsQuery) (dto.AdminAnalyticsPanelResponse[dto.AdminInvitationPaidSubscriptionsResponse], error) {
	query = normalizeAdminPaidSubscriptionAnalyticsQuery(query)
	data, err := buildAdminInvitationPaidSubscriptionsData(query)
	if err != nil {
		return dto.AdminAnalyticsPanelResponse[dto.AdminInvitationPaidSubscriptionsResponse]{}, err
	}
	paged, page := paginateAdminAnalyticsList(data.Inviters, query.Limit, query.Offset)
	return dto.AdminAnalyticsPanelResponse[dto.AdminInvitationPaidSubscriptionsResponse]{Range: adminAnalyticsRangeMeta(query), Data: dto.AdminInvitationPaidSubscriptionsResponse{Summary: data.Summary, Inviters: dto.AdminAnalyticsList[dto.AdminInvitationPaidInviter]{Items: paged, Page: page, SortBy: query.SortBy, SortOrder: query.SortOrder}}}, nil
}

func GetAdminInvitationPaidSubscriptionsInvitees(query AdminAnalyticsQuery) (dto.AdminAnalyticsPanelResponse[dto.AdminInvitationPaidSubscriptionsResponse], error) {
	query = normalizeAdminPaidSubscriptionAnalyticsQuery(query)
	data, err := buildAdminInvitationPaidSubscriptionsData(query)
	if err != nil {
		return dto.AdminAnalyticsPanelResponse[dto.AdminInvitationPaidSubscriptionsResponse]{}, err
	}
	paged, page := paginateAdminAnalyticsList(data.Invitees, query.Limit, query.Offset)
	return dto.AdminAnalyticsPanelResponse[dto.AdminInvitationPaidSubscriptionsResponse]{Range: adminAnalyticsRangeMeta(query), Data: dto.AdminInvitationPaidSubscriptionsResponse{Summary: data.Summary, Invitees: dto.AdminAnalyticsList[dto.AdminInvitationPaidInvitee]{Items: paged, Page: page, SortBy: query.SortBy, SortOrder: query.SortOrder}}}, nil
}

func GetAdminInvitationPaidSubscriptionsSubscriptions(query AdminAnalyticsQuery) (dto.AdminAnalyticsPanelResponse[dto.AdminInvitationPaidSubscriptionsResponse], error) {
	query = normalizeAdminPaidSubscriptionAnalyticsQuery(query)
	rows, err := loadAdminInvitationPaidRows(query, false)
	if err != nil {
		return dto.AdminAnalyticsPanelResponse[dto.AdminInvitationPaidSubscriptionsResponse]{}, err
	}
	relationships, err := loadAdminInvitationRelationshipRows(query)
	if err != nil {
		return dto.AdminAnalyticsPanelResponse[dto.AdminInvitationPaidSubscriptionsResponse]{}, err
	}
	data, err := adminBuildInvitationPaidDataFromRows(query, rows, relationships, true)
	if err != nil {
		return dto.AdminAnalyticsPanelResponse[dto.AdminInvitationPaidSubscriptionsResponse]{}, err
	}
	paged, page := paginateAdminAnalyticsList(data.Subscriptions, query.Limit, query.Offset)
	return dto.AdminAnalyticsPanelResponse[dto.AdminInvitationPaidSubscriptionsResponse]{Range: adminAnalyticsRangeMeta(query), Data: dto.AdminInvitationPaidSubscriptionsResponse{Summary: data.Summary, Subscriptions: dto.AdminAnalyticsList[dto.AdminInvitationPaidSubscriptionRecord]{Items: paged, Page: page, SortBy: query.SortBy, SortOrder: query.SortOrder}}}, nil
}

func adminSortInvitationInviters(items []dto.AdminInvitationPaidInviter, query AdminAnalyticsQuery) {
	desc := adminSortDesc(query.SortOrder)
	sort.SliceStable(items, func(i, j int) bool {
		left := items[i]
		right := items[j]
		cmp := 0
		switch query.SortBy {
		case "recognized_invitation_paid_amount":
			cmp = adminCompareFloat(adminAmountInBreakdown(left.RecognizedInvitationPaidAmountByCurrency, query.Currency), adminAmountInBreakdown(right.RecognizedInvitationPaidAmountByCurrency, query.Currency))
		case "active_invitation_paid_amount":
			cmp = adminCompareFloat(adminAmountInBreakdown(left.ActiveInvitationPaidAmountByCurrency, query.Currency), adminAmountInBreakdown(right.ActiveInvitationPaidAmountByCurrency, query.Currency))
		case "active_invitation_remaining_value":
			cmp = adminCompareFloat(adminAmountInBreakdown(left.ActiveInvitationRemainingValueByCurrency, query.Currency), adminAmountInBreakdown(right.ActiveInvitationRemainingValueByCurrency, query.Currency))
		case "paid_invitee_count":
			cmp = left.PaidInviteeCount - right.PaidInviteeCount
		case "active_paid_invitee_count":
			cmp = left.ActivePaidInviteeCount - right.ActivePaidInviteeCount
		default:
			cmp = left.InviterUserID - right.InviterUserID
		}
		if cmp == 0 {
			cmp = left.InviterUserID - right.InviterUserID
		}
		if desc {
			return cmp > 0
		}
		return cmp < 0
	})
}

func adminSortInvitationInvitees(items []dto.AdminInvitationPaidInvitee, query AdminAnalyticsQuery) {
	desc := adminSortDesc(query.SortOrder)
	sort.SliceStable(items, func(i, j int) bool {
		left := items[i]
		right := items[j]
		cmp := 0
		switch query.SortBy {
		case "recognized_paid_amount":
			cmp = adminCompareFloat(adminAmountInBreakdown(left.RecognizedPaidAmountByCurrency, query.Currency), adminAmountInBreakdown(right.RecognizedPaidAmountByCurrency, query.Currency))
		case "active_remaining_value":
			cmp = adminCompareFloat(adminAmountInBreakdown(left.ActiveRemainingValueByCurrency, query.Currency), adminAmountInBreakdown(right.ActiveRemainingValueByCurrency, query.Currency))
		case "paid_subscription_snapshot_count":
			cmp = left.PaidSubscriptionSnapshotCount - right.PaidSubscriptionSnapshotCount
		case "registered_at":
			cmp = adminCompareInt64(left.RegisteredAt, right.RegisteredAt)
		default:
			cmp = left.InviteeUserID - right.InviteeUserID
		}
		if cmp == 0 {
			cmp = left.InviteeUserID - right.InviteeUserID
		}
		if desc {
			return cmp > 0
		}
		return cmp < 0
	})
}

func adminSortInvitationSubscriptions(items []dto.AdminInvitationPaidSubscriptionRecord, query AdminAnalyticsQuery) {
	desc := adminSortDesc(query.SortOrder)
	sort.SliceStable(items, func(i, j int) bool {
		left := items[i]
		right := items[j]
		cmp := 0
		switch query.SortBy {
		case "recognized_paid_amount":
			cmp = adminCompareFloat(adminMoneyAmountForCurrency(left.RecognizedPaidAmount, query.Currency), adminMoneyAmountForCurrency(right.RecognizedPaidAmount, query.Currency))
		case "recognized_remaining_value":
			cmp = adminCompareFloat(adminOptionalMoneyAmountForCurrency(left.RecognizedRemainingValue, query.Currency), adminOptionalMoneyAmountForCurrency(right.RecognizedRemainingValue, query.Currency))
		case "start_time":
			cmp = adminCompareInt64(left.StartTime, right.StartTime)
		case "end_time":
			cmp = adminCompareInt64(left.EndTime, right.EndTime)
		case "plan_price":
			cmp = adminCompareFloat(adminMoneyAmountForCurrency(left.PlanPrice, query.Currency), adminMoneyAmountForCurrency(right.PlanPrice, query.Currency))
		default:
			cmp = left.SubscriptionID - right.SubscriptionID
		}
		if cmp == 0 {
			cmp = left.SubscriptionID - right.SubscriptionID
		}
		if desc {
			return cmp > 0
		}
		return cmp < 0
	})
}

func adminOptionalMoneyAmount(amount *dto.AdminAnalyticsMoneyAmount) float64 {
	if amount == nil {
		return 0
	}
	return amount.Amount
}
