package controller

import (
	"strconv"

	"github.com/QuantumNous/new-api/model"
)

type subscriptionConversionQuoteReasonResponse struct {
	Code string         `json:"code"`
	Data map[string]any `json:"data,omitempty"`
}

type subscriptionConversionQuoteItemResponse struct {
	SourceSubscriptionId     string                                      `json:"source_subscription_id"`
	PlanId                   string                                      `json:"plan_id"`
	PlanTitle                string                                      `json:"plan_title"`
	EntitlementType          string                                      `json:"entitlement_type"`
	GrantSource              string                                      `json:"grant_source"`
	Status                   string                                      `json:"status"`
	Category                 string                                      `json:"category"`
	DatabaseNow              string                                      `json:"database_now"`
	StartTime                string                                      `json:"start_time"`
	EndTime                  string                                      `json:"end_time"`
	RemainingSeconds         string                                      `json:"remaining_seconds"`
	Full31DayBlocks          string                                      `json:"full_31_day_blocks"`
	CreditBasis              string                                      `json:"credit_basis"`
	CreditBasisSource        string                                      `json:"credit_basis_source"`
	CurrentRemainingCredit   string                                      `json:"current_remaining_credit"`
	GrossCredit              string                                      `json:"gross_credit"`
	CurrentDebt              string                                      `json:"current_debt"`
	EstimatedDebtOffset      string                                      `json:"estimated_debt_offset"`
	NetAvailableCredit       string                                      `json:"net_available_credit"`
	LastGrantedAt            string                                      `json:"last_granted_at"`
	LastGrantTimeSource      string                                      `json:"last_grant_time_source"`
	LastGrantSource          string                                      `json:"last_grant_source"`
	CooldownStatus           string                                      `json:"cooldown_status"`
	CooldownRemainingSeconds string                                      `json:"cooldown_remaining_seconds"`
	GraceStatus              string                                      `json:"grace_status"`
	GraceRemainingSeconds    string                                      `json:"grace_remaining_seconds"`
	Expired                  bool                                        `json:"expired"`
	WithinGrace              bool                                        `json:"within_grace"`
	Eligible                 bool                                        `json:"eligible"`
	CanConfirm               bool                                        `json:"can_confirm"`
	ReasonCodes              []string                                    `json:"reason_codes"`
	Reasons                  []subscriptionConversionQuoteReasonResponse `json:"reasons"`
	CalculationErrorCode     string                                      `json:"calculation_error_code,omitempty"`
}

type subscriptionConversionQuoteListResponse struct {
	DatabaseNow string                                    `json:"database_now"`
	Quotes      []subscriptionConversionQuoteItemResponse `json:"quotes"`
}

func toSubscriptionConversionQuoteListResponse(input *model.TimedSubscriptionConversionQuoteList) subscriptionConversionQuoteListResponse {
	response := subscriptionConversionQuoteListResponse{DatabaseNow: "0", Quotes: []subscriptionConversionQuoteItemResponse{}}
	if input == nil {
		return response
	}
	response.DatabaseNow = strconv.FormatInt(input.DatabaseNow, 10)
	response.Quotes = make([]subscriptionConversionQuoteItemResponse, 0, len(input.Quotes))
	for i := range input.Quotes {
		quote := &input.Quotes[i]
		reasons := make([]subscriptionConversionQuoteReasonResponse, 0, len(quote.Reasons))
		for _, reason := range quote.Reasons {
			reasons = append(reasons, subscriptionConversionQuoteReasonResponse{
				Code: reason.Code,
				Data: stringifyConversionQuoteReasonData(reason.Data),
			})
		}
		response.Quotes = append(response.Quotes, subscriptionConversionQuoteItemResponse{
			SourceSubscriptionId:     strconv.FormatInt(int64(quote.SourceSubscriptionId), 10),
			PlanId:                   strconv.FormatInt(int64(quote.PlanId), 10),
			PlanTitle:                quote.PlanTitle,
			EntitlementType:          quote.EntitlementType,
			GrantSource:              quote.GrantSource,
			Status:                   quote.Status,
			Category:                 quote.Category,
			DatabaseNow:              strconv.FormatInt(quote.DatabaseNow, 10),
			StartTime:                strconv.FormatInt(quote.StartTime, 10),
			EndTime:                  strconv.FormatInt(quote.EndTime, 10),
			RemainingSeconds:         strconv.FormatInt(quote.RemainingSeconds, 10),
			Full31DayBlocks:          strconv.FormatInt(quote.Full31DayBlocks, 10),
			CreditBasis:              strconv.FormatInt(quote.CreditBasis, 10),
			CreditBasisSource:        quote.CreditBasisSource,
			CurrentRemainingCredit:   strconv.FormatInt(quote.CurrentRemainingCredit, 10),
			GrossCredit:              strconv.FormatInt(quote.GrossCredit, 10),
			CurrentDebt:              strconv.FormatInt(quote.CurrentDebt, 10),
			EstimatedDebtOffset:      strconv.FormatInt(quote.EstimatedDebtOffset, 10),
			NetAvailableCredit:       strconv.FormatInt(quote.NetAvailableCredit, 10),
			LastGrantedAt:            strconv.FormatInt(quote.LastGrantedAt, 10),
			LastGrantTimeSource:      quote.LastGrantTimeSource,
			LastGrantSource:          quote.LastGrantSource,
			CooldownStatus:           quote.CooldownStatus,
			CooldownRemainingSeconds: strconv.FormatInt(quote.CooldownRemainingSeconds, 10),
			GraceStatus:              quote.GraceStatus,
			GraceRemainingSeconds:    strconv.FormatInt(quote.GraceRemainingSeconds, 10),
			Expired:                  quote.Expired,
			WithinGrace:              quote.WithinGrace,
			Eligible:                 quote.Eligible,
			CanConfirm:               quote.CanConfirm,
			ReasonCodes:              append([]string(nil), quote.ReasonCodes...),
			Reasons:                  reasons,
			CalculationErrorCode:     quote.CalculationErrorCode,
		})
	}
	return response
}

func stringifyConversionQuoteReasonData(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	output := make(map[string]any, len(input))
	for key, value := range input {
		switch typed := value.(type) {
		case int:
			output[key] = strconv.FormatInt(int64(typed), 10)
		case int8:
			output[key] = strconv.FormatInt(int64(typed), 10)
		case int16:
			output[key] = strconv.FormatInt(int64(typed), 10)
		case int32:
			output[key] = strconv.FormatInt(int64(typed), 10)
		case int64:
			output[key] = strconv.FormatInt(typed, 10)
		case uint:
			output[key] = strconv.FormatUint(uint64(typed), 10)
		case uint8:
			output[key] = strconv.FormatUint(uint64(typed), 10)
		case uint16:
			output[key] = strconv.FormatUint(uint64(typed), 10)
		case uint32:
			output[key] = strconv.FormatUint(uint64(typed), 10)
		case uint64:
			output[key] = strconv.FormatUint(typed, 10)
		default:
			output[key] = value
		}
	}
	return output
}
