# new-api 资源受限并发矩阵压测套件实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 将 workbench 的资源受限并发矩阵能力迁移为 `new-api` 原生 Go 压测套件，支持 server-only 资源限制、250/500/750/1000 并发矩阵、完整资源采样、失败分类、报告和清理门禁。

**架构：** 在现有 `pkg/loadtest/*` 基础上新增 `profile`、`resource`、`monitor`、`analysis`、`orchestrator` 包和 `cmd/loadtest-resource-sweep` 命令。`loadtest-resource-sweep` 编排配置检查、mock upstream、受限 `new-api` server、逐点 sweep、指标采样、hard/diagnostic gate、报告和 cleanup；低层业务 diff 复用现有 `client`、`metrics`、`sweep`、`runner`、`report`。生产代码只扩展 gated runtime route 与 HTTP ConnState 计数，不改变普通生产路径。

**技术栈：** Go 1.25、标准库 `net/http`/`os/exec`/`runtime`/`syscall`，`golang.org/x/sys/windows`，`gopsutil`，GORM PostgreSQL collector，go-redis INFO parser，项目 `common` JSON 封装。

---

## 文件结构

### 新增文件

- `pkg/loadtest/profile/profile.go`：定义 smoke/benchmark/h2c diagnostic profile、矩阵参数、server limits、client transport limits。
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
- `pkg/loadtest/analysis/analysis.go`：hard gate、diagnostic gate、failure_class。
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
- `pkg/loadtest/client/client_test.go`：覆盖 H1 keepalive/H1 no keepalive/H2C diagnostic 配置、错误分类、summary 扩展。
- `pkg/loadtest/metrics/metrics.go`：暴露 business snapshot/diff 与 monitor system snapshot 合并所需 helper；保持 billing 语义以 request id 为准。
- `pkg/loadtest/sweep/sweep.go`：hard gate 支持 benchmark scenario，要求 max_requests、resource samples、ports-cleanup 前置状态。
- `pkg/loadtest/sweep/sweep_test.go`：覆盖 benchmark hard gate。
- `pkg/loadtest/report/report.go`：报告展示最大通过点、失败点、failure_class、资源峰值、ports_closed。
- `pkg/loadtest/report/report_test.go`：覆盖资源矩阵报告。
- `cmd/loadtest-concurrency-sweep/main.go`：只做必要小改，允许复用 monitor snapshots；不复制 orchestrator 逻辑。
- `controller/loadtest_runtime.go`：新增 GC/runtime 字段与 HTTP ConnState/accept 计数读取。
- `controller/loadtest_runtime_test.go`：覆盖新增字段和 loopback 安全。
- `router/main.go`：向 runtime route 注册 HTTP server state provider。
- `main.go`：配置 `http.Server.ConnState` 和 listener wrapper 计数；继续保持 route gated。
- `config.loadtest.yaml`：增加 `profiles.benchmark` 和 `profiles.h2c_diagnostic`，默认 client 仍为安全 smoke。
- `docs/superpowers/reports/2026-05-20-new-api-local-loadtest-sop.md`：补充 `loadtest-resource-sweep` 命令和清理规则。

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

func TestH2CDiagnosticIsNotHardGateProfile(t *testing.T) {
    p := H2CDiagnostic()
    if p.Transport.Mode != TransportH2CDiagnostic { t.Fatalf("transport mode = %q", p.Transport.Mode) }
    if p.HardGate { t.Fatal("h2c diagnostic must not be first-stage hard gate") }
}
```

- [ ] **步骤 2：运行 profile 测试确认失败**

运行：

```bash
go test ./pkg/loadtest/profile -run 'TestBenchmarkProfileMatchesWorkbenchMatrix|TestSmokeProfileKeepsLocalSafeConnectionLimits|TestH2CDiagnosticIsNotHardGateProfile' -count=1
```

预期：FAIL，包不存在。

- [ ] **步骤 3：实现 profile 包**

创建 `pkg/loadtest/profile/profile.go`，定义 `TransportH1KeepAlive`、`TransportH1NoKeepAlive`、`TransportH2CDiagnostic` 常量，以及 `ServerLimits`、`Transport`、`Relay`、`Profile` 结构。实现：

- `Benchmark()`：points `250,500,750,1000`，每点 `3000` 请求，`25/200ms` ramp，`45s` duration，`120s` timeout，H1 keepalive，server limits `2/100/384MiB/512MiB/2 cores`。
- `Smoke()`：`points=[2]`、每点 `10` 请求、transport `4` conns/host、relay `64/16`。
- `H2CDiagnostic()`：基于 benchmark，但 `Name="h2c_diagnostic"`、`HardGate=false`、transport mode 为 `h2c_diagnostic`。

- [ ] **步骤 4：扩展 artifact schema 测试**

在 `pkg/loadtest/artifact/artifact_test.go` 追加 `TestResourceSweepArtifactsRoundTrip`，构造 `ResourceLimitsArtifact`、扩展后的 `Summary`、`PointAnalysis`、`PortsClosedArtifact`，逐个 `MarshalCanonical` 后断言包含 `run_context`。

- [ ] **步骤 5：运行 artifact 测试确认失败**

运行：

```bash
go test ./pkg/loadtest/artifact -run TestResourceSweepArtifactsRoundTrip -count=1
```

预期：FAIL，缺少新增类型/字段。

- [ ] **步骤 6：扩展 artifact 类型**

在 `pkg/loadtest/artifact/artifact.go` 中增加 `TransportProfile`、`ErrorSample`、`ResourceLimitsArtifact`、`ResourceSample`、`ResourceSamplesArtifact`、`PortsClosedArtifact`、`PointAnalysis`。扩展现有类型：

- `Summary` 增加 `ProtocolCounts map[string]int`、`FirstErrorSamples []ErrorSample`、`Transport TransportProfile`。
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
- 修改：`pkg/loadtest/runner/runner.go`
- 修改：`pkg/loadtest/runner/runner_test.go`
- 修改：`config.loadtest.yaml`

### 目标

配置文件支持 smoke/benchmark/h2c profile。默认仍安全；只有显式 benchmark profile 才允许 1024 relay/client 连接池。runner 必须允许并校验 `GOMEMLIMIT=384MiB`。

- [ ] **步骤 1：编写 config profile 失败测试**

在 `pkg/loadtest/config/config_test.go` 追加 `TestBenchmarkProfileAllowsExplicitHighCapacityConnectionLimits` 和 `TestDefaultClientLimitsRemainSafeWithoutBenchmarkProfile`。前者构造 `Profiles["benchmark"]`，断言 relay/client `1024` 和 `GOMEMLIMIT=384MiB` 可通过；后者将顶层 `Client.MaxIdleConns=129`，断言 `Validate()` 失败。添加 `mustDuration(value string) Duration` helper 调用 `ParseDuration`。

- [ ] **步骤 2：运行 config profile 测试确认失败**

运行：

```bash
go test ./pkg/loadtest/config -run 'TestBenchmarkProfileAllowsExplicitHighCapacityConnectionLimits|TestDefaultClientLimitsRemainSafeWithoutBenchmarkProfile' -count=1
```

预期：FAIL，缺少 `Profiles`、`ProfileConfig` 等类型。

- [ ] **步骤 3：实现 config profile 类型**

在 `pkg/loadtest/config/config.go` 增加 `Profiles map[string]ProfileConfig`、`Duration`、`TransportConfig`、`ServerLimitsConfig`、`ProfileConfig`，并实现 `ParseDuration`、`(*Duration).UnmarshalYAML`、`(File).Profile(name)`。`validateProfile` 要求：points 正数递增、requests/ramp/duration/timeout 正数、transport mode 是 `h1_keepalive`/`h1_no_keepalive`/`h2c_diagnostic`、只有 profile 内允许 relay/client 超过顶层默认安全上限；`h2c_diagnostic` 的 `HardGate` 必须是 false。

- [ ] **步骤 4：runner 增加 GOMEMLIMIT 测试**

在 `pkg/loadtest/runner/runner_test.go` 的 `safeEnv()` 增加 `"GOMEMLIMIT": "384MiB"`。在 `TestBuildCommandUsesCleanAllowlistEnvironment` required 列表增加 `"GOMEMLIMIT=384MiB"`。新增 `TestBuildCommandRejectsUnexpectedGOMEMLIMIT`：设置 `env["GOMEMLIMIT"]="8GiB"` 后调用 `BuildCommand`，断言返回错误。

- [ ] **步骤 5：运行 runner 测试确认失败**

运行：

```bash
go test ./pkg/loadtest/runner -run 'TestBuildCommandUsesCleanAllowlistEnvironment|TestBuildCommandRejectsUnexpectedGOMEMLIMIT' -count=1
```

预期：FAIL，`GOMEMLIMIT` 未允许/校验。

- [ ] **步骤 6：实现 runner GOMEMLIMIT 校验**

在 `pkg/loadtest/runner/runner.go` 的 `allowedEnvKeys` 增加 `GOMEMLIMIT`。在 `validateEnv` required map 增加 `"GOMEMLIMIT": "384MiB"`。

- [ ] **步骤 7：更新 `config.loadtest.yaml`**

新增 `profiles.benchmark` 和 `profiles.h2c_diagnostic`。benchmark 使用 points `[250,500,750,1000]`、`requests_per_point: 3000`、`ramp_step: 25`、`ramp_interval: 200ms`、`duration: 45s`、`timeout: 120s`、relay/client `1024/1024`、server limits `2/100/384MiB/536870912/2`。h2c diagnostic 同矩阵但 `hard_gate: false`、`transport.mode: h2c_diagnostic`。不要修改顶层 `client.max_idle_conns: 64` / `max_idle_conns_per_host: 16`。

- [ ] **步骤 8：运行 config/runner 测试通过**

运行：

```bash
go test ./pkg/loadtest/config ./pkg/loadtest/runner -count=1
```

预期：PASS。

- [ ] **步骤 9：Commit**

```bash
git add pkg/loadtest/config pkg/loadtest/runner config.loadtest.yaml
git commit -m "feat(loadtest): 支持 benchmark profile 配置"
```

---

## 任务 3：Client transport、协议统计与错误分类

**文件：**
- 修改：`pkg/loadtest/client/client.go`
- 修改：`pkg/loadtest/client/client_test.go`
- 修改：`cmd/loadtest-client/main.go`

### 目标

client 支持 H1 keepalive、H1 no keepalive、H2C diagnostic schema；summary 记录协议计数和前 N 个错误样本；错误原因比现有 `http_client_do_error` 更细。

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
第一阶段明确不实现 `h2c_diagnostic` 的真实 HTTP/2 cleartext dial：当 `Mode == "h2c_diagnostic"` 时，`normalizeAndValidateOptions` 必须返回 config error：`h2c diagnostic transport is not implemented in this phase`。不要引入 `golang.org/x/net/http2`，避免把诊断模式混入第一阶段 hard gate。

- [ ] **步骤 4：实现协议统计和错误样本**

在 `requestResult` 增加 `protocol string` 和 `phase string`。`doOne` 收到 response 后设置 `protocol = resp.Proto`。实现 `classifyHTTPError(err error, ctx context.Context) string`：

- timeout：`connect_timeout` 或 `request_timeout`。
- Windows/Linux connection refused：`connect_refused`。
- connection reset / wsarecv：`connection_reset`。
- 默认：`http_client_do_error`。

`buildSummary` 统计 `ProtocolCounts`，最多收集 10 个 `FirstErrorSamples`。

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

把 `SetRouter` 改为接收 `loadtestStats *controller.LoadtestHTTPStats`，把 `RegisterLoadtestRuntimeRoute` 改为接收 stats。在 `main.go` 创建 `loadtestHTTPStats := controller.NewLoadtestHTTPStats()`，把 `server.Run(listenAddr)` 替换为显式 `http.Server{Addr, Handler, ConnState}` 和 listener wrapper，以便调用 `OnAccept()`。listener wrapper 放在 controller 中，命名为 `NewLoadtestCountingListener`。

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
```

测试必须 import `net`、`strconv`、`strings`、`testing`、`artifact`、`profile`。

- [ ] **步骤 2：运行 resource 测试确认失败**

运行：

```bash
go test ./pkg/loadtest/resource -run 'TestBuildLimitsArtifactRecordsServerOnlyScope|TestPortsClosedDetectsOpenAndClosedLoopbackPorts' -count=1
```

预期：FAIL，包不存在。

- [ ] **步骤 3：实现 limits.go**

实现 `ApplyResult`、`BuildLimitsArtifact(rc, limits, result)`、`ApplyServerLimits(pid, limits)`、`ServerEnv(limits)`。Artifact 必须写明 scope：`new-api server process only; load generator, mock upstream, PostgreSQL, Redis, and orchestrator remain uncapped except normal OS scheduling`。

- [ ] **步骤 4：实现 Windows limits**

`limits_windows.go` 使用 `golang.org/x/sys/windows` 实现 `OpenProcess`、`SetProcessAffinityMask`、`CreateJobObject`、`SetInformationJobObject(JobObjectExtendedLimitInformation)`、`AssignProcessToJobObject`。若当前 `x/sys/windows` 类型名称不匹配，按包实际定义调整；不要在业务文件散落 magic syscall。

- [ ] **步骤 5：实现 non-Windows limits**

`limits_other.go` 返回 `Status="best_effort"`，`MemoryLimitEnforced=false`，reason 明确说明第一阶段只有 Windows Job Object 强制 memory limit。

- [ ] **步骤 6：实现 process sampler**

`process.go` 使用 `gopsutil/process` 实现 `SampleProcess(pid int) artifact.ProcessSnapshot`，填充 RSS、CPUPercent、NumThreads、NumHandles、CPUTimeSeconds。不可用字段保持 0，`Statused.Status` 为 `ok` 或 `unavailable`。

- [ ] **步骤 7：实现 ports closed**

`ports.go` 实现 `CheckPortsClosed(rc artifact.RunContext, ports []int) artifact.PortsClosedArtifact`，对每个端口 `net.DialTimeout("tcp", "127.0.0.1:<port>", 300*time.Millisecond)`，open 则 `Passed=false`。

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

### 目标

资源采样从 minimal collector 升级为可复用 monitor。每点输出 resource samples/peaks，collector 命令也能采集 system snapshot。

- [ ] **步骤 1：编写 parser/peaks/drain 失败测试**

创建 `pkg/loadtest/monitor/monitor_test.go`，包含：

- `TestParseRedisInfoExtractsMemoryClientsAndCommands`：输入 Redis INFO fixture，断言 `ConnectedClients`、`UsedMemoryBytes`、`TotalCommandsProcessed`、`Keyspace`。
- `TestResourcePeaksUsesMaxAcrossSamples`：两条 sample，断言 goroutines、heap、RSS 取最大值。
- `TestDrainStatusRequiresEachTableStable`：consume logs、pre-consume records、subscription token used 全部达到预期才 passed，任一缺口 failed 且 reason 指出表名。
- `TestSamplerCollectsAtLeastTwoSamples`：用 10ms interval 和 fake sampler，35ms 后至少两条 sample。

- [ ] **步骤 2：运行 monitor 测试确认失败**

运行：

```bash
go test ./pkg/loadtest/monitor -run 'TestParseRedisInfoExtractsMemoryClientsAndCommands|TestResourcePeaksUsesMaxAcrossSamples|TestDrainStatusRequiresEachTableStable|TestSamplerCollectsAtLeastTwoSamples' -count=1
```

预期：FAIL，包不存在。

- [ ] **步骤 3：实现 Redis INFO parser**

`redis.go` 实现：

```go
func ParseRedisInfo(info string) artifact.RedisSnapshot
```

解析 `connected_clients`、`used_memory`、`used_memory_rss`、`mem_fragmentation_ratio`、`instantaneous_ops_per_sec`、`total_commands_processed` 和 `db*` keyspace。无效数字跳过，不 panic。

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

- [ ] **步骤 5：实现 sampler 和 peaks**

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
func NewSampler(opts SamplerOptions) *Sampler
func (s *Sampler) Start() func() []artifact.ResourceSample
func Peaks(samples []artifact.ResourceSample) artifact.ResourcePeaks
```

`Start()` 立即采样一次，然后按 interval 采样；stop function 停止 goroutine 并返回 copy。

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

`--out-snapshot` 时同时填充 process、runtime、postgres、redis、business、logs。runtime URL 必须 loopback；Redis addr 复用 localguard 校验。

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

把 workbench 的 hard gate 和 diagnostic gate 转为 new-api 业务语义：成功数、stream done/usage、mock delta、token quota billing invariants、resource samples、failure_class。

- [ ] **步骤 1：编写 analysis 失败测试**

创建 `pkg/loadtest/analysis/analysis_test.go`，覆盖：

- `TestBenchmarkHardGateRequiresAllRequestsAndResources`：`Total=3000`、`Success=2999` 或 resource samples 空时 hard gate failed。
- `TestFailureClassPrioritizesBillingInvariant`：invariant `subscription_token_used_matches_success_usage` failed => `billing_invariant`。
- `TestFailureClassDetectsClientTransport`：error reason `connect_refused`/`connect_timeout` 占主导 => `client_transport`。
- `TestFailureClassDetectsStreamProtocol`：`missing_done` 占主导且 HTTP 200 => `stream_protocol`。
- `TestFailureClassDetectsCleanupFailed`：ports closed failed => `cleanup_failed`。

- [ ] **步骤 2：运行 analysis 测试确认失败**

运行：

```bash
go test ./pkg/loadtest/analysis -run 'TestBenchmarkHardGateRequiresAllRequestsAndResources|TestFailureClass' -count=1
```

预期：FAIL，包不存在。

- [ ] **步骤 3：实现 analysis 包**

`analysis.go` 定义：

```go
type Inputs struct { Point artifact.PointResult; Summary artifact.Summary; Ports artifact.PortsClosedArtifact; RequirePorts bool; MaxRequests int }
func EvaluateBenchmarkPoint(in Inputs) artifact.PointAnalysis
func ClassifyFailure(in Inputs) string
```

Hard gate 要求：`success == max_requests`、`errors == 0`、`stop_reason == max_requests`、`max_observed_in_flight >= concurrency`、status 只有 200、stream done/usage 与 success 对齐、business invariants passed、resource peaks 非零。`RequirePorts=true` 时 ports 未关闭直接 fail。

- [ ] **步骤 4：扩展 sweep gate 测试**

在 `pkg/loadtest/sweep/sweep_test.go` 增加 `TestEvaluateGateBenchmarkRequiresExactMaxRequests`：scenario `benchmark`，`SummaryExcerpt.Total=3000`、`Success=3000`、`MaxObservedInFlight=250`、resource peaks 非零才通过；`Total=2999` 或 success 不等于 total 失败。

- [ ] **步骤 5：实现 sweep benchmark gate**

在 `pkg/loadtest/sweep/sweep.go` 的 `EvaluateGate` 增加 `case "benchmark":`，复用 `successStreamGateFailures`，额外要求 `Total == opts.MaxRequests`。若 `GateOptions` 尚无 `MaxRequests` 字段，则新增 `MaxRequests int` 并在调用方填入。

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

### 目标

实现 Go 原生资源受限并发矩阵入口。该命令必须显式 profile 运行，逐点失败即停，最终 cleanup 并写 ports-closed artifact。

- [ ] **步骤 1：编写 orchestrator 编排失败测试**

创建 `pkg/loadtest/orchestrator/orchestrator_test.go`。用 fake dependency 记录调用顺序，测试：

- `TestRunStopsAfterFirstFailedPoint`：points `[250,500]`，250 failed 后不运行 500。
- `TestRunAlwaysCleansUpAndWritesPortsArtifact`：mock/server 启动后 point 失败，仍调用 stop mock、stop server、ports check、write ports artifact。
- `TestRunAppliesLimitsOnlyToServerPID`：fake `ApplyServerLimits` 收到的 pid 等于 server pid，不等于 mock pid。

- [ ] **步骤 2：运行 orchestrator 测试确认失败**

运行：

```bash
go test ./pkg/loadtest/orchestrator -run 'TestRunStopsAfterFirstFailedPoint|TestRunAlwaysCleansUpAndWritesPortsArtifact|TestRunAppliesLimitsOnlyToServerPID' -count=1
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
}
type PointOptions struct {
    Concurrency int
    ArtifactDir string
    RunContext artifact.RunContext
}
type Dependencies struct {
    StartMock func(context.Context, Options, artifact.RunContext) (Process, error)
    StartServer func(context.Context, Options, map[string]string) (Process, error)
    RunPoint func(context.Context, PointOptions) (artifact.PointResult, artifact.PointAnalysis, error)
    ApplyLimits func(pid int, limits profile.ServerLimits) (resource.ApplyResult, error)
    CheckPorts func(artifact.RunContext, []int) artifact.PortsClosedArtifact
    WriteJSON func(string, any) error
}
func Run(ctx context.Context, opts Options, deps Dependencies) (artifact.SweepResult, int)
```

`Run` 负责：load config/profile、derive run_context、启动 mock、启动 server、施加 server-only limits、写 `resource-limits.json`、逐点运行、失败停止、defer cleanup、写 `ports-closed.json`。

- [ ] **步骤 4：编写 CLI 失败测试**

创建 `cmd/loadtest-resource-sweep/main_test.go`，覆盖：

- `TestRunRejectsMissingBenchmarkProfile`：未传 `--profile` 返回 2。
- `TestRunRejectsNonLoopbackURLInConfig`：复用 unsafe config 返回 2。
- `TestRunWritesPortsClosedOnInjectedFailure`：使用 fake orchestrator dependency 或 `RunWithDeps`，point 失败仍写 ports artifact。

- [ ] **步骤 5：实现 CLI**

`cmd/loadtest-resource-sweep/main.go` 必须暴露：

```go
func Run(args []string, stdout io.Writer, stderr io.Writer) int
```

Flags：`--config`、`--profile`、`--binary`、`--work-dir`、`--artifact-dir`、`--scenario`、`--path`、`--token-profile`、`--api-key`、`--mock-profile`、`--points`、`--requests-per-point`。`--points` 与 `--requests-per-point` 是显式覆盖参数；默认使用 profile 内矩阵。profile 为空返回 2；profile 不是 `benchmark` 或 `h2c_diagnostic` 返回 2；所有错误输出走 `artifact.Redact`。

- [ ] **步骤 6：接入真实 dependencies**

真实 dependency 复用现有包：

- config：`loadtestconfig.Load` + `Validate` + `Profile`。
- runner：`runner.BuildCommand` 启动 new-api。
- mock：启动 `.loadtest/bin/loadtest-mock-openai` 或当前 binary 路径旁的 `loadtest-mock-openai.exe`。
- point：调用现有 `loadtest-concurrency-sweep` 包级逻辑；如果 runPoint 仍在 cmd 包不可复用，则先提取到 `pkg/loadtest/sweep`，cmd 只做参数解析。
- monitor：每点开始前启动 sampler，point 结束后 stop 并写 samples/peaks。

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

在 `pkg/loadtest/report/report_test.go` 增加 `TestRenderResourceSweepReportIncludesCapacityAndResources`：构造 `artifact.SweepResult` + `PointAnalysis` + `ResourceLimitsArtifact` + `PortsClosedArtifact`，断言报告包含：

- `最高通过并发`
- `第一失败并发`
- `failure_class`
- `GOMEMLIMIT=384MiB`
- `RSS peak`
- `Redis`
- `PostgreSQL`
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
type ResourceSweepReportInput struct { Sweep artifact.SweepResult; Analyses []artifact.PointAnalysis; Limits artifact.ResourceLimitsArtifact; Ports artifact.PortsClosedArtifact }
func RenderResourceSweep(input ResourceSweepReportInput) string
```

报告不要声称未采集的指标；`Statused.Status != ok` 的指标显示 unavailable 和 reason。

- [ ] **步骤 4：cmd 接入 report**

`cmd/loadtest-report/main.go` 增加 flags：`--resource-sweep`、`--analysis-dir`、`--resource-limits`、`--ports-closed`。存在 `--resource-sweep` 时调用 `RenderResourceSweep`；这些 flags 只用于资源矩阵报告，不改变现有 `--sweep` 报告路径。

- [ ] **步骤 5：更新 SOP**

在 SOP 增加 benchmark 命令：

```bash
.loadtest/bin/loadtest-resource-sweep --config .loadtest/local-run/config/config.yaml --profile benchmark --binary .loadtest/bin/new-api.exe --work-dir .loadtest/local-run/runtime/new-api --artifact-dir .loadtest/local-run/benchmark --scenario benchmark --path /v1/responses --token-profile subscription --api-key sk-loadtestsub --mock-profile s2-short-stream
```

补充：benchmark 前必须确认 loadtest 端口关闭；benchmark 后必须检查 `ports-closed.json`；H2C diagnostic 不可替代 benchmark hard gate。

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

使用 profile override 或 CLI override 运行 `points=2,4`、每点 10 请求，不运行 250+ 高容量矩阵。验证 artifact 包含：`resource-limits.json`、`c2-resource-samples.json`、`c2-analysis.json`、`ports-closed.json`、report。

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
