# 管理员侧综合数据分析中心实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 实现管理员侧 `/admin-analytics` 综合数据分析中心，覆盖全站套餐结构、配额用量、用户生命周期、订阅转化、邀请奖励、调用消耗和风险异常。

**架构：** 后端新增 `/api/admin-analytics/*` 管理员接口，统一 `AdminAuth`，所有 DTO 在 `dto/admin_analytics.go` 中显式声明；业务库 `model.DB` 与日志库 `model.LOG_DB` 分阶段查询并在 Go 内存合并，禁止跨库 SQL JOIN。前端新增 `features/admin-analytics`，URL search 作为可分享状态，React Query 只请求 active tab，图表复用 VChart，drilldown 仅映射到白名单路由。

**技术栈：** Go 1.22+、Gin、GORM v2、SQLite/MySQL/PostgreSQL；React 19、TypeScript、TanStack Router、React Query、Base UI、Tailwind、`@visactor/react-vchart`、Bun、i18next。

---

## 依据与边界

- 规格文件：`C:/Users/34404/source/repos/new-api/docs/superpowers/specs/2026-05-20-admin-analytics-design.md`
- 项目规则：`C:/Users/34404/source/repos/new-api/AGENTS.md`
- 前端规则：`C:/Users/34404/source/repos/new-api/web/default/AGENTS.md`
- 实现直接在主工作区 `C:/Users/34404/source/repos/new-api` 开发，不创建、不切换 worktree。
- 不得修改、revert、stash unrelated 工作区改动。
- 子代理不运行项目级 build / lint / format；主代理最终统一运行验证。
- 不引入价格、余额、可用天数、runway 等指标。
- 不返回完整 API Key、渠道密钥、支付 payload、OAuth secret。
- 多选 query canonical wire format 使用 repeated query params。
- `web/default/src/routeTree.gen.ts` 只能通过 TanStack Router 生成，不手写业务逻辑。

## 文件职责

### 后端新增文件

- `dto/admin_analytics.go`：管理员分析枚举、请求 query 结构、响应 DTO、分页 envelope、drilldown target、warnings。
- `model/admin_analytics.go`：业务库聚合、公共 query scope、active subscription helper、来源归一化、quota 分类、Overview / Plans / Quota / Users / Conversion / Invitations 聚合。
- `model/admin_analytics_usage.go`：Usage Consumption 日志候选读取、metered token 口径、Top N / Other、`logs.other` Go 内存解析、current/event_time 归因。
- `model/admin_analytics_risk.go`：Risk Insights 默认阈值、风险聚合、system risk。
- `model/admin_analytics_test.go`：业务库分析、来源、quota、active scope、conversion、invitation 测试。
- `model/admin_analytics_usage_test.go`：LOG_DB 分离、candidate limit、metered token、Usage query enum、归因测试。
- `model/admin_analytics_risk_test.go`：风险阈值边界和 system risk 测试。
- `controller/admin_analytics.go`：管理员 API query 解析、校验、错误响应、调用 model/service、返回 DTO。
- `controller/admin_analytics_test.go`：AdminAuth、默认/最大时间窗口、enum/sort/limit、response shape、drilldown 不泄密测试。

### 后端修改文件

- `router/api-router.go`：注册 `/api/admin-analytics` 管理员路由组。
- `model/log.go`：如选择支持 Usage Logs `user_id` drilldown，则为管理员日志过滤新增 `UserId *int` 或等价字段，并只在 admin logs 生效。
- `controller/log.go`：如支持 Usage Logs `user_id` drilldown，则 `GetAllLogs` 与 `GetLogsStat` 解析 `user_id`，self logs 不接受前端传入的 `user_id`。
- `controller/log_usage_analytics_test.go`：覆盖管理员 `user_id` 日志过滤与 self logs 不接受外部 `user_id`。
- `model/user.go` / `controller/user.go`：如目标页直接扩展用户列表筛选，则新增 `user_id`、`plan_id`、`inviter_id` 过滤；如果目标页改用 `/api/admin-analytics/drilldown/users`，这两个文件不应修改。

### 前端新增文件

- `web/default/src/features/admin-analytics/types.ts`：后端 DTO、tab id、route search、canonical filters、drilldown target 类型。
- `web/default/src/features/admin-analytics/constants.ts`：tabs、metrics、bucket、risk severity、source、sort options，所有文案只存 `labelKey`。
- `web/default/src/features/admin-analytics/api.ts`：`/api/admin-analytics/*` 请求函数，GET 参数由 `URLSearchParams` repeated query params 序列化。
- `web/default/src/features/admin-analytics/lib/filters.ts`：Zod search schema、默认最近 30 天、canonical filters、query params 构造、数组去空去重排序、时间窗口校验。
- `web/default/src/features/admin-analytics/lib/format.ts`：token、percentage、latency、source、risk severity 格式化。
- `web/default/src/features/admin-analytics/lib/chart-data.ts`：VChart 数据转换、Top N / Other、bucket ordering、rate/latency 非加和处理。
- `web/default/src/features/admin-analytics/lib/page-contract.ts`：tab 到 endpoint 映射、query key、active tab request descriptors、partial unavailable 映射。
- `web/default/src/features/admin-analytics/lib/drilldown.ts`：后端 typed drilldown target 到前端白名单 route 的映射，拒绝未知 target。
- `web/default/src/features/admin-analytics/lib/filters.test.ts`
- `web/default/src/features/admin-analytics/lib/chart-data.test.ts`
- `web/default/src/features/admin-analytics/lib/page-contract.test.ts`
- `web/default/src/features/admin-analytics/lib/drilldown.test.ts`
- `web/default/src/features/admin-analytics/index.tsx`：页面容器、active tab、filters、React Query 编排。
- `web/default/src/features/admin-analytics/components/*`：filter bar、tabs、summary cards、8 个 panel、图表和表格组件。
- `web/default/src/routes/_authenticated/admin-analytics/index.tsx`：TanStack Router file route，`AdminAuth` 前端 guard、`validateSearch`、页面渲染。

### 前端修改文件

- `web/default/src/hooks/use-sidebar-data.ts`：管理员菜单新增 `Operations Analytics`。
- `web/default/src/hooks/use-sidebar-config.ts`：登记 `/admin-analytics`。
- `web/default/src/hooks/use-sidebar-config.test.ts`：管理员可见、普通用户隐藏、URL config 测试。
- `web/default/src/i18n/static-keys.ts`
- `web/default/src/i18n/locales/en.json`
- `web/default/src/i18n/locales/zh.json`
- `web/default/src/i18n/locales/fr.json`
- `web/default/src/i18n/locales/ru.json`
- `web/default/src/i18n/locales/ja.json`
- `web/default/src/i18n/locales/vi.json`
- `web/default/src/routeTree.gen.ts`：由任务 10 主代理运行 `bun run build` 后通过 TanStack Router plugin 生成并检查，不归任务 9 子代理直接拥有。
- `web/default/src/routes/_authenticated/users/index.tsx`、`web/default/src/features/users/components/users-table.tsx`、`web/default/src/features/users/api.ts`、`web/default/src/features/users/types.ts`：仅当选择扩展用户列表 drilldown 目标页筛选时修改。
- `web/default/src/routes/_authenticated/usage-logs/$section.tsx`、`web/default/src/features/usage-logs/types.ts`、`web/default/src/features/usage-logs/lib/utils.ts`、`web/default/src/features/usage-logs/components/common-logs-filter-bar.tsx`：仅当支持 `admin_usage_logs.user_id` drilldown 时修改。

## 并发与串行规则

- 任务 1（DTO 合同）必须先完成，后续任务依赖其类型命名。
- 任务 2（业务库 model）拥有 `model/admin_analytics.go` 与共享 helper，必须在任务 3 修改 Usage model 之前完成 helper 合同。
- 任务 3（Usage model）只创建/修改 `model/admin_analytics_usage.go` 与 `model/admin_analytics_usage_test.go`，只读复用任务 2 的共享 helper，不得修改 `model/admin_analytics.go`。
- 任务 4（Risk model）依赖任务 2 和任务 3 的 helper；可以在两者完成后独立执行。
- 任务 5（controller/router）依赖后端 model；`router/api-router.go` 由该任务单独修改。
- 任务 6（前端 API/types/lib）依赖 DTO 合同，可与后端 model 并行，但不得修改 route/sidebar/i18n。
- 任务 7（页面 panels）依赖任务 6，不修改 route/sidebar/i18n/routeTree。
- 任务 8（drilldown 目标页）依赖任务 6 的 drilldown 类型；可能修改 users、usage logs、log controller/model，必须单独执行。
- 任务 9（入口/sidebar/i18n）依赖任务 6、任务 7、任务 8；必须在 users 与 usage-logs route search contract 完成后执行；不修改 `routeTree.gen.ts`，只创建 route 文件和更新 sidebar/i18n。
- 主代理最终统一运行验证。

---

## 任务 1：后端 DTO 与查询合同

**文件：**
- 创建：`dto/admin_analytics.go`
- 不修改：`router/api-router.go`、`controller/*`、`model/*`、所有前端文件

- [ ] **步骤 1：定义枚举常量与 query 类型**

在 `dto/admin_analytics.go` 创建 package `dto`，定义 tab、granularity、sort order、source、risk severity、Usage group/metric/attribution 枚举。所有字符串值必须与规格一致。

```go
package dto

type AdminAnalyticsGranularity string

const (
	AdminAnalyticsGranularityHour  AdminAnalyticsGranularity = "hour"
	AdminAnalyticsGranularityDay   AdminAnalyticsGranularity = "day"
	AdminAnalyticsGranularityWeek  AdminAnalyticsGranularity = "week"
	AdminAnalyticsGranularityMonth AdminAnalyticsGranularity = "month"
)

type AdminAnalyticsSortOrder string

const (
	AdminAnalyticsSortAsc  AdminAnalyticsSortOrder = "asc"
	AdminAnalyticsSortDesc AdminAnalyticsSortOrder = "desc"
)

type AdminAnalyticsSource string

const (
	AdminAnalyticsSourceOrder                    AdminAnalyticsSource = "order"
	AdminAnalyticsSourceTrialCode                AdminAnalyticsSource = "trial_code"
	AdminAnalyticsSourceInviteTrial              AdminAnalyticsSource = "invite_trial"
	AdminAnalyticsSourceMonthlyInviteEntitlement AdminAnalyticsSource = "monthly_invite_entitlement"
	AdminAnalyticsSourceAdmin                    AdminAnalyticsSource = "admin"
	AdminAnalyticsSourceRedemption               AdminAnalyticsSource = "redemption"
	AdminAnalyticsSourceSystem                   AdminAnalyticsSource = "system"
	AdminAnalyticsSourceUnknown                  AdminAnalyticsSource = "unknown"
)

type AdminUsageGroupBy string

const (
	AdminUsageGroupByUser               AdminUsageGroupBy = "user"
	AdminUsageGroupByPlan               AdminUsageGroupBy = "plan"
	AdminUsageGroupByModel              AdminUsageGroupBy = "model"
	AdminUsageGroupByUserGroup          AdminUsageGroupBy = "user_group"
	AdminUsageGroupByRequestGroup       AdminUsageGroupBy = "request_group"
	AdminUsageGroupByStream             AdminUsageGroupBy = "stream"
	AdminUsageGroupByStatus             AdminUsageGroupBy = "status"
	AdminUsageGroupByChannel            AdminUsageGroupBy = "channel"
	AdminUsageGroupByEndpoint           AdminUsageGroupBy = "endpoint"
	AdminUsageGroupByBillingSource      AdminUsageGroupBy = "billing_source"
	AdminUsageGroupByToken              AdminUsageGroupBy = "token"
	AdminUsageGroupBySubscriptionSource AdminUsageGroupBy = "subscription_source"
)

type AdminUsageMetric string

const (
	AdminUsageMetricRequestCount  AdminUsageMetric = "request_count"
	AdminUsageMetricTotalTokens   AdminUsageMetric = "total_tokens"
	AdminUsageMetricQuota         AdminUsageMetric = "quota"
	AdminUsageMetricErrorRate     AdminUsageMetric = "error_rate"
	AdminUsageMetricAvgLatencyMs  AdminUsageMetric = "avg_latency_ms"
	AdminUsageMetricP95LatencyMs  AdminUsageMetric = "p95_latency_ms"
	AdminUsageMetricActiveUsers   AdminUsageMetric = "active_users"
	AdminUsageMetricActiveAPIKeys AdminUsageMetric = "active_api_keys"
)

type AdminPlanAttribution string

const (
	AdminPlanAttributionCurrent   AdminPlanAttribution = "current"
	AdminPlanAttributionEventTime AdminPlanAttribution = "event_time"
)
```

- [ ] **步骤 2：定义通用响应 envelope**

继续在同文件定义 `AdminAnalyticsRangeMeta`、`AdminAnalyticsPage`、`AdminAnalyticsList[T]`、`AdminAnalyticsAvailabilityWarning`、`AdminAnalyticsPanelResponse[T]`。

```go
type AdminAnalyticsRangeMeta struct {
	StartTimestamp int64 `json:"start_timestamp"`
	EndTimestamp   int64 `json:"end_timestamp"`
	SnapshotAt     int64 `json:"snapshot_at"`
}

type AdminAnalyticsPage struct {
	Limit   int  `json:"limit"`
	Offset  int  `json:"offset"`
	Total   int  `json:"total"`
	HasMore bool `json:"has_more"`
}

type AdminAnalyticsList[T any] struct {
	Page      AdminAnalyticsPage      `json:"page"`
	Items     []T                     `json:"items"`
	SortBy    string                  `json:"sort_by"`
	SortOrder AdminAnalyticsSortOrder `json:"sort_order"`
}

type AdminAnalyticsAvailabilityWarning struct {
	Section string `json:"section"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

type AdminAnalyticsPanelResponse[T any] struct {
	Range    AdminAnalyticsRangeMeta           `json:"range"`
	Data     T                                 `json:"data"`
	Warnings []AdminAnalyticsAvailabilityWarning `json:"warnings,omitempty"`
}
```

- [ ] **步骤 3：定义 overview / plan / quota / lifecycle / conversion / invitation DTO**

按规格字段逐项定义结构体。重点字段必须包含：

```go
type AdminAnalyticsOverviewResponse struct {
	Summary AdminAnalyticsOverviewSummary      `json:"summary"`
	Trends  []AdminAnalyticsOverviewTrendPoint `json:"trends"`
}

type AdminAnalyticsOverviewInvitations struct {
	UsersWithInviter              int `json:"users_with_inviter"`
	InvitersCount                 int `json:"inviters_count"`
	DirectInviteCount             int `json:"direct_invite_count"`
	QualifiedInviteCount          int `json:"qualified_invite_count"`
	QualifiedInviterCount         int `json:"qualified_inviter_count"`
	RewardUsers                   int `json:"reward_users"`
	RewardSubscriptions           int `json:"reward_subscriptions"`
	RewardActiveSubscriptionCount int `json:"reward_active_subscription_count"`
	RewardExpiredSubscriptionCount int `json:"reward_expired_subscription_count"`
}

type AdminAnalyticsPlanDistributionResponse struct {
	Groups          AdminAnalyticsList[AdminAnalyticsPlanGroup] `json:"groups"`
	Other           *AdminAnalyticsPlanGroup                    `json:"other"`
	LifecycleTrends []AdminAnalyticsPlanLifecycleTrendPoint     `json:"lifecycle_trends"`
	Health          []AdminAnalyticsPlanHealth                  `json:"health"`
}

type AdminAnalyticsQuotaDistributionResponse struct {
	Buckets                 []AdminAnalyticsQuotaBucket                         `json:"buckets"`
	Trends                  []AdminAnalyticsQuotaTrendPoint                     `json:"trends"`
	HighUsageUsers          AdminAnalyticsList[AdminAnalyticsSubscriptionRankingItem] `json:"high_usage_users"`
	IdleSubscriptions       AdminAnalyticsList[AdminAnalyticsSubscriptionRankingItem] `json:"idle_subscriptions"`
	ExhaustingSubscriptions AdminAnalyticsList[AdminAnalyticsSubscriptionRankingItem] `json:"exhausting_subscriptions"`
}
```

结构体字段按规格完整补齐，不得用 `map[string]any` 或 `gin.H` 作为业务 DTO。

- [ ] **步骤 4：定义 Usage / Risk / Drilldown DTO**

按规格定义：

```go
type AdminUsageConsumptionSummaryResponse struct {
	Total   AdminUsageMetrics              `json:"total"`
	Groups  AdminAnalyticsList[AdminUsageGroup] `json:"groups"`
	GroupBy AdminUsageGroupBy              `json:"group_by"`
	Other   *AdminUsageGroup               `json:"other"`
}

type AdminUsageBreakdownResponse struct {
	Groups  AdminAnalyticsList[AdminUsageGroup] `json:"groups"`
	GroupBy AdminUsageGroupBy              `json:"group_by"`
	Other   *AdminUsageGroup               `json:"other"`
}

type AdminAnalyticsRisksResponse struct {
	PlanRisks       AdminAnalyticsList[AdminAnalyticsRiskItem] `json:"plan_risks"`
	UserRisks       AdminAnalyticsList[AdminAnalyticsRiskItem] `json:"user_risks"`
	InvitationRisks AdminAnalyticsList[AdminAnalyticsRiskItem] `json:"invitation_risks"`
	SystemRisks     AdminAnalyticsList[AdminAnalyticsRiskItem] `json:"system_risks"`
}

type AdminAnalyticsDrilldownTarget struct {
	Kind           string `json:"kind"`
	UserID         *int   `json:"user_id,omitempty"`
	UserIDs        []int  `json:"user_ids,omitempty"`
	Username       string `json:"username,omitempty"`
	UserGroup      string `json:"user_group,omitempty"`
	UserStatus     string `json:"user_status,omitempty"`
	PlanID         *int   `json:"plan_id,omitempty"`
	InviterID      *int   `json:"inviter_id,omitempty"`
	TokenID        *int   `json:"token_id,omitempty"`
	Model          string `json:"model,omitempty"`
	RequestGroup   string `json:"request_group,omitempty"`
	ChannelID      *int   `json:"channel_id,omitempty"`
	Status         string `json:"status,omitempty"`
	StartTimestamp int64  `json:"start_timestamp,omitempty"`
	EndTimestamp   int64  `json:"end_timestamp,omitempty"`
	Tab            string `json:"tab,omitempty"`
}

type AdminAnalyticsDrilldownUsersResponse struct {
	Users AdminAnalyticsList[AdminAnalyticsDrilldownUserItem] `json:"users"`
}

type AdminAnalyticsDrilldownSubscriptionsResponse struct {
	Subscriptions AdminAnalyticsList[AdminAnalyticsDrilldownSubscriptionItem] `json:"subscriptions"`
}

type AdminAnalyticsDrilldownInvitationsResponse struct {
	Invitations AdminAnalyticsList[AdminAnalyticsDrilldownInvitationItem] `json:"invitations"`
}
```

`AdminAnalyticsDrilldownTarget` 必须与规格中的 union contract 等价；也可以拆为 per-kind struct，但 JSON 输出只能包含白名单字段。controller/model 必须按 `kind` 校验字段组合，不得把任意 URL、任意 query key 或敏感字段透传给前端。

drilldown list DTO 必须是瘦 DTO，不得直接序列化 `model.User`、`model.Token`、`model.Channel`、`SubscriptionOrder`、OAuth 绑定或 provider payload。允许字段按规格白名单补齐，禁止完整 API Key、渠道密钥、支付 payload、OAuth token/secret、password、access_token、setting、remark、key、allow_ips、keys、other_info、param_override、header_override。

- [ ] **步骤 5：运行 DTO 编译检查**

运行：

```bash
go test ./dto -count=1
```

预期：PASS 或 `? github.com/QuantumNous/new-api/dto [no test files]`。

---

## 任务 2：后端业务库聚合 model

**文件：**
- 创建/修改：`model/admin_analytics.go`
- 创建/修改：`model/admin_analytics_test.go`
- 依赖：任务 1 的 DTO
- 不修改：`controller/*`、`router/api-router.go`、前端文件

- [ ] **步骤 1：编写分离 DB 测试 fixture**

在 `model/admin_analytics_test.go` 创建两个独立 SQLite 内存库：`DB` 只迁移业务表，`LOG_DB` 只迁移 `Log`。参考用户侧 Usage Analytics 测试，但不得把业务表迁移到 `LOG_DB`，不得把 `logs` 迁移到 `DB`。

```go
func setupAdminAnalyticsTestDBs(t *testing.T) {
	t.Helper()
	oldDB := DB
	oldLogDB := LOG_DB
	// 保存 common.UsingSQLite / UsingMySQL / UsingPostgreSQL / RedisEnabled
	// DB = business sqlite, AutoMigrate(User, Token, SubscriptionPlan, UserSubscription, SubscriptionOrder, InvitationMonthlyEntitlement, Channel)
	// LOG_DB = log sqlite, AutoMigrate(Log)
	// Cleanup 恢复全局变量并关闭连接
}
```

- [ ] **步骤 2：编写 active scope 红灯测试**

测试名：`TestAdminAnalyticsActiveSubscriptionScopeIsSharedBySnapshotDomains`。

构造：

- active：`status=active,start_time=snapshot-10,end_time=snapshot+10`
- expired-but-status-active：`status=active,end_time=snapshot`
- future-start：`status=active,start_time=snapshot+10`
- inactive：`status=expired`

断言 Overview、Plan Distribution、Quota、Invitation、Risk 共享同一个 active scope，只计入第一条，过期但 status active 只进入 `expired_active_status` 风险。

- [ ] **步骤 3：实现 active subscription helper**

在 `model/admin_analytics.go` 实现共享 query scope / helper：

```go
func applyAdminActiveSubscriptionScope(tx *gorm.DB, snapshotAt int64) *gorm.DB {
	return tx.Where("status = ? AND start_time <= ? AND end_time > ?", "active", snapshotAt, snapshotAt)
}
```

所有 active/paid/trial/reward/expiring/idle/risk/attribution 的快照计算必须复用 helper 或同一段 query builder。

- [ ] **步骤 4：编写来源归一化与 token 分类测试**

测试名：`TestAdminAnalyticsNormalizesSubscriptionSourceAcrossDomains`。

覆盖：

- `grant_reason=admin,source=order` => admin 优先
- `grant_reason='',source=order` => order
- `grant_reason='',source=redemption` => redemption
- 未知值 => unknown
- paid_count 不靠价格或 token limit 推断，必须归一化为 order 且有成功订单或历史兼容记录

测试名：`TestAdminAnalyticsQuotaBucketsHandleZeroLimitUnlimitedAndOver100`。

覆盖：

- `token_limit=0, normalized_source=trial_code` => unlimited_or_invalid，`usage_rate=nil`，不进入 zero risk
- `token_limit=0, normalized_source=admin` => zero_limit，进入 zero risk
- `token_limit>0, token_used>token_limit` => over_100，remaining=0
- 负数 token limit / token used => system risk 输入

- [ ] **步骤 5：实现来源归一化与 token 分类**

实现纯函数：

```go
func normalizeAdminSubscriptionSource(grantReason string, source string) dto.AdminAnalyticsSource

type adminQuotaClass struct {
	UsageRate      *float64
	RemainingTokens *int64
	TokenUnlimited bool
	Bucket         string
	ValidForRate   bool
	SystemRisk     bool
}

func classifyAdminSubscriptionQuota(tokenLimit int64, tokenUsed int64, normalizedSource dto.AdminAnalyticsSource) adminQuotaClass
```

不得使用价格、余额、可用天数、runway 推导任何分类。

- [ ] **步骤 6：实现 Overview / Plan / Quota / Lifecycle / Conversion / Invitation 聚合**

在 `model/admin_analytics.go` 实现导出函数，命名建议：

```go
func GetAdminAnalyticsOverview(query AdminAnalyticsQuery) (dto.AdminAnalyticsPanelResponse[dto.AdminAnalyticsOverviewResponse], error)
func GetAdminAnalyticsPlanDistribution(query AdminAnalyticsQuery) (dto.AdminAnalyticsPanelResponse[dto.AdminAnalyticsPlanDistributionResponse], error)
func GetAdminAnalyticsQuotaDistribution(query AdminAnalyticsQuery) (dto.AdminAnalyticsPanelResponse[dto.AdminAnalyticsQuotaDistributionResponse], error)
func GetAdminAnalyticsUserLifecycle(query AdminAnalyticsQuery) (dto.AdminAnalyticsPanelResponse[dto.AdminAnalyticsUserLifecycleResponse], error)
func GetAdminAnalyticsSubscriptionConversion(query AdminAnalyticsQuery) (dto.AdminAnalyticsPanelResponse[dto.AdminAnalyticsSubscriptionConversionResponse], error)
func GetAdminAnalyticsInvitationRewards(query AdminAnalyticsQuery) (dto.AdminAnalyticsPanelResponse[dto.AdminAnalyticsInvitationRewardsResponse], error)
```

`AdminAnalyticsQuery` 可放 model 层，包含已解析好的 typed filters、range、pagination、sort。

- [ ] **步骤 7：实现排序与分页 helper**

实现对内存切片的安全排序和分页，不拼接任意 SQL 列名：

```go
func buildAdminAnalyticsPage(total int, limit int, offset int) dto.AdminAnalyticsPage
func paginateAdminAnalyticsList[T any](items []T, limit int, offset int) ([]T, dto.AdminAnalyticsPage)
```

每个 sort_by 只允许白名单字段，非法字段由 controller 层拒绝。

- [ ] **步骤 8：补齐业务库聚合测试**

覆盖测试名：

- `TestAdminAnalyticsPlanDistributionAggregatesTokenQuotaOnly`
- `TestAdminAnalyticsTrialToPaidConversionUsesUserTimeline`
- `TestAdminAnalyticsRenewalUsesGraceWindow`
- `TestAdminAnalyticsPlanMigrationBuildsMatrixWithoutPriceRunway`
- `TestAdminAnalyticsInvitationRewardsDeriveMonthlyStatus`
- `TestAdminAnalyticsSeparatesUserGroupsAndRequestGroups`

测试必须真实写入 `User`、`SubscriptionPlan`、`UserSubscription`、`SubscriptionOrder`、`InvitationMonthlyEntitlement`，不 mock model 业务逻辑。

- [ ] **步骤 9：补三库兼容与方言防回归测试**

测试名：`TestAdminAnalyticsSQLBuilderAvoidsDialectSpecificFunctions`。

覆盖：

- 业务库聚合和排序分页不拼接任意 SQL 列名，不使用裸 `group` / `key` 保留字。
- Raw SQL 如不可避免，必须使用项目已有 `commonGroupCol`、`commonKeyCol` 等跨库字段名；否则优先使用 GORM API。
- 使用 SQLite fixture 加 MySQL/PostgreSQL GORM DryRun 或 env-gated DSN 验证 SQL 不含 `DATE_TRUNC`、`FROM_UNIXTIME`、`strftime`、`FILTER`、`PERCENTILE_CONT`、数据库窗口函数、JSON operator、MySQL-only/PostgreSQL-only 函数。
- 此测试不要求连接真实 MySQL/PostgreSQL；如果本地未配置 DSN，DryRun 断言仍必须覆盖 SQL builder 的方言无关性。

- [ ] **步骤 10：运行 model 业务测试**

运行：

```bash
go test ./model -run 'AdminAnalytics.*(ActiveSubscription|QuotaBuckets|PlanDistribution|TrialToPaid|Renewal|PlanMigration|Invitation|Normalizes|Separates)' -count=1
```

预期：PASS。

---

## 任务 3：后端 Usage Consumption 聚合

**文件：**
- 创建/修改：`model/admin_analytics_usage.go`
- 创建/修改：`model/admin_analytics_usage_test.go`
- 只读复用：`model/admin_analytics.go` 中任务 2 已完成的共享 helper；任务 3 不得修改该文件
- 不修改：controller、router、前端文件

- [ ] **步骤 1：编写 LOG_DB 分离测试**

测试名：`TestAdminAnalyticsUsageUsesSeparatedDBAndLogDB`。

构造两个独立 DB：

- `LOG_DB` 只写 logs。
- `DB` 只写 users/tokens/subscriptions/plans/channels。

断言 Usage Consumption 能补充用户/套餐/渠道信息；如果实现跨库 JOIN，测试应失败。

- [ ] **步骤 2：编写 metered token 口径测试**

测试名：`TestAdminAnalyticsUsageUsesMeteredTokensFallbackAndExplicitZero`。

覆盖：

- `metered_tokens=80` 使用 80。
- `metered_tokens=nil` fallback prompt + completion。
- `metered_tokens=0` 保留 0，不 fallback。
- error log 的 tokens/quota 按 0 进入消耗，但参与 request/error/rpm/latency。

测试名：`TestAdminAnalyticsUsageParsesOtherBillingFieldsAndExplicitSubscriptionTokens`。

覆盖：

- `logs.other.billing_source` 为空或未知时归 `unknown`。
- `billing_source=subscription` 时，`subscription_tokens_consumed` 是 subscription-source token 消耗权威值。
- `subscription_tokens_consumed=0` 必须保留显式 0，不 fallback 到 metered_tokens 或 prompt+completion。
- `endpoint` / `request_path`、`subscription_source`、`request_group`、`channel_id` 等维度只通过 Go 内存解析 `logs.other` 获取，禁止数据库 JSON operator。
- candidate limit 超限时不继续解析 `logs.other`。

- [ ] **步骤 3：实现日志候选读取与 token 计算**

在 `model/admin_analytics_usage.go` 实现：

```go
const adminAnalyticsCandidateLogLimit = 100000

type adminUsageCandidateLog struct { ... }

func loadAdminUsageCandidateLogs(query AdminAnalyticsQuery) ([]adminUsageCandidateLog, []dto.AdminAnalyticsAvailabilityWarning, error)
func adminUsageLogTokens(log adminUsageCandidateLog) int64
```

`logs.other` 解析必须使用 `common.UnmarshalJsonStr` 或现有 common JSON wrapper，禁止数据库 JSON operator。

- [ ] **步骤 4：编写 candidate limit 与 query enum 测试**

测试名：`TestAdminAnalyticsUsageCandidateLimitReturnsPartialUnavailable`。

断言超过候选上限时停止扫描并返回 `candidate_limit_exceeded` warning 或 400，不继续解析 `logs.other`。

测试名：`TestAdminAnalyticsUsageQueryValidatesGroupByMetricAttributionAndTopN`。

覆盖 summary、timeseries、breakdown 的：

- `group_by` 白名单
- `metric` 白名单
- `plan_attribution` 白名单
- `top_n` 默认 20、最大 100
- 非法值 400，超出值 clamp
- timeseries 显式传 `sort_by` 返回 `unsupported sort_by`

- [ ] **步骤 5：编写 event_time 归因验收测试**

覆盖测试名：

- `TestAdminAnalyticsUsageEventTimeAttributionUsesSubscriptionIDFromOther`：`logs.other.subscription_id` 是权威来源，优先于 `subscription_plan_id` 和当前 active 设置。
- `TestAdminAnalyticsUsageEventTimeAttributionFallsBackToSubscriptionWindow`：历史日志缺少 `subscription_id` 时，按 `user_id + [start_time,end_time)` 匹配当时有效订阅。
- `TestAdminAnalyticsUsageEventTimeAttributionAmbiguousHistoryReturnsUnknownWarning`：多个候选且无权威 `subscription_id` 时归 unknown，并返回 `insufficient_history` warning。
- `TestAdminAnalyticsUsageEventTimeAttributionDoesNotUseActiveSubscriptionSettingOrReset`：归因 selector 纯只读，不读取当前 `ActiveSubscriptionId`，不触发 reset，不修改订阅或用户设置。

- [ ] **步骤 6：实现 current / event_time plan attribution**

实现：

```go
func enrichAdminUsageWithCurrentPlans(logs []adminUsageCandidateLog, snapshotAt int64) (...)
func enrichAdminUsageWithEventTimePlans(logs []adminUsageCandidateLog) (...)
```

规则：

- 优先使用 `logs.other.subscription_id`。
- `subscription_plan_id` 只能校验或降级展示。
- 历史日志缺少 `subscription_id` 时，按 `user_id` 与 `[start_time,end_time)` 匹配。
- selector 必须纯只读，不读取当前 `ActiveSubscriptionId`，不触发 reset，不修改订阅或用户设置。
- 多候选无权威 subscription_id 时归入 unknown，并返回 `partial_unavailable(reason='insufficient_history')`。

- [ ] **步骤 7：实现 summary / timeseries / breakdown**

导出函数：

```go
func GetAdminUsageConsumptionSummary(query AdminAnalyticsUsageQuery) (dto.AdminAnalyticsPanelResponse[dto.AdminUsageConsumptionSummaryResponse], error)
func GetAdminUsageConsumptionTimeseries(query AdminAnalyticsUsageQuery) (dto.AdminAnalyticsPanelResponse[dto.AdminUsageTimeseriesResponse], error)
func GetAdminUsageConsumptionBreakdown(query AdminAnalyticsUsageQuery) (dto.AdminAnalyticsPanelResponse[dto.AdminUsageBreakdownResponse], error)
```

Top N / Other：

- summary / breakdown groups 使用 `AdminAnalyticsList[AdminUsageGroup]`。
- timeseries 不接受 `sort_by`。
- additive metrics 可 sum Other；rate/latency 必须按分子分母或样本重算，不能简单平均。

- [ ] **步骤 8：补 LOG_DB 方言与 bool 查询防回归测试**

测试名：`TestAdminAnalyticsUsageLogDBBooleanAndDialectAreIndependentFromBusinessDB`。

覆盖：

- DB 与 LOG_DB 可使用不同方言设置，Usage 查询不得复用业务库 `commonTrueVal/commonFalseVal`。
- `logs.is_stream` 优先使用 GORM 参数绑定；如必须手写 bool literal，必须使用 LOG_DB 专用方言 helper（如 `logTrueVal/logFalseVal`）。
- DryRun SQL 不含业务库 bool literal 串、不含数据库 JSON operator、不含方言专用时间函数。

- [ ] **步骤 9：运行 Usage model 测试**

运行：

```bash
go test ./model -run 'AdminAnalyticsUsage' -count=1
```

预期：PASS。

---

## 任务 4：后端 Risk Insights 聚合

**文件：**
- 创建/修改：`model/admin_analytics_risk.go`
- 创建/修改：`model/admin_analytics_risk_test.go`
- 只读复用：`model/admin_analytics.go`、`model/admin_analytics_usage.go` 中任务 2/3 已完成的共享 helper；如风险实现确需新共享 helper，应在 `model/admin_analytics_risk.go` 内私有定义或由主代理协调单独修改

- [ ] **步骤 1：编写风险阈值边界测试**

在 `model/admin_analytics_risk_test.go` 覆盖：

- `high_exhaustion_risk`：usage rate 89.9% 不触发，90% 触发。
- `overused_subscription`：finite quota 超 100% 触发，unlimited/invalid 不触发。
- `zero_limit_active_subscription`：`token_limit=0` 且非 explicit unlimited trial 触发，explicit unlimited trial 不触发。
- `expired_active_status`：`status=active` 但 `end_time <= snapshot_at` 触发。
- `overlapping_active_subscription`：同一用户同一 snapshot 多个 active subscription 触发。
- `reset_overdue`：`next_reset_time <= snapshot_at - 24h` 且 finite `token_used > 0` 触发。
- `underused_plan_subscription`：订阅已过半但 usage rate 低于规格阈值触发。
- `reset_pressure`：接近 reset 且高使用率触发。
- `active_subscription_no_api_key`：active subscription 用户无可用 API key 触发。
- `active_subscription_no_request`：active subscription 在窗口内无请求触发。
- `high_error_rate_user`、`sudden_usage_spike`、`many_failed_requests`、`many_tokens_across_many_models`、`abnormal_stream_ratio`：按规格阈值分别覆盖低于阈值/达到阈值边界。
- `many_invites_low_qualified`：direct 19 不触发，direct 20 且 qualified/direct < 10% 触发。
- `reward_subscription_never_used`：发放未满 7 天不触发，满 7 天且无消耗/无请求触发。
- `reward_downgrade_frequently_triggered`、`reward_plan_exhausted_rapidly`：按规格阈值覆盖边界。
- `invalid_negative_token_quota`：负 `token_limit` 或负 `token_used` 触发 system risk。
- `candidate_log_limit_exceeded`：Usage/Risk 命中候选日志上限时返回 system risk，severity 为 warning，drilldown 为 null，并携带 panel warning。

- [ ] **步骤 2：实现风险常量和风险 item 构造**

在 `model/admin_analytics_risk.go` 定义规格中的全部默认阈值常量。至少包含：

```go
const adminRiskHighErrorMinRequests = 20
const adminRiskHighErrorRate = 0.20
const adminRiskManyFailedRequests = 50
const adminRiskInviteMinDirect = 20
const adminRiskInviteQualifiedRate = 0.10
const adminRiskRewardNeverUsedMinAgeSeconds = 7 * 24 * 3600
const adminRiskResetOverdueGraceSeconds = 24 * 3600
```

同时补齐规格中 `underused_plan_subscription`、`reset_pressure`、`sudden_usage_spike`、`many_tokens_across_many_models`、`abnormal_stream_ratio`、`reward_downgrade_frequently_triggered`、`reward_plan_exhausted_rapidly` 等规则所需阈值。不得把阈值写成散落的 magic number。

构造 `dto.AdminAnalyticsRiskItem` 时必须填充：`risk_key`、`severity`、`category`、`threshold`、`sample_size`、`drilldown`。

- [ ] **步骤 3：实现 `GetAdminAnalyticsRisks`**

返回：

```go
func GetAdminAnalyticsRisks(query AdminAnalyticsQuery) (dto.AdminAnalyticsPanelResponse[dto.AdminAnalyticsRisksResponse], error)
```

每个 risk list 使用 `AdminAnalyticsList[AdminAnalyticsRiskItem]`，默认 Top 20，最大 100。

- [ ] **步骤 4：运行风险测试**

运行：

```bash
go test ./model -run 'AdminAnalytics.*Risk|AdminAnalytics.*Risks' -count=1
```

预期：PASS。

---

## 任务 5：后端 controller 与 router

**文件：**
- 创建：`controller/admin_analytics.go`
- 创建：`controller/admin_analytics_test.go`
- 修改：`router/api-router.go`
- 依赖：任务 1-4
- 不修改：前端文件

- [ ] **步骤 1：编写 controller 参数测试**

在 `controller/admin_analytics_test.go` 创建测试 fixture，覆盖：

- 非管理员拒绝。
- 管理员可访问。
- 空 query 默认最近 30 天。
- `end_timestamp - start_timestamp > 365d` 返回 HTTP 400，message `time range exceeds 365 days`。
- `start_timestamp > end_timestamp` 返回 HTTP 400，message `invalid time range`。
- invalid enum 返回 400。
- unsupported sort_by 返回 400，message `unsupported sort_by`。
- endpoint 矩阵校验：plan groups、quota 三个 ranking list、invitation inviters、risks 四个 risk list、drilldown users/subscriptions/invitations 各自使用独立 `sort_by` 白名单，不能复用错误白名单。
- negative `offset`、非法 id、非法 repeated param 返回 400。
- `limit/top_n > 100` clamp 到 100；非法数字返回 400。
- repeated query params 解析为数组、trim、去空、去重、排序。

- [ ] **步骤 2：实现 query parser**

在 `controller/admin_analytics.go` 实现：

```go
func parseAdminAnalyticsQuery(c *gin.Context) (model.AdminAnalyticsQuery, error)
func parseAdminUsageAnalyticsQuery(c *gin.Context) (model.AdminAnalyticsUsageQuery, error)
```

解析规则必须与规格一致。不要把 comma-separated 作为 canonical wire format。

- [ ] **步骤 3：实现 handler**

实现：

```go
func GetAdminAnalyticsOverview(c *gin.Context)
func GetAdminAnalyticsPlanDistribution(c *gin.Context)
func GetAdminAnalyticsQuotaDistribution(c *gin.Context)
func GetAdminAnalyticsUserLifecycle(c *gin.Context)
func GetAdminAnalyticsSubscriptionConversion(c *gin.Context)
func GetAdminAnalyticsInvitationRewards(c *gin.Context)
func GetAdminUsageConsumptionSummary(c *gin.Context)
func GetAdminUsageConsumptionTimeseries(c *gin.Context)
func GetAdminUsageConsumptionBreakdown(c *gin.Context)
func GetAdminAnalyticsRisks(c *gin.Context)
func GetAdminAnalyticsDrilldownUsers(c *gin.Context)
func GetAdminAnalyticsDrilldownSubscriptions(c *gin.Context)
func GetAdminAnalyticsDrilldownInvitations(c *gin.Context)
```

所有成功响应使用统一 envelope；不要返回 `gin.H` 作为业务 data。

三个 drilldown handler 必须返回分页瘦 DTO：`AdminAnalyticsDrilldownUsersResponse`、`AdminAnalyticsDrilldownSubscriptionsResponse`、`AdminAnalyticsDrilldownInvitationsResponse`。每个 endpoint 必须支持 `limit` clamp、`offset`、独立 `sort_by` 白名单、typed filters，并用 `AdminAnalyticsList[T]` 返回分页元数据。

- [ ] **步骤 4：注册路由**

在 `router/api-router.go` 添加：

```go
adminAnalyticsRoute := apiRouter.Group("/admin-analytics")
adminAnalyticsRoute.Use(middleware.AdminAuth())
{
	adminAnalyticsRoute.GET("/overview", controller.GetAdminAnalyticsOverview)
	// 其余规格路由
}
```

必须用 group-level `AdminAuth`，不得仅依赖前端。

- [ ] **步骤 5：补全部响应不泄密测试**

测试所有 Admin Analytics 分析响应与 drilldown response 不包含：完整 API Key、渠道密钥、支付 payload、OAuth secret、`model.User` 原始敏感字段、`model.Token` 原始 key / allow_ips、`model.Channel` 原始配置字段、`SubscriptionOrder` provider payload、OAuth token/secret。禁用 JSON key 至少覆盖 `password`、`access_token`、`stripe_customer`、`aff_code`、`setting`、`remark`、`key`、`allow_ips`、`keys`、`other_info`、`param_override`、`header_override`。测试不得只覆盖一个 endpoint，应覆盖 overview/ranking/usage/risk/drilldown list 至少各一个响应。

- [ ] **步骤 6：运行 controller 测试**

运行：

```bash
go test ./controller -run 'AdminAnalytics' -count=1
```

预期：PASS。

---

## 任务 6：前端 API、类型、纯函数库

**文件：**
- 创建：`web/default/src/features/admin-analytics/types.ts`
- 创建：`web/default/src/features/admin-analytics/constants.ts`
- 创建：`web/default/src/features/admin-analytics/api.ts`
- 创建：`web/default/src/features/admin-analytics/lib/filters.ts`
- 创建：`web/default/src/features/admin-analytics/lib/format.ts`
- 创建：`web/default/src/features/admin-analytics/lib/chart-data.ts`
- 创建：`web/default/src/features/admin-analytics/lib/page-contract.ts`
- 创建：`web/default/src/features/admin-analytics/lib/drilldown.ts`
- 创建对应 `*.test.ts`
- 不修改：route、sidebar、i18n、routeTree、页面组件

- [ ] **步骤 1：编写 filters 测试**

`filters.test.ts` 覆盖：

- 空 URL => 最近 30 天、tab overview、granularity day。
- repeated query params 去空、去重、排序。
- `user_groups` 与 `request_groups` 分别序列化，不输出 `groups`。
- 单值 query 归一化为数组。
- invalid enum 进入错误状态。
- over-365d 前端策略与后端 400 不冲突。

- [ ] **步骤 2：实现 types/constants/filters**

`types.ts` 按 DTO 定义 TS 类型；`constants.ts` 所有 label 使用 `labelKey`；`filters.ts` 使用 Zod schema，输出 `AdminAnalyticsCanonicalFilters`。

- [ ] **步骤 3：编写 page-contract 测试**

覆盖：

- tab -> endpoint 映射：overview/plans/quota/users/conversion/invitations/risks 各一个 endpoint；usage 生成 summary/timeseries/breakdown 三个 endpoint。
- 未激活 tab 不产生 request descriptor 或 enabled=false。
- query key 形如 `['admin-analytics', tab, endpoint, canonicalFilters]`。
- canonical filters 不含 Date/function/t/draft/未排序数组。
- draft change 不改变 query key；Apply 后生成新 canonical filters。
- warnings -> partial unavailable 状态映射。

- [ ] **步骤 4：实现 api/page-contract**

`api.ts` 只接受 canonical params 并序列化为 `URLSearchParams`。Usage timeseries 不发送 `sort_by`。

- [ ] **步骤 5：编写 chart-data 测试并实现**

覆盖：

- quota bucket 固定排序。
- Top N + Other：Other 固定 key `__other__`，不可 drilldown。
- additive metrics 可 sum；rate/latency 不简单求和。
- VChart series key 稳定。

- [ ] **步骤 6：编写 drilldown 测试并实现**

覆盖：

- 后端 typed target -> 前端白名单 route。
- unknown kind rejected。
- `/usage-logs/$section` 使用 `{ section: 'common' }`。
- `start_timestamp/end_timestamp` 秒转毫秒 `startTime/endTime`。
- `userId`、`username` 行为符合规格。
- 不把完整 API Key、渠道密钥、支付 payload、OAuth secret 放入 URL。
- `admin_users` 支持 `user_id`、`user_ids`、`user_group`、`user_status`、`plan_id`、`inviter_id` 白名单映射。
- `admin_usage_logs` 支持 `user_id`、`username`、`token_id`、`model`、`request_group`、`channel_id`、`status`、`start_timestamp`、`end_timestamp` 白名单映射。
- `admin_subscriptions` 与 `admin_invitations` 只映射到语义匹配的目标页或本功能 drilldown list，不得跳到语义不匹配的套餐 CRUD 页。

- [ ] **步骤 7：运行前端纯函数测试**

运行：

```bash
cd web/default && bun test src/features/admin-analytics/lib/filters.test.ts src/features/admin-analytics/lib/chart-data.test.ts src/features/admin-analytics/lib/page-contract.test.ts src/features/admin-analytics/lib/drilldown.test.ts
```

预期：PASS。

---

## 任务 7：前端页面与 panel 组件

**文件：**
- 创建：`web/default/src/features/admin-analytics/index.tsx`
- 创建：`web/default/src/features/admin-analytics/components/*.tsx`
- 依赖：任务 6
- 不修改：route、sidebar、i18n、routeTree

- [ ] **步骤 1：实现页面 shell**

`index.tsx` 导出 `AdminAnalyticsPage`，props 包含：

```ts
interface AdminAnalyticsPageProps {
  search: AdminAnalyticsSearch
  onSearchChange: (next: AdminAnalyticsSearch) => void
  onDrilldown: (target: FrontendAdminAnalyticsDrilldownTarget) => void
}
```

组件从 props search 生成 canonical filters，不直接读 router。

- [ ] **步骤 2：实现 filter bar 和 tabs**

Filter bar 支持规格全局筛选；draft state 只在 Apply 后调用 `onSearchChange`。Tabs 切换写入 URL search。

- [ ] **步骤 3：实现 React Query active tab 请求**

只 mount 或只 enable 当前 active tab 的 queries。Usage tab 同时请求 summary/timeseries/breakdown。其他 tab 只请求对应 endpoint。

- [ ] **步骤 4：实现 8 个 panel**

创建：

- `overview-panel.tsx`
- `plan-distribution-panel.tsx`
- `quota-distribution-panel.tsx`
- `user-lifecycle-panel.tsx`
- `subscription-conversion-panel.tsx`
- `invitation-rewards-panel.tsx`
- `usage-consumption-panel.tsx`
- `risks-panel.tsx`

每个 panel 必须处理 loading、error、empty、background refetching、partial unavailable。

- [ ] **步骤 5：实现图表和表格组件**

复用 `@visactor/react-vchart`、`VCHART_OPTION`、现有 theme/empty/skeleton 模式。不得新增图表库。

表格移动端复用现有 DataTable / MobileCardList 模式，配置 mobile title / badge / hidden。

- [ ] **步骤 6：运行前端纯函数回归**

运行纯函数测试用于发现 page/panel 依赖的 contract 回归；页面组件 TSX 导入和 JSX 类型错误由任务 10 的统一 `bun run typecheck` / `bun run build` 验证。

```bash
cd web/default && bun test src/features/admin-analytics/lib/filters.test.ts src/features/admin-analytics/lib/chart-data.test.ts src/features/admin-analytics/lib/page-contract.test.ts src/features/admin-analytics/lib/drilldown.test.ts
```

预期：PASS。

---

## 任务 8：Drilldown 目标页 search contract

**文件：**
- 修改：`web/default/src/routes/_authenticated/users/index.tsx`
- 修改：`web/default/src/features/users/components/users-table.tsx`
- 修改：`web/default/src/features/users/api.ts`
- 修改：`web/default/src/features/users/types.ts`
- 修改：`controller/user.go`、`model/user.go`，或改为目标页调用 `/api/admin-analytics/drilldown/users`
- 修改：`web/default/src/routes/_authenticated/usage-logs/$section.tsx`
- 修改：`web/default/src/features/usage-logs/types.ts`
- 修改：`web/default/src/features/usage-logs/api.ts`
- 修改：`web/default/src/features/usage-logs/lib/utils.ts`
- 修改：`web/default/src/features/usage-logs/components/common-logs-filter-bar.tsx`
- 修改：`controller/log.go`、`model/log.go`、`controller/log_usage_analytics_test.go`，或从 DTO 移除 `admin_usage_logs.user_id`
- 依赖：任务 6

- [ ] **步骤 1：实现 users drilldown 策略**

选择一种策略并保持一致：

1. 扩展用户列表 API/types/后端筛选，支持 `userId`、`planId`、`inviterId`。
2. 或者目标页调用 `/api/admin-analytics/drilldown/users` 分页接口。

不得只在当前页数据上做客户端过滤。`/users?userId=...`、`/users?planId=...`、`/users?inviterId=...` 不得退化为无筛选列表。

如果选择策略 1，必须补充服务端和前端 API 验收：

- `controller/user*_test.go` 或等价测试覆盖 `user_id`、`plan_id`、`inviter_id` 过滤。
- `model/user*_test.go` 或等价测试覆盖 DB 层筛选不会返回无关用户。
- 前端 API/route 测试覆盖 URL search 进入请求参数，`getUsers` 序列化 `user_id` / `plan_id` / `inviter_id`。
- `UsersTable` 不得只对当前页数据做客户端过滤。

如果选择策略 2，则 `/api/admin-analytics/drilldown/users` 必须具备同等分页、sort、filter 和瘦 DTO 验收，并且前端目标页不能显示无筛选 users 列表。

- [ ] **步骤 2：实现 Usage Logs userId drilldown 或移除 user_id**

选择一种策略：

1. 支持 `userId`：扩展 Usage Logs route search、filters、params、Admin `/api/log` 与 `/api/log/stat` 的 `user_id` 过滤。self logs 不接受前端传入 `userId`。
2. 不支持 `userId`：从 `admin_usage_logs` DTO 和前端 target 移除 `user_id`，后端只在能提供 username 时返回 usage logs drilldown，否则返回 null 或 `/users?userId=...`。

推荐策略 1，因为管理员日志已经有全站视角。

选择策略 1 时，`web/default/src/features/usage-logs/api.ts` 必须保持 self API 类型边界：`getUserLogs` / `getUserLogStats` 不接收也不序列化 `user_id`；`buildApiParams` 仅在 admin `/api/log` 与 `/api/log/stat` 请求中发送 `user_id`。

- [ ] **步骤 3：补 drilldown tests**

`drilldown.test.ts` 覆盖：

- users target search 白名单。
- subscriptions active drilldown 不跳语义不匹配的套餐计划页。
- usage logs userId / username 行为。
- start/end 秒转毫秒。

后端测试覆盖：

- 管理员 `/api/log?user_id=<id>` 只返回该用户日志。
- `/api/log/stat?user_id=<id>` 只统计该用户。
- self logs 忽略或拒绝外部 user_id，不泄露其他用户数据。
- 用户列表或 `/api/admin-analytics/drilldown/users` 的后端测试覆盖 `user_id`、`plan_id`、`inviter_id` 不退化为无筛选列表。
- 前端 users API/route 测试覆盖 `userId`、`planId`、`inviterId` search 进入请求参数。

- [ ] **步骤 4：运行 drilldown 相关测试**

运行：

```bash
go test ./controller -run 'AdminLogs.*User|Log.*User|UsageAnalytics' -count=1
cd web/default && bun test src/features/admin-analytics/lib/drilldown.test.ts
```

预期：PASS。

---

## 任务 9：入口、sidebar、i18n、routeTree

**文件：**
- 创建：`web/default/src/routes/_authenticated/admin-analytics/index.tsx`
- 修改：`web/default/src/hooks/use-sidebar-data.ts`
- 修改：`web/default/src/hooks/use-sidebar-config.ts`
- 修改：`web/default/src/hooks/use-sidebar-config.test.ts`
- 修改：`web/default/src/i18n/static-keys.ts`
- 修改：`web/default/src/i18n/locales/en.json`
- 修改：`web/default/src/i18n/locales/zh.json`
- 修改：`web/default/src/i18n/locales/fr.json`
- 修改：`web/default/src/i18n/locales/ru.json`
- 修改：`web/default/src/i18n/locales/ja.json`
- 修改：`web/default/src/i18n/locales/vi.json`
- 不修改：`web/default/src/routeTree.gen.ts`（由任务 10 主代理 `bun run build` 生成并检查）
- 依赖：任务 6、任务 7、任务 8；必须在 users 与 usage-logs route search contract 完成后执行

- [ ] **步骤 1：创建 route 文件**

使用：

```ts
export const Route = createFileRoute('/_authenticated/admin-analytics/')({
  beforeLoad: () => {
    // role < ROLE.ADMIN redirect({ to: '/403' })
  },
  validateSearch: adminAnalyticsSearchSchema,
  component: AdminAnalyticsRoute,
})
```

Route 组件只负责 router search/navigate 与 `AdminAnalyticsPage` props 连接。

- [ ] **步骤 2：新增 sidebar 入口与 config**

管理员菜单新增 `Operations Analytics`；普通用户不可见。`URL_TO_CONFIG_MAP` 登记 `/admin-analytics`。

- [ ] **步骤 3：补 sidebar 测试**

`use-sidebar-config.test.ts` 覆盖：

- 管理员可见 `/admin-analytics`。
- 普通用户隐藏。
- URL config map 包含 `/admin-analytics`。

- [ ] **步骤 4：补 i18n**

新增所有用户可见文案和枚举 labelKey 到 `static-keys.ts` 与 6 个 locale。不要污染通用 key；使用 `adminAnalytics.*` 命名空间。

- [ ] **步骤 5：同步 i18n，记录 routeTree 生成待办**

任务 9 只运行会修改本任务拥有文件且非 build/typecheck/lint/format 的同步命令：

```bash
cd web/default && bun run i18n:sync
```

当前仓库没有独立的 TanStack Router 生成脚本；`routeTree.gen.ts` 由 `rsbuild.config.ts` 中的 `tanstackRouter` plugin 在 `bun run build` / dev server 期间生成。任务 9 不手写 `routeTree.gen.ts`，也不运行 build。`routeTree.gen.ts` 的生成和提交归入任务 10 主代理最终验证的 `bun run build` 后检查。

预期：

- i18n sync report 中新增 key 无 missing/extras；无新增 untranslated key。
- `routeTree.gen.ts` 暂不手写；任务 10 build 后必须包含 `/admin-analytics`。

---

## 任务 10：最终验证与修复

**文件：**
- 只修改前面任务引入的问题文件。
- 不处理 unrelated 工作区改动。

- [ ] **步骤 1：运行后端定向测试**

```bash
go test ./model ./controller -run 'AdminAnalytics|AdminLogs.*User|Log.*User|Subscription|Invitation' -count=1
go test ./model -run 'AdminAnalytics.*(SQLBuilder|LogDBBoolean|UsageEventTime|UsageParsesOther|CandidateLogLimit|Risk)' -count=1
```

预期：PASS。

- [ ] **步骤 2：运行前端纯函数测试**

```bash
cd web/default && bun test src/features/admin-analytics/lib/filters.test.ts src/features/admin-analytics/lib/chart-data.test.ts src/features/admin-analytics/lib/page-contract.test.ts src/features/admin-analytics/lib/drilldown.test.ts src/hooks/use-sidebar-config.test.ts
```

预期：PASS。

- [ ] **步骤 3：运行 typecheck / build / i18n sync**

```bash
cd web/default && bun run typecheck
cd web/default && bun run build
cd web/default && bun run i18n:sync
```

预期：全部 exit 0；`_sync-report.json` 无新增 missing/extras/untranslated。
`bun run build` 必须触发 TanStack Router plugin 更新 `web/default/src/routeTree.gen.ts`；build 后检查该文件包含 `/admin-analytics`，并将生成结果作为本功能相关文件纳入精确 pathspec。

- [ ] **步骤 4：检查工作区和精确提交路径**

运行：

```bash
git status --short
```

只允许提交本功能相关文件。禁止 `git add .`。提交前用精确 pathspec staging。

- [ ] **步骤 5：最终审查**

派发至少 3 个只读 review 子代理：后端数据/安全、前端 UX/合同、测试覆盖/验收。所有 reviewer PASS 后再提交。
