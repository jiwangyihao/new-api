package common

import (
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/pkg/creditbilling"
	"github.com/QuantumNous/new-api/pkg/tokenbilling"
	"github.com/stretchr/testify/require"
)

func TestDynamicBillingMultiplierHeadersDisabledIgnored(t *testing.T) {
	info := &RelayInfo{}

	applied := info.ApplyDynamicBillingMultiplierFromHeaders(http.Header{
		DynamicBillingMultiplierHeaderName: []string{"1.5"},
	}, DynamicBillingMultiplierSourceHeader)

	require.False(t, applied)
	require.InDelta(t, tokenbilling.DefaultMultiplier, info.FrozenDynamicBillingMultiplier(), tokenbilling.Epsilon)
	require.Equal(t, creditbilling.DynamicMultiplierDefaultSource, info.FrozenDynamicBillingMultiplierSource())
	require.Equal(t, DynamicBillingMultiplierIgnoredReasonDisabled, info.DynamicBillingMultiplierIgnoredReason)
}

func TestDynamicBillingMultiplierHeadersEnabledAcceptsNumeric(t *testing.T) {
	info := &RelayInfo{DynamicBillingMultiplierEnabled: true}

	applied := info.ApplyDynamicBillingMultiplierFromHeaders(http.Header{
		DynamicBillingMultiplierHeaderName:       []string{"1.5"},
		DynamicBillingMultiplierSourceHeaderName: []string{"priority_tier"},
	}, DynamicBillingMultiplierSourceHeader)

	require.True(t, applied)
	require.InDelta(t, 1.5, info.FrozenDynamicBillingMultiplier(), tokenbilling.Epsilon)
	require.Equal(t, "priority_tier", info.FrozenDynamicBillingMultiplierSource())
	require.Empty(t, info.DynamicBillingMultiplierIgnoredReason)
}

func TestDynamicBillingMultiplierSpecHeadersEnabledAcceptsNumeric(t *testing.T) {
	info := &RelayInfo{DynamicBillingMultiplierEnabled: true}

	applied := info.ApplyDynamicBillingMultiplierFromHeaders(http.Header{
		DynamicBillingMultiplierSpecHeaderName:       []string{"1.75"},
		DynamicBillingMultiplierSpecSourceHeaderName: []string{"spec_priority"},
	}, DynamicBillingMultiplierSourceHeader)

	require.True(t, applied)
	require.InDelta(t, 1.75, info.FrozenDynamicBillingMultiplier(), tokenbilling.Epsilon)
	require.Equal(t, "spec_priority", info.FrozenDynamicBillingMultiplierSource())
	require.Empty(t, info.DynamicBillingMultiplierIgnoredReason)
}

func TestDynamicBillingMultiplierInvalidHeadersFallBackToDefault(t *testing.T) {
	for _, value := range []string{"0", "-1", "NaN", "+Inf", "100.01", "not-a-number"} {
		t.Run(value, func(t *testing.T) {
			info := &RelayInfo{DynamicBillingMultiplierEnabled: true}

			applied := info.ApplyDynamicBillingMultiplierFromHeaders(http.Header{
				DynamicBillingMultiplierHeaderName: []string{value},
			}, DynamicBillingMultiplierSourceHeader)

			require.False(t, applied)
			require.InDelta(t, tokenbilling.DefaultMultiplier, info.FrozenDynamicBillingMultiplier(), tokenbilling.Epsilon)
			require.Equal(t, creditbilling.DynamicMultiplierDefaultSource, info.FrozenDynamicBillingMultiplierSource())
			require.Equal(t, DynamicBillingMultiplierIgnoredReasonInvalid, info.DynamicBillingMultiplierIgnoredReason)
		})
	}
}

func TestDynamicBillingMultiplierInvalidDoesNotOverwritePriorValidValue(t *testing.T) {
	info := &RelayInfo{DynamicBillingMultiplierEnabled: true}
	require.True(t, info.ApplyDynamicBillingMultiplierFromHeaders(http.Header{
		DynamicBillingMultiplierHeaderName:       []string{"1.5"},
		DynamicBillingMultiplierSourceHeaderName: []string{"accepted"},
	}, DynamicBillingMultiplierSourceHeader))

	applied := info.ApplyDynamicBillingMultiplierFromHeaders(http.Header{
		DynamicBillingMultiplierHeaderName: []string{"0"},
	}, DynamicBillingMultiplierSourceHeader)

	require.False(t, applied)
	require.InDelta(t, 1.5, info.FrozenDynamicBillingMultiplier(), tokenbilling.Epsilon)
	require.Equal(t, "accepted", info.FrozenDynamicBillingMultiplierSource())
	require.Equal(t, DynamicBillingMultiplierIgnoredReasonInvalid, info.DynamicBillingMultiplierIgnoredReason)
}

func TestDynamicBillingMultiplierBodyAndSSESources(t *testing.T) {
	for _, tc := range []struct {
		name       string
		body       []byte
		fallback   string
		wantSource string
	}{
		{
			name:       "body newapi_billing",
			body:       []byte(`{"usage":{"total_tokens":5},"newapi_billing":{"billing_multiplier":1.25,"billing_multiplier_source":"priority_tier"}}`),
			fallback:   DynamicBillingMultiplierSourceBody,
			wantSource: "priority_tier",
		},
		{
			name:       "responses final event",
			body:       []byte(`{"type":"response.completed","response":{"newapi_billing":{"billing_multiplier":2.5}}}`),
			fallback:   DynamicBillingMultiplierSourceSSE,
			wantSource: DynamicBillingMultiplierSourceSSE,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			info := &RelayInfo{DynamicBillingMultiplierEnabled: true}

			applied := info.ApplyDynamicBillingMultiplierFromBody(tc.body, tc.fallback)

			require.True(t, applied)
			require.Greater(t, info.FrozenDynamicBillingMultiplier(), 1.0)
			require.Equal(t, tc.wantSource, info.FrozenDynamicBillingMultiplierSource())
		})
	}
}

func TestCodexProServedHeaderDoesNotSetDynamicBillingMultiplier(t *testing.T) {
	info := &RelayInfo{
		DynamicBillingMultiplierEnabled: true,
		CodexProRequestMarker:          "codex-pro",
		CodexProRequestSent:            true,
	}
	trailers := http.Header{"X-NewAPI-Pro-Served": []string{"codex-pro"}}

	info.MarkCodexProServedCandidateFromTrailers(trailers)
	info.ConfirmCodexProServed()
	applied := info.ApplyDynamicBillingMultiplierFromHeaders(trailers, DynamicBillingMultiplierSourceTrailer)

	require.True(t, info.CodexProServed)
	require.False(t, applied)
	require.InDelta(t, tokenbilling.DefaultMultiplier, info.FrozenDynamicBillingMultiplier(), tokenbilling.Epsilon)
	require.Equal(t, creditbilling.DynamicMultiplierDefaultSource, info.FrozenDynamicBillingMultiplierSource())
}
