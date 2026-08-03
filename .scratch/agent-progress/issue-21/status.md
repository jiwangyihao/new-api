# Issue #21 状态

## 当前阶段

恢复交接 WIP：四处 nullable 金额接线已修复，指定 SQLite tracer 已从编译失败恢复到业务断言 RED；当前失败为 CNY `expected 10, actual 0`，说明 timed grant calculator 尚未接入 paid row 与五接口。

## 已完成

- 已保留 `ccd516aaa test(analytics): 保存计时 grant 时间线 RED` 作为此前可恢复 RED 安全点。
- 已修复 `model/admin_analytics_paid_subscription.go` 四处 nullable singular 值/指针兼容问题，并只格式化该文件。
- 已运行最窄命令 `go test ./model -run '^TestPaidSubscriptionValueUsesTimedGrantTimelineAcrossFiveViews$' -count=1`。
- 编译已恢复；真实 SQLite fixture 进入业务断言，当前准确结果为 CNY `expected 10, actual 0`。
- 未继续 UI、六语言、浏览器、Credit 核心、FX 或 marker/ready 工作。

## 下一步

1. 将 `adminCalculateTimedSubscriptionValue` 的逐币种结果接入 paid row，不再读取当前 Plan 价格。
2. 让 summary/users/subscriptions/plans/sources 复用同一 timed row，并保持跨币种 singular 为 null。
3. 第一条验证命令仍为 `go test ./model -run '^TestPaidSubscriptionValueUsesTimedGrantTimelineAcrossFiveViews$' -count=1`。
4. GREEN 后再逐个补重叠、裁剪和来源行为 tracer。

## 阻塞

- 当前阻塞是 timed calculator 尚未接入 paid row，不是外部依赖。
- 已知业务 RED 为 CNY `expected 10, actual 0`；Plan 的当前 `999 EUR` 仍错误主导旧行构建。

## 最近安全提交

- 此前安全点：`ccd516aaa test(analytics): 保存计时 grant 时间线 RED`。
- 当前恢复状态由紧随其后的明确 WIP 提交承载；SHA 通过 escalation 交接。
