package model

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestTopUpHistoryReturnsStableCreditedBalanceFields(t *testing.T) {
	setupTopUpHistoryCentsTestDB(t)
	setTopUpHistoryOptionForTest(t, OptionAccountBalanceCentsMigratedAt, "2000")
	now := common.GetTimestamp()
	require.NoError(t, DB.Create(&TopUp{UserId: 9301, Amount: 40, Money: 40, TradeNo: "legacy-epay", PaymentProvider: PaymentProviderEpay, PaymentMethod: "alipay", Status: common.TopUpStatusSuccess, CreateTime: now}).Error)
	require.NoError(t, DB.Create(&TopUp{UserId: 9301, Amount: 4000, Money: 40, TradeNo: "new-epay", PaymentProvider: PaymentProviderEpay, PaymentMethod: "alipay", Status: common.TopUpStatusSuccess, CreateTime: now - 1, AmountUnit: TopUpAmountUnitAccountBalanceCents}).Error)

	items, total, err := GetUserTopUpHistoryItems(9301, topUpHistoryPageForTest(10))
	require.NoError(t, err)
	assert.EqualValues(t, 2, total)
	legacy := findHistoryItemForTest(t, items, "legacy-epay")
	newer := findHistoryItemForTest(t, items, "new-epay")
	assert.EqualValues(t, 4000, legacy.CreditedBalanceCents)
	assert.Equal(t, "legacy", legacy.AmountUnit)
	assert.False(t, legacy.IsAccountBalanceCents)
	assert.Equal(t, "¥40.00", legacy.CreditedBalanceDisplay)
	assert.EqualValues(t, 4000, newer.CreditedBalanceCents)
	assert.Equal(t, TopUpAmountUnitAccountBalanceCents, newer.AmountUnit)
	assert.True(t, newer.IsAccountBalanceCents)
	assert.Equal(t, "¥40.00", newer.CreditedBalanceDisplay)
}

func TestTopUpHistoryDoesNotInferLegacyFromMigrationTimestamp(t *testing.T) {
	setupTopUpHistoryCentsTestDB(t)
	setTopUpHistoryOptionForTest(t, OptionAccountBalanceCentsMigratedAt, "1")
	recent := common.GetTimestamp()
	require.NoError(t, DB.Create(&TopUp{UserId: 9306, Amount: 20000000, Money: 40, TradeNo: "recent-legacy-kyren", PaymentProvider: PaymentProviderKyren, PaymentMethod: PaymentMethodKyren, Status: common.TopUpStatusSuccess, CreateTime: recent}).Error)

	items, _, err := GetUserTopUpHistoryItems(9306, topUpHistoryPageForTest(10))
	require.NoError(t, err)
	item := findHistoryItemForTest(t, items, "recent-legacy-kyren")
	assert.Equal(t, "legacy", item.AmountUnit)
	assert.False(t, item.IsAccountBalanceCents)
	assert.Zero(t, item.CreditedBalanceCents)
	assert.Contains(t, item.CreditedBalanceDisplay, "legacy/raw amount")
}

func TestTopUpHistoryKyrenAndCreemSnapshotFallbacks(t *testing.T) {
	setupTopUpHistoryCentsTestDB(t)
	setTopUpHistoryOptionForTest(t, OptionAccountBalanceCentsMigratedAt, "2000")
	now := common.GetTimestamp()
	kyrenSnapshot := `{"local_topup_id":"kyren_40","product_id":"prod_kyren_40","amount":"40.00","currency":"CNY","quota":4000}`
	require.NoError(t, DB.Create(&TopUp{UserId: 9302, Amount: 20000000, Money: 40, TradeNo: "legacy-kyren-snapshot", PaymentProvider: PaymentProviderKyren, PaymentMethod: PaymentMethodKyren, Status: common.TopUpStatusSuccess, CreateTime: now, KyrenSnapshot: kyrenSnapshot}).Error)
	require.NoError(t, DB.Create(&TopUp{UserId: 9302, Amount: 20000000, Money: 40, TradeNo: "legacy-kyren-raw", PaymentProvider: PaymentProviderKyren, PaymentMethod: PaymentMethodKyren, Status: common.TopUpStatusSuccess, CreateTime: now}).Error)
	setTopUpHistoryOptionForTest(t, "CreemProducts", `[{"productId":"prod_creem_40","name":"Creem 40","price":40,"currency":"USD","quota":4000}]`)
	require.NoError(t, DB.Create(&TopUp{UserId: 9302, Amount: 20000000, Money: 40, TradeNo: "legacy-creem-product", PaymentProvider: PaymentProviderCreem, PaymentMethod: PaymentMethodCreem, Status: common.TopUpStatusSuccess, CreateTime: now}).Error)
	require.NoError(t, DB.Create(&TopUp{UserId: 9302, Amount: 12345678, Money: 13, TradeNo: "legacy-creem-raw", PaymentProvider: PaymentProviderCreem, PaymentMethod: PaymentMethodCreem, Status: common.TopUpStatusSuccess, CreateTime: now}).Error)

	items, _, err := GetUserTopUpHistoryItems(9302, topUpHistoryPageForTest(20))
	require.NoError(t, err)
	assert.EqualValues(t, 4000, findHistoryItemForTest(t, items, "legacy-kyren-snapshot").CreditedBalanceCents)
	kyrenRaw := findHistoryItemForTest(t, items, "legacy-kyren-raw")
	assert.Equal(t, "legacy", kyrenRaw.AmountUnit)
	assert.Zero(t, kyrenRaw.CreditedBalanceCents)
	assert.Contains(t, kyrenRaw.CreditedBalanceDisplay, "legacy/raw amount")
	assert.Contains(t, findHistoryItemForTest(t, items, "legacy-creem-product").CreditedBalanceDisplay, "legacy/raw amount")
	creemRaw := findHistoryItemForTest(t, items, "legacy-creem-raw")
	assert.Equal(t, "legacy", creemRaw.AmountUnit)
	assert.Zero(t, creemRaw.CreditedBalanceCents)
	assert.Contains(t, creemRaw.CreditedBalanceDisplay, "legacy/raw amount")
}

func TestTopUpHistoryCreemLegacyDoesNotUseMutableProductConfig(t *testing.T) {
	setupTopUpHistoryCentsTestDB(t)
	setTopUpHistoryOptionForTest(t, "CreemProducts", `[{"productId":"prod_creem_40","name":"Creem 40","price":40,"currency":"USD","quota":4000}]`)
	require.NoError(t, DB.Create(&TopUp{UserId: 9308, Amount: 20000000, Money: 40, TradeNo: "legacy-creem-mutable-config", PaymentProvider: PaymentProviderCreem, PaymentMethod: PaymentMethodCreem, Status: common.TopUpStatusSuccess, CreateTime: common.GetTimestamp()}).Error)

	items, _, err := GetUserTopUpHistoryItems(9308, topUpHistoryPageForTest(20))
	require.NoError(t, err)
	item := findHistoryItemForTest(t, items, "legacy-creem-mutable-config")

	assert.Equal(t, "legacy", item.AmountUnit)
	assert.Zero(t, item.CreditedBalanceCents)
	assert.Contains(t, item.CreditedBalanceDisplay, "legacy/raw amount")
}

func TestTopUpHistoryKyrenLegacySnapshotConvertsOldQuotaWithMigrationRate(t *testing.T) {
	setupTopUpHistoryCentsTestDB(t)
	setTopUpHistoryOptionForTest(t, "QuotaPerUnit", "500000")
	kyrenSnapshot := `{"local_topup_id":"kyren_40","product_id":"prod_kyren_40","amount":"40.00","currency":"CNY","quota":20000000}`
	require.NoError(t, DB.Create(&TopUp{UserId: 9304, Amount: 20000000, Money: 40, TradeNo: "legacy-kyren-old-quota-snapshot", PaymentProvider: PaymentProviderKyren, PaymentMethod: PaymentMethodKyren, Status: common.TopUpStatusSuccess, CreateTime: common.GetTimestamp(), KyrenSnapshot: kyrenSnapshot}).Error)

	items, _, err := GetUserTopUpHistoryItems(9304, topUpHistoryPageForTest(20))
	require.NoError(t, err)
	item := findHistoryItemForTest(t, items, "legacy-kyren-old-quota-snapshot")
	assert.EqualValues(t, 4000, item.CreditedBalanceCents)
	assert.Equal(t, "¥40.00", item.CreditedBalanceDisplay)
	assert.Equal(t, "legacy", item.AmountUnit)
}

func TestTopUpHistoryKyrenLegacySnapshotConvertsWhenOldQuotaNearAmountCents(t *testing.T) {
	setupTopUpHistoryCentsTestDB(t)
	setTopUpHistoryOptionForTest(t, "QuotaPerUnit", "1000")
	kyrenSnapshot := `{"local_topup_id":"kyren_40","product_id":"prod_kyren_40","amount":"40.00","currency":"CNY","quota":40000}`
	require.NoError(t, DB.Create(&TopUp{UserId: 9307, Amount: 40000, Money: 40, TradeNo: "legacy-kyren-near-cents", PaymentProvider: PaymentProviderKyren, PaymentMethod: PaymentMethodKyren, Status: common.TopUpStatusSuccess, CreateTime: common.GetTimestamp(), KyrenSnapshot: kyrenSnapshot}).Error)

	items, _, err := GetUserTopUpHistoryItems(9307, topUpHistoryPageForTest(20))
	require.NoError(t, err)
	item := findHistoryItemForTest(t, items, "legacy-kyren-near-cents")
	assert.EqualValues(t, 4000, item.CreditedBalanceCents)
	assert.Equal(t, "¥40.00", item.CreditedBalanceDisplay)
	assert.Equal(t, "legacy", item.AmountUnit)
}

func TestTopUpHistoryKyrenLegacySnapshotDowngradesWhenMigrationRateUnavailable(t *testing.T) {
	setupTopUpHistoryCentsTestDB(t)
	setTopUpHistoryOptionForTest(t, "QuotaPerUnit", "0")
	kyrenSnapshot := `{"local_topup_id":"kyren_40","product_id":"prod_kyren_40","amount":"40.00","currency":"CNY","quota":20000000}`
	require.NoError(t, DB.Create(&TopUp{UserId: 9305, Amount: 20000000, Money: 40, TradeNo: "legacy-kyren-old-quota-no-rate", PaymentProvider: PaymentProviderKyren, PaymentMethod: PaymentMethodKyren, Status: common.TopUpStatusSuccess, CreateTime: common.GetTimestamp(), KyrenSnapshot: kyrenSnapshot}).Error)

	items, _, err := GetUserTopUpHistoryItems(9305, topUpHistoryPageForTest(20))
	require.NoError(t, err)
	item := findHistoryItemForTest(t, items, "legacy-kyren-old-quota-no-rate")
	assert.Equal(t, "legacy", item.AmountUnit)
	assert.False(t, item.IsAccountBalanceCents)
	assert.Zero(t, item.CreditedBalanceCents)
	assert.Contains(t, item.CreditedBalanceDisplay, "legacy/raw amount")
}

func TestAdminAndSearchTopUpHistoryReturnStableCreditedBalanceFields(t *testing.T) {
	setupTopUpHistoryCentsTestDB(t)
	now := common.GetTimestamp()
	require.NoError(t, DB.Create(&TopUp{UserId: 9303, Amount: 40, Money: 40, TradeNo: "legacy-only-admin", PaymentProvider: PaymentProviderEpay, PaymentMethod: "alipay", Status: common.TopUpStatusSuccess, CreateTime: now}).Error)
	require.NoError(t, DB.Create(&TopUp{UserId: 9303, Amount: 4000, Money: 40, TradeNo: "fresh-cents-admin", PaymentProvider: PaymentProviderEpay, PaymentMethod: "alipay", Status: common.TopUpStatusSuccess, CreateTime: now, AmountUnit: TopUpAmountUnitAccountBalanceCents}).Error)

	allItems, _, err := GetAllTopUpHistoryItems(topUpHistoryPageForTest(10))
	require.NoError(t, err)
	searchedLegacyUserItems, _, err := SearchUserTopUpHistoryItems(9303, "%legacy%", topUpHistoryPageForTest(10))
	require.NoError(t, err)
	searchedNewUserItems, _, err := SearchUserTopUpHistoryItems(9303, "%fresh-cents%", topUpHistoryPageForTest(10))
	require.NoError(t, err)
	searchedLegacyAllItems, _, err := SearchAllTopUpHistoryItems("%legacy%", topUpHistoryPageForTest(10))
	require.NoError(t, err)
	searchedNewAllItems, _, err := SearchAllTopUpHistoryItems("%fresh-cents%", topUpHistoryPageForTest(10))
	require.NoError(t, err)

	for _, items := range [][]TopUpHistoryItem{allItems, searchedLegacyUserItems, searchedLegacyAllItems} {
		assert.EqualValues(t, 4000, findHistoryItemForTest(t, items, "legacy-only-admin").CreditedBalanceCents)
	}
	for _, items := range [][]TopUpHistoryItem{allItems, searchedNewUserItems, searchedNewAllItems} {
		newItem := findHistoryItemForTest(t, items, "fresh-cents-admin")
		assert.EqualValues(t, 4000, newItem.CreditedBalanceCents)
		assert.Equal(t, TopUpAmountUnitAccountBalanceCents, newItem.AmountUnit)
		assert.True(t, newItem.IsAccountBalanceCents)
	}
}

func setupTopUpHistoryCentsTestDB(t *testing.T) {
	t.Helper()
	oldDB := DB
	oldLOGDB := LOG_DB
	oldUsingSQLite := common.UsingSQLite
	oldUsingMySQL := common.UsingMySQL
	oldUsingPostgreSQL := common.UsingPostgreSQL
	oldLogSQLType := common.LogSqlType
	oldOptionMap := common.OptionMap
	oldQuotaPerUnit := common.QuotaPerUnit
	oldCreemProducts := setting.CreemProducts

	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.LogSqlType = common.DatabaseTypeSQLite
	common.OptionMap = map[string]string{}
	common.QuotaPerUnit = 500000
	setting.CreemProducts = "[]"
	initCol()

	safeName := strings.NewReplacer("/", "_", " ", "_", ":", "_").Replace(t.Name())
	db, err := gorm.Open(sqlite.Open("file:"+safeName+"_topup_history?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	DB = db
	LOG_DB = db
	require.NoError(t, DB.AutoMigrate(&TopUp{}, &Option{}))

	t.Cleanup(func() {
		DB = oldDB
		LOG_DB = oldLOGDB
		common.UsingSQLite = oldUsingSQLite
		common.UsingMySQL = oldUsingMySQL
		common.UsingPostgreSQL = oldUsingPostgreSQL
		common.LogSqlType = oldLogSQLType
		common.OptionMap = oldOptionMap
		common.QuotaPerUnit = oldQuotaPerUnit
		setting.CreemProducts = oldCreemProducts
		initCol()
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
}

func setTopUpHistoryOptionForTest(t *testing.T, key string, value string) {
	t.Helper()
	require.NoError(t, DB.Save(&Option{Key: key, Value: value}).Error)
	common.OptionMap[key] = value
	if key == "CreemProducts" {
		setting.CreemProducts = value
	}
}

func topUpHistoryPageForTest(pageSize int) *common.PageInfo {
	return &common.PageInfo{Page: 1, PageSize: pageSize}
}

func findHistoryItemForTest(t *testing.T, items []TopUpHistoryItem, tradeNo string) TopUpHistoryItem {
	t.Helper()
	for _, item := range items {
		if item.TradeNo == tradeNo {
			return item
		}
	}
	t.Fatalf("history item %s not found in %#v", tradeNo, items)
	return TopUpHistoryItem{}
}
