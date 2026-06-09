# Codex Pro Dashboard 用量概览迁移实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 将 Codex Pro 三态设置从钱包订阅卡片迁移到 Dashboard 的用量概览卡片中，并将三个大按钮改为三段式 switch/segmented control，同时保持现有可见性、不可用状态、资格提示和保存语义。

**架构：** 抽出可复用的 Codex Pro mode control 组件与纯 helper，Dashboard 用量概览组件负责读取 `/api/subscription/self`、保存 `PUT /api/subscription/self/codex-pro-mode` 并渲染该控件；钱包订阅卡片移除 Codex Pro UI 和对应状态逻辑。测试迁移到共享控件与 Dashboard 位置，保留现有 i18n key。

**技术栈：** React 19、TypeScript、Bun、i18next、Base UI/Tailwind、项目统一 `api` 实例。

---

## 文件结构

- 创建：`web/default/src/features/subscriptions/components/codex-pro-mode-control.tsx`
  - 职责：导出 `CODEX_PRO_MODE_TITLE_KEY`、`CODEX_PRO_MODE_OPTIONS`、`canUseCodexProModeControl`、`getCodexProUnavailableMessageKey`、`getCodexProModeFailureRollback`、`normalizeCodexProMode`、`normalizeCodexProUnavailableReason` 和 `CodexProModeControl` 组件。
  - 组件只负责显示和触发 `onModeChange`，不直接请求 API。
- 创建：`web/default/src/features/subscriptions/components/codex-pro-mode-control.test.ts`
  - 职责：覆盖三态选项、不可用 reason、失败回滚、可用 off 可切回、segmented control 类名/禁用语义的纯逻辑。
- 修改：`web/default/src/features/wallet/components/subscription-plans-card.tsx`
  - 职责：移除 Codex Pro 查询状态、保存 handler、UI 区块和相关 imports；保留套餐购买、订阅列表、重置额度逻辑。
- 修改：`web/default/src/features/wallet/components/subscription-plans-card.test.ts`
  - 职责：移除 Codex Pro helper imports 与测试，钱包测试只保留套餐来源和额度重置。
- 修改：`web/default/src/features/dashboard/components/models/log-stat-cards.tsx`
  - 职责：在现有用量概览卡片底部加载 self subscription data，渲染 `CodexProModeControl`；保存成功刷新 self subscription data，保存失败回滚并 toast。
- 修改：`web/default/src/features/wallet/codex-pro-i18n.test.ts`
  - 职责：保留 i18n 合约测试，可按文件语义改名/迁移到 subscriptions；如果不改名，则更新 imports 指向 shared component。
- 修改：`web/default/src/i18n/static-keys.ts` 与 locale JSON
  - 职责：仅当新增 label 或 aria 文案时补齐六语言；优先复用既有 key，不扩大文案。

---

### 任务 1：红灯测试迁移目标位置和控件契约

**文件：**
- 创建：`web/default/src/features/subscriptions/components/codex-pro-mode-control.test.ts`
- 修改：`web/default/src/features/wallet/components/subscription-plans-card.test.ts`
- 修改：`web/default/src/features/wallet/codex-pro-i18n.test.ts`

- [ ] **步骤 1：编写失败测试**

在 shared component 测试中断言：

```ts
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import {
  CODEX_PRO_MODE_OPTIONS,
  CODEX_PRO_MODE_TITLE_KEY,
  canUseCodexProModeControl,
  getCodexProModeFailureRollback,
  getCodexProUnavailableMessageKey,
} from './codex-pro-mode-control'
import type { CodexProMode, CodexProUnavailableReason } from '@/features/subscriptions/types'

describe('Codex Pro mode control contract', () => {
  test('offers a segmented three-state switch contract', () => {
    assert.equal(CODEX_PRO_MODE_TITLE_KEY, 'Codex Pro')
    assert.deepEqual(
      CODEX_PRO_MODE_OPTIONS.map((option) => option.value),
      ['all', 'flexible', 'off'] satisfies CodexProMode[]
    )
    assert.deepEqual(
      CODEX_PRO_MODE_OPTIONS.map((option) => option.labelKey),
      ['All', 'Flexible', 'Off']
    )
  })

  test('keeps eligible off mode selectable so users can switch back', () => {
    assert.equal(canUseCodexProModeControl({ codex_pro_eligible: true }), true)
  })

  test('maps ineligible reasons to action-oriented copy', () => {
    const cases: Array<[CodexProUnavailableReason, string]> = [
      ['wallet_only', 'Your current billing preference will not create a subscription billing session.'],
      ['trial_subscription', 'Trial subscriptions do not support Codex Pro.'],
      ['reward_subscription', 'Invitation reward subscriptions do not support Codex Pro.'],
      ['no_paid_subscription', 'Please purchase an eligible paid subscription first.'],
    ]
    for (const [reason, expected] of cases) assert.equal(getCodexProUnavailableMessageKey(reason), expected)
  })

  test('rolls failed optimistic updates back to previous mode', () => {
    assert.deepEqual(getCodexProModeFailureRollback({ previousMode: 'flexible', requestedMode: 'all' }), {
      mode: 'flexible',
      messageKey: 'Request failed',
    })
  })
})
```

从 wallet 测试中删除 Codex Pro helper import 和 `wallet Codex Pro mode control contract` describe，保证这些 helper 不再由 wallet 组件导出。

更新 i18n 测试 import 到 shared component。

- [ ] **步骤 2：运行红灯测试验证失败**

运行：

```bash
cd web/default && bun test src/features/subscriptions/components/codex-pro-mode-control.test.ts src/features/wallet/components/subscription-plans-card.test.ts src/features/wallet/codex-pro-i18n.test.ts
```

预期：失败，原因是 `codex-pro-mode-control` 文件尚未存在或 wallet import 仍指向旧文件。

---

### 任务 2：抽出 shared Codex Pro 控件并清理钱包页

**文件：**
- 创建：`web/default/src/features/subscriptions/components/codex-pro-mode-control.tsx`
- 修改：`web/default/src/features/wallet/components/subscription-plans-card.tsx`
- 修改：`web/default/src/features/wallet/components/subscription-plans-card.test.ts`
- 修改：`web/default/src/features/wallet/codex-pro-i18n.test.ts`

- [ ] **步骤 1：实现 shared helper 和组件**

`CodexProModeControl` props：

```ts
interface CodexProModeControlProps {
  data: Pick<SelfSubscriptionData, 'codex_pro_mode' | 'codex_pro_eligible' | 'codex_pro_unavailable_reason'>
  saving?: boolean
  onModeChange: (mode: CodexProMode) => void
}
```

组件使用三段式 segmented control：一个 `div` + 三个 `button type="button"`，使用 `aria-pressed={selected}`，禁用条件为 `!canUseCodexProModeControl(data) || saving`。选中项用 `bg-background text-foreground shadow-sm`，未选中项用 `text-muted-foreground hover:text-foreground`。

保留当前显示语义：标题、Available/Not available badge、三条说明、weak intent code、不可用原因。

- [ ] **步骤 2：从钱包页移除 Codex Pro 逻辑**

删除 wallet component 中这些内容：

- `updateCodexProMode` import；
- `CodexProMode` / `CodexProUnavailableReason` type import；
- `CODEX_PRO_MODE_TITLE_KEY` / `CODEX_PRO_MODE_OPTIONS` / helper 定义；
- `savingCodexProMode` state；
- `handleCodexProModeChange`；
- `codexProControlAvailable` / `codexProUnavailableMessageKey`；
- JSX 中 `selfSubscriptionData &&` 的 Codex Pro 区块。

保留 `selfSubscriptionData`，因为钱包页仍需要 subscriptions、all_subscriptions、active_subscription_id 和 quota reset 刷新。

- [ ] **步骤 3：运行共享控件和钱包测试**

运行：

```bash
cd web/default && bun test src/features/subscriptions/components/codex-pro-mode-control.test.ts src/features/wallet/components/subscription-plans-card.test.ts src/features/wallet/codex-pro-i18n.test.ts
```

预期：通过。

---

### 任务 3：Dashboard 用量概览卡片接入控件

**文件：**
- 修改：`web/default/src/features/dashboard/components/models/log-stat-cards.tsx`
- 可选创建：`web/default/src/features/dashboard/components/models/log-stat-cards-codex-pro.test.ts`

- [ ] **步骤 1：编写失败测试**

如果新增纯逻辑 helper，例如：

```ts
export function shouldShowCodexProInUsageOverview(data: SelfSubscriptionData | null): boolean {
  return data !== null
}
```

则创建测试断言 `null` 不显示，非 null 显示。若直接组件渲染不适合当前测试框架，则用 shared component 测试覆盖显示语义，用 dashboard 手动/类型验证覆盖位置。

- [ ] **步骤 2：实现 Dashboard 数据加载与保存**

在 `log-stat-cards.tsx` 增加：

- imports：`useCallback`、`useTranslation`、`toast`、`getSelfSubscriptionFull`、`updateCodexProMode`、`CodexProModeControl`、`canUseCodexProModeControl`、`getCodexProModeFailureRollback`、`normalizeCodexProMode`、`normalizeCodexProUnavailableReason`、type `SelfSubscriptionData` / `CodexProMode`。
- state：`selfSubscriptionData`、`loadingSelfSubscription`、`savingCodexProMode`。
- `fetchSelfSubscription`：调用 `getSelfSubscriptionFull()`，成功后 normalize 后保存。
- `useEffect`：组件初始化时并行或独立加载 self subscription，不影响现有 stats loading。
- `handleCodexProModeChange`：复用 wallet 原逻辑，乐观更新、调用 `updateCodexProMode`、成功刷新、失败回滚并 toast。

在现有用量概览卡片 `<div className='overflow-hidden rounded-lg border'>` 中，统计 grid 之后追加：

```tsx
{selfSubscriptionData && (
  <div className='border-t p-3 sm:p-4'>
    <CodexProModeControl
      data={selfSubscriptionData}
      saving={savingCodexProMode || loadingSelfSubscription}
      onModeChange={handleCodexProModeChange}
    />
  </div>
)}
```

- [ ] **步骤 3：运行 Dashboard / shared 控件相关测试**

运行：

```bash
cd web/default && bun test src/features/subscriptions/components/codex-pro-mode-control.test.ts src/features/wallet/components/subscription-plans-card.test.ts src/features/wallet/codex-pro-i18n.test.ts
```

预期：通过。

---

### 任务 4：i18n、类型检查、审查和提交

**文件：**
- 修改：`web/default/src/i18n/static-keys.ts`
- 修改：`web/default/src/i18n/locales/{en,zh,fr,ja,ru,vi}.json`（仅在新增 key 时）

- [ ] **步骤 1：运行 i18n 同步**

运行：

```bash
cd web/default && bun run i18n:sync
```

预期：`i18n sync done`。若新增 key 缺少翻译，补齐六语言。

- [ ] **步骤 2：运行前端相关测试**

运行：

```bash
cd web/default && bun test src/features/subscriptions/components/codex-pro-mode-control.test.ts src/features/subscriptions/codex-pro-mode-api.test.ts src/features/wallet/components/subscription-plans-card.test.ts src/features/wallet/codex-pro-i18n.test.ts src/features/api-help/lib/usage-config.test.ts src/features/api-help/api-key-loading.test.ts
```

预期：全部通过。

- [ ] **步骤 3：运行类型检查**

运行：

```bash
cd web/default && bun run typecheck
```

预期：当前 Codex Pro 相关代码无类型错误。若仍出现 unrelated `src/features/gpt-abuse/lib/filters.ts(50,7): error TS6133`，记录为非本次变更遗留，不能修改 unrelated gpt-abuse 文件。

- [ ] **步骤 4：只读审查**

派发至少 3 个 reviewer 子代理，只读审查：

1. Dashboard 位置和 UI 语义；
2. shared component / wallet cleanup；
3. i18n / 类型 /测试覆盖。

所有 review pass 后再提交。

- [ ] **步骤 5：提交**

仅 stage 本次迁移相关文件。不要 stage unrelated GPT abuse / forgot-password / sidebar / reports 文件。

提交信息：

```bash
git commit -m "feat(codex-pro): 将模式控制移入用量概览"
```
