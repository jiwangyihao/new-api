package controller

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupInvitationCommissionControllerDB(t *testing.T) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.SubscriptionPlan{}, &model.SubscriptionOrder{}, &model.UserSubscription{}, &model.InvitationMonthlyEntitlement{}, &model.InvitationRewardEvent{}, &model.InvitationCommissionAccount{}, &model.InvitationCommissionRecord{}, &model.InvitationCommissionLedger{}, &model.InvitationCommissionWithdrawal{}))
	setting := operation_setting.GetInvitationCommissionSetting()
	oldSetting := *setting
	*setting = operation_setting.InvitationCommissionSetting{Enabled: true, RateBps: 1000, MinimumTransferCents: 1, MinimumWithdrawCents: 1000}
	t.Cleanup(func() { *setting = oldSetting })
	require.NoError(t, model.DB.Create(&model.User{Id: 1, Username: "admin", Role: common.RoleAdminUser, Status: common.UserStatusEnabled, AffCode: "admin"}).Error)
}

func seedControllerCommissionAccount(t *testing.T, userId int, available int64, pending int64, withdrawn int64, transferred int64) {
	t.Helper()
	require.NoError(t, model.DB.Create(&model.InvitationCommissionAccount{UserId: userId, AvailableCents: available, PendingCents: pending, WithdrawnCents: withdrawn, TransferredCents: transferred, CreatedAt: common.GetTimestamp(), UpdatedAt: common.GetTimestamp()}).Error)
}

func seedPendingCommissionWithdrawal(t *testing.T, userId int, withdrawalId int, amountCents int64, contactType string, contactValue string) {
	t.Helper()
	require.NoError(t, model.DB.Create(&model.User{Id: userId, Username: fmt.Sprintf("withdraw-user-%d", userId), Status: common.UserStatusEnabled, AffCode: fmt.Sprintf("withdraw-user-%d", userId), InvitationRewardMode: model.InvitationRewardModeCommission}).Error)
	seedControllerCommissionAccount(t, userId, 0, amountCents, 0, 0)
	contactSnapshot, err := common.Marshal(map[string]string{"type": contactType, "value": contactValue})
	require.NoError(t, err)
	now := common.GetTimestamp()
	require.NoError(t, model.DB.Create(&model.InvitationCommissionWithdrawal{Id: withdrawalId, UserId: userId, AmountCents: amountCents, Status: model.InvitationCommissionWithdrawalPending, Method: model.InvitationCommissionWithdrawalMethodManual, ContactSnapshot: string(contactSnapshot), UserRemark: "用户希望私聊确认", CreatedAt: now, UpdatedAt: now}).Error)
}

func performUserRequest(t *testing.T, userId int, method string, target string, body string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("id", userId)
		c.Set("username", fmt.Sprintf("user-%d", userId))
		c.Set("role", common.RoleCommonUser)
		c.Next()
	})
	router.GET("/api/user/invitation-commission/summary", GetInvitationCommissionSummary)
	router.GET("/api/user/invitation-commission/records", GetInvitationCommissionRecords)
	router.POST("/api/user/invitation-commission/transfer", TransferInvitationCommission)
	router.GET("/api/user/invitation-commission/withdrawals", GetInvitationCommissionWithdrawals)
	router.GET("/api/user/self", GetSelf)
	router.POST("/api/user/invitation-commission/withdrawals", CreateInvitationCommissionWithdrawal)
	router.PUT("/api/user/self", UpdateSelf)
	req := httptest.NewRequest(method, target, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, req)
	return recorder
}

func performAdminRequest(t *testing.T, adminId int, method string, target string, body string) *httptest.ResponseRecorder {
	return performAdminRequestWithRole(t, adminId, common.RoleAdminUser, method, target, body)
}

func performAdminRequestWithRole(t *testing.T, adminId int, role int, method string, target string, body string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("id", adminId)
		c.Set("username", fmt.Sprintf("admin-%d", adminId))
		c.Set("role", role)
		c.Next()
	})
	router.GET("/api/admin/invitation-commission/withdrawals", AdminListInvitationCommissionWithdrawals)
	router.POST("/api/admin/invitation-commission/withdrawals/:id/complete", AdminCompleteInvitationCommissionWithdrawal)
	router.POST("/api/admin/invitation-commission/withdrawals/:id/reject", AdminRejectInvitationCommissionWithdrawal)
	router.GET("/api/admin/tasks/summary", GetAdminTasksSummary)
	// Test helper only: mirrors the existing admin user update endpoint mounted as `/api/user/` in api-router adminRoute.
	router.GET("/api/user/", GetAllUsers)
	router.GET("/api/user/search", SearchUsers)
	router.PUT("/api/user/", UpdateUser)
	req := httptest.NewRequest(method, target, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, req)
	return recorder
}

func TestInvitationCommissionSummaryAndTransferPermissions(t *testing.T) {
	setupInvitationCommissionControllerDB(t)
	require.NoError(t, model.DB.Create(&model.User{Id: 9501, Username: "history-user", Status: common.UserStatusEnabled, AffCode: "history-user", InvitationRewardMode: model.InvitationRewardModeSubscription, Quota: 0}).Error)
	seedControllerCommissionAccount(t, 9501, 3000, 0, 0, 0)
	setting := operation_setting.GetInvitationCommissionSetting()
	oldSetting := *setting
	*setting = operation_setting.InvitationCommissionSetting{Enabled: false, RateBps: 1000, MinimumTransferCents: 1000, MinimumWithdrawCents: 1000}
	t.Cleanup(func() { *setting = oldSetting })

	summary := performUserRequest(t, 9501, http.MethodGet, "/api/user/invitation-commission/summary", "")
	require.Equal(t, http.StatusOK, summary.Code)
	assert.Contains(t, summary.Body.String(), `"reward_mode":"subscription"`)
	assert.Contains(t, summary.Body.String(), `"has_commission_account":true`)
	assert.Contains(t, summary.Body.String(), `"can_transfer":true`)
	assert.Contains(t, summary.Body.String(), `"can_request_withdrawal":true`)
	assert.Contains(t, summary.Body.String(), `"enabled":false`)

	transfer := performUserRequest(t, 9501, http.MethodPost, "/api/user/invitation-commission/transfer", `{"amount_cents":1000}`)
	require.Equal(t, http.StatusOK, transfer.Code)
	assert.Contains(t, transfer.Body.String(), `"available_cents":2000`)
}

func TestHistoricalCommissionAccountCanRequestManualCashbackInSubscriptionMode(t *testing.T) {
	setupInvitationCommissionControllerDB(t)
	require.NoError(t, model.DB.Create(&model.User{Id: 9502, Username: "history-withdraw", Status: common.UserStatusEnabled, AffCode: "history-withdraw", InvitationRewardMode: model.InvitationRewardModeSubscription}).Error)
	seedControllerCommissionAccount(t, 9502, 3000, 0, 0, 0)

	withdrawal := performUserRequest(t, 9502, http.MethodPost, "/api/user/invitation-commission/withdrawals", `{"amount_cents":1000,"contact":{"type":"wechat","value":"history-contact"},"remark":"历史余额返现"}`)

	require.Equal(t, http.StatusOK, withdrawal.Code)
	assert.Contains(t, withdrawal.Body.String(), `"status":"pending"`)
	assert.Contains(t, withdrawal.Body.String(), `"user_remark":"历史余额返现"`)
	assert.NotContains(t, withdrawal.Body.String(), `"remark":"历史余额返现"`)
	var account model.InvitationCommissionAccount
	require.NoError(t, model.DB.Where("user_id = ?", 9502).First(&account).Error)
	assert.Equal(t, int64(2000), account.AvailableCents)
	assert.Equal(t, int64(1000), account.PendingCents)
}

func TestInvitationCommissionRecordsExposeStableFieldContract(t *testing.T) {
	setupInvitationCommissionControllerDB(t)
	require.NoError(t, model.DB.Create(&model.User{Id: 9503, Username: "record-inviter", Status: common.UserStatusEnabled, AffCode: "record-inviter", InvitationRewardMode: model.InvitationRewardModeCommission}).Error)
	require.NoError(t, model.DB.Create(&model.User{Id: 9504, Username: "record-invitee", Status: common.UserStatusEnabled, AffCode: "record-invitee", InviterId: 9503}).Error)
	now := common.GetTimestamp()
	order := model.SubscriptionOrder{Id: 9505, UserId: 9504, TradeNo: "record-contract-order", AmountCents: 4000, Currency: "CNY", Status: common.TopUpStatusSuccess, CreateTime: now}
	require.NoError(t, model.DB.Create(&order).Error)
	event := model.InvitationRewardEvent{Id: 9506, InviterId: 9503, InviteeId: 9504, SourceType: model.InvitationRewardEventSourceSubscriptionOrder, SourceId: order.Id, SourceOrderId: order.Id, SourceAmountCents: 4000, SourceCurrency: "CNY", Status: model.InvitationRewardEventStatusActive, CreatedAt: now}
	require.NoError(t, model.DB.Create(&event).Error)
	record := model.InvitationCommissionRecord{EventId: event.Id, InviterId: 9503, InviteeId: 9504, SourceType: model.InvitationCommissionSourceSubscriptionOrder, SourceId: order.Id, SourceTradeNo: order.TradeNo, SourceAmountCents: 4000, SourceCurrency: "CNY", CommissionRateBps: 1000, CommissionCents: 400, Status: model.InvitationCommissionStatusAvailable, CreatedAt: now, AvailableAt: now}
	require.NoError(t, model.DB.Create(&record).Error)

	response := performUserRequest(t, 9503, http.MethodGet, "/api/user/invitation-commission/records?page=1&page_size=20", "")

	require.Equal(t, http.StatusOK, response.Code)
	assert.Contains(t, response.Body.String(), `"invitee_id":9504`)
	assert.Contains(t, response.Body.String(), `"source_trade_no":"record-contract-order"`)
	assert.Contains(t, response.Body.String(), `"status":"available"`)
	assert.Contains(t, response.Body.String(), `"available_at":`)
	assert.Contains(t, response.Body.String(), `"cancelled_at":0`)
}

func TestInvitationCommissionWithdrawalCreateReturnsPublicResponse(t *testing.T) {
	setupInvitationCommissionControllerDB(t)
	require.NoError(t, model.DB.Create(&model.User{Id: 9507, Username: "public-withdraw", Status: common.UserStatusEnabled, AffCode: "public-withdraw", InvitationRewardMode: model.InvitationRewardModeCommission}).Error)
	seedControllerCommissionAccount(t, 9507, 3000, 0, 0, 0)

	withdrawal := performUserRequest(t, 9507, http.MethodPost, "/api/user/invitation-commission/withdrawals", `{"amount_cents":1000,"contact":{"type":"wechat","value":"public-contact"},"remark":"公开响应"}`)

	require.Equal(t, http.StatusOK, withdrawal.Code)
	assert.Contains(t, withdrawal.Body.String(), `"contact":{"type":"wechat","value":"public-contact"}`)
	assert.Contains(t, withdrawal.Body.String(), `"user_remark":"公开响应"`)
	assert.NotContains(t, withdrawal.Body.String(), "contact_snapshot")
	assert.NotContains(t, withdrawal.Body.String(), `"remark":"公开响应"`)
}

func TestInvitationCommissionWithdrawalRejectsInvalidContactWithoutSideEffects(t *testing.T) {
	setupInvitationCommissionControllerDB(t)
	require.NoError(t, model.DB.Create(&model.User{Id: 9508, Username: "invalid-contact", Status: common.UserStatusEnabled, AffCode: "invalid-contact", InvitationRewardMode: model.InvitationRewardModeCommission}).Error)
	seedControllerCommissionAccount(t, 9508, 3000, 0, 0, 0)

	cases := []struct {
		name string
		body string
	}{
		{name: "invalid type", body: `{"amount_cents":1000,"contact":{"type":"qq","value":"contact"}}`},
		{name: "blank value", body: `{"amount_cents":1000,"contact":{"type":"wechat","value":"   "}}`},
		{name: "too long value", body: `{"amount_cents":1000,"contact":{"type":"email","value":"` + strings.Repeat("a", 129) + `"}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			response := performUserRequest(t, 9508, http.MethodPost, "/api/user/invitation-commission/withdrawals", tc.body)

			require.Equal(t, http.StatusOK, response.Code)
			assert.Contains(t, response.Body.String(), `"success":false`)
			var account model.InvitationCommissionAccount
			require.NoError(t, model.DB.Where("user_id = ?", 9508).First(&account).Error)
			assert.Equal(t, int64(3000), account.AvailableCents)
			assert.Equal(t, int64(0), account.PendingCents)
			var withdrawals int64
			require.NoError(t, model.DB.Model(&model.InvitationCommissionWithdrawal{}).Where("user_id = ?", 9508).Count(&withdrawals).Error)
			assert.Equal(t, int64(0), withdrawals)
		})
	}
}

func TestInvitationCommissionListEndpointsPassPagination(t *testing.T) {
	setupInvitationCommissionControllerDB(t)
	require.NoError(t, model.DB.Create(&model.User{Id: 9509, Username: "paging-user", Status: common.UserStatusEnabled, AffCode: "paging-user", InvitationRewardMode: model.InvitationRewardModeCommission}).Error)
	now := common.GetTimestamp()
	require.NoError(t, model.DB.Create(&model.InvitationCommissionRecord{Id: 9510, EventId: 9511, InviterId: 9509, InviteeId: 9512, SourceType: model.InvitationCommissionSourceSubscriptionOrder, SourceId: 9513, SourceTradeNo: "page-first", SourceAmountCents: 1000, SourceCurrency: "CNY", CommissionRateBps: 1000, CommissionCents: 100, Status: model.InvitationCommissionStatusAvailable, CreatedAt: now, AvailableAt: now}).Error)
	require.NoError(t, model.DB.Create(&model.InvitationCommissionRecord{Id: 9514, EventId: 9515, InviterId: 9509, InviteeId: 9516, SourceType: model.InvitationCommissionSourceSubscriptionOrder, SourceId: 9517, SourceTradeNo: "page-second", SourceAmountCents: 2000, SourceCurrency: "CNY", CommissionRateBps: 1000, CommissionCents: 200, Status: model.InvitationCommissionStatusAvailable, CreatedAt: now + 1, AvailableAt: now + 1}).Error)
	contactSnapshot, err := common.Marshal(map[string]string{"type": "wechat", "value": "paging"})
	require.NoError(t, err)
	require.NoError(t, model.DB.Create(&model.InvitationCommissionWithdrawal{Id: 9518, UserId: 9509, AmountCents: 1000, Status: model.InvitationCommissionWithdrawalPending, Method: model.InvitationCommissionWithdrawalMethodManual, ContactSnapshot: string(contactSnapshot), UserRemark: "first", CreatedAt: now, UpdatedAt: now}).Error)
	require.NoError(t, model.DB.Create(&model.InvitationCommissionWithdrawal{Id: 9519, UserId: 9509, AmountCents: 2000, Status: model.InvitationCommissionWithdrawalPending, Method: model.InvitationCommissionWithdrawalMethodManual, ContactSnapshot: string(contactSnapshot), UserRemark: "second", CreatedAt: now + 1, UpdatedAt: now + 1}).Error)

	records := performUserRequest(t, 9509, http.MethodGet, "/api/user/invitation-commission/records?page=2&page_size=1", "")
	require.Equal(t, http.StatusOK, records.Code)
	assert.Contains(t, records.Body.String(), `"page":2`)
	assert.Contains(t, records.Body.String(), `"page_size":1`)
	assert.Contains(t, records.Body.String(), `"total":2`)
	assert.Contains(t, records.Body.String(), `"source_trade_no":"page-first"`)
	assert.NotContains(t, records.Body.String(), `"source_trade_no":"page-second"`)

	withdrawals := performUserRequest(t, 9509, http.MethodGet, "/api/user/invitation-commission/withdrawals?page=2&page_size=1", "")
	require.Equal(t, http.StatusOK, withdrawals.Code)
	assert.Contains(t, withdrawals.Body.String(), `"page":2`)
	assert.Contains(t, withdrawals.Body.String(), `"page_size":1`)
	assert.Contains(t, withdrawals.Body.String(), `"total":2`)
	assert.Contains(t, withdrawals.Body.String(), `"user_remark":"first"`)
	assert.Contains(t, withdrawals.Body.String(), `"contact":{"type":"wechat","value":"paging"}`)
	assert.NotContains(t, withdrawals.Body.String(), `"user_remark":"second"`)

	adminWithdrawals := performAdminRequest(t, 1, http.MethodGet, "/api/admin/invitation-commission/withdrawals?page=2&page_size=1", "")
	require.Equal(t, http.StatusOK, adminWithdrawals.Code)
	assert.Contains(t, adminWithdrawals.Body.String(), `"page":2`)
	assert.Contains(t, adminWithdrawals.Body.String(), `"page_size":1`)
	assert.Contains(t, adminWithdrawals.Body.String(), `"total":2`)
	assert.Contains(t, adminWithdrawals.Body.String(), `"user_remark":"first"`)
	assert.NotContains(t, adminWithdrawals.Body.String(), `"user_remark":"second"`)
}

func TestInvitationRewardModeResponseContractsNormalizeMode(t *testing.T) {
	setupInvitationCommissionControllerDB(t)
	require.NoError(t, model.DB.Create(&model.User{Id: 9512, Username: "mode-response", DisplayName: "Mode Response", Status: common.UserStatusEnabled, AffCode: "mode-response", InvitationRewardMode: ""}).Error)

	self := performUserRequest(t, 9512, http.MethodGet, "/api/user/self", "")
	require.Equal(t, http.StatusOK, self.Code)
	assert.Contains(t, self.Body.String(), `"invitation_reward_mode":"subscription"`)

	list := performAdminRequest(t, 1, http.MethodGet, "/api/user/?p=0&page=1&page_size=20", "")
	require.Equal(t, http.StatusOK, list.Code)
	assert.Contains(t, list.Body.String(), `"username":"mode-response"`)
	assert.Contains(t, list.Body.String(), `"invitation_reward_mode":"subscription"`)

	search := performAdminRequest(t, 1, http.MethodGet, "/api/user/search?keyword=mode-response", "")
	require.Equal(t, http.StatusOK, search.Code)
	assert.Contains(t, search.Body.String(), `"username":"mode-response"`)
	assert.Contains(t, search.Body.String(), `"invitation_reward_mode":"subscription"`)
}

func TestInvitationCommissionWriteRejectsNonCommissionWithoutHistory(t *testing.T) {
	setupInvitationCommissionControllerDB(t)
	require.NoError(t, model.DB.Create(&model.User{Id: 9511, Username: "plain-user", Status: common.UserStatusEnabled, AffCode: "plain-user"}).Error)

	transfer := performUserRequest(t, 9511, http.MethodPost, "/api/user/invitation-commission/transfer", `{"amount_cents":100}`)
	assert.Contains(t, transfer.Body.String(), "返佣")

	withdrawal := performUserRequest(t, 9511, http.MethodPost, "/api/user/invitation-commission/withdrawals", `{"amount_cents":1000,"contact":{"type":"wechat","value":"u"}}`)
	assert.Contains(t, withdrawal.Body.String(), "返佣")
}
