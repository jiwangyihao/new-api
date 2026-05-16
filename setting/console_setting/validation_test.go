package console_setting

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/setting/config"
	"github.com/stretchr/testify/require"
)

func TestDefaultWelcomePopupContentMatchesIssue4(t *testing.T) {
	expected := "欢迎使用赔钱GPT！\n\n欢迎邀请好友使用赔钱GPT，邀请两位好友每月付费订阅，即可免费享受“一瓶盖可乐”付费订阅！\n\n填写邀请码或试用码可享用24小时GPT5.5畅用！\n\n官方QQ群：1106020227"

	require.Equal(t, expected, defaultConsoleSetting.WelcomePopupContent)
	require.True(t, defaultConsoleSetting.WelcomePopupEnabled)
	require.Equal(t, WelcomePopupFrequencyOncePerVersion, defaultConsoleSetting.WelcomePopupFrequency)
}

func TestWelcomePopupFrequencyValidation(t *testing.T) {
	require.NoError(t, ValidateWelcomePopupFrequency(WelcomePopupFrequencyOncePerVersion))
	require.NoError(t, ValidateWelcomePopupFrequency(WelcomePopupFrequencyOncePerDay))
	require.NoError(t, ValidateWelcomePopupFrequency(WelcomePopupFrequencyEverySession))
	require.Error(t, ValidateWelcomePopupFrequency("every_login"))
	require.Error(t, ValidateWelcomePopupFrequency(" once_per_day "))
	require.Equal(t, WelcomePopupFrequencyOncePerVersion, NormalizeWelcomePopupFrequency(""))
	require.Equal(t, WelcomePopupFrequencyOncePerVersion, NormalizeWelcomePopupFrequency("bad-value"))
}

func TestWelcomePopupContentValidation(t *testing.T) {
	require.NoError(t, ValidateWelcomePopupContent(""))
	require.NoError(t, ValidateWelcomePopupContent("欢迎使用 **Markdown**"))
	require.Error(t, ValidateWelcomePopupContent("<script>alert(1)</script>"))

	longChinese := strings.Repeat("你", 2001)
	require.Error(t, ValidateWelcomePopupContent(longChinese))

	twoThousandEmoji := strings.Repeat("😀", 2000)
	require.NoError(t, ValidateWelcomePopupContent(twoThousandEmoji))
	require.Error(t, ValidateWelcomePopupContent(twoThousandEmoji+"😀"))
}

func TestWelcomePopupConfigExport(t *testing.T) {
	exported := config.GlobalConfig.ExportAllConfigs()
	require.Equal(t, DefaultWelcomePopupContent, exported["console_setting.welcome_popup_content"])
	require.Equal(t, "true", exported["console_setting.welcome_popup_enabled"])
	require.Equal(t, WelcomePopupFrequencyOncePerVersion, exported["console_setting.welcome_popup_frequency"])
}
