package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/loadtest/artifact"
	loadtestclient "github.com/QuantumNous/new-api/pkg/loadtest/client"
	loadtestconfig "github.com/QuantumNous/new-api/pkg/loadtest/config"
	"github.com/QuantumNous/new-api/pkg/loadtest/localguard"
	"github.com/QuantumNous/new-api/pkg/loadtest/mockopenai"
	"github.com/QuantumNous/new-api/pkg/loadtest/profile"
	"github.com/QuantumNous/new-api/pkg/loadtest/report"
	"github.com/QuantumNous/new-api/pkg/loadtest/resource"
	"github.com/QuantumNous/new-api/pkg/loadtest/runner"
	"github.com/QuantumNous/new-api/pkg/loadtest/seed"
	"github.com/QuantumNous/new-api/pkg/loadtest/sweep"
	redis "github.com/go-redis/redis/v8"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

const (
	managedPostgresPort = 15432
	managedRedisPort    = 16379
)

var startManagedInfra = startManagedInfraProcesses

var startupTimeout = 60 * time.Second

var cleanupPortsTimeout = 30 * time.Second
var cleanupPortsPollInterval = 100 * time.Millisecond

const (
	postgresLoadtestUser     = "new_api_loadtest"
	postgresLoadtestPassword = "loadtest"
	postgresLoadtestDatabase = "new_api_loadtest"
)

type managedInfraProcess struct {
	postgres Process
	redis    Process
}

func (p *managedInfraProcess) PID() int {
	if p == nil || p.postgres == nil {
		return 0
	}
	return p.postgres.PID()
}

func (p *managedInfraProcess) Stop(ctx context.Context) error {
	var first error
	if p == nil {
		return nil
	}
	if p.postgres != nil {
		if err := p.postgres.Stop(ctx); err != nil && first == nil {
			first = err
		}
		p.postgres = nil
	}
	if p.redis != nil {
		if err := p.redis.Stop(ctx); err != nil && first == nil {
			first = err
		}
		p.redis = nil
	}
	return first
}

type redisManagedProcess struct {
	cmd    *exec.Cmd
	stdout *os.File
	stderr *os.File
}

func (p *redisManagedProcess) PID() int {
	if p == nil || p.cmd == nil || p.cmd.Process == nil {
		return 0
	}
	return p.cmd.Process.Pid
}

func (p *redisManagedProcess) Stop(ctx context.Context) error {
	if p == nil || p.cmd == nil || p.cmd.Process == nil {
		return nil
	}
	_ = p.cmd.Process.Kill()
	err := p.cmd.Wait()
	if p.stdout != nil {
		_ = p.stdout.Close()
	}
	if p.stderr != nil {
		_ = p.stderr.Close()
	}
	return err
}

type pgCtlProcess struct {
	pgCtl   string
	dataDir string
}

func (p *pgCtlProcess) PID() int { return 0 }

func (p *pgCtlProcess) Stop(ctx context.Context) error {
	if p == nil || p.pgCtl == "" || p.dataDir == "" {
		return nil
	}
	cmd := exec.CommandContext(ctx, p.pgCtl, "stop", "-D", p.dataDir, "-m", "fast", "-w", "-t", "30")
	return runCommandRedacted(cmd)
}

type Process interface {
	PID() int
	Stop(context.Context) error
}

type Options struct {
	ConfigPath            string
	Config                loadtestconfig.File
	Binary                string
	WorkDir               string
	ArtifactDir           string
	Profile               string
	Scenario              string
	Path                  string
	TokenProfile          string
	APIKey                string
	MockProfile           string
	Points                []int
	RequestsPerPoint      int
	RampStep              int
	RampInterval          time.Duration
	Duration              time.Duration
	Timeout               time.Duration
	StartupTimeout        time.Duration
	ExternalIsolatedInfra bool
	RunContext            artifact.RunContext
	Commit                string
}

type PointOptions struct {
	Concurrency      int
	BaseURL          string
	RuntimeURL       string
	APIKey           string
	TokenProfile     string
	Path             string
	Model            string
	Scenario         string
	ArtifactDir      string
	RunContext       artifact.RunContext
	Config           *loadtestconfig.File
	MockProfile      string
	MockHash         string
	MockStatsURL     string
	RequestsPerPoint int
	MaxRequests      int
	RampStep         int
	RampInterval     time.Duration
	Duration         time.Duration
	Timeout          time.Duration
	Transport        artifact.TransportProfile
	Seed             artifact.SeedOutput
	DB               *gorm.DB
	ServerPID        int
}

type Dependencies struct {
	BuildOrVerifyBinary func(context.Context, Options) error
	RunConfigCheck      func(context.Context, Options) error
	StartInfra          func(context.Context, Options, loadtestconfig.File) (Process, error)
	StopInfra           func(context.Context, Process) error
	PreflightInfra      func(context.Context, Options, loadtestconfig.File) error
	StartMock           func(context.Context, Options, artifact.RunContext) (Process, error)
	StopMock            func(context.Context, Process) error
	StartServer         func(context.Context, Options, map[string]string) (Process, error)
	StopServer          func(context.Context, Process) error
	BootstrapAndSeed    func(context.Context, Options, artifact.RunContext) (artifact.SeedOutput, error)
	RunPoint            func(context.Context, PointOptions) (artifact.PointResult, artifact.PointAnalysis, artifact.ResourceSamplesArtifact, error)
	ApplyLimits         func(pid int, limits profile.ServerLimits) (resource.ApplyResult, error)
	CheckPorts          func(artifact.RunContext, []int) artifact.PortsClosedArtifact
	RenderReport        func(context.Context, Options, artifact.SweepResult, []artifact.PointAnalysis, []artifact.ResourceSamplesArtifact, artifact.ResourceLimitsArtifact, artifact.PortsClosedArtifact) error
	WriteJSON           func(string, any) error
}

type osProcess struct {
	cmd *exec.Cmd
}

func (p *osProcess) PID() int {
	if p == nil || p.cmd == nil || p.cmd.Process == nil {
		return 0
	}
	return p.cmd.Process.Pid
}

func (p *osProcess) Stop(context.Context) error {
	if p == nil || p.cmd == nil || p.cmd.Process == nil {
		return nil
	}
	_ = p.cmd.Process.Kill()
	return p.cmd.Wait()
}

func Run(ctx context.Context, opts Options, deps Dependencies) (artifact.SweepResult, int) {
	deps = fillDependencies(deps)
	cfg, cfgSet, err := resolveConfig(opts)
	if err != nil {
		return artifact.SweepResult{}, 2
	}
	opts.Config = cfg
	if !cfgSet {
		return artifact.SweepResult{}, 2
	}
	p, err := cfg.Profile(opts.Profile)
	if err != nil {
		return artifact.SweepResult{}, 2
	}
	if p.Name != "benchmark" {
		return artifact.SweepResult{}, 2
	}
	if err := validateOptions(opts); err != nil {
		return artifact.SweepResult{}, 2
	}
	applyProfileDefaults(&opts, p)
	mockHash := cfg.MockProfileHash(opts.MockProfile)
	if mockHash == "" {
		return artifact.SweepResult{}, 2
	}
	baseRC, err := baseRunContext(opts, cfg)
	if err != nil {
		return artifact.SweepResult{}, 2
	}
	baseRC.Scenario = opts.Scenario
	baseRC.Path = opts.Path
	baseRC.TokenProfile = opts.TokenProfile
	baseRC.MockHash = mockHash
	baseRC.Model = cfg.Loadtest.Model

	if err := deps.BuildOrVerifyBinary(ctx, opts); err != nil {
		return artifact.SweepResult{}, 2
	}
	if err := deps.RunConfigCheck(ctx, opts); err != nil {
		return artifact.SweepResult{}, 2
	}
	if err := deps.PreflightInfra(ctx, opts, cfg); err != nil {
		return artifact.SweepResult{}, 2
	}

	var infra Process
	var mock Process
	var server Process
	var ports artifact.PortsClosedArtifact
	var limitsArtifact artifact.ResourceLimitsArtifact
	var finalCode int
	defer func() {
		_ = finalCode
	}()

	cleanup := func(rc artifact.RunContext, code int) int {
		if server != nil {
			_ = deps.StopServer(ctx, server)
			server = nil
		}
		if mock != nil {
			_ = deps.StopMock(ctx, mock)
			mock = nil
		}
		if infra != nil {
			_ = deps.StopInfra(ctx, infra)
			infra = nil
		}
		checkPorts, err := cleanupPortsForConfig(cfg, opts.ExternalIsolatedInfra)
		if err != nil {
			ports = failedPortsArtifact(rc, err)
		} else {
			ports = waitCleanupPortsClosed(ctx, deps, rc, checkPorts, cleanupPortsTimeout)
		}
		if opts.ArtifactDir != "" {
			if err := deps.WriteJSON(filepath.Join(opts.ArtifactDir, "ports-closed.json"), ports); err != nil && code == 0 {
				return 1
			}
		}
		if !ports.Passed && code == 0 {
			return 2
		}
		return code
	}

	startupCtx, cancelStartup := context.WithTimeout(ctx, effectiveStartupTimeout(opts))
	defer cancelStartup()

	infra, err = deps.StartInfra(startupCtx, opts, cfg)
	if err != nil {
		return artifact.SweepResult{SchemaVersion: artifact.SchemaVersion, RunContext: baseRC}, cleanup(baseRC, 1)
	}
	seedOut, err := deps.BootstrapAndSeed(startupCtx, opts, baseRC)
	if err != nil {
		return artifact.SweepResult{SchemaVersion: artifact.SchemaVersion, RunContext: baseRC}, cleanup(baseRC, 1)
	}
	seedHash, err := artifact.HashSeedOutput(seedOut)
	if err != nil {
		return artifact.SweepResult{SchemaVersion: artifact.SchemaVersion, RunContext: baseRC}, cleanup(baseRC, 1)
	}
	rc := baseRC
	rc.SeedOutputHash = seedHash

	mock, err = deps.StartMock(startupCtx, opts, rc)
	if err != nil {
		return artifact.SweepResult{SchemaVersion: artifact.SchemaVersion, RunContext: rc}, cleanup(rc, 1)
	}
	env, err := cfg.NewAPIEnvForProfile("benchmark")
	if err != nil {
		return artifact.SweepResult{SchemaVersion: artifact.SchemaVersion, RunContext: rc}, cleanup(rc, 2)
	}
	server, err = deps.StartServer(startupCtx, opts, env)
	if err != nil {
		return artifact.SweepResult{SchemaVersion: artifact.SchemaVersion, RunContext: rc}, cleanup(rc, 1)
	}
	limitResult, limitErr := deps.ApplyLimits(server.PID(), p.ServerLimits)
	limitsArtifact = resource.BuildLimitsArtifact(rc, p.ServerLimits, limitResult)
	if opts.ArtifactDir != "" {
		if err := deps.WriteJSON(filepath.Join(opts.ArtifactDir, "resource-limits.json"), limitsArtifact); err != nil {
			return artifact.SweepResult{SchemaVersion: artifact.SchemaVersion, RunContext: rc}, cleanup(rc, 1)
		}
	}
	if limitErr != nil || limitResult.ShouldFailOrchestrator() {
		return artifact.SweepResult{SchemaVersion: artifact.SchemaVersion, RunContext: rc}, cleanup(rc, 1)
	}

	result := artifact.SweepResult{SchemaVersion: artifact.SchemaVersion, RunContext: rc, Scenario: opts.Scenario, Path: opts.Path, TokenProfile: opts.TokenProfile, RunID: time.Now().UTC().Format("20060102T150405Z") + "-" + sanitizeName(opts.Scenario)}
	analyses := make([]artifact.PointAnalysis, 0, len(opts.Points))
	resources := make([]artifact.ResourceSamplesArtifact, 0, len(opts.Points))
	for _, c := range opts.Points {
		point, analysis, samples, err := deps.RunPoint(ctx, pointOptions(opts, cfg, p, rc, seedOut, c, server.PID()))
		if err != nil {
			point = artifact.PointResult{Concurrency: c, Passed: false, Gate: artifact.GateResult{Passed: false, FailedReasons: []string{err.Error()}}}
		}
		result.Points = append(result.Points, point)
		analyses = append(analyses, analysis)
		resources = append(resources, samples)
		if point.Passed {
			result.HighestPassedConcurrency = c
		} else if result.FirstFailedConcurrency == nil {
			failed := c
			result.FirstFailedConcurrency = &failed
		}
		if err != nil || !point.Passed {
			break
		}
	}
	code := 0
	if result.FirstFailedConcurrency != nil {
		code = 2
	}
	code = cleanup(rc, code)
	if err := deps.RenderReport(ctx, opts, result, analyses, resources, limitsArtifact, ports); err != nil && code == 0 {
		code = 1
	}
	return result, code
}

func DefaultDependencies() Dependencies {
	return Dependencies{
		BuildOrVerifyBinary: buildOrVerifyBinary,
		RunConfigCheck:      runConfigCheck,
		PreflightInfra:      preflightInfra,
		StartInfra:          startInfra,
		StopInfra:           stopProcess,
		StartMock:           startMock,
		StopMock:            stopProcess,
		StartServer:         startServer,
		StopServer:          stopProcess,
		BootstrapAndSeed:    bootstrapAndSeed,
		RunPoint:            runPoint,
		ApplyLimits:         resource.ApplyServerLimits,
		CheckPorts:          resource.CheckPortsClosed,
		RenderReport:        renderReport,
		WriteJSON:           writeJSONFile,
	}
}

func fillDependencies(deps Dependencies) Dependencies {
	defaults := DefaultDependencies()
	if deps.BuildOrVerifyBinary == nil {
		deps.BuildOrVerifyBinary = defaults.BuildOrVerifyBinary
	}
	if deps.RunConfigCheck == nil {
		deps.RunConfigCheck = defaults.RunConfigCheck
	}
	if deps.PreflightInfra == nil {
		deps.PreflightInfra = defaults.PreflightInfra
	}
	if deps.StartInfra == nil {
		deps.StartInfra = defaults.StartInfra
	}
	if deps.StopInfra == nil {
		deps.StopInfra = defaults.StopInfra
	}
	if deps.StartMock == nil {
		deps.StartMock = defaults.StartMock
	}
	if deps.StopMock == nil {
		deps.StopMock = defaults.StopMock
	}
	if deps.StartServer == nil {
		deps.StartServer = defaults.StartServer
	}
	if deps.StopServer == nil {
		deps.StopServer = defaults.StopServer
	}
	if deps.BootstrapAndSeed == nil {
		deps.BootstrapAndSeed = defaults.BootstrapAndSeed
	}
	if deps.RunPoint == nil {
		deps.RunPoint = defaults.RunPoint
	}
	if deps.ApplyLimits == nil {
		deps.ApplyLimits = defaults.ApplyLimits
	}
	if deps.CheckPorts == nil {
		deps.CheckPorts = defaults.CheckPorts
	}
	if deps.RenderReport == nil {
		deps.RenderReport = defaults.RenderReport
	}
	if deps.WriteJSON == nil {
		deps.WriteJSON = defaults.WriteJSON
	}
	return deps
}

func resolveConfig(opts Options) (loadtestconfig.File, bool, error) {
	if opts.Config.Server.Host != "" || opts.ConfigPath == "" {
		if err := opts.Config.Validate(); err != nil {
			return loadtestconfig.File{}, opts.Config.Server.Host != "", err
		}
		return opts.Config, opts.Config.Server.Host != "", nil
	}
	cfg, err := loadtestconfig.Load(opts.ConfigPath)
	if err != nil {
		return loadtestconfig.File{}, false, err
	}
	if err := cfg.Validate(); err != nil {
		return loadtestconfig.File{}, true, err
	}
	return *cfg, true, nil
}

func validateOptions(opts Options) error {
	if strings.TrimSpace(opts.ArtifactDir) == "" || strings.TrimSpace(opts.Profile) == "" {
		return fmt.Errorf("artifact-dir and profile are required")
	}
	if opts.Profile != "benchmark" {
		return fmt.Errorf("profile is not implemented")
	}
	if err := localguard.ValidateAPIKey(opts.APIKey); err != nil {
		return err
	}
	if strings.TrimSpace(opts.TokenProfile) == "" || strings.TrimSpace(opts.Path) == "" || strings.TrimSpace(opts.Scenario) == "" || strings.TrimSpace(opts.MockProfile) == "" {
		return fmt.Errorf("scenario, path, token-profile and mock-profile are required")
	}
	return loadtestclient.ValidateTokenProfile(opts.APIKey, opts.TokenProfile)
}

func applyProfileDefaults(opts *Options, p profile.Profile) {
	if len(opts.Points) == 0 {
		opts.Points = append([]int(nil), p.Points...)
	}
	if opts.RequestsPerPoint <= 0 {
		opts.RequestsPerPoint = p.RequestsPerPoint
	}
	if opts.RampStep <= 0 {
		opts.RampStep = p.RampStep
	}
	if opts.RampInterval <= 0 {
		opts.RampInterval = p.RampInterval
	}
	if opts.Duration <= 0 {
		opts.Duration = p.Duration
	}
	if opts.Timeout <= 0 {
		opts.Timeout = p.Timeout
	}
}

func effectiveStartupTimeout(opts Options) time.Duration {
	if opts.StartupTimeout > 0 {
		return opts.StartupTimeout
	}
	return startupTimeout
}

func baseRunContext(opts Options, cfg loadtestconfig.File) (artifact.RunContext, error) {
	if opts.RunContext != (artifact.RunContext{}) {
		return opts.RunContext, nil
	}
	return cfg.BaseRunContext(opts.Commit)
}

func pointOptions(opts Options, cfg loadtestconfig.File, p profile.Profile, rc artifact.RunContext, seed artifact.SeedOutput, c int, serverPID int) PointOptions {
	return PointOptions{
		Concurrency:      c,
		BaseURL:          baseURL(cfg),
		RuntimeURL:       runtimeURL(cfg),
		APIKey:           opts.APIKey,
		TokenProfile:     opts.TokenProfile,
		Path:             opts.Path,
		Model:            cfg.Loadtest.Model,
		Scenario:         opts.Scenario,
		ArtifactDir:      opts.ArtifactDir,
		RunContext:       rc,
		Config:           &cfg,
		MockProfile:      opts.MockProfile,
		MockHash:         rc.MockHash,
		MockStatsURL:     strings.TrimRight(cfg.MockUpstream.BaseURL, "/") + "/debug/loadtest/mock-stats",
		RequestsPerPoint: opts.RequestsPerPoint,
		MaxRequests:      opts.RequestsPerPoint,
		RampStep:         opts.RampStep,
		RampInterval:     opts.RampInterval,
		Duration:         opts.Duration,
		Timeout:          opts.Timeout,
		Transport:        artifact.TransportProfile{Mode: p.Transport.Mode, MaxConnsPerHost: p.Transport.MaxConnsPerHost, MaxIdleConns: p.Transport.MaxIdleConns, MaxIdleConnsPerHost: p.Transport.MaxIdleConnsPerHost},
		Seed:             seed,
		DB:               openDBUnchecked(cfg.Postgres.DSN),
		ServerPID:        serverPID,
	}
}

func portsForConfig(cfg loadtestconfig.File) ([]int, error) {
	return cleanupPortsForConfig(cfg, false)
}

func cleanupPortsForConfig(cfg loadtestconfig.File, externalInfra bool) ([]int, error) {
	ports, err := resource.PortsFromConfig(cfg)
	if err != nil {
		return nil, err
	}
	if !externalInfra {
		return ports, nil
	}
	out := ports[:0]
	for _, port := range ports {
		if port == managedPostgresPort || port == managedRedisPort {
			continue
		}
		out = append(out, port)
	}
	return out, nil
}

func waitCleanupPortsClosed(ctx context.Context, deps Dependencies, rc artifact.RunContext, ports []int, timeout time.Duration) artifact.PortsClosedArtifact {
	if timeout <= 0 {
		return deps.CheckPorts(rc, ports)
	}
	deadline, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	ticker := time.NewTicker(cleanupPortsPollInterval)
	defer ticker.Stop()
	latest := deps.CheckPorts(rc, ports)
	stableClosedChecks := 0
	for {
		if latest.Passed {
			stableClosedChecks++
			if stableClosedChecks >= 3 {
				return latest
			}
		} else {
			stableClosedChecks = 0
		}
		select {
		case <-deadline.Done():
			return latest
		case <-ticker.C:
			latest = deps.CheckPorts(rc, ports)
		}
	}
}

func failedPortsArtifact(rc artifact.RunContext, err error) artifact.PortsClosedArtifact {
	return artifact.PortsClosedArtifact{SchemaVersion: artifact.SchemaVersion, RunContext: rc, Ports: map[string]string{"config": "invalid: " + err.Error()}, Passed: false}
}

func baseURL(cfg loadtestconfig.File) string {
	return (&url.URL{Scheme: "http", Host: net.JoinHostPort(cfg.Server.Host, strconv.Itoa(cfg.Server.Port))}).String()
}

func runtimeURL(cfg loadtestconfig.File) string {
	return (&url.URL{Scheme: "http", Host: net.JoinHostPort(cfg.Server.Host, strconv.Itoa(cfg.Server.Port)), Path: "/debug/loadtest/runtime"}).String()
}

func openDBUnchecked(dsn string) *gorm.DB {
	db, _ := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	return db
}

func buildOrVerifyBinary(ctx context.Context, opts Options) error {
	if strings.TrimSpace(opts.Binary) == "" {
		return fmt.Errorf("binary is required")
	}
	if !filepath.IsAbs(opts.Binary) {
		return fmt.Errorf("binary must be an absolute path")
	}
	info, err := os.Stat(opts.Binary)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("binary must be a file")
	}
	return nil
}

func runConfigCheck(ctx context.Context, opts Options) error {
	_, err := opts.Config.NewAPIEnvForProfile("benchmark")
	return err
}

func preflightInfra(ctx context.Context, opts Options, cfg loadtestconfig.File) error {
	if err := localguard.RejectDefaultInfraPorts(cfg.Postgres.DSN, cfg.Redis.Addr); err != nil {
		return err
	}
	if err := validateManagedPorts(cfg); err != nil {
		return err
	}
	return nil
}

func validateManagedPorts(cfg loadtestconfig.File) error {
	ports, err := resource.PortsFromConfig(cfg)
	if err != nil {
		return err
	}
	seen := map[int]struct{}{}
	for _, port := range ports {
		seen[port] = struct{}{}
	}
	if _, ok := seen[managedPostgresPort]; !ok {
		return fmt.Errorf("PostgreSQL loadtest port 15432 is required")
	}
	if _, ok := seen[managedRedisPort]; !ok {
		return fmt.Errorf("Redis loadtest port 16379 is required")
	}
	return nil
}

func startInfra(ctx context.Context, opts Options, cfg loadtestconfig.File) (Process, error) {
	if strings.TrimSpace(opts.ArtifactDir) == "" {
		return nil, fmt.Errorf("artifact-dir is required for isolated infra")
	}
	if opts.ExternalIsolatedInfra {
		if err := verifyExternalIsolatedInfra(ctx, cfg); err != nil {
			return nil, err
		}
		return noopProcess{}, nil
	}
	return startManagedInfra(ctx, opts, cfg)
}

type noopProcess struct{}

func (noopProcess) PID() int                   { return 0 }
func (noopProcess) Stop(context.Context) error { return nil }

func verifyExternalIsolatedInfra(ctx context.Context, cfg loadtestconfig.File) error {
	if err := verifyPostgresMarker(ctx, cfg.Postgres.DSN); err != nil {
		return err
	}
	return verifyRedisMarker(ctx, cfg.Redis.Addr)
}

func verifyPostgresMarker(ctx context.Context, dsn string) error {
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return fmt.Errorf("postgres marker unavailable: %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("postgres marker unavailable: %w", err)
	}
	defer sqlDB.Close()
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	row := sqlDB.QueryRowContext(ctx, "SELECT current_database(), current_user")
	var database, user string
	if err := row.Scan(&database, &user); err != nil {
		return fmt.Errorf("postgres marker unavailable: %w", err)
	}
	if database != "new_api_loadtest" || user != "new_api_loadtest" {
		return fmt.Errorf("postgres marker mismatch: current database/user must be new_api_loadtest")
	}
	return nil
}

func verifyRedisMarker(ctx context.Context, addr string) error {
	opt, err := redis.ParseURL(redisURL(addr))
	if err != nil {
		return fmt.Errorf("redis marker unavailable: %w", err)
	}
	client := redis.NewClient(opt)
	defer client.Close()
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("redis marker unavailable: %w", err)
	}
	keys, err := scanRedisKeys(ctx, client)
	if err != nil {
		return fmt.Errorf("redis marker unavailable: %w", err)
	}
	for _, key := range keys {
		if !isLoadtestRedisKey(key) {
			return fmt.Errorf("redis marker mismatch: non-loadtest key exists")
		}
	}
	return nil
}

func isLoadtestRedisKey(key string) bool {
	if key == "loadtest:marker" || strings.HasPrefix(key, "loadtest:") {
		return true
	}
	if strings.HasPrefix(key, "new-api:subscription_plan:v1:") || strings.HasPrefix(key, "new-api:subscription_plan_info:v1:") {
		return true
	}
	if strings.HasPrefix(key, "token:") {
		return isHexDigest(strings.TrimPrefix(key, "token:"), 64)
	}
	if strings.HasPrefix(key, "user:") {
		_, err := strconv.Atoi(strings.TrimPrefix(key, "user:"))
		return err == nil
	}
	if strings.HasPrefix(key, "notify_limit:") {
		return true
	}
	if strings.HasPrefix(key, "rateLimit:") {
		return len(key) > len("rateLimit:")
	}
	if strings.HasPrefix(key, "subscription:concurrency:user:") {
		tail := strings.TrimPrefix(key, "subscription:concurrency:user:")
		tail = strings.TrimSuffix(tail, ":queue")
		_, err := strconv.Atoi(tail)
		return err == nil
	}
	if strings.HasPrefix(key, "perf:") {
		return validPerfRedisKey(key)
	}
	return false
}

func isHexDigest(value string, length int) bool {
	if len(value) != length {
		return false
	}
	for _, r := range value {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F') {
			continue
		}
		return false
	}
	return true
}

func validPerfRedisKey(key string) bool {
	parts := strings.Split(key, ":")
	if len(parts) != 4 || parts[1] == "" || parts[2] == "" {
		return false
	}
	_, err := strconv.ParseInt(parts[3], 10, 64)
	return err == nil
}

func scanRedisKeys(ctx context.Context, client *redis.Client) ([]string, error) {
	var cursor uint64
	keys := make([]string, 0)
	for {
		batch, next, err := client.Scan(ctx, cursor, "*", 100).Result()
		if err != nil {
			return nil, err
		}
		keys = append(keys, batch...)
		if next == 0 {
			sort.Strings(keys)
			return keys, nil
		}
		cursor = next
	}
}

func redisURL(addr string) string {
	if strings.Contains(addr, "://") {
		return addr
	}
	return "redis://" + addr + "/0"
}

func startMock(ctx context.Context, opts Options, rc artifact.RunContext) (Process, error) {
	return nil, fmt.Errorf("managed mock process startup is only available through loadtest-resource-sweep integration")
}

func startServer(ctx context.Context, opts Options, env map[string]string) (Process, error) {
	cmd, err := runner.BuildCommandWithExpectedLimits(runner.Config{Binary: opts.Binary, WorkDir: opts.WorkDir, Env: env}, runner.ExpectedLimits{RelayMaxIdleConns: "1024", RelayMaxIdleConnsPerHost: "1024", GOMEMLIMIT: "384MiB"})
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &osProcess{cmd: cmd}, nil
}

func stopProcess(ctx context.Context, proc Process) error {
	if proc == nil {
		return nil
	}
	return proc.Stop(ctx)
}

func bootstrapAndSeed(ctx context.Context, opts Options, rc artifact.RunContext) (artifact.SeedOutput, error) {
	db, err := gorm.Open(postgres.Open(opts.Config.Postgres.DSN), &gorm.Config{})
	if err != nil {
		return artifact.SeedOutput{}, err
	}
	common.UsingSQLite = false
	common.UsingMySQL = false
	common.UsingPostgreSQL = true
	common.LogSqlType = common.DatabaseTypePostgreSQL
	model.DB = db
	model.LOG_DB = db
	return seed.Apply(ctx, db, seed.Config{RunContext: rc.WithoutSeedOutputHash().WithoutMockHash(), Model: opts.Config.Loadtest.Model, Group: opts.Config.Loadtest.Group, MockBaseURL: opts.Config.MockUpstream.BaseURL, SubscriptionKey: opts.Config.Loadtest.SubscriptionKey, CompatKey: opts.Config.Loadtest.CompatKey})
}

func runPoint(ctx context.Context, opts PointOptions) (artifact.PointResult, artifact.PointAnalysis, artifact.ResourceSamplesArtifact, error) {
	return sweep.RunPoint(ctx, sweep.RunPointOptions{
		Concurrency:      opts.Concurrency,
		BaseURL:          opts.BaseURL,
		RuntimeURL:       opts.RuntimeURL,
		APIKey:           opts.APIKey,
		TokenProfile:     opts.TokenProfile,
		Path:             opts.Path,
		Model:            opts.Model,
		Scenario:         opts.Scenario,
		ArtifactDir:      opts.ArtifactDir,
		RunContext:       opts.RunContext,
		Config:           opts.Config,
		MockProfile:      opts.MockProfile,
		MockHash:         opts.MockHash,
		MockStats:        opts.MockStatsURL,
		RequestsPerPoint: opts.RequestsPerPoint,
		MaxRequests:      opts.MaxRequests,
		RampStep:         opts.RampStep,
		RampInterval:     opts.RampInterval,
		Duration:         opts.Duration,
		Timeout:          opts.Timeout,
		Transport:        opts.Transport,
		Seed:             opts.Seed,
		DB:               opts.DB,
		ServerPID:        opts.ServerPID,
	})
}

func renderReport(ctx context.Context, opts Options, sweepResult artifact.SweepResult, analyses []artifact.PointAnalysis, resources []artifact.ResourceSamplesArtifact, limits artifact.ResourceLimitsArtifact, ports artifact.PortsClosedArtifact) error {
	if opts.ArtifactDir == "" {
		return nil
	}
	md := report.RenderResourceSweep(report.ResourceSweepReportInput{Sweep: sweepResult, Analyses: analyses, ResourceSamples: resources, Limits: limits, Ports: ports})
	path := filepath.Join(opts.ArtifactDir, "reports", "resource-sweep.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(md), 0o600)
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

func sanitizeName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "run"
	}
	var b strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	return b.String()
}

var errNotImplemented = errors.New("not implemented")
var _ = mockopenai.Config{}
var _ = errNotImplemented
