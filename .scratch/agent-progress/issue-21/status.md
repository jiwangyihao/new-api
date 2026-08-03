# Issue #21 状态

## 当前阶段

准备：基线、规范与领域合同已确认，准备进入首个 TDD tracer。

## 已完成

- 确认隔离工作树 `jiwangyihao/issue-21-timed-grants` 与父集成工作树均位于 `53c91e6e3a795b01b4c426c9a69ff532cd8712c8`。
- 确认工作树初始 clean，未从 `origin/main` 取代码，未重做 Issue #20。
- 按指定顺序读取父 PRD #19、Issue #21、执行合同、Wave 1 共享合同、Issue #20 消费合同、领域上下文、ADR 0001/0002、新规格相关章节与实施计划任务 5/8/9。
- 读取并采用 `skill://tdd` 与 `skill://codebase-design`。

## 下一步

1. 定位现有计时权益创建、续期、订单履约、兑换、管理员授予与分析接缝。
2. 写一个通过公开领域入口观察 grant 与续期幂等行为的真实 SQLite RED 测试。
3. 最小实现 `TimedSubscriptionGrantRequest` 与 `GrantTimedSubscriptionTx`，再逐条扩展购买、兑换、管理员与续期行为。

## 阻塞

无。若通用分析 DTO 或共享前端金额组件需要 Issue #22 尚未落地的接口，将只建立 timed 最窄接缝并通过 Orca 报告协调器。

## 最近安全提交

- `99a7ce6f5 docs(issue-21): 建立计时估值恢复合同`。
