# GPT 安全警告管理与重复请求拦截规格说明

## 背景

当前 GPT abuse 机制已经能够记录上游安全警告，并在用户当日警告次数达到套餐阈值后暂停服务。但现有能力仍有两个缺口：

1. 管理员缺少统一面板，无法快速查看每个用户的警告触发情况、封禁状态，也无法安全地手动解封或重置当日警告次数。
2. 部分 harness（例如用户反馈的 Codex Desktop）可能在收到安全警告后静默原样重试，导致同一请求在短时间内反复触发上游警告，快速耗尽用户当日警告次数。

本规格定义两项能力：

- **GPT 安全警告管理面板**：提供用户维度聚合、详情查看、手动解封、重置当日警告次数、重复拦截记录查看。
- **已警告请求重复拦截**：对刚触发上游安全警告的请求做短期指纹缓存，命中原样重试时本地拦截，不再请求上游。

## 目标

### 管理面板

- 管理员可以按时间窗口查看用户维度的 GPT 安全警告统计。
- 管理员可以查看某个用户的警告明细，包括 `request_id`、`upstream_request_id`、模型、渠道、`kind`、`severity`、上游 warning detail。
- 管理员可以手动解除用户当前 active suspension。
- 管理员可以重置某个用户当日有效警告次数，但历史日志必须保留且可审计。
- 管理员可以查看某个用户因原样重试被本地拦截的记录。

### 重复请求拦截

- 用户请求首次触发上游 GPT 安全警告后，网关在短时间内缓存该请求的安全指纹。
- 同一用户、同一 token、同一 client-facing endpoint、同一原始 relay mode、同一原始请求模型、同一语义请求体在 TTL 内再次出现时，本地直接拦截。
- 本地拦截不请求上游，不增加上游警告次数，也不增加 `count_eligible` warning count。
- 本地拦截错误需要和原始上游警告明显区分，提示用户这是「已触发安全警告请求的原样重试」。

## 非目标

- 不保存请求体、prompt、响应正文或完整 SSE event。
- 不删除历史 `gpt_abuse_signal_logs`。
- 不通过批量修改历史 `count_eligible` 来实现重置次数。
- 不把本地重复拦截当作新的上游 warning 计数。
- 不试图兼容 Codex Desktop 内部闭源实现，仅在网关侧防止原样重试继续打上游。

## 现有基础

当前已有：

- `model.GPTAbuseSignalLog`
  - 表：`gpt_abuse_signal_logs`
  - 记录每次 GPT abuse signal。
  - 包含用户、token、渠道、请求 ID、上游请求 ID、endpoint、模型、`source`、`kind`、`severity`、`error_code`、`error_type`、`count_eligible`、`extra`。
- `model.GPTAbuseUserSuspension`
  - 表：`gpt_abuse_user_suspensions`
  - 记录用户 active / expired / cleared suspension。
  - active suspension 通过 `active_user_id` 唯一约束保证单用户同一时间只有一个 active suspension。
- `CountGPTAbuseSignalsForUser(userID, start, end)`
  - 当前按时间窗口统计 `count_eligible=true` 的 warning。
- `GetActiveGPTAbuseSuspension(userID, now)`
  - 查询用户当前 active suspension。
- `UpsertGPTAbuseSuspension(...)`
  - 当用户有效 warning count 达到阈值时写入或更新 active suspension。

当前缺少：

- Admin 聚合查询接口。
- 用户 warning 明细查询接口。
- 手动 clear suspension 接口。
- 可审计的 warning count reset 机制。
- 已触发 warning 请求的短期重复拦截。

## 数据模型

### 保留现有表

继续使用：

```text
gpt_abuse_signal_logs
gpt_abuse_user_suspensions
```

历史 warning 日志仍作为原始审计来源，不删除、不批量改写。

### 新增 `gpt_abuse_warning_resets`

用途：记录管理员对某个用户某个自然日 warning count 的 reset 操作。

模型：

```go
type GPTAbuseWarningReset struct {
    Id                  int    `json:"id" gorm:"primaryKey"`
    UserId              int    `json:"user_id" gorm:"not null;index;index:idx_gpt_abuse_reset_user_window,priority:1"`
    WindowStart         int64  `json:"window_start" gorm:"bigint;not null;index;index:idx_gpt_abuse_reset_user_window,priority:2"`
    WindowEnd           int64  `json:"window_end" gorm:"bigint;not null;index"`
    ResetAt             int64  `json:"reset_at" gorm:"bigint;not null;index"`
    ResetBy             int    `json:"reset_by" gorm:"default:0;index"`
    PreviousRawCount    int    `json:"previous_raw_count" gorm:"default:0"`
    PreviousCount       int    `json:"previous_count" gorm:"default:0"`
    CutoffSignalLogID   int    `json:"cutoff_signal_log_id" gorm:"default:0;index"`
    Reason              string `json:"reason" gorm:"type:varchar(255);default:''"`
    CreatedAt           int64  `json:"created_at" gorm:"bigint"`
}
```

字段语义：

- `PreviousRawCount`：reset 前当前窗口的原始 warning 数。
- `PreviousCount`：reset 前当前窗口的有效 warning 数。
- `CutoffSignalLogID`：reset 时当前窗口内已纳入 reset 的最大 `gpt_abuse_signal_logs.id`。
- `WindowEnd`：保存当时计算出的窗口结束时间，避免后续因时区或窗口规则变化导致审计口径漂移。
- reset 记录是 append-only 审计事实，禁止更新或删除；如需修正原因，应新增管理操作记录，不修改 reset 记录本身。

计数规则：

1. 确定当前自然日窗口：`window_start` / `window_end`。
2. 查询该用户该窗口内最新 reset，排序必须为：

```text
reset_at DESC, id DESC
```

3. 若存在 reset，则 effective count 统计：

```text
created_at >= window_start
AND created_at < window_end
AND id > latest_reset.cutoff_signal_log_id
AND count_eligible = true
```

4. 若不存在 reset，则 effective count 沿用窗口统计：

```text
created_at >= window_start
AND created_at < window_end
AND count_eligible = true
```

使用 `id > cutoff_signal_log_id` 而不是 `created_at > reset_at`，避免秒级时间相等导致 reset 后同一秒新 warning 被错误排除。

### Raw count 与 effective count

必须拆分计数函数，避免旧语义混淆：

```go
CountGPTAbuseSignalsForUserRaw(userID int, start int64, end int64) (int, error)
CountEffectiveGPTAbuseSignalsForUser(userID int, start int64, end int64) (int, *GPTAbuseWarningReset, error)
```

使用规则：

- 管理面板的 `warning_count` 使用 raw count。
- 管理面板的 `effective_warning_count` 使用 effective count。
- `GetSubscriptionSelfSummary` 必须使用 effective count。
- `applyGPTAbuseLimit` 的封禁判断必须使用 effective count。
- 用户自助 summary 中的 warning count 也必须使用 effective count。

### 新增 `gpt_abuse_repeat_block_logs`

用途：记录本地拦截的原样重试请求。

模型：

```go
type GPTAbuseRepeatBlockLog struct {
    Id                            int    `json:"id" gorm:"primaryKey"`
    CreatedAt                     int64  `json:"created_at" gorm:"bigint;index"`
    UserId                        int    `json:"user_id" gorm:"not null;index"`
    Username                      string `json:"username" gorm:"type:varchar(255);default:''"`
    TokenId                       int    `json:"token_id" gorm:"default:0;index"`
    TokenName                     string `json:"token_name" gorm:"type:varchar(255);default:''"`
    RequestId                     string `json:"request_id" gorm:"type:varchar(128);default:'';index"`
    Endpoint                      string `json:"endpoint" gorm:"type:varchar(255);default:''"`
    RelayMode                     int    `json:"relay_mode" gorm:"default:0"`
    RequestedModel                string `json:"requested_model" gorm:"type:varchar(255);default:''"`
    BodyFingerprint               string `json:"-" gorm:"type:varchar(128);default:'';index"`
    FirstWarningLogId             int    `json:"first_warning_log_id" gorm:"default:0;index"`
    FirstWarningAt                int64  `json:"first_warning_at" gorm:"bigint;default:0"`
    FirstWarningRequestId         string `json:"first_warning_request_id" gorm:"type:varchar(128);default:''"`
    FirstWarningUpstreamRequestId string `json:"first_warning_upstream_request_id" gorm:"type:varchar(128);default:''"`
    FirstWarningSource            string `json:"first_warning_source" gorm:"type:varchar(64);default:''"`
    FirstWarningKind              string `json:"first_warning_kind" gorm:"type:varchar(64);default:''"`
    FirstWarningSeverity          string `json:"first_warning_severity" gorm:"type:varchar(16);default:''"`
    ChannelId                     int    `json:"channel_id" gorm:"default:0;index"`
    ChannelName                   string `json:"channel_name" gorm:"type:varchar(255);default:''"`
    ChannelType                   int    `json:"channel_type" gorm:"default:0"`
}
```

说明：

- `BodyFingerprint` 保存完整 HMAC，用于服务端精确关联，禁止直接返回给前端。
- API DTO 只能返回 `body_fingerprint_prefix`，例如前 12 或 16 个 hex 字符。
- 重复拦截不参与 warning count。
- channel / upstream model 只用于观测，不参与缓存命中，避免跨 channel retry 时 miss。

### 迁移与索引

两个新 model 必须加入 `model/main.go` 的 AutoMigrate。

索引建议：

```text
gpt_abuse_warning_resets: (user_id, window_start, reset_at, id)
gpt_abuse_repeat_block_logs: (user_id, created_at)
gpt_abuse_repeat_block_logs: (body_fingerprint)
```

禁止使用数据库特有类型或特定 SQL：

- 不使用 JSONB。
- 不使用 PostgreSQL `DISTINCT ON`。
- 不使用 MySQL 专有函数。
- 不使用数据库日期函数推导自然日窗口。

## Repeat Block 指纹设计

### 指纹基准

必须新增统一的 `BuildRepeatBlockFingerprint` 流程，并在请求解析后、任何 relay mode / request path / model conversion 前捕获 client-facing 指纹上下文。

捕获内容：

```text
user_id
token_id
client_endpoint_path
client_relay_mode
client_origin_model
canonical_or_raw_body_digest
```

必须把捕获结果存入 `RelayInfo` 或 gin context，例如：

```text
GPTAbuseRepeatBlockFingerprint
GPTAbuseRepeatBlockFingerprintPrefix
GPTAbuseRepeatBlockClientEndpoint
GPTAbuseRepeatBlockClientRelayMode
GPTAbuseRepeatBlockClientModel
```

读缓存、写缓存、repeat block log 必须复用同一个已捕获 fingerprint，不能在写缓存时使用转换后的 `/v1/responses`、上游 model 或内部 relay mode。

原因：`chatCompletionsViaResponses` 会把 `RelayInfo` 临时改成 Responses 路径。如果读缓存使用原始 `/v1/chat/completions`，写缓存使用转换后的 `/v1/responses`，同一客户端重试会永远 miss。

### 请求体摘要

JSON 请求应使用 canonical JSON HMAC：

- 对象 key 排序。
- 保留数组顺序。
- 保留字符串、布尔、null 语义。
- 数值按解析后的 JSON number 规范化。
- canonical body 只用于 HMAC 输入，不落库、不写日志。

解析失败或非 JSON 请求回退 raw bytes HMAC。

测试必须覆盖：

- JSON 字段顺序不同但语义相同，应命中同一 fingerprint。
- 请求内容实际变化，应产生不同 fingerprint。

### HMAC

使用：

```text
HMAC-SHA256(secret, fingerprint_input)
```

不得使用裸 SHA256。

缓存 key：

```text
gpt_abuse:warned_request:v1:{user_id}:{token_id}:{endpoint_path}:{client_relay_mode}:{client_origin_model}:{hmac}
```

endpoint 使用 client-facing `URL.Path`，不包含 query。若未来需要 query 参与，必须显式列出白名单 query 参数。

## Repeat Block 缓存

### 缓存内容

Redis value：

```json
{
  "first_warning_log_id": 16,
  "user_id": 1958,
  "token_id": 1820,
  "request_id": "...",
  "upstream_request_id": "...",
  "source": "sse_response_failed",
  "kind": "cyber_policy",
  "severity": "high",
  "created_at": 1780633007,
  "channel_id": 4,
  "channel_name": "Sub2API",
  "channel_type": 1
}
```

不得保存：

- 请求体。
- prompt。
- 响应正文。
- 完整 SSE event。
- 完整 HMAC 到前端 DTO。

### TTL

默认 TTL：

```text
15 分钟
```

配置项：

```text
GPTAbuseRepeatBlockEnabled=true
GPTAbuseRepeatBlockTTLSeconds=900
GPTAbuseRepeatBlockRequireRedis=false
```

### Redis 与内存兜底

读策略：

1. feature disabled：直接 miss。
2. Redis 可用：先查 Redis。
3. Redis 读失败：记录 warn，然后查内存 fallback。
4. Redis miss：再查内存 fallback，兼容 Redis 写失败后已写内存的情况。
5. Redis 与内存都不可用：fail open。

写策略：

1. feature disabled：不写。
2. Redis 可用：`SET key value EX ttl NX`。
3. Redis 写失败：记录 warn，然后写内存 fallback。
4. 内存 fallback 写失败：fail open，仅记录 warn。

`GPTAbuseRepeatBlockRequireRedis=true` 语义：

- 禁用内存 fallback。
- Redis 故障时 fail open，并记录 warn。
- 该配置用于多实例环境避免本机内存造成不一致，而不是用于 fail closed。

## Repeat Block 写入与检查时机

### 检查时机

必须在每次 upstream attempt 前检查，而不是只在 retry loop 外检查一次。

挂点要求：

- 在 `controller.Relay` 的每次 retry attempt 中。
- 完成 channel 选择、模型映射、suspension 检查后。
- 进入任何 `relayHandler` / `geminiRelayHandler` / `WssHelper` / `adaptor.DoRequest` 前。
- 命中后直接返回本地 `NewAPIError`，跳出 retry loop。
- 不进入 `processChannelError`。
- 不请求上游。

对 `chatCompletionsViaResponses` 这类 helper 内部自行 `DoRequest` 的路径，也必须复用同一 pre-upstream gate 或保证外层每次 attempt gate 已覆盖。

### 写入时机

repeat-block cache 写入必须发生在 signal recording 内部。

要求：

- 只有 `RecordGPTAbuseSignalLog` 成功插入且拿到 `log.Id` 后才写 cache。
- DB 失败、dedupe 未插入或无法取得 `log.Id` 时不写 cache，只记录 warn。
- SSE `response.failed` / `response.error` / `response.metadata` 必须在事件处理回调中完成持久化与 cache 写入，不依赖 handler 最终返回 error。
- 本地 repeated block 不再调用 `RecordGPTAbuseSignal`，避免增加 warning count。

## 本地重复拦截错误

HTTP status：

```text
400 Bad Request
```

OpenAI-compatible body：

```json
{
  "error": {
    "message": "Repeated request blocked locally: this exact request recently triggered an upstream GPT safety warning. The request was not sent upstream again. Please review and change the request content before retrying. request_id={current_request_id}; first_warning_log_id={first_warning_log_id}; first_warning_at={first_warning_at}",
    "type": "invalid_request_error",
    "code": "gpt_abuse_repeated_warning_request"
  }
}
```

新增错误码：

```go
ErrorCodeGPTAbuseRepeatedWarningRequest = "gpt_abuse_repeated_warning_request"
```

必须使用：

```go
types.ErrOptionWithSkipRetry()
```

说明：

- 不使用 HTTP 429，避免 harness 按限流继续重试。
- 不复用 `cyber_policy`，避免误认为新的上游 warning。
- message 使用英文，便于 Codex 类客户端展示。
- message 可以包含当前 `request_id`、`first_warning_log_id`、`first_warning_at`，但不得包含 prompt、请求体、完整 SSE event 或上游原始错误正文。

## Admin 后端 API

所有接口使用 AdminAuth。写操作建议沿用敏感管理接口的限流策略，并记录 admin id、username、IP 与 reason。

统一前缀：

```text
/api/gpt-abuse
```

### 用户聚合列表

```http
GET /api/gpt-abuse/users
```

查询参数：

```text
start_timestamp
end_timestamp
keyword
user_id
status=all|active_suspended|warning_only
kind
severity
source
limit
offset
sort_by=warning_count|effective_warning_count|latest_warning_at|user_id
sort_order=asc|desc
```

响应 item 必须包含：

```json
{
  "user_id": 1958,
  "username": "zlqnldnzgifelu",
  "user_email": "",
  "warning_count": 5,
  "effective_warning_count": 5,
  "daily_limit": 5,
  "remaining_warning_count": 0,
  "high_count": 5,
  "medium_count": 0,
  "max_severity": "high",
  "latest_warning_at": 1780633007,
  "latest_kind": "cyber_policy",
  "latest_source": "sse_response_failed",
  "latest_requested_model": "gpt-5.5",
  "latest_upstream_model": "gpt-5.5",
  "latest_channel_id": 4,
  "latest_channel_name": "Sub2API",
  "suspension_status": "active",
  "active_suspension": {
    "id": 1,
    "reason": "gpt_abuse_daily_limit",
    "suspended_until": 1780684800,
    "daily_count": 5,
    "daily_limit": 5
  },
  "last_reset_at": 0,
  "last_reset_by": 0,
  "repeat_block_count": 3,
  "latest_repeat_block_at": 1780633300
}
```

无 active suspension 时：

```json
{
  "suspension_status": "none",
  "active_suspension": null
}
```

`remaining_warning_count = max(daily_limit - effective_warning_count, 0)`。

### 用户 warning 明细

```http
GET /api/gpt-abuse/users/:id/logs
```

查询参数：

```text
start_timestamp
end_timestamp
source
kind
severity
count_eligible=true|false|all
limit
offset
```

返回每条 `gpt_abuse_signal_logs`，包含 `extra.upstream_warning`。前端展示 `extra.raw_error` 时必须折叠并截断。

### 手动解封

```http
POST /api/gpt-abuse/users/:id/clear-suspension
```

请求：

```json
{
  "reason": "manual_review"
}
```

规则：

- `reason` 最大 255 字符。
- 无 active suspension 时幂等成功，返回 `had_active_suspension=false`。
- 有 active suspension 时，在主库事务中更新：
  - `status = cleared`
  - `active_user_id = nil`
  - `cleared_at = now`
  - `cleared_by = admin_id`

响应：

```json
{
  "success": true,
  "data": {
    "user_id": 1958,
    "had_active_suspension": true,
    "suspension_cleared": true,
    "cleared_suspension_id": 1
  }
}
```

### 重置当日 warning count

```http
POST /api/gpt-abuse/users/:id/reset-warnings
```

请求：

```json
{
  "reason": "manual_review",
  "clear_suspension": true
}
```

说明：

- `window_start` 由服务端使用 `GPTAbuseDayWindow(now)` 计算，不接受客户端传入，避免重置历史窗口或时区不一致。
- `reason` 最大 255 字符。
- reset marker 插入、可选 clear suspension 必须在同一个 `model.DB.Transaction` 内完成。

响应：

```json
{
  "success": true,
  "data": {
    "reset_id": 10,
    "user_id": 1958,
    "window_start": 1780588800,
    "window_end": 1780675200,
    "reset_at": 1780634000,
    "previous_raw_count": 5,
    "previous_effective_count": 5,
    "effective_warning_count": 0,
    "cutoff_signal_log_id": 16,
    "had_active_suspension": true,
    "suspension_cleared": true,
    "cleared_suspension_id": 1
  }
}
```

### 重复请求拦截明细

```http
GET /api/gpt-abuse/users/:id/repeat-blocks
```

查询参数：

```text
start_timestamp
end_timestamp
limit
offset
```

响应 item：

```json
{
  "id": 1,
  "created_at": 1780633300,
  "user_id": 1958,
  "token_id": 1820,
  "token_name": "1",
  "request_id": "...",
  "endpoint": "/v1/responses",
  "requested_model": "gpt-5.5",
  "body_fingerprint_prefix": "a1b2c3d4e5f6",
  "first_warning_log_id": 16,
  "first_warning_at": 1780633007,
  "first_warning_request_id": "...",
  "first_warning_upstream_request_id": "...",
  "first_warning_source": "sse_response_failed",
  "first_warning_kind": "cyber_policy",
  "first_warning_severity": "high",
  "channel_id": 4,
  "channel_name": "Sub2API"
}
```

## 前端管理面板

新增页面：

```text
/gpt-abuse
```

模块路径：

```text
web/default/src/features/gpt-abuse/
```

建议文件结构：

```text
features/gpt-abuse/
  api.ts
  types.ts
  index.tsx
  components/
    gpt-abuse-user-table.tsx
    gpt-abuse-log-drawer.tsx
    gpt-abuse-reset-dialog.tsx
    gpt-abuse-clear-suspension-dialog.tsx
    gpt-abuse-repeat-block-table.tsx
  lib/
    filters.ts
    format.ts
```

路由：

```text
web/default/src/routes/_authenticated/gpt-abuse/index.tsx
```

TanStack Router：

```ts
createFileRoute('/_authenticated/gpt-abuse/')
```

`beforeLoad` 必须按现有 admin 页面模式检查 `ROLE.ADMIN`，非管理员跳转 `/403`。

Sidebar：

- module key：`gpt_abuse`
- URL：`/gpt-abuse`
- 映射：`{ section: 'admin', module: 'gpt_abuse' }`
- 默认开启。
- 需要更新：
  - `use-sidebar-data.ts`
  - `use-sidebar-config.ts`
  - `DEFAULT_SIDEBAR_MODULES`
  - `URL_TO_CONFIG_MAP`
  - 侧边栏模块管理相关 i18n 文案。

### 搜索参数

路由 search schema 必须覆盖：

```text
start_timestamp
end_timestamp
keyword
user_id
status
kind
severity
source
limit
offset
sort_by
sort_order
```

行为：

- 默认当天数据可由后端默认，前端也可在 URL 中显式填充，但必须与后端窗口一致。
- 筛选条件变化时重置 `offset=0`。
- 分页、排序、筛选都进入 URL search，便于刷新和分享。

### 页面布局

顶部 Alert：

```text
查看 GPT 安全警告触发情况、封禁状态和本地重复请求拦截记录。重置次数不会删除历史日志，只会从当前时间重新计算当日有效警告次数。本地重复拦截由网关完成，不会再次请求上游，也不会增加当日警告数或有效警告数。
```

筛选区：

- 时间范围。
- 用户 ID / 用户名 / 邮箱。
- 状态：全部、已封禁、有警告。
- `kind`。
- `severity`。
- `source`。

用户聚合表字段：

- 用户 ID。
- 用户名。
- 邮箱。
- 当日警告数。
- 有效警告数。
- 阈值。
- 剩余次数。
- 最高严重级别。
- 最近触发时间。
- 最近模型。
- 最近渠道。
- 封禁状态。
- 重复拦截次数。
- 操作。

操作：

- 查看详情。
- 解封：仅 active suspension 时可用。
- 重置次数。
- 重置并解封：可作为 reset dialog checkbox 实现，不必单独提供重复入口。

### 详情 Drawer

Tab：

1. 上游警告。
2. 本地重复拦截。
3. 管理操作记录（本期可至少展示 `last_reset_at` / `last_reset_by`，完整操作记录可后续扩展）。

上游警告表字段：

- 时间。
- endpoint。
- requested model。
- upstream model。
- source。
- kind。
- severity。
- status code。
- error code。
- count eligible。
- request id。
- upstream request id。
- extra warning detail。

`extra.raw_error` 展示要求：

- 默认折叠。
- 表格中只显示摘要。
- Drawer 内放在固定高度滚动区域。
- 展示时截断，例如最多 500～1000 字符。
- 可提供复制或展开按钮。

本地重复拦截表字段：

- 时间。
- endpoint。
- requested model。
- request id。
- body fingerprint prefix。
- first warning log id。
- first warning time。
- first warning kind / severity。
- channel。

### 操作弹窗

解封确认文案：

```text
确认解除该用户当前 GPT 安全警告封禁？此操作不会重置今日警告次数。
```

重置确认文案：

```text
确认重置该用户今日 GPT 安全警告次数？历史警告日志会保留，但后续封禁判断将只统计本次重置之后的新警告。
```

表单字段：

- `reason`：原因输入框，最大 255 字符。可选但默认值为 `manual_review`。
- `clear_suspension`：重置弹窗中的 checkbox，文案为「同时解除当前封禁」。

Mutation UX：

- 提交中禁用确认按钮并展示 loading。
- 成功后 toast，并关闭弹窗。
- 失败后使用统一错误 toast，不关闭弹窗。
- 成功后 invalidate：
  - GPT abuse 用户列表 query。
  - 当前用户 warning logs query。
  - 当前用户 repeat blocks query。
  - 当前用户 active suspension 相关 query（如果单独存在）。

### i18n

所有用户可见文案必须使用 `useTranslation().t()`。

key 命名空间：

```text
gptAbuse.*
```

必须同步：

```text
web/default/src/i18n/locales/en.json
web/default/src/i18n/locales/zh.json
web/default/src/i18n/locales/fr.json
web/default/src/i18n/locales/ja.json
web/default/src/i18n/locales/ru.json
web/default/src/i18n/locales/vi.json
web/default/src/i18n/static-keys.ts
```

禁止中文硬编码。

## 操作语义

| 操作 | 删除历史日志 | 清除 active suspension | 改变当日有效计数 |
|---|---:|---:|---:|
| 解封 | 否 | 是 | 否 |
| 重置次数 | 否 | 可选 | 是 |
| 重置并解封 | 否 | 是 | 是 |

## 原始 warning 与本地 repeated block 区分

| 类型 | 来源 / 错误码 | 是否请求上游 | 是否计入 warning count |
|---|---|---:|---:|
| 原始上游 warning | `cyber_policy` 等 | 是 | 是 |
| 本地重复拦截 | `gpt_abuse_repeated_warning_request` | 否 | 否 |

本地重复拦截必须明确提示用户：这不是新的上游警告，而是网关发现用户正在原样重试刚被警告的请求，因此已在本地拦截。

## 实现顺序

### 阶段 1：重复请求拦截后端

优先实现，因为它能直接减少上游 warning 次数。

任务：

1. 新增 canonical JSON + HMAC fingerprint helper。
2. 在请求解析后捕获 client-facing fingerprint 上下文。
3. 新增 repeat block cache service。
4. 实现 Redis + 内存 TTL map。
5. 在每次 upstream attempt 前检查 cache。
6. 在 warning 记录成功插入后写入 cache。
7. 命中后返回 `gpt_abuse_repeated_warning_request`。
8. 写 `gpt_abuse_repeat_block_logs`。

### 阶段 2：Admin 后端 API

任务：

1. 新增 `gpt_abuse_warning_resets`。
2. 拆分 raw / effective warning count。
3. 改造 `GetSubscriptionSelfSummary` 与 `applyGPTAbuseLimit` 使用 effective count。
4. 新增用户聚合查询。
5. 新增用户 warning 明细查询。
6. 新增 clear suspension。
7. 新增 reset warnings。
8. 新增 repeat block 查询。

### 阶段 3：前端管理面板

任务：

1. 新增 `/gpt-abuse` 路由与权限守卫。
2. 新增 API client 和 types。
3. 新增用户聚合表。
4. 新增详情 Drawer。
5. 新增解封 / 重置弹窗。
6. 接入 sidebar、权限、i18n、static keys。
7. 补充前端测试。

## 测试要求

### 后端测试

Warning reset：

- 用户当日已有 5 条 warning。
- reset 后 effective count 为 0。
- reset 前日志仍能查到。
- reset 后同一秒新增 warning，因 `id > cutoff_signal_log_id` 计入 effective count。
- reset 后新增 1 条 warning，effective count 为 1。
- reset + clear suspension 在同一事务中生效。
- reset 后 `GetSubscriptionSelfSummary` 返回 effective count。
- reset 后 `applyGPTAbuseLimit` 使用 effective count。

Admin list：

- 能按用户聚合 raw warning count。
- 能展示 effective warning count。
- 能展示 daily limit、remaining count、active suspension。
- 能展示 latest warning。
- 能过滤 active suspended。
- 能过滤 kind / severity / source。

Repeat block：

- 第一次 warning 后成功插入 log 才写 cache。
- DB 插入失败或 dedupe 未插入时不写 cache。
- 同 body、同 user、同 token、同 endpoint、同 relay mode、同 model 命中。
- JSON 字段顺序不同但语义相同应命中。
- 修改请求内容后不命中。
- 命中后不上游。
- 命中后返回 `gpt_abuse_repeated_warning_request`。
- 命中后不增加 warning count。
- 命中后写 `gpt_abuse_repeat_block_logs`。
- Redis 不可用时内存兜底可用。
- TTL 过期后不再拦截。
- 不同 channel retry 仍命中，因为 channel 不参与 fingerprint。

错误语义：

- 返回 HTTP 400。
- `type=invalid_request_error`。
- `code=gpt_abuse_repeated_warning_request`。
- message 包含 `blocked locally` / `not sent upstream again`。
- 带 `skipRetry`。

### 前端测试

- 默认显示当天数据。
- 管理员能看到页面。
- 非管理员跳转 `/403`。
- 用户表正确渲染 warning count / effective count / daily limit / remaining count / suspension。
- URL search 能表达筛选、分页、排序。
- 筛选变化重置 offset。
- source / status / kind / severity 过滤可用。
- 点击详情能显示 warning logs。
- `extra.raw_error` 默认折叠并截断。
- 解封 mutation 发送 `reason`。
- reset mutation 发送 `reason` 与 `clear_suspension`。
- 解封成功后刷新列表和详情。
- reset 成功后刷新列表和详情。
- 操作 loading、防重复提交、错误 toast 正常。
- reset 弹窗文案明确说明不删除历史日志。
- 页面说明本地 repeated block 不消耗 warning count。
- i18n key 六语言完整，无中文硬编码。

## 风险与约束

### 请求体隐私

不得保存请求体。只能保存 HMAC 指纹。

### 误伤范围

指纹必须包含：

- `user_id`
- `token_id`
- client-facing `endpoint_path`
- client-facing `relay_mode`
- client-facing `origin_model`
- canonical JSON 或 raw body digest

不得只用 body hash。

### Channel retry

channel id、channel type、upstream model 不参与 fingerprint，避免服务端内部换 channel retry 时 miss。它们只进入 cache value 和 repeat-block log 供排障。

### 客户端 retry 行为

本地错误不得使用 HTTP 429，避免 harness 按限流重试。推荐 HTTP 400 + `skipRetry`。

### 审计语义

重置 warning count 必须通过 reset marker 实现，不删除、不批量改写历史 warning logs。

### 多实例一致性

Redis 可保证多实例一致；内存兜底只保证单实例有效。

## 最终推荐

采用以下组合：

1. 新增独立 `/gpt-abuse` 管理页面。
2. 新增 `gpt_abuse_warning_resets`，用 reset marker + cutoff signal log id 实现可审计的 warning count reset。
3. 新增 `gpt_abuse_repeat_block_logs`，单独记录本地重复请求拦截。
4. 使用 client-facing canonical request HMAC 指纹 + Redis / 内存 TTL cache 做短期拦截。
5. 默认 TTL 为 15 分钟。
6. 本地 repeated block 返回 HTTP 400、`code=gpt_abuse_repeated_warning_request`，并使用 `skipRetry`。
7. 本地 repeated block 不请求上游，不增加 warning count。
8. 管理 API 同时返回 raw count 和 effective count，避免审计与封禁语义混淆。
9. 前端只展示 fingerprint prefix，不展示完整 HMAC。

该方案能同时满足管理员可观测、手动干预可审计、减少静默重试消耗上游警告次数、保护请求体隐私，并保留现有 warning 日志作为完整审计证据。
