package service

import (
	"errors"
	"time"

	"github.com/QuantumNous/new-api/model"
)

// ---------------------------------------------------------------------------
// FundingSource — 资金来源接口（钱包 or 订阅）
// ---------------------------------------------------------------------------

// FundingSource 抽象了预扣费的资金来源。
type FundingSource interface {
	// Source 返回资金来源标识："wallet" 或 "subscription"
	Source() string
	// PreConsume 从该资金来源预扣 amount 额度
	PreConsume(amount int) error
	// Settle 根据差额调整资金来源（正数补扣，负数退还）
	Settle(delta int) error
	// Refund 退还所有预扣费
	Refund() error
}

var ErrLegacyWalletFundingDisabled = errors.New("legacy wallet funding is disabled; use subscription billing")

// ---------------------------------------------------------------------------
// WalletFunding — 钱包资金来源实现。
// 保留给余额/旧兼容路径；relay 请求计费由 NewBillingSession 强制使用 SubscriptionFunding。
// ---------------------------------------------------------------------------

type WalletFunding struct {
	userId   int
	consumed int // 实际预扣的用户额度
}

func (w *WalletFunding) Source() string { return BillingSourceWallet }

func (w *WalletFunding) PreConsume(amount int) error {
	if amount <= 0 {
		return nil
	}
	return ErrLegacyWalletFundingDisabled
}

func (w *WalletFunding) Settle(delta int) error {
	if delta == 0 {
		return nil
	}
	return ErrLegacyWalletFundingDisabled
}

func (w *WalletFunding) Refund() error {
	if w.consumed <= 0 {
		return nil
	}
	return ErrLegacyWalletFundingDisabled
}

// ---------------------------------------------------------------------------
// SubscriptionFunding — 订阅资金来源实现
// ---------------------------------------------------------------------------

type SubscriptionFunding struct {
	requestId         string
	userId            int
	modelName         string
	amount            int64 // 预扣的订阅 token 或 legacy quota 数
	distributorAmount int64
	subscriptionId    int
	preConsumed       int64
	// 以下字段在 PreConsume 成功后填充，供 RelayInfo 同步使用
	AmountTotal                int64
	AmountUsedAfter            int64
	TokenLimit                 int64
	TokenUsedAfter             int64
	TokenRemaining             int64
	DistributorTokenBilling    bool
	PlanId                     int
	EntitlementType            string
	PlanIsTrial                bool
	PlanTitle                  string
	PlanPriceAmount            float64
	PlanInviteTrial            bool
	SubscriptionSource         string
	SubscriptionGrantReason    string
	SubscriptionStatus         string
	SubscriptionEndTime        int64
	SubscriptionTokenRemaining int64
	concurrencyLimit           int
	queueCapacity              int
}

func (s *SubscriptionFunding) ConcurrencyLimit() int {
	if s == nil {
		return 0
	}
	if s.DistributorTokenBilling {
		return s.concurrencyLimit
	}
	return 0
}

func (s *SubscriptionFunding) QueueCapacity() int {
	if s == nil || !s.DistributorTokenBilling {
		return 0
	}
	return s.queueCapacity
}
func (s *SubscriptionFunding) Source() string { return BillingSourceSubscription }

func (s *SubscriptionFunding) PreConsume(_ int) error {
	// amount 参数被忽略，使用构造时设置的 legacy/distributor 双口径预扣值。
	res, err := model.PreConsumeUserSubscriptionByUnits(s.requestId, s.userId, s.modelName, 0, s.amount, s.distributorAmount)
	if err != nil {
		return err
	}
	s.subscriptionId = res.UserSubscriptionId
	s.preConsumed = res.PreConsumed
	s.AmountTotal = res.AmountTotal
	s.AmountUsedAfter = res.AmountUsedAfter
	s.TokenLimit = res.TokenLimit
	s.TokenUsedAfter = res.TokenUsedAfter
	s.DistributorTokenBilling = res.DistributorTokenBilling
	s.concurrencyLimit = res.ConcurrencyLimit
	s.queueCapacity = res.QueueCapacity
	s.TokenRemaining = res.TokenRemaining
	s.PlanId = res.PlanId
	s.EntitlementType = res.EntitlementType
	s.PlanIsTrial = res.PlanIsTrial
	s.PlanTitle = res.PlanTitle
	s.PlanPriceAmount = res.PlanPriceAmount
	s.PlanInviteTrial = res.PlanInviteTrial
	s.SubscriptionSource = res.SubscriptionSource
	s.SubscriptionGrantReason = res.SubscriptionGrantReason
	s.SubscriptionStatus = res.SubscriptionStatus
	s.SubscriptionEndTime = res.SubscriptionEndTime
	s.SubscriptionTokenRemaining = res.SubscriptionTokenRemaining
	return nil
}

func (s *SubscriptionFunding) Settle(delta int) error {
	if delta == 0 {
		return nil
	}
	if s.DistributorTokenBilling {
		if err := model.PostConsumeUserSubscriptionTokenDelta(s.subscriptionId, int64(delta)); err != nil {
			return err
		}
	} else if err := model.PostConsumeUserSubscriptionAmountDelta(s.subscriptionId, int64(delta)); err != nil {
		return err
	}
	if s.DistributorTokenBilling {
		s.TokenUsedAfter += int64(delta)
		if s.TokenUsedAfter < 0 {
			s.TokenUsedAfter = 0
		}
		if s.TokenLimit > 0 {
			s.TokenRemaining = s.TokenLimit - s.TokenUsedAfter
			if s.TokenRemaining < 0 {
				s.TokenRemaining = 0
			}
		}
	} else {
		s.AmountUsedAfter += int64(delta)
		if s.AmountUsedAfter < 0 {
			s.AmountUsedAfter = 0
		}
	}
	return nil
}

func (s *SubscriptionFunding) Refund() error {
	if s.preConsumed <= 0 {
		return nil
	}
	return refundWithRetry(func() error {
		return model.RefundSubscriptionPreConsume(s.requestId)
	})
}

// refundWithRetry 尝试多次执行退款操作以提高成功率，只能用于基于事务的退款函数！！！！！！
// try to refund with retries, only for refund functions based on transactions!!!
func refundWithRetry(fn func() error) error {
	if fn == nil {
		return nil
	}
	const maxAttempts = 3
	var lastErr error
	for i := 0; i < maxAttempts; i++ {
		if err := fn(); err == nil {
			return nil
		} else {
			lastErr = err
		}
		if i < maxAttempts-1 {
			time.Sleep(time.Duration(200*(i+1)) * time.Millisecond)
		}
	}
	return lastErr
}
