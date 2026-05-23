package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/loadtest/artifact"
	loadconfig "github.com/QuantumNous/new-api/pkg/loadtest/config"
)

func main() {
	os.Exit(Run(os.Args[1:], os.Stdout, os.Stderr))
}

func Run(args []string, stdout io.Writer, stderr io.Writer) int {
	fs := flag.NewFlagSet("loadtest-check-config", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	configPath := fs.String("config", "config.loadtest.yaml", "loadtest config path")
	outEnv := fs.String("out-env", "", "env output path")
	outRunContext := fs.String("out-run-context", "", "run context output path")
	role := fs.String("role", "baseline", "run role")
	commit := fs.String("commit", "", "commit id")
	if err := fs.Parse(args); err != nil {
		writeErr(stderr, err)
		return 2
	}
	if strings.TrimSpace(*role) == "" {
		writeErr(stderr, errors.New("role is required"))
		return 2
	}
	file, err := loadconfig.Load(*configPath)
	if err != nil {
		writeErr(stderr, err)
		if strings.HasPrefix(err.Error(), "parse config:") {
			return 2
		}
		return 1
	}
	if err := file.Validate(); err != nil {
		writeErr(stderr, err)
		return 2
	}
	resolvedCommit := strings.TrimSpace(*commit)
	if resolvedCommit == "" {
		resolvedCommit = gitCommit()
	}
	rc, err := file.BaseRunContext(resolvedCommit)
	if err != nil {
		writeErr(stderr, err)
		return 2
	}
	rc.Role = strings.TrimSpace(*role)
	if *outEnv != "" {
		if err := writeBenchmarkEnvFile(*outEnv, file); err != nil {
			writeErr(stderr, err)
			return 1
		}
	}
	if *outRunContext != "" {
		if err := writeRunContext(*outRunContext, rc); err != nil {
			writeErr(stderr, err)
			return 1
		}
	}
	fmt.Fprintf(stdout, "config ok role=%s commit=%s comparison_config_hash=%s\n", rc.Role, rc.Commit, rc.ComparisonConfigHash)
	return 0
}

func writeBenchmarkEnvFile(path string, file *loadconfig.File) error {
	env, err := file.NewAPIEnvForProfile("benchmark")
	if err != nil {
		return err
	}
	return writeEnvFile(path, env)
}

func writeEnvFile(path string, env map[string]string) error {
	var b bytes.Buffer
	for _, key := range loadconfig.EnvKeys {
		value, ok := env[key]
		if !ok {
			return fmt.Errorf("missing env key %s", key)
		}
		b.WriteString(key)
		b.WriteByte('=')
		b.WriteString(value)
		b.WriteByte('\n')
	}
	for _, key := range sortedExtraEnvKeys(env) {
		b.WriteString(key)
		b.WriteByte('=')
		b.WriteString(env[key])
		b.WriteByte('\n')
	}
	return writeFile(path, b.Bytes())
}

func sortedExtraEnvKeys(env map[string]string) []string {
	known := make(map[string]struct{}, len(loadconfig.EnvKeys))
	for _, key := range loadconfig.EnvKeys {
		known[key] = struct{}{}
	}
	extra := make([]string, 0)
	for key := range env {
		if _, ok := known[key]; !ok {
			extra = append(extra, key)
		}
	}
	sort.Strings(extra)
	return extra
}

func writeRunContext(path string, rc artifact.RunContext) error {
	b, err := common.Marshal(rc)
	if err != nil {
		return err
	}
	return writeFile(path, append(b, '\n'))
}

func writeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func gitCommit() string {
	cmd := exec.Command("git", "rev-parse", "--short", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	commit := strings.TrimSpace(string(out))
	if commit == "" {
		return "unknown"
	}
	return commit
}

func writeErr(w io.Writer, err error) {
	fmt.Fprintln(w, artifact.Redact(err.Error()))
}
