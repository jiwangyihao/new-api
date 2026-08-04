# Issue #23 状态

## 当前阶段
- 阶段：请求目标增加、原快照恢复和欠额优先退款已通过；领域入口已 clean cutover。
- 当前定位：公开入口现名 `SettleUserSubscriptionRequestTarget`，下一步补齐 original subscription 身份、absorbed/unknown 与稳定终态错误。

## 已完成
- 已读取仓库与全局规则、父 PRD #19、Issue #23、执行合同、第二波次合同、`CONTEXT.md`、ADR 0001/0002、规格 5.4/6/7.3–7.5/9/11.3/13/14、计划任务 3/6、Issue #20/#22 合同。
- 已读取并服从 `skill://tdd`、`skill://diagnosing-bugs`、`skill://codebase-design`。
- 已确认 #22 合同包含 `CreditValuation` 深模块、购买来源快照、最小同步 request tracer 与五接口 DTO。
- 已确认起始 HEAD 和 merge-base 都是 `ec1858fec89509bdec9a90a230a8496047c5becd`，起始工作树干净。

## 下一步
1. 为追加欠额、欠额先撤销、absorbed restore 和后来 ingress 抵债后的 unknown 恢复分别补垂直 RED/GREEN。
2. 完成终态纠正规则、稳定错误和故障原子回滚。
3. 随后迁移同步/流式调用链、合并器、Task 身份和安全清理。

## 阻塞
- 无。socket closed 未改变 Dispatch、HEAD 或工作树语义。

## 最近安全提交
- `bfefe5c27`（少结算按原请求快照恢复）；当前入口 clean cutover 与欠额回归待提交。
