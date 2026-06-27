package model

import (
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
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

func makeGroup(t *testing.T, name string, channelIds []int) *ChannelGroup {
	t.Helper()
	g := &ChannelGroup{Name: name, Enabled: true, CreditBillingMode: GroupCreditBillingModeInherit}
	require.NoError(t, g.Insert())
	require.NoError(t, SetChannelGroupChannels(g.Id, channelIds))
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
