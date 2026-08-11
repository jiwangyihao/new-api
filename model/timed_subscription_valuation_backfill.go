package model

import (
	"sort"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	timedValuationHistoricalGrantEntity     = "timed_subscription_valuation_grant"
	timedValuationHistoricalReasonNoSource  = "timed_source_missing"
	timedValuationHistoricalReasonAmbiguous = "timed_source_ambiguous"
	timedValuationHistoricalReasonInvalid   = "timed_source_invalid"
	timedValuationHistoricalReasonWindow    = "timed_event_window_missing"
	timedValuationHistoricalReasonFX        = "timed_source_fx_invalid"
	timedValuationHistoricalReasonExisting  = "timed_grant_existing"
)

type TimedSubscriptionValuationHistoricalBackfillDiagnostic struct {
	UserSubscriptionID int    `json:"user_subscription_id,omitempty"`
	SourceType         string `json:"source_type"`
	SourceKey          string `json:"source_key"`
	SourceID           int    `json:"source_id"`
	Reason             string `json:"reason"`
}

type TimedSubscriptionValuationHistoricalCurrencyAmount struct {
	Currency     string `json:"currency"`
	AmountMicros int64  `json:"amount_micros,string"`
}

type TimedSubscriptionValuationHistoricalBackfillReport struct {
	RowsTotal                     int64                                                    `json:"rows_total"`
	RowsEstimated                 int64                                                    `json:"rows_estimated"`
	RowsUnknown                   int64                                                    `json:"rows_unknown"`
	RowsSkippedExisting           int64                                                    `json:"rows_skipped_existing"`
	AmbiguousRows                 int64                                                    `json:"ambiguous_rows"`
	InvalidRows                   int64                                                    `json:"invalid_rows"`
	EstimatedCostMicros           int64                                                    `json:"estimated_cost_micros,string"`
	EstimatedCostMicrosByCurrency []TimedSubscriptionValuationHistoricalCurrencyAmount     `json:"estimated_cost_micros_by_currency"`
	UnknownCredit                 int64                                                    `json:"unknown_credit"`
	Reasons                       []CreditValuationMigrationReasonCount                    `json:"reasons"`
	Diagnostics                   []TimedSubscriptionValuationHistoricalBackfillDiagnostic `json:"diagnostics"`
	Batches                       []CreditValuationMigrationBatchBoundary                  `json:"batches"`
}

type timedHistoricalSource struct {
	SubscriptionID  int
	UserID          int
	PlanID          int
	SourceType      string
	SourceKey       string
	SourceID        int
	SourcePayload   string
	PriceMicros     int64
	Currency        string
	GrantCredit     int64
	StartTime       int64
	EndTime         int64
	ValuationMicros int64
	FX              CreditFXRateSnapshot
}

type timedHistoricalSourceIssue struct {
	SourceType string
	SourceKey  string
	SourceID   int
	Reason     string
}

type timedHistoricalSubscriptionCandidate struct {
	SubscriptionID int
	Sources        []timedHistoricalSource
	Issues         []timedHistoricalSourceIssue
}

type timedHistoricalExistingGrantIndex map[int]map[string]TimedSubscriptionValuationGrant

func newTimedSubscriptionValuationHistoricalBackfillReport() TimedSubscriptionValuationHistoricalBackfillReport {
	return TimedSubscriptionValuationHistoricalBackfillReport{
		Reasons:     make([]CreditValuationMigrationReasonCount, 0),
		Diagnostics: make([]TimedSubscriptionValuationHistoricalBackfillDiagnostic, 0),
		Batches:     make([]CreditValuationMigrationBatchBoundary, 0),
	}
}

func RunTimedSubscriptionValuationHistoricalBackfill(db *gorm.DB, request CreditValuationHistoricalBackfillRequest) (TimedSubscriptionValuationHistoricalBackfillReport, error) {
	report := newTimedSubscriptionValuationHistoricalBackfillReport()
	if db == nil {
		return report, ErrDatabase
	}
	normalized, batchSize, err := normalizeCreditValuationHistoricalBackfillRequest(request)
	if err != nil {
		return report, err
	}
	if !db.Migrator().HasTable(&UserSubscription{}) {
		return report, nil
	}
	var subscriptions []UserSubscription
	if err := db.Where("entitlement_type = ?", SubscriptionEntitlementTimed).Order("id ASC").Find(&subscriptions).Error; err != nil {
		return report, err
	}
	report.RowsTotal = int64(len(subscriptions))
	if len(subscriptions) == 0 {
		return report, nil
	}
	subscriptionsByID := make(map[int]UserSubscription, len(subscriptions))
	for _, subscription := range subscriptions {
		subscriptionsByID[subscription.Id] = subscription
	}
	existing, err := timedHistoricalExistingGrants(db, subscriptionsByID)
	if err != nil {
		return report, err
	}
	sourcesBySubscription, err := timedHistoricalSources(db, subscriptionsByID, normalized)
	if err != nil {
		return report, err
	}
	reasons := make(map[string]int64)
	pending := make([]timedHistoricalSource, 0)
	for _, subscription := range subscriptions {
		candidate := sourcesBySubscription[subscription.Id]
		existingForSubscription := existing[subscription.Id]
		existingIdentities := make(map[string]struct{}, len(existingForSubscription))
		occupied := make([]timedHistoricalWindow, 0, len(existingForSubscription))
		blocked := len(candidate.Issues) > 0
		for _, grant := range existingForSubscription {
			identity := timedHistoricalGrantIdentity(grant.SourceType, grant.SourceKey)
			existingIdentities[identity] = struct{}{}
			if !validTimedHistoricalExistingGrant(subscription, grant) {
				blocked = true
				reasons[timedValuationHistoricalReasonInvalid]++
				report.InvalidRows++
				report.Diagnostics = append(report.Diagnostics, TimedSubscriptionValuationHistoricalBackfillDiagnostic{UserSubscriptionID: subscription.Id, SourceType: grant.SourceType, SourceKey: grant.SourceKey, SourceID: grant.SourceId, Reason: timedValuationHistoricalReasonInvalid})
				continue
			}
			report.RowsSkippedExisting++
			occupied = append(occupied, timedHistoricalWindow{Start: grant.EventStartTime, End: grant.EventEndTime, SourceKey: grant.SourceKey})
		}
		sort.Slice(occupied, func(i, j int) bool {
			if occupied[i].Start != occupied[j].Start {
				return occupied[i].Start < occupied[j].Start
			}
			return occupied[i].SourceKey < occupied[j].SourceKey
		})
		sort.Slice(candidate.Sources, func(i, j int) bool { return timedHistoricalSourceLess(candidate.Sources[i], candidate.Sources[j]) })

		// First collect all not-yet-materialized sources.  Validation is performed
		// for the complete subscription before anything enters the write batch:
		// a renewal ambiguity must never leave an earlier window partially applied.
		newSources := make([]timedHistoricalSource, 0, len(candidate.Sources))
		for _, source := range candidate.Sources {
			identity := timedHistoricalGrantIdentity(source.SourceType, source.SourceKey)
			if _, found := existingIdentities[identity]; found {
				continue
			}
			newSources = append(newSources, source)
		}

		for _, issue := range candidate.Issues {
			reason := issue.Reason
			if reason == "" {
				reason = timedValuationHistoricalReasonInvalid
			}
			reasons[reason]++
			report.InvalidRows++
			report.Diagnostics = append(report.Diagnostics, TimedSubscriptionValuationHistoricalBackfillDiagnostic{UserSubscriptionID: subscription.Id, SourceType: issue.SourceType, SourceKey: issue.SourceKey, SourceID: issue.SourceID, Reason: reason})
		}

		// Existing forward grants and all candidate windows form one timeline.
		// Validate the complete timeline before accepting any candidate so a
		// renewal ambiguity cannot leave an earlier window partially applied.
		candidateWindows := append([]timedHistoricalWindow(nil), occupied...)
		for _, source := range newSources {
			if timedHistoricalWindowOverlapsAny(source.StartTime, source.EndTime, candidateWindows) {
				blocked = true
				reasons[timedValuationHistoricalReasonAmbiguous]++
				report.AmbiguousRows++
				report.InvalidRows++
				report.Diagnostics = append(report.Diagnostics, TimedSubscriptionValuationHistoricalBackfillDiagnostic{UserSubscriptionID: subscription.Id, SourceType: source.SourceType, SourceKey: source.SourceKey, SourceID: source.SourceID, Reason: timedValuationHistoricalReasonAmbiguous})
				break
			}
			candidateWindows = append(candidateWindows, timedHistoricalWindow{Start: source.StartTime, End: source.EndTime, SourceKey: source.SourceKey})
		}

		if !blocked && len(newSources) > 0 {
			report.RowsEstimated++
			for _, source := range newSources {
				pending = append(pending, source)
				if err := addTimedHistoricalCurrencyAmount(&report, source.Currency, source.PriceMicros); err != nil {
					return report, err
				}
			}
		}
		if blocked {
			report.RowsUnknown++
			available := maxInt64(subscription.TokenLimit-subscription.TokenUsed, 0)
			var addOK bool
			report.UnknownCredit, addOK = checkedAddInt64(report.UnknownCredit, available)
			if !addOK {
				return report, ErrCreditValuationOverflow
			}
		}
		if !blocked && len(newSources) == 0 && len(candidate.Issues) == 0 && len(candidate.Sources) == 0 && len(existingForSubscription) == 0 {
			report.RowsUnknown++
			report.InvalidRows++
			reasons[timedValuationHistoricalReasonNoSource]++
			report.Diagnostics = append(report.Diagnostics, TimedSubscriptionValuationHistoricalBackfillDiagnostic{UserSubscriptionID: subscription.Id, Reason: timedValuationHistoricalReasonNoSource})
			available := maxInt64(subscription.TokenLimit-subscription.TokenUsed, 0)
			var addOK bool
			report.UnknownCredit, addOK = checkedAddInt64(report.UnknownCredit, available)
			if !addOK {
				return report, ErrCreditValuationOverflow
			}
		}
	}
	for start := 0; start < len(pending); start += batchSize {
		end := start + batchSize
		if end > len(pending) {
			end = len(pending)
		}
		report.Batches = append(report.Batches, CreditValuationMigrationBatchBoundary{Entity: timedValuationHistoricalGrantEntity, StartID: int64(pending[start].SubscriptionID), EndID: int64(pending[end-1].SubscriptionID), Rows: int64(end - start)})
	}
	report.Reasons = sortedCreditValuationMigrationReasonCounts(reasons)
	normalizeTimedSubscriptionValuationHistoricalBackfillReport(&report)
	if !normalized.Apply {
		return report, nil
	}
	if err := writeTimedHistoricalGrants(db, normalized, pending); err != nil {
		return report, err
	}
	return report, nil
}

func timedHistoricalExistingGrants(db *gorm.DB, subscriptions map[int]UserSubscription) (timedHistoricalExistingGrantIndex, error) {
	result := make(timedHistoricalExistingGrantIndex)
	if len(subscriptions) == 0 || !db.Migrator().HasTable(&TimedSubscriptionValuationGrant{}) {
		return result, nil
	}
	ids := make([]int, 0, len(subscriptions))
	for id := range subscriptions {
		ids = append(ids, id)
	}
	var grants []TimedSubscriptionValuationGrant
	if err := db.Where("user_subscription_id IN ?", ids).Order("user_subscription_id ASC, id ASC").Find(&grants).Error; err != nil {
		return nil, err
	}
	for _, grant := range grants {
		if result[grant.UserSubscriptionId] == nil {
			result[grant.UserSubscriptionId] = make(map[string]TimedSubscriptionValuationGrant)
		}
		result[grant.UserSubscriptionId][timedHistoricalGrantIdentity(grant.SourceType, grant.SourceKey)] = grant
	}
	return result, nil
}

func addTimedHistoricalCurrencyAmount(report *TimedSubscriptionValuationHistoricalBackfillReport, currency string, amount int64) error {
	if report == nil || amount < 0 {
		return ErrCreditValuationOverflow
	}
	currency, err := NormalizeCreditValuationCurrency(currency)
	if err != nil {
		return err
	}
	for index := range report.EstimatedCostMicrosByCurrency {
		if report.EstimatedCostMicrosByCurrency[index].Currency != currency {
			continue
		}
		updated, ok := checkedAddInt64(report.EstimatedCostMicrosByCurrency[index].AmountMicros, amount)
		if !ok {
			return ErrCreditValuationOverflow
		}
		report.EstimatedCostMicrosByCurrency[index].AmountMicros = updated
		return nil
	}
	report.EstimatedCostMicrosByCurrency = append(report.EstimatedCostMicrosByCurrency, TimedSubscriptionValuationHistoricalCurrencyAmount{Currency: currency, AmountMicros: amount})
	return nil
}

type timedHistoricalWindow struct {
	Start     int64
	End       int64
	SourceKey string
}

func timedHistoricalGrantIdentity(sourceType, sourceKey string) string {
	return strings.TrimSpace(sourceType) + "\x00" + strings.TrimSpace(sourceKey)
}

func timedHistoricalSourceLess(left, right timedHistoricalSource) bool {
	if left.StartTime != right.StartTime {
		return left.StartTime < right.StartTime
	}
	if left.EndTime != right.EndTime {
		return left.EndTime < right.EndTime
	}
	if left.SourceType != right.SourceType {
		return left.SourceType < right.SourceType
	}
	return left.SourceKey < right.SourceKey
}

func validTimedHistoricalExistingGrant(subscription UserSubscription, grant TimedSubscriptionValuationGrant) bool {
	if !validTimedSubscriptionValuationGrant(grant) || grant.UserSubscriptionId != subscription.Id || grant.UserId != subscription.UserId || grant.PlanId != subscription.PlanId || grant.EventStartTime < subscription.StartTime || grant.EventEndTime > subscription.EndTime || strings.TrimSpace(grant.SourceSnapshot) == "" {
		return false
	}
	if grant.Confidence != TimedSubscriptionValuationConfidenceExact {
		return true
	}
	var snapshot timedSubscriptionGrantSourceSnapshot
	if err := common.UnmarshalJsonStr(grant.SourceSnapshot, &snapshot); err != nil {
		return false
	}
	return snapshot.IdempotencyKey == grant.IdempotencyKey && snapshot.SourceType == grant.SourceType && snapshot.SourceKey == grant.SourceKey && snapshot.SourceId == grant.SourceId && snapshot.UserId == grant.UserId && snapshot.PlanId == grant.PlanId && snapshot.SourcePriceMicros == grant.SourcePriceMicros && strings.EqualFold(snapshot.SourceCurrency, grant.SourceCurrency) && snapshot.GrantCredit == grant.GrantCredit && snapshot.ValuationRuleVersion == grant.RuleVersion
}

func timedHistoricalWindowOverlapsAny(start, end int64, occupied []timedHistoricalWindow) bool {
	for _, window := range occupied {
		if start < window.End && window.Start < end {
			return true
		}
	}
	return false
}
func timedHistoricalSources(db *gorm.DB, subscriptions map[int]UserSubscription, request CreditValuationHistoricalBackfillRequest) (map[int]timedHistoricalSubscriptionCandidate, error) {
	result := make(map[int]timedHistoricalSubscriptionCandidate, len(subscriptions))
	if db.Migrator().HasTable(&SubscriptionOrder{}) {
		var orders []SubscriptionOrder
		if err := db.Where("status = ? AND fulfilled_subscription_id > 0 AND entitlement_snapshot <> ?", common.TopUpStatusSuccess, "").Order("id ASC").Find(&orders).Error; err != nil {
			return nil, err
		}
		windowsByOrder := make(map[int][]InvitationRewardEvent)
		if db.Migrator().HasTable(&InvitationRewardEvent{}) {
			var windows []InvitationRewardEvent
			if err := db.Where("source_order_id > 0 AND source_subscription_id > 0 AND event_start_time > 0 AND event_end_time > event_start_time").Order("source_order_id ASC, id ASC").Find(&windows).Error; err != nil {
				return nil, err
			}
			for _, window := range windows {
				windowsByOrder[window.SourceOrderId] = append(windowsByOrder[window.SourceOrderId], window)
			}
		}
		for _, order := range orders {
			subscription, ok := subscriptions[order.FulfilledSubscriptionID]
			if !ok {
				continue
			}
			candidate := result[order.FulfilledSubscriptionID]
			candidate.SubscriptionID = order.FulfilledSubscriptionID
			source, issue := timedHistoricalOrderSource(order, subscription, windowsByOrder[order.Id], request)
			if issue != nil {
				candidate.Issues = append(candidate.Issues, *issue)
			} else {
				candidate.Sources = append(candidate.Sources, *source)
			}
			result[order.FulfilledSubscriptionID] = candidate
		}
	}
	if db.Migrator().HasTable(&Redemption{}) {
		var redemptions []Redemption
		if err := db.Where("status = ? AND fulfillment_mode = ? AND fulfillment_subscription_id > 0 AND fulfillment_snapshot <> ?", common.RedemptionCodeStatusUsed, RedemptionModeTimed, "").Order("id ASC").Find(&redemptions).Error; err != nil {
			return nil, err
		}
		for _, redemption := range redemptions {
			subscription, ok := subscriptions[redemption.FulfillmentSubscriptionId]
			if !ok {
				continue
			}
			candidate := result[redemption.FulfillmentSubscriptionId]
			candidate.SubscriptionID = redemption.FulfillmentSubscriptionId
			source, issue := timedHistoricalRedemptionSource(redemption, subscription, request)
			if issue != nil {
				candidate.Issues = append(candidate.Issues, *issue)
			} else {
				candidate.Sources = append(candidate.Sources, *source)
			}
			result[redemption.FulfillmentSubscriptionId] = candidate
		}
	}
	return result, nil
}

func timedHistoricalOrderSource(order SubscriptionOrder, subscription UserSubscription, windows []InvitationRewardEvent, request CreditValuationHistoricalBackfillRequest) (*timedHistoricalSource, *timedHistoricalSourceIssue) {
	sourceType := TimedSubscriptionGrantSourceOrder
	sourceKey := sourceType + ":" + strconv.Itoa(order.Id)
	issue := func(reason string) (*timedHistoricalSource, *timedHistoricalSourceIssue) {
		return nil, &timedHistoricalSourceIssue{SourceType: sourceType, SourceKey: sourceKey, SourceID: order.Id, Reason: reason}
	}
	if order.UserId != subscription.UserId || order.PlanId != subscription.PlanId || order.FulfilledSubscriptionID != subscription.Id {
		return issue(timedValuationHistoricalReasonInvalid)
	}
	if len(windows) > 1 {
		return issue(timedValuationHistoricalReasonWindow)
	}
	if len(windows) != 1 {
		return issue(timedValuationHistoricalReasonWindow)
	}
	window := windows[0]
	if window.SourceOrderId != order.Id || window.SourceSubscriptionId != subscription.Id || window.EventStartTime <= 0 || window.EventEndTime <= window.EventStartTime {
		return issue(timedValuationHistoricalReasonWindow)
	}
	eventStart, eventEnd := window.EventStartTime, window.EventEndTime

	snapshot, err := UnmarshalSubscriptionEntitlementSnapshot(order.EntitlementSnapshot)
	if err != nil {
		return issue(timedValuationHistoricalReasonInvalid)
	}
	return timedHistoricalSnapshotSource(snapshot, subscription, sourceType, sourceKey, order.Id, order.EntitlementSnapshot, eventStart, eventEnd, request)
}

func timedHistoricalRedemptionSource(redemption Redemption, subscription UserSubscription, request CreditValuationHistoricalBackfillRequest) (*timedHistoricalSource, *timedHistoricalSourceIssue) {
	sourceType := TimedSubscriptionGrantSourceRedemption
	sourceKey := sourceType + ":" + strconv.Itoa(redemption.Id)
	issue := func(reason string) (*timedHistoricalSource, *timedHistoricalSourceIssue) {
		return nil, &timedHistoricalSourceIssue{SourceType: sourceType, SourceKey: sourceKey, SourceID: redemption.Id, Reason: reason}
	}
	if redemption.UsedUserId != subscription.UserId || redemption.PlanId != subscription.PlanId || redemption.FulfillmentSubscriptionId != subscription.Id {
		return issue(timedValuationHistoricalReasonInvalid)
	}
	var fulfillment RedemptionFulfillmentSnapshot
	if err := common.UnmarshalJsonStr(redemption.FulfillmentSnapshot, &fulfillment); err != nil {
		return issue(timedValuationHistoricalReasonInvalid)
	}
	payload := redemption.FulfillmentSnapshot
	return timedHistoricalSnapshotSource(fulfillment.Entitlement, subscription, sourceType, sourceKey, redemption.Id, payload, fulfillment.EventStartTime, fulfillment.EventEndTime, request)
}

func timedHistoricalSnapshotSource(snapshot SubscriptionEntitlementSnapshot, subscription UserSubscription, sourceType, sourceKey string, sourceID int, payload string, eventStart, eventEnd int64, request CreditValuationHistoricalBackfillRequest) (*timedHistoricalSource, *timedHistoricalSourceIssue) {
	issue := func(reason string) (*timedHistoricalSource, *timedHistoricalSourceIssue) {
		return nil, &timedHistoricalSourceIssue{SourceType: sourceType, SourceKey: sourceKey, SourceID: sourceID, Reason: reason}
	}
	if snapshot.PlanID <= 0 || snapshot.PlanID != subscription.PlanId || snapshot.MonthlyTokenLimit <= 0 || snapshot.ListPriceMicros == nil || *snapshot.ListPriceMicros <= 0 {
		return issue(timedValuationHistoricalReasonInvalid)
	}
	mode, err := NormalizeSubscriptionPurchaseMode(snapshot.PurchaseMode)
	if err != nil || mode != SubscriptionPurchaseModeTimed || strings.TrimSpace(snapshot.PlanEntitlementType) != SubscriptionEntitlementTimed {
		return issue(timedValuationHistoricalReasonInvalid)
	}
	currency := strings.ToUpper(strings.TrimSpace(snapshot.ListPriceCurrency))
	if currency == "" {
		currency = strings.ToUpper(strings.TrimSpace(snapshot.Currency))
	}
	currency, err = NormalizeCreditValuationCurrency(currency)
	if err != nil {
		return issue(timedValuationHistoricalReasonInvalid)
	}
	if eventStart <= 0 || eventEnd <= eventStart {
		return issue(timedValuationHistoricalReasonWindow)
	}
	if eventStart < subscription.StartTime || eventEnd > subscription.EndTime {
		return issue(timedValuationHistoricalReasonWindow)
	}
	fx, err := historicalCreditFXForCurrency(request, currency)
	if err != nil {
		return issue(timedValuationHistoricalReasonFX)
	}
	valuationMicros, err := fx.ConvertMicros(*snapshot.ListPriceMicros)
	if err != nil || valuationMicros <= 0 {
		return issue(timedValuationHistoricalReasonFX)
	}
	return &timedHistoricalSource{
		SubscriptionID: subscription.Id, UserID: subscription.UserId, PlanID: subscription.PlanId,
		SourceType: sourceType, SourceKey: sourceKey, SourceID: sourceID, SourcePayload: payload,
		PriceMicros: *snapshot.ListPriceMicros, Currency: currency, GrantCredit: snapshot.MonthlyTokenLimit,
		StartTime: eventStart, EndTime: eventEnd,
		ValuationMicros: valuationMicros, FX: fx,
	}, nil
}

func writeTimedHistoricalGrants(db *gorm.DB, request CreditValuationHistoricalBackfillRequest, sources []timedHistoricalSource) error {
	return db.Transaction(func(tx *gorm.DB) error {
		for _, source := range sources {
			query := tx.Where("id = ? AND entitlement_type = ?", source.SubscriptionID, SubscriptionEntitlementTimed)
			if historicalCreditCanLock(tx) {
				query = query.Clauses(clause.Locking{Strength: "UPDATE"})
			}
			var subscription UserSubscription
			if err := query.First(&subscription).Error; err != nil {
				return err
			}
			if subscription.UserId != source.UserID || subscription.PlanId != source.PlanID || source.StartTime < subscription.StartTime || source.EndTime > subscription.EndTime {
				return ErrTimedSubscriptionGrantInvalid
			}
			var existing int64
			if err := tx.Model(&TimedSubscriptionValuationGrant{}).
				Where("idempotency_key = ? OR (source_type = ? AND source_key = ?)", "historical:"+source.SourceKey, source.SourceType, source.SourceKey).
				Count(&existing).Error; err != nil {
				return err
			}
			if existing != 0 {
				return ErrCreditValuationMigrationConflict
			}
			var overlap int64
			if err := tx.Model(&TimedSubscriptionValuationGrant{}).
				Where("user_subscription_id = ? AND event_start_time < ? AND event_end_time > ?", source.SubscriptionID, source.EndTime, source.StartTime).
				Count(&overlap).Error; err != nil {
				return err
			}
			if overlap != 0 {
				return ErrCreditValuationMigrationConflict
			}
			payload, err := common.Marshal(struct {
				SourceType       string `json:"source_type"`
				SourceKey        string `json:"source_key"`
				SourceID         int    `json:"source_id"`
				Historical       bool   `json:"historical"`
				OriginalSnapshot string `json:"original_snapshot"`
			}{source.SourceType, source.SourceKey, source.SourceID, true, source.SourcePayload})
			if err != nil {
				return err
			}
			now, err := getDBTimestampStrictTx(tx)
			if err != nil {
				return err
			}
			grant := TimedSubscriptionValuationGrant{IdempotencyKey: "historical:" + source.SourceKey, UserSubscriptionId: source.SubscriptionID, UserId: source.UserID, PlanId: source.PlanID, SourceType: source.SourceType, SourceKey: source.SourceKey, SourceId: source.SourceID, EventStartTime: source.StartTime, EventEndTime: source.EndTime, GrantCredit: source.GrantCredit, SourcePriceMicros: source.PriceMicros, SourceCurrency: source.Currency, ValuationAmountMicros: source.ValuationMicros, ValuationCurrency: source.FX.ValuationCurrency, Confidence: CreditValuationConfidenceEstimated, RuleVersion: CreditValuationRuleVersion, FxRateNumerator: source.FX.Numerator, FxRateDenominator: source.FX.Denominator, FxCapturedAt: source.FX.CapturedAt, SourceSnapshot: string(payload), CreatedAt: now}
			if err := tx.Create(&grant).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func normalizeTimedSubscriptionValuationHistoricalBackfillReport(report *TimedSubscriptionValuationHistoricalBackfillReport) {
	if report == nil {
		return
	}
	if report.EstimatedCostMicrosByCurrency == nil {
		report.EstimatedCostMicrosByCurrency = make([]TimedSubscriptionValuationHistoricalCurrencyAmount, 0)
	}
	if report.Reasons == nil {
		report.Reasons = make([]CreditValuationMigrationReasonCount, 0)
	}
	if report.Diagnostics == nil {
		report.Diagnostics = make([]TimedSubscriptionValuationHistoricalBackfillDiagnostic, 0)
	}
	if report.Batches == nil {
		report.Batches = make([]CreditValuationMigrationBatchBoundary, 0)
	}
	sort.Slice(report.EstimatedCostMicrosByCurrency, func(i, j int) bool {
		return report.EstimatedCostMicrosByCurrency[i].Currency < report.EstimatedCostMicrosByCurrency[j].Currency
	})
	report.EstimatedCostMicros = 0
	if len(report.EstimatedCostMicrosByCurrency) == 1 {
		report.EstimatedCostMicros = report.EstimatedCostMicrosByCurrency[0].AmountMicros
	}
	sort.Slice(report.Diagnostics, func(i, j int) bool {
		left, right := report.Diagnostics[i], report.Diagnostics[j]
		if left.UserSubscriptionID != right.UserSubscriptionID {
			return left.UserSubscriptionID < right.UserSubscriptionID
		}
		if left.SourceType != right.SourceType {
			return left.SourceType < right.SourceType
		}
		if left.SourceKey != right.SourceKey {
			return left.SourceKey < right.SourceKey
		}
		if left.SourceID != right.SourceID {
			return left.SourceID < right.SourceID
		}
		return left.Reason < right.Reason
	})
	sortCreditValuationMigrationBatches(report.Batches)
}
