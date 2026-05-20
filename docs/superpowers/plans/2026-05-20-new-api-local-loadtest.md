# new-api 本地受控压测 Harness 实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 在主分支直接实现一套只访问 loopback 资源的本地受控压测 harness，用于比较 `new-api` 不同 commit/config 下的流式 relay、订阅计费、日志、PostgreSQL、Redis 和运行时指标。

**架构：** 第一阶段由多个独立 Go binary 组成：配置检查生成干净 env 和 run context，mock OpenAI upstream 固定响应，client 产生可解析 summary，sweep 编排并发点，seed 写入隔离数据，collector 采集 snapshot/diff 和业务不变量，runner 以 clean env 启动 `new-api`，report 输出单轮和 baseline/candidate 对比。生产服务只增加 loopback 绑定、pprof 地址和 gated runtime stats route。

**技术栈：** Go 1.22+、Gin、GORM、PostgreSQL collector raw SQL、Redis INFO、标准库 `net/http`/`runtime`/`os/exec`；业务 JSON marshal/unmarshal 使用 `common` 封装。

---

## 约束与不变量

- 直接在当前主分支开发，不使用 worktree。
- 不触碰 `sub2api`。
- 不关闭 `DataExportEnabled`、`LogConsumeEnabled` 或订阅并发队列。
- 所有新命令必须 fail-closed：非 loopback URL、非 loadtest PostgreSQL、非 loopback Redis、真实 upstream key、生产 `.env` 继承都必须失败。
- 第一阶段端到端只支持 PostgreSQL；新增生产代码仍必须兼容 SQLite/MySQL/PostgreSQL，PostgreSQL-only 查询只能放在 `pkg/loadtest/metrics`。
- API key 固定：`sk-loadtestsub`、`sk-loadtestcompat`、`sk-loadtestinvalid`；DB token key 固定：`loadtestsub`、`loadtestcompat`。
- request join 只能使用 new-api 响应头 `X-Oneapi-Request-Id`，即 artifact 中的 `new_api_request_id`。
- `SeedOutput` hash 不能自引用：`seed.json` 内的 `run_context.seed_output_hash` 必须为空或省略；`.loadtest/run-context.seeded.json` 再加入 seed hash。
- mock stats snapshot 和 delta 必须包含当前 `run_context`；collector 必须校验 mock delta、summary、diff 的 `run_context` 一致。
- `stdout_full_params_lines` 只扫描 stdout；`perf_metric_upsert_errors` 必须扫描 stdout 和 stderr。
- 所有 `cmd/loadtest-*` 必须暴露 `Run(args []string, stdout io.Writer, stderr io.Writer) int`；`main` 只能调用 `os.Exit(Run(...))`。安全/配置/业务 gate 失败返回 `2`，网络/数据库/Redis/文件系统运行时失败返回 `1`，stderr 必须脱敏。

## 文件结构

### 新增 loadtest 包与命令

- 创建：`pkg/loadtest/artifact/artifact.go`、`pkg/loadtest/artifact/artifact_test.go`。定义稳定 JSON schema、规范化 hash、redaction。
- 创建：`pkg/loadtest/localguard/localguard.go`、`pkg/loadtest/localguard/localguard_test.go`。集中做 loopback/loadtest 安全检查。
- 创建：`pkg/loadtest/config/config.go`、`pkg/loadtest/config/config_test.go`、`cmd/loadtest-check-config/main.go`。读取 YAML、校验、生成 env 和 base run context。
- 创建：`config.loadtest.yaml`。提供可复制的默认本地配置，所有地址必须是 loopback/loadtest。
- 创建：`pkg/loadtest/mockopenai/mockopenai.go`、`pkg/loadtest/mockopenai/mockopenai_test.go`、`cmd/loadtest-mock-openai/main.go`。本地 mock OpenAI/Responses 服务。
- 创建：`pkg/loadtest/client/client.go`、`pkg/loadtest/client/client_test.go`、`cmd/loadtest-client/main.go`。压测客户端、S0 健康检查与 summary。
- 创建：`pkg/loadtest/seed/seed.go`、`pkg/loadtest/seed/seed_test.go`、`cmd/loadtest-seed/main.go`。幂等 seed loadtest 用户、token、channel、subscription 和 option。
- 创建：`pkg/loadtest/metrics/metrics.go`、`pkg/loadtest/metrics/metrics_test.go`、`cmd/loadtest-collect/main.go`。采集 snapshot/diff、日志和业务不变量。
- 创建：`pkg/loadtest/sweep/sweep.go`、`pkg/loadtest/sweep/sweep_test.go`、`cmd/loadtest-concurrency-sweep/main.go`。场景 run context 派生、并发点编排、gate。
- 创建：`pkg/loadtest/runner/runner.go`、`pkg/loadtest/runner/runner_test.go`、`cmd/loadtest-run-new-api/main.go`。clean env 启动 `new-api`。
- 创建：`pkg/loadtest/report/report.go`、`pkg/loadtest/report/report_test.go`、`cmd/loadtest-report/main.go`。报告与 regression gate。
- 创建：`docs/superpowers/reports/loadtest-report-template.md`。报告模板。
- 创建：`docs/superpowers/reports/2026-05-20-new-api-local-loadtest-sop.md`。记录从配置复制到 S0/S1 smoke 的命令顺序。

### 修改生产服务

- 修改：`main.go`。支持 `HOST`、`PPROF_ADDR`、`ENABLE_PPROF`、loadtest profile rate 和主服务 loopback bind。
- 修改：`router/main.go`。在 web no-route 前注册 gated runtime stats route。
- 创建：`controller/loadtest_runtime.go`、`controller/loadtest_runtime_test.go`。`/debug/loadtest/runtime`。
- 修改：`model/utils.go`。新增 batch update pending 只读快照 helper，供 runtime stats route 使用；不得暴露新的 DB/log DB 句柄，不得影响生产业务路径。

---

## 任务 1：共享 artifact schema 与 hash

**文件：**

- 创建：`pkg/loadtest/artifact/artifact.go`
- 创建：`pkg/loadtest/artifact/artifact_test.go`

### 目标

先落地所有跨命令 JSON 类型，禁止后续包重复定义跨命令 schema。

### 步骤

- [ ] **步骤 1：编写 schema round-trip、hash 和 redaction 测试**

在 `pkg/loadtest/artifact/artifact_test.go` 添加：

```go
package artifact

import (
    "strings"
    "testing"
)

func testRunContext() RunContext {
    return RunContext{SchemaVersion: 1, Role: "baseline", Commit: "abcdef0", ComparisonConfigHash: "sha256:cfg", SeedOutputHash: "sha256:seed", MockHash: "sha256:mock", CacheMode: "cold-fresh-role,warm-per-point", Scenario: "s2-short-stream", Path: "/v1/responses", TokenProfile: "subscription", Model: "gpt-5.5"}
}

func TestArtifactRoundTripIncludesRunContextAndSeedOutput(t *testing.T) {
    rc := testRunContext()
    summary := Summary{SchemaVersion: 1, RunContext: rc, Path: "/v1/responses", Scenario: "s2-short-stream", TokenProfile: "subscription", Model: "gpt-5.5", Total: 1, Success: 1, Requests: []RequestRecord{{RequestIndex: 1, ClientRequestID: "client-1", NewAPIRequestID: "rid-1", UpstreamRequestID: "upstream-loadtest-1", StatusCode: 200, Success: true, PromptTokens: 11, CompletionTokens: 17, TotalTokens: 28}}}
    seed := SeedOutput{SchemaVersion: 1, RunContext: rc.WithoutSeedOutputHash().WithoutMockHash(), UserIDSubscription: 1001, UserIDCompat: 1002, TokenSubscription: "sk-loadtestsub", TokenCompat: "sk-loadtestcompat", TokenDBKeySubscription: "loadtestsub", TokenDBKeyCompat: "loadtestcompat", ChannelID: 1, Model: "gpt-5.5", Group: "default", MockBaseURL: "http://127.0.0.1:19080", ExpectedUsagePerSuccess: Usage{PromptTokens: 11, CompletionTokens: 17, TotalTokens: 28}, RatioConfig: map[string]any{"ModelRatio": map[string]any{"gpt-5.5": float64(1)}}, FeatureOptions: map[string]any{"LogConsumeEnabled": true, "DataExportEnabled": true, "perf_metrics_setting.enabled": true, "RetryTimes": float64(0), "AutomaticRetryStatusCodes": ""}}
    mockDelta := MockStatsDelta{SchemaVersion: 1, RunContext: rc, Path: "c100-mock-stats-delta.json", Hash: "sha256:mockdelta", Actual429: 5, Actual502: 1, UpstreamAttemptsTotal: 100}
    diff := Diff{SchemaVersion: 1, RunContext: rc, SummaryPath: "c100-summary.json", MockStatsDeltaPath: mockDelta.Path, MockStatsDeltaHash: mockDelta.Hash, MockDelta: mockDelta, BusinessDelta: BusinessDelta{ChargesByRequest: []ChargeByRequest{{NewAPIRequestID: "rid-1", ClientRequestID: "client-1", UpstreamRequestID: "upstream-loadtest-1", StatusCode: 200, Success: true, LogQuota: 28, SubscriptionTokenDelta: 28, NetSubscriptionTokenDelta: 28}}}}
    for name, v := range map[string]any{"summary": summary, "seed": seed, "mockDelta": mockDelta, "diff": diff} {
        b, err := MarshalCanonical(v)
        if err != nil { t.Fatalf("%s marshal: %v", name, err) }
        if !strings.Contains(string(b), "run_context") { t.Fatalf("%s missing run_context: %s", name, b) }
    }
    if seed.RunContext.MockHash != "" { t.Fatalf("seed context contains scenario mock hash: %#v", seed.RunContext) }
}

func TestSeedOutputHashExcludesSelfReference(t *testing.T) {
    rc := testRunContext()
    seed := SeedOutput{SchemaVersion: 1, RunContext: rc.WithoutSeedOutputHash().WithoutMockHash(), TokenDBKeySubscription: "loadtestsub", TokenDBKeyCompat: "loadtestcompat"}
    hash1, err := HashCanonical(seed)
    if err != nil { t.Fatal(err) }
    seed.RunContext.SeedOutputHash = hash1
    hash2, err := HashSeedOutput(seed)
    if err != nil { t.Fatal(err) }
    if hash1 != hash2 { t.Fatalf("self-referential seed hash: %s != %s", hash1, hash2) }
    if seed.RunContext.MockHash != "" { t.Fatalf("seed hash input contains scenario mock hash: %#v", seed.RunContext) }
}

func TestRedactRemovesSecretsAndProductionURLs(t *testing.T) {
    input := "postgresql://user:secret@example.com:5432/prod OPENAI_API_KEY=sk-real-production"
    redacted := Redact(input)
    if strings.Contains(redacted, "secret") || strings.Contains(redacted, "example.com") || strings.Contains(redacted, "sk-real") { t.Fatalf("not redacted: %s", redacted) }
}
```

- [ ] **步骤 2：运行 artifact 测试确认失败**

```bash
go test ./pkg/loadtest/artifact -run 'TestArtifactRoundTripIncludesRunContextAndSeedOutput|TestSeedOutputHashExcludesSelfReference|TestRedactRemovesSecretsAndProductionURLs' -count=1
```

预期：FAIL，包不存在或类型不存在。

- [ ] **步骤 3：实现 artifact 包**

实现以下类型和函数：

```go
type RunContext struct {
    SchemaVersion int    `json:"schema_version"`
    Role string          `json:"role"`
    Commit string        `json:"commit"`
    ComparisonConfigHash string `json:"comparison_config_hash"`
    SeedOutputHash string `json:"seed_output_hash,omitempty"`
    MockHash string      `json:"mock_hash,omitempty"`
    CacheMode string     `json:"cache_mode"`
    Scenario string      `json:"scenario,omitempty"`
    Path string          `json:"path,omitempty"`
    TokenProfile string  `json:"token_profile,omitempty"`
    Model string         `json:"model"`
}

type Usage struct { PromptTokens int `json:"prompt_tokens"`; CompletionTokens int `json:"completion_tokens"`; TotalTokens int `json:"total_tokens"` }
```

`Usage` 需要显式 JSON 字段名：`prompt_tokens`、`completion_tokens`、`total_tokens`。

必须包含：`Statused`、`Summary`、`RequestRecord`、`StreamStats`、`Usage`、`SeedOutput`、`MockStats`、`MockStatsDelta`、`Snapshot`、`Diff`、`BusinessSnapshot`、`BusinessDelta`、`LogsSnapshot`、`ChargeByRequest`、`Invariant`、`PostgresSnapshot`、`RedisSnapshot`、`RuntimeSnapshot`、`ProcessSnapshot`、`PostgresDelta`、`RedisDelta`、`RuntimeDelta`、`ResourceDelta`、`SummaryExcerpt`、`ProfilePaths`、`ResourcePeaks`、`GateResult`、`SweepResult`、`PointResult`。
`Statused` 字段固定为 `status`、`reason`；所有可 unavailable 的 snapshot/delta 类型必须嵌入或包含 `Statused`。`PointResult` 必须包含 `SummaryExcerpt`、`MockDelta`、`Invariants`、`ResourcePeaks`、`ResourceDelta`、`ProfilePaths`、`GateResult`，供 sweep/report 跨包复用。

`MockStats.RunContext` 和 `MockStatsDelta.RunContext` 必须必填，不得 `omitempty`。

实现：

```go
func (r RunContext) WithoutSeedOutputHash() RunContext
func (r RunContext) WithoutMockHash() RunContext
func MarshalCanonical(v any) ([]byte, error)
func HashCanonical(v any) (string, error)
func HashSeedOutput(seed SeedOutput) (string, error)
func Redact(s string) string
```

所有 marshal 调用必须用 `common.Marshal`。

- [ ] **步骤 4：运行 artifact 测试通过**

```bash
go test ./pkg/loadtest/artifact -count=1
```

预期：PASS。

---

## 任务 2：localguard 与配置检查

**文件：**

- 创建：`pkg/loadtest/localguard/localguard.go`
- 创建：`pkg/loadtest/localguard/localguard_test.go`
- 创建：`pkg/loadtest/config/config.go`
- 创建：`pkg/loadtest/config/config_test.go`
- 创建：`cmd/loadtest-check-config/main.go`

### 目标

实现 fail-closed 安全边界、YAML 配置读取、env 输出、base run context。

### 步骤

- [ ] **步骤 1：编写 localguard 测试**

```go
func TestLocalGuardRejectsUnsafeTargets(t *testing.T) {
    cases := []string{
        "https://api.openai.com/v1",
        "postgresql://new_api:secret@example.com:5432/new_api",
        "redis://10.0.0.2:6379/0",
        "sk-loadtest-subscription",
        "",
        "http://192.168.1.10:19080",
        "http://service.internal:19080",
        "postgresql://new_api:secret@10.0.0.2:5432/new_api_loadtest",
        "postgresql://new_api:secret@127.0.0.1:5432/new_api",
        "redis://example.com:6379/0",
    }
    for _, value := range cases {
        if err := ValidateAny(value); err == nil { t.Fatalf("unsafe accepted: %s", value) }
    }
}

func TestLocalGuardAcceptsLoadtestTargets(t *testing.T) {
    values := []string{
        "http://127.0.0.1:19080",
        "postgresql://new_api_loadtest:loadtest@127.0.0.1:15432/new_api_loadtest?sslmode=disable",
        "redis://127.0.0.1:16379/0",
        "sk-loadtestsub",
        "sk-loadtestcompat",
        "http://localhost:19080",
        "http://[::1]:19080",
        "postgres://new_api_loadtest:loadtest@localhost:15432/new_api_loadtest?sslmode=disable",
    }
    for _, value := range values {
        if err := ValidateAny(value); err != nil { t.Fatalf("safe rejected %s: %v", value, err) }
    }
}
```

- [ ] **步骤 2：运行 localguard 测试确认失败**

```bash
go test ./pkg/loadtest/localguard -run 'TestLocalGuardRejectsUnsafeTargets|TestLocalGuardAcceptsLoadtestTargets' -count=1
```

预期：FAIL。

- [ ] **步骤 3：实现 localguard**

实现 `ValidateURL`、`ValidatePostgresDSN`、`ValidateRedisAddr`、`ValidateAPIKey`、`ValidateListenAddr`、`ValidateAny`。URL host 必须是 loopback IP、`localhost` 或解析后只包含 loopback；空 URL、公网 IP、私网非 loopback IP、解析后非 loopback 域名都必须失败。PostgreSQL DSN 只接受 `postgresql://` 或 `postgres://`，host 必须 loopback，database 名必须包含 `loadtest`。`LOG_SQL_DSN` 非空时必须与 `SQL_DSN` 同等校验。Redis 地址必须 loopback。listen addr 的 host 必须是 loopback，空 host、`0.0.0.0`、私网非 loopback 必须失败。API key 只接受 `sk-loadtestsub`、`sk-loadtestcompat`、`sk-loadtestinvalid`。

- [ ] **步骤 4：编写 config 测试**

```go
func TestLoadValidateAndWriteEnv(t *testing.T) {
    cfg := writeValidConfig(t)
    file, err := Load(cfg)
    if err != nil { t.Fatal(err) }
    if err := file.Validate(); err != nil { t.Fatal(err) }
    env := file.NewAPIEnv()
    for _, key := range []string{"HOST", "PORT", "PPROF_ADDR", "SQL_DSN", "LOG_SQL_DSN", "REDIS_CONN_STRING", "ENABLE_PPROF", "LOADTEST_RUNTIME_STATS_ENABLED", "LOADTEST_PROFILE_BLOCK_RATE", "LOADTEST_PROFILE_MUTEX_FRACTION", "CHANNEL_UPSTREAM_MODEL_UPDATE_TASK_ENABLED", "CHANNEL_UPDATE_FREQUENCY", "UPDATE_TASK", "CHANNEL_TEST_FREQUENCY", "PYROSCOPE_URL", "SYNC_UPSTREAM_BASE", "RetryTimes", "AutomaticRetryStatusCodes"} {
        if _, ok := env[key]; !ok { t.Fatalf("missing env %s", key) }
    }
    if env["RetryTimes"] != "0" || env["AutomaticRetryStatusCodes"] != "" { t.Fatalf("retry not disabled: %#v", env) }
    rc, err := file.BaseRunContext("abcdef0")
    if err != nil { t.Fatal(err) }
    if rc.ComparisonConfigHash == "" || rc.MockHash != "" || rc.SeedOutputHash != "" { t.Fatalf("bad base context: %#v", rc) }
    for _, profile := range []string{"s1-smoke", "s2-short-stream", "s3-long-stream", "s4-error-refund", "s5-large-payload"} {
        if _, ok := file.MockProfiles[profile]; !ok { t.Fatalf("missing mock profile %s", profile) }
        if file.MockProfileHash(profile) == "" { t.Fatalf("missing mock hash for %s", profile) }
    }
}

func TestDeterministicErrorCountsMatchesStatusHelper(t *testing.T) {
    rate := map[int]float64{429: 0.05, 502: 0.01}
    got429, got502 := DeterministicErrorCounts(1, 100, rate)
    var want429, want502 int
    for attempt := int64(1); attempt <= 100; attempt++ {
        switch {
        case ShouldInjectStatus(1, attempt, 429, rate[429]):
            want429++
        case ShouldInjectStatus(1, attempt, 502, rate[502]):
            want502++
        }
    }
    if got429 != want429 || got502 != want502 { t.Fatalf("counts = %d/%d want %d/%d", got429, got502, want429, want502) }
}

func TestConfigRejectsUnsafeValues(t *testing.T) {
    for _, mutate := range []func(*File){
        func(f *File) { f.Postgres.DSN = "host=127.0.0.1 dbname=new_api_loadtest" },
        func(f *File) { f.Redis.Addr = "redis://10.0.0.2:6379/0" },
        func(f *File) { f.MockUpstream.BaseURL = "https://api.openai.com" },
        func(f *File) { f.Server.Host = "0.0.0.0" },
        func(f *File) { f.Server.PprofAddr = "0.0.0.0:8005" },
        func(f *File) { f.Loadtest.SubscriptionKey = "sk-loadtest-subscription" },
        func(f *File) { f.Retry.RetryTimes = 1 },
        func(f *File) { f.Retry.AutomaticRetryStatusCodes = []int{429} },
        func(f *File) { f.LogPostgres.DSN = "postgresql://new_api:secret@example.com:5432/new_api_loadtest" },
    } {
        f := validFile()
        mutate(&f)
        if err := f.Validate(); err == nil { t.Fatalf("unsafe config accepted: %#v", f) }
    }
}
```

- [ ] **步骤 5：运行 config 测试确认失败**

```bash
go test ./pkg/loadtest/config -run 'TestLoadValidateAndWriteEnv|TestConfigRejectsUnsafeValues|TestDeterministicErrorCountsMatchesStatusHelper' -count=1
```

预期：FAIL。

- [ ] **步骤 6：实现 config 与命令**

如仓库尚无 YAML 依赖，添加 `gopkg.in/yaml.v3`。`NewAPIEnv()` 至少写出：

```text
HOST=127.0.0.1
PORT=13080
PPROF_ADDR=127.0.0.1:8005
ENABLE_PPROF=true
LOADTEST_RUNTIME_STATS_ENABLED=true
LOADTEST_PROFILE_BLOCK_RATE=1000
LOADTEST_PROFILE_MUTEX_FRACTION=5
SQL_DSN=postgresql://...
LOG_SQL_DSN=
REDIS_CONN_STRING=redis://127.0.0.1:16379/0
GOMAXPROCS=2
GOGC=100
BATCH_UPDATE_ENABLED=true
SQL_MAX_OPEN_CONNS=10
SQL_MAX_IDLE_CONNS=5
SQL_MAX_LIFETIME=60
CHANNEL_UPSTREAM_MODEL_UPDATE_TASK_ENABLED=false
CHANNEL_UPDATE_FREQUENCY=0
UPDATE_TASK=false
CHANNEL_TEST_FREQUENCY=0
PYROSCOPE_URL=
SYNC_UPSTREAM_BASE=
RetryTimes=0
AutomaticRetryStatusCodes=
```

新增根目录 `config.loadtest.yaml`，内容必须只包含 loopback PostgreSQL/Redis/mock URL、固定 API key、`mock_profiles`、retry=0 和 thresholds；它是 SOP 复制到 `.loadtest/config/config.yaml` 的源文件。

`pkg/loadtest/config` 必须导出并测试 `ShouldInjectStatus(seed int64, attempt int64, status int, rate float64) bool` 和 `DeterministicErrorCounts(seed int64, total int, statusRate map[int]float64) (actual429 int, actual502 int)`；mock 与 sweep 必须复用这两个 helper，不得重复实现错误注入算法。

`cmd/loadtest-check-config` 暴露 `Run(args []string, stdout io.Writer, stderr io.Writer) int`，`main()` 只调用 `os.Exit(Run(...))`。参数：`--config`、`--out-env`、`--out-run-context`、`--role`、`--commit`。`--role` 未传时默认 `baseline`；`--commit` 未传时自动读取当前 git HEAD。安全/配置失败返回 `2`，文件系统等运行时失败返回 `1`，stderr 必须用 `artifact.Redact` 脱敏。
配置 schema 必须包含 `mock_profiles`，至少定义 `s1-smoke`、`s2-short-stream`、`s3-long-stream`、`s4-error-refund`、`s5-large-payload`。每个 profile 至少包含 `first_token_delay`、`stream_duration`、`chunk_interval`、`output_bytes`、`prompt_tokens`、`completion_tokens`、`status_rate`、`seed`。`s1-smoke` 的 profile 必须与任务 10 smoke 中 mock 参数完全一致：`50ms/500ms/50ms/128/11/17/status-rate 429=0,502=0/seed=1`。`MockProfileHash(profile)` 的规范化输入就是这些字段；`comparison_config_hash` 必须包含所有 mock profiles。

- [ ] **步骤 7：运行 localguard/config 测试通过**

```bash
go test ./pkg/loadtest/localguard ./pkg/loadtest/config -count=1
```

预期：PASS。

---

## 任务 3：mock OpenAI upstream

**文件：**

- 创建：`pkg/loadtest/mockopenai/mockopenai.go`
- 创建：`pkg/loadtest/mockopenai/mockopenai_test.go`
- 创建：`cmd/loadtest-mock-openai/main.go`

### 目标

实现 loopback-only mock upstream，覆盖 `/v1/models`、`/v1/chat/completions`、`/v1/responses`、错误注入和 stats artifact。

### 依赖

任务 1、任务 2 必须完成后才能开始本任务；mock 必须复用任务 2 的 localguard、`MockProfileHash`、`ShouldInjectStatus` 和 `DeterministicErrorCounts`，不得重复定义 hash 或错误注入算法。

### 步骤

- [ ] **步骤 1：编写 SSE fixture 和 stats 测试**

```go
func TestResponsesSSEContractAndUsage(t *testing.T) {
    srv := NewServer(Config{RunContext: testRunContext(), FirstTokenDelay: time.Millisecond, StreamDuration: 10*time.Millisecond, ChunkInterval: time.Millisecond, OutputBytes: 12, PromptTokens: 11, CompletionTokens: 17})
    rr := httptest.NewRecorder()
    req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-5.5","stream":true,"input":"hello"}`))
    srv.ServeHTTP(rr, req)
    body := rr.Body.String()
    if rr.Header().Get("X-Oneapi-Request-Id") != "upstream-loadtest-1" { t.Fatalf("missing upstream id") }
    for _, want := range []string{"event: response.created", "event: response.output_text.delta", "event: response.completed", "\"input_tokens\":11", "data: [DONE]"} {
        if !strings.Contains(body, want) { t.Fatalf("missing %q in %s", want, body) }
    }
}

func TestChatCompletionsSSEContractAndErrorInjection(t *testing.T) {
    srv := NewServer(Config{RunContext: testRunContext(), StatusRate: map[int]float64{429: 1}, Seed: 1, PromptTokens: 11, CompletionTokens: 17})
    rr := httptest.NewRecorder()
    req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-5.5","stream":true,"messages":[{"role":"user","content":"hello"}]}`))
    srv.ServeHTTP(rr, req)
    if rr.Code != http.StatusTooManyRequests { t.Fatalf("status = %d", rr.Code) }
    if rr.Header().Get("X-Oneapi-Request-Id") == "" { t.Fatal("missing upstream id on error") }
}

func TestMockStatsIncludesRunContext(t *testing.T) {
    stats := MockStatsForTest(testRunContext())
    b, err := artifact.MarshalCanonical(stats)
    if err != nil { t.Fatal(err) }
    if !strings.Contains(string(b), "run_context") { t.Fatalf("missing run_context: %s", b) }
}
```

- [ ] **步骤 2：运行 mock 测试确认失败**

```bash
go test ./pkg/loadtest/mockopenai -run 'TestResponsesSSEContractAndUsage|TestChatCompletionsSSEContractAndErrorInjection|TestMockStatsIncludesRunContext' -count=1
```

预期：FAIL。

- [ ] **步骤 3：实现 mockopenai 与命令**

`Run` 参数：`--addr`、`--run-context`、`--first-token-delay`、`--stream-duration`、`--chunk-interval`、`--output-bytes`、`--prompt-tokens`、`--completion-tokens`、`--status-rate`、`--seed`、`--stats-out`。

- `Run` 必须遵守统一命令契约：安全/配置失败返回 `2`，运行时失败返回 `1`，stderr 通过 `artifact.Redact` 脱敏。
- stats artifact 必须写入所选 mock profile hash；如果 `run_context.mock_hash` 与当前参数计算出的 profile hash 不一致，启动失败并返回 `2`。
- mock 必须提供 loopback-only stats snapshot 能力：至少支持原子刷新 `--stats-out` 文件，或提供只监听 loopback 的 `/debug/loadtest/mock-stats` endpoint。sweep 依赖该能力复制 point before/after 并计算 delta。
要求：

- `--addr` 必须 loopback。
- 成功和错误都设置 `X-Oneapi-Request-Id: upstream-loadtest-<attempt>`。
- stats artifact 顶层包含 `run_context`。
- `status-rate` 只作用于主请求，不影响 `/v1/models`。
- 错误注入由 request attempt 和 seed 决定，必须稳定；算法固定为对 `fmt.Sprintf("%d:%d:%d", seed, attempt, status)` 计算 SHA-256，取前 8 字节 big-endian 后 `% 10000`，低于 `status_rate[status] * 10000` 时注入该 status。不同 status 按数值升序判定，避免重叠。

- [ ] **步骤 4：运行 mock 测试通过**

```bash
go test ./pkg/loadtest/mockopenai -count=1
```

预期：PASS。

---

## 任务 4：生产服务 loopback/pprof/runtime route

**文件：**

- 修改：`main.go`
- 修改：`router/main.go`
- 创建：`controller/loadtest_runtime.go`
- 创建：`controller/loadtest_runtime_test.go`

### 目标

让本地压测实例可绑定 loopback、pprof 可绑定 loopback，并暴露仅 loadtest/loopback 可用的 runtime stats route。

### 步骤

- [ ] **步骤 1：编写服务地址和 runtime route 测试**

在合适测试文件中添加：

```go
func TestServerListenAddr(t *testing.T) {
    t.Setenv("HOST", "127.0.0.1")
    t.Setenv("PORT", "13080")
    if got := serverListenAddr(); got != "127.0.0.1:13080" { t.Fatalf("addr = %q", got) }
}

func TestPprofListenAddr(t *testing.T) {
    t.Setenv("PPROF_ADDR", "127.0.0.1:8005")
    if got := pprofListenAddr(); got != "127.0.0.1:8005" { t.Fatalf("addr = %q", got) }
}
```

在 `controller/loadtest_runtime_test.go` 添加：

```go
func TestLoadtestRuntimeRouteDisabledByDefault(t *testing.T) {
    t.Setenv("LOADTEST_RUNTIME_STATS_ENABLED", "")
    r := gin.New()
    RegisterLoadtestRuntimeRoute(r, "127.0.0.1:13080")
    rr := httptest.NewRecorder()
    req := httptest.NewRequest(http.MethodGet, "/debug/loadtest/runtime", nil)
    req.RemoteAddr = "127.0.0.1:12345"
    r.ServeHTTP(rr, req)
    if rr.Code != http.StatusNotFound { t.Fatalf("status = %d", rr.Code) }
}

func TestLoadtestRuntimeRouteRequiresLoopback(t *testing.T) {
    t.Setenv("LOADTEST_RUNTIME_STATS_ENABLED", "true")
    r := gin.New()
    RegisterLoadtestRuntimeRoute(r, "127.0.0.1:13080")
    rr := httptest.NewRecorder()
    req := httptest.NewRequest(http.MethodGet, "/debug/loadtest/runtime", nil)
    req.RemoteAddr = "10.0.0.2:12345"
    r.ServeHTTP(rr, req)
    if rr.Code != http.StatusForbidden && rr.Code != http.StatusNotFound { t.Fatalf("status = %d", rr.Code) }
}

func TestLoadtestRuntimeRouteNotRegisteredWhenServerNotLoopback(t *testing.T) {
    t.Setenv("LOADTEST_RUNTIME_STATS_ENABLED", "true")
    for _, listenAddr := range []string{":13080", "0.0.0.0:13080", "10.0.0.2:13080"} {
        r := gin.New()
        RegisterLoadtestRuntimeRoute(r, listenAddr)
        rr := httptest.NewRecorder()
        req := httptest.NewRequest(http.MethodGet, "/debug/loadtest/runtime", nil)
        req.RemoteAddr = "127.0.0.1:12345"
        r.ServeHTTP(rr, req)
        if rr.Code != http.StatusNotFound { t.Fatalf("listen addr %q status = %d", listenAddr, rr.Code) }
    }
}

func TestLoadtestRuntimeRouteReturnsRuntimeFields(t *testing.T) {
    t.Setenv("LOADTEST_RUNTIME_STATS_ENABLED", "true")
    t.Setenv("LOADTEST_PROFILE_BLOCK_RATE", "1000")
    t.Setenv("LOADTEST_PROFILE_MUTEX_FRACTION", "5")
    r := gin.New()
    RegisterLoadtestRuntimeRoute(r, "127.0.0.1:13080")
    rr := httptest.NewRecorder()
    req := httptest.NewRequest(http.MethodGet, "/debug/loadtest/runtime", nil)
    req.RemoteAddr = "127.0.0.1:12345"
    r.ServeHTTP(rr, req)
    if rr.Code != http.StatusOK { t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String()) }
    for _, want := range []string{"goroutines", "heap_alloc_bytes", "block_profile_rate", "mutex_profile_fraction", "batch_update", "quota_data", "perf_metrics", "unavailable"} {
        if !strings.Contains(rr.Body.String(), want) { t.Fatalf("missing %s in %s", want, rr.Body.String()) }
    }
}
```

- [ ] **步骤 2：运行生产服务测试确认失败**

```bash
go test . -run 'TestServerListenAddr|TestPprofListenAddr' -count=1
go test ./controller -run TestLoadtestRuntime -count=1
```

预期：FAIL。

- [ ] **步骤 3：实现生产服务最小改动**

- `serverListenAddr()`：默认保持原行为 `":" + port`；当 `HOST` 非空时返回 `HOST + ":" + port`。
- `pprofListenAddr()`：默认保持原 `0.0.0.0:8005`；当 `PPROF_ADDR` 非空时使用它。
- 仅当 `LOADTEST_RUNTIME_STATS_ENABLED=true` 且生产注册点传入的实际 listen addr 明确为 loopback 时注册 `/debug/loadtest/runtime`；`":13080"`、`0.0.0.0:13080` 或非 loopback listen addr 时不注册。`router.SetRouter` 调用注册函数时必须传入 `serverListenAddr()` 或等价的实际监听地址，禁止无参注册路径绕过 loopback 判定。
- route 必须拒绝非 loopback remote address。
- route JSON 包含 goroutines、heap、GC、block/mutex profile rate、`batch_update`、`quota_data`、`perf_metrics`。`quota_data` 或 `perf_metrics` 尚无 pending/flush 能力时必须返回 `{status:"unavailable", reason:"..."}`，不得省略或用 0 伪装。
- `LOADTEST_PROFILE_BLOCK_RATE`、`LOADTEST_PROFILE_MUTEX_FRACTION` 只在 runtime stats enabled 时应用。
- `model/utils.go` 必须提供 batch update pending 的只读快照 helper，供 runtime route 返回 `batch_update` 字段。

- [ ] **步骤 4：运行生产服务测试通过**

```bash
go test . -run 'TestServerListenAddr|TestPprofListenAddr' -count=1
go test ./controller -run TestLoadtestRuntime -count=1
```

预期：PASS。

---

## 任务 5：client 与 summary

**文件：**

- 创建：`pkg/loadtest/client/client.go`
- 创建：`pkg/loadtest/client/client_test.go`
- 创建：`cmd/loadtest-client/main.go`

### 目标

实现压测 client，生成包含 `new_api_request_id`、stream、usage、latency、TTFT 的 summary。

### 依赖

任务 1、任务 2、任务 3 必须完成后才能开始本任务。

### 步骤

- [ ] **步骤 1：编写 parser 与 token profile 测试**

```go
func TestParseResponsesSSEExtractsUsageAndDone(t *testing.T) {
    body := strings.NewReader("event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"}\n\nevent: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":11,\"output_tokens\":17,\"total_tokens\":28}}}\n\ndata: [DONE]\n\n")
    rec, err := ParseResponsesStream(body)
    if err != nil { t.Fatal(err) }
    if !rec.DoneReceived || rec.Usage.TotalTokens != 28 { t.Fatalf("bad record: %#v", rec) }
}

func TestTokenProfileMustMatchAPIKey(t *testing.T) {
    if err := ValidateTokenProfile("sk-loadtestsub", "subscription"); err != nil { t.Fatal(err) }
    if err := ValidateTokenProfile("sk-loadtestsub", "compat"); err == nil { t.Fatal("mismatch accepted") }
}

func TestSummaryUsesNewAPIRequestIDHeader(t *testing.T) {
    rr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("X-Oneapi-Request-Id", "rid-new-api")
        w.Header().Set("X-Upstream-Request-Id", "upstream-loadtest-1")
        _, _ = w.Write([]byte("data: [DONE]\n\n"))
    }))
    defer rr.Close()
    summary, err := RunOnceForTest(rr.URL, "sk-loadtestsub", "subscription")
    if err != nil { t.Fatal(err) }
    if summary.Requests[0].NewAPIRequestID != "rid-new-api" { t.Fatalf("bad request id: %#v", summary.Requests[0]) }
}

func TestHealthCheckCoversStatusRuntimePprofModelsAndInvalidToken(t *testing.T) {
    srv := newHealthCheckTestServer(t)
    result, err := HealthCheck(context.Background(), HealthCheckOptions{BaseURL: srv.URL, ValidAPIKey: "sk-loadtestsub", InvalidAPIKey: "sk-loadtestinvalid", RuntimeURL: srv.URL + "/debug/loadtest/runtime", PprofURL: srv.URL + "/debug/pprof/goroutine?debug=1"})
    if err != nil { t.Fatal(err) }
    for _, check := range []string{"api_status", "runtime_stats", "pprof_goroutine", "models_valid_token", "invalid_token_rejected"} {
        if result.Checks[check].Status != "passed" { t.Fatalf("%s not passed: %#v", check, result.Checks[check]) }
    }
}
```

- [ ] **步骤 2：运行 client 测试确认失败**

```bash
go test ./pkg/loadtest/client -run 'TestParseResponsesSSEExtractsUsageAndDone|TestTokenProfileMustMatchAPIKey|TestSummaryUsesNewAPIRequestIDHeader|TestHealthCheckCoversStatusRuntimePprofModelsAndInvalidToken' -count=1
```

预期：FAIL。

- [ ] **步骤 3：实现 client 与命令**

参数：`--url`、`--api-key`、`--token-profile`、`--path`、`--model`、`--scenario`、`--concurrency`、`--rps`、`--duration`、`--max-requests`、`--ramp-step`、`--ramp-interval`、`--timeout`、`--input-bytes`、`--stream`、`--run-context`、`--out`、`--health-check`、`--valid-api-key`、`--invalid-api-key`、`--runtime-url`、`--pprof-url`。

要求：

- URL 必须 loopback。
- `--token-profile` 必须与 API key 固定映射匹配。
- summary 顶层包含 `run_context`。
- `new_api_request_id` 从响应头 `X-Oneapi-Request-Id` 提取。
- 错误响应也记录 request id、status code 和 error reason。
- `rps=0` 表示不限速。
- `Run` 必须遵守统一命令契约。
- `--health-check` 模式不发压测流量，只执行 S0：`/api/status`、`/debug/loadtest/runtime`、pprof goroutine、有效 token `/v1/models`、invalid token 401，并输出 `s0-health.json`。

- [ ] **步骤 4：运行 client 测试通过**

```bash
go test ./pkg/loadtest/client -count=1
```

预期：PASS。

---

## 任务 6：seed 数据与 route 隔离

**文件：**

- 创建：`pkg/loadtest/seed/seed.go`
- 创建：`pkg/loadtest/seed/seed_test.go`
- 创建：`cmd/loadtest-seed/main.go`

### 目标

幂等写入 loadtest 用户、token、channel、ability、subscription 和 option，保证 `default/gpt-5.5` 只路由到 loopback mock。

### 依赖

任务 1、任务 2 必须完成后才能开始本任务。

### 步骤

- [ ] **步骤 1：编写 seed 测试**

```go
func TestSeedIsIdempotentAndCreatesBillingObjects(t *testing.T) {
    db := openSeedTestDB(t)
    cfg := Config{Model: "gpt-5.5", Group: "default", MockBaseURL: "http://127.0.0.1:19080", SubscriptionKey: "sk-loadtestsub", CompatKey: "sk-loadtestcompat"}
    first, err := Apply(context.Background(), db, cfg); if err != nil { t.Fatal(err) }
    second, err := Apply(context.Background(), db, cfg); if err != nil { t.Fatal(err) }
    if first.TokenDBKeySubscription != "loadtestsub" || second.TokenDBKeySubscription != "loadtestsub" { t.Fatalf("bad token key") }
    assertCount(t, db, &model.Token{}, 2)
    assertCount(t, db, &model.UserSubscription{}, 2)
    assertOptionJSONContains(t, db, "ModelRatio", "gpt-5.5")
    assertOptionEnabled(t, db, "LogConsumeEnabled", true)
    assertOptionEnabled(t, db, "DataExportEnabled", true)
    assertOptionValue(t, db, "RetryTimes", "0")
    assertOptionValue(t, db, "AutomaticRetryStatusCodes", "")
    assertPerfMetricsEnabled(t, db)
    assertSubscriptionConcurrencyPositive(t, db)
}

func TestSeedDisablesUnsafeChannelsForModelRoute(t *testing.T) {
    db := openSeedTestDB(t)
    unsafeURL := "https://api.openai.com"
    unsafeChannel := model.Channel{Id: 99, Name: "unsafe", Type: constant.ChannelTypeOpenAI, Key: "real", BaseURL: &unsafeURL, Status: common.ChannelStatusEnabled, Models: "gpt-5.5", Group: "default"}
    if err := db.Create(&unsafeChannel).Error; err != nil { t.Fatal(err) }
    priority := int64(999)
    if err := db.Create(&model.Ability{Group: "default", Model: "gpt-5.5", ChannelId: unsafeChannel.Id, Enabled: true, Priority: &priority, Weight: 100}).Error; err != nil { t.Fatal(err) }
    _, err := Apply(context.Background(), db, Config{Model: "gpt-5.5", Group: "default", MockBaseURL: "http://127.0.0.1:19080", SubscriptionKey: "sk-loadtestsub", CompatKey: "sk-loadtestcompat"})
    if err != nil { t.Fatal(err) }
    var reloaded model.Channel
    if err := db.First(&reloaded, unsafeChannel.Id).Error; err != nil { t.Fatal(err) }
    if reloaded.Status == common.ChannelStatusEnabled && strings.Contains(reloaded.Models, "gpt-5.5") && strings.Contains(reloaded.Group, "default") { t.Fatalf("unsafe channel still routable: %#v", reloaded) }
}

func TestSeedOutputHashAndRunContext(t *testing.T) {
    out := SeedOutputForTest()
    if out.RunContext.SeedOutputHash != "" { t.Fatalf("seed output self hash present: %#v", out.RunContext) }
    hash, err := artifact.HashSeedOutput(out)
    if err != nil || hash == "" { t.Fatalf("hash=%q err=%v", hash, err) }
}
```

- [ ] **步骤 2：运行 seed 测试确认失败**

```bash
go test ./pkg/loadtest/seed -run 'TestSeedIsIdempotentAndCreatesBillingObjects|TestSeedDisablesUnsafeChannelsForModelRoute|TestSeedOutputHashAndRunContext' -count=1
```

预期：FAIL。

- [ ] **步骤 3：实现 seed 与命令**

参数：`--config`、`--run-context`、`--out`、`--out-run-context`。

要求：

- 写入两个用户：`loadtest_subscription`、`loadtest_compat`。
- 写入两个 token，DB key 为 `loadtestsub`、`loadtestcompat`。
- 两个用户都有有效订阅；compat 用户 wallet/user quota/token remain 只用于观测，不得作为 relay 扣费目标。
- 写入 loopback loadtest channel 和 ability。
- 禁用或移除非 loadtest channel 的 `default/gpt-5.5` 路由字段；只禁用 ability 不够。
- 写入 ratio option：`ModelRatio`、`CompletionRatio`、`GroupRatio`。
- 写入 option：`LogConsumeEnabled=true`、`DataExportEnabled=true`、`perf_metrics_setting.enabled=true`、`RetryTimes=0`、`AutomaticRetryStatusCodes=""`。
- 不关闭订阅并发队列。
- `seed.json` 使用 base context（无 seed hash），`run-context.seeded.json` 再写入 seed hash。
- `Run` 必须遵守统一命令契约。

- [ ] **步骤 4：运行 seed 测试通过**

```bash
go test ./pkg/loadtest/seed -count=1
```

预期：PASS。

---

## 任务 7：metrics collector 与业务 diff

**文件：**

- 创建：`pkg/loadtest/metrics/metrics.go`
- 创建：`pkg/loadtest/metrics/metrics_test.go`
- 创建：`cmd/loadtest-collect/main.go`

### 目标

生成 `Snapshot` 和 `Diff`，按 `new_api_request_id` join，校验 seed/mock/hash，正确传播 unavailable。

### 依赖

任务 1、任务 2、任务 4、任务 5、任务 6 必须完成后才能开始本任务。

### 步骤

- [ ] **步骤 1：编写 diff、hash 和日志测试**

```go
func TestBuildDiffRequiresSeedAndMockContext(t *testing.T) {
    rc := testRunContext()
    before := artifact.Snapshot{SchemaVersion: 1, RunContext: rc, Process: artifact.ProcessSnapshot{Statused: artifact.Statused{Status: "ok"}, RSSBytes: 100}}
    after := artifact.Snapshot{SchemaVersion: 1, RunContext: rc, Process: artifact.ProcessSnapshot{Statused: artifact.Statused{Status: "ok"}, RSSBytes: 130}}
    summary := artifact.Summary{RunContext: rc, Total: 100, Success: 94, Requests: []artifact.RequestRecord{{NewAPIRequestID: "rid-1", StatusCode: 200, Success: true, TotalTokens: 28}}}
    seed := artifact.SeedOutput{SchemaVersion: 1, RunContext: rc.WithoutSeedOutputHash().WithoutMockHash(), ExpectedUsagePerSuccess: artifact.Usage{PromptTokens: 11, CompletionTokens: 17, TotalTokens: 28}}
    seedHash, _ := artifact.HashSeedOutput(seed)
    rc.SeedOutputHash = seedHash
    seed.RunContext = rc.WithoutSeedOutputHash().WithoutMockHash()
    mock := artifact.MockStatsDelta{SchemaVersion: 1, RunContext: rc, Path: "c100-mock-stats-delta.json", Hash: "sha256:mockdelta", Actual429: 5, Actual502: 1, UpstreamAttemptsTotal: 100}
    diff, inv := BuildDiff(DiffInputs{Before: before, After: after, Summary: summary, SeedOutput: seed, MockDelta: mock, RunContext: rc})
    if inv.Status != "passed" { t.Fatalf("invariant failed: %#v", inv) }
    if diff.MockStatsDeltaPath == "" || diff.MockDelta.UpstreamAttemptsTotal != 100 { t.Fatalf("missing mock delta: %#v", diff) }
}

func TestBuildDiffFailsOnRunContextMismatch(t *testing.T) {
    rc := testRunContext()
    other := rc
    other.Scenario = "s3-long-stream"
    before := artifact.Snapshot{SchemaVersion: 1, RunContext: rc}
    after := artifact.Snapshot{SchemaVersion: 1, RunContext: rc}
    summary := artifact.Summary{RunContext: rc}
    seed := artifact.SeedOutput{SchemaVersion: 1, RunContext: rc.WithoutSeedOutputHash().WithoutMockHash()}
    seedHash, _ := artifact.HashSeedOutput(seed)
    rc.SeedOutputHash = seedHash
    seed.RunContext = rc.WithoutSeedOutputHash().WithoutMockHash()
    mock := artifact.MockStatsDelta{SchemaVersion: 1, RunContext: other}
    _, inv := BuildDiff(DiffInputs{Before: before, After: after, Summary: summary, SeedOutput: seed, MockDelta: mock, RunContext: rc})
    if inv.Status != "failed" { t.Fatalf("context mismatch should fail: %#v", inv) }
}

func TestBuildDiffFailsOnSeedHashMismatch(t *testing.T) {
    rc := testRunContext()
    rc.SeedOutputHash = "sha256:wrong"
    _, inv := BuildDiff(DiffInputs{RunContext: rc, SeedOutput: artifact.SeedOutput{RunContext: rc.WithoutSeedOutputHash().WithoutMockHash()}})
    if inv.Status != "failed" { t.Fatalf("want failed: %#v", inv) }
}

func TestBuildChargesByRequestJoinsOnlyNewAPIRequestID(t *testing.T) {
    summary := artifact.Summary{Requests: []artifact.RequestRecord{{ClientRequestID: "client-1", NewAPIRequestID: "rid-1", UpstreamRequestID: "upstream-1", StatusCode: 200, Success: true}}}
    logs := []ConsumeLogRow{{RequestID: "rid-1", Quota: 28}}
    records := []PreConsumeRow{{RequestID: "rid-1", Status: "settled", PreConsumed: 28}}
    charges, inv := BuildChargesByRequest(summary, logs, records)
    if inv.Status != "passed" || charges[0].NewAPIRequestID != "rid-1" { t.Fatalf("bad join: %#v %#v", charges, inv) }
    wrongLogs := []ConsumeLogRow{{RequestID: "client-1", Quota: 28}}
    _, inv = BuildChargesByRequest(summary, wrongLogs, records)
    if inv.Status != "failed" { t.Fatalf("wrong id join should fail: %#v", inv) }
}

func TestServerLogScanningUsesStdoutAndStderr(t *testing.T) {
    got := ScanServerLogs("record consume log: userId=1", "failed to flush perf metric bucket: column reference")
    if got.StdoutFullParamsLines != 0 || got.PerfMetricUpsertErrors != 1 { t.Fatalf("bad log scan: %#v", got) }
    got = ScanServerLogs("record consume log: userId=1, params={large_payload", "")
    if got.StdoutFullParamsLines != 1 { t.Fatalf("missing stdout params detection: %#v", got) }
}
```

- [ ] **步骤 2：运行 metrics 测试确认失败**

```bash
go test ./pkg/loadtest/metrics -run 'TestBuildDiffRequiresSeedAndMockContext|TestBuildDiffFailsOnSeedHashMismatch|TestBuildDiffFailsOnRunContextMismatch|TestBuildChargesByRequestJoinsOnlyNewAPIRequestID|TestServerLogScanningUsesStdoutAndStderr' -count=1
```

预期：FAIL。

- [ ] **步骤 3：实现 collector 与命令**

参数：`--config`、`--pid-file`、`--run-context`、`--seed-output`、`--summary`、`--before`、`--out-snapshot`、`--out-diff`、`--mock-stats-delta`、`--stdout-log`、`--stderr-log`、`--wait-drain`。

要求：

- PostgreSQL raw SQL 只存在于本包。
- 采集 `pg_stat_statements_top_total_time`；扩展不可用时标注 unavailable 或空 rows 并附 reason，不作为第一阶段硬门禁。
- 校验 `--seed-output` hash 与 `run_context.seed_output_hash`。
- `BuildDiff` 必须校验 before、after、summary、mock delta 的 `run_context` 全部等于 `--run-context`，且输出 diff 自身的 `run_context` 也必须等于 `--run-context`；任一不一致返回业务 gate 失败或 exit code `2`。
- 业务不变量至少包含：`subscription_token_used_matches_success_usage`、`compat_subscription_token_used_matches_success_usage`、`compat_wallet_not_charged`、`consume_logs_by_request`、`failure_refund_by_request`、`quota_data_pending_or_unavailable`、`perf_metrics_no_upsert_error`、`stdout_no_full_params`。
- 必需 invariant 缺失、`failed` 或不允许的 `unavailable` 都必须让 gate 失败。
- profile/resource 采集归 metrics 包实现：输入 pprof addr、profile 输出目录、采样间隔和采样窗口；输出 CPU/heap/goroutine/block/mutex profile 路径、during RSS/goroutine/heap peaks。pprof 不可用时相关字段必须为 `unavailable` 并携带 reason，不得写空路径伪装成功。
- collector 必须读取 runtime route 的 `batch_update`、`quota_data`、`perf_metrics` 状态；`quota_data_pending_or_unavailable` 只在 pending 可读且持续增长时失败，在来源不可用时输出 allowed unavailable。
- `Run` 必须遵守统一命令契约。

- [ ] **步骤 4：运行 metrics 测试通过**

```bash
go test ./pkg/loadtest/metrics -count=1
```

预期：PASS。

---

## 任务 8：concurrency sweep 与场景 gate

**文件：**

- 创建：`pkg/loadtest/sweep/sweep.go`
- 创建：`pkg/loadtest/sweep/sweep_test.go`
- 创建：`cmd/loadtest-concurrency-sweep/main.go`

### 目标

派生场景 run context，按并发点编排 client/collector，输出独立 point artifact 和 gate。

### 依赖

任务 1、任务 2、任务 5、任务 7 必须完成后才能开始本任务。

### 步骤

- [ ] **步骤 1：编写 gate 和 context 测试**

```go
func TestDeriveRunContextRequiresTokenProfileAndSeed(t *testing.T) {
    base := artifact.RunContext{SchemaVersion: 1, Commit: "abcdef0", ComparisonConfigHash: "sha256:cfg", SeedOutputHash: "sha256:seed", Model: "gpt-5.5"}
    got, err := DeriveRunContext(base, DeriveOptions{Scenario: "s2-short-stream", Path: "/v1/responses", TokenProfile: "subscription", APIKey: "sk-loadtestsub", MockHash: "sha256:mock"})
    if err != nil { t.Fatal(err) }
    if got.TokenProfile != "subscription" || got.Path != "/v1/responses" { t.Fatalf("bad context: %#v", got) }
}

func TestBuildMockStatsDeltaUsesPointWindowAndContext(t *testing.T) {
    rc := testRunContext()
    before := artifact.MockStats{SchemaVersion: 1, RunContext: rc, AttemptsTotal: 100, InjectedStatusCounts: map[string]int{"429": 2}}
    after := artifact.MockStats{SchemaVersion: 1, RunContext: rc, AttemptsTotal: 160, InjectedStatusCounts: map[string]int{"429": 5, "502": 1}}
    delta, err := BuildMockStatsDelta(before, after, rc)
    if err != nil { t.Fatal(err) }
    if delta.UpstreamAttemptsTotal != 60 || delta.Actual429 != 3 || delta.Actual502 != 1 || delta.RunContext != rc || delta.Hash == "" { t.Fatalf("bad delta: %#v", delta) }
}

func TestS1S2GateRequiresAllBusinessConditions(t *testing.T) {
    invariants := make([]artifact.Invariant, 0, len(RequiredInvariantNames()))
    for _, name := range RequiredInvariantNames() { invariants = append(invariants, artifact.Invariant{Name: name, Status: "passed"}) }
    point := artifact.PointResult{Concurrency: 100, SummaryExcerpt: artifact.SummaryExcerpt{Total: 1000, Success: 1000, StatusCodes: map[string]int{"200": 1000}, MaxObservedInFlight: 95, StreamDoneReceived: 1000, StreamUsageEvents: 1000, StreamBytes: 128000}, Invariants: invariants}
    gate := EvaluateGate("s2-short-stream", point, GateOptions{MockOutputBytes: 128, RequiredInvariantNames: RequiredInvariantNames()})
    if !gate.Passed { t.Fatalf("gate failed: %#v", gate) }
    point.Invariants = point.Invariants[:1]
    gate = EvaluateGate("s2-short-stream", point, GateOptions{MockOutputBytes: 128, RequiredInvariantNames: RequiredInvariantNames()})
    if gate.Passed { t.Fatalf("missing invariant passed") }
}

func TestS4GateUsesCurrentPointMockDeltaAndRefundInvariant(t *testing.T) {
    point := artifact.PointResult{SummaryExcerpt: artifact.SummaryExcerpt{Total: 100, Actual429: 5, Actual502: 1, UpstreamAttemptsTotal: 100}, MockDelta: artifact.MockStatsDelta{Actual429: 5, Actual502: 1, UpstreamAttemptsTotal: 100}, Invariants: []artifact.Invariant{{Name: "failure_refund_by_request", Status: "passed"}, {Name: "compat_wallet_not_charged", Status: "passed"}}}
    gate := EvaluateGate("s4-error-refund", point, GateOptions{RequiredInvariantNames: []string{"failure_refund_by_request", "compat_wallet_not_charged"}})
    if !gate.Passed { t.Fatalf("gate failed: %#v", gate) }
    point.MockDelta.UpstreamAttemptsTotal = 99
    gate = EvaluateGate("s4-error-refund", point, GateOptions{RequiredInvariantNames: []string{"failure_refund_by_request", "compat_wallet_not_charged"}})
    if gate.Passed { t.Fatalf("bad mock delta passed") }
}

func TestS4GateRequiresDeterministicExpectedErrors(t *testing.T) {
    rate := map[int]float64{429: 0.05, 502: 0.01}
    expected429, expected502 := DeterministicErrorCounts(1, 100, rate)
    expectedSuccess := 100 - expected429 - expected502
    point := artifact.PointResult{SummaryExcerpt: artifact.SummaryExcerpt{Total: 100, Success: expectedSuccess, Actual429: expected429, Actual502: expected502, UpstreamAttemptsTotal: 100, NonInjectedErrors: 0}, MockDelta: artifact.MockStatsDelta{Actual429: expected429, Actual502: expected502, UpstreamAttemptsTotal: 100}, Invariants: []artifact.Invariant{{Name: "failure_refund_by_request", Status: "passed"}, {Name: "compat_wallet_not_charged", Status: "passed"}}}
    gate := EvaluateGate("s4-error-refund", point, GateOptions{Seed: 1, StatusRate: rate, RequiredInvariantNames: []string{"failure_refund_by_request", "compat_wallet_not_charged"}})
    if !gate.Passed { t.Fatalf("gate failed: %#v", gate) }
    point.SummaryExcerpt.Success++
    gate = EvaluateGate("s4-error-refund", point, GateOptions{Seed: 1, StatusRate: rate, RequiredInvariantNames: []string{"failure_refund_by_request", "compat_wallet_not_charged"}})
    if gate.Passed { t.Fatalf("wrong deterministic error count passed") }
}

func TestS3S5GateFailsOnResourceGrowth(t *testing.T) {
    point := artifact.PointResult{ResourcePeaks: artifact.ResourcePeaks{RSSPeakBytes: 2 << 30, GoroutinesPeak: 100000}, ResourceDelta: artifact.ResourceDelta{RSSBeforeBytes: 100, RSSAfterDrainBytes: 500, GoroutinesBefore: 10, GoroutinesAfterDrain: 100}}
    gate := EvaluateGate("s5-large-payload", point, GateOptions{MaxRSSBytes: 1 << 30, MaxRSSAfterDrainGrowthBytes: 100, MaxGoroutineAfterDrainGrowth: 20})
    if gate.Passed { t.Fatalf("resource leak gate passed: %#v", gate) }
}
```

- [ ] **步骤 2：运行 sweep 测试确认失败**

```bash
go test ./pkg/loadtest/sweep -run 'TestDeriveRunContextRequiresTokenProfileAndSeed|TestBuildMockStatsDeltaUsesPointWindowAndContext|TestS1S2GateRequiresAllBusinessConditions|TestS4GateUsesCurrentPointMockDeltaAndRefundInvariant|TestS4GateRequiresDeterministicExpectedErrors|TestS3S5GateFailsOnResourceGrowth' -count=1
```

预期：FAIL。

- [ ] **步骤 3：实现 sweep 与命令**

参数：`--derive-run-context-only`、`--config`、`--url`、`--api-key`、`--token-profile`、`--path`、`--model`、`--scenario`、`--points`、`--rps`、`--duration`、`--max-requests-per-point`、`--ramp-step`、`--ramp-interval`、`--timeout`、`--input-bytes`、`--output-bytes`、`--cooldown`、`--pid-file`、`--seed-output`、`--mock-profile`、`--mock-stats`、`--mock-hash`、`--run-context`、`--out-run-context`、`--artifact-dir`、`--out`。

要求：

- `--derive-run-context-only` 只写 scenario context，不运行压测。
- `--derive-run-context-only` 必须读取 `--config` 并按 `--mock-profile` 计算 `MockProfileHash` 写入场景 `run_context.mock_hash`；如果未传 `--config` 且未显式传 `--mock-hash`，必须返回配置错误 `2`。
- 每个 point 写独立 `c100-before.json`、`c100-summary.json`、`c100-mock-stats-before.json`、`c100-mock-stats-after.json`、`c100-mock-stats-delta.json`、`c100-after.json`、`c100-diff.json`。
- during sampler 必须与 client 并发运行。
- S1/S2/S3/S4/S5 gate 必须使用当前 point 的 summary/diff/mock delta/resource peaks。
- 第一阶段默认 SOP 管理 mock 进程；sweep 仍要校验 mock profile/hash，并从 `--mock-stats` 指向的当前 snapshot source 在每个 point 前后原子读取 before/after，调用 `BuildMockStatsDelta(before, after, run_context)` 写出 `c100-mock-stats-delta.json`。delta 的 `run_context` 必须等于当前 point context，`hash` 必须按规范化 delta 计算；S4 gate 只能读取该 point delta，不得读取全局累计 stats。
- S4 gate 必须使用任务 2 `config.DeterministicErrorCounts`，根据 mock profile 的 seed、status-rate 和 point total 计算 `expected_429`、`expected_502`，并要求 `success == total - expected_429 - expected_502`、`non_injected_errors == 0`、`upstream_attempts_total == total`。S3/S5 gate 必须检查 RSS/goroutine/heap 的 peak 与 after-drain 增长阈值。
- sweep 负责调用 metrics 的 during sampler/profile collector，并把 profile paths/resource peaks 写入每个 point artifact；profile unavailable 按 unavailable 传播到 gate/report。
- `Run` 必须遵守统一命令契约。

- [ ] **步骤 4：运行 sweep 测试通过**

```bash
go test ./pkg/loadtest/sweep -count=1
```

预期：PASS。

---

## 任务 9：runner、report 与模板

**文件：**

- 创建：`pkg/loadtest/runner/runner.go`
- 创建：`pkg/loadtest/runner/runner_test.go`
- 创建：`cmd/loadtest-run-new-api/main.go`
- 创建：`pkg/loadtest/report/report.go`
- 创建：`pkg/loadtest/report/report_test.go`
- 创建：`cmd/loadtest-report/main.go`
- 创建：`docs/superpowers/reports/loadtest-report-template.md`
- 创建：`docs/superpowers/reports/2026-05-20-new-api-local-loadtest-sop.md`

### 目标

实现 clean env 启动器、bootstrap-only、stdout/stderr 捕获、报告、regression gate 和可复制的本地 SOP。

### 依赖

runner 部分依赖任务 2；report 部分依赖任务 1 和任务 8。

### 步骤

- [ ] **步骤 1：编写 runner 测试**

```go
func TestBuildCommandUsesCleanAllowlistEnvironment(t *testing.T) {
    hostile := map[string]string{"SQL_DSN": "postgresql://prod:prod@example.com:5432/prod", "LOG_SQL_DSN": "postgresql://prod:prod@example.com:5432/prod", "REDIS_CONN_STRING": "redis://example.com:6379/0", "OPENAI_API_KEY": "sk-real-production", "CHANNEL_TEST_FREQUENCY": "1", "CHANNEL_UPDATE_FREQUENCY": "1", "CHANNEL_UPSTREAM_MODEL_UPDATE_TASK_ENABLED": "true", "PYROSCOPE_URL": "http://example.com/pyroscope", "SYNC_UPSTREAM_BASE": "https://basellm.github.io/llm-metadata"}
    for k, v := range hostile { t.Setenv(k, v) }
    dir := t.TempDir()
    binary := filepath.Join(dir, "new-api")
    if err := os.WriteFile(binary, []byte("placeholder"), 0o700); err != nil { t.Fatal(err) }
    env := map[string]string{"HOST":"127.0.0.1", "SQL_DSN":"postgresql://new_api_loadtest:loadtest@127.0.0.1:15432/new_api_loadtest?sslmode=disable", "LOG_SQL_DSN":"", "REDIS_CONN_STRING":"redis://127.0.0.1:16379/0", "CHANNEL_UPSTREAM_MODEL_UPDATE_TASK_ENABLED":"false", "CHANNEL_UPDATE_FREQUENCY":"0", "UPDATE_TASK":"false", "CHANNEL_TEST_FREQUENCY":"0", "PYROSCOPE_URL":"", "SYNC_UPSTREAM_BASE":"", "RetryTimes":"0", "AutomaticRetryStatusCodes":""}
    cmd, err := BuildCommand(Config{Binary: binary, WorkDir: filepath.Join(dir, "runtime"), Env: env, PIDFile: filepath.Join(dir, "new-api.pid"), StdoutLog: filepath.Join(dir, "stdout.log"), StderrLog: filepath.Join(dir, "stderr.log")})
    if err != nil { t.Fatal(err) }
    if cmd.Dir != filepath.Join(dir, "runtime") { t.Fatalf("cmd.Dir = %q", cmd.Dir) }
    if len(cmd.Env) == 0 { t.Fatal("empty Env would inherit parent") }
    joined := strings.Join(cmd.Env, "\n")
    for _, forbidden := range []string{"OPENAI_API_KEY=", "example.com", "CHANNEL_UPDATE_FREQUENCY=1", "CHANNEL_UPSTREAM_MODEL_UPDATE_TASK_ENABLED=true", "PYROSCOPE_URL=http"} {
        if strings.Contains(joined, forbidden) { t.Fatalf("leaked hostile env %q in %s", forbidden, joined) }
    }
    for _, required := range []string{"CHANNEL_UPSTREAM_MODEL_UPDATE_TASK_ENABLED=false", "CHANNEL_UPDATE_FREQUENCY=0", "UPDATE_TASK=false", "CHANNEL_TEST_FREQUENCY=0", "PYROSCOPE_URL=", "SYNC_UPSTREAM_BASE=", "RetryTimes=0", "AutomaticRetryStatusCodes="} {
        if !strings.Contains(joined, required) { t.Fatalf("missing safe env %q in %s", required, joined) }
    }
}

func TestBuildCommandRejectsUnsafeEnvAndDotEnv(t *testing.T) {
    dir := t.TempDir()
    if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("OPENAI_API_KEY=sk-real"), 0o600); err != nil { t.Fatal(err) }
    env := map[string]string{"HOST":"0.0.0.0", "SQL_DSN":"postgresql://prod:prod@example.com:5432/prod", "REDIS_CONN_STRING":"redis://example.com:6379/0"}
    _, err := BuildCommand(Config{Binary: filepath.Join(dir, "new-api"), WorkDir: dir, Env: env, PIDFile: filepath.Join(dir, "new-api.pid")})
    if err == nil { t.Fatal("unsafe env/workdir accepted") }
}
```

- [ ] **步骤 2：编写 report 测试**

```go
func TestCompareReportRejectsEveryComparableMismatch(t *testing.T) {
    fields := map[string]func(*artifact.RunContext){
        "comparison_config_hash": func(r *artifact.RunContext) { r.ComparisonConfigHash = "sha256:other" },
        "mock_hash": func(r *artifact.RunContext) { r.MockHash = "sha256:different" },
        "cache_mode": func(r *artifact.RunContext) { r.CacheMode = "warm" },
        "scenario": func(r *artifact.RunContext) { r.Scenario = "s3-long-stream" },
        "path": func(r *artifact.RunContext) { r.Path = "/v1/chat/completions" },
        "token_profile": func(r *artifact.RunContext) { r.TokenProfile = "compat" },
    }
    for name, mutate := range fields {
        base := sweepWithContext(testRunContext())
        candidate := sweepWithContext(testRunContext())
        mutate(&candidate.RunContext)
        if _, err := BuildCompareReport(base, candidate, Thresholds{}); err == nil { t.Fatalf("%s mismatch accepted", name) }
    }
}

func TestCompareReportFailsOnRegressionThreshold(t *testing.T) {
    baseline := sweepWithPoint(100, 100, 200)
    candidate := sweepWithPoint(100, 130, 200)
    _, err := BuildCompareReport(baseline, candidate, Thresholds{LatencyP95RegressionRatio: 1.10})
    if err == nil { t.Fatal("want regression error") }
}

func TestReportRendersFirstFailedConcurrency(t *testing.T) {
    sweep := artifact.SweepResult{RunContext: testRunContext(), Points: []artifact.PointResult{{Concurrency: 50, Passed: true}, {Concurrency: 100, Passed: false}}, FirstFailedConcurrency: ptrInt(100), HighestPassedConcurrency: 50}
    md := RenderSingleReport(sweep, nil)
    if !strings.Contains(md, "100") || !strings.Contains(md, "50") { t.Fatalf("missing concurrency summary: %s", md) }
}

func TestReportRendersUnavailableAsUnavailableNotZero(t *testing.T) {
    md := RenderSingleReport(artifact.SweepResult{RunContext: testRunContext()}, []artifact.Diff{{RuntimeDelta: artifact.RuntimeDelta{Statused: artifact.Statused{Status: "unavailable", Reason: "runtime route missing"}}}})
    if !strings.Contains(md, "unavailable") || strings.Contains(md, "runtime route missing | 0") { t.Fatalf("bad unavailable rendering: %s", md) }
}
```

- [ ] **步骤 3：运行 runner/report 测试确认失败**

```bash
go test ./pkg/loadtest/runner ./pkg/loadtest/report -run 'TestBuildCommandUsesCleanAllowlistEnvironment|TestBuildCommandRejectsUnsafeEnvAndDotEnv|TestCompareReportRejectsEveryComparableMismatch|TestCompareReportFailsOnRegressionThreshold|TestReportRendersFirstFailedConcurrency|TestReportRendersUnavailableAsUnavailableNotZero' -count=1
```

预期：FAIL。

- [ ] **步骤 4：实现 runner/report/commands**

`loadtest-run-new-api` 参数：`--binary`、`--env`、`--work-dir`、`--pid-file`、`--stdout-log`、`--stderr-log`、`--bootstrap-only`。

`loadtest-report` 参数：`--sweep`、`--baseline-sweep`、`--candidate-sweep`、`--baseline-metrics`、`--candidate-metrics`、`--thresholds`、`--out`、`--fail-on-regression`。

要求：

- runner 使用 allowlist clean env，不继承父进程 env。
- runner 不从 `PATH` 猜 binary，不在 repo root 运行服务。
- stdout/stderr 分别写指定日志文件，创建父目录。
- bootstrap-only 等待 `/api/status` 或关键表存在后停止进程；非 bootstrap 模式必须后台启动 `new-api`、写入 pid file，并在健康检查成功后返回，后续命令依赖该行为继续执行。
- report 必须 table-driven 校验 baseline/candidate 的 `comparison_config_hash`、`mock_hash`、`cache_mode`、`scenario`、`path`、`token_profile`；任一不一致返回 exit code `2`。
- report 必须输出并渲染 `first_failed_concurrency`、`highest_passed_concurrency`，且不把 unavailable 渲染成 0。
- SOP 文档必须包含从 `config.loadtest.yaml` 复制到 `.loadtest/config/config.yaml`、S0 健康检查、S1 smoke、缺少 PostgreSQL/Redis 时如何记录未运行原因的完整命令顺序；任务 10 的 smoke 命令必须与 SOP 保持一致。
- runner 必须对 env 文件中的 `SQL_DSN`、非空 `LOG_SQL_DSN`、`REDIS_CONN_STRING`、`HOST`、`PPROF_ADDR` 调用 localguard；不安全返回 `2`。runner 必须拒绝 `--work-dir` 下存在 `.env`，避免 `godotenv.Load(".env")` 重新引入真实配置。
- runner/report 的 `Run` 都必须遵守统一命令契约。

- [ ] **步骤 5：运行 runner/report 测试通过**

```bash
go test ./pkg/loadtest/runner ./pkg/loadtest/report -count=1
```

预期：PASS。

---

## 任务 10：命令集成和最小验证

**文件：**

- 修改：所有 `cmd/loadtest-*` wiring，仅限本计划列出的命令文件。

### 目标

确认所有命令可构建，loadtest 包测试通过，生产服务新增测试通过；可用时执行 S0/S1 smoke。

### 步骤

- [ ] **步骤 1：构建主服务和所有 loadtest 命令**

```bash
mkdir -p .loadtest/bin
go build -o .loadtest/bin/new-api .
go build -o .loadtest/bin/loadtest-check-config ./cmd/loadtest-check-config
go build -o .loadtest/bin/loadtest-mock-openai ./cmd/loadtest-mock-openai
go build -o .loadtest/bin/loadtest-client ./cmd/loadtest-client
go build -o .loadtest/bin/loadtest-concurrency-sweep ./cmd/loadtest-concurrency-sweep
go build -o .loadtest/bin/loadtest-seed ./cmd/loadtest-seed
go build -o .loadtest/bin/loadtest-collect ./cmd/loadtest-collect
go build -o .loadtest/bin/loadtest-run-new-api ./cmd/loadtest-run-new-api
go build -o .loadtest/bin/loadtest-report ./cmd/loadtest-report
```

预期：成功，后续 smoke 命令统一使用 `.loadtest/bin/...`。

- [ ] **步骤 2：运行全部 loadtest 包测试**

```bash
go test ./pkg/loadtest/... -count=1
```

预期：PASS。

- [ ] **步骤 3：运行生产服务新增测试**

```bash
go test . -run 'TestServerListenAddr|TestPprofListenAddr' -count=1
go test ./controller -run TestLoadtestRuntime -count=1
```

预期：PASS。

- [ ] **步骤 4：可用时运行本地 S0/S1 smoke**

仅当本机已有隔离 PostgreSQL/Redis 前置条件时运行。没有前置条件时不得伪造通过，只记录缺失前置条件。可用时必须覆盖真实 S0/S1 闭环：配置检查、schema bootstrap、seed、场景 context、mock、正式 new-api、S0 鉴权健康检查、低并发 S1 sweep、collector、gate 和 report。

```bash
mkdir -p .loadtest/config .loadtest/logs .loadtest/baseline .loadtest/runtime/new-api .loadtest/reports
cp config.loadtest.yaml .loadtest/config/config.yaml
.loadtest/bin/loadtest-check-config --config .loadtest/config/config.yaml --out-env .loadtest/config/new-api.env --out-run-context .loadtest/run-context.base.json
.loadtest/bin/loadtest-run-new-api --binary .loadtest/bin/new-api --env .loadtest/config/new-api.env --work-dir .loadtest/runtime/new-api --pid-file .loadtest/new-api.pid --stdout-log .loadtest/logs/new-api.stdout.log --stderr-log .loadtest/logs/new-api.stderr.log --bootstrap-only
.loadtest/bin/loadtest-seed --config .loadtest/config/config.yaml --run-context .loadtest/run-context.base.json --out .loadtest/baseline/seed.json --out-run-context .loadtest/run-context.seeded.json
.loadtest/bin/loadtest-concurrency-sweep --derive-run-context-only --config .loadtest/config/config.yaml --run-context .loadtest/run-context.seeded.json --scenario s1-smoke --token-profile subscription --path /v1/responses --api-key sk-loadtestsub --seed-output .loadtest/baseline/seed.json --mock-profile s1-smoke --out-run-context .loadtest/baseline/s1-run-context.json
.loadtest/bin/loadtest-mock-openai --addr 127.0.0.1:19080 --run-context .loadtest/baseline/s1-run-context.json --first-token-delay 50ms --stream-duration 500ms --chunk-interval 50ms --output-bytes 128 --prompt-tokens 11 --completion-tokens 17 --status-rate 429=0,502=0 --seed 1 --stats-out .loadtest/baseline/mock-stats.json & echo $! > .loadtest/mock-openai.pid
.loadtest/bin/loadtest-run-new-api --binary .loadtest/bin/new-api --env .loadtest/config/new-api.env --work-dir .loadtest/runtime/new-api --pid-file .loadtest/new-api.pid --stdout-log .loadtest/logs/new-api.stdout.log --stderr-log .loadtest/logs/new-api.stderr.log
.loadtest/bin/loadtest-client --health-check --url http://127.0.0.1:13080 --valid-api-key sk-loadtestsub --invalid-api-key sk-loadtestinvalid --runtime-url http://127.0.0.1:13080/debug/loadtest/runtime --pprof-url 'http://127.0.0.1:8005/debug/pprof/goroutine?debug=1' --out .loadtest/baseline/s0-health.json
.loadtest/bin/loadtest-concurrency-sweep --config .loadtest/config/config.yaml --url http://127.0.0.1:13080 --api-key sk-loadtestsub --token-profile subscription --path /v1/responses --model gpt-5.5 --scenario s1-smoke --points 2 --rps 1 --duration 5s --max-requests-per-point 10 --ramp-step 2 --ramp-interval 1s --timeout 30s --input-bytes 128 --output-bytes 128 --cooldown 2s --pid-file .loadtest/new-api.pid --seed-output .loadtest/baseline/seed.json --mock-profile s1-smoke --mock-stats .loadtest/baseline/mock-stats.json --run-context .loadtest/baseline/s1-run-context.json --artifact-dir .loadtest/baseline --out .loadtest/baseline/s1-sweep.json
.loadtest/bin/loadtest-report --sweep .loadtest/baseline/s1-sweep.json --out .loadtest/reports/s1-smoke.md
```

预期：所有命令 exit `0`，`s0-health.json`、`s1-sweep.json` 和 `s1-smoke.md` 生成；若缺少 PostgreSQL/Redis，则明确记录未运行 smoke 的原因。smoke 结束后必须按 SOP 停止 `new-api`、mock upstream、PostgreSQL 和 Redis，并确认 15432、16379、13080/18081、19080、8005 端口关闭。
