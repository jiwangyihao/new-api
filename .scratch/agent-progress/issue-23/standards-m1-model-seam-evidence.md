# Issue #23 Standards M1 模型接缝修复证据

## 冻结现场

开始修改前执行：

```text
git branch --show-current
git rev-parse HEAD
git status --short
git merge-base HEAD ec1858fec89509bdec9a90a230a8496047c5becd
```

结果：

```text
jiwangyihao/issue-23-request-settlement
a72fe0416f30230971701fa8e36f6c42d1d0c998
<git status --short 无输出>
ec1858fec89509bdec9a90a230a8496047c5becd
```

## RED

新增公开 Model tracer `TestPostConsumeUserSubscriptionRequestDeltaRoutesCreditAndTimed`，使用真实 SQLite 与公开接口覆盖：

- Credit 预扣 100 后追加 50，期望 `PostDelta=50`、`ReplacePostDelta=true`；
- 相同 request、subscription、delta 重放返回同一结果，request record、`UserSubscription`、`CreditValuationState` 均不变；
- 缺 request 与原 subscription 映射冲突返回既有 sentinel，且相关持久化快照零写入；
- target 为负、checked-add 溢出、目标结算状态缺失分别传播既有 sentinel，且事务零写入；
- timed distributor 返回 `ReplacePostDelta=false`，复用既有 token delta 行为。

首次运行：

```text
go test ./model -run '^TestPostConsumeUserSubscriptionRequestDeltaRoutesCreditAndTimed$' -count=1
```

编译 RED：`PostConsumeUserSubscriptionRequestDelta` 与 `UserSubscriptionPostConsumeResult` 尚未定义。仅添加最小接口声明和无操作实现后再次运行；首次断言尝试暴露 tracer 快照错误地对 `CreditValuationState` 使用 `ORDER BY id`，修正为其主键 `user_subscription_id` 后得到断言级 RED：

```text
Credit target：expected {PostDelta:50 ReplacePostDelta:true}, actual {PostDelta:0 ReplacePostDelta:false}
missing request：expected ErrCreditValuationRequestNotFound, got nil
mapping conflict：expected ErrCreditValuationMappingConflict, got nil
timed distributor：expected {PostDelta:5 ReplacePostDelta:false}, actual {PostDelta:0 ReplacePostDelta:false}
FAIL github.com/QuantumNous/new-api/model
```

该断言级 RED 证明 tracer 能同时捕获 Credit 路由、稳定错误、结果模式与非 Credit 兼容行为，不以编译失败代替运行时反馈。

## GREEN 与接口选择

在 `model/subscription.go` 增加单一公开深模块接口：

```go
type UserSubscriptionPostConsumeResult struct {
    PostDelta        int64
    ReplacePostDelta bool
}

func PostConsumeUserSubscriptionRequestDelta(requestId string, userSubscriptionId int, delta int64, distributor bool) (UserSubscriptionPostConsumeResult, error)
```

Model 隐藏 entitlement 与 request record 查询：

- Credit 读取不可变 `record.PreConsumed`，验证 `record.UserSubscriptionId`，通过 checked add 形成 `pre_consumed + delta`；
- checked-add 回绕返回 `ErrCreditValuationOverflow`，负 target 返回 `ErrCreditValuationNegativeInput`；
- 调用既有 `SettleUserSubscriptionRequestTarget(requestId, userSubscriptionId, target, false)`，其既有 sentinel 原样传播；
- Credit 成功返回稳定的 `PostDelta=target-record.PreConsumed` 与 `ReplacePostDelta=true`；
- 非 Credit 复用既有 token/amount helper，成功返回 `PostDelta=delta` 与 `ReplacePostDelta=false`；
- 新增查询的 record-not-found、映射与数据库错误分别收敛为 `ErrCreditValuationRequestNotFound`、`ErrCreditValuationMappingConflict` 与 Model sentinel `ErrDatabase`，不把 GORM 错误暴露到 Service。

`service/quota.go` 的 `PostConsumeQuota` 现在只传递 request ID、subscription ID、delta 与 distributor 标志；Model 成功后才按结果执行 Credit 替换或非 Credit 累加。Service 已移除 `gorm.io/gorm`、`model.DB`、`gorm.ErrRecordNotFound`、`SubscriptionPreConsumeRecord` 与 `UserSubscription` 查询细节。`delta == 0` 的既有外层跳过行为、`final=false`、通知、违规费、退款、`BillingSession`、coalescer、converted 路由与缓存/锁/重试策略均未改变。

## 错误、事务与重放证据

Tracer 通过数据库 reload 验证 100 预扣 + 50 post delta 后：

```text
request.pre_consumed = 100
request.applied_credit = 150
subscription.token_used = 150
valuation.available_credit = 850
valuation.exact_cost_micros = 34000000
result = {PostDelta:50 ReplacePostDelta:true}
```

相同调用重放后，result 相同，request record、subscription、valuation 三份对象逐字段不变。缺 request、映射冲突、负 target、溢出与状态缺失均在返回空结果时保持调用前快照不变；状态缺失精确传播 `ErrCreditValuationStateMissing`，证明 Model 没有通过有限 allowlist 改写既有 target sentinel。

现有 Service 测试继续保留首次和重放后的数据库 reload，并继续断言两次调用后 `SubscriptionPostDelta == 50`；未删除或放宽既有断言。

## 聚焦门禁

最终实现上实际运行：

```text
go test ./model -run '^TestPostConsumeUserSubscriptionRequestDeltaRoutesCreditAndTimed$' -count=1
```

结果：PASS，`go test: 1 packages ok`。

```text
go test ./model -run '^TestPostConsumeUserSubscriptionRequestDeltaRoutesCreditAndTimed$' -count=10
```

结果：PASS，`go test: 1 packages ok`。

```text
go test ./service -run '^TestPostConsumeQuotaCreditUsesStableRequestTarget$' -count=10
```

结果：PASS，`go test: 1 packages ok`。

```text
go test ./model ./service -run "Test(CreditValuationAnonymousSubscriptionDeltasAreForbidden|TimedSubscriptionAnonymousDeltasRemainCompatible|PostConsumeQuotaCreditUsesStableRequestTarget|PostConsumeUserSubscriptionRequestDeltaRoutesCreditAndTimed)$" -count=10
```

结果：PASS，`go test: 2 packages ok`。

```text
go test -race ./model -run '^TestPostConsumeUserSubscriptionRequestDeltaRoutesCreditAndTimed$' -count=1
go test -race ./service -run '^TestPostConsumeQuotaCreditUsesStableRequestTarget$' -count=1
```

结果：两条命令均 PASS，各为 `go test: 1 packages ok`。

```text
go test ./model ./service -count=1
```

结果：PASS，`go test: 2 packages ok`。

仅对本次修改的 Go 文件运行：

```text
gofmt -w model/subscription.go model/subscription_post_consume_test.go service/quota.go
git diff --check
```

结果：`gofmt` 与 `git diff --check` 均无输出。

## 未运行边界

严格按 M1 范围未运行 controller、全项目套件、前端、真实 MySQL 5.7/PostgreSQL 9.6、服务启动或部署。未复评或重新设计 F1、coalescer、Task、cleanup、double-count、违规费、退款、`BillingSession`、#24–#28；未修改 schema、迁移、缓存、锁或重试策略。
