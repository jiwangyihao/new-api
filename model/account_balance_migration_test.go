package model

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestEnsureAccountBalanceCentsMigrationConvertsAccountBalanceFields(t *testing.T) {
	setupAccountBalanceMigrationTestDB(t)
	setOptionForMigrationTest(t, "QuotaPerUnit", "250000")
	seedRuntimeBalanceOptionsForMigrationTest(t, map[string]string{
		"QuotaForNewUser":    "1000000",
		"QuotaForInviter":    "500000",
		"QuotaForInvitee":    "250000",
		"checkin_setting":    `{"enabled":true,"min_quota":5000,"max_quota":10000}`,
		"KyrenTopUpProducts": `[{"id":"topup_40","name":"40 CNY","amount":"40.00","currency":"CNY","quota":20000000,"enabled":true}]`,
		"CreemProducts":      `[{"name":"Creem 40","productId":"prod_40","price":40,"quota":20000000,"currency":"USD"}]`,
	})
	require.NoError(t, DB.Create(&User{Id: 9201, Username: "migrate", Quota: 1000000, AffQuota: 250000, AffHistoryQuota: 997500}).Error)
	deletedRedemption := &Redemption{Id: 9208, Key: "deleted-wallet-key", Name: "deleted-wallet", Type: RedemptionTypeWallet, Quota: 500000}
	require.NoError(t, DB.Create(deletedRedemption).Error)
	require.NoError(t, DB.Delete(deletedRedemption).Error)
	require.NoError(t, DB.Create(&Redemption{Id: 9202, Key: "wallet-key", Name: "wallet", Type: RedemptionTypeWallet, Quota: 1000000}).Error)
	require.NoError(t, DB.Create(&Redemption{Id: 9203, Key: "sub-key", Name: "sub", Type: RedemptionTypeSubscription, Quota: 1000000, PlanId: 1}).Error)
	require.NoError(t, DB.Create(&Redemption{Id: 9206, Key: "blank-wallet-key", Name: "blank-wallet", Type: "", Quota: 1000000}).Error)
	deleted := &User{Id: 9207, Username: "deleted-migrate", Quota: 500000, AffQuota: 250000, AffHistoryQuota: 125000, AffCode: "deleted-migrate-aff"}
	require.NoError(t, DB.Create(deleted).Error)
	require.NoError(t, DB.Delete(deleted).Error)
	require.NoError(t, DB.Create(&Checkin{UserId: 9201, CheckinDate: "2026-05-30", QuotaAwarded: 5000}).Error)
	require.NoError(t, DB.Create(&TopUp{Id: 9204, UserId: 9201, Amount: 1000000, TradeNo: "pending-old", PaymentProvider: PaymentProviderEpay, PaymentMethod: "alipay", Status: common.TopUpStatusPending}).Error)
	require.NoError(t, DB.Create(&TopUp{Id: 9205, UserId: 9201, Amount: 1000000, TradeNo: "success-old", PaymentProvider: PaymentProviderEpay, PaymentMethod: "alipay", Status: common.TopUpStatusSuccess}).Error)

	require.NoError(t, EnsureAccountBalanceCentsMigration())

	assert.Equal(t, 400, getUserQuotaForAccountBalanceTest(t, 9201))
	assert.Equal(t, 100, getUserAffQuotaForMigrationTest(t, 9201))
	assert.Equal(t, 399, getUserAffHistoryForMigrationTest(t, 9201))
	assert.Equal(t, 200, getUserQuotaUnscopedForMigrationTest(t, 9207))
	assert.Equal(t, 100, getUserAffQuotaUnscopedForMigrationTest(t, 9207))
	assert.Equal(t, 50, getUserAffHistoryUnscopedForMigrationTest(t, 9207))
	assert.Equal(t, 400, getRedemptionQuotaForMigrationTest(t, 9202))
	assert.Equal(t, 200, getRedemptionQuotaUnscopedForMigrationTest(t, 9208))
	assert.Equal(t, 1000000, getRedemptionQuotaForMigrationTest(t, 9203))
	assert.Equal(t, 400, getRedemptionQuotaForMigrationTest(t, 9206))
	assert.Equal(t, RedemptionTypeWallet, getRedemptionTypeForMigrationTest(t, 9206))
	assert.Equal(t, 2, getCheckinQuotaForMigrationTest(t, 9201, "2026-05-30"))
	assert.Equal(t, common.TopUpStatusExpired, getTopUpStatusForMigrationTest(t, "pending-old"))
	assert.EqualValues(t, 1000000, getTopUpAmountForMigrationTest(t, "success-old"))
	assert.Equal(t, "", getTopUpAmountUnitForMigrationTest(t, "success-old"))
	assert.Equal(t, int64(8000), getKyrenTopUpProductQuotaForMigrationTest(t, "topup_40"))
	assert.Equal(t, int64(8000), getCreemProductQuotaForMigrationTest(t, "prod_40"))
	assert.Equal(t, 400, common.QuotaForNewUser)
	assert.Equal(t, 200, common.QuotaForInviter)
	assert.Equal(t, 100, common.QuotaForInvitee)
	assert.Equal(t, 2, operation_setting.GetCheckinSetting().MinQuota)
	assert.Equal(t, 4, operation_setting.GetCheckinSetting().MaxQuota)
	assert.Contains(t, common.OptionMap["KyrenTopUpProducts"], `"quota":8000`)
	assert.Contains(t, common.OptionMap["CreemProducts"], `"quota":8000`)
	assert.Equal(t, "true", getOptionValueForMigrationTest(t, OptionAccountBalanceCentsDataMigrated))
	assert.Equal(t, "true", getOptionValueForMigrationTest(t, OptionAccountBalanceCentsMigrated))
	assert.NotEmpty(t, getOptionValueForMigrationTest(t, OptionAccountBalanceCentsMigratedAt))
}

func TestEnsureAccountBalanceCentsMigrationInvalidatesOldUserCache(t *testing.T) {
	setupAccountBalanceMigrationTestDB(t)
	setOptionForMigrationTest(t, "QuotaPerUnit", "500000")
	require.NoError(t, DB.Create(&User{Id: 9230, Username: "cached", Quota: 20000000, Status: common.UserStatusEnabled}).Error)
	seedUserCacheForMigrationTest(t, &UserBase{Id: 9230, Quota: 20000000, Status: common.UserStatusEnabled, Username: "cached"})

	logs := captureAccountBalanceMigrationLogs(t, func() {
		require.NoError(t, EnsureAccountBalanceCentsMigration())
	})

	cache, err := GetUserCache(9230)
	require.NoError(t, err)
	assert.Equal(t, 4000, cache.Quota)
	assert.Contains(t, logs, `"user_cache_clear_mode":"redis_user_ids"`)
}

func TestEnsureAccountBalanceCentsMigrationDoesNotFinalizeWhenCacheClearFails(t *testing.T) {
	setupAccountBalanceMigrationTestDB(t)
	setOptionForMigrationTest(t, "QuotaPerUnit", "250000")
	require.NoError(t, DB.Create(&User{Id: 9231, Username: "cached-fail", Quota: 1000000, Status: common.UserStatusEnabled}).Error)
	seedUserCacheForMigrationTest(t, &UserBase{Id: 9231, Quota: 1000000, Status: common.UserStatusEnabled, Username: "cached-fail"})
	forceInvalidateAllUserCacheErrorForMigrationTest(errors.New("redis delete failed"))

	err := EnsureAccountBalanceCentsMigration()

	require.Error(t, err)
	assert.Equal(t, "true", getOptionValueForMigrationTest(t, OptionAccountBalanceCentsDataMigrated))
	assert.Empty(t, getOptionValueForMigrationTest(t, OptionAccountBalanceCentsMigrated))
	assert.Empty(t, getOptionValueForMigrationTest(t, OptionAccountBalanceCentsMigratedAt))
	forceInvalidateAllUserCacheErrorForMigrationTest(nil)
	require.NoError(t, EnsureAccountBalanceCentsMigration())
	assert.Equal(t, 400, getUserQuotaForAccountBalanceTest(t, 9231))
	assert.Equal(t, "true", getOptionValueForMigrationTest(t, OptionAccountBalanceCentsMigrated))
}

func TestEnsureAccountBalanceCentsMigrationUsesLoadedQuotaPerUnit(t *testing.T) {
	setupAccountBalanceMigrationTestDB(t)
	setOptionForMigrationTest(t, "QuotaPerUnit", "250000")
	common.QuotaPerUnit = 500000
	require.NoError(t, DB.Create(&User{Id: 9215, Username: "runtime-rate", Quota: 1000000}).Error)

	require.NoError(t, EnsureAccountBalanceCentsMigration())

	assert.Equal(t, 400, getUserQuotaForAccountBalanceTest(t, 9215))
}

func TestEnsureAccountBalanceCentsMigrationRejectsInvalidQuotaPerUnit(t *testing.T) {
	for _, value := range []string{"0", "-1", "not-number"} {
		t.Run(value, func(t *testing.T) {
			setupAccountBalanceMigrationTestDB(t)
			setOptionForMigrationTest(t, "QuotaPerUnit", value)
			require.NoError(t, DB.Create(&User{Id: 9216, Username: "invalid-qpu", Quota: 1000000}).Error)

			err := EnsureAccountBalanceCentsMigration()

			require.Error(t, err)
			assert.Empty(t, getOptionValueForMigrationTest(t, OptionAccountBalanceCentsDataMigrated))
			assert.Empty(t, getOptionValueForMigrationTest(t, OptionAccountBalanceCentsMigrated))
			assert.Empty(t, getOptionValueForMigrationTest(t, OptionAccountBalanceCentsMigratedAt))
		})
	}
}

func TestEnsureAccountBalanceCentsMigrationFallsBackToLoadedQuotaPerUnitWhenDBOptionMissing(t *testing.T) {
	setupAccountBalanceMigrationTestDB(t)
	common.OptionMap["QuotaPerUnit"] = "250000"
	common.QuotaPerUnit = 250000
	require.NoError(t, DB.Create(&User{Id: 9217, Username: "missing-qpu", Quota: 1000000}).Error)

	require.NoError(t, EnsureAccountBalanceCentsMigration())

	assert.Equal(t, 400, getUserQuotaForAccountBalanceTest(t, 9217))
	assert.Equal(t, "true", getOptionValueForMigrationTest(t, OptionAccountBalanceCentsDataMigrated))
	assert.Equal(t, "true", getOptionValueForMigrationTest(t, OptionAccountBalanceCentsMigrated))
}

func TestAccountBalanceMigrationOptionQueriesQuoteOptionKeyColumn(t *testing.T) {
	setupAccountBalanceMigrationTestDB(t)
	common.UsingSQLite = false
	common.UsingMySQL = true
	common.UsingPostgreSQL = false
	initCol()

	stmt := DB.Session(&gorm.Session{DryRun: true}).Model(&Option{}).Select("value").Where(commonKeyCol+" = ?", "QuotaPerUnit").Find(&Option{}).Statement

	assert.Contains(t, stmt.SQL.String(), commonKeyCol)
}

func TestEnsureAccountBalanceCentsMigrationLeavesNonAccountQuotaFieldsUnchanged(t *testing.T) {
	setupAccountBalanceMigrationTestDB(t)
	setOptionForMigrationTest(t, "QuotaPerUnit", "250000")
	seedNonAccountQuotaFieldsForMigrationTest(t, nonAccountQuotaSeed{
		LogQuota: 1000000, TokenRemainQuota: 1000000, TokenUsedQuota: 500000,
		ChannelUsedQuota: 750000, AbilityQuota: 1000000,
		UserSubscriptionTokenLimit: 1000000, UserSubscriptionTokenUsed: 100,
		SubscriptionPlanMonthlyTokenLimit: 1000000, TopUpMoney: 40,
	})

	require.NoError(t, EnsureAccountBalanceCentsMigration())

	assertNonAccountQuotaFieldsUnchanged(t, nonAccountQuotaSeed{
		LogQuota: 1000000, TokenRemainQuota: 1000000, TokenUsedQuota: 500000,
		ChannelUsedQuota: 750000, AbilityQuota: 1000000,
		UserSubscriptionTokenLimit: 1000000, UserSubscriptionTokenUsed: 100,
		SubscriptionPlanMonthlyTokenLimit: 1000000, TopUpMoney: 40,
	})
}

func TestEnsureAccountBalanceCentsMigrationRejectsInProgressMarker(t *testing.T) {
	setupAccountBalanceMigrationTestDB(t)
	setOptionForMigrationTest(t, OptionAccountBalanceCentsDataMigrated, accountBalanceCentsDataMigrationInProgress)
	setOptionForMigrationTest(t, "QuotaPerUnit", "250000")

	err := EnsureAccountBalanceCentsMigration()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "already in progress")
	assert.Empty(t, getOptionValueForMigrationTest(t, OptionAccountBalanceCentsMigrated))
}

func TestEnsureAccountBalanceCentsMigrationIsIdempotentAfterDataStage(t *testing.T) {
	setupAccountBalanceMigrationTestDB(t)
	setOptionForMigrationTest(t, "QuotaPerUnit", "250000")
	setOptionForMigrationTest(t, OptionAccountBalanceCentsDataMigrated, "true")
	require.NoError(t, DB.Create(&User{Id: 9210, Username: "already-data", Quota: 4000}).Error)

	require.NoError(t, EnsureAccountBalanceCentsMigration())

	assert.Equal(t, 4000, getUserQuotaForAccountBalanceTest(t, 9210))
	assert.Equal(t, "true", getOptionValueForMigrationTest(t, OptionAccountBalanceCentsMigrated))
}

func TestEnsureAccountBalanceCentsMigrationFinalMarkerReloadsMigratedAt(t *testing.T) {
	setupAccountBalanceMigrationTestDB(t)
	setOptionForMigrationTest(t, OptionAccountBalanceCentsMigrated, "true")
	setOptionForMigrationTest(t, OptionAccountBalanceCentsMigratedAt, "1777777777")
	delete(common.OptionMap, OptionAccountBalanceCentsMigrated)
	delete(common.OptionMap, OptionAccountBalanceCentsMigratedAt)

	require.NoError(t, EnsureAccountBalanceCentsMigration())

	assert.Equal(t, "true", common.OptionMap[OptionAccountBalanceCentsMigrated])
	assert.Equal(t, "1777777777", common.OptionMap[OptionAccountBalanceCentsMigratedAt])
}
func TestEnsureAccountBalanceCentsMigrationConcurrentDataCompletionFinalizes(t *testing.T) {
	setupAccountBalanceMigrationTestDB(t)
	setOptionForMigrationTest(t, "QuotaPerUnit", "250000")
	setOptionForMigrationTest(t, "QuotaForNewUser", "400")
	delete(common.OptionMap, OptionAccountBalanceCentsDataMigrated)
	delete(common.OptionMap, OptionAccountBalanceCentsMigrated)
	common.QuotaForNewUser = 1000000
	accountBalanceMigrationBeforeDataTxHook = func() error {
		return upsertOptionTx(DB, OptionAccountBalanceCentsDataMigrated, "true")
	}

	require.NoError(t, EnsureAccountBalanceCentsMigration())

	assert.Equal(t, 400, common.QuotaForNewUser)
	assert.Equal(t, "true", getOptionValueForMigrationTest(t, OptionAccountBalanceCentsMigrated))
}

func TestEnsureAccountBalanceCentsMigrationDataStageRetryReloadsRuntimeOptions(t *testing.T) {
	setupAccountBalanceMigrationTestDB(t)
	setOptionForMigrationTest(t, "QuotaPerUnit", "250000")
	setOptionForMigrationTest(t, OptionAccountBalanceCentsDataMigrated, "true")
	setOptionForMigrationTest(t, "QuotaForNewUser", "400")
	setOptionForMigrationTest(t, "QuotaForInviter", "200")
	setOptionForMigrationTest(t, "QuotaForInvitee", "100")
	setOptionForMigrationTest(t, "checkin_setting", `{"enabled":true,"min_quota":2,"max_quota":4}`)
	setOptionForMigrationTest(t, "KyrenTopUpProducts", `[{"id":"topup_40","name":"40 CNY","amount":"40.00","currency":"CNY","quota":8000,"enabled":true}]`)
	setOptionForMigrationTest(t, "CreemProducts", `[{"name":"Creem 40","productId":"prod_40","price":40,"quota":8000,"currency":"USD"}]`)
	common.OptionMap["QuotaForNewUser"] = "1000000"
	common.OptionMap["QuotaForInviter"] = "500000"
	common.OptionMap["QuotaForInvitee"] = "250000"
	common.OptionMap["checkin_setting"] = `{"enabled":true,"min_quota":5000,"max_quota":10000}`
	common.OptionMap["KyrenTopUpProducts"] = `[{"id":"topup_40","name":"40 CNY","amount":"40.00","currency":"CNY","quota":20000000,"enabled":true}]`
	common.OptionMap["CreemProducts"] = `[{"productId":"prod_40","quota":20000000}]`
	common.QuotaForNewUser = 1000000
	common.QuotaForInviter = 500000
	common.QuotaForInvitee = 250000
	operation_setting.GetCheckinSetting().MinQuota = 5000
	operation_setting.GetCheckinSetting().MaxQuota = 10000
	setting.KyrenTopUpProducts = common.OptionMap["KyrenTopUpProducts"]
	setting.CreemProducts = common.OptionMap["CreemProducts"]

	require.NoError(t, EnsureAccountBalanceCentsMigration())

	assert.Equal(t, 400, common.QuotaForNewUser)
	assert.Equal(t, 200, common.QuotaForInviter)
	assert.Equal(t, 100, common.QuotaForInvitee)
	assert.Equal(t, 2, operation_setting.GetCheckinSetting().MinQuota)
	assert.Equal(t, 4, operation_setting.GetCheckinSetting().MaxQuota)
	assert.Equal(t, "400", common.OptionMap["QuotaForNewUser"])
	assert.Equal(t, "200", common.OptionMap["QuotaForInviter"])
	assert.Equal(t, "100", common.OptionMap["QuotaForInvitee"])
	assert.Contains(t, common.OptionMap["checkin_setting"], `"min_quota":2`)
	assert.Contains(t, common.OptionMap["checkin_setting"], `"max_quota":4`)
	assert.Contains(t, common.OptionMap["KyrenTopUpProducts"], `"quota":8000`)
	assert.Contains(t, common.OptionMap["CreemProducts"], `"quota":8000`)
	assert.Contains(t, setting.KyrenTopUpProducts, `"quota":8000`)
	assert.Contains(t, setting.CreemProducts, `"quota":8000`)
	assert.Equal(t, "true", getOptionValueForMigrationTest(t, OptionAccountBalanceCentsMigrated))
}

func TestEnsureAccountBalanceCentsMigrationSkipsMissingRuntimeOptions(t *testing.T) {
	setupAccountBalanceMigrationTestDB(t)
	setOptionForMigrationTest(t, "QuotaPerUnit", "250000")
	common.OptionMap["QuotaForNewUser"] = "1000000"
	common.OptionMap["checkin_setting"] = `{"enabled":true,"min_quota":5000,"max_quota":10000}`
	common.QuotaForNewUser = 1000000
	operation_setting.GetCheckinSetting().MinQuota = 5000
	operation_setting.GetCheckinSetting().MaxQuota = 10000

	require.NoError(t, EnsureAccountBalanceCentsMigration())

	assert.Empty(t, getOptionValueForMigrationTest(t, "QuotaForNewUser"))
	assert.Empty(t, getOptionValueForMigrationTest(t, "checkin_setting"))
	assert.Equal(t, "1000000", common.OptionMap["QuotaForNewUser"])
	assert.Equal(t, 1000000, common.QuotaForNewUser)
	assert.Equal(t, 5000, operation_setting.GetCheckinSetting().MinQuota)
	assert.Equal(t, 10000, operation_setting.GetCheckinSetting().MaxQuota)
}

func TestEnsureAccountBalanceCentsMigrationDataStageRetrySkipsQuotaPerUnitValidation(t *testing.T) {
	setupAccountBalanceMigrationTestDB(t)
	setOptionForMigrationTest(t, OptionAccountBalanceCentsDataMigrated, "true")
	setOptionForMigrationTest(t, "QuotaPerUnit", "0")
	setOptionForMigrationTest(t, "QuotaForNewUser", "400")
	common.QuotaForNewUser = 1000000

	require.NoError(t, EnsureAccountBalanceCentsMigration())

	assert.Equal(t, 400, common.QuotaForNewUser)
	assert.Equal(t, "true", getOptionValueForMigrationTest(t, OptionAccountBalanceCentsMigrated))
}

func TestEnsureAccountBalanceCentsMigrationDataStageRetryDoesNotFinalizeWhenRuntimeSyncFails(t *testing.T) {
	setupAccountBalanceMigrationTestDB(t)
	setOptionForMigrationTest(t, OptionAccountBalanceCentsDataMigrated, "true")
	setOptionForMigrationTest(t, "CreemProducts", `[{"productId":"prod_40","quota":8000}]`)
	setOptionForMigrationTest(t, "KyrenTopUpProducts", `not-json`)
	oldCreemProducts := setting.CreemProducts
	oldCreemOption := common.OptionMap["CreemProducts"]

	err := EnsureAccountBalanceCentsMigration()

	require.Error(t, err)
	assert.Empty(t, getOptionValueForMigrationTest(t, OptionAccountBalanceCentsMigrated))
	assert.Empty(t, getOptionValueForMigrationTest(t, OptionAccountBalanceCentsMigratedAt))
	assert.Equal(t, oldCreemProducts, setting.CreemProducts)
	assert.Equal(t, oldCreemOption, common.OptionMap["CreemProducts"])
}

func TestEnsureAccountBalanceCentsMigrationRejectsPendingUserQuotaBatch(t *testing.T) {
	setupAccountBalanceMigrationTestDB(t)
	setOptionForMigrationTest(t, "QuotaPerUnit", "250000")
	addNewRecord(BatchUpdateTypeUserQuota, 9220, 500000)

	err := EnsureAccountBalanceCentsMigration()

	require.Error(t, err)
	assert.Empty(t, getOptionValueForMigrationTest(t, OptionAccountBalanceCentsDataMigrated))
	clearBatchUpdateTypeForMigrationTest(t, BatchUpdateTypeUserQuota)
}

func TestFlushBatchUpdateTypeForMigrationKeepsPendingOnFailure(t *testing.T) {
	setupAccountBalanceMigrationTestDB(t)
	addNewRecord(BatchUpdateTypeUserQuota, 9299, 500)

	err := FlushBatchUpdateTypeForMigration(BatchUpdateTypeUserQuota)

	require.Error(t, err)
	assert.Equal(t, 1, BatchUpdatePendingCount(BatchUpdateTypeUserQuota))
	require.NoError(t, DB.Create(&User{Id: 9299, Username: "flush-retry", Status: common.UserStatusEnabled}).Error)
	require.NoError(t, FlushBatchUpdateTypeForMigration(BatchUpdateTypeUserQuota))
	assert.Equal(t, 0, BatchUpdatePendingCount(BatchUpdateTypeUserQuota))
	assert.Equal(t, 500, getUserQuotaForAccountBalanceTest(t, 9299))
}

func TestFlushBatchUpdateTypeForMigrationPreservesConcurrentDelta(t *testing.T) {
	setupAccountBalanceMigrationTestDB(t)
	require.NoError(t, DB.Create(&User{Id: 9300, Username: "flush-race", Status: common.UserStatusEnabled}).Error)
	addNewRecord(BatchUpdateTypeUserQuota, 9300, 500)
	setMigrationFlushAfterSwapHookForTest(func() {
		addNewRecord(BatchUpdateTypeUserQuota, 9300, 700)
	})
	require.NoError(t, FlushBatchUpdateTypeForMigration(BatchUpdateTypeUserQuota))
	assert.Equal(t, 700, pendingBatchDeltaForMigrationTest(BatchUpdateTypeUserQuota, 9300))
	assert.Equal(t, 500, getUserQuotaForAccountBalanceTest(t, 9300))
	setMigrationFlushAfterSwapHookForTest(nil)
	require.NoError(t, FlushBatchUpdateTypeForMigration(BatchUpdateTypeUserQuota))
	assert.Equal(t, 0, BatchUpdatePendingCount(BatchUpdateTypeUserQuota))
	assert.Equal(t, 1200, getUserQuotaForAccountBalanceTest(t, 9300))
}

func TestFlushBatchUpdateTypeForMigrationSkipsZeroDelta(t *testing.T) {
	setupAccountBalanceMigrationTestDB(t)
	require.NoError(t, DB.Create(&User{Id: 9301, Username: "flush-zero", Quota: 900, Status: common.UserStatusEnabled}).Error)
	addNewRecord(BatchUpdateTypeUserQuota, 9301, 700)
	addNewRecord(BatchUpdateTypeUserQuota, 9301, -700)

	require.NoError(t, FlushBatchUpdateTypeForMigration(BatchUpdateTypeUserQuota))

	assert.Equal(t, 0, BatchUpdatePendingCount(BatchUpdateTypeUserQuota))
	assert.Equal(t, 900, getUserQuotaForAccountBalanceTest(t, 9301))
}

func TestEnsureAccountBalanceCentsMigrationFailsWhenFinalOptionWriteFails(t *testing.T) {
	setupAccountBalanceMigrationTestDB(t)
	setOptionForMigrationTest(t, "QuotaPerUnit", "250000")
	setOptionForMigrationTest(t, OptionAccountBalanceCentsDataMigrated, "true")
	closeAccountBalanceMigrationDBForTest(t)

	err := EnsureAccountBalanceCentsMigration()

	require.Error(t, err)
	assert.NotEqual(t, "true", common.OptionMap[OptionAccountBalanceCentsMigrated])
}

func TestEnsureAccountBalanceCentsMigrationFailsWhenUserCacheClearFails(t *testing.T) {
	setupAccountBalanceMigrationTestDB(t)
	setOptionForMigrationTest(t, "QuotaPerUnit", "250000")
	require.NoError(t, DB.Create(&User{Id: 9240, Username: "cache-fail", Quota: 1000000, Status: common.UserStatusEnabled}).Error)
	forceInvalidateAllUserCacheErrorForMigrationTest(errors.New("redis delete failed"))

	err := EnsureAccountBalanceCentsMigration()

	require.Error(t, err)
	assert.Equal(t, "true", getOptionValueForMigrationTest(t, OptionAccountBalanceCentsDataMigrated))
	assert.NotEqual(t, "true", getOptionValueForMigrationTest(t, OptionAccountBalanceCentsMigrated))
	assert.Empty(t, getOptionValueForMigrationTest(t, OptionAccountBalanceCentsMigratedAt))
	forceInvalidateAllUserCacheErrorForMigrationTest(nil)
	require.NoError(t, EnsureAccountBalanceCentsMigration())
	assert.Equal(t, 400, getUserQuotaForAccountBalanceTest(t, 9240))
}

func TestEnsureAccountBalanceCentsMigrationLogsStructuredStats(t *testing.T) {
	setupAccountBalanceMigrationTestDB(t)
	setOptionForMigrationTest(t, "QuotaPerUnit", "250000")
	require.NoError(t, DB.Create(&Checkin{UserId: 9241, CheckinDate: "2026-05-31", QuotaAwarded: 1}).Error)

	logs := captureAccountBalanceMigrationLogs(t, func() {
		require.NoError(t, EnsureAccountBalanceCentsMigration())
	})

	assert.Contains(t, logs, `"quota_per_unit":"250000"`)
	assert.Contains(t, logs, `"quota_per_unit_source":"db_option"`)
	assert.Contains(t, logs, `"checkin_rounded_to_zero":1`)
	assert.Contains(t, logs, `"skipped_successful_top_ups":0`)
	assert.Contains(t, logs, `"data_marker_written":true`)
	assert.Contains(t, logs, `"final_marker_written":true`)
	assert.Contains(t, logs, `"migrated_at_written":true`)
	assert.Contains(t, logs, `"user_cache_clear_mode":"redis_disabled"`)
	assert.Contains(t, logs, `"user_cache_clear_skip_reason":"redis_disabled"`)
}

func TestEnsureAccountBalanceCentsMigrationRollbackDoesNotChangeRuntimeOptions(t *testing.T) {
	setupAccountBalanceMigrationTestDB(t)
	setOptionForMigrationTest(t, "QuotaPerUnit", "250000")
	seedRuntimeBalanceOptionsForMigrationTest(t, map[string]string{"KyrenTopUpProducts": `[{"id":"topup_40","name":"40 CNY","amount":"40.00","currency":"CNY","quota":1000000,"enabled":true}]`})
	oldRuntime := setting.KyrenTopUpProducts
	forceAccountBalanceDataMigrationErrorForTest(errors.New("stop before commit"))

	err := EnsureAccountBalanceCentsMigration()

	require.Error(t, err)
	assert.Empty(t, getOptionValueForMigrationTest(t, OptionAccountBalanceCentsDataMigrated))
	assert.Equal(t, oldRuntime, setting.KyrenTopUpProducts)
	assert.Equal(t, oldRuntime, common.OptionMap["KyrenTopUpProducts"])
}

func TestTopUpAmountUnitColumnAutoMigrateSQLite(t *testing.T) {
	setupAccountBalanceMigrationTestDB(t)
	require.NoError(t, DB.Migrator().DropTable(&TopUp{}))
	require.NoError(t, DB.Exec(`CREATE TABLE top_ups (
		id integer PRIMARY KEY,
		user_id integer,
		amount bigint,
		money real,
		trade_no varchar(255),
		payment_method varchar(50),
		payment_provider varchar(50) DEFAULT '',
		create_time bigint,
		complete_time bigint,
		status text,
		kyren_snapshot text
	)`).Error)
	require.NoError(t, DB.Exec(`INSERT INTO top_ups (id, user_id, amount, money, trade_no, payment_method, payment_provider, status) VALUES (9401, 9402, 4000, 40, 'legacy-topup', 'alipay', 'epay', 'success')`).Error)

	require.NoError(t, ensureTopUpAmountUnitColumnSQLite())

	require.True(t, DB.Migrator().HasColumn(&TopUp{}, "amount_unit"))
	assert.Equal(t, "", getTopUpAmountUnitForMigrationTest(t, "legacy-topup"))
	require.NoError(t, DB.Create(&TopUp{Id: 9403, UserId: 9402, Amount: 4000, AmountUnit: TopUpAmountUnitAccountBalanceCents, TradeNo: "new-topup", Status: common.TopUpStatusPending}).Error)
	assert.Equal(t, TopUpAmountUnitAccountBalanceCents, getTopUpAmountUnitForMigrationTest(t, "new-topup"))
}

type nonAccountQuotaSeed struct {
	LogQuota                          int
	TokenRemainQuota                  int
	TokenUsedQuota                    int
	ChannelUsedQuota                  int64
	AbilityQuota                      int
	UserSubscriptionTokenLimit        int64
	UserSubscriptionTokenUsed         int64
	SubscriptionPlanMonthlyTokenLimit int64
	TopUpMoney                        float64
}

type migrationAbilityQuota struct {
	Group     string `gorm:"column:group"`
	Model     string `gorm:"column:model"`
	ChannelID int    `gorm:"column:channel_id"`
	Quota     int    `gorm:"column:quota"`
}

func (migrationAbilityQuota) TableName() string { return "abilities" }

func setupAccountBalanceMigrationTestDB(t *testing.T) {
	t.Helper()

	oldDB := DB
	oldLOGDB := LOG_DB
	oldUsingSQLite := common.UsingSQLite
	oldUsingMySQL := common.UsingMySQL
	oldUsingPostgreSQL := common.UsingPostgreSQL
	oldLogSQLType := common.LogSqlType
	oldRedisEnabled := common.RedisEnabled
	oldRDB := common.RDB
	oldOptionMap := common.OptionMap
	oldQuotaPerUnit := common.QuotaPerUnit
	oldQuotaForNewUser := common.QuotaForNewUser
	oldQuotaForInviter := common.QuotaForInviter
	oldQuotaForInvitee := common.QuotaForInvitee
	oldKyrenTopUpProducts := setting.KyrenTopUpProducts
	oldCreemProducts := setting.CreemProducts
	oldCheckinSetting := *operation_setting.GetCheckinSetting()
	oldInvalidateHook := accountBalanceMigrationInvalidateAllUserCachesHook
	oldBeforeDataTxHook := accountBalanceMigrationBeforeDataTxHook
	oldDataHook := accountBalanceDataMigrationBeforeCommitHook
	oldFlushHook := migrationFlushAfterSwapHookForTest

	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.LogSqlType = common.DatabaseTypeSQLite
	common.RedisEnabled = false
	common.RDB = nil
	common.OptionMap = map[string]string{}
	common.QuotaPerUnit = 500000
	common.QuotaForNewUser = 0
	common.QuotaForInviter = 0
	common.QuotaForInvitee = 0
	setting.KyrenTopUpProducts = "[]"
	setting.CreemProducts = "[]"
	*operation_setting.GetCheckinSetting() = operation_setting.CheckinSetting{}
	accountBalanceMigrationInvalidateAllUserCachesHook = nil
	accountBalanceMigrationBeforeDataTxHook = nil
	accountBalanceDataMigrationBeforeCommitHook = nil
	migrationFlushAfterSwapHookForTest = nil
	initCol()
	clearAllBatchUpdatesForMigrationTest()

	safeName := strings.NewReplacer("/", "_", " ", "_", ":", "_").Replace(t.Name())
	db, err := gorm.Open(sqlite.Open("file:"+safeName+"_account_balance_migration?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	DB = db
	LOG_DB = db
	require.NoError(t, DB.AutoMigrate(&User{}, &Option{}, &Redemption{}, &Checkin{}, &TopUp{}, &Log{}, &Token{}, &Channel{}, &Ability{}, &UserSubscription{}, &SubscriptionPlan{}))
	require.NoError(t, DB.Exec("ALTER TABLE abilities ADD COLUMN quota integer DEFAULT 0").Error)

	t.Cleanup(func() {
		clearAllBatchUpdatesForMigrationTest()
		DB = oldDB
		LOG_DB = oldLOGDB
		common.UsingSQLite = oldUsingSQLite
		common.UsingMySQL = oldUsingMySQL
		common.UsingPostgreSQL = oldUsingPostgreSQL
		common.LogSqlType = oldLogSQLType
		common.RedisEnabled = oldRedisEnabled
		common.RDB = oldRDB
		common.OptionMap = oldOptionMap
		common.QuotaPerUnit = oldQuotaPerUnit
		common.QuotaForNewUser = oldQuotaForNewUser
		common.QuotaForInviter = oldQuotaForInviter
		common.QuotaForInvitee = oldQuotaForInvitee
		setting.KyrenTopUpProducts = oldKyrenTopUpProducts
		setting.CreemProducts = oldCreemProducts
		*operation_setting.GetCheckinSetting() = oldCheckinSetting
		accountBalanceMigrationInvalidateAllUserCachesHook = oldInvalidateHook
		accountBalanceMigrationBeforeDataTxHook = oldBeforeDataTxHook
		accountBalanceDataMigrationBeforeCommitHook = oldDataHook
		migrationFlushAfterSwapHookForTest = oldFlushHook
		initCol()
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
}

func seedUserCacheForMigrationTest(t *testing.T, userCache *UserBase) {
	t.Helper()
	require.NotNil(t, userCache)

	oldRedisEnabled := common.RedisEnabled
	oldRDB := common.RDB
	server, err := miniredis.Run()
	require.NoError(t, err)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	common.RedisEnabled = true
	common.RDB = client
	t.Cleanup(func() {
		common.RedisEnabled = oldRedisEnabled
		common.RDB = oldRDB
		_ = client.Close()
		server.Close()
	})

	require.NoError(t, updateUserCache(User{
		Id:       userCache.Id,
		Username: userCache.Username,
		Email:    userCache.Email,
		Quota:    userCache.Quota,
		Status:   userCache.Status,
		Setting:  userCache.Setting,
	}))
}

func setOptionForMigrationTest(t *testing.T, key string, value string) {
	t.Helper()
	require.NoError(t, DB.Save(&Option{Key: key, Value: value}).Error)
	common.OptionMap[key] = value
}

func seedRuntimeBalanceOptionsForMigrationTest(t *testing.T, values map[string]string) {
	t.Helper()
	for key, value := range values {
		setOptionForMigrationTest(t, key, value)
		require.NoError(t, updateOptionMap(key, value))
	}
}

func getUserAffQuotaForMigrationTest(t *testing.T, userId int) int {
	t.Helper()
	var user User
	require.NoError(t, DB.Select("aff_quota").First(&user, userId).Error)
	return user.AffQuota
}

func getUserQuotaUnscopedForMigrationTest(t *testing.T, userId int) int {
	t.Helper()
	var user User
	require.NoError(t, DB.Unscoped().Select("quota").First(&user, userId).Error)
	return user.Quota
}

func getUserAffQuotaUnscopedForMigrationTest(t *testing.T, userId int) int {
	t.Helper()
	var user User
	require.NoError(t, DB.Unscoped().Select("aff_quota").First(&user, userId).Error)
	return user.AffQuota
}

func getUserAffHistoryUnscopedForMigrationTest(t *testing.T, userId int) int {
	t.Helper()
	var user User
	require.NoError(t, DB.Unscoped().Select("aff_history").First(&user, userId).Error)
	return user.AffHistoryQuota
}
func getUserAffHistoryForMigrationTest(t *testing.T, userId int) int {
	t.Helper()
	var user User
	require.NoError(t, DB.Select("aff_history").First(&user, userId).Error)
	return user.AffHistoryQuota
}
func getRedemptionQuotaUnscopedForMigrationTest(t *testing.T, id int) int {
	t.Helper()
	var redemption Redemption
	require.NoError(t, DB.Unscoped().Select("quota").First(&redemption, id).Error)
	return redemption.Quota
}

func getRedemptionQuotaForMigrationTest(t *testing.T, id int) int {
	t.Helper()
	var redemption Redemption
	require.NoError(t, DB.Select("quota").First(&redemption, id).Error)
	return redemption.Quota
}

func getRedemptionTypeForMigrationTest(t *testing.T, id int) string {
	t.Helper()
	var redemption Redemption
	require.NoError(t, DB.Select("type").First(&redemption, id).Error)
	return redemption.Type
}

func getCheckinQuotaForMigrationTest(t *testing.T, userId int, date string) int {
	t.Helper()
	var checkin Checkin
	require.NoError(t, DB.Select("quota_awarded").Where("user_id = ? AND checkin_date = ?", userId, date).First(&checkin).Error)
	return checkin.QuotaAwarded
}

func getTopUpStatusForMigrationTest(t *testing.T, tradeNo string) string {
	t.Helper()
	var topUp TopUp
	require.NoError(t, DB.Select("status").Where("trade_no = ?", tradeNo).First(&topUp).Error)
	return topUp.Status
}

func getTopUpAmountForMigrationTest(t *testing.T, tradeNo string) int64 {
	t.Helper()
	var topUp TopUp
	require.NoError(t, DB.Select("amount").Where("trade_no = ?", tradeNo).First(&topUp).Error)
	return topUp.Amount
}

func getTopUpAmountUnitForMigrationTest(t *testing.T, tradeNo string) string {
	t.Helper()
	var topUp TopUp
	require.NoError(t, DB.Select("amount_unit").Where("trade_no = ?", tradeNo).First(&topUp).Error)
	return topUp.AmountUnit
}

func getKyrenTopUpProductQuotaForMigrationTest(t *testing.T, id string) int64 {
	t.Helper()
	var products []setting.KyrenTopUpProduct
	require.NoError(t, common.UnmarshalJsonStr(common.OptionMap["KyrenTopUpProducts"], &products))
	for _, product := range products {
		if product.ID == id {
			return product.Quota
		}
	}
	t.Fatalf("Kyren product %s not found", id)
	return 0
}

func getCreemProductQuotaForMigrationTest(t *testing.T, productID string) int64 {
	t.Helper()
	var products []struct {
		ProductID string `json:"productId"`
		Quota     int64  `json:"quota"`
	}
	require.NoError(t, common.UnmarshalJsonStr(common.OptionMap["CreemProducts"], &products))
	for _, product := range products {
		if product.ProductID == productID {
			return product.Quota
		}
	}
	t.Fatalf("Creem product %s not found", productID)
	return 0
}

func getOptionValueForMigrationTest(t *testing.T, key string) string {
	t.Helper()
	var option Option
	err := DB.Select("value").Where("key = ?", key).First(&option).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ""
	}
	require.NoError(t, err)
	return option.Value
}

func seedNonAccountQuotaFieldsForMigrationTest(t *testing.T, seed nonAccountQuotaSeed) {
	t.Helper()
	require.NoError(t, DB.Create(&Log{Id: 9311, Quota: seed.LogQuota}).Error)
	require.NoError(t, DB.Create(&Token{Id: 9312, UserId: 9312, Key: "non-account-token", RemainQuota: seed.TokenRemainQuota, UsedQuota: seed.TokenUsedQuota}).Error)
	require.NoError(t, DB.Create(&Channel{Id: 9313, Type: 1, Key: "sk-non-account", Name: "non-account", UsedQuota: seed.ChannelUsedQuota}).Error)
	require.NoError(t, DB.Table("abilities").Create(&migrationAbilityQuota{Group: "default", Model: "quota-model", ChannelID: 9314, Quota: seed.AbilityQuota}).Error)
	require.NoError(t, DB.Create(&UserSubscription{Id: 9315, UserId: 9315, TokenLimit: seed.UserSubscriptionTokenLimit, TokenUsed: seed.UserSubscriptionTokenUsed}).Error)
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: 9316, Title: "non-account-plan", MonthlyTokenLimit: seed.SubscriptionPlanMonthlyTokenLimit}).Error)
	require.NoError(t, DB.Create(&TopUp{Id: 9317, UserId: 9317, Amount: 1000000, Money: seed.TopUpMoney, TradeNo: "non-account-success", Status: common.TopUpStatusSuccess}).Error)
}

func assertNonAccountQuotaFieldsUnchanged(t *testing.T, want nonAccountQuotaSeed) {
	t.Helper()
	var log Log
	require.NoError(t, DB.Select("quota").First(&log, 9311).Error)
	assert.Equal(t, want.LogQuota, log.Quota)
	var token Token
	require.NoError(t, DB.Select("remain_quota", "used_quota").First(&token, 9312).Error)
	assert.Equal(t, want.TokenRemainQuota, token.RemainQuota)
	assert.Equal(t, want.TokenUsedQuota, token.UsedQuota)
	var channel Channel
	require.NoError(t, DB.Select("used_quota").First(&channel, 9313).Error)
	assert.Equal(t, want.ChannelUsedQuota, channel.UsedQuota)
	var ability migrationAbilityQuota
	require.NoError(t, DB.Table("abilities").Where("channel_id = ?", 9314).First(&ability).Error)
	assert.Equal(t, want.AbilityQuota, ability.Quota)
	var subscription UserSubscription
	require.NoError(t, DB.Select("token_limit", "token_used").First(&subscription, 9315).Error)
	assert.Equal(t, want.UserSubscriptionTokenLimit, subscription.TokenLimit)
	assert.Equal(t, want.UserSubscriptionTokenUsed, subscription.TokenUsed)
	var plan SubscriptionPlan
	require.NoError(t, DB.Select("monthly_token_limit").First(&plan, 9316).Error)
	assert.Equal(t, want.SubscriptionPlanMonthlyTokenLimit, plan.MonthlyTokenLimit)
	var topUp TopUp
	require.NoError(t, DB.Select("money", "amount").Where("trade_no = ?", "non-account-success").First(&topUp).Error)
	assert.Equal(t, want.TopUpMoney, topUp.Money)
	assert.EqualValues(t, 1000000, topUp.Amount)
}

func captureAccountBalanceMigrationLogs(t *testing.T, fn func()) string {
	t.Helper()
	oldWriter := gin.DefaultWriter
	var buf bytes.Buffer
	common.LogWriterMu.Lock()
	gin.DefaultWriter = io.MultiWriter(oldWriter, &buf)
	common.LogWriterMu.Unlock()
	t.Cleanup(func() {
		common.LogWriterMu.Lock()
		gin.DefaultWriter = oldWriter
		common.LogWriterMu.Unlock()
	})
	fn()
	return buf.String()
}

func clearBatchUpdateTypeForMigrationTest(t *testing.T, type_ int) {
	t.Helper()
	batchUpdateLocks[type_].Lock()
	batchUpdateStores[type_] = make(map[int]int)
	batchUpdateLocks[type_].Unlock()
}

func clearAllBatchUpdatesForMigrationTest() {
	for i := 0; i < BatchUpdateTypeCount; i++ {
		batchUpdateLocks[i].Lock()
		batchUpdateStores[i] = make(map[int]int)
		batchUpdateLocks[i].Unlock()
	}
}

func setMigrationFlushAfterSwapHookForTest(hook func()) {
	migrationFlushAfterSwapHookForTest = hook
}

func pendingBatchDeltaForMigrationTest(type_ int, id int) int {
	batchUpdateLocks[type_].Lock()
	defer batchUpdateLocks[type_].Unlock()
	return batchUpdateStores[type_][id]
}

func closeAccountBalanceMigrationDBForTest(t *testing.T) {
	t.Helper()
	sqlDB, err := DB.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
}

func forceInvalidateAllUserCacheErrorForMigrationTest(err error) {
	if err == nil {
		accountBalanceMigrationInvalidateAllUserCachesHook = nil
		return
	}
	accountBalanceMigrationInvalidateAllUserCachesHook = func() error { return err }
}

func forceAccountBalanceDataMigrationErrorForTest(err error) {
	if err == nil {
		accountBalanceDataMigrationBeforeCommitHook = nil
		return
	}
	accountBalanceDataMigrationBeforeCommitHook = func() error { return err }
}
