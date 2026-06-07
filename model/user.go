package model

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/bytedance/gopkg/util/gopool"
	"gorm.io/gorm"
)

const UserNameMaxLength = 20

const (
	InvitationRewardModeSubscription = "subscription"
	InvitationRewardModeCommission   = "commission"
)

func NormalizeInvitationRewardMode(mode string) string {
	switch strings.TrimSpace(mode) {
	case InvitationRewardModeCommission:
		return InvitationRewardModeCommission
	default:
		return InvitationRewardModeSubscription
	}
}

func (user *User) NormalizedInvitationRewardMode() string {
	if user == nil {
		return InvitationRewardModeSubscription
	}
	return NormalizeInvitationRewardMode(user.InvitationRewardMode)
}

// User if you add sensitive fields, don't forget to clean them in setupLogin function.
// Otherwise, the sensitive information will be saved on local storage in plain text!
type User struct {
	Id                                             int            `json:"id"`
	Username                                       string         `json:"username" gorm:"unique;index" validate:"max=20"`
	Password                                       string         `json:"password" gorm:"not null;" validate:"min=8,max=20"`
	OriginalPassword                               string         `json:"original_password" gorm:"-:all"` // this field is only for Password change verification, don't save it to database!
	DisplayName                                    string         `json:"display_name" gorm:"index" validate:"max=20"`
	Role                                           int            `json:"role" gorm:"type:int;default:1"`   // admin, common
	Status                                         int            `json:"status" gorm:"type:int;default:1"` // enabled, disabled
	Email                                          string         `json:"email" gorm:"index" validate:"max=50"`
	GitHubId                                       string         `json:"github_id" gorm:"column:github_id;index"`
	DiscordId                                      string         `json:"discord_id" gorm:"column:discord_id;index"`
	OidcId                                         string         `json:"oidc_id" gorm:"column:oidc_id;index"`
	WeChatId                                       string         `json:"wechat_id" gorm:"column:wechat_id;index"`
	TelegramId                                     string         `json:"telegram_id" gorm:"column:telegram_id;index"`
	VerificationCode                               string         `json:"verification_code" gorm:"-:all"`                                    // this field is only for Email verification, don't save it to database!
	AccessToken                                    *string        `json:"access_token" gorm:"type:char(32);column:access_token;uniqueIndex"` // this token is for system management
	Quota                                          int            `json:"quota" gorm:"type:int;default:0"`
	UsedQuota                                      int            `json:"used_quota" gorm:"type:int;default:0;column:used_quota"` // used quota
	RequestCount                                   int            `json:"request_count" gorm:"type:int;default:0;"`               // request number
	Group                                          string         `json:"-" gorm:"type:varchar(64);default:''"`
	AffCode                                        string         `json:"aff_code" gorm:"type:varchar(32);column:aff_code;uniqueIndex"`
	AffCount                                       int            `json:"aff_count" gorm:"type:int;default:0;column:aff_count"`
	AffQuota                                       int            `json:"aff_quota" gorm:"type:int;default:0;column:aff_quota"`           // 邀请剩余额度
	AffHistoryQuota                                int            `json:"aff_history_quota" gorm:"type:int;default:0;column:aff_history"` // 邀请历史额度
	InviterId                                      int            `json:"inviter_id" gorm:"type:int;column:inviter_id;index"`
	InvitationRewardMode                           string         `json:"invitation_reward_mode" gorm:"type:varchar(32);default:'subscription'"`
	DeletedAt                                      gorm.DeletedAt `gorm:"index"`
	LinuxDOId                                      string         `json:"linux_do_id" gorm:"column:linux_do_id;index"`
	Setting                                        string         `json:"setting" gorm:"type:text;column:setting"`
	Remark                                         string         `json:"remark,omitempty" gorm:"type:varchar(255)" validate:"max=255"`
	StripeCustomer                                 string         `json:"stripe_customer" gorm:"type:varchar(64);column:stripe_customer;index"`
	CreatedAt                                      int64          `json:"created_at" gorm:"autoCreateTime;column:created_at"`
	LastLoginAt                                    int64          `json:"last_login_at" gorm:"default:0;column:last_login_at"`
	DirectInviteCount                              int            `json:"direct_invite_count" gorm:"-:all"`
	QualifiedPaidInviteCount                       int            `json:"qualified_paid_invite_count" gorm:"-:all"`
	InvitationRewardStatus                         string         `json:"invitation_reward_status" gorm:"-:all"`
	InvitationCommissionAvailableCents             int64          `json:"invitation_commission_available_cents" gorm:"-:all"`
	InvitationCommissionPendingCents               int64          `json:"invitation_commission_pending_cents" gorm:"-:all"`
	InvitationCommissionWithdrawnCents             int64          `json:"invitation_commission_withdrawn_cents" gorm:"-:all"`
	InvitationCommissionTransferredCents           int64          `json:"invitation_commission_transferred_cents" gorm:"-:all"`
	InvitationCommissionEarnedCents                int64          `json:"invitation_commission_earned_cents" gorm:"-:all"`
	InvitationCommissionEstimatedCents             int64          `json:"invitation_commission_estimated_cents" gorm:"-:all"`
	InvitationCommissionEstimatedSourceAmountCents int64          `json:"invitation_commission_estimated_source_amount_cents" gorm:"-:all"`
	InvitationCommissionEstimatedEventCount        int            `json:"invitation_commission_estimated_event_count" gorm:"-:all"`
	InvitationRewardPlanTitle                      string         `json:"invitation_reward_plan_title" gorm:"-:all"`
	RewardPlanId                                   int            `json:"reward_plan_id" gorm:"-:all"`
	RewardPlanTitle                                string         `json:"reward_plan_title" gorm:"-:all"`
	RewardPlanBusinessCode                         string         `json:"reward_plan_business_code" gorm:"-:all"`
	RewardTierRank                                 int            `json:"reward_tier_rank" gorm:"-:all"`
	RewardTierQualifiedCount                       int            `json:"reward_tier_qualified_count" gorm:"-:all"`
	DowngradeRewardPlanId                          int            `json:"downgrade_reward_plan_id" gorm:"-:all"`
	DowngradeRewardPlanTitle                       string         `json:"downgrade_reward_plan_title" gorm:"-:all"`
	DowngradeRewardPlanBusinessCode                string         `json:"downgrade_reward_plan_business_code" gorm:"-:all"`
	DowngradeRewardTierRank                        int            `json:"downgrade_reward_tier_rank" gorm:"-:all"`
	DowngradeRewardTierQualifiedCount              int            `json:"downgrade_reward_tier_qualified_count" gorm:"-:all"`
	DowngradeEntitlementEndTime                    int64          `json:"downgrade_entitlement_end_time" gorm:"-:all"`
}

func (user *User) ToBaseUser() *UserBase {
	cache := &UserBase{
		Id:       user.Id,
		Quota:    user.Quota,
		Status:   user.Status,
		Username: user.Username,
		Setting:  user.Setting,
		Email:    user.Email,
	}
	return cache
}

func (user *User) GetAccessToken() string {
	if user.AccessToken == nil {
		return ""
	}
	return *user.AccessToken
}

func (user *User) SetAccessToken(token string) {
	user.AccessToken = &token
}

func (user *User) GetSetting() dto.UserSetting {
	setting := dto.UserSetting{}
	if user.Setting != "" {
		err := common.Unmarshal([]byte(user.Setting), &setting)
		if err != nil {
			common.SysLog("failed to unmarshal setting: " + err.Error())
		}
	}
	return setting
}

func (user *User) SetSetting(setting dto.UserSetting) {
	settingBytes, err := common.Marshal(setting)
	if err != nil {
		common.SysLog("failed to marshal setting: " + err.Error())
		return
	}
	user.Setting = string(settingBytes)
}

// 根据用户角色生成默认的边栏配置
func generateDefaultSidebarConfigForRole(userRole int) string {
	defaultConfig := map[string]interface{}{}

	// 聊天区域 - 所有用户都可以访问
	defaultConfig["chat"] = map[string]interface{}{
		"enabled":    true,
		"playground": true,
		"chat":       true,
	}

	// 控制台区域 - 所有用户都可以访问
	defaultConfig["console"] = map[string]interface{}{
		"enabled":    true,
		"detail":     true,
		"token":      true,
		"log":        true,
		"midjourney": true,
		"task":       true,
	}

	// 个人中心区域 - 所有用户都可以访问
	defaultConfig["personal"] = map[string]interface{}{
		"enabled":  true,
		"topup":    true,
		"personal": true,
	}

	// 管理员区域 - 根据角色决定
	if userRole == common.RoleAdminUser {
		// 管理员可以访问管理员区域，但不能访问系统设置
		defaultConfig["admin"] = map[string]interface{}{
			"enabled":               true,
			"channel":               true,
			"models":                true,
			"redemption":            true,
			"trial_code":            true,
			"trial_abuse":           true,
			"subscription":          true,
			"user":                  true,
			"invitation_commission": true,
			"setting":               false, // 管理员不能访问系统设置
		}
	} else if userRole == common.RoleRootUser {
		// 超级管理员可以访问所有功能
		defaultConfig["admin"] = map[string]interface{}{
			"enabled":               true,
			"channel":               true,
			"models":                true,
			"redemption":            true,
			"trial_code":            true,
			"subscription":          true,
			"trial_abuse":           true,
			"user":                  true,
			"invitation_commission": true,
			"setting":               true,
		}
	}
	// 普通用户不包含admin区域

	// 转换为JSON字符串
	configBytes, err := common.Marshal(defaultConfig)
	if err != nil {
		common.SysLog("生成默认边栏配置失败: " + err.Error())
		return ""
	}

	return string(configBytes)
}

// CheckUserExistOrDeleted check if user exist or deleted, if not exist, return false, nil, if deleted or exist, return true, nil
func CheckUserExistOrDeleted(username string, email string) (bool, error) {
	var user User

	// err := DB.Unscoped().First(&user, "username = ? or email = ?", username, email).Error
	// check email if empty
	var err error
	if email == "" {
		err = DB.Unscoped().First(&user, "username = ?", username).Error
	} else {
		err = DB.Unscoped().First(&user, "username = ? or email = ?", username, email).Error
	}
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// not exist, return false, nil
			return false, nil
		}
		// other error, return false, err
		return false, err
	}
	// exist, return true, nil
	return true, nil
}

func GetMaxUserId() int {
	var user User
	DB.Unscoped().Last(&user)
	return user.Id
}

func GetAllUsers(pageInfo *common.PageInfo) (users []*User, total int64, err error) {
	// Start transaction
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Get total count within transaction
	err = tx.Unscoped().Model(&User{}).Count(&total).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// Get paginated users within same transaction
	err = tx.Unscoped().Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Omit("password", "group").Find(&users).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}
	if err = fillUserInvitationSummariesTx(tx, users); err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// Commit transaction
	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}

	return users, total, nil
}

func SearchUsers(keyword string, _ string, startIdx int, num int) ([]*User, int64, error) {
	var users []*User
	var total int64
	var err error

	// 开始事务
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 构建基础查询
	query := tx.Unscoped().Model(&User{})

	// 构建搜索条件
	likeCondition := "username LIKE ? OR email LIKE ? OR display_name LIKE ?"

	// 尝试将关键字转换为整数ID
	keywordInt, err := strconv.Atoi(keyword)
	if err == nil {
		// 如果是数字，同时搜索ID和其他字段
		likeCondition = "id = ? OR " + likeCondition
		query = query.Where(likeCondition,
			keywordInt, "%"+keyword+"%", "%"+keyword+"%", "%"+keyword+"%")
	} else {
		// 非数字关键字，只搜索字符串字段
		query = query.Where(likeCondition,
			"%"+keyword+"%", "%"+keyword+"%", "%"+keyword+"%")
	}

	// 获取总数
	err = query.Count(&total).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// 获取分页数据
	err = query.Omit("password", "group").Order("id desc").Limit(num).Offset(startIdx).Find(&users).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}
	if err = fillUserInvitationSummariesTx(tx, users); err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// 提交事务
	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}

	return users, total, nil
}

func FillUserInvitationSummariesForUsers(users []*User) error {
	return fillUserInvitationSummariesTx(DB, users)
}

func fillUserInvitationSummariesTx(tx *gorm.DB, users []*User) error {
	if len(users) == 0 {
		return nil
	}
	userIds := make([]int, 0, len(users))
	for _, user := range users {
		if user != nil {
			userIds = append(userIds, user.Id)
		}
	}
	if len(userIds) == 0 {
		return nil
	}
	type inviteCountRow struct {
		InviterId int
		Count     int
	}
	var directRows []inviteCountRow
	if err := tx.Model(&User{}).
		Select("inviter_id, count(*) as count").
		Where("inviter_id IN ?", userIds).
		Group("inviter_id").
		Scan(&directRows).Error; err != nil {
		return err
	}
	directCounts := make(map[int]int, len(directRows))
	for _, row := range directRows {
		directCounts[row.InviterId] = row.Count
	}

	now := common.GetTimestamp()
	var qualifiedRows []inviteCountRow
	if err := tx.Model(&User{}).
		Select("users.inviter_id, count(distinct users.id) as count").
		Joins("JOIN user_subscriptions ON user_subscriptions.user_id = users.id").
		Joins("JOIN subscription_plans ON subscription_plans.id = user_subscriptions.plan_id").
		Where("users.inviter_id IN ?", userIds).
		Where("user_subscriptions.status = ?", "active").
		Where("user_subscriptions.start_time <= ? AND user_subscriptions.end_time > ?", now, now).
		Where("subscription_plans.reward_eligible = ?", true).
		Where("subscription_plans.is_trial = ?", false).
		Where("subscription_plans.invite_trial = ?", false).
		Where("(user_subscriptions.grant_reason = '' OR user_subscriptions.grant_reason <> ?)", SubscriptionGrantMonthlyInviteEntitlement).
		Where("(user_subscriptions.source = '' OR user_subscriptions.source <> ?)", SubscriptionGrantMonthlyInviteEntitlement).
		Group("users.inviter_id").
		Scan(&qualifiedRows).Error; err != nil {
		return err
	}
	qualifiedCounts := make(map[int]int, len(qualifiedRows))
	for _, row := range qualifiedRows {
		qualifiedCounts[row.InviterId] = row.Count
	}

	type rewardRow struct {
		InviterId              int
		Status                 string
		RewardPlanId           int
		RewardPlanTitle        string
		RewardPlanBusinessCode string
	}
	var rewardRows []rewardRow
	if err := tx.Table("invitation_monthly_entitlements").
		Select("invitation_monthly_entitlements.inviter_id, invitation_monthly_entitlements.status, subscription_plans.id AS reward_plan_id, subscription_plans.title AS reward_plan_title, subscription_plans.business_code AS reward_plan_business_code").
		Joins("LEFT JOIN subscription_plans ON subscription_plans.id = invitation_monthly_entitlements.reward_plan_id").
		Where("invitation_monthly_entitlements.inviter_id IN ?", userIds).
		Where("invitation_monthly_entitlements.reward_month = ?", rewardMonthStringFromUnix(now)).
		Scan(&rewardRows).Error; err != nil {
		return err
	}
	rewards := make(map[int]rewardRow, len(rewardRows))
	for _, row := range rewardRows {
		rewards[row.InviterId] = row
	}
	type commissionAccountRow struct {
		UserId           int
		AvailableCents   int64
		PendingCents     int64
		WithdrawnCents   int64
		TransferredCents int64
	}
	commissionAccounts := make(map[int]commissionAccountRow)
	if tx.Migrator().HasTable(&InvitationCommissionAccount{}) {
		var commissionAccountRows []commissionAccountRow
		if err := tx.Model(&InvitationCommissionAccount{}).
			Select("user_id, available_cents, pending_cents, withdrawn_cents, transferred_cents").
			Where("user_id IN ?", userIds).
			Scan(&commissionAccountRows).Error; err != nil {
			return err
		}
		commissionAccounts = make(map[int]commissionAccountRow, len(commissionAccountRows))
		for _, row := range commissionAccountRows {
			commissionAccounts[row.UserId] = row
		}
	}

	type commissionEstimateRow struct {
		EstimatedSourceAmountCents int64
		EstimatedCents             int64
		EstimatedEventCount        int
	}
	type commissionEstimateEventRow struct {
		InviterId         int
		SourceAmountCents int64
	}
	commissionSetting := operation_setting.GetInvitationCommissionSetting()
	commissionRateBps := 0
	if commissionSetting.Enabled && commissionSetting.RateBps > 0 {
		commissionRateBps = commissionSetting.RateBps
	}
	commissionEstimates := make(map[int]commissionEstimateRow)
	if commissionRateBps > 0 && tx.Migrator().HasTable(&InvitationRewardEvent{}) && tx.Migrator().HasTable(&InvitationCommissionRecord{}) {
		var commissionEstimateEventRows []commissionEstimateEventRow
		if err := tx.Table("invitation_reward_events").
			Select("invitation_reward_events.inviter_id, invitation_reward_events.source_amount_cents").
			Joins("JOIN user_subscriptions ON user_subscriptions.id = invitation_reward_events.source_subscription_id").
			Joins("JOIN subscription_plans ON subscription_plans.id = user_subscriptions.plan_id").
			Joins("LEFT JOIN invitation_commission_records ON invitation_commission_records.source_type = invitation_reward_events.source_type AND invitation_commission_records.source_id = invitation_reward_events.source_id").
			Where("invitation_reward_events.inviter_id IN ?", userIds).
			Where("invitation_reward_events.status = ?", InvitationRewardEventStatusActive).
			Where("invitation_reward_events.source_amount_cents > 0").
			Where("UPPER(TRIM(invitation_reward_events.source_currency)) = ?", "CNY").
			Where("invitation_commission_records.id IS NULL").
			Where("user_subscriptions.status = ?", "active").
			Where("user_subscriptions.start_time <= ? AND user_subscriptions.end_time > ?", now, now).
			Where("subscription_plans.reward_eligible = ?", true).
			Where("subscription_plans.is_trial = ?", false).
			Where("subscription_plans.invite_trial = ?", false).
			Where("(user_subscriptions.grant_reason IS NULL OR TRIM(user_subscriptions.grant_reason) <> ?)", SubscriptionGrantMonthlyInviteEntitlement).
			Where("(user_subscriptions.source IS NULL OR TRIM(user_subscriptions.source) <> ?)", SubscriptionGrantMonthlyInviteEntitlement).
			Where("(subscription_plans.business_code IS NULL OR TRIM(subscription_plans.business_code) <> ?)", SubscriptionGrantMonthlyInviteEntitlement).
			Scan(&commissionEstimateEventRows).Error; err != nil {
			return err
		}
		commissionEstimates = make(map[int]commissionEstimateRow, len(commissionEstimateEventRows))
		for _, row := range commissionEstimateEventRows {
			estimate := commissionEstimates[row.InviterId]
			estimate.EstimatedSourceAmountCents += row.SourceAmountCents
			estimate.EstimatedCents += row.SourceAmountCents * int64(commissionRateBps) / 10000
			estimate.EstimatedEventCount++
			commissionEstimates[row.InviterId] = estimate
		}
	}

	for _, user := range users {
		if user == nil {
			continue
		}
		user.InvitationCommissionAvailableCents = 0
		user.InvitationCommissionPendingCents = 0
		user.InvitationCommissionWithdrawnCents = 0
		user.InvitationCommissionTransferredCents = 0
		user.InvitationCommissionEarnedCents = 0
		user.InvitationCommissionEstimatedCents = 0
		user.InvitationCommissionEstimatedSourceAmountCents = 0
		user.InvitationCommissionEstimatedEventCount = 0
		user.DirectInviteCount = directCounts[user.Id]
		user.QualifiedPaidInviteCount = qualifiedCounts[user.Id]
		if account, ok := commissionAccounts[user.Id]; ok {
			user.InvitationCommissionAvailableCents = account.AvailableCents
			user.InvitationCommissionPendingCents = account.PendingCents
			user.InvitationCommissionWithdrawnCents = account.WithdrawnCents
			user.InvitationCommissionTransferredCents = account.TransferredCents
			user.InvitationCommissionEarnedCents = account.AvailableCents + account.PendingCents + account.WithdrawnCents + account.TransferredCents
		}
		if estimate, ok := commissionEstimates[user.Id]; ok {
			user.InvitationCommissionEstimatedSourceAmountCents = estimate.EstimatedSourceAmountCents
			user.InvitationCommissionEstimatedEventCount = estimate.EstimatedEventCount
			user.InvitationCommissionEstimatedCents = estimate.EstimatedCents
		}
		if reward, ok := rewards[user.Id]; ok {
			user.InvitationRewardStatus = reward.Status
			user.InvitationRewardPlanTitle = reward.RewardPlanTitle
			user.RewardPlanId = reward.RewardPlanId
			user.RewardPlanTitle = reward.RewardPlanTitle
			user.RewardPlanBusinessCode = reward.RewardPlanBusinessCode
		}
	}
	return nil
}

func rewardMonthStringFromUnix(timestamp int64) string {
	return time.Unix(timestamp, 0).UTC().Format("2006-01")
}

func GetUserById(id int, selectAll bool) (*User, error) {
	if id == 0 {
		return nil, errors.New("id 为空！")
	}
	user := User{Id: id}
	var err error = nil
	if selectAll {
		err = DB.Omit("group").First(&user, "id = ?", id).Error
	} else {
		err = DB.Omit("password", "group").First(&user, "id = ?", id).Error
	}
	return &user, err
}

func GetUserIdByAffCode(affCode string) (int, error) {
	if affCode == "" {
		return 0, errors.New("affCode 为空！")
	}
	var user User
	err := DB.Select("id").First(&user, "aff_code = ?", affCode).Error
	return user.Id, err
}

func DeleteUserById(id int) (err error) {
	if id == 0 {
		return errors.New("id 为空！")
	}
	user := User{Id: id}
	return user.Delete()
}

func HardDeleteUserById(id int) error {
	if id == 0 {
		return errors.New("id 为空！")
	}
	err := DB.Unscoped().Delete(&User{}, "id = ?", id).Error
	return err
}

func accountBalanceCNYAmount(cents int) string {
	return AccountBalanceCNYFromCents(cents).StringFixed(2)
}

func invalidateUserCacheBestEffort(userId int) {
	if err := InvalidateUserCache(userId); err != nil {
		common.SysLog(fmt.Sprintf("failed to invalidate user cache for user %d: %s", userId, err.Error()))
	}
}

func recordLogTx(tx *gorm.DB, userId int, logType int, content string) {
	if tx == nil {
		RecordLog(userId, logType, content)
		return
	}
	if LOG_DB != DB {
		return
	}
	if logType == LogTypeConsume && !common.LogConsumeEnabled {
		return
	}
	var username string
	_ = tx.Model(&User{}).Select("username").Where("id = ?", userId).Scan(&username).Error
	log := &Log{
		UserId:    userId,
		Username:  username,
		CreatedAt: common.GetTimestamp(),
		Type:      logType,
		Content:   content,
	}
	if err := tx.Create(log).Error; err != nil {
		common.SysLog("failed to record log: " + err.Error())
	}
}

func inviteUser(inviterId int) (err error) {
	user, err := GetUserById(inviterId, true)
	if err != nil {
		return err
	}
	user.AffCount++
	user.AffQuota += common.QuotaForInviter
	user.AffHistoryQuota += common.QuotaForInviter
	return DB.Save(user).Error
}

func inviteUserTx(tx *gorm.DB, inviterId int) error {
	if tx == nil {
		return errors.New("tx is nil")
	}
	return tx.Model(&User{}).Where("id = ?", inviterId).Updates(map[string]any{
		"aff_count":   gorm.Expr("aff_count + ?", 1),
		"aff_quota":   gorm.Expr("aff_quota + ?", common.QuotaForInviter),
		"aff_history": gorm.Expr("aff_history + ?", common.QuotaForInviter),
	}).Error
}

func (user *User) TransferAffQuotaToQuota(quota int) error {
	if quota < 1 {
		return errors.New("转移额度最小为0.01！")
	}

	tx := DB.Begin()
	if tx.Error != nil {
		return tx.Error
	}
	defer tx.Rollback()

	err := tx.Set("gorm:query_option", "FOR UPDATE").First(user, user.Id).Error
	if err != nil {
		return err
	}

	if user.AffQuota < quota {
		return errors.New("邀请额度不足！")
	}

	user.AffQuota -= quota
	user.Quota += quota

	if err := tx.Save(user).Error; err != nil {
		return err
	}
	if err := tx.Commit().Error; err != nil {
		return err
	}
	invalidateUserCacheBestEffort(user.Id)
	return nil
}

func (user *User) Insert(inviterId int) error {
	var err error
	if user.Password != "" {
		user.Password, err = common.Password2Hash(user.Password)
		if err != nil {
			return err
		}
	}
	user.Quota = common.QuotaForNewUser
	//user.SetAccessToken(common.GetUUID())
	user.AffCode = common.GetRandomString(4)
	user.Group = ""

	// 初始化用户设置，包括默认的边栏配置
	if user.Setting == "" {
		defaultSetting := dto.UserSetting{}
		// 这里暂时不设置SidebarModules，因为需要在用户创建后根据角色设置
		user.SetSetting(defaultSetting)
	}

	result := DB.Create(user)
	if result.Error != nil {
		return result.Error
	}

	// 用户创建成功后，根据角色初始化边栏配置
	// 需要重新获取用户以确保有正确的ID和Role
	var createdUser User
	if err := DB.Where("username = ?", user.Username).First(&createdUser).Error; err == nil {
		// 生成基于角色的默认边栏配置
		defaultSidebarConfig := generateDefaultSidebarConfigForRole(createdUser.Role)
		if defaultSidebarConfig != "" {
			currentSetting := createdUser.GetSetting()
			currentSetting.SidebarModules = defaultSidebarConfig
			createdUser.SetSetting(currentSetting)
			createdUser.Update(false)
			common.SysLog(fmt.Sprintf("为新用户 %s (角色: %d) 初始化边栏配置", createdUser.Username, createdUser.Role))
		}
	}

	if common.QuotaForNewUser > 0 {
		RecordLog(user.Id, LogTypeSystem, fmt.Sprintf("新用户注册赠送 %s", accountBalanceCNYAmount(common.QuotaForNewUser)))
	}
	if inviterId != 0 {
		if common.QuotaForInvitee > 0 {
			if err := DB.Transaction(func(tx *gorm.DB) error {
				return IncreaseUserAccountBalanceTx(tx, user.Id, common.QuotaForInvitee)
			}); err != nil {
				return err
			}
			invalidateUserCacheBestEffort(user.Id)
			RecordLog(user.Id, LogTypeSystem, fmt.Sprintf("使用邀请码赠送 %s", accountBalanceCNYAmount(common.QuotaForInvitee)))
		}
		if common.QuotaForInviter > 0 {
			RecordLog(inviterId, LogTypeSystem, fmt.Sprintf("邀请用户赠送 %s", accountBalanceCNYAmount(common.QuotaForInviter)))
			_ = inviteUser(inviterId)
		}
	}
	return nil
}

// InsertWithTx inserts a new user within an existing transaction.
// This is used for OAuth registration where user creation and binding need to be atomic.
// Post-creation tasks (sidebar config, logs, inviter rewards) are handled after the transaction commits.
func (user *User) InsertWithTx(tx *gorm.DB, inviterId int) error {
	var err error
	if user.Password != "" {
		user.Password, err = common.Password2Hash(user.Password)
		if err != nil {
			return err
		}
	}
	user.Quota = common.QuotaForNewUser
	user.AffCode = common.GetRandomString(4)
	user.Group = ""

	// 初始化用户设置
	if user.Setting == "" {
		defaultSetting := dto.UserSetting{}
		user.SetSetting(defaultSetting)
	}

	result := tx.Create(user)
	if result.Error != nil {
		return result.Error
	}

	return nil
}

func (user *User) FinalizeCreationTx(tx *gorm.DB, inviterId int) error {
	if tx == nil {
		return errors.New("tx is nil")
	}
	var createdUser User
	if err := tx.Where("id = ?", user.Id).First(&createdUser).Error; err != nil {
		return err
	}
	defaultSidebarConfig := generateDefaultSidebarConfigForRole(createdUser.Role)
	if defaultSidebarConfig != "" {
		currentSetting := createdUser.GetSetting()
		currentSetting.SidebarModules = defaultSidebarConfig
		createdUser.SetSetting(currentSetting)
		if err := tx.Save(&createdUser).Error; err != nil {
			return err
		}
		common.SysLog(fmt.Sprintf("为新用户 %s (角色: %d) 初始化边栏配置", createdUser.Username, createdUser.Role))
	}
	if common.QuotaForNewUser > 0 {
		recordLogTx(tx, user.Id, LogTypeSystem, fmt.Sprintf("新用户注册赠送 %s", accountBalanceCNYAmount(common.QuotaForNewUser)))
	}
	if inviterId != 0 {
		if common.QuotaForInvitee > 0 {
			if err := IncreaseUserAccountBalanceTx(tx, user.Id, common.QuotaForInvitee); err != nil {
				return err
			}
			recordLogTx(tx, user.Id, LogTypeSystem, fmt.Sprintf("使用邀请码赠送 %s", accountBalanceCNYAmount(common.QuotaForInvitee)))
		}
		if common.QuotaForInviter > 0 {
			recordLogTx(tx, inviterId, LogTypeSystem, fmt.Sprintf("邀请用户赠送 %s", accountBalanceCNYAmount(common.QuotaForInviter)))
			if err := inviteUserTx(tx, inviterId); err != nil {
				return err
			}
		}
	}
	return nil
}

func (user *User) RecordAccountBalanceRewardLogsAfterTx(inviterId int) {
	if LOG_DB == DB {
		return
	}
	if common.QuotaForNewUser > 0 {
		RecordLog(user.Id, LogTypeSystem, fmt.Sprintf("新用户注册赠送 %s", accountBalanceCNYAmount(common.QuotaForNewUser)))
	}
	if inviterId != 0 {
		if common.QuotaForInvitee > 0 {
			RecordLog(user.Id, LogTypeSystem, fmt.Sprintf("使用邀请码赠送 %s", accountBalanceCNYAmount(common.QuotaForInvitee)))
		}
		if common.QuotaForInviter > 0 {
			RecordLog(inviterId, LogTypeSystem, fmt.Sprintf("邀请用户赠送 %s", accountBalanceCNYAmount(common.QuotaForInviter)))
		}
	}
}

// FinalizeOAuthUserCreation performs post-transaction tasks for OAuth user creation.
// This should be called after the transaction commits successfully.
func (user *User) FinalizeOAuthUserCreation(inviterId int) {
	err := DB.Transaction(func(tx *gorm.DB) error {
		return user.FinalizeCreationTx(tx, inviterId)
	})
	if err != nil {
		common.SysLog("failed to finalize OAuth user creation: " + err.Error())
		return
	}
	user.RecordAccountBalanceRewardLogsAfterTx(inviterId)
}

func (user *User) Update(updatePassword bool) error {
	var err error
	if updatePassword {
		user.Password, err = common.Password2Hash(user.Password)
		if err != nil {
			return err
		}
	}
	newUser := *user
	DB.First(&user, user.Id)
	newUser.InvitationRewardMode = user.InvitationRewardMode
	if err = DB.Model(user).Updates(newUser).Error; err != nil {
		return err
	}

	// Update cache
	return updateUserCache(*user)
}

func (user *User) Edit(updatePassword bool) error {
	var err error
	if updatePassword {
		user.Password, err = common.Password2Hash(user.Password)
		if err != nil {
			return err
		}
	}

	newUser := *user
	updates := map[string]interface{}{
		"username":               newUser.Username,
		"display_name":           newUser.DisplayName,
		"remark":                 newUser.Remark,
		"invitation_reward_mode": newUser.NormalizedInvitationRewardMode(),
	}
	if updatePassword {
		updates["password"] = newUser.Password
	}

	DB.First(&user, user.Id)
	if err = DB.Model(user).Updates(updates).Error; err != nil {
		return err
	}

	// Update cache
	return updateUserCache(*user)
}

func (user *User) ClearBinding(bindingType string) error {
	if user.Id == 0 {
		return errors.New("user id is empty")
	}

	bindingColumnMap := map[string]string{
		"email":    "email",
		"github":   "github_id",
		"discord":  "discord_id",
		"oidc":     "oidc_id",
		"wechat":   "wechat_id",
		"telegram": "telegram_id",
		"linuxdo":  "linux_do_id",
	}

	column, ok := bindingColumnMap[bindingType]
	if !ok {
		return errors.New("invalid binding type")
	}

	if err := DB.Model(&User{}).Where("id = ?", user.Id).Update(column, "").Error; err != nil {
		return err
	}

	if err := DB.Where("id = ?", user.Id).First(user).Error; err != nil {
		return err
	}

	return updateUserCache(*user)
}

func (user *User) Delete() error {
	if user.Id == 0 {
		return errors.New("id 为空！")
	}
	if err := DB.Delete(user).Error; err != nil {
		return err
	}

	// 清除缓存
	return invalidateUserCache(user.Id)
}

func (user *User) HardDelete() error {
	if user.Id == 0 {
		return errors.New("id 为空！")
	}
	err := DB.Unscoped().Delete(user).Error
	return err
}

// ValidateAndFill check password & user status
func (user *User) ValidateAndFill() (err error) {
	// When querying with struct, GORM will only query with non-zero fields,
	// that means if your field's value is 0, '', false or other zero values,
	// it won't be used to build query conditions
	password := user.Password
	username := strings.TrimSpace(user.Username)
	if username == "" || password == "" {
		return ErrUserEmptyCredentials
	}
	// find by username or email
	err = DB.Where("username = ? OR email = ?", username, username).First(user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrInvalidCredentials
		}
		return fmt.Errorf("%w: %v", ErrDatabase, err)
	}
	okay := common.ValidatePasswordAndHash(password, user.Password)
	if !okay || user.Status != common.UserStatusEnabled {
		return ErrInvalidCredentials
	}
	return nil
}

func (user *User) FillUserById() error {
	if user.Id == 0 {
		return errors.New("id 为空！")
	}
	DB.Where(User{Id: user.Id}).First(user)
	return nil
}

func (user *User) FillUserByEmail() error {
	if user.Email == "" {
		return errors.New("email 为空！")
	}
	DB.Where(User{Email: user.Email}).First(user)
	return nil
}

func (user *User) FillUserByGitHubId() error {
	if user.GitHubId == "" {
		return errors.New("GitHub id 为空！")
	}
	DB.Where(User{GitHubId: user.GitHubId}).First(user)
	return nil
}

// UpdateGitHubId updates the user's GitHub ID (used for migration from login to numeric ID)
func (user *User) UpdateGitHubId(newGitHubId string) error {
	if user.Id == 0 {
		return errors.New("user id is empty")
	}
	return DB.Model(user).Update("github_id", newGitHubId).Error
}

func (user *User) FillUserByDiscordId() error {
	if user.DiscordId == "" {
		return errors.New("discord id 为空！")
	}
	DB.Where(User{DiscordId: user.DiscordId}).First(user)
	return nil
}

func (user *User) FillUserByOidcId() error {
	if user.OidcId == "" {
		return errors.New("oidc id 为空！")
	}
	DB.Where(User{OidcId: user.OidcId}).First(user)
	return nil
}

func (user *User) FillUserByWeChatId() error {
	if user.WeChatId == "" {
		return errors.New("WeChat id 为空！")
	}
	DB.Where(User{WeChatId: user.WeChatId}).First(user)
	return nil
}

func (user *User) FillUserByTelegramId() error {
	if user.TelegramId == "" {
		return errors.New("Telegram id 为空！")
	}
	err := DB.Where(User{TelegramId: user.TelegramId}).First(user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return errors.New("该 Telegram 账户未绑定")
	}
	return nil
}

func IsEmailAlreadyTaken(email string) bool {
	return DB.Unscoped().Where("email = ?", email).Find(&User{}).RowsAffected == 1
}

func IsWeChatIdAlreadyTaken(wechatId string) bool {
	return DB.Unscoped().Where("wechat_id = ?", wechatId).Find(&User{}).RowsAffected == 1
}

func IsGitHubIdAlreadyTaken(githubId string) bool {
	return DB.Unscoped().Where("github_id = ?", githubId).Find(&User{}).RowsAffected == 1
}

func IsDiscordIdAlreadyTaken(discordId string) bool {
	return DB.Unscoped().Where("discord_id = ?", discordId).Find(&User{}).RowsAffected == 1
}

func IsOidcIdAlreadyTaken(oidcId string) bool {
	return DB.Where("oidc_id = ?", oidcId).Find(&User{}).RowsAffected == 1
}

func IsTelegramIdAlreadyTaken(telegramId string) bool {
	return DB.Unscoped().Where("telegram_id = ?", telegramId).Find(&User{}).RowsAffected == 1
}

func ResetUserPasswordByEmail(email string, password string) error {
	if email == "" || password == "" {
		return errors.New("邮箱地址或密码为空！")
	}
	hashedPassword, err := common.Password2Hash(password)
	if err != nil {
		return err
	}
	err = DB.Model(&User{}).Where("email = ?", email).Update("password", hashedPassword).Error
	return err
}

func IsAdmin(userId int) bool {
	if userId == 0 {
		return false
	}
	var user User
	err := DB.Where("id = ?", userId).Select("role").Find(&user).Error
	if err != nil {
		common.SysLog("no such user " + err.Error())
		return false
	}
	return user.Role >= common.RoleAdminUser
}

//// IsUserEnabled checks user status from Redis first, falls back to DB if needed
//func IsUserEnabled(id int, fromDB bool) (status bool, err error) {
//	defer func() {
//		// Update Redis cache asynchronously on successful DB read
//		if shouldUpdateRedis(fromDB, err) {
//			gopool.Go(func() {
//				if err := updateUserStatusCache(id, status); err != nil {
//					common.SysError("failed to update user status cache: " + err.Error())
//				}
//			})
//		}
//	}()
//	if !fromDB && common.RedisEnabled {
//		// Try Redis first
//		status, err := getUserStatusCache(id)
//		if err == nil {
//			return status == common.UserStatusEnabled, nil
//		}
//		// Don't return error - fall through to DB
//	}
//	fromDB = true
//	var user User
//	err = DB.Where("id = ?", id).Select("status").Find(&user).Error
//	if err != nil {
//		return false, err
//	}
//
//	return user.Status == common.UserStatusEnabled, nil
//}

func ValidateAccessToken(token string) (*User, error) {
	if token == "" {
		return nil, nil
	}
	token = strings.Replace(token, "Bearer ", "", 1)
	user := &User{}
	err := DB.Where("access_token = ?", token).First(user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("%w: %v", ErrDatabase, err)
	}
	return user, nil
}

// GetUserQuota gets quota from Redis first, falls back to DB if needed
func GetUserQuota(id int, fromDB bool) (quota int, err error) {
	defer func() {
		// Update Redis cache asynchronously on successful DB read
		if shouldUpdateRedis(fromDB, err) {
			gopool.Go(func() {
				if err := updateUserQuotaCache(id, quota); err != nil {
					common.SysLog("failed to update user quota cache: " + err.Error())
				}
			})
		}
	}()
	if !fromDB && common.RedisEnabled {
		quota, err := getUserQuotaCache(id)
		if err == nil {
			return quota, nil
		}
		// Don't return error - fall through to DB
	}
	fromDB = true
	err = DB.Model(&User{}).Where("id = ?", id).Select("quota").Find(&quota).Error
	if err != nil {
		return 0, err
	}

	return quota, nil
}

func GetUserUsedQuota(id int) (quota int, err error) {
	err = DB.Model(&User{}).Where("id = ?", id).Select("used_quota").Find(&quota).Error
	return quota, err
}

func GetUserEmail(id int) (email string, err error) {
	err = DB.Model(&User{}).Where("id = ?", id).Select("email").Find(&email).Error
	return email, err
}

// GetUserGroup is a legacy compatibility helper. Business group selection is removed.
func GetUserGroup(id int, fromDB bool) (group string, err error) {
	return "", nil
}

// GetUserSetting gets setting from Redis first, falls back to DB if needed
func GetUserSetting(id int, fromDB bool) (settingMap dto.UserSetting, err error) {
	var setting string
	defer func() {
		// Update Redis cache asynchronously on successful DB read
		if shouldUpdateRedis(fromDB, err) {
			gopool.Go(func() {
				if err := updateUserSettingCache(id, setting); err != nil {
					common.SysLog("failed to update user setting cache: " + err.Error())
				}
			})
		}
	}()
	if !fromDB && common.RedisEnabled {
		setting, err := getUserSettingCache(id)
		if err == nil {
			return setting, nil
		}
		// Don't return error - fall through to DB
	}
	fromDB = true
	// can be nil setting
	var safeSetting sql.NullString
	err = DB.Model(&User{}).Where("id = ?", id).Select("setting").Find(&safeSetting).Error
	if err != nil {
		return settingMap, err
	}
	if safeSetting.Valid {
		setting = safeSetting.String
	} else {
		setting = ""
	}
	userBase := &UserBase{
		Setting: setting,
	}
	return userBase.GetSetting(), nil
}

func IncreaseUserQuota(id int, quota int, db bool) (err error) {
	if quota < 0 {
		return errors.New("quota 不能为负数！")
	}
	gopool.Go(func() {
		err := cacheIncrUserQuota(id, int64(quota))
		if err != nil {
			common.SysLog("failed to increase user quota: " + err.Error())
		}
	})
	if !db && common.BatchUpdateEnabled {
		addNewRecord(BatchUpdateTypeUserQuota, id, quota)
		return nil
	}
	return increaseUserQuota(id, quota)
}

func increaseUserQuota(id int, quota int) (err error) {
	err = DB.Model(&User{}).Where("id = ?", id).Update("quota", gorm.Expr("quota + ?", quota)).Error
	if err != nil {
		return err
	}
	return err
}

func increaseUserQuotaTx(tx *gorm.DB, id int, quota int) error {
	if tx == nil {
		return errors.New("tx is nil")
	}
	return tx.Model(&User{}).Where("id = ?", id).Update("quota", gorm.Expr("quota + ?", quota)).Error
}

func DecreaseUserQuota(id int, quota int, db bool) (err error) {
	if quota < 0 {
		return errors.New("quota 不能为负数！")
	}
	gopool.Go(func() {
		err := cacheDecrUserQuota(id, int64(quota))
		if err != nil {
			common.SysLog("failed to decrease user quota: " + err.Error())
		}
	})
	if !db && common.BatchUpdateEnabled {
		addNewRecord(BatchUpdateTypeUserQuota, id, -quota)
		return nil
	}
	return decreaseUserQuota(id, quota)
}

func decreaseUserQuota(id int, quota int) (err error) {
	err = DB.Model(&User{}).Where("id = ?", id).Update("quota", gorm.Expr("quota - ?", quota)).Error
	if err != nil {
		return err
	}
	return err
}

func DeltaUpdateUserQuota(id int, delta int) (err error) {
	if delta == 0 {
		return nil
	}
	if delta > 0 {
		return IncreaseUserQuota(id, delta, false)
	} else {
		return DecreaseUserQuota(id, -delta, false)
	}
}

//func GetRootUserEmail() (email string) {
//	DB.Model(&User{}).Where("role = ?", common.RoleRootUser).Select("email").Find(&email)
//	return email
//}

func GetRootUser() (user *User) {
	DB.Where("role = ?", common.RoleRootUser).First(&user)
	return user
}

func UpdateUserLastLoginAt(id int) {
	if err := DB.Model(&User{}).Where("id = ?", id).Update("last_login_at", common.GetTimestamp()).Error; err != nil {
		common.SysLog("failed to update user last_login_at: " + err.Error())
	}
}

func UpdateUserUsedQuotaAndRequestCount(id int, quota int) {
	if common.BatchUpdateEnabled {
		addNewRecord(BatchUpdateTypeUsedQuota, id, quota)
		addNewRecord(BatchUpdateTypeRequestCount, id, 1)
		return
	}
	userUsageCounterCoalescer.add(id, quota, 1)
}

func updateUserUsedQuotaAndRequestCount(id int, quota int, count int) {
	err := DB.Model(&User{}).Where("id = ?", id).Updates(
		map[string]interface{}{
			"used_quota":    gorm.Expr("used_quota + ?", quota),
			"request_count": gorm.Expr("request_count + ?", count),
		},
	).Error
	if err != nil {
		common.SysLog("failed to update user used quota and request count: " + err.Error())
		return
	}

	//// 更新缓存
	//if err := invalidateUserCache(id); err != nil {
	//	common.SysError("failed to invalidate user cache: " + err.Error())
	//}
}

func updateUserUsedQuota(id int, quota int) {
	err := DB.Model(&User{}).Where("id = ?", id).Updates(
		map[string]interface{}{
			"used_quota": gorm.Expr("used_quota + ?", quota),
		},
	).Error
	if err != nil {
		common.SysLog("failed to update user used quota: " + err.Error())
	}
}

func updateUserRequestCount(id int, count int) {
	err := DB.Model(&User{}).Where("id = ?", id).Update("request_count", gorm.Expr("request_count + ?", count)).Error
	if err != nil {
		common.SysLog("failed to update user request count: " + err.Error())
	}
}

// GetUsernameById gets username from Redis first, falls back to DB if needed
func GetUsernameById(id int, fromDB bool) (username string, err error) {
	defer func() {
		// Update Redis cache asynchronously on successful DB read
		if shouldUpdateRedis(fromDB, err) {
			gopool.Go(func() {
				if err := updateUserNameCache(id, username); err != nil {
					common.SysLog("failed to update user name cache: " + err.Error())
				}
			})
		}
	}()
	if !fromDB && common.RedisEnabled {
		username, err := getUserNameCache(id)
		if err == nil {
			return username, nil
		}
		// Don't return error - fall through to DB
	}
	fromDB = true
	err = DB.Model(&User{}).Where("id = ?", id).Select("username").Find(&username).Error
	if err != nil {
		return "", err
	}

	return username, nil
}

func IsLinuxDOIdAlreadyTaken(linuxDOId string) bool {
	var user User
	err := DB.Unscoped().Where("linux_do_id = ?", linuxDOId).First(&user).Error
	return !errors.Is(err, gorm.ErrRecordNotFound)
}

func (user *User) FillUserByLinuxDOId() error {
	if user.LinuxDOId == "" {
		return errors.New("linux do id is empty")
	}
	err := DB.Where("linux_do_id = ?", user.LinuxDOId).First(user).Error
	return err
}

func RootUserExists() bool {
	var user User
	err := DB.Where("role = ?", common.RoleRootUser).First(&user).Error
	if err != nil {
		return false
	}
	return true
}
