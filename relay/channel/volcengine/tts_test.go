package volcengine

import (
	"testing"

	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/require"
)

func TestBuildVolcengineTTSDialHeaderFinalizesSubscriptionMarker(t *testing.T) {
	t.Parallel()

	header := buildVolcengineTTSDialHeader("token", &relaycommon.RelayInfo{SubscriptionTrialMarker: "trial"})

	require.Equal(t, "Bearer;token", header.Get("Authorization"))
	require.Equal(t, "trial", header.Get(channel.SubscriptionMarkerHeaderName))
}

func TestBuildVolcengineTTSDialHeaderOmitsSubscriptionMarkerWhenNotTrial(t *testing.T) {
	t.Parallel()

	header := buildVolcengineTTSDialHeader("token", &relaycommon.RelayInfo{SubscriptionTrialMarker: "paid"})

	require.Equal(t, "Bearer;token", header.Get("Authorization"))
	require.Empty(t, header.Get(channel.SubscriptionMarkerHeaderName))
}
