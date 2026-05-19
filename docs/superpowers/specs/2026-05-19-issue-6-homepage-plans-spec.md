# Issue #6 首页文案与套餐介绍规格

> 面向 AI 代理的工作者：本规格用于处理 GitHub fork 仓库 `jiwangyihao/new-api` 的 Issue #6「主页内容：更新标题、API 展示和功能模块文案」。实现前必须读取仓库根目录 `AGENTS.md` 与 `web/default/AGENTS.md`，并遵守 Go + Gin、React 19、TypeScript、TanStack Router、TanStack Query、i18n、Base UI / Tailwind CSS、Bun 以及项目受保护标识约束。

**工作区要求：** 本规格位于主工作区 `C:/Users/34404/source/repos/new-api/docs/superpowers/specs/2026-05-19-issue-6-homepage-plans-spec.md`。后续实现必须直接在主工作区当前 `main` 分支进行；不要在 `.worktrees/issue-6-homepage-plans` 中开发，不要修改或清理其他 worktree，不要删除或改动与 Issue #6 无关的未跟踪文件。

**目标：** 将默认首页从通用模板展示切换为更贴合「赔钱GPT」的落地页：主标题改为「超便宜低价高速的GPT」，API 展示只保留 chat / responses，原数字 Stats 区域改为公开套餐介绍，删除默认「核心功能」模板模块，并保留与 Quick Start 一致的三步快速上手。

**架构：** 默认首页仍通过 `Home` 组合各区块；当管理员配置了自定义首页内容时，继续优先展示自定义 Markdown / iframe，不受本规格影响。Stats 区域替换为 `PlansPreview`，通过公开的 `GET /api/subscription/plans` 读取公开套餐。后端将该列表接口改为公开读取，但必须使用公开字段白名单 DTO，只返回首页展示所需字段；购买、支付、用户订阅和管理员完整配置仍保持受保护。

**技术栈：** Go、Gin、GORM、React 19、TypeScript、TanStack Router、TanStack Query、i18next、Base UI / shadcn 风格组件、Tailwind CSS、Bun。

---

## 1. 背景

Issue #6 要求默认网站首页清理通用模板内容，并替换为更贴合当前站点定位的文案。原始验收项包括：

- 首页主标题替换为「超便宜低价高速的GPT」。
- 保留「前往仪表盘」入口。
- API 展示区域只保留 `chat` 和 `responses`。
- 删除默认数字展示，例如 `50+`、`100+`、`50+`。
- 删除默认「核心功能」等模板化模块。
- 可以保留「三步快速上手」，但内容需要和 Quick Start 保持一致：
  - 第一步：创建 API
  - 第二步：游乐场试用（适配 OpenCode）
  - 第三步：选择套餐

用户进一步明确：**希望把首页数字部分 Stats 改成套餐介绍**。因此本规格不再把 Stats 区域直接移除，而是把该位置替换为套餐介绍模块；默认数字和计数动画必须消失。

## 2. 当前代码基线

已确认的关键文件和现有能力：

- `router/api-router.go`
  - `/api/subscription/plans` 当前位于 `subscriptionRoute.Use(middleware.UserAuth())` 下。
  - 首页未登录用户调用该接口会收到 401，不符合公开首页套餐介绍需求。
- `controller/subscription.go`
  - `GetSubscriptionPlans` 已只查询 `enabled = true AND public_visible = true AND is_trial = false`，并按 `sort_order desc, id desc` 返回公开、启用、非试用套餐。
  - 现有 `SubscriptionPlanDTO` 直接返回完整 `model.SubscriptionPlan`，包含 `stripe_price_id`、`creem_product_id`、`max_purchase_per_user`、`upgrade_group`、`business_code`、`reward_eligible` 等购买或管理配置字段。公开接口不能直接暴露完整模型。
- `model/subscription.go`
  - `SubscriptionPlan` 的首页展示必需字段包括：`id`、`title`、`subtitle`、`price_amount`、`currency`、`duration_unit`、`duration_value`、`custom_seconds`、`monthly_token_limit`、`concurrency_limit`、`public_visible`。
  - 公开响应不得包含支付渠道、购买限制、用户分组升级、业务编码、奖励资格、创建更新时间等内部配置字段。
- `web/default/src/lib/api.ts`
  - Axios 全局错误处理对 401 会 toast `Session expired!` 并清理 auth store，对其他 HTTP 错误也会 toast。
  - 仅设置 React Query `throwOnError: false` 不能抑制 Axios 拦截器副作用。
- `web/default/src/main.tsx`
  - QueryCache `onError` 对 401 / 500 也会触发全局处理；首页套餐查询若让错误继续抛给 React Query，仍可能打扰访客。
- `web/default/src/features/home/index.tsx`
  - 默认首页在无自定义首页内容时渲染：`Hero`、`Stats`、`Features`、`HowItWorks`、`CTA`、`Footer`。
  - 自定义首页内容来自 `useHomePageContent()`；有自定义内容时渲染 Markdown 或 iframe，并跳过默认落地页。
- `web/default/src/features/home/components/sections/hero.tsx`
  - 当前标题为 `Unified API Gateway for` + `All Your AI Models`。
  - 已登录用户按钮为 `Go to Dashboard`，跳转 `/dashboard`。
  - 未登录用户主按钮为 `Get Started`，次按钮为 `Browse Models`，跳转 `/pricing`。
- `web/default/src/components/layout/components/public-header.tsx`
  - 顶部导航中已登录用户也有 `Go to Dashboard`，跳转 `/dashboard`。
- `web/default/src/features/home/components/hero-terminal-demo.tsx`
  - 当前 `API_DEMOS` 包含 `Chat`、`Responses`、`Claude`、`Gemini` 四项。
  - 自动轮播基于 `API_DEMOS.length`，删除 demo 后无需额外配置。
- `web/default/src/features/home/components/sections/stats.tsx`
  - 当前渲染 `50+`、`100+`、`50+`、`10+` 等默认数字和说明。
  - 内含 `Counter` 动画和 `IntersectionObserver`，套餐介绍不需要保留。
- `web/default/src/features/home/components/sections/features.tsx`
  - 当前渲染 `Core Features`、`Built for developers,`、`designed for scale` 以及默认功能卡片。
  - Issue 要求删除此类模板模块。
- `web/default/src/features/home/components/sections/how-it-works.tsx`
  - 当前三步为 `Create API`、`Try Playground`、`Choose a plan`。
  - 描述包含 `OpenCode-ready API help`，与 Issue #3 Quick Start 文案保持一致。
- `web/default/src/features/home/quick-start-copy.test.ts`
  - 已有 source-level 测试验证首页和 dashboard Quick Start 顺序一致。
  - 已断言首页 / CTA / dashboard 的模型目录入口不得出现 `View Pricing|Review model rates`，并应保留 `Model Directory|Browse Models|Browse available models`。
- `web/default/src/features/subscriptions/api.ts`
  - 已有 `getPublicPlans()`，请求 `/api/subscription/plans`，返回 `PlanRecord[]`。
  - 当前 `getPublicPlans()` 不支持静默配置，首页失败请求会走全局错误处理。
- `web/default/src/features/subscriptions/types.ts`
  - `SubscriptionPlan` 包含完整管理字段；首页可继续在前端类型层复用该类型，但后端公开响应必须是白名单字段子集。
- `web/default/src/features/subscriptions/lib/format.ts`
  - 已有 `formatPlanPrice`、`formatDuration`、`formatTokenLimit`、`formatConcurrencyLimit`。
- `web/default/src/features/wallet/components/subscription-plans-card.tsx`
  - 可作为套餐卡字段选择与格式化参考，但不能直接复用整个组件；钱包组件含购买弹窗、当前订阅、支付方式等认证场景逻辑，不适合公开首页。
- `web/default/src/routes/_authenticated/route.tsx`
  - 未登录访问 `_authenticated` 子路由会跳转 `/sign-in?redirect=<原地址>`。
- `web/default/src/routes/(auth)/sign-in.tsx`
  - `redirect` 搜索参数用于登录后跳回目标页面。
- `web/default/src/routes/_authenticated/wallet/index.tsx`
  - 钱包订阅套餐入口的实际路由是 `/wallet`，不是 `/pricing`。
- `web/default/src/routes/pricing/index.tsx`
  - `/pricing` 是模型目录 / 模型价格页，当前不承载订阅套餐购买。
- `web/default/src/i18n/locales/{en,zh,fr,ja,ru,vi}.json`
  - default 前端支持六种语言；新用户可见文案必须补齐 locale。
- `web/default/package.json`
  - `typecheck` 脚本为 `tsc -b`。

## 3. 决策

1. **直接在主分支实现。** 规格文件可以作为只读参考，但所有代码、测试、计划和提交都必须落在主工作区 `C:/Users/34404/source/repos/new-api`。
2. **主要改 default 首页，并做最小后端鉴权边界调整。** Issue 描述中的 `50+`、`100+`、`Core Features` 等来自 `web/default` 首页；classic 首页没有同构 Stats / Core Features 模块。本次不修改 classic。
3. **Stats 区域改造成套餐介绍模块，而不是删除位置。** 用户明确要求「把数字部分 Stats 改成套餐介绍」。实现应保留 Hero 下方的视觉节奏，但内容改为公开套餐卡。
4. **套餐数据使用真实公开接口。** 首页套餐介绍通过 `getPublicPlans()` 获取公开套餐，不硬编码套餐名、价格或额度，确保管理员配置变更后首页自动反映。
5. **`GET /api/subscription/plans` 必须允许未登录读取，但必须返回公开 DTO。** 该接口只返回 `enabled/public_visible/non-trial` 套餐的首页展示字段，不包含用户订阅、购买状态、支付配置、购买限制、升级分组、业务编码或其他内部管理字段。
6. **受保护订阅接口继续受保护。** `/api/subscription/self`、购买、支付等接口继续要求 `UserAuth`。
7. **首页套餐请求必须静默失败。** 接口异常、网络错误或业务失败时，首页不 toast、不重置 auth、不跳转；组件最终隐藏套餐模块。实现必须使用跳过全局错误处理的 API helper，并在 queryFn 内吞掉错误后返回失败/空结果，避免 Axios 与 QueryCache 全局错误副作用。
8. **无公开套餐或接口失败时隐藏套餐模块。** 首页不展示空壳、不展示错误模板。首次加载可使用轻量骨架或最小高度占位减少跳动。
9. **套餐模块只做介绍和导流，不做购买。** 首页公开区域不弹购买对话框，不拉取用户订阅，不处理支付方式。套餐卡按钮导向实际套餐选择入口 `/wallet`：已登录用户直接到钱包；未登录用户由 `_authenticated` 路由跳转登录并保留 redirect，登录后回到钱包。
10. **Quick Start 第三步保持既有 `/pricing` 契约。** Issue #3 的 Quick Start 仍指向模型目录 / 价格页，不与首页套餐卡 CTA 混淆。套餐介绍模块的 CTA 使用 `/wallet`。
11. **默认核心功能模块从默认首页路径移除。** `Features` 不再由默认首页渲染；相关导出和无用常量应清理，但不得删除或移除受保护版权头、项目归属信息。
12. **API demo 只保留 Chat / Responses。** `Claude`、`Gemini` demo 和对应 response 文案分支删除。同步收窄已无用的 tone / placeholder 处理，避免死代码。
13. **Hero 主标题使用 i18n，不硬编码中文。** 新增英文 key `Affordable, low-cost, high-speed GPT`；`zh` 翻译必须严格为 Issue 指定的 `超便宜低价高速的GPT`。
14. **Hero 副标题和按钮本次不改。** 只改主标题；未登录主按钮继续是 `Get Started`，次按钮继续是 `Browse Models` 并指向模型目录 `/pricing`。不得恢复 `View Pricing` 文案。
15. **超过三个套餐时必须提供查看全部入口。** 首页最多展示前三个公开套餐；如果公开套餐超过三个，显示 `View all plans` 链接到 `/wallet`。
16. **复用现有 `Choose a plan` key。** 套餐按钮使用已有 `Choose a plan`，避免新增 `Choose Plan` 造成翻译漂移。
17. **保留 Dashboard 入口。** 不删除 `Hero` 和 `PublicHeader` 中已登录态 `/dashboard` 入口；测试覆盖入口仍存在。
18. **Quick Start 文案保持现有契约。** 不破坏 `quick-start-copy.test.ts` 对 `Create API -> Try Playground -> Choose a plan`、`OpenCode-ready API help`、模型目录文案和不出现 `View Pricing` 的断言。
19. **不修改受保护项目标识。** 不触碰版权头、项目品牌、组织归属、README、package 元数据等受保护内容。

## 4. 业务范围

### 4.1 必须满足

- 默认首页主标题展示为中文「超便宜低价高速的GPT」（在中文 locale 下）。
- 默认首页仍保留已登录用户可见的「前往仪表盘」入口，跳转 `/dashboard`。
- Hero API demo 只展示 Chat / Responses 两个 API 示例。
- 默认首页不再展示 `50+`、`100+`、`50+`、`10+` 数字统计。
- Hero 下方原 Stats 区域改为套餐介绍模块。
- 未登录访客可以读取公开套餐列表，不触发 401 toast、auth reset 或跳转。
- 套餐介绍模块读取公开套餐接口 `/api/subscription/plans`。
- 套餐接口只返回启用、公开、非试用套餐的公开字段；隐藏套餐、禁用套餐、试用套餐和内部管理字段不得出现在公开首页套餐响应。
- 套餐介绍模块展示每个公开套餐的名称、可选副标题、价格、有效期、月 Token 限额、并发限制。
- 套餐介绍模块提供清晰的「选择套餐」入口，导向 `/wallet`。
- 公开套餐超过三个时，显示「查看全部套餐」入口，导向 `/wallet`。
- 没有公开套餐或接口失败时，套餐介绍模块不渲染空状态卡，不影响首页其他内容，也不触发全局错误提示。
- 默认首页不再渲染 `Core Features` / 「核心功能」模板模块。
- 三步快速上手继续保持 `Create API` → `Try Playground` → `Choose a plan`，且包含 OpenCode 相关描述。
- 首页模型目录入口继续使用现有 `Browse Models` / `Model Directory` 语义，不恢复 `View Pricing` 文案。
- 所有新增用户可见文案通过 `t()` 和 locale 文件管理。
- 修改 TypeScript / TSX 后必须运行 `bun run typecheck`。

### 4.2 非目标

- 不改数据库、套餐模型或种子数据。
- 不新增购买流程、支付弹窗或订阅状态展示到公开首页。
- 不修改 classic 首页。
- 不改管理员自定义首页内容能力；一旦 `home_page_content` 存在，仍优先展示自定义内容。
- 不清理全项目所有旧 i18n key；只补齐本次新增 key，避免大规模 locale churn。
- 不新增依赖。
- 不修改受保护品牌、版权、归属信息。

## 5. 后端设计

### 5.1 公开套餐响应 DTO

文件：`controller/subscription.go`

新增公开套餐 DTO，替代公开接口直接返回完整 `model.SubscriptionPlan`：

```go
type PublicSubscriptionPlan struct {
    Id                 int     `json:"id"`
    Title              string  `json:"title"`
    Subtitle           string  `json:"subtitle,omitempty"`
    PriceAmount        float64 `json:"price_amount"`
    Currency           string  `json:"currency"`
    DurationUnit       string  `json:"duration_unit"`
    DurationValue      int     `json:"duration_value"`
    CustomSeconds      int64   `json:"custom_seconds"`
    MonthlyTokenLimit  int64   `json:"monthly_token_limit"`
    ConcurrencyLimit   int     `json:"concurrency_limit"`
    PublicVisible      bool    `json:"public_visible"`
}

type PublicSubscriptionPlanDTO struct {
    Plan PublicSubscriptionPlan `json:"plan"`
}
```

允许公开字段仅限首页展示需要：

- `id`
- `title`
- `subtitle`
- `price_amount`
- `currency`
- `duration_unit`
- `duration_value`
- `custom_seconds`
- `monthly_token_limit`
- `concurrency_limit`
- `public_visible`

公开响应不得包含：

- `stripe_price_id`
- `creem_product_id`
- `max_purchase_per_user`
- `upgrade_group`
- `business_code`
- `reward_eligible`
- `total_amount`
- `enabled`
- `is_trial`
- `invite_trial`
- `trial_duration_hours`
- `quota_reset_period`
- `quota_reset_custom_seconds`
- `created_at`
- `updated_at`

管理员完整字段继续通过 `/api/subscription/admin/plans` 返回；需要购买限制、支付产品 ID 等完整字段的认证购买流程不得改用公开 DTO。

### 5.2 公开套餐路由

文件：`router/api-router.go`

调整路由分组：

- `GET /api/subscription/plans` 从 `subscriptionRoute.Use(middleware.UserAuth())` 的分组中移出，挂到不要求登录的 `/api/subscription` 分组。
- 需要用户身份的订阅接口继续在 `middleware.UserAuth()` 分组中：
  - `GET /api/subscription/self`
  - `PUT /api/subscription/self/preference`
  - `POST /api/subscription/epay/pay`
  - `POST /api/subscription/balance/pay`
  - `POST /api/subscription/stripe/pay`
  - `POST /api/subscription/creem/pay`

目标结构示意：

```go
subscriptionPublicRoute := apiRouter.Group("/subscription")
{
    subscriptionPublicRoute.GET("/plans", controller.GetSubscriptionPlans)
}

subscriptionRoute := apiRouter.Group("/subscription")
subscriptionRoute.Use(middleware.UserAuth())
{
    subscriptionRoute.GET("/self", controller.GetSubscriptionSelf)
    // 其他需要登录的订阅接口保持不变
}
```

如果实现时更偏好单个 `subscriptionRoute := apiRouter.Group("/subscription")`，也可以先注册 `GET /plans`，再对同一 group 使用 `subscriptionRoute.Use(middleware.UserAuth())` 注册后续路由；但必须通过路由测试证明 `/api/subscription/plans` 未登录可访问，而 `/api/subscription/self` 未登录仍是 401。

### 5.3 套餐过滤契约

文件：`controller/subscription.go`

`GetSubscriptionPlans` 查询过滤条件必须保持：

```go
enabled = true AND public_visible = true AND is_trial = false
```

排序保持：

```go
sort_order desc, id desc
```

不得为了首页展示而返回隐藏套餐、禁用套餐、试用套餐、用户订阅状态、购买次数或支付配置。

### 5.4 后端测试

新增或扩展路由测试，建议文件：`router/subscription_public_plans_route_test.go` 或现有合适测试文件。

覆盖：

- 未登录 `GET /api/subscription/plans` 返回 200。
- 未登录 `GET /api/subscription/self` 仍返回 401。
- 响应只包含公开、启用、非试用套餐。
- 过滤测试必须覆盖四类套餐：公开启用普通套餐、隐藏套餐、禁用套餐、试用套餐。
- 公开响应不包含内部字段：`stripe_price_id`、`creem_product_id`、`max_purchase_per_user`、`upgrade_group`、`business_code`、`reward_eligible`、`total_amount`、`enabled`、`is_trial`、`created_at`、`updated_at` 等。

测试可参考 `router/welcome_popup_route_test.go` 的 `SetApiRouter(engine)` 模式，以及 `controller/subscription_trial_purchase_test.go` 中直接验证 `GetSubscriptionPlans` 隐藏试用套餐的断言。不要依赖外部数据库；按项目已有 model 测试初始化模式设置测试 DB。

后端验证命令必须覆盖鉴权和过滤：

- 如果路由测试同时覆盖鉴权、过滤和 DTO 字段白名单，运行：

```bash
go test ./router -run 'TestSubscriptionPlansPublicRoute' -count=1
```

- 如果过滤或 DTO 字段白名单断言放在 controller/model 包，则还必须运行对应精确命令，例如：

```bash
go test ./controller -run 'TestGetSubscriptionPlans' -count=1
```

## 6. 前端设计

### 6.1 首页组合

文件：`web/default/src/features/home/index.tsx`

默认首页组合调整为：

```tsx
<PublicLayout showMainContainer={false}>
  <Hero isAuthenticated={isAuthenticated} />
  <PlansPreview />
  <HowItWorks />
  <CTA isAuthenticated={isAuthenticated} />
  <Footer />
</PublicLayout>
```

`Features` 从默认首页移除。`Stats` 不再作为数字统计存在，替换为 `PlansPreview`。

自定义首页分支保持不变：

- `content` 为 URL：继续 iframe。
- `content` 为 Markdown：继续 `Markdown`。
- 该分支不渲染 `Hero` / `PlansPreview` / `HowItWorks`。

### 6.2 Hero 标题与按钮

文件：`web/default/src/features/home/components/sections/hero.tsx`

标题改为单一主标题：

```tsx
{t('Affordable, low-cost, high-speed GPT')}
```

中文 locale：

```json
"Affordable, low-cost, high-speed GPT": "超便宜低价高速的GPT"
```

Hero 副标题和按钮本次不改：

- 已登录：`Go to Dashboard`，`to='/dashboard'`。
- 未登录主按钮：`Get Started`。
- 未登录次按钮：`Browse Models`，`to='/pricing'`。
- 不得恢复 `View Pricing` 文案。

### 6.3 API demo

文件：`web/default/src/features/home/components/hero-terminal-demo.tsx`

`API_DEMOS` 只保留：

- `gpt-chat`：`label: 'Chat'`，`endpoint: '/v1/chat/completions'`。
- `responses`：`label: 'Responses'`，`endpoint: '/v1/responses'`。

删除：

- `claude` demo。
- `gemini` demo。
- `truncateResponse()` 中 `claude` / `gemini` 分支。
- 不再使用的 `AccentTone` 值和 `ACCENT_CLASSES` 项。
- 若 `<in>` / `<out>` placeholder 处理删除后无引用，也同步删除对应分支。

轮播逻辑继续使用 `API_DEMOS.length`，无需特殊处理。

### 6.4 套餐介绍数据辅助函数

建议文件：`web/default/src/features/home/lib/plans-preview.ts`

新增小型纯函数，用于过滤异常数据、限制展示数量，并避免把数据筛选逻辑藏在组件里：

```ts
import type { PlanRecord } from '@/features/subscriptions/types'

export const HOME_PLANS_PREVIEW_LIMIT = 3

type MaybePlanRecord = PlanRecord | null | undefined

function isVisiblePlanRecord(record: MaybePlanRecord): record is PlanRecord {
  return Boolean(record?.plan && record.plan.public_visible !== false)
}

export function selectHomePlanRecords(
  records: readonly MaybePlanRecord[] = []
): PlanRecord[] {
  return records.filter(isVisiblePlanRecord).slice(0, HOME_PLANS_PREVIEW_LIMIT)
}

export function hasMoreHomePlans(
  records: readonly MaybePlanRecord[] = []
): boolean {
  return records.filter(isVisiblePlanRecord).length > HOME_PLANS_PREVIEW_LIMIT
}
```

对外行为必须满足：

- 过滤异常空 record。
- 过滤 `public_visible === false` 的防御性异常数据。
- 保持后端返回顺序，不在前端重新排序。
- 最多返回 3 个。
- 能判断是否需要展示 `View all plans`。

### 6.5 首页套餐静默 API helper

文件：`web/default/src/features/subscriptions/api.ts`

保留现有 `getPublicPlans()` 作为普通调用；新增或调整静默 helper 给首页使用。推荐新增专用函数，避免影响钱包等已有调用语义：

```ts
export async function getPublicPlansQuiet(): Promise<ApiResponse<PlanRecord[]>> {
  try {
    const res = await api.get('/api/subscription/plans', {
      skipErrorHandler: true,
      skipBusinessError: true,
    } as Record<string, unknown>)
    return res.data
  } catch {
    return { success: false, data: [] }
  }
}
```

要求：

- HTTP 错误、网络错误、业务失败都不得在首页触发全局 toast、auth reset 或跳转。
- queryFn 不得把错误抛给 React Query QueryCache。
- 现有钱包 / 认证场景如仍使用普通 `getPublicPlans()`，不强制改变其错误提示语义。

### 6.6 套餐介绍组件

建议文件：`web/default/src/features/home/components/sections/plans-preview.tsx`

职责：在默认首页公开展示可购买套餐摘要。

数据加载：

```tsx
const plansQuery = useQuery({
  queryKey: ['home', 'subscription-plans'],
  queryFn: getPublicPlansQuiet,
  staleTime: 60_000,
})
```

数据处理：

- 使用 `plansQuery.data?.success ? plansQuery.data.data ?? [] : []`。
- 使用 `selectHomePlanRecords(records)` 得到展示卡片。
- 使用 `hasMoreHomePlans(records)` 决定是否展示 `View all plans`。
- `plansQuery.isLoading` 时显示轻量 skeleton 或最小高度占位。
- 查询完成后，若 `plans.length === 0` 或 `plansQuery.data?.success === false`，返回 `null`。
- 因 queryFn 已吞掉错误，不需要依赖 React Query `throwOnError: false` 抑制错误。

展示字段：

- 标题：`plan.title || t('Subscription Plans')`。
- 副标题：`plan.subtitle` 非空时展示。
- 价格：`formatPlanPrice(plan.price_amount, plan.currency)`。
- 有效期：`formatDuration(plan, t)`。
- 月 Token 限额：`formatTokenLimit(plan.monthly_token_limit, t)`。
- 并发限制：`formatConcurrencyLimit(plan.concurrency_limit, t)`。
- CTA：复用 `t('Choose a plan')`，跳转 `/wallet`。

布局建议：

- 外层沿用 Stats 区域位置：`border-border/40 bg-muted/10 relative z-10 border-y`。
- 容器：`mx-auto max-w-6xl px-6 py-10 md:py-12`。
- 标题区：短标题 + 简短说明：
  - `Subscription Plans`
  - `Pick a plan that fits your GPT usage.`
- 卡片 grid：移动端单列，平板两列，桌面三列。
- 最多展示前三个套餐。
- 如果公开套餐超过三个，必须显示 `View all plans` 链接到 `/wallet`。

性能和可维护性：

- 不引入钱包购买组件，避免公开首页加载支付弹窗和当前订阅状态逻辑。
- 使用 `import type` 引入 `PlanRecord` / `SubscriptionPlan`。
- 组件 props 非必要不解构，遵守 `web/default/AGENTS.md`。

### 6.7 删除核心功能模板模块

文件：

- `web/default/src/features/home/components/sections/features.tsx`
- `web/default/src/features/home/components/index.ts`
- `web/default/src/features/home/index.tsx`
- `web/default/src/features/home/constants.ts`

要求：

- `Home` 不再导入或渲染 `Features`。
- `components/index.ts` 不再导出 `Features`。
- 不强制物理删除 `features.tsx`。如果删除文件，必须确认不会违反受保护版权 / 归属信息要求；更保守做法是让文件不再被默认首页路径引用。
- `constants.ts` 中 `DEFAULT_FEATURES` / `getDefaultFeatures` 如果无引用，可以删除对应常量和函数；不得误删文件头版权信息。
- `GATEWAY_FEATURES` / `getGatewayFeatures` 仍被 `gateway-card.tsx` 使用，不得误删。
- 不批量删除 locale 中旧 `Core Features` 等 key，避免无关 diff。

### 6.8 组件导出命名

文件：`web/default/src/features/home/components/index.ts`

目标导出：

```ts
export { CTA } from './sections/cta'
export { Hero } from './sections/hero'
export { HowItWorks } from './sections/how-it-works'
export { PlansPreview } from './sections/plans-preview'
```

`Stats` export 删除。

## 7. i18n 设计

新增 key 至 `web/default/src/i18n/locales/{en,zh,fr,ja,ru,vi}.json`：

| Key | en | zh | fr | ja | ru | vi |
|---|---|---|---|---|---|---|
| `Affordable, low-cost, high-speed GPT` | `Affordable, low-cost, high-speed GPT` | `超便宜低价高速的GPT` | `GPT ultra abordable, économique et rapide` | `超低価格で高速なGPT` | `Очень доступный, недорогой и быстрый GPT` | `GPT giá cực rẻ, chi phí thấp và tốc độ cao` |
| `Pick a plan that fits your GPT usage.` | `Pick a plan that fits your GPT usage.` | `选择适合你 GPT 使用量的套餐。` | `Choisissez un forfait adapté à votre usage de GPT.` | `GPT の利用量に合ったプランを選択してください。` | `Выберите тариф, который подходит вашему использованию GPT.` | `Chọn gói phù hợp với nhu cầu sử dụng GPT của bạn.` |
| `View all plans` | `View all plans` | `查看全部套餐` | `Voir tous les forfaits` | `すべてのプランを見る` | `Посмотреть все тарифы` | `Xem tất cả gói` |

已有 key 必须复用：

- `Subscription Plans`
- `Validity Period`
- `Monthly Token Limit`
- `Concurrency Limit`
- `Choose a plan`
- `Browse Models`
- `Go to Dashboard`
- `Create API`
- `Try Playground`
- `OpenCode-ready API help` 所在描述 key

不得新增或恢复 `View Pricing` 文案。

实施后运行：

```bash
cd web/default
bun run i18n:sync
```

新增窄范围 i18n smoke test，验证本次新增 key 在六种 locale 均存在且非空，并验证 `zh` 的主标题严格等于 `超便宜低价高速的GPT`。不要要求现有全局 untranslatedCount 归零。

## 8. 测试设计

### 8.1 后端路由与公开 DTO 测试

新增或扩展：`router/subscription_public_plans_route_test.go`

覆盖：

1. 未登录 `GET /api/subscription/plans` 返回 200。
2. 未登录 `GET /api/subscription/self` 仍返回 401。
3. 响应中只包含公开、启用、非试用套餐。
4. 过滤测试覆盖公开启用普通套餐、隐藏套餐、禁用套餐、试用套餐。
5. 公开响应不包含内部字段：`stripe_price_id`、`creem_product_id`、`max_purchase_per_user`、`upgrade_group`、`business_code`、`reward_eligible`、`total_amount`、`enabled`、`is_trial`、`created_at`、`updated_at`。

后端验证命令必须覆盖鉴权、过滤和 DTO 字段白名单：

- 如果路由测试同时覆盖三者，运行：

```bash
go test ./router -run 'TestSubscriptionPlansPublicRoute' -count=1
```

- 如果过滤或 DTO 字段白名单断言放在 controller/model 包，则还必须运行对应精确命令，例如：

```bash
go test ./controller -run 'TestGetSubscriptionPlans' -count=1
```

### 8.2 Source-level 首页契约测试

新增文件：`web/default/src/features/home/home-page-copy.test.ts`

使用项目现有 `node:test` + `assert` + `readFileSync` 模式，避免引入 React Testing Library。

覆盖：

1. `Home` 默认组合使用 `PlansPreview`，不再使用 `Stats` / `Features`。
2. `plans-preview.tsx` 调用 `getPublicPlansQuiet`，而不是普通 `getPublicPlans`。
3. `plans-preview.tsx` 使用 `formatPlanPrice`、`formatDuration`、`formatTokenLimit`、`formatConcurrencyLimit`。
4. `Hero` 包含新标题 key，并保留 `to='/dashboard'` 与 `Go to Dashboard`。
5. `Hero` 未登录次按钮继续使用 `Browse Models`，不得出现 `View Pricing`。
6. `HeroTerminalDemo` 包含 `gpt-chat` 和 `responses`，不包含 `claude` / `gemini` demo。
7. `Features` / `Core Features` 不再出现在默认首页组合路径。
8. `PlansPreview` 的套餐 CTA 链接目标为 `/wallet`，而不是 `/pricing`。
9. `PlansPreview` 使用 `hasMoreHomePlans(records)` 控制渲染 `t('View all plans')`，且该链接目标为 `/wallet`。

### 8.3 套餐预览纯函数测试

新增文件：`web/default/src/features/home/lib/plans-preview.test.ts`

覆盖：

- `selectHomePlanRecords` 保持后端返回顺序。
- 最多返回 3 个套餐。
- 过滤 `public_visible === false` 的异常数据。
- 过滤 `null` / `undefined` / 缺失 `plan` 的异常数据。
- 空输入返回空数组。
- `hasMoreHomePlans` 在过滤后数量超过 3 时返回 true。

推荐命令：

```bash
cd web/default
bunx tsx --test src/features/home/lib/plans-preview.test.ts
```

### 8.4 首页静默 API helper 测试

可在 `web/default/src/features/home/home-page-copy.test.ts` 或 `web/default/src/features/subscriptions/api.test.ts` 中用 source-level 测试覆盖：

- `getPublicPlansQuiet` 使用 `skipErrorHandler: true`。
- `getPublicPlansQuiet` 使用 `skipBusinessError: true`。
- `getPublicPlansQuiet` catch 错误并返回 `{ success: false, data: [] }`，避免错误抛给 QueryCache。
- `PlansPreview` 使用 `getPublicPlansQuiet`。

### 8.5 i18n smoke 测试

可放在 `web/default/src/features/home/home-page-copy.test.ts` 或单独文件 `web/default/src/features/home/home-page-i18n.test.ts`。

覆盖：

- `Affordable, low-cost, high-speed GPT` 在 `en/zh/fr/ja/ru/vi` 都存在且非空。
- `Pick a plan that fits your GPT usage.` 在 `en/zh/fr/ja/ru/vi` 都存在且非空。
- `View all plans` 在 `en/zh/fr/ja/ru/vi` 都存在且非空。
- `zh.translation['Affordable, low-cost, high-speed GPT'] === '超便宜低价高速的GPT'`。

### 8.6 Quick Start 回归测试

保留并继续运行：

```bash
cd web/default
bunx tsx --test src/features/home/quick-start-copy.test.ts
```

不得破坏现有断言：

- `Create API`
- `Try Playground`
- `Choose a plan`
- `OpenCode-ready API help`
- 不出现 `View Pricing|Review model rates`
- 保留 `Model Directory|Browse Models|Browse available models`

### 8.7 类型检查

修改 TS / TSX 后必须运行：

```bash
cd web/default
bun run typecheck
```

### 8.8 前端定向测试总命令

实现完成后至少运行：

```bash
cd web/default
bunx tsx --test \
  src/features/home/home-page-copy.test.ts \
  src/features/home/lib/plans-preview.test.ts \
  src/features/home/quick-start-copy.test.ts
bun run typecheck
```

如 i18n smoke test 单独建文件，则加入同一 `bunx tsx --test` 命令。

## 9. 验收清单

- [ ] 默认首页主标题在中文环境显示为「超便宜低价高速的GPT」。
- [ ] 已登录用户仍能从首页进入 `/dashboard`。
- [ ] API demo 标签只剩 Chat / Responses。
- [ ] 默认首页不再展示 `50+`、`100+`、`50+`、`10+` 数字统计。
- [ ] 原 Stats 位置展示公开套餐介绍。
- [ ] 未登录访客可访问公开套餐列表，不触发 401。
- [ ] 套餐介绍接口失败时不触发 toast、auth reset 或路由跳转。
- [ ] 套餐介绍来自 `getPublicPlansQuiet()` / 公开套餐接口，不是硬编码价格或套餐名。
- [ ] 套餐接口不返回隐藏、禁用或试用套餐。
- [ ] 套餐接口不返回支付配置、购买限制、升级分组、业务编码、奖励资格等内部字段。
- [ ] 套餐卡展示套餐名、价格、有效期、月 Token 限额、并发限制。
- [ ] 套餐卡「选择套餐」入口导向 `/wallet`。
- [ ] 公开套餐超过三个时，「查看全部套餐」入口导向 `/wallet`。
- [ ] 无公开套餐或接口失败时，套餐介绍模块不渲染空状态卡，不影响首页其他内容。
- [ ] 默认首页不再渲染 `Core Features` / 「核心功能」模板模块。
- [ ] 三步快速上手仍与 Quick Start 保持一致。
- [ ] 首页模型目录入口继续使用 `Browse Models` / `Model Directory` 语义，不恢复 `View Pricing` 文案。
- [ ] 新增文案已补齐六种 locale，并有 i18n smoke test 覆盖。
- [ ] 后端路由与公开 DTO 测试通过。
- [ ] 前端定向测试通过。
- [ ] `bun run typecheck` 通过。

## 10. 实施顺序建议

1. 在主工作区 `C:/Users/34404/source/repos/new-api` 编写计划和代码；不要在 worktree 中开发。
2. 编写后端路由 / DTO 失败测试，证明未登录 `/api/subscription/plans` 当前不可访问、公开响应字段过宽，而 `/api/subscription/self` 必须继续受保护。
3. 新增公开套餐 DTO，调整 `GetSubscriptionPlans` 输出白名单字段，保持过滤条件不变。
4. 调整 `router/api-router.go` 的订阅路由鉴权边界。
5. 运行后端定向测试通过并提交后端路由 / DTO 改动。
6. 编写前端 source-level 首页契约测试、套餐预览纯函数测试和首页静默 API helper 测试。
7. 新增 `getPublicPlansQuiet`。
8. 新建 `plans-preview` 纯函数与 `PlansPreview` 组件。
9. 更新 `Home` 默认组合：`Stats` → `PlansPreview`，移除 `Features`。
10. 更新 `components/index.ts` 导出。
11. 修改 Hero 主标题，保留 `Browse Models`。
12. 修改 `HeroTerminalDemo`，只保留 Chat / Responses，并清理无用 demo 分支。
13. 清理 `constants.ts` 中已无引用的 Stats / Features 数据；不得误删仍被使用的 `GATEWAY_FEATURES` / `getGatewayFeatures`。
14. 补齐 locale，运行 `bun run i18n:sync`，并添加 i18n smoke test。
15. 运行后端定向测试、前端定向测试与 `bun run typecheck`。
16. 进行子代理审查修改循环；所有 review 子代理通过后继续下一阶段。

## 11. 风险与约束

- **公开接口鉴权：** `/api/subscription/plans` 必须公开，但只返回公开、启用、非试用套餐；订阅购买和用户订阅接口必须继续受保护。
- **公开字段白名单：** 公开套餐接口必须返回专用 DTO，不得直接暴露完整 `model.SubscriptionPlan` 的支付配置、购买限制、升级分组、业务编码、奖励资格、时间戳等内部字段。
- **公开接口加载失败：** 首页不能出现错误 toast 或破损卡片，模块最终静默隐藏；queryFn 必须吞掉错误，不能把错误抛给 QueryCache；首次加载可用 skeleton / 最小高度占位减少跳动。
- **自定义首页优先级：** 管理员配置 `home_page_content` 时，本次默认首页改造不会显示；这是现有行为，不能改变。
- **套餐数量过多：** 首页只展示前三个公开套餐，完整入口通过 `/wallet` 进入，避免首页过长。
- **CTA 目标：** 当前 `/pricing` 是模型目录，不是订阅套餐选择页；套餐卡 CTA 必须导向 `/wallet`。Quick Start 第三步继续保持既有 `/pricing` 契约。
- **模型目录文案：** 首页模型目录入口继续使用 `Browse Models` / `Model Directory` 语义，不得恢复 `View Pricing`。
- **i18n key 风格：** 当前 default 项目多用英文源文案作为 key；本规格沿用英文 key，并在 zh 中给出 Issue 指定中文。
- **文件清理：** 不要删除或移除含受保护版权头 / 归属信息的内容。移除默认首页引用即可满足模板模块删除；如清理无用常量，必须保留文件头。
- **受保护信息：** 不修改 `new-api` / `QuantumNous` 相关版权、品牌、归属信息。
- **主分支直接开发：** 后续实现必须在 `C:/Users/34404/source/repos/new-api` 主工作区进行，不在 `.worktrees/issue-6-homepage-plans` 中开发。
- **无关工作保护：** 主仓库存在其他未跟踪文件；不得删除、格式化、移动或修改与 Issue #6 无关的未跟踪文件。不得清理 `.worktrees/api-help-followup` 或其他非本任务 worktree。
