# 套餐制 token 统计去价格化实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。执行本计划时必须直接在主分支当前工作区进行，不创建 worktree；每个实现子代理必须遵循 TDD，先写失败测试，再改生产代码。子代理不得运行项目级 build/test/lint/format，只运行任务指定的定向测试；最终由主会话统一验证。

**目标：** 将 default 用户侧 API 用量统计从旧价格 / quota 口径切换为套餐制 token 口径，保留支付、套餐售价、账户余额、渠道余额和管理员成本分析能力。

**架构：** 先补强后端 token 契约：日志统计、`quota_data.token_used`、订阅日志 `other`、`/api/subscription/self.summary`、`/api/pricing` 脱敏和倍率同步迁移。再让 default 前端消费这些契约：dashboard 概览 / 图表、usage logs、home / AI context、模型目录和 i18n。普通用户只看到 token、请求数、RPM、TPM、并发和模型能力；价格、倍率和成本分析只在管理员或支付场景出现。

**技术栈：** Go 1.25.1、Gin、GORM v2、SQLite / MySQL / PostgreSQL、React 19、TypeScript、TanStack Query、Rsbuild、Bun、i18next。

---

## 文件结构

### 后端契约

- 修改：`model/log.go`
  - `Log` 增加规范化 token 数值字段：`MeteredTokens *int`，标签为 ``json:"metered_tokens" gorm:"default:null"``。
  - `RecordConsumeLog` 写入规范化 token。
  - `LogQuotaData` 使用规范化 token 写入 `quota_data.token_used`。
  - `Stat` 增加 `TotalTokens`。
  - `SumUsedQuota` 用数值字段聚合 `total_tokens` 和 `tpm`，避免跨库 JSON 提取。
- 修改：`model/usedata.go`
  - 保留 `QuotaData.Quota`，确保 `QuotaData.TokenUsed` 来源是规范化 token。
- 修改：`controller/log.go`
  - `/api/log/stat` 与 `/api/log/self/stat` 返回 `total_tokens`。
- 修改：`service/log_info_generate.go`
  - distributor token subscription 写入 `subscription_token_limit`、`subscription_token_used`、`subscription_token_remaining`、`subscription_token_unlimited`、`subscription_tokens_consumed`。
  - legacy amount subscription 不写 token 权威字段。
- 修改：`relay/common/relay_info.go`
  - 如现有 `RelayInfo` 缺少 token 命名字段，增加 token 字段，避免继续复用 amount 命名字段作为新契约。
- 修改：`service/funding_source.go`、`service/billing_session.go`
  - 同步 token 字段到 `RelayInfo`。
- 修改：`model/subscription.go`
  - 抽取 self summary 与预扣共用的主订阅选择 helper。
- 修改：`controller/subscription.go`
  - `/api/subscription/self` 保留 `billing_preference`、`subscriptions`、`all_subscriptions`，新增 `summary`。
- 修改：`controller/pricing.go`
  - `/api/pricing` 对非管理员 / 未登录返回模型目录 DTO，不返回成本字段。
  - 管理员成本接口或受控 DTO 保留成本字段。
- 修改：`controller/ratio_sync.go`
  - 默认同步入口不再依赖脱敏 `/api/pricing`，改用 `/api/ratio_config` 或受控成本 DTO。
- 修改：`router/api-router.go`
  - 如新增管理员成本 DTO 路由，在管理员鉴权组中注册。
- 测试：新增或修改 `model/log_stat_token_test.go`、`service/log_info_generate_test.go`、`controller/subscription_self_summary_test.go`、`controller/pricing_directory_test.go`、`controller/ratio_sync_test.go`。

### 前端契约与展示

- 修改：`web/default/src/features/subscriptions/types.ts`
  - `SelfSubscriptionData` 增加 `billing_preference` 与 `summary`。
- 修改：`web/default/src/features/usage-logs/types.ts`
  - `LogStatistics` 增加 `total_tokens`。
  - `LogOtherData` 增加订阅 token 字段。
- 修改：`web/default/src/features/usage-logs/constants.ts`
  - `DEFAULT_LOG_STATS.total_tokens = 0`。
- 修改：`web/default/src/features/usage-logs/lib/format.ts`
  - 增加 `getLogTokenUsage` 与 `getLegacyPromptCompletionTokens`。
- 修改：`web/default/src/features/dashboard/types.ts`
  - `ProcessedChartData.totalQuotaDisplay` 改为 `totalTokensDisplay`。
- 修改：`web/default/src/features/dashboard/lib/charts.ts`
  - `processChartData` 和 `processUserChartData` 使用 `token_used` 排序、聚合、tooltip 和总计。
- 修改：`web/default/src/features/dashboard/components/overview/summary-cards.tsx`
  - 改用 `/api/subscription/self.summary` 和 `/api/data/self.token_used`。
- 修改：`web/default/src/features/dashboard/hooks/use-dashboard-config.tsx`
  - summary cards 和 model stat cards 文案 token-only。
- 修改：`web/default/src/features/dashboard/components/models/consumption-distribution-chart.tsx`
  - 显示 `totalTokensDisplay` 和 token 分布标题。
- 修改：`web/default/src/features/dashboard/components/users/user-charts.tsx`
  - 管理员用户排行文案改 token；非管理员深链不挂载用户排行。
- 修改：`web/default/src/features/usage-logs/components/common-logs-stats.tsx`
  - 顶部统计从 `quota` 改为 `total_tokens`。
- 修改：`web/default/src/features/usage-logs/components/columns/common-logs-columns.tsx`
  - Token Usage 列使用 helper/accessorFn；普通用户 TokenNameCell 不展示倍率；内联 details token-only。
- 修改：`web/default/src/features/usage-logs/components/dialogs/details-dialog.tsx`
  - 普通用户隐藏成本详情；管理员保留 legacy/cost audit。
- 修改：`web/default/src/components/ai-elements/context.tsx`
  - 默认删除 USD cost 展示。
- 修改：`web/default/src/features/home/components/hero-terminal-demo.tsx`
  - 删除 demo cost。
- 修改：`web/default/src/features/home/components/sections/hero.tsx`、`web/default/src/features/home/components/sections/cta.tsx`、`web/default/src/features/dashboard/components/overview/overview-dashboard.tsx`
  - `/pricing` 入口文案改模型目录语义。
- 修改：`web/default/src/features/pricing/**`
  - 普通用户视图改模型目录；价格、倍率、dynamic pricing 仅管理员或管理员成本入口可见。
- 修改：`web/default/src/features/system-settings/models/constants.ts`
  - 默认 endpoint 从 `/api/pricing` 改为 `/api/ratio_config`。
- 修改：`web/default/src/i18n/locales/{en,zh,fr,ja,ru,vi}.json`、`web/default/src/i18n/static-keys.ts`
  - 补齐新增 token / model directory 文案。
- 测试：新增或修改 `web/default/src/features/dashboard/lib/charts.test.ts`、`web/default/src/features/dashboard/lib/subscription-summary.test.ts`、`web/default/src/features/usage-logs/lib/format.test.ts`、相关 home/dashboard copy 测试。

---

## 执行调度与子代理约束

- **子代理提示词：** 每个新启动的实现或审查子代理提示词必须超过 2000 字，必须包含完整规格路径 `C:/Users/34404/source/repos/new-api/docs/superpowers/specs/2026-05-18-token-stats-depricing-spec.md` 与完整计划路径 `C:/Users/34404/source/repos/new-api/docs/superpowers/plans/2026-05-18-token-stats-depricing-implementation-plan.md`。
- **工作区：** 直接在当前主分支工作区开发，不创建 worktree，不 stash，不 commit。
- **验证边界：** 开发子代理只运行本任务列出的定向测试；不运行项目级 build/test/lint/formatter。最终验证由主会话统一执行。
- **审查边界：** review 类任务按用户要求 3 个以上并发启动；审查子代理只读，不运行 gates，不修改文件。
- **串行链：** 任务 1 → 任务 2 必须串行，因为二者都修改 `model/log.go` 与 `model/log_stat_token_test.go`；任务 5 → 任务 8 必须串行，因为二者都修改 `usage-logs/lib/format.ts` 与 `format.test.ts`；任务 6 → 任务 7 必须串行，因为二者都修改 `use-dashboard-config.tsx`；任务 4 → 任务 9 → 任务 10 必须串行，因为后端 pricing 脱敏先定义前端 DTO，任务 10 在全部 UI 文案落地后统一补 i18n。
- **可并行组：** 任务 3 可与任务 4 并行；任务 5 可与任务 6 并行；任务 3/4 完成后，任务 6 可与任务 9 在不共享文件时并行。任何两个任务触碰同一文件时不得并发写入。
- **冲突处理：** 子代理发现计划外共享文件或不一致契约时，先通过 IRC 询问主会话，不得自行扩大范围。

---

## 任务 1：后端日志 token 统计契约

**文件：**
- 修改：`model/log.go`
- 修改：`model/usedata.go`
- 修改：`controller/log.go`
- 测试：`model/log_stat_token_test.go`
- 测试：`controller/log_stat_token_test.go`

- [ ] **步骤 1：编写失败的 model 统计测试**

在 `model/log_stat_token_test.go` 中新增测试，使用项目现有测试 DB 初始化模式。测试必须覆盖：

```go
func TestSumUsedQuotaUsesMeteredTokensForSubscriptionLogs(t *testing.T) {
    // Arrange: 插入三条 LogTypeConsume 日志。
    // 1. 最近 60 秒内订阅日志：PromptTokens=10, CompletionTokens=5, Quota=100, MeteredTokens=80。
    // 2. 最近 60 秒内 legacy 日志：PromptTokens=7, CompletionTokens=3, Quota=40, MeteredTokens=nil（不设置字段）。
    // 3. 120 秒前订阅日志：PromptTokens=1, CompletionTokens=1, Quota=9, MeteredTokens=50。
    // Act: SumUsedQuota(LogTypeConsume, start, end, "", "", "", 0, "")。
    // Assert:
    //   stat.Quota == 149
    //   stat.TotalTokens == 140 // 80 + (7 + 3) + 50
    //   stat.Rpm == 2
    //   stat.Tpm == 90 // 最近 60 秒内 80 + (7 + 3)，不包含 120 秒前的 50
}

func TestSumUsedQuotaPreservesAuthoritativeZeroMeteredTokens(t *testing.T) {
    // Arrange: 插入最近 60 秒内 consume log，PromptTokens=10, CompletionTokens=5, Quota=100, MeteredTokens 指向 0。
    // Act: SumUsedQuota(LogTypeConsume, start, end, "", "", "", 0, "")。
    // Assert:
    //   stat.TotalTokens == 0
    //   stat.Tpm == 0
    //   stat.Quota == 100
}

func TestLogQuotaDataStoresMeteredTokens(t *testing.T) {
    // Arrange: 调用 LogQuotaData(userID, username, model, quota=123, createdAt, tokenUsed=80)，再 SaveQuotaDataCache()。
    // Act: 查询 quota_data。
    // Assert:
    //   quota_data.quota == 123
    //   quota_data.token_used == 80
}
```

测试必须断言 `MeteredTokens == nil` 时 fallback 到 `prompt_tokens + completion_tokens`，`MeteredTokens` 指向 `0` 时保留权威 0。

- [ ] **步骤 2：运行 model 测试验证失败**

运行：

```bash
go test ./model -run 'Test(SumUsedQuotaUsesMeteredTokensForSubscriptionLogs|SumUsedQuotaPreservesAuthoritativeZeroMeteredTokens|LogQuotaDataStoresMeteredTokens)' -count=1
```

预期：失败，原因是 `Stat.TotalTokens` / `Log.MeteredTokens` 或对应规范化字段尚不存在，或 `SumUsedQuota` 未按规范化 token 聚合。

- [ ] **步骤 3：实现最少后端 model 代码**

实现要求：

```go
type Log struct {
    // existing fields...
    MeteredTokens *int `json:"metered_tokens" gorm:"default:null"`
}

type Stat struct {
    Quota       int `json:"quota"`
    TotalTokens int `json:"total_tokens"`
    Rpm         int `json:"rpm"`
    Tpm         int `json:"tpm"`
}

func meteredTokensExpr() string {
    return "COALESCE(SUM(CASE WHEN metered_tokens IS NOT NULL THEN metered_tokens ELSE prompt_tokens + completion_tokens END), 0)"
}
```

`RecordConsumeLog` 写入 `MeteredTokens` 指针。最小实现可先用 `params.PromptTokens + params.CompletionTokens` 的局部变量地址，任务 2 会接入订阅 `other` 的 `subscription_tokens_consumed`；旧日志数据库列为 `NULL` 时 fallback，新日志权威值为 `0` 时必须保持 `0`，不得 fallback。

`SumUsedQuota`：

- 区间查询 select：`COALESCE(SUM(quota), 0) AS quota` 和规范化 token expr `AS total_tokens`。
- 最近 60 秒查询 select：`COUNT(*) AS rpm` 和规范化 token expr `AS tpm`。
- 继续使用 `logGroupCol`。
- 不做 `logs.other` JSON 提取。

`LogQuotaData` 继续接收 `tokenUsed` 参数，但调用方必须传规范化 token。

- [ ] **步骤 4：编写失败的 controller 响应测试**

在 `controller/log_stat_token_test.go` 中新增测试：

```go
func TestGetLogsStatReturnsTotalTokensAndTpm(t *testing.T) {
    // Arrange: 初始化 LOG_DB，插入最近 60 秒内 consume log，MeteredTokens=80，PromptTokens+CompletionTokens=15。
    // Act: 管理员鉴权上下文调用 GetLogsStat 或路由。
    // Assert: JSON data.total_tokens == 80，data.tpm == 80，data.quota 仍存在。
}

func TestGetLogsSelfStatReturnsTotalTokensAndTpm(t *testing.T) {
    // Arrange: 设置 username，插入当前用户最近 60 秒内 consume log，MeteredTokens=80。
    // Act: 调用 GetLogsSelfStat。
    // Assert: JSON data.total_tokens == 80，data.tpm == 80。
}
```

- [ ] **步骤 5：运行 controller 测试验证失败**

运行：

```bash
go test ./controller -run 'TestGetLogs(Stat|SelfStat)ReturnsTotalTokensAndTpm' -count=1
```

预期：失败，原因是 controller 响应尚未包含 `total_tokens`。

- [ ] **步骤 6：实现 controller 响应**

`controller/log.go` 中 `GetLogsStat` 与 `GetLogsSelfStat` 的 `data` 增加：

```go
"total_tokens": stat.TotalTokens,
```

保持 `quota`、`rpm`、`tpm` 不变。

- [ ] **步骤 7：运行任务 1 测试验证通过**

运行：

```bash
go test ./model ./controller -run 'Test(SumUsedQuotaUsesMeteredTokensForSubscriptionLogs|SumUsedQuotaPreservesAuthoritativeZeroMeteredTokens|LogQuotaDataStoresMeteredTokens|GetLogs(Stat|SelfStat)ReturnsTotalTokensAndTpm)' -count=1
```

预期：通过。

---

## 任务 2：订阅日志 token 字段与 quota_data 规范化写入

**文件：**
- 修改：`relay/common/relay_info.go`
- 修改：`service/funding_source.go`
- 修改：`service/billing_session.go`
- 修改：`service/log_info_generate.go`
- 修改：`model/log.go`
- 测试：`service/log_info_generate_test.go`
- 测试：`model/log_stat_token_test.go`

- [ ] **步骤 1：编写失败的订阅 other 测试**

在 `service/log_info_generate_test.go` 中新增测试：

```go
// 本测试文件必须 import net/http/httptest、testing、github.com/gin-gonic/gin、github.com/stretchr/testify/assert。

func testBillingInfoContext(t *testing.T) *gin.Context {
    t.Helper()
    recorder := httptest.NewRecorder()
    ctx, _ := gin.CreateTestContext(recorder)
    return ctx
}

func int64FromOtherValue(t *testing.T, value interface{}) int64 {
    t.Helper()
    switch v := value.(type) {
    case int64:
        return v
    case int:
        return int64(v)
    case float64:
        return int64(v)
    default:
        t.Fatalf("unexpected numeric type %T", value)
        return 0
    }
}

func TestAppendBillingInfoWritesSubscriptionTokenFields(t *testing.T) {
    relayInfo := &relaycommon.RelayInfo{
        BillingSource: "subscription",
        SubscriptionId: 10,
        SubscriptionPlanId: 2,
        SubscriptionPlanTitle: "Basic",
        SubscriptionPreConsumed: 3000,
        SubscriptionPostDelta: -952,
        SubscriptionTokenLimit: 1000000000,
        SubscriptionTokenUsedAfterPreConsume: 123000,
        SubscriptionTokenUnlimited: false,
        SubscriptionDistributorTokenBilling: true,
    }

    other := GenerateTextOtherInfo(testBillingInfoContext(t), relayInfo, 0, 0, 0, 0, 0, 0, 0)

    assert.Equal(t, int64(2048), int64FromOtherValue(t, other["subscription_tokens_consumed"]))
    assert.Equal(t, int64(122048), int64FromOtherValue(t, other["subscription_token_used"]))
    assert.Equal(t, int64(999877952), int64FromOtherValue(t, other["subscription_token_remaining"]))
    assert.Equal(t, false, other["subscription_token_unlimited"])
}

func TestAppendBillingInfoClampsNegativeSubscriptionTokenConsumption(t *testing.T) {
    relayInfo := &relaycommon.RelayInfo{
        BillingSource: "subscription",
        SubscriptionId: 10,
        SubscriptionPreConsumed: 10,
        SubscriptionPostDelta: -50,
        SubscriptionTokenLimit: 1000,
        SubscriptionTokenUsedAfterPreConsume: 20,
        SubscriptionDistributorTokenBilling: true,
    }

    other := GenerateTextOtherInfo(testBillingInfoContext(t), relayInfo, 0, 0, 0, 0, 0, 0, 0)

    assert.Equal(t, int64(0), int64FromOtherValue(t, other["subscription_tokens_consumed"]))
    assert.Equal(t, int64(0), int64FromOtherValue(t, other["subscription_token_used"]))
    assert.Equal(t, int64(1000), int64FromOtherValue(t, other["subscription_token_remaining"]))
}

func TestAppendBillingInfoDoesNotWriteTokenFieldsForLegacyAmountSubscription(t *testing.T) {
    relayInfo := &relaycommon.RelayInfo{
        BillingSource: "subscription",
        SubscriptionPreConsumed: 30,
        SubscriptionPostDelta: 20,
        SubscriptionAmountTotal: 100,
        SubscriptionAmountUsedAfterPreConsume: 50,
    }
    other := GenerateTextOtherInfo(testBillingInfoContext(t), relayInfo, 0, 0, 0, 0, 0, 0, 0)
    for _, key := range []string{
        "subscription_tokens_consumed",
        "subscription_token_limit",
        "subscription_token_used",
        "subscription_token_remaining",
        "subscription_token_unlimited",
    } {
        _, exists := other[key]
        assert.False(t, exists, key)
    }
}
```

- [ ] **步骤 2：运行 service 测试验证失败**

运行：

```bash
go test ./service -run 'TestAppendBillingInfo(WritesSubscriptionTokenFields|ClampsNegativeSubscriptionTokenConsumption|DoesNotWriteTokenFieldsForLegacyAmountSubscription)' -count=1
```

预期：失败，原因是 `RelayInfo` token 字段或 `other` 输出不存在。

- [ ] **步骤 3：实现 RelayInfo token 字段与同步**

在 `relay/common/relay_info.go` 增加 token 命名字段：

```go
SubscriptionTokenLimit               int64
SubscriptionTokenUsedAfterPreConsume int64
SubscriptionTokenUnlimited           bool
SubscriptionDistributorTokenBilling  bool
```

在 `service/billing_session.go` 同步 `SubscriptionFunding` 到 `RelayInfo`：

- distributor token billing：写入 `SubscriptionTokenLimit`、`SubscriptionTokenUsedAfterPreConsume`、`SubscriptionDistributorTokenBilling=true`。
- token limit 为 0 且后端选择规则判定不限量：`SubscriptionTokenUnlimited=true`。
- legacy amount subscription：不写 token 权威字段。

保持旧 `SubscriptionAmountTotal` / `SubscriptionAmountUsedAfterPreConsume` 兼容字段。

- [ ] **步骤 4：实现 `appendBillingInfo` token other 字段**

`service/log_info_generate.go`：

- 仅当 `relayInfo.SubscriptionDistributorTokenBilling` 为 true 时写入 `subscription_token_*` 字段。
- `consumed := SubscriptionPreConsumed + SubscriptionPostDelta`，小于 0 时归 0。
- `usedFinal := SubscriptionTokenUsedAfterPreConsume + SubscriptionPostDelta`，小于 0 时归 0。
- 不限量：`subscription_token_unlimited=true`，`subscription_token_remaining=0`。
- 有限量：`remaining := max(0, SubscriptionTokenLimit - usedFinal)`。
- 旧 `subscription_total` 等兼容字段可继续写入，但前端不作为权威字段。

- [ ] **步骤 5：编写失败的 `RecordConsumeLog` 规范化写入测试**

在 `model/log_stat_token_test.go` 中新增测试：

```go
func TestRecordConsumeLogUsesSubscriptionConsumedForMeteredTokensAndQuotaData(t *testing.T) {
    // Arrange: Gin context 带 username/request id；params PromptTokens=10, CompletionTokens=5, Quota=100,
    // Other: map[string]interface{}{"billing_source":"subscription","subscription_tokens_consumed":int64(80)}，覆盖 `GenerateTextOtherInfo` 由 RelayInfo int64 字段写出的真实类型。
    // Act: 设置 common.DataExportEnabled=true，调用 RecordConsumeLog；在同 package 测试中轮询 CacheQuotaData 直到 token_used=80 后调用 SaveQuotaDataCache。
    // Assert:
    //   logs.metered_tokens == 80
    //   quota_data.token_used == 80
    //   quota_data.quota == 100
}

func TestRecordConsumeLogTreatsZeroSubscriptionConsumedAsAuthoritative(t *testing.T) {
    // Arrange: PromptTokens=10, CompletionTokens=5, Quota=100,
    // Other: map[string]interface{}{"billing_source":"subscription","subscription_tokens_consumed":int64(0)}。
    // Act: RecordConsumeLog + SaveQuotaDataCache。
    // Assert:
    //   logs.metered_tokens == 0
    //   quota_data.token_used == 0
    //   quota_data.quota == 100
}

func TestRecordConsumeLogFallsBackWhenSubscriptionConsumedMissing(t *testing.T) {
    // Arrange: PromptTokens=10, CompletionTokens=5, Quota=100, Other: map[string]interface{}{"billing_source":"subscription"}
    // Act: RecordConsumeLog + SaveQuotaDataCache。
    // Assert:
    //   logs.metered_tokens == 15
    //   quota_data.token_used == 15
    //   quota_data.quota == 100
}
```

- [ ] **步骤 6：运行 model 测试验证失败**

运行：

```bash
go test ./model -run 'TestRecordConsumeLog(UsesSubscriptionConsumedForMeteredTokensAndQuotaData|TreatsZeroSubscriptionConsumedAsAuthoritative|FallsBackWhenSubscriptionConsumedMissing)' -count=1
```

预期：失败，原因是 `RecordConsumeLog` 未从 `Other.subscription_tokens_consumed` 写入 `MeteredTokens`。

- [ ] **步骤 7：实现 `RecordConsumeLog` 规范化 token helper**

`model/log.go` 增加 helper：

```go
func normalizedMeteredTokens(promptTokens, completionTokens int, other map[string]interface{}) int {
    if other != nil {
        if v, ok := intFromMapValue(other["subscription_tokens_consumed"]); ok {
            if v < 0 {
                return 0
            }
            return v
        }
    }
    total := promptTokens + completionTokens
    if total < 0 {
        return 0
    }
    return total
}
```

`intFromMapValue` 必须至少支持 `int`、`int64`、`float64`；若解析已经过 JSON decoder，还必须支持 `json.Number`。字段存在但为负数时归 0；字段缺失或不可解析才 fallback 到 prompt + completion。

`RecordConsumeLog`：

- `meteredTokens := normalizedMeteredTokens(params.PromptTokens, params.CompletionTokens, params.Other)`。
- `Log{MeteredTokens: &meteredTokens}`，确保 `meteredTokens == 0` 也写入非 `NULL` 权威值。
- `LogQuotaData(..., meteredTokens)`。

- [ ] **步骤 8：运行任务 2 测试验证通过**

运行：

```bash
go test ./model ./service -run 'Test(AppendBillingInfo(WritesSubscriptionTokenFields|ClampsNegativeSubscriptionTokenConsumption|DoesNotWriteTokenFieldsForLegacyAmountSubscription)|RecordConsumeLog(UsesSubscriptionConsumedForMeteredTokensAndQuotaData|TreatsZeroSubscriptionConsumedAsAuthoritative|FallsBackWhenSubscriptionConsumedMissing)|SumUsedQuotaUsesMeteredTokensForSubscriptionLogs|SumUsedQuotaPreservesAuthoritativeZeroMeteredTokens|LogQuotaDataStoresMeteredTokens)' -count=1
```

预期：通过。

---

## 任务 3：订阅 self summary 主订阅契约

**文件：**
- 修改：`model/subscription.go`
- 修改：`controller/subscription.go`
- 测试：`controller/subscription_self_summary_test.go`

- [ ] **步骤 1：编写失败的 self summary controller 测试**

新增测试：

```go
func TestGetSubscriptionSelfReturnsSummaryAndCompatFields(t *testing.T) {
    // Arrange: 用户存在 active distributor subscription。
    // Act: 调用 GetSubscriptionSelf。
    // Assert: data 包含 billing_preference、subscriptions、all_subscriptions、summary。
}

func TestGetSubscriptionSelfSummaryUsesPrimaryBillableSubscription(t *testing.T) {
    // Arrange: 同用户有两个 active subscription：
    //   A: end_time 较早且可扣费，TokenLimit=1000, TokenUsed=200, ConcurrencyLimit=1。
    //   B: end_time 较晚，TokenLimit=9999, TokenUsed=0, ConcurrencyLimit=50。
    // Act: GetSubscriptionSelf。
    // Assert: summary 来自 A，不是 A+B 求和。
}

func TestGetSubscriptionSelfSummarySkipsExhaustedPrimaryCandidate(t *testing.T) {
    // Arrange: 同用户有两个 active subscription：
    //   A: end_time 较早但 TokenLimit=1000, TokenUsed=1000。
    //   B: end_time 较晚且 TokenLimit=9999, TokenUsed=0。
    // Act: GetSubscriptionSelf。
    // Assert: summary 来自 B；A 已耗尽，不是实际请求扣费层会使用的主订阅。
}

func TestGetSubscriptionSelfSummaryReturnsExplicitUnlimitedTrial(t *testing.T) {
    // Arrange: active trial_code 或 invite_trial 分销订阅，TokenLimit=0，ConcurrencyLimit=1。
    // Act: GetSubscriptionSelf。
    // Assert: summary.token_unlimited=true，summary.token_remaining=0，并返回 subscription_id、plan_id、primary_plan_title、concurrency_limit。
}

func TestGetSubscriptionSelfSummaryReturnsZeroWhenNoBillableSubscription(t *testing.T) {
    // Arrange: 用户没有 active subscription，或只有已耗尽有限订阅。
    // Act: GetSubscriptionSelf。
    // Assert: summary.active_count=0，token_limit/token_used/token_remaining/concurrency_limit 均为 0，token_unlimited=false。
}

func TestGetSubscriptionSelfSummaryDoesNotTreatLegacyZeroLimitAsUnlimited(t *testing.T) {
    // Arrange: active legacy / non-distributor subscription token_limit=0。
    // Assert: summary.token_unlimited=false 或 active_count=0，不显示 Unlimited。
}
```

- [ ] **步骤 2：运行测试验证失败**

运行：

```bash
go test ./controller -run 'TestGetSubscriptionSelf(ReturnsSummaryAndCompatFields|SummaryUsesPrimaryBillableSubscription|SummarySkipsExhaustedPrimaryCandidate|SummaryReturnsExplicitUnlimitedTrial|SummaryReturnsZeroWhenNoBillableSubscription|SummaryDoesNotTreatLegacyZeroLimitAsUnlimited)' -count=1
```

预期：失败，原因是 `summary` 不存在或逻辑不符合要求。

- [ ] **步骤 3：抽取 model 层主订阅选择 helper**

在 `model/subscription.go` 抽取供预扣和 self summary 复用的 helper。要求：

- active：`user_id = ? AND status = active AND end_time > now`。
- 排序与 `PreConsumeUserSubscriptionByUnits` 当前规则一致。
- 必须过滤非分销订阅。
- 有限 token：`TokenLimit > 0` 且 `TokenLimit - TokenUsed >= 1` 才能作为 self summary 主订阅；已耗尽有限订阅必须跳过。
- 不限量：只接受请求层明确判定的不限量试用 / 不限量订阅。
- 返回主订阅、plan、summary 所需字段。

不改变请求预扣行为；如重构 `PreConsumeUserSubscriptionByUnits`，必须保持现有测试通过。

- [ ] **步骤 4：实现 controller summary 响应**

`controller/subscription.go`：

```go
common.ApiSuccess(c, gin.H{
    "billing_preference": pref,
    "subscriptions": activeSubscriptions,
    "all_subscriptions": allSubscriptions,
    "summary": summary,
})
```

summary 字段：

- `active_count`
- `subscription_id`
- `plan_id`
- `primary_plan_title`
- `token_limit`
- `token_used`
- `token_remaining`
- `token_unlimited`
- `concurrency_limit`
- `next_reset_time`
- `end_time`

- [ ] **步骤 5：运行任务 3 测试验证通过**

运行：

```bash
go test ./controller ./model -run 'Test(GetSubscriptionSelf(ReturnsSummaryAndCompatFields|SummaryUsesPrimaryBillableSubscription|SummarySkipsExhaustedPrimaryCandidate|SummaryReturnsExplicitUnlimitedTrial|SummaryReturnsZeroWhenNoBillableSubscription|SummaryDoesNotTreatLegacyZeroLimitAsUnlimited)|SubscriptionDistributor)' -count=1
```

预期：通过。

---

## 任务 4：Pricing API 脱敏与倍率同步迁移

**文件：**
- 修改：`controller/pricing.go`
- 修改：`controller/ratio_sync.go`
- 修改：`router/api-router.go`（本计划不新增 pricing 路由；该文件仅用于确认 `/api/pricing` 仍使用 `TryUserAuth`）
- 测试：`controller/pricing_directory_test.go`
- 测试：`controller/ratio_sync_test.go`

- [ ] **步骤 1：编写失败的 pricing 脱敏测试**

新增测试：

```go
func TestGetPricingRedactsCostFieldsForAnonymousAndUser(t *testing.T) {
    // Arrange: pricing cache 包含 model_ratio/model_price/billing_expr/group_ratio/cache/audio/image 字段。
    // Act: 未登录和普通用户调用 GetPricing。
    // Assert: JSON data 中没有 model_ratio、model_price、completion_ratio、cache_ratio、create_cache_ratio、image_ratio、audio_ratio、audio_completion_ratio、billing_mode、billing_expr；根对象没有 group_ratio。
}

func TestGetPricingKeepsDirectoryFieldsForAnonymousAndUser(t *testing.T) {
    // Assert: model_name、description、icon、tags、vendor_id、enable_groups、supported_endpoint_types 仍存在。
}

func TestGetPricingKeepsCostFieldsForAdmin(t *testing.T) {
    // Arrange: 管理员用户通过 TryUserAuth 等价上下文调用 GetPricing。
    // Assert: JSON data 中保留 model_ratio、model_price、billing_expr，根对象保留 group_ratio。
}
```

- [ ] **步骤 2：运行 pricing 测试验证失败**

运行：

```bash
go test ./controller -run 'TestGetPricing(RedactsCostFieldsForAnonymousAndUser|KeepsDirectoryFieldsForAnonymousAndUser|KeepsCostFieldsForAdmin)' -count=1
```

预期：失败，当前 `/api/pricing` 返回成本字段。

- [ ] **步骤 3：实现用户模型目录 DTO**

`controller/pricing.go`：

- 增加用户侧 DTO，不包含成本字段。
- `TryUserAuth` 未登录或普通用户：返回目录 DTO，不返回 `group_ratio`。
- 管理员：同一路径保留现有成本响应；在 `GetPricing` 中根据 `id` 加载用户并使用 `user.Role >= common.RoleAdminUser` 判断，不依赖 `TryUserAuth` 设置 `role`。

不要删除 `model.Pricing` 成本字段。

- [ ] **步骤 4：编写失败的 ratio sync 迁移测试**

新增测试：

```go
func TestRatioSyncDefaultEndpointUsesRatioConfig(t *testing.T) {
    // Assert: defaultEndpoint == "/api/ratio_config"。
}

func TestRatioSyncRejectsRedactedPricingPayload(t *testing.T) {
    // Arrange: 模拟 /api/pricing 脱敏响应，data 中有 model_name 等目录字段，但没有任何 pricingSyncFields。
    // Act: 调用新抽取的 parsePricingSyncPayload 或 convertPricingPayloadToRatioData。
    // Assert: 返回包含 "no sync fields" 的错误，且转换结果不包含任何 model_ratio/completion_ratio/cache_ratio/model_price 空配置。
}
```

- [ ] **步骤 5：运行 ratio sync 测试验证失败**

运行：

```bash
go test ./controller -run 'TestRatioSync(DefaultEndpointUsesRatioConfig|RejectsRedactedPricingPayload)' -count=1
```

预期：失败，默认 endpoint 仍是 `/api/pricing` 或脱敏响应被当作有效源。

- [ ] **步骤 6：实现 ratio sync 迁移**

`controller/ratio_sync.go`：

- `defaultEndpoint = "/api/ratio_config"`。
- 抽取 `parsePricingSyncPayload` 或 `convertPricingPayloadToRatioData` helper；`FetchUpstreamRatios` 复用该 helper。
- type2 `/api/pricing` 解析使用 raw map 或指针字段检测字段是否真实存在，不能把缺失成本字段反序列化成 `0`。
- 对脱敏 `/api/pricing` 响应，如果解析不到任何 `pricingSyncFields`，返回包含 `no sync fields` 的错误并不写配置。

前端系统设置默认 endpoint 在任务 9 修改。

- [ ] **步骤 7：运行任务 4 测试验证通过**

运行：

```bash
go test ./controller -run 'Test(GetPricing(RedactsCostFieldsForAnonymousAndUser|KeepsDirectoryFieldsForAnonymousAndUser|KeepsCostFieldsForAdmin)|RatioSync(DefaultEndpointUsesRatioConfig|RejectsRedactedPricingPayload))' -count=1
```

预期：通过。

---

## 任务 5：前端类型、日志 helper 与统计 badge

**文件：**
- 修改：`web/default/src/features/usage-logs/types.ts`
- 修改：`web/default/src/features/usage-logs/constants.ts`
- 修改：`web/default/src/features/usage-logs/lib/format.ts`
- 修改：`web/default/src/features/usage-logs/components/common-logs-stats.tsx`
- 测试：`web/default/src/features/usage-logs/lib/format.test.ts`

- [ ] **步骤 1：编写失败的 usage logs helper 测试**

`format.test.ts` 增加；若文件已有同源 import，合并 import，避免重复：

```ts
import assert from 'node:assert/strict'
import { test } from 'node:test'
import type { UsageLog } from '../data/schema'
import { getLogTokenUsage } from './format'

function makeUsageLog(overrides: Partial<UsageLog>): UsageLog {
  return {
    id: 1,
    created_at: 1700000000,
    type: 2,
    username: 'alice',
    model_name: 'gpt-test',
    token_name: 'test-key',
    quota: 1000,
    prompt_tokens: 0,
    completion_tokens: 0,
    user_id: 1,
    content: '',
    use_time: 1,
    is_stream: false,
    channel: 1,
    channel_name: 'test-channel',
    token_id: 1,
    group: 'default',
    ip: '',
    request_id: '',
    other: '',
    ...overrides,
  } as UsageLog
}

test('getLogTokenUsage prefers subscription consumed tokens', () => {
  const log = makeUsageLog({ prompt_tokens: 10, completion_tokens: 5 })
  const other = { subscription_tokens_consumed: 80, subscription_consumed: 20 }
  assert.equal(getLogTokenUsage(log, other), 80)
})

test('getLogTokenUsage falls back to legacy subscription_consumed', () => {
  const log = makeUsageLog({ prompt_tokens: 10, completion_tokens: 5 })
  const other = { subscription_consumed: 20 }
  assert.equal(getLogTokenUsage(log, other), 20)
})

test('getLogTokenUsage falls back to legacy prompt and completion tokens', () => {
  const log = makeUsageLog({ prompt_tokens: 10, completion_tokens: 5 })
  assert.equal(getLogTokenUsage(log, null), 15)
})
```

- [ ] **步骤 2：运行测试验证失败**

运行：

```bash
cd web/default
bun test src/features/usage-logs/lib/format.test.ts
```

预期：失败，helper 不存在。

- [ ] **步骤 3：实现 types/constants/helper**

- `LogStatistics` 增加 `total_tokens`。
- `DEFAULT_LOG_STATS` 增加 `total_tokens: 0`。
- `LogOtherData` 增加：

```ts
subscription_token_limit?: number
subscription_token_used?: number
subscription_token_remaining?: number
subscription_token_unlimited?: boolean
subscription_tokens_consumed?: number
subscription_consumed?: number
```

- `format.ts` 导出：

```ts
export function getLegacyPromptCompletionTokens(log: UsageLog): number
export function getLogTokenUsage(log: UsageLog, other: LogOtherData | null): number
// 优先级必须是 subscription_tokens_consumed -> subscription_consumed -> prompt_tokens + completion_tokens。
```

- [ ] **步骤 4：更新 stats badge**

`common-logs-stats.tsx`：

- import `formatTokens`，移除 `formatLogQuota`。
- label 改为 `Total Tokens`。
- value 改为 `formatTokens(stats?.total_tokens || 0)`。

- [ ] **步骤 5：运行任务 5 测试验证通过**

运行：

```bash
cd web/default
bun test src/features/usage-logs/lib/format.test.ts
```

预期：测试通过。

---

## 任务 6：Dashboard chart token-only

**文件：**
- 修改：`web/default/src/features/dashboard/lib/charts.ts`
- 修改：`web/default/src/features/dashboard/lib/stats.ts`
- 修改：`web/default/src/features/dashboard/types.ts`
- 修改：`web/default/src/features/dashboard/components/models/consumption-distribution-chart.tsx`
- 修改：`web/default/src/features/dashboard/components/models/log-stat-cards.tsx`
- 修改：`web/default/src/features/dashboard/hooks/use-dashboard-config.tsx`
- 修改：`web/default/src/features/dashboard/index.tsx`
- 修改：`web/default/src/routes/_authenticated/dashboard/$section.tsx`
- 修改：`web/default/src/features/dashboard/components/users/user-charts.tsx`
- 测试：`web/default/src/features/dashboard/lib/charts.test.ts`

- [ ] **步骤 1：编写失败的 chart 测试**

新增 `charts.test.ts`：

```ts
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import { processChartData, processUserChartData } from './charts'
import { calculateDashboardStats } from './stats'

function readDashboardIndexSource(): string {
  return readFileSync(new URL('../index.tsx', import.meta.url), 'utf8')
}

test('processChartData token totals and token trend use token_used instead of quota', () => {
  const data = [
    { created_at: 1700000000, model_name: 'expensive-low-token', quota: 100000, token_used: 10, count: 100 },
    { created_at: 1700000000, model_name: 'cheap-high-token', quota: 1, token_used: 1000, count: 1 },
  ]
  const result = processChartData(data, 'day', (x) => x)
  assert.equal(result.totalTokensDisplay, '1,010')
  const trendValues = result.spec_line.data[0].values as Array<{ Tokens: number }>
  assert.equal(trendValues.reduce((sum, item) => sum + item.Tokens, 0), 1010)
  const rankValues = result.spec_rank_bar.data[0].values as Array<{ Model: string; Count: number }>
  assert.equal(rankValues[0].Model, 'expensive-low-token')
})

test('calculateDashboardStats totals by token_used instead of quota', () => {
  const data = [
    { created_at: 1700000000, model_name: 'expensive-low-token', quota: 100000, token_used: 10, count: 1 },
    { created_at: 1700000000, model_name: 'cheap-high-token', quota: 1, token_used: 1000, count: 1 },
  ]
  const stats = calculateDashboardStats(data)
  assert.equal(stats.totalTokens, 1010)
  assert.ok(!('totalQuota' in stats))
  // token 用量聚合中 cheap-high-token 应贡献更多 token，但请求数排行仍由 count 测试覆盖。
})

test('processUserChartData ranks users by token_used instead of quota', () => {
  const data = [
    { created_at: 1700000000, username: 'alice', quota: 100000, token_used: 10, count: 1 },
    { created_at: 1700000000, username: 'bob', quota: 1, token_used: 1000, count: 1 },
  ]
  const result = processUserChartData(data, 'day', (x) => x)
  const rankValues = result.spec_user_rank.data[0].values as Array<{ User: string }>
  assert.equal(rankValues[0].User, 'bob')
})

test('dashboard users section is admin-only at mount point', () => {
  const source = readDashboardIndexSource()
  assert.match(source, /activeSection === 'users' && isAdmin/)
  assert.doesNotMatch(source, /activeSection === 'users' &&\s*\([\s\S]*?<LazyUserCharts[\s\S]*?\/\>/)
})
```

- [ ] **步骤 2：运行 chart 测试验证失败**

运行：

```bash
cd web/default
bun test src/features/dashboard/lib/charts.test.ts
```

预期：失败，当前代码仍使用 quota / `totalQuotaDisplay`。

- [ ] **步骤 3：实现 chart token-only**

`charts.ts`：

- 删除 `getCurrencyDisplay` 与 `renderQuotaCompat`。
- 内部 raw 字段改为 `rawTokens`。
- yField 改为 `Tokens`。
- `totalQuotaDisplay` 改为 `totalTokensDisplay`。
- `processUserChartData` 全部按 `token_used` 聚合、排序、tooltip。
- `calculateDashboardStats` / `LogStatCards` / `useModelStatCardsConfig` 不再使用 `totalQuota`、`quota` key 或 `formatQuota()` 作为主展示。
- `Total Tokens Used` 必须读取 `token_used` / `totalTokens` 并用 `formatTokens()` 或整数格式化。

`types.ts`：

```ts
export interface ProcessedChartData {
  // ...
  totalTokensDisplay: string
  totalCountDisplay: string
}
```

`consumption-distribution-chart.tsx` 使用 `totalTokensDisplay`。

- [ ] **步骤 4：更新 dashboard config 和用户图文案**

`use-dashboard-config.tsx`：

- `Total Quota` -> `Total Tokens Used`。
- 删除 currency 拼接。

`user-charts.tsx`：

- `User Consumption Ranking` -> `User Token Usage Ranking`。
- `User Consumption Trend` -> `User Token Usage Trend`。
- `web/default/src/features/dashboard/index.tsx` 或 dashboard section route 必须按 `ROLE.ADMIN` 处理 `/dashboard/users`：普通用户深链重定向 overview 或显示无权限提示，不挂载 `UserCharts`，不得调用 `/api/data/users`；不得改为 `/api/data/self`。

- [ ] **步骤 5：运行任务 6 测试验证通过**

运行：

```bash
cd web/default
bun test src/features/dashboard/lib/charts.test.ts
```

预期：通过。

---

## 任务 7：Dashboard 概览订阅 summary

**文件：**
- 修改：`web/default/src/features/subscriptions/types.ts`
- 修改：`web/default/src/features/dashboard/components/overview/summary-cards.tsx`
- 修改：`web/default/src/features/dashboard/hooks/use-dashboard-config.tsx`
- 测试：`web/default/src/features/dashboard/lib/subscription-summary.test.ts`

- [ ] **步骤 1：编写失败的 summary 纯函数测试**

新增纯函数 helper：`web/default/src/features/dashboard/lib/subscription-summary.ts`，测试：

```ts
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { buildSubscriptionSummaryView } from './subscription-summary'

test('formatSubscriptionSummary displays finite remaining tokens', () => {
  const summary = { token_limit: 1000, token_used: 250, token_remaining: 750, token_unlimited: false, active_count: 1, concurrency_limit: 1 }
  const result = buildSubscriptionSummaryView(summary)
  assert.equal(result.remainingLabel, '750')
  assert.equal(result.healthLevel, 'healthy')
})

test('formatSubscriptionSummary treats unlimited only when token_unlimited is true', () => {
  const summary = { token_limit: 0, token_used: 0, token_remaining: 0, token_unlimited: false, active_count: 1, concurrency_limit: 1 }
  const result = buildSubscriptionSummaryView(summary)
  assert.notEqual(result.remainingLabel, 'Unlimited')
})

test('formatSubscriptionSummary displays Unlimited only for explicit unlimited summary', () => {
  const summary = { token_limit: 0, token_used: 250, token_remaining: 0, token_unlimited: true, active_count: 1, concurrency_limit: 1 }
  const result = buildSubscriptionSummaryView(summary)
  assert.equal(result.remainingLabel, 'Unlimited')
  assert.equal(result.healthLevel, 'healthy')
})

test('formatSubscriptionSummary marks missing subscription as required', () => {
  const summary = { token_limit: 0, token_used: 0, token_remaining: 0, token_unlimited: false, active_count: 0, concurrency_limit: 0 }
  const result = buildSubscriptionSummaryView(summary)
  assert.equal(result.remainingLabel, 'Subscription required')
  assert.equal(result.healthLevel, 'critical')
})
```

- [ ] **步骤 2：运行 summary 测试验证失败**

运行：

```bash
cd web/default
bun test src/features/dashboard/lib/subscription-summary.test.ts
```

预期：失败，helper 不存在。

- [ ] **步骤 3：实现 subscription summary types/helper**

`subscriptions/types.ts`：

```ts
export interface SelfSubscriptionSummary {
  active_count: number
  subscription_id?: number
  plan_id?: number
  primary_plan_title?: string
  token_limit: number
  token_used: number
  token_remaining: number
  token_unlimited: boolean
  concurrency_limit: number
  next_reset_time?: number
  end_time?: number
}

export interface SelfSubscriptionData {
  billing_preference: string
  subscriptions: UserSubscriptionRecord[]
  all_subscriptions: UserSubscriptionRecord[]
  summary: SelfSubscriptionSummary
}
```

`subscription-summary.ts` 输出 UI 所需数据，不在组件里重复业务判断；有限订阅显示 remaining/used，`token_unlimited === true` 才显示 `Unlimited`，`active_count === 0` 或无 summary 时显示 `Subscription required` 且 `healthLevel='critical'`。

- [ ] **步骤 4：改造 SummaryCards**

`summary-cards.tsx`：

- 使用 `getSelfSubscriptionFull()` 获取 summary。
- 不读取 `user.quota` / `user.used_quota`。
- 最近 24h 使用 `item.token_used`。
- 删除 currency display 逻辑。
- 删除 balance 回推 sparkline。
- 卡片显示：套餐剩余 Token、本周期已用 Token、最近 24h Token 用量、请求数。

`useSummaryCardsConfig` 入参改为：

```ts
{
  remainingTokensDisplay: string
  cycleTokensDisplay: string
  recentTokensDisplay: string
  requestCountDisplay: string
}
```

- [ ] **步骤 5：运行任务 7 测试验证通过**

运行：

```bash
cd web/default
bun test src/features/dashboard/lib/subscription-summary.test.ts
```

预期：通过。

---

## 任务 8：Usage logs 表格与详情去价格化

**文件：**
- 修改：`web/default/src/features/usage-logs/components/columns/common-logs-columns.tsx`
- 修改：`web/default/src/features/usage-logs/components/dialogs/details-dialog.tsx`
- 修改：`web/default/src/features/usage-logs/lib/format.ts`
- 测试：`web/default/src/features/usage-logs/lib/format.test.ts`

- [ ] **步骤 1：编写失败的 helper / policy 测试**

在 `format.test.ts` 增加；沿用任务 5 的 `makeUsageLog`，若文件已有同源 import，合并 import，避免重复：

```ts
import assert from 'node:assert/strict'
import { test } from 'node:test'
import {
  getLogTokenUsageColumnValue,
  getTokenNameMeta,
  shouldShowCostDetails,
} from './format'

test('shouldShowCostDetails only allows admins', () => {
  assert.equal(shouldShowCostDetails(false), false)
  assert.equal(shouldShowCostDetails(true), true)
})

test('getTokenNameMeta hides ratios for non-admin users', () => {
  const other = { group_ratio: 2, user_group_ratio: 3, group: 'default' }
  assert.deepEqual(getTokenNameMeta(other, false), ['default'])
  assert.deepEqual(getTokenNameMeta(other, true), ['default', '3x'])
})

test('getLogTokenUsageColumnValue sorts by helper result instead of quota', () => {
  const rows = [
    makeUsageLog({ quota: 100000, prompt_tokens: 10, completion_tokens: 5 }),
    makeUsageLog({ quota: 1, prompt_tokens: 1000, completion_tokens: 0 }),
  ]
  assert.equal(getLogTokenUsageColumnValue(rows[0]), 15)
  assert.equal(getLogTokenUsageColumnValue(rows[1]), 1000)
})
```

- [ ] **步骤 2：运行测试验证失败**

运行：

```bash
cd web/default
bun test src/features/usage-logs/lib/format.test.ts
```

预期：失败，policy helper 不存在。

- [ ] **步骤 3：实现 helper 并改造 Token Usage 列**

`common-logs-columns.tsx`：

- Token Usage 列使用可测试 helper `getLogTokenUsageColumnValue(row)` 或等价 `accessorFn: (row) => getLogTokenUsage(row, parseLogOther(row.other))`；不得使用 `accessorKey: 'quota'`。
- Header：`Token Usage` 或 `Deducted Tokens`。
- 普通用户不显示 `formatLogQuota(log.quota)`。
- 订阅 badge tooltip 显示 token 数。

- [ ] **步骤 4：隐藏普通用户倍率和成本摘要**

- `TokenNameCell` 使用 `getTokenNameMeta(other, isAdmin && sensitiveVisible)`，普通用户不展示 `group_ratio` / `user_group_ratio` / `x`。
- `buildDetailSegments` 对普通用户不生成价格、倍率、`Dynamic Pricing`、`$/M` 片段。
- `DetailsDialog` 中 `BillingBreakdown`、`DynamicPricingBreakdown`、legacy fee quota 仅管理员可见。
- 普通用户优先显示 `TokenBreakdown`。

- [ ] **步骤 5：运行任务 8 测试验证通过**

运行：

```bash
cd web/default
bun test src/features/usage-logs/lib/format.test.ts
```

预期：通过。

---

## 任务 9：模型目录与公开 pricing API 前端适配

**文件：**
- 修改：`web/default/src/features/pricing/api.ts`
- 修改：`web/default/src/features/pricing/types.ts`
- 修改：`web/default/src/features/pricing/index.tsx`
- 修改：`web/default/src/features/pricing/constants.ts`
- 修改：`web/default/src/features/pricing/hooks/use-filters.ts`
- 修改：`web/default/src/features/pricing/lib/filters.ts`
- 创建：`web/default/src/features/pricing/columns.ts`，导出 `buildModelDirectoryColumns(options: { isAdmin: boolean })`，由 `pricing-columns.tsx` 复用。
- 创建：`web/default/src/features/pricing/search.ts`，导出 `sanitizePricingSearchForRole(search: PricingSearch, isAdmin: boolean)`，由 pricing route 深链恢复逻辑复用。
- 修改：`web/default/src/features/pricing/components/pricing-toolbar.tsx`
- 修改：`web/default/src/features/pricing/components/pricing-sidebar.tsx`
- 修改：`web/default/src/features/pricing/components/pricing-table.tsx`
- 修改：`web/default/src/features/pricing/components/pricing-columns.tsx`
- 修改：`web/default/src/features/pricing/components/model-card-grid.tsx`
- 修改：`web/default/src/features/pricing/components/model-card.tsx`
- 修改：`web/default/src/features/pricing/components/model-details.tsx`
- 修改：`web/default/src/routes/pricing/index.tsx`
- 修改：`web/default/src/routes/pricing/$modelId/index.tsx`
- 修改：`web/default/src/features/system-settings/models/constants.ts`
- 测试：`web/default/src/features/pricing/model-directory.test.ts`

- [ ] **步骤 1：编写失败的 model directory 测试**

新增测试：

```ts
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { DEFAULT_ENDPOINT } from '@/features/system-settings/models/constants'
import { buildModelDirectoryColumns } from './columns'
import { sanitizePricingSearchForRole } from './search'

test('public model directory columns do not include pricing fields', () => {
  const columns = buildModelDirectoryColumns({ isAdmin: false })
  assert.doesNotMatch(columns.map((c) => c.id || c.accessorKey).join(','), /price|ratio|billing/i)
})

test('admin model directory columns keep pricing fields', () => {
  const columns = buildModelDirectoryColumns({ isAdmin: true })
  assert.match(columns.map((c) => c.id || c.accessorKey).join(','), /price|ratio|billing/i)
})


test('public pricing route strips cost search params', () => {
  const search = sanitizePricingSearchForRole({ sort: 'price-low', quotaType: 1, tokenUnit: 'M', rechargePrice: 10 }, false)
  assert.deepEqual(search, {})
})

test('admin pricing route preserves cost search params', () => {
  const search = sanitizePricingSearchForRole({ sort: 'price-low', quotaType: 1, tokenUnit: 'M', rechargePrice: 10 }, true)
  assert.equal(search.sort, 'price-low')
  assert.equal(search.quotaType, 1)
  assert.equal(search.rechargePrice, 10)
})
test('default ratio sync endpoint uses ratio_config', () => {
  assert.equal(DEFAULT_ENDPOINT, '/api/ratio_config')
})

// 若 buildModelDirectoryColumns 当前不存在，任务 9 必须先抽取这个纯函数，让测试失败原因是函数缺失而非测试脚手架缺失。
```

- [ ] **步骤 2：运行测试验证失败**

运行：

```bash
cd web/default
bun test src/features/pricing/model-directory.test.ts
```

预期：失败，测试 helper 或默认 endpoint 不符合要求。

- [ ] **步骤 3：更新 pricing 类型与 API 消费**

`PricingModel` 中成本字段改为 optional，前端必须能处理用户侧响应缺少成本字段。

- `/pricing` 页面从 auth store 计算 `isAdmin = role >= ROLE.ADMIN`，并向 `PricingToolbar`、`PricingSidebar`、`PricingTable/usePricingColumns`、`ModelCardGrid/ModelCard`、`ModelDetailsDrawer/ModelDetailsContent` 与模型详情路由传递。
- `columns.ts` 导出 `buildModelDirectoryColumns({ isAdmin })`；`pricing-columns.tsx` 只消费该 helper 输出，不在组件内复制 public/admin 列分支。
- `search.ts` 导出 `sanitizePricingSearchForRole(search, isAdmin)`；`web/default/src/routes/pricing/index.tsx` 与 `web/default/src/routes/pricing/$modelId/index.tsx` 的 validate/search 恢复均调用该 helper。
- pricing 路由 search schema 与深链恢复逻辑必须按角色处理：普通用户忽略并清理 `sort=price-*`、`quotaType`、`tokenUnit`、`rechargePrice` 等成本参数；管理员保留。
- 普通用户 model directory 不读取价格字段，不读取或传递根级 `group_ratio` 成本语义；`usable_group` 只可用于“模型是否可用 / 分组名筛选”，不得携带倍率或用于生成 `1x` / ratio 后缀。
- 管理员成本分析路径可读取成本字段与 `group_ratio`。

- [ ] **步骤 4：改造 pricing 页面文案与列**

- `Pricing` -> `Model Directory`。
- 普通用户隐藏价格显示模式、Price sort keys、Pricing Type、price/cached price 列、卡片价格摘要、Base Price、Pricing by Group、DynamicPricingBreakdown 和 group ratio suffix。
- 管理员保留上述成本分析入口。
- 公开响应缺少根级成本字段时普通用户视图必须不崩溃、不显示 ratio/`1x`/Pricing by Group。

- [ ] **步骤 5：更新 ratio sync 前端默认 endpoint**

`web/default/src/features/system-settings/models/constants.ts`：

```ts
export const DEFAULT_ENDPOINT = '/api/ratio_config'
export const ENDPOINT_OPTIONS = [
  { label: 'ratio_config', value: '/api/ratio_config' },
  { label: 'OpenRouter', value: OPENROUTER_ENDPOINT },
  { label: 'custom', value: 'custom' },
]
```

`ENDPOINT_OPTIONS` 不包含 `/api/pricing` 默认项；管理员手动输入 legacy 成本源时使用 `custom`。


- [ ] **步骤 6：运行任务 9 测试验证通过**

运行：

```bash
cd web/default
bun test src/features/pricing/model-directory.test.ts
```

预期：通过。

---

## 任务 10：首页、AI context、入口文案与 i18n

**文件：**
- 修改：`web/default/src/components/ai-elements/context.tsx`
- 修改：`web/default/src/features/home/components/hero-terminal-demo.tsx`
- 修改：`web/default/src/features/home/components/sections/hero.tsx`
- 修改：`web/default/src/features/home/components/sections/cta.tsx`
- 修改：`web/default/src/features/dashboard/components/overview/overview-dashboard.tsx`
- 修改：`web/default/src/i18n/locales/{en,zh,fr,ja,ru,vi}.json`
- 修改：`web/default/src/i18n/static-keys.ts`
- 测试：`web/default/src/features/home/quick-start-copy.test.ts`
- 测试：`web/default/src/features/pricing/model-directory.test.ts`

- [ ] **步骤 1：编写失败的文案测试**

更新 `quick-start-copy.test.ts` 或新增测试；若在现有文件内追加测试，保留现有 `describe` 与 import，只补齐缺失 import，避免重复导入：

```ts
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'

function readSource(relativePath: string): string {
  return readFileSync(new URL(relativePath, import.meta.url), 'utf8')
}

test('home and dashboard model directory links avoid pricing wording', () => {
  const heroSource = readSource('./components/sections/hero.tsx')
  const ctaSource = readSource('./components/sections/cta.tsx')
  const dashboardSource = readSource('../dashboard/components/overview/overview-dashboard.tsx')
  assert.doesNotMatch(heroSource + ctaSource + dashboardSource, /View Pricing|Review model rates/)
  assert.match(heroSource + ctaSource + dashboardSource, /Model Directory|Browse Models|Browse available models/)
})

test('home terminal demo does not simulate cost', () => {
  const demoSource = readSource('./components/hero-terminal-demo.tsx')
  assert.doesNotMatch(demoSource, /cost\s*\$|0\.00003|demo\.tokens\s*\*|Total cost/i)
  assert.match(demoSource, /tokens|latency|plan/i)
})

test('ai context default usage does not include usd cost', () => {
  const contextSource = readSource('../../components/ai-elements/context.tsx')
  assert.doesNotMatch(contextSource, /getUsage|costUSD|Total cost|inputCost|outputCost|cache(?:Read|Creation)?Cost/i)
  assert.match(contextSource, /input|output|reasoning|cached|total/i)
})
```

- [ ] **步骤 2：运行文案测试验证失败**

运行：

```bash
cd web/default
bun test src/features/home/quick-start-copy.test.ts
```

预期：失败，当前仍有 `View Pricing` / `Pricing` / `Review model rates`。

- [ ] **步骤 3：删除 home demo 和 AI context 默认 cost**

- `hero-terminal-demo.tsx` 删除 `cost $...`、`0.00003` 模拟单价，改为 token / latency / plan。
- `context.tsx` 默认只显示 input、output、reasoning、cached、total tokens；删除 `getUsage` 默认调用、`costUSD`、`Total cost` 和各项 USD cost；删除或重命名 `TokensWithCost`，避免默认 children 以外路径继续附带 USD。

- [ ] **步骤 4：改造入口文案**

- 首页 hero / CTA：`View Pricing` -> `Browse Models` 或 `Model Directory`。
- Dashboard overview quick action：`Pricing` -> `Model Directory`，`Review model rates before scaling traffic` -> `Browse available models and capabilities`。

- [ ] **步骤 5：补齐 i18n**

按 `i18n-translate` 工作流：

```bash
cd web/default
bun run i18n:sync
```

读取 `_sync-report.json`，补齐 `en`、`zh`、`fr`、`ja`、`ru`、`vi`。至少包含规格列出的 key：

- `Subscription tokens remaining`
- `Tokens used this cycle`
- `Tokens used in the last 24 hours`
- `Monitor subscription tokens and request volume`
- `Low token balance`
- `Tokens depleted`
- `Subscription required`
- `Total Tokens Used`
- `Tokens used in selected range`
- `Token Usage Distribution`
- `Deducted Tokens`
- `Model Directory`
- `Browse available models and capabilities`


在 `web/default/src/features/pricing/model-directory.test.ts` 内新增 i18n 断言测试：枚举本计划新增 key，断言 `web/default/src/i18n/static-keys.ts`（动态 key）和 `en/zh/fr/ja/ru/vi.json` 均包含对应 key，且非英文 locale 不保留英文占位。不得另建未纳入命令的脚本。
- [ ] **步骤 6：运行任务 10 验证通过**

运行：

```bash
cd web/default
bun test src/features/home/quick-start-copy.test.ts src/features/pricing/model-directory.test.ts
bun run i18n:sync
```

预期：通过；i18n sync report 无新增 missing key。

---

## 残留文案与普通用户视图核对

在最终验证前由主会话执行专用搜索，不交给实现子代理运行：

```text
使用 search 工具扫描以下路径：
web/default/src/features/dashboard
web/default/src/features/usage-logs
web/default/src/features/pricing
web/default/src/features/home
web/default/src/components/ai-elements/context.tsx

搜索模式：Cost|Total Cost|Model Price|Dynamic Pricing|\$/M|Credit remaining|Balance depleted|Total Quota|Statistical quota|ratio|quota|Pricing|Price
```

允许保留白名单：管理员成本审计路径、支付 / 充值 / 套餐购买售价、账户余额、渠道余额、系统设置、管理员模型成本配置、测试中明确断言“不应出现”的字符串。

人工核对：普通用户 dashboard、usage logs、模型目录、首页 demo 与 AI context 不出现价格制表达；管理员仍能看到成本分析、套餐售价、账户余额、渠道余额和系统倍率配置。

---

## 最终验证

所有任务完成后由主会话统一运行：

```bash
go test ./model ./controller ./service -run 'Test.*(LogStat|Subscription.*Other|SubscriptionSelf|SelfSummary|Pricing|RatioSync|QuotaData|TotalTokens|SumUsedQuotaUsesMeteredTokens|SumUsedQuotaPreservesAuthoritativeZeroMeteredTokens|AppendBillingInfo(WritesSubscriptionTokenFields|ClampsNegativeSubscriptionTokenConsumption|DoesNotWriteTokenFieldsForLegacyAmountSubscription)|RecordConsumeLog(TreatsZeroSubscriptionConsumedAsAuthoritative|FallsBackWhenSubscriptionConsumedMissing|UsesSubscriptionConsumedForMeteredTokensAndQuotaData))' -count=1
```

```bash
cd web/default
bun test src/features/dashboard/lib/charts.test.ts src/features/dashboard/lib/subscription-summary.test.ts src/features/usage-logs/lib/format.test.ts src/features/pricing/model-directory.test.ts src/features/home/quick-start-copy.test.ts
bun run i18n:sync
bun run typecheck
```

验收必须逐项核对：

- 普通用户 dashboard 不再以旧 quota / 钱包余额展示 API 可用额度。
- 普通用户 usage logs 不再以 Cost / Total Cost / ratio 展示请求消耗。
- 模型统计图和管理员用户排行按 `token_used` 展示和排序。
- `/api/log/stat` 返回规范化 `total_tokens`，包含订阅日志的 `subscription_tokens_consumed`。
- `/api/subscription/self` 返回 `summary`，并保留 `billing_preference`、`subscriptions`、`all_subscriptions`。
- `/api/pricing` 对普通用户和未登录访问不返回价格、倍率和 billing expression。
- 支付、套餐售价、账户余额、渠道余额和管理员成本分析未被误删。
- 所有新增或修改前端文案补齐 6 种 locale。
