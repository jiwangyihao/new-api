package model

import (
	"sort"
	"strconv"

	"github.com/QuantumNous/new-api/dto"
)

type AdminAnalyticsDrilldownFilter struct {
	UserID     int
	PlanID     int
	InviterID  int
	UserGroup  string
	UserStatus string
	Status     string
}

func GetAdminAnalyticsDrilldownUsers(query AdminAnalyticsQuery, filter AdminAnalyticsDrilldownFilter) (dto.AdminAnalyticsPanelResponse[dto.AdminAnalyticsDrilldownUsersResponse], error) {
	query = normalizeAdminAnalyticsQuery(query)
	var users []User
	db := DB.Model(&User{})
	if filter.UserID > 0 {
		db = db.Where("id = ?", filter.UserID)
	}
	if filter.InviterID > 0 {
		db = db.Where("inviter_id = ?", filter.InviterID)
	}
	if filter.UserGroup != "" {
		db = db.Where(commonGroupCol+" = ?", filter.UserGroup)
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
		item := dto.AdminAnalyticsDrilldownUserItem{UserID: user.Id, Username: user.Username, DisplayName: user.DisplayName, Email: user.Email, UserGroup: user.Group, Status: user.Status, Role: user.Role, CreatedAt: user.CreatedAt, LastLoginAt: user.LastLoginAt, InviterID: user.InviterId}
		if active, ok := activeByUser[user.Id]; ok {
			item.ActivePlanID = active.Subscription.PlanId
			item.ActivePlanTitle = active.Plan.Title
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].UserID < items[j].UserID })
	paged, page := paginateAdminAnalyticsList(items, query.Limit, query.Offset)
	data := dto.AdminAnalyticsDrilldownUsersResponse{Users: dto.AdminAnalyticsList[dto.AdminAnalyticsDrilldownUserItem]{Items: paged, Page: page, SortBy: query.SortBy, SortOrder: query.SortOrder}}
	return dto.AdminAnalyticsPanelResponse[dto.AdminAnalyticsDrilldownUsersResponse]{Range: adminAnalyticsRangeMeta(query), Data: data}, nil
}

func GetAdminAnalyticsDrilldownSubscriptions(query AdminAnalyticsQuery, filter AdminAnalyticsDrilldownFilter) (dto.AdminAnalyticsPanelResponse[dto.AdminAnalyticsDrilldownSubscriptionsResponse], error) {
	query = normalizeAdminAnalyticsQuery(query)
	rows, err := loadAdminActiveSubscriptions(query)
	if err != nil {
		return dto.AdminAnalyticsPanelResponse[dto.AdminAnalyticsDrilldownSubscriptionsResponse]{}, err
	}
	items := make([]dto.AdminAnalyticsDrilldownSubscriptionItem, 0, len(rows))
	for i := range rows {
		row := rows[i]
		if filter.UserID > 0 && row.Subscription.UserId != filter.UserID {
			continue
		}
		if filter.PlanID > 0 && row.Subscription.PlanId != filter.PlanID {
			continue
		}
		remaining := int64(0)
		if row.Quota.RemainingTokens != nil {
			remaining = *row.Quota.RemainingTokens
		}
		items = append(items, dto.AdminAnalyticsDrilldownSubscriptionItem{SubscriptionID: row.Subscription.Id, UserID: row.Subscription.UserId, Username: row.User.Username, PlanID: row.Subscription.PlanId, PlanTitle: row.Plan.Title, Source: row.Source, Status: row.Subscription.Status, StartTime: row.Subscription.StartTime, EndTime: row.Subscription.EndTime, TokenLimit: row.Subscription.TokenLimit, TokenUsed: row.Subscription.TokenUsed, RemainingTokens: remaining, UsageRate: row.Quota.UsageRate})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].SubscriptionID < items[j].SubscriptionID })
	paged, page := paginateAdminAnalyticsList(items, query.Limit, query.Offset)
	data := dto.AdminAnalyticsDrilldownSubscriptionsResponse{Subscriptions: dto.AdminAnalyticsList[dto.AdminAnalyticsDrilldownSubscriptionItem]{Items: paged, Page: page, SortBy: query.SortBy, SortOrder: query.SortOrder}}
	return dto.AdminAnalyticsPanelResponse[dto.AdminAnalyticsDrilldownSubscriptionsResponse]{Range: adminAnalyticsRangeMeta(query), Data: data}, nil
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
	sort.Slice(items, func(i, j int) bool { return items[i].InviteeID < items[j].InviteeID })
	paged, page := paginateAdminAnalyticsList(items, query.Limit, query.Offset)
	data := dto.AdminAnalyticsDrilldownInvitationsResponse{Invitations: dto.AdminAnalyticsList[dto.AdminAnalyticsDrilldownInvitationItem]{Items: paged, Page: page, SortBy: query.SortBy, SortOrder: query.SortOrder}}
	return dto.AdminAnalyticsPanelResponse[dto.AdminAnalyticsDrilldownInvitationsResponse]{Range: adminAnalyticsRangeMeta(query), Data: data}, nil
}
