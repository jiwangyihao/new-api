package openai

import (
	"io"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

func OaiResponsesCompactionHandler(c *gin.Context, args ...any) (*dto.Usage, *types.NewAPIError) {
	var info *relaycommon.RelayInfo
	var resp *http.Response
	for _, arg := range args {
		switch v := arg.(type) {
		case *relaycommon.RelayInfo:
			info = v
		case *http.Response:
			resp = v
		}
	}
	if info == nil && c != nil {
		if value, ok := c.Get("relay_info"); ok {
			if relayInfo, ok := value.(*relaycommon.RelayInfo); ok {
				info = relayInfo
			}
		}
	}
	if resp == nil || resp.Body == nil {
		return nil, types.NewError(nil, types.ErrorCodeBadResponse)
	}
	defer service.CloseResponseBodyGracefully(resp)

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}

	var compactResp dto.OpenAIResponsesCompactionResponse
	if err := common.Unmarshal(responseBody, &compactResp); err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	if oaiError := compactResp.GetOpenAIError(); oaiError != nil && oaiError.Type != "" {
		return nil, types.WithOpenAIError(*oaiError, resp.StatusCode)
	}

	usage := dto.Usage{}
	if compactResp.Usage != nil {
		usage.PromptTokens = compactResp.Usage.InputTokens
		usage.CompletionTokens = compactResp.Usage.OutputTokens
		usage.TotalTokens = compactResp.Usage.TotalTokens
		if compactResp.Usage.InputTokensDetails != nil {
			usage.PromptTokensDetails.CachedTokens = compactResp.Usage.InputTokensDetails.CachedTokens
		}
	}
	applyDynamicBillingMultiplierFromHTTPResponse(info, resp, responseBody, relaycommon.DynamicBillingMultiplierSourceBody)
	if compactResp.Usage != nil && info != nil && openAIResponseStatusCompleted(compactResp.Status) {
		markCodexProServedCandidateFromResponseTrailer(info, resp)
		info.ConfirmCodexProServed()
		if billing := service.NewAPIBillingFromUsage(info, &usage); billing != nil {
			compactResp.NewAPIBilling = billing
			service.SeedNewAPIBillingRelayInfo(info, *billing)
			if responseBody, err = common.Marshal(compactResp); err != nil {
				return nil, types.NewOpenAIError(err, types.ErrorCodeJsonMarshalFailed, http.StatusInternalServerError)
			}
		}
	}

	writeOK := service.IOCopyBytesGracefully(c, resp, responseBody)
	if !writeOK && info != nil {
		info.ClearCodexProServedCandidate()
		info.CodexProServed = false
	}

	return &usage, nil
}
