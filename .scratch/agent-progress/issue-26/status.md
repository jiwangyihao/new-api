# Issue #26 状态

## 当前状态

- 阶段：`UI_FROZEN_FACTS_GREEN`；既有钱包 conversion history 已展示冻结估值/FX/rule-state versions，组件测试、typecheck、build 与六语言 missing/extras 全部通过。
- 最近 clean SHA：`2687cde91`（`feat(issue-26): 暴露转换估值状态版本`）；当前仅 UI/types/六语言与 progress 证据待提交。
- 工作分支：`jiwangyihao/issue-26-conversion-fx`。
- 当前工作树：`C:/Users/34404/source/repos/new-api/.workspaces/new-api/issue-26-conversion-fx`。
- Orca parentWorktreeId：`1bd24578-ec8b-4492-961c-108ab229f4e7::C:/Users/34404/source/repos/new-api/.workspaces/new-api/credit-operational-value-integration`。
- 父分支/baseRef：`jiwangyihao/credit-operational-value-integration`。

## 下一条命令

`bun test src/features/subscription-conversion/components/timed-subscription-conversion-quotes-card.test.tsx`

下一步提交 UI GREEN 安全点，再启动真实 Go embed 应用并用 Orca 浏览器 smoke；不新增 UI/schema/端点。

## 未提交文件

- conversion card/types、六语言 locale、status/evidence；无其他生产文件。

## 上下文风险

- 当前有效 Dispatch：task `task_f80f4a22a9be`、dispatch `ctx_d834ee4a8128`；只通过注入的 Orca orchestration capability 与协调器通信。
- Orca runtime `ready`（`1.4.170`）；当前 worktree HEAD 与 Git 一致，parentWorktreeId 仍严格指向 `credit-operational-value-integration`。
- 现有可见 UI：`web/default/src/pages/wallet/index.tsx` 挂载 `TimedSubscriptionConversionQuotesCard`；组件展示 31 日块公式、quote preview/confirm 与 conversion history；`features/admin-analytics` 提供 conversion summary/history。
- 是否已完整展示冻结精确价格/currency/FX/micros/rule version 仍需通过 DTO、真实 API 与浏览器验收判定；若缺失，只补既有 API/UI 纵切，不创建新 UI。
- parser/provider 是唯一运行时 FX seam，禁止触碰 `float64 USDExchangeRate`；冻结 conversion 与在途 settlement 合同不得重写或“优化”。

## 恢复入口

1. 运行 `git status --short --branch` 与 `git rev-parse HEAD`，确认冻结起点 `c709ccb2c375031eabf43703334dffd44b39856a` clean。
2. 读取本目录 `contract.md`、`status.md`、`evidence.md`；不要重新探索规范。
3. 现有产品路径已判定为钱包 conversion quote/confirm/history 与管理员 conversion analytics；不得新建第二套 UI。
4. 下一步从真实 SQLite route tracer 开始，按本文件“下一条命令”执行。

## 2026-08-07 — Analytics/API/UI 收敛候选

- 恢复点：`8bd77ecdda6643326b5537933f9ec17c95e8b375`；开工时 staged `0`、unstaged `7`、untracked `0`，`git diff --check` 通过。
- 现有七文件候选已收敛：conversion summary 只聚合 committed `SubscriptionConversion` 冻结事实；五个付费价值接口只展示目标 Credit 混合池；route tracer 解析真实 JSON；前端仅改既有 `ConversionPanel`。
- 协调器独立实跑 `TestTimedConversionRealPathFeedsFiveAnalyticsWithoutNewPaymentAttribution` 与 `TestSubscriptionConversionRoutesExposeFrozenCrossCurrencyFactsAcrossHistoryAndAnalytics`，两项均 PASS。
- 本工作树前端门禁：受影响组件测试 `20 pass / 0 fail`；`bun run typecheck` 的 `tsc -b` 无诊断；`bun run i18n:sync` 六语言 missing/extras 均为 `0`；`bun run build` 为 `ready built in 13.9 s`。
- 六语言新增七个 conversion analytics 指标键，均为真实本地化文本；所有 Credit/micros 字段继续以十进制字符串传递，金额格式化沿用 `BigInt` 路径。
- 浏览器主路径与历史冻结证据继续由 `coordinator-browser-evidence.md` 提供；本轮按协调器收敛指令未另起服务或重复 Chromium smoke。
- 提交前候选共修改 13 个受控文件：原七文件加 en、zh、fr、ru、ja、vi 六个 locale；无 migration、marker、部署或其他 Issue 范围改动。
