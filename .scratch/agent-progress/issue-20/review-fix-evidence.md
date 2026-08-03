# Issue #20 Standards 评审修复证据

## 基线证据

- `git rev-parse HEAD && git status --short --branch`
  - HEAD：`9e3329d0f4b509d1179c895c52f01af7a19f0ca4`
  - 分支：`jiwangyihao/issue-20-valuation-foundation`
  - staged 0、unstaged 0、untracked 0。
- 已读取父 PRD #19、Issue #20、ADR 0002、规格、计划、共享执行文档、Issue #20 指令、协调器验收清单及 Standards 完整报告。
- 已加载 `diagnosing-bugs`、`tdd`、`codebase-design`、`shadcn-ui`、`orca-cli` 与 `orchestration` 技能。

## Finding 1：历史精确价格污染

### 待复现症状

历史有价套餐的 `price_amount_micros` 为 NULL 时，仅修改名称、状态或其他非价格字段，提交 payload 不得从 JavaScript `number` 推导 micros，数据库精确列必须继续为 NULL。

### RED / GREEN

- RED（前端）：`bun test src/features/subscriptions/lib/plan-form.test.ts --test-name-pattern "does not promote"`；`0 pass / 1 fail`，`'price_amount' in payload.plan` 实际为 `true`，证明表单把兼容 Number 显示值重新提升为提交权威值。
- RED（后端）：`go test ./controller -run TestAdminUpdateSubscriptionPlanPreservesLegacyPriceWhenPriceFieldsAreAbsent -count=1`；失败，历史 `price_amount` 从 `40.123456` 被覆盖为 `0`，说明后端当前无法区分更新请求中价格字段缺失与显式零。
- 根因：`planToFormValues` 把历史 `price_amount` Number 字符串化但不保留来源权威性；`formValuesToPlanPayload` 无条件生成 micros；`decodeAdminUpsertSubscriptionPlanRequest` 与更新 map 又无条件写两列。
- 修复：表单显式保存 `new` / `exact` / `legacy` 来源与 `price_amount_changed`，只在新建或用户明确输入时提交原始十进制文本及由 `BigInt` 生成的 micros；后端保留价格字段存在性，仅在 update 请求提供价格时写两列。
- GREEN（前端）：`bun test src/features/subscriptions/lib/plan-form.test.ts`；`13 pass / 0 fail`。覆盖历史无关编辑、非权威显示标记、`0`、`Number.MAX_SAFE_INTEGER`、`0.1 + 0.2`、显式六位小数及 `int64` 最大边界。
- GREEN（后端）：`go test ./controller -run "TestAdmin(Create|Update)SubscriptionPlan.*ExactPrice|TestAdminUpdateSubscriptionPlanPreservesLegacyPriceWhenPriceFieldsAreAbsent" -count=1`；通过。数据库历史 `price_amount` 保持 `40.123456` 且 micros 继续为 NULL，显式更新继续精确往返。
- 反证：创建有价套餐仍由现有规范化逻辑要求 micros；显式零继续作为提供过的价格处理；没有 `toFixed`、Number 反推或容差旁路。

## Finding 2：schema fail-open

### 待复现症状

确认旧兼容 `price_amount` 扩宽是否为 #20 前向合同必需；若不必需，删除高风险 ALTER 并证明权威 micros 与历史 NULL 合同不受影响；若必需，则用行为测试证明错误传播。

### RED / GREEN

待记录。

## Finding 3：计划级线性化

### 待复现症状

并发修改 `valuation_currency` 与创建首个 Credit 权益时，只允许两个串行结果之一，不得两者均成功并留下权益存在但币种被普通接口修改的状态。

### RED / GREEN

待记录。

## 外部数据库边界

尚未检测 `TEST_MYSQL_DSN` / `TEST_POSTGRES_DSN`；没有真实 DSN 时必须记录 SKIP，不宣称三库实测通过。
