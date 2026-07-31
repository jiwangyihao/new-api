package service

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	invitationCommissionRecordReference     = "commission_record"
	invitationCommissionTransferReference   = "transfer_to_balance"
	invitationCommissionWithdrawalReference = "withdrawal"
	invitationRewardEventRetryBatchSize     = 100
	invitationRewardEventRetryInterval      = 5 * time.Minute
)

var (
	invitationRewardEventRetryOnce    sync.Once
	invitationRewardEventRetryRunning atomic.Bool
)

type InvitationCommissionContact struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

type InvitationCommissionWithdrawalRequest struct {
	AmountCents int64                       `json:"amount_cents"`
	Contact     InvitationCommissionContact `json:"contact"`
	Remark      string                      `json:"remark"`
}

type InvitationCommissionTransferResult struct {
	AvailableCents   int64 `json:"available_cents"`
	TransferredCents int64 `json:"transferred_cents"`
	UserQuota        int   `json:"user_quota"`
}

type InvitationCommissionAccountSummary struct {
	AvailableCents   int64 `json:"available_cents"`
	PendingCents     int64 `json:"pending_cents"`
	WithdrawnCents   int64 `json:"withdrawn_cents"`
	TransferredCents int64 `json:"transferred_cents"`
}

type InvitationCommissionSummary struct {
	RewardMode               string                                        `json:"reward_mode"`
	HasCommissionAccount     bool                                          `json:"has_commission_account"`
	CanTransfer              bool                                          `json:"can_transfer"`
	CanRequestWithdrawal     bool                                          `json:"can_request_withdrawal"`
	DirectInviteCount        int                                           `json:"direct_invite_count"`
	QualifiedPaidInviteCount int                                           `json:"qualified_paid_invite_count"`
	Account                  InvitationCommissionAccountSummary            `json:"account"`
	Setting                  operation_setting.InvitationCommissionSetting `json:"setting"`
}

func InvitationCommissionWithdrawalToResponse(withdrawal model.InvitationCommissionWithdrawal) (InvitationCommissionWithdrawalResponse, error) {
	return invitationCommissionWithdrawalResponse(withdrawal)
}

type InvitationCommissionPageResult[T any] struct {
	Items    []T   `json:"items"`
	Total    int64 `json:"total"`
	Page     int   `json:"page"`
	PageSize int   `json:"page_size"`
}

type InvitationCommissionWithdrawalFilter struct {
	Status   string
	UserId   int
	Page     int
	PageSize int
}

type InvitationCommissionRecordResponse struct {
	Id                int    `json:"id"`
	EventId           int    `json:"event_id"`
	InviteeId         int    `json:"invitee_id"`
	SourceType        string `json:"source_type"`
	SourceId          int    `json:"source_id"`
	SourceTradeNo     string `json:"source_trade_no"`
	SourceAmountCents int64  `json:"source_amount_cents"`
	SourceCurrency    string `json:"source_currency"`
	CommissionRateBps int    `json:"commission_rate_bps"`
	CommissionCents   int64  `json:"commission_cents"`
	Status            string `json:"status"`
	Reason            string `json:"reason"`
	CreatedAt         int64  `json:"created_at"`
	AvailableAt       int64  `json:"available_at"`
	CancelledAt       int64  `json:"cancelled_at"`
	ReversalStatus    string `json:"reversal_status,omitempty"`
	RecoveredCents    int64  `json:"recovered_cents,omitempty"`
	UnrecoveredCents  int64  `json:"unrecovered_cents,omitempty"`
	ReversalReason    string `json:"reversal_reason,omitempty"`
	ReversedAt        int64  `json:"reversed_at,omitempty"`
}

type InvitationCommissionWithdrawalResponse struct {
	Id          int                         `json:"id"`
	UserId      int                         `json:"user_id"`
	AmountCents int64                       `json:"amount_cents"`
	Status      string                      `json:"status"`
	Method      string                      `json:"method"`
	Contact     InvitationCommissionContact `json:"contact"`
	UserRemark  string                      `json:"user_remark"`
	AdminRemark string                      `json:"admin_remark"`
	ReviewerId  int                         `json:"reviewer_id"`
	ReviewedAt  int64                       `json:"reviewed_at"`
	CompletedBy int                         `json:"completed_by"`
	CompletedAt int64                       `json:"completed_at"`
	CreatedAt   int64                       `json:"created_at"`
	UpdatedAt   int64                       `json:"updated_at"`
}

type AdminInvitationCommissionWithdrawalResponse struct {
	InvitationCommissionWithdrawalResponse
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
}

func HandleInvitationRewardForCompletedSubscriptionOrder(orderId int) error {
	if orderId <= 0 {
		return nil
	}
	event, err := getInvitationRewardEventBySource(model.InvitationRewardEventSourceSubscriptionOrder, orderId)
	if err != nil {
		return err
	}
	if event != nil {
		return dispatchInvitationRewardEvent(event)
	}
	var order model.SubscriptionOrder
	if err := model.DB.Select("id", "user_id").Where("id = ?", orderId).First(&order).Error; err != nil {
		return err
	}
	TryEnsureInvitationEntitlementForPaidUser(order.UserId)
	return nil
}

func HandleInvitationRewardForSubscriptionRedemption(redemptionId int) error {
	if redemptionId <= 0 {
		return nil
	}
	event, err := getInvitationRewardEventBySource(model.InvitationRewardEventSourceSubscriptionRedemption, redemptionId)
	if err != nil {
		return err
	}
	if event != nil {
		return dispatchInvitationRewardEvent(event)
	}
	redemption, err := model.GetRedemptionById(redemptionId)
	if err != nil {
		return err
	}
	if redemption.UsedUserId > 0 {
		TryEnsureInvitationEntitlementForPaidUser(redemption.UsedUserId)
	}
	return nil
}

func CreateInvitationCommissionForRewardEvent(eventId int) error {
	_, err := createInvitationCommissionForRewardEvent(eventId)
	return err
}

func TransferInvitationCommissionToBalance(userId int, amountCents int64) (*InvitationCommissionTransferResult, error) {
	setting := *operation_setting.GetInvitationCommissionSetting()
	if amountCents < maxInt64(setting.MinimumTransferCents, 1) {
		return nil, errors.New("返佣划转金额不足")
	}
	quotaCents, err := model.AccountBalanceIntFromCents(amountCents)
	if err != nil {
		return nil, err
	}
	var result InvitationCommissionTransferResult
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		account, err := loadUsableCommissionAccountTx(tx, userId)
		if err != nil {
			return err
		}
		if account.AvailableCents <= 0 {
			return errors.New("返佣余额不足")
		}
		now := common.GetTimestamp()
		updated := tx.Model(&model.InvitationCommissionAccount{}).
			Where("user_id = ? AND available_cents >= ?", userId, amountCents).
			Updates(map[string]any{
				"available_cents":   gorm.Expr("available_cents - ?", amountCents),
				"transferred_cents": gorm.Expr("transferred_cents + ?", amountCents),
				"updated_at":        now,
			})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected == 0 {
			return errors.New("返佣余额不足")
		}
		if err := model.IncreaseUserAccountBalanceTx(tx, userId, quotaCents); err != nil {
			return err
		}
		if err := tx.Where("user_id = ?", userId).First(account).Error; err != nil {
			return err
		}
		ledger := model.InvitationCommissionLedger{
			UserId:              userId,
			Type:                model.InvitationCommissionLedgerTransferred,
			AmountCents:         amountCents,
			AvailableAfterCents: account.AvailableCents,
			PendingAfterCents:   account.PendingCents,
			ReferenceType:       invitationCommissionTransferReference,
			CreatedAt:           now,
		}
		if err := tx.Create(&ledger).Error; err != nil {
			return err
		}
		var user model.User
		if err := tx.Select("quota").Where("id = ?", userId).First(&user).Error; err != nil {
			return err
		}
		result.AvailableCents = account.AvailableCents
		result.TransferredCents = account.TransferredCents
		result.UserQuota = user.Quota
		return nil
	})
	if err != nil {
		return nil, err
	}
	if err := model.InvalidateUserCache(userId); err != nil {
		common.SysLog(fmt.Sprintf("failed to invalidate user cache for user %d: %s", userId, err.Error()))
	}
	return &result, nil
}

func RequestInvitationCommissionWithdrawal(userId int, req InvitationCommissionWithdrawalRequest) (*model.InvitationCommissionWithdrawal, error) {
	setting := *operation_setting.GetInvitationCommissionSetting()
	if req.AmountCents < maxInt64(setting.MinimumWithdrawCents, 1) {
		return nil, errors.New("返现金额不足")
	}
	contact, remark, err := normalizeInvitationCommissionWithdrawalRequest(req)
	if err != nil {
		return nil, err
	}
	contactSnapshot, err := common.Marshal(contact)
	if err != nil {
		return nil, err
	}
	var withdrawal model.InvitationCommissionWithdrawal
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		if _, err := loadUsableCommissionAccountTx(tx, userId); err != nil {
			return err
		}
		now := common.GetTimestamp()
		updated := tx.Model(&model.InvitationCommissionAccount{}).
			Where("user_id = ? AND available_cents >= ?", userId, req.AmountCents).
			Updates(map[string]any{
				"available_cents": gorm.Expr("available_cents - ?", req.AmountCents),
				"pending_cents":   gorm.Expr("pending_cents + ?", req.AmountCents),
				"updated_at":      now,
			})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected == 0 {
			return errors.New("返佣余额不足")
		}
		withdrawal = model.InvitationCommissionWithdrawal{
			UserId:          userId,
			AmountCents:     req.AmountCents,
			Status:          model.InvitationCommissionWithdrawalPending,
			Method:          model.InvitationCommissionWithdrawalMethodManual,
			ContactSnapshot: string(contactSnapshot),
			UserRemark:      remark,
			CreatedAt:       now,
			UpdatedAt:       now,
		}
		if err := tx.Create(&withdrawal).Error; err != nil {
			return err
		}
		var account model.InvitationCommissionAccount
		if err := tx.Where("user_id = ?", userId).First(&account).Error; err != nil {
			return err
		}
		ledger := model.InvitationCommissionLedger{
			UserId:              userId,
			Type:                model.InvitationCommissionLedgerWithdrawalCreated,
			AmountCents:         req.AmountCents,
			AvailableAfterCents: account.AvailableCents,
			PendingAfterCents:   account.PendingCents,
			ReferenceType:       invitationCommissionWithdrawalReference,
			ReferenceId:         withdrawal.Id,
			CreatedAt:           now,
		}
		return tx.Create(&ledger).Error
	})
	if err != nil {
		return nil, err
	}
	return &withdrawal, nil
}

func CompleteInvitationCommissionWithdrawal(withdrawalId int, reviewerId int, adminRemark string) error {
	if reviewerId <= 0 {
		return errors.New("invalid reviewer id")
	}
	return model.DB.Transaction(func(tx *gorm.DB) error {
		var withdrawal model.InvitationCommissionWithdrawal
		if err := tx.Where("id = ?", withdrawalId).First(&withdrawal).Error; err != nil {
			return err
		}
		now := common.GetTimestamp()
		updated := tx.Model(&model.InvitationCommissionWithdrawal{}).
			Where("id = ? AND status = ?", withdrawalId, model.InvitationCommissionWithdrawalPending).
			Updates(map[string]any{
				"status":       model.InvitationCommissionWithdrawalCompleted,
				"admin_remark": strings.TrimSpace(adminRemark),
				"reviewer_id":  reviewerId,
				"reviewed_at":  now,
				"completed_by": reviewerId,
				"completed_at": now,
				"updated_at":   now,
			})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected == 0 {
			return errors.New("返现申请状态已变化，必须为 pending")
		}
		accountUpdate := tx.Model(&model.InvitationCommissionAccount{}).
			Where("user_id = ? AND pending_cents >= ?", withdrawal.UserId, withdrawal.AmountCents).
			Updates(map[string]any{
				"pending_cents":   gorm.Expr("pending_cents - ?", withdrawal.AmountCents),
				"withdrawn_cents": gorm.Expr("withdrawn_cents + ?", withdrawal.AmountCents),
				"updated_at":      now,
			})
		if accountUpdate.Error != nil {
			return accountUpdate.Error
		}
		if accountUpdate.RowsAffected == 0 {
			return errors.New("返佣冻结余额不足")
		}
		return createWithdrawalLedgerTx(tx, withdrawal.UserId, model.InvitationCommissionLedgerWithdrawalCompleted, withdrawal.AmountCents, withdrawal.Id, now)
	})
}

func RejectInvitationCommissionWithdrawal(withdrawalId int, reviewerId int, adminRemark string) error {
	if reviewerId <= 0 {
		return errors.New("invalid reviewer id")
	}
	return model.DB.Transaction(func(tx *gorm.DB) error {
		var withdrawal model.InvitationCommissionWithdrawal
		if err := tx.Where("id = ?", withdrawalId).First(&withdrawal).Error; err != nil {
			return err
		}
		now := common.GetTimestamp()
		updated := tx.Model(&model.InvitationCommissionWithdrawal{}).
			Where("id = ? AND status = ?", withdrawalId, model.InvitationCommissionWithdrawalPending).
			Updates(map[string]any{
				"status":       model.InvitationCommissionWithdrawalRejected,
				"admin_remark": strings.TrimSpace(adminRemark),
				"reviewer_id":  reviewerId,
				"reviewed_at":  now,
				"updated_at":   now,
			})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected == 0 {
			return errors.New("返现申请状态已变化，必须为 pending")
		}
		accountUpdate := tx.Model(&model.InvitationCommissionAccount{}).
			Where("user_id = ? AND pending_cents >= ?", withdrawal.UserId, withdrawal.AmountCents).
			Updates(map[string]any{
				"pending_cents":   gorm.Expr("pending_cents - ?", withdrawal.AmountCents),
				"available_cents": gorm.Expr("available_cents + ?", withdrawal.AmountCents),
				"updated_at":      now,
			})
		if accountUpdate.Error != nil {
			return accountUpdate.Error
		}
		if accountUpdate.RowsAffected == 0 {
			return errors.New("返佣冻结余额不足")
		}
		return createWithdrawalLedgerTx(tx, withdrawal.UserId, model.InvitationCommissionLedgerWithdrawalRejected, withdrawal.AmountCents, withdrawal.Id, now)
	})
}

func RetryPendingInvitationRewardEvents(limit int) (int, error) {
	if limit <= 0 {
		limit = invitationRewardEventRetryBatchSize
	}
	processed := 0
	lastId := 0
	for {
		batchSize := limit
		if batchSize < invitationRewardEventRetryBatchSize {
			batchSize = invitationRewardEventRetryBatchSize
		}
		var events []model.InvitationRewardEvent
		if err := model.DB.Table("invitation_reward_events AS events").
			Select("events.*").
			Joins("LEFT JOIN invitation_commission_records AS records ON records.source_type = events.source_type AND records.source_id = events.source_id").
			Where("events.status = ?", model.InvitationRewardEventStatusActive).
			Where("records.id IS NULL").
			Where("events.id > ?", lastId).
			Order("events.id asc").
			Limit(batchSize).
			Find(&events).Error; err != nil {
			return processed, err
		}
		if len(events) == 0 {
			return processed, nil
		}
		for _, event := range events {
			lastId = event.Id
			created, err := createInvitationCommissionForRewardEvent(event.Id)
			if err != nil {
				return processed, err
			}
			if created {
				processed++
				if processed >= limit {
					return processed, nil
				}
			}
		}
	}
}

func StartInvitationRewardEventRetryTask() {
	invitationRewardEventRetryOnce.Do(func() {
		if !common.IsMasterNode {
			return
		}
		go func() {
			common.SysLog("invitation reward event retry task started")
			ticker := time.NewTicker(invitationRewardEventRetryInterval)
			defer ticker.Stop()
			for {
				runInvitationRewardEventRetryOnce()
				<-ticker.C
			}
		}()
	})
}

func CountPendingInvitationCommissionWithdrawals() (int64, error) {
	var count int64
	err := model.DB.Model(&model.InvitationCommissionWithdrawal{}).
		Where("status = ?", model.InvitationCommissionWithdrawalPending).
		Count(&count).Error
	return count, err
}

func GetInvitationCommissionSummary(userId int) (*InvitationCommissionSummary, error) {
	var summary InvitationCommissionSummary
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		var user model.User
		if err := tx.Select("id", "invitation_reward_mode").Where("id = ?", userId).First(&user).Error; err != nil {
			return err
		}
		now := common.GetTimestamp()
		direct, err := countDirectInviteesTx(tx, userId)
		if err != nil {
			return err
		}
		qualified, err := countQualifiedActiveInviteesTx(tx, userId, now)
		if err != nil {
			return err
		}
		account, accountExists, err := getInvitationCommissionAccountTx(tx, userId)
		if err != nil {
			return err
		}
		hasHistory := accountExists
		if !hasHistory {
			hasHistory, err = hasInvitationCommissionHistoryTx(tx, userId)
			if err != nil {
				return err
			}
		}
		setting := *operation_setting.GetInvitationCommissionSetting()
		summary.RewardMode = user.NormalizedInvitationRewardMode()
		summary.HasCommissionAccount = hasHistory
		summary.DirectInviteCount = direct
		summary.QualifiedPaidInviteCount = qualified
		summary.Setting = setting
		if accountExists {
			summary.Account = invitationCommissionAccountSummary(account)
		}
		summary.CanTransfer = accountExists && account.AvailableCents >= maxInt64(setting.MinimumTransferCents, 1)
		summary.CanRequestWithdrawal = accountExists && account.AvailableCents >= maxInt64(setting.MinimumWithdrawCents, 1)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &summary, nil
}

func ListInvitationCommissionRecords(userId int, page int, pageSize int) (*InvitationCommissionPageResult[InvitationCommissionRecordResponse], error) {
	page, pageSize = normalizeInvitationCommissionPage(page, pageSize)
	query := model.DB.Model(&model.InvitationCommissionRecord{}).Where("inviter_id = ?", userId)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}
	var records []model.InvitationCommissionRecord
	if err := query.Order("id desc").Limit(pageSize).Offset((page - 1) * pageSize).Find(&records).Error; err != nil {
		return nil, err
	}
	items := make([]InvitationCommissionRecordResponse, 0, len(records))
	for _, record := range records {
		items = append(items, invitationCommissionRecordResponse(record))
	}
	return &InvitationCommissionPageResult[InvitationCommissionRecordResponse]{Items: items, Total: total, Page: page, PageSize: pageSize}, nil
}

func ListInvitationCommissionWithdrawals(userId int, page int, pageSize int) (*InvitationCommissionPageResult[InvitationCommissionWithdrawalResponse], error) {
	page, pageSize = normalizeInvitationCommissionPage(page, pageSize)
	query := model.DB.Model(&model.InvitationCommissionWithdrawal{}).Where("user_id = ?", userId)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}
	var withdrawals []model.InvitationCommissionWithdrawal
	if err := query.Order("id desc").Limit(pageSize).Offset((page - 1) * pageSize).Find(&withdrawals).Error; err != nil {
		return nil, err
	}
	items := make([]InvitationCommissionWithdrawalResponse, 0, len(withdrawals))
	for _, withdrawal := range withdrawals {
		item, err := invitationCommissionWithdrawalResponse(withdrawal)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return &InvitationCommissionPageResult[InvitationCommissionWithdrawalResponse]{Items: items, Total: total, Page: page, PageSize: pageSize}, nil
}

func AdminListInvitationCommissionWithdrawals(filter InvitationCommissionWithdrawalFilter) (*InvitationCommissionPageResult[AdminInvitationCommissionWithdrawalResponse], error) {
	page, pageSize := normalizeInvitationCommissionPage(filter.Page, filter.PageSize)
	query := model.DB.Model(&model.InvitationCommissionWithdrawal{})
	if status := normalizeInvitationCommissionWithdrawalStatusFilter(filter.Status); status != "" {
		query = query.Where("status = ?", status)
	}
	if filter.UserId > 0 {
		query = query.Where("user_id = ?", filter.UserId)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}
	var withdrawals []model.InvitationCommissionWithdrawal
	if err := query.Order("id desc").Limit(pageSize).Offset((page - 1) * pageSize).Find(&withdrawals).Error; err != nil {
		return nil, err
	}
	users, err := invitationCommissionUsersByID(withdrawals)
	if err != nil {
		return nil, err
	}
	items := make([]AdminInvitationCommissionWithdrawalResponse, 0, len(withdrawals))
	for _, withdrawal := range withdrawals {
		base, err := invitationCommissionWithdrawalResponse(withdrawal)
		if err != nil {
			return nil, err
		}
		item := AdminInvitationCommissionWithdrawalResponse{InvitationCommissionWithdrawalResponse: base}
		if user, ok := users[withdrawal.UserId]; ok {
			item.Username = user.Username
			item.DisplayName = user.DisplayName
		}
		items = append(items, item)
	}
	return &InvitationCommissionPageResult[AdminInvitationCommissionWithdrawalResponse]{Items: items, Total: total, Page: page, PageSize: pageSize}, nil
}

func createInvitationCommissionForRewardEvent(eventId int) (bool, error) {
	if eventId <= 0 {
		return false, nil
	}
	createdAvailable := false
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		var event model.InvitationRewardEvent
		if err := tx.Where("id = ? AND status = ?", eventId, model.InvitationRewardEventStatusActive).First(&event).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		inviter, ok, err := getCommissionInviterTx(tx, event.InviterId)
		if err != nil || !ok {
			return err
		}
		eligible, err := invitationRewardEventSourceEligibleTx(tx, &event)
		if err != nil || !eligible {
			return err
		}
		setting := *operation_setting.GetInvitationCommissionSetting()
		if err := operation_setting.ValidateInvitationCommissionSetting(setting); err != nil {
			return err
		}
		if !setting.Enabled || setting.RateBps <= 0 {
			return nil
		}
		reason := invitationCommissionUnsettleableReason(&event, setting.RateBps)
		commissionCents := int64(0)
		if reason == "" {
			commissionCents = event.SourceAmountCents * int64(setting.RateBps) / 10000
			if commissionCents <= 0 {
				return nil
			}
		}
		now := common.GetTimestamp()
		record := model.InvitationCommissionRecord{
			EventId:           event.Id,
			InviterId:         inviter.Id,
			InviteeId:         event.InviteeId,
			SourceType:        event.SourceType,
			SourceId:          event.SourceId,
			SourceTradeNo:     invitationCommissionSourceTradeNoTx(tx, &event),
			SourceAmountCents: event.SourceAmountCents,
			SourceCurrency:    event.SourceCurrency,
			CommissionRateBps: setting.RateBps,
			CommissionCents:   commissionCents,
			Status:            model.InvitationCommissionStatusAvailable,
			CreatedAt:         now,
			AvailableAt:       now,
		}
		if reason != "" {
			record.Status = model.InvitationCommissionStatusSkipped
			record.Reason = reason
			record.CommissionCents = 0
			record.AvailableAt = 0
		}
		insert := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "source_type"}, {Name: "source_id"}}, DoNothing: true}).Create(&record)
		if insert.Error != nil {
			return insert.Error
		}
		if insert.RowsAffected == 0 || record.Status == model.InvitationCommissionStatusSkipped {
			return nil
		}
		if err := ensureInvitationCommissionAccountTx(tx, inviter.Id, now); err != nil {
			return err
		}
		updated := tx.Model(&model.InvitationCommissionAccount{}).
			Where("user_id = ?", inviter.Id).
			Updates(map[string]any{
				"available_cents": gorm.Expr("available_cents + ?", commissionCents),
				"updated_at":      now,
			})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected == 0 {
			return errors.New("返佣账户不存在")
		}
		var account model.InvitationCommissionAccount
		if err := tx.Where("user_id = ?", inviter.Id).First(&account).Error; err != nil {
			return err
		}
		ledger := model.InvitationCommissionLedger{
			UserId:              inviter.Id,
			Type:                model.InvitationCommissionLedgerEarned,
			AmountCents:         commissionCents,
			AvailableAfterCents: account.AvailableCents,
			PendingAfterCents:   account.PendingCents,
			ReferenceType:       invitationCommissionRecordReference,
			ReferenceId:         record.Id,
			CreatedAt:           now,
		}
		if err := tx.Create(&ledger).Error; err != nil {
			return err
		}
		createdAvailable = true
		return nil
	})
	return createdAvailable, err
}

func getInvitationRewardEventBySource(sourceType string, sourceId int) (*model.InvitationRewardEvent, error) {
	var event model.InvitationRewardEvent
	err := model.DB.Where("source_type = ? AND source_id = ?", sourceType, sourceId).First(&event).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &event, nil
}

func dispatchInvitationRewardEvent(event *model.InvitationRewardEvent) error {
	if event == nil {
		return nil
	}
	var inviter model.User
	err := model.DB.Select("id", "invitation_reward_mode").Where("id = ?", event.InviterId).First(&inviter).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	if inviter.NormalizedInvitationRewardMode() == model.InvitationRewardModeCommission {
		return CreateInvitationCommissionForRewardEvent(event.Id)
	}
	TryEnsureInvitationEntitlementForPaidUser(event.InviteeId)
	return nil
}

func getCommissionInviterTx(tx *gorm.DB, inviterId int) (*model.User, bool, error) {
	if inviterId <= 0 {
		return nil, false, nil
	}
	var inviter model.User
	if err := tx.Select("id", "invitation_reward_mode").Where("id = ?", inviterId).First(&inviter).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, false, nil
		}
		return nil, false, err
	}
	if inviter.NormalizedInvitationRewardMode() != model.InvitationRewardModeCommission {
		return nil, false, nil
	}
	return &inviter, true, nil
}

func invitationRewardEventSourceEligibleTx(tx *gorm.DB, event *model.InvitationRewardEvent) (bool, error) {
	if event == nil || event.SourceSubscriptionId <= 0 {
		return false, nil
	}
	now := common.GetTimestamp()
	var row struct {
		SubscriptionId int
		Status         string
		StartTime      int64
		EndTime        int64
		ConvertedAt    int64
		GrantReason    string
		Source         string
		RewardEligible bool
		IsTrial        bool
		InviteTrial    bool
		BusinessCode   *string
	}
	err := tx.Table("user_subscriptions").
		Select("user_subscriptions.id AS subscription_id, user_subscriptions.status, user_subscriptions.start_time, user_subscriptions.end_time, user_subscriptions.converted_at, user_subscriptions.grant_reason, user_subscriptions.source, subscription_plans.reward_eligible, subscription_plans.is_trial, subscription_plans.invite_trial, subscription_plans.business_code").
		Joins("JOIN subscription_plans ON subscription_plans.id = user_subscriptions.plan_id").
		Where("user_subscriptions.id = ?", event.SourceSubscriptionId).
		Where("user_subscriptions.entitlement_type = ?", model.SubscriptionEntitlementTimed).
		Where("subscription_plans.entitlement_type = ?", model.SubscriptionEntitlementTimed).
		Scan(&row).Error
	if err != nil {
		return false, err
	}
	activeNow := row.Status == model.SubscriptionStatusActive && row.StartTime <= now && row.EndTime > now
	convertedHistory := row.Status == model.SubscriptionStatusConverted && row.ConvertedAt > 0 && event.CreatedAt > 0 && event.CreatedAt < row.ConvertedAt
	if row.SubscriptionId == 0 || (!activeNow && !convertedHistory) {
		return false, nil
	}
	if !row.RewardEligible || row.IsTrial || row.InviteTrial {
		return false, nil
	}
	if strings.TrimSpace(row.GrantReason) == model.SubscriptionGrantMonthlyInviteEntitlement || strings.TrimSpace(row.Source) == model.SubscriptionGrantMonthlyInviteEntitlement {
		return false, nil
	}
	if row.BusinessCode != nil && strings.TrimSpace(*row.BusinessCode) == model.SubscriptionGrantMonthlyInviteEntitlement {
		return false, nil
	}
	return true, nil
}

func invitationCommissionUnsettleableReason(event *model.InvitationRewardEvent, rateBps int) string {
	if event.SourceAmountCents <= 0 {
		return model.InvitationCommissionReasonInvalidSourceAmount
	}
	currency := strings.ToUpper(strings.TrimSpace(event.SourceCurrency))
	if currency != "CNY" {
		return model.InvitationCommissionReasonUnsupportedCurrency
	}
	if rateBps > 0 && event.SourceAmountCents > math.MaxInt64/int64(rateBps) {
		return model.InvitationCommissionReasonCommissionOverflow
	}
	return ""
}

func invitationCommissionSourceTradeNoTx(tx *gorm.DB, event *model.InvitationRewardEvent) string {
	if event == nil || event.SourceOrderId <= 0 {
		return ""
	}
	var order model.SubscriptionOrder
	if err := tx.Select("trade_no").Where("id = ?", event.SourceOrderId).First(&order).Error; err != nil {
		return ""
	}
	return order.TradeNo
}

func ensureInvitationCommissionAccountTx(tx *gorm.DB, userId int, now int64) error {
	account := model.InvitationCommissionAccount{UserId: userId, CreatedAt: now, UpdatedAt: now}
	return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "user_id"}}, DoNothing: true}).Create(&account).Error
}

func getInvitationCommissionAccountTx(tx *gorm.DB, userId int) (*model.InvitationCommissionAccount, bool, error) {
	var account model.InvitationCommissionAccount
	err := tx.Where("user_id = ?", userId).First(&account).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &account, false, nil
		}
		return nil, false, err
	}
	return &account, true, nil
}

func hasInvitationCommissionHistoryTx(tx *gorm.DB, userId int) (bool, error) {
	var count int64
	if err := tx.Model(&model.InvitationCommissionRecord{}).Where("inviter_id = ?", userId).Count(&count).Error; err != nil {
		return false, err
	}
	if count > 0 {
		return true, nil
	}
	if err := tx.Model(&model.InvitationCommissionWithdrawal{}).Where("user_id = ?", userId).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func loadUsableCommissionAccountTx(tx *gorm.DB, userId int) (*model.InvitationCommissionAccount, error) {
	var user model.User
	if err := tx.Select("id", "invitation_reward_mode").Where("id = ?", userId).First(&user).Error; err != nil {
		return nil, err
	}
	account, exists, err := getInvitationCommissionAccountTx(tx, userId)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.New("返佣账户不存在")
	}
	if user.NormalizedInvitationRewardMode() != model.InvitationRewardModeCommission && account.AvailableCents <= 0 {
		return nil, errors.New("当前用户没有可用返佣余额")
	}
	return account, nil
}

func createWithdrawalLedgerTx(tx *gorm.DB, userId int, ledgerType string, amountCents int64, withdrawalId int, now int64) error {
	var account model.InvitationCommissionAccount
	if err := tx.Where("user_id = ?", userId).First(&account).Error; err != nil {
		return err
	}
	ledger := model.InvitationCommissionLedger{
		UserId:              userId,
		Type:                ledgerType,
		AmountCents:         amountCents,
		AvailableAfterCents: account.AvailableCents,
		PendingAfterCents:   account.PendingCents,
		ReferenceType:       invitationCommissionWithdrawalReference,
		ReferenceId:         withdrawalId,
		CreatedAt:           now,
	}
	return tx.Create(&ledger).Error
}

func normalizeInvitationCommissionWithdrawalRequest(req InvitationCommissionWithdrawalRequest) (InvitationCommissionContact, string, error) {
	contact := InvitationCommissionContact{Type: strings.TrimSpace(req.Contact.Type), Value: strings.TrimSpace(req.Contact.Value)}
	switch contact.Type {
	case "wechat", "telegram", "email", "other":
	default:
		return contact, "", errors.New("返现联系方式类型无效")
	}
	if contact.Value == "" || len([]rune(contact.Value)) > 128 {
		return contact, "", errors.New("返现联系方式无效")
	}
	remark := strings.TrimSpace(req.Remark)
	if len([]rune(remark)) > 500 {
		return contact, "", errors.New("返现备注过长")
	}
	return contact, remark, nil
}

func runInvitationRewardEventRetryOnce() {
	if !invitationRewardEventRetryRunning.CompareAndSwap(false, true) {
		return
	}
	defer invitationRewardEventRetryRunning.Store(false)
	processed, err := RetryPendingInvitationRewardEvents(invitationRewardEventRetryBatchSize)
	if err != nil {
		common.SysError(fmt.Sprintf("invitation reward event retry failed: %v", err))
		return
	}
	if processed > 0 {
		common.SysLog(fmt.Sprintf("invitation reward event retry processed=%d", processed))
	}
}

func invitationCommissionAccountSummary(account *model.InvitationCommissionAccount) InvitationCommissionAccountSummary {
	if account == nil {
		return InvitationCommissionAccountSummary{}
	}
	return InvitationCommissionAccountSummary{
		AvailableCents:   account.AvailableCents,
		PendingCents:     account.PendingCents,
		WithdrawnCents:   account.WithdrawnCents,
		TransferredCents: account.TransferredCents,
	}
}

func invitationCommissionRecordResponse(record model.InvitationCommissionRecord) InvitationCommissionRecordResponse {
	return InvitationCommissionRecordResponse{
		Id:                record.Id,
		EventId:           record.EventId,
		InviteeId:         record.InviteeId,
		SourceType:        record.SourceType,
		SourceId:          record.SourceId,
		SourceTradeNo:     record.SourceTradeNo,
		SourceAmountCents: record.SourceAmountCents,
		SourceCurrency:    record.SourceCurrency,
		CommissionRateBps: record.CommissionRateBps,
		CommissionCents:   record.CommissionCents,
		Status:            record.Status,
		Reason:            record.Reason,
		CreatedAt:         record.CreatedAt,
		AvailableAt:       record.AvailableAt,
		CancelledAt:       record.CancelledAt,
		ReversalStatus:    record.ReversalStatus,
		RecoveredCents:    record.RecoveredCents,
		UnrecoveredCents:  record.UnrecoveredCents,
		ReversalReason:    record.ReversalReason,
		ReversedAt:        record.ReversedAt,
	}
}

func invitationCommissionWithdrawalResponse(withdrawal model.InvitationCommissionWithdrawal) (InvitationCommissionWithdrawalResponse, error) {
	contact, err := invitationCommissionContactFromSnapshot(withdrawal.ContactSnapshot)
	if err != nil {
		return InvitationCommissionWithdrawalResponse{}, err
	}
	return InvitationCommissionWithdrawalResponse{
		Id:          withdrawal.Id,
		UserId:      withdrawal.UserId,
		AmountCents: withdrawal.AmountCents,
		Status:      withdrawal.Status,
		Method:      withdrawal.Method,
		Contact:     contact,
		UserRemark:  withdrawal.UserRemark,
		AdminRemark: withdrawal.AdminRemark,
		ReviewerId:  withdrawal.ReviewerId,
		ReviewedAt:  withdrawal.ReviewedAt,
		CompletedBy: withdrawal.CompletedBy,
		CompletedAt: withdrawal.CompletedAt,
		CreatedAt:   withdrawal.CreatedAt,
		UpdatedAt:   withdrawal.UpdatedAt,
	}, nil
}

func invitationCommissionContactFromSnapshot(snapshot string) (InvitationCommissionContact, error) {
	if strings.TrimSpace(snapshot) == "" {
		return InvitationCommissionContact{}, nil
	}
	var contact InvitationCommissionContact
	if err := common.UnmarshalJsonStr(snapshot, &contact); err != nil {
		return InvitationCommissionContact{}, err
	}
	return contact, nil
}

func invitationCommissionUsersByID(withdrawals []model.InvitationCommissionWithdrawal) (map[int]model.User, error) {
	ids := make([]int, 0, len(withdrawals))
	seen := make(map[int]struct{}, len(withdrawals))
	for _, withdrawal := range withdrawals {
		if withdrawal.UserId <= 0 {
			continue
		}
		if _, ok := seen[withdrawal.UserId]; ok {
			continue
		}
		seen[withdrawal.UserId] = struct{}{}
		ids = append(ids, withdrawal.UserId)
	}
	if len(ids) == 0 {
		return map[int]model.User{}, nil
	}
	var users []model.User
	if err := model.DB.Select("id", "username", "display_name").Where("id IN ?", ids).Find(&users).Error; err != nil {
		return nil, err
	}
	result := make(map[int]model.User, len(users))
	for _, user := range users {
		result[user.Id] = user
	}
	return result, nil
}

func normalizeInvitationCommissionPage(page int, pageSize int) (int, int) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
}

func normalizeInvitationCommissionWithdrawalStatusFilter(status string) string {
	switch strings.TrimSpace(status) {
	case model.InvitationCommissionWithdrawalPending, model.InvitationCommissionWithdrawalCompleted, model.InvitationCommissionWithdrawalRejected:
		return strings.TrimSpace(status)
	default:
		return ""
	}
}

func maxInt64(left int64, right int64) int64 {
	if left > right {
		return left
	}
	return right
}
