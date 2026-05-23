package service

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskBillingIgnoresLegacyTaskAndUserGroupsForTokenRecalculation(t *testing.T) {
	truncate(t)

	originalRatios := ratio_setting.GroupRatio2JSONString()
	originalGroupRatios := ratio_setting.GroupGroupRatio2JSONString()
	originalModelRatios := ratio_setting.ModelRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(originalRatios))
		require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(originalGroupRatios))
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(originalModelRatios))
	})
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"vip":9,"default":1}`))
	require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(`{"vip":{"vip":7}}`))
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"test-model":2}`))

	const userID, tokenID, channelID = 510, 510, 510
	const initQuota = 10000
	const tokenRemain = 9000
	seedUser(t, userID, initQuota)
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", userID).Update("group", "vip").Error)
	seedToken(t, tokenID, userID, "sk-task-group-ignored", tokenRemain)
	seedChannel(t, channelID)

	task := makeTask(userID, channelID, 100, tokenID, BillingSourceWallet, 0)
	task.Group = "vip"
	task.PrivateData.BillingContext.QuotaMultiplier = 9

	RecalculateTaskQuotaByTokens(context.Background(), task, 10)

	assert.Equal(t, 20, task.Quota)
	assert.Equal(t, initQuota+80, getUserQuota(t, userID))
	assert.Equal(t, tokenRemain+80, getTokenRemainQuota(t, tokenID))
	log := getLastLog(t)
	require.NotNil(t, log)
	assert.Empty(t, log.Group)
	assert.NotContains(t, log.Content, "groupRatio")
	assert.NotContains(t, log.Content, "分组倍率")
	assert.NotContains(t, log.Other, "group_ratio")
	assert.NotContains(t, log.Other, "user_group_ratio")
}

func TestTaskBillingLogsDoNotPersistBusinessGroup(t *testing.T) {
	truncate(t)
	const userID, tokenID, channelID = 520, 520, 520
	seedUser(t, userID, 10000)
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", userID).Update("group", "vip").Error)
	seedToken(t, tokenID, userID, "sk-task-log-group", 9000)
	seedChannel(t, channelID)
	task := makeTask(userID, channelID, 300, tokenID, BillingSourceWallet, 0)
	task.Group = "vip"
	task.PrivateData.BillingContext.QuotaMultiplier = 9

	RefundTaskQuota(context.Background(), task, "legacy grouped task failed")

	log := getLastLog(t)
	require.NotNil(t, log)
	assert.Empty(t, log.Group)
	assert.NotContains(t, log.Other, "group_ratio")
	assert.NotContains(t, log.Other, "user_group_ratio")
}

func TestTaskBillingSourceNoLongerContainsMJBusinessGroupTerms(t *testing.T) {
	for _, path := range []string{"task_billing.go", "../relay/mjproxy_handler.go"} {
		source := readSourceForTaskBillingTest(t, path)
		assert.NotContains(t, source, "分组倍率", path)
		assert.NotContains(t, source, "Group:     info.UsingGroup", path)
		assert.NotContains(t, source, "Group:            relayInfo.UsingGroup", path)
	}
}

func readSourceForTaskBillingTest(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(data)
}

func TestTaskBillingOtherOmitsLegacyBusinessGroupRatio(t *testing.T) {
	task := makeTask(1, 1, 100, 1, BillingSourceWallet, 0)
	task.PrivateData.BillingContext.QuotaMultiplier = 9
	other := taskBillingOther(task)
	assert.NotContains(t, other, "group_ratio")
	for key := range other {
		assert.False(t, strings.Contains(key, "group"), key)
	}
}
