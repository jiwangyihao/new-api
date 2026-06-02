package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultSidebarConfigForRoleIncludesTrialAbuseAdminModule(t *testing.T) {
	config := generateDefaultSidebarConfigForRole(common.RoleAdminUser)
	require.NotEmpty(t, config)
	assert.Contains(t, config, `"trial_code":true`)
	assert.Contains(t, config, `"trial_abuse":true`)
	assert.Contains(t, config, `"subscription":true`)
}

func TestDefaultSidebarConfigForRoleIncludesTrialAbuseRootModule(t *testing.T) {
	config := generateDefaultSidebarConfigForRole(common.RoleRootUser)
	require.NotEmpty(t, config)
	assert.Contains(t, config, `"trial_abuse":true`)
}

func TestDefaultSidebarConfigForRoleDoesNotAddAdminModulesForCommonUser(t *testing.T) {
	config := generateDefaultSidebarConfigForRole(common.RoleCommonUser)
	require.NotEmpty(t, config)
	assert.NotContains(t, config, `"admin"`)
	assert.NotContains(t, config, `"trial_abuse"`)
}
