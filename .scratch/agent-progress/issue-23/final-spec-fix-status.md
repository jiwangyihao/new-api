# Issue #23 最终 Spec F1/F2 修复状态

## 当前阶段

准备完成，进入 F1 RED。

## 已完成

- 冻结现场核验：分支、HEAD、clean 状态与 merge-base 均符合指令。
- 读取父 PRD #19、Issue #23、依赖 #22、执行合同、Wave 2 合同、Issue #23 指令/验收、最终 Spec FAIL 报告、历史进度证据、`CONTEXT.md`、ADR 与 2026-08-02 spec/plan 相关合同。
- 已加载 `diagnosing-bugs`、`tdd`、`codebase-design` 与 Orca orchestration 指南。
- 根因定位：`preConsumeUserSubscriptionByUnits` 的既有记录分支未绑定调用不可变参数。

## 下一步

1. 通过公开 `PreConsumeUserSubscriptionByUnits` 和真实 SQLite 写 F1 RED。
2. 实现附加式持久化指纹、稳定 sentinel 与 fail-closed 重放。
3. 完成 F1 单次、重复、并发、故障注入和 race 验证并提交安全点。
4. 仅在 F1 安全提交后进入 F2。

## 阻塞

无。

## 最近安全提交

起始安全点：`8cdfd4acb78b502af4c0232460baf7df852b7b2c`。
