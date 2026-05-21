package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunWritesSummaryOnRuntimeError(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run([]string{
		"--url", "http://127.0.0.1:1",
		"--api-key", "sk-loadtestsub",
		"--token-profile", "subscription",
		"--path", "/v1/responses",
		"--model", "gpt-5.5",
		"--scenario", "runtime-error",
		"--concurrency", "1",
		"--max-requests", "1",
		"--timeout", "200ms",
	}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code = %d, stderr=%s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, `"error_reasons":{"connect_refused":1}`) {
		t.Fatalf("summary missing connect_refused: %s", out)
	}
	if !strings.Contains(out, `"first_error_samples"`) {
		t.Fatalf("summary missing first_error_samples: %s", out)
	}
}

func TestRunRejectsUnsafeClientTransportLimits(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run([]string{
		"--url", "http://127.0.0.1:1",
		"--api-key", "sk-loadtestsub",
		"--token-profile", "subscription",
		"--max-requests", "1",
		"--max-conns-per-host", "1024",
	}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("exit code = %d, stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "client transport limits exceed smoke safety maximum") {
		t.Fatalf("stderr = %s", stderr.String())
	}
}
