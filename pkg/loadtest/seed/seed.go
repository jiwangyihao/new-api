package seed

import (
	"context"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/loadtest/artifact"
	loadtestconfig "github.com/QuantumNous/new-api/pkg/loadtest/config"
	"github.com/QuantumNous/new-api/pkg/loadtest/localguard"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Config struct {
	RunContext      artifact.RunContext
	Model           string
	Group           string // Deprecated: ignored.
	MockBaseURL     string
	SubscriptionKey string
	CompatKey       string
}

const (
	subscriptionUserID   = 910001
	compatUserID         = 910002
	planID               = 910010
	subscriptionID       = 910011
	compatSubscriptionID = 910012
	channelID            = 910020
)

func Apply(ctx context.Context, db *gorm.DB, cfg Config) (artifact.SeedOutput, error) {
	cfg = cfg.withDefaults()
	if err := cfg.Validate(); err != nil {
		return artifact.SeedOutput{}, err
	}
	if err := migrateRequiredTables(ctx, db); err != nil {
		return artifact.SeedOutput{}, err
	}
	if err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ensureUsers(tx); err != nil {
			return err
		}
		if err := ensurePlanAndSubscriptions(tx, cfg); err != nil {
			return err
		}
		if err := ensureTokens(tx, cfg); err != nil {
			return err
		}
		if err := isolateRoute(tx, cfg); err != nil {
			return err
		}
		return ensureOptions(tx)
	}); err != nil {
		return artifact.SeedOutput{}, err
	}
	if common.MemoryCacheEnabled {
		model.InitChannelCache()
	}
	return seedOutput(cfg), nil
}

func SeedOutputForTest() artifact.SeedOutput {
	return seedOutput(Config{RunContext: artifact.RunContext{SchemaVersion: artifact.SchemaVersion, Role: "baseline", Commit: "abcdef0", ComparisonConfigHash: "sha256:cfg", CacheMode: "cold-fresh-role,warm-per-point", Model: "gpt-5.5"}, Model: "gpt-5.5", MockBaseURL: "http://127.0.0.1:19080", SubscriptionKey: loadtestconfig.SubscriptionAPIKey, CompatKey: loadtestconfig.CompatAPIKey})
}

func (c Config) withDefaults() Config {
	if c.Model == "" {
		c.Model = "gpt-5.5"
	}
	if c.MockBaseURL == "" {
		c.MockBaseURL = "http://127.0.0.1:19080"
	}
	if c.SubscriptionKey == "" {
		c.SubscriptionKey = loadtestconfig.SubscriptionAPIKey
	}
	if c.CompatKey == "" {
		c.CompatKey = loadtestconfig.CompatAPIKey
	}
	c.RunContext = c.RunContext.WithoutSeedOutputHash().WithoutMockHash()
	if c.RunContext.SchemaVersion == 0 {
		c.RunContext.SchemaVersion = artifact.SchemaVersion
	}
	if c.RunContext.Model == "" {
		c.RunContext.Model = c.Model
	}
	return c
}

func (c Config) Validate() error {
	if strings.TrimSpace(c.Model) == "" {
		return fmt.Errorf("model is required")
	}
	if err := localguard.ValidateURL(c.MockBaseURL); err != nil {
		return fmt.Errorf("mock base url: %w", err)
	}
	if err := localguard.ValidateAPIKey(c.SubscriptionKey); err != nil {
		return fmt.Errorf("subscription key: %w", err)
	}
	if c.SubscriptionKey != loadtestconfig.SubscriptionAPIKey {
		return fmt.Errorf("subscription key must be %s", loadtestconfig.SubscriptionAPIKey)
	}
	if err := localguard.ValidateAPIKey(c.CompatKey); err != nil {
		return fmt.Errorf("compat key: %w", err)
	}
	if c.CompatKey != loadtestconfig.CompatAPIKey {
		return fmt.Errorf("compat key must be %s", loadtestconfig.CompatAPIKey)
	}
	return nil
}

func migrateRequiredTables(ctx context.Context, db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("db is nil")
	}
	db = db.WithContext(ctx)
	if db.Dialector != nil && db.Dialector.Name() == "postgres" {
		return withDatabaseFlags(common.DatabaseTypePostgreSQL, func() error {
			return db.AutoMigrate(&model.User{}, &model.Token{}, &model.Channel{}, &model.Ability{}, &model.SubscriptionPlan{}, &model.UserSubscription{}, &model.SubscriptionPreConsumeRecord{}, &model.Option{}, &model.Model{}, &model.Log{})
		})
	}
	for _, table := range []any{&model.User{}, &model.Token{}, &model.Channel{}, &model.Ability{}, &model.SubscriptionPlan{}, &model.UserSubscription{}, &model.SubscriptionPreConsumeRecord{}, &model.Option{}, &model.Model{}, &model.Log{}} {
		if db.Migrator().HasTable(table) {
			continue
		}
		if err := db.AutoMigrate(table); err != nil {
			return err
		}
	}
	return nil
}

func withDatabaseFlags(sqlType string, fn func() error) error {
	oldSQLite := common.UsingSQLite
	oldMySQL := common.UsingMySQL
	oldPostgreSQL := common.UsingPostgreSQL
	oldLogType := common.LogSqlType
	switch sqlType {
	case common.DatabaseTypePostgreSQL:
		common.UsingSQLite = false
		common.UsingMySQL = false
		common.UsingPostgreSQL = true
		common.LogSqlType = common.DatabaseTypePostgreSQL
	case common.DatabaseTypeMySQL:
		common.UsingSQLite = false
		common.UsingMySQL = true
		common.UsingPostgreSQL = false
		common.LogSqlType = common.DatabaseTypeMySQL
	default:
		common.UsingSQLite = true
		common.UsingMySQL = false
		common.UsingPostgreSQL = false
		common.LogSqlType = common.DatabaseTypeSQLite
	}
	defer func() {
		common.UsingSQLite = oldSQLite
		common.UsingMySQL = oldMySQL
		common.UsingPostgreSQL = oldPostgreSQL
		common.LogSqlType = oldLogType
	}()
	return fn()
}

func ensureUsers(tx *gorm.DB) error {
	users := []model.User{
		{Id: subscriptionUserID, Username: "loadtest_subscription", Password: "loadtest", DisplayName: "loadtest subscription", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Quota: 1_000_000, Group: "", AffCode: "loadtestsub"},
		{Id: compatUserID, Username: "loadtest_compat", Password: "loadtest", DisplayName: "loadtest compat", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Quota: 1_000_000, Group: "", AffCode: "loadtestcompat"},
	}
	for _, user := range users {
		if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "id"}}, DoUpdates: clause.AssignmentColumns([]string{"username", "display_name", "role", "status", "quota", "group", "aff_code"})}).Create(&user).Error; err != nil {
			return err
		}
	}
	return nil
}

func ensurePlanAndSubscriptions(tx *gorm.DB, cfg Config) error {
	businessCode := "loadtest_basic"
	plan := model.SubscriptionPlan{Id: planID, Title: "Loadtest Basic", PriceAmount: 0, Currency: "USD", DurationUnit: "month", DurationValue: 1, Enabled: true, PublicVisible: false, TotalAmount: 1_000_000, MonthlyTokenLimit: 1_000_000, ConcurrencyLimit: 2000, BusinessCode: &businessCode, QuotaResetPeriod: "never"}
	if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "id"}}, DoUpdates: clause.AssignmentColumns([]string{"title", "enabled", "public_visible", "total_amount", "monthly_token_limit", "concurrency_limit", "business_code"})}).Create(&plan).Error; err != nil {
		return err
	}
	now := common.GetTimestamp()
	subs := []model.UserSubscription{
		{Id: subscriptionID, UserId: subscriptionUserID, PlanId: planID, AmountTotal: 1_000_000, TokenLimit: 1_000_000, TokenUsed: 0, ConcurrencyLimit: 2000, GrantReason: model.SubscriptionGrantOrder, StartTime: now - 3600, EndTime: now + 30*86400, Status: "active", Source: model.SubscriptionGrantOrder},
		{Id: compatSubscriptionID, UserId: compatUserID, PlanId: planID, AmountTotal: 1_000_000, TokenLimit: 1_000_000, TokenUsed: 0, ConcurrencyLimit: 2000, GrantReason: model.SubscriptionGrantOrder, StartTime: now - 3600, EndTime: now + 30*86400, Status: "active", Source: model.SubscriptionGrantOrder},
	}
	for _, sub := range subs {
		if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "id"}}, DoUpdates: clause.AssignmentColumns([]string{"user_id", "plan_id", "amount_total", "amount_used", "token_limit", "token_used", "concurrency_limit", "grant_reason", "start_time", "end_time", "status", "source"})}).Create(&sub).Error; err != nil {
			return err
		}
	}
	return nil
}

func ensureTokens(tx *gorm.DB, cfg Config) error {
	tokens := []model.Token{
		{UserId: subscriptionUserID, Key: loadtestconfig.SubscriptionDBKey, Status: common.TokenStatusEnabled, Name: "loadtest subscription", ExpiredTime: -1, RemainQuota: 1_000_000, UnlimitedQuota: false, Group: ""},
		{UserId: compatUserID, Key: loadtestconfig.CompatDBKey, Status: common.TokenStatusEnabled, Name: "loadtest compat", ExpiredTime: -1, RemainQuota: 1_000_000, UnlimitedQuota: false, Group: ""},
	}
	for _, token := range tokens {
		if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "key"}}, DoUpdates: clause.AssignmentColumns([]string{"user_id", "status", "name", "expired_time", "remain_quota", "unlimited_quota", "group"})}).Create(&token).Error; err != nil {
			return err
		}
	}
	return nil
}

func isolateRoute(tx *gorm.DB, cfg Config) error {
	if err := tx.Model(&model.Ability{}).Where(&model.Ability{Model: cfg.Model}).Where("channel_id <> ?", channelID).Update("enabled", false).Error; err != nil {
		return err
	}
	var channels []model.Channel
	if err := tx.Where("id <> ?", channelID).Find(&channels).Error; err != nil {
		return err
	}
	for _, ch := range channels {
		models := removeCSVValue(ch.Models, cfg.Model)
		if models != ch.Models {
			if err := tx.Model(&model.Channel{}).Where("id = ?", ch.Id).Update("models", models).Error; err != nil {
				return err
			}
		}
	}
	baseURL := cfg.MockBaseURL
	priority := int64(1000)
	weight := uint(100)
	otherSettings := fmt.Sprintf(`{"supported_endpoint_types":["%s"]}`, constant.EndpointTypeOpenAIResponse)
	channel := model.Channel{Id: channelID, Type: constant.ChannelTypeOpenAI, Key: "sk-loadtest-mock", Status: common.ChannelStatusEnabled, Name: "loadtest-loopback-openai", BaseURL: &baseURL, Models: cfg.Model, Group: "", Priority: &priority, Weight: &weight, OtherSettings: otherSettings}
	if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "id"}}, DoUpdates: clause.Assignments(map[string]any{"type": channel.Type, "key": channel.Key, "status": channel.Status, "name": channel.Name, "base_url": channel.BaseURL, "models": channel.Models, "group": channel.Group, "priority": channel.Priority, "weight": channel.Weight, "settings": channel.OtherSettings})}).Create(&channel).Error; err != nil {
		return err
	}
	ability := model.Ability{Group: "", Model: cfg.Model, ChannelId: channelID, Enabled: true, Priority: &priority, Weight: weight}
	return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "group"}, {Name: "model"}, {Name: "channel_id"}}, DoUpdates: clause.AssignmentColumns([]string{"enabled", "priority", "weight"})}).Create(&ability).Error
}

func ensureOptions(tx *gorm.DB) error {
	options := map[string]string{
		"ModelRatio":                           "{\"gpt-5.5\":1}",
		"CompletionRatio":                      "{\"gpt-5.5\":1}",
		"LogConsumeEnabled":                    "true",
		"DataExportEnabled":                    "true",
		"perf_metrics_setting.enabled":         "true",
		"RetryTimes":                           "0",
		"AutomaticRetryStatusCodes":            "",
		"SubscriptionConcurrencyQueueCapacity": "2000",
		"performance_setting.monitor_enabled":  "false",
		"AutomaticDisableChannelEnabled":       "false",
		"AutomaticEnableChannelEnabled":        "false",
	}
	for key, value := range options {
		opt := model.Option{Key: key, Value: value}
		if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "key"}}, DoUpdates: clause.AssignmentColumns([]string{"value"})}).Create(&opt).Error; err != nil {
			return err
		}
	}
	return nil
}

func seedOutput(cfg Config) artifact.SeedOutput {
	cfg = cfg.withDefaults()
	return artifact.SeedOutput{SchemaVersion: artifact.SchemaVersion, RunContext: cfg.RunContext.WithoutSeedOutputHash().WithoutMockHash(), UserIDSubscription: subscriptionUserID, UserIDCompat: compatUserID, TokenSubscription: cfg.SubscriptionKey, TokenCompat: cfg.CompatKey, TokenDBKeySubscription: loadtestconfig.SubscriptionDBKey, TokenDBKeyCompat: loadtestconfig.CompatDBKey, ChannelID: channelID, Model: cfg.Model, MockBaseURL: cfg.MockBaseURL, ExpectedUsagePerSuccess: artifact.Usage{PromptTokens: 11, CompletionTokens: 17, TotalTokens: 28}, RatioConfig: map[string]any{"ModelRatio": map[string]any{cfg.Model: float64(1)}, "CompletionRatio": map[string]any{cfg.Model: float64(1)}}, FeatureOptions: map[string]any{"LogConsumeEnabled": true, "DataExportEnabled": true, "perf_metrics_setting.enabled": true, "RetryTimes": float64(0), "AutomaticRetryStatusCodes": ""}}
}

func removeCSVValue(raw, value string) string {
	parts := strings.Split(raw, ",")
	kept := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || part == value {
			continue
		}
		kept = append(kept, part)
	}
	return strings.Join(kept, ",")
}
