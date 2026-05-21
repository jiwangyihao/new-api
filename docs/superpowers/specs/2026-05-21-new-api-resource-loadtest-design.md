# new-api 资源受限并发矩阵压测套件设计

## 背景

当前 `new-api` 已有本地受控压测 harness：`artifact`、`config`、`localguard`、`mockopenai`、`client`、`seed`、`metrics`、`sweep`、`runner`、`report` 以及 gated runtime route。它已经能完成 loopback-only smoke、mock upstream、token quota seed、单轮 sweep、业务 diff 和报告生成。

目标是把 `~/Documents/GitHub/workbench` 中已验证的压测套件能力迁移为 `new-api` 原生 Go 压测套件，而不是复用 `sub2api` 业务假设。新的套件必须直接服务 `new-api` 的 plan/token-quota billing：`consume_logs`、`subscription_pre_consume_records`、`user_subscriptions`、`tokens`，不得重新引入 price/runway dashboard metrics。

## 目标

1. 提供 Go 原生 orchestrator，完整编排：构建/检查、隔离 infra、mock upstream、受限 `new-api` server、并发矩阵、指标采集、gate、报告、清理与 ports-closed artifact。
2. 第一阶段直接对齐 workbench 高容量矩阵：`250,500,750,1000` 并发点，每点 `3000` 请求，`ramp-step=25`，`ramp-interval=200ms`，`duration=45s`，`timeout=120s`。
3. 资源限制只作用于被测 `new-api` server，不限制 load generator、mock OpenAI、PostgreSQL、Redis 或 orchestrator。
4. 保留安全默认 smoke，但高容量矩阵必须由显式命令/profile 触发；不能把危险连接池配置设为默认。
5. 输出机器可读 artifacts，足以定位失败类别、资源瓶颈、业务不变量失败和端口清理失败。

## 非目标

- 不在 RackNerd 或任何远端环境运行压测。
- 不测试真实 OpenAI 或真实生产 upstream。
- 不复用 `sub2api` 的 usage logs/dedup 业务语义。
- 不关闭 `DataExportEnabled`、`LogConsumeEnabled`、订阅并发队列或 plan/token-quota billing。
- 不把 H2C 作为第一阶段 hard gate；H2C 只作为本地诊断实验模式。

## 总体架构

采用 Go 原生命令扩展，而非 Python orchestrator。

新增/扩展模块：

- `pkg/loadtest/orchestrator`：端到端编排 run：配置、二进制路径、启动/停止、逐点矩阵、失败停止、最终清理。
- `cmd/loadtest-resource-sweep`：高容量资源受限矩阵入口。
- `pkg/loadtest/resource`：server-only 资源限制与 process metrics。
- `pkg/loadtest/monitor`：runtime、PostgreSQL、Redis、process 周期采样与 peaks 汇总。
- `pkg/loadtest/analysis`：failure_class、hard gate、diagnostic gate 与报告输入归一化。
- 扩展 `pkg/loadtest/artifact`：transport profile、protocol counts、first error samples、resource limits、resource samples、ports closed、failure class。
- 扩展 `pkg/loadtest/client`：transport mode、keepalive profile、细粒度错误分类和协议统计。
- 扩展 runtime route：HTTP accept/ConnState 计数、GC/runtime 统计、现有 goroutines/heap/batch snapshot。

现有 `loadtest-concurrency-sweep` 保持可用，作为低层单场景 sweep；`loadtest-resource-sweep` 调用其包级能力或直接使用同一 client/metrics/gate 逻辑，避免复制业务判断。

## Profile

### benchmark profile

默认高容量矩阵：

- `points`: `[250,500,750,1000]`
- `requests_per_point`: `3000`
- `ramp_step`: `25`
- `ramp_interval`: `200ms`
- `duration`: `45s`
- `timeout`: `120s`
- `transport`: `h1_keepalive`
- `server_limits`:
  - `GOMAXPROCS=2`
  - `GOGC=100`
  - `GOMEMLIMIT=384MiB`
  - Windows：CPU affinity 2 cores，Job Object process memory limit `512MiB`
  - 非 Windows：记录 best-effort 限制能力，不伪造 Windows Job Object 语义

### smoke profile

保留现有安全本机 smoke：小并发、小请求数、小连接池，用于快速验证配置和 loopback 安全边界。smoke 不代表容量结果。

### h2c diagnostic profile

H2C 是明文 HTTP/2。它可用于判断连接 churn/accept 是否是瓶颈：若 H1 失败但 H2C 明显改善，则瓶颈偏连接层；若 H2C 仍失败，则更可能是 handler、DB、Redis、CPU 或流式处理。H2C 结果必须在 artifact/report 中标为 diagnostic，不作为第一阶段 hard gate。

## 资源限制

资源限制由 `pkg/loadtest/resource` 提供：

- `ApplyServerLimits(pid, limits)`：只作用于 `new-api` server 进程。
- `WriteResourceLimitsArtifact(path)`：记录实际施加/未能施加的限制。
- Windows 实现：Job Object memory limit、CPU affinity；环境变量由 runner 注入。
- 非 Windows 实现：使用可用平台能力；无法施加时 artifact 标为 `status=unavailable` 或 `status=best_effort`，原因必须明确。

限制不作用于：load generator、mock upstream、PostgreSQL、Redis、orchestrator。报告必须明确这一点，避免把 generator 被限制造成的结果误判为 server 容量。

## Client 能力

`pkg/loadtest/client` 扩展：

- transport modes：`h1_keepalive`、`h1_no_keepalive`、`h2c_diagnostic`。
- keepalive profile：显式配置 `MaxConnsPerHost`、`MaxIdleConns`、`MaxIdleConnsPerHost`、`IdleConnTimeout`。
- benchmark profile 可使用高连接数，但必须由 `loadtest-resource-sweep --profile benchmark` 显式启用；默认 smoke 继续使用本机安全小连接池。
- summary 增加：
  - `protocol_counts`: `HTTP/1.1`、`HTTP/2.0` 等。
  - `first_error_samples`: 前 N 个错误样本，包含 request index、phase、error reason、status、duration、request id。
  - 细粒度 error reasons：`connect_refused`、`connect_timeout`、`connection_reset`、`request_timeout`、`missing_first_token`、`missing_done`、`bad_status`、`read_error`、`json_error`、`client_canceled`。

## Metrics 与采样

`pkg/loadtest/monitor` 在每个并发点运行前、运行中、运行后采样。采样输出既保留 timeline，也汇总 peaks。

必采指标：

- process：RSS/WorkingSet、CPU time、thread count、handle count、open TCP sockets（可用时）。
- runtime route：goroutines、heap alloc/sys、GC count/pause、GOMAXPROCS、GOMEMLIMIT、batch status、HTTP ConnState/accept counters。
- PostgreSQL：active/idle 连接数、DB size、关键表 row counts、等待中的锁数量。慢查询摘要不进入第一阶段验收，避免依赖本机 PostgreSQL 扩展或日志配置。
- Redis：connected clients、used_memory、used_memory_rss、mem_fragmentation_ratio、instantaneous ops/sec、total_commands_processed、keyspace。
- business drain：等待 consume logs/subscription pre-consume records/token counters 到达稳定状态；逐表独立 timeout，不能因一个表卡住掩盖其他表状态。

PostgreSQL-only 查询只能位于 loadtest 包，不进入生产业务路径。

## Gates

### Hard gate

每个点必须满足：

- `success == max_requests`
- `errors == 0`
- `stop_reason == max_requests`
- `max_observed_in_flight >= point`
- HTTP status codes 只有 `200`
- stream：`done_received == success`，usage events 与成功数一致，bytes > 0。
- mock delta：upstream attempts 与请求数对齐，429/502 只允许按场景配置出现。
- business invariants：
  - subscription：`subscription_token_used`、`consume_logs`、`subscription_pre_consume_records` 与成功 usage 对齐。
  - compat：token quota/wallet 相关 delta 与成功 usage 对齐。
  - 退款判断必须使用 `new_api_request_id`，不能用 client local id。
- resource samples 存在且时间范围覆盖该点。
- ports_closed 在最终 cleanup 后通过。

250 点失败则停止后续点；更高点失败也停止后续点，并保留失败点完整 artifact。

### Diagnostic gate

用于分类，不必都作为 hard fail：

- connection reuse/accept 是否异常。
- Redis commands/success 是否异常。
- PostgreSQL active/idle 是否接近连接池上限。
- process memory 是否接近 limit。
- failure_class：`capacity_limit`、`client_transport`、`server_resource_limit`、`upstream_mock`、`postgres_bottleneck`、`redis_bottleneck`、`billing_invariant`、`stream_protocol`、`cleanup_failed`、`unknown`。

## Artifacts

每次 run 产出目录结构：

```text
.loadtest/local-run/<run-id>/
  config/
  logs/
  infra/
  baseline-or-candidate/
    resource-limits.json
    run-context.json
    points/
      c250-summary.json
      c250-diff.json
      c250-resource-samples.json
      c250-resource-peaks.json
      c250-analysis.json
      ...
    sweep.json
    ports-closed.json
  reports/
    resource-sweep.md
```

所有 artifact 必须包含 `run_context` 和 `schema_version`。所有 hash 使用 canonical JSON。stderr/stdout 中出现 DSN、API key、真实 host 必须脱敏。

## Runtime route 扩展

`/debug/loadtest/runtime` 继续只在 loopback server 且 `LOADTEST_RUNTIME_STATS_ENABLED=true` 时注册。

新增字段：

- `gomaxprocs`
- `gomemlimit_bytes`
- `gc_count`
- `last_gc_unix_ms`
- `pause_total_ns`
- `http_conn_state`: `new`、`active`、`idle`、`hijacked`、`closed`
- `http_accept_total`
- `http_active_current`
- 保留现有 goroutines、heap、profile rates、batch status。

ConnState/accept 计数不得影响生产路径；未启用 loadtest route 时仅为零成本或近零成本。

## CLI

新增命令示例：

```bash
.loadtest/bin/loadtest-resource-sweep \
  --config .loadtest/local-run/config/config.yaml \
  --profile benchmark \
  --binary .loadtest/bin/new-api.exe \
  --work-dir .loadtest/local-run/runtime/new-api \
  --artifact-dir .loadtest/local-run/benchmark \
  --points 250,500,750,1000 \
  --requests-per-point 3000 \
  --ramp-step 25 \
  --ramp-interval 200ms \
  --duration 45s \
  --timeout 120s
```

命令必须：

1. 校验 config 和 loopback 边界。
2. 拒绝生产 `.env`。
3. 启动/复用隔离 PostgreSQL 和 Redis；若无法可靠管理隔离 infra，则 fail closed，不污染本机默认 5432/6379。
4. 启动 mock upstream。
5. bootstrap + seed。
6. 启动 `new-api`，施加 server-only limits。
7. 逐点运行矩阵，采集 metrics，执行 gates。
8. 生成报告。
9. 清理所有本轮启动的进程。
10. 写 `ports-closed.json`；端口未关闭则命令返回非 0。

## 测试策略

- TDD：新增行为先写失败测试。
- Unit tests：resource limits artifact、profile validation、client transport/protocol/error reason、monitor parsers、analysis gates、ports closed。
- Command tests：`Run(args, stdout, stderr)` 覆盖 loopback fail-closed、benchmark profile 参数、cleanup on failure、ports-closed failure。
- Integration smoke：小矩阵 `2,4`，每点 10 请求，验证 orchestrator 端到端 artifact，不代表容量。
- 不用 mock 替代真实 PostgreSQL/Redis parser 行为；parser 可用 fixture，端到端 smoke 使用真实本地隔离服务。

## 验收标准

1. `loadtest-resource-sweep --profile benchmark` 能按 workbench 矩阵运行，并在 250 失败时停止后续点。
2. resource limits artifact 明确记录 server-only 限制和平台能力。
3. 每个点都有 summary、diff、resource samples、resource peaks、analysis。
4. 报告给出最大通过并发点、失败点、failure_class、RSS/CPU/Redis/Postgres/runtime 峰值。
5. 最终 cleanup 写 ports-closed artifact；端口未关闭时整体失败。
6. 保持现有 smoke 和 `loadtest-concurrency-sweep` 可用。
7. 默认配置不再有会耗尽本机 TCP 端口的无界或上千级连接池。
