package controller

import (
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

type adminTrialCodeRequest struct {
	Code           string `json:"code"`
	PlanId         int    `json:"plan_id"`
	Enabled        bool   `json:"enabled"`
	MaxRedemptions int    `json:"max_redemptions"`
	ExpiresAt      int64  `json:"expires_at"`
}

func AdminListTrialCodes(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	var total int64
	query := model.DB.Model(&model.TrialCode{})
	filter := strings.TrimSpace(c.Query("filter"))
	if filter != "" {
		like := "%" + filter + "%"
		query = query.Where("code LIKE ?", like)
		if id, err := strconv.Atoi(filter); err == nil {
			query = model.DB.Model(&model.TrialCode{}).Where("code LIKE ? OR id = ? OR plan_id = ?", like, id, id)
		}
	}
	if err := query.Count(&total).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	var trialCodes []model.TrialCode
	if err := query.Order("id desc").Offset(pageInfo.GetStartIdx()).Limit(pageInfo.GetPageSize()).Find(&trialCodes).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(trialCodes)
	common.ApiSuccess(c, pageInfo)
}

func AdminCreateTrialCode(c *gin.Context) {
	var req adminTrialCodeRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	if msg := validateAdminTrialCodeRequest(req, true); msg != "" {
		common.ApiErrorMsg(c, msg)
		return
	}
	trialCode := &model.TrialCode{Code: req.Code, PlanId: req.PlanId, Enabled: req.Enabled, MaxRedemptions: req.MaxRedemptions, ExpiresAt: req.ExpiresAt}
	if err := model.DB.Select("Code", "PlanId", "Enabled", "MaxRedemptions", "ExpiresAt").Create(trialCode).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, trialCode)
}

func AdminUpdateTrialCode(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	if id <= 0 {
		common.ApiErrorMsg(c, "无效的ID")
		return
	}
	var req adminTrialCodeRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	if msg := validateAdminTrialCodeRequest(req, true); msg != "" {
		common.ApiErrorMsg(c, msg)
		return
	}
	var trialCode model.TrialCode
	if err := model.DB.First(&trialCode, id).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	trialCode.Code = strings.TrimSpace(req.Code)
	trialCode.PlanId = req.PlanId
	trialCode.Enabled = req.Enabled
	trialCode.MaxRedemptions = req.MaxRedemptions
	trialCode.ExpiresAt = req.ExpiresAt
	if err := model.DB.Save(&trialCode).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, trialCode)
}

func AdminUpdateTrialCodeStatus(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	if id <= 0 {
		common.ApiErrorMsg(c, "无效的ID")
		return
	}
	var req struct {
		Enabled *bool `json:"enabled"`
	}
	if err := common.DecodeJson(c.Request.Body, &req); err != nil || req.Enabled == nil {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	if err := model.DB.Model(&model.TrialCode{}).Where("id = ?", id).Update("enabled", *req.Enabled).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"id": id, "enabled": *req.Enabled})
}

func AdminDeleteTrialCode(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	if id <= 0 {
		common.ApiErrorMsg(c, "无效的ID")
		return
	}
	if err := model.DB.Delete(&model.TrialCode{}, id).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"id": id})
}

func validateAdminTrialCodeRequest(req adminTrialCodeRequest, requireCode bool) string {
	if requireCode && strings.TrimSpace(req.Code) == "" {
		return "试用码不能为空"
	}
	if req.PlanId <= 0 {
		return "试用套餐无效"
	}
	if req.MaxRedemptions < 0 {
		return "最大兑换次数不能为负数"
	}
	var plan model.SubscriptionPlan
	if err := model.DB.Where("id = ?", req.PlanId).First(&plan).Error; err != nil {
		return "试用套餐不存在"
	}
	if !plan.IsTrial {
		return "试用码必须绑定试用套餐"
	}
	return ""
}
