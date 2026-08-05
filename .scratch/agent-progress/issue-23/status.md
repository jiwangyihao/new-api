# Issue #23 状态

## 当前阶段
- 阶段：恢复现场已核对；追加目标、少结算原快照、欠额优先退款及 original subscription identity 定向回归保持 GREEN，正在提交恢复安全点。
- 当前定位：公开入口 `SettleUserSubscriptionRequestTarget(request_id, original_subscription_id, target_applied_credit, final)` 已 clean cutover；下一步补齐 absorbed/unknown 与稳定终态错误。

## 已完成
- 已读取仓库与全局规则、父 PRD #19、Issue #23、执行合同、第二波次合同、`CONTEXT.md`、ADR 0001/0002、规格 5.4/6/7.3–7.5/9/11.3/13/14、计划任务 3/6、Issue #20/#22 合同。
- 已读取并服从 `skill://tdd`、`skill://diagnosing-bugs`、`skill://codebase-design`。
- 已确认 #22 合同包含 `CreditValuation` 深模块、购买来源快照、最小同步 request tracer 与五接口 DTO。
- 已确认起始 HEAD 和 merge-base 都是 `ec1858fec89509bdec9a90a230a8496047c5becd`，起始工作树干净。

## 下一步
1. 提交当前五个代码/测试文件及本次恢复证据为安全恢复点。
2. 为 absorbed restore 和后来 ingress 抵债后的 unknown 恢复分别补垂直 RED/GREEN。
3. 完成终态纠正规则、稳定错误和故障原子回滚，再迁移同步/流式调用链、合并器、Task 身份和安全清理。

## 阻塞
- 无。恢复时精确保留并核对五个 dirty 文件，未执行清理、覆盖、切分或重做探索。

## 最近安全提交
- 本恢复记录所在提交（提交前 HEAD：`c146e5aad7a77bd4520ddf2dc9e7ab7a77ac4aa3`）；包含五个恢复现场文件、original subscription identity clean cutover 及定向 GREEN 证据。
