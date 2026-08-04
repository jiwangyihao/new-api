# Issue #21 夹具迁移 C 状态

状态：INVESTIGATING

## 冻结现场

- 分支：`jiwangyihao/issue-21-fixture-c-controller`
- 冻结基线：`774b35740c1879b285537031410731317d0142fc`
- 初始工作树：clean
- 所有权：仅迁移 `controller` 中余额、Kyren、Stripe、Epay/通用支付 webhook、邀请订单及其共享测试 helper。
- 最近安全提交：冻结基线 `774b35740c1879b285537031410731317d0142fc`；本文件首次提交后更新。
- 当前未提交文件：本次首次创建的 `fixture-c-status.md`、`fixture-c-evidence.md`、`fixture-c-contract.md`。

## 已完成

- 已读取并服从仓库/全局规则、Issues #19/#21/#22、共享夹具迁移合同、执行协议、Wave 1 合同、Issue #21 acceptance、`CONTEXT.md`、ADR 0002、2026-08-02 spec 的订单快照/支付/计时 grant/disabled-plan 段，以及冻结树 `spec-fix-*`、`final-spec-fix-*`。
- 已读取 `diagnosing-bugs`、`tdd`、`codebase-design`、Orca orchestration/CLI 指南。
- 已确认不得放宽生产 fail-closed、不得修改 `model`/`service`/前端/生产支付与履约代码。

## 当前阶段

- 下一步：运行 `go test ./controller -count=1`，按余额、Kyren、Stripe、Epay/通用支付、邀请订单分组记录原始 RED、panic 与缺表日志。
- 随后逐组以完整权威 Plan 或合法订单授权快照迁移夹具，并执行最小 RED→GREEN。

## 阻塞

- 无。
