package service

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"gorm.io/gorm"
)

const (
	gptAbuseAdminDefaultLimit     = 20
	gptAbuseAdminMaxLimit         = 100
	gptAbuseAdminMaxWindowSeconds = 7 * 24 * 60 * 60
	gptAbuseReasonDefault         = "manual_review"
	gptAbuseFingerprintPrefix     = 12
)

type gptAbuseUserAggregate struct {
	item        dto.GPTAbuseUserListItem
	latestLog   model.GPTAbuseSignalLog
	logs        []model.GPTAbuseSignalLog
	effective   int
	resetCutoff int
}

type gptAbuseUserAggregateRow struct {
	UserID       int `gorm:"column:user_id"`
	WarningCount int `gorm:"column:warning_count"`
	HighCount    int `gorm:"column:high_count"`
	MediumCount  int `gorm:"column:medium_count"`
}

type gptAbuseRepeatSummaryRow struct {
	UserID         int   `gorm:"column:user_id"`
	RepeatCount    int64 `gorm:"column:repeat_count"`
	LatestRepeatAt int64 `gorm:"column:latest_repeat_at"`
}

type gptAbuseSignalIDRow struct {
	UserID int `gorm:"column:user_id"`
	ID     int `gorm:"column:id"`
}

type gptAbuseLatestSignalRow struct {
	UserID          int   `gorm:"column:user_id"`
	LatestCreatedAt int64 `gorm:"column:latest_created_at"`
	LatestID        int   `gorm:"column:latest_id"`
}

func ListGPTAbuseUsers(ctx context.Context, query dto.GPTAbuseUserListQuery) (*dto.GPTAbuseUserListResponse, error) {
	query = normalizeGPTAbuseUserListQuery(query)
	rows, err := loadGPTAbuseUserAggregateRows(ctx, query)
	if err != nil {
		return nil, err
	}

	aggregates := make(map[int]*gptAbuseUserAggregate, len(rows))
	userIDs := make([]int, 0, len(rows))
	for _, row := range rows {
		if row.UserID <= 0 {
			continue
		}
		aggregate := &gptAbuseUserAggregate{}
		aggregate.item.UserID = row.UserID
		aggregate.item.WarningCount = row.WarningCount
		aggregate.item.HighCount = row.HighCount
		aggregate.item.MediumCount = row.MediumCount
		aggregate.item.SuspensionStatus = "none"
		aggregates[row.UserID] = aggregate
		userIDs = append(userIDs, row.UserID)
	}

	latestLogs, err := loadLatestGPTAbuseSignalLogsForUsers(ctx, query, userIDs)
	if err != nil {
		return nil, err
	}
	for userID, log := range latestLogs {
		aggregate := aggregates[userID]
		if aggregate == nil {
			continue
		}
		aggregate.latestLog = log
		aggregate.item.Username = log.Username
		aggregate.item.UserEmail = log.UserEmail
		aggregate.item.MaxSeverity = log.Severity
		aggregate.item.LatestWarningAt = log.CreatedAt
		aggregate.item.LatestKind = log.Kind
		aggregate.item.LatestSource = log.Source
		aggregate.item.LatestRequestedModel = log.RequestedModel
		aggregate.item.LatestUpstreamModel = log.UpstreamModel
		aggregate.item.LatestChannelID = log.ChannelId
		aggregate.item.LatestChannelName = log.ChannelName
	}
	for _, aggregate := range aggregates {
		if aggregate.item.HighCount > 0 {
			aggregate.item.MaxSeverity = model.GPTAbuseSeverityHigh
		} else if aggregate.item.MediumCount > 0 {
			aggregate.item.MaxSeverity = model.GPTAbuseSeverityMedium
		}
	}

	if err := enrichGPTAbuseUserAggregates(ctx, aggregates, query); err != nil {
		return nil, err
	}

	items := make([]dto.GPTAbuseUserListItem, 0, len(aggregates))
	for _, aggregate := range aggregates {
		if !gptAbuseUserMatchesKeyword(aggregate.item, query.Keyword) {
			continue
		}
		if !gptAbuseUserMatchesStatus(aggregate.item, query.Status) {
			continue
		}
		items = append(items, aggregate.item)
	}
	sortGPTAbuseUserItems(items, query.SortBy, query.SortOrder)
	total := len(items)
	items = pageGPTAbuseUserItems(items, query.Offset, query.Limit)

	return &dto.GPTAbuseUserListResponse{Items: items, Total: total, Limit: query.Limit, Offset: query.Offset, StartTimestamp: query.StartTimestamp, EndTimestamp: query.EndTimestamp}, nil
}

func ListGPTAbuseUserLogs(ctx context.Context, userID int, query dto.GPTAbuseLogQuery) (*dto.GPTAbuseLogListResponse, error) {
	if userID <= 0 {
		return nil, errors.New("invalid user id")
	}
	query = normalizeGPTAbuseLogQuery(query)
	db := dbWithContext(ctx).Model(&model.GPTAbuseSignalLog{}).Where("user_id = ? AND created_at >= ? AND created_at < ?", userID, query.StartTimestamp, query.EndTimestamp)
	db = applyGPTAbuseSignalLogFilters(db, query.Source, query.Kind, query.Severity)
	switch strings.TrimSpace(query.CountEligible) {
	case "true", "1":
		db = db.Where("count_eligible = ?", true)
	case "false", "0":
		db = db.Where("count_eligible = ?", false)
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, err
	}
	var logs []model.GPTAbuseSignalLog
	if err := db.Order("created_at desc, id desc").Limit(query.Limit).Offset(query.Offset).Find(&logs).Error; err != nil {
		return nil, err
	}
	items := make([]dto.GPTAbuseSignalLogItem, 0, len(logs))
	for _, log := range logs {
		items = append(items, gptAbuseSignalLogItem(log))
	}
	return &dto.GPTAbuseLogListResponse{Items: items, Total: int(total), Limit: query.Limit, Offset: query.Offset, StartTimestamp: query.StartTimestamp, EndTimestamp: query.EndTimestamp}, nil
}

func ListGPTAbuseRepeatBlocks(ctx context.Context, userID int, query dto.GPTAbuseRepeatBlockQuery) (*dto.GPTAbuseRepeatBlockListResponse, error) {
	if userID <= 0 {
		return nil, errors.New("invalid user id")
	}
	query = normalizeGPTAbuseRepeatBlockQuery(query)
	db := dbWithContext(ctx).Model(&model.GPTAbuseRepeatBlockLog{}).Where("user_id = ? AND created_at >= ? AND created_at < ?", userID, query.StartTimestamp, query.EndTimestamp)
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, err
	}
	var logs []model.GPTAbuseRepeatBlockLog
	if err := db.Order("created_at desc, id desc").Limit(query.Limit).Offset(query.Offset).Find(&logs).Error; err != nil {
		return nil, err
	}
	items := make([]dto.GPTAbuseRepeatBlockItem, 0, len(logs))
	for _, log := range logs {
		items = append(items, gptAbuseRepeatBlockItem(log))
	}
	return &dto.GPTAbuseRepeatBlockListResponse{Items: items, Total: int(total), Limit: query.Limit, Offset: query.Offset, StartTimestamp: query.StartTimestamp, EndTimestamp: query.EndTimestamp}, nil
}

func ClearGPTAbuseSuspension(ctx context.Context, adminID int, userID int, reason string) (*dto.GPTAbuseClearSuspensionResponse, error) {
	if userID <= 0 {
		return nil, errors.New("invalid user id")
	}
	if adminID <= 0 {
		return nil, errors.New("invalid admin id")
	}
	if _, err := normalizeGPTAbuseReason(reason); err != nil {
		return nil, err
	}
	response := &dto.GPTAbuseClearSuspensionResponse{UserID: userID}
	now := common.GetTimestamp()
	err := dbWithContext(ctx).Transaction(func(tx *gorm.DB) error {
		had, cleared, clearedID, err := clearActiveGPTAbuseSuspensionTx(tx, adminID, userID, now)
		if err != nil {
			return err
		}
		response.HadActiveSuspension = had
		response.SuspensionCleared = cleared
		response.ClearedSuspensionID = clearedID
		return nil
	})
	if err != nil {
		return nil, err
	}
	return response, nil
}

func ResetGPTAbuseWarnings(ctx context.Context, adminID int, userID int, reason string, clearSuspension bool) (*dto.GPTAbuseResetWarningsResponse, error) {
	if userID <= 0 {
		return nil, errors.New("invalid user id")
	}
	if adminID <= 0 {
		return nil, errors.New("invalid admin id")
	}
	reason, err := normalizeGPTAbuseReason(reason)
	if err != nil {
		return nil, err
	}
	response := &dto.GPTAbuseResetWarningsResponse{UserID: userID, EffectiveWarningCount: 0}
	now := common.GetTimestamp()
	start, end := model.GPTAbuseDayWindow(now)
	err = dbWithContext(ctx).Transaction(func(tx *gorm.DB) error {
		previousRaw, err := model.CountGPTAbuseSignalsForUserRawTx(tx, userID, start, end)
		if err != nil {
			return err
		}
		previousEffective, _, err := model.CountEffectiveGPTAbuseSignalsForUserTx(tx, userID, start, end)
		if err != nil {
			return err
		}
		cutoffID, err := model.MaxGPTAbuseSignalLogIDForUserWindowTx(tx, userID, start, end)
		if err != nil {
			return err
		}
		reset := &model.GPTAbuseWarningReset{UserId: userID, WindowStart: start, WindowEnd: end, ResetAt: now, ResetBy: adminID, PreviousRawCount: previousRaw, PreviousCount: previousEffective, CutoffSignalLogID: cutoffID, Reason: reason}
		if err := model.CreateGPTAbuseWarningResetTx(tx, reset); err != nil {
			return err
		}
		response.ResetID = reset.Id
		response.WindowStart = start
		response.WindowEnd = end
		response.ResetAt = reset.ResetAt
		response.PreviousRawCount = previousRaw
		response.PreviousEffectiveCount = previousEffective
		response.CutoffSignalLogID = cutoffID
		if clearSuspension {
			had, cleared, clearedID, err := clearActiveGPTAbuseSuspensionTx(tx, adminID, userID, now)
			if err != nil {
				return err
			}
			response.HadActiveSuspension = had
			response.SuspensionCleared = cleared
			response.ClearedSuspensionID = clearedID
			return nil
		}
		suspension, err := findActiveGPTAbuseSuspensionTx(tx, userID, now)
		if err != nil {
			return err
		}
		response.HadActiveSuspension = suspension != nil
		return nil
	})
	if err != nil {
		return nil, err
	}
	return response, nil
}

func loadGPTAbuseSignalLogsForUserList(ctx context.Context, query dto.GPTAbuseUserListQuery) ([]model.GPTAbuseSignalLog, error) {
	db := dbWithContext(ctx).Where("created_at >= ? AND created_at < ?", query.StartTimestamp, query.EndTimestamp)
	if query.UserID > 0 {
		db = db.Where("user_id = ?", query.UserID)
	}
	db = applyGPTAbuseSignalLogFilters(db, query.Source, query.Kind, query.Severity)
	var logs []model.GPTAbuseSignalLog
	if err := db.Order("created_at desc, id desc").Find(&logs).Error; err != nil {
		return nil, err
	}
	return logs, nil
}

func loadGPTAbuseUserAggregateRows(ctx context.Context, query dto.GPTAbuseUserListQuery) ([]gptAbuseUserAggregateRow, error) {
	db := dbWithContext(ctx).Model(&model.GPTAbuseSignalLog{}).
		Select("user_id, SUM(CASE WHEN count_eligible = ? THEN 1 ELSE 0 END) AS warning_count, SUM(CASE WHEN severity = ? THEN 1 ELSE 0 END) AS high_count, SUM(CASE WHEN severity = ? THEN 1 ELSE 0 END) AS medium_count", true, model.GPTAbuseSeverityHigh, model.GPTAbuseSeverityMedium).
		Where("created_at >= ? AND created_at < ?", query.StartTimestamp, query.EndTimestamp)
	if query.UserID > 0 {
		db = db.Where("user_id = ?", query.UserID)
	}
	db = applyGPTAbuseSignalLogFilters(db, query.Source, query.Kind, query.Severity)
	var rows []gptAbuseUserAggregateRow
	if err := db.Group("user_id").Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func loadLatestGPTAbuseSignalLogsForUsers(ctx context.Context, query dto.GPTAbuseUserListQuery, userIDs []int) (map[int]model.GPTAbuseSignalLog, error) {
	latestByUser := make(map[int]model.GPTAbuseSignalLog, len(userIDs))
	if len(userIDs) == 0 {
		return latestByUser, nil
	}
	base := dbWithContext(ctx).Model(&model.GPTAbuseSignalLog{}).Where("user_id IN ? AND created_at >= ? AND created_at < ?", userIDs, query.StartTimestamp, query.EndTimestamp)
	base = applyGPTAbuseSignalLogFilters(base, query.Source, query.Kind, query.Severity)
	var latestCreatedRows []gptAbuseLatestSignalRow
	if err := base.Select("user_id, MAX(created_at) AS latest_created_at").Group("user_id").Scan(&latestCreatedRows).Error; err != nil {
		return nil, err
	}
	if len(latestCreatedRows) == 0 {
		return latestByUser, nil
	}
	latestCreatedByUser := make(map[int]int64, len(latestCreatedRows))
	for _, row := range latestCreatedRows {
		if row.UserID <= 0 || row.LatestCreatedAt <= 0 {
			continue
		}
		latestCreatedByUser[row.UserID] = row.LatestCreatedAt
	}
	if len(latestCreatedByUser) == 0 {
		return latestByUser, nil
	}
	base = dbWithContext(ctx).Model(&model.GPTAbuseSignalLog{}).Where("user_id IN ? AND created_at >= ? AND created_at < ?", userIDs, query.StartTimestamp, query.EndTimestamp)
	base = applyGPTAbuseSignalLogFilters(base, query.Source, query.Kind, query.Severity)
	var candidateRows []gptAbuseLatestSignalRow
	if err := base.Select("user_id, created_at AS latest_created_at, MAX(id) AS latest_id").Group("user_id, created_at").Scan(&candidateRows).Error; err != nil {
		return nil, err
	}
	latestIDs := make([]int, 0, len(latestCreatedByUser))
	for _, row := range candidateRows {
		if row.LatestID <= 0 || latestCreatedByUser[row.UserID] != row.LatestCreatedAt {
			continue
		}
		latestIDs = append(latestIDs, row.LatestID)
	}
	if len(latestIDs) == 0 {
		return latestByUser, nil
	}
	var logs []model.GPTAbuseSignalLog
	if err := dbWithContext(ctx).Where("id IN ?", latestIDs).Find(&logs).Error; err != nil {
		return nil, err
	}
	for _, log := range logs {
		latestByUser[log.UserId] = log
	}
	return latestByUser, nil
}

func enrichGPTAbuseUserAggregates(ctx context.Context, aggregates map[int]*gptAbuseUserAggregate, query dto.GPTAbuseUserListQuery) error {
	if len(aggregates) == 0 {
		return nil
	}
	userIDs := make([]int, 0, len(aggregates))
	for userID := range aggregates {
		userIDs = append(userIDs, userID)
	}
	usersByID, err := loadGPTAbuseUsersByID(ctx, userIDs)
	if err != nil {
		return err
	}
	resetsByUser, err := latestGPTAbuseWarningResetsForUsers(ctx, userIDs, query.StartTimestamp)
	if err != nil {
		return err
	}
	cutoffs := make(map[int]int)
	for userID, aggregate := range aggregates {
		if user, ok := usersByID[userID]; ok {
			if aggregate.item.Username == "" {
				aggregate.item.Username = user.Username
			}
			if aggregate.item.UserEmail == "" {
				aggregate.item.UserEmail = user.Email
			}
		}
		if reset, ok := resetsByUser[userID]; ok {
			aggregate.item.LastResetAt = reset.ResetAt
			aggregate.item.LastResetBy = reset.ResetBy
			aggregate.resetCutoff = reset.CutoffSignalLogID
			cutoffs[userID] = reset.CutoffSignalLogID
			continue
		}
		aggregate.effective = aggregate.item.WarningCount
		aggregate.item.EffectiveWarningCount = aggregate.effective
	}
	if len(cutoffs) > 0 {
		effectiveCounts, err := countEffectiveGPTAbuseSignalsAfterCutoffs(ctx, query, cutoffs)
		if err != nil {
			return err
		}
		for userID := range cutoffs {
			aggregate := aggregates[userID]
			if aggregate == nil {
				continue
			}
			aggregate.effective = effectiveCounts[userID]
			aggregate.item.EffectiveWarningCount = aggregate.effective
		}
	}
	dailyLimits, err := gptAbuseDailyLimitsForUsers(ctx, userIDs, usersByID)
	if err != nil {
		return err
	}
	suspensions, err := activeGPTAbuseSuspensionsForUsers(ctx, userIDs)
	if err != nil {
		return err
	}
	repeatSummaries, err := gptAbuseRepeatBlockSummaries(ctx, userIDs, query.StartTimestamp, query.EndTimestamp)
	if err != nil {
		return err
	}
	for userID, aggregate := range aggregates {
		dailyLimit := dailyLimits[userID]
		aggregate.item.DailyLimit = dailyLimit
		if dailyLimit > aggregate.item.EffectiveWarningCount {
			aggregate.item.RemainingWarningCount = dailyLimit - aggregate.item.EffectiveWarningCount
		}
		if suspension, ok := suspensions[userID]; ok {
			aggregate.item.SuspensionStatus = suspension.Status
			aggregate.item.ActiveSuspension = &dto.GPTAbuseActiveSuspension{ID: suspension.Id, Reason: suspension.Reason, SuspendedUntil: suspension.SuspendedUntil, DailyCount: suspension.DailyCount, DailyLimit: suspension.DailyLimit}
		} else {
			aggregate.item.SuspensionStatus = "none"
		}
		if summary, ok := repeatSummaries[userID]; ok {
			aggregate.item.RepeatBlockCount = int(summary.RepeatCount)
			aggregate.item.LatestRepeatBlockAt = summary.LatestRepeatAt
		}
	}
	return nil
}

func loadGPTAbuseUsersByID(ctx context.Context, userIDs []int) (map[int]model.User, error) {
	usersByID := make(map[int]model.User, len(userIDs))
	if len(userIDs) == 0 {
		return usersByID, nil
	}
	var users []model.User
	if err := dbWithContext(ctx).Select("id", "username", "email", "setting").Where("id IN ?", userIDs).Find(&users).Error; err != nil {
		return nil, err
	}
	for _, user := range users {
		usersByID[user.Id] = user
	}
	return usersByID, nil
}

func latestGPTAbuseWarningResetsForUsers(ctx context.Context, userIDs []int, windowStart int64) (map[int]model.GPTAbuseWarningReset, error) {
	resetsByUser := make(map[int]model.GPTAbuseWarningReset, len(userIDs))
	if len(userIDs) == 0 {
		return resetsByUser, nil
	}
	var resets []model.GPTAbuseWarningReset
	if err := dbWithContext(ctx).Where("user_id IN ? AND window_start = ?", userIDs, windowStart).Order("user_id asc, reset_at desc, id desc").Find(&resets).Error; err != nil {
		return nil, err
	}
	for _, reset := range resets {
		if _, ok := resetsByUser[reset.UserId]; !ok {
			resetsByUser[reset.UserId] = reset
		}
	}
	return resetsByUser, nil
}

func countEffectiveGPTAbuseSignalsAfterCutoffs(ctx context.Context, query dto.GPTAbuseUserListQuery, cutoffs map[int]int) (map[int]int, error) {
	counts := make(map[int]int, len(cutoffs))
	if len(cutoffs) == 0 {
		return counts, nil
	}
	userIDs := make([]int, 0, len(cutoffs))
	for userID := range cutoffs {
		userIDs = append(userIDs, userID)
	}
	db := dbWithContext(ctx).Model(&model.GPTAbuseSignalLog{}).
		Select("user_id", "id").
		Where("user_id IN ? AND count_eligible = ? AND created_at >= ? AND created_at < ?", userIDs, true, query.StartTimestamp, query.EndTimestamp)
	db = applyGPTAbuseSignalLogFilters(db, query.Source, query.Kind, query.Severity)
	var rows []gptAbuseSignalIDRow
	if err := db.Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		if row.ID > cutoffs[row.UserID] {
			counts[row.UserID]++
		}
	}
	return counts, nil
}

func gptAbuseDailyLimitsForUsers(ctx context.Context, userIDs []int, usersByID map[int]model.User) (map[int]int, error) {
	limits := make(map[int]int, len(userIDs))
	defaultLimit := defaultGPTAbuseAdminWarningLimit()
	for _, userID := range userIDs {
		limits[userID] = defaultLimit
	}
	if len(userIDs) == 0 {
		return limits, nil
	}
	now := common.GetTimestamp()
	var subs []model.UserSubscription
	if err := dbWithContext(ctx).Where("user_id IN ? AND status = ? AND end_time > ?", userIDs, "active", now).Order("user_id asc, end_time desc, id desc").Find(&subs).Error; err != nil {
		return nil, err
	}
	selectedByUser := make(map[int]model.UserSubscription, len(userIDs))
	for _, sub := range subs {
		user := usersByID[sub.UserId]
		activeID := user.GetSetting().ActiveSubscriptionId
		if activeID > 0 && sub.Id == activeID {
			selectedByUser[sub.UserId] = sub
			continue
		}
		if _, ok := selectedByUser[sub.UserId]; !ok {
			selectedByUser[sub.UserId] = sub
		}
	}
	planIDs := make([]int, 0, len(selectedByUser))
	seenPlanIDs := map[int]struct{}{}
	for _, sub := range selectedByUser {
		if sub.PlanId <= 0 {
			continue
		}
		if _, ok := seenPlanIDs[sub.PlanId]; ok {
			continue
		}
		seenPlanIDs[sub.PlanId] = struct{}{}
		planIDs = append(planIDs, sub.PlanId)
	}
	plansByID := make(map[int]model.SubscriptionPlan, len(planIDs))
	if len(planIDs) > 0 {
		var plans []model.SubscriptionPlan
		if err := dbWithContext(ctx).Where("id IN ?", planIDs).Find(&plans).Error; err != nil {
			return nil, err
		}
		for _, plan := range plans {
			plansByID[plan.Id] = plan
		}
	}
	for userID, sub := range selectedByUser {
		plan := plansByID[sub.PlanId]
		limits[userID] = gptAbuseDailyLimitFromPlanValue(&plan)
	}
	return limits, nil
}

func activeGPTAbuseSuspensionsForUsers(ctx context.Context, userIDs []int) (map[int]model.GPTAbuseUserSuspension, error) {
	suspensionsByUser := make(map[int]model.GPTAbuseUserSuspension, len(userIDs))
	if len(userIDs) == 0 {
		return suspensionsByUser, nil
	}
	var suspensions []model.GPTAbuseUserSuspension
	now := common.GetTimestamp()
	if err := dbWithContext(ctx).Where("user_id IN ? AND status = ? AND suspended_until > ?", userIDs, model.GPTAbuseSuspensionStatusActive, now).Order("user_id asc, suspended_until desc, id desc").Find(&suspensions).Error; err != nil {
		return nil, err
	}
	for _, suspension := range suspensions {
		if _, ok := suspensionsByUser[suspension.UserId]; !ok {
			suspensionsByUser[suspension.UserId] = suspension
		}
	}
	return suspensionsByUser, nil
}

func gptAbuseRepeatBlockSummaries(ctx context.Context, userIDs []int, start int64, end int64) (map[int]gptAbuseRepeatSummaryRow, error) {
	summaryByUser := make(map[int]gptAbuseRepeatSummaryRow, len(userIDs))
	if len(userIDs) == 0 {
		return summaryByUser, nil
	}
	var rows []gptAbuseRepeatSummaryRow
	if err := dbWithContext(ctx).Model(&model.GPTAbuseRepeatBlockLog{}).
		Select("user_id, COUNT(*) AS repeat_count, COALESCE(MAX(created_at), 0) AS latest_repeat_at").
		Where("user_id IN ? AND created_at >= ? AND created_at < ?", userIDs, start, end).
		Group("user_id").Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		summaryByUser[row.UserID] = row
	}
	return summaryByUser, nil
}

func defaultGPTAbuseAdminWarningLimit() int {
	if common.GPTAbuseDefaultWarningLimit < 1 {
		return 1
	}
	return common.GPTAbuseDefaultWarningLimit
}

func gptAbuseDailyLimitFromPlanValue(plan *model.SubscriptionPlan) int {
	defaultLimit := defaultGPTAbuseAdminWarningLimit()
	if plan == nil || plan.Id <= 0 || plan.GPTAbuseWarningLimit <= 0 {
		return defaultLimit
	}
	if plan.GPTAbuseWarningLimit < defaultLimit {
		return defaultLimit
	}
	return plan.GPTAbuseWarningLimit
}

func gptAbuseDailyLimitForUser(userID int) (int, error) {
	if userID <= 0 {
		return defaultGPTAbuseAdminWarningLimit(), nil
	}
	now := common.GetTimestamp()
	setting, err := model.GetUserSetting(userID, true)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, err
	}
	if setting.ActiveSubscriptionId > 0 {
		limit, found, err := gptAbuseDailyLimitFromSubscription(userID, setting.ActiveSubscriptionId, now)
		if err != nil || found {
			return limit, err
		}
	}
	var sub model.UserSubscription
	err = model.DB.Where("user_id = ? AND status = ? AND end_time > ?", userID, "active", now).Order("end_time desc, id desc").First(&sub).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return defaultGPTAbuseAdminWarningLimit(), nil
	}
	if err != nil {
		return 0, err
	}
	return gptAbuseDailyLimitFromPlan(sub.PlanId)
}

func gptAbuseDailyLimitFromSubscription(userID int, subscriptionID int, now int64) (int, bool, error) {
	var sub model.UserSubscription
	err := model.DB.Where("id = ? AND user_id = ? AND status = ? AND end_time > ?", subscriptionID, userID, "active", now).First(&sub).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	limit, err := gptAbuseDailyLimitFromPlan(sub.PlanId)
	return limit, true, err
}

func gptAbuseDailyLimitFromPlan(planID int) (int, error) {
	if planID <= 0 {
		return defaultGPTAbuseAdminWarningLimit(), nil
	}
	plan, err := model.GetSubscriptionPlanById(planID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return defaultGPTAbuseAdminWarningLimit(), nil
	}
	if err != nil {
		return 0, err
	}
	return gptAbuseDailyLimitFromPlanValue(plan), nil
}

func gptAbuseRepeatBlockSummary(ctx context.Context, userID int, start int64, end int64) (int, int64, error) {
	db := dbWithContext(ctx).Model(&model.GPTAbuseRepeatBlockLog{}).Where("user_id = ? AND created_at >= ? AND created_at < ?", userID, start, end)
	var count int64
	if err := db.Count(&count).Error; err != nil {
		return 0, 0, err
	}
	if count == 0 {
		return 0, 0, nil
	}
	var latest model.GPTAbuseRepeatBlockLog
	if err := db.Order("created_at desc, id desc").First(&latest).Error; err != nil {
		return 0, 0, err
	}
	return int(count), latest.CreatedAt, nil
}

func applyGPTAbuseSignalLogFilters(db *gorm.DB, source string, kind string, severity string) *gorm.DB {
	if source = strings.TrimSpace(source); source != "" {
		db = db.Where("source = ?", source)
	}
	if kind = strings.TrimSpace(kind); kind != "" {
		db = db.Where("kind = ?", kind)
	}
	if severity = strings.TrimSpace(severity); severity != "" {
		db = db.Where("severity = ?", severity)
	}
	return db
}

func gptAbuseSignalLogItem(log model.GPTAbuseSignalLog) dto.GPTAbuseSignalLogItem {
	return dto.GPTAbuseSignalLogItem{ID: log.Id, CreatedAt: log.CreatedAt, UserID: log.UserId, Username: log.Username, UserEmail: log.UserEmail, TokenID: log.TokenId, TokenName: log.TokenName, ChannelID: log.ChannelId, ChannelName: log.ChannelName, ChannelType: log.ChannelType, ChannelMultiKeyIndex: log.ChannelMultiKeyIndex, RequestID: log.RequestId, UpstreamRequestID: log.UpstreamRequestId, Endpoint: log.Endpoint, RelayMode: log.RelayMode, RequestedModel: log.RequestedModel, UpstreamModel: log.UpstreamModel, IsStream: log.IsStream, Source: log.Source, Kind: log.Kind, Severity: log.Severity, StatusCode: log.StatusCode, ErrorCode: log.ErrorCode, ErrorType: log.ErrorType, CountEligible: log.CountEligible, Extra: parseGPTAbuseExtra(log.Extra)}
}

func gptAbuseRepeatBlockItem(log model.GPTAbuseRepeatBlockLog) dto.GPTAbuseRepeatBlockItem {
	return dto.GPTAbuseRepeatBlockItem{ID: log.Id, CreatedAt: log.CreatedAt, UserID: log.UserId, Username: log.Username, TokenID: log.TokenId, TokenName: log.TokenName, RequestID: log.RequestId, Endpoint: log.Endpoint, RelayMode: log.RelayMode, RequestedModel: log.RequestedModel, BodyFingerprintPrefix: gptAbuseBodyFingerprintPrefix(log.BodyFingerprint), FirstWarningLogID: log.FirstWarningLogId, FirstWarningAt: log.FirstWarningAt, FirstWarningRequestID: log.FirstWarningRequestId, FirstWarningUpstreamRequestID: log.FirstWarningUpstreamRequestId, FirstWarningSource: log.FirstWarningSource, FirstWarningKind: log.FirstWarningKind, FirstWarningSeverity: log.FirstWarningSeverity, ChannelID: log.ChannelId, ChannelName: log.ChannelName, ChannelType: log.ChannelType}
}

func parseGPTAbuseExtra(raw string) any {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var envelope map[string]any
	if err := common.Unmarshal([]byte(raw), &envelope); err != nil {
		return nil
	}
	warning, ok := envelope["upstream_warning"]
	if !ok {
		return nil
	}
	return map[string]any{"upstream_warning": sanitizeGPTAbuseWarningExtra(warning)}
}

func sanitizeGPTAbuseWarningExtra(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		allowed := map[string]struct{}{"event_type": {}, "error_code": {}, "error_type": {}, "response_status": {}}
		result := make(map[string]any, len(typed))
		for key, item := range typed {
			if _, ok := allowed[key]; !ok {
				continue
			}
			result[key] = sanitizeGPTAbuseWarningExtra(item)
		}
		return result
	case []any:
		result := make([]any, 0, len(typed))
		for _, item := range typed {
			result = append(result, sanitizeGPTAbuseWarningExtra(item))
		}
		return result
	case string:
		return truncateGPTAbuseAdminExtraString(typed)
	default:
		return typed
	}
}

func truncateGPTAbuseAdminExtraString(value string) string {
	const maxRunes = 1000
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	return string(runes[:maxRunes])
}

func gptAbuseBodyFingerprintPrefix(fingerprint string) string {
	fingerprint = strings.TrimSpace(fingerprint)
	if len(fingerprint) <= gptAbuseFingerprintPrefix {
		return fingerprint
	}
	return fingerprint[:gptAbuseFingerprintPrefix]
}

func clearActiveGPTAbuseSuspensionTx(tx *gorm.DB, adminID int, userID int, now int64) (bool, bool, int, error) {
	suspension, err := findActiveGPTAbuseSuspensionTx(tx, userID, now)
	if err != nil {
		return false, false, 0, err
	}
	if suspension == nil {
		return false, false, 0, nil
	}
	updates := map[string]any{"status": model.GPTAbuseSuspensionStatusCleared, "active_user_id": nil, "cleared_at": now, "cleared_by": adminID, "updated_at": now}
	if err := tx.Model(&model.GPTAbuseUserSuspension{}).Where("id = ?", suspension.Id).Updates(updates).Error; err != nil {
		return true, false, 0, err
	}
	return true, true, suspension.Id, nil
}

func findActiveGPTAbuseSuspensionTx(tx *gorm.DB, userID int, now int64) (*model.GPTAbuseUserSuspension, error) {
	var suspension model.GPTAbuseUserSuspension
	err := tx.Where("user_id = ? AND status = ? AND suspended_until > ?", userID, model.GPTAbuseSuspensionStatusActive, now).Order("suspended_until desc, id desc").First(&suspension).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &suspension, nil
}

func normalizeGPTAbuseUserListQuery(query dto.GPTAbuseUserListQuery) dto.GPTAbuseUserListQuery {
	query.StartTimestamp, query.EndTimestamp = normalizeGPTAbuseWindow(query.StartTimestamp, query.EndTimestamp)
	query.Limit, query.Offset = normalizeGPTAbusePagination(query.Limit, query.Offset)
	query.Keyword = strings.TrimSpace(query.Keyword)
	query.Status = strings.TrimSpace(query.Status)
	if query.Status == "" {
		query.Status = "all"
	}
	query.Kind = strings.TrimSpace(query.Kind)
	query.Severity = strings.TrimSpace(query.Severity)
	query.Source = strings.TrimSpace(query.Source)
	query.SortBy = strings.TrimSpace(query.SortBy)
	if query.SortBy == "" {
		query.SortBy = "latest_warning_at"
	}
	query.SortOrder = strings.ToLower(strings.TrimSpace(query.SortOrder))
	if query.SortOrder != "asc" {
		query.SortOrder = "desc"
	}
	return query
}

func normalizeGPTAbuseLogQuery(query dto.GPTAbuseLogQuery) dto.GPTAbuseLogQuery {
	query.StartTimestamp, query.EndTimestamp = normalizeGPTAbuseWindow(query.StartTimestamp, query.EndTimestamp)
	query.Limit, query.Offset = normalizeGPTAbusePagination(query.Limit, query.Offset)
	query.Source = strings.TrimSpace(query.Source)
	query.Kind = strings.TrimSpace(query.Kind)
	query.Severity = strings.TrimSpace(query.Severity)
	query.CountEligible = strings.ToLower(strings.TrimSpace(query.CountEligible))
	return query
}

func normalizeGPTAbuseRepeatBlockQuery(query dto.GPTAbuseRepeatBlockQuery) dto.GPTAbuseRepeatBlockQuery {
	query.StartTimestamp, query.EndTimestamp = normalizeGPTAbuseWindow(query.StartTimestamp, query.EndTimestamp)
	query.Limit, query.Offset = normalizeGPTAbusePagination(query.Limit, query.Offset)
	return query
}

func normalizeGPTAbuseWindow(start int64, end int64) (int64, int64) {
	if start > 0 && end > start {
		if end-start > gptAbuseAdminMaxWindowSeconds {
			end = start + gptAbuseAdminMaxWindowSeconds
		}
		return start, end
	}
	return model.GPTAbuseDayWindow(common.GetTimestamp())
}

func normalizeGPTAbusePagination(limit int, offset int) (int, int) {
	if limit <= 0 {
		limit = gptAbuseAdminDefaultLimit
	}
	if limit > gptAbuseAdminMaxLimit {
		limit = gptAbuseAdminMaxLimit
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

func normalizeGPTAbuseReason(reason string) (string, error) {
	reason = strings.TrimSpace(reason)
	if len([]rune(reason)) > 255 {
		return "", errors.New("reason must be at most 255 characters")
	}
	if reason == "" {
		return gptAbuseReasonDefault, nil
	}
	return reason, nil
}

func gptAbuseSeverityRank(severity string) int {
	switch severity {
	case model.GPTAbuseSeverityHigh:
		return 2
	case model.GPTAbuseSeverityMedium:
		return 1
	default:
		return 0
	}
}

func gptAbuseLogNewer(candidate model.GPTAbuseSignalLog, current model.GPTAbuseSignalLog) bool {
	if current.Id == 0 {
		return true
	}
	if candidate.CreatedAt != current.CreatedAt {
		return candidate.CreatedAt > current.CreatedAt
	}
	return candidate.Id > current.Id
}

func gptAbuseUserMatchesKeyword(item dto.GPTAbuseUserListItem, keyword string) bool {
	keyword = strings.ToLower(strings.TrimSpace(keyword))
	if keyword == "" {
		return true
	}
	return strings.Contains(strings.ToLower(item.Username), keyword) || strings.Contains(strings.ToLower(item.UserEmail), keyword) || strings.Contains(strconv.Itoa(item.UserID), keyword)
}

func gptAbuseUserMatchesStatus(item dto.GPTAbuseUserListItem, status string) bool {
	switch strings.TrimSpace(status) {
	case "active_suspended":
		return item.ActiveSuspension != nil
	case "warning_only":
		return item.ActiveSuspension == nil
	default:
		return true
	}
}

func sortGPTAbuseUserItems(items []dto.GPTAbuseUserListItem, sortBy string, sortOrder string) {
	desc := sortOrder != "asc"
	sort.SliceStable(items, func(i int, j int) bool {
		left := gptAbuseUserSortValue(items[i], sortBy)
		right := gptAbuseUserSortValue(items[j], sortBy)
		if left == right {
			left = int64(items[i].UserID)
			right = int64(items[j].UserID)
		}
		if desc {
			return left > right
		}
		return left < right
	})
}

func gptAbuseUserSortValue(item dto.GPTAbuseUserListItem, sortBy string) int64 {
	switch sortBy {
	case "warning_count":
		return int64(item.WarningCount)
	case "effective_warning_count":
		return int64(item.EffectiveWarningCount)
	case "user_id":
		return int64(item.UserID)
	default:
		return item.LatestWarningAt
	}
}

func pageGPTAbuseUserItems(items []dto.GPTAbuseUserListItem, offset int, limit int) []dto.GPTAbuseUserListItem {
	if offset >= len(items) {
		return []dto.GPTAbuseUserListItem{}
	}
	end := offset + limit
	if end > len(items) {
		end = len(items)
	}
	return items[offset:end]
}

func dbWithContext(ctx context.Context) *gorm.DB {
	if ctx == nil {
		ctx = context.Background()
	}
	return model.DB.WithContext(ctx)
}
