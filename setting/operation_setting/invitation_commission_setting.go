package operation_setting

import (
	"errors"

	"github.com/QuantumNous/new-api/setting/config"
)

type InvitationCommissionSetting struct {
	Enabled              bool  `json:"enabled"`
	RateBps              int   `json:"rate_bps"`
	MinimumWithdrawCents int64 `json:"minimum_withdraw_cents"`
	MinimumTransferCents int64 `json:"minimum_transfer_cents"`
}

var invitationCommissionSetting = InvitationCommissionSetting{
	Enabled:              true,
	RateBps:              1000,
	MinimumWithdrawCents: 1000,
	MinimumTransferCents: 1,
}

func init() {
	config.GlobalConfig.Register("invitation_commission_setting", &invitationCommissionSetting)
}

func GetInvitationCommissionSetting() *InvitationCommissionSetting {
	return &invitationCommissionSetting
}

func ValidateInvitationCommissionSetting(setting InvitationCommissionSetting) error {
	if setting.RateBps < 0 || setting.RateBps > 10000 {
		return errors.New("invitation commission rate_bps must be between 0 and 10000")
	}
	if setting.MinimumWithdrawCents < 0 {
		return errors.New("invitation commission minimum_withdraw_cents must be non-negative")
	}
	if setting.MinimumTransferCents < 0 {
		return errors.New("invitation commission minimum_transfer_cents must be non-negative")
	}
	return nil
}
