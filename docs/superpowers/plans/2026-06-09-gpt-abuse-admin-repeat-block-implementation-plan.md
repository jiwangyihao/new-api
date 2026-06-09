# GPT 安全警告管理与重复请求拦截实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 实现 GPT 安全警告后台管理面板、管理员解封/重置当日警告次数，以及对已触发警告请求的短 TTL 本地重复拦截。

**架构：** 后端新增 GPT abuse 管理 DTO、模型查询、管理接口和 repeat-block 服务。Repeat block 使用请求解析后立即捕获的 client-facing canonical request HMAC 指纹，Redis 优先、内存 TTL 兜底；命中后写本地重复拦截记录，并返回 OpenAI-compatible 400 错误且 skip retry。前端新增 `/gpt-abuse` 管理页，使用 React Query + TanStack Router search 参数，接入 sidebar、权限和 6 语言 i18n。

**技术栈：** Go、Gin、GORM、Redis、React 19、TanStack Router、React Query、i18next、Bun。

**规格来源：** `docs/superpowers/specs/2026-06-09-gpt-abuse-admin-repeat-block-design.md`。

---

## 执行边界

- 不使用 worktree，直接在当前主分支工作。
- 优先按任务边界并发，但必须避免多个子代理同时修改同一文件。
- 任务 1 是基础模型与计数函数，必须先完成并通过审查。
- 任务 2 和任务 3 依赖任务 1，可并发执行：任务 2 只负责 repeat block 和 relay gate，任务 3 只负责 admin API。
- 任务 4 前端依赖任务 3 API contract；如提前并发，必须以后端 DTO 字段名为冻结契约，后端字段变更需通过 IRC 通知前端任务。
- 每个实现子代理不得运行项目级全量 build/test/lint/format，只运行本任务新增/修改测试。
- 子代理不要提交 commit；主代理统一验证和提交。

## 文件职责

### 后端基础与模型

- 修改：`model/gpt_abuse.go`
  - 新增 `GPTAbuseWarningReset`、`GPTAbuseRepeatBlockLog`。
  - 新增 raw/effective count 查询。
  - 新增 latest reset 查询，排序固定为 `reset_at DESC, id DESC`。
  - 新增 tx-aware reset helpers，供 admin reset 在同一事务内读取 previous count、cutoff id 并插入 reset marker。
  - 新增 repeat block log 写入/查询。
- 修改：`model/main.go`
  - AutoMigrate 新增两个 model。
- 修改/新增测试：`model/gpt_abuse_test.go`
  - 覆盖 raw/effective count、cutoff id、reset 后同秒 warning、多次 reset 排序、repeat block log。

### Repeat block 后端

- 新增：`service/gpt_abuse_repeat_block.go`
  - canonical JSON HMAC 指纹。
  - 捕获并复用 client-facing fingerprint 上下文。
  - Redis + 内存 TTL cache。
  - cache lookup/store。
  - cache hit 写 `gpt_abuse_repeat_block_logs`。
  - 本地 repeated block 错误构造。
- 新增/修改测试：`service/gpt_abuse_repeat_block_test.go`、`service/gpt_abuse_signal_test.go`
  - 覆盖 canonical JSON、TTL、Redis fallback、cache 写入条件、cache hit 写日志、SSE handler nil 也写 cache。
- 修改：`service/gpt_abuse_signal.go`
  - `RecordGPTAbuseSignal` 在 signal log 成功插入并取得 id 后写 repeat-block cache。
  - 写 cache 时不得使用当前可能已被转换的 `RelayInfo.RequestURLPath` / `RelayMode`，必须只读取 Capture 阶段保存的 client-facing fingerprint context。
- 修改：`controller/relay.go`
  - 每次 upstream attempt 前执行 repeat-block check。
- 修改：`types/error.go`
  - 新增 `gpt_abuse_repeated_warning_request` 错误码。

### Admin API 后端

- 新增：`dto/gpt_abuse.go`
  - API query、response、action request/response 类型。
- 新增：`service/gpt_abuse_admin.go`
  - 用户聚合列表、用户 logs、repeat blocks、clear suspension、reset warnings。
- 新增：`controller/gpt_abuse.go`
  - HTTP query parsing、reason 校验、handler。
- 修改：`router/api-router.go`
  - 注册 `/api/gpt-abuse` AdminAuth 路由；代码必须显式加 `middleware.AdminAuth()`，不能只在注释中说明。
- 新增测试：`service/gpt_abuse_admin_test.go`、`controller/gpt_abuse_test.go`、`router/gpt_abuse_routes_test.go`
  - 覆盖聚合、详情、reset、clear、路由权限。

### 前端管理页

- 新增：`web/default/src/features/gpt-abuse/api.ts`
- 新增：`web/default/src/features/gpt-abuse/types.ts`
- 新增：`web/default/src/features/gpt-abuse/lib/filters.ts`
- 新增：`web/default/src/features/gpt-abuse/lib/format.ts`
- 新增：`web/default/src/features/gpt-abuse/index.tsx`
- 新增：`web/default/src/features/gpt-abuse/components/gpt-abuse-user-table.tsx`
- 新增：`web/default/src/features/gpt-abuse/components/gpt-abuse-log-drawer.tsx`
- 新增：`web/default/src/features/gpt-abuse/components/gpt-abuse-reset-dialog.tsx`
- 新增：`web/default/src/features/gpt-abuse/components/gpt-abuse-clear-suspension-dialog.tsx`
- 新增：`web/default/src/features/gpt-abuse/components/gpt-abuse-repeat-block-table.tsx`
- 新增：`web/default/src/routes/_authenticated/gpt-abuse/index.tsx`
- 修改：`web/default/src/hooks/use-sidebar-data.ts`
- 修改：`web/default/src/hooks/use-sidebar-config.ts`
- 修改：`web/default/src/i18n/static-keys.ts`
- 修改：`web/default/src/i18n/locales/{en,zh,fr,ja,ru,vi}.json`
- 新增测试：`web/default/src/features/gpt-abuse/lib/filters.test.ts`
- 新增测试：`web/default/src/features/gpt-abuse/lib/format.test.ts`
- 新增测试：`web/default/src/features/gpt-abuse/gpt-abuse-page.test.tsx`
- 新增测试：`web/default/src/hooks/use-sidebar-config.gpt-abuse.test.ts`

---

## 任务 1：GPT abuse 模型与计数基础

**文件：**

- 修改：`model/gpt_abuse.go`
- 修改：`model/main.go`
- 测试：`model/gpt_abuse_test.go`
- 测试：`service/gpt_abuse_signal_test.go`
- 测试：相关 subscription summary 测试文件（若已有 GPT abuse summary 测试则就近添加；否则在 `service/gpt_abuse_signal_test.go` 覆盖 `applyGPTAbuseLimit`）

### 步骤 1：编写失败测试

在 `model/gpt_abuse_test.go` 中新增：

```go
func TestGPTAbuseEffectiveCountUsesLatestResetCutoffLogID(t *testing.T) {
    // 创建同一用户同一窗口内 3 条 count_eligible warning。
    // Insert reset marker，CutoffSignalLogID 指向第 2 条。
    // 第 3 条 CreatedAt 与 ResetAt 同秒。
    // CountGPTAbuseSignalsForUserRaw == 3。
    // CountEffectiveGPTAbuseSignalsForUser == 1。
}

func TestLatestGPTAbuseWarningResetOrdersByResetAtThenID(t *testing.T) {
    // 同一用户同一 window 插入两个 ResetAt 相同的 reset。
    // 后插入 id 更大，CutoffSignalLogID 不同。
    // LatestGPTAbuseWarningReset 必须返回 id 更大的 reset。
}

func TestGPTAbuseRepeatBlockLogStoresWarningAttributionWithoutBody(t *testing.T) {
    // 写入 GPTAbuseRepeatBlockLog，BodyFingerprint 使用完整 HMAC。
    // 模型层只验证完整 HMAC 可落库且不存在请求体/prompt 字段。
}
```

在 `service/gpt_abuse_signal_test.go` 或现有 summary 测试中新增：

```go
func TestGPTAbuseResetMakesSubscriptionSummaryUseEffectiveCount(t *testing.T) {
    // 用户当天 5 条 warning，套餐 limit 5。
    // 插入 reset marker cutoff 到第 5 条。
    // GetSubscriptionSelfSummary 返回 GPTAbuseWarningCount == 0。
}

func TestGPTAbuseLimitUsesEffectiveCountAfterReset(t *testing.T) {
    // 用户当天 5 条 warning 后 reset。
    // reset 后新增第 1 条 warning，不应立即创建 suspension。
    // 再补足到 limit 后才创建 suspension。
}
```

### 步骤 2：运行红灯测试

运行：

```bash
go test ./model -run 'GPTAbuseEffectiveCount|LatestGPTAbuseWarningReset|GPTAbuseRepeatBlock'
go test ./service -run 'GPTAbuseResetMakesSubscriptionSummaryUseEffectiveCount|GPTAbuseLimitUsesEffectiveCountAfterReset'
```

预期：FAIL，原因是新类型/新函数不存在或旧 summary 仍按 raw count。

### 步骤 3：实现 model

在 `model/gpt_abuse.go` 新增 `GPTAbuseWarningReset` 与 `GPTAbuseRepeatBlockLog`，字段完全按规格。

新增函数：

```go
func CountGPTAbuseSignalsForUserRaw(userID int, start, end int64) (int, error)
func CountEffectiveGPTAbuseSignalsForUser(userID int, start, end int64) (int, *GPTAbuseWarningReset, error)
func LatestGPTAbuseWarningReset(userID int, windowStart int64) (*GPTAbuseWarningReset, error)
func MaxGPTAbuseSignalLogIDForUserWindow(userID int, start, end int64) (int, error)
func CreateGPTAbuseWarningReset(reset *GPTAbuseWarningReset) error
func RecordGPTAbuseRepeatBlockLog(log *GPTAbuseRepeatBlockLog) error
```

新增 tx-aware helper，供 `ResetGPTAbuseWarnings` 使用同一个事务句柄：

```go
func CountGPTAbuseSignalsForUserRawTx(tx *gorm.DB, userID int, start, end int64) (int, error)
func CountEffectiveGPTAbuseSignalsForUserTx(tx *gorm.DB, userID int, start, end int64) (int, *GPTAbuseWarningReset, error)
func MaxGPTAbuseSignalLogIDForUserWindowTx(tx *gorm.DB, userID int, start, end int64) (int, error)
func CreateGPTAbuseWarningResetTx(tx *gorm.DB, reset *GPTAbuseWarningReset) error
```

`CountGPTAbuseSignalsForUser` 保留但改为调用 `CountEffectiveGPTAbuseSignalsForUser`，确保旧调用点默认获得 effective count。

`LatestGPTAbuseWarningReset` 排序必须是：

```go
Order("reset_at desc, id desc")
```

在 `model/main.go` 的 AutoMigrate 列表加入：

```go
&GPTAbuseWarningReset{},
&GPTAbuseRepeatBlockLog{},
```

### 步骤 4：运行绿灯测试

运行：

```bash
go test ./model -run 'GPTAbuseEffectiveCount|LatestGPTAbuseWarningReset|GPTAbuseRepeatBlock|GPTAbuse'
go test ./service -run 'GPTAbuseResetMakesSubscriptionSummaryUseEffectiveCount|GPTAbuseLimitUsesEffectiveCountAfterReset|GPTAbuse'
```

预期：PASS。

---

## 任务 2：Repeat block 指纹、缓存与 relay 拦截

**依赖：** 任务 1 完成。

**文件：**

- 新增：`service/gpt_abuse_repeat_block.go`
- 新增：`service/gpt_abuse_repeat_block_test.go`
- 修改：`service/gpt_abuse_signal.go`
- 修改：`service/gpt_abuse_signal_test.go`
- 修改：`controller/relay.go`
- 新增/修改测试：`controller/relay_gpt_abuse_repeat_block_test.go`（或就近放入现有 relay controller 测试）
- 修改：`types/error.go`

### 步骤 1：编写失败测试

新增 `service/gpt_abuse_repeat_block_test.go`。

canonical JSON 测试：

```go
func TestGPTAbuseRepeatBlockFingerprintCanonicalizesJSONObjects(t *testing.T) {
    a := []byte(`{"model":"gpt-5.5","input":[{"role":"user","content":"x"}],"stream":true}`)
    b := []byte(`{"stream":true,"input":[{"content":"x","role":"user"}],"model":"gpt-5.5"}`)
    fpA, err := BuildGPTAbuseRepeatBlockFingerprint(7001, 8001, "/v1/responses", 0, "gpt-5.5", "application/json", a)
    require.NoError(t, err)
    fpB, err := BuildGPTAbuseRepeatBlockFingerprint(7001, 8001, "/v1/responses", 0, "gpt-5.5", "application/json", b)
    require.NoError(t, err)
    assert.Equal(t, fpA.Value, fpB.Value)
}

func TestGPTAbuseRepeatBlockFingerprintChangesWhenPromptChanges(t *testing.T) {
    // Same user/token/endpoint/model, different content -> different fingerprint.
}

func TestGPTAbuseRepeatBlockFingerprintPreservesJSONNumbers(t *testing.T) {
    // 使用大整数或高精度小数，确认 canonical 过程不经 float64 造成误合并。
}
```

cache / error 测试：

```go
func TestCheckGPTAbuseRepeatBlockReturnsLocalOpenAIError(t *testing.T) {
    // Store a repeat block cache value.
    // Check same captured fingerprint.
    // Assert status == 400, type == invalid_request_error,
    // code == gpt_abuse_repeated_warning_request,
    // message contains blocked locally and not sent upstream again,
    // skip retry option is set.
}

func TestCheckGPTAbuseRepeatBlockWritesRepeatBlockLog(t *testing.T) {
    // Cache hit 后写 gpt_abuse_repeat_block_logs。
    // Assert log.UserId/TokenId/FirstWarningLogId/Kind/Severity/Channel fields.
    // Assert warning count 不增加。
}

func TestGPTAbuseRepeatBlockRedisMissChecksMemoryFallback(t *testing.T) {
    // 先模拟 Redis 写失败写入 memory，再模拟 Redis 恢复但 miss。
    // Check 必须查 memory fallback 并命中。
}
```

在 `service/gpt_abuse_signal_test.go` 增加：

```go
func TestRecordGPTAbuseSignalStoresRepeatBlockOnlyAfterInserted(t *testing.T) {
    // context 中预先放 Capture 阶段的 client-facing fingerprint。
    // 第一次 RecordGPTAbuseSignal 插入成功 -> cache 写入。
    // 同 dedupe 再调用一次 -> inserted=false -> 不覆盖 first warning log。
}

func TestRecordGPTAbuseSignalUsesCapturedClientFacingFingerprint(t *testing.T) {
    // Capture 保存 /v1/chat/completions + RelayModeChatCompletions。
    // 模拟 info 后续被改成 /v1/responses + RelayModeResponses。
    // Store 后的 cache key/metadata 必须仍为 Capture 阶段的 client-facing endpoint/mode/model。
}

func TestResponsesSSEWarningStoresRepeatBlockBeforeHandlerReturns(t *testing.T) {
    // 模拟 SSE event callback 调用 RecordGPTAbuseSignal 后 handler 最终 nil。
    // Assert cache 已写。
}
```

在 controller relay 测试中增加：

```go
func TestRelayChecksGPTAbuseRepeatBlockOnEveryRetryAttempt(t *testing.T) {
    // 第一次 attempt 触发 warning 并写 cache。
    // 第二次内部 retry 在 helper/adaptor 前命中 repeat block。
    // Assert 上游 helper/adaptor 调用次数不超过第一次。
    // Assert 不进入 processChannelError 的后续上游请求路径。
}
```

### 步骤 2：运行红灯测试

运行：

```bash
go test ./service -run 'GPTAbuseRepeatBlock|RecordGPTAbuseSignalStoresRepeatBlock|RecordGPTAbuseSignalUsesCapturedClientFacingFingerprint|ResponsesSSEWarningStoresRepeatBlock'
go test ./controller -run 'Relay.*GPTAbuseRepeatBlock|GPTAbuseRepeatBlock'
```

预期：FAIL，新函数/错误码不存在。

### 步骤 3：实现 repeat block service

在 `types/error.go` 新增错误码：

```go
ErrorCodeGPTAbuseRepeatedWarningRequest = "gpt_abuse_repeated_warning_request"
```

在 `service/gpt_abuse_repeat_block.go` 实现：

```go
type GPTAbuseRepeatBlockFingerprint struct {
    Value  string
    Prefix string
}

type GPTAbuseRepeatBlockContext struct {
    Fingerprint   GPTAbuseRepeatBlockFingerprint
    EndpointPath  string
    RelayMode     int
    OriginModel   string
    UserID        int
    TokenID       int
}

type GPTAbuseRepeatBlockCacheValue struct {
    FirstWarningLogID int    `json:"first_warning_log_id"`
    UserID            int    `json:"user_id"`
    TokenID           int    `json:"token_id"`
    RequestID         string `json:"request_id"`
    UpstreamRequestID string `json:"upstream_request_id"`
    Source            string `json:"source"`
    Kind              string `json:"kind"`
    Severity          string `json:"severity"`
    CreatedAt         int64  `json:"created_at"`
    ChannelID         int    `json:"channel_id"`
    ChannelName       string `json:"channel_name"`
    ChannelType       int    `json:"channel_type"`
}
```

必须使用 `common.Marshal` / `common.Unmarshal`。

实现函数：

```go
func BuildGPTAbuseRepeatBlockFingerprint(userID int, tokenID int, endpointPath string, relayMode int, originModel string, contentType string, body []byte) (GPTAbuseRepeatBlockFingerprint, error)
func CaptureGPTAbuseRepeatBlockFingerprint(c *gin.Context, info *relaycommon.RelayInfo, bodyStorage common.BodyStorage) error
func GPTAbuseRepeatBlockContextFromGin(c *gin.Context) (GPTAbuseRepeatBlockContext, bool)
func CheckGPTAbuseRepeatBlock(c *gin.Context, info *relaycommon.RelayInfo) *types.NewAPIError
func StoreGPTAbuseRepeatBlock(c *gin.Context, info *relaycommon.RelayInfo, log *model.GPTAbuseSignalLog)
```

关键要求：

- `CaptureGPTAbuseRepeatBlockFingerprint` 必须在 relay conversion 前保存 client-facing endpoint/mode/model。
- `CheckGPTAbuseRepeatBlock`、`StoreGPTAbuseRepeatBlock`、repeat-block log 都必须只使用 Capture 阶段 context，不得从可能被转换后的 `info.RequestURLPath` / `info.RelayMode` 重算 key。
- canonical JSON 使用无损 number 表示，例如 `common.DecodeJson` 包装不满足时需使用 `json.Decoder.UseNumber` 只作为解析方式；实际 marshal/unmarshal 仍遵守项目规则，业务 JSON 序列化使用 `common.*`。

缓存策略：

- Redis key：`gpt_abuse:warned_request:v1:{user}:{token}:{endpoint}:{relayMode}:{model}:{hmac}`。
- Redis 读失败查内存 fallback。
- Redis miss 也查内存 fallback。
- Redis 写失败写内存 fallback。
- `GPTAbuseRepeatBlockRequireRedis=true` 禁用内存 fallback；Redis 故障 fail open 并记录 warn。

### 步骤 4：接入 `RecordGPTAbuseSignal`

在 `service/gpt_abuse_signal.go` 中，`model.RecordGPTAbuseSignalLog(log)` 成功插入且 `inserted == true` 后调用：

```go
StoreGPTAbuseRepeatBlock(c, info, log)
```

要求：

- `log.Id > 0` 才写。
- `log.CountEligible == true` 才写。
- dedupe 未插入不写。
- 无 captured repeat-block context 不写。

### 步骤 5：接入 relay attempt gate

在 `controller/relay.go` 的每次 retry attempt 中，`bodyStorage` 读取后、调用任何 helper 前：

```go
if err := service.CaptureGPTAbuseRepeatBlockFingerprint(c, relayInfo, bodyStorage); err != nil {
    newAPIError = types.NewErrorWithStatusCode(err, types.ErrorCodeReadRequestBodyFailed, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
    break
}
if repeatedErr := service.CheckGPTAbuseRepeatBlock(c, relayInfo); repeatedErr != nil {
    newAPIError = repeatedErr
    break
}
```

命中后：

- 不进入 helper/adaptor。
- 不进入 `processChannelError`。
- 不请求上游。
- `CheckGPTAbuseRepeatBlock` 内写 repeat-block log。

### 步骤 6：运行绿灯测试

运行：

```bash
go test ./service -run 'GPTAbuseRepeatBlock|RecordGPTAbuseSignalStoresRepeatBlock|RecordGPTAbuseSignalUsesCapturedClientFacingFingerprint|ResponsesSSEWarningStoresRepeatBlock'
go test ./controller -run 'Relay.*GPTAbuseRepeatBlock|GPTAbuseRepeatBlock'
```

预期：PASS。

---

## 任务 3：GPT abuse Admin API

**依赖：** 任务 1 完成。

**文件：**

- 新增：`dto/gpt_abuse.go`
- 新增：`service/gpt_abuse_admin.go`
- 新增：`service/gpt_abuse_admin_test.go`
- 新增：`controller/gpt_abuse.go`
- 新增：`controller/gpt_abuse_test.go`
- 修改：`router/api-router.go`
- 新增：`router/gpt_abuse_routes_test.go`

### 步骤 1：编写失败测试

在 `service/gpt_abuse_admin_test.go` 新增：

```go
func TestListGPTAbuseUsersReturnsRawAndEffectiveCounts(t *testing.T) {
    // Seed user, subscription plan, warning logs, reset marker, active suspension.
    // Assert warning_count raw, effective_warning_count reset 后口径, daily_limit, remaining_warning_count, active_suspension.
}

func TestListGPTAbuseRepeatBlocksReturnsFingerprintPrefixAndWarningAttribution(t *testing.T) {
    // Seed repeat block log with full BodyFingerprint.
    // Assert API item contains body_fingerprint_prefix only, and first_warning kind/severity/channel/token fields.
}

func TestResetGPTAbuseWarningsClearsSuspensionTransactionally(t *testing.T) {
    // Seed active suspension and warning logs.
    // Call ResetGPTAbuseWarnings(clearSuspension=true).
    // Assert reset row created, suspension status cleared, active_user_id nil.
}

func TestClearGPTAbuseSuspensionIsIdempotent(t *testing.T) {
    // No active suspension -> success with had_active_suspension=false.
}
```

在 `controller/gpt_abuse_test.go` 新增：

```go
func TestGPTAbuseControllerRejectsInvalidReason(t *testing.T) {
    // reason > 255 -> 400.
}
```

在 `router/gpt_abuse_routes_test.go` 新增：

```go
func TestGPTAbuseRoutesRequireAdminAuth(t *testing.T) {
    // 未登录/普通用户访问 GET /api/gpt-abuse/users 和 POST reset/clear 均不可用。
    // 管理员可访问。
}
```

### 步骤 2：运行红灯测试

运行：

```bash
go test ./service -run 'GPTAbuseAdmin|ResetGPTAbuse|ClearGPTAbuse|ListGPTAbuseRepeatBlocks'
go test ./controller -run GPTAbuse
go test ./router -run GPTAbuse
```

预期：FAIL，新 service/controller/router 不存在。

### 步骤 3：实现 DTO

在 `dto/gpt_abuse.go` 定义：

```go
type GPTAbuseUserListQuery struct { ... }
type GPTAbuseUserListItem struct { ... }
type GPTAbuseSignalLogItem struct { ... }
type GPTAbuseRepeatBlockItem struct { ... }
type GPTAbuseClearSuspensionRequest struct { Reason string `json:"reason"` }
type GPTAbuseResetWarningsRequest struct { Reason string `json:"reason"`; ClearSuspension bool `json:"clear_suspension"` }
type GPTAbuseResetWarningsResponse struct { ... }
```

必须包含：`daily_limit`、`remaining_warning_count`、`max_severity`、`suspension_status`、`active_suspension` 可为 null、`body_fingerprint_prefix`。

### 步骤 4：实现 service

在 `service/gpt_abuse_admin.go` 实现：

```go
func ListGPTAbuseUsers(ctx context.Context, query dto.GPTAbuseUserListQuery) (*dto.GPTAbuseUserListResponse, error)
func ListGPTAbuseUserLogs(ctx context.Context, userID int, query dto.GPTAbuseLogQuery) (*dto.GPTAbuseLogListResponse, error)
func ListGPTAbuseRepeatBlocks(ctx context.Context, userID int, query dto.GPTAbuseRepeatBlockQuery) (*dto.GPTAbuseRepeatBlockListResponse, error)
func ClearGPTAbuseSuspension(ctx context.Context, adminID int, userID int, reason string) (*dto.GPTAbuseClearSuspensionResponse, error)
func ResetGPTAbuseWarnings(ctx context.Context, adminID int, userID int, reason string, clearSuspension bool) (*dto.GPTAbuseResetWarningsResponse, error)
```

`ResetGPTAbuseWarnings` 必须使用同一个 `model.DB.Transaction`：

1. 计算 `window_start/window_end`。
2. 使用 tx-aware helper 查询 previous raw/effective count。
3. 使用 tx-aware helper 查询 cutoff max log id。
4. 插入 reset marker。
5. 如果 `clearSuspension`，在同一 tx 更新 active suspension。
6. 返回 reset id 与 suspension 状态。

### 步骤 5：实现 controller/router

在 `controller/gpt_abuse.go`：

- 解析 query。
- 校验 `reason` 长度 <= 255。
- 调用 service。
- 使用 `common.ApiSuccess` / `common.ApiError`。

在 `router/api-router.go` AdminAuth group 注册，代码必须显式包含 admin auth 中间件：

```go
gptAbuseRoute := apiRouter.Group("/gpt-abuse")
gptAbuseRoute.Use(middleware.AdminAuth())
gptAbuseRoute.GET("/users", controller.ListGPTAbuseUsers)
gptAbuseRoute.GET("/users/:id/logs", controller.ListGPTAbuseUserLogs)
gptAbuseRoute.GET("/users/:id/repeat-blocks", controller.ListGPTAbuseRepeatBlocks)
gptAbuseRoute.POST("/users/:id/clear-suspension", controller.ClearGPTAbuseSuspension)
gptAbuseRoute.POST("/users/:id/reset-warnings", controller.ResetGPTAbuseWarnings)
```

### 步骤 6：运行绿灯测试

运行：

```bash
go test ./service -run 'GPTAbuseAdmin|ResetGPTAbuse|ClearGPTAbuse|ListGPTAbuseRepeatBlocks'
go test ./controller -run GPTAbuse
go test ./router -run GPTAbuse
```

预期：PASS。

---

## 任务 4：前端 GPT abuse 管理页面

**依赖：** 任务 3 API contract。可按本计划 DTO 先行开发。

**文件：**

- 新增：`web/default/src/features/gpt-abuse/api.ts`
- 新增：`web/default/src/features/gpt-abuse/types.ts`
- 新增：`web/default/src/features/gpt-abuse/lib/filters.ts`
- 新增：`web/default/src/features/gpt-abuse/lib/format.ts`
- 新增：`web/default/src/features/gpt-abuse/index.tsx`
- 新增：`web/default/src/features/gpt-abuse/components/gpt-abuse-user-table.tsx`
- 新增：`web/default/src/features/gpt-abuse/components/gpt-abuse-log-drawer.tsx`
- 新增：`web/default/src/features/gpt-abuse/components/gpt-abuse-reset-dialog.tsx`
- 新增：`web/default/src/features/gpt-abuse/components/gpt-abuse-clear-suspension-dialog.tsx`
- 新增：`web/default/src/features/gpt-abuse/components/gpt-abuse-repeat-block-table.tsx`
- 新增：`web/default/src/routes/_authenticated/gpt-abuse/index.tsx`
- 修改：`web/default/src/hooks/use-sidebar-data.ts`
- 修改：`web/default/src/hooks/use-sidebar-config.ts`
- 修改：`web/default/src/i18n/static-keys.ts`
- 修改：`web/default/src/i18n/locales/en.json`
- 修改：`web/default/src/i18n/locales/zh.json`
- 修改：`web/default/src/i18n/locales/fr.json`
- 修改：`web/default/src/i18n/locales/ja.json`
- 修改：`web/default/src/i18n/locales/ru.json`
- 修改：`web/default/src/i18n/locales/vi.json`
- 测试：`web/default/src/features/gpt-abuse/lib/filters.test.ts`
- 测试：`web/default/src/features/gpt-abuse/lib/format.test.ts`
- 测试：`web/default/src/features/gpt-abuse/gpt-abuse-page.test.tsx`
- 测试：`web/default/src/hooks/use-sidebar-config.gpt-abuse.test.ts`

### 步骤 1：编写失败测试

在 `filters.test.ts` 覆盖：

```ts
it('resets offset when filters change', () => {
  // Given current search offset 40.
  // When kind/severity/status/source/keyword changes.
  // Then offset is 0.
})

it('preserves filters when pagination or sorting changes', () => {
  // Pagination/sort 写回 URL search，但不清空其他筛选条件。
})
```

在 `format.test.ts` 覆盖：

```ts
it('truncates raw warning detail for display', () => {
  expect(formatRawWarning('x'.repeat(1200))).toHaveLength(1000)
})
```

在 `use-sidebar-config.gpt-abuse.test.ts` 覆盖：

```ts
it('maps /gpt-abuse to admin gpt_abuse module', () => {
  // Assert default sidebar modules includes admin.gpt_abuse and URL map respects disabling.
})
```

在 `gpt-abuse-page.test.tsx` 覆盖：

```ts
it('submits clear suspension with reason and refreshes related queries', async () => {
  // Render page with mocked API.
  // Open clear dialog, type reason, submit.
  // Assert request payload contains reason.
  // Assert button disabled/loading during submit.
  // Assert dialog closes on success and list/detail query invalidation happens.
})


it('submits reset warnings with reason and clear_suspension then refreshes related queries', async () => {
  // Render page with mocked API and a selected user.
  // Open reset dialog, type reason, enable clear_suspension, submit.
  // Assert resetGPTAbuseWarnings payload contains reason and clear_suspension.
  // Assert button disabled/loading during submit.
  // Assert success closes the reset dialog.
  // Assert invalidation covers ['gpt-abuse','users'], ['gpt-abuse','logs', userId], and ['gpt-abuse','repeat-blocks', userId].
})
it('keeps reset dialog open on failure and shows error', async () => {
  // Mock reset failure.
  // Assert failure toast path and dialog remains open.
})

it('renders raw warning detail collapsed and truncated by default', () => {
  // Provide long raw_error.
  // Assert full string is not rendered in table by default.
})
```

### 步骤 2：运行红灯测试

运行：

```bash
cd web/default
bun test src/features/gpt-abuse/lib/filters.test.ts src/features/gpt-abuse/lib/format.test.ts src/features/gpt-abuse/gpt-abuse-page.test.tsx src/hooks/use-sidebar-config.gpt-abuse.test.ts
```

预期：FAIL，新文件不存在。

### 步骤 3：实现前端 API/types/filters

`types.ts` 定义后端 contract。

`api.ts` 使用统一 `api` 实例：

```ts
export async function getGPTAbuseUsers(params: GPTAbuseUserListSearch): Promise<GPTAbuseApiResponse<GPTAbuseUserListResponse>>
export async function getGPTAbuseUserLogs(userId: number, params: GPTAbuseLogSearch): Promise<GPTAbuseApiResponse<GPTAbuseLogListResponse>>
export async function getGPTAbuseRepeatBlocks(userId: number, params: GPTAbuseRepeatBlockSearch): Promise<GPTAbuseApiResponse<GPTAbuseRepeatBlockListResponse>>
export async function clearGPTAbuseSuspension(userId: number, payload: GPTAbuseReasonPayload): Promise<GPTAbuseApiResponse<GPTAbuseClearSuspensionResponse>>
export async function resetGPTAbuseWarnings(userId: number, payload: GPTAbuseResetWarningsPayload): Promise<GPTAbuseApiResponse<GPTAbuseResetWarningsResponse>>
```

`filters.ts` 定义 TanStack Router search schema，包含：

```text
start_timestamp, end_timestamp, keyword, user_id, status, kind, severity, source, limit, offset, sort_by, sort_order
```

必须提供 helper：

```ts
updateGPTAbuseSearchForFilterChange(current, patch)
updateGPTAbuseSearchForPagination(current, patch)
updateGPTAbuseSearchForSorting(current, patch)
```

筛选条件变化时 `offset=0`；分页/排序变化时保留其他筛选条件。

### 步骤 4：实现页面和组件

`index.tsx`：

- 使用 `useQuery` 加载用户列表。
- 使用 `useMutation` 执行 clear/reset。
- 定义 query key：
  - `['gpt-abuse', 'users', search]`
  - `['gpt-abuse', 'logs', userId, logSearch]`
  - `['gpt-abuse', 'repeat-blocks', userId, repeatSearch]`
- clear/reset 成功后 invalidate：
  - `['gpt-abuse', 'users']`
  - `['gpt-abuse', 'logs', userId]`
  - `['gpt-abuse', 'repeat-blocks', userId]`
- 顶部 Alert 明确 repeated block 不消耗 warning count。
- 表格展示 raw/effective count、daily limit、remaining count、active suspension、repeat block count。
- Drawer 展示 warning logs 和 repeat blocks。
- Dialog 包含 reason 输入、loading、防重复提交、错误 toast。
- 失败时不关闭弹窗。

`raw_error` 展示：

- 表格只显示摘要。
- Drawer 默认折叠。
- 展开后固定高度滚动区域。
- 展示最多 1000 字符，提供复制按钮可复制完整 `raw_error`（如果实现复制，则测试不要求）。

所有文案必须使用 `t('gptAbuse.xxx')`。

### 步骤 5：实现路由与 sidebar

新增路由：

```ts
export const Route = createFileRoute('/_authenticated/gpt-abuse/')({
  beforeLoad: () => {
    const { auth } = useAuthStore.getState()
    if (!auth.user || auth.user.role < ROLE.ADMIN) {
      throw redirect({ to: '/403' })
    }
  },
  validateSearch: gptAbuseSearchSchema,
  component: GPTAbuseRoute,
})
```

侧栏：

- `use-sidebar-data.ts` admin group 增加 `t('gptAbuse.title')`，url `/gpt-abuse`，icon `ShieldAlert`。
- `use-sidebar-config.ts`：
  - `DEFAULT_SIDEBAR_MODULES.admin.gpt_abuse = true`
  - `URL_TO_CONFIG_MAP['/gpt-abuse'] = { section: 'admin', module: 'gpt_abuse' }`

### 步骤 6：实现 i18n

补充 6 个 locale 文件与 `static-keys.ts`。

必须至少包含：

```text
gptAbuse.title
gptAbuse.description
gptAbuse.notice.title
gptAbuse.notice.description
gptAbuse.filters.*
gptAbuse.table.*
gptAbuse.details.*
gptAbuse.actions.clearSuspension
gptAbuse.actions.resetWarnings
gptAbuse.dialog.reason
gptAbuse.dialog.clearSuspensionConfirm
gptAbuse.dialog.resetWarningsConfirm
gptAbuse.repeatBlock.notCountedNotice
```

侧栏模块管理中新增 `gpt_abuse` 的展示文案也必须使用 `gptAbuse.*` 或本项目既有 sidebar module key 策略，不得硬编码中文。

### 步骤 7：运行绿灯测试

运行：

```bash
cd web/default
bun test src/features/gpt-abuse/lib/filters.test.ts src/features/gpt-abuse/lib/format.test.ts src/features/gpt-abuse/gpt-abuse-page.test.tsx src/hooks/use-sidebar-config.gpt-abuse.test.ts
bun run typecheck
```

预期：PASS。

---

## 最终验证

所有任务完成后由主代理执行：

```bash
go test ./model -run GPTAbuse
go test ./service -run GPTAbuse
go test ./controller -run GPTAbuse
go test ./router -run GPTAbuse
```

前端：

```bash
cd web/default
bun test src/features/gpt-abuse/lib/filters.test.ts src/features/gpt-abuse/lib/format.test.ts src/features/gpt-abuse/gpt-abuse-page.test.tsx src/hooks/use-sidebar-config.gpt-abuse.test.ts
bun run typecheck
```

验收：

- 所有命令 exit 0。
- Repeat block 命中不请求上游、不增加 warning count。
- Repeat block 使用 captured client-facing fingerprint，chat-completions-via-responses 不会读写 key 不一致。
- Reset 后 raw count 和 effective count 分离。
- Reset 后 self summary 和封禁判断使用 effective count。
- Admin 面板能展示 warning、suspension、repeat block，并能执行 reset/clear。
- 前端无中文硬编码，6 语言 key 完整。
