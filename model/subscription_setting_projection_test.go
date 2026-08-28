package model

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseSubscriptionSelectionSettingMatchesFullUserSetting(t *testing.T) {
	cases := []string{
		"",
		`{}`,
		`{"subscription_billing_strategy":"active_fallback","active_subscription_id":42}`,
		`{"subscription_billing_strategy":" timed_first ","active_subscription_id":0,"webhook_url":"https://example.com/` + strings.Repeat("x", 8192) + `"}`,
		`{"active_subscription_id":7,"subscription_billing_strategy":"single_active","active_subscription_id":8}`,
	}
	for _, raw := range cases {
		full, err := ParseUserSettingString(raw)
		require.NoError(t, err)
		projected, err := parseSubscriptionSelectionSetting(raw)
		require.NoError(t, err)
		require.Equal(t, full.SubscriptionBillingStrategy, projected.SubscriptionBillingStrategy)
		require.Equal(t, full.ActiveSubscriptionId, projected.ActiveSubscriptionId)
	}
}

func TestParseSubscriptionSelectionSettingReturnsParsedFieldsWithError(t *testing.T) {
	setting, err := parseSubscriptionSelectionSetting(`{"subscription_billing_strategy":"active_fallback","active_subscription_id":"bad"}`)
	require.Error(t, err)
	require.Equal(t, SubscriptionBillingStrategyActiveFallback, setting.SubscriptionBillingStrategy)
	require.Zero(t, setting.ActiveSubscriptionId)
}

func TestParseSubscriptionSelectionSettingRejectsMalformedJSON(t *testing.T) {
	_, err := parseSubscriptionSelectionSetting(`{"active_subscription_id":`)
	require.Error(t, err)
}

func TestParseSubscriptionSelectionSettingRejectsWrongFieldTypes(t *testing.T) {
	for _, raw := range []string{
		`{"subscription_billing_strategy":42}`,
		`{"active_subscription_id":"42"}`,
	} {
		_, fullErr := ParseUserSettingString(raw)
		_, projectedErr := parseSubscriptionSelectionSetting(raw)
		require.Error(t, fullErr)
		require.Error(t, projectedErr)
	}
}

func BenchmarkParseSubscriptionSelectionSetting(b *testing.B) {
	raw := `{"subscription_billing_strategy":"active_fallback","active_subscription_id":42,"webhook_url":"https://example.com/` + strings.Repeat("x", 1<<20) + `","sidebar_modules":"` + strings.Repeat("y", 1<<20) + `"}`
	b.Run("full", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, err := ParseUserSettingString(raw); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("projection", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, err := parseSubscriptionSelectionSetting(raw); err != nil {
				b.Fatal(err)
			}
		}
	})
}
