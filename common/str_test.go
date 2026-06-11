package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeCodexProModeDefaultsAndLegacyValues(t *testing.T) {
	tests := []struct {
		name string
		mode string
		want string
	}{
		{name: "empty defaults to flexible", mode: "", want: "flexible"},
		{name: "all stays all", mode: "all", want: "all"},
		{name: "flexible stays flexible", mode: "flexible", want: "flexible"},
		{name: "off stays off", mode: "off", want: "off"},
		{name: "legacy dirty value reads as flexible", mode: "legacy-pro", want: "flexible"},
		{name: "unknown value reads as flexible", mode: "codex-pro", want: "flexible"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, NormalizeCodexProMode(tt.mode))
		})
	}
}

func TestValidateCodexProModeForUpdateAcceptsKnownModes(t *testing.T) {
	for _, mode := range []string{"all", "flexible", "off"} {
		t.Run(mode, func(t *testing.T) {
			require.NoError(t, ValidateCodexProModeForUpdate(mode))
		})
	}
}

func TestValidateCodexProModeForUpdateRejectsInvalidMode(t *testing.T) {
	for _, mode := range []string{"", "legacy-pro", "codex-pro", "subscription_first"} {
		t.Run(mode, func(t *testing.T) {
			require.Error(t, ValidateCodexProModeForUpdate(mode))
		})
	}
}

func TestNormalizeAPIKeyCodexProModeDefaultsToInherit(t *testing.T) {
	tests := []struct {
		name string
		mode string
		want string
	}{
		{name: "empty defaults to inherit", mode: "", want: "inherit"},
		{name: "inherit stays inherit", mode: "inherit", want: "inherit"},
		{name: "all stays all", mode: "all", want: "all"},
		{name: "flexible stays flexible", mode: "flexible", want: "flexible"},
		{name: "off stays off", mode: "off", want: "off"},
		{name: "unknown value reads as inherit", mode: "legacy-pro", want: "inherit"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, NormalizeAPIKeyCodexProMode(tt.mode))
		})
	}
}

func TestValidateAPIKeyCodexProModeForUpdateAcceptsInheritAndKnownModes(t *testing.T) {
	for _, mode := range []string{"inherit", "all", "flexible", "off"} {
		t.Run(mode, func(t *testing.T) {
			require.NoError(t, ValidateAPIKeyCodexProModeForUpdate(mode))
		})
	}
}

func TestValidateAPIKeyCodexProModeForUpdateRejectsInvalidMode(t *testing.T) {
	for _, mode := range []string{"", "legacy-pro", "codex-pro", "subscription_first"} {
		t.Run(mode, func(t *testing.T) {
			require.Error(t, ValidateAPIKeyCodexProModeForUpdate(mode))
		})
	}
}

func TestEffectiveCodexProModeUsesAPIKeyOverrideWhenPresent(t *testing.T) {
	assert.Equal(t, "off", EffectiveCodexProMode("off", "all"))
	assert.Equal(t, "all", EffectiveCodexProMode("all", "off"))
	assert.Equal(t, "flexible", EffectiveCodexProMode("inherit", "flexible"))
	assert.Equal(t, "flexible", EffectiveCodexProMode("", ""))
}
