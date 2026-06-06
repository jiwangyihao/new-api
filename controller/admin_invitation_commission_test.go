package controller

import (
	"net/http"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdminInvitationCommissionWithdrawalListCompleteRejectAndTaskSummary(t *testing.T) {
	setupInvitationCommissionControllerDB(t)
	seedPendingCommissionWithdrawal(t, 9521, 9522, 5000, "wechat", "contact")

	list := performAdminRequest(t, 1, http.MethodGet, "/api/admin/invitation-commission/withdrawals?status=pending&page=1&page_size=20", "")
	require.Equal(t, http.StatusOK, list.Code)
	assert.Contains(t, list.Body.String(), `"status":"pending"`)
	assert.Contains(t, list.Body.String(), `"contact":{"type":"wechat","value":"contact"}`)
	assert.Contains(t, list.Body.String(), `"user_remark":"用户希望私聊确认"`)
	assert.NotContains(t, list.Body.String(), `"remark":"用户希望私聊确认"`)

	summary := performAdminRequest(t, 1, http.MethodGet, "/api/admin/tasks/summary", "")
	require.Equal(t, http.StatusOK, summary.Code)
	assert.Contains(t, summary.Body.String(), `"pending_commission_withdrawals":1`)

	complete := performAdminRequest(t, 1, http.MethodPost, "/api/admin/invitation-commission/withdrawals/9522/complete", `{"admin_remark":"已线下返现"}`)
	require.Equal(t, http.StatusOK, complete.Code)
	assert.Contains(t, complete.Body.String(), `"status":"completed"`)

	retry := performAdminRequest(t, 1, http.MethodPost, "/api/admin/invitation-commission/withdrawals/9522/reject", `{"admin_remark":"重复"}`)
	assert.Contains(t, retry.Body.String(), "pending")

	summaryAfter := performAdminRequest(t, 1, http.MethodGet, "/api/admin/tasks/summary", "")
	assert.Contains(t, summaryAfter.Body.String(), `"pending_commission_withdrawals":0`)
}

func TestAdminInvitationCommissionWithdrawalRejectsInvalidRemarkWithoutSideEffects(t *testing.T) {
	setupInvitationCommissionControllerDB(t)
	seedPendingCommissionWithdrawal(t, 9523, 9524, 5000, "wechat", "contact")
	seedPendingCommissionWithdrawal(t, 9525, 9526, 4000, "email", "contact@example.com")

	for _, tc := range []struct {
		name   string
		path   string
		body   string
		userId int
	}{
		{name: "complete blank", path: "/api/admin/invitation-commission/withdrawals/9524/complete", body: `{"admin_remark":"   "}`, userId: 9523},
		{name: "reject too long", path: "/api/admin/invitation-commission/withdrawals/9526/reject", body: `{"admin_remark":"` + strings.Repeat("a", 501) + `"}`, userId: 9525},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response := performAdminRequest(t, 1, http.MethodPost, tc.path, tc.body)

			require.Equal(t, http.StatusOK, response.Code)
			assert.Contains(t, response.Body.String(), `"success":false`)
			var withdrawal model.InvitationCommissionWithdrawal
			require.NoError(t, model.DB.Where("user_id = ?", tc.userId).First(&withdrawal).Error)
			assert.Equal(t, model.InvitationCommissionWithdrawalPending, withdrawal.Status)
			var account model.InvitationCommissionAccount
			require.NoError(t, model.DB.Where("user_id = ?", tc.userId).First(&account).Error)
			assert.Equal(t, withdrawal.AmountCents, account.PendingCents)
			assert.Equal(t, int64(0), account.AvailableCents)
			assert.Equal(t, int64(0), account.WithdrawnCents)
		})
	}
}

func TestInvitationRewardModeCanOnlyBeUpdatedByAdminUserEndpoint(t *testing.T) {
	setupInvitationCommissionControllerDB(t)
	require.NoError(t, model.DB.Create(&model.User{Id: 9531, Username: "mode-target", Status: common.UserStatusEnabled, AffCode: "mode-target"}).Error)

	adminUpdate := performAdminRequest(t, 1, http.MethodPut, "/api/user/", `{"id":9531,"username":"mode-target","display_name":"mode-target","invitation_reward_mode":"commission"}`)
	require.Equal(t, http.StatusOK, adminUpdate.Code)
	var user model.User
	require.NoError(t, model.DB.First(&user, 9531).Error)
	assert.Equal(t, model.InvitationRewardModeCommission, user.InvitationRewardMode)

	rootUpdate := performAdminRequestWithRole(t, 2, common.RoleRootUser, http.MethodPut, "/api/user/", `{"id":9531,"username":"mode-target","display_name":"mode-target","invitation_reward_mode":"subscription"}`)
	require.Equal(t, http.StatusOK, rootUpdate.Code)
	require.NoError(t, model.DB.First(&user, 9531).Error)
	assert.Equal(t, model.InvitationRewardModeSubscription, user.InvitationRewardMode)

	invalidUpdate := performAdminRequest(t, 1, http.MethodPut, "/api/user/", `{"id":9531,"username":"mode-target","display_name":"mode-target","invitation_reward_mode":"invalid"}`)
	require.Equal(t, http.StatusOK, invalidUpdate.Code)
	assert.Contains(t, invalidUpdate.Body.String(), `"success":false`)
	require.NoError(t, model.DB.First(&user, 9531).Error)
	assert.Equal(t, model.InvitationRewardModeSubscription, user.InvitationRewardMode)

	selfUpdate := performUserRequest(t, 9531, http.MethodPut, "/api/user/self", `{"display_name":"self","invitation_reward_mode":"commission"}`)
	require.Equal(t, http.StatusOK, selfUpdate.Code)
	require.NoError(t, model.DB.First(&user, 9531).Error)
	assert.Equal(t, model.InvitationRewardModeSubscription, user.InvitationRewardMode)
}
