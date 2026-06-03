package model

import (
	"errors"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/alicebob/miniredis/v2"
	"github.com/glebarez/sqlite"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupRewardCentsTestDB(t *testing.T) {
	t.Helper()

	oldDB := DB
	oldLogDB := LOG_DB
	oldUsingSQLite := common.UsingSQLite
	oldUsingMySQL := common.UsingMySQL
	oldUsingPostgreSQL := common.UsingPostgreSQL
	oldLogSQLType := common.LogSqlType
	oldRedisEnabled := common.RedisEnabled
	oldRDB := common.RDB
	oldBatchUpdateEnabled := common.BatchUpdateEnabled
	oldQuotaForNewUser := common.QuotaForNewUser
	oldQuotaForInviter := common.QuotaForInviter
	oldQuotaForInvitee := common.QuotaForInvitee
	oldQuotaPerUnit := common.QuotaPerUnit

	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.LogSqlType = common.DatabaseTypeSQLite
	common.RedisEnabled = false
	common.RDB = nil
	common.BatchUpdateEnabled = false
	common.QuotaPerUnit = 500000
	initCol()

	safeName := strings.NewReplacer("/", "_", " ", "_", ":", "_").Replace(t.Name())
	db, err := gorm.Open(sqlite.Open("file:"+safeName+"_rewards?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	DB = db
	LOG_DB = db
	require.NoError(t, db.AutoMigrate(&User{}, &Log{}))

	t.Cleanup(func() {
		DB = oldDB
		LOG_DB = oldLogDB
		common.UsingSQLite = oldUsingSQLite
		common.UsingMySQL = oldUsingMySQL
		common.UsingPostgreSQL = oldUsingPostgreSQL
		common.LogSqlType = oldLogSQLType
		common.RedisEnabled = oldRedisEnabled
		common.RDB = oldRDB
		common.BatchUpdateEnabled = oldBatchUpdateEnabled
		common.QuotaForNewUser = oldQuotaForNewUser
		common.QuotaForInviter = oldQuotaForInviter
		common.QuotaForInvitee = oldQuotaForInvitee
		common.QuotaPerUnit = oldQuotaPerUnit
		initCol()
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
}

func TestInviteRewardsUseAccountBalanceCents(t *testing.T) {
	setupRewardCentsTestDB(t)
	common.QuotaForNewUser = 2000
	common.QuotaForInviter = 1000
	common.QuotaForInvitee = 500
	inviter := &User{Id: 9410, Username: "inviter", Status: common.UserStatusEnabled, AffCode: "AFF9410"}
	require.NoError(t, DB.Create(inviter).Error)
	invitee := &User{Username: "invitee", Status: common.UserStatusEnabled, InviterId: 9410}
	require.NoError(t, invitee.Insert(9410))

	assert.Equal(t, 1000, getUserAffQuotaForRewardTest(t, 9410))
	assert.Equal(t, 1000, getUserAffHistoryForRewardTest(t, 9410))
	assert.Equal(t, 2500, getUserQuotaForRewardTest(t, invitee.Id))
	assertRewardLogUsesAccountBalanceFormat(t, invitee.Id, "5.00")
	assertRewardLogUsesAccountBalanceFormat(t, 9410, "10.00")
}

func TestTransferAffQuotaToQuotaUsesAccountBalanceCentsAndInvalidatesCache(t *testing.T) {
	setupRewardCentsTestDB(t)
	setupRewardCentsRedis(t)
	user := &User{Id: 9412, Username: "transfer", Status: common.UserStatusEnabled, AffCode: "AFF9412", AffQuota: 100, Quota: 0}
	require.NoError(t, DB.Create(user).Error)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", user.Id).Update("username", "transfer-updated").Error)
	require.NoError(t, updateUserCache(*user))

	require.NoError(t, user.TransferAffQuotaToQuota(1))

	assert.Equal(t, 99, getUserAffQuotaForRewardTest(t, 9412))
	assert.Equal(t, 1, getUserQuotaForRewardTest(t, 9412))
	cache, err := GetUserCache(9412)
	require.NoError(t, err)
	assert.Equal(t, "transfer-updated", cache.Username)
	assert.Equal(t, 1, cache.Quota)
}

func TestTransferAffQuotaToQuotaIgnoresCacheInvalidationFailure(t *testing.T) {
	setupRewardCentsTestDB(t)
	setupRewardCentsBrokenRedis(t)
	user := &User{Id: 9414, Username: "transfer-broken-cache", Status: common.UserStatusEnabled, AffCode: "AFF9414", AffQuota: 100, Quota: 0}
	require.NoError(t, DB.Create(user).Error)

	require.NoError(t, user.TransferAffQuotaToQuota(1))

	assert.Equal(t, 99, getUserAffQuotaForRewardTest(t, user.Id))
	assert.Equal(t, 1, getUserQuotaForRewardTest(t, user.Id))
}

func TestRegistrationRewardEntryPointsUseAccountBalanceCents(t *testing.T) {
	setupRewardCentsTestDB(t)
	common.QuotaForNewUser = 2000
	common.QuotaForInviter = 1000
	common.QuotaForInvitee = 500
	inviter := &User{Id: 9413, Username: "entry-inviter", Status: common.UserStatusEnabled, AffCode: "AFF9413"}
	require.NoError(t, DB.Create(inviter).Error)

	inserted := &User{Username: "insert", Quota: common.QuotaForNewUser}
	require.NoError(t, inserted.Insert(0))
	assert.Equal(t, 2000, getUserQuotaForRewardTest(t, inserted.Id))
	assertRewardLogUsesAccountBalanceFormat(t, inserted.Id, "20.00")

	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		withTx := &User{Username: "insert-tx", Quota: common.QuotaForNewUser}
		return withTx.InsertWithTx(tx, 0)
	}))
	assert.Equal(t, 2000, getUserQuotaByUsernameForRewardTest(t, "insert-tx"))

	finalized := &User{Username: "finalize", InviterId: 9413, Status: common.UserStatusEnabled}
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		if err := finalized.InsertWithTx(tx, 9413); err != nil {
			return err
		}
		return finalized.FinalizeCreationTx(tx, 9413)
	}))
	assert.Equal(t, 2500, getUserQuotaForRewardTest(t, finalized.Id))
	assert.Equal(t, 1000, getUserAffQuotaForRewardTest(t, 9413))
	assertRewardLogUsesAccountBalanceFormat(t, finalized.Id, "5.00")
	assertRewardLogUsesAccountBalanceFormat(t, 9413, "10.00")

	oauth := &User{Username: "oauth", InviterId: 9413, Status: common.UserStatusEnabled}
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return oauth.InsertWithTx(tx, 9413)
	}))
	oauth.FinalizeOAuthUserCreation(9413)
	assert.Equal(t, 2500, getUserQuotaForRewardTest(t, oauth.Id))
	assertRewardLogUsesAccountBalanceFormat(t, oauth.Id, "5.00")
}

func TestFinalizeCreationTxWithSeparateLogDBDefersRewardLogsUntilCommit(t *testing.T) {
	setupRewardCentsTestDB(t)
	common.QuotaForNewUser = 2000
	common.QuotaForInviter = 1000
	common.QuotaForInvitee = 500
	inviter := &User{Id: 9415, Username: "separate-log-inviter", Status: common.UserStatusEnabled, AffCode: "AFF9415"}
	require.NoError(t, DB.Create(inviter).Error)

	safeName := strings.NewReplacer("/", "_", " ", "_", ":", "_").Replace(t.Name())
	logDB, err := gorm.Open(sqlite.Open("file:"+safeName+"_logs?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, logDB.AutoMigrate(&Log{}))
	LOG_DB = logDB
	t.Cleanup(func() {
		if sqlDB, err := logDB.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})

	rolledBack := &User{Username: "separate-log-rollback", InviterId: inviter.Id, Status: common.UserStatusEnabled}
	err = DB.Transaction(func(tx *gorm.DB) error {
		if err := rolledBack.InsertWithTx(tx, inviter.Id); err != nil {
			return err
		}
		if err := rolledBack.FinalizeCreationTx(tx, inviter.Id); err != nil {
			return err
		}
		return errors.New("force rollback")
	})
	require.Error(t, err)
	assertRewardLogCountForAccountBalanceTest(t, 0)

	committed := &User{Username: "separate-log-commit", InviterId: inviter.Id, Status: common.UserStatusEnabled}
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		if err := committed.InsertWithTx(tx, inviter.Id); err != nil {
			return err
		}
		return committed.FinalizeCreationTx(tx, inviter.Id)
	}))
	assertRewardLogCountForAccountBalanceTest(t, 0)

	committed.RecordAccountBalanceRewardLogsAfterTx(inviter.Id)

	assertRewardLogUsesAccountBalanceFormat(t, committed.Id, "20.00")
	assertRewardLogUsesAccountBalanceFormat(t, committed.Id, "5.00")
	assertRewardLogUsesAccountBalanceFormat(t, inviter.Id, "10.00")
}

func TestFinalizeOAuthUserCreationWithSeparateLogDBDoesNotLogWhenRewardTransactionFails(t *testing.T) {
	setupRewardCentsTestDB(t)
	common.QuotaForNewUser = 2000
	common.QuotaForInviter = 1000
	common.QuotaForInvitee = 500
	inviter := &User{Id: 9416, Username: "oauth-failure-inviter", Status: common.UserStatusEnabled, AffCode: "AFF9416"}
	require.NoError(t, DB.Create(inviter).Error)

	safeName := strings.NewReplacer("/", "_", " ", "_", ":", "_").Replace(t.Name())
	logDB, err := gorm.Open(sqlite.Open("file:"+safeName+"_logs?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, logDB.AutoMigrate(&Log{}))
	LOG_DB = logDB
	t.Cleanup(func() {
		if sqlDB, err := logDB.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})

	missingUser := &User{Id: 9417, Username: "oauth-missing-user", InviterId: inviter.Id, Status: common.UserStatusEnabled}

	missingUser.FinalizeOAuthUserCreation(inviter.Id)

	assertRewardLogCountForAccountBalanceTest(t, 0)
	assert.Equal(t, 0, getUserAffQuotaForRewardTest(t, inviter.Id))
}

func getUserQuotaForRewardTest(t *testing.T, userId int) int {
	t.Helper()
	var user User
	require.NoError(t, DB.Select("quota").First(&user, userId).Error)
	return user.Quota
}

func getUserQuotaByUsernameForRewardTest(t *testing.T, username string) int {
	t.Helper()
	var user User
	require.NoError(t, DB.Select("quota").Where("username = ?", username).First(&user).Error)
	return user.Quota
}

func getUserAffQuotaForRewardTest(t *testing.T, userId int) int {
	t.Helper()
	var user User
	require.NoError(t, DB.Select("aff_quota").First(&user, userId).Error)
	return user.AffQuota
}

func getUserAffHistoryForRewardTest(t *testing.T, userId int) int {
	t.Helper()
	var user User
	require.NoError(t, DB.Select("aff_history").First(&user, userId).Error)
	return user.AffHistoryQuota
}

func assertRewardLogUsesAccountBalanceFormat(t *testing.T, userId int, expected string) {
	t.Helper()
	var logs []Log
	require.NoError(t, LOG_DB.Where("user_id = ? AND type = ?", userId, LogTypeSystem).Order("id DESC").Find(&logs).Error)
	for _, log := range logs {
		if strings.Contains(log.Content, expected) {
			assert.NotContains(t, log.Content, "500000")
			assert.NotContains(t, log.Content, "额度")
			assert.NotContains(t, log.Content, "¥0.00")
			return
		}
	}
	t.Fatalf("missing account balance log %q for user %d: %#v", expected, userId, logs)
}

func assertRewardLogCountForAccountBalanceTest(t *testing.T, expected int64) {
	t.Helper()
	var count int64
	require.NoError(t, LOG_DB.Model(&Log{}).Where("type = ?", LogTypeSystem).Count(&count).Error)
	assert.Equal(t, expected, count)
}

func setupRewardCentsRedis(t *testing.T) {
	t.Helper()
	server, err := miniredis.Run()
	require.NoError(t, err)
	common.RedisEnabled = true
	common.RDB = redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() {
		_ = common.RDB.Close()
		server.Close()
	})
}

func setupRewardCentsBrokenRedis(t *testing.T) {
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
