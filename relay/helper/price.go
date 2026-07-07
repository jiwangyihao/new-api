package helper

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

// https://docs.claude.com/en/docs/build-with-claude/prompt-caching#1-hour-cache-duration
const claudeCacheCreation1hMultiplier = 6 / 3.75

func HandleQuotaMultiplier(ctx *gin.Context, relayInfo *relaycommon.RelayInfo) types.QuotaMultiplierInfo {
	return types.QuotaMultiplierInfo{
		Ratio:        1.0,
		SpecialRatio: -1,
	}
}

func ModelPriceHelper(c *gin.Context, info *relaycommon.RelayInfo, promptTokens int, meta *types.TokenCountMeta) (types.PriceData, error) {
	billingModelName := relaycommon.ResolveBillingModelName(info)
	modelPrice, usePrice := ratio_setting.GetModelPrice(billingModelName, false)

	quotaMultiplierInfo := HandleQuotaMultiplier(c, info)

	// Check if this model uses tiered_expr billing
	if billing_setting.GetBillingMode(billingModelName) == billing_setting.BillingModeTieredExpr {
		return modelPriceHelperTiered(c, info, promptTokens, meta, quotaMultiplierInfo)
	}

	var preConsumedQuota int
	var modelRatio float64
	var completionRatio float64
	var cacheRatio float64
	var imageRatio float64
	var cacheCreationRatio float64
	var cacheCreationRatio5m float64
	var cacheCreationRatio1h float64
	var audioRatio float64
	var audioCompletionRatio float64
	var freeModel bool
	if !usePrice {
		preConsumedTokens := common.Max(promptTokens, common.PreConsumedQuota)
		if meta.MaxTokens != 0 {
			preConsumedTokens += meta.MaxTokens
		}
		// 未配置价格/倍率的模型不再被拒绝访问：GetModelRatio 在未配置时返回默认倍率 37.5，按该倍率计费放行。
		modelRatio, _, _ = ratio_setting.GetModelRatio(billingModelName)
		completionRatio = ratio_setting.GetCompletionRatio(billingModelName)
		cacheRatio, _ = ratio_setting.GetCacheRatio(billingModelName)
		cacheCreationRatio, _ = ratio_setting.GetCreateCacheRatio(billingModelName)
		cacheCreationRatio5m = cacheCreationRatio
		// 固定1h和5min缓存写入价格的比例
		cacheCreationRatio1h = cacheCreationRatio * claudeCacheCreation1hMultiplier
		imageRatio, _ = ratio_setting.GetImageRatio(billingModelName)
		audioRatio = ratio_setting.GetAudioRatio(billingModelName)
		audioCompletionRatio = ratio_setting.GetAudioCompletionRatio(billingModelName)
		ratio := modelRatio
		preConsumedQuota = common.QuotaFromFloat(float64(preConsumedTokens) * ratio)
	} else {
		if meta.ImagePriceRatio != 0 {
			modelPrice = modelPrice * meta.ImagePriceRatio
		}
		preConsumedQuota = common.QuotaFromFloat(modelPrice * common.QuotaPerUnit)
	}

	// check if free model pre-consume is disabled
	if !operation_setting.GetQuotaSetting().EnableFreeModelPreConsume {
		// if model price or ratio is 0, do not pre-consume quota
		if usePrice {
			if modelPrice == 0 {
				preConsumedQuota = 0
				freeModel = true
			}
		} else {
			if modelRatio == 0 {
				preConsumedQuota = 0
				freeModel = true
			}
		}
	}

	priceData := types.PriceData{
		FreeModel:            freeModel,
		ModelPrice:           modelPrice,
		ModelRatio:           modelRatio,
		CompletionRatio:      completionRatio,
		QuotaMultiplierInfo:  quotaMultiplierInfo,
		UsePrice:             usePrice,
		CacheRatio:           cacheRatio,
		ImageRatio:           imageRatio,
		AudioRatio:           audioRatio,
		AudioCompletionRatio: audioCompletionRatio,
		CacheCreationRatio:   cacheCreationRatio,
		CacheCreation5mRatio: cacheCreationRatio5m,
		CacheCreation1hRatio: cacheCreationRatio1h,
		QuotaToPreConsume:    preConsumedQuota,
	}

	if common.DebugEnabled {
		println(fmt.Sprintf("model_price_helper result: %s", priceData.ToSetting()))
	}
	info.PriceData = priceData
	return priceData, nil
}

// ModelPriceHelperPerCall 按次/按量计费的 PriceHelper (MJ、Task)
func ModelPriceHelperPerCall(c *gin.Context, info *relaycommon.RelayInfo) (types.PriceData, error) {
	quotaMultiplierInfo := HandleQuotaMultiplier(c, info)
	billingModelName := relaycommon.ResolveBillingModelName(info)

	modelPrice, success := ratio_setting.GetModelPrice(billingModelName, true)
	usePrice := success
	var modelRatio float64

	if !success {
		defaultPrice, ok := ratio_setting.GetDefaultModelPriceMap()[billingModelName]
		if ok {
			modelPrice = defaultPrice
			usePrice = true
		} else {
			// 未配置价格的模型不再被拒绝访问：GetModelRatio 在未配置时返回默认倍率 37.5，按该倍率计费放行。
			modelRatio, _, _ = ratio_setting.GetModelRatio(billingModelName)
		}
	}

	var quota int
	freeModel := false

	if usePrice {
		quota = common.QuotaFromFloat(modelPrice * common.QuotaPerUnit)
		if !operation_setting.GetQuotaSetting().EnableFreeModelPreConsume {
			if modelPrice == 0 {
				quota = 0
				freeModel = true
			}
		}
	} else {
		// 按量计费：以模型倍率的一半作为预扣额度
		quota = common.QuotaFromFloat(modelRatio / 2 * common.QuotaPerUnit)
		modelPrice = -1
		if !operation_setting.GetQuotaSetting().EnableFreeModelPreConsume {
			if modelRatio == 0 {
				quota = 0
				freeModel = true
			}
		}
	}

	priceData := types.PriceData{
		FreeModel:           freeModel,
		ModelPrice:          modelPrice,
		ModelRatio:          modelRatio,
		UsePrice:            usePrice,
		Quota:               quota,
		QuotaMultiplierInfo: quotaMultiplierInfo,
	}
	return priceData, nil
}

func HasModelBillingConfig(modelName string) bool {
	if _, ok := ratio_setting.GetModelPrice(modelName, false); ok {
		return true
	}
	if _, ok, _ := ratio_setting.GetModelRatio(modelName); ok {
		return true
	}
	if billing_setting.GetBillingMode(modelName) != billing_setting.BillingModeTieredExpr {
		return false
	}
	expr, ok := billing_setting.GetBillingExpr(modelName)
	return ok && strings.TrimSpace(expr) != ""
}

func modelPriceHelperTiered(c *gin.Context, info *relaycommon.RelayInfo, promptTokens int, meta *types.TokenCountMeta, quotaMultiplierInfo types.QuotaMultiplierInfo) (types.PriceData, error) {
	billingModelName := relaycommon.ResolveBillingModelName(info)
	exprStr, ok := billing_setting.GetBillingExpr(billingModelName)
	if !ok {
		return types.PriceData{}, fmt.Errorf("model %s is configured as tiered_expr but has no billing expression", billingModelName)
	}

	estimatedCompletionTokens := 0
	if meta.MaxTokens != 0 {
		estimatedCompletionTokens = meta.MaxTokens
	}

	requestInput, err := ResolveIncomingBillingExprRequestInput(c, info)
	if err != nil {
		return types.PriceData{}, err
	}

	rawCost, trace, err := billingexpr.RunExprWithRequest(exprStr, billingexpr.TokenParams{
		P:   float64(promptTokens),
		C:   float64(estimatedCompletionTokens),
		Len: float64(promptTokens),
	}, requestInput)
	if err != nil {
		return types.PriceData{}, fmt.Errorf("model %s tiered expr run failed: %w", billingModelName, err)
	}

	// Expression coefficients are $/1M tokens prices; convert to quota the same way per-call billing does.
	quotaBeforeRatio := rawCost / 1_000_000 * common.QuotaPerUnit
	preConsumedQuota := billingexpr.QuotaRound(quotaBeforeRatio)

	freeModel := false
	if !operation_setting.GetQuotaSetting().EnableFreeModelPreConsume {
		if preConsumedQuota == 0 {
			freeModel = true
		}
	}

	exprHash := billingexpr.ExprHashString(exprStr)
	snapshot := &billingexpr.BillingSnapshot{
		BillingMode:               billing_setting.BillingModeTieredExpr,
		ModelName:                 billingModelName,
		ExprString:                exprStr,
		ExprHash:                  exprHash,
		QuotaMultiplier:           1,
		EstimatedPromptTokens:     promptTokens,
		EstimatedCompletionTokens: estimatedCompletionTokens,
		EstimatedQuotaBeforeRatio: quotaBeforeRatio,
		EstimatedQuota:            preConsumedQuota,
		EstimatedTier:             trace.MatchedTier,
		QuotaPerUnit:              common.QuotaPerUnit,
		ExprVersion:               billingexpr.ExprVersion(exprStr),
	}
	info.TieredBillingSnapshot = snapshot
	info.BillingRequestInput = &requestInput

	priceData := types.PriceData{
		FreeModel:           freeModel,
		QuotaMultiplierInfo: quotaMultiplierInfo,
		QuotaToPreConsume:   preConsumedQuota,
	}

	if common.DebugEnabled {
		println(fmt.Sprintf("model_price_helper_tiered result: model=%s preConsume=%d quotaBeforeRatio=%.2f tier=%s", billingModelName, preConsumedQuota, quotaBeforeRatio, trace.MatchedTier))
	}

	info.PriceData = priceData
	return priceData, nil
}
