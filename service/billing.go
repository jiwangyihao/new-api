package service

import (
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
	relayInfo.Billing = session
	return nil
}

// ---------------------------------------------------------------------------
// SettleBilling — 后结算辅助函数
// ---------------------------------------------------------------------------

type BillingSettleInput struct {
	WalletQuota        int
	SubscriptionTokens int64
	UsageEstimated     bool
}

// SettleBilling 执行计费结算。如果 RelayInfo 上有 BillingSession 则通过 session 结算，
// 否则回退到旧的 PostConsumeQuota 路径（兼容按次计费等场景）。
func SettleBilling(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, actualQuota int) error {
	return SettleBillingWithInput(ctx, relayInfo, BillingSettleInput{
		WalletQuota:        actualQuota,
		SubscriptionTokens: int64(actualQuota),
	})
}

func SettleBillingWithInput(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, input BillingSettleInput) error {
	if input.SubscriptionTokens < 0 {
		input.SubscriptionTokens = 0
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
				return err
			}
		} else if err := relayInfo.Billing.Settle(input.WalletQuota); err != nil {
			return err
		}

		if input.WalletQuota != 0 || input.SubscriptionTokens != 0 {
			if relayInfo.BillingSource == BillingSourceSubscription {
				checkAndSendSubscriptionQuotaNotify(relayInfo)
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
