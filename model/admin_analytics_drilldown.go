package model

import (
	"sort"
	"strconv"

	"github.com/QuantumNous/new-api/dto"
)

type AdminAnalyticsDrilldownFilter struct {
	UserIDs    []int
	PlanID     int
	InviterID  int
	UserStatus string
	Status     string
}

func GetAdminAnalyticsDrilldownUsers(query AdminAnalyticsQuery, filter AdminAnalyticsDrilldownFilter) (dto.AdminAnalyticsPanelResponse[dto.AdminAnalyticsDrilldownUsersResponse], error) {
	query = normalizeAdminAnalyticsQuery(query)
	var users []User
	db := DB.Model(&User{})
	if len(filter.UserIDs) > 0 {
		db = db.Where("id IN ?", filter.UserIDs)
	}
	if filter.InviterID > 0 {
		db = db.Where("inviter_id = ?", filter.InviterID)
	}
	if filter.UserStatus != "" {
		status, err := strconv.Atoi(filter.UserStatus)
		if err == nil {
			db = db.Where("status = ?", status)
		}
	}
	if err := db.Find(&users).Error; err != nil {
		return dto.AdminAnalyticsPanelResponse[dto.AdminAnalyticsDrilldownUsersResponse]{}, err
	}
	activeRows, err := loadAdminActiveSubscriptions(query)
	if err != nil {
		return dto.AdminAnalyticsPanelResponse[dto.AdminAnalyticsDrilldownUsersResponse]{}, err
	}
	activeByUser := map[int]adminActiveSubscriptionRow{}
	for i := range activeRows {
		if _, ok := activeByUser[activeRows[i].Subscription.UserId]; !ok {
			activeByUser[activeRows[i].Subscription.UserId] = activeRows[i]
		}
	}
	items := make([]dto.AdminAnalyticsDrilldownUserItem, 0, len(users))
	for i := range users {
		user := users[i]
		if filter.PlanID > 0 {
			active, ok := activeByUser[user.Id]
			if !ok || active.Subscription.PlanId != filter.PlanID {
				continue
			}
		}
		item := dto.AdminAnalyticsDrilldownUserItem{UserID: user.Id, Username: user.Username, DisplayName: user.DisplayName, Email: user.Email, Status: user.Status, Role: user.Role, CreatedAt: user.CreatedAt, LastLoginAt: user.LastLoginAt, InviterID: user.InviterId}
		if active, ok := activeByUser[user.Id]; ok {
			item.ActivePlanID = active.Subscription.PlanId
			item.ActivePlanTitle = active.Plan.Title
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool { return adminDrilldownUserLess(items[i], items[j], query.SortBy, query.SortOrder) })
	paged, page := paginateAdminAnalyticsList(items, query.Limit, query.Offset)
	data := dto.AdminAnalyticsDrilldownUsersResponse{Users: dto.AdminAnalyticsList[dto.AdminAnalyticsDrilldownUserItem]{Items: paged, Page: page, SortBy: query.SortBy, SortOrder: query.SortOrder}}
	return dto.AdminAnalyticsPanelResponse[dto.AdminAnalyticsDrilldownUsersResponse]{Range: adminAnalyticsRangeMeta(query), Data: data}, nil
}

func GetAdminAnalyticsDrilldownSubscriptions(query AdminAnalyticsQuery, filter AdminAnalyticsDrilldownFilter) (dto.AdminAnalyticsPanelResponse[dto.AdminAnalyticsDrilldownSubscriptionsResponse], error) {
	query = normalizeAdminAnalyticsQuery(query)
	rows, err := loadAdminAnalyticsDrilldownSubscriptionRows(query)
	if err != nil {
		return dto.AdminAnalyticsPanelResponse[dto.AdminAnalyticsDrilldownSubscriptionsResponse]{}, err
	}
	items := make([]dto.AdminAnalyticsDrilldownSubscriptionItem, 0, len(rows))
	for i := range rows {
		row := rows[i]
		if len(filter.UserIDs) > 0 && !adminIntInSet(row.Subscription.UserId, filter.UserIDs) {
			continue
		}
		if filter.PlanID > 0 && row.Subscription.PlanId != filter.PlanID {
			continue
		}
		if filter.Status != "" && row.Subscription.Status != filter.Status {
			continue
		}
		items = append(items, buildAdminAnalyticsDrilldownSubscriptionItem(row, query.SnapshotAt))
	}
	if err := enrichAdminAnalyticsConversionTargets(items); err != nil {
		return dto.AdminAnalyticsPanelResponse[dto.AdminAnalyticsDrilldownSubscriptionsResponse]{}, err
	}
	sort.Slice(items, func(i, j int) bool {
		return adminDrilldownSubscriptionLess(items[i], items[j], query.SortBy, query.SortOrder)
	})
	paged, page := paginateAdminAnalyticsList(items, query.Limit, query.Offset)
	data := dto.AdminAnalyticsDrilldownSubscriptionsResponse{Subscriptions: dto.AdminAnalyticsList[dto.AdminAnalyticsDrilldownSubscriptionItem]{Items: paged, Page: page, SortBy: query.SortBy, SortOrder: query.SortOrder}}
	return dto.AdminAnalyticsPanelResponse[dto.AdminAnalyticsDrilldownSubscriptionsResponse]{Range: adminAnalyticsRangeMeta(query), Data: data}, nil
}

func loadAdminAnalyticsDrilldownSubscriptionRows(query AdminAnalyticsQuery) ([]adminActiveSubscriptionRow, error) {
	db := DB.Model(&UserSubscription{}).
		Where("start_time <= ?", query.SnapshotAt).
		Where("(status = ? AND ((entitlement_type = ?) OR (entitlement_type = ? AND end_time > ?))) OR (entitlement_type = ? AND status = ?) OR (entitlement_type = ? AND status = ? AND end_time >= ? AND end_time <= ?)",
			SubscriptionStatusActive, SubscriptionEntitlementCreditBalance, SubscriptionEntitlementTimed, query.SnapshotAt,
			SubscriptionEntitlementTimed, SubscriptionStatusConverted,
			SubscriptionEntitlementTimed, SubscriptionStatusExpired, query.SnapshotAt-TimedSubscriptionConversionGraceSeconds, query.SnapshotAt)
	if statuses := adminActiveSubscriptionStatuses(query); len(statuses) > 0 {
		db = db.Where("status IN ?", statuses)
	}
	return loadAdminSubscriptionRows(query, db)
}

func buildAdminAnalyticsDrilldownSubscriptionItem(row adminActiveSubscriptionRow, snapshotAt int64) dto.AdminAnalyticsDrilldownSubscriptionItem {
	remaining := int64(0)
	if row.Quota.RemainingTokens != nil {
		remaining = *row.Quota.RemainingTokens
	}
	item := dto.AdminAnalyticsDrilldownSubscriptionItem{
		SubscriptionID: row.Subscription.Id, UserID: row.Subscription.UserId, Username: row.User.Username,
		PlanID: row.Subscription.PlanId, PlanTitle: row.Plan.Title, Source: row.Source,
		Status: row.Subscription.Status, StartTime: row.Subscription.StartTime, EndTime: row.Subscription.EndTime,
		TokenLimit: row.Subscription.TokenLimit, TokenUsed: row.Subscription.TokenUsed, RemainingTokens: remaining,
		UsageRate: row.Quota.UsageRate, EntitlementType: row.Subscription.EntitlementType,
	}
	classification := classifyAdminCreditLifecycle(row.Subscription.TokenLimit, row.Subscription.TokenUsed)
	switch {
	case row.Subscription.EntitlementType == SubscriptionEntitlementCreditBalance:
		item.LifecycleState = classification.State
		item.AvailableCredit = classification.AvailableCredit
		item.SettlementDebt = classification.SettlementDebt
	case row.Subscription.Status == SubscriptionStatusConverted:
		item.LifecycleState = SubscriptionStatusConverted
		item.ConversionID = row.Subscription.ConversionId
		item.TargetSubscriptionID = row.Subscription.ConvertedToSubscriptionId
	case row.Subscription.Status == SubscriptionStatusExpired:
		item.LifecycleState = ConversionQuoteCategoryGrace
		item.GraceRemainingSeconds = row.Subscription.EndTime + TimedSubscriptionConversionGraceSeconds - snapshotAt
	default:
		item.LifecycleState = "active_timed"
	}
	return item
}

func enrichAdminAnalyticsConversionTargets(items []dto.AdminAnalyticsDrilldownSubscriptionItem) error {
	conversionIDs := make([]int, 0)
	expectedTargetBySource := make(map[int]int, len(items))
	for i := range items {
		if items[i].ConversionID > 0 {
			conversionIDs = append(conversionIDs, items[i].ConversionID)
		}
		expectedTargetBySource[items[i].SubscriptionID] = items[i].TargetSubscriptionID
		items[i].TargetSubscriptionID = 0
		items[i].TargetUserID = 0
		items[i].TargetPlanID = 0
		items[i].TargetPlanTitle = ""
	}
	var conversions []SubscriptionConversion
	if len(conversionIDs) > 0 {
		if err := DB.Where("id IN ?", adminUniquePositiveInts(conversionIDs)).Find(&conversions).Error; err != nil {
			return err
		}
	}
	conversionByID := make(map[int]SubscriptionConversion, len(conversions))
	for i := range conversions {
		conversionByID[conversions[i].Id] = conversions[i]
	}
	validConversionBySource := make(map[int]SubscriptionConversion, len(conversions))
	targetIDs := make([]int, 0, len(conversions))
	for i := range items {
		conversion, ok := conversionByID[items[i].ConversionID]
		expectedTargetID := expectedTargetBySource[items[i].SubscriptionID]
		if !ok || conversion.SourceSubscriptionId != items[i].SubscriptionID || conversion.SourcePlanId != items[i].PlanID || conversion.UserId != items[i].UserID || expectedTargetID <= 0 || conversion.TargetSubscriptionId != expectedTargetID {
			continue
		}
		validConversionBySource[items[i].SubscriptionID] = conversion
		targetIDs = append(targetIDs, conversion.TargetSubscriptionId)
	}
	var targets []UserSubscription
	if len(targetIDs) > 0 {
		if err := DB.Where("id IN ?", adminUniquePositiveInts(targetIDs)).Find(&targets).Error; err != nil {
			return err
		}
	}
	targetByID := make(map[int]UserSubscription, len(targets))
	planIDs := make([]int, 0, len(targets))
	for i := range targets {
		targetByID[targets[i].Id] = targets[i]
		planIDs = append(planIDs, targets[i].PlanId)
	}
	plans, err := adminPlansByID(planIDs)
	if err != nil {
		return err
	}
	for i := range items {
		conversion, ok := validConversionBySource[items[i].SubscriptionID]
		if !ok {
			continue
		}
		target, ok := targetByID[conversion.TargetSubscriptionId]
		if !ok || target.UserId != items[i].UserID || target.PlanId != conversion.TargetPlanId || target.EntitlementType != SubscriptionEntitlementCreditBalance {
			continue
		}
		plan, ok := plans[target.PlanId]
		if !ok || plan.EntitlementType != SubscriptionEntitlementCreditBalance {
			continue
		}
		items[i].TargetSubscriptionID = target.Id
		items[i].TargetUserID = target.UserId
		items[i].TargetPlanID = target.PlanId
		items[i].TargetPlanTitle = plan.Title
	}
	return nil
}

func GetAdminAnalyticsDrilldownInvitations(query AdminAnalyticsQuery, filter AdminAnalyticsDrilldownFilter) (dto.AdminAnalyticsPanelResponse[dto.AdminAnalyticsDrilldownInvitationsResponse], error) {
	query = normalizeAdminAnalyticsQuery(query)
	var invitees []User
	db := DB.Where("inviter_id > ?", 0)
	if filter.InviterID > 0 {
		db = db.Where("inviter_id = ?", filter.InviterID)
	}
	if err := db.Find(&invitees).Error; err != nil {
		return dto.AdminAnalyticsPanelResponse[dto.AdminAnalyticsDrilldownInvitationsResponse]{}, err
	}
	inviterIDs := make([]int, 0, len(invitees))
	for i := range invitees {
		inviterIDs = append(inviterIDs, invitees[i].InviterId)
	}
	inviters, err := adminUsersByID(inviterIDs)
	if err != nil {
		return dto.AdminAnalyticsPanelResponse[dto.AdminAnalyticsDrilldownInvitationsResponse]{}, err
	}
	items := make([]dto.AdminAnalyticsDrilldownInvitationItem, 0, len(invitees))
	for i := range invitees {
		invitee := invitees[i]
		items = append(items, dto.AdminAnalyticsDrilldownInvitationItem{InviterID: invitee.InviterId, InviterUsername: inviters[invitee.InviterId].Username, InviteeID: invitee.Id, InviteeUsername: invitee.Username, InviteeStatus: invitee.Status, CreatedAt: invitee.CreatedAt})
	}
	sort.Slice(items, func(i, j int) bool {
		return adminDrilldownInvitationLess(items[i], items[j], query.SortBy, query.SortOrder)
	})
	paged, page := paginateAdminAnalyticsList(items, query.Limit, query.Offset)
	data := dto.AdminAnalyticsDrilldownInvitationsResponse{Invitations: dto.AdminAnalyticsList[dto.AdminAnalyticsDrilldownInvitationItem]{Items: paged, Page: page, SortBy: query.SortBy, SortOrder: query.SortOrder}}
	return dto.AdminAnalyticsPanelResponse[dto.AdminAnalyticsDrilldownInvitationsResponse]{Range: adminAnalyticsRangeMeta(query), Data: data}, nil
}

func adminSortDesc(order dto.AdminAnalyticsSortOrder) bool {
	return order == dto.AdminAnalyticsSortDesc
}

func adminDrilldownUserLess(left dto.AdminAnalyticsDrilldownUserItem, right dto.AdminAnalyticsDrilldownUserItem, sortBy string, order dto.AdminAnalyticsSortOrder) bool {
	var less bool
	switch sortBy {
	case "username":
		less = left.Username < right.Username
	case "status":
		less = left.Status < right.Status
	default:
		less = left.UserID < right.UserID
	}
	if adminSortDesc(order) {
		return !less
	}
	return less
}

func adminDrilldownSubscriptionLess(left dto.AdminAnalyticsDrilldownSubscriptionItem, right dto.AdminAnalyticsDrilldownSubscriptionItem, sortBy string, order dto.AdminAnalyticsSortOrder) bool {
	var less bool
	switch sortBy {
	case "user_id":
		less = left.UserID < right.UserID
	case "plan_id":
		less = left.PlanID < right.PlanID
	case "status":
		less = left.Status < right.Status
	case "usage_rate":
		less = adminUsageRateValue(left.UsageRate) < adminUsageRateValue(right.UsageRate)
	case "token_used":
		less = left.TokenUsed < right.TokenUsed
	case "end_time":
		less = left.EndTime < right.EndTime
	default:
		less = left.SubscriptionID < right.SubscriptionID
	}
	if adminSortDesc(order) {
		return !less
	}
	return less
}

func adminDrilldownInvitationLess(left dto.AdminAnalyticsDrilldownInvitationItem, right dto.AdminAnalyticsDrilldownInvitationItem, sortBy string, order dto.AdminAnalyticsSortOrder) bool {
	var less bool
	switch sortBy {
	case "inviter_id":
		less = left.InviterID < right.InviterID
	default:
		less = left.InviteeID < right.InviteeID
	}
	if adminSortDesc(order) {
		return !less
	}
	return less
}

func adminUsageRateValue(value *float64) float64 {
	if value == nil {
		return -1
	}
	return *value
}
