package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckinRewardUsesAccountBalanceCents(t *testing.T) {
	setupRewardCentsTestDB(t)
	require.NoError(t, DB.AutoMigrate(&Checkin{}))
	checkinSetting := operation_setting.GetCheckinSetting()
	oldCheckinSetting := *checkinSetting
	t.Cleanup(func() {
		*checkinSetting = oldCheckinSetting
	})
	checkinSetting.Enabled = true
	checkinSetting.MinQuota = 20
	checkinSetting.MaxQuota = 20
	user := &User{Id: 9510, Username: "checkin", Status: common.UserStatusEnabled, Quota: 1000}
	require.NoError(t, DB.Create(user).Error)

	checkin, err := UserCheckin(user.Id)

	require.NoError(t, err)
	require.NotNil(t, checkin)
	assert.Equal(t, 20, checkin.QuotaAwarded)
	assert.Equal(t, 1020, getUserQuotaForRewardTest(t, user.Id))
	assert.Equal(t, 20, getCheckinQuotaAwardedForAccountBalanceTest(t, user.Id, checkin.CheckinDate))
}

func TestCheckinRewardInvalidatesUserCacheAfterCentsCredit(t *testing.T) {
	setupRewardCentsTestDB(t)
	setupRewardCentsRedis(t)
	require.NoError(t, DB.AutoMigrate(&Checkin{}))
	checkinSetting := operation_setting.GetCheckinSetting()
	oldCheckinSetting := *checkinSetting
	t.Cleanup(func() {
		*checkinSetting = oldCheckinSetting
	})
	checkinSetting.Enabled = true
	checkinSetting.MinQuota = 20
	checkinSetting.MaxQuota = 20
	user := &User{Id: 9511, Username: "checkin-cache", Status: common.UserStatusEnabled, Quota: 1000}
	require.NoError(t, DB.Create(user).Error)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", user.Id).Update("username", "checkin-cache-updated").Error)
	require.NoError(t, updateUserCache(*user))

	_, err := UserCheckin(user.Id)

	require.NoError(t, err)
	cache, err := GetUserCache(user.Id)
	require.NoError(t, err)
	assert.Equal(t, "checkin-cache-updated", cache.Username)
	assert.Equal(t, 1020, cache.Quota)
}

func TestCheckinRewardIgnoresCacheInvalidationFailure(t *testing.T) {
	setupRewardCentsTestDB(t)
	setupRewardCentsBrokenRedis(t)
	require.NoError(t, DB.AutoMigrate(&Checkin{}))
	checkinSetting := operation_setting.GetCheckinSetting()
	oldCheckinSetting := *checkinSetting
	t.Cleanup(func() {
		*checkinSetting = oldCheckinSetting
	})
	checkinSetting.Enabled = true
	checkinSetting.MinQuota = 20
	checkinSetting.MaxQuota = 20
	user := &User{Id: 9512, Username: "checkin-broken-cache", Status: common.UserStatusEnabled, Quota: 1000}
	require.NoError(t, DB.Create(user).Error)

	checkin, err := UserCheckin(user.Id)

	require.NoError(t, err)
	require.NotNil(t, checkin)
	assert.Equal(t, 1020, getUserQuotaForRewardTest(t, user.Id))
}

func getCheckinQuotaAwardedForAccountBalanceTest(t *testing.T, userId int, date string) int {
	t.Helper()
	var checkin Checkin
	require.NoError(t, DB.Where("user_id = ? AND checkin_date = ?", userId, date).First(&checkin).Error)
	return checkin.QuotaAwarded
}
