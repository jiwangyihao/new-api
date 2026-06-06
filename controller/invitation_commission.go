package controller

import (
	"errors"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

type invitationCommissionTransferRequest struct {
	AmountCents int64 `json:"amount_cents"`
}

type invitationCommissionWithdrawalRequestBody struct {
	AmountCents int64                               `json:"amount_cents"`
	Contact     service.InvitationCommissionContact `json:"contact"`
	Remark      string                              `json:"remark"`
}

type adminInvitationCommissionWithdrawalActionRequest struct {
	AdminRemark string `json:"admin_remark"`
}

func GetInvitationCommissionSummary(c *gin.Context) {
	summary, err := service.GetInvitationCommissionSummary(c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, summary)
}

func GetInvitationCommissionRecords(c *gin.Context) {
	page, pageSize := invitationCommissionPageQuery(c)
	records, err := service.ListInvitationCommissionRecords(c.GetInt("id"), page, pageSize)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, records)
}

func TransferInvitationCommission(c *gin.Context) {
	var req invitationCommissionTransferRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiError(c, err)
		return
	}
	result, err := service.TransferInvitationCommissionToBalance(c.GetInt("id"), req.AmountCents)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, result)
}

func GetInvitationCommissionWithdrawals(c *gin.Context) {
	page, pageSize := invitationCommissionPageQuery(c)
	withdrawals, err := service.ListInvitationCommissionWithdrawals(c.GetInt("id"), page, pageSize)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, withdrawals)
}

func CreateInvitationCommissionWithdrawal(c *gin.Context) {
	var req invitationCommissionWithdrawalRequestBody
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiError(c, err)
		return
	}
	contact, err := normalizeInvitationCommissionContact(req.Contact)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	withdrawal, err := service.RequestInvitationCommissionWithdrawal(c.GetInt("id"), service.InvitationCommissionWithdrawalRequest{
		AmountCents: req.AmountCents,
		Contact:     contact,
		Remark:      strings.TrimSpace(req.Remark),
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	response, err := service.InvitationCommissionWithdrawalToResponse(*withdrawal)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, response)
}

func AdminListInvitationCommissionWithdrawals(c *gin.Context) {
	page, pageSize := invitationCommissionPageQuery(c)
	filter := service.InvitationCommissionWithdrawalFilter{
		Status:   c.Query("status"),
		UserId:   invitationCommissionIntQuery(c, "user_id"),
		Page:     page,
		PageSize: pageSize,
	}
	withdrawals, err := service.AdminListInvitationCommissionWithdrawals(filter)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, withdrawals)
}

func AdminCompleteInvitationCommissionWithdrawal(c *gin.Context) {
	withdrawalId, err := strconv.Atoi(c.Param("id"))
	if err != nil || withdrawalId <= 0 {
		common.ApiError(c, errors.New("invalid withdrawal id"))
		return
	}
	remark, ok := invitationCommissionAdminRemark(c)
	if !ok {
		return
	}
	if err := service.CompleteInvitationCommissionWithdrawal(withdrawalId, c.GetInt("id"), remark); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"status": "completed"})
}

func AdminRejectInvitationCommissionWithdrawal(c *gin.Context) {
	withdrawalId, err := strconv.Atoi(c.Param("id"))
	if err != nil || withdrawalId <= 0 {
		common.ApiError(c, errors.New("invalid withdrawal id"))
		return
	}
	remark, ok := invitationCommissionAdminRemark(c)
	if !ok {
		return
	}
	if err := service.RejectInvitationCommissionWithdrawal(withdrawalId, c.GetInt("id"), remark); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"status": "rejected"})
}

func invitationCommissionPageQuery(c *gin.Context) (int, int) {
	return invitationCommissionIntQuery(c, "page"), invitationCommissionIntQuery(c, "page_size")
}

func invitationCommissionIntQuery(c *gin.Context, key string) int {
	value, err := strconv.Atoi(strings.TrimSpace(c.Query(key)))
	if err != nil {
		return 0
	}
	return value
}

func normalizeInvitationCommissionContact(contact service.InvitationCommissionContact) (service.InvitationCommissionContact, error) {
	contact.Type = strings.ToLower(strings.TrimSpace(contact.Type))
	contact.Value = strings.TrimSpace(contact.Value)
	switch contact.Type {
	case "wechat", "telegram", "email", "other":
	default:
		return service.InvitationCommissionContact{}, errors.New("invalid invitation commission contact type")
	}
	if contact.Value == "" || len([]rune(contact.Value)) > 128 {
		return service.InvitationCommissionContact{}, errors.New("invalid invitation commission contact value")
	}
	return contact, nil
}

func invitationCommissionAdminRemark(c *gin.Context) (string, bool) {
	var req adminInvitationCommissionWithdrawalActionRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiError(c, err)
		return "", false
	}
	remark := strings.TrimSpace(req.AdminRemark)
	if remark == "" || len([]rune(remark)) > 500 {
		common.ApiError(c, errors.New("invalid admin_remark"))
		return "", false
	}
	return remark, true
}
