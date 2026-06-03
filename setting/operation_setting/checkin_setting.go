package operation_setting

import "github.com/QuantumNous/new-api/setting/config"

// CheckinSetting 签到功能配置
type CheckinSetting struct {
	Enabled  bool `json:"enabled"`   // 是否启用签到功能
	MinQuota int  `json:"min_quota"` // 签到最小账户余额奖励（CNY 分）
	MaxQuota int  `json:"max_quota"` // 签到最大账户余额奖励（CNY 分）
}

// 默认配置
var checkinSetting = CheckinSetting{
	Enabled:  false, // 默认关闭
	MinQuota: 20,    // 默认最小账户余额奖励 ¥0.20
	MaxQuota: 20,    // 默认最大账户余额奖励 ¥0.20
}

func init() {
	// 注册到全局配置管理器
	config.GlobalConfig.Register("checkin_setting", &checkinSetting)
}

// GetCheckinSetting 获取签到配置
func GetCheckinSetting() *CheckinSetting {
	return &checkinSetting
}

// IsCheckinEnabled 是否启用签到功能
func IsCheckinEnabled() bool {
	return checkinSetting.Enabled
}

// GetCheckinQuotaRange 获取签到额度范围
func GetCheckinQuotaRange() (min, max int) {
	return checkinSetting.MinQuota, checkinSetting.MaxQuota
}
