package model

import (
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

var commonGroupCol = "`group`"
var commonKeyCol = "`key`"
var commonTrueVal = "1"
var commonFalseVal = "0"

var logKeyCol = "`key`"
var logGroupCol = "`group`"

func initCol() {
	// init common column names
	if common.UsingPostgreSQL {
		commonGroupCol = `"group"`
		commonKeyCol = `"key"`
		commonTrueVal = "true"
		commonFalseVal = "false"
	} else {
		commonGroupCol = "`group`"
		commonKeyCol = "`key`"
		commonTrueVal = "1"
		commonFalseVal = "0"
	}
	if os.Getenv("LOG_SQL_DSN") != "" {
		switch common.LogSqlType {
		case common.DatabaseTypePostgreSQL:
			logGroupCol = `"group"`
			logKeyCol = `"key"`
		default:
			logGroupCol = commonGroupCol
			logKeyCol = commonKeyCol
		}
	} else {
		// LOG_SQL_DSN 为空时，日志数据库与主数据库相同
		if common.UsingPostgreSQL {
			logGroupCol = `"group"`
			logKeyCol = `"key"`
		} else {
			logGroupCol = commonGroupCol
			logKeyCol = commonKeyCol
		}
	}
	// log sql type and database type
	//common.SysLog("Using Log SQL Type: " + common.LogSqlType)
}

var DB *gorm.DB

var LOG_DB *gorm.DB

func createRootAccountIfNeed() error {
	var user User
	//if user.Status != common.UserStatusEnabled {
	if err := DB.First(&user).Error; err != nil {
		common.SysLog("no user exists, create a root user for you: username is root, password is 123456")
		hashedPassword, err := common.Password2Hash("123456")
		if err != nil {
			return err
		}
		rootUser := User{
			Username:    "root",
			Password:    hashedPassword,
			Role:        common.RoleRootUser,
			Status:      common.UserStatusEnabled,
			DisplayName: "Root User",
			AccessToken: nil,
			Quota:       100000000,
		}
		DB.Create(&rootUser)
	}
	return nil
}

func CheckSetup() {
	setup := GetSetup()
	if setup == nil {
		// No setup record exists, check if we have a root user
		if RootUserExists() {
			common.SysLog("system is not initialized, but root user exists")
			// Create setup record
			newSetup := Setup{
				Version:       common.Version,
				InitializedAt: time.Now().Unix(),
			}
			err := DB.Create(&newSetup).Error
			if err != nil {
				common.SysLog("failed to create setup record: " + err.Error())
			}
			constant.Setup = true
		} else {
			common.SysLog("system is not initialized and no root user exists")
			constant.Setup = false
		}
	} else {
		// Setup record exists, system is initialized
		common.SysLog("system is already initialized at: " + time.Unix(setup.InitializedAt, 0).String())
		constant.Setup = true
	}
}

func chooseDB(envName string, isLog bool) (*gorm.DB, error) {
	return chooseDBWithSelectionLog(envName, isLog, true)
}

func chooseDBWithSelectionLog(envName string, isLog bool, logSelection bool) (*gorm.DB, error) {
	defer func() {
		initCol()
	}()
	dsn := os.Getenv(envName)
	if dsn != "" {
		if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
			// Use PostgreSQL
			if logSelection {
				common.SysLog("using PostgreSQL as database")
			}
			if !isLog {
				common.UsingPostgreSQL = true
			} else {
				common.LogSqlType = common.DatabaseTypePostgreSQL
			}
			return gorm.Open(postgres.New(postgres.Config{
				DSN:                  dsn,
				PreferSimpleProtocol: true, // disables implicit prepared statement usage
			}), &gorm.Config{
				PrepareStmt: true, // precompile SQL
			})
		}
		if strings.HasPrefix(dsn, "local") {
			if logSelection {
				common.SysLog("SQL_DSN not set, using SQLite as database")
			}
			if !isLog {
				common.UsingSQLite = true
			} else {
				common.LogSqlType = common.DatabaseTypeSQLite
			}
			return gorm.Open(sqlite.Open(common.SQLitePath), &gorm.Config{
				PrepareStmt: true, // precompile SQL
			})
		}
		// Use MySQL
		if logSelection {
			common.SysLog("using MySQL as database")
		}
		// check parseTime
		if !strings.Contains(dsn, "parseTime") {
			if strings.Contains(dsn, "?") {
				dsn += "&parseTime=true"
			} else {
				dsn += "?parseTime=true"
			}
		}
		if !isLog {
			common.UsingMySQL = true
		} else {
			common.LogSqlType = common.DatabaseTypeMySQL
		}
		return gorm.Open(mysql.Open(dsn), &gorm.Config{
			PrepareStmt: true, // precompile SQL
		})
	}
	// Use SQLite
	if logSelection {
		common.SysLog("SQL_DSN not set, using SQLite as database")
	}
	common.UsingSQLite = true
	return gorm.Open(sqlite.Open(common.SQLitePath), &gorm.Config{
		PrepareStmt: true, // precompile SQL
	})
}

var maintenanceDB *gorm.DB

func InitMaintenanceDB() (*gorm.DB, error) {
	if maintenanceDB != nil {
		return maintenanceDB, nil
	}
	if os.Getenv("SQL_DSN") == "" {
		if sqlitePath := strings.TrimSpace(os.Getenv("SQLITE_PATH")); sqlitePath != "" {
			common.SQLitePath = sqlitePath
		}
	}

	db, err := chooseDBWithSelectionLog("SQL_DSN", false, false)
	if err != nil {
		return nil, err
	}
	// Maintenance commands own stdout/stderr as a machine-readable JSON
	// protocol; database diagnostics must be returned as errors, not logged.
	db = db.Session(&gorm.Session{Logger: gormlogger.Default.LogMode(gormlogger.Silent)})

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxIdleConns(2)
	sqlDB.SetMaxOpenConns(4)
	sqlDB.SetConnMaxLifetime(60 * time.Second)
	if err := sqlDB.Ping(); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("failed to connect to maintenance database: %w", err)
	}
	if common.UsingMySQL {
		if err := checkMySQLChineseSupport(db); err != nil {
			_ = sqlDB.Close()
			return nil, err
		}
	}

	maintenanceDB = db
	return db, nil
}

func CloseMaintenanceDB() error {
	if maintenanceDB == nil {
		return nil
	}
	db := maintenanceDB
	maintenanceDB = nil
	return closeDB(db)
}

func InitDB() (err error) {
	db, err := chooseDB("SQL_DSN", false)
	if err == nil {
		if common.DebugEnabled {
			db = db.Debug()
		}
		DB = db
		// MySQL charset/collation startup check: ensure Chinese-capable charset
		if common.UsingMySQL {
			if err := checkMySQLChineseSupport(DB); err != nil {
				panic(err)
			}
		}
		sqlDB, err := DB.DB()
		if err != nil {
			return err
		}
		sqlDB.SetMaxIdleConns(common.GetEnvOrDefault("SQL_MAX_IDLE_CONNS", 100))
		sqlDB.SetMaxOpenConns(common.GetEnvOrDefault("SQL_MAX_OPEN_CONNS", 1000))
		sqlDB.SetConnMaxLifetime(time.Second * time.Duration(common.GetEnvOrDefault("SQL_MAX_LIFETIME", 60)))

		if !common.IsMasterNode {
			return nil
		}
		if common.UsingMySQL {
			//_, _ = sqlDB.Exec("ALTER TABLE channels MODIFY model_mapping TEXT;") // TODO: delete this line when most users have upgraded
		}
		common.SysLog("database migration started")
		err = migrateDB()
		return err
	} else {
		common.FatalLog(err)
	}
	return err
}

func InitLogDB() (err error) {
	if os.Getenv("LOG_SQL_DSN") == "" {
		LOG_DB = DB
		return
	}
	db, err := chooseDB("LOG_SQL_DSN", true)
	if err == nil {
		if common.DebugEnabled {
			db = db.Debug()
		}
		LOG_DB = db
		// If log DB is MySQL, also ensure Chinese-capable charset
		if common.LogSqlType == common.DatabaseTypeMySQL {
			if err := checkMySQLChineseSupport(LOG_DB); err != nil {
				panic(err)
			}
		}
		sqlDB, err := LOG_DB.DB()
		if err != nil {
			return err
		}
		sqlDB.SetMaxIdleConns(common.GetEnvOrDefault("SQL_MAX_IDLE_CONNS", 100))
		sqlDB.SetMaxOpenConns(common.GetEnvOrDefault("SQL_MAX_OPEN_CONNS", 1000))
		sqlDB.SetConnMaxLifetime(time.Second * time.Duration(common.GetEnvOrDefault("SQL_MAX_LIFETIME", 60)))

		if !common.IsMasterNode {
			return nil
		}
		common.SysLog("database migration started")
		err = migrateLOGDB()
		return err
	} else {
		common.FatalLog(err)
	}
	return err
}

func migrateDB() error {
	// The compatibility column still stores the display value for existing callers,
	// so widening it must complete before exact micros writes are accepted.
	if err := migrateSubscriptionPlanPriceAmount(); err != nil {
		return err
	}
	// Migrate model_limits column from varchar to text for existing tables
	if err := migrateTokenModelLimitsToText(); err != nil {
		return err
	}

	err := DB.AutoMigrate(
		&Channel{},
		&Token{},
		&User{},
		&PasskeyCredential{},
		&Option{},
		&Redemption{},
		&Ability{},
		&Midjourney{},
		&TopUp{},
		&QuotaData{},
		&Task{},
		&Model{},
		&Vendor{},
		&PrefillGroup{},
		&Setup{},
		&TwoFA{},
		&TwoFABackupCode{},
		&Checkin{},
		&SubscriptionOrder{},
		&PaymentProviderOrder{},
		&PaymentProviderCreationLock{},
		&PaymentProviderEvent{},
		&UserSubscription{},
		&SubscriptionPreConsumeRecord{},
		&CreditBalanceLedger{},
		&CreditBalanceAdjustment{},
		&SubscriptionConversion{},
		&SubscriptionConversionQuote{},
		&SubscriptionPlanCreditChangeAudit{},
		&CreditValuationState{},
		&CreditValuationMigration{},
		&TimedSubscriptionValuationGrant{},
		&TokenLimitPreConsumeRecord{},
		&TrialCode{},
		&TrialRedemption{},
		&InvitationMonthlyEntitlement{},
		&InvitationCommissionAccount{},
		&InvitationRewardEvent{},
		&InvitationCommissionRecord{},
		&InvitationCommissionLedger{},
		&InvitationCommissionWithdrawal{},
		&CustomOAuthProvider{},
		&UserOAuthBinding{},
		&OAuthProviderLock{},
		&GPTAbuseSignalLog{},
		&GPTAbuseUserSuspension{},
		&GPTAbuseWarningReset{},
		&GPTAbuseRepeatBlockLog{},
		&PerfMetric{},
		&ChannelGroup{},
		&ChannelGroupChannel{},
		&TokenGroupBinding{},
	)
	if err != nil {
		return err
	}
	if os.Getenv("LOG_SQL_DSN") == "" {
		if err := migrateLogSchema(DB); err != nil {
			return err
		}
	}
	if err := ensureTopUpAmountUnitColumnSQLite(); err != nil {
		return err
	}
	if common.UsingSQLite {
		if err := ensureSubscriptionPlanTableSQLite(); err != nil {
			return err
		}
	} else {
		if err := DB.AutoMigrate(&SubscriptionPlan{}); err != nil {
			return err
		}
	}
	if err := ensureCreditBalanceSingletonConstraints(); err != nil {
		return err
	}
	if err := ensureCreditBalanceSubscriptionPlan(); err != nil {
		return err
	}
	if err := ensureCreditValuationMigration(DB); err != nil {
		return err
	}
	if err := BackfillTimedSubscriptionGrantMetadata(); err != nil {
		return err
	}
	if err := migrateLegacyTrialPlanTitle(); err != nil {
		return err
	}
	if err := backfillLegacyInvitationRewardEventsAfterMigration(); err != nil {
		return err
	}
	if err := ensureDefaultChannelGroup(); err != nil {
		return err
	}
	return nil
}

func migrateDBFast() error {

	var wg sync.WaitGroup

	migrations := []struct {
		model interface{}
		name  string
	}{
		{&Channel{}, "Channel"},
		{&Token{}, "Token"},
		{&User{}, "User"},
		{&PasskeyCredential{}, "PasskeyCredential"},
		{&Option{}, "Option"},
		{&Redemption{}, "Redemption"},
		{&Ability{}, "Ability"},
		{&Midjourney{}, "Midjourney"},
		{&TopUp{}, "TopUp"},
		{&QuotaData{}, "QuotaData"},
		{&Task{}, "Task"},
		{&Model{}, "Model"},
		{&Vendor{}, "Vendor"},
		{&PrefillGroup{}, "PrefillGroup"},
		{&Setup{}, "Setup"},
		{&TwoFA{}, "TwoFA"},
		{&TwoFABackupCode{}, "TwoFABackupCode"},
		{&Checkin{}, "Checkin"},
		{&SubscriptionOrder{}, "SubscriptionOrder"},
		{&PaymentProviderOrder{}, "PaymentProviderOrder"},
		{&PaymentProviderCreationLock{}, "PaymentProviderCreationLock"},
		{&PaymentProviderEvent{}, "PaymentProviderEvent"},
		{&UserSubscription{}, "UserSubscription"},
		{&SubscriptionPreConsumeRecord{}, "SubscriptionPreConsumeRecord"},
		{&CreditBalanceLedger{}, "CreditBalanceLedger"},
		{&CreditBalanceAdjustment{}, "CreditBalanceAdjustment"},
		{&SubscriptionConversion{}, "SubscriptionConversion"},
		{&SubscriptionConversionQuote{}, "SubscriptionConversionQuote"},
		{&SubscriptionPlanCreditChangeAudit{}, "SubscriptionPlanCreditChangeAudit"},
		{&CreditValuationState{}, "CreditValuationState"},
		{&CreditValuationMigration{}, "CreditValuationMigration"},
		{&TimedSubscriptionValuationGrant{}, "TimedSubscriptionValuationGrant"},
		{&TokenLimitPreConsumeRecord{}, "TokenLimitPreConsumeRecord"},
		{&TrialCode{}, "TrialCode"},
		{&TrialRedemption{}, "TrialRedemption"},
		{&InvitationMonthlyEntitlement{}, "InvitationMonthlyEntitlement"},
		{&InvitationCommissionAccount{}, "InvitationCommissionAccount"},
		{&InvitationRewardEvent{}, "InvitationRewardEvent"},
		{&InvitationCommissionRecord{}, "InvitationCommissionRecord"},
		{&InvitationCommissionLedger{}, "InvitationCommissionLedger"},
		{&InvitationCommissionWithdrawal{}, "InvitationCommissionWithdrawal"},
		{&CustomOAuthProvider{}, "CustomOAuthProvider"},
		{&UserOAuthBinding{}, "UserOAuthBinding"},
		{&OAuthProviderLock{}, "OAuthProviderLock"},
		{&GPTAbuseSignalLog{}, "GPTAbuseSignalLog"},
		{&GPTAbuseUserSuspension{}, "GPTAbuseUserSuspension"},
		{&GPTAbuseWarningReset{}, "GPTAbuseWarningReset"},
		{&GPTAbuseRepeatBlockLog{}, "GPTAbuseRepeatBlockLog"},
		{&PerfMetric{}, "PerfMetric"},
		{&ChannelGroup{}, "ChannelGroup"},
		{&ChannelGroupChannel{}, "ChannelGroupChannel"},
		{&TokenGroupBinding{}, "TokenGroupBinding"},
	}
	// 动态计算migration数量，确保errChan缓冲区足够大
	errChan := make(chan error, len(migrations))

	for _, m := range migrations {
		wg.Add(1)
		go func(model interface{}, name string) {
			defer wg.Done()
			if err := DB.AutoMigrate(model); err != nil {
				errChan <- fmt.Errorf("failed to migrate %s: %v", name, err)
			}
		}(m.model, m.name)
	}

	// Wait for all migrations to complete
	wg.Wait()
	close(errChan)

	// Check for any errors
	for err := range errChan {
		if err != nil {
			return err
		}
	}
	if os.Getenv("LOG_SQL_DSN") == "" {
		if err := migrateLogSchema(DB); err != nil {
			return err
		}
	}
	if err := ensureTopUpAmountUnitColumnSQLite(); err != nil {
		return err
	}
	if common.UsingSQLite {
		if err := ensureSubscriptionPlanTableSQLite(); err != nil {
			return err
		}
	} else {
		if err := DB.AutoMigrate(&SubscriptionPlan{}); err != nil {
			return err
		}
	}
	if err := ensureCreditBalanceSingletonConstraints(); err != nil {
		return err
	}
	if err := ensureCreditBalanceSubscriptionPlan(); err != nil {
		return err
	}
	if err := ensureCreditValuationMigration(DB); err != nil {
		return err
	}
	if err := BackfillTimedSubscriptionGrantMetadata(); err != nil {
		return err
	}
	if err := migrateLegacyTrialPlanTitle(); err != nil {
		return err
	}
	if err := backfillLegacyInvitationRewardEventsAfterMigration(); err != nil {
		return err
	}
	if err := ensureDefaultChannelGroup(); err != nil {
		return err
	}
	common.SysLog("database migrated")
	return nil
}

func backfillLegacyInvitationRewardEventsAfterMigration() error {
	err := DB.Transaction(func(tx *gorm.DB) error {
		return BackfillLegacyInvitationRewardEventsTx(tx, common.GetTimestamp())
	})
	if err != nil {
		common.SysError("failed to backfill legacy invitation reward events: " + err.Error())
		return err
	}
	return nil
}

func migrateLOGDB() error {
	return migrateLogSchema(LOG_DB)
}

type logManualColumnDef struct {
	name       string
	columnType string
}

var logManualColumns = []logManualColumnDef{
	{name: "user_id", columnType: "integer"},
	{name: "created_at", columnType: "bigint"},
	{name: "type", columnType: "integer"},
	{name: "content", columnType: "text"},
	{name: "username", columnType: "text"},
	{name: "token_name", columnType: "text"},
	{name: "model_name", columnType: "text"},
	{name: "quota", columnType: "integer"},
	{name: "prompt_tokens", columnType: "integer"},
	{name: "completion_tokens", columnType: "integer"},
	{name: "metered_tokens", columnType: "integer"},
	{name: "use_time", columnType: "integer"},
	{name: "is_stream", columnType: "boolean"},
	{name: "channel_id", columnType: "integer"},
	{name: "token_id", columnType: "integer"},
	{name: "group", columnType: "text"},
	{name: "ip", columnType: "text"},
	{name: "request_id", columnType: "varchar(64)"},
	{name: "upstream_request_id", columnType: "varchar(128)"},
	{name: "other", columnType: "text"},
	{name: "subscription_id", columnType: "integer"},
	{name: "subscription_tokens_consumed", columnType: "bigint"},
	{name: "billing_source", columnType: "varchar(32)"},
	{name: "endpoint", columnType: "varchar(255)"},
}

func migrateLogSchema(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("log database is nil")
	}
	if db.Migrator().HasTable(&Log{}) {
		if err := migrateLogManualColumns(db); err != nil {
			return err
		}
	} else {
		if err := db.AutoMigrate(&Log{}); err != nil {
			return err
		}
		if err := migrateLogManualColumns(db); err != nil {
			return err
		}
	}
	return db.AutoMigrate(&LogAggregationEvent{}, &FreeSubscriptionUsageHourly{}, &LogUsageHourly{})
}

func migrateLogManualColumns(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("log database is nil")
	}
	if !db.Migrator().HasTable(&Log{}) {
		return nil
	}
	for _, column := range logManualColumns {
		if db.Migrator().HasColumn(&Log{}, column.name) {
			continue
		}
		if err := db.Exec("ALTER TABLE " + quoteLogMigrationIdentifier(db, "logs") + " ADD COLUMN " + quoteLogMigrationIdentifier(db, column.name) + " " + column.columnType).Error; err != nil {
			return err
		}
	}
	return nil
}

func migrateLogDerivedColumns(db *gorm.DB) error {
	return migrateLogManualColumns(db)
}

func quoteLogMigrationIdentifier(db *gorm.DB, identifier string) string {
	if db != nil && db.Dialector != nil && db.Dialector.Name() == "postgres" {
		return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
	}
	return "`" + strings.ReplaceAll(identifier, "`", "``") + "`"
}

type sqliteColumnDef struct {
	Name string
	DDL  string
}

func ensureTopUpAmountUnitColumnSQLite() error {
	if !common.UsingSQLite {
		return nil
	}
	if !DB.Migrator().HasTable(&TopUp{}) {
		return DB.AutoMigrate(&TopUp{})
	}
	if DB.Migrator().HasColumn(&TopUp{}, "amount_unit") {
		return nil
	}
	return DB.Exec("ALTER TABLE `top_ups` ADD COLUMN `amount_unit` varchar(32) DEFAULT ''").Error
}

func ensureSubscriptionPlanTableSQLite() error {
	if !common.UsingSQLite {
		return nil
	}
	tableName := "subscription_plans"
	if !DB.Migrator().HasTable(tableName) {
		createSQL := `CREATE TABLE ` + "`" + tableName + "`" + ` (
` + "`id`" + ` integer,
` + "`title`" + ` varchar(128) NOT NULL,
` + "`subtitle`" + ` varchar(255) DEFAULT '',
` + "`price_amount`" + ` decimal(19,6) NOT NULL,
` + "`price_amount_micros`" + ` bigint,
` + "`currency`" + ` varchar(8) NOT NULL DEFAULT 'USD',
` + "`valuation_currency`" + ` varchar(8),
` + "`duration_unit`" + ` varchar(16) NOT NULL DEFAULT 'month',
` + "`duration_value`" + ` integer NOT NULL DEFAULT 1,
` + "`custom_seconds`" + ` bigint NOT NULL DEFAULT 0,
` + "`enabled`" + ` numeric DEFAULT 1,
` + "`sort_order`" + ` integer DEFAULT 0,
` + "`stripe_price_id`" + ` varchar(128) DEFAULT '',
` + "`creem_product_id`" + ` varchar(128) DEFAULT '',
` + "`kyren_product_id`" + ` varchar(128) DEFAULT '',
` + "`max_purchase_per_user`" + ` integer DEFAULT 0,
` + "`upgrade_group`" + ` varchar(64) DEFAULT '',
` + "`total_amount`" + ` bigint NOT NULL DEFAULT 0,
` + "`monthly_token_limit`" + ` bigint NOT NULL DEFAULT 0,
` + "`concurrency_limit`" + ` integer NOT NULL DEFAULT 0,
` + "`queue_capacity`" + ` integer NOT NULL DEFAULT 0,
` + "`gpt_abuse_warning_limit`" + ` integer NOT NULL DEFAULT 0,
` + "`is_trial`" + ` numeric DEFAULT 0,
` + "`invite_trial`" + ` numeric DEFAULT 0,
` + "`public_visible`" + ` numeric DEFAULT 1,
` + "`trial_duration_hours`" + ` integer NOT NULL DEFAULT 0,
` + "`reward_eligible`" + ` numeric DEFAULT 1,
` + "`business_code`" + ` varchar(64) DEFAULT NULL,
` + "`entitlement_type`" + ` varchar(32) NOT NULL DEFAULT 'timed',
` + "`singleton_key`" + ` varchar(32) DEFAULT NULL,
` + "`model_limits`" + ` text,
` + "`credit_balance_configured`" + ` numeric NOT NULL DEFAULT 0,
` + "`credit_balance_purchase_enabled`" + ` numeric NOT NULL DEFAULT 0,
` + "`credit_balance_redemption_enabled`" + ` numeric NOT NULL DEFAULT 0,
` + "`credit_balance_conversion_enabled`" + ` numeric NOT NULL DEFAULT 0,
` + "`unlimited_purchase_enabled`" + ` numeric NOT NULL DEFAULT 0,
` + "`timed_conversion_enabled`" + ` numeric NOT NULL DEFAULT 0,
` + "`conversion_guard_version`" + ` bigint NOT NULL DEFAULT 0,
` + "`quota_reset_period`" + ` varchar(16) DEFAULT 'never',
` + "`quota_reset_custom_seconds`" + ` bigint DEFAULT 0,
` + "`created_at`" + ` bigint,
` + "`updated_at`" + ` bigint,
PRIMARY KEY (` + "`id`" + `)
)`
		if err := DB.Exec(createSQL).Error; err != nil {
			return err
		}
	} else {
		var cols []struct {
			Name string `gorm:"column:name"`
		}
		if err := DB.Raw("PRAGMA table_info(`" + tableName + "`)").Scan(&cols).Error; err != nil {
			return err
		}
		existing := make(map[string]struct{}, len(cols))
		for _, c := range cols {
			existing[c.Name] = struct{}{}
		}
		required := []sqliteColumnDef{
			{Name: "title", DDL: "`title` varchar(128) NOT NULL"},
			{Name: "subtitle", DDL: "`subtitle` varchar(255) DEFAULT ''"},
			{Name: "price_amount", DDL: "`price_amount` decimal(19,6) NOT NULL"},
			{Name: "price_amount_micros", DDL: "`price_amount_micros` bigint"},
			{Name: "currency", DDL: "`currency` varchar(8) NOT NULL DEFAULT 'USD'"},
			{Name: "valuation_currency", DDL: "`valuation_currency` varchar(8)"},
			{Name: "duration_unit", DDL: "`duration_unit` varchar(16) NOT NULL DEFAULT 'month'"},
			{Name: "duration_value", DDL: "`duration_value` integer NOT NULL DEFAULT 1"},
			{Name: "custom_seconds", DDL: "`custom_seconds` bigint NOT NULL DEFAULT 0"},
			{Name: "enabled", DDL: "`enabled` numeric DEFAULT 1"},
			{Name: "sort_order", DDL: "`sort_order` integer DEFAULT 0"},
			{Name: "stripe_price_id", DDL: "`stripe_price_id` varchar(128) DEFAULT ''"},
			{Name: "creem_product_id", DDL: "`creem_product_id` varchar(128) DEFAULT ''"},
			{Name: "kyren_product_id", DDL: "`kyren_product_id` varchar(128) DEFAULT ''"},
			{Name: "max_purchase_per_user", DDL: "`max_purchase_per_user` integer DEFAULT 0"},
			{Name: "upgrade_group", DDL: "`upgrade_group` varchar(64) DEFAULT ''"},
			{Name: "total_amount", DDL: "`total_amount` bigint NOT NULL DEFAULT 0"},
			{Name: "monthly_token_limit", DDL: "`monthly_token_limit` bigint NOT NULL DEFAULT 0"},
			{Name: "concurrency_limit", DDL: "`concurrency_limit` integer NOT NULL DEFAULT 0"},
			{Name: "queue_capacity", DDL: "`queue_capacity` integer NOT NULL DEFAULT 0"},
			{Name: "gpt_abuse_warning_limit", DDL: "`gpt_abuse_warning_limit` integer NOT NULL DEFAULT 0"},
			{Name: "is_trial", DDL: "`is_trial` numeric DEFAULT 0"},
			{Name: "invite_trial", DDL: "`invite_trial` numeric DEFAULT 0"},
			{Name: "public_visible", DDL: "`public_visible` numeric DEFAULT 1"},
			{Name: "trial_duration_hours", DDL: "`trial_duration_hours` integer NOT NULL DEFAULT 0"},
			{Name: "reward_eligible", DDL: "`reward_eligible` numeric DEFAULT 1"},
			{Name: "business_code", DDL: "`business_code` varchar(64) DEFAULT NULL"},
			{Name: "entitlement_type", DDL: "`entitlement_type` varchar(32) NOT NULL DEFAULT 'timed'"},
			{Name: "singleton_key", DDL: "`singleton_key` varchar(32) DEFAULT NULL"},
			{Name: "model_limits", DDL: "`model_limits` text"},
			{Name: "credit_balance_configured", DDL: "`credit_balance_configured` numeric NOT NULL DEFAULT 0"},
			{Name: "credit_balance_purchase_enabled", DDL: "`credit_balance_purchase_enabled` numeric NOT NULL DEFAULT 0"},
			{Name: "credit_balance_redemption_enabled", DDL: "`credit_balance_redemption_enabled` numeric NOT NULL DEFAULT 0"},
			{Name: "credit_balance_conversion_enabled", DDL: "`credit_balance_conversion_enabled` numeric NOT NULL DEFAULT 0"},
			{Name: "unlimited_purchase_enabled", DDL: "`unlimited_purchase_enabled` numeric NOT NULL DEFAULT 0"},
			{Name: "timed_conversion_enabled", DDL: "`timed_conversion_enabled` numeric NOT NULL DEFAULT 0"},
			{Name: "conversion_guard_version", DDL: "`conversion_guard_version` bigint NOT NULL DEFAULT 0"},
			{Name: "quota_reset_period", DDL: "`quota_reset_period` varchar(16) DEFAULT 'never'"},
			{Name: "quota_reset_custom_seconds", DDL: "`quota_reset_custom_seconds` bigint DEFAULT 0"},
			{Name: "created_at", DDL: "`created_at` bigint"},
			{Name: "updated_at", DDL: "`updated_at` bigint"},
		}
		for _, col := range required {
			if _, ok := existing[col.Name]; ok {
				continue
			}
			if err := DB.Exec("ALTER TABLE `" + tableName + "` ADD COLUMN " + col.DDL).Error; err != nil {
				return err
			}
		}
	}
	if !DB.Migrator().HasIndex(tableName, "idx_subscription_plans_business_code") {
		if err := DB.Exec("CREATE UNIQUE INDEX `idx_subscription_plans_business_code` ON `" + tableName + "` (`business_code`)").Error; err != nil {
			return err
		}
	}
	if !DB.Migrator().HasIndex(tableName, "idx_subscription_plans_singleton_key") {
		if err := DB.Exec("CREATE UNIQUE INDEX `idx_subscription_plans_singleton_key` ON `" + tableName + "` (`singleton_key`)").Error; err != nil {
			return err
		}
	}
	return nil
}

const (
	creditBalancePlanIdentityIndex   = "idx_subscription_plans_credit_balance_identity"
	creditBalanceUserIdentityIndex   = "idx_user_subscriptions_credit_balance_identity"
	creditBalanceIdentityGuardColumn = "credit_balance_identity_guard"
)

type creditBalanceMySQLConstraintDDL struct {
	model          any
	tableName      string
	indexName      string
	addColumnSQL   string
	createIndexSQL string
}

func creditBalancePostgreSQLPartialUniqueIndexDDL() []string {
	return []string{
		`CREATE UNIQUE INDEX IF NOT EXISTS "` + creditBalancePlanIdentityIndex + `" ON "subscription_plans" ("entitlement_type") WHERE "entitlement_type" = 'credit_balance'`,
		`CREATE UNIQUE INDEX IF NOT EXISTS "` + creditBalanceUserIdentityIndex + `" ON "user_subscriptions" ("user_id") WHERE "entitlement_type" = 'credit_balance'`,
	}
}

func creditBalanceSQLitePartialUniqueIndexDDL() []string {
	return []string{
		"CREATE UNIQUE INDEX IF NOT EXISTS `" + creditBalancePlanIdentityIndex + "` ON `subscription_plans` (`entitlement_type`) WHERE `entitlement_type` = 'credit_balance'",
		"CREATE UNIQUE INDEX IF NOT EXISTS `" + creditBalanceUserIdentityIndex + "` ON `user_subscriptions` (`user_id`) WHERE `entitlement_type` = 'credit_balance'",
	}
}

func creditBalanceMySQL57ConstraintDDL() []creditBalanceMySQLConstraintDDL {
	return []creditBalanceMySQLConstraintDDL{
		{
			model:          &SubscriptionPlan{},
			tableName:      "subscription_plans",
			indexName:      creditBalancePlanIdentityIndex,
			addColumnSQL:   "ALTER TABLE `subscription_plans` ADD COLUMN `" + creditBalanceIdentityGuardColumn + "` TINYINT GENERATED ALWAYS AS (CASE WHEN `entitlement_type` = 'credit_balance' THEN 1 ELSE NULL END) STORED",
			createIndexSQL: "CREATE UNIQUE INDEX `" + creditBalancePlanIdentityIndex + "` ON `subscription_plans` (`" + creditBalanceIdentityGuardColumn + "`)",
		},
		{
			model:          &UserSubscription{},
			tableName:      "user_subscriptions",
			indexName:      creditBalanceUserIdentityIndex,
			addColumnSQL:   "ALTER TABLE `user_subscriptions` ADD COLUMN `" + creditBalanceIdentityGuardColumn + "` BIGINT GENERATED ALWAYS AS (CASE WHEN `entitlement_type` = 'credit_balance' THEN `user_id` ELSE NULL END) STORED",
			createIndexSQL: "CREATE UNIQUE INDEX `" + creditBalanceUserIdentityIndex + "` ON `user_subscriptions` (`" + creditBalanceIdentityGuardColumn + "`)",
		},
	}
}

func ensureCreditBalanceSingletonConstraints() error {
	if DB == nil {
		return fmt.Errorf("database is nil")
	}
	switch {
	case common.UsingMySQL:
		return ensureCreditBalanceSingletonConstraintsMySQL()
	case common.UsingPostgreSQL:
		return ensureCreditBalancePartialUniqueIndexes(creditBalancePostgreSQLPartialUniqueIndexDDL())
	case common.UsingSQLite:
		return ensureCreditBalancePartialUniqueIndexes(creditBalanceSQLitePartialUniqueIndexDDL())
	default:
		return fmt.Errorf("unsupported database dialect for Credit balance singleton constraints: %s", DB.Dialector.Name())
	}
}

func ensureCreditBalancePartialUniqueIndexes(statements []string) error {
	for _, statement := range statements {
		if err := DB.Exec(statement).Error; err != nil {
			return err
		}
	}
	return nil
}

func ensureCreditBalanceSingletonConstraintsMySQL() error {
	for _, constraint := range creditBalanceMySQL57ConstraintDDL() {
		if !DB.Migrator().HasColumn(constraint.model, creditBalanceIdentityGuardColumn) {
			if err := DB.Exec(constraint.addColumnSQL).Error; err != nil {
				return err
			}
		}
		if !DB.Migrator().HasIndex(constraint.model, constraint.indexName) {
			if err := DB.Exec(constraint.createIndexSQL).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

func ensureCreditBalanceSubscriptionPlan() error {
	if DB == nil {
		return fmt.Errorf("database is nil")
	}

	var existing []SubscriptionPlan
	if err := DB.Where("entitlement_type = ?", SubscriptionEntitlementCreditBalance).Order("id asc").Limit(2).Find(&existing).Error; err != nil {
		return err
	}
	if len(existing) > 1 {
		return fmt.Errorf("multiple credit balance subscription plans exist")
	}
	if len(existing) == 1 {
		updates := make(map[string]any)
		if existing[0].SingletonKey == nil || *existing[0].SingletonKey != creditBalancePlanSingletonKey {
			updates["singleton_key"] = creditBalancePlanSingletonKey
		}
		if existing[0].PriceAmountMicros == nil && existing[0].PriceAmount == 0 {
			updates["price_amount_micros"] = int64(0)
		}
		if len(updates) == 0 {
			return nil
		}
		return DB.Model(&SubscriptionPlan{}).Where("id = ?", existing[0].Id).Updates(updates).Error
	}

	now := common.GetTimestamp()
	createErr := DB.Model(&SubscriptionPlan{}).Create(map[string]any{
		"title":                             "Credit 余额套餐",
		"price_amount":                      0,
		"price_amount_micros":               int64(0),
		"currency":                          "CNY",
		"duration_unit":                     SubscriptionDurationMonth,
		"duration_value":                    1,
		"enabled":                           true,
		"public_visible":                    false,
		"reward_eligible":                   false,
		"entitlement_type":                  SubscriptionEntitlementCreditBalance,
		"singleton_key":                     creditBalancePlanSingletonKey,
		"quota_reset_period":                SubscriptionResetNever,
		"credit_balance_configured":         false,
		"credit_balance_purchase_enabled":   false,
		"credit_balance_redemption_enabled": false,
		"credit_balance_conversion_enabled": false,
		"created_at":                        now,
		"updated_at":                        now,
	}).Error
	if createErr == nil {
		return nil
	}

	var concurrent SubscriptionPlan
	if err := DB.Where("singleton_key = ? AND entitlement_type = ?", creditBalancePlanSingletonKey, SubscriptionEntitlementCreditBalance).First(&concurrent).Error; err == nil {
		return nil
	}
	return createErr
}

// migrateTokenModelLimitsToText migrates model_limits column from varchar(1024) to text
// This is safe to run multiple times - it checks the column type first
func migrateTokenModelLimitsToText() error {
	// SQLite uses type affinity, so TEXT and VARCHAR are effectively the same — no migration needed
	if common.UsingSQLite {
		return nil
	}

	tableName := "tokens"
	columnName := "model_limits"

	if !DB.Migrator().HasTable(tableName) {
		return nil
	}

	if !DB.Migrator().HasColumn(&Token{}, columnName) {
		return nil
	}

	var alterSQL string
	if common.UsingPostgreSQL {
		var dataType string
		if err := DB.Raw(`SELECT data_type FROM information_schema.columns
			WHERE table_schema = current_schema() AND table_name = ? AND column_name = ?`,
			tableName, columnName).Scan(&dataType).Error; err != nil {
			common.SysLog(fmt.Sprintf("Warning: failed to query metadata for %s.%s: %v", tableName, columnName, err))
		} else if dataType == "text" {
			return nil
		}
		alterSQL = fmt.Sprintf(`ALTER TABLE %s ALTER COLUMN %s TYPE text`, tableName, columnName)
	} else if common.UsingMySQL {
		var columnType string
		if err := DB.Raw(`SELECT COLUMN_TYPE FROM information_schema.columns
				WHERE table_schema = DATABASE() AND table_name = ? AND column_name = ?`,
			tableName, columnName).Scan(&columnType).Error; err != nil {
			common.SysLog(fmt.Sprintf("Warning: failed to query metadata for %s.%s: %v", tableName, columnName, err))
		} else if strings.ToLower(columnType) == "text" {
			return nil
		}
		alterSQL = fmt.Sprintf("ALTER TABLE %s MODIFY COLUMN %s text", tableName, columnName)
	} else {
		return nil
	}

	if alterSQL != "" {
		if err := DB.Exec(alterSQL).Error; err != nil {
			return fmt.Errorf("failed to migrate %s.%s to text: %w", tableName, columnName, err)
		}
		common.SysLog(fmt.Sprintf("Successfully migrated %s.%s to text", tableName, columnName))
	}
	return nil
}

// migrateSubscriptionPlanPriceAmount widens the compatibility display column.
// Exact micros are authoritative, but both columns are written together for existing callers.
func migrateSubscriptionPlanPriceAmount() error {
	// SQLite doesn't support ALTER COLUMN, and its type affinity handles this automatically.
	if common.UsingSQLite {
		return nil
	}

	tableName := "subscription_plans"
	columnName := "price_amount"
	if !DB.Migrator().HasTable(tableName) {
		return nil
	}
	if !DB.Migrator().HasColumn(&SubscriptionPlan{}, columnName) {
		return nil
	}

	var alterSQL string
	if common.UsingPostgreSQL {
		var precision, scale int
		if err := DB.Raw(`SELECT COALESCE(numeric_precision, 0), COALESCE(numeric_scale, 0) FROM information_schema.columns
			WHERE table_schema = current_schema() AND table_name = ? AND column_name = ?`,
			tableName, columnName).Row().Scan(&precision, &scale); err != nil {
			return fmt.Errorf("failed to query metadata for %s.%s: %w", tableName, columnName, err)
		}
		if precision >= 19 && scale == 6 {
			return nil
		}
		alterSQL = fmt.Sprintf(`ALTER TABLE %s ALTER COLUMN %s TYPE decimal(19,6) USING %s::decimal(19,6)`,
			tableName, columnName, columnName)
	} else if common.UsingMySQL {
		var columnType string
		if err := DB.Raw(`SELECT COLUMN_TYPE FROM information_schema.columns
				WHERE table_schema = DATABASE() AND table_name = ? AND column_name = ?`,
			tableName, columnName).Scan(&columnType).Error; err != nil {
			return fmt.Errorf("failed to query metadata for %s.%s: %w", tableName, columnName, err)
		}
		if strings.HasPrefix(strings.ToLower(columnType), "decimal(19,6)") {
			return nil
		}
		alterSQL = fmt.Sprintf("ALTER TABLE %s MODIFY COLUMN %s decimal(19,6) NOT NULL DEFAULT 0", tableName, columnName)
	} else {
		return nil
	}

	if err := DB.Exec(alterSQL).Error; err != nil {
		return fmt.Errorf("failed to migrate %s.%s to decimal(19,6): %w", tableName, columnName, err)
	}
	common.SysLog(fmt.Sprintf("Successfully migrated %s.%s to decimal(19,6)", tableName, columnName))
	return nil
}

func closeDB(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	err = sqlDB.Close()
	return err
}

func CloseDB() error {
	FlushUsageCounterUpdates()
	FlushSubscriptionTokenDeltaUpdates()
	FlushConsumeLogUpdates()
	if LOG_DB != DB {
		err := closeDB(LOG_DB)
		if err != nil {
			return err
		}
	}
	return closeDB(DB)
}

// checkMySQLChineseSupport ensures the MySQL connection and current schema
// default charset/collation can store Chinese characters. It allows common
// Chinese-capable charsets (utf8mb4, utf8, gbk, big5, gb18030) and panics otherwise.
func checkMySQLChineseSupport(db *gorm.DB) error {
	// 仅检测：当前库默认字符集/排序规则 + 各表的排序规则（隐含字符集）

	// Read current schema defaults
	var schemaCharset, schemaCollation string
	err := db.Raw("SELECT DEFAULT_CHARACTER_SET_NAME, DEFAULT_COLLATION_NAME FROM information_schema.SCHEMATA WHERE SCHEMA_NAME = DATABASE()").Row().Scan(&schemaCharset, &schemaCollation)
	if err != nil {
		return fmt.Errorf("读取当前库默认字符集/排序规则失败 / Failed to read schema default charset/collation: %v", err)
	}

	toLower := func(s string) string { return strings.ToLower(s) }
	// Allowed charsets that can store Chinese text
	allowedCharsets := map[string]string{
		"utf8mb4": "utf8mb4_",
		"utf8":    "utf8_",
		"gbk":     "gbk_",
		"big5":    "big5_",
		"gb18030": "gb18030_",
	}
	isChineseCapable := func(cs, cl string) bool {
		csLower := toLower(cs)
		clLower := toLower(cl)
		if prefix, ok := allowedCharsets[csLower]; ok {
			if clLower == "" {
				return true
			}
			return strings.HasPrefix(clLower, prefix)
		}
		// 如果仅提供了排序规则，尝试按排序规则前缀判断
		for _, prefix := range allowedCharsets {
			if strings.HasPrefix(clLower, prefix) {
				return true
			}
		}
		return false
	}

	// 1) 当前库默认值必须支持中文
	if !isChineseCapable(schemaCharset, schemaCollation) {
		return fmt.Errorf("当前库默认字符集/排序规则不支持中文：schema(%s/%s)。请将库设置为 utf8mb4/utf8/gbk/big5/gb18030 / Schema default charset/collation is not Chinese-capable: schema(%s/%s). Please set to utf8mb4/utf8/gbk/big5/gb18030",
			schemaCharset, schemaCollation, schemaCharset, schemaCollation)
	}

	// 2) 所有物理表的排序规则（隐含字符集）必须支持中文
	type tableInfo struct {
		Name      string
		Collation *string
	}
	var tables []tableInfo
	if err := db.Raw("SELECT TABLE_NAME, TABLE_COLLATION FROM information_schema.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_TYPE = 'BASE TABLE'").Scan(&tables).Error; err != nil {
		return fmt.Errorf("读取表排序规则失败 / Failed to read table collations: %v", err)
	}

	var badTables []string
	for _, t := range tables {
		// NULL 或空表示继承库默认设置，已在上面校验库默认，视为通过
		if t.Collation == nil || *t.Collation == "" {
			continue
		}
		cl := *t.Collation
		// 仅凭排序规则判断是否中文可用
		ok := false
		lower := strings.ToLower(cl)
		for _, prefix := range allowedCharsets {
			if strings.HasPrefix(lower, prefix) {
				ok = true
				break
			}
		}
		if !ok {
			badTables = append(badTables, fmt.Sprintf("%s(%s)", t.Name, cl))
		}
	}

	if len(badTables) > 0 {
		// 限制输出数量以避免日志过长
		maxShow := 20
		shown := badTables
		if len(shown) > maxShow {
			shown = shown[:maxShow]
		}
		return fmt.Errorf(
			"存在不支持中文的表，请修复其排序规则/字符集。示例（最多展示 %d 项）：%v / Found tables not Chinese-capable. Please fix their collation/charset. Examples (showing up to %d): %v",
			maxShow, shown, maxShow, shown,
		)
	}
	return nil
}

var (
	lastPingTime time.Time
	pingMutex    sync.Mutex
)

func PingDB() error {
	pingMutex.Lock()
	defer pingMutex.Unlock()

	if time.Since(lastPingTime) < time.Second*10 {
		return nil
	}

	sqlDB, err := DB.DB()
	if err != nil {
		log.Printf("Error getting sql.DB from GORM: %v", err)
		return err
	}

	err = sqlDB.Ping()
	if err != nil {
		log.Printf("Error pinging DB: %v", err)
		return err
	}

	lastPingTime = time.Now()
	common.SysLog("Database pinged successfully")
	return nil
}
