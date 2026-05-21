# new-api 资源受限并发矩阵压测套件实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 将 workbench 的资源受限并发矩阵能力迁移为 `new-api` 原生 Go 压测套件，支持 server-only 资源限制、250/500/750/1000 并发矩阵、完整资源采样、失败分类、报告和清理门禁。

**架构：** 在现有 `pkg/loadtest/*` 基础上新增 `profile`、`resource`、`monitor`、`analysis`、`orchestrator` 包和 `cmd/loadtest-resource-sweep` 命令。`loadtest-resource-sweep` 编排配置检查、隔离 PostgreSQL/Redis preflight、mock upstream、受限 `new-api` server、bootstrap/seed、逐点 sweep、指标采样、hard/diagnostic gate、报告和 cleanup；低层业务 diff 复用现有 `client`、`metrics`、`sweep`、`runner`、`report`。生产代码只扩展 gated runtime route 与 HTTP ConnState 计数，不改变普通生产路径。

**技术栈：** Go 1.25、标准库 `net/http`/`os/exec`/`runtime`/`syscall`，`golang.org/x/sys/windows`，`gopsutil`，GORM PostgreSQL collector，go-redis INFO parser，项目 `common` JSON 封装。

---

## 文件结构

### 新增文件

- `pkg/loadtest/profile/profile.go`：定义 smoke/benchmark profile、矩阵参数、server limits、client transport limits；H2C 仅保留为后续扩展说明，不在第一阶段 CLI 中启用。
- `pkg/loadtest/profile/profile_test.go`：profile 默认值和安全校验。
- `pkg/loadtest/resource/limits.go`：跨平台 resource limits 类型、artifact builder、process sampler 接口。
- `pkg/loadtest/resource/limits_windows.go`：Windows Job Object memory limit 和 CPU affinity 实现。
- `pkg/loadtest/resource/limits_other.go`：非 Windows best-effort 实现。
- `pkg/loadtest/resource/process.go`：process RSS/CPU/thread/handle/socket snapshot。
- `pkg/loadtest/resource/ports.go`：loopback ports closed 检查。
- `pkg/loadtest/resource/resource_test.go`：limits artifact、ports closed、process sampler 测试。
- `pkg/loadtest/monitor/monitor.go`：周期采样 runtime/process/Postgres/Redis，输出 timeline 和 peaks。
- `pkg/loadtest/monitor/postgres.go`：PostgreSQL system metrics 与关键表 rows。
- `pkg/loadtest/monitor/redis.go`：Redis INFO 采集与 delta。
- `pkg/loadtest/monitor/drain.go`：业务 drain wait，逐表独立 timeout。
- `pkg/loadtest/monitor/monitor_test.go`：fixture parser、peaks、drain 判断测试。
- `pkg/loadtest/analysis/analysis.go`：failure_class、diagnostic gate 与资源/清理归一化；业务 hard gate 只消费 `sweep.EvaluateGate` 结果，不重复实现 billing/stream/mock 判断。
- `pkg/loadtest/analysis/analysis_test.go`：失败分类和 gate 测试。
- `pkg/loadtest/orchestrator/orchestrator.go`：端到端 run 编排。
- `pkg/loadtest/orchestrator/orchestrator_test.go`：用 fake dependencies 验证编排顺序、失败停止和 cleanup。
- `cmd/loadtest-resource-sweep/main.go`：资源受限矩阵 CLI，暴露 `Run(args, stdout, stderr) int`。
- `cmd/loadtest-resource-sweep/main_test.go`：CLI 参数、安全边界、cleanup 门禁测试。

### 修改文件

- `pkg/loadtest/artifact/artifact.go`：扩展 artifact schema，增加 transport profile、protocol counts、first error samples、resource limits、resource samples、ports closed、analysis。
- `pkg/loadtest/artifact/artifact_test.go`：扩展 round-trip、hash、redaction 测试。
- `pkg/loadtest/config/config.go`：增加 benchmark profile 配置和 server limits env `GOMEMLIMIT`；保留默认安全连接池。
- `pkg/loadtest/config/config_test.go`：覆盖 benchmark profile 读取与默认 smoke 安全限制。
- `pkg/loadtest/runner/runner.go`：允许/校验 `GOMEMLIMIT`，支持 benchmark profile 覆盖 relay idle pool。
- `pkg/loadtest/runner/runner_test.go`：覆盖 `GOMEMLIMIT` 和 profile-specific relay limits。
- `pkg/loadtest/client/client.go`：支持 transport modes、protocol counts、first error samples、细粒度 error reason。
- `pkg/loadtest/client/client_test.go`：覆盖 H1 keepalive/H1 no keepalive 配置、H2C 未实现时的清晰错误、错误分类、summary 扩展。
- `pkg/loadtest/localguard/localguard.go`：补齐 loadtest 安全矩阵校验 helper（固定 API key、生产 env 拒绝、默认端口拒绝、URL/DSN/Redis/listen/runtime/mock 校验）。
- `pkg/loadtest/localguard/localguard_test.go`：覆盖安全矩阵、生产 env、默认 5432/6379、真实 key/host 拒绝。
- `pkg/loadtest/metrics/metrics.go`：暴露 business snapshot/diff 与 monitor system snapshot 合并所需 helper；保持 billing 语义以 request id 为准。
- `pkg/loadtest/sweep/sweep.go`：提取 `RunPoint`/`RunPointOptions` 包级能力；hard gate 支持 benchmark scenario，要求 max_requests、resource samples、ports-cleanup 前置状态。
- `pkg/loadtest/sweep/sweep_test.go`：覆盖 benchmark hard gate。
- `pkg/loadtest/report/report.go`：报告展示最大通过点、失败点、failure_class、资源峰值、ports_closed。
- `pkg/loadtest/report/report_test.go`：覆盖资源矩阵报告。
- `cmd/loadtest-concurrency-sweep/main.go`：只做必要小改，允许复用 monitor snapshots；不复制 orchestrator 逻辑。
- `controller/loadtest_runtime.go`：新增 GC/runtime 字段与 HTTP ConnState/accept 计数读取。
- `controller/loadtest_runtime_test.go`：覆盖新增字段和 loopback 安全。
- `router/main.go`：向 runtime route 注册 HTTP server state provider。
- `main.go`：配置 `http.Server.ConnState` 和 listener wrapper 计数；继续保持 route gated。
- `config.loadtest.yaml`：增加 `profiles.benchmark`，默认 client 仍为安全 smoke；不加入可运行的 `profiles.h2c_diagnostic`；保持隔离端口 `15432/16379/13080/19080/8005`。
- `docs/superpowers/reports/2026-05-20-new-api-local-loadtest-sop.md`：补充 `loadtest-resource-sweep` 命令和清理规则。

---

## 并发执行策略与子代理提示词要求

### 主分支开发规则

- 不创建 worktree，不 stash，不 rebase，不 amend；所有实现直接在当前主分支小步提交。
- 每个实现子代理只修改分配给自己的文件白名单。发现非本任务文件已有改动时，视为其他工作者的改动：先停止并通过 IRC/主代理协调，不得覆盖。
- 所有 fresh 子代理提示词必须超过 2000 字，并包含完整路径：规格文件 `C:/Users/34404/source/repos/new-api/docs/superpowers/specs/2026-05-21-new-api-resource-loadtest-design.md`，计划文件 `C:/Users/34404/source/repos/new-api/docs/superpowers/plans/2026-05-21-new-api-resource-loadtest.md`，项目规则 `C:/Users/34404/source/repos/new-api/AGENTS.md`，全局规则 `C:/Users/34404/.omp/agent/AGENTS.md`。
- 每个实现子代理提示词必须写明：目标、文件白名单、非目标、禁止事项、验收标准、允许运行的精确测试命令、与其他并行任务的依赖关系、不得运行高容量 benchmark、不得运行项目级全量 build/test/lint、不得格式化非任务文件。
- 每个任务完成后必须经过至少两类审查：规格/计划符合性审查、代码质量/安全审查。重要集成点和最终实现必须并发派发 3 个以上只读 review 子代理；review 全部通过后才进入下一批。

### 批次与文件所有权

1. **批次 1（串行）**：任务 1。稳定 `profile` 与 `artifact` schema。下游任务只能消费任务 1 已提交的 schema，不得自行新增重复 schema。
2. **批次 2（可并发，任务 1 之后）**：任务 2、任务 3、任务 4、任务 5。文件边界分别为 config/runner、client、runtime route/main、resource；不得互改对方文件。
3. **批次 3（可并发，批次 2 之后）**：任务 6 与任务 7。任务 6 拥有 `pkg/loadtest/monitor/*`、`cmd/loadtest-collect/main.go`、`pkg/loadtest/metrics/metrics.go` 中 monitor/drain 所需 helper；任务 7 拥有 `pkg/loadtest/analysis/*`、`pkg/loadtest/sweep/sweep.go`、`pkg/loadtest/sweep/sweep_test.go`。两者不得并发修改同一文件；任务 6 不得修改 `pkg/loadtest/sweep/*`，任务 7 不得修改 `pkg/loadtest/metrics/metrics.go`。
4. **批次 4（串行收口）**：任务 8。任务 8 依赖任务 1-7 全部提交；任务 8 拥有 `pkg/loadtest/orchestrator/*`、`cmd/loadtest-resource-sweep/*`，并可在任务 7 之后补充修改 `pkg/loadtest/sweep/*` 和 `cmd/loadtest-concurrency-sweep/main.go` 做 `RunPoint` 提取。任务 8 不得与任务 7 并发；任何 `RunPointOptions` 合约变更都必须同步旧 cmd 与 orchestrator 调用点。
5. **批次 5（串行）**：任务 9。报告与 SOP 依赖任务 8 的 artifact 合约。
6. **批次 6（并发只读 review）**：至少三路 review：规格覆盖、安全资源边界、集成/并发冲突。所有 `must_fix` 修完并复审通过后进入最终验证。
7. **批次 7（串行）**：任务 10。只跑小矩阵 smoke，不跑 250/500/750/1000 高容量 benchmark。

批次边界验证：批次 2 后运行 profile/artifact/config/runner/client/resource/controller 的 targeted tests；批次 3 后运行 monitor/analysis/sweep targeted tests；任务 8 后运行 orchestrator/CLI command tests。所有批次验证均不得运行高容量 benchmark。

---

## 任务 1：Profile 与 artifact schema

**文件：**
- 创建：`pkg/loadtest/profile/profile.go`
- 创建：`pkg/loadtest/profile/profile_test.go`
- 修改：`pkg/loadtest/artifact/artifact.go`
- 修改：`pkg/loadtest/artifact/artifact_test.go`

### 目标

定义稳定的 benchmark profile 和 artifacts，后续任务禁止重复定义矩阵参数、server limits、transport profile、failure class 等 schema。

- [ ] **步骤 1：编写 profile 失败测试**

在 `pkg/loadtest/profile/profile_test.go` 写入：

```go
package profile

import (
    "strings"
    "testing"
    "time"
)

func TestBenchmarkProfileMatchesWorkbenchMatrix(t *testing.T) {
    p := Benchmark()
    wantPoints := []int{250, 500, 750, 1000}
    if len(p.Points) != len(wantPoints) { t.Fatalf("points len = %d", len(p.Points)) }
    for i := range wantPoints {
        if p.Points[i] != wantPoints[i] { t.Fatalf("point[%d] = %d want %d", i, p.Points[i], wantPoints[i]) }
    }
    if p.RequestsPerPoint != 3000 || p.RampStep != 25 || p.RampInterval != 200*time.Millisecond || p.Duration != 45*time.Second || p.Timeout != 120*time.Second {
        t.Fatalf("benchmark timings mismatch: %#v", p)
    }
    if p.Transport.Mode != TransportH1KeepAlive { t.Fatalf("transport mode = %q", p.Transport.Mode) }
    if p.ServerLimits.GOMAXPROCS != "2" || p.ServerLimits.GOGC != "100" || p.ServerLimits.GOMEMLIMIT != "384MiB" || p.ServerLimits.ProcessMemoryLimitBytes != 512*1024*1024 || p.ServerLimits.CPUAffinityCores != 2 {
        t.Fatalf("server limits mismatch: %#v", p.ServerLimits)
    }
}

func TestSmokeProfileKeepsLocalSafeConnectionLimits(t *testing.T) {
    p := Smoke()
    if p.Transport.MaxConnsPerHost > 16 || p.Relay.MaxIdleConns > 128 || p.Relay.MaxIdleConnsPerHost > 64 { t.Fatalf("smoke profile is unsafe for local loopback: %#v", p) }
    if p.RequestsPerPoint >= 3000 || len(p.Points) != 1 || p.Points[0] >= 250 { t.Fatalf("smoke profile must not be benchmark: %#v", p) }
}

func TestH2CDiagnosticIsNotImplementedProfile(t *testing.T) {
    _, err := ProfileByName("h2c_diagnostic")
    if err == nil || !strings.Contains(err.Error(), "not implemented") {
        t.Fatalf("h2c diagnostic should be explicitly unavailable in first stage: %v", err)
    }
}
```

- [ ] **步骤 2：运行 profile 测试确认失败**

运行：

```bash
go test ./pkg/loadtest/profile -run 'TestBenchmarkProfileMatchesWorkbenchMatrix|TestSmokeProfileKeepsLocalSafeConnectionLimits|TestH2CDiagnosticIsNotImplementedProfile' -count=1
```

预期：FAIL，包不存在。

- [ ] **步骤 3：实现 profile 包**

创建 `pkg/loadtest/profile/profile.go`，定义 `TransportH1KeepAlive` 和 `TransportH1NoKeepAlive` 常量，以及 `ServerLimits`、`Transport`、`Relay`、`Profile` 结构。第一阶段不提供可运行的 `h2c_diagnostic` profile；H2C 只保留在设计文档中作为后续诊断扩展，不进入本计划实现和 CLI 可选值。实现：

- `Benchmark()`：points `250,500,750,1000`，每点 `3000` 请求，`25/200ms` ramp，`45s` duration，`120s` timeout，H1 keepalive，server limits `2/100/384MiB/512MiB/2 cores`。
- `Smoke()`：`points=[2]`、每点 `10` 请求、transport `4` conns/host、relay `64/16`。
- `ProfileByName("h2c_diagnostic")` 返回明确错误 `h2c diagnostic profile is not implemented in this phase`。

- [ ] **步骤 4：扩展 artifact schema 测试**

在 `pkg/loadtest/artifact/artifact_test.go` 追加 `TestResourceSweepArtifactsRoundTrip`，构造 `ResourceLimitsArtifact`、扩展后的 `Summary`、`ResourceSamplesArtifact`、`PointAnalysis`、`PortsClosedArtifact`，逐个 `MarshalCanonical` 后断言包含 `run_context`，并断言关键 JSON 字段存在：`target_process`、`server_env.GOMEMLIMIT`、`protocol_counts.HTTP/1.1`、`first_error_samples[0].reason`、`samples`、`failure_class`、`hard_gate.failed_reasons`、`ports.13080`、`passed`。

- [ ] **步骤 5：运行 artifact 测试确认失败**

运行：

```bash
go test ./pkg/loadtest/artifact -run TestResourceSweepArtifactsRoundTrip -count=1
```

预期：FAIL，缺少新增类型/字段。

- [ ] **步骤 6：扩展 artifact 类型**

在 `pkg/loadtest/artifact/artifact.go` 中增加以下完整类型，字段名和 JSON tag 不得由实现子代理自由发挥：

```go
type TransportProfile struct {
    Mode string `json:"mode,omitempty"`
    MaxConnsPerHost int `json:"max_conns_per_host,omitempty"`
    MaxIdleConns int `json:"max_idle_conns,omitempty"`
    MaxIdleConnsPerHost int `json:"max_idle_conns_per_host,omitempty"`
}
type ErrorSample struct {
    RequestIndex int `json:"request_index"`
    Phase string `json:"phase"`
    Reason string `json:"reason"`
    StatusCode int `json:"status_code,omitempty"`
    LatencyMS float64 `json:"latency_ms,omitempty"`
    RequestID string `json:"request_id,omitempty"`
}
type ResourceLimitsArtifact struct {
    SchemaVersion int `json:"schema_version"`
    RunContext RunContext `json:"run_context"`
    TargetProcess string `json:"target_process"`
    OSProcessMemoryLimitEnforced bool `json:"os_process_memory_limit_enforced"`
    OSCPUAffinityEnforced bool `json:"os_cpu_affinity_enforced"`
    ServerCPUAffinityCores int `json:"server_cpu_affinity_cores"`
    ServerCPUAffinityMask uintptr `json:"server_cpu_affinity_mask"`
    ServerProcessMemoryLimitBytes uint64 `json:"server_process_memory_limit_bytes"`
    ServerEnv map[string]string `json:"server_env"`
    Scope string `json:"scope"`
    Statused
}
type ResourceSample struct {
    UnixMilli int64 `json:"unix_milli"`
    Process ProcessSnapshot `json:"process,omitempty"`
    Runtime RuntimeSnapshot `json:"runtime,omitempty"`
    Postgres PostgresSnapshot `json:"postgres,omitempty"`
    Redis RedisSnapshot `json:"redis,omitempty"`
}
type ResourcePeaks struct {
    RSSPeakBytes uint64 `json:"rss_peak_bytes,omitempty"`
    CPUPercentPeak float64 `json:"cpu_percent_peak,omitempty"`
    CPUTimeSecondsPeak float64 `json:"cpu_time_seconds_peak,omitempty"`
    ThreadCountPeak int `json:"thread_count_peak,omitempty"`
    HandleCountPeak int `json:"handle_count_peak,omitempty"`
    OpenTCPSocketsPeak int `json:"open_tcp_sockets_peak,omitempty"`
    GoroutinesPeak int `json:"goroutines_peak,omitempty"`
    HeapAllocPeakBytes uint64 `json:"heap_alloc_peak_bytes,omitempty"`
    HeapSysPeakBytes uint64 `json:"heap_sys_peak_bytes,omitempty"`
    GCCountPeak uint32 `json:"gc_count_peak,omitempty"`
    PauseTotalNSPeak uint64 `json:"pause_total_ns_peak,omitempty"`
    HTTPAcceptTotalPeak uint64 `json:"http_accept_total_peak,omitempty"`
    HTTPActiveCurrentPeak int64 `json:"http_active_current_peak,omitempty"`
    RedisConnectedClientsPeak int `json:"redis_connected_clients_peak,omitempty"`
    RedisUsedMemoryPeakBytes uint64 `json:"redis_used_memory_peak_bytes,omitempty"`
    RedisUsedMemoryRSSPeakBytes uint64 `json:"redis_used_memory_rss_peak_bytes,omitempty"`
    RedisInstantaneousOpsPeak int `json:"redis_instantaneous_ops_peak,omitempty"`
    RedisTotalCommandsProcessedPeak uint64 `json:"redis_total_commands_processed_peak,omitempty"`
    PostgresActiveConnectionsPeak int `json:"postgres_active_connections_peak,omitempty"`
    PostgresIdleConnectionsPeak int `json:"postgres_idle_connections_peak,omitempty"`
    PostgresWaitingLocksPeak int `json:"postgres_waiting_locks_peak,omitempty"`
    PostgresDatabaseSizePeakBytes uint64 `json:"postgres_database_size_peak_bytes,omitempty"`
}
type ResourceSamplesArtifact struct {
    SchemaVersion int `json:"schema_version"`
    RunContext RunContext `json:"run_context"`
    Concurrency int `json:"concurrency"`
    Samples []ResourceSample `json:"samples"`
    Peaks ResourcePeaks `json:"peaks"`
    Drain Statused `json:"drain,omitempty"`
}
type PortsClosedArtifact struct {
    SchemaVersion int `json:"schema_version"`
    RunContext RunContext `json:"run_context"`
    Ports map[string]string `json:"ports"`
    Passed bool `json:"passed"`
}
type GateResult struct {
    Passed bool `json:"passed"`
    FailedReasons []string `json:"failed_reasons,omitempty"`
    DiagnosticReasons []string `json:"diagnostic_reasons,omitempty"`
}
type PointAnalysis struct {
    SchemaVersion int `json:"schema_version"`
    RunContext RunContext `json:"run_context"`
    Concurrency int `json:"concurrency"`
    FailureClass string `json:"failure_class"`
    HardGate GateResult `json:"hard_gate"`
    DiagnosticGate GateResult `json:"diagnostic_gate,omitempty"`
}
```

并扩展现有类型：

- `Summary` 增加 `ProtocolCounts map[string]int`、`FirstErrorSamples []ErrorSample`、`Transport TransportProfile`。
- `SummaryExcerpt` 增加 `StopReason string `json:"stop_reason,omitempty"``，`summaryExcerpt(summary, delta)` 从 summary 中带出 `stop_reason`。
- `ProcessSnapshot` 增加 `ThreadCount`、`HandleCount`、`OpenTCPSockets`、`CPUTimeSeconds`。
- `PostgresSnapshot` 增加 `ActiveConnections`、`IdleConnections`、`WaitingLocks`、`DatabaseSizeBytes`。
- `RedisSnapshot` 增加 `ConnectedClients`、`UsedMemoryBytes`、`UsedMemoryRSSBytes`、`MemFragmentationRatio`、`InstantaneousOpsPerSec`、`TotalCommandsProcessed`、`Keyspace`。
- `RuntimeSnapshot` 增加 `GOMAXPROCS`、`GOMEMLimitBytes`、`GCCount`、`LastGCUnixMS`、`PauseTotalNS`、`HTTPConnState`、`HTTPAcceptTotal`、`HTTPActiveCurrent`。

所有 JSON tag 必须使用 snake_case；新增 artifact 必须包含 `schema_version` 和 `run_context`。

- [ ] **步骤 7：运行 profile 和 artifact 测试通过**

运行：

```bash
go test ./pkg/loadtest/profile ./pkg/loadtest/artifact -count=1
```

预期：PASS。

- [ ] **步骤 8：Commit**

```bash
git add pkg/loadtest/profile pkg/loadtest/artifact
git commit -m "feat(loadtest): 新增资源矩阵 profile 与 artifact schema"
```

---

## 任务 2：Config 与 runner 支持 benchmark profile

**文件：**
- 修改：`pkg/loadtest/config/config.go`
- 修改：`pkg/loadtest/config/config_test.go`
- 修改：`pkg/loadtest/localguard/localguard.go`
- 修改：`pkg/loadtest/localguard/localguard_test.go`
- 修改：`pkg/loadtest/runner/runner.go`
- 修改：`pkg/loadtest/runner/runner_test.go`
- 修改：`config.loadtest.yaml`

### 目标

配置文件支持 smoke/benchmark profile。默认仍安全；只有显式 benchmark profile 才允许 1024 relay/client 连接池。runner 必须允许并校验 `GOMEMLIMIT=384MiB`。H2C diagnostic 第一阶段不可运行。

- [ ] **步骤 1：编写 config profile 失败测试**

在 `pkg/loadtest/config/config_test.go` 追加 `TestBenchmarkProfileAllowsExplicitHighCapacityConnectionLimits`、`TestDefaultClientLimitsRemainSafeWithoutBenchmarkProfile`、`TestNewAPIEnvForProfileOnlyRaisesRelayPoolForBenchmark`、`TestH2CDiagnosticProfileRejectedInFirstStage`。前者构造 `Profiles["benchmark"]`，断言 relay/client `1024` 和 `GOMEMLIMIT=384MiB` 可通过；第二个将顶层 `Client.MaxIdleConns=129`，断言 `Validate()` 失败；第三个断言 `NewAPIEnv()` 仍输出 `RELAY_MAX_IDLE_CONNS=64`/`RELAY_MAX_IDLE_CONNS_PER_HOST=16`，而 `NewAPIEnvForProfile("benchmark")` 精确输出 `1024/1024` 与 `GOMEMLIMIT=384MiB`，`NewAPIEnvForProfile("benchmark")` 对超过 profile 声明值的 env 不放行；第四个断言配置或请求 `h2c_diagnostic` profile 返回 `h2c diagnostic profile is not implemented in this phase`。添加 `mustDuration(value string) Duration` helper 调用 `ParseDuration`。

- [ ] **步骤 2：运行 config profile 测试确认失败**

运行：

```bash
go test ./pkg/loadtest/config ./pkg/loadtest/localguard -run 'TestBenchmarkProfileAllowsExplicitHighCapacityConnectionLimits|TestDefaultClientLimitsRemainSafeWithoutBenchmarkProfile|TestNewAPIEnvForProfileOnlyRaisesRelayPoolForBenchmark|TestH2CDiagnosticProfileRejectedInFirstStage|TestValidateLoadtestSafetyMatrixRejectsProductionInputs|TestValidateCleanEnvRejectsProductionEnv|TestConfigRejectsUnsafeValues' -count=1
```

预期：FAIL，缺少 `Profiles`、`ProfileConfig` 等类型。

- [ ] **步骤 2.5：编写安全矩阵失败测试**

在 `pkg/loadtest/localguard/localguard_test.go` 增加：

```go
func TestValidateLoadtestSafetyMatrixRejectsProductionInputs(t *testing.T) {
    cases := []struct{ name string; value string }{
        {"client url", "https://api.openai.com"},
        {"mock url", "http://192.0.2.10:19080"},
        {"runtime url", "http://10.0.0.2:13080/debug/loadtest/runtime"},
        {"postgres default", "postgresql://new_api_loadtest:loadtest@127.0.0.1:5432/new_api_loadtest?sslmode=disable"},
        {"redis default", "redis://127.0.0.1:6379/0"},
        {"listen wildcard", "0.0.0.0:13080"},
        {"real key", "sk-realproductionkey"},
    }
    for _, tc := range cases {
        if err := ValidateAny(tc.value); err == nil { t.Fatalf("%s accepted", tc.name) }
    }
}
func TestValidateCleanEnvRejectsProductionEnv(t *testing.T) {
    env := map[string]string{"OPENAI_API_KEY":"sk-prod", "SQL_DSN":"postgresql://new_api:secret@example.com:5432/new_api"}
    if err := ValidateCleanEnv(env); err == nil { t.Fatal("production env accepted") }
}
```

在 `pkg/loadtest/config/config_test.go` 扩展 `TestConfigRejectsUnsafeValues`，逐项覆盖 `server.host` / `server.pprof_addr` / `postgres.dsn` / `log_postgres.dsn` / `redis.addr` / `mock_upstream.base_url` / 三个 `loadtest.*_key`，其中 PostgreSQL 5432、Redis 6379、非 loopback runtime/mock URL、真实 API key 均必须失败，错误消息不得包含完整 DSN 或 key。

- [ ] **步骤 3：实现 config profile 类型**

在 `pkg/loadtest/config/config.go` 增加 `Profiles map[string]ProfileConfig`、`Duration`、`TransportConfig`、`RelayConfig`、`ServerLimitsConfig`、`ProfileConfig`，并实现 `ParseDuration`、`(*Duration).UnmarshalYAML`、`(File).Profile(name) (profile.Profile, error)`、`(File).NewAPIEnvForProfile(name string) (map[string]string, error)`。结构字段固定如下，JSON/YAML tag 均为对应 snake_case：

```go
type Duration struct { Duration time.Duration }
type TransportConfig struct {
    Mode string `json:"mode" yaml:"mode"`
    MaxConnsPerHost int `json:"max_conns_per_host" yaml:"max_conns_per_host"`
    MaxIdleConns int `json:"max_idle_conns" yaml:"max_idle_conns"`
    MaxIdleConnsPerHost int `json:"max_idle_conns_per_host" yaml:"max_idle_conns_per_host"`
}
type RelayConfig struct {
    MaxIdleConns int `json:"max_idle_conns" yaml:"max_idle_conns"`
    MaxIdleConnsPerHost int `json:"max_idle_conns_per_host" yaml:"max_idle_conns_per_host"`
}
type ServerLimitsConfig struct {
    GOMAXPROCS string `json:"gomaxprocs" yaml:"gomaxprocs"`
    GOGC string `json:"gogc" yaml:"gogc"`
    GOMEMLIMIT string `json:"gomemlimit" yaml:"gomemlimit"`
    ProcessMemoryLimitBytes uint64 `json:"process_memory_limit_bytes" yaml:"process_memory_limit_bytes"`
    CPUAffinityCores int `json:"cpu_affinity_cores" yaml:"cpu_affinity_cores"`
}
type ProfileConfig struct {
    Points []int `json:"points" yaml:"points"`
    RequestsPerPoint int `json:"requests_per_point" yaml:"requests_per_point"`
    RampStep int `json:"ramp_step" yaml:"ramp_step"`
    RampInterval Duration `json:"ramp_interval" yaml:"ramp_interval"`
    Duration Duration `json:"duration" yaml:"duration"`
    Timeout Duration `json:"timeout" yaml:"timeout"`
    Transport TransportConfig `json:"transport" yaml:"transport"`
    Relay RelayConfig `json:"relay" yaml:"relay"`
    ServerLimits ServerLimitsConfig `json:"server_limits" yaml:"server_limits"`
}
```

`validateProfile` 要求：points 正数递增、requests/ramp/duration/timeout 正数、transport mode 第一阶段只接受 `h1_keepalive` 或 `h1_no_keepalive`；出现 `h2c_diagnostic` 时返回明确未实现错误。只有 profile 内允许 relay/client 超过顶层默认安全上限。`File.Profile("benchmark")` 返回任务 1 的 canonical `profile.Profile`；映射规则：`ProfileConfig.Transport` -> `profile.Profile.Transport` 和后续 `artifact.TransportProfile`，`ProfileConfig.Relay` -> `profile.Profile.Relay` 和 `runner.ExpectedLimits`，`ProfileConfig.ServerLimits` -> `profile.Profile.ServerLimits` 和 `resource.ApplyLimits`。`NewAPIEnvForProfile` 必须从普通 `NewAPIEnv()` 起步，只覆盖 profile 声明的 `GOMAXPROCS`、`GOGC`、`GOMEMLIMIT`、`RELAY_MAX_IDLE_CONNS`、`RELAY_MAX_IDLE_CONNS_PER_HOST`，不得改变 DSN、Redis、task-disable、retry-disable 等安全 env。

同时在 `pkg/loadtest/localguard/localguard.go` 增加 `ValidateCleanEnv(env map[string]string) error`、`ValidateLoadtestAPIKey(key string) error`、`RejectDefaultInfraPorts(postgresDSN, redisAddr string) error`。`ValidateURL`/`ValidatePostgresDSN`/`ValidateRedisAddr`/`ValidateListenAddr` 必须拒绝非 loopback；`ValidatePostgresDSN` 必须拒绝 `:5432`，只允许 database/user 含 `loadtest`；`ValidateRedisAddr` 必须拒绝 `:6379`；`ValidateLoadtestAPIKey` 只允许 `sk-loadtestsub`、`sk-loadtestcompat`、`sk-loadtestinvalid`；`ValidateCleanEnv` 必须拒绝 inherited provider keys（如 `OPENAI_API_KEY`、`ANTHROPIC_API_KEY`、`AZURE_OPENAI_API_KEY`）、非 loadtest `SQL_DSN`/`REDIS_CONN_STRING`、真实 upstream base URL、工作目录中的 `.env`。所有返回错误都不得包含完整 key、DSN 密码或真实 host，调用方输出前仍走 `artifact.Redact`。

- [ ] **步骤 4：runner 增加 GOMEMLIMIT 测试**

在 `pkg/loadtest/runner/runner_test.go` 的 `safeEnv()` 增加 `"GOMEMLIMIT": "384MiB"`。在 `TestBuildCommandUsesCleanAllowlistEnvironment` required 列表增加 `"GOMEMLIMIT=384MiB"`。新增 `TestBuildCommandRejectsUnexpectedGOMEMLIMIT`：设置 `env["GOMEMLIMIT"]="8GiB"` 且不声明 benchmark expected limits 时调用 `BuildCommand`，断言返回错误。新增 `TestBuildCommandAllowsBenchmarkRelayLimitsOnlyWhenExpected`：构造 env 中 `RELAY_MAX_IDLE_CONNS=1024`/`RELAY_MAX_IDLE_CONNS_PER_HOST=1024`，调用新的 profile-aware runner API 时通过；同样 env 调普通 `BuildCommand` 必须失败；env 超过 expected 值必须失败。

- [ ] **步骤 5：运行 runner 测试确认失败**

运行：

```bash
go test ./pkg/loadtest/runner -run 'TestBuildCommandUsesCleanAllowlistEnvironment|TestBuildCommandRejectsUnexpectedGOMEMLIMIT|TestBuildCommandAllowsBenchmarkRelayLimitsOnlyWhenExpected' -count=1
```

预期：FAIL，`GOMEMLIMIT` 未允许/校验。

- [ ] **步骤 6：实现 runner profile-aware env 校验**

在 `pkg/loadtest/runner/runner.go` 的 `allowedEnvKeys` 增加 `GOMEMLIMIT`。新增：

```go
type ExpectedLimits struct { RelayMaxIdleConns string; RelayMaxIdleConnsPerHost string; GOMEMLIMIT string }
func BuildCommandWithExpectedLimits(cfg Config, expected ExpectedLimits) (*exec.Cmd, error)
```

`BuildCommand` 保持默认安全行为，内部调用 `BuildCommandWithExpectedLimits` 且 expected 为 `64/16/384MiB`。`loadtest-run-new-api`、`loadtest-concurrency-sweep` 等现有命令继续使用 `BuildCommand`，因此仍拒绝 benchmark 高连接池。只有 `loadtest-resource-sweep --profile benchmark` 可调用 `BuildCommandWithExpectedLimits`，expected 必须来自 `File.Profile("benchmark")`，只接受 exact profile 值；不得接受任意更大值。

- [ ] **步骤 7：更新 `config.loadtest.yaml`**

新增 `profiles.benchmark`。benchmark 使用 points `[250,500,750,1000]`、`requests_per_point: 3000`、`ramp_step: 25`、`ramp_interval: 200ms`、`duration: 45s`、`timeout: 120s`、relay/client `1024/1024`、server limits `2/100/384MiB/536870912/2`。不要在第一阶段 `config.loadtest.yaml` 中加入可运行的 `profiles.h2c_diagnostic`；H2C 作为后续诊断扩展，不得被 CLI 接受。不要修改顶层 `client.max_idle_conns: 64` / `max_idle_conns_per_host: 16`。

- [ ] **步骤 8：运行 config/runner 测试通过**

运行：

```bash
go test ./pkg/loadtest/config ./pkg/loadtest/runner ./pkg/loadtest/localguard -count=1
```

预期：PASS。

- [ ] **步骤 9：Commit**

```bash
git add pkg/loadtest/config pkg/loadtest/localguard pkg/loadtest/runner config.loadtest.yaml
git commit -m "feat(loadtest): 支持 benchmark profile 配置"
```

---

## 任务 3：Client transport、协议统计与错误分类

**文件：**
- 修改：`pkg/loadtest/client/client.go`
- 修改：`pkg/loadtest/client/client_test.go`
- 修改：`cmd/loadtest-client/main.go`

### 目标

client 支持 H1 keepalive、H1 no keepalive；H2C diagnostic 只在 client flag 层返回清晰未实现错误，不进入 resource sweep profile。summary 记录协议计数和前 N 个错误样本；错误原因比现有 `http_client_do_error` 更细。

- [ ] **步骤 1：编写 transport 和 summary 失败测试**

在 `pkg/loadtest/client/client_test.go` 追加两个测试：

```go
func TestRunLoadRecordsProtocolCountsAndTransportProfile(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "text/event-stream")
        _, _ = io.WriteString(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"x\"}\n\n")
        _, _ = io.WriteString(w, "event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":1,\"output_tokens\":1,\"total_tokens\":2}}}\n\n")
        _, _ = io.WriteString(w, "data: [DONE]\n\n")
    }))
    defer server.Close()

    summary, err := RunLoad(context.Background(), Options{
        BaseURL: server.URL, APIKey: "sk-loadtestsub", TokenProfile: "subscription",
        Path: "/v1/responses", Model: "gpt-5.5", Scenario: "test",
        Concurrency: 2, MaxRequests: 2, Timeout: 5 * time.Second, Stream: true,
        Transport: TransportOptions{Mode: "h1_keepalive", MaxConnsPerHost: 2, MaxIdleConns: 2, MaxIdleConnsPerHost: 2},
    })
    if err != nil { t.Fatal(err) }
    if summary.ProtocolCounts["HTTP/1.1"] != 2 { t.Fatalf("protocol counts = %#v", summary.ProtocolCounts) }
    if summary.Transport.Mode != "h1_keepalive" || summary.Transport.MaxConnsPerHost != 2 { t.Fatalf("transport = %#v", summary.Transport) }
}

func TestRunLoadClassifiesConnectionRefused(t *testing.T) {
    summary, err := RunLoad(context.Background(), Options{
        BaseURL: "http://127.0.0.1:1", APIKey: "sk-loadtestsub", TokenProfile: "subscription",
        Path: "/v1/responses", Model: "gpt-5.5", Scenario: "test",
        Concurrency: 1, MaxRequests: 1, Timeout: 200 * time.Millisecond, Stream: true,
        Transport: TransportOptions{Mode: "h1_keepalive", MaxConnsPerHost: 1},
    })
    if err == nil { t.Fatal("expected runtime error") }
    if summary.ErrorReasons["connect_refused"] != 1 { t.Fatalf("error reasons = %#v", summary.ErrorReasons) }
    if len(summary.FirstErrorSamples) != 1 || summary.FirstErrorSamples[0].Reason != "connect_refused" { t.Fatalf("samples = %#v", summary.FirstErrorSamples) }
}
```

- [ ] **步骤 2：运行 client 新测试确认失败**

运行：

```bash
go test ./pkg/loadtest/client -run 'TestRunLoadRecordsProtocolCountsAndTransportProfile|TestRunLoadClassifiesConnectionRefused' -count=1
```

预期：FAIL，缺少 `TransportOptions`、summary 字段未填或错误分类不匹配。

- [ ] **步骤 3：实现 TransportOptions 和 bounded transport**

在 `pkg/loadtest/client/client.go` 增加 `TransportOptions`，并在 `Options` 增加 `Transport TransportOptions`。用 `newTransport(opts TransportOptions) *http.Transport` 替换 `newBoundedTransport()`：

- 默认 `Mode=h1_keepalive`。
- 默认 `MaxConnsPerHost=4`、`MaxIdleConns=MaxConnsPerHost`、`MaxIdleConnsPerHost=MaxConnsPerHost`、`IdleConnTimeout=5s`。
- `h1_no_keepalive` 设置 `DisableKeepAlives=true`。
第一阶段明确不实现 `h2c_diagnostic` 的真实 HTTP/2 cleartext dial：当 `Mode == "h2c_diagnostic"` 时，`normalizeAndValidateOptions` 必须返回 config error：`h2c diagnostic transport is not implemented in this phase`。`cmd/loadtest-client` 可保留该 flag 值用于给出清晰错误，但 `loadtest-resource-sweep` 第一阶段不得接受 `--profile h2c_diagnostic`。不要引入 `golang.org/x/net/http2`，避免把诊断模式混入第一阶段 hard gate。

- [ ] **步骤 4：实现协议统计和错误样本**

在 `requestResult` 增加 `protocol string` 和 `phase string`。`doOne` 收到 response 后设置 `protocol = resp.Proto`。实现 `classifyHTTPError(err error, ctx context.Context) string`：

- timeout：`connect_timeout` 或 `request_timeout`。
- Windows/Linux connection refused：`connect_refused`。
- connection reset / wsarecv：`connection_reset`。
- 默认：`http_client_do_error`。

`buildSummary` 统计 `ProtocolCounts`，最多收集 10 个 `FirstErrorSamples`。`FirstErrorSamples` 中的 `RequestID` 只能来自 `X-Oneapi-Request-Id` / artifact `new_api_request_id`，不得使用本地 client request id 伪装服务端 request id。HTTP 2xx 但 SSE parser 失败（例如 `missing_done`）必须保留 `status_code=200`、`phase="stream_parse"`，用于 metrics 区分客户端解析失败与服务端退款失败。

- [ ] **步骤 5：cmd 增加 transport flags**

在 `cmd/loadtest-client/main.go` 增加：

```go
transportMode := fs.String("transport", "h1_keepalive", "h1_keepalive, h1_no_keepalive, h2c_diagnostic")
maxConnsPerHost := fs.Int("max-conns-per-host", 0, "client max conns per host")
maxIdleConns := fs.Int("max-idle-conns", 0, "client max idle conns")
maxIdleConnsPerHost := fs.Int("max-idle-conns-per-host", 0, "client max idle conns per host")
```

赋值：

```go
opts.Transport = loadclient.TransportOptions{Mode: *transportMode, MaxConnsPerHost: *maxConnsPerHost, MaxIdleConns: *maxIdleConns, MaxIdleConnsPerHost: *maxIdleConnsPerHost}
```

- [ ] **步骤 6：运行 client 测试通过**

运行：

```bash
go test ./pkg/loadtest/client ./cmd/loadtest-client -count=1
```

预期：PASS。

- [ ] **步骤 7：Commit**

```bash
git add pkg/loadtest/client cmd/loadtest-client
git commit -m "feat(loadtest): 扩展客户端传输配置与错误分类"
```

---

## 任务 4：Runtime route 增加 GC 与 HTTP ConnState 指标

**文件：**
- 修改：`controller/loadtest_runtime.go`
- 修改：`controller/loadtest_runtime_test.go`
- 修改：`router/main.go`
- 修改：`main.go`

### 目标

runtime route 暴露 `GOMAXPROCS`、`GOMEMLIMIT`、GC 统计和 HTTP accept/ConnState 计数。普通生产未启用 loadtest route 时不暴露 endpoint。

- [ ] **步骤 1：用 LSP 查引用**

运行 LSP references：

- `router/main.go` 第 16 行 `SetRouter`
- `controller/loadtest_runtime.go` 第 31 行 `RegisterLoadtestRuntimeRoute`

确认所有调用点，避免漏改签名。

- [ ] **步骤 2：编写 runtime route 失败测试**

在 `controller/loadtest_runtime_test.go` 增加：

```go
func TestLoadtestRuntimeRouteIncludesGCAndHTTPStats(t *testing.T) {
    t.Setenv("LOADTEST_RUNTIME_STATS_ENABLED", "true")
    t.Setenv("LOADTEST_PROFILE_BLOCK_RATE", "1000")
    t.Setenv("LOADTEST_PROFILE_MUTEX_FRACTION", "5")
    gin.SetMode(gin.TestMode)
    r := gin.New()
    stats := NewLoadtestHTTPStats()
    stats.OnAccept()
    stats.OnConnState(http.StateNew)
    stats.OnConnState(http.StateActive)
    stats.OnConnState(http.StateIdle)
    RegisterLoadtestRuntimeRoute(r, "127.0.0.1:13080", stats)
    req := httptest.NewRequest(http.MethodGet, "/debug/loadtest/runtime", nil)
    req.RemoteAddr = "127.0.0.1:12345"
    w := httptest.NewRecorder()
    r.ServeHTTP(w, req)
    if w.Code != http.StatusOK { t.Fatalf("status = %d body=%s", w.Code, w.Body.String()) }
    var body map[string]any
    if err := common.Unmarshal(w.Body.Bytes(), &body); err != nil { t.Fatal(err) }
    for _, key := range []string{"gomaxprocs", "gomemlimit_bytes", "gc_count", "pause_total_ns", "http_conn_state", "http_accept_total", "http_active_current"} {
        if _, ok := body[key]; !ok { t.Fatalf("missing %s in %s", key, w.Body.String()) }
    }
}
```

- [ ] **步骤 3：运行 runtime 测试确认失败**

运行：

```bash
go test ./controller -run TestLoadtestRuntimeRouteIncludesGCAndHTTPStats -count=1
```

预期：FAIL，缺少 `NewLoadtestHTTPStats` 或 route 签名不匹配。

- [ ] **步骤 4：实现 HTTP stats 类型**

在 `controller/loadtest_runtime.go` 增加 `LoadtestHTTPStats`，包含 accept total、conn state counters、active current。实现 `NewLoadtestHTTPStats`、`OnAccept`、`OnConnState`、`Snapshot`。`OnConnState` 对 `StateActive` 增加 active current，对 `StateIdle`/`StateClosed` 减少 active current 时要防止负数。

- [ ] **步骤 5：扩展 route response**

`loadtestRuntimeResponse` 增加 `gomaxprocs`、`gomemlimit_bytes`、`gc_count`、`last_gc_unix_ms`、`pause_total_ns`、`http_conn_state`、`http_accept_total`、`http_active_current`。从 `runtime.MemStats`、`runtime.GOMAXPROCS(0)`、`debug.SetMemoryLimit(-1)` 读取。

- [ ] **步骤 6：修改 router/main 签名**

把 `SetRouter` 改为接收 `loadtestStats *controller.LoadtestHTTPStats`，把 `RegisterLoadtestRuntimeRoute` 改为接收 stats。在 `main.go` 创建 `loadtestHTTPStats := controller.NewLoadtestHTTPStats()`，把 `server.Run(listenAddr)` 替换为显式 `http.Server{Addr, Handler, ConnState}` 和 listener wrapper，以便调用 `OnAccept()`。listener wrapper 放在 controller 中，命名为 `NewLoadtestCountingListener`。route 注册条件仍必须同时满足 `LOADTEST_RUNTIME_STATS_ENABLED=true` 且主服务监听地址是 loopback；`HOST` 为空、`0.0.0.0`、`::`、非 loopback 或代理转发非 loopback client 都不得暴露该 endpoint。

- [ ] **步骤 7：运行 controller 测试通过**

运行：

```bash
go test ./controller -run TestLoadtestRuntime -count=1
```

预期：PASS。

- [ ] **步骤 8：Commit**

```bash
git add controller/loadtest_runtime.go controller/loadtest_runtime_test.go router/main.go main.go
git commit -m "feat(loadtest): 扩展运行时资源指标"
```

---

## 任务 5：Resource limits、process sampler 与 ports closed

**文件：**
- 创建：`pkg/loadtest/resource/limits.go`
- 创建：`pkg/loadtest/resource/limits_windows.go`
- 创建：`pkg/loadtest/resource/limits_other.go`
- 创建：`pkg/loadtest/resource/process.go`
- 创建：`pkg/loadtest/resource/ports.go`
- 创建：`pkg/loadtest/resource/resource_test.go`

### 目标

实现 server-only 资源限制 artifact、Windows Job Object/CPU affinity、非 Windows best-effort、process snapshot 和 ports-closed artifact。

- [ ] **步骤 1：编写 resource 失败测试**

创建 `pkg/loadtest/resource/resource_test.go`，覆盖：

```go
func TestBuildLimitsArtifactRecordsServerOnlyScope(t *testing.T) { /* 使用 profile.Benchmark().ServerLimits，断言 target_process=server、memory/cpu enforced、scope 包含 load generator/mock/PostgreSQL/Redis remain uncapped */ }
func TestPortsClosedDetectsOpenAndClosedLoopbackPorts(t *testing.T) { /* net.Listen 127.0.0.1:0，CheckPortsClosed 返回 open 且 Passed=false */ }
func TestPortsFromConfigIncludesAllLoadtestPorts(t *testing.T) { /* 用默认 config，断言端口集合精确包含 15432、16379、13080、19080、8005 */ }
```

测试必须 import `net`、`strconv`、`strings`、`testing`、`artifact`、`profile`、`loadtestconfig`。

- [ ] **步骤 2：运行 resource 测试确认失败**

运行：

```bash
go test ./pkg/loadtest/resource -run 'TestBuildLimitsArtifactRecordsServerOnlyScope|TestPortsClosedDetectsOpenAndClosedLoopbackPorts|TestPortsFromConfigIncludesAllLoadtestPorts' -count=1
```

预期：FAIL，包不存在。

- [ ] **步骤 3：实现 limits.go**

实现 `ApplyResult`、`BuildLimitsArtifact(rc, limits, result)`、`ApplyServerLimits(pid, limits)`、`ServerEnv(limits)`。Artifact 必须写明 scope：`new-api server process only; load generator, mock upstream, PostgreSQL, Redis, and orchestrator remain uncapped except normal OS scheduling`。

- [ ] **步骤 4：实现 Windows limits**

`limits_windows.go` 使用 `golang.org/x/sys/windows` 实现 `OpenProcess`、`SetProcessAffinityMask`、`CreateJobObject`、`SetInformationJobObject(JobObjectExtendedLimitInformation)`、`AssignProcessToJobObject`。若当前 `x/sys/windows` 类型名称不匹配，按包实际定义调整；不要在业务文件散落 magic syscall。

- [ ] **步骤 5：实现 non-Windows limits**

`limits_other.go` 返回 `Status="best_effort"`，`MemoryLimitEnforced=false`，reason 明确说明第一阶段只有 Windows Job Object 强制 memory limit。

- [ ] **步骤 6：实现 process sampler**

`process.go` 使用 `gopsutil/process` 实现 `SampleProcess(pid int) artifact.ProcessSnapshot`，填充 RSS、CPUPercent、NumThreads、NumHandles、CPUTimeSeconds，并尽力统计该 pid 的 open TCP sockets 数量写入 `OpenTCPSockets`。不可用字段保持 0，`Statused.Status` 为 `ok` 或 `unavailable`。

- [ ] **步骤 7：实现 ports closed**

`ports.go` 实现 `CheckPortsClosed(rc artifact.RunContext, ports []int) artifact.PortsClosedArtifact`，对每个端口 `net.DialTimeout("tcp", "127.0.0.1:<port>", 300*time.Millisecond)`，open 则 `Passed=false`。同时实现 `PortsFromConfig(file loadtestconfig.File) ([]int, error)`，从 `postgres.dsn`、`redis.addr`、`server.host:server.port`、`server.pprof_addr`、`mock_upstream.base_url` 提取端口；默认配置必须返回 `[15432,16379,13080,19080,8005]`。端口缺失、0 或无法解析时返回错误。

- [ ] **步骤 8：运行 resource 测试通过**

运行：

```bash
go test ./pkg/loadtest/resource -count=1
```

预期：PASS。

- [ ] **步骤 9：Commit**

```bash
git add pkg/loadtest/resource
git commit -m "feat(loadtest): 添加资源限制与端口清理检查"
```

---

## 任务 6：Monitor 采集 process/runtime/Postgres/Redis/drain

**文件：**
- 创建：`pkg/loadtest/monitor/monitor.go`
- 创建：`pkg/loadtest/monitor/postgres.go`
- 创建：`pkg/loadtest/monitor/redis.go`
- 创建：`pkg/loadtest/monitor/drain.go`
- 创建：`pkg/loadtest/monitor/monitor_test.go`
- 修改：`cmd/loadtest-collect/main.go`
- 修改：`pkg/loadtest/metrics/metrics.go`

### 目标

资源采样从 minimal collector 升级为可复用 monitor。每点输出 resource samples/peaks，collector 命令也能采集 system snapshot。`pkg/loadtest/metrics/metrics.go` 只新增 monitor/drain 所需的业务 snapshot helper：`func BusinessDrainSample(db *gorm.DB, tokenProfile string) (monitor.DrainSample, error)`，返回 consume logs、subscription pre-consume records、subscription token used；不得在 metrics 中实现资源采样或 gate。

- [ ] **步骤 1：编写 parser/peaks/drain 失败测试**

创建 `pkg/loadtest/monitor/monitor_test.go`，包含：

- `TestParseRedisInfoExtractsMemoryClientsAndCommands`：输入 Redis INFO fixture，断言 `ConnectedClients`、`UsedMemoryBytes`、`TotalCommandsProcessed`、`Keyspace`。
- `TestResourcePeaksUsesMaxAcrossSamples`：两条 sample，断言 goroutines、heap、RSS、process CPU、thread、open TCP sockets、Redis used_memory/commands、PostgreSQL active/idle/locks/db size、runtime GC/HTTP active 均取最大值。
- `TestDrainStatusRequiresEachTableStable`：consume logs、pre-consume records、subscription token used 全部达到预期才 passed，任一缺口 failed 且 reason 指出表名。
- `TestSamplerCollectsAtLeastTwoSamples`：用 10ms interval 和 fake sampler，35ms 后至少两条 sample。
- `TestReadRuntimeSnapshotRequiresLoopbackURL`：非 loopback runtime URL 返回 config error；loopback httptest 返回 runtime JSON 时能填充 `RuntimeSnapshot`。
- `TestCollectSnapshotReadsPIDFileAndRuntimeURL`：`cmd/loadtest-collect` 使用 `--pid-file` 与 `--runtime-url` 后 snapshot 中 process/runtime 不再是 minimal unavailable。
- `TestCollectSnapshotReadsRedisInfo`：fake Redis INFO 或 parser fixture 接入后 snapshot 中 redis memory/commands 字段非零。

- [ ] **步骤 2：运行 monitor 测试确认失败**

运行：

```bash
go test ./pkg/loadtest/monitor ./cmd/loadtest-collect -run 'TestParseRedisInfoExtractsMemoryClientsAndCommands|TestResourcePeaksUsesMaxAcrossSamples|TestDrainStatusRequiresEachTableStable|TestSamplerCollectsAtLeastTwoSamples|TestReadRuntimeSnapshotRequiresLoopbackURL|TestCollectSnapshotReadsPIDFileAndRuntimeURL|TestCollectSnapshotReadsRedisInfo' -count=1
```

预期：FAIL，包不存在。

- [ ] **步骤 3：实现 Redis INFO parser 与读取器**

`redis.go` 实现：

```go
func ParseRedisInfo(info string) artifact.RedisSnapshot
func LoadRedisSnapshot(ctx context.Context, addr string) artifact.RedisSnapshot
```

`LoadRedisSnapshot` 使用 go-redis `ParseURL` 或 `NewClient` 创建短生命周期 client，设置超时，执行 `INFO`，defer `Close()`；addr 必须先经 localguard 校验。`ParseRedisInfo` 解析 `connected_clients`、`used_memory`、`used_memory_rss`、`mem_fragmentation_ratio`、`instantaneous_ops_per_sec`、`total_commands_processed` 和 `db*` keyspace。无效数字跳过，不 panic。

- [ ] **步骤 4：实现 Postgres snapshot**

`postgres.go` 实现：

```go
func LoadPostgresSnapshot(db *gorm.DB, tableNames []string) artifact.PostgresSnapshot
```

查询：

- `pg_stat_activity` active/idle。
- `pg_database_size(current_database())`。
- 等待锁数量：`pg_locks` 中 `NOT granted`。
- 关键表 row counts：`consume_logs`、`subscription_pre_consume_records`、`user_subscriptions`、`tokens`。

查询失败时 `Statused.Status="unavailable"` 且 reason 写明失败点；不要 panic。

- [ ] **步骤 5：实现 runtime reader、sampler 和 peaks**

`monitor.go` 定义：

```go
type SamplerOptions struct {
    Interval time.Duration
    Process func() artifact.ProcessSnapshot
    Runtime func() artifact.RuntimeSnapshot
    Postgres func() artifact.PostgresSnapshot
    Redis func() artifact.RedisSnapshot
}
type Sampler struct {
    interval time.Duration
    process func() artifact.ProcessSnapshot
    runtime func() artifact.RuntimeSnapshot
    postgres func() artifact.PostgresSnapshot
    redis func() artifact.RedisSnapshot
    done chan struct{}
}
func ReadRuntimeSnapshot(ctx context.Context, rawURL string) artifact.RuntimeSnapshot
func NewSampler(opts SamplerOptions) *Sampler
func (s *Sampler) Start() func() []artifact.ResourceSample
func Peaks(samples []artifact.ResourceSample) artifact.ResourcePeaks
```

`ReadRuntimeSnapshot` 必须校验 URL 是 loopback，使用短超时 HTTP client GET，`common.DecodeJson` 到 `artifact.RuntimeSnapshot`；非 2xx 或字段缺失返回 `Statused.Status="unavailable"` 和 reason。`Start()` 立即采样一次，然后按 interval 采样；stop function 停止 goroutine 并返回 copy。`Peaks` 必须填充任务 1 定义的完整 `artifact.ResourcePeaks` 字段，不得只填 RSS/heap/goroutines；任一 subsystem 的 snapshot `Statused.Status != "ok"` 时 peaks 保留已有可用字段，报告阶段显示 unavailable reason。

- [ ] **步骤 6：实现 drain wait**

`drain.go` 定义：

```go
type DrainSample struct { ConsumeLogs int64; PreConsumeRecords int64; SubscriptionTokenUsed int64 }
type DrainExpectations struct { Success int; Tokens int64 }
func EvaluateDrain(samples []DrainSample, expect DrainExpectations) artifact.Statused
func WaitDrain(ctx context.Context, interval time.Duration, sample func() DrainSample, expect DrainExpectations) ([]DrainSample, artifact.Statused)
```

每个表独立判断；reason 必须包含未达标表名。

- [ ] **步骤 7：扩展 `loadtest-collect`**

在 `cmd/loadtest-collect/main.go` 让已存在但未使用的 `--pid-file` 真正读取 pid；新增 flags：

```go
runtimeURL := fs.String("runtime-url", "", "runtime stats URL")
redisAddr := fs.String("redis-addr", "", "redis addr")
```

`--out-snapshot` 时同时填充 process、runtime、postgres、redis、business、logs。runtime URL 必须 loopback；Redis addr 复用 localguard 校验。新增 helper `ReadPIDFile(path) (int, error)`：空 pid-file 返回 0 且 process snapshot unavailable；非数字或 pid 不存在返回运行时错误并让命令 exit 1；有效 pid 调用 `resource.SampleProcess`。PostgreSQL 任一系统查询失败时 `PostgresSnapshot.Statused.Status="unavailable"` 且 reason 标明失败点，不允许 panic 或静默 ok。

- [ ] **步骤 8：运行 monitor/collector 测试通过**

运行：

```bash
go test ./pkg/loadtest/monitor ./cmd/loadtest-collect -count=1
```

预期：PASS。

- [ ] **步骤 9：Commit**

```bash
git add pkg/loadtest/monitor cmd/loadtest-collect
git commit -m "feat(loadtest): 采集资源与业务 drain 指标"
```

---

## 任务 7：Analysis gate 与 failure_class

**文件：**
- 创建：`pkg/loadtest/analysis/analysis.go`
- 创建：`pkg/loadtest/analysis/analysis_test.go`
- 修改：`pkg/loadtest/sweep/sweep.go`
- 修改：`pkg/loadtest/sweep/sweep_test.go`

### 目标

把 workbench 的 diagnostic gate 和 failure_class 转为 new-api 业务语义：成功数、stream done/usage、mock delta、token quota billing invariants 由 `sweep.EvaluateGate` 作为唯一 hard gate 来源；analysis 只消费 `PointResult.Gate` 并补充 resource samples、ports、diagnostic reasons、failure_class。

- [ ] **步骤 1：编写 analysis 失败测试**

创建 `pkg/loadtest/analysis/analysis_test.go`，覆盖：

- `TestBenchmarkHardGateRequiresAllRequestsAndResources`：先构造 `PointResult.Gate.Passed=false` 且 failed reason 为 `all requests must succeed`，断言 `EvaluateBenchmarkPoint` 的 hard gate 保留 sweep gate failed reason；再构造 `PointResult.Gate.Passed=true` 但 resource samples 空，断言 hard gate failed。
- `TestFailureClassPrioritizesBillingInvariant`：invariant `subscription_token_used_matches_success_usage` failed 且无 client parser/transport 主导 => `billing_invariant`。
- `TestFailureClassDetectsClientTransport`：error reason `connect_refused`/`connect_timeout` 占主导 => `client_transport`。
- `TestFailureClassDetectsStreamProtocol`：`missing_done` 占主导且 HTTP 200 => `stream_protocol`。
- `TestFailureClassDetectsCleanupFailed`：ports closed failed => `cleanup_failed`。
- `TestDiagnosticGateClassifiesResourceBottlenecks`：process memory 接近 limit => `server_resource_limit`，PostgreSQL active/idle 接近连接池上限 => `postgres_bottleneck`，Redis commands/success 异常 => `redis_bottleneck`，mock delta status/attempt mismatch => `upstream_mock`，无明确原因但 concurrency 到达 profile 上限 => `capacity_limit`，其他未知 => `unknown`。

- [ ] **步骤 2：运行 analysis 测试确认失败**

运行：

```bash
go test ./pkg/loadtest/analysis -run 'TestBenchmarkHardGateRequiresAllRequestsAndResources|TestFailureClass|TestDiagnosticGateClassifiesResourceBottlenecks' -count=1
```

预期：FAIL，包不存在。

- [ ] **步骤 3：实现 analysis 包**

`analysis.go` 定义：

```go
type Inputs struct { Point artifact.PointResult; Summary artifact.Summary; Ports artifact.PortsClosedArtifact; ResourceSamples artifact.ResourceSamplesArtifact; BusinessDiff artifact.Diff; RequirePorts bool; MaxRequests int }
func EvaluateBenchmarkPoint(in Inputs) artifact.PointAnalysis
func ClassifyFailure(in Inputs) string
```

`EvaluateBenchmarkPoint` 必须先复制 `in.Point.Gate` 到 `PointAnalysis.HardGate`，不得重新实现 subscription/compat billing、stream done、mock delta 或 HTTP status hard gate。若 `in.Point.Gate.Passed=true`，再追加 resource samples 覆盖、resource peaks 非零和 `RequirePorts` 的 hard gate 检查；若追加检查失败，将 `HardGate.Passed=false` 并追加 failed reasons。`ClassifyFailure` 的优先级固定为：`cleanup_failed`、`client_transport`、`stream_protocol`/`client_parser_failure`、`upstream_mock`、`billing_invariant`、`server_resource_limit`、`postgres_bottleneck`、`redis_bottleneck`、`capacity_limit`、`unknown`。Diagnostic gate 只写 diagnostic reasons，不改变 `sweep.EvaluateGate` 的业务判断结果。

- [ ] **步骤 4：扩展 sweep gate 测试**

在 `pkg/loadtest/sweep/sweep_test.go` 增加 `TestEvaluateGateBenchmarkRequiresExactMaxRequests`：scenario `benchmark`，`SummaryExcerpt.Total=3000`、`Success=3000`、`Errors=0`、`StopReason="max_requests"`、`MaxObservedInFlight=250`、resource peaks 非零才通过；`Total=2999`、`StopReason!="max_requests"`、`MaxObservedInFlight=249` 或 success 不等于 total 均失败。

- [ ] **步骤 5：实现 sweep benchmark gate**

在 `pkg/loadtest/sweep/sweep.go` 的 `EvaluateGate` 增加 `case "benchmark":`，这是唯一实现 success/status/stream/mock/business hard gate 的位置；benchmark 必须严格要求 `Total == opts.MaxRequests`、`Success == opts.MaxRequests`、`Errors == 0`、`StopReason == "max_requests"`、`MaxObservedInFlight >= Concurrency`、status 只有 200、stream done/usage 与 success 对齐、mock attempts/status delta 与 summary 对齐、business invariants passed。若 `GateOptions` 尚无 `MaxRequests` 字段，则新增 `MaxRequests int` 并在调用方填入。`SummaryExcerpt.StopReason` 已在任务 1 增加，任务 7 不得修改 artifact schema。

- [ ] **步骤 6：运行 analysis/sweep 测试通过**

运行：

```bash
go test ./pkg/loadtest/analysis ./pkg/loadtest/sweep -count=1
```

预期：PASS。

- [ ] **步骤 7：Commit**

```bash
git add pkg/loadtest/analysis pkg/loadtest/sweep
git commit -m "feat(loadtest): 增加资源矩阵 gate 与失败分类"
```

---

## 任务 8：Orchestrator 和 `loadtest-resource-sweep` CLI

**文件：**
- 创建：`pkg/loadtest/orchestrator/orchestrator.go`
- 创建：`pkg/loadtest/orchestrator/orchestrator_test.go`
- 创建：`cmd/loadtest-resource-sweep/main.go`
- 创建：`cmd/loadtest-resource-sweep/main_test.go`
- 修改：`pkg/loadtest/sweep/sweep.go`
- 修改：`cmd/loadtest-concurrency-sweep/main.go`

### 目标

实现 Go 原生资源受限并发矩阵入口。该命令必须显式 profile 运行，逐点失败即停，最终 cleanup 并写 ports-closed artifact。

- [ ] **步骤 1：编写 orchestrator 编排失败测试**

创建 `pkg/loadtest/orchestrator/orchestrator_test.go`。用 fake dependency 记录调用顺序，测试：

- `TestRunStopsAfterFirstFailedPointAndPassesProfileTimings`：points `[250,500]`，250 failed 后不运行 500；fake `RunPoint` 必须收到 `RequestsPerPoint=3000`、`MaxRequests=3000`、`RampStep=25`、`RampInterval=200ms`、`Duration=45s`、`Timeout=120s`、`Transport.Mode=h1_keepalive`。
- `TestRunAlwaysCleansUpAndWritesPortsArtifact`：mock/server/infra 启动后 point 失败，仍调用 stop mock、stop server、stop infra、ports check、write ports artifact。
- `TestRunAppliesLimitsOnlyToServerPID`：fake `ApplyServerLimits` 收到的 pid 等于 server pid，不等于 mock 或 infra pid。
- `TestRunFailsClosedWhenIsolatedInfraUnavailable`：infra preflight 失败时，不启动 mock/server，不运行 point，返回 2。
- `TestRunPreflightBinaryAndConfigBeforeStartingProcesses`：binary 不存在、不可执行或 config check 失败时返回 2，不启动 infra/mock/server/seed，stderr 经 `artifact.Redact`。
- `TestRunPointReceivesFullSweepOptions`：fake `RunPoint` 断言收到 `BaseURL`、`RuntimeURL`、`APIKey`、`TokenProfile`、`Path`、`Model`、`Scenario`、`Config`、`MockProfile`、`MockHash`、`MockStatsURL`、`Seed`、`ArtifactDir`、`RequestsPerPoint`、`MaxRequests`、ramp/duration/timeout、transport。

- [ ] **步骤 2：运行 orchestrator 测试确认失败**

运行：

```bash
go test ./pkg/loadtest/orchestrator -run 'TestRunStopsAfterFirstFailedPointAndPassesProfileTimings|TestRunAlwaysCleansUpAndWritesPortsArtifact|TestRunAppliesLimitsOnlyToServerPID|TestRunFailsClosedWhenIsolatedInfraUnavailable|TestRunPreflightBinaryAndConfigBeforeStartingProcesses|TestRunPointReceivesFullSweepOptions' -count=1
```

预期：FAIL，包不存在。

- [ ] **步骤 3：实现 orchestrator interfaces**

`orchestrator.go` 定义：

```go
type Process interface {
    PID() int
    Stop(context.Context) error
}
type Options struct {
    ConfigPath string
    Binary string
    WorkDir string
    ArtifactDir string
    Profile string
    Scenario string
    Path string
    TokenProfile string
    APIKey string
    MockProfile string
    Points []int
    RequestsPerPoint int
    RampStep int
    RampInterval time.Duration
    Duration time.Duration
    Timeout time.Duration
    ExternalIsolatedInfra bool
}
// PointOptions mirrors sweep.RunPointOptions; keep fields in sync with pkg/loadtest/sweep.
type PointOptions struct {
    Concurrency int
    BaseURL string
    RuntimeURL string
    APIKey string
    TokenProfile string
    Path string
    Model string
    Scenario string
    ArtifactDir string
    RunContext artifact.RunContext
    Config *loadtestconfig.File
    MockProfile string
    MockHash string
    MockStatsURL string
    RequestsPerPoint int
    MaxRequests int
    RampStep int
    RampInterval time.Duration
    Duration time.Duration
    Timeout time.Duration
    Transport artifact.TransportProfile
    Seed artifact.SeedOutput
    DB *gorm.DB
}
type Dependencies struct {
    BuildOrVerifyBinary func(context.Context, Options) error
    RunConfigCheck func(context.Context, Options) error
    StartInfra func(context.Context, Options, loadtestconfig.File) (Process, error)
    StopInfra func(context.Context, Process) error
    PreflightInfra func(context.Context, Options, loadtestconfig.File) error
    StartMock func(context.Context, Options, artifact.RunContext) (Process, error)
    StartServer func(context.Context, Options, map[string]string) (Process, error)
    BootstrapAndSeed func(context.Context, Options, artifact.RunContext) (artifact.SeedOutput, error)
    RunPoint func(context.Context, PointOptions) (artifact.PointResult, artifact.PointAnalysis, artifact.ResourceSamplesArtifact, error)
    ApplyLimits func(pid int, limits profile.ServerLimits) (resource.ApplyResult, error)
    CheckPorts func(artifact.RunContext, []int) artifact.PortsClosedArtifact
    RenderReport func(context.Context, Options, artifact.SweepResult, []artifact.PointAnalysis, []artifact.ResourceSamplesArtifact, artifact.ResourceLimitsArtifact, artifact.PortsClosedArtifact) error
    WriteJSON func(string, any) error
}
func Run(ctx context.Context, opts Options, deps Dependencies) (artifact.SweepResult, int)
```

`Run` 负责：load config/profile、derive run_context、先执行 `BuildOrVerifyBinary` 和 `RunConfigCheck`，通过后才允许 infra preflight 或启动隔离 infra、启动 mock、bootstrap+seed、启动 server、施加 server-only limits、写 `resource-limits.json`、把 profile 矩阵参数和 CLI/config 派生出的完整 `PointOptions` 传给每个 point、逐点运行、等待 drain、生成 resource samples/analysis、失败停止、生成资源矩阵 report、defer cleanup、写 `ports-closed.json`。如果不能可靠确认 PostgreSQL/Redis 是隔离 loadtest infra，必须 fail closed 返回 2，且不得启动 mock/server 或执行 seed。`RunPoint` 必须调用 `sweep.RunPoint` 或同名包级函数，不得复制旧 cmd 的业务 diff/gate 逻辑。

- [ ] **步骤 4：编写 CLI 失败测试**

创建 `cmd/loadtest-resource-sweep/main_test.go`，覆盖：

- `TestRunRejectsMissingBenchmarkProfile`：未传 `--profile` 返回 2。
- `TestRunRejectsH2CDiagnosticInFirstStage`：传 `--profile h2c_diagnostic` 返回 2 且 stderr 包含未实现原因。
- `TestRunRejectsNonLoopbackURLInConfig`：复用 unsafe config 返回 2。
- `TestRunRejectsDefaultPostgresOrRedisPortsWithoutExplicitIsolatedInfra`：配置指向 PostgreSQL 5432 或 Redis 6379 时返回 2。
- `TestRunRejectsExternalInfraMarkerMismatch`：传 `--external-isolated-infra` 时，PostgreSQL `current_database()` 或 `current_user` 不是 `new_api_loadtest`、Redis 非专用 DB/key prefix 或存在非本轮 key，均返回 2。
- `TestRunRejectsProductionEnvAndRealAPIKey`：work-dir 存在 `.env`、继承 provider key、`--api-key` 不是固定 loadtest key 时返回 2，stderr 脱敏。
- `TestRunWritesPortsClosedOnInjectedFailure`：使用 `RunWithDeps` fake dependency，point 失败仍写 ports artifact，任一端口 open 时 exit != 0。

- [ ] **步骤 4.5：运行 CLI 测试确认失败**

运行：

```bash
go test ./cmd/loadtest-resource-sweep -run 'TestRunRejectsMissingBenchmarkProfile|TestRunRejectsH2CDiagnosticInFirstStage|TestRunRejectsNonLoopbackURLInConfig|TestRunRejectsDefaultPostgresOrRedisPortsWithoutExplicitIsolatedInfra|TestRunRejectsExternalInfraMarkerMismatch|TestRunRejectsProductionEnvAndRealAPIKey|TestRunWritesPortsClosedOnInjectedFailure' -count=1
```

预期：FAIL，命令尚不存在。

- [ ] **步骤 5：实现 CLI**

`cmd/loadtest-resource-sweep/main.go` 必须暴露：

```go
func Run(args []string, stdout io.Writer, stderr io.Writer) int
```

Flags：`--config`、`--profile`、`--binary`、`--work-dir`、`--artifact-dir`、`--scenario`、`--path`、`--token-profile`、`--api-key`、`--mock-profile`、`--points`、`--requests-per-point`、`--ramp-step`、`--ramp-interval`、`--duration`、`--timeout`、`--external-isolated-infra`。`--points` 与 `--requests-per-point` 是显式 smoke 覆盖参数，默认使用 profile 内矩阵。profile 为空返回 2；第一阶段 profile 只接受 `benchmark`，`h2c_diagnostic` 返回 2 并说明未实现；`--api-key` 只接受固定 loadtest key；所有错误输出走 `artifact.Redact`。同时暴露 `RunWithDeps(args []string, stdout io.Writer, stderr io.Writer, deps orchestrator.Dependencies) int` 供 CLI 测试注入 fake dependency；`Run` 只调用 `RunWithDeps(args, stdout, stderr, orchestrator.DefaultDependencies())`。

- [ ] **步骤 6：接入真实 dependencies**

真实 dependency 复用现有包：

- config：`loadtestconfig.Load` + `Validate` + `Profile` + `NewAPIEnvForProfile("benchmark")`，并调用 `localguard.ValidateCleanEnv` / `RejectDefaultInfraPorts` / fixed API key 校验。
- infra：新增受管 infra preflight。默认只允许 PostgreSQL `127.0.0.1:15432`、database/user 均为 `new_api_loadtest`，Redis `127.0.0.1:16379`；禁止 5432/6379。传入 `--external-isolated-infra` 时仍必须通过 marker 校验：PostgreSQL `current_database()` 与 `current_user` 均为 `new_api_loadtest`，Redis 使用 loadtest 专用 DB/key prefix 且预检确认不存在非本轮 key。第一阶段不自动管理默认 5432/6379，也不污染既有 Redis dump；若受管 infra 不可用则 fail closed，不启动 mock/server/seed。
- runner：使用 `runner.BuildCommandWithExpectedLimits` 启动 new-api，expected limits 来自 benchmark profile；启动前 `BuildOrVerifyBinary` 要求 `--binary` 存在、绝对路径、可执行，缺失或不可执行返回 2；`RunConfigCheck` 等价执行 `loadtest-check-config` 的校验逻辑但不得写入生产 `.env`。
- mock：启动 `.loadtest/bin/loadtest-mock-openai` 或当前 binary 路径旁的 `loadtest-mock-openai.exe`。
- point：先把现有 `cmd/loadtest-concurrency-sweep/main.go` 的单点执行提取到 `pkg/loadtest/sweep.RunPoint`，导出完整 `RunPointOptions`，字段至少包含任务 8 `PointOptions` 的全部字段，再让旧 cmd 和 orchestrator 共用。不得复制业务 diff/gate 逻辑。
- monitor：每点开始前启动 sampler，point 结束后先 `WaitDrain`，再 stop sampler、采集 after snapshot、写 `points/cN-resource-samples.json`、`points/cN-resource-peaks.json`、`points/cN-analysis.json`。
- report：调用 resource sweep report renderer 写 `reports/resource-sweep.md`，传入 sweep、analyses、resource samples/peaks、limits、ports，不能只传 sweep。

- [ ] **步骤 7：运行 orchestrator/CLI 测试通过**

运行：

```bash
go test ./pkg/loadtest/orchestrator ./cmd/loadtest-resource-sweep -count=1
```

预期：PASS。

- [ ] **步骤 8：Commit**

```bash
git add pkg/loadtest/orchestrator cmd/loadtest-resource-sweep
git commit -m "feat(loadtest): 新增资源受限矩阵编排命令"
```

---

## 任务 9：Report 与 SOP

**文件：**
- 修改：`pkg/loadtest/report/report.go`
- 修改：`pkg/loadtest/report/report_test.go`
- 修改：`cmd/loadtest-report/main.go`
- 修改：`docs/superpowers/reports/2026-05-20-new-api-local-loadtest-sop.md`

### 目标

报告展示 workbench 同等级信息：最大通过并发、第一失败点、failure_class、资源限制、RSS/CPU/Redis/Postgres/runtime 峰值、ports_closed。

- [ ] **步骤 1：编写 report 失败测试**

在 `pkg/loadtest/report/report_test.go` 增加 `TestRenderResourceSweepReportIncludesCapacityAndResources`：构造 `artifact.SweepResult` + `PointAnalysis` + `ResourceLimitsArtifact` + `PortsClosedArtifact` + 两个 `ResourceSamplesArtifact`（包含非零 process/runtime/Redis/PostgreSQL peaks），断言报告包含：

- `最高通过并发`
- `第一失败并发`
- `failure_class`
- `GOMEMLIMIT=384MiB`
- `RSS peak`
- `CPU peak`
- `runtime heap`
- `Redis used_memory`
- `PostgreSQL active`
- `ports closed`

- [ ] **步骤 2：运行 report 测试确认失败**

运行：

```bash
go test ./pkg/loadtest/report -run TestRenderResourceSweepReportIncludesCapacityAndResources -count=1
```

预期：FAIL，报告缺字段。

- [ ] **步骤 3：实现 resource sweep report**

在 `report.go` 增加：

```go
type ResourceSweepReportInput struct { Sweep artifact.SweepResult; Analyses []artifact.PointAnalysis; ResourceSamples []artifact.ResourceSamplesArtifact; Limits artifact.ResourceLimitsArtifact; Ports artifact.PortsClosedArtifact }
func RenderResourceSweep(input ResourceSweepReportInput) string
```

报告不要声称未采集的指标；`Statused.Status != ok` 的指标显示 unavailable 和 reason。资源峰值必须来自 `ResourceSamples[].Peaks` 或每点 resource peaks artifact，不得只输出标题。最大通过并发与第一失败并发来自 `SweepResult`，每点 `failure_class` 来自对应 `PointAnalysis`。

- [ ] **步骤 4：cmd 接入 report**

`cmd/loadtest-report/main.go` 增加 flags：`--resource-sweep`、`--analysis-dir`、`--resource-limits`、`--ports-closed`、`--resource-samples-dir`。存在 `--resource-sweep` 时调用 `RenderResourceSweep`；`--resource-samples-dir` 从 `points/c*-resource-samples.json` 或 `points/c*-resource-peaks.json` 读取每点资源峰值。这些 flags 只用于资源矩阵报告，不改变现有 `--sweep` 报告路径。

- [ ] **步骤 5：更新 SOP**

在 SOP 增加 benchmark 命令：

```bash
.loadtest/bin/loadtest-resource-sweep --config .loadtest/local-run/config/config.yaml --profile benchmark --binary .loadtest/bin/new-api.exe --work-dir .loadtest/local-run/runtime/new-api --artifact-dir .loadtest/local-run/benchmark --scenario benchmark --path /v1/responses --token-profile subscription --api-key sk-loadtestsub --mock-profile s2-short-stream
```

补充：benchmark 前必须确认 loadtest 端口关闭；benchmark 后必须检查 `ports-closed.json`；H2C diagnostic 是后续扩展，第一阶段 `loadtest-resource-sweep` 不接受该 profile，不能用 H2C 结果替代 benchmark hard gate。

- [ ] **步骤 6：运行 report 测试通过**

运行：

```bash
go test ./pkg/loadtest/report ./cmd/loadtest-report -count=1
```

预期：PASS。

- [ ] **步骤 7：Commit**

```bash
git add pkg/loadtest/report cmd/loadtest-report docs/superpowers/reports/2026-05-20-new-api-local-loadtest-sop.md
git commit -m "docs(loadtest): 补充资源矩阵报告与 SOP"
```

---

## 任务 9.5：三路只读 review 与修复循环

**文件：**
- 不直接修改文件；review 发现问题后由对应任务所有者或主代理按文件归属修复。

### 目标

在最终 smoke 前并发派发 3 个以上只读 review 子代理，覆盖规格符合性、安全资源边界、集成/并发冲突。所有 review 的 `must_fix` 必须修复并复审通过后，才能进入任务 10。

- [ ] **步骤 1：并发派发 review**

至少派发以下 3 个 reviewer：

- 规格覆盖 reviewer：对照设计规格与计划验收标准，检查实现是否遗漏 resource limits、矩阵参数、metrics、gate、report、cleanup。
- 安全资源 reviewer：检查默认连接池、benchmark 显式 profile、loopback/localguard、隔离 PostgreSQL/Redis、ports_closed、server-only limits。
- 集成并发 reviewer：检查 artifact schema、orchestrator 与旧 sweep/cmd 复用、monitor/drain 接线、报告输入、主分支并发开发是否产生冲突残留。

- [ ] **步骤 2：修复 must_fix**

所有 `must_fix` 必须按文件所有权修复。修复时只运行对应 targeted tests，不运行高容量 benchmark。

- [ ] **步骤 3：复审通过**

重新派发相同方向 review。只有全部 reviewer 返回 `verdict: pass` 且 `must_fix: []`，才进入任务 10。

---

## 任务 10：最终验证与本地 smoke

**文件：**
- 修改：`.gitignore`（仅当新增 artifact 目录未被忽略时）
- 不提交 `.loadtest/local-run/**` 运行产物。

### 目标

证明实现没有破坏现有 harness，并用小矩阵端到端 smoke 验证 orchestrator 真实工作。高容量 benchmark 由用户显式运行；不要在普通验证中自动跑 250/500/750/1000。

- [ ] **步骤 1：运行 targeted Go 测试**

```bash
go test ./pkg/loadtest/profile ./pkg/loadtest/artifact ./pkg/loadtest/client ./pkg/loadtest/config ./pkg/loadtest/runner ./pkg/loadtest/resource ./pkg/loadtest/monitor ./pkg/loadtest/analysis ./pkg/loadtest/orchestrator -count=1
```

预期：PASS。

- [ ] **步骤 2：运行 command 测试**

```bash
go test ./cmd/loadtest-client ./cmd/loadtest-collect ./cmd/loadtest-concurrency-sweep ./cmd/loadtest-resource-sweep ./cmd/loadtest-report ./cmd/loadtest-run-new-api -count=1
```

预期：PASS。

- [ ] **步骤 3：运行 runtime route 测试**

```bash
go test ./controller -run TestLoadtestRuntime -count=1
```

预期：PASS。

- [ ] **步骤 4：构建 loadtest commands**

```bash
go build ./cmd/loadtest-check-config ./cmd/loadtest-mock-openai ./cmd/loadtest-client ./cmd/loadtest-concurrency-sweep ./cmd/loadtest-seed ./cmd/loadtest-collect ./cmd/loadtest-run-new-api ./cmd/loadtest-resource-sweep ./cmd/loadtest-report
```

预期：exit 0。

- [ ] **步骤 5：运行小矩阵端到端 smoke**

使用 CLI override 运行 `--profile benchmark --points 2,4 --requests-per-point 10 --ramp-step 1 --ramp-interval 10ms --duration 5s --timeout 30s`，不运行 250+ 高容量矩阵。smoke 仍必须使用隔离 PostgreSQL `15432`、隔离 Redis `16379`、mock `19080`、new-api `13080`、runtime/pprof `8005`，不得连接默认 5432/6379。验证 artifact 包含：`resource-limits.json`、`points/c2-resource-samples.json`、`points/c2-resource-peaks.json`、`points/c2-analysis.json`、`ports-closed.json`、`reports/resource-sweep.md`。

- [ ] **步骤 6：运行 diff check**

```bash
git diff --check
```

预期：无输出。

- [ ] **步骤 7：确认没有未提交验证修复**

运行：

```bash
git status --short
```

预期：只包含本计划范围内已完成任务产生并已提交的文件；`.loadtest/local-run/**` 不应出现在待提交列表。如果 smoke 期间产生修复，按所属任务文件范围提交；如果没有修复，不创建空提交。
