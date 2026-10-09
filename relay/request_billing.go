package relay

import (
	"context"
	"errors"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"net/http"
)

// One worker owns a reservation across retries; each create gets a fresh owner.
type responsesWSBilling struct {
	lease    service.SubscriptionConcurrencyLease
	settled  bool
	finished bool
}

func prepareResponsesWSBilling(c *gin.Context, info *relaycommon.RelayInfo) (*responsesWSBilling, *types.NewAPIError) {
	lifecycle := &responsesWSBilling{}
	meta := &types.TokenCountMeta{TokenType: types.TokenTypeTokenizer}
	if setting.ShouldCheckPromptSensitive() || constant.CountToken {
		meta = info.Request.GetTokenCountMeta()
	} else if req, ok := info.Request.(*dto.OpenAIResponsesRequest); ok && req.MaxOutputTokens != nil {
		meta.MaxTokens = int(*req.MaxOutputTokens)
	}
	if setting.ShouldCheckPromptSensitive() && meta != nil {
		if contains, _ := service.CheckSensitiveText(meta.CombineText); contains {
			return lifecycle, types.NewError(errors.New("sensitive prompt is not allowed"), types.ErrorCodeSensitiveWordsDetected, types.ErrOptionWithSkipRetry())
		}
	}
	tokens, err := service.EstimateRequestToken(c, meta, info)
	if err != nil {
		return lifecycle, types.NewError(err, types.ErrorCodeCountTokenFailed, types.ErrOptionWithSkipRetry())
	}
	info.SetEstimatePromptTokens(tokens)
	price, err := helper.ModelPriceHelper(c, info, tokens, meta)
	if err != nil {
		return lifecycle, types.NewError(err, types.ErrorCodeModelPriceError, types.ErrOptionWithStatusCode(http.StatusBadRequest), types.ErrOptionWithSkipRetry())
	}
	info.FreeModel = price.FreeModel
	if err = info.FreezeChannelTokenBillingSnapshot(c); err != nil {
		return lifecycle, types.NewError(err, types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
	}
	// A free model still requires a subscription, concurrency admission and token cap.
	if apiErr := service.PreConsumeBilling(c, price.QuotaToPreConsume, info); apiErr != nil {
		return lifecycle, apiErr
	}
	var apiErr *types.NewAPIError
	lifecycle.lease, apiErr = service.AcquireSubscriptionConcurrency(c.Request.Context(), info)
	if apiErr != nil {
		return lifecycle, apiErr
	}
	info.TokenLimit = service.NewTokenLimitSession(info)
	if info.TokenLimit != nil {
		if apiErr = info.TokenLimit.PreConsume(info.SubscriptionPreConsumedTokens()); apiErr != nil {
			service.RefundBillingAfterTokenLimitReject(info.Billing)
			return lifecycle, apiErr
		}
	}
	return lifecycle, nil
}

func (b *responsesWSBilling) finish(c *gin.Context, info *relaycommon.RelayInfo) {
	if b == nil || b.finished {
		return
	}
	b.finished = true
	if b.lease != nil {
		_ = b.lease.Release(context.Background())
		b.lease = nil
	}
	if b.settled || info == nil {
		return
	}
	if info.Billing != nil {
		info.Billing.Refund(c)
	}
	service.RefundTokenLimitOnRelayFailure(info, "responses_websocket_no_output")
}

func (b *responsesWSBilling) settle(c *gin.Context, info *relaycommon.RelayInfo, usage *dto.Usage) *types.NewAPIError {
	if b == nil || b.settled || !service.PrepareRelaySettlement(c, info) {
		return nil
	}
	b.settled = true
	if err := service.PostTextConsumeQuota(c, info, usage, nil); err != nil {
		service.MarkTokenLimitAfterResponseFailure(info, "responses_websocket_settlement_failed")
		if info.Billing != nil {
			info.Billing.CommitPreConsumedOnFailure()
		}
		return service.PostSettleErrorToOpenAIError(info, err)
	}
	return nil
}
