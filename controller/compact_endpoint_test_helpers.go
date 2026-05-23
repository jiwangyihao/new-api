package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func endpointStringsForControllerTest(t *testing.T, endpoints map[string]any) string {
	t.Helper()
	data, err := common.Marshal(endpoints)
	require.NoError(t, err)
	return string(data)
}
