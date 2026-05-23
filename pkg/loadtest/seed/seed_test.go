package seed

import (
	"context"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/loadtest/artifact"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestSeedIsIdempotentAndCreatesBillingObjects(t *testing.T) {
	db := openSeedTestDB(t)
	cfg := Config{Model: "gpt-5.5", MockBaseURL: "http://127.0.0.1:19080", SubscriptionKey: "sk-loadtestsub", CompatKey: "sk-loadtestcompat"}
	first, err := Apply(context.Background(), db, cfg)
	require.NoError(t, err)
	second, err := Apply(context.Background(), db, cfg)
	require.NoError(t, err)
	if first.TokenDBKeySubscription != "loadtestsub" || second.TokenDBKeySubscription != "loadtestsub" {
		t.Fatalf("bad token key")
	}
	assertCount(t, db, &model.Token{}, 2)
	assertCount(t, db, &model.UserSubscription{}, 2)
	assertOptionJSONContains(t, db, "ModelRatio", "gpt-5.5")
	assertOptionEnabled(t, db, "LogConsumeEnabled", true)
	assertOptionEnabled(t, db, "DataExportEnabled", true)
	assertOptionValue(t, db, "RetryTimes", "0")
	assertOptionValue(t, db, "AutomaticRetryStatusCodes", "")
	assertOptionValue(t, db, "perf_metrics_setting.enabled", "true")
	assertOptionValue(t, db, "performance_setting.monitor_enabled", "false")
	assertOptionValue(t, db, "AutomaticDisableChannelEnabled", "false")
	assertOptionValue(t, db, "AutomaticEnableChannelEnabled", "false")
	assertSubscriptionConcurrencyPositive(t, db)
}

func TestSeedDisablesUnsafeChannelsForModelRoute(t *testing.T) {
	db := openSeedTestDB(t)
	unsafeURL := "https://api.openai.com"
	unsafeChannel := model.Channel{Id: 99, Name: "unsafe", Type: constant.ChannelTypeOpenAI, Key: "real", BaseURL: &unsafeURL, Status: common.ChannelStatusEnabled, Models: "gpt-5.5,gpt-4", Group: "default,other"}
	require.NoError(t, db.Create(&unsafeChannel).Error)
	priority := int64(999)
	require.NoError(t, db.Create(&model.Ability{Group: "default", Model: "gpt-5.5", ChannelId: unsafeChannel.Id, Enabled: true, Priority: &priority, Weight: 100}).Error)
	_, err := Apply(context.Background(), db, Config{Model: "gpt-5.5", Group: "legacy-ignored", MockBaseURL: "http://127.0.0.1:19080", SubscriptionKey: "sk-loadtestsub", CompatKey: "sk-loadtestcompat"})
	require.NoError(t, err)
	var reloaded model.Channel
	require.NoError(t, db.First(&reloaded, unsafeChannel.Id).Error)
	if reloaded.Status == common.ChannelStatusEnabled && strings.Contains(reloaded.Models, "gpt-5.5") {
		t.Fatalf("unsafe channel still routable: %#v", reloaded)
	}
	var ability model.Ability
	require.NoError(t, db.First(&ability, "channel_id = ? AND model = ? AND `group` = ?", unsafeChannel.Id, "gpt-5.5", "default").Error)
	if ability.Enabled {
		t.Fatalf("unsafe ability still enabled")
	}
}

func TestSeedOutputHashAndRunContext(t *testing.T) {
	out := SeedOutputForTest()
	if out.RunContext.SeedOutputHash != "" || out.RunContext.MockHash != "" {
		t.Fatalf("seed output self/scenario hash present: %#v", out.RunContext)
	}
	hash, err := artifact.HashSeedOutput(out)
	if err != nil || hash == "" {
		t.Fatalf("hash=%q err=%v", hash, err)
	}
}

func openSeedTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	oldDB := model.DB
	oldLogDB := model.LOG_DB
	oldSQLite := common.UsingSQLite
	common.UsingSQLite = true
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	model.LOG_DB = db
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.Channel{}, &model.Ability{}, &model.SubscriptionPlan{}, &model.UserSubscription{}, &model.Option{}, &model.Model{}))
	t.Cleanup(func() {
		model.DB = oldDB
		model.LOG_DB = oldLogDB
		common.UsingSQLite = oldSQLite
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func assertCount(t *testing.T, db *gorm.DB, value any, want int64) {
	t.Helper()
	var count int64
	require.NoError(t, db.Model(value).Count(&count).Error)
	if count != want {
		t.Fatalf("count = %d want %d", count, want)
	}
}

func assertOptionJSONContains(t *testing.T, db *gorm.DB, key, want string) {
	t.Helper()
	var opt model.Option
	require.NoError(t, db.First(&opt, "key = ?", key).Error)
	if !strings.Contains(opt.Value, want) {
		t.Fatalf("option %s=%q missing %q", key, opt.Value, want)
	}
}

func assertOptionEnabled(t *testing.T, db *gorm.DB, key string, want bool) {
	t.Helper()
	value := "false"
	if want {
		value = "true"
	}
	assertOptionValue(t, db, key, value)
}

func assertOptionValue(t *testing.T, db *gorm.DB, key, want string) {
	t.Helper()
	var opt model.Option
	require.NoError(t, db.First(&opt, "key = ?", key).Error)
	if opt.Value != want {
		t.Fatalf("option %s=%q want %q", key, opt.Value, want)
	}
}

func assertSubscriptionConcurrencyPositive(t *testing.T, db *gorm.DB) {
	t.Helper()
	var opt model.Option
	require.NoError(t, db.First(&opt, "key = ?", "SubscriptionConcurrencyQueueCapacity").Error)
	if opt.Value == "" || opt.Value == "0" {
		t.Fatalf("subscription concurrency disabled: %#v", opt)
	}
}
