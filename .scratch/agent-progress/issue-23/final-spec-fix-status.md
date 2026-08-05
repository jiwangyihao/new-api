# Issue #23 最终 Spec F1/F2 修复状态

## 当前阶段

F1 已完成并提交 clean 安全点；当前进入 F2 RED。

## 已完成

- 冻结现场核验：分支、HEAD、clean 状态与 merge-base 均符合指令。
- 必读合同、最终 Spec FAIL 报告与历史证据已读取；诊断、TDD、模块设计与 Orca 调度约束已加载。
- F1 断言级 RED：旧实现对同 `request_id` 的 user、规范化 model、quota_type、distributor amount 冲突及缺指纹记录均错误返回成功。
- F1 GREEN：请求记录在预扣事务内持久化版本 1、SHA-256 确定性指纹；固定宽度大端编码覆盖 user/quota/amount，长度前缀覆盖规范化 model，无分隔符歧义。
- 同指纹重放严格无写入；异参、缺失或未知版本指纹通过 `ErrSubscriptionPreConsumeRequestConflict` 失败关闭。
- 双连接同 `request_id` 并发测试只接受一个创建成功，另一个同指纹幂等成功或稳定冲突；最终仅一条请求记录和一次 200 Credit 扣除。
- F1 三测单次、四测 `-count=10`、并发窄 `-race` 与附加式 SQLite schema 断言均已实际通过；精确命令记录于 evidence。

## 下一步

1. 为 Credit 匿名 token/amount helper 与 `PostConsumeQuota` 匿名绕路写最小运行时 RED。
2. 只实现稳定匿名拒绝 sentinel 与现有 request-aware 累计目标迁移，保留 timed/converted。

## 阻塞

无。

## 最近安全提交

- `0855b96b5`：F1 断言级 RED 与稳定 sentinel/附加字段声明。
- `07801e667`：F1 版本化不可变请求指纹、失败关闭、双连接并发与验证证据。
