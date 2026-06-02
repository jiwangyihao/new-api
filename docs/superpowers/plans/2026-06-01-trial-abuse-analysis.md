# 注册风控独立分析页实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 构建独立 `/trial-abuse` 注册风控分析页，按需手动查询已结束试用的高用量未转化风险，不进入 Admin Ops 实时刷新链路，也不自动处置账号。

**架构：** 后端新增只读 trial-abuse DTO、model 聚合与 service 风险分类，controller 只负责参数解析和管理员接口输出。前端新增 `features/trial-abuse` 页面模块与 TanStack Router 路由，React Query 使用已提交条件手动查询，sidebar 通过固定模块键 `admin.trial_abuse` 接入。风险口径严格排除试用未结束、历史有价权益、低用量与托管邀请渠道整体误伤。

**技术栈：** Go 1.22+、Gin、GORM、React 19、TypeScript、TanStack Router、React Query、i18next、Bun。

---

## 文件结构

### 后端

- 创建：`dto/trial_abuse.go`
  - 定义 API 响应 DTO、warning/partial section/reason 枚举、risk reason 枚举、criteria/overview/cluster/user 行结构。
- 创建：`model/trial_abuse.go`
  - 查询已结束试用、历史有价权益、试用窗口消费日志；实现 source 归一化、托管邀请识别、warning/partial 标记辅助逻辑。
- 创建：`model/trial_abuse_test.go`
  - 覆盖核心风险口径、source 归一化、历史有价权益排除、托管邀请渠道、日志 warning、IP 降级与纯函数 IP 分类。
- 创建：`service/trial_abuse.go`
  - 归一化查询参数、调用 model 聚合、返回 DTO。
- 创建：`service/trial_abuse_test.go`
  - 覆盖参数默认值、非法参数、时间窗口、limit clamp。
- 创建：`controller/trial_abuse.go`
  - 解析 query 参数，暴露 `GetTrialAbuseSummary`。
- 创建：`controller/trial_abuse_test.go`
  - 覆盖非法参数 400、管理员接口成功返回。
- 修改：`router/api-router.go`
  - 在管理员路由组注册 `GET /api/trial-abuse/summary`。
- 修改：`model/user.go`
  - 默认 sidebar 配置新增 `trial_abuse: true`。
- 修改：`model/user_sidebar_config_test.go`
  - 覆盖默认 sidebar 包含 `trial_abuse`。

### 前端

- 创建：`web/default/src/features/trial-abuse/types.ts`
  - 对齐后端 DTO 的 TypeScript 类型。
- 创建：`web/default/src/features/trial-abuse/api.ts`
  - `getTrialAbuseSummary(params)`。
- 创建：`web/default/src/features/trial-abuse/lib/filters.ts`
  - 默认筛选、校验、提交参数构造、日期工具、risk reason key 映射。
- 创建：`web/default/src/features/trial-abuse/lib/filters.test.ts`
  - 覆盖默认值、非法范围、参数构造、risk reason key。
- 创建：`web/default/src/features/trial-abuse/index.tsx`
  - 页面组件，手动查询、概览、分布、聚类、风险用户表、错误/空状态。
- 创建：`web/default/src/features/trial-abuse/trial-abuse-page.test.tsx`
  - 覆盖首屏不请求、点击查询才请求、修改草稿不触发、刷新使用已提交条件、partial 展示。
- 创建：`web/default/src/routes/_authenticated/trial-abuse/index.tsx`
  - TanStack Router 路由。
- 修改：`web/default/src/hooks/use-sidebar-config.ts`
  - `DEFAULT_SIDEBAR_MODULES.admin.trial_abuse = true`；`URL_TO_CONFIG_MAP['/trial-abuse'] = { section: 'admin', module: 'trial_abuse' }`。
- 修改：`web/default/src/hooks/use-sidebar-config.test.ts`
  - 覆盖 `/trial-abuse` 映射与模块开关。
- 修改：`web/default/src/hooks/use-sidebar-data.ts`
  - Admin 分组新增 `注册风控` 菜单项。
- 修改：`web/default/src/features/profile/components/sidebar-modules-card.tsx`
  - Admin sectionDefs 新增 `trial_abuse`。
- 修改：`web/default/src/features/system-settings/maintenance/config.ts`
  - `SIDEBAR_MODULES_DEFAULT.admin.trial_abuse = true`。
- 修改：`web/default/src/features/system-settings/maintenance/sidebar-modules-section.tsx`
  - Admin module metadata 新增 `trial_abuse`。
- 修改：`web/default/src/features/system-settings/maintenance/sidebar-config-removal.test.ts`
  - 覆盖全局默认配置包含 `trial_abuse`。
- 修改：`web/default/src/i18n/static-keys.ts`
  - 注册 trialAbuse 页面、表格、风险原因、warning、sidebar 文案 key。
- 修改：`web/default/src/i18n/locales/{en,zh,fr,ja,ru,vi}.json`
  - 补齐翻译。

---

## 任务 1：后端 DTO 与风险枚举

**文件：**
- 创建：`dto/trial_abuse.go`

- [ ] **步骤 1：编写 DTO 文件**

创建 `dto/trial_abuse.go`，定义稳定 JSON 契约：

```go
package dto

const (
	TrialAbuseRiskLevelHigh   = "high"
	TrialAbuseRiskLevelMedium = "medium"
	TrialAbuseRiskLevelLow    = "low"

	TrialAbuseWarningLogUnavailable          = "log_unavailable"
	TrialAbuseWarningRegistrationIPUnavailable = "registration_ip_unavailable"
	TrialAbuseWarningCandidateLimitExceeded  = "candidate_limit_exceeded"
	TrialAbuseWarningLogLimitExceeded        = "log_limit_exceeded"

	TrialAbuseSectionOverview         = "overview"
	TrialAbuseSectionUsageDistribution = "usage_distribution"
	TrialAbuseSectionRiskUsers        = "risk_users"
	TrialAbuseSectionRiskCounts       = "risk_counts"
	TrialAbuseSectionIPClusters       = "ip_clusters"
	TrialAbuseSectionInviterClusters  = "inviter_clusters"
	TrialAbuseSectionSelfInviteChains = "self_invite_chains"

	TrialAbuseRiskReasonSameRegistrationIPCluster     = "sameRegistrationIpCluster"
	TrialAbuseRiskReasonSameRegistrationIPSelfInviteChain = "sameRegistrationIpSelfInviteChain"
	TrialAbuseRiskReasonInviterLowPaidConversion      = "inviterLowPaidConversion"
	TrialAbuseRiskReasonManagedInviterDisplayOnly     = "managedInviterDisplayOnly"
	TrialAbuseRiskReasonRegistrationIPUnavailable     = "registrationIpUnavailable"
	TrialAbuseRiskReasonLogUnavailable                = "logUnavailable"
	TrialAbuseRiskReasonCandidateLimitExceeded        = "candidateLimitExceeded"
	TrialAbuseRiskReasonLogLimitExceeded              = "logLimitExceeded"
)

type TrialAbuseSummaryResponse struct {
	GeneratedAt       int64                           `json:"generated_at"`
	Criteria          TrialAbuseCriteria              `json:"criteria"`
	Warnings          []TrialAbuseWarning             `json:"warnings"`
	PartialSections   map[string][]string             `json:"partial_sections"`
	Overview          TrialAbuseOverview              `json:"overview"`
	RiskCounts        TrialAbuseRiskCounts            `json:"risk_counts"`
	UsageDistribution TrialAbuseUsageDistribution     `json:"usage_distribution"`
	IPClusters        []TrialAbuseIPCluster           `json:"ip_clusters"`
	InviterClusters   []TrialAbuseInviterCluster      `json:"inviter_clusters"`
	SelfInviteChains  []TrialAbuseSelfInviteChain     `json:"self_invite_chains"`
	RiskUsers         []TrialAbuseRiskUser            `json:"risk_users"`
}

type TrialAbuseCriteria struct {
	TrialEndStart  int64 `json:"trial_end_start"`
	TrialEndEnd    int64 `json:"trial_end_end"`
	RegisteredStart int64 `json:"registered_start,omitempty"`
	RegisteredEnd   int64 `json:"registered_end,omitempty"`
	SnapshotAt      int64 `json:"snapshot_at"`
	MinConsumeCount int   `json:"min_consume_count"`
	MinClusterSize  int   `json:"min_cluster_size"`
	RiskLimit       int   `json:"risk_limit"`
	GroupLimit      int   `json:"group_limit"`
}

type TrialAbuseWarning struct {
	Section string `json:"section"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

type TrialAbusePartial struct {
	Partial        bool     `json:"partial"`
	PartialReasons []string `json:"partial_reasons"`
}

type TrialAbuseOverview struct {
	TrialAbusePartial
	TotalTrialUsers             int `json:"total_trial_users"`
	ActiveTrialUsers            int `json:"active_trial_users"`
	ExpiredTrialUsers           int `json:"expired_trial_users"`
	ExpiredUnpaidTrialUsers     int `json:"expired_unpaid_trial_users"`
	HighUsageCandidateUsers     int `json:"high_usage_candidate_users"`
	RiskUserCount               int `json:"risk_user_count"`
	HighRiskUserCount           int `json:"high_risk_user_count"`
	MediumRiskUserCount         int `json:"medium_risk_user_count"`
	LowRiskUserCount            int `json:"low_risk_user_count"`
	ManagedInviterClusterCount  int `json:"managed_inviter_cluster_count"`
}

type TrialAbuseRiskCounts struct {
	TrialAbusePartial
	High   int `json:"high"`
	Medium int `json:"medium"`
	Low    int `json:"low"`
}

type TrialAbuseUsageDistribution struct {
	TrialAbusePartial
	SampleSize          int `json:"sample_size"`
	ZeroUsageCount      int `json:"zero_usage_count"`
	AboveThresholdCount int `json:"above_threshold_count"`
	P50                 int `json:"p50"`
	P75                 int `json:"p75"`
	P90                 int `json:"p90"`
	P95                 int `json:"p95"`
	P99                 int `json:"p99"`
}

type TrialAbuseRiskUser struct {
	TrialAbusePartial
	UserID                  int      `json:"user_id"`
	Username                string   `json:"username"`
	CreatedAt               int64    `json:"created_at"`
	TrialSource             string   `json:"trial_source"`
	TrialStartTime          int64    `json:"trial_start_time"`
	TrialEndTime            int64    `json:"trial_end_time"`
	InviterID               int      `json:"inviter_id"`
	InviterUsername         string   `json:"inviter_username"`
	ConsumeCount            int      `json:"consume_count"`
	UsedQuota               int64    `json:"used_quota"`
	MeteredTokens           int64    `json:"metered_tokens"`
	ObservedIP              string   `json:"observed_ip"`
	IPSource                string   `json:"ip_source"`
	RegistrationIPAvailable bool     `json:"registration_ip_available"`
	RiskLevel               string   `json:"risk_level"`
	RiskScore               int      `json:"risk_score"`
	RiskReasons             []string `json:"risk_reasons"`
	PaidEntitlementExcluded bool     `json:"paid_entitlement_excluded"`
}

type TrialAbuseIPCluster struct {
	TrialAbusePartial
	ObservedIP                 string `json:"observed_ip"`
	IPSource                   string `json:"ip_source"`
	RegistrationIPAvailable    bool   `json:"registration_ip_available"`
	CandidateCount             int    `json:"candidate_count"`
	ExpiredUnpaidTrialCount    int    `json:"expired_unpaid_trial_count"`
	PaidEntitlementCount       int    `json:"paid_entitlement_count"`
	TotalConsumeCount          int    `json:"total_consume_count"`
	SampleUserIDs              []int  `json:"sample_user_ids"`
}

type TrialAbuseInviterCluster struct {
	TrialAbusePartial
	InviterID                 int     `json:"inviter_id"`
	InviterUsername           string  `json:"inviter_username"`
	Managed                   bool    `json:"managed"`
	CandidateCount            int     `json:"candidate_count"`
	ExpiredTrialInviteeCount  int     `json:"expired_trial_invitee_count"`
	ExpiredUnpaidTrialCount   int     `json:"expired_unpaid_trial_count"`
	PaidEntitlementCount      int     `json:"paid_entitlement_count"`
	PaidConversionRate        float64 `json:"paid_conversion_rate"`
	TotalConsumeCount         int     `json:"total_consume_count"`
	RiskParticipation         string  `json:"risk_participation"`
	SampleUserIDs             []int   `json:"sample_user_ids"`
}

type TrialAbuseSelfInviteChain struct {
	TrialAbusePartial
	ChainID                 string                    `json:"chain_id"`
	RegistrationIPAvailable bool                      `json:"registration_ip_available"`
	RegistrationIP          string                    `json:"registration_ip"`
	CandidateCount          int                       `json:"candidate_count"`
	TotalConsumeCount       int                       `json:"total_consume_count"`
	Nodes                   []TrialAbuseSelfInviteNode `json:"nodes"`
}

type TrialAbuseSelfInviteNode struct {
	UserID    int    `json:"user_id"`
	Username  string `json:"username"`
	InviterID int    `json:"inviter_id"`
}
```

- [ ] **步骤 2：运行 gofmt**

运行：`gofmt -w dto/trial_abuse.go`
预期：无输出。

---

## 任务 2：后端模型层查询与风险分类

**文件：**
- 创建：`model/trial_abuse.go`
- 创建：`model/trial_abuse_test.go`

- [ ] **步骤 1：编写失败测试**

创建 `model/trial_abuse_test.go`，至少包含这些测试函数：

```go
func TestTrialAbuseSummaryExcludesActiveTrialPaidAndLowUsage(t *testing.T) { /* seed trial active, paid redemption/admin/order, low usage; assert risk empty */ }
func TestTrialAbuseSourceNormalizationUsesGrantReasonBeforeSource(t *testing.T) { /* grant_reason trial_code source order is still trial source; paid source only when grant_reason empty */ }
func TestTrialAbuseInviterConversionUsesExpiredInviteesDenominator(t *testing.T) { /* denominator includes paid invitees, not just candidates */ }
func TestTrialAbuseManagedInviterDoesNotPromoteToMedium(t *testing.T) { /* inviter role admin -> cluster display only, no medium/high */ }
func TestTrialAbuseRegistrationIPUnavailableDisablesStrongIPRisk(t *testing.T) { /* production path no registration IP -> registration_ip_unavailable warning */ }
func TestClassifyTrialAbuseWithRegistrationIPAvailable(t *testing.T) { /* pure function input -> same IP/self invite high risk */ }
func TestTrialAbusePartialWarningsForLimits(t *testing.T) { /* force low internal limit via test hook if practical; assert partial_sections */ }
```

测试要使用真实 GORM 测试数据库和 `model.LOG_DB`，不要 mock 数据库。IP 可用场景可测试纯风险分类函数，生产查询不新增注册 IP 字段。

- [ ] **步骤 2：运行测试确认失败**

运行：`go test -p 1 ./model -run 'TrialAbuse' -count=1`
预期：编译失败或测试失败，因为实现不存在。

- [ ] **步骤 3：实现模型层**

创建 `model/trial_abuse.go`，实现：

```go
const (
	trialAbuseDefaultWindowSeconds = int64(14 * 24 * 3600)
	trialAbuseMaxWindowSeconds     = int64(90 * 24 * 3600)
	trialAbuseCandidateLimit       = 5000
	trialAbuseLogScanLimit         = 200000
	trialAbuseLogAggregateUserLimit = 5000
)

type TrialAbuseQuery struct { /* 与 dto criteria 对齐 */ }

type TrialAbuseDataset struct { /* 汇总 DTO 所需的中间结构 */ }

func GetTrialAbuseSummary(query TrialAbuseQuery) (*dto.TrialAbuseSummaryResponse, error) { ... }
func normalizeTrialAbuseSource(grantReason string, source string) string { ... }
func isTrialAbusePaidEntitlementSource(source string) bool { ... }
func buildTrialAbusePartialSections(warnings []dto.TrialAbuseWarning) map[string][]string { ... }
func classifyTrialAbuseRisks(input trialAbuseClassificationInput) trialAbuseClassificationOutput { ... }
```

实现要点：

1. 候选试用订阅：`grant_reason IN ('trial_code','invite_trial')` 且 `end_time` 在观察窗口内，`end_time <= snapshot_at`。
2. 历史有价权益：同 user 任意 `subscription_plans.price_amount > 0` 且归一化 source 在 `order/redemption/admin/monthly_invite_entitlement`，即排除。
3. 用量：对候选 user 批量聚合 `LOG_DB`，条件为 `type=LogTypeConsume`、`created_at BETWEEN trial.start_time AND trial.end_time`、`user_id IN ?`。聚合 `COUNT(*)`、`SUM(quota)`、`SUM(metered_tokens)`。
4. 日志不可用：`common.LogConsumeEnabled == false` 或 LOG_DB 查询失败时返回 warning；普通用户无日志不是 log_unavailable。
5. 注册 IP：生产路径 `RegistrationIPAvailable=false`，返回 `registration_ip_unavailable` warning；消费日志 IP 只可作为 observed_ip 弱展示，不参与 high/medium 强规则。
6. managed inviter：inviter `Role == common.RoleAdminUser` 或 `common.RoleRootUser` 视为 managed；后续 managed 配置不做本期存储。
7. 邀请人转化：分母为观察窗口内已结束邀请试用 invitee 总数，分子为分母集合内历史有价权益用户数。
8. 部分结果：warning 带 section/reason/message，并填充 DTO partial 字段与 `PartialSections`。

- [ ] **步骤 4：运行模型测试**

运行：`go test -p 1 ./model -run 'TrialAbuse' -count=1`
预期：PASS。

---

## 任务 3：后端 service/controller/router

**文件：**
- 创建：`service/trial_abuse.go`
- 创建：`service/trial_abuse_test.go`
- 创建：`controller/trial_abuse.go`
- 创建：`controller/trial_abuse_test.go`
- 修改：`router/api-router.go`

- [ ] **步骤 1：编写 service/controller 失败测试**

`service/trial_abuse_test.go` 覆盖：

```go
func TestNormalizeTrialAbuseQueryDefaults(t *testing.T) { ... }
func TestNormalizeTrialAbuseQueryRejectsTooWideWindow(t *testing.T) { ... }
func TestNormalizeTrialAbuseQueryClampsLimits(t *testing.T) { ... }
```

`controller/trial_abuse_test.go` 覆盖：

```go
func TestGetTrialAbuseSummaryRejectsInvalidWindow(t *testing.T) { ... }
func TestGetTrialAbuseSummaryRequiresAdmin(t *testing.T) { ... }
```

- [ ] **步骤 2：运行测试确认失败**

运行：`go test -p 1 ./service ./controller -run 'TrialAbuse' -count=1`
预期：失败。

- [ ] **步骤 3：实现 service/controller**

`service/trial_abuse.go`：

```go
type TrialAbuseSummaryQuery struct { ... }

func GetTrialAbuseSummary(ctx context.Context, query TrialAbuseSummaryQuery) (*dto.TrialAbuseSummaryResponse, error) {
    normalized, err := normalizeTrialAbuseSummaryQuery(query, time.Now().Unix())
    if err != nil { return nil, err }
    return model.GetTrialAbuseSummary(model.TrialAbuseQuery{...})
}
```

`controller/trial_abuse.go`：

```go
func GetTrialAbuseSummary(c *gin.Context) {
    query, err := parseTrialAbuseSummaryQuery(c)
    if err != nil { common.ApiError(c, err); return }
    data, err := service.GetTrialAbuseSummary(c.Request.Context(), query)
    if err != nil { common.ApiError(c, err); return }
    common.ApiSuccess(c, data)
}
```

`router/api-router.go`：在管理员认证组中新增：

```go
adminRoute.GET("/trial-abuse/summary", controller.GetTrialAbuseSummary)
```

实际变量名以现有 router 文件为准，必须复用 `middleware.AdminAuth()` 的管理员路由。

- [ ] **步骤 4：运行后端目标测试**

运行：`go test -p 1 ./model ./service ./controller -run 'TrialAbuse' -count=1`
预期：PASS。

---

## 任务 4：Sidebar 配置接入

**文件：**
- 修改：`model/user.go`
- 修改：`model/user_sidebar_config_test.go`
- 修改：`web/default/src/hooks/use-sidebar-config.ts`
- 修改：`web/default/src/hooks/use-sidebar-config.test.ts`
- 修改：`web/default/src/hooks/use-sidebar-data.ts`
- 修改：`web/default/src/features/profile/components/sidebar-modules-card.tsx`
- 修改：`web/default/src/features/system-settings/maintenance/config.ts`
- 修改：`web/default/src/features/system-settings/maintenance/sidebar-modules-section.tsx`
- 修改：`web/default/src/features/system-settings/maintenance/sidebar-config-removal.test.ts`

- [ ] **步骤 1：编写/更新失败测试**

后端测试新增断言：

```go
assert.Contains(t, config, `"trial_abuse":true`)
```

前端测试新增：

```ts
expect(isSidebarUrlVisible('/trial-abuse', adminConfig, userConfig)).toBe(true)
expect(SIDEBAR_MODULES_DEFAULT.admin?.trial_abuse).toBe(true)
```

- [ ] **步骤 2：运行目标测试确认失败**

运行：

```bash
go test -p 1 ./model -run 'SidebarConfig|DefaultSidebar' -count=1
```

前端运行：

```bash
bun test ./src/hooks/use-sidebar-config.test.ts ./src/features/system-settings/maintenance/sidebar-config-removal.test.ts
```

预期：失败。

- [ ] **步骤 3：实现 sidebar 配置**

修改点：

- `model/user.go`：admin/root 默认配置加入 `trial_abuse: true`。
- `use-sidebar-config.ts`：`DEFAULT_SIDEBAR_MODULES.admin.trial_abuse = true`，`URL_TO_CONFIG_MAP['/trial-abuse'] = { section: 'admin', module: 'trial_abuse' }`。
- `use-sidebar-data.ts`：Admin 分组新增菜单项，路径 `/trial-abuse`，title `t('trialAbuse.title')`。
- `sidebar-modules-card.tsx`：个人侧边栏配置 Admin section 新增 module `trial_abuse`。
- `system-settings/maintenance/config.ts`：`SIDEBAR_MODULES_DEFAULT.admin.trial_abuse = true`。
- `sidebar-modules-section.tsx`：Admin module metadata 新增 title `t('trialAbuse.title')` 和 description `t('trialAbuse.description')`。

- [ ] **步骤 4：运行 sidebar 相关测试**

运行同步骤 2。
预期：PASS。

---

## 任务 5：前端 API、类型、筛选逻辑

**文件：**
- 创建：`web/default/src/features/trial-abuse/types.ts`
- 创建：`web/default/src/features/trial-abuse/api.ts`
- 创建：`web/default/src/features/trial-abuse/lib/filters.ts`
- 创建：`web/default/src/features/trial-abuse/lib/filters.test.ts`

- [ ] **步骤 1：编写失败测试**

`filters.test.ts` 覆盖：

```ts
it('builds default draft without requesting immediately', () => { ... })
it('rejects trial end ranges wider than 90 days', () => { ... })
it('clamps risk and group limits through API contract helpers', () => { ... })
it('maps risk reason keys to trialAbuse i18n keys', () => { ... })
it('keeps submitted filters separate from draft filters', () => { ... })
```

- [ ] **步骤 2：运行测试确认失败**

运行：`bun test ./src/features/trial-abuse/lib/filters.test.ts`
预期：失败。

- [ ] **步骤 3：实现类型/API/filters**

`types.ts` 对齐 `dto/trial_abuse.go` JSON 字段，使用具体类型，不使用 `any`。

`api.ts`：

```ts
export async function getTrialAbuseSummary(params: TrialAbuseSummaryParams): Promise<ApiResponse<TrialAbuseSummaryResponse>> {
  const res = await api.get('/api/trial-abuse/summary', { params, disableDuplicate: true } as Record<string, unknown>)
  return res.data
}
```

`filters.ts`：提供默认值、校验函数、参数构造、risk reason i18n key：

```ts
export const TRIAL_ABUSE_RISK_REASON_KEYS = [
  'sameRegistrationIpCluster',
  'sameRegistrationIpSelfInviteChain',
  'inviterLowPaidConversion',
  'managedInviterDisplayOnly',
  'registrationIpUnavailable',
  'logUnavailable',
  'candidateLimitExceeded',
  'logLimitExceeded',
] as const

export function trialAbuseRiskReasonI18nKey(key: TrialAbuseRiskReasonKey): string {
  return `trialAbuse.riskReason.${key}`
}
```

- [ ] **步骤 4：运行 filters 测试**

运行：`bun test ./src/features/trial-abuse/lib/filters.test.ts`
预期：PASS。

---

## 任务 6：前端页面与路由

**文件：**
- 创建：`web/default/src/features/trial-abuse/index.tsx`
- 创建：`web/default/src/features/trial-abuse/trial-abuse-page.test.tsx`
- 创建：`web/default/src/routes/_authenticated/trial-abuse/index.tsx`

- [ ] **步骤 1：编写页面失败测试**

测试行为：

```ts
it('does not request summary before submit', async () => { ... })
it('requests summary when query button is clicked', async () => { ... })
it('does not request when draft filters change', async () => { ... })
it('refreshes with last submitted filters', async () => { ... })
it('renders partial warnings on affected sections', async () => { ... })
it('shows empty state when risk users and clusters are empty', async () => { ... })
```

- [ ] **步骤 2：运行测试确认失败**

运行：`bun test ./src/features/trial-abuse/trial-abuse-page.test.tsx`
预期：失败。

- [ ] **步骤 3：实现页面**

页面要求：

- `useQuery` 使用 `submittedCriteria`；无提交时 `enabled: false`。
- `refetchInterval: false`，`refetchOnWindowFocus: false`，`refetchOnReconnect: false`。
- queryKey：`['trial-abuse', 'summary', submittedCriteria]`。
- 表单字段：试用结束范围、可选注册范围、minConsumeCount、minClusterSize、查询、重置。
- 查询中禁用按钮。
- 错误态可重试。
- 无数据空态。
- partial warning 根据 `partial_sections` 和各区块 `partial/partial_reasons` 展示。
- 风险原因通过 `trialAbuse.riskReason.<key>` 翻译。

路由文件：

```tsx
import { createFileRoute } from '@tanstack/react-router'
import { TrialAbusePage } from '@/features/trial-abuse'

export const Route = createFileRoute('/_authenticated/trial-abuse/')({
  component: TrialAbusePage,
})
```

- [ ] **步骤 4：运行页面测试**

运行：`bun test ./src/features/trial-abuse/trial-abuse-page.test.tsx`
预期：PASS。

---

## 任务 7：i18n 翻译与静态 key

**文件：**
- 修改：`web/default/src/i18n/static-keys.ts`
- 修改：`web/default/src/i18n/locales/en.json`
- 修改：`web/default/src/i18n/locales/zh.json`
- 修改：`web/default/src/i18n/locales/fr.json`
- 修改：`web/default/src/i18n/locales/ja.json`
- 修改：`web/default/src/i18n/locales/ru.json`
- 修改：`web/default/src/i18n/locales/vi.json`
- 可修改：现有 i18n key 测试文件或新增 `web/default/src/features/trial-abuse/trial-abuse-i18n.test.ts`

- [ ] **步骤 1：添加 i18n 覆盖测试**

测试确保 `TRIAL_ABUSE_RISK_REASON_KEYS` 在所有 locale 中都有 `trialAbuse.riskReason.<key>`。

- [ ] **步骤 2：运行测试确认失败**

运行：`bun test ./src/features/trial-abuse/trial-abuse-i18n.test.ts`
预期：失败。

- [ ] **步骤 3：补齐 static keys 与 locale**

必须包含：

- `trialAbuse.title`
- `trialAbuse.description`
- `trialAbuse.readOnlyNotice`
- `trialAbuse.query`
- `trialAbuse.reset`
- `trialAbuse.refreshCurrentResult`
- `trialAbuse.empty.title`
- `trialAbuse.empty.description`
- `trialAbuse.partialResult`
- `trialAbuse.riskLevel.high|medium|low`
- `trialAbuse.riskReason.sameRegistrationIpCluster`
- `trialAbuse.riskReason.sameRegistrationIpSelfInviteChain`
- `trialAbuse.riskReason.inviterLowPaidConversion`
- `trialAbuse.riskReason.managedInviterDisplayOnly`
- `trialAbuse.riskReason.registrationIpUnavailable`
- `trialAbuse.riskReason.logUnavailable`
- `trialAbuse.riskReason.candidateLimitExceeded`
- `trialAbuse.riskReason.logLimitExceeded`

- [ ] **步骤 4：运行 i18n sync 与测试**

运行：

```bash
bun run i18n:sync
bun test ./src/features/trial-abuse/trial-abuse-i18n.test.ts
```

预期：PASS，i18n sync 无需生成无关变更。

---

## 任务 8：端到端验证与最终审查

**文件：**
- `dto/trial_abuse.go`
- `model/trial_abuse.go`
- `model/trial_abuse_test.go`
- `service/trial_abuse.go`
- `service/trial_abuse_test.go`
- `controller/trial_abuse.go`
- `controller/trial_abuse_test.go`
- `router/api-router.go`
- `model/user.go`
- `model/user_sidebar_config_test.go`
- `web/default/src/features/trial-abuse/types.ts`
- `web/default/src/features/trial-abuse/api.ts`
- `web/default/src/features/trial-abuse/lib/filters.ts`
- `web/default/src/features/trial-abuse/lib/filters.test.ts`
- `web/default/src/features/trial-abuse/index.tsx`
- `web/default/src/features/trial-abuse/trial-abuse-page.test.tsx`
- `web/default/src/features/trial-abuse/trial-abuse-i18n.test.ts`
- `web/default/src/routes/_authenticated/trial-abuse/index.tsx`
- sidebar 与 i18n 修改文件

- [ ] **步骤 1：运行后端目标测试**

运行：

```bash
go test -p 1 ./model ./service ./controller -run 'TrialAbuse|SidebarConfig|DefaultSidebar' -count=1
```

预期：PASS。

- [ ] **步骤 2：运行前端目标测试**

在 `web/default` 目录运行：

```bash
bun test ./src/features/trial-abuse/lib/filters.test.ts ./src/features/trial-abuse/trial-abuse-page.test.tsx ./src/features/trial-abuse/trial-abuse-i18n.test.ts ./src/hooks/use-sidebar-config.test.ts ./src/features/system-settings/maintenance/sidebar-config-removal.test.ts
```

预期：PASS。

- [ ] **步骤 3：运行 typecheck 与 i18n sync**

在 `web/default` 目录运行：

```bash
bun run typecheck
bun run i18n:sync
```

预期：PASS，无错误。

- [ ] **步骤 4：运行格式/空白检查**

运行：`git diff --check`
预期：无输出。

- [ ] **步骤 5：请求代码审查**

并发派发至少 3 个只读 reviewer：后端风险口径、前端页面与手动查询、产品/运营口径。所有 Critical/Important 必须修复后再完成。
