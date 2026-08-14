package model

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupChannelGroupSelectionTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	oldDB := DB
	oldUsingSQLite := common.UsingSQLite
	oldUsingMySQL := common.UsingMySQL
	oldUsingPostgreSQL := common.UsingPostgreSQL
	oldMemoryCache := common.MemoryCacheEnabled
	oldModel2channels := model2channels
	oldGroupModel2channels := groupModel2channels
	oldChannelsIDM := channelsIDM
	oldDefaultExplicit := defaultGroupHasExplicitMembersCache

	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.MemoryCacheEnabled = true
	initCol()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	DB = db
	require.NoError(t, db.AutoMigrate(&Channel{}, &Ability{}, &Model{}, &ChannelGroup{}, &ChannelGroupChannel{}, &TokenGroupBinding{}))
	require.NoError(t, ensureDefaultChannelGroup())

	t.Cleanup(func() {
		DB = oldDB
		common.UsingSQLite = oldUsingSQLite
		common.UsingMySQL = oldUsingMySQL
		common.UsingPostgreSQL = oldUsingPostgreSQL
		common.MemoryCacheEnabled = oldMemoryCache
		model2channels = oldModel2channels
		groupModel2channels = oldGroupModel2channels
		channelsIDM = oldChannelsIDM
		defaultGroupHasExplicitMembersCache = oldDefaultExplicit
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})

	return db
}

func seedSelectionChannel(t *testing.T, db *gorm.DB, id int, model string) {
	t.Helper()
	priority := int64(0)
	channel := &Channel{Id: id, Type: constant.ChannelTypeOpenAI, Key: "sk-test", Status: common.ChannelStatusEnabled, Name: fmt.Sprintf("ch-%d", id), Models: model, Priority: &priority}
	require.NoError(t, db.Create(channel).Error)
}

type effectiveGroupSQLCaptureLogger struct {
	logger.Interface
	statements []string
}

func (l *effectiveGroupSQLCaptureLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	sql, rows := fc()
	if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(sql)), "SELECT") {
		l.statements = append(l.statements, sql)
	}
	if l.Interface != nil {
		l.Interface.Trace(ctx, begin, func() (string, int64) { return sql, rows }, err)
	}
}

func makeGroup(t *testing.T, name string, channelIds []int) *ChannelGroup {
	t.Helper()
	g := &ChannelGroup{Name: name, Enabled: true, CreditBillingMode: GroupCreditBillingModeInherit}
	require.NoError(t, g.Insert())
	require.NoError(t, SetChannelGroupChannels(g.Id, channelIds))
	// 重建 abilities，使 InitChannelCache 能从 abilities 表读到 (group, model, channel) 行。
	_, _, err := FixAbility()
	require.NoError(t, err)
	return g
}

func TestGroupSelectionFiltersByGroupMembership(t *testing.T) {
	db := setupChannelGroupSelectionTestDB(t)
	const model = "gpt-grp"
	seedSelectionChannel(t, db, 5001, model)
	seedSelectionChannel(t, db, 5002, model)
	makeGroup(t, "alpha", []int{5001})
	makeGroup(t, "beta", []int{5002})
	InitChannelCache()

	// alpha → only 5001
	ch, err := GetRandomSatisfiedChannelForEndpointWithGroups([]string{"alpha"}, model, 0, "", nil, 0, false, ChannelBillingProfile{}, false)
	require.NoError(t, err)
	require.NotNil(t, ch)
	assert.Equal(t, 5001, ch.Id)

	// beta → only 5002
	ch, err = GetRandomSatisfiedChannelForEndpointWithGroups([]string{"beta"}, model, 0, "", nil, 0, false, ChannelBillingProfile{}, false)
	require.NoError(t, err)
	require.NotNil(t, ch)
	assert.Equal(t, 5002, ch.Id)
}

func TestGroupSelectionUnionAcrossMultipleGroups(t *testing.T) {
	db := setupChannelGroupSelectionTestDB(t)
	const model = "gpt-union"
	seedSelectionChannel(t, db, 5101, model)
	seedSelectionChannel(t, db, 5102, model)
	seedSelectionChannel(t, db, 5103, model)
	makeGroup(t, "g1", []int{5101})
	makeGroup(t, "g2", []int{5102})
	InitChannelCache()

	seen := map[int]bool{}
	for i := 0; i < 50; i++ {
		ch, err := GetRandomSatisfiedChannelForEndpointWithGroups([]string{"g1", "g2"}, model, 0, "", nil, 0, false, ChannelBillingProfile{}, false)
		require.NoError(t, err)
		require.NotNil(t, ch)
		seen[ch.Id] = true
	}
	assert.True(t, seen[5101], "union should include g1 channel")
	assert.True(t, seen[5102], "union should include g2 channel")
	assert.False(t, seen[5103], "union must exclude channel outside both groups")
}

func TestDefaultGroupWithoutExplicitMembersAllowsAllChannels(t *testing.T) {
	db := setupChannelGroupSelectionTestDB(t)
	const model = "gpt-default"
	seedSelectionChannel(t, db, 5201, model)
	seedSelectionChannel(t, db, 5202, model)
	// no explicit default members → default group = all channels
	InitChannelCache()

	seen := map[int]bool{}
	for i := 0; i < 50; i++ {
		ch, err := GetRandomSatisfiedChannelForEndpointWithGroups([]string{DefaultChannelGroupName}, model, 0, "", nil, 0, false, ChannelBillingProfile{}, false)
		require.NoError(t, err)
		require.NotNil(t, ch)
		seen[ch.Id] = true
	}
	assert.True(t, seen[5201])
	assert.True(t, seen[5202])
}

func TestDefaultGroupWithExplicitMembersNarrowsChannels(t *testing.T) {
	db := setupChannelGroupSelectionTestDB(t)
	const model = "gpt-default-narrow"
	seedSelectionChannel(t, db, 5301, model)
	seedSelectionChannel(t, db, 5302, model)
	defGroup, err := GetChannelGroupByName(DefaultChannelGroupName)
	require.NoError(t, err)
	require.NoError(t, SetChannelGroupChannels(defGroup.Id, []int{5301}))
	// 默认分组改为显式成员后，重建 abilities 使 cache 反映收窄。
	_, _, err = FixAbility()
	require.NoError(t, err)
	InitChannelCache()

	for i := 0; i < 30; i++ {
		ch, err := GetRandomSatisfiedChannelForEndpointWithGroups([]string{DefaultChannelGroupName}, model, 0, "", nil, 0, false, ChannelBillingProfile{}, false)
		require.NoError(t, err)
		require.NotNil(t, ch)
		assert.Equal(t, 5301, ch.Id, "explicit default members must narrow to listed channels")
	}
}

func TestGroupSelectionEmptyGroupsFallsBackToAllChannels(t *testing.T) {
	db := setupChannelGroupSelectionTestDB(t)
	const model = "gpt-empty"
	seedSelectionChannel(t, db, 5401, model)
	makeGroup(t, "iso", []int{5401})
	InitChannelCache()

	ch, err := GetRandomSatisfiedChannelForEndpointWithGroups(nil, model, 0, "", nil, 0, false, ChannelBillingProfile{}, false)
	require.NoError(t, err)
	require.NotNil(t, ch)
	assert.Equal(t, 5401, ch.Id)
}

func TestGetEffectiveGroupNamesByTokenFallsBackToDefault(t *testing.T) {
	setupChannelGroupSelectionTestDB(t)
	names, err := GetEffectiveGroupNamesByToken(99999)
	require.NoError(t, err)
	assert.Equal(t, []string{DefaultChannelGroupName}, names)
}

func TestGetEffectiveGroupNamesByTokenReturnsBoundEnabledGroups(t *testing.T) {
	setupChannelGroupSelectionTestDB(t)
	g := makeGroup(t, "bound", nil)
	require.NoError(t, SetTokenGroupBindings(7001, []int{g.Id}))
	names, err := GetEffectiveGroupNamesByToken(7001)
	require.NoError(t, err)
	assert.Equal(t, []string{"bound"}, names)
}

func TestGetEffectiveGroupNamesByTokenDisabledBindingsReturnSentinel(t *testing.T) {
	setupChannelGroupSelectionTestDB(t)
	g := makeGroup(t, "soon-disabled", nil)
	g.Enabled = false
	require.NoError(t, g.Update())
	require.NoError(t, SetTokenGroupBindings(7101, []int{g.Id}))

	names, err := GetEffectiveGroupNamesByToken(7101)
	require.NoError(t, err)
	// 已绑定但全部禁用：返回哨兵分组（拒绝），绝不回落默认分组。
	assert.Equal(t, []string{DisabledTokenGroupSentinel}, names)
	assert.NotContains(t, names, DefaultChannelGroupName)
}

func TestGroupSelectionDisabledSentinelDeniesAllChannels(t *testing.T) {
	db := setupChannelGroupSelectionTestDB(t)
	const model = "gpt-disabled-deny"
	seedSelectionChannel(t, db, 7201, model)
	seedSelectionChannel(t, db, 7202, model)
	makeGroup(t, "served", []int{7201, 7202})
	InitChannelCache()

	// 哨兵分组不匹配任何渠道：缓存路径拒绝。
	ch, err := GetRandomSatisfiedChannelForEndpointWithGroups([]string{DisabledTokenGroupSentinel}, model, 0, "", nil, 0, false, ChannelBillingProfile{}, false)
	require.NoError(t, err)
	assert.Nil(t, ch, "disabled sentinel must not widen to any channel via cache path")

	// DB 路径同样拒绝。
	oldMemory := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false
	t.Cleanup(func() { common.MemoryCacheEnabled = oldMemory })
	ch, err = GetChannelForEndpointWithGroups([]string{DisabledTokenGroupSentinel}, model, 0, "", nil, 0, false, ChannelBillingProfile{}, false)
	require.NoError(t, err)
	assert.Nil(t, ch, "disabled sentinel must not widen to any channel via DB path")
}

func TestDefaultGroupWithoutExplicitMembersDBEndpointUsesLegacyAbilityRows(t *testing.T) {
	db := setupChannelGroupSelectionTestDB(t)
	const modelName = "gpt-default-db-endpoint"
	priority := int64(0)
	channel := &Channel{Id: 7301, Type: constant.ChannelTypeCodex, Key: "sk-test", Status: common.ChannelStatusEnabled, Name: "legacy-default", Models: modelName, Priority: &priority}
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, db.Create(&Ability{Group: legacyAbilityGroup, Model: modelName, ChannelId: channel.Id, Enabled: true, Priority: &priority}).Error)

	oldMemory := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false
	t.Cleanup(func() { common.MemoryCacheEnabled = oldMemory })

	names, err := GetEffectiveGroupNamesByToken(99999)
	require.NoError(t, err)
	require.Equal(t, []string{DefaultChannelGroupName}, names)

	selected, err := GetRandomSatisfiedChannelForEndpointWithGroups(names, modelName, 0, constant.EndpointTypeOpenAIResponse, nil, 0, false, ChannelBillingProfile{}, false)
	require.NoError(t, err)
	require.NotNil(t, selected)
	assert.Equal(t, channel.Id, selected.Id)
	assert.True(t, IsChannelEnabledForAnyGroupModel(names, modelName, channel.Id))
}

func TestChannelGroupQueriesQuoteReservedGroupColumnForPostgres(t *testing.T) {
	setupChannelGroupSelectionTestDB(t)

	baseDB := DB
	oldUsingSQLite := common.UsingSQLite
	oldUsingMySQL := common.UsingMySQL
	oldUsingPostgreSQL := common.UsingPostgreSQL
	oldMemoryCache := common.MemoryCacheEnabled
	oldModel2channels := model2channels
	oldGroupModel2channels := groupModel2channels
	oldChannelsIDM := channelsIDM
	oldDefaultExplicit := defaultGroupHasExplicitMembersCache

	baseStmt := baseDB.Session(&gorm.Session{DryRun: true}).Find(&[]Ability{}).Statement
	capture := &sqlCaptureLogger{}
	pgDB, err := gorm.Open(postgres.New(postgres.Config{
		DSN:                  "host=localhost user=test dbname=test sslmode=disable",
		PreferSimpleProtocol: true,
		Conn:                 baseStmt.ConnPool,
	}), &gorm.Config{DryRun: true, Logger: capture})
	require.NoError(t, err)

	common.UsingSQLite = false
	common.UsingMySQL = false
	common.UsingPostgreSQL = true
	common.MemoryCacheEnabled = true
	initCol()
	DB = pgDB
	model2channels = nil
	groupModel2channels = nil
	channelsIDM = nil
	defaultGroupHasExplicitMembersCache = false
	t.Cleanup(func() {
		DB = baseDB
		common.UsingSQLite = oldUsingSQLite
		common.UsingMySQL = oldUsingMySQL
		common.UsingPostgreSQL = oldUsingPostgreSQL
		common.MemoryCacheEnabled = oldMemoryCache
		model2channels = oldModel2channels
		groupModel2channels = oldGroupModel2channels
		channelsIDM = oldChannelsIDM
		defaultGroupHasExplicitMembersCache = oldDefaultExplicit
		initCol()
	})

	InitChannelCache()
	common.MemoryCacheEnabled = false
	_, err = GetRandomSatisfiedChannelForEndpointWithGroups([]string{"GitLab"}, "claude-opus-4-8", 0, "", nil, 0, false, ChannelBillingProfile{}, false)
	require.NoError(t, err)
	assert.False(t, IsChannelEnabledForAnyGroupModel([]string{"GitLab"}, "claude-opus-4-8", 9))

	sql := strings.Join(capture.statements, "\n")
	require.Contains(t, sql, `"group"`)
	require.NotContains(t, sql, "`group`")
}

func TestResolveEffectiveGroupPrefersNonDefault(t *testing.T) {
	db := setupChannelGroupSelectionTestDB(t)
	const model = "gpt-eff"
	seedSelectionChannel(t, db, 6001, model)
	g := makeGroup(t, "paid", []int{6001})
	g.CreditBillingMode = "fixed_request"
	g.FixedRequestCredits = 80_000
	require.NoError(t, g.Update())
	InitChannelCache()

	// token selected both default and paid; channel 6001 in paid → effective = paid (non-default).
	eff, err := ResolveEffectiveGroupForChannel([]string{DefaultChannelGroupName, "paid"}, 6001)
	require.NoError(t, err)
	require.NotNil(t, eff)
	assert.Equal(t, "paid", eff.Name)
	channel := &Channel{Id: 6001, CreditBillingMode: "usage_tokens", TokenBillingMultiplier: 1}
	profile := ResolveEffectiveBillingProfile(eff, channel)
	assert.Equal(t, "fixed_request", profile.CreditBillingMode)
	assert.Equal(t, int64(80_000), profile.FixedRequestCredits)
}

func TestResolveEffectiveGroupForSelectedChannelUsesSingleQuery(t *testing.T) {
	db := setupChannelGroupSelectionTestDB(t)
	const model = "gpt-eff-query-count"
	seedSelectionChannel(t, db, 6051, model)
	makeGroup(t, "paid-query-count", []int{6051})

	capture := &effectiveGroupSQLCaptureLogger{Interface: logger.Default.LogMode(logger.Silent)}
	countedDB := db.Session(&gorm.Session{Logger: capture})
	DB = countedDB
	require.Same(t, countedDB, DB)

	effective, err := ResolveEffectiveGroupForChannel([]string{DefaultChannelGroupName, "paid-query-count"}, 6051)
	require.NoError(t, err)
	require.NotNil(t, effective)
	assert.Equal(t, "paid-query-count", effective.Name)
	require.Len(t, capture.statements, 1, "actual SQL: %#v", capture.statements)
	assert.Contains(t, capture.statements[0], "selected_membership")
}

func TestLoadEffectiveGroupsForChannelGeneratesPortableSQL(t *testing.T) {
	baseDB := setupChannelGroupSelectionTestDB(t)
	baseStatement := baseDB.Session(&gorm.Session{DryRun: true}).Find(&[]ChannelGroup{}).Statement
	tests := []struct {
		name      string
		dialector gorm.Dialector
	}{
		{name: "sqlite", dialector: baseDB.Dialector},
		{name: "mysql", dialector: mysql.New(mysql.Config{
			Conn:                      baseStatement.ConnPool,
			SkipInitializeWithVersion: true,
		})},
		{name: "postgres", dialector: postgres.New(postgres.Config{
			DSN:                  "host=localhost user=test dbname=test sslmode=disable",
			PreferSimpleProtocol: true,
			Conn:                 baseStatement.ConnPool,
		})},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			capture := &effectiveGroupSQLCaptureLogger{Interface: logger.Default.LogMode(logger.Silent)}
			dryRunDB, err := gorm.Open(testCase.dialector, &gorm.Config{DryRun: true, Logger: capture})
			require.NoError(t, err)
			oldDB := DB
			DB = dryRunDB
			defer func() { DB = oldDB }()

			_, err = loadEffectiveGroupsForChannel(6051)
			require.NoError(t, err)
			targetStatements := make([]string, 0, len(capture.statements))
			for _, statement := range capture.statements {
				if strings.Contains(strings.ToLower(statement), "selected_membership") {
					targetStatements = append(targetStatements, statement)
				}
			}
			require.Len(t, targetStatements, 1, "captured SQL: %#v", capture.statements)
			normalized := strings.ToLower(strings.Join(strings.Fields(targetStatements[0]), " "))
			normalized = strings.NewReplacer("\"", "", "`", "").Replace(normalized)
			assert.Contains(t, normalized, "channel_groups.*")
			assert.Contains(t, normalized, "case when selected_membership.channel_id is null")
			assert.Contains(t, normalized, "select count(*) from channel_group_channels")
			assert.Contains(t, normalized, "deleted_at is null")
		})
	}
}

func TestResolveEffectiveGroupDefaultWithoutExplicitMembers(t *testing.T) {
	db := setupChannelGroupSelectionTestDB(t)
	seedSelectionChannel(t, db, 6061, "gpt-eff-default-implicit")

	effective, err := ResolveEffectiveGroupForChannel([]string{DefaultChannelGroupName}, 6061)
	require.NoError(t, err)
	require.NotNil(t, effective)
	assert.True(t, effective.IsDefault())
}

func TestResolveEffectiveGroupDefaultWithExplicitMembersStillFallsBackForUnexpectedChannel(t *testing.T) {
	db := setupChannelGroupSelectionTestDB(t)
	seedSelectionChannel(t, db, 6071, "gpt-eff-default-explicit-target")
	seedSelectionChannel(t, db, 6072, "gpt-eff-default-explicit-member")
	defaultGroup, err := GetChannelGroupByName(DefaultChannelGroupName)
	require.NoError(t, err)
	require.NoError(t, SetChannelGroupChannels(defaultGroup.Id, []int{6072}))

	effective, err := ResolveEffectiveGroupForChannel([]string{DefaultChannelGroupName}, 6071)
	require.NoError(t, err)
	require.NotNil(t, effective)
	assert.True(t, effective.IsDefault())
}

func TestResolveEffectiveGroupMissingDefaultPreservesRecordNotFound(t *testing.T) {
	db := setupChannelGroupSelectionTestDB(t)
	seedSelectionChannel(t, db, 6081, "gpt-eff-default-missing")
	require.NoError(t, db.Unscoped().Where("name = ?", DefaultChannelGroupName).Delete(&ChannelGroup{}).Error)

	_, err := ResolveEffectiveGroupForChannel([]string{DefaultChannelGroupName}, 6081)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
}

func TestResolveEffectiveGroupMissingDefaultStillReturnsMatchedNonDefault(t *testing.T) {
	db := setupChannelGroupSelectionTestDB(t)
	seedSelectionChannel(t, db, 6091, "gpt-eff-paid-without-default")
	paid := makeGroup(t, "paid-without-default", []int{6091})
	paid.CreditBillingMode = "fixed_request"
	paid.FixedRequestCredits = 90_000
	require.NoError(t, paid.Update())
	require.NoError(t, db.Unscoped().Where("name = ?", DefaultChannelGroupName).Delete(&ChannelGroup{}).Error)

	effective, err := ResolveEffectiveGroupForChannel([]string{"paid-without-default"}, 6091)
	require.NoError(t, err)
	require.NotNil(t, effective)
	assert.Equal(t, "paid-without-default", effective.Name)
}

func TestResolveEffectiveGroupInheritFallsBackToChannelProfile(t *testing.T) {
	db := setupChannelGroupSelectionTestDB(t)
	const model = "gpt-eff-inherit"
	seedSelectionChannel(t, db, 6101, model)
	makeGroup(t, "inheritgrp", []int{6101}) // inherit (empty mode)
	InitChannelCache()

	eff, err := ResolveEffectiveGroupForChannel([]string{"inheritgrp"}, 6101)
	require.NoError(t, err)
	require.NotNil(t, eff)
	assert.Equal(t, GroupCreditBillingModeInherit, eff.CreditBillingMode)

	channel := &Channel{Id: 6101, CreditBillingMode: "fixed_request", FixedRequestCredits: 12345, TokenBillingMultiplier: 1}
	profile := ResolveEffectiveBillingProfile(eff, channel)
	// inherit group → channel profile wins.
	assert.Equal(t, "fixed_request", profile.CreditBillingMode)
	assert.Equal(t, int64(12345), profile.FixedRequestCredits)
}

func seedSelectionChannelWithProfile(t *testing.T, db *gorm.DB, id int, model string, mode string, fixedCredits int64, multiplier float64) {
	t.Helper()
	priority := int64(0)
	channel := &Channel{
		Id:                     id,
		Type:                   constant.ChannelTypeOpenAI,
		Key:                    "sk-test",
		Status:                 common.ChannelStatusEnabled,
		Name:                   fmt.Sprintf("ch-%d", id),
		Models:                 model,
		Priority:               &priority,
		CreditBillingMode:      mode,
		FixedRequestCredits:    fixedCredits,
		TokenBillingMultiplier: multiplier,
	}
	require.NoError(t, db.Create(channel).Error)
}

// 验收标准 7：分组覆盖计费时，retry same-profile 过滤必须比较候选渠道的“生效分组 profile”，
// 而非渠道自身 profile，否则同分组内的可用渠道会被误排除导致 failover 失败。
func TestRetrySameProfileUsesEffectiveGroupBillingMemoryCache(t *testing.T) {
	db := setupChannelGroupSelectionTestDB(t)
	const model = "gpt-grp-retry-effective"
	// 两个成员渠道都保持 usage_tokens（渠道自身 profile），分组覆盖成 fixed_request 80000。
	seedSelectionChannelWithProfile(t, db, 8801, model, "usage_tokens", 0, 1)
	seedSelectionChannelWithProfile(t, db, 8802, model, "usage_tokens", 0, 1)
	g := makeGroup(t, "paid-retry", []int{8801, 8802})
	g.CreditBillingMode = "fixed_request"
	g.FixedRequestCredits = 80_000
	require.NoError(t, g.Update())
	_, _, err := FixAbility()
	require.NoError(t, err)
	InitChannelCache()

	frozen := ChannelBillingProfile{CreditBillingMode: "fixed_request", FixedRequestCredits: 80_000, TokenBillingMultiplier: 1}
	// 第一个渠道 8801 已用过；retry 必须仍选到同分组的 8802（其生效分组 profile 与冻结 profile 相同）。
	ch, err := GetRandomSatisfiedChannelForEndpointWithGroups([]string{"paid-retry"}, model, 0, "", []int{8801}, 1, false, frozen, true)
	require.NoError(t, err)
	require.NotNil(t, ch, "retry must select the other group member whose effective group billing matches")
	assert.Equal(t, 8802, ch.Id)
}

func TestRetrySameProfileUsesEffectiveGroupBillingDatabaseFallback(t *testing.T) {
	db := setupChannelGroupSelectionTestDB(t)
	const model = "gpt-grp-retry-effective-db"
	seedSelectionChannelWithProfile(t, db, 8811, model, "usage_tokens", 0, 1)
	seedSelectionChannelWithProfile(t, db, 8812, model, "usage_tokens", 0, 1)
	g := makeGroup(t, "paid-retry-db", []int{8811, 8812})
	g.CreditBillingMode = "fixed_request"
	g.FixedRequestCredits = 80_000
	require.NoError(t, g.Update())
	_, _, err := FixAbility()
	require.NoError(t, err)

	oldMemory := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false
	t.Cleanup(func() { common.MemoryCacheEnabled = oldMemory })

	frozen := ChannelBillingProfile{CreditBillingMode: "fixed_request", FixedRequestCredits: 80_000, TokenBillingMultiplier: 1}
	ch, err := GetChannelForEndpointWithGroups([]string{"paid-retry-db"}, model, 0, "", []int{8811}, 1, false, frozen, true)
	require.NoError(t, err)
	require.NotNil(t, ch, "DB retry must select the other group member whose effective group billing matches")
	assert.Equal(t, 8812, ch.Id)
}
