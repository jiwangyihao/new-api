package service

import (
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/types"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
)

// ---------------------------------------------------------------------------
// BillingSession — 统一计费会话
// ---------------------------------------------------------------------------

// BillingSession 封装单次请求的预扣费/结算/退款生命周期。
// 实现 relaycommon.BillingSettler 接口。
type BillingSession struct {
	relayInfo                   *relaycommon.RelayInfo
	funding                     FundingSource
	preConsumedQuota            int   // 实际预扣额度（信任用户可能为 0）
	preConsumedSubscription     int64 // 订阅资金来源预扣 token 数；钱包路径不用
	extraReserved               int   // 发送前补充预扣的额度（订阅退款时需要单独回滚）
	realtimeSubscriptionTokens  int64
	committedSubscriptionTokens int64
	trusted                     bool // 是否命中信任额度旁路
	fundingSettled              bool // funding.Settle 已成功，资金来源已提交
	settled                     bool // Settle 全部完成
	refunded                    bool // Refund 已调用
	mu                          sync.Mutex
}

// Settle 根据实际消耗额度进行结算。
// 请求资金来源为订阅-only；token key quota 不参与请求预扣、结算或退款。
func (s *BillingSession) Settle(actualQuota int) error {
	return s.SettleWithInput(BillingSettleInput{
		WalletQuota:        actualQuota,
		SubscriptionTokens: int64(actualQuota),
	})
}

func (s *BillingSession) SettleWithInput(input BillingSettleInput) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.settled {
		return nil
	}
	if input.SubscriptionTokens < 0 {
		input.SubscriptionTokens = 0
	}
	fundingDelta := input.WalletQuota - s.preConsumedQuota
	if s.funding.Source() == BillingSourceSubscription {
		fundingDelta64 := input.SubscriptionTokens - s.preConsumedSubscription
		if fundingDelta64 > int64(^uint(0)>>1) || fundingDelta64 < -int64(^uint(0)>>1)-1 {
			return fmt.Errorf("subscription token delta out of int range: %d", fundingDelta64)
		}
		fundingDelta = int(fundingDelta64)
	}
	if fundingDelta == 0 {
		s.settled = true
		return nil
	}

	// 1) 调整资金来源（仅在尚未提交时执行，防止重复调用）。
	// 请求级计费不再调整 token key quota；token key 只作为认证凭证。
	if !s.fundingSettled {
		if err := s.funding.Settle(fundingDelta); err != nil {
			return err
		}
		s.fundingSettled = true
		s.syncRelayInfoPreservingPostDelta()
	}

	// 2) 更新 relayInfo 上的订阅 PostDelta（用于日志）。
	if s.funding.Source() == BillingSourceSubscription {
		s.relayInfo.SubscriptionPostDelta += int64(fundingDelta)
		s.syncRelayInfoToActualUsed()
	}
	s.settled = true
	return nil
}
func (s *BillingSession) SettleSubscriptionIncrement(deltaTokens int64) error {
	if deltaTokens == 0 {
		return nil
	}
	if deltaTokens < 0 || deltaTokens > int64(^uint(0)>>1) {
		return fmt.Errorf("subscription token delta out of int range: %d", deltaTokens)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.refunded {
		return nil
	}
	if s.funding.Source() != BillingSourceSubscription {
		return fmt.Errorf("subscription billing source required")
	}
	if err := s.funding.Settle(int(deltaTokens)); err != nil {
		return err
	}

	s.realtimeSubscriptionTokens += deltaTokens
	s.committedSubscriptionTokens += deltaTokens
	s.relayInfo.SubscriptionPostDelta += deltaTokens
	return nil
}

func (s *BillingSession) RealtimeSubscriptionTokens() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.realtimeSubscriptionTokens
}

func (s *BillingSession) CommitPreConsumedOnFailure() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.refunded {
		return
	}
	s.settled = true
}

// Refund 退还所有预扣费，幂等安全，异步执行。
func (s *BillingSession) Refund(c *gin.Context) {
	s.mu.Lock()
	if s.settled || s.refunded || !s.needsRefundLocked() {
		s.mu.Unlock()
		return
	}
	s.refunded = true
	s.mu.Unlock()

	logger.LogInfo(c, fmt.Sprintf("用户 %d 请求失败, 返还预扣费（funding=%s）",
		s.relayInfo.UserId,
		s.funding.Source(),
	))

	// 复制需要的值到闭包中
	extraReserved := s.extraReserved
	subscriptionId := s.relayInfo.SubscriptionId
	committedSubscriptionTokens := s.committedSubscriptionTokens
	funding := s.funding

	gopool.Go(func() {
		// 1) 退还资金来源
		if err := funding.Refund(); err != nil {
			common.SysLog("error refunding billing source: " + err.Error())
		}
		if committedSubscriptionTokens > 0 && funding.Source() == BillingSourceSubscription && subscriptionId > 0 {
			err := model.PostConsumeUserSubscriptionAmountDelta(subscriptionId, -committedSubscriptionTokens)
			if sub, ok := funding.(*SubscriptionFunding); ok && sub.DistributorTokenBilling {
				err = model.PostConsumeUserSubscriptionTokenDelta(subscriptionId, -committedSubscriptionTokens)
			}
			if err != nil {
				common.SysLog("error refunding committed subscription tokens: " + err.Error())
			}
		}
		if extraReserved > 0 && funding.Source() == BillingSourceSubscription && subscriptionId > 0 {
			err := model.PostConsumeUserSubscriptionAmountDelta(subscriptionId, -int64(extraReserved))
			if sub, ok := funding.(*SubscriptionFunding); ok && sub.DistributorTokenBilling {
				err = model.PostConsumeUserSubscriptionTokenDelta(subscriptionId, -int64(extraReserved))
			}
			if err != nil {
				common.SysLog("error refunding subscription extra reserved quota: " + err.Error())
			}
		}
	})
}

func (s *BillingSession) refundSync() {
	s.mu.Lock()
	if s.settled || s.refunded || !s.needsRefundLocked() {
		s.mu.Unlock()
		return
	}
	s.refunded = true
	extraReserved := s.extraReserved
	subscriptionId := s.relayInfo.SubscriptionId
	committedSubscriptionTokens := s.committedSubscriptionTokens
	funding := s.funding
	s.mu.Unlock()

	if err := funding.Refund(); err != nil {
		common.SysLog("error refunding billing source: " + err.Error())
	}
	if committedSubscriptionTokens > 0 && funding.Source() == BillingSourceSubscription && subscriptionId > 0 {
		err := model.PostConsumeUserSubscriptionAmountDelta(subscriptionId, -committedSubscriptionTokens)
		if sub, ok := funding.(*SubscriptionFunding); ok && sub.DistributorTokenBilling {
			err = model.PostConsumeUserSubscriptionTokenDelta(subscriptionId, -committedSubscriptionTokens)
		}
		if err != nil {
			common.SysLog("error refunding committed subscription tokens: " + err.Error())
		}
	}
	if extraReserved > 0 && funding.Source() == BillingSourceSubscription && subscriptionId > 0 {
		err := model.PostConsumeUserSubscriptionAmountDelta(subscriptionId, -int64(extraReserved))
		if sub, ok := funding.(*SubscriptionFunding); ok && sub.DistributorTokenBilling {
			err = model.PostConsumeUserSubscriptionTokenDelta(subscriptionId, -int64(extraReserved))
		}
		if err != nil {
			common.SysLog("error refunding subscription extra reserved quota: " + err.Error())
		}
	}
}

// NeedsRefund 返回是否存在需要退还的预扣状态。
func (s *BillingSession) NeedsRefund() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.needsRefundLocked()
}

func (s *BillingSession) needsRefundLocked() bool {
	if s.settled || s.refunded || s.fundingSettled {
		// fundingSettled 时资金来源已提交结算，不能再退预扣费
		return false
	}
	if wallet, ok := s.funding.(*WalletFunding); ok && wallet.consumed > 0 {
		return true
	}
	// 订阅可能在没有 token key quota 预扣时仍预扣了额度
	if sub, ok := s.funding.(*SubscriptionFunding); ok && sub.preConsumed > 0 {
		return true
	}
	return false
}

// GetPreConsumedQuota 返回实际预扣的额度。
func (s *BillingSession) GetPreConsumedQuota() int {
	return s.preConsumedQuota
}

func (s *BillingSession) IsDistributorTokenBilling() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	sub, ok := s.funding.(*SubscriptionFunding)
	return ok && sub.DistributorTokenBilling
}

func (s *BillingSession) SubscriptionConcurrencyLimit() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	sub, ok := s.funding.(*SubscriptionFunding)
	if !ok || !sub.DistributorTokenBilling {
		return 0
	}
	return sub.ConcurrencyLimit()
}

func (s *BillingSession) SubscriptionQueueCapacity() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	sub, ok := s.funding.(*SubscriptionFunding)
	if !ok || !sub.DistributorTokenBilling {
		return 0
	}
	return sub.QueueCapacity()
}

func (s *BillingSession) Reserve(targetQuota int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.settled || s.refunded || s.trusted || targetQuota <= s.preConsumedQuota {
		return nil
	}

	delta := targetQuota - s.preConsumedQuota
	if delta <= 0 {
		return nil
	}

	if err := s.reserveFunding(delta); err != nil {
		return err
	}

	if sub, ok := s.funding.(*SubscriptionFunding); ok {
		sub.preConsumed += int64(delta)
		sub.TokenUsedAfter += int64(delta)
		sub.AmountUsedAfter += int64(delta)
		if sub.TokenLimit > 0 {
			remaining := sub.TokenLimit - sub.TokenUsedAfter
			if remaining < 0 {
				remaining = 0
			}
			sub.TokenRemaining = remaining
		}
		s.preConsumedSubscription += int64(delta)
	}
	s.preConsumedQuota += delta
	s.extraReserved += delta
	s.syncRelayInfoPreservingPostDelta()
	return nil
}

func newSubscriptionBillingError(errMsg string) *types.NewAPIError {
	openAIError := types.OpenAIError{
		Message: "active subscription is required",
		Type:    "insufficient_quota",
		Code:    string(types.ErrorCodeSubscriptionRequired),
	}
	if strings.Contains(errMsg, "subscription token quota insufficient") {
		openAIError.Message = "subscription token exhausted"
		openAIError.Code = string(types.ErrorCodeSubscriptionTokenExhausted)
	}
	if errMsg != "" {
		openAIError.Message = openAIError.Message + ": " + errMsg
	}
	return types.WithOpenAIError(openAIError, http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
}

// ---------------------------------------------------------------------------
// PreConsume — 统一预扣费入口（含信任额度旁路）
// ---------------------------------------------------------------------------

// preConsume 执行预扣费：信任检查 -> 资金来源预扣。
func (s *BillingSession) preConsume(c *gin.Context, quota int) *types.NewAPIError {
	effectiveQuota := quota

	// ---- 信任额度旁路 ----
	if s.shouldTrust(c) {
		s.trusted = true
		effectiveQuota = 0
		logger.LogInfo(c, fmt.Sprintf("用户 %d 额度充足, 信任且不需要预扣费 (funding=%s)", s.relayInfo.UserId, s.funding.Source()))
	} else if effectiveQuota > 0 {
		logger.LogInfo(c, fmt.Sprintf("用户 %d 需要预扣费 %s (funding=%s)", s.relayInfo.UserId, logger.FormatQuota(effectiveQuota), s.funding.Source()))
	}

	// ---- 1) 预扣资金来源 ----
	// 请求级计费强制走订阅，token key quota 不再作为资金来源或预扣检查。
	if err := s.funding.PreConsume(effectiveQuota); err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "no active subscription") || strings.Contains(errMsg, "subscription quota insufficient") || strings.Contains(errMsg, "subscription token quota insufficient") {
			return newSubscriptionBillingError(errMsg)
		}
		return types.NewError(err, types.ErrorCodeUpdateDataError, types.ErrOptionWithSkipRetry())
	}

	s.preConsumedQuota = effectiveQuota
	if sub, ok := s.funding.(*SubscriptionFunding); ok {
		s.preConsumedSubscription = sub.preConsumed
	} else {
		s.preConsumedSubscription = int64(effectiveQuota)
	}

	// ---- 同步 RelayInfo 兼容字段 ----
	s.syncRelayInfo()

	return nil
}

func (s *BillingSession) reserveFunding(delta int) error {
	switch funding := s.funding.(type) {
	case *WalletFunding:
		if err := model.DecreaseUserQuota(funding.userId, delta, false); err != nil {
			return types.NewError(err, types.ErrorCodeUpdateDataError, types.ErrOptionWithSkipRetry())
		}
		funding.consumed += delta
		return nil
	case *SubscriptionFunding:
		err := model.PostConsumeUserSubscriptionAmountDelta(funding.subscriptionId, int64(delta))
		if funding.DistributorTokenBilling {
			err = model.PostConsumeUserSubscriptionTokenDelta(funding.subscriptionId, int64(delta))
		}
		if err != nil {
			return types.NewErrorWithStatusCode(
				fmt.Errorf("订阅额度不足或未配置订阅: %s", err.Error()),
				types.ErrorCodeInsufficientUserQuota,
				http.StatusForbidden,
				types.ErrOptionWithSkipRetry(),
				types.ErrOptionWithNoRecordErrorLog(),
			)
		}
		return nil
	default:
		return types.NewError(fmt.Errorf("unsupported funding source: %s", s.funding.Source()), types.ErrorCodeUpdateDataError, types.ErrOptionWithSkipRetry())
	}
}

// shouldTrust 统一信任额度检查，适用于钱包和订阅。
func (s *BillingSession) shouldTrust(c *gin.Context) bool {
	// 异步任务（ForcePreConsume=true）必须预扣全额，不允许信任旁路
	if s.relayInfo.ForcePreConsume {
		return false
	}

	trustQuota := common.GetTrustQuota()
	if trustQuota <= 0 {
		return false
	}

	// 检查令牌是否充足
	tokenTrusted := s.relayInfo.TokenUnlimited
	if !tokenTrusted {
		tokenQuota := c.GetInt("token_quota")
		tokenTrusted = tokenQuota > trustQuota
	}
	if !tokenTrusted {
		return false
	}

	if s.funding.Source() == BillingSourceSubscription {
		// 订阅不能启用信任旁路。原因：
		// 1. PreConsumeUserSubscription 要求 amount>0 来创建预扣记录并锁定订阅
		// 2. SubscriptionFunding.PreConsume 忽略参数，始终用 s.amount 预扣
		// 3. 若信任旁路将 effectiveQuota 设为 0，会导致 preConsumedQuota 与实际订阅预扣不一致
		return false
	}
	return false
}

// syncRelayInfo 将 BillingSession 的状态同步到 RelayInfo 的兼容字段上。
func (s *BillingSession) syncRelayInfo() {
	info := s.relayInfo
	info.FinalPreConsumedQuota = s.preConsumedQuota
	info.BillingSource = s.funding.Source()

	if sub, ok := s.funding.(*SubscriptionFunding); ok {
		info.SubscriptionId = sub.subscriptionId
		info.SubscriptionPreConsumed = sub.preConsumed
		info.SubscriptionPostDelta = 0
		if sub.DistributorTokenBilling {
			info.SubscriptionAmountTotal = sub.TokenLimit
			info.SubscriptionAmountUsedAfterPreConsume = sub.TokenUsedAfter
			info.SubscriptionTokenLimit = sub.TokenLimit
			info.SubscriptionTokenUsedAfterPreConsume = sub.TokenUsedAfter
			info.SubscriptionTokenUnlimited = sub.TokenLimit == 0
			info.SubscriptionDistributorTokenBilling = true
		} else {
			info.SubscriptionAmountTotal = sub.AmountTotal
			info.SubscriptionAmountUsedAfterPreConsume = sub.AmountUsedAfter
			info.SubscriptionTokenLimit = 0
			info.SubscriptionTokenUsedAfterPreConsume = 0
			info.SubscriptionTokenUnlimited = false
			info.SubscriptionDistributorTokenBilling = false
		}
		info.SubscriptionPlanId = sub.PlanId
		info.SubscriptionPlanTitle = sub.PlanTitle
	} else {
		info.SubscriptionId = 0
		info.SubscriptionPreConsumed = 0
	}
}

func (s *BillingSession) syncRelayInfoPreservingPostDelta() {
	postDelta := s.relayInfo.SubscriptionPostDelta
	s.syncRelayInfo()
	s.relayInfo.SubscriptionPostDelta = postDelta
}

func (s *BillingSession) syncRelayInfoToActualUsed() {
	postDelta := s.relayInfo.SubscriptionPostDelta
	s.syncRelayInfo()
	if sub, ok := s.funding.(*SubscriptionFunding); ok {
		if sub.DistributorTokenBilling {
			s.relayInfo.SubscriptionAmountUsedAfterPreConsume -= postDelta
			s.relayInfo.SubscriptionTokenUsedAfterPreConsume -= postDelta
			if s.relayInfo.SubscriptionAmountUsedAfterPreConsume < 0 {
				s.relayInfo.SubscriptionAmountUsedAfterPreConsume = 0
			}
			if s.relayInfo.SubscriptionTokenUsedAfterPreConsume < 0 {
				s.relayInfo.SubscriptionTokenUsedAfterPreConsume = 0
			}
		} else {
			s.relayInfo.SubscriptionAmountUsedAfterPreConsume -= postDelta
			if s.relayInfo.SubscriptionAmountUsedAfterPreConsume < 0 {
				s.relayInfo.SubscriptionAmountUsedAfterPreConsume = 0
			}
		}
	}
	s.relayInfo.SubscriptionPostDelta = postDelta
}

func clearRelayBillingState(info *relaycommon.RelayInfo) {
	if info == nil {
		return
	}
	info.Billing = nil
	info.BillingSource = ""
	info.SubscriptionId = 0
	info.SubscriptionPreConsumed = 0
	info.FinalPreConsumedQuota = 0
	info.SubscriptionPostDelta = 0
	info.SubscriptionAmountTotal = 0
	info.SubscriptionAmountUsedAfterPreConsume = 0
	info.SubscriptionTokenLimit = 0
	info.SubscriptionTokenUsedAfterPreConsume = 0
	info.SubscriptionTokenUnlimited = false
	info.SubscriptionDistributorTokenBilling = false
	info.SubscriptionPlanId = 0
	info.SubscriptionPlanTitle = ""
}

func distributorSubscriptionEligibleForBilling(relayInfo *relaycommon.RelayInfo) bool {
	if relayInfo == nil {
		return false
	}
	if distributorTokenBillingEligibleForText(relayInfo) {
		return true
	}
	switch relayInfo.RelayFormat {
	case types.RelayFormatClaude:
		return true
	case types.RelayFormatGemini:
		return relayInfo.RelayMode == relayconstant.RelayModeGemini && !nativeGeminiEmbeddingRequest(relayInfo)
	default:
		return false
	}
}

func distributorSubscriptionRelayError(relayInfo *relaycommon.RelayInfo) error {
	if relayInfo != nil && relayInfo.RelayFormat == types.RelayFormatTask {
		return fmt.Errorf("非文本异步任务不支持分销订阅扣费")
	}
	return fmt.Errorf("该接口不支持分销订阅扣费")
}

// ---------------------------------------------------------------------------
// NewBillingSession 工厂 — 创建订阅-only 会话
// ---------------------------------------------------------------------------

// NewBillingSession 创建订阅-only BillingSession。
// billing_preference 只保留为用户设置兼容字段；请求计费不再 fallback 到钱包或 token key quota。
func NewBillingSession(c *gin.Context, relayInfo *relaycommon.RelayInfo, preConsumedQuota int) (*BillingSession, *types.NewAPIError) {
	if relayInfo == nil {
		return nil, types.NewError(fmt.Errorf("relayInfo is nil"), types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
	}
	if !distributorSubscriptionEligibleForBilling(relayInfo) {
		clearRelayBillingState(relayInfo)
		return nil, types.NewOpenAIError(distributorSubscriptionRelayError(relayInfo), types.ErrorCodeSubscriptionRequired, http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
	}

	trySubscription := func() (*BillingSession, *types.NewAPIError) {
		distributorConsume := int64(relayInfo.GetEstimatePromptTokens())
		if distributorConsume <= 0 {
			distributorConsume = int64(preConsumedQuota)
		}
		if distributorConsume <= 0 {
			distributorConsume = 1
		}
		legacyConsume := int64(preConsumedQuota)
		session := &BillingSession{
			relayInfo: relayInfo,
			funding: &SubscriptionFunding{
				requestId:         relayInfo.RequestId,
				userId:            relayInfo.UserId,
				modelName:         relaycommon.ResolveBillingModelName(relayInfo),
				amount:            legacyConsume,
				distributorAmount: distributorConsume,
			},
		}
		// preConsume 入参保留 wallet quota 口径兼容字段；SubscriptionFunding 使用构造时的订阅单位预扣。
		if apiErr := session.preConsume(c, preConsumedQuota); apiErr != nil {
			return nil, apiErr
		}
		return session, nil
	}

	session, apiErr := trySubscription()
	if apiErr != nil {
		return nil, apiErr
	}

	return session, nil
}
