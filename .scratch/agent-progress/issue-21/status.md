# Issue #21 状态

## 当前阶段

领域 RED：接缝探索已按协调器指令收敛，立即通过公开 timed 授予入口建立真实 SQLite tracer。

## 已完成

- 确认隔离工作树 `jiwangyihao/issue-21-timed-grants` 与父集成工作树均位于 `53c91e6e3a795b01b4c426c9a69ff532cd8712c8`。
- 确认工作树初始 clean，未从 `origin/main` 取代码，未重做 Issue #20。
- 按指定顺序读取父 PRD #19、Issue #21、执行合同、Wave 1 共享合同、Issue #20 消费合同、领域上下文、ADR 0001/0002、新规格相关章节与实施计划任务 5/8/9。
- 读取并采用 `skill://tdd` 与 `skill://codebase-design`；确认现有 schema 已注册不可变 grant，低层 timed 创建函数返回实际服务窗口。

## 下一步

1. 编写真实 SQLite RED：公开 `GrantTimedSubscriptionTx` 同事务创建权益和 grant，相同来源重放不续期。
2. 只实现 RED 所需的最小 `TimedSubscriptionGrantRequest`、确定性 source identity 与 grant 写入。
3. RED→GREEN 后立即提交，再逐条接入订单、兑换和管理员入口。

## 阻塞

无。Issue #22 通用 DTO 尚未落地不阻塞此 tracer；后续仅保留最窄 timed seam 并记录，不等待、不复制 Credit 实现。

## 最近安全提交

- `f60433f52 docs(issue-21): 记录恢复合同核验`。
