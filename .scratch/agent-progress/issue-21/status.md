# Issue #21 状态

## 当前阶段

分析 RED：timed 时间线计算器已落盘但尚未接入五接口，当前只形成可恢复安全点，不宣称 GREEN。

## 已完成

- 确认隔离工作树 `jiwangyihao/issue-21-timed-grants` 与父集成工作树均位于 `53c91e6e3a795b01b4c426c9a69ff532cd8712c8`。
- 确认工作树初始 clean，未从 `origin/main` 取代码，未重做 Issue #20。
- 按指定顺序读取父 PRD #19、Issue #21、执行合同、Wave 1 共享合同、Issue #20 消费合同、领域上下文、ADR 0001/0002、新规格相关章节与实施计划任务 5/8/9。
- 领域定向测试保持 GREEN；分析新增真实 SQLite RED `TestPaidSubscriptionValueUsesTimedGrantTimelineAcrossFiveViews`，证明旧五接口仍按当前 Plan 的 EUR 估值而非 grant 时间线。

## 下一步

1. 保持 `model/timed_subscription_analytics.go` 为独立最窄计算器，修复/确认 duplicate helper 后运行编译与单个 RED。
2. 下一步仅把 calculator 接入 paid row 与五接口；跨币种 singular null、source breakdown、warning 仍未完成。
3. 分析 GREEN 后再进入管理员 UI 与六语言；当前不得视为完成。

## 阻塞

无外部阻塞；Issue #22 通用 DTO 尚未集成，当前 `dto/admin_analytics.go` 仅含 timed 最小增量，集成时需保留 #22 通用骨架优先权。

## 最近安全提交

- `14361af41 fix(subscription): 规范化计时授予来源身份`。
