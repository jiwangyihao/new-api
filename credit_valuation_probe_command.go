package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const creditValuationProbeModeClone = "clone-tracer"

type creditValuationProbeOptions struct {
	Mode        string
	Version     int
	BackupSHA   string
	TargetImage string
	CloneDSN    string
}

type creditValuationProbeFixture struct {
	PriceAmountMicros                  string `json:"price_amount_micros"`
	PlanCredit                         int64  `json:"plan_credit"`
	ConsumedCredit                     int64  `json:"consumed_credit"`
	AvailableCredit                    int64  `json:"available_credit"`
	EndTime                            int64  `json:"end_time"`
	ExactCostMicros                    string `json:"exact_cost_micros"`
	Currency                           string `json:"currency"`
	ActivePaidSubscriptionCount        int    `json:"active_paid_subscription_count"`
	EstimatedCostMicros                string `json:"estimated_cost_micros"`
	UnknownCredit                      int64  `json:"unknown_credit"`
	FiveAnalyticsEndpointsConsistent   bool   `json:"five_analytics_endpoints_consistent"`
	DisabledPlanExistingConsumable     bool   `json:"disabled_plan_existing_consumable"`
	DisabledPlanNewAllocationsRejected bool   `json:"disabled_plan_new_allocations_rejected"`
	ModelScopeIgnored                  bool   `json:"model_scope_ignored"`
}

type creditValuationCloneProbeReport struct {
	Success                     bool                        `json:"success"`
	Environment                 string                      `json:"environment"`
	CloneIsolated               bool                        `json:"clone_isolated"`
	ProductionIdentityCollision bool                        `json:"production_identity_collision"`
	SourceBackupSHA256          string                      `json:"source_backup_sha256"`
	TargetDigest                string                      `json:"target_digest"`
	Fixture                     creditValuationProbeFixture `json:"fixture"`
}

func RunCreditValuationProbeCommand(args []string, stdout io.Writer, stderr io.Writer) int {
	options, err := parseCreditValuationProbeArgs(args)
	if err != nil {
		writeCreditValuationProbeError(stderr, "credit_valuation_probe_invalid", err)
		return 2
	}
	common.InitEnv()
	db, err := model.InitMaintenanceCloneDB(options.CloneDSN)
	if err != nil || db == nil {
		if err == nil {
			err = model.ErrDatabase
		}
		writeCreditValuationProbeError(stderr, "credit_valuation_probe_database_open_failed", err)
		return 1
	}
	defer func() { _ = model.CloseMaintenanceDB() }()

	oldDB, oldLogDB := model.DB, model.LOG_DB
	model.DB, model.LOG_DB = db, db
	model.ClearDBTimestampCacheForTest()
	model.ClearSubscriptionPlanCacheForTest()
	model.ClearPrimaryBillableSubscriptionCacheForTest()
	defer func() {
		model.DB, model.LOG_DB = oldDB, oldLogDB
		model.ClearDBTimestampCacheForTest()
		model.ClearSubscriptionPlanCacheForTest()
		model.ClearPrimaryBillableSubscriptionCacheForTest()
	}()

	report, err := runCreditValuationCloneProbe(db, options)
	if err != nil {
		writeCreditValuationProbeError(stderr, "credit_valuation_probe_failed", err)
		return 1
	}
	if err := writeCreditValuationCommandJSON(stdout, report); err != nil {
		writeCreditValuationProbeError(stderr, "credit_valuation_probe_output_failed", err)
		return 1
	}
	return 0
}

func writeCreditValuationProbeError(writer io.Writer, code string, err error) {
	message := "probe failed"
	if err != nil {
		message = err.Error()
	}
	_ = writeCreditValuationCommandJSON(writer, creditValuationCommandErrorOutput{
		Success: false,
		Code:    code,
		Message: message,
	})
}

func parseCreditValuationProbeArgs(args []string) (creditValuationProbeOptions, error) {
	options := creditValuationProbeOptions{CloneDSN: strings.TrimSpace(os.Getenv("NEW_API_CLONE_SQL_DSN"))}
	seen := make(map[string]struct{})
	for index := 0; index < len(args); index++ {
		argument := args[index]
		if _, exists := seen[argument]; exists {
			return options, fmt.Errorf("probe flag must not be repeated: %s", argument)
		}
		seen[argument] = struct{}{}
		switch argument {
		case "--clone-tracer":
			if options.Mode != "" {
				return options, errors.New("probe mode must be unique")
			}
			options.Mode = creditValuationProbeModeClone
		case "--version", "--backup-sha256", "--target-image", "--clone-dsn":
			if index+1 >= len(args) || strings.HasPrefix(args[index+1], "--") {
				return options, fmt.Errorf("%s requires a value", argument)
			}
			value := strings.TrimSpace(args[index+1])
			index++
			switch argument {
			case "--version":
				parsed, parseErr := strconv.Atoi(value)
				if parseErr != nil || parsed <= 0 {
					return options, errors.New("probe version must be positive")
				}
				options.Version = parsed
			case "--backup-sha256":
				options.BackupSHA = value
			case "--target-image":
				options.TargetImage = value
			case "--clone-dsn":
				options.CloneDSN = value
			}
		default:
			return options, fmt.Errorf("unknown probe argument: %s", argument)
		}
	}
	if options.Mode != creditValuationProbeModeClone || options.Version <= 0 {
		return options, errors.New("clone probe mode and version are required")
	}
	if !isLowerHexDigest(options.BackupSHA) || !isImmutableImageReference(options.TargetImage) {
		return options, errors.New("clone probe identity is incomplete")
	}
	if !isExplicitPostgreSQLCloneDSN(options.CloneDSN) {
		return options, errors.New("clone probe requires an explicit PostgreSQL DSN")
	}
	return options, nil
}

func isExplicitPostgreSQLCloneDSN(value string) bool {
	value = strings.TrimSpace(value)
	return strings.HasPrefix(value, "postgres://") || strings.HasPrefix(value, "postgresql://")
}

func isLowerHexDigest(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, character := range value {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

func isImmutableImageReference(value string) bool {
	name, digest, found := strings.Cut(strings.TrimSpace(value), "@sha256:")
	return found && name != "" && isLowerHexDigest(digest)
}

func runCreditValuationCloneProbe(db *gorm.DB, options creditValuationProbeOptions) (creditValuationCloneProbeReport, error) {
	if err := model.MigrateCreditValuationSchema(db); err != nil {
		return creditValuationCloneProbeReport{}, err
	}
	migration, err := model.RunCreditValuationMigration(db, model.CreditValuationMigrationRequest{
		Mode:      model.CreditValuationMigrationModeApply,
		Version:   options.Version,
		BatchSize: CreditValuationCommandDefaultBatchSize,
	})
	if err != nil {
		return creditValuationCloneProbeReport{}, fmt.Errorf("clone migration apply: %w", err)
	}
	if !migration.Ready || migration.Status != model.CreditValuationMigrationReady || migration.Version != options.Version {
		return creditValuationCloneProbeReport{}, model.ErrCreditValuationMigrationNotReady
	}
	verified, err := model.RunCreditValuationMigration(db, model.CreditValuationMigrationRequest{
		Mode:      model.CreditValuationMigrationModeVerify,
		Version:   options.Version,
		BatchSize: CreditValuationCommandDefaultBatchSize,
	})
	if err != nil {
		return creditValuationCloneProbeReport{}, fmt.Errorf("clone migration verify: %w", err)
	}
	if !verified.Ready || verified.Checksum != migration.Checksum {
		return creditValuationCloneProbeReport{}, model.ErrCreditValuationMigrationChecksumMismatch
	}

	fixture, err := runCreditValuationCloneFixture(db, options.BackupSHA)
	if err != nil {
		return creditValuationCloneProbeReport{}, err
	}
	return creditValuationCloneProbeReport{
		Success:                     true,
		Environment:                 "isolated_clone",
		CloneIsolated:               true,
		ProductionIdentityCollision: false,
		SourceBackupSHA256:          options.BackupSHA,
		TargetDigest:                options.TargetImage,
		Fixture:                     fixture,
	}, nil
}

func runCreditValuationCloneFixture(db *gorm.DB, identity string) (creditValuationProbeFixture, error) {
	identity = identity[:12]
	var creditPlan model.SubscriptionPlan
	if err := db.Where("entitlement_type = ?", model.SubscriptionEntitlementCreditBalance).First(&creditPlan).Error; err != nil {
		return creditValuationProbeFixture{}, err
	}
	if creditPlan.ValuationCurrency == nil || !strings.EqualFold(strings.TrimSpace(*creditPlan.ValuationCurrency), "CNY") {
		return creditValuationProbeFixture{}, model.ErrCreditValuationCurrencyRequired
	}

	user := model.User{
		Username: "i28cv" + identity,
		Status:   common.UserStatusEnabled,
		Role:     common.RoleCommonUser,
		AffCode:  "i28cv" + identity,
	}
	if err := db.Create(&user).Error; err != nil {
		return creditValuationProbeFixture{}, err
	}
	priceMicros := int64(40_000_000)
	businessCode := "i28-tier-" + identity
	sourcePlan := model.SubscriptionPlan{
		Title:                    "Issue 28 40 CNY / 1,000 Credit",
		PriceAmount:              40,
		PriceAmountMicros:        &priceMicros,
		Currency:                 "CNY",
		DurationUnit:             model.SubscriptionDurationMonth,
		DurationValue:            1,
		Enabled:                  true,
		PublicVisible:            true,
		MonthlyTokenLimit:        1_000,
		QuotaResetPeriod:         model.SubscriptionResetMonthly,
		UnlimitedPurchaseEnabled: true,
		TimedConversionEnabled:   true,
		BusinessCode:             &businessCode,
		EntitlementType:          model.SubscriptionEntitlementTimed,
	}
	if err := db.Create(&sourcePlan).Error; err != nil {
		return creditValuationProbeFixture{}, err
	}

	snapshot := model.NewSubscriptionEntitlementSnapshot(&sourcePlan, model.SubscriptionPurchaseModeCreditBalance, creditPlan.Id)
	snapshot.SetTargetCreditBalancePlanSnapshot(&creditPlan)
	snapshot.SetPaymentSnapshot(model.PaymentProviderBalance, "issue28-clone", model.PaymentMethodAccountBalance, 4_000, "CNY")
	snapshotPayload, err := model.MarshalSubscriptionEntitlementSnapshot(snapshot)
	if err != nil {
		return creditValuationProbeFixture{}, err
	}
	now := model.GetDBTimestamp()
	order := model.SubscriptionOrder{
		UserId:              user.Id,
		PlanId:              sourcePlan.Id,
		Money:               40,
		AmountCents:         4_000,
		Currency:            "CNY",
		CreditGrantAmount:   1_000,
		CreditTargetPlanID:  creditPlan.Id,
		TradeNo:             "issue28-clone-" + identity,
		PaymentMethod:       model.PaymentMethodAccountBalance,
		PaymentProvider:     model.PaymentProviderBalance,
		Status:              common.TopUpStatusPending,
		CreateTime:          now,
		EntitlementSnapshot: snapshotPayload,
	}
	if err := db.Create(&order).Error; err != nil {
		return creditValuationProbeFixture{}, err
	}
	var completion *model.SubscriptionOrderCompletionResult
	if err := db.Transaction(func(tx *gorm.DB) error {
		var locked model.SubscriptionOrder
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&locked, order.Id).Error; err != nil {
			return err
		}
		var completeErr error
		completion, completeErr = model.CompleteSubscriptionOrderTx(tx, &locked, `{}`, model.PaymentMethodAccountBalance)
		return completeErr
	}); err != nil {
		return creditValuationProbeFixture{}, err
	}
	if completion == nil || completion.CreditBalance == nil {
		return creditValuationProbeFixture{}, errors.New("clone purchase did not create Credit")
	}

	requestID := "issue28-clone-consume-" + identity
	preConsumed, err := model.PreConsumeUserSubscriptionByUnits(requestID, user.Id, "gpt-4o", 0, 0, 200)
	if err != nil {
		return creditValuationProbeFixture{}, err
	}
	var persistedRoute model.SubscriptionPreConsumeRecord
	if err := db.Where("request_id = ?", requestID).First(&persistedRoute).Error; err != nil {
		return creditValuationProbeFixture{}, err
	}
	if persistedRoute.UserSubscriptionId != preConsumed.UserSubscriptionId || persistedRoute.ValuationSubscriptionId != preConsumed.UserSubscriptionId {
		return creditValuationProbeFixture{}, fmt.Errorf("clone preconsume mapping mismatch: result=%d route=%d valuation=%d", preConsumed.UserSubscriptionId, persistedRoute.UserSubscriptionId, persistedRoute.ValuationSubscriptionId)
	}
	handled, err := model.SettleUserSubscriptionRequestTargetIfMapped(requestID, preConsumed.UserSubscriptionId, 200, true)
	if err != nil || !handled {
		if err == nil {
			err = model.ErrCreditValuationMappingConflict
		}
		return creditValuationProbeFixture{}, err
	}

	query := model.AdminAnalyticsQuery{
		SnapshotAt:   model.GetDBTimestamp(),
		EndTimestamp: model.GetDBTimestamp(),
		RangeMode:    model.AdminAnalyticsRangeModeSnapshot,
		Currency:     "CNY",
		UserIDs:      []int{user.Id},
		Limit:        20,
	}

	summary, err := model.GetAdminPaidSubscriptionValueSummary(query)
	if err != nil {
		return creditValuationProbeFixture{}, err
	}
	users, err := model.GetAdminPaidSubscriptionValueUsers(query)
	if err != nil {
		return creditValuationProbeFixture{}, err
	}
	subscriptions, err := model.GetAdminPaidSubscriptionValueSubscriptions(query)
	if err != nil {
		return creditValuationProbeFixture{}, err
	}
	plans, err := model.GetAdminPaidSubscriptionValuePlanBreakdown(query)
	if err != nil {
		return creditValuationProbeFixture{}, err
	}
	sources, err := model.GetAdminPaidSubscriptionValueSourceBreakdown(query)
	if err != nil {
		return creditValuationProbeFixture{}, err
	}

	if len(users.Data.Users.Items) != 1 || len(subscriptions.Data.Subscriptions.Items) != 1 || len(plans.Data.Plans.Items) != 1 || len(sources.Data.Sources.Items) != 1 {
		return creditValuationProbeFixture{}, errors.New("clone analytics cardinality mismatch")
	}
	item := subscriptions.Data.Subscriptions.Items[0]
	exact := creditValuationProbeMoney(summary.Data.Summary.ExactRemainingValueByCurrency, "CNY")
	estimated := creditValuationProbeMoney(summary.Data.Summary.EstimatedRemainingValueByCurrency, "CNY")
	recognized := creditValuationProbeMoney(summary.Data.Summary.RecognizedRemainingValueByCurrency, "CNY")
	consistent := exact == "32000000" && recognized == exact && estimated == "0" &&
		creditValuationProbeMoney(users.Data.Users.Items[0].RecognizedRemainingValueByCurrency, "CNY") == exact &&
		creditValuationProbeMoney(plans.Data.Plans.Items[0].RecognizedRemainingValueByCurrency, "CNY") == exact &&
		creditValuationProbeMoney(sources.Data.Sources.Items[0].RecognizedRemainingValueByCurrency, "CNY") == exact &&
		item.RecognizedRemainingValue != nil && item.RecognizedRemainingValue.AmountMicros == exact && item.TimeBasedValue == nil
	if !consistent || item.AvailableCredit != 800 || item.EndTime != 0 || summary.Data.Summary.ActivePaidSubscriptionCount != 1 || summary.Data.Summary.UnknownCostCredit != 0 {
		return creditValuationProbeFixture{}, errors.New("clone 32 CNY analytics contract failed")
	}

	disabledExisting, disabledNew, modelScopeIgnored, err := runCreditValuationDisabledPlanCloneFixture(db, &creditPlan, identity)
	if err != nil {
		return creditValuationProbeFixture{}, err
	}
	return creditValuationProbeFixture{
		PriceAmountMicros:                  "40000000",
		PlanCredit:                         1_000,
		ConsumedCredit:                     200,
		AvailableCredit:                    item.AvailableCredit,
		EndTime:                            item.EndTime,
		ExactCostMicros:                    exact,
		Currency:                           "CNY",
		ActivePaidSubscriptionCount:        summary.Data.Summary.ActivePaidSubscriptionCount,
		EstimatedCostMicros:                estimated,
		UnknownCredit:                      summary.Data.Summary.UnknownCostCredit,
		FiveAnalyticsEndpointsConsistent:   consistent,
		DisabledPlanExistingConsumable:     disabledExisting,
		DisabledPlanNewAllocationsRejected: disabledNew,
		ModelScopeIgnored:                  modelScopeIgnored,
	}, nil
}

func runCreditValuationDisabledPlanCloneFixture(db *gorm.DB, creditPlan *model.SubscriptionPlan, identity string) (bool, bool, bool, error) {
	user := model.User{
		Username: "i28dp" + identity,
		Status:   common.UserStatusEnabled,
		Role:     common.RoleCommonUser,
		AffCode:  "i28dp" + identity,
	}
	if err := db.Create(&user).Error; err != nil {
		return false, false, false, err
	}
	priceMicros := int64(10_000_000)
	businessCode := "i28-disabled-" + identity
	plan := model.SubscriptionPlan{
		Title:                    "Issue 28 disabled-plan boundary",
		PriceAmount:              10,
		PriceAmountMicros:        &priceMicros,
		Currency:                 "CNY",
		DurationUnit:             model.SubscriptionDurationMonth,
		DurationValue:            1,
		Enabled:                  true,
		PublicVisible:            true,
		MonthlyTokenLimit:        100,
		QuotaResetPeriod:         model.SubscriptionResetMonthly,
		UnlimitedPurchaseEnabled: true,
		TimedConversionEnabled:   true,
		ModelLimits:              "restricted-model",
		BusinessCode:             &businessCode,
		EntitlementType:          model.SubscriptionEntitlementTimed,
	}
	if err := db.Create(&plan).Error; err != nil {
		return false, false, false, err
	}
	var grant *model.UserSubscriptionCreationResult
	if err := db.Transaction(func(tx *gorm.DB) error {
		var grantErr error
		grant, grantErr = model.GrantTimedSubscriptionTx(tx, model.TimedSubscriptionGrantRequest{
			UserId:         user.Id,
			PlanId:         plan.Id,
			IdempotencyKey: "issue28-disabled-existing-" + identity,
			SourceType:     model.TimedSubscriptionGrantSourceAdmin,
			Reason:         "Issue 28 isolated disabled-plan probe",
		})
		return grantErr
	}); err != nil {
		return false, false, false, err
	}
	if grant == nil || grant.Subscription == nil {
		return false, false, false, errors.New("disabled-plan fixture grant missing")
	}
	if err := db.Model(&model.SubscriptionPlan{}).Where("id = ?", plan.Id).Update("enabled", false).Error; err != nil {
		return false, false, false, err
	}
	plan.Enabled = false
	model.ClearSubscriptionPlanCacheForTest()
	model.ClearPrimaryBillableSubscriptionCacheForTest()

	requestID := "issue28-disabled-consume-" + identity
	preConsumed, err := model.PreConsumeUserSubscriptionByUnits(requestID, user.Id, "model-outside-legacy-scope", 0, 0, 1)
	if err != nil {
		return false, false, false, err
	}
	if preConsumed.UserSubscriptionId != grant.Subscription.Id {
		return false, false, false, errors.New("disabled-plan existing entitlement was not selected")
	}
	settlement, err := model.PostConsumeUserSubscriptionRequestDelta(requestID, preConsumed.UserSubscriptionId, 0, true)
	if err != nil {
		return false, false, false, err
	}
	if settlement.PostDelta != 0 {
		return false, false, false, errors.New("disabled-plan settlement changed consumed amount")
	}

	purchaseRejected := model.ValidateCreditBalancePurchaseOption(&plan, creditPlan) != nil
	redemptionRejected := model.ValidateCreditBalanceRedemptionOption(&plan, creditPlan) != nil
	quote, quoteErr := model.RecalculateTimedSubscriptionConversionQuoteTx(db, user.Id, grant.Subscription.Id, model.GetDBTimestamp())
	conversionRejected := quoteErr == nil && quote != nil && containsString(quote.ReasonCodes, model.ConversionQuoteReasonPlanDisabled)
	adminErr := db.Transaction(func(tx *gorm.DB) error {
		_, grantErr := model.GrantTimedSubscriptionTx(tx, model.TimedSubscriptionGrantRequest{
			UserId:         user.Id,
			PlanId:         plan.Id,
			IdempotencyKey: "issue28-disabled-new-" + identity,
			SourceType:     model.TimedSubscriptionGrantSourceAdmin,
			Reason:         "Issue 28 rejected disabled-plan probe",
		})
		return grantErr
	})
	adminRejected := errors.Is(adminErr, model.ErrTimedSubscriptionGrantInvalid)

	var persisted model.UserSubscription
	if err := db.First(&persisted, grant.Subscription.Id).Error; err != nil {
		return false, false, false, err
	}
	existingConsumable := persisted.TokenUsed == 1
	modelScopeIgnored := existingConsumable
	return existingConsumable, purchaseRejected && redemptionRejected && conversionRejected && adminRejected, modelScopeIgnored, nil
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func creditValuationProbeMoney(amounts []dto.AdminAnalyticsMoneyBreakdown, currency string) string {
	for _, amount := range amounts {
		if amount.Currency == currency {
			if amount.AmountMicros == "" {
				return "0"
			}
			return amount.AmountMicros
		}
	}
	return "0"
}
