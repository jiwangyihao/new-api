package model

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFillLogDerivedFieldsPreservesJSONMapSemantics(t *testing.T) {
	cases := []struct {
		name  string
		other string
	}{
		{name: "numeric_and_string", other: `{"subscription_id":123,"subscription_tokens_consumed":"456","billing_source":"subscription","request_path":"/v1/responses"}`},
		{name: "json_numbers", other: `{"subscription_id":789,"subscription_tokens_consumed":1234,"billing_source":"subscription","endpoint":"/v1/chat/completions"}`},
		{name: "float_strings", other: `{"subscription_id":"1.9","subscription_tokens_consumed":"2.9","billing_source":"","endpoint":""}`},
		{name: "null_values", other: `{"subscription_id":null,"subscription_tokens_consumed":null,"billing_source":null,"endpoint":null,"request_path":"/fallback"}`},
		{name: "duplicate_keys_last_wins", other: `{"subscription_id":1,"subscription_id":2,"subscription_tokens_consumed":3,"subscription_tokens_consumed":4,"endpoint":"/last"}`},
		{name: "nested_non_targets", other: `{"metadata":{"subscription_id":999},"request_path":"/nested"}`},
		{name: "escaped_key", other: `{"\u0073ubscription_id":321,"billing_source":"escaped"}`},
		{name: "malformed", other: `{"subscription_id":`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			actual := &Log{Other: tc.other}
			fillLogDerivedFields(actual)
			switch tc.name {
			case "numeric_and_string":
				require.Equal(t, 123, *actual.SubscriptionID)
				require.EqualValues(t, 456, *actual.SubscriptionTokensConsumed)
				require.Equal(t, "subscription", *actual.BillingSource)
				require.Equal(t, "/v1/responses", *actual.Endpoint)
			case "json_numbers":
				require.Equal(t, 789, *actual.SubscriptionID)
				require.EqualValues(t, 1234, *actual.SubscriptionTokensConsumed)
				require.Equal(t, "/v1/chat/completions", *actual.Endpoint)
			case "float_strings":
				require.Equal(t, 1, *actual.SubscriptionID)
				require.EqualValues(t, 2, *actual.SubscriptionTokensConsumed)
				require.Equal(t, "", *actual.BillingSource)
				require.Equal(t, "", *actual.Endpoint)
			case "null_values":
				require.Nil(t, actual.SubscriptionID)
				require.Nil(t, actual.SubscriptionTokensConsumed)
				require.Nil(t, actual.BillingSource)
				require.Equal(t, "/fallback", *actual.Endpoint)
			case "duplicate_keys_last_wins":
				require.Equal(t, 2, *actual.SubscriptionID)
				require.EqualValues(t, 4, *actual.SubscriptionTokensConsumed)
				require.Equal(t, "/last", *actual.Endpoint)
			case "nested_non_targets":
				require.Nil(t, actual.SubscriptionID)
				require.Equal(t, "/nested", *actual.Endpoint)
			case "escaped_key":
				require.Equal(t, 321, *actual.SubscriptionID)
				require.Equal(t, "escaped", *actual.BillingSource)
			case "malformed":
				require.Nil(t, actual.SubscriptionID)
				require.Nil(t, actual.SubscriptionTokensConsumed)
			}
		})
	}
}

func BenchmarkFillLogDerivedFieldsLargeOther(b *testing.B) {
	other := `{"model":"gpt-5","input":"` + strings.Repeat("x", 1<<20) + `","subscription_id":123,"subscription_tokens_consumed":456,"billing_source":"subscription","endpoint":"/v1/responses"}`
	b.ReportAllocs()
	b.SetBytes(int64(len(other)))
	for i := 0; i < b.N; i++ {
		log := &Log{Other: other}
		fillLogDerivedFields(log)
		if log.SubscriptionID == nil {
			b.Fatal("missing subscription id")
		}
	}
}
