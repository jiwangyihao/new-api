package xunfei

import (
	"testing"

	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/require"
)

func TestBuildXunfeiDialHeaderFinalizesSubscriptionMarker(t *testing.T) {
	t.Parallel()

	header := buildXunfeiDialHeader(&relaycommon.RelayInfo{SubscriptionTrialMarker: "trial"})

	require.Equal(t, "trial", header.Get(channel.SubscriptionMarkerHeaderName))
}

func TestBuildXunfeiDialHeaderOmitsSubscriptionMarkerWhenNotTrial(t *testing.T) {
	t.Parallel()

	header := buildXunfeiDialHeader(&relaycommon.RelayInfo{SubscriptionTrialMarker: "paid"})

	require.Empty(t, header.Get(channel.SubscriptionMarkerHeaderName))
}
