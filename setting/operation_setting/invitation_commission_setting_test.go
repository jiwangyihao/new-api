package operation_setting

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateInvitationCommissionSettingRejectsInvalidRate(t *testing.T) {
	valid := InvitationCommissionSetting{Enabled: true, RateBps: 10000, MinimumWithdrawCents: 1000, MinimumTransferCents: 1}
	require.NoError(t, ValidateInvitationCommissionSetting(valid))

	invalid := valid
	invalid.RateBps = 10001
	require.Error(t, ValidateInvitationCommissionSetting(invalid))

	invalid = valid
	invalid.RateBps = -1
	require.Error(t, ValidateInvitationCommissionSetting(invalid))
}
