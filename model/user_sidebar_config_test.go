package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultSidebarConfigForRoleIncludesTrialCodeAdminModule(t *testing.T) {
	config := generateDefaultSidebarConfigForRole(common.RoleAdminUser)
	require.NotEmpty(t, config)
	assert.Contains(t, config, `"trial_code":true`)
	assert.Contains(t, config, `"subscription":true`)
}
