package controller

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

type redemptionBatchRequest struct {
	Ids []int `json:"ids"`
}

func GetAllRedemptions(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	redemptions, total, err := model.ListRedemptions(redemptionListOptionsFromQuery(c, pageInfo.GetStartIdx(), pageInfo.GetPageSize()))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(redemptions)
	common.ApiSuccess(c, pageInfo)
	return
}

func SearchRedemptions(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	redemptions, total, err := model.ListRedemptions(redemptionListOptionsFromQuery(c, pageInfo.GetStartIdx(), pageInfo.GetPageSize()))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(redemptions)
	common.ApiSuccess(c, pageInfo)
	return
}

func redemptionListOptionsFromQuery(c *gin.Context, startIdx int, pageSize int) model.RedemptionListOptions {
	status, _ := strconv.Atoi(c.Query("status"))
	keyword := strings.TrimSpace(c.Query("keyword"))
	if keyword == "" {
		keyword = strings.TrimSpace(c.Query("search"))
	}
	return model.RedemptionListOptions{
		Keyword:  keyword,
		Type:     strings.TrimSpace(c.Query("type")),
		Status:   status,
		BatchId:  strings.TrimSpace(c.Query("batch_id")),
		StartIdx: startIdx,
		Num:      pageSize,
	}
}

func GetRedemptionsByBatch(c *gin.Context) {
	batchId := strings.TrimSpace(c.Param("batch_id"))
	redemptions, err := model.GetRedemptionsByBatchId(batchId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, redemptions)
}

func GetRedemption(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	redemption, err := model.GetRedemptionById(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    redemption,
	})
	return
}

func AddRedemption(c *gin.Context) {
	redemption := model.Redemption{}
	err := c.ShouldBindJSON(&redemption)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if utf8.RuneCountInString(redemption.Name) == 0 || utf8.RuneCountInString(redemption.Name) > 20 {
		common.ApiErrorI18n(c, i18n.MsgRedemptionNameLength)
		return
	}
	if redemption.Count <= 0 {
		common.ApiErrorI18n(c, i18n.MsgRedemptionCountPositive)
		return
	}
	if redemption.Count > 100 {
		common.ApiErrorI18n(c, i18n.MsgRedemptionCountMax)
		return
	}
	if valid, msg := validateExpiredTime(c, redemption.ExpiredTime); !valid {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": msg})
		return
	}
	redemptions, err := buildRedemptionsForCreate(c.GetInt("id"), redemption, common.GetUUID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	keys := make([]string, 0, len(redemptions))
	for i := range redemptions {
		err = redemptions[i].Insert()
		if err != nil {
			common.SysError("failed to insert redemption: " + err.Error())
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": i18n.T(c, i18n.MsgRedemptionCreateFailed),
				"data":    keys,
			})
			return
		}
		keys = append(keys, redemptions[i].Key)
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    keys,
	})
	return
}

func DeleteRedemption(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	err := model.DeleteRedemptionById(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
	return
}

func UpdateRedemption(c *gin.Context) {
	statusOnly := c.Query("status_only")
	redemption := model.Redemption{}
	err := c.ShouldBindJSON(&redemption)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	cleanRedemption, err := model.GetRedemptionById(redemption.Id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if statusOnly == "" {
		if valid, msg := validateExpiredTime(c, redemption.ExpiredTime); !valid {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": msg})
			return
		}
		if err := applyRedemptionUpdate(cleanRedemption, redemption); err != nil {
			common.ApiError(c, err)
			return
		}
	}
	if statusOnly != "" {
		cleanRedemption.Status = redemption.Status
	}
	err = cleanRedemption.Update()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    cleanRedemption,
	})
	return
}

func DeleteInvalidRedemption(c *gin.Context) {
	rows, err := model.DeleteInvalidRedemptions()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    rows,
	})
	return
}

func DeleteAllRedemptions(c *gin.Context) {
	rows, err := model.DeleteAllRedemptions()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    rows,
	})
}

func BatchDeleteRedemptions(c *gin.Context) {
	redemptionBatch := redemptionBatchRequest{}
	if err := c.ShouldBindJSON(&redemptionBatch); err != nil || len(redemptionBatch.Ids) == 0 {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	rows, err := model.BatchDeleteRedemptions(redemptionBatch.Ids)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    rows,
	})
}

func validateExpiredTime(c *gin.Context, expired int64) (bool, string) {
	if expired != 0 && expired < common.GetTimestamp() {
		return false, i18n.T(c, i18n.MsgRedemptionExpireTimeInvalid)
	}
	return true, ""
}

func validateRedemptionWalletCents(cents int) (int, error) {
	if cents <= 0 {
		return 0, errors.New("兑换码金额必须大于0")
	}
	return cents, nil
}

func normalizeRedemptionCreateType(redemptionType string) string {
	if redemptionType == model.RedemptionTypeSubscription {
		return model.RedemptionTypeSubscription
	}
	return model.RedemptionTypeWallet
}

func validateRedemptionSubscriptionPlan(planId int) (*model.SubscriptionPlan, error) {
	if planId <= 0 {
		return nil, errors.New("套餐不存在")
	}
	plan, err := model.GetSubscriptionPlanById(planId)
	if err != nil {
		return nil, errors.New("套餐不存在")
	}
	if strings.TrimSpace(plan.Title) == "" {
		return nil, errors.New("套餐不存在")
	}
	return plan, nil
}

func redemptionSubscriptionPlanSnapshot(plan *model.SubscriptionPlan) (int64, string, error) {
	amountCents, currency, ok := model.SubscriptionPlanAmountSnapshot(plan)
	if !ok {
		return 0, "", errors.New("套餐价格无效")
	}
	return amountCents, currency, nil
}

func buildRedemptionsForCreate(userId int, redemption model.Redemption, nextKey func() string) ([]model.Redemption, error) {
	redemptionType := normalizeRedemptionCreateType(redemption.Type)
	quota := 0
	planId := 0
	var amountCents int64
	currency := ""
	if redemptionType == model.RedemptionTypeSubscription {
		plan, err := validateRedemptionSubscriptionPlan(redemption.PlanId)
		if err != nil {
			return nil, err
		}
		planId = redemption.PlanId
		amountCents, currency, err = redemptionSubscriptionPlanSnapshot(plan)
		if err != nil {
			return nil, err
		}
	} else {
		var err error
		quota, err = validateRedemptionWalletCents(redemption.Quota)
		if err != nil {
			return nil, err
		}
	}
	batchId := common.GetUUID()
	redemptions := make([]model.Redemption, 0, redemption.Count)
	for i := 0; i < redemption.Count; i++ {
		redemptions = append(redemptions, model.Redemption{
			UserId:      userId,
			Name:        redemption.Name,
			Key:         nextKey(),
			CreatedTime: common.GetTimestamp(),
			Quota:       quota,
			Type:        redemptionType,
			PlanId:      planId,
			AmountCents: amountCents,
			Currency:    currency,
			BatchId:     batchId,
			ExpiredTime: redemption.ExpiredTime,
		})
	}
	return redemptions, nil
}

func applyRedemptionUpdate(cleanRedemption *model.Redemption, redemption model.Redemption) error {
	if cleanRedemption == nil {
		return errors.New("兑换码不存在")
	}
	redemptionType := normalizeRedemptionCreateType(redemption.Type)
	quota := 0
	planId := 0
	var amountCents int64
	currency := ""
	if redemptionType == model.RedemptionTypeSubscription {
		planId = redemption.PlanId
		if cleanRedemption.Type == model.RedemptionTypeSubscription && cleanRedemption.PlanId == planId {
			amountCents = cleanRedemption.AmountCents
			currency = cleanRedemption.Currency
		} else {
			plan, err := validateRedemptionSubscriptionPlan(planId)
			if err != nil {
				return err
			}
			amountCents, currency, err = redemptionSubscriptionPlanSnapshot(plan)
			if err != nil {
				return err
			}
		}
	} else {
		var err error
		quota, err = validateRedemptionWalletCents(redemption.Quota)
		if err != nil {
			return err
		}
	}
	if cleanRedemption.Status == common.RedemptionCodeStatusUsed && (cleanRedemption.Type != redemptionType || cleanRedemption.PlanId != planId || cleanRedemption.AmountCents != amountCents || cleanRedemption.Currency != currency) {
		return errors.New("已使用订阅兑换码不可修改套餐快照")
	}
	cleanRedemption.Name = redemption.Name
	cleanRedemption.Quota = quota
	cleanRedemption.Type = redemptionType
	cleanRedemption.PlanId = planId
	cleanRedemption.AmountCents = amountCents
	cleanRedemption.Currency = currency
	cleanRedemption.ExpiredTime = redemption.ExpiredTime
	return nil
}
