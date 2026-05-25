package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/loadtest/artifact"
	loadtestconfig "github.com/QuantumNous/new-api/pkg/loadtest/config"
	"github.com/QuantumNous/new-api/pkg/loadtest/metrics"
	"github.com/QuantumNous/new-api/pkg/loadtest/monitor"
	"github.com/QuantumNous/new-api/pkg/loadtest/resource"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func main() { os.Exit(Run(os.Args[1:], os.Stdout, os.Stderr)) }

func Run(args []string, stdout io.Writer, stderr io.Writer) int {
	fs := flag.NewFlagSet("loadtest-collect", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	configPath := fs.String("config", "", "loadtest config")
	pidFilePath := fs.String("pid-file", "", "pid file")
	runtimeURL := fs.String("runtime-url", "", "runtime stats URL")
	redisAddr := fs.String("redis-addr", "", "redis addr")
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
	var logDB *gorm.DB
	if cfg != nil {
		opened, err := gorm.Open(postgres.Open(cfg.Postgres.DSN), &gorm.Config{})
		if err != nil {
			writeErr(stderr, err)
			return 1
		}
		db = opened
		logDB = db
		if cfg.LogPostgres.DSN != "" && cfg.LogPostgres.DSN != cfg.Postgres.DSN {
			openedLog, err := gorm.Open(postgres.Open(cfg.LogPostgres.DSN), &gorm.Config{})
			if err != nil {
				writeErr(stderr, err)
				return 1
			}
			logDB = openedLog
		}
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
		postgresSnapshot := artifact.PostgresSnapshot{Statused: artifact.Statused{Status: "unavailable", Reason: "collector database system snapshot is not configured"}}
		if db != nil {
			var err error
			business, err = metrics.LoadBusinessSnapshot(db, seed)
			if err != nil {
				writeErr(stderr, err)
				return 1
			}
			postgresSnapshot = monitor.LoadPostgresSnapshotWithLogDB(db, logDB, nil)
		}
		processSnapshot, err := collectProcessSnapshot(*pidFilePath)
		if err != nil {
			writeErr(stderr, err)
			return 1
		}
		runtimeSnapshot := artifact.RuntimeSnapshot{Statused: artifact.Statused{Status: "unavailable", Reason: "runtime URL is not provided"}}
		if strings.TrimSpace(*runtimeURL) != "" {
			runtimeSnapshot = monitor.ReadRuntimeSnapshot(context.Background(), *runtimeURL)
			if strings.HasPrefix(runtimeSnapshot.Reason, "config:") {
				writeErr(stderr, fmt.Errorf("%s", runtimeSnapshot.Reason))
				return 2
			}
		}
		redisSnapshot := artifact.RedisSnapshot{Statused: artifact.Statused{Status: "unavailable", Reason: "redis addr is not provided"}}
		if strings.TrimSpace(*redisAddr) != "" {
			redisSnapshot = monitor.LoadRedisSnapshot(context.Background(), *redisAddr)
			if strings.HasPrefix(redisSnapshot.Reason, "config:") {
				writeErr(stderr, fmt.Errorf("%s", redisSnapshot.Reason))
				return 2
			}
		}
		snap := artifact.Snapshot{SchemaVersion: artifact.SchemaVersion, RunContext: rc, Process: processSnapshot, Postgres: postgresSnapshot, Redis: redisSnapshot, Runtime: runtimeSnapshot, Logs: metrics.ScanServerLogs(readText(*stdoutLogPath), readText(*stderrLogPath)), Business: business}
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
			logRows, preRows, err = metrics.LoadBusinessRowsWithLogDB(db, logDB, summary)
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
		if businessInv := metrics.DiffFailedInvariant(diff); businessInv.Status == "failed" {
			_, _ = fmt.Fprintln(stderr, artifact.Redact(businessInv.Reason))
			return 2
		}
		fmt.Fprintf(stdout, "diff written %s\n", *outDiffPath)
	}
	return 0
}

func collectProcessSnapshot(pidFile string) (artifact.ProcessSnapshot, error) {
	pid, err := ReadPIDFile(pidFile)
	if err != nil {
		return artifact.ProcessSnapshot{}, err
	}
	if pid == 0 {
		return artifact.ProcessSnapshot{Statused: artifact.Statused{Status: "unavailable", Reason: "pid file is not provided"}}, nil
	}
	snapshot := resource.SampleProcess(pid)
	if snapshot.Status == "unavailable" {
		return artifact.ProcessSnapshot{}, fmt.Errorf("process snapshot unavailable: %s", snapshot.Reason)
	}
	return snapshot, nil
}

func ReadPIDFile(path string) (int, error) {
	if strings.TrimSpace(path) == "" {
		return 0, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	raw := strings.TrimSpace(string(b))
	if raw == "" {
		return 0, nil
	}
	pid, err := strconv.Atoi(raw)
	if err != nil || pid <= 0 {
		return 0, fmt.Errorf("invalid pid file %s", path)
	}
	return pid, nil
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
