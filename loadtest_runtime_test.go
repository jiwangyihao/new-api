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
