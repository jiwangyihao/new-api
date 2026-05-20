package service

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type countingConcurrencyLease struct {
	releaseCount atomic.Int32
	lastErr      error
}

func (l *countingConcurrencyLease) Release(ctx context.Context) error {
	l.releaseCount.Add(1)
	if ctx != nil && ctx.Err() != nil {
		l.lastErr = ctx.Err()
	}
	return l.lastErr
}

func newConcurrencyRelayInfoForTest(limit int) *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		BillingSource: BillingSourceSubscription,
		RelayMode:     relayconstant.RelayModeChatCompletions,
		UserId:        1,
		RequestId:     "req-concurrency",
		UserSetting:   dto.UserSetting{BillingPreference: "subscription_only"},
		Billing: &BillingSession{funding: &SubscriptionFunding{
			subscriptionId:          1,
			preConsumed:             1,
			DistributorTokenBilling: true,
			concurrencyLimit:        limit,
		}},
	}
}

func TestAcquireSubscriptionConcurrencySkipsNonTargetRelayModes(t *testing.T) {
	resetSubscriptionConcurrencyForTest(t)
	relayInfo := newConcurrencyRelayInfoForTest(1)
	relayInfo.RelayMode = relayconstant.RelayModeEmbeddings
	lease, apiErr := AcquireSubscriptionConcurrency(context.Background(), relayInfo)
	require.Nil(t, apiErr)
	require.NoError(t, lease.Release(context.Background()))
}

func TestAcquireSubscriptionConcurrencyReturns429WhenExceeded(t *testing.T) {
	resetSubscriptionConcurrencyForTest(t)
	common.SubscriptionConcurrencyQueueCapacity = 0
	relayInfo := newConcurrencyRelayInfoForTest(1)
	lease, apiErr := AcquireSubscriptionConcurrency(context.Background(), relayInfo)
	require.Nil(t, apiErr)
	defer lease.Release(context.Background())
	_, apiErr = AcquireSubscriptionConcurrency(context.Background(), &relaycommon.RelayInfo{
		BillingSource: BillingSourceSubscription,
		RelayMode:     relayconstant.RelayModeChatCompletions,
		UserId:        1,
		RequestId:     "req-concurrency-2",
		Billing: &BillingSession{funding: &SubscriptionFunding{
			subscriptionId:          2,
			preConsumed:             1,
			DistributorTokenBilling: true,
			concurrencyLimit:        1,
		}},
	})
	require.NotNil(t, apiErr)
	assert.Equal(t, http.StatusTooManyRequests, apiErr.StatusCode)
}

func TestAcquireSubscriptionConcurrencyReturns503WhenUnavailable(t *testing.T) {
	resetSubscriptionConcurrencyForTest(t)
	common.SubscriptionConcurrencyRequireRedis = true

	_, apiErr := AcquireSubscriptionConcurrency(context.Background(), newConcurrencyRelayInfoForTest(1))

	require.NotNil(t, apiErr)
	assert.Equal(t, http.StatusServiceUnavailable, apiErr.StatusCode)
}

func TestAcquireSubscriptionConcurrencyReturns503OnCanceledQueueWait(t *testing.T) {
	resetSubscriptionConcurrencyForTest(t)
	common.SubscriptionConcurrencyQueueCapacity = 1
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	lease, apiErr := AcquireSubscriptionConcurrency(context.Background(), newConcurrencyRelayInfoForTest(1))
	require.Nil(t, apiErr)
	defer lease.Release(context.Background())

	queuedErr := make(chan *types.NewAPIError, 1)
	go func() {
		_, err := AcquireSubscriptionConcurrency(ctx, &relaycommon.RelayInfo{
			BillingSource: BillingSourceSubscription,
			RelayMode:     relayconstant.RelayModeChatCompletions,
			UserId:        1,
			RequestId:     "req-concurrency-2",
			Billing: &BillingSession{funding: &SubscriptionFunding{
				subscriptionId:          2,
				preConsumed:             1,
				DistributorTokenBilling: true,
				concurrencyLimit:        1,
			}},
		})
		queuedErr <- err
	}()

	cancel()
	apiErr = <-queuedErr
	require.NotNil(t, apiErr)
	assert.Equal(t, http.StatusServiceUnavailable, apiErr.StatusCode)
}

func TestConcurrencyLeaseReleaseUsesLiveContextExpectation(t *testing.T) {
	lease := &countingConcurrencyLease{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	assert.ErrorIs(t, lease.Release(ctx), context.Canceled)
	assert.Equal(t, int32(1), lease.releaseCount.Load())
}
