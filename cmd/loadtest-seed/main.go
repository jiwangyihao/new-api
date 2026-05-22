package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/loadtest/artifact"
	loadtestconfig "github.com/QuantumNous/new-api/pkg/loadtest/config"
	"github.com/QuantumNous/new-api/pkg/loadtest/seed"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func main() {
	os.Exit(Run(os.Args[1:], os.Stdout, os.Stderr))
}

func Run(args []string, stdout io.Writer, stderr io.Writer) int {
	fs := flag.NewFlagSet("loadtest-seed", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	configPath := fs.String("config", "", "loadtest config")
	runContextPath := fs.String("run-context", "", "base run context")
	outPath := fs.String("out", "", "seed output")
	outRunContextPath := fs.String("out-run-context", "", "seeded run context output")
	if err := fs.Parse(args); err != nil {
		writeErr(stderr, err)
		return 2
	}
	if *configPath == "" || *runContextPath == "" || *outPath == "" || *outRunContextPath == "" {
		writeErr(stderr, fmt.Errorf("--config, --run-context, --out and --out-run-context are required"))
		return 2
	}
	cfgFile, err := loadtestconfig.Load(*configPath)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	if err := cfgFile.Validate(); err != nil {
		writeErr(stderr, err)
		return 2
	}
	var rc artifact.RunContext
	if err := readJSON(*runContextPath, &rc); err != nil {
		writeErr(stderr, err)
		return 1
	}
	db, err := gorm.Open(postgres.Open(cfgFile.Postgres.DSN), &gorm.Config{})
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	common.UsingSQLite = false
	common.UsingMySQL = false
	common.UsingPostgreSQL = true
	common.LogSqlType = common.DatabaseTypePostgreSQL
	model.DB = db
	model.LOG_DB = db
	seedCfg := seed.Config{RunContext: rc.WithoutSeedOutputHash().WithoutMockHash(), Model: cfgFile.Loadtest.Model, Group: cfgFile.Loadtest.Group, MockBaseURL: cfgFile.MockUpstream.BaseURL, SubscriptionKey: cfgFile.Loadtest.SubscriptionKey, CompatKey: cfgFile.Loadtest.CompatKey}
	ctx, cancel := seedContext()
	defer cancel()
	out, err := seed.Apply(ctx, db, seedCfg)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	if err := writeJSONFile(*outPath, out); err != nil {
		writeErr(stderr, err)
		return 1
	}
	seedHash, err := artifact.HashSeedOutput(out)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	seeded := rc.WithoutMockHash()
	seeded.SeedOutputHash = seedHash
	if err := writeJSONFile(*outRunContextPath, seeded); err != nil {
		writeErr(stderr, err)
		return 1
	}
	fmt.Fprintf(stdout, "seed ok seed_output_hash=%s\n", seedHash)
	return 0
}

func seedContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt)
}

func readJSON(path string, v any) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return common.DecodeJson(f, v)
}

func writeJSONFile(path string, v any) error {
	b, err := common.Marshal(v)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o600)
}

func writeErr(w io.Writer, err error) {
	_, _ = fmt.Fprintln(w, artifact.Redact(err.Error()))
}
