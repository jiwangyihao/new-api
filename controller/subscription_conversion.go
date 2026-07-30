package controller

import (
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
	Id                     string `json:"id"`
	SourceSubscriptionId   string `json:"source_subscription_id"`
	SourcePlanId           string `json:"source_plan_id"`
	SourcePlanTitle        string `json:"source_plan_title"`
	TargetSubscriptionId   string `json:"target_subscription_id"`
	TargetPlanId           string `json:"target_plan_id"`
	LedgerId               string `json:"ledger_id"`
	SourceStatus           string `json:"source_status"`
	GrantSource            string `json:"grant_source"`
	DatabaseNow            string `json:"database_now"`
	SourceStartTime        string `json:"source_start_time"`
	SourceEndTime          string `json:"source_end_time"`
	RemainingSeconds       string `json:"remaining_seconds"`
	Full31DayBlocks        string `json:"full_31_day_blocks"`
	CreditBasis            string `json:"credit_basis"`
	CreditBasisSource      string `json:"credit_basis_source"`
	CurrentRemainingCredit string `json:"current_remaining_credit"`
	GrossCredit            string `json:"gross_credit"`
	DebtOffset             string `json:"debt_offset"`
	NetAvailableCredit     string `json:"net_available_credit"`
	AvailableCreditAfter   string `json:"available_credit_after"`
	SettlementDebtAfter    string `json:"settlement_debt_after"`
	BalanceBefore          string `json:"balance_before"`
	BalanceAfter           string `json:"balance_after"`
	LastGrantedAt          string `json:"last_granted_at"`
	LastGrantTimeSource    string `json:"last_grant_time_source"`
	LastGrantSource        string `json:"last_grant_source"`
	ConvertedAt            string `json:"converted_at"`
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
		common.ApiErrorMsg(c, err.Error())
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
	return subscriptionConversionHistoryResponse{
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
}
