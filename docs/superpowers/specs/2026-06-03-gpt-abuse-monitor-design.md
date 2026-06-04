# GPT 上游滥用监测与用户临时中断服务设计

## 背景

当前项目只计划支持 GPT / OpenAI 上游滥用监测，不扩展到 Gemini、Claude 或其他 provider。目标是在 GPT 上游返回 abuse、safety、cyber 相关信号时，系统能够记录触发用户，并在启用限制逻辑后按套餐配置的阈值临时中断该用户服务，直到下一个自然日恢复。

本设计覆盖本阶段需要一并完成的能力：

1. 静态观测 GPT 上游 abuse 信号。
2. 记录并展示触发用户、Token、渠道、请求和上游 request id。
3. 按用户当前套餐配置计算可触发警告总次数。
4. 达到阈值后中断用户 GPT 服务到次日。
5. 管理端系统设置新增限制逻辑开关和默认最小可触发次数。
6. 订阅套餐配置新增 GPT abuse 可触发次数字段。
7. 用户控制台显式展示可触发警告总次数与已触发次数。

## 目标

- 只支持 GPT / OpenAI 风格响应。
- 第一阶段记录信号时不保存用户 prompt、请求 body 或响应 body。
- 限制逻辑由全局开关控制，默认不应意外阻断线上请求。
- 套餐可配置 `gpt_abuse_warning_limit`；未配置或为 0 时按并发数派生。
- 默认最小可触发警告次数由系统设置控制，建议默认值为 5。
- 若套餐并发数小于默认最小值，则可触发次数使用默认最小值。
- 达到限制后暂停到下一个自然日开始，不能复用永久禁用用户状态。

## 非目标

- 不支持其他 provider。
- 不做请求前置内容审核。
- 不做 prompt hash 黑名单。
- 不做管理员手动复核工作流。
- 不把 `safety_identifier` 作为本系统用户定位依据。
- 不保存完整 GPT 上游错误 message，除非后续单独加管理员审计开关。

## 核心术语

- **GPT abuse 信号**：GPT 上游返回的、可归类为 cyber/policy/safety abuse 的响应信号。
- **警告次数**：当前自然日内计入限制的 GPT abuse 信号次数。
- **可触发警告总次数**：当前用户在当前自然日可触发的最大计数，来自用户当前有效套餐配置或系统默认规则。
- **临时中断服务**：拒绝用户继续调用 GPT 接口，直到下一个自然日自动恢复。

## GPT abuse 分类

### 计入限制的类型

| kind | severity | 触发来源 | 规则 |
|---|---|---|---|
| `cyber_policy` | `high` | HTTP error / SSE `response.failed` | `error.code == "cyber_policy"`，或 message 命中 `possible cybersecurity risk`、`high-risk cyber activity` |
| `high_risk_cyber_reroute` | `high` | SSE metadata / header model signal | 出现 `trusted_access_for_cyber`，或 GPT 风险重路由信号 |
| `invalid_prompt_safety` | `medium` | HTTP error / SSE error | `error.code == "invalid_prompt"` 且 message 命中 safety / policy / disallowed marker |
| `content_policy_violation` | `medium` | HTTP error / SSE error | code/type/message 命中 `content_policy`、`policy_violation`、`safety_violation`、`moderation_blocked`、`content_filter` |
| `generic_policy_violation` | `medium` | HTTP error / SSE error | message 命中 `usage policy`、`policy violation`、`not allowed`、`violat` |
| `generic_abuse_security_warning` | `medium` | HTTP error / SSE error | message 命中 `network security warning`、`cyber abuse`、`abuse policy` |

所有上述类型默认 `count_eligible = true`。

### 明确排除

以下错误不应记录为 GPT abuse，也不应计入限制：

- `rate_limit_exceeded`
- `insufficient_quota`
- `server_is_overloaded`
- `overloaded`
- `slow_down`
- `context_length_exceeded`
- `unsupported_parameter`
- `invalid_image`
- `previous_response_not_found`
- `invalid_encrypted_content`
- 普通参数错误

设计原则：宁可漏记，不误伤。

## 数据模型

### `GPTAbuseSignalLog`

新增模型，建议表名：`gpt_abuse_signal_logs`。

字段：

| 字段 | 类型 | 说明 |
|---|---|---|
| `id` | int | 主键 |
| `created_at` | int64 | Unix 秒 |
| `user_id` | int | 触发用户 |
| `username` | string | 用户名快照 |
| `user_email` | string | 邮箱快照 |
| `token_id` | int | 触发 Token |
| `token_name` | string | Token 名称快照 |
| `channel_id` | int | GPT 渠道 |
| `channel_name` | string | 渠道名称快照 |
| `channel_type` | int | 渠道类型 |
| `channel_multi_key_index` | int | 多 key 槽位，无则为 0 |
| `request_id` | string | 本地请求 ID |
| `upstream_request_id` | string | GPT 上游 request id |
| `endpoint` | string | 请求路径 |
| `relay_mode` | int | relay mode |
| `requested_model` | string | 用户请求模型 |
| `upstream_model` | string | 上游实际模型 |
| `is_stream` | bool | 是否流式 |
| `source` | string | `http_error` / `sse_response_failed` / `sse_metadata` / `model_reroute` |
| `kind` | string | abuse 类型 |
| `severity` | string | `high` / `medium` |
| `status_code` | int | 上游 HTTP 状态 |
| `error_code` | string | GPT error code |
| `error_type` | string | GPT error type |
| `count_eligible` | bool | 是否计入限制 |
| `dedupe_key` | string | 幂等键 |
| `extra` | text | 少量 JSON 扩展 |

索引：

- `idx_gpt_abuse_user_created (user_id, created_at)`
- `idx_gpt_abuse_kind_created (kind, created_at)`
- `idx_gpt_abuse_request_id (request_id)`
- `idx_gpt_abuse_upstream_request_id (upstream_request_id)`
- `idx_gpt_abuse_channel_created (channel_id, created_at)`
- `idx_gpt_abuse_token_created (token_id, created_at)`
- unique `idx_gpt_abuse_dedupe_key (dedupe_key)`

### `GPTAbuseUserSuspension`

新增模型，建议表名：`gpt_abuse_user_suspensions`。

字段：

| 字段 | 类型 | 说明 |
|---|---|---|
| `id` | int | 主键 |
| `user_id` | int | 用户 |
| `status` | string | `active` / `expired` / `cleared` |
| `reason` | string | 固定为 `gpt_abuse_daily_limit` |
| `suspended_until` | int64 | Unix 秒，到期后自动允许 |
| `trigger_log_id` | int | 触发的 abuse log |
| `daily_count` | int | 触发时当天计数 |
| `daily_limit` | int | 触发时阈值 |
| `created_at` | int64 | 创建时间 |
| `updated_at` | int64 | 更新时间 |
| `cleared_at` | int64 | 手动解除时间，当前阶段可保留字段不做 UI |
| `cleared_by` | int | 管理员 ID，当前阶段可保留字段不做 UI |

索引：

- `idx_gpt_abuse_susp_user_status_until (user_id, status, suspended_until)`
- `idx_gpt_abuse_susp_until (suspended_until)`

### 订阅计划字段

在 `SubscriptionPlan` 新增：

```go
GPTAbuseWarningLimit int `json:"gpt_abuse_warning_limit" gorm:"type:int;not null;default:0"`
```

语义：

- `0` 表示自动：`max(plan.concurrency_limit, global_default_min)`。
- `> 0` 表示该套餐显式可触发次数，但最终仍不得低于全局默认最小值。

当前阶段不需要在 `UserSubscription` 上新增快照字段。原因是现有并发展示使用 `livePlanConcurrencyLimit(sub, plan)` 动态读取套餐；GPT abuse 限制也应读取当前套餐配置，保持管理端修改套餐后即时生效。

## 全局设置

新增设置项：

| key | 类型 | 默认 | 说明 |
|---|---|---:|---|
| `GPTAbuseLimitEnabled` | bool | `false` | 是否启用达到阈值后的中断服务逻辑 |
| `GPTAbuseDefaultWarningLimit` | int | `5` | 默认最小可触发警告次数，必须 >= 1，建议默认 5 |

说明：

- 即使 `GPTAbuseLimitEnabled=false`，仍记录 GPT abuse 观测日志和用户已触发次数。
- 开关仅控制是否创建/执行 `GPTAbuseUserSuspension`。
- `GPTAbuseDefaultWarningLimit` 是全局下限，不是所有套餐固定值。

## 阈值计算

函数：

```go
ResolveGPTAbuseWarningLimit(plan *SubscriptionPlan) int
```

规则：

1. `defaultMin = max(1, GPTAbuseDefaultWarningLimit)`。
2. 如果 `plan == nil`，返回 `defaultMin`。
3. 如果 `plan.GPTAbuseWarningLimit > 0`，返回 `max(plan.GPTAbuseWarningLimit, defaultMin)`。
4. 否则返回 `max(plan.ConcurrencyLimit, defaultMin)`。

示例：

| 套餐并发 | 套餐显式配置 | 默认最小值 | 最终可触发次数 |
|---:|---:|---:|---:|
| 1 | 0 | 5 | 5 |
| 3 | 0 | 5 | 5 |
| 10 | 0 | 5 | 10 |
| 10 | 8 | 5 | 8 |
| 3 | 2 | 5 | 5 |
| 0 | 0 | 5 | 5 |

## 自然日计数

- 计数窗口使用服务器本地时区的自然日。
- Go 侧计算 `[day_start, next_day_start)`，不要使用数据库日期函数。
- 只统计：
  - `user_id = 当前用户`
  - `count_eligible = true`
  - `created_at >= day_start`
  - `created_at < next_day_start`

## 中断服务流程

### 请求入口检查

在 Token 鉴权完成、请求解析出 relay/model 信息之后，检查 GPT abuse suspension：

1. 仅当 `GPTAbuseLimitEnabled=true` 且当前请求属于 GPT/OpenAI 监控范围时检查。
2. 查询用户是否存在 `active` 且 `suspended_until > now` 的 suspension。
3. 若存在，返回 403。
4. 错误 code：`gpt_abuse_suspended`。
5. 错误消息：`当前账号因触发 GPT 安全策略警告已暂停服务，请于次日恢复后重试`。

检查位置应在用户身份和请求模型已确定之后，优先放在 `controller/relay.go` 的 relay info 构造后，并在选定渠道后再做一次检查以覆盖 OpenAI 类型渠道和模型别名场景；不要在 `TokenAuth` 中全局拒绝所有 token 请求。

### 记录 abuse 后触发限制

`RecordGPTAbuseSignal` 成功插入非重复日志后：

1. 计算当前用户可触发次数。
2. 计算当天已触发次数。
3. 如果 `GPTAbuseLimitEnabled=false`，只记录，不创建 suspension。
4. 如果已触发次数 >= 可触发次数，创建或复用 active suspension。
5. `suspended_until = next_day_start`。
6. 失败只写系统日志，不影响原请求处理。

### 幂等

`dedupe_key`：

```text
user_id + token_id + channel_id + request_id + upstream_request_id + source + kind
```

若 `upstream_request_id` 为空，则省略该段。

唯一冲突时视为已记录，不重复计数。

## GPT 上游捕获点

### HTTP error

当前 `RelayErrorHandler` 会读取 response body，因此 GPT abuse 分类必须在读取 body 的同一流程中完成。

新增 helper：

```go
GPTAwareRelayErrorHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response, showBodyWhenFail bool) *types.NewAPIError
```

职责：

1. 读取 body。
2. 捕获 GPT 上游 request id。
3. 分类 GPT abuse。
4. 记录日志并可能触发 suspension。
5. 返回与原 `RelayErrorHandler` 一致的错误语义。

优先替换 GPT 相关链路：

- `relay/compatible_handler.go`
- `relay/responses_handler.go`
- `relay/chat_completions_via_responses.go`
- `relay/channel/openai/*` 中 GPT 非 200 分支

### SSE

GPT Responses / ChatCompletions via Responses 处理流式事件时，观察：

- `response.failed`
- `response.error`
- `response.metadata`

命中分类后调用 `RecordGPTAbuseSignal`。不要改变原 SSE 输出行为。

### 上游 request id

新增 GPT header 提取：

```go
GPTUpstreamRequestID(headers http.Header) string
```

优先级：

1. `x-request-id`
2. `X-Request-ID`
3. `openai-request-id`
4. `X-Oneapi-Request-Id`

提取结果写入 `common.UpstreamRequestIdKey`，复用现有日志关联能力。

## API 设计

### 用户控制台 summary

扩展 `GetSubscriptionSelf` 返回的 `summary`：

```json
{
  "gpt_abuse_warning_limit": 5,
  "gpt_abuse_warning_count": 2,
  "gpt_abuse_warning_remaining": 3,
  "gpt_abuse_suspended_until": 0,
  "gpt_abuse_limit_enabled": true
}
```

字段说明：

- `gpt_abuse_warning_limit`：当前自然日可触发总次数。
- `gpt_abuse_warning_count`：当前自然日已触发次数。
- `gpt_abuse_warning_remaining`：剩余次数，最小为 0。
- `gpt_abuse_suspended_until`：如果当前处于中断状态，则为 Unix 秒；否则为 0。
- `gpt_abuse_limit_enabled`：全局限制开关状态，用于 UI 文案区分「仅观测」和「会中断」。

### 管理端 GPT abuse 日志

当前阶段可以先实现后端模型与记录；如果实现管理端列表，建议接口：

```text
GET /api/gpt-abuse/logs
```

筛选：

- `user_id`
- `token_id`
- `channel_id`
- `kind`
- `severity`
- `source`
- `request_id`
- `upstream_request_id`
- `start_timestamp`
- `end_timestamp`

本阶段验收重点是用户定位与限制逻辑，管理端列表可作为后续增强，但数据模型必须支持。

## 前端设计

### 系统设置面板

位置：`Security & Limits`。

可新增一个独立 section：`GPT Abuse Limits`，也可以放在现有 `Rate Limiting` 下方。推荐独立 section，避免与普通请求速率限制混淆。

字段：

1. `Enable GPT abuse service interruption`
   - 映射 `GPTAbuseLimitEnabled`。
   - 描述：启用后，用户当天 GPT abuse 警告达到套餐阈值会暂停服务到次日。
2. `Default minimum GPT abuse warnings`
   - 映射 `GPTAbuseDefaultWarningLimit`。
   - number，min=1，默认 5。
   - 描述：套餐未配置时使用 `max(套餐并发数, 此值)`；套餐配置也不会低于此值。

### 订阅配置

位置：订阅套餐抽屉的并发设置附近。

新增字段：

- `GPT abuse warning limit`
- 输入类型：number，min=0。
- 描述：`0 means automatic: max(concurrency limit, system minimum)`。

类型和表单需同步：

- `web/default/src/features/subscriptions/types.ts`
- `web/default/src/features/subscriptions/lib/plan-form.ts`
- `web/default/src/features/subscriptions/components/subscriptions-mutate-drawer.tsx`
- 订阅计划列表列可选展示，推荐在详情/抽屉展示即可。

### 用户控制台

位置：现有「Usage at a glance」或订阅摘要卡片附近。

展示：

```text
GPT 安全警告：2 / 5
```

当 `gpt_abuse_limit_enabled = true`：

- 剩余次数为 0 且未过期时显示中断状态和恢复时间。
- 未中断时显示「达到上限后将暂停服务到次日」。

当 `gpt_abuse_limit_enabled = false`：

- 显示「当前仅统计，不会自动暂停服务」。

必须支持 i18n，新增文案写入所有 locale。

## 错误处理

### 用户请求被中断

返回 OpenAI 兼容错误，HTTP 403：

```json
{
  "error": {
    "message": "当前账号因触发 GPT 安全策略警告已暂停服务，请于次日恢复后重试",
    "type": "gpt_abuse_suspended",
    "code": "gpt_abuse_suspended"
  }
}
```

该错误应设置 skip retry，避免 relay 重试浪费资源。

### 记录失败

GPT abuse 记录失败不能影响原请求响应，只写系统日志。

## 测试要求

### 后端单元测试

1. `ClassifyGPTAbuseSignalFromHTTPError`：
   - `cyber_policy` 命中 high。
   - `invalid_prompt` + policy message 命中 medium。
   - `rate_limit_exceeded` 不命中。
   - `insufficient_quota` 不命中。
2. `ClassifyGPTAbuseSignalFromSSEEvent`：
   - `response.failed` + `cyber_policy` 命中。
   - metadata 中 `trusted_access_for_cyber` 命中 reroute。
3. 阈值计算：
   - 并发 1、默认 5 → 5。
   - 并发 10、默认 5 → 10。
   - 套餐显式 3、默认 5 → 5。
   - 套餐显式 8、默认 5 → 8。
4. 记录幂等：
   - 同一 dedupe key 插入一次。
   - 重复记录不增加当天计数。
5. suspension：
   - 达阈值后创建 active suspension。
   - 未启用开关时不创建 suspension。
   - 到期后鉴权允许并标记 expired。
6. `GetSubscriptionSelfSummary` 或对应服务：
   - 返回 limit/count/remaining/suspended_until/enabled。

### 后端集成测试

1. 模拟 GPT 非 200 HTTP body，确认 abuse log 包含 `user_id`、`token_id`、`channel_id`、`request_id`、`upstream_request_id`。
2. 模拟达到阈值后的下一次 TokenAuth 请求返回 403。

### 前端测试

1. `plan-form.test.ts` 覆盖 `gpt_abuse_warning_limit` 默认值、plan -> form、form -> payload。
2. 系统设置 section 覆盖开关和默认最小值提交。
3. 用户控制台展示 `2 / 5` 和中断恢复文案。
4. i18n sync 无缺失 key。

## 迁移与兼容

- 使用 GORM AutoMigrate，避免数据库专用 SQL。
- 新字段默认值必须兼容老数据。
- 时间统一使用 Unix 秒。
- JSON 扩展使用 text，不使用 JSONB。
- 不使用 PostgreSQL 专用 `ON CONFLICT`，唯一冲突通过 GORM error 判断或先查后写处理。

## 验收标准

1. 管理端可以配置 GPT abuse 限制开关和默认最小可触发次数。
2. 订阅套餐可以配置 GPT abuse 可触发警告次数，0 表示按并发和系统最小值自动计算。
3. 用户控制台显示当前 GPT abuse 可触发总次数与已触发次数。
4. GPT 上游 cyber/policy/safety 信号能记录到 `gpt_abuse_signal_logs`，并能定位用户、Token、渠道、request id、upstream request id。
5. 启用限制后，用户当天达到阈值会被暂停 GPT 服务到次日。
6. 禁用限制后，只记录不暂停。
7. 普通 GPT 错误不会计入 abuse。
8. 不保存 prompt、request body、response body 或密钥。
9. 所有新增/修改测试通过。
