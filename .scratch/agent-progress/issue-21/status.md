# Issue #21 状态

## 当前阶段

恢复交接 WIP：`timed_subscription_analytics.go` 已存在，但 paid row 与五接口仍未完成最窄接线。2026-08-03 指定 SQLite tracer 在编译阶段失败，未进入业务断言；当前不得视为 GREEN 或完成。

## 已完成

- 已保留 `ccd516aaa test(analytics): 保存计时 grant 时间线 RED` 作为此前可恢复 RED 安全点。
- 已按协调器收敛要求仅对 `dto/admin_analytics.go` 与 `model/admin_analytics_paid_subscription.go` 执行 `gofmt -w`。
- 已运行最窄命令 `go test ./model -run '^TestPaidSubscriptionValueUsesTimedGrantTimelineAcrossFiveViews$' -count=1`。
- 当前准确结果为编译 RED：subscription singular 字段已改为指针，但 row builder 仍写入值，sorter 仍把指针传给只接受值的 helper。
- 未继续 UI、六语言、浏览器、Credit 核心、FX 或 marker/ready 工作。

## 下一步

1. 只修复 `model/admin_analytics_paid_subscription.go:855-856,999` 的四处值/指针不兼容，不再扩展 DTO。
2. 第一条验证命令仍为 `go test ./model -run '^TestPaidSubscriptionValueUsesTimedGrantTimelineAcrossFiveViews$' -count=1`。
3. 编译恢复后，继续完成 `ccd516aaa` RED 所需的最窄 paid-row/summary/users/subscriptions/plans/sources 接线；不得回退到当前 Plan 价格。
4. UI、六语言与浏览器证据交给后续 Agent，当前提交明确为 WIP。

## 阻塞

- 当前阻塞是四处本地编译错误，不是外部依赖：两处 `dto.AdminAnalyticsMoneyAmount` 值不能赋给指针字段，两处 sorter 将指针传给值参数。
- 最近一次运行未到达 SQLite fixture 或业务断言；此前已知业务 RED 仍是 CNY `expected 10, actual 0`。

## 最近安全提交

- 此前安全点：`ccd516aaa test(analytics): 保存计时 grant 时间线 RED`。
- 当前恢复状态由紧随其后的明确 WIP 提交承载；SHA 通过 escalation 交接。
