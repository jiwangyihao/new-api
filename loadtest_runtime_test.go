package main

import "testing"

func TestServerListenAddr(t *testing.T) {
	t.Setenv("HOST", "127.0.0.1")
	t.Setenv("PORT", "13080")
	if got := serverListenAddr(); got != "127.0.0.1:13080" {
		t.Fatalf("addr = %q", got)
	}
}

func TestPprofListenAddr(t *testing.T) {
	t.Setenv("PPROF_ADDR", "127.0.0.1:8005")
	if got := pprofListenAddr(); got != "127.0.0.1:8005" {
		t.Fatalf("addr = %q", got)
	}
}

func TestChannelUpdateFrequencyFromEnvDisablesZeroAndEmpty(t *testing.T) {
	for _, value := range []string{"", "0", "-1"} {
		frequency, enabled, err := channelUpdateFrequencyFromEnv(value)
		if err != nil {
			t.Fatalf("value %q returned error: %v", value, err)
		}
		if enabled || frequency != 0 {
			t.Fatalf("value %q enabled updater with frequency %d", value, frequency)
		}
	}
}

func TestChannelUpdateFrequencyFromEnvEnablesPositiveFrequency(t *testing.T) {
	frequency, enabled, err := channelUpdateFrequencyFromEnv("5")
	if err != nil {
		t.Fatal(err)
	}
	if !enabled || frequency != 5 {
		t.Fatalf("enabled=%v frequency=%d, want enabled frequency 5", enabled, frequency)
	}
}

func TestChannelUpdateFrequencyFromEnvRejectsInvalidValue(t *testing.T) {
	if _, _, err := channelUpdateFrequencyFromEnv("abc"); err == nil {
		t.Fatal("invalid frequency accepted")
	}
}
