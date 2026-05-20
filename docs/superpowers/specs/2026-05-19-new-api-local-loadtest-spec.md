# new-api 本地受控压测规格

## 背景

线上已经确认 `new-api` 在小资源环境下对并发较敏感。仅凭生产现象无法判断下一步代码级优化应优先处理哪条路径：流式转发、订阅计费、日志写入、Redis 并发租约、数据导出、token 估算、JSON 转换或 DB 连接池都有可能成为瓶颈。

本规格参考 `workbench` 中的本地受控并发压测 SOP，面向 `new-api` 设计一套隔离、可重复、可对比的压测闭环。压测结论只用于比较不同代码版本和配置下的相对表现，不直接作为生产容量承诺。

## 目标

1. 构建只面向本地或隔离环境的 `new-api` 压测 harness。
2. 覆盖 `/v1/responses` 和 `/v1/chat/completions` 两条主要入口。
3. 覆盖多个订阅用户形态，区分 subscription 用户、兼容字段用户和错误退款路径。
4. 使用 mock OpenAI upstream 固定流式响应行为，剥离真实上游延迟和风控变量。
5. 输出可比较的客户端、运行时、PostgreSQL、Redis、业务表和日志指标。
6. 用固定点并发矩阵定位首个失败并发点和失败形态。
7. 输出 baseline 与 candidate 对比报告，为后续代码级优化提供可执行的归因依据。

## 非目标

1. 不直接压测生产域名、生产容器、生产 PostgreSQL 或生产 Redis。
2. 不打真实 OpenAI 或其他真实上游，不消耗真实账号额度。
3. 不通过关闭 `DataExportEnabled`、关闭 `LogConsumeEnabled`、关闭订阅并发队列等方式制造好看的结果。
4. 不在第一阶段构建完整图形化压测平台。
5. 不把本地单机压测结果包装成生产容量承诺。
6. 不涉及任何 `sub2api` 代码、配置或运行时分析；历史 SOP 只作为方法论参考。
7. 第一阶段端到端压测只支持 PostgreSQL；新增生产代码仍必须保持 SQLite、MySQL、PostgreSQL 兼容，PostgreSQL-only 查询只能存在于 loadtest collector 中。
8. 不恢复 relay 请求的钱包 fallback。当前 `new-api` 请求计费是 subscription-only，钱包字段只作为兼容观测对象。

## 方案选择

### 方案 A：本地 mock upstream + 临时 PostgreSQL/Redis（推荐）

本地启动 `new-api`，接入临时 PostgreSQL、临时 Redis 和本地 mock OpenAI upstream。压测客户端只请求本地 `new-api`。

优点：覆盖真实 HTTP、鉴权、relay、订阅计费、日志、统计、数据库和 Redis 链路；没有生产污染；结果可重复。

缺点：需要实现 seed、mock upstream、collector 和 client 工具；本机资源与生产 VPS 不完全一致。

### 方案 B：服务内 benchmark / 单元级压测

用 Go benchmark 直接压测 token 估算、DTO 转换、日志构造、计费函数等局部热点。

优点：定位微观热点快，结果稳定。

缺点：不能证明端到端流式长连接、写库、Redis、后结算链路的稳定性。

### 方案 C：VPS 影子实例压测

在 VPS 备用端口启动候选 `new-api`，接入隔离数据库和 mock upstream。

优点：更接近生产硬件。

缺点：容易争抢生产机器资源；误操作风险高；结果受邻居服务影响。

### 结论

第一阶段实施方案 A。方案 B 作为发现热点后的补充。方案 C 暂不执行。

## 架构

```text
loadtest-client
  -> http://127.0.0.1:<new-api-port>/v1/responses
  -> http://127.0.0.1:<new-api-port>/v1/chat/completions
  -> 本地 new-api（GOMAXPROCS=2，ENABLE_PPROF=true）
  -> 本地 PostgreSQL / Redis
  -> 本地 mock OpenAI upstream

loadtest-collect
  -> PostgreSQL 只读统计
  -> Redis INFO / commandstats
  -> new-api pprof / runtime stats
  -> 业务表计数与日志摘要
  -> 输出 JSON snapshot / diff

loadtest-report
  -> 读取 baseline 与 candidate 的 JSON artifacts
  -> 输出单轮报告与跨 commit/config 对比报告
```

## 统一运行上下文

所有可比较 artifact 必须包含同一个 `run_context` 对象。report 只从输入 artifact 读取该对象，不从目录名推断。

```json
{
  "run_context": {
    "schema_version": 1,
    "role": "baseline",
    "commit": "abcdef0",
    "comparison_config_hash": "sha256:...",
    "seed_output_hash": "sha256:...",
    "mock_hash": "sha256:...",
    "cache_mode": "cold-fresh-role,warm-per-point",
    "scenario": "s2-short-stream",
    "path": "/v1/responses",
    "token_profile": "subscription",
    "model": "gpt-5.5"
  }
}
```

要求：

- `loadtest-client` summary 顶层必须包含 `run_context`。
- `loadtest-concurrency-sweep` 顶层必须包含 `run_context`。
- `loadtest-collect` snapshot 和 diff 顶层必须包含 `run_context`。
- `loadtest-report` 必须校验 baseline/candidate 的 `comparison_config_hash`、`mock_hash`、`cache_mode`、`scenario`、`path`、`token_profile` 一致。

生成责任：

- `loadtest-check-config` 生成不含 `seed_output_hash` 的基础 `.loadtest/run-context.base.json`，包含 commit、comparison_config_hash、cache_mode、model 和全局配置。
- `loadtest-seed` 读取 base context，写入 seed_output_hash，输出 `.loadtest/run-context.seeded.json`。
- `loadtest-concurrency-sweep` 按 scenario/path/token_profile/mock_hash 派生每个场景自己的 `.loadtest/<scenario>/run-context.json`，并传给 client/collector/report。
- 多场景并行运行时不得复用同一个 `.loadtest/run-context.json` 路径。

## 实现位置与命令形态

第一阶段固定为多个独立 Go binary，避免引入新的子命令框架：

```text
cmd/loadtest-check-config
cmd/loadtest-mock-openai
cmd/loadtest-client
cmd/loadtest-concurrency-sweep
cmd/loadtest-seed
cmd/loadtest-collect
cmd/loadtest-run-new-api
cmd/loadtest-report
pkg/loadtest/localguard
pkg/loadtest/mockopenai
pkg/loadtest/client
pkg/loadtest/metrics
pkg/loadtest/artifact
```

命令约定：

- 成功时 exit code 为 `0`。
- 配置错误、安全检查失败、业务不变量失败、回归门禁失败时 exit code 为 `2`。
- 网络、数据库、Redis、文件系统等运行时错误时 exit code 为 `1`。
- 默认 stdout 输出人类可读摘要；指定 `--out` 时必须写完整 JSON artifact。
- stderr 只输出错误摘要，不能打印完整 API Key、DSN 密码或请求体。
- 所有 JSON marshal/unmarshal 必须使用项目 `common` 封装。
- 新增业务表访问使用 GORM；collector 中 PostgreSQL 系统视图查询可以使用 raw SQL，并隔离在 collector 包内。
- 启动 `new-api` 时必须使用干净环境：不得继承当前 shell 的 `.env`、`LOG_SQL_DSN`、生产 `SQL_DSN`、生产 Redis 或任何真实上游 key。
- 如当前 `new-api` 仍会自动 `godotenv.Load(".env")`，loadtest 启动器必须在不含生产 `.env` 的工作目录启动，或设置等价的禁用 `.env` 加载开关。
- `LOG_SQL_DSN` 若存在，必须与 `SQL_DSN` 执行同等 localguard 校验：loopback host、数据库名包含 `loadtest`、PostgreSQL 使用 `postgresql://` 或 `postgres://`。第一阶段推荐显式设置 `LOG_SQL_DSN=`，让日志库复用主库。

## 安全边界：`localguard`

`localguard` 是所有压测命令共享的安全边界。安全检查必须 fail-closed，第一阶段不提供绕过开关。

### 配置优先级

1. CLI 参数优先级最高，只能覆盖 `--config` 中已定义的 loadtest 字段。
2. YAML 是基准配置。
3. 当前 shell 环境变量不得隐式覆盖 YAML；`loadtest-check-config` 只输出 `.loadtest/config/new-api.env`。
4. `loadtest-*` 命令不得读取生产环境中的 `SQL_DSN`、`REDIS_CONN_STRING`、`OPENAI_API_KEY` 等变量作为默认值。
5. `.loadtest/config/new-api.env` 是启动 `new-api` 的唯一环境来源。

### URL 规则

允许：

- `http://localhost:<port>`
- `http://127.0.0.1:<port>`
- `http://[::1]:<port>`

禁止：

- `https://`；
- 公网域名；
- 公网 IP；
- 私网但非 loopback IP；
- DNS 解析后不是 loopback 的 hostname；
- 空 upstream base URL；
- `api.openai.com` 或任何真实上游域名。

### PostgreSQL 规则

- DSN host 必须是 loopback。
- 数据库名必须包含 `loadtest`。
- `new-api` 的 `SQL_DSN` 必须使用 `postgresql://` 或 `postgres://` URL 形式，例如：

```text
postgresql://new_api_loadtest:loadtest@127.0.0.1:15432/new_api_loadtest?sslmode=disable
```

- 不允许把 libpq keyword DSN 直接写入 `SQL_DSN`，因为当前 `model.chooseDB` 只通过 `postgres://` / `postgresql://` 前缀选择 PostgreSQL。
- collector 如需接受 keyword DSN，必须转换为 `new-api` 可识别的 URL 形式后再写入 env。

### Redis 规则

- Redis 地址必须是 loopback。
- 默认端口为 `16379`。
- 不允许使用空地址回退到生产默认配置。

### API Key 规则

`new-api` 当前 `TokenAuth` 会先去掉 `sk-`，再按第一个 `-` 截断，只使用截断后的 key 查询 `tokens.key`。因此 loadtest token 的数据库 key 不能依赖连字符后缀区分用户。

固定约定：

| 用途 | DB `tokens.key` | 客户端 `--api-key` | Authorization header |
|---|---|---|---|
| subscription | `loadtestsub` | `sk-loadtestsub` | `Bearer sk-loadtestsub` |
| compat | `loadtestcompat` | `sk-loadtestcompat` | `Bearer sk-loadtestcompat` |
| invalid | 不存在 | `sk-loadtestinvalid` | `Bearer sk-loadtestinvalid` |

localguard 对 API Key 的要求：

- CLI 接受裸 `sk-...`，client 负责生成 `Authorization: Bearer ...`。
- 原始 Authorization 必须是 `Bearer sk-...`。
- 去掉 `Bearer ` 和 `sk-` 后不能包含 `-`。
- 解析出的 DB key 必须在白名单：`loadtestsub`、`loadtestcompat`、`loadtestinvalid`。
- seed 只能创建 `loadtestsub` 和 `loadtestcompat`。
- 命令输出不得泄露除上述固定测试 key 以外的任何真实 key。

## 服务绑定与 pprof 安全要求

当前 `main.go` 默认 `server.Run(":" + port)`，pprof 默认 `http.ListenAndServe("0.0.0.0:8005", nil)`。仅设置端口不能证明 loopback 隔离。

第一阶段必须增加或使用显式监听地址配置：

```text
HOST=127.0.0.1
PORT=13080
PPROF_ADDR=127.0.0.1:8005
```

要求：

- `new-api` 主服务必须支持 `HOST` 或等价 bind 参数；为空时保持现有行为，避免改变生产默认。
- loadtest SOP 必须设置 `HOST=127.0.0.1`。
- pprof 必须支持 `PPROF_ADDR`；为空时保持现有 `0.0.0.0:8005` 行为，避免改变生产默认。
- loadtest SOP 必须设置 `PPROF_ADDR=127.0.0.1:8005`。
- `loadtest-check-config` 必须校验最终监听地址是 loopback；无法确认时失败。
- `/debug/loadtest/runtime` 只在服务绑定 loopback 且 `LOADTEST_RUNTIME_STATS_ENABLED=true` 时注册。
- `/debug/loadtest/runtime` 对非 loopback remote address 返回 404 或 403。

## 运行时 profile 要求

普通 pprof 不会自动打开 block/mutex profile。第一阶段必须增加 loadtest 专用采样开关：

```text
LOADTEST_PROFILE_BLOCK_RATE=1000
LOADTEST_PROFILE_MUTEX_FRACTION=5
```

要求：

- 仅当 `LOADTEST_RUNTIME_STATS_ENABLED=true` 时应用这两个开关。
- `LOADTEST_PROFILE_BLOCK_RATE > 0` 时调用 `runtime.SetBlockProfileRate`。
- `LOADTEST_PROFILE_MUTEX_FRACTION > 0` 时调用 `runtime.SetMutexProfileFraction`。
- 采集报告必须记录实际采样值。
- block/mutex profile 不可用时输出 `unavailable`，不得伪造空结果为「无争用」。

## 配置文件契约

`config.loadtest.yaml` 是 loadtest harness 消费的配置文件，不要求 `new-api` 新增 YAML 配置加载能力。`loadtest-check-config` 负责读取该 YAML，并输出可用于启动 `new-api` 和其他命令的 `.env` 片段。

示例 schema：

```yaml
server:
  host: "127.0.0.1"
  port: 13080
  pprof_addr: "127.0.0.1:8005"
  runtime_stats_enabled: true
postgres:
  dsn: "postgresql://new_api_loadtest:loadtest@127.0.0.1:15432/new_api_loadtest?sslmode=disable"
redis:
  addr: "127.0.0.1:16379"
mock_upstream:
  base_url: "http://127.0.0.1:19080"
loadtest:
  model: "gpt-5.5"
  group: "default"
  subscription_key: "sk-loadtestsub"
  compat_key: "sk-loadtestcompat"
  pid_file: ".loadtest/new-api.pid"
retry:
  retry_times: 0
  automatic_retry_status_codes: []
thresholds:
  latency_p95_regression_ratio: 1.10
  ttft_p95_regression_ratio: 1.10
```

输出 `.loadtest/config/new-api.env` 至少包含：

```text
HOST=127.0.0.1
PORT=13080
PPROF_ADDR=127.0.0.1:8005
ENABLE_PPROF=true
LOADTEST_RUNTIME_STATS_ENABLED=true
LOADTEST_PROFILE_BLOCK_RATE=1000
LOADTEST_PROFILE_MUTEX_FRACTION=5
SQL_DSN=postgresql://new_api_loadtest:loadtest@127.0.0.1:15432/new_api_loadtest?sslmode=disable
LOG_SQL_DSN=
REDIS_CONN_STRING=redis://127.0.0.1:16379/0
GOMAXPROCS=2
GOGC=100
BATCH_UPDATE_ENABLED=true
SQL_MAX_OPEN_CONNS=10
SQL_MAX_IDLE_CONNS=5
SQL_MAX_LIFETIME=60
```

哈希字段拆分：

- `comparison_config_hash`：YAML 中除 commit 外的运行条件、生成 env、mock 参数、seed 输入、retry 配置、threshold 配置。baseline/candidate 必须一致，才能进行收益比较。
- `commit`：单独记录，不参与 `comparison_config_hash`。
- `seed_output_hash`：seed 执行后的输出 hash，单独记录。
- `mock_hash`：mock 参数 hash，单独记录，并应包含在 `comparison_config_hash` 的输入中。

## Mock OpenAI upstream 契约

### 命令

```text
loadtest-mock-openai \
  --addr 127.0.0.1:19080 \
  --first-token-delay 100ms \
  --stream-duration 1s \
  --chunk-interval 100ms \
  --output-bytes 128 \
  --prompt-tokens 11 \
  --completion-tokens 17 \
  --status-rate 429=0.05,502=0.01 \
  --seed 1 \
  --stats-out .loadtest/mock-stats.json
```

### 通用行为

- 只监听 loopback。
- `GET /v1/models` 返回包含 `gpt-5.5` 的 model list。
- 错误注入按请求序号和 seed 确定，必须可重复。
- `status-rate` 只影响主请求，不影响 `/v1/models`。
- `output-bytes` 指模型文本 delta 的总字节数，不含 SSE framing。
- usage 固定为 `prompt_tokens=11`、`completion_tokens=17`、`total_tokens=28`，除非命令参数覆盖。
- 每个成功响应必须包含可关联的 request id。
- 成功和错误响应都必须设置 `X-Oneapi-Request-Id: upstream-loadtest-<attempt>`，供 `new-api` 写入 `logs.upstream_request_id`。
- 第一阶段 retry 固定关闭：`retry.retry_times=0`、`automatic_retry_status_codes=[]`。S4 覆盖错误和退款，不覆盖 retry 放大；retry 行为作为后续扩展场景。
- mock stats 必须按并发点隔离：每个 sweep point 使用独立 `--stats-out`，或在 point 前后采集 stats snapshot 并用 delta 判定；不得把全局累计 stats 直接与当前点 `total` 比较。

### Mock stats artifact

mock 必须输出可选统计 artifact，供 S4 校验上游注入错误数量：

```json
{
  "schema_version": 1,
  "attempts_total": 1000,
  "injected_status_counts": {"429": 50, "502": 10},
  "attempts": [
    {
      "attempt_index": 1,
      "method": "POST",
      "path": "/v1/responses",
      "upstream_request_id": "upstream-loadtest-1",
      "injected_status": 0
    }
  ]
}
```

### `/v1/chat/completions` 请求体

client 发送：

```json
{
  "model": "gpt-5.5",
  "stream": true,
  "stream_options": {"include_usage": true},
  "messages": [
    {"role": "user", "content": "<input-bytes generated text>"}
  ]
}
```

### `/v1/responses` 请求体

client 发送：

```json
{
  "model": "gpt-5.5",
  "stream": true,
  "input": "<input-bytes generated text>"
}
```

### `/v1/chat/completions` 流式响应

响应头：

```text
Content-Type: text/event-stream
Cache-Control: no-cache
Connection: keep-alive
X-Oneapi-Request-Id: upstream-loadtest-<attempt>
```

SSE data 形态：

```text
data: {"id":"chatcmpl-loadtest-<n>","object":"chat.completion.chunk","created":1710000000,"model":"gpt-5.5","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}

data: {"id":"chatcmpl-loadtest-<n>","object":"chat.completion.chunk","created":1710000000,"model":"gpt-5.5","choices":[{"index":0,"delta":{"content":"..."},"finish_reason":null}]}

data: {"id":"chatcmpl-loadtest-<n>","object":"chat.completion.chunk","created":1710000000,"model":"gpt-5.5","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":11,"completion_tokens":17,"total_tokens":28}}

data: [DONE]
```

### `/v1/responses` 流式响应

响应头：

```text
Content-Type: text/event-stream
Cache-Control: no-cache
Connection: keep-alive
X-Oneapi-Request-Id: upstream-loadtest-<attempt>
```

SSE event 形态：

```text
event: response.created
data: {"type":"response.created","response":{"id":"resp_loadtest_<n>","model":"gpt-5.5","status":"in_progress"}}

event: response.output_text.delta
data: {"type":"response.output_text.delta","item_id":"msg_loadtest_<n>","output_index":0,"content_index":0,"delta":"..."}

event: response.completed
data: {"type":"response.completed","response":{"id":"resp_loadtest_<n>","model":"gpt-5.5","status":"completed","usage":{"input_tokens":11,"output_tokens":17,"total_tokens":28}}}

data: [DONE]
```

### 错误响应

错误响应不使用 SSE framing，直接返回 JSON。

响应头：

```text
Content-Type: application/json
X-Oneapi-Request-Id: upstream-loadtest-<attempt>
```

429：

```json
{"error":{"message":"loadtest injected rate limit","type":"rate_limit_error","code":"rate_limit_exceeded"}}
```

502：

```json
{"error":{"message":"loadtest injected upstream failure","type":"upstream_error","code":"bad_gateway"}}
```

## `loadtest-client` 契约

### 参数

```text
--url http://127.0.0.1:13080
--api-key sk-loadtestsub
--path /v1/responses
--model gpt-5.5
--scenario s2-short-stream
--concurrency 100
--rps 0
--duration 30s
--max-requests 1000
--ramp-step 100
--ramp-interval 2s
--timeout 90s
--input-bytes 1024
--stream true
--run-context .loadtest/s2-responses-subscription/run-context.json
--out .loadtest/.../summary.json
```

`--scenario` 用于标记场景，不再表示钱包/订阅资金来源。第一阶段所有 relay 请求都使用订阅计费。

调度语义：

- `rps=0` 表示不限速，只受 concurrency 限制。
- `duration` 和 `max-requests` 同时存在时，先达到者停止。
- `timeout` 是单请求完整读取超时。
- `ramp-step` 表示每轮增加的目标并发；等于 concurrency 时直接升到目标并发。
- `stop_reason` 枚举：`duration`、`max_requests`、`fatal_error`、`context_cancelled`。

### Summary JSON schema

```json
{
  "schema_version": 1,
  "run_context": {},
  "run_id": "20260519T120000Z-responses-subscription-c100",
  "started_at": "2026-05-19T12:00:00Z",
  "ended_at": "2026-05-19T12:00:30Z",
  "path": "/v1/responses",
  "scenario": "s2-short-stream",
  "token_profile": "subscription",
  "model": "gpt-5.5",
  "target_concurrency": 100,
  "max_observed_in_flight": 100,
  "total": 1000,
  "success": 1000,
  "errors": 0,
  "status_codes": {"200": 1000},
  "error_reasons": {},
  "first_error_samples": [
    {
      "request_index": 12,
      "new_api_request_id": "20260519120000-abcdef12",
      "reason": "status_non_2xx",
      "status_code": 502,
      "message": "loadtest injected upstream failure"
    }
  ],
  "latency_ms": {"p50": 0, "p95": 0, "p99": 0},
  "ttft_ms": {"p50": 0, "p95": 0, "p99": 0},
  "duration_ms": 0,
  "requests_per_second": 0,
  "stream": {
    "first_token_received": 1000,
    "done_received": 1000,
    "completed_event_received": 1000,
    "chunks": 10000,
    "bytes": 128000,
    "usage_events": 1000,
    "missing_first_token": 0,
    "missing_done": 0,
    "invalid_sse": 0
  },
  "usage": {
    "prompt_tokens": 11000,
    "completion_tokens": 17000,
    "total_tokens": 28000
  },
  "requests": [
    {
      "request_index": 1,
      "client_request_id": "client-loadtest-1",
      "new_api_request_id": "20260519120000-abcdef12",
      "upstream_request_id": "upstream-loadtest-1",
      "status_code": 200,
      "success": true,
      "prompt_tokens": 11,
      "completion_tokens": 17,
      "total_tokens": 28,
      "latency_ms": 1200,
      "ttft_ms": 120
    }
  ],
  "stop_reason": "duration"
}
```

request id 规则：

- `client_request_id` 只是本地序号，可选用于日志。
- `new_api_request_id` 必须从 `new-api` 响应头 `X-Oneapi-Request-Id` 提取；错误响应也必须提取。
- `upstream_request_id` 来自上游响应头或 SSE body，只用于 upstream 关联。
- collector 的 `charges_by_request` 必须以 `new_api_request_id` join `logs.request_id` 和 `subscription_pre_consume_records.request_id`。

错误分类枚举：

- `request_build_error`
- `http_client_do_error`
- `client_timeout`
- `status_non_2xx`
- `stream_read_error`
- `missing_first_token`
- `missing_done`
- `invalid_sse`
- `usage_mismatch`

## `loadtest-concurrency-sweep` 契约

### 参数

```text
--config .loadtest/config/config.yaml
--url http://127.0.0.1:13080
--api-key sk-loadtestsub
--path /v1/responses
--model gpt-5.5
--scenario s2-short-stream
--points 10,20,50,100,150,200,300
--duration 30s
--max-requests-per-point 1000
--timeout 90s
--cooldown 2s
--pid-file .loadtest/new-api.pid
--mock-stats .loadtest/concurrency-sweep/responses-subscription/c100-mock-stats.json
--run-context .loadtest/s2-responses-subscription/run-context.json
--artifact-dir .loadtest/concurrency-sweep/responses-subscription
--out .loadtest/concurrency-sweep/responses-subscription.json
```

`sweep` 每个并发点必须执行：

1. `loadtest-collect` before；
2. 启动 during process/runtime sampler，并与 client 并发运行；
3. `loadtest-client`；
4. 等待 cooldown；
5. `loadtest-collect` after + diff；
6. 按 scenario gate 判定 passed。

### 输出 schema

```json
{
  "schema_version": 1,
  "run_context": {},
  "run_id": "20260519T120000Z-s2-responses-subscription",
  "scenario": "s2-short-stream",
  "path": "/v1/responses",
  "token_profile": "subscription",
  "points": [
    {
      "concurrency": 100,
      "passed": true,
      "summary_path": ".loadtest/.../c100-summary.json",
      "metrics_before_path": ".loadtest/.../c100-before.json",
      "metrics_after_path": ".loadtest/.../c100-after.json",
      "metrics_diff_path": ".loadtest/.../c100-diff.json",
      "profile_paths": {
        "cpu": ".loadtest/.../c100-cpu.pprof",
        "heap": ".loadtest/.../c100-heap.pprof",
        "block": ".loadtest/.../c100-block.pprof",
        "mutex": ".loadtest/.../c100-mutex.pprof",
        "goroutine": ".loadtest/.../c100-goroutine.txt"
      },
      "summary_excerpt": {
        "total": 1000,
        "success": 1000,
        "errors": 0,
        "latency_p95_ms": 0,
        "ttft_p95_ms": 0,
        "requests_per_second": 0,
        "max_observed_in_flight": 100,
        "actual_429": 0,
        "actual_502": 0,
        "upstream_attempts_total": 1000
      },
      "resource_peaks": {
        "rss_peak_bytes": 0,
        "goroutines_peak": 0,
        "heap_alloc_peak_bytes": 0
      },
      "gate": {
        "passed": true,
        "failed_reasons": []
      }
    }
  ],
  "first_failed_concurrency": null,
  "highest_passed_concurrency": 300
}
```

## 场景 gate

### S1/S2：常规流式请求

```text
passed = errors == 0
      && success == total
      && status_codes 只有 200
      && max_observed_in_flight >= concurrency * 0.9
      && stream.done_received == success
      && stream.usage_events == success
      && stream.bytes == success * mock.output_bytes
      && business_invariants 全部通过
```

### S3：长连接占用

```text
passed = errors == 0
      && success == total
      && status_codes 只有 200
      && max_observed_in_flight >= concurrency * 0.9
      && stream.done_received == success
      && stream.usage_events == success
      && stream.bytes == success * mock.output_bytes
      && business_invariants 全部通过
      && goroutines_after_drain <= goroutines_before + max(50, concurrency * 2)
      && rss_after_drain_bytes <= rss_before_bytes + max(128MB, rss_before_bytes * 0.25)
```

其中资源增长 gate 是泄漏哨兵，不用于证明容量；如果触发必须标记失败并保存 profile。

### S4：错误和退款路径

确定性错误注入规则：对第 `n` 个主请求，使用 `hash(seed, n) % 10000`。429 命中区间 `[0, rate429*10000)`，502 命中接续区间。这样每个并发点都能预先计算期望错误数。

```text
expected_429 = deterministic_count(seed, total, 429)
expected_502 = deterministic_count(seed, total, 502)
passed = actual_429 == expected_429
      && actual_502 == expected_502
      && non_injected_errors == 0
      && success == total - expected_429 - expected_502
      && failed_request_charge_delta == 0
      && upstream_attempts_total == total
      && business_invariants 全部通过
```

### S5：大上下文 / 大输出

固定参数：

```text
first-token-delay=150ms
stream-duration=3s
chunk-interval=100ms
prompt-tokens=11
completion-tokens=17
duration=60s
max-requests=300
timeout=120s
ramp-step=concurrency
cooldown=2s
```

```text
passed = errors == 0
      && success == total
      && status_codes 只有 200
      && stream.done_received == success
      && stream.bytes == success * mock.output_bytes
      && stream.usage_events == success
      && business_invariants 全部通过
      && rss_after_drain_bytes <= rss_before_bytes + max(256MB, rss_before_bytes * 0.5)
```

S5 的 CPU/heap 结果主要用于归因；资源 gate 只捕捉明显泄漏或截断。

## `loadtest-seed` 契约

职责：初始化本地临时 DB 的最小业务数据。seed 必须幂等，重复执行不能让用户、token、channel 或订阅数量膨胀。

### 固定数据

| 对象 | 字段 | 值 |
|---|---|---|
| subscription user | `username` | `loadtest_subscription` |
| subscription user | `status` | enabled |
| subscription user | `group` | `default` |
| subscription token | `key` | `loadtestsub` |
| subscription token | `status` | enabled |
| subscription token | `expired_time` | `-1` |
| compat user | `username` | `loadtest_compat` |
| compat user | `status` | enabled |
| compat user | `group` | `default` |
| compat token | `key` | `loadtestcompat` |
| compat token | `status` | enabled |
| compat token | `expired_time` | `-1` |
| group | name | `default` |
| model | name | `gpt-5.5` |
| channel | `type` | OpenAI channel type |
| channel | `status` | enabled |
| channel | `name` | `loadtest-openai` |
| channel | `base_url` | `http://127.0.0.1:19080` |
| channel | `key` | `sk-loadtest-upstream` |
| channel | `models` | `gpt-5.5` |
| channel | `group` | `default` |
| channel | `priority` / `weight` | 固定值，确保稳定路由 |


### 计费配置

seed 必须写入并刷新运行时配置，确保 `gpt-5.5` 可计费、可在 `/v1/models` 中展示、可进入 relay：

```text
ModelRatio={"gpt-5.5":1}
CompletionRatio={"gpt-5.5":1}
GroupRatio={"default":1}
```

要求：

- 写入 `options` 表后必须调用项目现有 ratio setting 更新函数或等价刷新逻辑。
- 不使用 `AcceptUnsetRatioModel` 作为主要方案；该字段只能作为额外兼容观测。
- seed 输出必须记录实际 ratio 配置。
### subscription 数据

两个用户都必须有有效订阅，避免触发 subscription-only 请求计费失败：

- 创建或更新 enabled `SubscriptionPlan`。
- `MonthlyTokenLimit` 至少为 `1_000_000_000`。
- `ConcurrencyLimit` 使用配置值，默认 `1_000`。
- 创建或更新 active `UserSubscription`。
- `TokenLimit` 至少为 `1_000_000_000`。
- `TokenUsed=0`。
- `Status="active"`。
- `StartTime <= now`，`EndTime >= now + 24h`。
- subscription 用户设置 `BillingPreference="subscription_first"` 或项目精确等价字段。
- compat 用户设置 `BillingPreference="wallet_only"`，用于验证请求仍走 subscription-only 且钱包/token quota 不扣减。

### 兼容字段数据

- compat 用户 `Quota` 至少为 `1_000_000_000`。
- compat token `RemainQuota` 至少为 `1_000_000_000`。
- 这些字段在 relay 请求后不应减少；它们只用于确认当前 subscription-only 行为没有回退到钱包。

### channel/ability 数据

- 禁用或排除所有非 loadtest channel，确保不会随机选到真实 upstream。
- loadtest channel 必须 enabled。
- 必须创建对应 ability，使 `default` group 的 `gpt-5.5` 只路由到 mock upstream。
- seed 后必须清理或刷新 token、user、channel、ability、subscription 相关缓存，避免重复运行时读到旧状态。

### options

必须写入或确认：

```text
LogConsumeEnabled=true
DataExportEnabled=true
perf_metrics_setting.enabled=true
```

不得关闭订阅并发队列。

### 输出 JSON

```json
{
  "schema_version": 1,
  "user_id_subscription": 1001,
  "user_id_compat": 1002,
  "token_subscription": "sk-loadtestsub",
  "token_compat": "sk-loadtestcompat",
  "token_db_key_subscription": "loadtestsub",
  "token_db_key_compat": "loadtestcompat",
  "channel_id": 1,
  "model": "gpt-5.5",
  "group": "default",
  "mock_base_url": "http://127.0.0.1:19080",
  "expected_usage_per_success": {
    "prompt_tokens": 11,
    "completion_tokens": 17,
    "total_tokens": 28
  }
}
```

## `loadtest-collect` 契约

职责：只读采集压测前后指标，输出 snapshot 和 diff。PostgreSQL、Redis、pprof、runtime route 任一不可用时：

- S0/S1/S2 的必需资源不可用时 exit code 为 `1`。
- 可选 profile 或 runtime 内部指标不可用时，字段输出 `{"status":"unavailable","reason":"..."}` 并继续。
- 业务不变量无法计算时 exit code 为 `2`。

### 命令

before：

```text
loadtest-collect \
  --config .loadtest/config/config.yaml \
  --pid-file .loadtest/new-api.pid \
  --run-context .loadtest/s2-responses-subscription/run-context.json \
  --out-snapshot .loadtest/.../metrics-before.json
```

after + diff：

```text
loadtest-collect \
  --config .loadtest/config/config.yaml \
  --pid-file .loadtest/new-api.pid \
  --run-context .loadtest/s2-responses-subscription/run-context.json \
  --summary .loadtest/.../c100-summary.json \
  --before .loadtest/.../metrics-before.json \
  --out-snapshot .loadtest/.../metrics-after.json \
  --out-diff .loadtest/.../metrics-diff.json \
  --mock-stats-delta .loadtest/concurrency-sweep/responses-subscription/c100-mock-stats-delta.json \
  --wait-drain
```

要求：

- `--pid-file` 必须存在且指向当前 `new-api` 进程。
- `--summary` 是计算请求级不变量的基准来源。
- diff 以 `summary.requests[*].new_api_request_id` join DB 日志和预扣记录。
- diff 需要读取当前并发点对应的 mock stats delta，S4 的 `actual_429`、`actual_502`、`upstream_attempts_total` 只来自该并发点窗口。
- `--mock-stats-delta` 是当前并发点的 mock stats 差量文件；如果 S4 gate 由 sweep 计算，collector 仍必须把该路径写入 diff artifact。

### Snapshot JSON schema

```json
{
  "schema_version": 1,
  "run_context": {},
  "run_id": "20260519T120000Z-before",
  "collected_at": "2026-05-19T12:00:00Z",
  "process": {
    "status": "ok",
    "pid": 12345,
    "rss_bytes": 0,
    "cpu_percent": 0,
    "rss_peak_bytes": 0
  },
  "postgres": {
    "status": "ok",
    "connections": {"active": 0, "idle": 0, "total": 0},
    "wait_events": [],
    "locks": {"waiting": 0, "blocked_pids": []},
    "database": {},
    "tables": {},
    "pg_stat_statements_top_total_time": [],
    "write_counters": {"inserts": 0, "updates": 0, "deletes": 0}
  },
  "redis": {
    "status": "ok",
    "clients": {},
    "memory": {},
    "stats": {"total_commands_processed": 0},
    "commandstats": {},
    "keyspace": {}
  },
  "runtime": {
    "status": "ok",
    "goroutines": 0,
    "goroutines_peak": 0,
    "heap_alloc_bytes": 0,
    "heap_alloc_peak_bytes": 0,
    "heap_inuse_bytes": 0,
    "heap_objects": 0,
    "mallocs": 0,
    "frees": 0,
    "num_gc": 0,
    "pause_total_ns": 0,
    "gc_cpu_fraction": 0,
    "next_gc_bytes": 0,
    "block_profile_rate": 1000,
    "mutex_profile_fraction": 5
  },
  "business": {
    "logs_count": 0,
    "quota_data_count": 0,
    "subscription_pre_consume_records_count": 0,
    "subscription_token_used_sum": 0,
    "compat_subscription_token_used_sum": 0,
    "compat_user_quota_sum": 0,
    "compat_token_remain_quota_sum": 0,
    "perf_metrics_count": 0,
    "charges_by_request": [
      {
        "new_api_request_id": "20260519120000-abcdef12",
        "client_request_id": "client-loadtest-1",
        "status_code": 200,
        "success": true,
        "log_quota": 28,
        "subscription_token_delta": 28,
        "net_subscription_token_delta": 28,
        "compat_user_quota_delta": 0,
        "compat_token_remain_delta": 0,
        "pre_consume_status": "settled"
      }
    ]
  },
  "logs": {
    "consume_logs_by_path_model_token": [],
    "stdout_full_params_lines": 0,
    "perf_metric_upsert_errors": 0
  }
}
```

### Diff JSON schema

```json
{
  "schema_version": 1,
  "run_context": {},
  "before_path": "metrics-before.json",
  "after_path": "metrics-after.json",
  "summary_path": "c100-summary.json",
  "business_delta": {
    "logs_count": 0,
    "subscription_token_used_sum": 0,
    "compat_subscription_token_used_sum": 0,
    "compat_user_quota_sum": 0,
    "compat_token_remain_quota_sum": 0,
    "perf_metrics_count": 0,
    "charges_by_request": [
      {
        "new_api_request_id": "20260519120000-abcdef12",
        "client_request_id": "client-loadtest-1",
        "status_code": 200,
        "success": true,
        "subscription_token_delta": 28,
        "net_subscription_token_delta": 28,
        "compat_user_quota_delta": 0,
        "compat_token_remain_delta": 0,
        "pre_consume_status": "settled"
      }
    ]
  },
  "postgres_delta": {
    "writes_total": 0,
    "inserts": 0,
    "updates": 0,
    "deletes": 0,
    "writes_per_success": 0
  },
  "redis_delta": {
    "commands_total": 0,
    "commands_per_success": 0,
    "commandstats_delta": {}
  },
  "runtime_delta": {
    "rss_peak_bytes": 0,
    "goroutines_peak": 0,
    "heap_alloc_peak_bytes": 0,
    "num_gc_delta": 0,
    "pause_total_ns_delta": 0
  },
  "invariants": [
    {
      "name": "subscription_token_used_matches_success_usage",
      "status": "passed",
      "expected": 28000,
      "actual": 28000,
      "details": ""
    }
  ]
}
```

Invariant status 枚举：`passed`、`failed`、`unavailable`、`not_applicable`。

`unavailable` 传播规则：任一必读字段不可用时，对应派生指标也必须输出 `{"status":"unavailable","reason":"source unavailable"}`，report 不得把它当 0。

### PostgreSQL 指标

必须采集：

- active / idle / total connections；
- `pg_stat_activity.wait_event_type`、`wait_event`；
- 长事务数量；
- lock wait / blocked pid；
- `pg_stat_database` commit / rollback / blocks / temp / deadlocks；
- `pg_stat_user_tables` 中关键表 scan、insert、update、dead tuple；
- 关键表行数增量；
- 如果启用 `pg_stat_statements`，采 `calls`、`total_exec_time`、`mean_exec_time`、`rows`、query prefix Top 30。

关键表：

```text
logs
users
tokens
channels
user_subscriptions
subscription_pre_consume_records
quota_data
perf_metrics
```

### Redis 指标

必须采集：

- connected clients；
- blocked clients；
- rejected connections；
- used memory；
- used memory RSS；
- fragmentation；
- instantaneous ops/sec；
- total commands processed；
- expired keys；
- evicted keys；
- keyspace hits / misses；
- commandstats，至少包括 `eval`、`evalsha`、`hgetall`、`hmget`、`hset`、`hincrby`、`set`、`get`、`del`、`expire`、`incrby`。

## `new-api` loadtest runtime stats route

路径：

```text
GET /debug/loadtest/runtime
```

第一阶段必做，但 collector 必须能在该 route 不可用时输出 `unavailable`，用于兼容旧 commit 的 baseline 采集。

启用条件：

```text
LOADTEST_RUNTIME_STATS_ENABLED=true
HOST=127.0.0.1 或等价 loopback bind
```

安全语义：

- 未启用时不注册路由，访问应得到 404。
- 服务未绑定 loopback 或无法确认 loopback 时，不注册路由并输出启动日志。
- 已注册时仍要校验 `RemoteAddr` 是 loopback；否则返回 403 或 404。
- 不依赖反向代理头判断来源。

返回 schema：

```json
{
  "schema_version": 1,
  "runtime": {
    "status": "ok",
    "goroutines": 0,
    "heap_alloc_bytes": 0,
    "heap_inuse_bytes": 0,
    "num_gc": 0,
    "pause_total_ns": 0,
    "gc_cpu_fraction": 0,
    "block_profile_rate": 1000,
    "mutex_profile_fraction": 5
  },
  "batch_update": {
    "status": "ok",
    "pending_user_quota": 0,
    "pending_token_quota": 0,
    "pending_used_quota": 0,
    "pending_channel_used_quota": 0
  },
  "quota_data": {
    "status": "unavailable",
    "reason": "not exposed yet"
  },
  "perf_metrics": {
    "status": "unavailable",
    "reason": "not exposed yet"
  }
}
```

没有实现某项时必须显式返回 `unavailable`，不得伪造。

## 环境配置

### 推荐端口

```text
new-api:       127.0.0.1:13080 或 127.0.0.1:18081
mock upstream: 127.0.0.1:19080
PostgreSQL:    127.0.0.1:15432
Redis:         127.0.0.1:16379
pprof:         127.0.0.1:8005
```

### `new-api` 推荐运行参数

```text
HOST=127.0.0.1
PORT=13080
PPROF_ADDR=127.0.0.1:8005
GOMAXPROCS=2
GOGC=100
ENABLE_PPROF=true
LOADTEST_RUNTIME_STATS_ENABLED=true
LOADTEST_PROFILE_BLOCK_RATE=1000
LOADTEST_PROFILE_MUTEX_FRACTION=5
BATCH_UPDATE_ENABLED=true
SQL_MAX_OPEN_CONNS=10
SQL_MAX_IDLE_CONNS=5
SQL_MAX_LIFETIME=60
```

必须保留：

```text
LogConsumeEnabled=true
DataExportEnabled=true
SubscriptionConcurrencyQueueCapacity=线上同等配置
```

## 场景矩阵

### S0：健康检查和鉴权基线

目的：排除网络、配置和鉴权错误。

seed 前请求：

- `GET /api/status`
- `GET /debug/loadtest/runtime`
- `GET http://127.0.0.1:8005/debug/pprof/goroutine?debug=1`

seed 后请求：

- `GET /v1/models` with `Bearer sk-loadtestsub`
- `POST /v1/chat/completions` with `Bearer sk-loadtestinvalid`

通过标准：

- `/api/status` 返回 200；
- runtime stats 和 pprof 在 loopback 可访问，或按配置输出明确 unavailable；
- `/v1/models` 使用有效 token 返回 200；
- invalid token 返回应用层 401；
- 不出现 DB/Redis 连接错误。

### S1：低并发 smoke

目的：验证完整链路和计费日志正确。

mock：

```text
first-token-delay=150ms
stream-duration=3s
chunk-interval=100ms
output-bytes=128
prompt-tokens=11
completion-tokens=17
```

client：

```text
concurrency=10
rps=5
duration=30s
max-requests=120
timeout=60s
```

组合：

- `/v1/responses` + `sk-loadtestsub`；
- `/v1/chat/completions` + `sk-loadtestsub`；
- `/v1/responses` + `sk-loadtestcompat`；
- `/v1/chat/completions` + `sk-loadtestcompat`。

通过标准：使用 S1/S2 gate。

### S2：固定点并发矩阵

目的：定位首个失败并发点。

mock：

```text
first-token-delay=100ms
stream-duration=1s
chunk-interval=100ms
output-bytes=128
prompt-tokens=11
completion-tokens=17
```

固定点：

```text
10,20,50,100,150,200,300
```

单点参数：

```text
duration=30s
max-requests=1000
timeout=90s
rps=0
ramp-step=concurrency
cooldown=2s
```

入口组合：

- responses-subscription；
- chat-subscription；
- responses-compat；
- chat-compat。

如果 300 通过，再追加：

```text
500,750,1000
```

1000 只是工具量程，不是必须通过的目标。

### S3：长连接占用场景

目的：模拟线上长流式请求对连接、goroutine、订阅并发和后结算的占用。

mock：

```text
first-token-delay=150ms
stream-duration=30s
chunk-interval=250ms
output-bytes=4096
prompt-tokens=11
completion-tokens=17
```

固定点：

```text
10,20,50
```

参数：

```text
duration=120s
timeout=180s
```

通过标准：使用 S3 gate。

### S4：错误和退款路径

目的：验证预扣、退款、retry 和错误日志路径。

mock：

```text
first-token-delay=100ms
stream-duration=1s
status-rate=429=0.05,502=0.01
seed=1
```

固定点：

```text
20,50,100
```

通过标准：使用 S4 gate。

### S5：大上下文 / 大输出场景

目的：定位 JSON、token 估算、request body、SSE buffer 和内存分配热点。

矩阵：

```text
input-bytes: 4KB, 64KB, 256KB
output-bytes: 4KB, 64KB
concurrency: 5, 10, 20
```

通过标准：使用 S5 gate。

## 业务不变量

每轮必须输出机器可读 `invariants`。

### 订阅计费

```text
expected_success_tokens = success * usage.total_tokens
actual_delta = after.subscription_token_used_sum - before.subscription_token_used_sum
passed = actual_delta == expected_success_tokens
```

如项目使用 quota 而非 raw token 结算，必须在 invariant 中同时输出 conversion ratio 和转换后期望值。

### compat 用户不回退钱包

```text
expected_compat_wallet_delta = 0
actual_user_quota_delta = before.compat_user_quota_sum - after.compat_user_quota_sum
actual_token_remain_delta = before.compat_token_remain_quota_sum - after.compat_token_remain_quota_sum
passed = actual_user_quota_delta == 0 && actual_token_remain_delta == 0
```

compat 用户仍然必须通过订阅扣费：

```text
expected_compat_subscription_tokens = success_compat * usage.total_tokens
actual_compat_subscription_delta = after.compat_subscription_token_used_sum - before.compat_subscription_token_used_sum
passed = actual_compat_subscription_delta == expected_compat_subscription_tokens
```

### 消费日志

```text
expected_consume_logs = success
actual_consume_logs_delta = after.logs_count - before.logs_count
passed = actual_consume_logs_delta >= expected_consume_logs
```

允许多日志时必须按 request id / path / token 解释差异。

### 失败退款

必须按请求维度判定，不能只用总量抵消：

```text
for each failed request in summary.requests where success=false:
  key = request.new_api_request_id
  passed = charges_by_request[key].pre_consume_status == "refunded"
        && charges_by_request[key].net_subscription_token_delta == 0
        && charges_by_request[key].compat_user_quota_delta == 0
        && charges_by_request[key].compat_token_remain_delta == 0
```

### quota_data

```text
passed = flush 后 quota_data 增量 >= success 相关统计项，且 pending 不持续增长
```

无法读取 pending 时输出 `unavailable`，不能判为通过。

### perf_metrics

```text
if perf_metrics runtime pending 或强制 flush 能力可用:
  passed = pending_or_flushed_perf_metric_samples > 0
        && stdout/log 中 perf metric upsert error 数量 == 0
else:
  status = unavailable，不参与通过判定
```

第一阶段不得要求 `perf_metrics_count 增量 > 0` 作为硬门禁，因为默认 flush 周期和 bucket 策略可能在短压测窗口内不落库。

### 日志输出

```text
passed = stdout_full_params_lines == 0
```

必须确认不再出现完整 `params={` 大日志。

## profile 与采样窗口

每个并发点必须采集：

1. before snapshot：压测前。
2. during samples：压测开始后进入稳态再采样。
3. after snapshot：cooldown 与 drain 后。

CPU profile 采集窗口：

```text
start = 压测开始后 min(5s, duration * 0.1)
duration = min(30s, max(10s, duration * 0.5))
end 必须早于 client 结束
```

heap/goroutine/block/mutex profile：

- before：压测前；
- during：CPU profile 同窗口内；
- after：drain 后。

`rss_peak_bytes`、`goroutines_peak`、`heap_alloc_peak_bytes` 来源于 during samples，采样间隔不超过 1s。

## 压测 SOP

### 0. 安全前置检查

1. 目标 URL 必须是 loopback。
2. PostgreSQL 数据库名必须包含 `loadtest`，`SQL_DSN` 必须是 `postgresql://` 或 `postgres://`。
3. Redis 地址必须是 loopback，默认端口 `16379`。
4. API Key 必须符合 `sk-loadtestsub`、`sk-loadtestcompat` 或 `sk-loadtestinvalid` 规则。
5. mock upstream 必须是 loopback。
6. new-api 主服务必须设置 `HOST=127.0.0.1`。
7. pprof 必须设置 `PPROF_ADDR=127.0.0.1:8005`。
8. 不允许使用生产域名、生产 DSN、生产 Redis 或真实 OpenAI key。

### 1. 准备目录

```text
.loadtest/baseline
.loadtest/candidate
.loadtest/concurrency-sweep
.loadtest/pprof
.loadtest/logs
.loadtest/config
.loadtest/reports
```

### 2. 启动本地 PostgreSQL / Redis

生成 `.loadtest/infra-status.json`。

通过标准：

```text
postgres_ready=true
redis_ready=true
```

### 3. 准备配置

复制：

```text
config.loadtest.yaml -> .loadtest/config/config.yaml
```

运行：

```text
loadtest-check-config --config .loadtest/config/config.yaml --out-env .loadtest/config/new-api.env --out-run-context .loadtest/run-context.base.json
```

通过标准：命令 exit `0`，最终配置没有生产地址。

### 4. 启动 mock upstream

```text
loadtest-mock-openai --addr 127.0.0.1:19080 --first-token-delay 150ms --stream-duration 3s --chunk-interval 100ms --output-bytes 128 --prompt-tokens 11 --completion-tokens 17 --stats-out .loadtest/mock-stats.json
```

### 5. 启动 new-api 执行 schema bootstrap

必须使用 `.loadtest/config/new-api.env` 中的环境变量。

```text
loadtest-run-new-api --binary .loadtest/bin/new-api --env .loadtest/config/new-api.env --work-dir .loadtest/runtime/new-api --pid-file .loadtest/new-api.pid --bootstrap-only
```

`--bootstrap-only` 必须用干净环境启动 `new-api`，等待数据库迁移完成后退出或终止进程；该步骤只负责创建 schema，不做压测。

### 6. Seed 数据

运行：

```text
loadtest-seed --config .loadtest/config/config.yaml --run-context .loadtest/run-context.base.json --out .loadtest/baseline/seed.json --out-run-context .loadtest/run-context.seeded.json
```

### 7. 启动 new-api 压测实例

```text
loadtest-run-new-api --binary .loadtest/bin/new-api --env .loadtest/config/new-api.env --work-dir .loadtest/runtime/new-api --pid-file .loadtest/new-api.pid
```

启动器必须使用干净环境、避开生产 `.env`，并把 `new-api` 进程 ID 写入 `.loadtest/new-api.pid`；`loadtest-collect` 只通过该 pid file 采集进程指标。`--binary` 必须指向已构建的 `new-api` 可执行文件；启动器不得从 `PATH` 猜测二进制，不得在仓库根目录运行服务。

### 8. seed 后鉴权健康检查

```text
GET http://127.0.0.1:<port>/v1/models Authorization: Bearer sk-loadtestsub
POST http://127.0.0.1:<port>/v1/chat/completions Authorization: Bearer sk-loadtestinvalid
```

### 9. 采集 before metrics

```text
loadtest-collect --config .loadtest/config/config.yaml --pid-file .loadtest/new-api.pid --run-context .loadtest/s2-responses-subscription/run-context.json --out-snapshot .loadtest/baseline/metrics-before.json
```

### 10. 执行场景

第一阶段必须可机器运行 S1、S2、S3、S4、S5。第一份最小 baseline 报告至少包含 S1 和 S2；未产出 S3、S4、S5 baseline 前，不得据此给出完整代码级优化排序，只能给出「短流式常规并发」结论。

### 11. usage / batch / data export drain

等待至少：

```text
BatchUpdateInterval + 2s
```

如果 `DataExportInterval` 很长，不强行等待完整周期；必须记录 quota data cache pending 状态或 `unavailable`。
`quota_data` 和 `perf_metrics` 的 drain/flush 契约：

- `quota_data`：如果 pending 状态可读，drain 后必须不持续增长；如果 pending 不可读，输出 `unavailable`，不作为硬门禁。
- `perf_metrics`：优先读取 runtime pending 或调用 loadtest 专用强制 flush；如果二者都不可用，输出 `unavailable`，不作为硬门禁；仍必须检查日志中没有 perf metric upsert error。

### 12. 采集 after metrics

```text
loadtest-collect --config .loadtest/config/config.yaml --pid-file .loadtest/new-api.pid --run-context .loadtest/s2-responses-subscription/run-context.json --summary .loadtest/baseline/c100-summary.json --before .loadtest/baseline/metrics-before.json --mock-stats-delta .loadtest/concurrency-sweep/responses-subscription/c100-mock-stats-delta.json --out-snapshot .loadtest/baseline/metrics-after.json --out-diff .loadtest/baseline/metrics-diff.json --wait-drain
```

额外保存：

- pprof CPU；
- pprof heap；
- pprof goroutine；
- pprof block；
- pprof mutex；
- db-counts；
- redis-info；
- server log excerpt；
- process snapshot。

### 13. 清理

停止：

- `new-api`；
- mock upstream；
- PostgreSQL；
- Redis。

确认端口关闭：

```text
15432
16379
13080 或 18081
19080
8005
```

## 缓存、warmup 与可比性

默认对比模式是 cold-fresh：

1. baseline 和 candidate 各自使用全新的 PostgreSQL 数据库、Redis DB 和 `new-api` 进程。
2. 每个 role 执行相同 seed。
3. 每个 scenario 的第一个小 warmup run 不计入报告：`concurrency=2`、`max-requests=10`。
4. warmup 后重置 metrics before，但不重启进程，保留 JIT 不相关的 Go 热路径缓存状态。
5. 每个并发点之间执行 cooldown 和 drain，但不重启服务；artifact 中必须记录 `cache_mode="cold-fresh-role,warm-per-point"`。
6. 若改为热缓存模式，report 必须标记 `cache_mode` 不同，并禁止与 cold-fresh 结果直接比较。

## `loadtest-report` 契约

### 单轮报告命令

```text
loadtest-report \
  --sweep .loadtest/concurrency-sweep/responses-subscription.json \
  --out .loadtest/reports/responses-subscription.md
```

### 对比报告命令

```text
loadtest-report \
  --baseline-sweep .loadtest/baseline/s2-responses-subscription.json \
  --candidate-sweep .loadtest/candidate/s2-responses-subscription.json \
  --baseline-metrics .loadtest/baseline/metrics-after.json \
  --candidate-metrics .loadtest/candidate/metrics-after.json \
  --thresholds .loadtest/config/regression-thresholds.json \
  --out .loadtest/reports/s2-responses-subscription-compare.md \
  --fail-on-regression
```

要求：

- baseline/candidate 的 `comparison_config_hash` 必须一致；不一致时 exit code 为 `2`。
- 输入 artifact schema version 不兼容时 exit code 为 `2`。
- `--fail-on-regression` 且任一 regression gate 失败时 exit code 为 `2`。
- 缺失可选 pprof 时报告 `unavailable`，不得失败；缺失 summary/diff 必需字段时失败。

### 默认 regression thresholds

```json
{
  "latency_p95_regression_ratio": 1.10,
  "ttft_p95_regression_ratio": 1.10,
  "error_rate_absolute_increase": 0.001,
  "postgres_writes_per_success_ratio": 1.10,
  "redis_commands_per_success_ratio": 1.10,
  "rss_peak_bytes_ratio": 1.15,
  "goroutines_peak_ratio": 1.15
}
```

Better 判定：

- latency、TTFT、错误率、写放大、命令放大、RSS、goroutine 越低越好。
- highest passed concurrency 越高越好。
- first failed concurrency 越高越好；`null` 表示未失败，优于任何数值失败点。

## 报告格式

### 单轮报告

每轮生成 Markdown：

```markdown
# new-api 压测报告 YYYY-MM-DD

## 环境

- run_id:
- commit:
- comparison_config_hash:
- seed_output_hash:
- mock_hash:
- cache_mode:
- Go:
- OS:
- GOMAXPROCS:
- GOGC:
- DB:
- Redis:
- pprof:
- block_profile_rate:
- mutex_profile_fraction:

## Profile

- scenario:
- path:
- token profile:
- first-token-delay:
- stream-duration:
- chunk-interval:
- input-bytes:
- output-bytes:

## Client Summary

| Concurrency | Passed | Total | Success | Errors | RPS | Lat P95 | TTFT P95 | Max In-flight |
|---:|:---:|---:|---:|---:|---:|---:|---:|---:|

## PostgreSQL Delta

| Metric | Before | After | Delta |
|---|---:|---:|---:|

## Redis Delta

| Metric | Before | After | Delta |
|---|---:|---:|---:|

## Business Invariants

| Check | Expected | Actual | Pass |
|---|---:|---:|:---:|

## Pprof Findings

- CPU top:
- Heap top:
- Goroutine count:
- Block profile:
- Mutex profile:

## 结论

- 首个失败并发点：
- 主要失败原因：
- 结论适用范围：
- 下一步代码优化建议：
```

### baseline vs candidate 对比报告

```markdown
# new-api 压测对比报告 YYYY-MM-DD

## 对比对象

| Role | Commit | Comparison Config Hash | Seed Output Hash | Mock Hash | Cache Mode |
|---|---|---|---|---|---|
| baseline |  |  |  |  |  |
| candidate |  |  |  |  |  |

## 固定条件

- scenario:
- path:
- token profile:
- mock profile:
- points:

## 核心结果

| Metric | Baseline | Candidate | Delta | Better |
|---|---:|---:|---:|:---:|
| highest_passed_concurrency |  |  |  |  |
| first_failed_concurrency |  |  |  |  |
| p95_latency_at_100 |  |  |  |  |
| p95_ttft_at_100 |  |  |  |  |
| error_rate_at_100 |  |  |  |  |
| postgres_writes_per_success |  |  |  |  |
| redis_commands_per_success |  |  |  |  |
| rss_peak_bytes |  |  |  |  |
| goroutines_peak |  |  |  |  |

## Regression Gate

- 是否出现新错误类型：
- 是否业务不变量失败：
- 是否 p95 latency 回退超过阈值：
- 是否 PostgreSQL/Redis 写放大：
```

## 归因规则

### DB 先成为瓶颈

特征：active connection 堆积、slow SQL 增多、lock wait 增多、`logs`/`user_subscriptions`/`subscription_pre_consume_records` 写入慢。

下一步优先：

1. 消费日志异步批量写入；
2. 订阅预扣记录压缩或批量化；
3. `quota_data` 锁外批量 upsert；
4. 减少事务内查询；
5. 检查索引与 `pg_stat_statements` Top query。

### Redis 先成为瓶颈

特征：Redis ops/sec、`eval`、`evalsha`、hash 命令明显上升，TTFT 随并发上升。

下一步优先：

1. 订阅并发租约轮询优化；
2. token/user cache 命中率；
3. rate limit 命令数；
4. perf metrics Redis 写入；
5. channel affinity 访问频率。

### CPU profile 指向 JSON / token 估算

下一步优先：

1. `GetTokenCountMeta`；
2. request conversion；
3. response stream parser；
4. 避免重复 marshal / unmarshal；
5. 减少 DTO 复制。

### Heap / GC 上升

下一步优先：

1. 大请求 body storage；
2. SSE buffer；
3. `CombineText`；
4. full response capture；
5. `Other` map 和日志字段构造。

### 请求结束时出现长尾

下一步优先：

1. 后结算；
2. 消费日志写入；
3. data export；
4. batch update；
5. perf metrics flush。

## 第一阶段交付范围

第一阶段交付最小但完整的可运行闭环：

1. `localguard`；
2. `config.loadtest.yaml` 和 `loadtest-check-config`；
3. `loadtest-mock-openai`；
4. `loadtest-client`；
5. `loadtest-concurrency-sweep`；
6. `loadtest-seed`；
7. `loadtest-collect`；
8. `loadtest-run-new-api`；
9. `new-api` loopback bind 与 loadtest runtime stats route；
10. `loadtest-report`；
11. SOP 文档和报告模板。

第一阶段必须可运行 S1、S2、S3、S4、S5；第一份最小 baseline 至少要求 S1、S2。未运行 S3、S4、S5 前，不能给出覆盖长连接、错误退款和大 payload 的完整优化排序。

## 测试类别

下一步实现计划必须覆盖：

1. `localguard` 单元测试：拒绝公网 URL、生产 DSN、非 loopback Redis、含连字符的 loadtest API Key。
2. mock SSE fixture 测试：Chat 与 Responses 输出 exact event/data 形态。
3. client parser 测试：TTFT、`[DONE]`、usage、缺失 done、错误 status 分类。
4. sweep 判定测试：S2 全 200 通过、S3/S5 资源 gate、S4 确定性错误通过。
5. seed 幂等测试：重复执行对象数量不膨胀，token/channel/ability 指向 mock，两个用户都有有效订阅。
6. collector schema 测试：资源可用与 unavailable 分支都可生成稳定 JSON。
7. runtime stats route 测试：未启用 404、非 loopback 403/404、启用后返回 runtime 字段。
8. report 测试：baseline/candidate 对比能输出首个失败并发点和 regression gate。

## 验收标准

1. 任意命令尝试连接非本地 URL、非 loadtest 数据库或非本地 Redis 时必须失败。
2. loadtest API Key 规则与 `TokenAuth` 解析一致，subscription 与 compat 能稳定命中不同 token。
3. `new-api` loadtest 运行时主服务和 pprof 均绑定 loopback。
4. smoke 场景可以稳定通过，并生成完整 client summary 和 metrics before/after。
5. concurrency sweep 能输出每个固定点的独立结果。
6. 压测报告能指出首个失败并发点。
7. baseline/candidate 对比报告能比较同场景、同 `comparison_config_hash` 下的优化前后结果。
8. 业务不变量能自动校验，并以机器可读 JSON 输出。
9. pprof 或 runtime stats 不可用时必须显示 `unavailable`，不得伪造。
10. 清理后所有压测端口关闭。
11. 压测过程中不修改生产配置，不访问生产资源。
12. `DataExportEnabled`、`LogConsumeEnabled` 和订阅并发队列保持开启。
13. compat 用户请求不扣减用户钱包 quota 或 token remain quota，只扣减其有效订阅。

## 后续计划入口

规格批准后，下一步应编写实现计划：

```text
docs/superpowers/plans/YYYY-MM-DD-new-api-local-loadtest.md
```

实现计划必须直接在主分支开发，不使用 worktree。计划按低冲突文件边界拆分为：

1. localguard + config 校验；
2. mock upstream；
3. load client + concurrency sweep；
4. seed；
5. metrics collector；
6. loopback bind + pprof addr + runtime stats route；
7. report + SOP 文档；
8. 集成验证。
