package main

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMainStartsInvitationRewardEventRetryTask(t *testing.T) {
	content, err := os.ReadFile("main.go")
	require.NoError(t, err)
	source := string(content)

	entitlementCall := "service.StartInvitationEntitlementRefreshTask()"
	retryCall := "service.StartInvitationRewardEventRetryTask()"
	require.Contains(t, source, entitlementCall)
	require.Contains(t, source, retryCall)
	assert.Greater(t, strings.Index(source, retryCall), strings.Index(source, entitlementCall))
}
