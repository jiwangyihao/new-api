package controller

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupManageUserAccountBalanceTestDB(t *testing.T) {
	t.Helper()
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	originalQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 500000
	t.Cleanup(func() {
		common.QuotaPerUnit = originalQuotaPerUnit
	})
}

func TestManageUserQuotaUsesAccountBalanceCents(t *testing.T) {
	setupManageUserAccountBalanceTestDB(t)
	admin := seedManageUserAccountBalanceUser(t, 9420, "admin", common.RoleRootUser, 0)
	user := seedManageUserAccountBalanceUser(t, 9421, "managed", common.RoleCommonUser, 4000)

	performManageUserQuotaRequest(t, admin, user.Id, "add", 250)
	assert.Equal(t, 4250, getManageUserQuotaForAccountBalanceTest(t, user.Id))
	assertManageLogContainsAccountBalance(t, user.Id, "2.50")

	performManageUserQuotaRequest(t, admin, user.Id, "subtract", 125)
	assert.Equal(t, 4125, getManageUserQuotaForAccountBalanceTest(t, user.Id))
	assertManageLogContainsAccountBalance(t, user.Id, "1.25")

	performManageUserQuotaRequest(t, admin, user.Id, "override", 3990)
	assert.Equal(t, 3990, getManageUserQuotaForAccountBalanceTest(t, user.Id))
	assertManageLogContainsAccountBalance(t, user.Id, "39.90")
}

func TestManageUserQuotaIgnoresCacheInvalidationFailure(t *testing.T) {
	setupManageUserAccountBalanceTestDB(t)
	setupControllerBrokenRedis(t)
	admin := seedManageUserAccountBalanceUser(t, 9422, "admin-broken-cache", common.RoleRootUser, 0)
	user := seedManageUserAccountBalanceUser(t, 9423, "managed-broken-cache", common.RoleCommonUser, 4000)

	performManageUserQuotaRequest(t, admin, user.Id, "add", 250)

	assert.Equal(t, 4250, getManageUserQuotaForAccountBalanceTest(t, user.Id))
}

func seedManageUserAccountBalanceUser(t *testing.T, id int, username string, role int, quota int) *model.User {
	t.Helper()
	user := &model.User{Id: id, Username: username, Role: role, Status: common.UserStatusEnabled, Quota: quota, AffCode: username}
	require.NoError(t, model.DB.Create(user).Error)
	return user
}

func performManageUserQuotaRequest(t *testing.T, admin *model.User, userID int, mode string, value int) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/user/manage", strings.NewReader(`{"id":`+strconvItoaForManageTest(userID)+`,"action":"add_quota","mode":"`+mode+`","value":`+strconvItoaForManageTest(value)+`}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("id", admin.Id)
	ctx.Set("username", admin.Username)
	ctx.Set("role", admin.Role)

	ManageUser(ctx)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	assert.Contains(t, recorder.Body.String(), `"success":true`)
	return recorder
}

func getManageUserQuotaForAccountBalanceTest(t *testing.T, userID int) int {
	t.Helper()
	var user model.User
	require.NoError(t, model.DB.Select("quota").First(&user, userID).Error)
	return user.Quota
}

func assertManageLogContainsAccountBalance(t *testing.T, userID int, expected string) {
	t.Helper()
	var logs []model.Log
	require.NoError(t, model.LOG_DB.Where("user_id = ? AND type = ?", userID, model.LogTypeManage).Order("id DESC").Find(&logs).Error)
	for _, log := range logs {
		if strings.Contains(log.Content, expected) {
			assert.NotContains(t, log.Content, "500000")
			return
		}
	}
	t.Fatalf("missing manage log %q for user %d: %#v", expected, userID, logs)
}

func strconvItoaForManageTest(value int) string {
	return strconv.FormatInt(int64(value), 10)
}

func setupControllerBrokenRedis(t *testing.T) {
	t.Helper()
	oldRedisEnabled := common.RedisEnabled
	oldRDB := common.RDB
	server, err := miniredis.Run()
	require.NoError(t, err)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	common.RedisEnabled = true
	common.RDB = client
	require.NoError(t, client.Close())
	t.Cleanup(func() {
		common.RedisEnabled = oldRedisEnabled
		common.RDB = oldRDB
		server.Close()
	})
}
