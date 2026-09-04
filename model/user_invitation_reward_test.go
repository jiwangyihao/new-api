package model

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRewardMonthStringFromUnixUsesShanghaiMonthBoundary(t *testing.T) {
	shanghai, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)
	boundary := time.Date(2026, 6, 1, 0, 0, 0, 0, shanghai).Unix()
	assert.Equal(t, "2026-06", rewardMonthStringFromUnix(boundary))
}
