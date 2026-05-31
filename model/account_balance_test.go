package model

import (
	"math"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/alicebob/miniredis/v2"
	"github.com/glebarez/sqlite"
	"github.com/go-redis/redis/v8"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestAccountBalanceCentsFromCNY(t *testing.T) {
	cases := []struct {
		name    string
		amount  string
		want    int
		wantErr bool
	}{
		{name: "forty yuan", amount: "40", want: 4000},
		{name: "thirty nine point nine", amount: "39.9", want: 3990},
		{name: "round half up to cents", amount: "0.015", want: 2},
		{name: "reject sub cent rounded to zero", amount: "0.004", wantErr: true},
		{name: "reject zero", amount: "0", wantErr: true},
		{name: "reject negative", amount: "-1", wantErr: true},
		{name: "reject overflow", amount: decimal.NewFromInt(int64(math.MaxInt)).Div(decimal.NewFromInt(100)).Add(decimal.NewFromInt(1)).String(), wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			amount, err := decimal.NewFromString(tc.amount)
			require.NoError(t, err)

			got, err := AccountBalanceCentsFromCNY(amount)
			if tc.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
			wantCNY := decimal.NewFromInt(int64(got)).Div(decimal.NewFromInt(100))
			assert.True(t, AccountBalanceCNYFromCents(got).Equal(wantCNY))
		})
	}
}

func TestDeductAndIncreaseUserAccountBalanceTxUseCents(t *testing.T) {
	setupAccountBalanceTestDB(t)
	user := &User{Id: 9101, Username: "balance-cents", Quota: 4000, Status: common.UserStatusEnabled}
	require.NoError(t, DB.Create(user).Error)

	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return DeductUserAccountBalanceTx(tx, user.Id, 3990)
	}))
	assert.Equal(t, 10, getUserQuotaForAccountBalanceTest(t, user.Id))

	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return IncreaseUserAccountBalanceTx(tx, user.Id, 250)
	}))
	assert.Equal(t, 260, getUserQuotaForAccountBalanceTest(t, user.Id))

	err := DB.Transaction(func(tx *gorm.DB) error {
		return DeductUserAccountBalanceTx(tx, user.Id, 261)
	})
	require.EqualError(t, err, "余额不足")
	assert.Equal(t, 260, getUserQuotaForAccountBalanceTest(t, user.Id))
}

func TestAccountBalanceTxHelpersValidateInputs(t *testing.T) {
	setupAccountBalanceTestDB(t)
	require.Error(t, DeductUserAccountBalanceTx(nil, 1, 1))
	require.Error(t, IncreaseUserAccountBalanceTx(nil, 1, 1))
	require.Error(t, DB.Transaction(func(tx *gorm.DB) error {
		return DeductUserAccountBalanceTx(tx, 0, 1)
	}))
	require.Error(t, DB.Transaction(func(tx *gorm.DB) error {
		return DeductUserAccountBalanceTx(tx, 1, 0)
	}))
	require.Error(t, DB.Transaction(func(tx *gorm.DB) error {
		return IncreaseUserAccountBalanceTx(tx, 0, 1)
	}))
	require.Error(t, DB.Transaction(func(tx *gorm.DB) error {
		return IncreaseUserAccountBalanceTx(tx, 1, 0)
	}))
}

func TestAccountBalanceTxHelpersInvalidateUserCache(t *testing.T) {
	setupAccountBalanceTestDB(t)
	setupAccountBalanceTestRedis(t)
	user := &User{Id: 9102, Username: "cache-balance", Quota: 4000, Status: common.UserStatusEnabled}
	require.NoError(t, DB.Create(user).Error)
	require.NoError(t, updateUserCache(*user))

	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return IncreaseUserAccountBalanceTx(tx, user.Id, 250)
	}))
	stale, err := GetUserCache(user.Id)
	require.NoError(t, err)
	assert.Equal(t, 4000, stale.Quota)

	require.NoError(t, InvalidateUserCache(user.Id))
	cache, err := GetUserCache(user.Id)
	require.NoError(t, err)
	assert.Equal(t, 4250, cache.Quota)
}

func TestAccountBalanceTxInvalidatesAfterCommitOnly(t *testing.T) {
	setupAccountBalanceTestDB(t)
	setupAccountBalanceTestRedis(t)
	user := &User{Id: 9103, Username: "cache-race", Quota: 4000, Status: common.UserStatusEnabled}
	require.NoError(t, DB.Create(user).Error)
	require.NoError(t, updateUserCache(*user))

	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		err := IncreaseUserAccountBalanceTx(tx, user.Id, 250)
		cache, cacheErr := GetUserCache(user.Id)
		require.NoError(t, cacheErr)
		assert.Equal(t, 4000, cache.Quota)
		return err
	}))

	stale, err := GetUserCache(user.Id)
	require.NoError(t, err)
	assert.Equal(t, 4000, stale.Quota)
	require.NoError(t, InvalidateUserCache(user.Id))
	fresh, err := GetUserCache(user.Id)
	require.NoError(t, err)
	assert.Equal(t, 4250, fresh.Quota)
}

func setupAccountBalanceTestDB(t *testing.T) {
	t.Helper()

	oldDB := DB
	oldLogDB := LOG_DB
	oldUsingSQLite := common.UsingSQLite
	oldUsingMySQL := common.UsingMySQL
	oldUsingPostgreSQL := common.UsingPostgreSQL
	oldLogSQLType := common.LogSqlType
	oldRedisEnabled := common.RedisEnabled
	oldRDB := common.RDB

	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.LogSqlType = common.DatabaseTypeSQLite
	common.RedisEnabled = false
	common.RDB = nil
	initCol()

	safeName := strings.NewReplacer("/", "_", " ", "_", ":", "_").Replace(t.Name())
	db, err := gorm.Open(sqlite.Open("file:"+safeName+"_account_balance?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)

	DB = db
	LOG_DB = db
	require.NoError(t, DB.AutoMigrate(&User{}))

	t.Cleanup(func() {
		DB = oldDB
		LOG_DB = oldLogDB
		common.UsingSQLite = oldUsingSQLite
		common.UsingMySQL = oldUsingMySQL
		common.UsingPostgreSQL = oldUsingPostgreSQL
		common.LogSqlType = oldLogSQLType
		common.RedisEnabled = oldRedisEnabled
		common.RDB = oldRDB
		initCol()
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
}

func getUserQuotaForAccountBalanceTest(t *testing.T, userId int) int {
	t.Helper()

	var user User
	require.NoError(t, DB.Select("quota").First(&user, userId).Error)
	return user.Quota
}
func setupAccountBalanceTestRedis(t *testing.T) {
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
