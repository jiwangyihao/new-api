# Issue #22 窄验收修复状态

## 冻结基线
- 冻结 clean HEAD：`d5bba460f633ffd2943b1d13bb88b65cea338733`。
- 工作树：`jiwangyihao/issue-22-credit-tracer`。
- 启动时 `git status --short`：空。

## 当前阶段
- 阶段：Finding B 已 GREEN，提交安全点后运行窄回归。
- 已完成：真实 SQLite 32 CNY tracer 通过 `updated_at > snapshot_at` 触发 current-only；summary/users/subscriptions/plans/sources 五个现有入口均返回同一条结构化 warning（`section=credit_valuation`、`reason=current_only`），聚合布尔值保证多行去重；subscription 明细继续保留 `snapshot_semantics=current_only`、state version 与 updated_at；快照追平后 warning 为空。
- 下一步：提交 Finding B 安全点；运行既定后端 paid-value/32 CNY/controller 定向回归、前端 format/panel/page 定向测试与 typecheck、格式及 clean-tree 门禁。
- 阻塞：无。
- 最近安全提交：Finding B 本提交（提交后以 HEAD 为准）。

## 当前实现约束
- Finding A 只严格解析现有 `amount_micros`；禁止兼容 float 回退。
- 无效值只复用语义匹配的既有 `ErrCreditValuationSourceInvalid` / `ErrCreditValuationOverflow`，不新增 sentinel，不做 API 重构。
- Finding A 完成 RED→GREEN 并形成安全提交后，才处理 Finding B。

## 范围边界
- 不重做 CreditValuation、人民币余额、Kyren、BillingSession/request_id、32 CNY tracer、六语言或浏览器 smoke。
- 不实现 Issue #23–#28，不写 migration marker，不切换 `ready`，不实现 FX。
- 仅运行 model/controller 与实际相关前端定向门禁，不运行全仓测试。
