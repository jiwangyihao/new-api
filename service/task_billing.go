package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
)

// LogTaskConsumption 记录任务消费日志和统计信息（仅记录，不涉及实际扣费）。
// 实际扣费已由 BillingSession（PreConsumeBilling + SettleBilling）完成。
func LogTaskConsumption(c *gin.Context, info *relaycommon.RelayInfo) {
	tokenName := c.GetString("token_name")
	logContent := fmt.Sprintf("操作 %s", info.Action)
	// 支持任务仅按次计费
	if common.StringsContains(constant.TaskPricePatches, info.OriginModelName) {
		logContent = fmt.Sprintf("%s，按次计费", logContent)
	} else {
		if len(info.PriceData.OtherRatios) > 0 {
			var contents []string
			for key, ra := range info.PriceData.OtherRatios {
				if 1.0 != ra {
					contents = append(contents, fmt.Sprintf("%s: %.2f", key, ra))
				}
			}
			if len(contents) > 0 {
				logContent = fmt.Sprintf("%s, 计算参数：%s", logContent, strings.Join(contents, ", "))
			}
		}
	}
	other := make(map[string]interface{})
	other["is_task"] = true
	other["request_path"] = c.Request.URL.Path
	other["model_price"] = info.PriceData.ModelPrice
	if info.PriceData.ModelRatio > 0 {
		other["model_ratio"] = info.PriceData.ModelRatio
	}
	if info.IsModelMapped {
		other["is_model_mapped"] = true
		other["upstream_model_name"] = info.UpstreamModelName
	}
	attachQuotaSaturation(other, info)
	model.RecordConsumeLog(c, info.UserId, model.RecordConsumeLogParams{
		ChannelId: info.ChannelId,
		ModelName: info.OriginModelName,
		TokenName: tokenName,
		Quota:     info.PriceData.Quota,
		Content:   logContent,
		TokenId:   info.TokenId,
		Other:     other,
	})
	model.UpdateUserUsedQuotaAndRequestCount(info.UserId, info.PriceData.Quota)
	model.UpdateChannelUsedQuota(info.ChannelId, info.PriceData.Quota)
}

// ---------------------------------------------------------------------------
// 异步任务计费辅助函数
// ---------------------------------------------------------------------------

// taskIsSubscription 判断任务是否通过订阅计费。
func taskIsSubscription(task *model.Task) bool {
	return task.PrivateData.BillingSource == BillingSourceSubscription && task.PrivateData.SubscriptionId > 0
}

var ErrTaskSubscriptionRequestIdentityUnavailable = errors.New("persisted task identity is required for subscription settlement")

func taskSubscriptionRequestID(task *model.Task) (string, error) {
	if task == nil {
		return "", ErrTaskSubscriptionRequestIdentityUnavailable
	}
	if requestID := strings.TrimSpace(task.PrivateData.SubscriptionRequestId); requestID != "" {
		return requestID, nil
	}
	if task.ID <= 0 {
		return "", ErrTaskSubscriptionRequestIdentityUnavailable
	}
	return fmt.Sprintf("legacy-task:%d", task.ID), nil
}

func loadTaskSubscription(task *model.Task) (*model.UserSubscription, error) {
	if !taskIsSubscription(task) {
		return nil, errors.New("task subscription funding is required")
	}
	var sub model.UserSubscription
	if err := model.DB.Where("id = ?", task.PrivateData.SubscriptionId).First(&sub).Error; err != nil {
		return nil, err
	}
	return &sub, nil
}

func taskUsesDistributorSubscription(sub *model.UserSubscription) (bool, error) {
	if sub == nil {
		return false, errors.New("task subscription is required")
	}
	if sub.TokenLimit > 0 || sub.ConcurrencyLimit > 0 || sub.GrantReason == "trial_code" || sub.GrantReason == "invite_trial" || sub.GrantReason == "monthly_invite_entitlement" {
		return true, nil
	}
	if sub.PlanId <= 0 {
		return false, nil
	}
	var plan model.SubscriptionPlan
	if err := model.DB.Select("business_code").Where("id = ?", sub.PlanId).First(&plan).Error; err != nil {
		return false, err
	}
	return plan.BusinessCode != nil && strings.TrimSpace(*plan.BusinessCode) != "", nil
}

// taskAdjustFunding 只调整任务资金来源；API Key 旧 quota 不参与请求或任务结算。
func taskAdjustFunding(task *model.Task, delta int, final bool) error {
	if taskIsSubscription(task) {
		sub, err := loadTaskSubscription(task)
		if err != nil {
			return err
		}
		if sub.Status == model.SubscriptionStatusConverted {
			return model.PostConsumeUserSubscriptionTokenDelta(task.PrivateData.SubscriptionId, int64(delta))
		}
		if sub.EntitlementType == model.SubscriptionEntitlementCreditBalance {
			valuationReady, err := model.CreditValuationRuntimeReadyTx(model.DB)
			if err != nil {
				return err
			}
			if !valuationReady {
				return model.ErrCreditValuationStateMismatch
			}
			requestID, err := taskSubscriptionRequestID(task)
			if err != nil {
				return err
			}
			target := int64(task.Quota) + int64(delta)
			if target < 0 {
				return model.ErrCreditValuationNegativeInput
			}
			if strings.TrimSpace(task.PrivateData.SubscriptionRequestId) == "" {
				return model.SettleLegacyCreditTaskRequestTarget(requestID, task.PrivateData.SubscriptionId, int64(task.Quota), target, final)
			}
			return model.SettleUserSubscriptionRequestTarget(requestID, task.PrivateData.SubscriptionId, target, final)
		}
		if delta == 0 {
			return nil
		}
		distributor, err := taskUsesDistributorSubscription(sub)
		if err != nil {
			return err
		}
		if distributor {
			return fmt.Errorf("非文本异步任务不支持分销订阅扣费")
		}
		return model.PostConsumeUserSubscriptionAmountDelta(task.PrivateData.SubscriptionId, int64(delta))
	}
	if delta == 0 {
		return nil
	}
	return ErrLegacyWalletFundingDisabled
}

// taskBillingOther 从 task 的 BillingContext 构建日志 Other 字段。
func taskBillingOther(task *model.Task) map[string]interface{} {
	other := make(map[string]interface{})
	if bc := task.PrivateData.BillingContext; bc != nil {
		other["model_price"] = bc.ModelPrice
		if bc.ModelRatio > 0 {
			other["model_ratio"] = bc.ModelRatio
		}
		if len(bc.OtherRatios) > 0 {
			for k, v := range bc.OtherRatios {
				other[k] = v
			}
		}
	}
	if taskIsSubscription(task) {
		other["billing_source"] = BillingSourceSubscription
		other["subscription_id"] = task.PrivateData.SubscriptionId
	}
	props := task.Properties
	if props.UpstreamModelName != "" && props.UpstreamModelName != props.OriginModelName {
		other["is_model_mapped"] = true
		other["upstream_model_name"] = props.UpstreamModelName
	}
	return other
}

// taskModelName 从 BillingContext 或 Properties 中获取模型名称。
func taskModelName(task *model.Task) string {
	if bc := task.PrivateData.BillingContext; bc != nil && bc.OriginModelName != "" {
		return bc.OriginModelName
	}
	return task.Properties.OriginModelName
}

// RefundTaskQuota 统一的任务失败退款逻辑。
// 当异步任务失败时，只退还任务资金来源；API Key 旧 quota 不参与任务结算。
func RefundTaskQuota(ctx context.Context, task *model.Task, reason string) {
	quota := task.Quota
	if quota == 0 {
		return
	}

	// 1. 退还任务资金来源（订阅资金来源；旧 wallet 路径已禁用）
	if err := taskAdjustFunding(task, -quota, true); err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("退还资金来源失败 task %s: %s", task.TaskID, err.Error()))
		return
	}

	// 2. 记录日志
	other := taskBillingOther(task)
	other["task_id"] = task.TaskID
	other["reason"] = reason
	model.RecordTaskBillingLog(model.RecordTaskBillingLogParams{
		UserId:    task.UserId,
		LogType:   model.LogTypeRefund,
		Content:   "",
		ChannelId: task.ChannelId,
		ModelName: taskModelName(task),
		Quota:     quota,
		TokenId:   task.PrivateData.TokenId,
		Other:     other,
	})
}

// RecalculateTaskQuota 通用的异步差额结算。
// actualQuota 是任务完成后的实际应扣额度，与预扣额度 (task.Quota) 做差额结算。
// reason 用于日志记录（例如 "token重算" 或 "adaptor调整"）。
func RecalculateTaskQuota(ctx context.Context, task *model.Task, actualQuota int, reason string) {
	RecalculateTaskQuotaWithClamp(ctx, task, actualQuota, reason, nil)
}

func RecalculateTaskQuotaWithClamp(ctx context.Context, task *model.Task, actualQuota int, reason string, clamp *common.QuotaClamp) {
	if actualQuota <= 0 {
		return
	}
	preConsumedQuota := task.Quota
	quotaDelta := actualQuota - preConsumedQuota

	if quotaDelta == 0 {
		if err := taskAdjustFunding(task, 0, true); err != nil {
			logger.LogError(ctx, fmt.Sprintf("任务终态结算失败 task %s: %s", task.TaskID, err.Error()))
			return
		}
		logger.LogInfo(ctx, fmt.Sprintf("任务 %s 预扣费准确（%s，%s）",
			task.TaskID, logger.LogQuota(actualQuota), reason))
		return
	}

	logger.LogInfo(ctx, fmt.Sprintf("任务 %s 差额结算：delta=%s（实际：%s，预扣：%s，%s）",
		task.TaskID,
		logger.LogQuota(quotaDelta),
		logger.LogQuota(actualQuota),
		logger.LogQuota(preConsumedQuota),
		reason,
	))

	// 调整任务资金来源；API Key 旧 quota 不参与差额结算。
	if err := taskAdjustFunding(task, quotaDelta, true); err != nil {
		logger.LogError(ctx, fmt.Sprintf("差额结算资金调整失败 task %s: %s", task.TaskID, err.Error()))
		return
	}

	task.Quota = actualQuota
	if err := task.UpdateQuota(); err != nil {
		logger.LogError(ctx, fmt.Sprintf("差额结算回写 quota 失败 task %s: %s", task.TaskID, err.Error()))
	}

	var logType int
	var logQuota int
	if quotaDelta > 0 {
		logType = model.LogTypeConsume
		logQuota = quotaDelta
		model.UpdateUserUsedQuotaAndRequestCount(task.UserId, quotaDelta)
		model.UpdateChannelUsedQuota(task.ChannelId, quotaDelta)
	} else {
		logType = model.LogTypeRefund
		logQuota = -quotaDelta
	}
	other := taskBillingOther(task)
	other["task_id"] = task.TaskID
	other["pre_consumed_quota"] = preConsumedQuota
	other["actual_quota"] = actualQuota
	attachQuotaClamp(other, clamp)
	model.RecordTaskBillingLog(model.RecordTaskBillingLogParams{
		UserId:    task.UserId,
		LogType:   logType,
		Content:   reason,
		ChannelId: task.ChannelId,
		ModelName: taskModelName(task),
		Quota:     logQuota,
		TokenId:   task.PrivateData.TokenId,
		Other:     other,
	})
}

// RecalculateTaskQuotaByTokens 根据实际 token 消耗重新计费（异步差额结算）。
// 当任务成功且返回了 totalTokens 时，根据模型倍率重新计算实际扣费额度，
// 与预扣费的差额进行补扣或退还；仅调整任务资金来源，不调整 API Key 旧 quota。
func RecalculateTaskQuotaByTokens(ctx context.Context, task *model.Task, totalTokens int) {
	if totalTokens <= 0 {
		return
	}

	modelName := taskModelName(task)

	// 获取模型价格和倍率
	modelRatio, hasRatioSetting, _ := ratio_setting.GetModelRatio(modelName)
	// 只有配置了倍率(非固定价格)时才按 token 重新计费
	if !hasRatioSetting || modelRatio <= 0 {
		return
	}

	// 计算 OtherRatios 乘积（视频折扣、时长等）
	otherMultiplier := 1.0
	if bc := task.PrivateData.BillingContext; bc != nil {
		for _, r := range bc.OtherRatios {
			if r != 1.0 && r > 0 {
				otherMultiplier *= r
			}
		}
	}

	// 计算实际应扣费额度: totalTokens * modelRatio * otherMultiplier
	actualQuota, clamp := common.QuotaFromFloatChecked(float64(totalTokens) * modelRatio * otherMultiplier)

	reason := fmt.Sprintf("token重算：tokens=%d, modelRatio=%.2f, otherMultiplier=%.4f", totalTokens, modelRatio, otherMultiplier)
	RecalculateTaskQuotaWithClamp(ctx, task, actualQuota, reason, clamp)
}
