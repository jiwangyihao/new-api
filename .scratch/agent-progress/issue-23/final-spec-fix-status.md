# Issue #23 最终 Spec F1/F2 修复状态

## 当前阶段

F1 已 GREEN 并完成真实 SQLite 双连接并发、`-count=10` 与窄 `-race` 验证，待格式检查与 clean 安全提交。

## 已完成

- 冻结现场核验：分支、HEAD、clean 状态与 merge-base 均符合指令。
- 必读合同、最终 Spec FAIL 报告与历史证据已读取；诊断、TDD、模块设计与 Orca 调度约束已加载。
- F1 断言级 RED：旧实现对同 `request_id` 的 user、规范化 model、quota_type、distributor amount 冲突及缺指纹记录均错误返回成功。
- F1 GREEN：请求记录在预扣事务内持久化版本 1、SHA-256 确定性指纹；固定宽度大端编码覆盖 user/quota/amount，长度前缀覆盖规范化 model，无分隔符歧义。
- 同指纹重放严格无写入；异参、缺失或未知版本指纹通过 `ErrSubscriptionPreConsumeRequestConflict` 失败关闭。
- 双连接同 `request_id` 并发测试只接受一个创建成功，另一个同指纹幂等成功或稳定冲突；最终仅一条请求记录和一次 200 Credit 扣除。
- F1 三测单次、四测 `-count=10`、并发窄 `-race` 与附加式 SQLite schema 断言均已实际通过；精确命令记录于 evidence。

## 下一步

1. 对修改 Go 文件运行 `gofmt`，执行 `git diff --check` 并提交 F1 clean 安全点。
2. F1 提交完成后进入 F2 RED；不再扩展 F1 schema、接口、缓存或通用重试。

## 阻塞

无。

## 最近安全提交

- `0855b96b5`：F1 断言级 RED 与稳定 sentinel/附加字段声明。
- F1 GREEN 提交：待创建。
