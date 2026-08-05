# Issue #23 状态

## 当前阶段
- 阶段：`COMPLETE`；Task identity 最终文件状态已通过完整定向 `count=10`、窄 `-race` 与 `git diff --check`，旧 Dispatch capability 已失效，不发送旧 `worker_done`。
- 当前结果：新 Task 持久化 `subscription_request_id`；真实 Task 初始 BillingSession reserve/settle 保持请求 route 非终态；轮询追加、成功 final、失败 refund 与重放复用同一身份；legacy Task 使用稳定 `legacy-task:<task_pk>` 并仅在数量归属可证明时构造 unknown 快照，否则失败关闭；timed Task 兼容条件保持不变。

## 已完成
- 已读取仓库与全局规则、父 PRD #19、Issue #23、执行合同、第二波次合同、`CONTEXT.md`、ADR 0001/0002、规格 5.4/6/7.3–7.5/9/11.3/13/14、计划任务 3/6、Issue #20/#22 合同。
- 已读取并服从 `skill://tdd`、`skill://diagnosing-bugs`、`skill://codebase-design`。
- 已确认 #22 合同包含 `CreditValuation` 深模块、购买来源快照、最小同步 request tracer 与五接口 DTO。
- 已确认起始 HEAD 和 merge-base 都是 `ec1858fec89509bdec9a90a230a8496047c5becd`，起始工作树干净。

## 下一步
1. 保持已提交 Task identity 安全点，等待协调器使用新 Dispatch capability 继续清理/最终交付。
2. 工作树中仅剩并行清理阶段的 `.scratch/agent-progress/issue-23/cleanup-status.md` 与 `model/subscription_preconsume_cleanup_test.go`；本安全点不暂存、不覆盖。

## 阻塞与风险
- 当前 Task identity 无技术阻塞；旧 Dispatch capability 已 revoked，不能发送有效 `worker_done`。
- 最终完整 regex 的 `go test ./service ... -count=10` 与 `go test -race ./service ... -count=1` 均为 `1 packages ok`，`git diff --check` 无输出。
- 清理实现、SQLite/API smoke 与最终 worker 交付仍需新 Dispatch 续作；不在失效 Dispatch 中越界执行。

## 最近安全提交
- `e83551e89`（Task 生命周期现场）及其前置 `d9e620191`、`578551963`、`55e5c50f6`、`ea016089a`；最终验证后的状态/证据将单独提交安全点。


## 2026-08-05 最终完成
- 请求记录清理合同已交付：终态资格、排他 cutoff、Task 投影与引用保护、历史 NULL fail-closed、稳定有界 batch、幂等、失败原子性、真实 SQLite 并发串行化、只读诊断、审计事实保留。
- 最终聚焦 model/service/controller 门禁及 cleanup/coalescer/Task identity 窄 race 均通过。
- 未运行真实 MySQL/PostgreSQL、全项目测试或部署；这些结果未声称通过。
