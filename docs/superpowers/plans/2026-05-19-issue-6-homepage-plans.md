# Issue #6 首页套餐介绍实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 将 default 首页默认落地页改为「超便宜低价高速的GPT」主页，并把原数字 Stats 区域替换为公开套餐介绍，同时保留购买流程完整套餐接口不回归。

**架构：** 后端新增公开只读 `GET /api/subscription/public/plans`，返回严格白名单 public DTO；现有受保护 `GET /api/subscription/plans` 保持完整购买 DTO。前端新增 public plan 类型、静默首页 API helper、套餐筛选纯函数和 `PlansPreview` 区块；默认首页使用 `PlansPreview` 替代 `Stats` 并移除 `Features`，Hero API demo 只保留 Chat / Responses。

**技术栈：** Go、Gin、GORM、React 19、TypeScript、TanStack Query、TanStack Router、i18next、Tailwind CSS、Bun。

---

## 0. 执行约束

- 实现根目录：`C:/Users/34404/source/repos/new-api`。
- 规格文件：`C:/Users/34404/source/repos/new-api/docs/superpowers/specs/2026-05-19-issue-6-homepage-plans-spec.md`。
- 实现前必须读取并遵守仓库根目录 `AGENTS.md` 与 `web/default/AGENTS.md`。
- 直接在主工作区 `main` 分支开发，不使用 `.worktrees/issue-6-homepage-plans`。
- 不删除、不格式化、不移动与 Issue #6 无关的未跟踪文件。
- 不清理 `.worktrees/api-help-followup` 或任何其他 worktree。
- 不修改受保护品牌、版权、归属信息，例如 `new-api`、`QuantumNous`、版权头。
- 子代理实现任务只做指定文件范围内的变更，不运行项目级 build/test/lint/typecheck/formatter；主控在合并后统一运行验证。
- 所有新增用户可见文案必须走 i18n。
- 主控在启动代码实现前必须先提交本计划与规格文件变更，并确认没有 Issue #6 以外的 tracked 或 staged diff；若存在无关未跟踪文件，只记录并保持不动，不为追求“干净”而删除或移动。
- 每个任务提交前必须执行 `git status --short`、`git diff -- <本任务文件>`、`git diff --cached -- <本任务文件>` 和 `git diff --cached --name-only`；只有确认目标文件和暂存区内没有无关 hunk 时才可 `git add <file>`，否则必须用 `git add -p` 只暂存 Issue #6 hunk，或先取消暂存无关 hunk。

## 1. 文件结构

### 后端

- 修改：`controller/subscription.go`
  - 新增 `PublicSubscriptionPlan` / `PublicSubscriptionPlanDTO`。
  - 新增 `GetPublicSubscriptionPlans`，复用现有过滤和排序，输出 public DTO。
  - 保持 `GetSubscriptionPlans` 返回完整 `SubscriptionPlanDTO` 不变。
- 修改：`router/api-router.go`
  - 新增公开路由 `GET /api/subscription/public/plans`。
  - 保留受保护 `GET /api/subscription/plans`、`GET /api/subscription/self` 和支付接口。
- 创建：`router/subscription_public_plans_route_test.go`
  - 路由级测试公开 endpoint 未登录可访问。
  - 断言受保护 endpoints 未登录仍 401。
  - 断言过滤、排序、public exact key set、内部字段不泄露。
  - 断言受保护购买 DTO 仍保留完整购买字段。

### 前端订阅类型与 API

- 修改：`web/default/src/features/subscriptions/types.ts`
  - 新增 `PublicSubscriptionPlan` / `PublicPlanRecord` 类型。
- 修改：`web/default/src/features/subscriptions/api.ts`
  - 新增 `getHomePublicPlansQuiet()`。
  - 保持 `getPublicPlans()` 请求 `/api/subscription/plans` 不变。
- 修改：`web/default/src/features/subscriptions/lib/format.ts`
  - 收窄或新增 `formatDuration` 的公共输入类型，允许 `PublicSubscriptionPlan` 无类型断言传入。
- 修改：`web/default/src/features/subscriptions/api.test.ts`
  - 增加 source-level 测试覆盖 quiet helper 路径、skip flags、`disableDuplicate`、catch 和普通 helper 未替换。

### 前端首页套餐模块

- 创建：`web/default/src/features/home/lib/plans-preview.ts`
  - `HOME_PLANS_PREVIEW_LIMIT`。
  - `selectHomePlanRecords()`。
  - `hasMoreHomePlans()`。
- 创建：`web/default/src/features/home/lib/plans-preview.test.ts`
  - 纯函数测试，覆盖异常输入、过滤、截断、顺序、more 判断。
- 创建：`web/default/src/features/home/components/sections/plans-preview.tsx`
  - 首页套餐介绍区块。
  - 使用 `getHomePublicPlansQuiet()`、格式化函数、`/wallet` CTA。
  - 失败/空结果最终返回 `null`。

### 前端首页默认落地页

- 修改：`web/default/src/features/home/index.tsx`
  - 默认组合中 `Stats` → `PlansPreview`。
  - 移除 `Features`。
- 修改：`web/default/src/features/home/components/index.ts`
  - 导出 `PlansPreview`。
  - 不再导出 `Stats` / `Features`。
- 修改：`web/default/src/features/home/components/sections/hero.tsx`
  - 主标题改为 `t('Affordable, low-cost, high-speed GPT')`。
  - 保留 `Browse Models` 和 `/pricing` 模型目录入口。
  - 保留已登录 `Go to Dashboard`。
- 修改：`web/default/src/features/home/components/hero-terminal-demo.tsx`
  - `API_DEMOS` 只保留 `gpt-chat` 和 `responses`。
  - 删除 Claude / Gemini 分支和无用 placeholder / tone 分支。
- 修改：`web/default/src/features/home/constants.ts`
  - 清理已无引用的 `DEFAULT_STATS` / `getDefaultStats` / `DEFAULT_FEATURES` / `getDefaultFeatures`。
  - 保留 `GATEWAY_FEATURES` / `getGatewayFeatures`。

### 前端 i18n 与首页契约测试

- 修改：`web/default/src/i18n/locales/en.json`
- 修改：`web/default/src/i18n/locales/zh.json`
- 修改：`web/default/src/i18n/locales/fr.json`
- 修改：`web/default/src/i18n/locales/ja.json`
- 修改：`web/default/src/i18n/locales/ru.json`
- 修改：`web/default/src/i18n/locales/vi.json`
  - 新增 `Affordable, low-cost, high-speed GPT`。
  - 新增 `Pick a plan that fits your GPT usage.`。
  - 新增 `View all plans`。
- 创建：`web/default/src/features/home/home-page-copy.test.ts`
  - 首页组合、Hero、API demo、PlansPreview wiring、i18n smoke。
- 保留：`web/default/src/features/home/quick-start-copy.test.ts`
  - 不修改或只在必要时追加不冲突断言。

---

## 2. 任务 A：后端公开套餐端点与测试

**文件：**
- 修改：`controller/subscription.go`
- 修改：`router/api-router.go`
- 创建：`router/subscription_public_plans_route_test.go`

### 步骤 A1：编写后端失败测试

- [ ] 新建 `router/subscription_public_plans_route_test.go`。

测试必须包含以下形态（可按实际测试初始化函数调整，但断言语义不变）：

```go
package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type subscriptionPlansPublicRouteResponse struct {
	Success bool `json:"success"`
	Data []struct {
		Plan map[string]any `json:"plan"`
	} `json:"data"`
}

func TestSubscriptionPlansPublicRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldGlobalApiRateLimitEnable := common.GlobalApiRateLimitEnable
	oldRedisEnabled := common.RedisEnabled
	common.GlobalApiRateLimitEnable = false
	common.RedisEnabled = false
	t.Cleanup(func() {
		common.GlobalApiRateLimitEnable = oldGlobalApiRateLimitEnable
		common.RedisEnabled = oldRedisEnabled
	})

	setupSubscriptionPublicPlansRouteTestDB(t)
	seedSubscriptionPublicPlanRouteTestPlans(t)

	engine := gin.New()
	engine.Use(sessions.Sessions("session", cookie.NewStore([]byte("secret"))))
	SetApiRouter(engine)

	publicRecorder := httptest.NewRecorder()
	publicReq := httptest.NewRequest(http.MethodGet, "/api/subscription/public/plans", nil)
	engine.ServeHTTP(publicRecorder, publicReq)
	require.Equal(t, http.StatusOK, publicRecorder.Code)

	var payload subscriptionPlansPublicRouteResponse
	require.NoError(t, common.Unmarshal(publicRecorder.Body.Bytes(), &payload))
	require.True(t, payload.Success)
	require.Len(t, payload.Data, 2)

	allowedPlanKeys := map[string]struct{}{
		"id": {},
		"title": {},
		"subtitle": {},
		"price_amount": {},
		"currency": {},
		"duration_unit": {},
		"duration_value": {},
		"custom_seconds": {},
		"monthly_token_limit": {},
		"concurrency_limit": {},
		"public_visible": {},
	}

	assert.Equal(t, "Public High", payload.Data[0].Plan["title"])
	assert.Equal(t, "Public Low", payload.Data[1].Plan["title"])
	for _, record := range payload.Data {
		require.Len(t, record.Plan, len(allowedPlanKeys))
		for key := range record.Plan {
			_, ok := allowedPlanKeys[key]
			assert.Truef(t, ok, "unexpected public plan key %q", key)
		}
	}

	body := publicRecorder.Body.String()
	assert.NotContains(t, body, "Hidden Plan")
	assert.NotContains(t, body, "Disabled Plan")
	assert.NotContains(t, body, "Trial Plan")
	assert.NotContains(t, body, "stripe_price_id")
	assert.NotContains(t, body, "creem_product_id")
	assert.NotContains(t, body, "max_purchase_per_user")
	assert.NotContains(t, body, "upgrade_group")
	assert.NotContains(t, body, "business_code")
	assert.NotContains(t, body, "reward_eligible")
	assert.NotContains(t, body, "total_amount")
	assert.NotContains(t, body, "sort_order")
	assert.NotContains(t, body, "is_trial")
	assert.NotContains(t, body, "created_at")

	protectedPlansRecorder := httptest.NewRecorder()
	protectedPlansReq := httptest.NewRequest(http.MethodGet, "/api/subscription/plans", nil)
	engine.ServeHTTP(protectedPlansRecorder, protectedPlansReq)
	require.Equal(t, http.StatusUnauthorized, protectedPlansRecorder.Code)

	selfRecorder := httptest.NewRecorder()
	selfReq := httptest.NewRequest(http.MethodGet, "/api/subscription/self", nil)
	engine.ServeHTTP(selfRecorder, selfReq)
	require.Equal(t, http.StatusUnauthorized, selfRecorder.Code)
}

func TestSubscriptionPlansProtectedDTOStillIncludesPurchaseFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupSubscriptionPublicPlansRouteTestDB(t)
	seedSubscriptionPublicPlanRouteTestPlans(t)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/subscription/plans", nil)
	controller.GetSubscriptionPlans(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	body := recorder.Body.String()
	assert.Contains(t, body, "stripe_price_id")
	assert.Contains(t, body, "creem_product_id")
	assert.Contains(t, body, "max_purchase_per_user")
	assert.Contains(t, body, "upgrade_group")
	assert.Contains(t, body, "business_code")
	assert.Contains(t, body, "reward_eligible")
}
```

- [ ] `setupSubscriptionPublicPlansRouteTestDB(t)` 必须使用独立内存 SQLite，保存并恢复 `model.DB`、`model.LOG_DB`、`common.UsingSQLite`、`common.UsingMySQL`、`common.UsingPostgreSQL`、`common.RedisEnabled` 等全局状态，`AutoMigrate(&model.SubscriptionPlan{})`，并在清理阶段关闭底层 `sql.DB`。
- [ ] `seedSubscriptionPublicPlanRouteTestPlans(t)` 必须创建公开启用普通套餐、隐藏套餐、禁用套餐、试用套餐；对 `Enabled=false`、`PublicVisible=false`、`IsTrial=true` 等受 GORM default/零值影响的字段，必须在 `Create` 后用 `Updates(map[string]interface{}{...})` 或 `Select` 强制落库。
- [ ] 测试必须解析 JSON 并断言每个 `data[*].plan` key 集严格等于公开字段集合；字符串 `NotContains` 只能作为额外泄露保护，不能替代 exact key set。
- [ ] 必须包含受保护购买 DTO 回归测试：可以直接调用 `controller.GetSubscriptionPlans`，也可以构造带 session cookie 和 `New-Api-User` header 的认证路由请求；断言完整购买字段仍存在。

### 步骤 A2：运行后端失败测试

- [ ] 运行：

```bash
go test ./router -run 'TestSubscriptionPlans(PublicRoute|ProtectedDTOStillIncludesPurchaseFields)' -count=1
```

- [ ] 预期：失败，原因是 `/api/subscription/public/plans` 当前未注册或返回 404。

### 步骤 A3：新增 public DTO 和 handler

- [ ] 在 `controller/subscription.go` 中新增：

```go
type PublicSubscriptionPlan struct {
	Id                int     `json:"id"`
	Title             string  `json:"title"`
	Subtitle          string  `json:"subtitle"`
	PriceAmount       float64 `json:"price_amount"`
	Currency          string  `json:"currency"`
	DurationUnit      string  `json:"duration_unit"`
	DurationValue     int     `json:"duration_value"`
	CustomSeconds     int64   `json:"custom_seconds"`
	MonthlyTokenLimit int64   `json:"monthly_token_limit"`
	ConcurrencyLimit  int     `json:"concurrency_limit"`
	PublicVisible     bool    `json:"public_visible"`
}

type PublicSubscriptionPlanDTO struct {
	Plan PublicSubscriptionPlan `json:"plan"`
}

func toPublicSubscriptionPlan(p model.SubscriptionPlan) PublicSubscriptionPlanDTO {
	return PublicSubscriptionPlanDTO{
		Plan: PublicSubscriptionPlan{
			Id:                p.Id,
			Title:             p.Title,
			Subtitle:          p.Subtitle,
			PriceAmount:       p.PriceAmount,
			Currency:          p.Currency,
			DurationUnit:      p.DurationUnit,
			DurationValue:     p.DurationValue,
			CustomSeconds:     p.CustomSeconds,
			MonthlyTokenLimit: p.MonthlyTokenLimit,
			ConcurrencyLimit:  p.ConcurrencyLimit,
			PublicVisible:     p.PublicVisible,
		},
	}
}
```

- [ ] 新增 handler：

```go
func GetPublicSubscriptionPlans(c *gin.Context) {
	var plans []model.SubscriptionPlan
	if err := model.DB.Where("enabled = ? AND public_visible = ? AND is_trial = ?", true, true, false).Order("sort_order desc, id desc").Find(&plans).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	result := make([]PublicSubscriptionPlanDTO, 0, len(plans))
	for _, p := range plans {
		result = append(result, toPublicSubscriptionPlan(p))
	}
	common.ApiSuccess(c, result)
}
```

- [ ] 不改 `GetSubscriptionPlans` 的返回 DTO；受保护购买场景必须继续返回完整 `SubscriptionPlanDTO`。

### 步骤 A4：注册公开路由

- [ ] 在 `router/api-router.go` 中订阅路由附近新增：

```go
subscriptionPublicRoute := apiRouter.Group("/subscription/public")
{
	subscriptionPublicRoute.GET("/plans", controller.GetPublicSubscriptionPlans)
}
```

- [ ] 保持现有受保护分组：

```go
subscriptionRoute := apiRouter.Group("/subscription")
subscriptionRoute.Use(middleware.UserAuth())
{
	subscriptionRoute.GET("/plans", controller.GetSubscriptionPlans)
	// ... self / pay routes unchanged
}
```

### 步骤 A5：运行后端测试验证通过

- [ ] 运行：

```bash
go test ./router -run 'TestSubscriptionPlans(PublicRoute|ProtectedDTOStillIncludesPurchaseFields)' -count=1
```

- [ ] 预期：通过。

### 步骤 A6：提交后端变更

- [ ] 提交范围：

```bash
git add controller/subscription.go router/api-router.go router/subscription_public_plans_route_test.go
git commit -m "feat(subscription): 新增公开套餐展示接口"
```

---

## 3. 任务 B：前端公开套餐类型、静默 API 和纯函数

**文件：**
- 修改：`web/default/src/features/subscriptions/types.ts`
- 修改：`web/default/src/features/subscriptions/api.ts`
- 修改：`web/default/src/features/subscriptions/lib/format.ts`
- 修改：`web/default/src/features/subscriptions/api.test.ts`
- 创建：`web/default/src/features/home/lib/plans-preview.ts`
- 创建：`web/default/src/features/home/lib/plans-preview.test.ts`

### 步骤 B1：编写 API helper source-level 失败测试

- [ ] 修改 `web/default/src/features/subscriptions/api.test.ts`，增加测试：

```ts
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { describe, test } from 'node:test'

const apiSource = readFileSync(new URL('./api.ts', import.meta.url), 'utf8')

function exportedFunctionSource(name: string): string {
  const match = apiSource.match(
    new RegExp(`export async function ${name}\\([^]*?\\n}`)
  )
  assert.ok(match, `missing exported function ${name}`)
  return match[0]
}

describe('home public plans API helper', () => {
  test('uses an isolated quiet public endpoint for the home page', () => {
    const source = exportedFunctionSource('getHomePublicPlansQuiet')
    assert.match(source, /\/api\/subscription\/public\/plans/)
    assert.match(source, /skipErrorHandler:\s*true/)
    assert.match(source, /skipBusinessError:\s*true/)
    assert.match(source, /disableDuplicate:\s*true/)
    assert.match(source, /catch\s*\{/)
    assert.match(source, /success:\s*false/)
    assert.match(source, /data:\s*\[\]/)
  })

  test('keeps the purchasable plans helper on the protected endpoint', () => {
    const source = exportedFunctionSource('getPublicPlans')
    assert.match(source, /\/api\/subscription\/plans/)
    assert.doesNotMatch(source, /\/api\/subscription\/public\/plans/)
  })
})
```

- [ ] 如果文件已有 imports / describe，合并而不是重复冲突。

### 步骤 B2：编写纯函数失败测试

- [ ] 创建 `web/default/src/features/home/lib/plans-preview.test.ts`：

```ts
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

type PublicPlanRecordForTest = {
  plan: {
    id: number
    title: string
    subtitle: string
    price_amount: number
    currency: string
    duration_unit: string
    duration_value: number
    custom_seconds: number
    monthly_token_limit: number
    concurrency_limit: number
    public_visible: boolean
  }
}

type PlansPreviewModule = {
  HOME_PLANS_PREVIEW_LIMIT?: number
  selectHomePlanRecords?: (records?: readonly unknown[]) => PublicPlanRecordForTest[]
  hasMoreHomePlans?: (records?: readonly unknown[]) => boolean
}

async function loadPlansPreviewModule(): Promise<PlansPreviewModule> {
  try {
    return (await import('./plans-preview')) as unknown as PlansPreviewModule
  } catch {
    return {}
  }
}

function record(id: number, publicVisible = true): PublicPlanRecordForTest {
  return {
    plan: {
      id,
      title: `Plan ${id}`,
      subtitle: '',
      price_amount: id,
      currency: 'CNY',
      duration_unit: 'month',
      duration_value: 1,
      custom_seconds: 0,
      monthly_token_limit: 1000,
      concurrency_limit: 1,
      public_visible: publicVisible,
    },
  }
}

describe('home plans preview selection', () => {
  test('keeps backend order and limits to three visible plans', async () => {
    const mod = await loadPlansPreviewModule()
    assert.equal(mod.HOME_PLANS_PREVIEW_LIMIT, 3)
    assert.equal(typeof mod.selectHomePlanRecords, 'function')

    const selected = mod.selectHomePlanRecords?.([
      record(5),
      record(4),
      record(3),
      record(2),
    ])

    assert.deepEqual(
      selected?.map((item) => item.plan.id),
      [5, 4, 3]
    )
  })

  test('filters hidden and malformed records without type assertions', async () => {
    const mod = await loadPlansPreviewModule()
    assert.equal(typeof mod.selectHomePlanRecords, 'function')

    const selected = mod.selectHomePlanRecords?.([
      record(3, false),
      null,
      undefined,
      {},
      record(2),
    ])

    assert.deepEqual(
      selected?.map((item) => item.plan.id),
      [2]
    )
  })

  test('detects when more visible plans are available after filtering', async () => {
    const mod = await loadPlansPreviewModule()
    assert.equal(typeof mod.hasMoreHomePlans, 'function')

    assert.equal(
      mod.hasMoreHomePlans?.([record(4), record(3), record(2), record(1)]),
      true
    )
    assert.equal(
      mod.hasMoreHomePlans?.([record(4), record(3), record(2), record(1, false)]),
      false
    )
  })
})
```

### 步骤 B3：运行前端失败测试

- [ ] 运行：

```bash
cd web/default
bunx tsx --test src/features/subscriptions/api.test.ts src/features/home/lib/plans-preview.test.ts
```

- [ ] 预期：失败，原因是 `getHomePublicPlansQuiet` 或 `plans-preview` 不存在。

### 步骤 B4：新增前端 public plan 类型

- [ ] 修改 `web/default/src/features/subscriptions/types.ts`：

```ts
export interface PublicSubscriptionPlan {
  id: number
  title: string
  subtitle: string
  price_amount: number
  currency: string
  duration_unit: SubscriptionPlan['duration_unit']
  duration_value: number
  custom_seconds: number
  monthly_token_limit: number
  concurrency_limit: number
  public_visible: boolean
}

export interface PublicPlanRecord {
  plan: PublicSubscriptionPlan
}
```

### 步骤 B5：收窄 `formatDuration` 输入类型

- [ ] 修改 `web/default/src/features/subscriptions/lib/format.ts`，将 `formatDuration` 参数从 `Partial<SubscriptionPlan>` 改成公共输入类型：

```ts
type DurationPlanLike = {
  duration_unit?: SubscriptionPlan['duration_unit'] | string
  duration_value?: number
  custom_seconds?: number
}

export function formatDuration(plan: DurationPlanLike, t: TranslationFn): string {
  const unit = plan?.duration_unit || 'month'
  const value = plan?.duration_value || 1
  const unitLabels: Record<string, string> = {
    year: t('years'),
    month: t('months'),
    day: t('days'),
    hour: t('hours'),
    custom: t('Custom (seconds)'),
  }
  if (unit === 'custom') {
    const seconds = plan?.custom_seconds || 0
    if (seconds >= 86400) return `${Math.floor(seconds / 86400)} ${t('days')}`
    if (seconds >= 3600) return `${Math.floor(seconds / 3600)} ${t('hours')}`
    return `${seconds} ${t('seconds')}`
  }
  return `${value} ${unitLabels[unit] || unit}`
}
```

- [ ] 确保现有 `SubscriptionPlan` 和新增 `PublicSubscriptionPlan` 都可传入，无需类型断言。

### 步骤 B6：新增静默 API helper

- [ ] 修改 `web/default/src/features/subscriptions/api.ts`，导入 `PublicPlanRecord` 类型并新增：

```ts
export async function getHomePublicPlansQuiet(): Promise<
  ApiResponse<PublicPlanRecord[]>
> {
  try {
    const res = await api.get('/api/subscription/public/plans', {
      skipErrorHandler: true,
      skipBusinessError: true,
      disableDuplicate: true,
    } as Record<string, unknown>)
    return res.data
  } catch {
    return { success: false, data: [] }
  }
}
```

- [ ] 保持 `getPublicPlans()` 请求 `/api/subscription/plans` 不变。

### 步骤 B7：新增套餐预览纯函数

- [ ] 创建 `web/default/src/features/home/lib/plans-preview.ts`：

```ts
/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import type { PublicPlanRecord } from '@/features/subscriptions/types'

export const HOME_PLANS_PREVIEW_LIMIT = 3

type MaybePublicPlanRecord = Partial<PublicPlanRecord> | null | undefined

function isVisiblePlanRecord(
  record: MaybePublicPlanRecord
): record is PublicPlanRecord {
  return Boolean(record?.plan && record.plan.public_visible !== false)
}

export function selectHomePlanRecords(
  records: readonly MaybePublicPlanRecord[] = []
): PublicPlanRecord[] {
  return records.filter(isVisiblePlanRecord).slice(0, HOME_PLANS_PREVIEW_LIMIT)
}

export function hasMoreHomePlans(
  records: readonly MaybePublicPlanRecord[] = []
): boolean {
  return records.filter(isVisiblePlanRecord).length > HOME_PLANS_PREVIEW_LIMIT
}
```

### 步骤 B8：运行任务 B 测试通过

- [ ] 运行：

```bash
cd web/default
bunx tsx --test src/features/subscriptions/api.test.ts src/features/home/lib/plans-preview.test.ts
```

- [ ] 预期：通过。

### 步骤 B9：提交任务 B

- [ ] 提交范围：

```bash
git add \
  web/default/src/features/subscriptions/types.ts \
  web/default/src/features/subscriptions/api.ts \
  web/default/src/features/subscriptions/lib/format.ts \
  web/default/src/features/subscriptions/api.test.ts \
  web/default/src/features/home/lib/plans-preview.ts \
  web/default/src/features/home/lib/plans-preview.test.ts
git commit -m "feat(web): 添加首页公开套餐数据契约"
```

---

## 4. 任务 C：前端首页套餐区块、Hero、API demo 与默认组合

**文件：**
- 创建：`web/default/src/features/home/components/sections/plans-preview.tsx`
- 修改：`web/default/src/features/home/index.tsx`
- 修改：`web/default/src/features/home/components/index.ts`
- 修改：`web/default/src/features/home/components/sections/hero.tsx`
- 修改：`web/default/src/features/home/components/hero-terminal-demo.tsx`
- 修改：`web/default/src/features/home/constants.ts`
- 创建：`web/default/src/features/home/home-page-copy.test.ts`

### 步骤 C1：编写首页契约失败测试

- [ ] 创建 `web/default/src/features/home/home-page-copy.test.ts`：

```ts
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { describe, test } from 'node:test'

function readSource(relativePath: string): string {
  return readFileSync(new URL(relativePath, import.meta.url), 'utf8')
}

function readOptionalSource(relativePath: string): string {
  try {
    return readSource(relativePath)
  } catch {
    return ''
  }
}

const homeSource = readSource('./index.tsx')
const heroSource = readSource('./components/sections/hero.tsx')
const demoSource = readSource('./components/hero-terminal-demo.tsx')
const componentsIndexSource = readSource('./components/index.ts')
const plansPreviewSource = readOptionalSource(
  './components/sections/plans-preview.tsx'
)

describe('Issue #6 home page copy and sections', () => {
  test('keeps custom home page content before the default landing sections', () => {
    assert.match(homeSource, /useHomePageContent/)
    assert.match(homeSource, /if \(content\)/)
    assert.match(homeSource, /isUrl \? \(/)
    assert.match(homeSource, /<iframe/)
    assert.match(homeSource, /Markdown/)
    assert.ok(
      homeSource.indexOf('if (content)') >= 0 &&
        homeSource.indexOf('if (content)') < homeSource.indexOf('<PlansPreview />')
    )
  })

  test('uses plans preview instead of default stats and features', () => {
    assert.match(homeSource, /<PlansPreview \/>/)
    assert.doesNotMatch(homeSource, /<Stats \/>/)
    assert.doesNotMatch(homeSource, /<Features \/>/)
    assert.match(componentsIndexSource, /PlansPreview/)
    assert.doesNotMatch(componentsIndexSource, /sections\/stats/)
    assert.doesNotMatch(componentsIndexSource, /sections\/features/)
  })

  test('keeps dashboard and model directory entry points while replacing the hero title', () => {
    assert.match(heroSource, /Affordable, low-cost, high-speed GPT/)
    assert.match(heroSource, /to='\/dashboard'|to="\/dashboard"/)
    assert.match(heroSource, /Go to Dashboard/)
    assert.match(heroSource, /Browse Models/)
    assert.doesNotMatch(heroSource, /View Pricing|Review model rates/)
  })

  test('limits terminal API demos to chat and responses', () => {
    assert.match(demoSource, /id: 'gpt-chat'/)
    assert.match(demoSource, /id: 'responses'/)
    assert.doesNotMatch(demoSource, /id: 'claude'|Claude message routed|<in>|<out>/)
    assert.doesNotMatch(demoSource, /id: 'gemini'|Gemini request served/)
  })

  test('plans preview uses quiet public data and wallet links', () => {
    assert.match(plansPreviewSource, /getHomePublicPlansQuiet/)
    assert.doesNotMatch(plansPreviewSource, /getPublicPlans\(/)
    assert.match(plansPreviewSource, /formatPlanPrice/)
    assert.match(plansPreviewSource, /formatDuration/)
    assert.match(plansPreviewSource, /formatTokenLimit/)
    assert.match(plansPreviewSource, /formatConcurrencyLimit/)
    assert.match(plansPreviewSource, /t\('Choose a plan'\)/)
    assert.match(plansPreviewSource, /t\('View all plans'\)/)
    assert.match(plansPreviewSource, /hasMoreHomePlans/)
    assert.match(plansPreviewSource, /to='\/wallet'|to="\/wallet"/)
    assert.doesNotMatch(plansPreviewSource, /View Pricing|Choose Plan|Review model rates/)
  })
})
```

- [ ] 该测试会先以断言失败方式失败：`plans-preview.tsx` 不存在时 `plansPreviewSource` 为 `''`，相关 `assert.match` 失败；首页未改时默认组合、Hero 或 API demo 断言失败。不得让测试因 ENOENT 或模块解析错误失败。

### 步骤 C2：运行首页契约失败测试

- [ ] 运行：

```bash
cd web/default
bunx tsx --test src/features/home/home-page-copy.test.ts
```

- [ ] 预期：失败。

### 步骤 C3：新增 `PlansPreview` 组件

- [ ] 创建 `web/default/src/features/home/components/sections/plans-preview.tsx`。
- [ ] 组件骨架：

```tsx
/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { Link } from '@tanstack/react-router'
import { Check, Sparkles } from 'lucide-react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { getHomePublicPlansQuiet } from '@/features/subscriptions/api'
import {
  formatConcurrencyLimit,
  formatDuration,
  formatPlanPrice,
  formatTokenLimit,
} from '@/features/subscriptions/lib'
import {
  hasMoreHomePlans,
  selectHomePlanRecords,
} from '../../lib/plans-preview'

export function PlansPreview() {
  const { t } = useTranslation()
  const plansQuery = useQuery({
    queryKey: ['home', 'subscription-public-plans'],
    queryFn: getHomePublicPlansQuiet,
    staleTime: 60_000,
  })

  if (plansQuery.isLoading) {
    return (
      <section className='border-border/40 bg-muted/10 relative z-10 border-y'>
        <div className='mx-auto max-w-6xl px-6 py-10 md:py-12'>
          <Skeleton className='h-8 w-48' />
        </div>
      </section>
    )
  }

  const records = plansQuery.data?.success ? (plansQuery.data.data ?? []) : []
  const plans = selectHomePlanRecords(records)
  if (plans.length === 0) return null

  const showAllPlansLink = hasMoreHomePlans(records)

  return (
    <section className='border-border/40 bg-muted/10 relative z-10 border-y'>
      <div className='mx-auto max-w-6xl px-6 py-10 md:py-12'>
        <div className='mb-8 flex flex-col gap-3 md:flex-row md:items-end md:justify-between'>
          <div>
            <p className='text-muted-foreground mb-3 text-xs font-medium tracking-widest uppercase'>
              {t('Subscription Plans')}
            </p>
            <h2 className='text-2xl font-bold tracking-tight md:text-3xl'>
              {t('Pick a plan that fits your GPT usage.')}
            </h2>
          </div>
          {showAllPlansLink ? (
            <Button variant='outline' render={<Link to='/wallet' />}>
              {t('View all plans')}
            </Button>
          ) : null}
        </div>
        <div className='grid gap-4 md:grid-cols-3'>
          {plans.map((record, index) => {
            const plan = record.plan
            const benefits = [
              `${t('Validity Period')}: ${formatDuration(plan, t)}`,
              `${t('Monthly Token Limit')}: ${formatTokenLimit(plan.monthly_token_limit, t)}`,
              `${t('Concurrency Limit')}: ${formatConcurrencyLimit(plan.concurrency_limit, t)}`,
            ]

            return (
              <Card key={plan.id} className={index === 0 ? 'border-primary/70 shadow-sm' : ''}>
                <CardContent className='flex h-full flex-col p-5'>
                  <div className='mb-3 flex items-start justify-between gap-3'>
                    <div>
                      <h3 className='font-semibold'>{plan.title || t('Subscription Plans')}</h3>
                      {plan.subtitle ? (
                        <p className='text-muted-foreground mt-1 text-sm'>{plan.subtitle}</p>
                      ) : null}
                    </div>
                    {index === 0 ? <Sparkles className='text-primary size-4' /> : null}
                  </div>
                  <div className='text-primary mb-4 text-2xl font-bold'>
                    {formatPlanPrice(plan.price_amount, plan.currency)}
                  </div>
                  <div className='flex-1 space-y-2'>
                    {benefits.map((label) => (
                      <div key={label} className='text-muted-foreground flex items-center gap-2 text-sm'>
                        <Check className='text-primary size-3.5 shrink-0' />
                        <span>{label}</span>
                      </div>
                    ))}
                  </div>
                  <Button className='mt-5 w-full' render={<Link to='/wallet' />}>
                    {t('Choose a plan')}
                  </Button>
                </CardContent>
              </Card>
            )
          })}
        </div>
      </div>
    </section>
  )
}
```

- [ ] 可以调整样式，但必须保留数据流、文案、`/wallet` 链接和格式化函数。

### 步骤 C4：更新首页组合和导出

- [ ] 修改 `web/default/src/features/home/index.tsx`：

```tsx
import { CTA, Hero, HowItWorks, PlansPreview } from './components'
```

默认分支：

```tsx
<Hero isAuthenticated={isAuthenticated} />
<PlansPreview />
<HowItWorks />
<CTA isAuthenticated={isAuthenticated} />
<Footer />
```

- [ ] 修改 `web/default/src/features/home/components/index.ts`：

```ts
export { CTA } from './sections/cta'
export { Hero } from './sections/hero'
export { HowItWorks } from './sections/how-it-works'
export { PlansPreview } from './sections/plans-preview'
```

### 步骤 C5：修改 Hero 主标题

- [ ] 修改 `web/default/src/features/home/components/sections/hero.tsx`：

```tsx
{t('Affordable, low-cost, high-speed GPT')}
```

- [ ] 移除旧标题中的 `Unified API Gateway for` / `All Your AI Models` 组合。
- [ ] 保留 `Browse Models` 和 `/pricing`。
- [ ] 保留 `Go to Dashboard` 和 `/dashboard`。

### 步骤 C6：缩减 Hero API demo

- [ ] 修改 `web/default/src/features/home/components/hero-terminal-demo.tsx`。
- [ ] `API_DEMOS` 只保留 `gpt-chat`、`responses`。
- [ ] 删除 `claude` / `gemini` demo。
- [ ] 删除 `truncateResponse()` 中 `claude` / `gemini`。
- [ ] 删除无用 `<in>` / `<out>` placeholder 分支和不再使用的 `AccentTone` 项。

### 步骤 C7：清理默认 stats/features 常量

- [ ] 修改 `web/default/src/features/home/constants.ts`。
- [ ] 删除 `DEFAULT_STATS` / `getDefaultStats`。
- [ ] 删除 `DEFAULT_FEATURES` / `getDefaultFeatures`。
- [ ] 保留 `GATEWAY_FEATURES` / `getGatewayFeatures`。
- [ ] 保留文件头版权。

### 步骤 C8：运行任务 C 前端测试通过

- [ ] 运行：

```bash
cd web/default
bunx tsx --test src/features/home/home-page-copy.test.ts src/features/home/quick-start-copy.test.ts
```

- [ ] 预期：通过。

### 步骤 C9：提交任务 C

- [ ] 提交范围：

```bash
git add \
  web/default/src/features/home/components/sections/plans-preview.tsx \
  web/default/src/features/home/index.tsx \
  web/default/src/features/home/components/index.ts \
  web/default/src/features/home/components/sections/hero.tsx \
  web/default/src/features/home/components/hero-terminal-demo.tsx \
  web/default/src/features/home/constants.ts \
  web/default/src/features/home/home-page-copy.test.ts
git commit -m "feat(web): 将首页统计区改为套餐介绍"
```

---

## 5. 任务 D：i18n 翻译与最终前端验证

**文件：**
- 修改：`web/default/src/i18n/locales/en.json`
- 修改：`web/default/src/i18n/locales/zh.json`
- 修改：`web/default/src/i18n/locales/fr.json`
- 修改：`web/default/src/i18n/locales/ja.json`
- 修改：`web/default/src/i18n/locales/ru.json`
- 修改：`web/default/src/i18n/locales/vi.json`
- 修改：`web/default/src/features/home/home-page-copy.test.ts`

### 步骤 D1：添加 i18n smoke 失败测试

- [ ] 在 `web/default/src/features/home/home-page-copy.test.ts` 中追加：

```ts
const localeFiles = ['en', 'zh', 'fr', 'ja', 'ru', 'vi'] as const
const requiredHomeKeys = [
  'Affordable, low-cost, high-speed GPT',
  'Pick a plan that fits your GPT usage.',
  'View all plans',
] as const

test('home page issue 6 translation keys are complete', () => {
  for (const locale of localeFiles) {
    const json = JSON.parse(
      readFileSync(new URL(`../../i18n/locales/${locale}.json`, import.meta.url), 'utf8')
    )
    for (const key of requiredHomeKeys) {
      assert.equal(typeof json.translation[key], 'string')
      assert.ok(json.translation[key].trim().length > 0)
    }
  }

  const zh = JSON.parse(
    readFileSync(new URL('../../i18n/locales/zh.json', import.meta.url), 'utf8')
  )
  assert.equal(
    zh.translation['Affordable, low-cost, high-speed GPT'],
    '超便宜低价高速的GPT'
  )
})
```

### 步骤 D2：运行 i18n smoke 失败测试

- [ ] 运行：

```bash
cd web/default
bunx tsx --test src/features/home/home-page-copy.test.ts
```

- [ ] 预期：失败，原因是新增 key 尚未补齐。

### 步骤 D3：补齐六种 locale

- [ ] 在六个 locale 的 `translation` 中添加：

`en.json`：

```json
"Affordable, low-cost, high-speed GPT": "Affordable, low-cost, high-speed GPT",
"Pick a plan that fits your GPT usage.": "Pick a plan that fits your GPT usage.",
"View all plans": "View all plans"
```

`zh.json`：

```json
"Affordable, low-cost, high-speed GPT": "超便宜低价高速的GPT",
"Pick a plan that fits your GPT usage.": "选择适合你 GPT 使用量的套餐。",
"View all plans": "查看全部套餐"
```

`fr.json`：

```json
"Affordable, low-cost, high-speed GPT": "GPT ultra abordable, économique et rapide",
"Pick a plan that fits your GPT usage.": "Choisissez un forfait adapté à votre usage de GPT.",
"View all plans": "Voir tous les forfaits"
```

`ja.json`：

```json
"Affordable, low-cost, high-speed GPT": "超低価格で高速なGPT",
"Pick a plan that fits your GPT usage.": "GPT の利用量に合ったプランを選択してください。",
"View all plans": "すべてのプランを見る"
```

`ru.json`：

```json
"Affordable, low-cost, high-speed GPT": "Очень доступный, недорогой и быстрый GPT",
"Pick a plan that fits your GPT usage.": "Выберите тариф, который подходит вашему использованию GPT.",
"View all plans": "Посмотреть все тарифы"
```

`vi.json`：

```json
"Affordable, low-cost, high-speed GPT": "GPT giá cực rẻ, chi phí thấp và tốc độ cao",
"Pick a plan that fits your GPT usage.": "Chọn gói phù hợp với nhu cầu sử dụng GPT của bạn.",
"View all plans": "Xem tất cả gói"
```

### 步骤 D4：运行 i18n sync

- [ ] 运行：

```bash
cd web/default
bun run i18n:sync
```

- [ ] 检查 locale JSON 仍包含新增 key，且 `zh` 标题值未被改坏。

### 步骤 D5：运行前端定向测试和 typecheck

- [ ] 运行：

```bash
cd web/default
bunx tsx --test \
  src/features/subscriptions/api.test.ts \
  src/features/home/home-page-copy.test.ts \
  src/features/home/lib/plans-preview.test.ts \
  src/features/home/quick-start-copy.test.ts
bun run typecheck
```

- [ ] 预期：全部通过。

### 步骤 D6：提交 i18n 与验证测试

- [ ] 提交范围：

```bash
git add \
  web/default/src/i18n/locales/en.json \
  web/default/src/i18n/locales/zh.json \
  web/default/src/i18n/locales/fr.json \
  web/default/src/i18n/locales/ja.json \
  web/default/src/i18n/locales/ru.json \
  web/default/src/i18n/locales/vi.json \
  web/default/src/features/home/home-page-copy.test.ts
git commit -m "feat(web): 补齐首页套餐文案翻译"
```

---

## 6. 最终验证

完成所有任务和审查修复后，主控逐条运行并记录每条命令的退出码；不得把多条验证命令拼成会掩盖前序失败的同一条 shell 链：

```bash
go test ./router -run 'TestSubscriptionPlans(PublicRoute|ProtectedDTOStillIncludesPurchaseFields)' -count=1
```

```bash
cd web/default && bunx tsx --test \
  src/features/subscriptions/api.test.ts \
  src/features/home/home-page-copy.test.ts \
  src/features/home/lib/plans-preview.test.ts \
  src/features/home/quick-start-copy.test.ts
```

```bash
cd web/default && bun run typecheck
```

若任何后端过滤 / DTO 测试放在 `controller` 包，还必须运行对应命令：

```bash
go test ./controller -run 'Test(GetPublicSubscriptionPlans|GetSubscriptionPlans)' -count=1
```

验收证据必须包括：

- 后端公开 endpoint 未登录 200。
- 后端受保护 endpoints 未登录 401。
- public DTO exact key set 通过。
- protected purchase DTO 字段保留。
- 首页不再渲染 Stats / Features。
- Hero 标题 key 与 zh 翻译正确。
- API demo 只剩 Chat / Responses。
- PlansPreview CTA 和 View all plans 都到 `/wallet`。
- Quick Start 回归测试通过。
- `bun run typecheck` 通过。

---

## 7. 子代理拆分建议

推荐执行顺序：

1. **后端代理**：任务 A，文件限定 `controller/subscription.go`、`router/api-router.go`、`router/subscription_public_plans_route_test.go`。可与任务 B 并行。
2. **前端数据代理**：任务 B，文件限定 `web/default/src/features/subscriptions/types.ts`、`api.ts`、`lib/format.ts`、`api.test.ts`、`web/default/src/features/home/lib/plans-preview.ts`、`plans-preview.test.ts`。可与任务 A 并行。
3. **前端首页代理**：任务 C，必须在任务 B 的契约文件落地后执行；文件限定 `web/default/src/features/home/components/sections/plans-preview.tsx`、`index.tsx`、`components/index.ts`、`hero.tsx`、`hero-terminal-demo.tsx`、`constants.ts`、`home-page-copy.test.ts`。
4. **i18n 代理**：任务 D，必须在任务 C 的 `home-page-copy.test.ts` 稳定后执行；若必须提前并行，只允许修改 locale 文件，i18n smoke 测试由主控或首页代理合并。

冲突控制：

- 前端首页代理依赖前端数据代理提供的 `PublicPlanRecord`、`getHomePublicPlansQuiet` 和 `selectHomePlanRecords` 契约；任务 C 不得与任务 B 竞争修改这些文件。
- i18n 代理不得与前端首页代理同时改写 `home-page-copy.test.ts`；如果发现该文件已有未合并改动，必须停止并通过 IRC 向主控确认。
- 所有代理不得运行项目级验证、lint 或 formatter；主控统一验证。
