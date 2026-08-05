package controller

import (
	"errors"
	"math/big"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

type subscriptionConversionConfirmRequest struct {
	SubscriptionId string `json:"subscription_id"`
	IdempotencyKey string `json:"idempotency_key"`
}

type subscriptionConversionConfirmDataResponse struct {
	Replayed   bool                                  `json:"replayed"`
	Conversion subscriptionConversionHistoryResponse `json:"conversion"`
}

type subscriptionConversionHistoryResponse struct {
	Id                       string `json:"id"`
	SourceSubscriptionId     string `json:"source_subscription_id"`
	SourcePlanId             string `json:"source_plan_id"`
	SourcePlanTitle          string `json:"source_plan_title"`
	TargetSubscriptionId     string `json:"target_subscription_id"`
	TargetPlanId             string `json:"target_plan_id"`
	LedgerId                 string `json:"ledger_id"`
	SourceStatus             string `json:"source_status"`
	GrantSource              string `json:"grant_source"`
	DatabaseNow              string `json:"database_now"`
	SourceStartTime          string `json:"source_start_time"`
	SourceEndTime            string `json:"source_end_time"`
	RemainingSeconds         string `json:"remaining_seconds"`
	Full31DayBlocks          string `json:"full_31_day_blocks"`
	CreditBasis              string `json:"credit_basis"`
	CreditBasisSource        string `json:"credit_basis_source"`
	CurrentRemainingCredit   string `json:"current_remaining_credit"`
	GrossCredit              string `json:"gross_credit"`
	DebtOffset               string `json:"debt_offset"`
	NetAvailableCredit       string `json:"net_available_credit"`
	AvailableCreditAfter     string `json:"available_credit_after"`
	SettlementDebtAfter      string `json:"settlement_debt_after"`
	BalanceBefore            string `json:"balance_before"`
	BalanceAfter             string `json:"balance_after"`
	LastGrantedAt            string `json:"last_granted_at"`
	LastGrantTimeSource      string `json:"last_grant_time_source"`
	LastGrantSource          string `json:"last_grant_source"`
	ConvertedAt              string `json:"converted_at"`
	SourcePriceMicros        string `json:"source_price_micros,omitempty"`
	SourceCurrency           string `json:"source_currency,omitempty"`
	TargetCurrency           string `json:"target_currency,omitempty"`
	ValuationCreditBasis     string `json:"valuation_credit_basis,omitempty"`
	GrossCostMicros          string `json:"gross_cost_micros,omitempty"`
	NetCostMicros            string `json:"net_cost_micros,omitempty"`
	UnitValueNumeratorMicros string `json:"unit_value_numerator_micros,omitempty"`
	UnitValueDenominator     string `json:"unit_value_denominator,omitempty"`
	RuleVersion              int    `json:"rule_version,omitempty"`
	FxNumerator              string `json:"fx_numerator,omitempty"`
	FxDenominator            string `json:"fx_denominator,omitempty"`
	FxCapturedAt             string `json:"fx_captured_at,omitempty"`
	FxDirection              string `json:"fx_direction,omitempty"`
}

func ConfirmSubscriptionConversion(c *gin.Context) {
	var request subscriptionConversionConfirmRequest
	if err := c.ShouldBindJSON(&request); err != nil || strings.TrimSpace(request.IdempotencyKey) == "" {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	subscriptionId, err := strconv.ParseInt(strings.TrimSpace(request.SubscriptionId), 10, strconv.IntSize)
	if err != nil || subscriptionId <= 0 {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	result, err := model.ConfirmTimedSubscriptionConversion(c.GetInt("id"), int(subscriptionId), request.IdempotencyKey)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
			"code":    subscriptionConversionErrorCode(err),
		})
		return
	}
	common.ApiSuccess(c, subscriptionConversionConfirmDataResponse{
		Replayed:   result.Replayed,
		Conversion: toSubscriptionConversionHistoryResponse(result.Conversion),
	})
}

func toSubscriptionConversionHistoryResponse(conversion *model.SubscriptionConversion) subscriptionConversionHistoryResponse {
	if conversion == nil {
		return subscriptionConversionHistoryResponse{}
	}
	response := subscriptionConversionHistoryResponse{
		Id:                     strconv.Itoa(conversion.Id),
		SourceSubscriptionId:   strconv.Itoa(conversion.SourceSubscriptionId),
		SourcePlanId:           strconv.Itoa(conversion.SourcePlanId),
		SourcePlanTitle:        conversion.SourcePlanTitle,
		TargetSubscriptionId:   strconv.Itoa(conversion.TargetSubscriptionId),
		TargetPlanId:           strconv.Itoa(conversion.TargetPlanId),
		LedgerId:               strconv.Itoa(conversion.LedgerId),
		SourceStatus:           conversion.SourceStatus,
		GrantSource:            conversion.GrantSource,
		DatabaseNow:            strconv.FormatInt(conversion.DatabaseNow, 10),
		SourceStartTime:        strconv.FormatInt(conversion.SourceStartTime, 10),
		SourceEndTime:          strconv.FormatInt(conversion.SourceEndTime, 10),
		RemainingSeconds:       strconv.FormatInt(conversion.RemainingSeconds, 10),
		Full31DayBlocks:        strconv.FormatInt(conversion.Full31DayBlocks, 10),
		CreditBasis:            strconv.FormatInt(conversion.CreditBasis, 10),
		CreditBasisSource:      conversion.CreditBasisSource,
		CurrentRemainingCredit: strconv.FormatInt(conversion.CurrentRemainingCredit, 10),
		GrossCredit:            strconv.FormatInt(conversion.GrossCredit, 10),
		DebtOffset:             strconv.FormatInt(conversion.DebtOffset, 10),
		NetAvailableCredit:     strconv.FormatInt(conversion.NetAvailableCredit, 10),
		AvailableCreditAfter:   strconv.FormatInt(conversion.AvailableCreditAfter, 10),
		SettlementDebtAfter:    strconv.FormatInt(conversion.SettlementDebtAfter, 10),
		BalanceBefore:          strconv.FormatInt(conversion.BalanceBefore, 10),
		BalanceAfter:           strconv.FormatInt(conversion.BalanceAfter, 10),
		LastGrantedAt:          strconv.FormatInt(conversion.LastGrantedAt, 10),
		LastGrantTimeSource:    conversion.LastGrantTimeSource,
		LastGrantSource:        conversion.LastGrantSource,
		ConvertedAt:            strconv.FormatInt(conversion.ConvertedAt, 10),
	}
	if conversion.ValuationCurrency != "" && conversion.FxRateDenominator > 0 {
		response.SourcePriceMicros = strconv.FormatInt(conversion.ValuationSourcePriceMicros, 10)
		response.SourceCurrency = conversion.FxSourceCurrency
		response.TargetCurrency = conversion.ValuationCurrency
		response.ValuationCreditBasis = strconv.FormatInt(conversion.ValuationCreditBasis, 10)
		response.GrossCostMicros = strconv.FormatInt(conversion.ValuationGrossCostMicros, 10)
		response.NetCostMicros = strconv.FormatInt(conversion.ValuationNetCostMicros, 10)
		response.UnitValueNumeratorMicros, response.UnitValueDenominator = subscriptionConversionUnitValue(conversion)
		response.RuleVersion = conversion.ValuationRuleVersion
		response.FxNumerator = strconv.FormatInt(conversion.FxRateNumerator, 10)
		response.FxDenominator = strconv.FormatInt(conversion.FxRateDenominator, 10)
		response.FxCapturedAt = strconv.FormatInt(conversion.FxCapturedAt, 10)
		response.FxDirection = subscriptionConversionFXDirection(conversion.FxSourceCurrency, conversion.ValuationCurrency)
	}
	return response
}

func subscriptionConversionUnitValue(conversion *model.SubscriptionConversion) (string, string) {
	if conversion.ValuationSourcePriceMicros <= 0 || conversion.ValuationCreditBasis <= 0 ||
		conversion.FxRateNumerator <= 0 || conversion.FxRateDenominator <= 0 {
		return "", ""
	}
	numerator := new(big.Int).Mul(
		big.NewInt(conversion.ValuationSourcePriceMicros),
		big.NewInt(conversion.FxRateNumerator),
	)
	denominator := new(big.Int).Mul(
		big.NewInt(conversion.ValuationCreditBasis),
		big.NewInt(conversion.FxRateDenominator),
	)
	unitValue := new(big.Rat).SetFrac(numerator, denominator)
	return unitValue.Num().String(), unitValue.Denom().String()
}

func subscriptionConversionFXDirection(sourceCurrency string, targetCurrency string) string {
	switch {
	case sourceCurrency == "" || targetCurrency == "":
		return ""
	case sourceCurrency == targetCurrency:
		return model.CreditFXDirectionIdentity
	case sourceCurrency == "USD" && targetCurrency == "CNY":
		return model.CreditFXDirectionUSDtoCNY
	case sourceCurrency == "CNY" && targetCurrency == "USD":
		return model.CreditFXDirectionCNYtoUSD
	default:
		return ""
	}
}

func subscriptionConversionErrorCode(err error) string {
	switch {
	case errors.Is(err, model.ErrConversionIdempotencyConflict):
		return model.ErrConversionIdempotencyConflict.Error()
	case errors.Is(err, model.ErrCreditFXRateMissing):
		return model.ErrCreditFXRateMissing.Error()
	case errors.Is(err, model.ErrCreditFXRateEmpty):
		return model.ErrCreditFXRateEmpty.Error()
	case errors.Is(err, model.ErrCreditFXInvalidDecimal):
		return model.ErrCreditFXInvalidDecimal.Error()
	case errors.Is(err, model.ErrCreditFXPrecisionExceeded):
		return model.ErrCreditFXPrecisionExceeded.Error()
	case errors.Is(err, model.ErrCreditFXNonPositive):
		return model.ErrCreditFXNonPositive.Error()
	case errors.Is(err, model.ErrCreditFXUnsupportedCurrency):
		return model.ErrCreditFXUnsupportedCurrency.Error()
	case errors.Is(err, model.ErrCreditFXDirectionMismatch):
		return model.ErrCreditFXDirectionMismatch.Error()
	case errors.Is(err, model.ErrCreditFXOverflow):
		return model.ErrCreditFXOverflow.Error()
	case strings.HasPrefix(err.Error(), "subscription conversion rejected:"):
		return "subscription_conversion_ineligible"
	default:
		return "subscription_conversion_failed"
	}
}
