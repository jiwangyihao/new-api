package service

import (
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/logger"
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
}

func codexProAdjustedSubscriptionTokens(relayInfo *relaycommon.RelayInfo, tokens int64, walletQuota int) int64 {
	if tokens <= 0 || relayInfo == nil || relayInfo.BillingSource != BillingSourceSubscription || !relayInfo.CodexProServed {
		return tokens
	}
	if relayInfo.FreeModel || relayInfo.PriceData.FreeModel {
		return 0
	}
	switch relayInfo.CodexProUnavailableReason {
	case "trial_subscription", "reward_subscription":
		return tokens
	}
	session, ok := relayInfo.Billing.(*BillingSession)
	if !ok || !session.IsDistributorTokenBilling() {
		return tokens
	}
	if walletQuota < 0 {
		return tokens
	}
	return tokens * 2
}

// SettleBilling 执行计费结算。如果 RelayInfo 上有 BillingSession 则通过 session 结算，
// 否则回退到旧的 PostConsumeQuota 路径（兼容按次计费等场景）。
func SettleBilling(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, actualQuota int) error {
	input := BillingSettleInput{WalletQuota: actualQuota}
	if session, ok := relayInfo.Billing.(*BillingSession); ok && !session.IsDistributorTokenBilling() {
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
	if input.SubscriptionTokens < 0 {
		input.SubscriptionTokens = 0
	}
	if !input.SubscriptionTokensCodexProAdjusted {
		input.SubscriptionTokens = codexProAdjustedSubscriptionTokens(relayInfo, input.SubscriptionTokens, input.WalletQuota)
		input.SubscriptionTokensCodexProAdjusted = true
	}
	apiKeyTokens := input.ApiKeyTokens
	if apiKeyTokens == 0 {
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
