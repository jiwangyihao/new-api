package model

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	RedemptionTypeWallet       = "wallet"
	RedemptionTypeSubscription = "subscription"
)

const (
	RedemptionModeTimed         = SubscriptionPurchaseModeTimed
	RedemptionModeCreditBalance = SubscriptionPurchaseModeCreditBalance
)

type RedemptionListOptions struct {
	Keyword  string
	Type     string
	Status   int
	BatchId  string
	StartIdx int
	Num      int
}

type RedemptionResult struct {
	Type                      string                    `json:"type"`
	Quota                     int                       `json:"quota"`
	Plan                      *SubscriptionPlan         `json:"plan,omitempty"`
	RedemptionId              int                       `json:"redemption_id"`
	RedemptionMode            string                    `json:"redemption_mode,omitempty"`
	FulfillmentSubscriptionId int                       `json:"fulfillment_subscription_id,omitempty"`
	CreditBalance             *CreditBalanceGrantResult `json:"credit_balance,omitempty"`
	Replayed                  bool                      `json:"replayed"`
}

type Redemption struct {
	Id                        int               `json:"id"`
	UserId                    int               `json:"user_id"`
	Key                       string            `json:"key" gorm:"type:char(32);uniqueIndex"`
	Status                    int               `json:"status" gorm:"default:1"`
	Name                      string            `json:"name" gorm:"index"`
	Quota                     int               `json:"quota" gorm:"default:100"`
	Type                      string            `json:"type" gorm:"type:varchar(32);not null;default:'wallet';index"`
	PlanId                    int               `json:"plan_id" gorm:"type:int;not null;default:0;index"`
	AmountCents               int64             `json:"amount_cents" gorm:"type:bigint;not null;default:0"`
	Currency                  string            `json:"currency" gorm:"type:varchar(8);not null;default:''"`
	FulfillmentMode           string            `json:"fulfillment_mode,omitempty" gorm:"type:varchar(32);not null;default:'';index"`
	FulfillmentSnapshot       string            `json:"fulfillment_snapshot,omitempty" gorm:"type:text"`
	FulfillmentSubscriptionId int               `json:"fulfillment_subscription_id,omitempty" gorm:"type:int;not null;default:0;index"`
	BatchId                   string            `json:"batch_id" gorm:"type:varchar(36);index"`
	Plan                      *SubscriptionPlan `json:"plan,omitempty" gorm:"-"`
	CreatedTime               int64             `json:"created_time" gorm:"bigint"`
	RedeemedTime              int64             `json:"redeemed_time" gorm:"bigint"`
	Count                     int               `json:"count" gorm:"-:all"` // only for api request
	UsedUserId                int               `json:"used_user_id"`
	DeletedAt                 gorm.DeletedAt    `gorm:"index"`
	ExpiredTime               int64             `json:"expired_time" gorm:"bigint"` // 过期时间，0 表示不过期
}

func GetAllRedemptions(startIdx int, num int) (redemptions []*Redemption, total int64, err error) {
	return ListRedemptions(RedemptionListOptions{StartIdx: startIdx, Num: num})
}

func SearchRedemptions(keyword string, startIdx int, num int) (redemptions []*Redemption, total int64, err error) {
	return ListRedemptions(RedemptionListOptions{Keyword: keyword, StartIdx: startIdx, Num: num})
}

func ListRedemptions(options RedemptionListOptions) (redemptions []*Redemption, total int64, err error) {
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	query := applyRedemptionListFilters(tx.Model(&Redemption{}), options)
	if err = query.Count(&total).Error; err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	if options.Num > 0 {
		query = query.Limit(options.Num).Offset(options.StartIdx)
	}
	if err = query.Order("id desc").Find(&redemptions).Error; err != nil {
		tx.Rollback()
		return nil, 0, err
	}
	attachRedemptionPlans(tx, redemptions)

	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}
	return redemptions, total, nil
}

func applyRedemptionListFilters(query *gorm.DB, options RedemptionListOptions) *gorm.DB {
	keyword := strings.TrimSpace(options.Keyword)
	if keyword != "" {
		likeKeyword := keyword + "%"
		if id, err := strconv.Atoi(keyword); err == nil {
			query = query.Where("id = ? OR name LIKE ? OR "+commonKeyCol+" = ? OR batch_id = ?", id, likeKeyword, keyword, keyword)
		} else {
			query = query.Where("name LIKE ? OR "+commonKeyCol+" = ? OR batch_id = ?", likeKeyword, keyword, keyword)
		}
	}

	if redemptionType := normalizeRedemptionTypeFilter(options.Type); redemptionType != "" {
		query = query.Where("type = ?", redemptionType)
	}
	if options.Status > 0 {
		query = query.Where("status = ?", options.Status)
	}
	if batchId := strings.TrimSpace(options.BatchId); batchId != "" {
		query = query.Where("batch_id = ?", batchId)
	}
	return query
}

func normalizeRedemptionTypeFilter(redemptionType string) string {
	switch redemptionType {
	case RedemptionTypeWallet, RedemptionTypeSubscription:
		return redemptionType
	default:
		return ""
	}
}

func GetRedemptionsByBatchId(batchId string) (redemptions []*Redemption, err error) {
	batchId = strings.TrimSpace(batchId)
	if batchId == "" {
		return nil, errors.New("batch_id 为空！")
	}
	if err = DB.Where("batch_id = ?", batchId).Order("id desc").Find(&redemptions).Error; err != nil {
		return nil, err
	}
	attachRedemptionPlans(DB, redemptions)
	return redemptions, nil
}
func GetRedemptionById(id int) (*Redemption, error) {
	if id == 0 {
		return nil, errors.New("id 为空！")
	}
	redemption := Redemption{Id: id}
	var err error = nil
	err = DB.First(&redemption, "id = ?", id).Error
	if err == nil {
		attachRedemptionPlans(DB, []*Redemption{&redemption})
	}
	return &redemption, err
}

func attachRedemptionPlans(tx *gorm.DB, redemptions []*Redemption) {
	if tx == nil || len(redemptions) == 0 {
		return
	}
	planIDSet := make(map[int]struct{})
	planIDs := make([]int, 0)
	for _, redemption := range redemptions {
		if redemption == nil || normalizeRedemptionType(redemption.Type) != RedemptionTypeSubscription || redemption.PlanId <= 0 {
			continue
		}
		if _, ok := planIDSet[redemption.PlanId]; ok {
			continue
		}
		planIDSet[redemption.PlanId] = struct{}{}
		planIDs = append(planIDs, redemption.PlanId)
	}
	if len(planIDs) == 0 {
		return
	}
	var plans []SubscriptionPlan
	if err := tx.Where("id IN ?", planIDs).Find(&plans).Error; err != nil {
		return
	}
	plansByID := make(map[int]*SubscriptionPlan, len(plans))
	for i := range plans {
		plansByID[plans[i].Id] = &plans[i]
	}
	for _, redemption := range redemptions {
		if redemption == nil {
			continue
		}
		redemption.Plan = plansByID[redemption.PlanId]
	}
}

func normalizeRedemptionType(redemptionType string) string {
	if redemptionType == RedemptionTypeSubscription {
		return RedemptionTypeSubscription
	}
	return RedemptionTypeWallet
}

type RedemptionFulfillmentSnapshot struct {
	Entitlement    SubscriptionEntitlementSnapshot `json:"entitlement"`
	CreditBalance  *CreditBalanceGrantResult       `json:"credit_balance,omitempty"`
	EventStartTime int64                           `json:"event_start_time,omitempty"`
	EventEndTime   int64                           `json:"event_end_time,omitempty"`
}

func normalizeRedemptionMode(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", ErrRedemptionModeRequired
	}
	switch value {
	case RedemptionModeTimed, RedemptionModeCreditBalance:
		return value, nil
	default:
		return "", ErrRedemptionModeInvalid
	}
}

func Redeem(key string, userId int, redemptionMode string) (*RedemptionResult, error) {
	if strings.TrimSpace(key) == "" {
		return nil, errors.New("未提供兑换码")
	}
	if userId <= 0 {
		return nil, errors.New("无效的 user id")
	}

	keyCol := "`key`"
	if common.UsingPostgreSQL {
		keyCol = `"key"`
	}
	common.RandomSleep()
	var result *RedemptionResult
	var fulfilled *Redemption
	err := transactionWithUserSettingCASRetry(func(tx *gorm.DB) error {
		result = nil
		fulfilled = nil
		var redemption Redemption
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where(keyCol+" = ?", key).First(&redemption).Error; err != nil {
			return errors.New("无效的兑换码")
		}
		redemptionType := normalizeRedemptionType(redemption.Type)
		mode := ""
		if redemptionType == RedemptionTypeSubscription {
			var err error
			mode, err = normalizeRedemptionMode(redemptionMode)
			if err != nil {
				return err
			}
		}
		if redemption.Status != common.RedemptionCodeStatusEnabled {
			replay, err := redemptionResultFromFulfillment(&redemption, userId)
			result = replay
			if err != nil {
				return err
			}
			fulfilled = &redemption
			return nil
		}
		if redemption.ExpiredTime != 0 && redemption.ExpiredTime < common.GetTimestamp() {
			return errors.New("该兑换码已过期")
		}

		redeemedTime := getDBTimestampTx(tx)
		claim := tx.Model(&Redemption{}).
			Where("id = ? AND status = ?", redemption.Id, common.RedemptionCodeStatusEnabled).
			Updates(map[string]any{
				"status":           common.RedemptionCodeStatusUsed,
				"used_user_id":     userId,
				"redeemed_time":    redeemedTime,
				"fulfillment_mode": mode,
			})
		if claim.Error != nil {
			return claim.Error
		}
		if claim.RowsAffected == 0 {
			var claimed Redemption
			if err := tx.Where("id = ?", redemption.Id).First(&claimed).Error; err != nil {
				return err
			}
			replay, err := redemptionResultFromFulfillment(&claimed, userId)
			result = replay
			if err != nil {
				return err
			}
			fulfilled = &claimed
			return nil
		}

		redemption.Status = common.RedemptionCodeStatusUsed
		redemption.UsedUserId = userId
		redemption.RedeemedTime = redeemedTime
		redemption.FulfillmentMode = mode
		current := &RedemptionResult{
			Type:           redemptionType,
			RedemptionId:   redemption.Id,
			RedemptionMode: mode,
		}
		if redemptionType == RedemptionTypeWallet {
			if err := IncreaseUserAccountBalanceTx(tx, userId, redemption.Quota); err != nil {
				return err
			}
			current.Quota = redemption.Quota
			result = current
			fulfilled = &redemption
			return nil
		}

		currentPlan, err := getRedemptionPlanTx(tx, redemption.PlanId)
		if err != nil {
			return err
		}
		fulfillment, plan, err := redemptionFulfillmentFromSourceSnapshot(&redemption, currentPlan, mode)
		if err != nil {
			return err
		}
		sourceSnapshotPayload, err := common.Marshal(fulfillment)
		if err != nil {
			return err
		}
		redemption.FulfillmentSnapshot = string(sourceSnapshotPayload)
		if err := tx.Model(&Redemption{}).Where("id = ?", redemption.Id).Update("fulfillment_snapshot", redemption.FulfillmentSnapshot).Error; err != nil {
			return err
		}
		if mode == RedemptionModeCreditBalance {
			creditPlan, err := GetCreditBalancePlanTx(tx)
			if err != nil {
				return ErrCreditBalanceRedemptionUnavailable
			}
			if err := ValidateCreditBalanceRedemptionOption(plan, creditPlan); err != nil {
				return err
			}
			fulfillment.Entitlement.TargetCreditBalancePlanID = creditPlan.Id
			sourceSnapshot, err := MarshalSubscriptionEntitlementSnapshot(fulfillment.Entitlement)
			if err != nil {
				return err
			}
			grant, err := GrantCreditBalanceTx(tx, CreditBalanceGrantRequest{
				UserId:         userId,
				GrossCredit:    plan.MonthlyTokenLimit,
				IdempotencyKey: fmt.Sprintf("redemption:%d", redemption.Id),
				SourceType:     CreditBalanceLedgerSourceRedemption,
				SourceId:       redemption.Id,
				Type:           CreditBalanceLedgerTypeRedemption,
				SourceSnapshot: sourceSnapshot,
				TargetPlanId:   creditPlan.Id,
				Reason:         "兑换码兑换 Credit 余额",
			})
			if err != nil {
				return err
			}
			fulfillment.CreditBalance = grant
			current.CreditBalance = grant
			current.FulfillmentSubscriptionId = grant.UserSubscriptionId
		} else {
			var creation *UserSubscriptionCreationResult
			if plan.IsTrial || plan.InviteTrial {
				creation, err = CreateUserSubscriptionFromPlanWithResultTx(tx, userId, plan, SubscriptionGrantRedemption)
			} else {
				if !currentPlan.Enabled {
					return ErrRedemptionPlanIneligible
				}
				creation, err = GrantTimedSubscriptionTx(tx, TimedSubscriptionGrantRequest{
					UserId:         userId,
					PlanId:         plan.Id,
					IdempotencyKey: TimedSubscriptionGrantSourceRedemption + ":" + strconv.Itoa(redemption.Id),
					SourceType:     TimedSubscriptionGrantSourceRedemption,
					SourceId:       redemption.Id,
				})
			}
			if err != nil {
				return err
			}
			if err := createInvitationRewardEventForSubscriptionRedemptionTx(tx, &redemption, userId, plan, creation); err != nil {
				return err
			}
			if creation != nil {
				fulfillment.EventStartTime = creation.EventStartTime
				fulfillment.EventEndTime = creation.EventEndTime
				if creation.Subscription != nil {
					current.FulfillmentSubscriptionId = creation.Subscription.Id
				}
			}
		}
		current.Plan = plan
		snapshotPayload, err := common.Marshal(fulfillment)
		if err != nil {
			return err
		}
		redemption.FulfillmentSnapshot = string(snapshotPayload)
		redemption.FulfillmentSubscriptionId = current.FulfillmentSubscriptionId
		if err := tx.Model(&Redemption{}).Where("id = ?", redemption.Id).Updates(map[string]any{
			"fulfillment_snapshot":        redemption.FulfillmentSnapshot,
			"fulfillment_subscription_id": redemption.FulfillmentSubscriptionId,
		}).Error; err != nil {
			return err
		}
		result = current
		fulfilled = &redemption
		return nil
	})
	if err != nil {
		if isPublicRedemptionError(err) {
			return result, err
		}
		common.SysError("redemption failed: " + err.Error())
		return nil, ErrRedeemFailed
	}
	if result == nil || fulfilled == nil {
		return nil, ErrRedeemFailed
	}
	if result.Replayed {
		return result, nil
	}
	if result.Type == RedemptionTypeSubscription {
		if result.RedemptionMode == RedemptionModeCreditBalance {
			RecordLog(userId, LogTypeTopup, fmt.Sprintf("通过兑换码兑换 Credit 余额，兑换码ID %d", fulfilled.Id))
		} else {
			planTitle := ""
			if result.Plan != nil {
				planTitle = result.Plan.Title
			}
			RecordLog(userId, LogTypeTopup, fmt.Sprintf("通过兑换码兑换订阅套餐 %s，兑换码ID %d", planTitle, fulfilled.Id))
		}
	} else {
		invalidateUserCacheBestEffort(userId)
		amountCNY := AccountBalanceCNYFromCents(fulfilled.Quota).StringFixed(2)
		RecordLog(userId, LogTypeTopup, fmt.Sprintf("通过兑换码充值 %s，兑换码ID %d", amountCNY, fulfilled.Id))
	}
	return result, nil
}

func getRedemptionPlanTx(tx *gorm.DB, planId int) (*SubscriptionPlan, error) {
	if tx == nil || planId <= 0 {
		return nil, errors.New("invalid redemption plan")
	}
	var plan SubscriptionPlan
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", planId).First(&plan).Error; err != nil {
		return nil, err
	}
	return &plan, nil
}

func redemptionFulfillmentFromSourceSnapshot(redemption *Redemption, currentPlan *SubscriptionPlan, mode string) (RedemptionFulfillmentSnapshot, *SubscriptionPlan, error) {
	if redemption == nil || currentPlan == nil || redemption.PlanId <= 0 || currentPlan.Id != redemption.PlanId {
		return RedemptionFulfillmentSnapshot{}, nil, ErrRedemptionPlanIneligible
	}
	fulfillment := RedemptionFulfillmentSnapshot{}
	if payload := strings.TrimSpace(redemption.FulfillmentSnapshot); payload != "" {
		if err := common.UnmarshalJsonStr(payload, &fulfillment); err != nil {
			return RedemptionFulfillmentSnapshot{}, nil, ErrRedemptionPlanIneligible
		}
	}
	if fulfillment.Entitlement.PlanID == 0 {
		fulfillment.Entitlement = NewSubscriptionEntitlementSnapshot(currentPlan, mode, 0)
	}
	if fulfillment.Entitlement.PlanID != redemption.PlanId {
		return RedemptionFulfillmentSnapshot{}, nil, ErrRedemptionPlanIneligible
	}
	fulfillment.Entitlement.PurchaseMode = mode
	fulfillment.CreditBalance = nil
	fulfillment.EventStartTime = 0
	fulfillment.EventEndTime = 0
	plan, err := SubscriptionPlanFromEntitlementSnapshot(fulfillment.Entitlement)
	if err != nil {
		return RedemptionFulfillmentSnapshot{}, nil, ErrRedemptionPlanIneligible
	}
	return fulfillment, plan, nil
}

func redemptionResultFromFulfillment(redemption *Redemption, userId int) (*RedemptionResult, error) {
	if redemption == nil || redemption.Status != common.RedemptionCodeStatusUsed {
		return nil, ErrRedemptionAlreadyUsed
	}
	result := &RedemptionResult{
		Type:           normalizeRedemptionType(redemption.Type),
		RedemptionId:   redemption.Id,
		RedemptionMode: strings.TrimSpace(redemption.FulfillmentMode),
		Replayed:       true,
	}
	if redemption.UsedUserId != userId {
		return result, ErrRedemptionAlreadyUsed
	}
	if result.Type == RedemptionTypeWallet {
		return nil, ErrRedeemFailed
	}
	if result.RedemptionMode == "" || strings.TrimSpace(redemption.FulfillmentSnapshot) == "" {
		return result, ErrRedemptionAlreadyUsed
	}
	var fulfillment RedemptionFulfillmentSnapshot
	if err := common.UnmarshalJsonStr(redemption.FulfillmentSnapshot, &fulfillment); err != nil {
		return nil, err
	}
	plan, err := SubscriptionPlanFromEntitlementSnapshot(fulfillment.Entitlement)
	if err != nil {
		return nil, err
	}
	result.Plan = plan
	result.CreditBalance = fulfillment.CreditBalance
	result.FulfillmentSubscriptionId = redemption.FulfillmentSubscriptionId
	return result, nil
}

func isPublicRedemptionError(err error) bool {
	return errors.Is(err, ErrRedemptionModeRequired) ||
		errors.Is(err, ErrRedemptionModeInvalid) ||
		errors.Is(err, ErrCreditBalanceRedemptionUnavailable) ||
		errors.Is(err, ErrRedemptionPlanIneligible) ||
		errors.Is(err, ErrRedemptionAlreadyUsed)
}

func (redemption *Redemption) Insert() error {
	if redemption == nil {
		return errors.New("invalid redemption")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if normalizeRedemptionType(redemption.Type) == RedemptionTypeSubscription {
			plan, err := getRedemptionPlanTx(tx, redemption.PlanId)
			if err != nil {
				return err
			}
			snapshotPayload, err := common.Marshal(RedemptionFulfillmentSnapshot{
				Entitlement: NewSubscriptionEntitlementSnapshot(plan, "", 0),
			})
			if err != nil {
				return err
			}
			redemption.FulfillmentSnapshot = string(snapshotPayload)
		}
		return tx.Create(redemption).Error
	})
}

func (redemption *Redemption) SelectUpdate() error {
	// This can update zero values
	return DB.Model(redemption).Select("redeemed_time", "status").Updates(redemption).Error
}

// Update Make sure your token's fields is completed, because this will update non-zero values
func (redemption *Redemption) Update() error {
	var err error
	err = DB.Model(redemption).Select("name", "status", "quota", "type", "plan_id", "amount_cents", "currency", "redeemed_time", "expired_time", "batch_id").Updates(redemption).Error
	return err
}

func (redemption *Redemption) Delete() error {
	var err error
	err = DB.Delete(redemption).Error
	return err
}

func DeleteRedemptionById(id int) (err error) {
	if id == 0 {
		return errors.New("id 为空！")
	}
	redemption := Redemption{Id: id}
	err = DB.Where(redemption).First(&redemption).Error
	if err != nil {
		return err
	}
	return redemption.Delete()
}

func DeleteInvalidRedemptions() (int64, error) {
	now := common.GetTimestamp()
	result := DB.Where("status IN ? OR (status = ? AND expired_time != 0 AND expired_time < ?)", []int{common.RedemptionCodeStatusUsed, common.RedemptionCodeStatusDisabled}, common.RedemptionCodeStatusEnabled, now).Delete(&Redemption{})
	return result.RowsAffected, result.Error
}

func BatchDeleteRedemptions(ids []int) (int64, error) {
	if len(ids) == 0 {
		return 0, errors.New("ids 不能为空！")
	}
	result := DB.Where("id IN ?", ids).Delete(&Redemption{})
	return result.RowsAffected, result.Error
}

func DeleteAllRedemptions() (int64, error) {
	result := DB.Where("1 = 1").Delete(&Redemption{})
	return result.RowsAffected, result.Error
}
