package model

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	OptionAccountBalanceCentsDataMigrated = "AccountBalanceCentsDataMigrated"
	OptionAccountBalanceCentsMigrated     = "AccountBalanceCentsMigrated"
	OptionAccountBalanceCentsMigratedAt   = "AccountBalanceCentsMigratedAt"
)

const accountBalanceMigrationQuotaPerUnitOption = "QuotaPerUnit"
const accountBalanceCentsDataMigrationInProgress = "running"

var errAccountBalanceDataMigrationAlreadyDone = errors.New("account balance data migration was completed by another process")

var accountBalanceMigrationInvalidateAllUserCachesHook func() error
var accountBalanceMigrationBeforeDataTxHook func() error
var accountBalanceDataMigrationBeforeCommitHook func() error

type accountBalanceMigrationStats struct {
	QuotaPerUnit             string `json:"quota_per_unit,omitempty"`
	QuotaPerUnitSource       string `json:"quota_per_unit_source,omitempty"`
	DataStageAlreadyMigrated bool   `json:"data_stage_already_migrated"`
	Users                    int64  `json:"users"`
	UserAffQuota             int64  `json:"user_aff_quota"`
	UserAffHistory           int64  `json:"user_aff_history"`
	WalletRedemptions        int64  `json:"wallet_redemptions"`
	BlankWalletRedemptions   int64  `json:"blank_wallet_redemptions"`
	Checkins                 int64  `json:"checkins"`
	CheckinRoundedToZero     int64  `json:"checkin_rounded_to_zero"`
	KyrenProducts            int64  `json:"kyren_products"`
	CreemProducts            int64  `json:"creem_products"`
	RuntimeOptions           int64  `json:"runtime_options"`
	PendingTopUpsExpired     int64  `json:"pending_top_ups_expired"`
	SkippedSuccessfulTopUps  int64  `json:"skipped_successful_top_ups"`
	DataMarkerWritten        bool   `json:"data_marker_written"`
	FinalMarkerWritten       bool   `json:"final_marker_written"`
	MigratedAtWritten        bool   `json:"migrated_at_written"`
	UserCacheClearMode       string `json:"user_cache_clear_mode"`
	UserCacheCleared         int64  `json:"user_cache_cleared"`
	UserCacheClearSkipReason string `json:"user_cache_clear_skip_reason,omitempty"`
	Error                    string `json:"error,omitempty"`
}

func EnsureAccountBalanceCentsMigration() error {
	finalMigrated, err := accountBalanceMigrationOptionTrue(OptionAccountBalanceCentsMigrated)
	if err != nil {
		return err
	}
	if finalMigrated {
		values, err := loadAccountBalanceFinalMarkerOptionValuesFromDB()
		if err != nil {
			return err
		}
		return syncMigratedOptionsRuntime(values)
	}
	if BatchUpdatePendingCount(BatchUpdateTypeUserQuota) > 0 {
		return errors.New("pending user quota batch updates must be flushed on every old instance before account balance migration")
	}

	stats := accountBalanceMigrationStats{UserCacheClearMode: accountBalanceMigrationUserCacheClearMode()}
	dataMarkerValue, dataMarkerExists, err := accountBalanceMigrationDBOptionValue(OptionAccountBalanceCentsDataMigrated)
	if err != nil {
		return err
	}
	if strings.EqualFold(strings.TrimSpace(dataMarkerValue), accountBalanceCentsDataMigrationInProgress) {
		return errors.New("account balance data migration is already in progress")
	}
	dataMigrated := dataMarkerExists && accountBalanceMigrationOptionValueIsTrue(dataMarkerValue)

	var runtimeValues map[string]string
	if dataMigrated {
		stats.DataStageAlreadyMigrated = true
		runtimeValues, err = loadAccountBalanceMigratedOptionValuesFromDB()
		if err != nil {
			stats.Error = err.Error()
			logAccountBalanceMigrationStats(stats)
			return err
		}
	} else {
		quotaPerUnit, quotaPerUnitSource, err := accountBalanceMigrationQuotaPerUnitForDataStage()
		if err != nil {
			return err
		}
		stats.QuotaPerUnit = quotaPerUnit.String()
		stats.QuotaPerUnitSource = quotaPerUnitSource
		if accountBalanceMigrationBeforeDataTxHook != nil {
			if err := accountBalanceMigrationBeforeDataTxHook(); err != nil {
				return err
			}
		}
		runtimeValues, stats, err = migrateAccountBalanceData(quotaPerUnit, quotaPerUnitSource, stats)
		if errors.Is(err, errAccountBalanceDataMigrationAlreadyDone) {
			stats.DataStageAlreadyMigrated = true
			runtimeValues, err = loadAccountBalanceMigratedOptionValuesFromDB()
		}
		if err != nil {
			return err
		}
	}

	if err = syncMigratedOptionsRuntime(runtimeValues); err != nil {
		stats.Error = err.Error()
		logAccountBalanceMigrationStats(stats)
		return err
	}
	cleared, skipReason, err := invalidateAllUserCachesForAccountBalanceMigration()
	stats.UserCacheCleared = cleared
	stats.UserCacheClearSkipReason = skipReason
	if err != nil {
		stats.Error = err.Error()
		logAccountBalanceMigrationStats(stats)
		return err
	}

	migratedAt := strconv.FormatInt(common.GetTimestamp(), 10)
	if err = writeAccountBalanceMigrationFinalMarkers(migratedAt); err != nil {
		stats.Error = err.Error()
		logAccountBalanceMigrationStats(stats)
		return err
	}
	if err = syncMigratedOptionsRuntime(map[string]string{
		OptionAccountBalanceCentsMigratedAt: migratedAt,
		OptionAccountBalanceCentsMigrated:   "true",
	}); err != nil {
		stats.Error = err.Error()
		logAccountBalanceMigrationStats(stats)
		return err
	}
	stats.MigratedAtWritten = true
	stats.FinalMarkerWritten = true
	logAccountBalanceMigrationStats(stats)
	return nil
}

func lockAccountBalanceDataMigrationMarkerTx(tx *gorm.DB) (bool, error) {
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&Option{Key: OptionAccountBalanceCentsDataMigrated, Value: accountBalanceCentsDataMigrationInProgress}).Error; err != nil {
		return false, err
	}
	var option Option
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("key = ?", OptionAccountBalanceCentsDataMigrated).First(&option).Error
	if err != nil {
		return false, err
	}
	if accountBalanceMigrationOptionValueIsTrue(option.Value) {
		return true, nil
	}
	if !strings.EqualFold(strings.TrimSpace(option.Value), accountBalanceCentsDataMigrationInProgress) {
		return false, errors.New("account balance data migration has invalid marker state")
	}
	return false, nil
}

func migrateAccountBalanceData(quotaPerUnit decimal.Decimal, quotaPerUnitSource string, stats accountBalanceMigrationStats) (map[string]string, accountBalanceMigrationStats, error) {
	runtimeValues := make(map[string]string)
	err := DB.Transaction(func(tx *gorm.DB) error {
		alreadyMigrated, err := lockAccountBalanceDataMigrationMarkerTx(tx)
		if err != nil {
			return err
		}
		if alreadyMigrated {
			return errAccountBalanceDataMigrationAlreadyDone
		}
		if err := migrateAccountBalanceUsersTx(tx, quotaPerUnit, &stats); err != nil {
			return err
		}
		if err := migrateAccountBalanceRedemptionsTx(tx, quotaPerUnit, &stats); err != nil {
			return err
		}
		if err := migrateAccountBalanceCheckinsTx(tx, quotaPerUnit, &stats); err != nil {
			return err
		}
		if err := expirePendingTopUpsForAccountBalanceMigrationTx(tx, &stats); err != nil {
			return err
		}
		optionValues, err := migrateAccountBalanceRuntimeOptionsTx(tx, quotaPerUnit, &stats)
		if err != nil {
			return err
		}
		for key, value := range optionValues {
			runtimeValues[key] = value
		}
		if err := upsertOptionTx(tx, OptionAccountBalanceCentsDataMigrated, "true"); err != nil {
			return err
		}
		runtimeValues[OptionAccountBalanceCentsDataMigrated] = "true"
		if accountBalanceDataMigrationBeforeCommitHook != nil {
			if err := accountBalanceDataMigrationBeforeCommitHook(); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		stats.QuotaPerUnit = quotaPerUnit.String()
		stats.QuotaPerUnitSource = quotaPerUnitSource
		stats.Error = err.Error()
		logAccountBalanceMigrationStats(stats)
		return nil, stats, err
	}
	stats.DataMarkerWritten = true
	return runtimeValues, stats, nil
}

func migrateAccountBalanceUsersTx(tx *gorm.DB, quotaPerUnit decimal.Decimal, stats *accountBalanceMigrationStats) error {
	var users []User
	if err := tx.Unscoped().Select("id", "quota", "aff_quota", "aff_history").Find(&users).Error; err != nil {
		return err
	}
	for i := range users {
		quota, err := legacyQuotaToCentsInt(users[i].Quota, quotaPerUnit)
		if err != nil {
			return err
		}
		affQuota, err := legacyQuotaToCentsInt(users[i].AffQuota, quotaPerUnit)
		if err != nil {
			return err
		}
		affHistory, err := legacyQuotaToCentsInt(users[i].AffHistoryQuota, quotaPerUnit)
		if err != nil {
			return err
		}
		if err := tx.Unscoped().Model(&User{}).Where("id = ?", users[i].Id).Updates(map[string]any{
			"quota":       quota,
			"aff_quota":   affQuota,
			"aff_history": affHistory,
		}).Error; err != nil {
			return err
		}
		stats.Users++
		stats.UserAffQuota++
		stats.UserAffHistory++
	}
	return nil
}

func migrateAccountBalanceRedemptionsTx(tx *gorm.DB, quotaPerUnit decimal.Decimal, stats *accountBalanceMigrationStats) error {
	var redemptions []Redemption
	if err := tx.Unscoped().Select("id", "quota", "type").Where("type = ? OR type = ?", RedemptionTypeWallet, "").Find(&redemptions).Error; err != nil {
		return err
	}
	for i := range redemptions {
		quota, err := legacyQuotaToCentsInt(redemptions[i].Quota, quotaPerUnit)
		if err != nil {
			return err
		}
		updates := map[string]any{"quota": quota}
		if strings.TrimSpace(redemptions[i].Type) == "" {
			updates["type"] = RedemptionTypeWallet
			stats.BlankWalletRedemptions++
		}
		if err := tx.Unscoped().Model(&Redemption{}).Where("id = ?", redemptions[i].Id).Updates(updates).Error; err != nil {
			return err
		}
		stats.WalletRedemptions++
	}
	return nil
}

func migrateAccountBalanceCheckinsTx(tx *gorm.DB, quotaPerUnit decimal.Decimal, stats *accountBalanceMigrationStats) error {
	var checkins []Checkin
	if err := tx.Select("id", "quota_awarded").Find(&checkins).Error; err != nil {
		return err
	}
	for i := range checkins {
		oldQuota := checkins[i].QuotaAwarded
		quota, err := legacyQuotaToCentsInt(oldQuota, quotaPerUnit)
		if err != nil {
			return err
		}
		if oldQuota > 0 && quota == 0 {
			stats.CheckinRoundedToZero++
		}
		if err := tx.Model(&Checkin{}).Where("id = ?", checkins[i].Id).Update("quota_awarded", quota).Error; err != nil {
			return err
		}
		stats.Checkins++
	}
	return nil
}

func expirePendingTopUpsForAccountBalanceMigrationTx(tx *gorm.DB, stats *accountBalanceMigrationStats) error {
	var pending []TopUp
	if err := tx.Select("id").Where("status = ?", common.TopUpStatusPending).Find(&pending).Error; err != nil {
		return err
	}
	for i := range pending {
		if err := tx.Model(&TopUp{}).Where("id = ?", pending[i].Id).Update("status", common.TopUpStatusExpired).Error; err != nil {
			return err
		}
		stats.PendingTopUpsExpired++
	}
	if err := tx.Model(&TopUp{}).Where("status = ?", common.TopUpStatusSuccess).Count(&stats.SkippedSuccessfulTopUps).Error; err != nil {
		return err
	}
	return nil
}

func migrateAccountBalanceRuntimeOptionsTx(tx *gorm.DB, quotaPerUnit decimal.Decimal, stats *accountBalanceMigrationStats) (map[string]string, error) {
	values := make(map[string]string)
	for _, key := range []string{"QuotaForNewUser", "QuotaForInviter", "QuotaForInvitee"} {
		value, ok, err := accountBalanceMigrationOptionValueTx(tx, key)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		converted, err := migrateAccountBalanceIntegerOptionValue(key, value, quotaPerUnit)
		if err != nil {
			return nil, err
		}
		if err := upsertOptionTx(tx, key, converted); err != nil {
			return nil, err
		}
		values[key] = converted
		stats.RuntimeOptions++
	}

	if err := migrateKyrenTopUpProductsOptionTx(tx, quotaPerUnit, values, stats); err != nil {
		return nil, err
	}
	if err := migrateCreemProductsOptionTx(tx, quotaPerUnit, values, stats); err != nil {
		return nil, err
	}
	if err := migrateCheckinSettingOptionsTx(tx, quotaPerUnit, values, stats); err != nil {
		return nil, err
	}
	return values, nil
}

func migrateAccountBalanceIntegerOptionValue(key string, value string, quotaPerUnit decimal.Decimal) (string, error) {
	legacy, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil {
		return "", fmt.Errorf("invalid %s for account balance migration: %w", key, err)
	}
	converted, err := legacyQuotaToCentsInt64(legacy, quotaPerUnit)
	if err != nil {
		return "", err
	}
	if converted > int64(math.MaxInt) || converted < -int64(math.MaxInt)-1 {
		return "", fmt.Errorf("migrated %s overflows int", key)
	}
	return strconv.FormatInt(converted, 10), nil
}

func migrateKyrenTopUpProductsOptionTx(tx *gorm.DB, quotaPerUnit decimal.Decimal, values map[string]string, stats *accountBalanceMigrationStats) error {
	value, ok, err := accountBalanceMigrationOptionValueTx(tx, "KyrenTopUpProducts")
	if err != nil || !ok || strings.TrimSpace(value) == "" {
		return err
	}
	var products []map[string]any
	if err := common.UnmarshalJsonStr(value, &products); err != nil {
		return err
	}
	for i := range products {
		quota, ok, err := accountBalanceMigrationOptionNumber(products[i]["quota"])
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		converted, err := legacyQuotaToCentsInt64(quota, quotaPerUnit)
		if err != nil {
			return err
		}
		products[i]["quota"] = converted
		stats.KyrenProducts++
	}
	encoded, err := common.Marshal(products)
	if err != nil {
		return err
	}
	result := string(encoded)
	if err := upsertOptionTx(tx, "KyrenTopUpProducts", result); err != nil {
		return err
	}
	values["KyrenTopUpProducts"] = result
	stats.RuntimeOptions++
	return nil
}

func migrateCreemProductsOptionTx(tx *gorm.DB, quotaPerUnit decimal.Decimal, values map[string]string, stats *accountBalanceMigrationStats) error {
	value, ok, err := accountBalanceMigrationOptionValueTx(tx, "CreemProducts")
	if err != nil || !ok || strings.TrimSpace(value) == "" {
		return err
	}
	var products []map[string]any
	if err := common.UnmarshalJsonStr(value, &products); err != nil {
		return err
	}
	for i := range products {
		for _, quotaKey := range []string{"quota", "Quota"} {
			quota, ok, err := accountBalanceMigrationOptionNumber(products[i][quotaKey])
			if err != nil {
				return err
			}
			if !ok {
				continue
			}
			converted, err := legacyQuotaToCentsInt64(quota, quotaPerUnit)
			if err != nil {
				return err
			}
			products[i][quotaKey] = converted
			stats.CreemProducts++
		}
	}
	encoded, err := common.Marshal(products)
	if err != nil {
		return err
	}
	result := string(encoded)
	if err := upsertOptionTx(tx, "CreemProducts", result); err != nil {
		return err
	}
	values["CreemProducts"] = result
	stats.RuntimeOptions++
	return nil
}

func migrateCheckinSettingOptionsTx(tx *gorm.DB, quotaPerUnit decimal.Decimal, values map[string]string, stats *accountBalanceMigrationStats) error {
	value, ok, err := accountBalanceMigrationOptionValueTx(tx, "checkin_setting")
	if err != nil {
		return err
	}
	if ok && strings.TrimSpace(value) != "" {
		var checkinSetting operation_setting.CheckinSetting
		if err := common.UnmarshalJsonStr(value, &checkinSetting); err != nil {
			return err
		}
		minQuota, err := legacyQuotaToCentsInt(checkinSetting.MinQuota, quotaPerUnit)
		if err != nil {
			return err
		}
		maxQuota, err := legacyQuotaToCentsInt(checkinSetting.MaxQuota, quotaPerUnit)
		if err != nil {
			return err
		}
		checkinSetting.MinQuota = minQuota
		checkinSetting.MaxQuota = maxQuota
		encoded, err := common.Marshal(checkinSetting)
		if err != nil {
			return err
		}
		result := string(encoded)
		if err := upsertOptionTx(tx, "checkin_setting", result); err != nil {
			return err
		}
		values["checkin_setting"] = result
		stats.RuntimeOptions++
	}

	for _, key := range []string{"checkin_setting.min_quota", "checkin_setting.max_quota"} {
		value, ok, err := accountBalanceMigrationOptionValueTx(tx, key)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		converted, err := migrateAccountBalanceIntegerOptionValue(key, value, quotaPerUnit)
		if err != nil {
			return err
		}
		if err := upsertOptionTx(tx, key, converted); err != nil {
			return err
		}
		values[key] = converted
		stats.RuntimeOptions++
	}
	return nil
}

func accountBalanceMigrationOptionNumber(value any) (int64, bool, error) {
	switch v := value.(type) {
	case nil:
		return 0, false, nil
	case int:
		return int64(v), true, nil
	case int64:
		return v, true, nil
	case float64:
		return decimal.NewFromFloat(v).Round(0).IntPart(), true, nil
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			return 0, false, nil
		}
		parsed, err := decimal.NewFromString(trimmed)
		if err != nil {
			return 0, false, err
		}
		return parsed.Round(0).IntPart(), true, nil
	default:
		return 0, false, fmt.Errorf("unsupported option quota type %T", value)
	}
}

func accountBalanceMigrationOptionValueTx(tx *gorm.DB, key string) (string, bool, error) {
	var value string
	row := tx.Model(&Option{}).Select("value").Where("key = ?", key).Row()
	err := row.Scan(&value)
	if err == nil {
		return value, true, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) && !errors.Is(err, sql.ErrNoRows) {
		return "", false, err
	}
	return "", false, nil
}

func loadAccountBalanceFinalMarkerOptionValuesFromDB() (map[string]string, error) {
	keys := []string{OptionAccountBalanceCentsMigrated, OptionAccountBalanceCentsMigratedAt}
	var options []Option
	if err := DB.Where("key IN ?", keys).Find(&options).Error; err != nil {
		return nil, err
	}
	values := make(map[string]string, len(options))
	for i := range options {
		values[options[i].Key] = options[i].Value
	}
	if _, ok := values[OptionAccountBalanceCentsMigrated]; !ok {
		values[OptionAccountBalanceCentsMigrated] = "true"
	}
	return values, nil
}

func loadAccountBalanceMigratedOptionValuesFromDB() (map[string]string, error) {
	keys := []string{
		OptionAccountBalanceCentsDataMigrated,
		"QuotaForNewUser",
		"QuotaForInviter",
		"QuotaForInvitee",
		"KyrenTopUpProducts",
		"CreemProducts",
		"checkin_setting",
		"checkin_setting.min_quota",
		"checkin_setting.max_quota",
	}
	var options []Option
	if err := DB.Where("key IN ?", keys).Find(&options).Error; err != nil {
		return nil, err
	}
	values := make(map[string]string, len(options))
	for i := range options {
		values[options[i].Key] = options[i].Value
	}
	return values, nil
}

func accountBalanceMigrationQuotaPerUnitForDataStage() (decimal.Decimal, string, error) {
	value, ok, err := accountBalanceMigrationDBOptionValue(accountBalanceMigrationQuotaPerUnitOption)
	if err != nil {
		return decimal.Zero, "", err
	}
	if !ok || strings.TrimSpace(value) == "" {
		return decimal.Zero, "", errors.New("missing database QuotaPerUnit for account balance migration")
	}
	quotaPerUnit, err := decimal.NewFromString(strings.TrimSpace(value))
	if err != nil {
		return decimal.Zero, "db_option", fmt.Errorf("invalid QuotaPerUnit for account balance migration: %w", err)
	}
	if quotaPerUnit.LessThanOrEqual(decimal.Zero) {
		return decimal.Zero, "db_option", errors.New("invalid QuotaPerUnit for account balance migration")
	}
	return quotaPerUnit, "db_option", nil
}

func accountBalanceMigrationOptionTrue(key string) (bool, error) {
	value, ok, err := accountBalanceMigrationDBOptionValue(key)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, nil
	}
	return accountBalanceMigrationOptionValueIsTrue(value), nil
}

func accountBalanceMigrationDBOptionValue(key string) (string, bool, error) {
	if DB == nil {
		return "", false, errors.New("database is not initialized")
	}
	var value string
	row := DB.Model(&Option{}).Select("value").Where("key = ?", key).Row()
	err := row.Scan(&value)
	if err == nil {
		return value, true, nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	return "", false, err
}

func accountBalanceMigrationOptionValueIsTrue(value string) bool {
	return strings.EqualFold(strings.TrimSpace(value), "true")
}

func legacyQuotaToCentsInt(value int, quotaPerUnit decimal.Decimal) (int, error) {
	converted, err := legacyQuotaToCentsInt64(int64(value), quotaPerUnit)
	if err != nil {
		return 0, err
	}
	if converted > int64(math.MaxInt) || converted < -int64(math.MaxInt)-1 {
		return 0, errors.New("migrated account balance overflows int")
	}
	return int(converted), nil
}

func legacyQuotaToCentsInt64(value int64, quotaPerUnit decimal.Decimal) (int64, error) {
	if quotaPerUnit.LessThanOrEqual(decimal.Zero) {
		return 0, errors.New("invalid QuotaPerUnit for account balance migration")
	}
	converted := decimal.NewFromInt(value).Mul(decimal.NewFromInt(100)).Div(quotaPerUnit).Round(0)
	maxInt64 := decimal.NewFromInt(math.MaxInt64)
	minInt64 := decimal.NewFromInt(math.MinInt64)
	if converted.GreaterThan(maxInt64) || converted.LessThan(minInt64) {
		return 0, errors.New("migrated account balance overflows int64")
	}
	return converted.IntPart(), nil
}

func invalidateAllUserCachesForAccountBalanceMigration() (int64, string, error) {
	if accountBalanceMigrationInvalidateAllUserCachesHook != nil {
		return 0, "test_hook", accountBalanceMigrationInvalidateAllUserCachesHook()
	}
	if !common.RedisEnabled {
		return 0, "redis_disabled", nil
	}
	if common.RDB == nil {
		return 0, "redis_client_nil", errors.New("redis is enabled but client is nil")
	}
	ctx := context.Background()
	var cursor uint64
	var deleted int64
	for {
		keys, next, err := common.RDB.Scan(ctx, cursor, "user:*", 1000).Result()
		if err != nil {
			return deleted, "", err
		}
		if len(keys) > 0 {
			removed, err := common.RDB.Del(ctx, keys...).Result()
			if err != nil {
				return deleted, "", err
			}
			deleted += removed
		}
		cursor = next
		if cursor == 0 {
			return deleted, "", nil
		}
	}
}

func accountBalanceMigrationUserCacheClearMode() string {
	if accountBalanceMigrationInvalidateAllUserCachesHook != nil {
		return "test_hook"
	}
	if !common.RedisEnabled {
		return "redis_disabled"
	}
	return "redis_scan_user_prefix"
}

func writeAccountBalanceMigrationFinalMarkers(migratedAt string) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := upsertOptionTx(tx, OptionAccountBalanceCentsMigratedAt, migratedAt); err != nil {
			return err
		}
		return upsertOptionTx(tx, OptionAccountBalanceCentsMigrated, "true")
	})
}

func logAccountBalanceMigrationStats(stats accountBalanceMigrationStats) {
	payload, err := common.Marshal(stats)
	if err != nil {
		common.SysLog("account_balance_cents_migration stats_marshal_error=" + err.Error())
		return
	}
	common.SysLog("account_balance_cents_migration " + string(payload))
}

func sortedAccountBalanceMigrationKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func parseCheckinSettingJSON(value string) (operation_setting.CheckinSetting, error) {
	var checkinSetting operation_setting.CheckinSetting
	if err := common.UnmarshalJsonStr(value, &checkinSetting); err != nil {
		return operation_setting.CheckinSetting{}, err
	}
	return checkinSetting, nil
}

func normalizeMigratedKyrenTopUpProductsRuntime(value string) (string, error) {
	return setting.NormalizeKyrenTopUpProductsJSON(value)
}
