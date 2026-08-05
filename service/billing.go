package service

import (
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/pkg/creditbilling"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

const (
	BillingSourceWallet       = "wallet"
	BillingSourceSubscription = "subscription"
)

// PreConsumeBilling 根据用户计费偏好创建 BillingSession 并执行预扣费。
// 会话存储在 relayInfo.Billing 上，供后续 Settle / Refund 使用。
func PreConsumeBilling(c *gin.Context, preConsumedQuota int, relayInfo *relaycommon.RelayInfo) *types.NewAPIError {
	session, apiErr := NewBillingSession(c, relayInfo, preConsumedQuota)
	if apiErr != nil {
		return apiErr
	}
	if session != nil {
		relayInfo.Billing = session
	}
	return nil
}

// ---------------------------------------------------------------------------
// SettleBilling — 后结算辅助函数
// ---------------------------------------------------------------------------

type BillingSettleInput struct {
	WalletQuota                        int
	SubscriptionTokens                 int64
	UsageEstimated                     bool
	SubscriptionTokensCodexProAdjusted bool
	ApiKeyTokens                       int64
	ResponseStarted                    bool
	SkipDefaultApiKeyTokens            bool
	UseCreditBilling                   bool
	HasTrustedUsage                    bool
	RawMeteredTokens                   int64
	RawMeteredTokensSet                bool
}

func codexProAdjustedSubscriptionTokens(relayInfo *relaycommon.RelayInfo, tokens int64, walletQuota int) int64 {
	if tokens <= 0 || relayInfo == nil || relayInfo.BillingSource != BillingSourceSubscription {
		return tokens
	}
	if relayInfo.FreeModel || relayInfo.PriceData.FreeModel {
		return 0
	}
	return tokens
}

const (
	BillingMultiplierSourceNormal           = "normal"
	BillingMultiplierSourceFreeModel        = "free_model"
	BillingMultiplierSourceUsageUnavailable = "usage_unavailable"
)

func creditBillingInputFromRelayInfo(relayInfo *relaycommon.RelayInfo, hasTrustedUsage bool, rawMeteredTokens int64) creditbilling.CreditBillingInput {
	if rawMeteredTokens < 0 {
		rawMeteredTokens = 0
	}
	dynamicMultiplier := float64(1)
	dynamicSource := creditbilling.DynamicMultiplierDefaultSource
	if relayInfo != nil && relayInfo.DynamicBillingMultiplierEnabled {
		dynamicMultiplier = relayInfo.FrozenDynamicBillingMultiplier()
		dynamicSource = relayInfo.FrozenDynamicBillingMultiplierSource()
	}
	input := creditbilling.CreditBillingInput{
		Chargeable:                     false,
		HasTrustedUsage:                hasTrustedUsage,
		RawMeteredTokens:               rawMeteredTokens,
		CreditBillingMode:              creditbilling.ModeUsageTokens,
		FixedRequestCredits:            0,
		ChannelTokenBillingMultiplier:  1,
		DynamicBillingMultiplier:       dynamicMultiplier,
		DynamicBillingMultiplierSource: dynamicSource,
	}
	if relayInfo == nil {
		return input
	}
	input.Chargeable = !(relayInfo.FreeModel || relayInfo.PriceData.FreeModel)
	input.CreditBillingMode = relayInfo.FrozenCreditBillingMode()
	input.FixedRequestCredits = relayInfo.FixedRequestCredits
	input.ChannelTokenBillingMultiplier = relayInfo.FrozenChannelTokenBillingMultiplier()
	return input
}

func calculateCreditBillingResult(relayInfo *relaycommon.RelayInfo, hasTrustedUsage bool, rawMeteredTokens int64) (creditbilling.CreditBillingResult, error) {
	return creditbilling.Calculate(creditBillingInputFromRelayInfo(relayInfo, hasTrustedUsage, rawMeteredTokens))
}

func applyCreditBillingResultToRelayInfo(relayInfo *relaycommon.RelayInfo, result creditbilling.CreditBillingResult, rawMeteredTokens int64) {
	if relayInfo == nil {
		return
	}
	if rawMeteredTokens < 0 || !result.HasTrustedUsage {
		rawMeteredTokens = 0
	}
	relayInfo.HasTrustedUsage = result.HasTrustedUsage
	relayInfo.RawMeteredTokens = rawMeteredTokens
	relayInfo.CreditBillingMode = result.CreditBillingMode
	relayInfo.CreditBillingBaseCredits = result.BaseCredits
	relayInfo.CreditBillingZeroReason = result.ZeroReason
	relayInfo.ChannelBillableTokens = result.BaseCredits
	relayInfo.ApiKeyBillableTokens = result.APIKeyCredits
	relayInfo.SubscriptionBillableTokens = result.SubscriptionCredits
	if result.DynamicBillingMultiplierSource != "" && relayInfo.DynamicBillingMultiplierEnabled {
		relayInfo.DynamicBillingMultiplierSource = result.DynamicBillingMultiplierSource
	}
}

func int64ToIntCredit(value int64) (int, error) {
	if value > int64(^uint(0)>>1) || value < -int64(^uint(0)>>1)-1 {
		return 0, fmt.Errorf("credit billing result out of int range: %d", value)
	}
	return int(value), nil
}

func NewAPIBillingFromRelayInfo(relayInfo *relaycommon.RelayInfo) dto.NewAPIBilling {
	if relayInfo == nil {
		return dto.NewAPIBilling{BillingMultiplierSource: BillingMultiplierSourceUsageUnavailable}
	}
	meteredTokens := relayInfo.RawMeteredTokens
	if meteredTokens < 0 {
		meteredTokens = 0
	}
	billableTokens := relayInfo.SubscriptionBillableTokens
	if billableTokens == 0 && relayInfo.BillingSource != BillingSourceSubscription {
		billableTokens = relayInfo.ApiKeyBillableTokens
	}
	if billableTokens < 0 {
		billableTokens = 0
	}

	multiplier := 0.0
	source := BillingMultiplierSourceUsageUnavailable
	if relayInfo.FreeModel || relayInfo.PriceData.FreeModel {
		billableTokens = 0
		source = BillingMultiplierSourceFreeModel
	} else if relayInfo.HasTrustedUsage {
		multiplier = relayInfo.FrozenDynamicBillingMultiplier()
		source = relayInfo.FrozenDynamicBillingMultiplierSource()
	}

	return dto.NewAPIBilling{
		MeteredTokens:           meteredTokens,
		BillableTokens:          billableTokens,
		BillingMultiplier:       multiplier,
		BillingMultiplierSource: source,
		CodexProRequested:       relayInfo.CodexProRequestSent,
		CodexProServed:          relayInfo.CodexProServed,
	}
}

func NewAPIBillingFromUsage(relayInfo *relaycommon.RelayInfo, usage *dto.Usage) *dto.NewAPIBilling {
	if usage == nil {
		return nil
	}
	meteredTokens := SubscriptionMeteredTokens(usage)
	result, err := calculateCreditBillingResult(relayInfo, true, meteredTokens)
	if err != nil {
		return nil
	}
	if relayInfo != nil {
		applyCreditBillingResultToRelayInfo(relayInfo, result, meteredTokens)
	}
	billing := dto.NewAPIBilling{
		MeteredTokens:           meteredTokens,
		BillableTokens:          result.APIKeyCredits,
		BillingMultiplier:       result.DynamicBillingMultiplier,
		BillingMultiplierSource: result.DynamicBillingMultiplierSource,
	}
	if relayInfo != nil {
		billing.CodexProRequested = relayInfo.CodexProRequestSent
		billing.CodexProServed = relayInfo.CodexProServed
		if relayInfo.FreeModel || relayInfo.PriceData.FreeModel {
			billing.BillableTokens = 0
			billing.BillingMultiplier = 0
			billing.BillingMultiplierSource = BillingMultiplierSourceFreeModel
		}
	}
	return &billing
}

func SeedNewAPIBillingRelayInfo(relayInfo *relaycommon.RelayInfo, billing dto.NewAPIBilling) {
	if relayInfo == nil {
		return
	}
	result, err := calculateCreditBillingResult(relayInfo, true, billing.MeteredTokens)
	if err != nil {
		relayInfo.HasTrustedUsage = true
		relayInfo.RawMeteredTokens = billing.MeteredTokens
		relayInfo.ChannelBillableTokens = billing.BillableTokens
		relayInfo.ApiKeyBillableTokens = billing.BillableTokens
		relayInfo.SubscriptionBillableTokens = billing.BillableTokens
		return
	}
	applyCreditBillingResultToRelayInfo(relayInfo, result, billing.MeteredTokens)
}

// SettleBilling 执行计费结算。如果 RelayInfo 上有 BillingSession 则通过 session 结算，
// 否则回退到旧的 PostConsumeQuota 路径（兼容按次计费等场景）。
func SettleBilling(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, actualQuota int) error {
	input := BillingSettleInput{WalletQuota: actualQuota}
	if session, ok := relayInfo.Billing.(*BillingSession); ok && (!session.IsDistributorTokenBilling() || session.UsesCreditRequestTarget()) {
		input.SubscriptionTokens = int64(actualQuota)
	}
	return SettleBillingWithInput(ctx, relayInfo, input)
}

func PostSettleErrorToOpenAIError(relayInfo *relaycommon.RelayInfo, err error) *types.NewAPIError {
	if err == nil {
		return nil
	}
	var apiErr *types.NewAPIError
	if errors.As(err, &apiErr) && apiErr.GetErrorCode() == types.ErrorCodeAPIKeyTokenLimitExhausted {
		return apiErr
	}
	if relayInfo != nil && relayInfo.Billing != nil {
		relayInfo.Billing.CommitPreConsumedOnFailure()
	}
	return types.NewOpenAIError(err, types.ErrorCodeSubscriptionTokenExhausted, 403, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
}

func ResponseAlreadyWritten(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, explicit bool) bool {
	relayStarted := relayInfo != nil && relayInfo.HasSendResponse()
	ginStarted := ctx != nil && ctx.Writer != nil && ctx.Writer.Written()
	return explicit || relayStarted || ginStarted
}

func ShouldAuditTokenLimitSettle(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, explicit bool) bool {
	return ResponseAlreadyWritten(ctx, relayInfo, explicit) || (relayInfo != nil && relayInfo.IsStream)
}

func SettleBillingWithInput(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, input BillingSettleInput) error {
	if relayInfo == nil {
		return nil
	}
	if input.UseCreditBilling {
		rawMeteredTokens := input.RawMeteredTokens
		if !input.RawMeteredTokensSet && rawMeteredTokens == 0 && input.HasTrustedUsage {
			rawMeteredTokens = relayInfo.RawMeteredTokens
		}
		result, err := calculateCreditBillingResult(relayInfo, input.HasTrustedUsage, rawMeteredTokens)
		if err != nil {
			return err
		}
		applyCreditBillingResultToRelayInfo(relayInfo, result, rawMeteredTokens)
		walletQuota, err := int64ToIntCredit(result.SubscriptionCredits)
		if err != nil {
			return err
		}
		input.WalletQuota = walletQuota
		input.SubscriptionTokens = result.SubscriptionCredits
		if !input.SkipDefaultApiKeyTokens {
			input.ApiKeyTokens = result.APIKeyCredits
		}
		input.SkipDefaultApiKeyTokens = true
		input.SubscriptionTokensCodexProAdjusted = true
	} else if !input.SubscriptionTokensCodexProAdjusted {
		input.SubscriptionTokens = codexProAdjustedSubscriptionTokens(relayInfo, input.SubscriptionTokens, input.WalletQuota)
		input.SubscriptionTokensCodexProAdjusted = true
	}
	apiKeyTokens := input.ApiKeyTokens
	if apiKeyTokens == 0 && !input.SkipDefaultApiKeyTokens {
		apiKeyTokens = input.SubscriptionTokens
	}
	auditSettle := ShouldAuditTokenLimitSettle(ctx, relayInfo, input.ResponseStarted)
	settleTokenLimit := relayInfo.TokenLimit != nil && (apiKeyTokens > 0 || relayInfo.TokenLimit.PreConsumedTokens() > 0)
	if settleTokenLimit {
		if auditSettle {
			if err := relayInfo.TokenLimit.SettleForAudit(apiKeyTokens, "api_key_token_limit_settle_failed"); err != nil {
				return err
			}
		} else if err := relayInfo.TokenLimit.Settle(apiKeyTokens); err != nil {
			if isTokenLimitExceededError(err) {
				relayInfo.TokenLimit.Refund(string(types.ErrorCodeAPIKeyTokenLimitExhausted))
				RefundBillingAfterTokenLimitReject(relayInfo.Billing)
				return newAPIKeyTokenLimitError(err)
			}
			return err
		}
	}
	if relayInfo.Billing != nil {
		preConsumed := relayInfo.Billing.GetPreConsumedQuota()
		walletDelta := input.WalletQuota - preConsumed

		if walletDelta > 0 {
			logger.LogInfo(ctx, fmt.Sprintf("预扣费后补扣费：%s（实际消耗：%s，预扣费：%s）",
				logger.FormatQuota(walletDelta),
				logger.FormatQuota(input.WalletQuota),
				logger.FormatQuota(preConsumed),
			))
		} else if walletDelta < 0 {
			logger.LogInfo(ctx, fmt.Sprintf("预扣费后返还扣费：%s（实际消耗：%s，预扣费：%s）",
				logger.FormatQuota(-walletDelta),
				logger.FormatQuota(input.WalletQuota),
				logger.FormatQuota(preConsumed),
			))
		} else {
			logger.LogInfo(ctx, fmt.Sprintf("预扣费与实际消耗一致，无需调整：%s（按次计费）",
				logger.FormatQuota(input.WalletQuota),
			))
		}

		if session, ok := relayInfo.Billing.(*BillingSession); ok {
			if err := session.SettleWithInput(input); err != nil {
				if relayInfo.TokenLimit != nil && !auditSettle {
					relayInfo.TokenLimit.Refund(errorCodeForRefund(err))
				}
				return err
			}
		} else if err := relayInfo.Billing.Settle(input.WalletQuota); err != nil {
			if relayInfo.TokenLimit != nil && !auditSettle {
				relayInfo.TokenLimit.Refund(errorCodeForRefund(err))
			}
			return err
		}

		if input.WalletQuota != 0 || input.SubscriptionTokens != 0 {
			if relayInfo.BillingSource == BillingSourceSubscription {
				distributorTokenBilling := false
				if session, ok := relayInfo.Billing.(*BillingSession); ok {
					distributorTokenBilling = session.IsDistributorTokenBilling()
				}
				checkAndSendSubscriptionQuotaNotify(relayInfo, distributorTokenBilling)
			} else {
				checkAndSendQuotaNotify(relayInfo, walletDelta, preConsumed)
			}
		}
		return nil
	}

	quotaDelta := input.WalletQuota - relayInfo.FinalPreConsumedQuota
	if quotaDelta != 0 {
		return PostConsumeQuota(relayInfo, quotaDelta, relayInfo.FinalPreConsumedQuota, true)
	}
	return nil
}

func errorCodeForRefund(err error) string {
	var apiErr *types.NewAPIError
	if errors.As(err, &apiErr) {
		return string(apiErr.GetErrorCode())
	}
	if err != nil {
		return err.Error()
	}
	return "settle_failed"
}
