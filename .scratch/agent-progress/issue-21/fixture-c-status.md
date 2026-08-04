# Issue #21 夹具迁移 C 状态

状态：HANDOFF_READY

## 冻结现场

- 分支：`jiwangyihao/issue-21-fixture-c-controller`
- 冻结基线：`774b35740c1879b285537031410731317d0142fc`
- 初始工作树：clean
- 所有权：仅迁移 `controller` 中余额、Kyren、Stripe、Epay/通用支付 webhook、邀请订单及其共享测试 helper。
- 最近安全提交：`ab12c1c2a`（迁移邀请订单授权快照夹具）；最终证据提交后更新。
- 当前未提交文件：仅本状态/证据最终收口；提交后为 0。

## 已完成

- 已读取并服从仓库/全局规则、Issues #19/#21/#22、共享夹具迁移合同、执行协议、Wave 1 合同、Issue #21 acceptance、`CONTEXT.md`、ADR 0002、2026-08-02 spec 的订单快照/支付/计时 grant/disabled-plan 段，以及冻结树 `spec-fix-*`、`final-spec-fix-*`。
- 已读取 `diagnosing-bugs`、`tdd`、`codebase-design`、Orca orchestration/CLI 指南。
- 已确认不得放宽生产 fail-closed、不得修改 `model`/`service`/前端/生产支付与履约代码。

## 当前阶段

- 余额、Kyren、Stripe、Epay、邀请订单夹具全部迁移并按组 GREEN。
- 聚焦 controller 正则回归、关键 provider/重放 `-count=10`、完整 `go test ./controller -count=1` 均 PASS。
- 下一步：`git diff --check`、提交最终证据、确认 clean tree、发送唯一 `worker_done`。

## 阻塞

- 无。Redis 仅观察到既有 client-closed 缓存日志，无 panic 或测试失败；缺表仅来自显式故障注入场景。
