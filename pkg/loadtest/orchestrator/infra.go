package orchestrator

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/pkg/loadtest/artifact"
	loadtestconfig "github.com/QuantumNous/new-api/pkg/loadtest/config"
	"github.com/QuantumNous/new-api/pkg/loadtest/resource"
	redis "github.com/go-redis/redis/v8"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var findExecutableFn = findExecutable

func startManagedInfraProcesses(ctx context.Context, opts Options, cfg loadtestconfig.File) (Process, error) {
	if err := ensureManagedInfraPortsClosed(ctx, cfg); err != nil {
		return nil, err
	}
	redisProc, err := startManagedRedis(ctx, opts, cfg)
	if err != nil {
		return nil, err
	}
	proc := &managedInfraProcess{redis: redisProc}
	postgresProc, err := startManagedPostgres(ctx, opts, cfg)
	if err != nil {
		_ = proc.Stop(ctx)
		return nil, err
	}
	proc.postgres = postgresProc
	if err := verifyExternalIsolatedInfra(ctx, cfg); err != nil {
		_ = proc.Stop(ctx)
		return nil, err
	}
	return proc, nil
}

func ensureManagedInfraPortsClosed(ctx context.Context, cfg loadtestconfig.File) error {
	ports, err := resource.PortsFromConfig(cfg)
	if err != nil {
		return err
	}
	for _, port := range ports {
		if port != managedPostgresPort && port != managedRedisPort {
			continue
		}
		conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), 300*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return fmt.Errorf("loadtest infra port %d is already open; stop existing service or use --external-isolated-infra", port)
		}
	}
	return nil
}

func startManagedRedis(ctx context.Context, opts Options, cfg loadtestconfig.File) (Process, error) {
	redisServer, err := findExecutableFn("redis-server", "redis-server.exe", []string{filepath.Join(os.Getenv("USERPROFILE"), "redis", "redis-server.exe"), filepath.Join("C:", "Users", "34404", "redis", "redis-server.exe")})
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(opts.ArtifactDir, "infra", "redis")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	confPath := filepath.Join(dir, "redis.conf")
	conf := fmt.Sprintf("bind 127.0.0.1\nport %d\ndir %s\nsave \"\"\nappendonly no\nprotected-mode yes\ndaemonize no\n", managedRedisPort, filepath.ToSlash(dir))
	if err := os.WriteFile(confPath, []byte(conf), 0o600); err != nil {
		return nil, err
	}
	stdout, stderr, err := processLogs(opts.ArtifactDir, "redis")
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(redisServer, confPath)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		_ = stdout.Close()
		_ = stderr.Close()
		return nil, err
	}
	proc := &redisManagedProcess{cmd: cmd, stdout: stdout, stderr: stderr}
	if err := waitRedis(ctx, cfg.Redis.Addr, 10*time.Second); err != nil {
		_ = proc.Stop(ctx)
		return nil, err
	}
	return proc, nil
}

func startManagedPostgres(ctx context.Context, opts Options, cfg loadtestconfig.File) (Process, error) {
	binDir := postgresBinDir()
	initdbPath, err := findExecutableFn("initdb", "initdb.exe", []string{filepath.Join(binDir, "initdb.exe")})
	if err != nil {
		return nil, err
	}
	pgCtlPath, err := findExecutableFn("pg_ctl", "pg_ctl.exe", []string{filepath.Join(binDir, "pg_ctl.exe")})
	if err != nil {
		return nil, err
	}
	createdbPath, err := findExecutableFn("createdb", "createdb.exe", []string{filepath.Join(binDir, "createdb.exe")})
	if err != nil {
		return nil, err
	}
	dataDir := filepath.Join(opts.ArtifactDir, "infra", "postgres-data")
	if err := os.MkdirAll(filepath.Dir(dataDir), 0o755); err != nil {
		return nil, err
	}
	if _, err := os.Stat(filepath.Join(dataDir, "PG_VERSION")); os.IsNotExist(err) {
		cmd := exec.CommandContext(ctx, initdbPath, "-D", dataDir, "-U", postgresLoadtestUser, "--auth=trust", "--encoding=UTF8")
		if err := runCommandRedacted(cmd); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	logPath := filepath.Join(opts.ArtifactDir, "logs", "postgres.log")
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, pgCtlPath, "start", "-D", dataDir, "-l", logPath, "-o", fmt.Sprintf("-h 127.0.0.1 -p %d", managedPostgresPort), "-w", "-t", "30")
	if err := runCommandRedacted(cmd); err != nil {
		return nil, err
	}
	proc := &pgCtlProcess{pgCtl: pgCtlPath, dataDir: dataDir}
	if err := createLoadtestDatabase(ctx, createdbPath, cfg.Postgres.DSN); err != nil {
		_ = proc.Stop(ctx)
		return nil, err
	}
	if err := waitPostgres(ctx, cfg.Postgres.DSN, 10*time.Second); err != nil {
		_ = proc.Stop(ctx)
		return nil, err
	}
	return proc, nil
}

func processLogs(artifactDir string, name string) (*os.File, *os.File, error) {
	logDir := filepath.Join(artifactDir, "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, nil, err
	}
	stdout, err := os.OpenFile(filepath.Join(logDir, name+".stdout.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, nil, err
	}
	stderr, err := os.OpenFile(filepath.Join(logDir, name+".stderr.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		_ = stdout.Close()
		return nil, nil, err
	}
	return stdout, stderr, nil
}

func postgresBinDir() string {
	if runtime.GOOS == "windows" {
		return filepath.Join(`C:\`, "Program Files", "PostgreSQL", "18", "bin")
	}
	return ""
}

func findExecutable(name string, windowsName string, candidates []string) (string, error) {
	if path, err := exec.LookPath(name); err == nil {
		return path, nil
	}
	if runtime.GOOS == "windows" {
		if path, err := exec.LookPath(windowsName); err == nil {
			return path, nil
		}
	}
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate) == "" {
			continue
		}
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("%s executable not found", name)
}

func waitRedis(ctx context.Context, addr string, timeout time.Duration) error {
	opt, err := redis.ParseURL(redisURL(addr))
	if err != nil {
		return err
	}
	client := redis.NewClient(opt)
	defer client.Close()
	return waitFor(ctx, timeout, func(ctx context.Context) error { return client.Ping(ctx).Err() })
}

func waitPostgres(ctx context.Context, dsn string, timeout time.Duration) error {
	return waitFor(ctx, timeout, func(ctx context.Context) error {
		db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
		if err != nil {
			return err
		}
		sqlDB, err := db.DB()
		if err != nil {
			return err
		}
		defer sqlDB.Close()
		return sqlDB.PingContext(ctx)
	})
}

func waitFor(ctx context.Context, timeout time.Duration, probe func(context.Context) error) error {
	deadline, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	var last error
	for {
		last = probe(deadline)
		if last == nil {
			return nil
		}
		select {
		case <-deadline.Done():
			return last
		case <-ticker.C:
		}
	}
}

func createLoadtestDatabase(ctx context.Context, createdbPath string, targetDSN string) error {
	if err := waitPostgres(ctx, postgresAdminDSNString(targetDSN), 10*time.Second); err != nil {
		return err
	}
	cmd, err := buildCreateDatabaseCommand(ctx, createdbPath, targetDSN)
	if err != nil {
		return err
	}
	if err := runCommandRedacted(cmd); err != nil {
		if strings.Contains(err.Error(), "already exists") || strings.Contains(err.Error(), "已存在") {
			return nil
		}
		return err
	}
	return nil
}

func buildCreateDatabaseCommand(ctx context.Context, createdbPath string, targetDSN string) (*exec.Cmd, error) {
	adminDSN, err := postgresAdminDSN(targetDSN)
	if err != nil {
		return nil, err
	}
	return exec.CommandContext(ctx, createdbPath, "--maintenance-db", adminDSN, postgresLoadtestDatabase), nil
}

func postgresAdminDSNString(raw string) string {
	dsn, err := postgresAdminDSN(raw)
	if err != nil {
		return raw
	}
	return dsn
}

func postgresAdminDSN(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	u.Path = "/postgres"
	return u.String(), nil
}

func runCommandRedacted(cmd *exec.Cmd) error {
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s failed: %w: %s", filepath.Base(cmd.Path), err, artifact.Redact(stderr.String()))
	}
	return nil
}
