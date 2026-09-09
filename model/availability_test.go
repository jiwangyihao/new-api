package model

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Real database runs require an explicitly supplied, empty isolation database.
// No production DSN is inherited and no database is created or dropped.
func setupAvailabilityTest(t *testing.T) time.Time {
	t.Helper()
	var dialector gorm.Dialector = sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_")))
	external := false
	if dsn := os.Getenv("AVAILABILITY_TEST_MYSQL_DSN"); dsn != "" {
		dialector = mysql.Open(dsn)
		external = true
	}
	if dsn := os.Getenv("AVAILABILITY_TEST_POSTGRES_DSN"); dsn != "" {
		require.Empty(t, os.Getenv("AVAILABILITY_TEST_MYSQL_DSN"))
		dialector = postgres.Open(dsn)
		external = true
	}
	db, err := gorm.Open(dialector, &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	conn, err := db.DB()
	require.NoError(t, err)
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, conn.Close()) })
	if external {
		var name string
		query := "SELECT DATABASE()"
		if db.Dialector.Name() == "postgres" {
			query = "SELECT current_database()"
		}
		require.NoError(t, db.Raw(query).Scan(&name).Error)
		require.Contains(t, strings.ToLower(name), "availability_test", "refusing non-isolation database")
		tables, err := db.Migrator().GetTables()
		require.NoError(t, err)
		require.Empty(t, tables, "refusing nonempty isolation database")
	}
	oldDB, oldLogDB := DB, LOG_DB
	DB, LOG_DB = db, db
	t.Cleanup(func() {
		DB, LOG_DB = oldDB, oldLogDB
		if external {
			tables, err := db.Migrator().GetTables()
			require.NoError(t, err)
			for _, table := range tables {
				require.NoError(t, db.Migrator().DropTable(table))
			}
		}
	})
	require.NoError(t, db.AutoMigrate(&Channel{}, &ChannelGroup{}, &ChannelGroupChannel{}))
	require.NoError(t, MigrateAvailability(db))
	require.NoError(t, db.Create(&ChannelGroup{Id: 1, Name: DefaultChannelGroupName, Enabled: true}).Error)
	require.NoError(t, db.Create(&ChannelGroup{Id: 2, Name: "Alpha", Enabled: true}).Error)
	require.NoError(t, db.Create(&Channel{Id: 10, Name: "private-upstream", Key: "private-secret", Status: common.ChannelStatusEnabled, Models: "a,b", TokenBillingMultiplier: 1}).Error)
	require.NoError(t, db.Create(&ChannelGroupChannel{ChannelGroupId: 2, ChannelId: 10}).Error)
	return time.Now().Add(2 * time.Hour).Truncate(time.Second)
}

func TestAvailabilityPendingFinalIdempotence(t *testing.T) {
	now := setupAvailabilityTest(t)
	ctx := context.Background()
	observation := AvailabilityObservation{ID: "server-request", StartedAt: now.Unix() - 30, GroupID: 2, ModelName: "a", Outcome: "pending"}
	require.NoError(t, RecordAvailability(ctx, observation))
	report, err := GetAvailabilityReport(ctx, "1h", now)
	require.NoError(t, err)
	require.Len(t, report.Groups, 1)
	assert.Equal(t, "unknown", report.Groups[0].Current.State)
	assert.Nil(t, report.Groups[0].Current.SuccessRate)
	observation.CompletedAt = now.Unix() - 1
	observation.Outcome = AvailabilitySuccess
	require.NoError(t, RecordAvailability(ctx, observation))
	require.NoError(t, RecordAvailability(ctx, observation))
	require.NoError(t, RecordAvailability(ctx, AvailabilityObservation{ID: "failure", StartedAt: now.Unix() - 20, CompletedAt: now.Unix() - 1, GroupID: 2, ModelName: "a", Outcome: AvailabilityFailure}))
	report, err = GetAvailabilityReport(ctx, "1h", now.Add(61*time.Second))
	require.NoError(t, err)
	require.NotNil(t, report.Groups[0].Current.SuccessRate)
	assert.Equal(t, 50.0, *report.Groups[0].Current.SuccessRate)
	assert.True(t, report.Groups[0].Current.LowSample)
}

func availabilityObserve(t *testing.T, now time.Time, id, name, outcome string, latency *int64) {
	t.Helper()
	require.NoError(t, RecordAvailability(context.Background(), AvailabilityObservation{ID: id, StartedAt: now.Unix() - 120, CompletedAt: now.Unix() - 60, GroupID: 2, ModelName: name, Outcome: outcome, FirstResponseMs: latency}))
}

func TestAvailabilityThresholdsAndWeightedMedian(t *testing.T) {
	now := setupAvailabilityTest(t)
	zero, ten, hundred := int64(0), int64(10), int64(100)
	for i := range 100 {
		outcome := AvailabilitySuccess
		if i == 99 {
			outcome = AvailabilityFailure
		}
		availabilityObserve(t, now, fmt.Sprintf("a-%d", i), "a", outcome, &ten)
	}
	for i := range 20 {
		outcome := AvailabilitySuccess
		if i == 19 {
			outcome = AvailabilityFailure
		}
		availabilityObserve(t, now, fmt.Sprintf("b-%d", i), "b", outcome, &hundred)
	}
	availabilityObserve(t, now, "zero", "zero", AvailabilitySuccess, &zero)
	availabilityObserve(t, now, "missing", "missing", AvailabilitySuccess, nil)
	availabilityObserve(t, now, "excluded", "a", AvailabilityExcluded, nil)
	availabilityObserve(t, now, "bad", "bad", AvailabilityFailure, nil)
	report, err := GetAvailabilityReport(context.Background(), "1h", now)
	require.NoError(t, err)
	g := report.Groups[0]
	assert.Equal(t, "healthy", g.Current.State)
	require.NotNil(t, g.Current.SuccessRate)
	assert.InDelta(t, 120.0/123*100, *g.Current.SuccessRate, 0.000001)
	require.NotNil(t, g.Current.FirstResponseMs)
	assert.Equal(t, 10.0, *g.Current.FirstResponseMs, "group median must not average model medians")
	byName := map[string]AvailabilityModel{}
	for _, m := range g.Models {
		byName[m.Name] = m
	}
	assert.Equal(t, "healthy", byName["a"].Current.State)
	assert.Equal(t, "healthy", byName["b"].Current.State)
	assert.False(t, byName["b"].Current.LowSample)
	assert.Equal(t, "unhealthy", byName["bad"].Current.State)
	assert.True(t, byName["bad"].Current.LowSample)
	require.NotNil(t, byName["zero"].Current.FirstResponseMs)
	assert.Zero(t, *byName["zero"].Current.FirstResponseMs)
	assert.Nil(t, byName["missing"].Current.FirstResponseMs)
	encoded, err := common.Marshal(report)
	require.NoError(t, err)
	var public map[string]interface{}
	require.NoError(t, common.Unmarshal(encoded, &public))
	var check func(interface{})
	check = func(value interface{}) {
		switch v := value.(type) {
		case map[string]interface{}:
			for key, child := range v {
				assert.NotContains(t, []string{"count", "frequency", "success_count", "failure_count", "request_id", "channel_id", "key", "reason"}, key)
				check(child)
			}
		case []interface{}:
			for _, child := range v {
				check(child)
			}
		}
	}
	check(public)
	assert.NotContains(t, string(encoded), "private-upstream")
	assert.NotContains(t, string(encoded), "private-secret")
}

func TestAvailabilityRelaxedHealthThresholds(t *testing.T) {
	now := setupAvailabilityTest(t)
	for _, successCount := range []int{79, 80, 89, 90} {
		name := fmt.Sprintf("boundary-%d", successCount)
		for i := range 100 {
			outcome := AvailabilityFailure
			if i < successCount {
				outcome = AvailabilitySuccess
			}
			availabilityObserve(t, now, fmt.Sprintf("%s-%d", name, i), name, outcome, nil)
		}
	}
	report, err := GetAvailabilityReport(context.Background(), "1h", now)
	require.NoError(t, err)
	require.Len(t, report.Groups, 1)
	want := map[string]string{"boundary-79": "unhealthy", "boundary-80": "degraded", "boundary-89": "degraded", "boundary-90": "healthy"}
	for _, model := range report.Groups[0].Models {
		expected, exists := want[model.Name]
		if !exists {
			continue
		}
		assert.Equal(t, expected, model.Current.State, model.Name)
		assert.Equal(t, expected, model.Summary.State, model.Name)
		observed := false
		for _, bucket := range model.Buckets {
			if bucket.SuccessRate != nil {
				assert.Equal(t, expected, bucket.State, model.Name)
				observed = true
			}
		}
		assert.True(t, observed, model.Name)
		delete(want, model.Name)
	}
	assert.Empty(t, want)
}

func TestAvailabilityCatalogHistoryAndOwnership(t *testing.T) {
	now := setupAvailabilityTest(t)
	require.NoError(t, DB.Create(&ChannelGroup{Id: 3, Name: "Beta", Enabled: true}).Error)
	require.NoError(t, DB.Create(&ChannelGroupChannel{ChannelGroupId: 3, ChannelId: 10}).Error)
	require.NoError(t, DB.Create(&Channel{Id: 11, Name: "other-private", Key: "other-secret", Status: common.ChannelStatusEnabled, Models: "b", TokenBillingMultiplier: 1}).Error)
	require.NoError(t, DB.Create(&ChannelGroupChannel{ChannelGroupId: 3, ChannelId: 11}).Error)
	catalog, err := LoadAvailabilityCatalog(context.Background())
	require.NoError(t, err)
	require.Len(t, catalog.Groups, 2, "all multipliers equal one still yields visible groups")
	assert.Equal(t, 2, catalog.ChannelGroups[10])
	assert.ElementsMatch(t, []string{"a", "b"}, catalog.Groups[0].Models)
	assert.ElementsMatch(t, []int{10, 11}, catalog.NamedGroups[DefaultChannelGroupName])
	assert.NotContains(t, catalog.ChannelNames[10], "private-upstream")
	availabilityObserve(t, now, "old-failure", "retired", AvailabilityFailure, nil)
	require.NoError(t, DB.Where("channel_group_id = ? AND channel_id = ?", 2, 10).Delete(&ChannelGroupChannel{}).Error)
	// Keep Alpha visible through a different enabled channel. Its historical
	// failed request must not follow channel 10 to Beta.
	require.NoError(t, DB.Create(&Channel{Id: 12, Name: "replacement", Key: "replacement-secret", Status: common.ChannelStatusEnabled, Models: "fresh", TokenBillingMultiplier: 1}).Error)
	require.NoError(t, DB.Create(&ChannelGroupChannel{ChannelGroupId: 2, ChannelId: 12}).Error)
	InvalidateAvailabilityCatalog()
	currentCatalog, err := LoadAvailabilityCatalog(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 3, currentCatalog.ChannelGroups[10])
	assert.Equal(t, 2, catalog.ChannelGroups[10], "existing snapshot must remain frozen")
	report, err := GetAvailabilityReport(context.Background(), "1h", now)
	require.NoError(t, err)
	require.Len(t, report.Groups, 2)
	assert.Equal(t, "unhealthy", report.Groups[0].Current.State)
	assert.Equal(t, "unknown", report.Groups[1].Current.State)
	require.Len(t, report.Groups[0].Models, 2)
	assert.Equal(t, "fresh", report.Groups[0].Models[0].Name)
	assert.True(t, report.Groups[0].Models[0].Active)
	assert.Equal(t, "unknown", report.Groups[0].Models[0].Current.State)
	assert.Equal(t, "retired", report.Groups[0].Models[1].Name)
	assert.False(t, report.Groups[0].Models[1].Active)
	require.NoError(t, DB.Delete(&ChannelGroup{}, 2).Error)
	report, err = GetAvailabilityReport(context.Background(), "1h", now)
	require.NoError(t, err)
	require.Len(t, report.Groups, 1, "deleted group must bypass cached history")
	assert.Equal(t, 3, report.Groups[0].ID)
	require.NoError(t, DB.Model(&Channel{}).Where("id IN ?", []int{10, 11}).Update("status", common.ChannelStatusManuallyDisabled).Error)
	report, err = GetAvailabilityReport(context.Background(), "1h", now)
	require.NoError(t, err)
	assert.Empty(t, report.Groups, "group without enabled channels is no longer publicly visible")
}

func TestAvailabilityCoverageAndLateFinal(t *testing.T) {
	now := setupAvailabilityTest(t).Add(48 * time.Hour)
	ctx := context.Background()
	availabilityObserve(t, now, "good", "a", AvailabilitySuccess, nil)
	pending := AvailabilityObservation{ID: "late", GroupID: 2, ModelName: "a", StartedAt: now.Unix() - 25*3600, Outcome: "pending"}
	require.NoError(t, RecordAvailability(ctx, pending))
	report, err := GetAvailabilityReport(ctx, "1h", now)
	require.NoError(t, err)
	assert.Equal(t, "incomplete", report.Groups[0].Current.State)
	assert.Equal(t, "incomplete", report.Groups[0].Summary.Coverage)
	assert.Equal(t, "complete", report.Groups[0].Models[1].Current.Coverage, "unrelated known model stays complete")
	pending.Outcome, pending.CompletedAt = AvailabilitySuccess, now.Unix()-1
	require.NoError(t, RecordAvailability(ctx, pending))
	report, err = GetAvailabilityReport(ctx, "1h", now.Add(61*time.Second))
	require.NoError(t, err)
	assert.Equal(t, "healthy", report.Groups[0].Current.State)
	MarkAvailabilityGap(now.Unix()-10, now.Unix())
	report, err = GetAvailabilityReport(ctx, "1h", now.Add(62*time.Second))
	require.NoError(t, err)
	assert.Equal(t, "incomplete", report.Groups[0].Current.State)
	require.NoError(t, MaintainAvailability(ctx, now))
	report, err = GetAvailabilityReport(ctx, "1h", now.Add(63*time.Second))
	require.NoError(t, err)
	assert.Equal(t, "incomplete", report.Groups[0].Current.State, "recovered storage preserves failed-write evidence")
	before := report.CoverageStart
	require.NoError(t, MigrateAvailability(LOG_DB))
	report, err = GetAvailabilityReport(ctx, "30d", now)
	require.NoError(t, err)
	assert.Equal(t, before, report.CoverageStart)
	assert.Equal(t, "incomplete", report.Groups[0].Summary.State)
}

func TestAvailabilityExactBoundariesAndLastObservation(t *testing.T) {
	now := setupAvailabilityTest(t).Truncate(time.Minute).Add(37 * time.Second)
	ctx := context.Background()
	for _, item := range []struct {
		id      string
		at      int64
		outcome string
	}{
		{"before", now.Unix() - 3601, AvailabilityFailure},
		{"start", now.Unix() - 3600, AvailabilitySuccess},
		{"before-current", now.Unix() - 901, AvailabilityFailure},
		{"current", now.Unix() - 900, AvailabilitySuccess},
		{"end", now.Unix(), AvailabilityFailure},
	} {
		require.NoError(t, RecordAvailability(ctx, AvailabilityObservation{ID: item.id, StartedAt: item.at - 1, CompletedAt: item.at, GroupID: 2, ModelName: "a", Outcome: item.outcome}))
	}
	old := now.Unix() - 7200
	require.NoError(t, RecordAvailability(ctx, AvailabilityObservation{ID: "old-only", StartedAt: old - 1, CompletedAt: old, GroupID: 2, ModelName: "b", Outcome: AvailabilitySuccess}))
	report, err := GetAvailabilityReport(ctx, "1h", now)
	require.NoError(t, err)
	g := report.Groups[0]
	require.NotNil(t, g.Current.SuccessRate)
	assert.Equal(t, 100.0, *g.Current.SuccessRate)
	require.NotNil(t, g.Summary.SuccessRate)
	assert.InDelta(t, 2.0/3*100, *g.Summary.SuccessRate, 0.00001)
	assert.Equal(t, now.Unix()-3600, g.Buckets[0].Start)
	assert.Equal(t, now.Unix(), g.Buckets[len(g.Buckets)-1].End)
	for i := 1; i < len(g.Buckets); i++ {
		assert.Equal(t, g.Buckets[i-1].End, g.Buckets[i].Start)
	}
	assert.Equal(t, "unknown", g.Models[1].Current.State)
	require.NotNil(t, g.Models[1].Current.LastObservedAt)
	assert.Equal(t, old, *g.Models[1].Current.LastObservedAt)
}

func TestAvailabilityUnknownOwnershipAndExcludedTraffic(t *testing.T) {
	now := setupAvailabilityTest(t).Truncate(time.Minute).Add(37 * time.Second)
	ctx := context.Background()
	availabilityObserve(t, now, "excluded-only", "b", AvailabilityExcluded, nil)
	require.NoError(t, RecordAvailability(ctx, AvailabilityObservation{ID: "unowned", StartedAt: now.Unix() - 10, CompletedAt: now.Unix() - 1, GroupID: 0, ModelName: "not-a-public-model", Outcome: AvailabilityFailure}))
	report, err := GetAvailabilityReport(ctx, "1h", now)
	require.NoError(t, err)
	assert.Equal(t, "incomplete", report.Groups[0].Current.State)
	assert.Nil(t, report.Groups[0].Current.SuccessRate)
	assert.False(t, report.Groups[0].Current.HasFailures, "unattributable failure must not be assigned to this group")
	require.Len(t, report.Groups[0].Models, 2, "unowned model must not leak into visible model catalogs")
	for _, m := range report.Groups[0].Models {
		assert.Equal(t, "incomplete", m.Current.Coverage)
		assert.Nil(t, m.Current.SuccessRate)
		assert.Nil(t, m.Current.LastObservedAt)
	}
}

func TestAvailabilityUnknownOutsideCurrentDoesNotTaintCurrent(t *testing.T) {
	now := setupAvailabilityTest(t).Truncate(time.Minute).Add(37 * time.Second)
	ctx := context.Background()
	availabilityObserve(t, now, "successful", "a", AvailabilitySuccess, nil)
	require.NoError(t, RecordAvailability(ctx, AvailabilityObservation{ID: "unknown-before-current", StartedAt: now.Unix() - 902, CompletedAt: now.Unix() - 901, GroupID: 2, ModelName: "a", Outcome: AvailabilityUnknown}))
	report, err := GetAvailabilityReport(ctx, "1h", now)
	require.NoError(t, err)
	assert.Equal(t, "healthy", report.Groups[0].Current.State)
	assert.Equal(t, "incomplete", report.Groups[0].Summary.State)
	assert.Equal(t, "healthy", report.Groups[0].Models[0].Current.State)
}

func TestAvailabilityEvenMedianAndImmutableCachedReport(t *testing.T) {
	now := setupAvailabilityTest(t)
	zero, ten := int64(0), int64(10)
	availabilityObserve(t, now, "median-zero", "a", AvailabilitySuccess, &zero)
	availabilityObserve(t, now, "median-ten", "a", AvailabilitySuccess, &ten)
	availabilityObserve(t, now, "median-missing", "a", AvailabilitySuccess, nil)
	first, err := GetAvailabilityReport(context.Background(), "1h", now)
	require.NoError(t, err)
	require.NotNil(t, first.Groups[0].Current.FirstResponseMs)
	assert.Equal(t, 5.0, *first.Groups[0].Current.FirstResponseMs)
	first.Groups[0].Name = "mutated-by-caller"
	*first.Groups[0].Current.SuccessRate = 0
	second, err := GetAvailabilityReport(context.Background(), "1h", now.Add(time.Second))
	require.NoError(t, err)
	assert.Equal(t, "Alpha", second.Groups[0].Name)
	assert.Equal(t, 100.0, *second.Groups[0].Current.SuccessRate)
	assert.Equal(t, now.Unix(), second.GeneratedAt)
	assert.Equal(t, now.Unix(), second.WindowEnd)
}

func TestAvailabilityLongRangeRollupsAndMaintenance(t *testing.T) {
	now := setupAvailabilityTest(t).Add(40 * 24 * time.Hour).Truncate(time.Minute).Add(17 * time.Second)
	ctx := context.Background()
	for _, item := range []struct {
		id      string
		age     time.Duration
		outcome string
	}{
		{"recent", time.Minute, AvailabilitySuccess},
		{"yesterday", 23 * time.Hour, AvailabilityFailure},
		{"last-week", 6 * 24 * time.Hour, AvailabilitySuccess},
		{"last-month", 29 * 24 * time.Hour, AvailabilityFailure},
		{"expired", 32 * 24 * time.Hour, AvailabilityFailure},
	} {
		at := now.Add(-item.age).Unix()
		require.NoError(t, RecordAvailability(ctx, AvailabilityObservation{ID: item.id, StartedAt: at - 2, CompletedAt: at, GroupID: 2, ModelName: "a", Outcome: item.outcome}))
	}
	require.NoError(t, MaintainAvailability(ctx, now))
	require.NoError(t, MaintainAvailability(ctx, now))
	for _, test := range []struct {
		rangeName string
		rate      float64
	}{{"1h", 100}, {"24h", 50}, {"7d", 200.0 / 3}, {"30d", 50}} {
		report, err := GetAvailabilityReport(ctx, test.rangeName, now)
		require.NoError(t, err)
		require.NotNil(t, report.Groups[0].Summary.SuccessRate)
		assert.InDelta(t, test.rate, *report.Groups[0].Summary.SuccessRate, 0.00001)
		assert.Equal(t, "complete", report.Groups[0].Summary.Coverage)
		assert.Equal(t, 100.0, *report.Groups[0].Current.SuccessRate)
		buckets := report.Groups[0].Buckets
		assert.Equal(t, report.WindowStart, buckets[0].Start)
		assert.Equal(t, report.WindowEnd, buckets[len(buckets)-1].End)
		for i := 1; i < len(buckets); i++ {
			assert.Equal(t, buckets[i-1].End, buckets[i].Start)
		}
	}
}

func TestAvailabilityFinalAttemptCanChangePendingOwnership(t *testing.T) {
	now := setupAvailabilityTest(t)
	ctx := context.Background()
	require.NoError(t, DB.Create(&ChannelGroup{Id: 3, Name: "Beta", Enabled: true}).Error)
	require.NoError(t, DB.Create(&Channel{Id: 11, Name: "other", Key: "secret", Models: "a", Status: common.ChannelStatusEnabled, TokenBillingMultiplier: 1}).Error)
	require.NoError(t, DB.Create(&ChannelGroupChannel{ChannelGroupId: 3, ChannelId: 11}).Error)
	observation := AvailabilityObservation{ID: "retry-across-groups", StartedAt: now.Unix() - 120, GroupID: 2, ModelName: "a", Outcome: "pending"}
	require.NoError(t, RecordAvailability(ctx, observation))
	observation.GroupID, observation.Outcome, observation.CompletedAt = 3, AvailabilitySuccess, now.Unix()-1
	require.NoError(t, RecordAvailability(ctx, observation))
	// Even a contradictory final retry must not rewrite the first terminal result.
	observation.GroupID, observation.Outcome = 2, AvailabilityFailure
	require.NoError(t, RecordAvailability(ctx, observation))
	report, err := GetAvailabilityReport(ctx, "1h", now)
	require.NoError(t, err)
	require.Len(t, report.Groups, 2)
	assert.Equal(t, "unknown", report.Groups[0].Current.State)
	assert.Equal(t, "healthy", report.Groups[1].Current.State)
	assert.False(t, report.Groups[0].Current.HasFailures)
}

func TestAvailabilityExplicitDefaultAndDisabledChannelSnapshot(t *testing.T) {
	setupAvailabilityTest(t)
	require.NoError(t, DB.Create(&Channel{Id: 11, Name: "disabled-upstream", Key: "hidden", Models: "legacy", Status: common.ChannelStatusManuallyDisabled, TokenBillingMultiplier: 1}).Error)
	require.NoError(t, DB.Create(&ChannelGroupChannel{ChannelGroupId: 2, ChannelId: 11}).Error)
	require.NoError(t, DB.Create(&ChannelGroupChannel{ChannelGroupId: 1, ChannelId: 11}).Error)
	catalog, err := LoadAvailabilityCatalog(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []int{11}, catalog.NamedGroups[DefaultChannelGroupName])
	assert.NotContains(t, catalog.ChannelNames[10], DefaultChannelGroupName)
	assert.Contains(t, catalog.ChannelNames[11], DefaultChannelGroupName)
	assert.Equal(t, 2, catalog.ChannelGroups[11], "disabled channel keeps failure attribution")
	require.Len(t, catalog.Groups, 1)
	assert.NotContains(t, catalog.Groups[0].Models, "legacy", "disabled channel does not advertise an active model")
}

func TestAvailabilityRetiredModelOutsideSelectedWindow(t *testing.T) {
	now := setupAvailabilityTest(t)
	at := now.Unix() - 7200
	require.NoError(t, RecordAvailability(context.Background(), AvailabilityObservation{ID: "retired-two-hours-ago", StartedAt: at - 1, CompletedAt: at, GroupID: 2, ModelName: "retired", Outcome: AvailabilityFailure}))
	report, err := GetAvailabilityReport(context.Background(), "1h", now)
	require.NoError(t, err)
	require.Len(t, report.Groups[0].Models, 2)
	for _, model := range report.Groups[0].Models {
		assert.NotEqual(t, "retired", model.Name)
	}
	report, err = GetAvailabilityReport(context.Background(), "24h", now)
	require.NoError(t, err)
	require.Len(t, report.Groups[0].Models, 3)
	retired := report.Groups[0].Models[2]
	assert.Equal(t, "retired", retired.Name)
	assert.False(t, retired.Active)
	assert.Equal(t, "unknown", retired.Current.State)
	require.NotNil(t, retired.Current.LastObservedAt)
	assert.Equal(t, at, *retired.Current.LastObservedAt)
}

func TestAvailabilityAbandonedPendingStopsTaintingFuture(t *testing.T) {
	now := setupAvailabilityTest(t).Add(10 * 24 * time.Hour)
	ctx := context.Background()
	pending := AvailabilityObservation{ID: "abandoned", StartedAt: now.Unix() - 72*3600, GroupID: 2, ModelName: "a", Outcome: "pending"}
	require.NoError(t, RecordAvailability(ctx, pending))
	availabilityObserve(t, now, "current-good", "a", AvailabilitySuccess, nil)
	report, err := GetAvailabilityReport(ctx, "1h", now)
	require.NoError(t, err)
	assert.Equal(t, "healthy", report.Groups[0].Current.State)
	report, err = GetAvailabilityReport(ctx, "7d", now)
	require.NoError(t, err)
	assert.Equal(t, "incomplete", report.Groups[0].Summary.Coverage)
	// Maintenance must preserve the historical gap even though today's current
	// window is complete. The seven-day range is entirely after migration.
	require.NoError(t, MaintainAvailability(ctx, now))
	report, err = GetAvailabilityReport(ctx, "7d", now)
	require.NoError(t, err)
	assert.Equal(t, "incomplete", report.Groups[0].Summary.Coverage)
	for _, bucket := range report.Groups[0].Buckets {
		gapStart, gapEnd := pending.StartedAt+24*3600, pending.StartedAt+48*3600
		if bucket.Start < gapEnd && bucket.End > gapStart {
			assert.Equal(t, "incomplete", bucket.Coverage)
		}
	}
	pending.CompletedAt, pending.Outcome = now.Unix()-1, AvailabilityFailure
	require.NoError(t, RecordAvailability(ctx, pending))
	report, err = GetAvailabilityReport(ctx, "1h", now.Add(61*time.Second))
	require.NoError(t, err)
	assert.Equal(t, 50.0, *report.Groups[0].Current.SuccessRate)
	assert.Equal(t, "complete", report.Groups[0].Current.Coverage)
}

func TestAvailabilityChannelMutationRefreshesFutureSnapshots(t *testing.T) {
	setupAvailabilityTest(t)
	before, err := LoadAvailabilityCatalog(context.Background())
	require.NoError(t, err)
	var channel Channel
	require.NoError(t, DB.First(&channel, 10).Error)
	channel.Models = "replacement"
	require.NoError(t, channel.Save())
	after, err := LoadAvailabilityCatalog(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []string{"replacement"}, after.Groups[0].Models)
	assert.Equal(t, []string{"a", "b"}, before.Groups[0].Models, "in-flight snapshot must remain unchanged")
	channel.Status = common.ChannelStatusManuallyDisabled
	require.NoError(t, channel.SaveWithoutKey())
	after, err = LoadAvailabilityCatalog(context.Background())
	require.NoError(t, err)
	assert.Empty(t, after.Groups)
}

func TestAvailabilityFinalReplayPreservesFirstOutcome(t *testing.T) {
	for _, pendingFirst := range []bool{false, true} {
		for _, outcome := range []string{AvailabilitySuccess, AvailabilityFailure, AvailabilityExcluded, AvailabilityUnknown} {
			t.Run(fmt.Sprintf("pending=%t/%s", pendingFirst, outcome), func(t *testing.T) {
				now := setupAvailabilityTest(t)
				ctx := context.Background()
				pending := AvailabilityObservation{ID: "replayed-terminal", StartedAt: now.Unix() - 30, GroupID: 2, ModelName: "a", Outcome: availabilityPending}
				if pendingFirst {
					require.NoError(t, RecordAvailability(ctx, pending))
				}
				zero := int64(0)
				final := pending
				final.CompletedAt, final.Outcome, final.FirstResponseMs = now.Unix()-1, outcome, &zero
				final.GroupID, final.ModelName, final.Reason = 3, "retry-model", "finished"
				require.NoError(t, RecordAvailability(ctx, final))
				var first availabilityRequest
				require.NoError(t, LOG_DB.First(&first, "id = ?", pending.ID).Error)
				require.Equal(t, pending.StartedAt, first.StartedAt)
				require.Equal(t, final.CompletedAt, first.CompletedAt)
				require.Equal(t, outcome, first.Outcome)
				require.Equal(t, final.GroupID, first.GroupID)
				require.Equal(t, final.ModelName, first.ModelName)
				require.NotNil(t, first.FirstResponseMs)
				require.Zero(t, *first.FirstResponseMs)
				// Neither a late start nor a contradictory terminal may rewrite the winner.
				require.NoError(t, RecordAvailability(ctx, pending))
				replay := final
				replay.Outcome, replay.GroupID, replay.ModelName = AvailabilityFailure, 2, "a"
				replay.CompletedAt, replay.FirstResponseMs, replay.Reason = now.Unix(), nil, "contradictory"
				require.NoError(t, RecordAvailability(ctx, replay))
				var stored availabilityRequest
				require.NoError(t, LOG_DB.First(&stored, "id = ?", pending.ID).Error)
				require.Equal(t, first, stored)
				var frequencies []availabilityFrequency
				require.NoError(t, LOG_DB.Find(&frequencies).Error)
				if outcome == AvailabilityExcluded {
					require.Empty(t, frequencies)
				} else {
					require.Len(t, frequencies, 1)
					require.Equal(t, int64(1), frequencies[0].Frequency)
					require.Equal(t, outcome, frequencies[0].Outcome)
					require.Equal(t, final.GroupID, frequencies[0].GroupID)
					require.Equal(t, final.ModelName, frequencies[0].ModelName)
					require.Equal(t, final.CompletedAt, frequencies[0].LastObserved)
					if outcome == AvailabilitySuccess {
						require.Zero(t, frequencies[0].Latency)
					} else {
						require.Equal(t, int64(-1), frequencies[0].Latency)
					}
				}
			})
		}
	}
}

func TestAvailabilityRollupFailureRollsBackTerminal(t *testing.T) {
	for _, pendingFirst := range []bool{false, true} {
		t.Run(fmt.Sprintf("pending=%t", pendingFirst), func(t *testing.T) {
			now := setupAvailabilityTest(t)
			ctx := context.Background()
			observation := AvailabilityObservation{ID: "rollup-rollback", StartedAt: now.Unix() - 30, GroupID: 2, ModelName: "a", Outcome: availabilityPending}
			if pendingFirst {
				require.NoError(t, RecordAvailability(ctx, observation))
			}
			injected := errors.New("injected availability rollup failure")
			const callback = "test:availability-rollup-failure"
			require.NoError(t, LOG_DB.Callback().Create().After("gorm:create").Register(callback, func(tx *gorm.DB) {
				if tx.Statement.Table == "availability_frequencies" {
					tx.AddError(injected)
				}
			}))
			t.Cleanup(func() { LOG_DB.Callback().Create().Remove(callback) })
			observation.Outcome, observation.CompletedAt = AvailabilitySuccess, now.Unix()-1
			require.ErrorIs(t, RecordAvailability(ctx, observation), injected)
			var rows []availabilityRequest
			require.NoError(t, LOG_DB.Find(&rows).Error)
			if pendingFirst {
				require.Len(t, rows, 1)
				require.Equal(t, availabilityPending, rows[0].Outcome)
				require.Zero(t, rows[0].CompletedAt)
			} else {
				require.Empty(t, rows)
			}
			var frequencyCount int64
			require.NoError(t, LOG_DB.Model(&availabilityFrequency{}).Count(&frequencyCount).Error)
			require.Zero(t, frequencyCount)
			require.NoError(t, LOG_DB.Callback().Create().Remove(callback))
			require.NoError(t, RecordAvailability(ctx, observation))
			require.NoError(t, RecordAvailability(ctx, observation))
			var frequencies []availabilityFrequency
			require.NoError(t, LOG_DB.Find(&frequencies).Error)
			require.Len(t, frequencies, 1)
			require.Equal(t, int64(1), frequencies[0].Frequency)
		})
	}
}

func TestAvailabilityConcurrentStartAndFinalClaimOnce(t *testing.T) {
	for _, pendingFirst := range []bool{false, true} {
		t.Run(fmt.Sprintf("pending=%t", pendingFirst), func(t *testing.T) {
			now := setupAvailabilityTest(t)
			if LOG_DB.Dialector.Name() != "sqlite" {
				pool, err := LOG_DB.DB()
				require.NoError(t, err)
				pool.SetMaxOpenConns(8)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			pending := AvailabilityObservation{ID: "concurrent-terminal", StartedAt: now.Unix() - 30, GroupID: 2, ModelName: "a", Outcome: availabilityPending}
			if pendingFirst {
				require.NoError(t, RecordAvailability(ctx, pending))
			}
			start := make(chan struct{})
			errs := make(chan error, 12)
			var ready sync.WaitGroup
			ready.Add(cap(errs))
			for i := 0; i < cap(errs); i++ {
				observation := pending
				if i%3 != 0 {
					observation.CompletedAt = now.Unix() - 1
					observation.Outcome = AvailabilitySuccess
					if i%3 == 2 {
						observation.Outcome = AvailabilityFailure
					}
				}
				go func(observation AvailabilityObservation) {
					ready.Done()
					<-start
					errs <- RecordAvailability(ctx, observation)
				}(observation)
			}
			ready.Wait()
			close(start)
			for i := 0; i < cap(errs); i++ {
				assert.NoError(t, <-errs)
			}
			var rows []availabilityRequest
			require.NoError(t, LOG_DB.Find(&rows).Error)
			require.Len(t, rows, 1)
			require.Contains(t, []string{AvailabilitySuccess, AvailabilityFailure}, rows[0].Outcome)
			var frequencies []availabilityFrequency
			require.NoError(t, LOG_DB.Find(&frequencies).Error)
			require.Len(t, frequencies, 1)
			require.Equal(t, rows[0].Outcome, frequencies[0].Outcome)
			require.Equal(t, int64(1), frequencies[0].Frequency)
		})
	}
}
func TestRecoverOrphanedAvailabilityPendingPreservesAsyncTasks(t *testing.T) {
	setupAvailabilityTest(t)
	require.NoError(t, DB.AutoMigrate(&Task{}))
	ctx := context.Background()
	processStartedAt := time.Now().Unix()

	orphan := AvailabilityObservation{ID: "orphaned-process-request", StartedAt: processStartedAt - availabilityPendingSeconds - 60, GroupID: 2, ModelName: "a", Outcome: availabilityPending}
	require.NoError(t, RecordAvailability(ctx, orphan))

	legacyAsync := orphan
	legacyAsync.ID = "legacy-async-request"
	legacyData, err := common.Marshal(legacyAsync)
	require.NoError(t, err)
	require.NoError(t, DB.Create(&Task{TaskID: "legacy-async", Status: TaskStatusInProgress, AvailabilityData: string(legacyData), AvailabilityPending: true}).Error)
	require.NoError(t, RecordAvailability(ctx, legacyAsync))

	markedAsync := orphan
	markedAsync.ID = "marked-async-request"
	markedAsync.Reason = AvailabilityAsyncPendingReason
	require.NoError(t, RecordAvailability(ctx, markedAsync))

	require.NoError(t, RecoverOrphanedAvailability(ctx, time.Unix(processStartedAt, 0)))

	var rows []availabilityRequest
	require.NoError(t, LOG_DB.Order("id").Find(&rows).Error)
	require.Len(t, rows, 3)
	byID := make(map[string]availabilityRequest, len(rows))
	for _, row := range rows {
		byID[row.ID] = row
	}
	require.Equal(t, AvailabilityUnknown, byID[orphan.ID].Outcome)
	require.Equal(t, AvailabilityProcessInterruptedReason, byID[orphan.ID].Reason)
	require.Equal(t, orphan.StartedAt+availabilityPendingSeconds, byID[orphan.ID].CompletedAt)
	require.Equal(t, availabilityPending, byID[legacyAsync.ID].Outcome)
	require.Equal(t, availabilityPending, byID[markedAsync.ID].Outcome)

	var frequencies []availabilityFrequency
	require.NoError(t, LOG_DB.Find(&frequencies).Error)
	require.Len(t, frequencies, 1)
	require.Equal(t, AvailabilityUnknown, frequencies[0].Outcome)
}
