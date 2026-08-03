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

待记录。

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
