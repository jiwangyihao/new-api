# Issue #21 状态

## 当前阶段

领域 GREEN：公开 timed 授予 tracer 已通过，准备提交后追加冲突/续期行为。

## 已完成

- 确认隔离工作树 `jiwangyihao/issue-21-timed-grants` 与父集成工作树均位于 `53c91e6e3a795b01b4c426c9a69ff532cd8712c8`。
- 确认工作树初始 clean，未从 `origin/main` 取代码，未重做 Issue #20。
- 按指定顺序读取父 PRD #19、Issue #21、执行合同、Wave 1 共享合同、Issue #20 消费合同、领域上下文、ADR 0001/0002、新规格相关章节与实施计划任务 5/8/9。
- 首个真实 SQLite RED→GREEN 已完成：显式权威 micros/原币种、确定性 source identity、同事务 grant、相同来源重放不续期。

## 下一步

1. 提交首个 timed 授予 tracer 安全点。
2. 追加参数冲突与第二个稳定来源续期追加 grant 的行为测试。
3. 逐条迁移订单、兑换与管理员调用点。

## 阻塞

无。Issue #22 通用 DTO 尚未落地不阻塞此 tracer；后续仅保留最窄 timed seam 并记录，不等待、不复制 Credit 实现。

## 最近安全提交

- `4f6768767 docs(issue-21): 收敛首个领域 tracer`。
