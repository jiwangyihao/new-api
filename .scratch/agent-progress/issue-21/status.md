# Issue #21 状态

## 当前阶段

计时五接口最窄接线已 GREEN：paid row 只读不可变 grant 时间线，summary/users/subscriptions/plans/sources 统一返回 CNY/USD，当前 Plan 的 `999 EUR` 不再进入计时估值。

## 已完成

- 已保留 `ccd516aaa test(analytics): 保存计时 grant 时间线 RED` 与 `226d3c76d fix(analytics): 恢复计时五接口业务 RED` 两个可恢复安全点。
- 已将 `adminCalculateTimedSubscriptionValue` 接入 timed paid row，并按 grant 来源、币种和窗口投影。
- summary/users/subscriptions/plans/sources 复用同一 row；跨币种 subscription singular 为 null，`*_by_currency` 返回精确 micros。
- 当前 Plan 的 `999 EUR` 不参与计时金额；测试断言 CNY 10、USD 5 且无 EUR。
- 指定真实 SQLite tracer 已 GREEN；未扩展 UI、Credit、FX 或 marker。

## 下一步

1. 在新 RED→GREEN 周期补重叠窗口去重、失效裁剪和 missing grant warning。
2. 再完成管理员 reason/key UI、六语言与浏览器证据。
3. 保持 Credit 核心、FX、marker/ready 为明确非所有权。

## 阻塞

- 当前没有外部阻塞。
- 下一行为缺口是重叠/裁剪/warning 的公开 tracer；五接口主路径已 GREEN。

## 最近安全提交

- 上一安全点：`226d3c76d fix(analytics): 恢复计时五接口业务 RED`。
- 当前 GREEN 由紧随其后的提交承载。
