# Issue #21 夹具迁移 C 状态

状态：BALANCE_KYREN_GREEN

## 冻结现场

- 分支：`jiwangyihao/issue-21-fixture-c-controller`
- 冻结基线：`774b35740c1879b285537031410731317d0142fc`
- 初始工作树：clean
- 所有权：仅迁移 `controller` 中余额、Kyren、Stripe、Epay/通用支付 webhook、邀请订单及其共享测试 helper。
- 最近安全提交：`4529dfb93`（记录 controller 夹具冻结失败）；余额/Kyren GREEN 提交完成后更新。
- 当前未提交文件：共享 controller 测试 helper、余额/Kyren 测试调用点及本状态/证据更新。

## 已完成

- 已读取并服从仓库/全局规则、Issues #19/#21/#22、共享夹具迁移合同、执行协议、Wave 1 合同、Issue #21 acceptance、`CONTEXT.md`、ADR 0002、2026-08-02 spec 的订单快照/支付/计时 grant/disabled-plan 段，以及冻结树 `spec-fix-*`、`final-spec-fix-*`。
- 已读取 `diagnosing-bugs`、`tdd`、`codebase-design`、Orca orchestration/CLI 指南。
- 已确认不得放宽生产 fail-closed、不得修改 `model`/`service`/前端/生产支付与履约代码。

## 当前阶段

- 已完成共享权威 timed Plan/订单快照测试 helper；余额真实购买入口与 Kyren 已授权 webhook 定向 GREEN。
- 下一步：提交余额/Kyren 安全点，再复用同一 helper 迁移 Stripe、Epay 与邀请订单。
- Redis 仅有 client closed 日志，无 panic；缺表均为显式故障注入测试。

## 阻塞

- 无。
