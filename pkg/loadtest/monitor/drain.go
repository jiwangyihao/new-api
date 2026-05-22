package monitor

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/pkg/loadtest/artifact"
)

type DrainSample struct {
	ConsumeLogs           int64
	PreConsumeRecords     int64
	SubscriptionTokenUsed int64
}

type DrainExpectations struct {
	Success int
	Tokens  int64
}

func EvaluateDrain(samples []DrainSample, expect DrainExpectations) artifact.Statused {
	if len(samples) == 0 {
		return artifact.Statused{Status: "failed", Reason: "no drain samples collected"}
	}
	baseline := samples[0]
	latest := samples[len(samples)-1]
	if len(samples) == 1 {
		baseline = DrainSample{}
	}
	wantSuccess := int64(expect.Success)
	var missing []string
	if delta := latest.ConsumeLogs - baseline.ConsumeLogs; delta < wantSuccess {
		missing = append(missing, fmt.Sprintf("consume_logs delta got %d want at least %d", delta, wantSuccess))
	}
	if delta := latest.PreConsumeRecords - baseline.PreConsumeRecords; delta < wantSuccess {
		missing = append(missing, fmt.Sprintf("subscription_pre_consume_records delta got %d want at least %d", delta, wantSuccess))
	}
	if delta := latest.SubscriptionTokenUsed - baseline.SubscriptionTokenUsed; delta < expect.Tokens {
		missing = append(missing, fmt.Sprintf("user_subscriptions token_used delta got %d want at least %d", delta, expect.Tokens))
	}
	if len(missing) != 0 {
		return artifact.Statused{Status: "failed", Reason: strings.Join(missing, "; ")}
	}
	return artifact.Statused{Status: "passed"}
}

func WaitDrain(ctx context.Context, interval time.Duration, sample func() DrainSample, expect DrainExpectations) ([]DrainSample, artifact.Statused) {
	if sample == nil {
		return nil, artifact.Statused{Status: "unavailable", Reason: "drain sampler is not configured"}
	}
	if interval <= 0 {
		interval = time.Second
	}
	samples := make([]DrainSample, 0, 2)
	samples = append(samples, sample())
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			status := EvaluateDrain(samples, expect)
			if status.Status == "failed" && ctx.Err() != nil {
				status.Reason = status.Reason + "; " + ctx.Err().Error()
			}
			return samples, status
		case <-ticker.C:
			samples = append(samples, sample())
			if status := EvaluateDrain(samples, expect); status.Status == "passed" {
				return samples, status
			}
		}
	}
}
