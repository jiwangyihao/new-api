# Issue #21 夹具迁移 C 状态

状态：RED_CAPTURED

## 冻结现场

- 分支：`jiwangyihao/issue-21-fixture-c-controller`
- 冻结基线：`774b35740c1879b285537031410731317d0142fc`
- 初始工作树：clean
- 所有权：仅迁移 `controller` 中余额、Kyren、Stripe、Epay/通用支付 webhook、邀请订单及其共享测试 helper。
- 最近安全提交：`173bea6b6`（建立 controller 夹具迁移恢复现场）。
- 当前未提交文件：`fixture-c-status.md`、`fixture-c-evidence.md`（包级冻结 RED 证据）。

## 已完成

- 已读取并服从仓库/全局规则、Issues #19/#21/#22、共享夹具迁移合同、执行协议、Wave 1 合同、Issue #21 acceptance、`CONTEXT.md`、ADR 0002、2026-08-02 spec 的订单快照/支付/计时 grant/disabled-plan 段，以及冻结树 `spec-fix-*`、`final-spec-fix-*`。
- 已读取 `diagnosing-bugs`、`tdd`、`codebase-design`、Orca orchestration/CLI 指南。
- 已确认不得放宽生产 fail-closed、不得修改 `model`/`service`/前端/生产支付与履约代码。

## 当前阶段

- 已运行 `go test ./controller -count=1`：27 个失败测试，29 次 `timed_subscription_grant_invalid`，无 panic；已按余额/共享购买、Kyren、Stripe、Epay、邀请订单分组。
- 下一步：审阅现有 controller 测试 helper 与合法订单快照构造入口，先迁移余额/共享购买夹具并执行最小 RED→GREEN，再迁移已授权 provider order 与邀请订单。
- 缺表日志当前均来自无关局部 setup 或显式故障注入；Redis 仅有 client closed 日志，无 panic。

## 阻塞

- 无。
