# Issue #21 状态

## 当前阶段

领域 GREEN：购买与兑换真实入口均原子写 timed grant；兑换码改价/改币种历史已冻结。

## 已完成

- 确认隔离工作树 `jiwangyihao/issue-21-timed-grants` 与父集成工作树均位于 `53c91e6e3a795b01b4c426c9a69ff532cd8712c8`。
- 确认工作树初始 clean，未从 `origin/main` 取代码，未重做 Issue #20。
- 按指定顺序读取父 PRD #19、Issue #21、执行合同、Wave 1 共享合同、Issue #20 消费合同、领域上下文、ADR 0001/0002、新规格相关章节与实施计划任务 5/8/9。
- 首个真实 SQLite RED→GREEN 已完成：显式权威 micros/原币种、确定性 source identity、同事务 grant、相同来源重放不续期。

## 下一步

1. 提交兑换 timed grant 安全点。
2. 通过管理员 HTTP handler 写 RED，要求 reason/idempotency 并迁移售后授予。
3. 补 disabled plan、试用/邀请排除与不可变更新/删除行为。

## 阻塞

无。Issue #22 通用 DTO 尚未落地不阻塞此 tracer；后续仅保留最窄 timed seam 并记录，不等待、不复制 Credit 实现。

## 最近安全提交

- `ff635938c feat(subscription): 为计时订单冻结估值授予`。
