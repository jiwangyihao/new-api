package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultSidebarConfigForRoleExcludesApplicationGuidesModule(t *testing.T) {
	config := generateDefaultSidebarConfigForRole(common.RoleAdminUser)
	require.NotEmpty(t, config)
	assert.NotContains(t, config, `"app_guides"`)
}
