package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/loadtest/artifact"
	loadtestconfig "github.com/QuantumNous/new-api/pkg/loadtest/config"
	"github.com/QuantumNous/new-api/pkg/loadtest/metrics"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func main() { os.Exit(Run(os.Args[1:], os.Stdout, os.Stderr)) }

func Run(args []string, stdout io.Writer, stderr io.Writer) int {
	fs := flag.NewFlagSet("loadtest-collect", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	configPath := fs.String("config", "", "loadtest config")
	_ = fs.String("pid-file", "", "pid file")
	runContextPath := fs.String("run-context", "", "run context")
	seedOutputPath := fs.String("seed-output", "", "seed output")
	summaryPath := fs.String("summary", "", "summary")
	beforePath := fs.String("before", "", "before snapshot")
	outSnapshotPath := fs.String("out-snapshot", "", "snapshot output")
	outDiffPath := fs.String("out-diff", "", "diff output")
	mockDeltaPath := fs.String("mock-stats-delta", "", "mock stats delta")
	stdoutLogPath := fs.String("stdout-log", "", "stdout log")
	stderrLogPath := fs.String("stderr-log", "", "stderr log")
	_ = fs.Duration("wait-drain", 0, "drain wait")
	if err := fs.Parse(args); err != nil {
		writeErr(stderr, err)
		return 2
	}
	var cfg *loadtestconfig.File
	if *configPath != "" {
		loaded, err := loadtestconfig.Load(*configPath)
		if err != nil {
			writeErr(stderr, err)
			return 1
		}
		if err := loaded.Validate(); err != nil {
			writeErr(stderr, err)
			return 2
		}
		cfg = loaded
	}
	var rc artifact.RunContext
	if *runContextPath != "" {
		if err := readJSON(*runContextPath, &rc); err != nil {
			writeErr(stderr, err)
			return 1
		}
	}
	var db *gorm.DB
	if cfg != nil {
		opened, err := gorm.Open(postgres.Open(cfg.Postgres.DSN), &gorm.Config{})
		if err != nil {
			writeErr(stderr, err)
			return 1
		}
		db = opened
	}
	var seed artifact.SeedOutput
	if *seedOutputPath != "" {
		if err := readJSON(*seedOutputPath, &seed); err != nil {
			writeErr(stderr, err)
			return 1
		}
	}
	if *outSnapshotPath != "" {
		business := artifact.BusinessSnapshot{Statused: artifact.Statused{Status: "unavailable", Reason: "collector database is not configured"}}
		if db != nil {
			var err error
			business, err = metrics.LoadBusinessSnapshot(db, seed)
			if err != nil {
				writeErr(stderr, err)
				return 1
			}
		}
		snap := artifact.Snapshot{SchemaVersion: artifact.SchemaVersion, RunContext: rc, Postgres: artifact.PostgresSnapshot{Statused: artifact.Statused{Status: "unavailable", Reason: "collector database system snapshot is not configured in minimal mode"}}, Redis: artifact.RedisSnapshot{Statused: artifact.Statused{Status: "unavailable", Reason: "collector redis snapshot is not configured in minimal mode"}}, Runtime: artifact.RuntimeSnapshot{Statused: artifact.Statused{Status: "unavailable", Reason: "runtime URL is not provided in minimal mode"}}, Logs: metrics.ScanServerLogs(readText(*stdoutLogPath), readText(*stderrLogPath))}
		snap.Process.Statused = artifact.Statused{Status: "unavailable", Reason: "process sampler is not configured in minimal mode"}
		snap.Postgres.Statused = business.Statused
		snap.Business = business
		if err := writeJSONFile(*outSnapshotPath, snap); err != nil {
			writeErr(stderr, err)
			return 1
		}
		fmt.Fprintf(stdout, "snapshot written %s\n", *outSnapshotPath)
	}
	if *outDiffPath != "" {
		var before artifact.Snapshot
		if *beforePath != "" {
			if err := readJSON(*beforePath, &before); err != nil {
				writeErr(stderr, err)
				return 1
			}
		}
		var after artifact.Snapshot
		if *outSnapshotPath != "" {
			if err := readJSON(*outSnapshotPath, &after); err != nil {
				writeErr(stderr, err)
				return 1
			}
		}
		var summary artifact.Summary
		if *summaryPath != "" {
			if err := readJSON(*summaryPath, &summary); err != nil {
				writeErr(stderr, err)
				return 1
			}
		}
		var mock artifact.MockStatsDelta
		if *mockDeltaPath != "" {
			if err := readJSON(*mockDeltaPath, &mock); err != nil {
				writeErr(stderr, err)
				return 1
			}
		}
		var logRows []metrics.ConsumeLogRow
		var preRows []metrics.PreConsumeRow
		if db != nil {
			var err error
			logRows, preRows, err = metrics.LoadBusinessRows(db, summary)
			if err != nil {
				writeErr(stderr, err)
				return 1
			}
		}
		diff, inv := metrics.BuildDiff(metrics.DiffInputs{Before: before, After: after, Summary: summary, SeedOutput: seed, MockDelta: mock, RunContext: rc, StdoutLog: readText(*stdoutLogPath), StderrLog: readText(*stderrLogPath), ConsumeLogRows: logRows, PreConsumeRows: preRows, BusinessBefore: before.Business, BusinessAfter: after.Business})
		if err := writeJSONFile(*outDiffPath, diff); err != nil {
			writeErr(stderr, err)
			return 1
		}
		if inv.Status == "failed" {
			_, _ = fmt.Fprintln(stderr, artifact.Redact(inv.Reason))
			return 2
		}
		fmt.Fprintf(stdout, "diff written %s\n", *outDiffPath)
	}
	return 0
}

func readText(path string) string {
	if path == "" {
		return ""
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(b)
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

func writeErr(w io.Writer, err error) { _, _ = fmt.Fprintln(w, artifact.Redact(err.Error())) }
