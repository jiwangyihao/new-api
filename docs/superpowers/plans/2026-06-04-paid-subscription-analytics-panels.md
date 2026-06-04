# 付费套餐剩余价值与邀请付费统计面板实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法跟踪进度。实现直接在主工作区开发，不创建、不切换 worktree。

**目标：** 在现有 `/admin-analytics` 中新增「付费套餐剩余价值」和「邀请付费」两个后台统计面板，并新增可配置的统计排除用户列表。

**架构：** 后端沿用现有 `AdminAnalyticsPanelResponse[T]`、Gin controller 和 GORM model 聚合模式；新增专用 query parser 与专用 range normalizer，避免现有默认最近 30 天范围影响新面板。金额以 `UserSubscription + SubscriptionPlan.price_amount/currency` 为主口径，订单只做辅助追溯。前端沿用 `features/admin-analytics` 的 tab、React Query、URL search、drilldown 和 i18n 模式，新增两个 tab 和系统设置 Billing/Statistics section。

**技术栈：** Go 1.22+、Gin、GORM v2、SQLite/MySQL/PostgreSQL；React 19、TypeScript、TanStack Router、React Query、Base UI、Tailwind、Bun、i18next。

---

## 依据与边界

- 规格文件：`C:/Users/34404/source/repos/new-api/docs/superpowers/specs/2026-06-04-paid-subscription-analytics-design.md`
- 计划文件：`C:/Users/34404/source/repos/new-api/docs/superpowers/plans/2026-06-04-paid-subscription-analytics-panels.md`
- 项目规则：`C:/Users/34404/source/repos/new-api/AGENTS.md`
- 前端规则：`C:/Users/34404/source/repos/new-api/web/default/AGENTS.md`
- 当前实现直接在主分支主工作区 `C:/Users/34404/source/repos/new-api` 开发；禁止创建、切换或依赖 worktree。
- 子代理不得运行项目级 build / lint / format；主代理最终统一运行验证。子代理可运行自己新增或修改的定向测试。
- 不修改支付、退款、发票、对账、兑换码发放、管理员分配、邀请奖励发放流程。
- 不回填历史获得流水。本阶段基于现有 `UserSubscription` 权益快照推导确认单元。
- `SubscriptionOrder.money` 不作为主金额，不命名为「实收金额」。订单字段只返回 `order_recorded_amount` 等辅助追溯字段。`order_recorded_amount.currency` 使用关联 `SubscriptionPlan.Currency` 作为展示币种，并在说明中标记为订单辅助展示币种，不代表订单真实收款币种。
- 核心业务口径固定：`SubscriptionPlan.price_amount > 0` 且来源不是 `monthly_invite_entitlement`、`invite_trial`、`trial_code`。这三个来源不计入主统计、付费用户、剩余价值，也不进入 `excluded_*` / `would_have_*` 审计金额。
- 多币种不得混加。所有汇总金额返回 `*_by_currency`，金额类排序必须携带 `currency`，否则 HTTP 400。

## 文件职责

### 后端新增文件

- `setting/subscription_analytics.go`：注册 `subscription_analytics.excluded_users` 配置，只提供统计侧只读 accessor，返回 `map[int]SubscriptionAnalyticsExcludedUser` 拷贝。
- `setting/subscription_analytics_test.go`：配置 accessor 拷贝语义测试。
- `model/admin_analytics_paid_subscription.go`：两个新面板的核心聚合、金额拆分、排除用户判断、专用 range normalization、剩余价值算法、邀请确认单元推导、订单辅助关联、列表排序分页。
- `model/admin_analytics_paid_subscription_test.go`：剩余价值、排除过滤、非销售赠送来源、邀请金额、确认单元、多币种排序、range normalization 的 model 测试。

### 后端修改文件

- `dto/admin_analytics.go`：新增 `MoneyAmount` / `MoneyBreakdown`，新增两个面板的 summary/list/breakdown DTO，扩展 drilldown target 的 `subscription_id`、`invitee_id` 字段。
- `model/admin_analytics.go`：只允许扩展 `AdminAnalyticsQuery` 字段和真正共享 helper；新增面板聚合逻辑不得写回这个旧文件。新增字段包括 `SubscriptionID`、`InviteeID`、`Currency`、`ExcludedMode`、`ActiveOnly`、`TimeRangeExplicit`、`RangeMode`。
- `controller/admin_analytics.go`：新增专用 parser、sort 白名单、controller handler。现有 handler 继续使用现有默认 30 天 parser。
- `router/api-router.go`：注册 9 个新增 endpoint。
- `controller/admin_analytics_test.go`：新增 parser、snapshot、all-history、currency sort、endpoint 参数测试。

### 前端新增文件

- `web/default/src/features/system-settings/billing/statistics-section.tsx`：Billing / Statistics 设置表单，维护 `subscription_analytics.excluded_users`，遵循现有系统设置表单模式和版权头。

### 前端修改文件

- `web/default/src/features/admin-analytics/types.ts`：新增 tab、query 字段、Money DTO、两个面板 DTO、drilldown 字段。
- `web/default/src/features/admin-analytics/constants.ts`：新增两个 tab，必须使用 labelKey，不得通过 hyphen tab id 拼 key。
- `web/default/src/features/admin-analytics/api.ts`：新增新面板 endpoint 请求函数，支持 summary 先协商 snapshot，再加载其他 endpoint。
- `web/default/src/features/admin-analytics/lib/filters.ts`：新增 `snapshot_at`、`currency`、`excluded_mode`、`active_only`、`time_range_explicit`、`inviter_id`、`invitee_id`、`subscription_id`，并支持新面板初始请求不发送 `start_timestamp/end_timestamp`。
- `web/default/src/features/admin-analytics/lib/page-contract.ts`：新增两个 tab 的多 endpoint descriptors，并明确各 descriptor 是否发送时间范围、是否发送 `subscription_id`。
- `web/default/src/features/admin-analytics/lib/drilldown.ts`：新增 `paid_subscription_value_*` 与 `invitation_paid_*` drilldown 映射。
- `web/default/src/features/admin-analytics/lib/format.ts`：新增 `MoneyAmount` / `MoneyBreakdown[]` 格式化 helper。
- `web/default/src/features/admin-analytics/index.tsx`：新增两个 tab 的展示面板、统计卡、列表、filter 控件、snapshot 协商、`fetchDescriptor` switch、drilldown 白名单。
- `web/default/src/features/admin-analytics/lib/*.test.ts`：补充 filter、page-contract、drilldown、format 测试。
- `web/default/src/features/system-settings/types.ts`：新增 `SubscriptionAnalyticsExcludedUser` 类型和 `subscription_analytics.excluded_users` 设置字段，字段类型为数组。
- `web/default/src/features/system-settings/billing/index.tsx`：默认设置加入 `subscription_analytics.excluded_users: []`。
- `web/default/src/features/system-settings/billing/section-registry.tsx`：新增 `statistics` section，路径 `/system-settings/billing/statistics`。
- `web/default/src/i18n/static-keys.ts` 与 `web/default/src/i18n/locales/{en,zh,fr,ru,ja,vi}.json`：新增所有 UI 文案。

## 并发与串行规则

- 任务 1（后端配置、DTO、query/parser 合同）必须先完成，后端 model 和前端类型依赖字段命名。
- 任务 2（后端 model 算法与聚合）依赖任务 1；只改 `model/admin_analytics_paid_subscription.go`、`model/admin_analytics.go`、`model/admin_analytics_paid_subscription_test.go`，不改 controller/router/frontend。
- 任务 3（后端 controller/router）依赖任务 1 和任务 2；只改 `controller/admin_analytics.go`、`controller/admin_analytics_test.go`、`router/api-router.go`。
- 任务 4（前端 contracts/lib/API）依赖任务 1 的 DTO 命名，可与任务 2 并行；不改 page JSX、system settings、locale JSON。
- 任务 5（前端页面与系统设置/i18n）依赖任务 4；拥有 `index.tsx`、Billing statistics section、i18n 文件。
- 后端和前端实现可以并发，但不得让两个子代理同时修改同一文件。若必须改同一文件，后一个子代理应读取最新版本后做窄范围编辑。
- 主代理最终统一运行后端定向测试、前端定向测试、i18n sync、typecheck。

---

## 任务 1：后端配置、DTO 与查询合同

**文件：**
- 创建：`setting/subscription_analytics.go`
- 创建：`setting/subscription_analytics_test.go`
- 修改：`dto/admin_analytics.go`
- 修改：`model/admin_analytics.go`
- 修改：`controller/admin_analytics.go`
- 测试：`controller/admin_analytics_test.go`
- 不修改：`router/api-router.go`、`model/admin_analytics_paid_subscription.go`、前端文件

- [ ] **步骤 1：编写配置 accessor 红灯测试**

在 `setting/subscription_analytics_test.go` 新增测试，验证 `GetSubscriptionAnalyticsExcludedUsers()` 返回 map 拷贝，而不是可变全局对象。测试可以通过受控测试 helper 设置 `subscriptionAnalyticsSetting.ExcludedUsers`，但业务代码不得暴露可变全局 getter。

```go
func TestSubscriptionAnalyticsExcludedUsersReturnsCopy(t *testing.T) {
    old := subscriptionAnalyticsSetting.ExcludedUsers
    t.Cleanup(func() { subscriptionAnalyticsSetting.ExcludedUsers = old })
    subscriptionAnalyticsSetting.ExcludedUsers = []SubscriptionAnalyticsExcludedUser{{UserID: 10, Reason: "original"}}

    excluded := GetSubscriptionAnalyticsExcludedUsers()
    excluded[10] = SubscriptionAnalyticsExcludedUser{UserID: 10, Reason: "mutated"}

    again := GetSubscriptionAnalyticsExcludedUsers()
    require.Equal(t, "original", again[10].Reason)
}
```

同时新增 `TestSubscriptionAnalyticsExcludedUsersLoadsFromGlobalConfig`：调用 `config.GlobalConfig.LoadFromDB(map[string]string{"subscription_analytics.excluded_users":"[{\"user_id\":10,\"reason\":\"ops\",\"excluded_at\":123,\"excluded_by\":7}]"})`，断言 `GetSubscriptionAnalyticsExcludedUsers()` 能读到 `user_id=10`、`reason=ops`、`excluded_at=123`、`excluded_by=7`。测试结束必须恢复 `subscriptionAnalyticsSetting.ExcludedUsers`，避免污染其他测试。

- [ ] **步骤 2：实现配置注册**

新增 `setting/subscription_analytics.go`：

```go
package setting

import "github.com/QuantumNous/new-api/setting/config"

type SubscriptionAnalyticsExcludedUser struct {
    UserID     int    `json:"user_id"`
    Username   string `json:"username,omitempty"`
    Reason     string `json:"reason,omitempty"`
    ExcludedAt int64  `json:"excluded_at,omitempty"`
    ExcludedBy int    `json:"excluded_by,omitempty"`
}

type SubscriptionAnalyticsSetting struct {
    ExcludedUsers []SubscriptionAnalyticsExcludedUser `json:"excluded_users"`
}

var subscriptionAnalyticsSetting = SubscriptionAnalyticsSetting{ExcludedUsers: []SubscriptionAnalyticsExcludedUser{}}

func init() {
    config.GlobalConfig.Register("subscription_analytics", &subscriptionAnalyticsSetting)
}

func GetSubscriptionAnalyticsExcludedUsers() map[int]SubscriptionAnalyticsExcludedUser {
    result := make(map[int]SubscriptionAnalyticsExcludedUser, len(subscriptionAnalyticsSetting.ExcludedUsers))
    for _, item := range subscriptionAnalyticsSetting.ExcludedUsers {
        if item.UserID > 0 { result[item.UserID] = item }
    }
    return result
}
```

不要添加普通业务可调用的 `GetSubscriptionAnalyticsSetting()` 可变指针 getter。`config.GlobalConfig.LoadFromDB` 通过反射更新注册 struct；统计代码只能调用拷贝 accessor。若测试需要改配置，只在同 package 测试里直接设置包内变量。

- [ ] **步骤 3：新增 DTO 类型**

在 `dto/admin_analytics.go` 中新增：

```go
type AdminAnalyticsExcludedMode string

const (
    AdminAnalyticsExcludedModeIncludedOnly    AdminAnalyticsExcludedMode = "included_only"
    AdminAnalyticsExcludedModeIncludeExcluded AdminAnalyticsExcludedMode = "include_excluded"
    AdminAnalyticsExcludedModeExcludedOnly    AdminAnalyticsExcludedMode = "excluded_only"
)

type AdminAnalyticsMoneyAmount struct {
    Amount   float64 `json:"amount"`
    Currency string  `json:"currency"`
}

type AdminAnalyticsMoneyBreakdown struct {
    Amount   float64 `json:"amount"`
    Currency string  `json:"currency"`
}
```

命名可按项目风格调整，但 JSON 字段必须是 `amount` / `currency`，前端类型需一致。

- [ ] **步骤 4：新增两个面板 DTO**

继续在 `dto/admin_analytics.go` 中新增两个面板 DTO。字段必须按规格 7.10、7.11、8.7 到 8.10 完整定义。不得使用 `map[string]any` 作为业务 DTO。

付费剩余价值至少包含：

```go
type AdminPaidSubscriptionValueSummary struct {
    RecognizedRemainingValueByCurrency []AdminAnalyticsMoneyBreakdown `json:"recognized_remaining_value_by_currency"`
    TokenBasedValueByCurrency          []AdminAnalyticsMoneyBreakdown `json:"token_based_value_by_currency"`
    TimeBasedValueByCurrency           []AdminAnalyticsMoneyBreakdown `json:"time_based_value_by_currency"`
    ExcludedRemainingValueByCurrency   []AdminAnalyticsMoneyBreakdown `json:"excluded_remaining_value_by_currency"`
    ActivePaidSubscriptionCount        int                            `json:"active_paid_subscription_count"`
    ActivePaidUserCount                int                            `json:"active_paid_user_count"`
    TokenValueUnavailableCount         int                            `json:"token_value_unavailable_count"`
}
```

邀请付费 summary 必须包含：`recognized_invitation_paid_amount_by_currency`、`active_invitation_paid_amount_by_currency`、`active_invitation_remaining_value_by_currency`、`excluded_invitation_paid_amount_by_currency`、`excluded_active_remaining_value_by_currency`。
邀请付费 summary 还必须包含计数字段：`inviter_count`、`invitee_count`、`paid_invitee_count`、`active_paid_invitee_count`。这些字段对应顶部统计卡，不得从金额字段或列表长度临时推导。

每个多 endpoint response 的 JSON 字段固定为：`summary`、`users`、`subscriptions`、`plans`、`sources`、`inviters`、`invitees`。即使某个 endpoint 只返回其中一个列表，也使用同一 DTO 或专用 DTO，字段名不得在前后端之间另行解释。

- [ ] **步骤 5：扩展 query 合同与 range 模式**

在 `model.AdminAnalyticsQuery` 追加字段，保持现有字段不删除：

```go
type AdminAnalyticsRangeMode string

const (
    AdminAnalyticsRangeModeDefault    AdminAnalyticsRangeMode = ""
    AdminAnalyticsRangeModeSnapshot   AdminAnalyticsRangeMode = "snapshot"
    AdminAnalyticsRangeModeAllHistory AdminAnalyticsRangeMode = "all_history"
)

SubscriptionID int
InviteeID int
Currency string
ExcludedMode dto.AdminAnalyticsExcludedMode
ActiveOnly bool
TimeRangeExplicit bool
RangeMode AdminAnalyticsRangeMode
```

旧 `parseAdminAnalyticsQuery` 和旧 model 函数继续使用 `AdminAnalyticsRangeModeDefault`，现有 `normalizeAdminAnalyticsQuery` 的默认最近 30 天逻辑不能被破坏。两个新面板的 model 入口必须使用 `normalizeAdminPaidSubscriptionAnalyticsQuery` 或等价专用 normalizer：

- `RangeModeSnapshot`：用于面板一，默认 `StartTimestamp=0`、`EndTimestamp=SnapshotAt`，不触发旧 30 天默认。
- `RangeModeAllHistory`：用于面板二，未显式传时间范围时 `StartTimestamp=0`、`EndTimestamp=SnapshotAt`，不触发旧 30 天默认；显式传入 start/end 时 `TimeRangeExplicit=true` 并回显归一化范围。
- `adminAnalyticsRangeMeta(query)` 必须收到已专用 normalize 的 query，才能返回 `start_timestamp=0`。

- [ ] **步骤 6：为专用 parser 写红灯测试**

在 `controller/admin_analytics_test.go` 中新增直接测试 parser / sort helper 的测试，不要求 handler 或 model 已存在：

- `TestPaidSubscriptionValueParserDefaultsToSnapshotRangeWithoutThirtyDayWindow`：构造 Gin context `?snapshot_at=123`，调用 `parseAdminPaidSubscriptionValueQuery`，断言 `RangeMode=AdminAnalyticsRangeModeSnapshot`、`StartTimestamp=0`、`EndTimestamp=123`、`SnapshotAt=123`、`TimeRangeExplicit=false`。
- `TestInvitationPaidParserAllowsAllHistoryWithoutRangeLimit`：不传 start/end，调用 `parseAdminInvitationPaidSubscriptionsQuery`，断言 `RangeMode=AdminAnalyticsRangeModeAllHistory`、`StartTimestamp=0`、`EndTimestamp=SnapshotAt`、`TimeRangeExplicit=false`。
- `TestInvitationPaidParserParsesExplicitRange`：传 start/end，断言 `TimeRangeExplicit=true` 且 start/end 回显。
- `TestPaidSubscriptionMoneySortRequiresCurrency`：构造 `sort_by=recognized_remaining_value` 且无 currency，调用 money sort helper，断言返回 false 或 bad request。
- `TestPaidSubscriptionNonMoneySortDoesNotRequireCurrency`：`sort_by=user_id` 不传 `currency`，断言不因 currency 失败。
- `TestAdminAnalyticsRejectsInvalidSnapshotAt`：`snapshot_at=-1` 或非数字返回错误。
- `TestAdminAnalyticsOverviewParserStillDefaultsToThirtyDays`：旧 `parseAdminAnalyticsQuery` 在无 start/end 时仍产生最近 30 天范围，保护旧 tab 行为。
- `TestAdminAnalyticsAcceptsExplicitZeroSnapshotAt`：显式传 `snapshot_at=0` 时不得当作缺省值替换为当前时间；新面板 parser / range meta 必须回显 `snapshot_at=0`。
- `TestPaidSubscriptionValueParserParsesSharedFilters`：传 `plan_ids`、`user_ids`、`sources`、`grant_reasons`、`business_codes`、`currency`、`excluded_mode`、`active_only`、`subscription_id`、`limit`、`offset`、`sort_order`，断言 query 字段完整透传。
- `TestInvitationPaidParserParsesSharedFilters`：传 `plan_ids`、`user_ids`、`sources`、`grant_reasons`、`business_codes`、`inviter_id`、`invitee_id`、`active_only`、`subscription_id`、`currency`、`excluded_mode`、`limit`、`offset`、`sort_order`，断言 query 字段完整透传。

先运行：

```bash
go test ./controller -run 'Test(PaidSubscriptionValueParser|PaidSubscriptionValueParserParsesSharedFilters|InvitationPaidParser|PaidSubscription(MoneySort|NonMoneySort)|AdminAnalytics(RejectsInvalidSnapshotAt|AcceptsExplicitZeroSnapshotAt|OverviewParserStillDefaultsToThirtyDays))' -count=1
```

预期：红灯失败，因为 helper 尚未实现。

- [ ] **步骤 7：实现专用 parser 与 sort 校验**

在 `controller/admin_analytics.go` 新增 helper：

```go
func parseAdminAnalyticsSnapshotAt(c *gin.Context) (int64, error)
func parseAdminPaidSubscriptionValueQuery(c *gin.Context) (model.AdminAnalyticsQuery, error)
func parseAdminInvitationPaidSubscriptionsQuery(c *gin.Context) (model.AdminAnalyticsQuery, error)
func normalizeAdminAnalyticsMoneySortByOrAbort(c *gin.Context, query model.AdminAnalyticsQuery, allowed map[string]string, moneySorts map[string]struct{}) (model.AdminAnalyticsQuery, bool)
```

规则：

- `snapshot_at` 缺省为 `time.Now().Unix()`，显式传入必须 `>= 0`。
- 面板一默认不读取 start/end；range mode 为 snapshot。
- 面板二未传 start/end 表示 all-history；显式传入 start/end 时校验 start<=end，可复用 365 天限制，且 `TimeRangeExplicit=true`。
- `excluded_mode` 只允许 `included_only|include_excluded|excluded_only`，默认 `included_only`。
- `active_only` 解析为 bool，默认 false。
- `subscription_id`、`inviter_id`、`invitee_id` 必须是正整数或缺省。
- money sort 需要 `currency`，非 money sort 不需要。

- [ ] **步骤 8：运行任务 1 定向测试**

运行：

```bash
go test ./controller -run 'Test(PaidSubscriptionValueParser|PaidSubscriptionValueParserParsesSharedFilters|InvitationPaidParser|PaidSubscription(MoneySort|NonMoneySort)|AdminAnalytics(RejectsInvalidSnapshotAt|AcceptsExplicitZeroSnapshotAt|OverviewParserStillDefaultsToThirtyDays))' -count=1
go test ./setting -run SubscriptionAnalytics -count=1
```

预期：PASS。

---

## 任务 2：后端 model 算法与聚合

**文件：**
- 创建：`model/admin_analytics_paid_subscription.go`
- 创建/修改：`model/admin_analytics_paid_subscription_test.go`
- 可能修改：`model/admin_analytics.go` 中只允许增加通用 helper 或 query 字段使用
- 不修改：controller、router、前端、setting

- [ ] **步骤 1：编写剩余价值红灯测试**

在 `model/admin_analytics_paid_subscription_test.go` 新增测试：

`TestPaidSubscriptionValueCalculatesMinTokenAndTimeValue` 使用固定 UTC 时间戳，避免当前日期影响 monthly reset 边界。构造：

- snapshot 固定为 `2026-01-30T00:00:00Z`（月末前一天）或另一个明确写死的 UTC 时间；所有 start/end 均由该 snapshot 推导，测试不得依赖 `time.Now()`。
- plan：`PriceAmount=40,Currency="CNY",DurationUnit="day",DurationValue=30,MonthlyTokenLimit=1000000000,QuotaResetPeriod="monthly"`
- subscription：`StartTime=snapshot-27天,EndTime=snapshot+33天,TokenLimit=1000000000,TokenUsed=200000000,Status="active",GrantReason="order",Source="order"`
- query：`SnapshotAt=snapshot, EndTimestamp=snapshot, Currency="CNY", RangeMode=AdminAnalyticsRangeModeSnapshot`

断言 `recognized_remaining_value` 为 44（允许浮点误差 0.0001），`time_based_value` 为 44，`token_based_value` 大于 44，summary 的 `recognized_remaining_value_by_currency` 为 CNY/44。

先运行：

```bash
go test ./model -run TestPaidSubscriptionValueCalculatesMinTokenAndTimeValue -count=1
```

预期：失败，因为函数尚未实现。

- [ ] **步骤 2：实现专用 normalizer、Money accumulator 和剩余价值纯函数**

在 `model/admin_analytics_paid_subscription.go` 实现内部 helper：

```go
func normalizeAdminPaidSubscriptionAnalyticsQuery(query AdminAnalyticsQuery) AdminAnalyticsQuery

type adminMoneyAccumulator map[string]float64
func (a adminMoneyAccumulator) add(currency string, amount float64)
func (a adminMoneyAccumulator) amount(currency string) float64
func (a adminMoneyAccumulator) breakdown() []dto.AdminAnalyticsMoneyBreakdown
func adminPlanDurationSeconds(start int64, plan *SubscriptionPlan) (int64, error)
func adminSubscriptionTimeValue(sub UserSubscription, plan SubscriptionPlan, snapshotAt int64) (float64, error)
func adminSubscriptionTokenValue(sub UserSubscription, plan SubscriptionPlan, snapshotAt int64, planDurationSeconds int64) (float64, bool, error)
func adminRecognizedRemainingValue(sub UserSubscription, plan SubscriptionPlan, snapshotAt int64) (adminSubscriptionValue, error)
```

`normalizeAdminPaidSubscriptionAnalyticsQuery` 不得调用会把 `StartTimestamp=0` 改为最近 30 天的旧 `normalizeAdminAnalyticsQuery`，只能复用 limit/offset/sort_order 默认化逻辑。

`adminSubscriptionTokenValue` 规则：

- `sub.TokenLimit <= 0` 返回 unavailable，调用方退化为 time value，并累计 `TokenValueUnavailableCount`。
- 当前周期价值：`cycle_value * max(token_limit-token_used,0)/token_limit`。
- `cycle_value = plan.PriceAmount * cycle_seconds / plan_duration_seconds`。
- 未来完整周期和尾段按周期秒数比例累计。
- `quota_reset_period=never` 的有限 token 套餐没有未来重置周期。
- daily/weekly/custom 每个周期只能按占套餐周期比例计价，不能按完整套餐价。
- month/year 套餐用真实 `AddDate` 周期差，不硬编码 30 天/月或 365 天/年。

- [ ] **步骤 3：编写估值边界红灯测试**

新增测试：

- `TestPaidSubscriptionValueTimeOnlyWhenTokenUnavailable`：`TokenLimit=0` 时 `recognized_remaining_value=time_based_value`，`token_value_unavailable_count=1`。
- `TestPaidSubscriptionValueTokenOveruseCurrentCycleIsZero`：`TokenUsed > TokenLimit` 时当前周期 token 价值为 0，但未来周期仍按重置规则计入。
- `TestPaidSubscriptionValueNeverResetDoesNotAddFutureCycles`：`quota_reset_period=never` 时有限 token 套餐没有未来 token 重置价值。
- `TestPaidSubscriptionValueDailyWeeklyCustomResetProratesCycleValue`：30 天套餐 daily/weekly/custom 重置时未来周期价值按周期秒数比例，不按整包价。
- `TestPaidSubscriptionValueMonthYearUsesCalendarDuration`：month/year 套餐按真实日历周期计算。

- [ ] **步骤 4：编写非销售赠送、排除用户和精确过滤红灯测试**

新增测试：

- `TestPaidSubscriptionValueExcludesGiftTrialAndInviteSourcesFromMainAndAudit`：被排除用户持有 `monthly_invite_entitlement`、`invite_trial`、`trial_code` 的有价套餐时，summary 主金额、`active_paid_user_count` 和 `excluded_remaining_value_by_currency` 都为 0。
- `TestPaidSubscriptionValueIncludesPaidSourcesWithoutOrders`：`order`、`admin`、`redemption` 来源，无 `SubscriptionOrder` 也计入。
- `TestPaidSubscriptionValueExcludedModeAuditsPaidExcludedUsers`：排除用户持有 `admin` 或 `redemption` 有价套餐，`included_only` 主统计为 0，`include_excluded` / `excluded_only` 明细可见，`excluded_remaining_value_by_currency` 有值。
- `TestPaidSubscriptionValueEmptyExcludedListDoesNotFilterRows`：排除列表为空时正常返回，不因空 NOT IN 变空。
- `TestPaidSubscriptionValueSubscriptionsFiltersBySubscriptionID`：subscriptions endpoint 的 model 函数收到 `SubscriptionID` 时只返回该订阅；summary/users/breakdown 不要求使用此过滤。
- `TestPaidSubscriptionValueSortsMoneyBySelectedCurrencyOnly`：同一列表有 USD/CNY 多币种金额，`Currency="CNY"` 时只按 CNY 金额排序，无 CNY 的行按 0，不跨币种相加。
- `TestPaidSubscriptionValueSubscriptionsIncludesOrderAuxiliaryAmountWithPlanCurrency`：同一 `user_id + plan_id` 创建订阅和成功订单，订单 `Money` 与 plan price 故意不同；断言 subscriptions 明细返回 `possible_order_id`、`payment_provider`、`payment_method`、`order_recorded_amount.amount=SubscriptionOrder.Money`、`order_recorded_amount.currency=SubscriptionPlan.Currency`，同时主金额仍来自 plan price 而不是 order money。

- [ ] **步骤 5：实现过滤、排除模式和面板一聚合**

实现：

```go
func GetAdminPaidSubscriptionValueSummary(query AdminAnalyticsQuery) (dto.AdminAnalyticsPanelResponse[dto.AdminPaidSubscriptionValueResponse], error)
func GetAdminPaidSubscriptionValueUsers(query AdminAnalyticsQuery) (...)
func GetAdminPaidSubscriptionValueSubscriptions(query AdminAnalyticsQuery) (...)
func GetAdminPaidSubscriptionValuePlanBreakdown(query AdminAnalyticsQuery) (...)
func GetAdminPaidSubscriptionValueSourceBreakdown(query AdminAnalyticsQuery) (...)
```

允许内部共享一个 `buildAdminPaidSubscriptionValueData(query)`，按 endpoint 裁剪返回字段。实现必须：

- 所有入口先调用 `normalizeAdminPaidSubscriptionAnalyticsQuery`，保持新面板 `range.start_timestamp=0`。
- 加载 active subscriptions：`status='active' AND start_time<=snapshot AND end_time>snapshot`。
- 只计 `PriceAmount > 0`。
- 只用正向 gift set 判断排除 `monthly_invite_entitlement`、`invite_trial`、`trial_code`；空字符串和 NULL 不等于 gift。
- 加载 `setting.GetSubscriptionAnalyticsExcludedUsers()` 为 map；空 map 不追加 `NOT IN` SQL。
- 支持 query 过滤落点：`PlanIDs`、`UserIDs`、`Sources`、`GrantReasons`、`BusinessCodes`；`Currency` 不裁剪行集，只用于金额排序和前端展示偏好；`SubscriptionID` 只过滤 subscriptions endpoint。
- `included_only` 只返回未排除用户主数据。
- `include_excluded` 主 summary 仍只累计未排除用户，同时返回排除用户行和 `excluded_*`。
- `excluded_only` 主统计为 0 或只返回 `excluded_*`，列表只含排除用户审计行。
- 金额汇总按币种拆分。
- `SubscriptionOrder` 不参与是否计入。
- 不得复用现有 `loadAdminActiveSubscriptions`。该 helper 会调用旧 `normalizeAdminAnalyticsQuery`，可能把 `StartTimestamp=0` 改写为最近 30 天；如需复用过滤逻辑，只能抽取不改写 range 的低层 helper，且入口 query 必须保持 `normalizeAdminPaidSubscriptionAnalyticsQuery` 的结果。

- [ ] **步骤 6：编写邀请统计红灯测试**

新增测试：

- `TestInvitationPaidSubscriptionsCountsAllHistoryByDefault`：未传时间范围且 `RangeMode=AllHistory` 时，过期和当前有效的非销售赠送以外有价套餐都计入历史邀请金额，range start 为 0。
- `TestInvitationPaidSubscriptionsFiltersConfirmationUnitTimeRange`：显式 start/end 只过滤确认单元获得时间。
- `TestInvitationPaidSubscriptionsInfersRepeatedUnitsFromExtendedSnapshot`：同一下级同一 plan 的 `UserSubscription` 时间跨度为 2.5 个套餐周期时，`recognized_paid_units=2.5`，金额为 `plan_price*2.5`，不是只计 1 行。
- `TestInvitationPaidSubscriptionsExcludesGiftSourcesFromMainAndAudit`：邀请奖励、邀请试用、试用码不计入主统计、`paid_invitee_count`、`active_paid_invitee_count`，也不进入排除审计金额。
- `TestInvitationPaidSubscriptionsActiveAmountAndRemainingValue`：`active_invitation_paid_amount_by_currency` 使用当前有效付费快照 plan price，`active_invitation_remaining_value_by_currency` 复用面板一剩余价值。
- `TestInvitationPaidSubscriptionsActiveOnlyDoesNotChangeHistorySummary`：`active_only=true` 时历史 summary 总额仍包含历史确认单元，但 subscriptions/invitee 明细按当前有效语义过滤。
- `TestInvitationPaidSubscriptionsSubscriptionsFiltersBySubscriptionID`：面板二 subscriptions endpoint model 函数收到 `SubscriptionID` 时只返回该权益记录。
- `TestInvitationPaidSubscriptionsSortsMoneyBySelectedCurrencyOnly`：金额排序只取 `query.Currency` 对应币种金额，无对应币种按 0。
- `TestInvitationPaidSubscriptionsExcludedModeAuditsPaidExcludedInvitees`：配置 `subscription_analytics.excluded_users` 排除一个下级用户，该下级持有 `order`、`admin` 或 `redemption` 非销售赠送以外有价套餐；断言 `included_only` 主统计不计入且默认明细不返回排除行，`include_excluded` 返回排除行并填充 `excluded_invitation_paid_amount_by_currency` / `excluded_active_remaining_value_by_currency`，`excluded_only` 只返回排除行且主统计为 0 或只返回 excluded 字段。

- [ ] **步骤 7：实现邀请确认单元和面板二聚合**

实现：

```go
func GetAdminInvitationPaidSubscriptionsSummary(query AdminAnalyticsQuery) (...)
func GetAdminInvitationPaidSubscriptionsInviters(query AdminAnalyticsQuery) (...)
func GetAdminInvitationPaidSubscriptionsInvitees(query AdminAnalyticsQuery) (...)
func GetAdminInvitationPaidSubscriptionsSubscriptions(query AdminAnalyticsQuery) (...)
```

内部确认单元规则：

- 所有入口先调用 `normalizeAdminPaidSubscriptionAnalyticsQuery`，未显式时间范围时不得被旧 30 天默认截断。
- 没有历史流水时，gift source 整条快照排除并返回 `source_attribution=mixed_or_unknown` 或 warning。
- 付费来源快照按 `calcPlanEndTime(start, plan)` 周期推进到 `end_time`：完整周期 1，尾段按比例，异常不可推导才 `snapshot_minimum=1`。
- `recognized_paid_amount = plan.PriceAmount * recognized_paid_units`。
- 时间范围只过滤确认单元获得时间；未显式传时间范围则 all-history。
- 邀请关系用 `invitee.inviter_id = inviter.id`。
- 支持 query 过滤落点：`PlanIDs`、`UserIDs`、`Sources`、`GrantReasons`、`BusinessCodes`、`InviterID`、`InviteeID`；`Currency` 不裁剪行集，只用于金额排序和前端展示偏好；`SubscriptionID` 只过滤 subscriptions endpoint；summary/inviters/invitees 可忽略。
- `active_only` 只影响明细或当前有效字段，不改变历史 summary 总额。
- 所有邀请入口都必须加载 `setting.GetSubscriptionAnalyticsExcludedUsers()`，空 map 不追加 `NOT IN` SQL；`included_only`、`include_excluded`、`excluded_only` 语义与面板一一致。排除用户持有 `monthly_invite_entitlement`、`invite_trial`、`trial_code` 时仍不进入 excluded/would_have 审计金额。
- 邀请面板必须使用独立 all-history subscription loader，不能复用 active-only helper；只有 `active_only` 明细、当前有效金额和当前剩余价值字段才按当前有效语义过滤。

- [ ] **步骤 8：实现订单辅助关联**

在 subscriptions 记录中可 best-effort 返回：`possible_order_id`、`payment_provider`、`payment_method`、`order_recorded_amount`。候选订单限定同 `user_id + plan_id`，按 `complete_time DESC, id DESC` 稳定选择。无法可信匹配返回空值。`order_recorded_amount.amount = SubscriptionOrder.Money`，`order_recorded_amount.currency = SubscriptionPlan.Currency`，并作为辅助展示币种；不得用订单金额覆盖主金额。

- [ ] **步骤 9：实现排序和分页**

每个 endpoint 按规格白名单排序；金额排序调用 `query.Currency` 取对应币种金额，无对应币种按 0。未知 sort 由 controller 拒绝。分页复用 `paginateAdminAnalyticsList`。

- [ ] **步骤 10：运行任务 2 定向测试**

运行：

```bash
go test ./model -run 'Test(PaidSubscriptionValue|InvitationPaidSubscriptions)' -count=1
```

预期：PASS。

---

## 任务 3：后端 controller 与 router 集成

**文件：**
- 修改：`controller/admin_analytics.go`
- 修改：`controller/admin_analytics_test.go`
- 修改：`router/api-router.go`
- 只读：`model/admin_analytics_paid_subscription.go`、`dto/admin_analytics.go`

- [ ] **步骤 1：编写 endpoint 红灯测试**

在 `controller/admin_analytics_test.go` 新增测试：

- `TestPaidSubscriptionValueSummaryEndpointReturnsPanelEnvelope`
- `TestInvitationPaidSubscriptionsSummaryEndpointReturnsPanelEnvelope`
- `TestPaidSubscriptionValueSubscriptionsFiltersBySubscriptionIDEndpoint`：种两条订阅，请求 subscriptions endpoint 携带 `subscription_id`，只返回目标订阅。
- `TestInvitationPaidSubscriptionsSubscriptionsFiltersBySubscriptionIDEndpoint`：种两条权益记录，请求 subscriptions endpoint 携带 `subscription_id`，只返回目标记录。
- `TestInvitationPaidSubscriptionsDefaultRangeIsAllHistorySnapshot`：不传 start/end，返回 `range.start_timestamp=0`、`range.end_timestamp=range.snapshot_at`，不返回 365 天错误。

使用 `performAdminAnalyticsRequest` 直接调用新 handler。运行：

```bash
go test ./controller -run 'Test(PaidSubscriptionValue|InvitationPaidSubscriptions)' -count=1
```

预期：红灯失败。

- [ ] **步骤 2：新增 sort 白名单**

在 `controller/admin_analytics.go` 增加白名单 map：

```go
adminPaidSubscriptionValueUserSortBy
adminPaidSubscriptionValueSubscriptionSortBy
adminPaidSubscriptionValuePlanSortBy
adminPaidSubscriptionValueSourceSortBy
adminInvitationPaidInviterSortBy
adminInvitationPaidInviteeSortBy
adminInvitationPaidSubscriptionSortBy
```

金额字段另建 money sort set：`recognized_remaining_value`、`plan_price`、`recognized_invitation_paid_amount`、`active_invitation_paid_amount`、`active_invitation_remaining_value`、`recognized_paid_amount`、`active_remaining_value`。

- [ ] **步骤 3：新增 controller handler**

实现 9 个 handler：

```go
GetAdminPaidSubscriptionValueSummary
GetAdminPaidSubscriptionValueUsers
GetAdminPaidSubscriptionValueSubscriptions
GetAdminPaidSubscriptionValuePlanBreakdown
GetAdminPaidSubscriptionValueSourceBreakdown
GetAdminInvitationPaidSubscriptionsSummary
GetAdminInvitationPaidSubscriptionsInviters
GetAdminInvitationPaidSubscriptionsInvitees
GetAdminInvitationPaidSubscriptionsSubscriptions
```

每个 handler 使用对应专用 parser 和 sort 校验，然后调用 model，最后 `writeAdminAnalyticsResponse`。

- [ ] **步骤 4：注册路由**

在 `router/api-router.go` 的 `/api/admin-analytics` AdminAuth group 增加：

```go
adminAnalyticsRoute.GET("/paid-subscription-value/summary", controller.GetAdminPaidSubscriptionValueSummary)
adminAnalyticsRoute.GET("/paid-subscription-value/users", controller.GetAdminPaidSubscriptionValueUsers)
adminAnalyticsRoute.GET("/paid-subscription-value/subscriptions", controller.GetAdminPaidSubscriptionValueSubscriptions)
adminAnalyticsRoute.GET("/paid-subscription-value/breakdown/plans", controller.GetAdminPaidSubscriptionValuePlanBreakdown)
adminAnalyticsRoute.GET("/paid-subscription-value/breakdown/sources", controller.GetAdminPaidSubscriptionValueSourceBreakdown)
adminAnalyticsRoute.GET("/invitation-paid-subscriptions/summary", controller.GetAdminInvitationPaidSubscriptionsSummary)
adminAnalyticsRoute.GET("/invitation-paid-subscriptions/inviters", controller.GetAdminInvitationPaidSubscriptionsInviters)
adminAnalyticsRoute.GET("/invitation-paid-subscriptions/invitees", controller.GetAdminInvitationPaidSubscriptionsInvitees)
adminAnalyticsRoute.GET("/invitation-paid-subscriptions/subscriptions", controller.GetAdminInvitationPaidSubscriptionsSubscriptions)
```

- [ ] **步骤 5：运行 controller 定向测试**

运行：

```bash
go test ./controller -run 'Test(PaidSubscriptionValue|InvitationPaidSubscriptions|AdminAnalyticsRejectsInvalidSnapshotAt)' -count=1
```

预期：PASS。

---

## 任务 4：前端类型、API、过滤、page contract 与 drilldown

**文件：**
- 修改：`web/default/src/features/admin-analytics/types.ts`
- 修改：`web/default/src/features/admin-analytics/constants.ts`
- 修改：`web/default/src/features/admin-analytics/api.ts`
- 修改：`web/default/src/features/admin-analytics/lib/filters.ts`
- 修改：`web/default/src/features/admin-analytics/lib/page-contract.ts`
- 修改：`web/default/src/features/admin-analytics/lib/drilldown.ts`
- 修改：`web/default/src/features/admin-analytics/lib/format.ts`
- 测试：`web/default/src/features/admin-analytics/lib/*.test.ts`
- 不修改：`index.tsx`、system settings、locale JSON

- [ ] **步骤 1：编写 filter/page-contract/drilldown/format 红灯测试**

新增或修改测试：

- `filters.test.ts`：`tab=paid-subscription-value` 和 `tab=invitation-paid-subscriptions` 被接受；`excluded_mode` 只接受三态 enum；`snapshot_at/currency/active_only/time_range_explicit/inviter_id/invitee_id/subscription_id` 被 canonical 保留；新面板初始 canonical `time_range_explicit=false`，API params 不发送最近 30 天范围；旧 tab 仍发送默认时间范围。
- `page-contract.test.ts`：新 tab 返回多个 descriptors，summary descriptor 排第一；URL 没有 `snapshot_at` 时后续 endpoint disabled；已有 snapshot 时 subscriptions/users/breakdown 启用；两个 subscriptions descriptor 标记会发送 `subscription_id`。
- `drilldown.test.ts`：`paid_subscription_value_subscription` 把 `subscription_id` 写入 `/admin-analytics?tab=paid-subscription-value`；`invitation_paid_invitee` 保留 filters 并写入 inviter/invitee。
- `format.test.ts`：`MoneyBreakdown[]` 空数组显示 `—`，0 金额显示币种和 0，不混币种。

运行：

```bash
cd web/default
bun test src/features/admin-analytics/lib/filters.test.ts src/features/admin-analytics/lib/page-contract.test.ts src/features/admin-analytics/lib/drilldown.test.ts src/features/admin-analytics/lib/format.test.ts
```

预期：红灯失败。

- [ ] **步骤 2：扩展类型**

在 `types.ts` 增加：

- tab union：`'paid-subscription-value' | 'invitation-paid-subscriptions'`
- search 字段：`snapshot_at?: number`、`currency?: string`、`excluded_mode: 'included_only' | 'include_excluded' | 'excluded_only'`、`active_only: boolean`、`time_range_explicit: boolean`、`inviter_id?: number`、`invitee_id?: number`、`subscription_id?: number`。
- `start_timestamp` / `end_timestamp` 继续保留 number，用于旧 tab 默认范围和新 tab 显式时间范围。新 tab 是否发送时间由 `time_range_explicit` 与 descriptor option 决定。
- `MoneyAmount`、`MoneyBreakdown`。
- 两个面板所有 DTO，字段名与后端 JSON 一致。
- `AdminAnalyticsDrilldownTarget` 增加 `subscription_id?: number`、`invitee_id?: number`。

- [ ] **步骤 3：扩展 constants 和 filters**

`ADMIN_ANALYTICS_TABS` 新增：

```ts
{ id: 'paid-subscription-value', labelKey: 'adminAnalytics.tabs.paidSubscriptionValue' }
{ id: 'invitation-paid-subscriptions', labelKey: 'adminAnalytics.tabs.invitationPaidSubscriptions' }
```

插入位置：`paid-subscription-value` 在 `quota` 后，`invitation-paid-subscriptions` 在现有 `invitations` 后。

`filters.ts` 固定采用以下方案：

- 旧 tab canonical 继续默认 `start_timestamp/end_timestamp` 最近 30 天，`time_range_explicit=true` 或对旧 tab 忽略该字段，API params 继续发送时间范围。
- 新 tab 初始 canonical 保留 `start_timestamp/end_timestamp` 数值供 UI 显示，但 `time_range_explicit=false`，API params 不发送 `start_timestamp/end_timestamp`。
- 用户修改时间范围或开启时间范围筛选时设置 `time_range_explicit=true`。
- `buildAdminAnalyticsApiParams` 增加 options：`includeTimeRange?: boolean`、`includeSubscriptionID?: boolean`、`includeUsage?: boolean`、`includeSort?: boolean`。旧 tab descriptor/API 调用必须显式或默认 `includeTimeRange=true`，继续发送 `start_timestamp/end_timestamp`；新 tab 只有 `includeTimeRange=true` 且 `time_range_explicit=true` 时发送 start/end。
- `subscription_id` 只在 `includeSubscriptionID=true` 的两个 subscriptions endpoint 请求里发送；其他 endpoint 忽略。
- `currency` 始终可发送，用于金额排序币种和展示偏好，不表示裁剪行集。
- `snapshot_at` 在存在时必须发送到所有新面板 endpoint；判断必须使用 `value !== undefined`，不能用 truthy 判断，因为 `snapshot_at=0` 是合法显式值。
- `excluded_mode`、`active_only`、`inviter_id`、`invitee_id` 在存在时必须发送到支持这些筛选的新面板 endpoint；`user_ids`、`plan_ids` 继续沿用 repeated query params。
- `filters.test.ts` 必须断言新面板 URLSearchParams 包含 `snapshot_at`、`excluded_mode`、`active_only`、`currency`、`inviter_id`、`invitee_id`，且只有 subscriptions descriptor 会包含 `subscription_id`。

- [ ] **步骤 4：扩展 API 与 page contract**

`api.ts` 新增函数：

```ts
paidSubscriptionValueSummary
paidSubscriptionValueUsers
paidSubscriptionValueSubscriptions
paidSubscriptionValuePlans
paidSubscriptionValueSources
invitationPaidSummary
invitationPaidInviters
invitationPaidInvitees
invitationPaidSubscriptions
```

`page-contract.ts`：

- 新 tab summary descriptor 必须排第一。
- 无 `snapshot_at` 时，只启用 summary；有 snapshot 后启用同 tab 其他 endpoint。
- descriptor 显式带 `includeTimeRange`、`includeSubscriptionID`、`includeSort` 元数据，供 API params 与 `fetchDescriptor` 使用。
- descriptor 的 options 必须被实际用于请求构造。推荐把 `fetchDescriptor` 改为接收完整 descriptor：`fetchDescriptor(descriptor, filters)`；如果实现选择让各 API 函数内部固定 options，也必须在测试中覆盖 subscriptions descriptor 会实际序列化 `subscription_id`。
- query key 必须包含 canonical filters 中影响结果的字段。

- [ ] **步骤 5：扩展 drilldown 与 money format**

`drilldown.ts` 新增：

- `paid_subscription_value_user` => `tab=paid-subscription-value,user_ids=[user_id]`
- `paid_subscription_value_subscription` => `tab=paid-subscription-value,subscription_id=<id>`，若 target 无 subscription_id 则 fallback `user_ids + plan_ids`
- `invitation_paid_inviter` => `tab=invitation-paid-subscriptions,inviter_id=<id>`
- `invitation_paid_invitee` => `tab=invitation-paid-subscriptions,inviter_id=<id>,invitee_id=<id>`

`format.ts` 新增 `formatAdminMoneyAmount`、`formatAdminMoneyBreakdown`。

- [ ] **步骤 6：运行前端 lib 定向测试**

运行：

```bash
cd web/default
bun test src/features/admin-analytics/lib/filters.test.ts src/features/admin-analytics/lib/page-contract.test.ts src/features/admin-analytics/lib/drilldown.test.ts src/features/admin-analytics/lib/format.test.ts
```

预期：PASS。

---

## 任务 5：前端页面、系统设置与 i18n

**文件：**
- 修改：`web/default/src/features/admin-analytics/index.tsx`
- 创建：`web/default/src/features/system-settings/billing/statistics-section.tsx`
- 修改：`web/default/src/features/system-settings/types.ts`
- 修改：`web/default/src/features/system-settings/billing/index.tsx`
- 测试：`web/default/src/features/admin-analytics/lib/page-contract.test.ts` 或承载 `switchAdminAnalyticsTab` helper 的等价 lib 测试文件
- 修改：`web/default/src/i18n/static-keys.ts`
- 修改：`web/default/src/i18n/locales/en.json`
- 修改：`web/default/src/i18n/locales/zh.json`
- 修改：`web/default/src/i18n/locales/fr.json`
- 修改：`web/default/src/i18n/locales/ru.json`
- 修改：`web/default/src/i18n/locales/ja.json`
- 修改：`web/default/src/i18n/locales/vi.json`

- [ ] **步骤 1：编写页面契约红灯测试**
- 同时新增 `switchAdminAnalyticsTab` 或等价 helper 的定向测试：从旧 tab 默认最近 30 天切到两个新 tab 时 `time_range_explicit=false` 且 API params 不带 start/end；从新 tab 切回旧 tab 时旧 tab 仍发送默认最近 30 天；切换新 tab 时 offset 被清理且 snapshot 使用一致。

如现有项目已有组件测试设施，则新增轻量测试验证新 tab labelKey 来自 `ADMIN_ANALYTICS_TABS`，而不是 `adminAnalytics.tabs.${tab}` 拼接。若组件测试设施不稳定，至少补 `page-contract.test.ts` 或常量测试覆盖两个 tab 顺序和 labelKey。

运行：

```bash
cd web/default
bun test src/features/admin-analytics/lib/page-contract.test.ts
```

预期：红灯或待实现。

- [ ] **步骤 2：新增两个面板 UI 与 descriptor 调度**

在 `index.tsx`：

- `ActivePanel` 获取当前 tab 的 labelKey 必须从 `ADMIN_ANALYTICS_TABS` 查找，不再用 `adminAnalytics.tabs.${props.tab}` 拼接。
- `fetchDescriptor` 必须为 9 个新增 descriptor id 添加 switch case，分别调用任务 4 新增的 API 函数；未知 descriptor 不得 fallback 到 overview 造成错误 cast。
- 新 tab 的 response 数组按 page-contract 顺序显式拆成 summary/users/subscriptions/breakdown 或 summary/inviters/invitees/subscriptions。
- `isSupportedDrilldownTarget` 必须把 `paid_subscription_value_user`、`paid_subscription_value_subscription`、`invitation_paid_inviter`、`invitation_paid_invitee` 纳入可点击白名单，或抽取为与 `lib/drilldown.ts` 共享的白名单。
- 新增 `PaidSubscriptionValuePanel`：展示 summary cards、plan/source breakdown、用户列表、订阅列表。金额使用 `formatAdminMoneyBreakdown`。
- 新增 `InvitationPaidSubscriptionsPanel`：展示 summary cards、邀请人列表、下级列表、权益记录列表。金额使用 `formatAdminMoneyBreakdown`。
- 新面板在 summary 尚未返回 snapshot 时只显示 summary loading；后续 endpoint 响应为空时显示 `No data`。
- 文案避免「实收金额」「真实订单金额」「每次购买记录」。订单辅助金额只用「订单记录金额」「支付订单金额」「可追溯订单金额」。

- [ ] **步骤 3：实现 snapshot 协商与新筛选控件**

当 summary response 返回 `range.snapshot_at` 且 URL/canonical filters 没有 snapshot 时，页面应调用 `onSearchChange` 写入 `snapshot_at`。实现必须避免无限循环：只有 `props.search.snapshot_at === undefined` 且 response snapshot `!== undefined` 时写入。

`AdminAnalyticsTabs` 的 `onChange` 不得直接使用 `{ ...filters, tab }`。必须通过 helper（例如 `switchAdminAnalyticsTab(filters, tab)`）处理跨 tab 状态：

- 切换到旧 tab 时保留旧 tab 默认时间范围发送行为。
- 切换到 `paid-subscription-value` 或 `invitation-paid-subscriptions` 时，如果用户没有显式开启时间范围筛选，必须设置 `time_range_explicit=false`，避免把旧 tab 默认最近 30 天误传给新面板。
- 切换新 tab 时清空旧 tab 不适用的分页 offset；`snapshot_at` 可保留用于同一快照复用，也可在用户点击刷新时清除，但不得导致后续 endpoint 与 summary 使用不同快照。

在 `AdminAnalyticsFilterBar` 或新面板局部控件中增加：

- `excluded_mode` 三态控件：`included_only`、`include_excluded`、`excluded_only`。
- `currency` 输入或选择，用于金额排序币种和展示偏好。
- `active_only` 开关。
- `snapshot_at` datetime 输入。
- 新 tab 时间范围显式开关：关闭时 `time_range_explicit=false` 且请求不发送 start/end；开启或用户修改 start/end 时 `time_range_explicit=true`。
- URL/drilldown 写入的 `user_ids`、`plan_ids`、`inviter_id`、`invitee_id`、`subscription_id` 至少要能通过筛选摘要显示并一键清除，避免只能手写 URL。

- [ ] **步骤 4：新增 Billing Statistics section**

创建 `statistics-section.tsx`，带现有项目版权头，遵循系统设置现有表单模式。实现要求：

- 在 `types.ts` 定义：

```ts
export interface SubscriptionAnalyticsExcludedUser {
  user_id: number
  username?: string
  reason?: string
  excluded_at?: number
  excluded_by?: number
}
```

并在 `BillingSettings` 中定义：`'subscription_analytics.excluded_users': SubscriptionAnalyticsExcludedUser[]`。

- `defaultBillingSettings` 使用 `'subscription_analytics.excluded_users': []`。现有 `getOptionValue` 会在默认值是数组时 JSON.parse option string。
- 保存时调用 `useUpdateOption` / `updateSystemOption`，请求必须是：

```ts
updateOption.mutateAsync({
  key: 'subscription_analytics.excluded_users',
  value: JSON.stringify(excludedUsers),
})
```

不要把数组直接传给 `value`。

- 表单维护结构化行列表，至少支持 user_id、username、reason；user_id 必须为正整数。
- 保存响应确认 `success=true` 后，除 `useUpdateOption` 自动刷新 `['system-options']` 外，还要 `queryClient.invalidateQueries({ queryKey: ['admin-analytics'] })`；保存失败不得刷新统计缓存造成误解。

修改 `section-registry.tsx` 新增：

```ts
{
  id: 'statistics',
  titleKey: 'systemSettings.billing.statistics.title',
  descriptionKey: 'systemSettings.billing.statistics.description',
  build: (settings) => <SubscriptionAnalyticsStatisticsSection defaultValues={{ excludedUsers: settings['subscription_analytics.excluded_users'] }} />,
}
```

现有 `createSectionRegistry` 使用 `titleKey` 作为导航标题，本计划不要求扩展单独 `labelKey`。

- [ ] **步骤 5：补 i18n 与 static keys**

新增可见文案必须全部进入六语言 locale，并登记到 `static-keys.ts` 或以 `t('...')` 字面量出现。至少覆盖：

- 新 tab：`adminAnalytics.tabs.paidSubscriptionValue`、`adminAnalytics.tabs.invitationPaidSubscriptions`。
- summary 和 ranking：付费套餐剩余价值、token 口径、时间口径、排除用户审计金额、邀请付费金额、当前有效邀请付费金额、当前有效邀请剩余价值、付费套餐用户/套餐/来源、邀请人/下级/权益记录。
- filter：`excluded_mode` 三态 label、currency、active_only、snapshot_at、time_range_explicit、清除 user/plan/inviter/invitee/subscription filter。
- table columns：排除原因、本应计入金额、订单记录金额、确认单元、source attribution、valuation basis、unit inference basis、plan price、recognized remaining value。
- system settings：Statistics section 标题、描述、排除用户列表、帮助文案、添加/删除行、用户 ID、用户名、原因、保存成功/校验错误。
- 文案不得把 `SubscriptionOrder.money` 叫作「实收金额」。

实现验收必须运行 `bun run i18n:sync` 并确认无缺失 key。

- [ ] **步骤 6：运行前端定向验证**

运行：

```bash
cd web/default
bun test src/features/admin-analytics/lib/page-contract.test.ts src/features/admin-analytics/lib/drilldown.test.ts src/features/admin-analytics/lib/filters.test.ts src/features/admin-analytics/lib/format.test.ts
bun run i18n:sync
```

预期：PASS，i18n sync 无缺失 key。

---

## 任务 6：最终验证与收口

**文件：**
- 不新增业务文件；只允许根据验证失败做最小修复。

- [ ] **步骤 1：运行后端定向测试**

```bash
go test ./model -run 'Test(PaidSubscriptionValue|InvitationPaidSubscriptions)' -count=1
go test ./controller -run 'Test(PaidSubscriptionValue|InvitationPaidSubscriptions|AdminAnalyticsRejectsInvalidSnapshotAt|AdminAnalyticsOverviewParserStillDefaultsToThirtyDays)' -count=1
go test ./setting -run SubscriptionAnalytics -count=1
```

预期：PASS。

- [ ] **步骤 2：运行前端定向测试**

```bash
cd web/default
bun test src/features/admin-analytics/lib/filters.test.ts src/features/admin-analytics/lib/page-contract.test.ts src/features/admin-analytics/lib/drilldown.test.ts src/features/admin-analytics/lib/format.test.ts
bun run i18n:sync
```

预期：PASS 且无缺失 key。

- [ ] **步骤 3：运行类型检查**

```bash
cd web/default
bun run typecheck
```

预期：PASS。

- [ ] **步骤 4：提交实现（主代理收口）**

此步骤由主代理在所有实现子代理、规格审查、代码质量审查和验证命令全部通过后统一执行；开发子代理不得自行提交。

```bash
git add <changed-files>
git commit -m "feat(analytics): 新增付费套餐统计面板"
```

提交前不得 revert、stash 或覆盖用户未相关改动。
