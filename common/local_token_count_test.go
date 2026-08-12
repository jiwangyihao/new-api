package common

import (
	"os"
	"testing"
)

func TestLocalTokenCountingDefaultsToDisabled(t *testing.T) {
	oldValue, hadValue := os.LookupEnv("CountToken")
	t.Cleanup(func() {
		if hadValue {
			_ = os.Setenv("CountToken", oldValue)
		} else {
			_ = os.Unsetenv("CountToken")
		}
	})

	_ = os.Unsetenv("CountToken")
	if localTokenCountingEnabled() {
		t.Fatal("local token counting must default to disabled")
	}

	_ = os.Setenv("CountToken", "true")
	if !localTokenCountingEnabled() {
		t.Fatal("CountToken=true must explicitly enable local token counting")
	}

	_ = os.Setenv("CountToken", "false")
	if localTokenCountingEnabled() {
		t.Fatal("CountToken=false must disable local token counting")
	}
}
