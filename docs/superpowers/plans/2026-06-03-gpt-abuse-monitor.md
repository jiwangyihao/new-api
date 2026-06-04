# GPT 上游滥用监测与中断服务实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 实现 GPT 上游 abuse 信号记录、套餐级警告次数配置、用户控制台展示，以及启用后按自然日暂停用户服务。

**架构：** 后端新增 GPT abuse 分类器、日志表和 suspension 表；relay 在 GPT HTTP error / SSE 事件处记录信号；订阅 summary 返回当天阈值与已触发次数；前端在系统设置、订阅计划配置和用户控制台展示新增字段。限制逻辑默认由全局开关关闭，开启后达到套餐阈值暂停到次日。

**技术栈：** Go 1.22、Gin、GORM、React 19、TypeScript、React Hook Form、Zod、Bun、i18next。

---

## 文件结构

### 后端

- 创建：`service/gpt_abuse_signal.go`
  - GPT abuse 分类、阈值计算、自然日窗口、上游 request id 提取。
- 创建：`service/gpt_abuse_signal_test.go`
  - 分类和阈值单元测试。
- 创建：`model/gpt_abuse.go`
  - `GPTAbuseSignalLog`、`GPTAbuseUserSuspension`、记录、计数、暂停查询。
- 创建：`model/gpt_abuse_test.go`
  - 记录幂等、计数、暂停过期测试。
- 修改：`model/main.go`
  - AutoMigrate 新表。
- 修改：`model/subscription.go`
  - 套餐字段、public summary、self summary 增加 GPT abuse 字段，创建/更新订阅时保留现有行为。
- 修改：`controller/subscription.go`
  - public plan DTO、管理端更新 map 增加 `gpt_abuse_warning_limit`。
- 修改：`model/option.go`
  - 初始化和运行时更新 `GPTAbuseLimitEnabled`、`GPTAbuseDefaultWarningLimit`。
- 修改：`common/constants.go`
  - 新增 GPT abuse 全局设置变量。
- 修改：`controller/relay.go`
  - 在 relay/model 信息确定后检查 active suspension，仅中断 GPT/OpenAI 范围请求；选定渠道后复查渠道类型，避免 `TokenAuth` 全局阻断非 GPT 请求。
- 修改：`types/error.go`
  - 新增 `gpt_abuse_suspended` 错误码。
- 修改：`service/error.go`
  - 增加读取 body 后可复用的 error 构造 helper，或新增 GPT-aware 包装。
- 修改：GPT relay 相关文件
  - `relay/compatible_handler.go`
  - `relay/responses_handler.go`
  - `relay/chat_completions_via_responses.go`
  - `relay/channel/openai/relay_responses.go`
  - `relay/channel/openai/chat_via_responses.go`
  - 捕获 HTTP error / SSE GPT abuse。
- 修改/新增测试：
  - `controller/subscription_self_summary_test.go`
  - `controller/subscription_admin_plan_fields_test.go`
  - 根据实际 relay helper 增加最小集成测试。

### 前端

- 修改：`web/default/src/features/system-settings/types.ts`
  - `SecuritySettings` 增加 GPT abuse 设置。
- 创建：`web/default/src/features/system-settings/request-limits/gpt-abuse-limits-section.tsx`
  - 系统设置开关和默认最小次数表单。
- 修改：`web/default/src/features/system-settings/security/section-registry.tsx`
  - 注册 `gpt-abuse-limits` section。
- 修改：`web/default/src/features/subscriptions/types.ts`
  - plan/public plan/user summary 增加 GPT abuse 字段。
- 修改：`web/default/src/features/subscriptions/lib/plan-form.ts`
  - 表单 schema/default/映射新增 `gpt_abuse_warning_limit`。
- 修改：`web/default/src/features/subscriptions/components/subscriptions-mutate-drawer.tsx`
  - 在并发字段附近新增输入框。
- 修改：`web/default/src/features/subscriptions/lib/plan-form.test.ts`
  - 表单字段测试。
- 修改：`web/default/src/features/dashboard/lib/subscription-summary.ts`
  - summary view 增加 GPT abuse 展示字段。
- 修改：`web/default/src/features/dashboard/lib/subscription-summary.test.ts`
  - 展示逻辑测试。
- 修改：`web/default/src/features/dashboard/components/overview/summary-cards.tsx`
  - 控制台显式展示 `已触发 / 可触发总次数`。
- 修改：`web/default/src/i18n/locales/{en,zh,fr,ja,ru,vi}.json`
  - 新增全部文案翻译。

---

## 任务 1：后端 GPT abuse 分类与数据模型

**文件：**
- 创建：`service/gpt_abuse_signal.go`
- 创建：`service/gpt_abuse_signal_test.go`
- 创建：`model/gpt_abuse.go`
- 创建：`model/gpt_abuse_test.go`
- 修改：`model/main.go`
- 修改：`common/constants.go`
- 修改：`model/option.go`

- [ ] **步骤 1：编写分类测试**

覆盖：

```go
func TestClassifyGPTAbuseSignalFromHTTPErrorCyberPolicy(t *testing.T) {}
func TestClassifyGPTAbuseSignalFromHTTPErrorExcludesRateLimit(t *testing.T) {}
func TestClassifyGPTAbuseSignalFromSSETrustedAccessForCyber(t *testing.T) {}
func TestResolveGPTAbuseWarningLimit(t *testing.T) {}
```

运行：`go test ./service -run 'TestClassifyGPTAbuseSignal|TestResolveGPTAbuseWarningLimit'`

预期：失败，提示函数不存在。

- [ ] **步骤 2：实现分类器和阈值计算**

实现：

```go
ClassifyGPTAbuseSignalFromHTTPError(statusCode int, body []byte) GPTAbuseSignal
ClassifyGPTAbuseSignalFromSSEEvent(eventType string, data []byte) GPTAbuseSignal
GPTUpstreamRequestID(headers http.Header) string
ResolveGPTAbuseWarningLimit(plan *model.SubscriptionPlan) int
```

要求：所有 JSON 解析使用 `common.Unmarshal`。

- [ ] **步骤 3：运行分类测试验证通过**

运行：`go test ./service -run 'TestClassifyGPTAbuseSignal|TestResolveGPTAbuseWarningLimit'`

预期：PASS。

- [ ] **步骤 4：编写模型测试**

覆盖：

```go
func TestRecordGPTAbuseSignalLogDedupesByKey(t *testing.T) {}
func TestCountGPTAbuseSignalsForUserToday(t *testing.T) {}
func TestGPTAbuseSuspensionExpires(t *testing.T) {}
```

运行：`go test ./model -run 'TestRecordGPTAbuse|TestCountGPTAbuse|TestGPTAbuseSuspension'`

预期：失败，提示模型/函数不存在。

- [ ] **步骤 5：实现模型和迁移**

新增：

```go
type GPTAbuseSignalLog struct { ... }
type GPTAbuseUserSuspension struct { ... }
func RecordGPTAbuseSignalLog(log *GPTAbuseSignalLog) (bool, error)
func CountGPTAbuseSignalsForUser(userID int, start, end int64) (int, error)
func GetActiveGPTAbuseSuspension(userID int, now int64) (*GPTAbuseUserSuspension, error)
func MarkExpiredGPTAbuseSuspensions(userID int, now int64) error
func UpsertGPTAbuseSuspension(userID int, triggerLogID int, dailyCount int, dailyLimit int, suspendedUntil int64) error
```

在 `migrateDB` 和 `migrateDBFast` 增加新模型。

- [ ] **步骤 6：运行模型测试验证通过**

运行：`go test ./model -run 'TestRecordGPTAbuse|TestCountGPTAbuse|TestGPTAbuseSuspension'`

预期：PASS。

---

## 任务 2：后端订阅字段、summary 和中断服务

**文件：**
- 修改：`model/subscription.go`
- 修改：`controller/subscription.go`
- 修改：`controller/subscription_admin_plan_fields_test.go`
- 修改：`controller/subscription_self_summary_test.go`
- 修改：`controller/relay.go`
- 修改：`service/gpt_abuse_signal.go`
- 修改：`middleware/gpt_abuse_auth_test.go`
- 修改：`types/error.go`

- [ ] **步骤 1：编写订阅字段测试**

在 `controller/subscription_admin_plan_fields_test.go` 增加断言：

```go
assert.Equal(t, 7, plan.GPTAbuseWarningLimit)
```

请求 JSON 增加：

```json
"gpt_abuse_warning_limit":7
```

运行：`go test ./controller -run TestAdminUpdateSubscriptionPlan`

预期：失败，字段不存在或未保存。

- [ ] **步骤 2：实现订阅计划字段保存与 DTO**

增加 `SubscriptionPlan.GPTAbuseWarningLimit`，更新 public DTO、管理端 update map、表单输入相关后端字段。

- [ ] **步骤 3：编写用户 summary 测试**

在 `controller/subscription_self_summary_test.go` 增加：

```go
assert.Equal(t, int64(5), summaryInt64(t, summary, "gpt_abuse_warning_limit"))
assert.Equal(t, int64(2), summaryInt64(t, summary, "gpt_abuse_warning_count"))
assert.Equal(t, int64(3), summaryInt64(t, summary, "gpt_abuse_warning_remaining"))
```

插入当天两条 `GPTAbuseSignalLog`。

运行：`go test ./controller -run TestGetSubscriptionSelfSummary`

预期：失败，字段缺失。

- [ ] **步骤 4：实现 summary 字段**

`SelfSubscriptionSummary` 增加：

```go
GPTAbuseWarningLimit int `json:"gpt_abuse_warning_limit"`
GPTAbuseWarningCount int `json:"gpt_abuse_warning_count"`
GPTAbuseWarningRemaining int `json:"gpt_abuse_warning_remaining"`
GPTAbuseSuspendedUntil int64 `json:"gpt_abuse_suspended_until,omitempty"`
GPTAbuseLimitEnabled bool `json:"gpt_abuse_limit_enabled"`
```

在 `GetSubscriptionSelfSummary` 中读取当天计数和 active suspension。

- [ ] **步骤 5：编写中断服务测试**

新增 service/controller 层测试，构造 active suspension 后：

- GPT/OpenAI 范围请求应返回 403 / `gpt_abuse_suspended`。
- 非 GPT 请求应允许继续。
- `TokenAuth` 即使看到 active suspension 也应允许通过，由 relay 层按请求模型和渠道类型决定是否中断。

运行：`go test ./service -run TestEnforceGPTAbuseSuspension && go test ./middleware -run TestTokenAuth.*GPTAbuseSuspension`

预期：失败。

- [ ] **步骤 6：实现 GPT-only 中断检查**

在 `controller/relay.go` 的 relay info 构造后检查 suspension，并在选定渠道后复查上下文渠道类型；错误使用 `gpt_abuse_suspended`，skip retry。不得在 `TokenAuth` 中全局拒绝所有 token 请求。

- [ ] **步骤 7：运行订阅与鉴权测试**

运行：

```bash
go test ./controller -run 'TestAdmin.*SubscriptionPlan|TestGetSubscriptionSelfSummary'
go test ./service -run TestEnforceGPTAbuseSuspension
go test ./middleware -run 'TestTokenAuth.*GPTAbuseSuspension'
```

预期：PASS。

---

## 任务 3：后端 relay 捕获与限制触发

**文件：**
- 修改：`service/error.go`
- 修改：`relay/compatible_handler.go`
- 修改：`relay/responses_handler.go`
- 修改：`relay/chat_completions_via_responses.go`
- 修改：`relay/channel/openai/relay_responses.go`
- 修改：`relay/channel/openai/chat_via_responses.go`
- 新增或修改相关 relay/service 测试

- [ ] **步骤 1：编写 GPT-aware error handler 测试**

构造 GPT HTTP error body：

```json
{"error":{"message":"Possible cybersecurity risk detected","type":"invalid_request_error","code":"cyber_policy"}}
```

断言：

- 返回原有 OpenAI 错误。
- 写入 `GPTAbuseSignalLog`。
- 记录 `user_id`、`token_id`、`channel_id`、`request_id`、`upstream_request_id`。

运行对应最小测试。

- [ ] **步骤 2：实现 GPT-aware error handler**

保留原 `RelayErrorHandler` 行为，新增 helper 读取 body 后同时分类和记录。

- [ ] **步骤 3：替换 GPT 相关 HTTP error 调用点**

只替换 GPT / OpenAI 链路，不改其他 provider。

- [ ] **步骤 4：编写 SSE 分类记录测试**

对 `response.failed` / `trusted_access_for_cyber` 事件调用 helper，断言插入日志。

- [ ] **步骤 5：实现 SSE 插桩**

在 GPT Responses stream 解析处调用记录函数，不能改变 SSE 输出。

- [ ] **步骤 6：运行 relay/service 测试**

运行：

```bash
go test ./service -run GPTAbuse
go test ./relay/... -run GPTAbuse
```

预期：PASS。

---

## 任务 4：前端系统设置、订阅配置和控制台展示

**文件：**
- 修改：`web/default/src/features/system-settings/types.ts`
- 创建：`web/default/src/features/system-settings/request-limits/gpt-abuse-limits-section.tsx`
- 修改：`web/default/src/features/system-settings/security/section-registry.tsx`
- 修改：`web/default/src/features/subscriptions/types.ts`
- 修改：`web/default/src/features/subscriptions/lib/plan-form.ts`
- 修改：`web/default/src/features/subscriptions/lib/plan-form.test.ts`
- 修改：`web/default/src/features/subscriptions/components/subscriptions-mutate-drawer.tsx`
- 修改：`web/default/src/features/dashboard/lib/subscription-summary.ts`
- 修改：`web/default/src/features/dashboard/lib/subscription-summary.test.ts`
- 修改：`web/default/src/features/dashboard/components/overview/summary-cards.tsx`
- 修改：`web/default/src/i18n/locales/{en,zh,fr,ja,ru,vi}.json`

- [ ] **步骤 1：编写前端表单测试**

`plan-form.test.ts` 增加 `gpt_abuse_warning_limit` 默认、planToForm、formValuesToPayload 断言。

运行：`cd web/default && bun test src/features/subscriptions/lib/plan-form.test.ts`

预期：失败。

- [ ] **步骤 2：实现订阅表单字段与类型**

更新 schema、types、drawer UI。

- [ ] **步骤 3：运行表单测试验证通过**

运行同上，预期 PASS。

- [ ] **步骤 4：编写控制台 summary 测试**

`subscription-summary.test.ts` 增加 `gpt_abuse_warning_count / limit` view 字段测试。

运行：`cd web/default && bun test src/features/dashboard/lib/subscription-summary.test.ts`

预期：失败。

- [ ] **步骤 5：实现控制台展示**

在 summary view 和 summary cards 中显式展示 GPT 安全警告次数。

- [ ] **步骤 6：实现系统设置 section**

新增 GPT abuse limits section，注册到 Security & Limits，更新类型。

- [ ] **步骤 7：补齐 i18n**

新增全部 locale 翻译，运行 `bun run i18n:sync`。

- [ ] **步骤 8：运行前端验证**

运行：

```bash
cd web/default
bun test src/features/subscriptions/lib/plan-form.test.ts src/features/dashboard/lib/subscription-summary.test.ts
bun run typecheck
bun run i18n:sync
```

预期：PASS。

---

## 任务 5：最终验证

**文件：** 全部受影响文件。

- [ ] **步骤 1：运行后端目标测试**

```bash
go test ./service -run GPTAbuse
go test ./model -run GPTAbuse
go test ./controller -run 'Subscription.*GPT|TestAdmin.*SubscriptionPlan|TestGetSubscriptionSelfSummary'
go test ./middleware -run GPTAbuse
go test ./relay/... -run GPTAbuse
```

预期：PASS。

- [ ] **步骤 2：运行前端目标测试和类型检查**

```bash
cd web/default
bun test src/features/subscriptions/lib/plan-form.test.ts src/features/dashboard/lib/subscription-summary.test.ts
bun run typecheck
bun run i18n:sync
```

预期：PASS。

- [ ] **步骤 3：规格覆盖自检**

确认：

- GPT abuse 日志能定位用户。
- 系统开关控制是否中断服务。
- 套餐字段支持配置，0 自动按并发和默认最小值计算。
- 用户控制台展示 limit/count。
- 达阈值暂停到次日。
- 普通错误不计入 abuse。
- 不保存 prompt/body/key。
