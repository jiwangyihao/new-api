package controller

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func TestSubscriptionConversionErrorCodeUsesStableSentinels(t *testing.T) {
	tests := []struct {
		name string
		err  error
		code string
	}{
		{name: "ineligible", err: model.ErrConversionIneligible, code: "subscription_conversion_ineligible"},
		{name: "stale quote", err: model.ErrConversionQuoteStale, code: "subscription_conversion_quote_stale"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			wrapped := fmt.Errorf("localized detail: %w", test.err)
			require.ErrorIs(t, wrapped, test.err)
			require.Equal(t, test.code, subscriptionConversionErrorCode(wrapped))
		})
	}
}
