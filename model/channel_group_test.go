package model

import (
	"testing"

	"github.com/QuantumNous/new-api/pkg/creditbilling"
	"github.com/QuantumNous/new-api/pkg/tokenbilling"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeGroupCreditBillingModeKeepsInheritEmpty(t *testing.T) {
	mode, err := normalizeGroupCreditBillingMode("")
	require.NoError(t, err)
	assert.Equal(t, GroupCreditBillingModeInherit, mode)
	assert.Equal(t, "", mode)

	mode, err = normalizeGroupCreditBillingMode("  ")
	require.NoError(t, err)
	assert.Equal(t, GroupCreditBillingModeInherit, mode)
}

func TestNormalizeGroupCreditBillingModeAcceptsKnownModes(t *testing.T) {
	for _, m := range []string{creditbilling.ModeUsageTokens, creditbilling.ModeFixedRequest} {
		got, err := normalizeGroupCreditBillingMode(m)
		require.NoError(t, err)
		assert.Equal(t, m, got)
	}
}

func TestNormalizeGroupCreditBillingModeRejectsUnknown(t *testing.T) {
	_, err := normalizeGroupCreditBillingMode("bogus")
	assert.Error(t, err)
}

func TestChannelGroupOverridesBilling(t *testing.T) {
	inherit := &ChannelGroup{CreditBillingMode: ""}
	assert.False(t, inherit.OverridesBilling())

	usage := &ChannelGroup{CreditBillingMode: creditbilling.ModeUsageTokens}
	assert.True(t, usage.OverridesBilling())

	fixed := &ChannelGroup{CreditBillingMode: creditbilling.ModeFixedRequest}
	assert.True(t, fixed.OverridesBilling())
}

func TestResolveEffectiveBillingProfileInheritFallsBackToChannel(t *testing.T) {
	channel := &Channel{
		CreditBillingMode:      creditbilling.ModeFixedRequest,
		FixedRequestCredits:    80_000,
		TokenBillingMultiplier: 2,
	}
	// inherit group → channel profile
	group := &ChannelGroup{CreditBillingMode: GroupCreditBillingModeInherit}
	profile := ResolveEffectiveBillingProfile(group, channel)
	assert.Equal(t, creditbilling.ModeFixedRequest, profile.CreditBillingMode)
	assert.Equal(t, int64(80_000), profile.FixedRequestCredits)

	// nil group → channel profile
	profile = ResolveEffectiveBillingProfile(nil, channel)
	assert.Equal(t, creditbilling.ModeFixedRequest, profile.CreditBillingMode)
	assert.Equal(t, int64(80_000), profile.FixedRequestCredits)
}

func TestResolveEffectiveBillingProfileOverrideUsesGroup(t *testing.T) {
	channel := &Channel{
		CreditBillingMode:      creditbilling.ModeFixedRequest,
		FixedRequestCredits:    80_000,
		TokenBillingMultiplier: 2,
	}
	group := &ChannelGroup{
		CreditBillingMode:      creditbilling.ModeUsageTokens,
		TokenBillingMultiplier: 3,
	}
	profile := ResolveEffectiveBillingProfile(group, channel)
	assert.Equal(t, creditbilling.ModeUsageTokens, profile.CreditBillingMode)
	// usage-token mode → group multiplier wins, channel fixed credits ignored
	assert.True(t, tokenbilling.SameMultiplier(3, profile.TokenBillingMultiplier))
}

func TestChannelGroupValidateInheritSkipsFixedCreditsRule(t *testing.T) {
	// inherit with zero fixed credits must be valid (no usage_tokens default coercion).
	group := &ChannelGroup{CreditBillingMode: ""}
	require.NoError(t, group.Validate())
	assert.Equal(t, GroupCreditBillingModeInherit, group.CreditBillingMode)
}

func TestChannelGroupValidateFixedRequestRequiresPositiveCredits(t *testing.T) {
	group := &ChannelGroup{CreditBillingMode: creditbilling.ModeFixedRequest, FixedRequestCredits: 0}
	assert.Error(t, group.Validate())

	group = &ChannelGroup{CreditBillingMode: creditbilling.ModeFixedRequest, FixedRequestCredits: 80_000}
	assert.NoError(t, group.Validate())
}

func TestChannelGroupIsDefault(t *testing.T) {
	assert.True(t, (&ChannelGroup{Name: DefaultChannelGroupName}).IsDefault())
	assert.False(t, (&ChannelGroup{Name: "vip"}).IsDefault())
}
