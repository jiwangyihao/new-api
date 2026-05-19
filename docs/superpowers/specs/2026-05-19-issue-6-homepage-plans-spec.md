# Issue #6 首页文案与套餐介绍规格

> 面向 AI 代理的工作者：本规格用于处理 GitHub fork 仓库 `jiwangyihao/new-api` 的 Issue #6「主页内容：更新标题、API 展示和功能模块文案」。实现前必须读取仓库根目录 `AGENTS.md` 与 `web/default/AGENTS.md`，并遵守 React 19、TypeScript、TanStack Router、TanStack Query、i18n、Base UI / Tailwind CSS、Bun 以及项目受保护标识约束。

**目标：** 将默认首页从通用模板展示切换为更贴合「赔钱GPT」的落地页：主标题改为「超便宜低价高速的GPT」，API 展示只保留 chat / responses，原数字 Stats 区域改为公开套餐介绍，删除默认「核心功能」模板模块，并保留与 Quick Start 一致的三步快速上手。

**架构：** 仅改 `web/default` 的默认首页。默认首页仍通过 `Home` 组合各区块；当管理员配置了自定义首页内容时，继续优先展示自定义 Markdown / iframe，不受本规格影响。Stats 区域重命名并替换为 `PlansPreview`，通过现有 `getPublicPlans()` 公开接口读取套餐，复用订阅模块现有格式化函数渲染价格、有效期、Token 限额和并发限制。

**技术栈：** React 19、TypeScript、TanStack Router、TanStack Query、i18next、Base UI / shadcn 风格组件、Tailwind CSS、Bun。

---

## 1. 背景

Issue #6 要求默认网站首页清理通用模板内容，并替换为更贴合当前站点定位的文案。原始验收项包括：

- 首页主标题替换为「超便宜低价高速的GPT」。
- 保留「前往仪表盘」入口。
- API 展示区域只保留 `chat` 和 `responses`。
- 删除默认数字展示，例如 `50+`、`100+`、`50+`。
- 删除默认「核心功能」等模板化模块。
- 可保留「三步快速上手」，但内容需要和 Quick Start 保持一致：
  - 第一步：创建 API
  - 第二步：游乐场试用（适配 OpenCode）
  - 第三步：选择套餐

用户进一步明确：**希望把首页数字部分 Stats 改成套餐介绍**。因此本规格不再把 Stats 区域直接移除，而是把该位置替换为套餐介绍模块；默认数字和计数动画必须消失。

## 2. 当前代码基线

已确认的关键文件和现有能力：

- `web/default/src/features/home/index.tsx`
  - 默认首页在无自定义首页内容时渲染：`Hero`、`Stats`、`Features`、`HowItWorks`、`CTA`、`Footer`。
  - 自定义首页内容来自 `useHomePageContent()`；有自定义内容时渲染 Markdown 或 iframe，并跳过默认落地页。
- `web/default/src/features/home/components/sections/hero.tsx`
  - 当前标题为 `Unified API Gateway for` + `All Your AI Models`。
  - 已登录用户按钮为 `Go to Dashboard`，跳转 `/dashboard`。
  - 未登录用户按钮为 `Get Started` 和 `View Pricing`。
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
- `web/default/src/features/subscriptions/api.ts`
  - 已有 `getPublicPlans()`，请求 `/api/subscription/plans`，返回 `PlanRecord[]`。
- `web/default/src/features/subscriptions/types.ts`
  - `SubscriptionPlan` 包含 `title`、`subtitle`、`price_amount`、`currency`、`duration_*`、`monthly_token_limit`、`concurrency_limit`、`public_visible` 等字段。
- `web/default/src/features/subscriptions/lib/format.ts`
  - 已有 `formatPlanPrice`、`formatDuration`、`formatTokenLimit`、`formatConcurrencyLimit`。
- `web/default/src/features/wallet/components/subscription-plans-card.tsx`
  - 可作为套餐卡字段选择与格式化参考，但不能直接复用整个组件；钱包组件含购买弹窗、当前订阅、支付方式等认证场景逻辑，不适合公开首页。
- `web/default/src/i18n/locales/{en,zh,fr,ja,ru,vi}.json`
  - default 前端支持六种语言；新用户可见文案必须补齐 locale。
- `web/default/package.json`
  - `typecheck` 脚本为 `tsc -b`。

## 3. 决策

1. **只改 default 首页。** Issue 描述中的 `50+`、`100+`、`Core Features` 等均来自 `web/default` 首页；classic 首页没有同构 Stats / Core Features 模块。本次不修改 classic，避免扩大范围。
2. **Stats 区域改造成套餐介绍模块，而不是删除位置。** 用户明确要求「把数字部分 Stats 改成套餐介绍」。实现应保留 Hero 下方的视觉节奏，但内容改为公开套餐卡。
3. **套餐数据使用真实公开接口。** 首页套餐介绍通过 `getPublicPlans()` 获取公开套餐，不硬编码套餐名、价格或额度，确保管理员配置变更后首页自动反映。
4. **无公开套餐时隐藏套餐模块。** 首页不展示空壳、不展示错误模板。加载失败或返回空数组时该模块返回 `null`。
5. **套餐模块只做介绍和导流，不做购买。** 首页公开区域不弹购买对话框，不拉取用户订阅，不处理支付方式。按钮跳转到 `/pricing` 或用户后续入口，由现有页面负责后续流程。
6. **默认核心功能模块删除。** `Features` 不再由默认首页渲染；相关导出和死代码应清理，避免模板内容继续存在于默认首页路径。
7. **API demo 只保留 Chat / Responses。** `Claude`、`Gemini` demo 和对应 response 文案分支删除。标签大小写可继续使用现有 `Chat` / `Responses` 视觉标签，但测试应证明不再包含 `Claude` / `Gemini` demo。
8. **Hero 主标题使用 i18n，不硬编码中文。** 新增英文 key，例如 `Affordable, low-cost, high-speed GPT`；`zh` 翻译为 Issue 指定的 `超便宜低价高速的GPT`。
9. **保留 Dashboard 入口。** 不删除 `Hero` 和 `PublicHeader` 中已登录态 `/dashboard` 入口；测试覆盖入口仍存在。
10. **Quick Start 文案保持现有契约。** 不破坏 `quick-start-copy.test.ts` 对 `Create API -> Try Playground -> Choose a plan` 与 `OpenCode-ready API help` 的断言。
11. **不修改受保护项目标识。** 不触碰版权头、项目品牌、组织归属、README、package 元数据等受保护内容。

## 4. 业务范围

### 4.1 必须满足

- 默认首页主标题展示为中文「超便宜低价高速的GPT」（在中文 locale 下）。
- 默认首页仍保留已登录用户可见的「前往仪表盘」入口，跳转 `/dashboard`。
- Hero API demo 只展示 Chat / Responses 两个 API 示例。
- 默认首页不再展示 `50+`、`100+`、`50+`、`10+` 数字统计。
- Hero 下方原 Stats 区域改为套餐介绍模块。
- 套餐介绍模块读取公开套餐接口 `/api/subscription/plans`。
- 套餐介绍模块展示每个公开套餐的名称、可选副标题、价格、有效期、月 Token 限额、并发限制。
- 套餐介绍模块提供清晰的「选择套餐」入口。
- 没有公开套餐或接口失败时，套餐介绍模块不渲染空状态卡，不影响首页其他内容。
- 默认首页不再渲染 `Core Features` / 「核心功能」模板模块。
- 三步快速上手继续保持 `Create API` → `Try Playground` → `Choose a plan`，且包含 OpenCode 相关描述。
- 所有新增用户可见文案通过 `t()` 和 locale 文件管理。
- 修改 TypeScript / TSX 后必须运行 `bun run typecheck`。

### 4.2 非目标

- 不改后端接口、数据库、套餐模型或种子数据。
- 不新增购买流程、支付弹窗或订阅状态展示到公开首页。
- 不修改 classic 首页。
- 不改管理员自定义首页内容能力；一旦 `home_page_content` 存在，仍优先展示自定义内容。
- 不清理全项目所有旧 i18n key；只补齐本次新增 key，避免大规模 locale churn。
- 不新增依赖。
- 不修改受保护品牌、版权、归属信息。

## 5. 前端设计

### 5.1 首页组合

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

### 5.2 Hero 标题与按钮

文件：`web/default/src/features/home/components/sections/hero.tsx`

标题改为单一主标题：

```tsx
{t('Affordable, low-cost, high-speed GPT')}
```

中文 locale：

```json
"Affordable, low-cost, high-speed GPT": "超便宜低价高速的GPT"
```

副标题可保留现有文案，也可改成更贴合站点定位的轻量文案；若改动，必须补齐 locale。为降低范围，推荐仅改主标题。

按钮保持：

- 已登录：`Go to Dashboard`，`to='/dashboard'`。
- 未登录：保留 `Get Started` 和 `View Pricing`。

### 5.3 API demo

文件：`web/default/src/features/home/components/hero-terminal-demo.tsx`

`API_DEMOS` 只保留：

- `gpt-chat`：`label: 'Chat'`，`endpoint: '/v1/chat/completions'`。
- `responses`：`label: 'Responses'`，`endpoint: '/v1/responses'`。

删除：

- `claude` demo。
- `gemini` demo。
- `truncateResponse()` 中 `claude` / `gemini` 分支。
- 如果删除后某个 `AccentTone` 只服务被删 demo 且不再使用，可以一起收窄类型和 `ACCENT_CLASSES`；保留未使用 tone 也可接受，但推荐清理未用类型以减少死代码。

轮播逻辑继续使用 `API_DEMOS.length`，无需特殊处理。

### 5.4 套餐介绍模块

建议文件：`web/default/src/features/home/components/sections/plans-preview.tsx`

职责：在默认首页公开展示可购买套餐摘要。

数据加载：

```tsx
const plansQuery = useQuery({
  queryKey: ['home', 'subscription-plans'],
  queryFn: getPublicPlans,
  staleTime: 60_000,
})
```

数据筛选：

- 使用 `res.success ? res.data ?? [] : []`。
- 若接口本身已只返回公开套餐，则直接展示返回值；为防御异常数据，可额外过滤 `record.plan.public_visible !== false`。
- 若 `plans.length === 0`，返回 `null`。
- 查询失败时返回 `null`；不 toast，避免公开首页首屏出现干扰性错误提示。

展示字段：

- 标题：`plan.title || t('Subscription Plans')`。
- 副标题：`plan.subtitle` 非空时展示。
- 价格：`formatPlanPrice(plan.price_amount, plan.currency)`。
- 有效期：`formatDuration(plan, t)`。
- 月 Token 限额：`formatTokenLimit(plan.monthly_token_limit, t)`。
- 并发限制：`formatConcurrencyLimit(plan.concurrency_limit, t)`。
- CTA：`Choose Plan` / `选择套餐`，跳转 `/pricing`。

布局建议：

- 外层沿用 Stats 区域位置：`border-border/40 bg-muted/10 relative z-10 border-y`。
- 容器：`mx-auto max-w-6xl px-6 py-10 md:py-12`。
- 标题区：短标题 + 简短说明，例如：
  - `Subscription Plans`
  - `Pick a plan that fits your GPT usage.`
- 卡片 grid：移动端单列，平板两列，桌面三列。
- 为避免首页过长，最多展示前三个套餐：`plans.slice(0, 3)`。
- 如果公开套餐超过三个，可在标题区或底部提供 `View all plans` 链接到 `/pricing`。

性能和可维护性：

- 不引入钱包购买组件，避免公开首页加载支付弹窗和当前订阅状态逻辑。
- 不在渲染路径中创建过多复杂对象；套餐 benefits 可由小函数构建，或在 map 内局部构建，数据量最多三项，成本可接受。
- 使用 `import type` 引入 `PlanRecord` / `SubscriptionPlan`。

### 5.5 删除核心功能模板模块

文件：

- `web/default/src/features/home/components/sections/features.tsx`
- `web/default/src/features/home/components/index.ts`
- `web/default/src/features/home/index.tsx`
- `web/default/src/features/home/constants.ts`

要求：

- `Home` 不再导入或渲染 `Features`。
- `components/index.ts` 不再导出 `Features`。
- 若 `features.tsx` 无其他引用，删除该文件。
- `constants.ts` 中 `DEFAULT_FEATURES` / `getDefaultFeatures` 如果无引用，删除。
- `GATEWAY_FEATURES` / `getGatewayFeatures` 仍被 `gateway-card.tsx` 使用，不得误删。
- 不批量删除 locale 中旧 `Core Features` 等 key，除非 i18n sync 自动处理；避免无关 diff。

### 5.6 组件导出命名

文件：`web/default/src/features/home/components/index.ts`

目标导出：

```ts
export { CTA } from './sections/cta'
export { Hero } from './sections/hero'
export { HowItWorks } from './sections/how-it-works'
export { PlansPreview } from './sections/plans-preview'
```

`Stats` export 删除。

## 6. i18n 设计

新增 key 至 `web/default/src/i18n/locales/{en,zh,fr,ja,ru,vi}.json`：

| Key | en | zh | fr | ja | ru | vi |
|---|---|---|---|---|---|---|
| `Affordable, low-cost, high-speed GPT` | `Affordable, low-cost, high-speed GPT` | `超便宜低价高速的GPT` | `GPT ultra abordable, économique et rapide` | `超低価格で高速なGPT` | `Очень доступный, недорогой и быстрый GPT` | `GPT giá cực rẻ, chi phí thấp và tốc độ cao` |
| `Pick a plan that fits your GPT usage.` | same as key | `选择适合你 GPT 使用量的套餐。` | `Choisissez un forfait adapté à votre usage de GPT.` | `GPT の利用量に合ったプランを選択してください。` | `Выберите тариф, который подходит вашему использованию GPT.` | `Chọn gói phù hợp với nhu cầu sử dụng GPT của bạn.` |
| `View all plans` | same as key | `查看全部套餐` | `Voir tous les forfaits` | `すべてのプランを見る` | `Посмотреть все тарифы` | `Xem tất cả gói` |
| `Choose Plan` | same as key | `选择套餐` | `Choisir le forfait` | `プランを選択` | `Выбрать тариф` | `Chọn gói` |

已有 key 可复用：

- `Subscription Plans`
- `Validity Period`
- `Monthly Token Limit`
- `Concurrency Limit`
- `View Pricing`
- `Go to Dashboard`
- `Create API`
- `Try Playground`
- `Choose a plan`
- `OpenCode-ready API help` 所在描述 key

实施后运行：

```bash
cd web/default
bun run i18n:sync
```

若 sync 生成报告文件变动，需要检查是否属于脚本正常输出；不要提交临时脚本。

## 7. 测试设计

### 7.1 Source-level 首页契约测试

新增文件建议：`web/default/src/features/home/home-page-copy.test.ts`

使用项目现有 `node:test` + `assert` + `readFileSync` 模式，避免引入 React Testing Library。

覆盖：

1. `Home` 默认组合使用 `PlansPreview`，不再使用 `Stats` / `Features`。
2. `plans-preview.tsx` 调用 `getPublicPlans`。
3. `plans-preview.tsx` 使用 `formatPlanPrice`、`formatDuration`、`formatTokenLimit`、`formatConcurrencyLimit`。
4. `plans-preview.tsx` 在无公开套餐时返回 `null`，避免空模块。
5. `Hero` 包含新标题 key，并保留 `to='/dashboard'` 与 `Go to Dashboard`。
6. `HeroTerminalDemo` 包含 `gpt-chat` 和 `responses`，不包含 `claude` / `gemini` demo。
7. `Features` / `Core Features` 不再出现在默认首页组合路径。

### 7.2 Quick Start 回归测试

保留并继续运行：

```bash
bunx tsx --test src/features/home/quick-start-copy.test.ts
```

不得破坏现有断言：

- `Create API`
- `Try Playground`
- `Choose a plan`
- `OpenCode-ready API help`

### 7.3 类型检查

修改 TS / TSX 后必须运行：

```bash
cd web/default
bun run typecheck
```

## 8. 验收清单

- [ ] 默认首页主标题在中文环境显示为「超便宜低价高速的GPT」。
- [ ] 已登录用户仍能从首页进入 `/dashboard`。
- [ ] API demo 标签只剩 Chat / Responses。
- [ ] 默认首页不再展示 `50+`、`100+`、`50+`、`10+` 数字统计。
- [ ] 原 Stats 位置展示公开套餐介绍。
- [ ] 套餐介绍来自 `getPublicPlans()`，不是硬编码价格或套餐名。
- [ ] 套餐卡展示套餐名、价格、有效期、月 Token 限额、并发限制。
- [ ] 无公开套餐或接口失败时，套餐介绍模块不渲染空壳。
- [ ] 默认首页不再渲染 `Core Features` / 「核心功能」模板模块。
- [ ] 三步快速上手仍与 Quick Start 保持一致。
- [ ] 新增文案已补齐六种 locale。
- [ ] 定向测试通过。
- [ ] `bun run typecheck` 通过。

## 9. 实施顺序建议

1. 修改 / 新增 source-level 测试，先让首页契约测试体现 Issue #6 期望。
2. 新建 `plans-preview.tsx`，读取 `getPublicPlans()` 并渲染最多三个套餐卡。
3. 更新 `Home` 默认组合：`Stats` → `PlansPreview`，移除 `Features`。
4. 更新 `components/index.ts` 导出。
5. 修改 Hero 主标题。
6. 修改 `HeroTerminalDemo`，只保留 Chat / Responses。
7. 清理无用 `stats.tsx`、`features.tsx`、`constants.ts` 中已无引用的 Stats / Features 数据。
8. 补齐 locale 并运行 `bun run i18n:sync`。
9. 运行定向测试与 `bun run typecheck`。

## 10. 风险与约束

- **公开接口加载失败：** 首页不能出现错误 toast 或破损卡片，模块静默隐藏。
- **自定义首页优先级：** 管理员配置 `home_page_content` 时，本次默认首页改造不会显示；这是现有行为，不能改变。
- **套餐数量过多：** 首页只展示前三个公开套餐，完整列表通过 `/pricing` 进入，避免首页过长。
- **i18n key 风格：** 当前 default 项目多用英文源文案作为 key；本规格沿用英文 key，并在 zh 中给出 Issue 指定中文。
- **受保护信息：** 不修改 `new-api` / `QuantumNous` 相关版权、品牌、归属信息。
- **现有未跟踪文件：** 主仓库存在其他未跟踪测试文件；Issue #6 必须在专用 worktree 中完成，避免污染或误删用户工作。
