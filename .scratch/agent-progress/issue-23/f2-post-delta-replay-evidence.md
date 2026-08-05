# Issue #23 F2 post-delta 重放收敛证据

## 冻结现场

开始修改前核验：

```text
git branch --show-current
git rev-parse HEAD
git status --short
git merge-base HEAD ec1858fec89509bdec9a90a230a8496047c5becd
```

结果：

```text
jiwangyihao/issue-23-request-settlement
3cc5608f88b395057efc7abac04b93965866c1aa
<git status --short 无输出>
ec1858fec89509bdec9a90a230a8496047c5becd
```

## RED

只在 `service/post_consume_quota_credit_test.go` 的第二次相同 `PostConsumeQuota(relayInfo, 50, 100, false)` 调用及三份数据库 reload/逐字段不变断言之后，新增 `relayInfo.SubscriptionPostDelta == 50` 可观察合同。第一次调用后的同值断言保持不变，测试继续复用同一 `RelayInfo`、同一 `request_id`，没有手工归零或跳过第二次调用。

命令：

```text
go test ./service -run ^TestPostConsumeQuotaCreditUsesStableRequestTarget$ -count=1
```

旧冻结实现精确 RED：

```text
--- FAIL: TestPostConsumeQuotaCreditUsesStableRequestTarget
    post_consume_quota_credit_test.go:73:
        Error:      Not equal:
                    expected: 50
                    actual  : 100
FAIL github.com/QuantumNous/new-api/service
```

该断言之前的 request record、`UserSubscription`、`CreditValuationState` 三份 reload 与逐字段不变断言均已通过；唯一漂移是同一 `RelayInfo.SubscriptionPostDelta` 被第二次匿名 `+= 50`，由 50 变为 100。

## GREEN

最小生产修复仅修改 `service/quota.go` 的 `PostConsumeQuota` Credit 分支：仍以真实 `RequestId`、原 `SubscriptionId` 和 `record.PreConsumed + delta` 调用 `SettleUserSubscriptionRequestTarget(..., false)`；成功后将 `SubscriptionPostDelta` 赋为 `target - record.PreConsumed`，表达该 request 的实际 post-consume adjustment。相同调用重放时该值稳定为 50；timed/converted/legacy 非 Credit 分支仍执行原有 `+= delta`。

所有映射冲突、缺记录、目标溢出、负目标和 request-aware 结算错误仍在修改 `RelayInfo` 前返回；未改变 `final=false`、违规费协议、退款入口、通知逻辑、`BillingSession`、模型接口、schema、错误 sentinel 或 #26 seam。

聚焦验证：

```text
go test ./service -run ^TestPostConsumeQuotaCreditUsesStableRequestTarget$ -count=1
go test ./service -run ^TestPostConsumeQuotaCreditUsesStableRequestTarget$ -count=10
go test -race ./service -run ^TestPostConsumeQuotaCreditUsesStableRequestTarget$ -count=1
```

结果：三条命令均 PASS，均输出 `go test: 1 packages ok`。

## 未运行边界

按任务边界未运行 formatter、lint、三包宽回归、全项目测试、前端、真实 MySQL 5.7/PostgreSQL 9.6、服务启动或部署；未复评 F1、coalescer、Task、cleanup、double-count，也未触碰或实现 #24、#25、#26–#28。
