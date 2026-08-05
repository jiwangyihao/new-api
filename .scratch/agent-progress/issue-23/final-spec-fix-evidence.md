# Issue #23 最终 Spec F1/F2 修复证据

## 冻结现场

命令：

```text
git branch --show-current
git rev-parse HEAD
git status --short
git merge-base HEAD ec1858fec89509bdec9a90a230a8496047c5becd
```

结果：

```text
jiwangyihao/issue-23-request-settlement
8cdfd4acb78b502af4c0232460baf7df852b7b2c
<git status --short 无输出>
ec1858fec89509bdec9a90a230a8496047c5becd
```

## 最终 Spec FAIL 复现依据

最终复评报告：`C:/Users/34404/AppData/Local/Temp/new-api-issue23-spec-final-rereview.md`。

- F1：既有 `request_id` 分支仅拒绝 refunded，随后返回旧结果，未比较本次调用的不可变参数。
- F2：`PostConsumeQuota` 仍可对 subscription 调用匿名 token delta，且导出 helper 未拒绝 Credit target。

## F1 根因

- 位置：`model/subscription.go` 的 `SubscriptionPreConsumeRecord` 与 `preConsumeUserSubscriptionByUnits`。
- 观察：记录没有版本化请求指纹；命中既有 request_id 后未核对 user/model/quota_type/distributor amount。
- 反馈循环：将通过公开 `PreConsumeUserSubscriptionByUnits` 与真实 SQLite 构造同 request_id 异参重放，断言稳定冲突与所有状态零写入。

## RED / GREEN / 回归

### F1 RED：公开预扣接口缺少请求指纹冲突合同

命令：

```text
go test ./model -run 'TestPreConsumeUserSubscriptionByUnits(RejectsConflictingRequestReplayWithoutWrites|ReplaysEquivalentNormalizedRequestWithoutWrites|RejectsMissingRequestFingerprintWithoutWrites)$' -count=1
```

结果：测试骨架编译 RED；仅证明稳定 sentinel 尚不存在，尚未验证旧实现的运行时冲突行为：

```text
# github.com/QuantumNous/new-api/model [github.com/QuantumNous/new-api/model.test]
model\credit_valuation_request_test.go:97:35: undefined: ErrSubscriptionPreConsumeRequestConflict
model\credit_valuation_request_test.go:146:33: undefined: ErrSubscriptionPreConsumeRequestConflict
FAIL github.com/QuantumNous/new-api/model [build failed]
```

测试骨架预期通过公开 `PreConsumeUserSubscriptionByUnits` 和真实 SQLite 覆盖四类冲突、完整参数重放、缺指纹失败关闭和持久化快照零写入；本次编译失败发生在执行前，因此这些运行时断言尚未得到验证。下一步只添加 sentinel 与附加式字段声明，使测试进入断言级 RED，再记录旧实现的精确行为。

### F1 RED：旧实现静默接受异参和缺指纹重放

仅添加导出 sentinel 与附加式字段声明、尚未加入任何比较/写入行为后，再次运行同一定向命令。

结果：断言级 RED。四类异参重放与缺指纹重放均错误返回 `nil`；等价规范化参数重放通过。`different user` 还观察到旧分支继续读取原权益并尝试刷新传入用户的缓存，但没有数据库写入。

```text
--- FAIL: TestPreConsumeUserSubscriptionByUnitsRejectsConflictingRequestReplayWithoutWrites
    --- FAIL: .../different_user
        Error: An error is expected but got nil.
    --- FAIL: .../different_normalized_model
        Error: An error is expected but got nil.
    --- FAIL: .../different_quota_type
        Error: An error is expected but got nil.
    --- FAIL: .../different_distributor_amount
        Error: An error is expected but got nil.
--- FAIL: TestPreConsumeUserSubscriptionByUnitsRejectsMissingRequestFingerprintWithoutWrites
    Error: An error is expected but got nil.
FAIL github.com/QuantumNous/new-api/model
```

该运行已通过真实 SQLite 和公开接口精确复现最终 Spec F1；失败发生在预期的稳定冲突断言，不是夹具、编译或邻近路径噪声。

后续 GREEN 与回归命令在实际运行后追加；未运行项不记为 PASS。

### F1 GREEN：完整参数、四类冲突与缺指纹失败关闭

命令：

```text
go test ./model -run 'TestPreConsumeUserSubscriptionByUnits(RejectsConflictingRequestReplayWithoutWrites|ReplaysEquivalentNormalizedRequestWithoutWrites|RejectsMissingRequestFingerprintWithoutWrites)$' -count=1
```

结果：PASS，`go test: 1 packages ok`。

验证行为：

- 完整规范化参数重放返回原请求结果，记录、权益、估值状态、版本和 ledger 均不变；
- user、规范化 model、quota_type、distributor amount 任一变化均满足 `errors.Is(err, ErrSubscriptionPreConsumeRequestConflict)`；
- 缺失或版本为 0 的指纹失败关闭，持久化快照不变。

### F1 GREEN：附加式 SQLite schema

命令：

```text
go test ./model -run 'Test(PreConsumeUserSubscriptionByUnits(RejectsConflictingRequestReplayWithoutWrites|ReplaysEquivalentNormalizedRequestWithoutWrites|RejectsMissingRequestFingerprintWithoutWrites)|CreditValuationSchemaSQLiteMigrationIsAdditiveAndRepeatable)$' -count=1
```

结果：PASS，`go test: 1 packages ok`。两次迁移均成功，`request_fingerprint_version` 与 `request_fingerprint` 列存在；未切换 migration marker、未回填历史。

### F1 GREEN：真实 SQLite 双连接同 request_id 并发

单次命令：

```text
go test ./model -run 'TestPreConsumeUserSubscriptionByUnitsConcurrentSameRequestHasSingleWrite$' -count=1
```

结果：PASS，`go test: 1 packages ok`。测试将 SQLite 连接池设为两个连接，并用事务起点屏障同时提交相同指纹：至少一个调用成功，另一个只允许同指纹幂等成功或 `ErrSubscriptionPreConsumeRequestConflict`；最终恰有一条 request record、一次 200 Credit 扣除、available=800、state_version=2。

重复与 race 命令：

```text
go test ./model -run 'TestPreConsumeUserSubscriptionByUnits(RejectsConflictingRequestReplayWithoutWrites|ReplaysEquivalentNormalizedRequestWithoutWrites|RejectsMissingRequestFingerprintWithoutWrites|ConcurrentSameRequestHasSingleWrite)$' -count=10
go test -race ./model -run 'TestPreConsumeUserSubscriptionByUnitsConcurrentSameRequestHasSingleWrite$' -count=1
```

结果：两条命令均 PASS，各输出 `go test: 1 packages ok`。

F1 实现仅增加版本 1 的确定性 SHA-256 指纹：固定宽度大端整数编码 user/quota/amount，长度前缀编码经 `FormatMatchingModelName` 规范化的 model；不使用 map、分隔字符串、时间、随机数、浮点或进程状态。

### F1 clean 安全点

- 提交：`07801e667`（`fix(issue-23): 绑定预扣请求不可变指纹`）。
- 提交后 `git status --short` 无输出。
- F1 至此冻结；后续只处理 F2，不再扩展 F1 schema、接口、缓存或重试。

### F2 RED：Credit 匿名 delta 与 PostConsumeQuota 绕路

命令：

```text
go test ./model ./service -run 'Test(CreditValuationAnonymousSubscriptionDeltasAreForbidden|PostConsumeQuotaCreditUsesStableRequestTarget)$' -count=1
```

结果：RED。

```text
model\subscription_anonymous_delta_test.go:51:28: undefined: ErrCreditValuationAnonymousDeltaForbidden
FAIL github.com/QuantumNous/new-api/model [build failed]
--- FAIL: TestPostConsumeQuotaCreditUsesStableRequestTarget
    expected AppliedCredit: 150
    actual AppliedCredit:   100
FAIL github.com/QuantumNous/new-api/service
```

分类：model 测试当前只证明稳定匿名拒绝 sentinel 尚不存在，尚未运行 helper 的零写入断言；service 运行时测试已证明 `PostConsumeQuota` 将 `token_used` 匿名增加，却没有把同一 `request_id` 的累计目标从 100 更新为 150，请求快照仍停在 100。

### F2 RED：匿名 helper 运行时未拒绝 Credit

加入稳定 sentinel 声明但尚未添加 helper 门禁后运行：

```text
go test ./model -run 'TestCreditValuationAnonymousSubscriptionDeltasAreForbidden$' -count=1
```

结果：token delta 与 amount delta 两个子测试均 RED，`Expected error ... but got nil`；旧实现实际修改了 Credit 数量或 amount 路径而未返回稳定错误。

### F2 GREEN：匿名拒绝、request-aware target 与兼容边界

单次命令：

```text
go test ./model ./service -run 'Test(CreditValuationAnonymousSubscriptionDeltasAreForbidden|TimedSubscriptionAnonymousDeltasRemainCompatible|PostConsumeQuotaCreditUsesStableRequestTarget)$' -count=1
```

结果：PASS，`go test: 2 packages ok`。

- token/amount 两个匿名 helper 在任何写入或入队前读取目标权益；`credit_balance` 返回 `ErrCreditValuationAnonymousDeltaForbidden`，权益、估值状态、版本、请求记录和 ledger 快照零写入。
- timed token/amount 匿名兼容分别成功更新 `token_used` 与 `amount_used`。
- `PostConsumeQuota` 单次调用最初读取已提交 request record 并复用 `SettleUserSubscriptionRequestTarget`；100 预扣 + 50 增量得到 applied=150、available=850、exact=34,000,000 micros、state_version=3、单一 request record。后续相同调用重放缺陷及最小修复见下节。

重复、race 与 converted/timed 回归命令：

```text
go test ./model ./service -run 'Test(CreditValuationAnonymousSubscriptionDeltasAreForbidden|TimedSubscriptionAnonymousDeltasRemainCompatible|PostConsumeQuotaCreditUsesStableRequestTarget)$' -count=10
go test -race ./model ./service -run 'Test(CreditValuationAnonymousSubscriptionDeltasAreForbidden|PostConsumeQuotaCreditUsesStableRequestTarget)$' -count=1
go test ./model -run 'TestConvertedSubscription' -count=1
go test ./service -run 'TestPostConsumeQuota(SubscriptionDoesNotConsumeTokenKeyQuota|LegacySubscriptionUsesAmountUsed)$' -count=1
```

结果：四条命令均 PASS，依次输出 `go test: 2 packages ok`、`go test: 2 packages ok`、`go test: 1 packages ok`、`go test: 1 packages ok`。converted source 路由与 timed token/amount 兼容未回归。

### F2 匿名 helper 调用点审计

修改导出符号前及最终收敛时的 LSP references 已记录：token helper 13 个引用，amount helper 10 个引用；新增差异只来自回归测试引用，生产调用边界未扩大。

- `service/quota.go`：Credit 已迁移到 `request_id + pre_consumed + delta` 的稳定累计目标入口；仅非 Credit 调用匿名 helper。
- `service/funding_source.go`：已跟踪 Credit 先走 `settleCreditRequestTarget`；未跟踪 Credit 落到 helper 时由稳定 sentinel 失败关闭；timed 保持兼容。
- `service/billing_session.go`：兼容退款 helper 只用于非 Credit request-aware 分支；误入 Credit 时由 helper 失败关闭。
- `service/task_billing.go`：Credit target 使用 request-aware/legacy task 入口；converted source 继续允许原映射路径；amount helper 保留 timed 路径。
- `model/subscription.go`：`PostConsumeUserSubscriptionDelta` 是 token helper 别名，因此继承相同 Credit 拒绝；converted source 内部路由保持原逻辑。
- 其余引用均为 model/controller 测试；最终包门禁会捕获仍假定 Credit 匿名写入成功的旧测试，并仅按 F2 合同迁移测试夹具，不放宽生产门禁。

结论：production service/controller/relay/Task 不存在可成功写入 Credit target 的匿名 helper 绕路；保留的成功匿名路径仅为 timed 与 converted source 明确兼容边界。

### F2 follow-up RED/GREEN：PostConsumeQuota 相同调用重放

紧急范围裁决只允许修复 `2bb68e770` 中 `applied_credit + delta` 导致的同 request 重放累加。测试保持第一次调用后的业务断言，再以同一 `RelayInfo`、同一 request_id、相同 `quota=50` 和 `preConsumedQuota=100` 调用第二次，并分别从 SQLite 重新读取 request record、权益和估值状态。

旧行为 RED 命令：

```text
go test ./service -run '^TestPostConsumeQuotaCreditUsesStableRequestTarget$' -count=1
```

旧行为精确变化：

- request record：`applied_credit 150 -> 200`、`deducted_available_credit 150 -> 200`、`deducted_exact_cost_micros 6,000,000 -> 8,000,000`、`settlement_version 2 -> 3`；
- `UserSubscription.token_used 150 -> 200`；
- `CreditValuationState`：`available_credit 850 -> 800`、`exact_cost_micros 34,000,000 -> 32,000,000`、`state_version 3 -> 4`。

三份差异由同一 RED 使用三次数据库 reload 和非短路 `assert.Equal` 一次性输出；失败不是夹具或邻近噪声。

最小 GREEN 仅把累计目标基准从可变 `record.AppliedCredit` 改为不可变 `record.PreConsumed`，并让既有溢出比较使用相同基准。保留 `final=false`、`delta != 0` 外层、`SubscriptionPostDelta += delta` 和所有其他生产行为。命令：

```text
go test ./service -run '^TestPostConsumeQuotaCreditUsesStableRequestTarget$' -count=1
```

结果：PASS，`go test: 1 packages ok`；第二次相同调用后 request record、权益和估值状态逐字段不变。

调用点边界：`ChargeViolationFeeIfNeeded` 在 `service/violation_fee.go` 中于普通结算/退款后调用 `PostConsumeQuota`。终态 Credit 请求继续由既有 finalized conflict fail-closed；本修复不设计终态后追加费协议、不生成新 request_id、不放宽终态冲突。该后续领域设计不属于 F1/F2。

### F2 clean 安全点

- `2bb68e770`（`fix(issue-23): 禁止 Credit 匿名结算`）：匿名 helper 门禁与初始 request-aware `PostConsumeQuota` 迁移。
- `45b9d64f4`（`fix(issue-23): 保持同步结算重放幂等`）：累计目标改为不可变 `pre_consumed + delta`，同调用重放零写入。
- `dc333c928`（`test(issue-23): 迁移 Credit 结算回归夹具`）：仅迁移被新门禁揭露的旧测试夹具。
- 每个生产安全点提交前对应反馈循环均 GREEN；未扩展违规费协议或 #24–#28。

### 最终冻结门禁

稳定性与 race：

```text
go test ./model -run 'Test(PreConsumeUserSubscriptionByUnits(RejectsConflictingRequestReplayWithoutWrites|ReplaysEquivalentNormalizedRequestWithoutWrites|RejectsMissingRequestFingerprintWithoutWrites|ConcurrentSameRequestHasSingleWrite)|CreditValuationSchemaSQLiteMigrationIsAdditiveAndRepeatable)$' -count=10
go test -race ./model -run 'TestPreConsumeUserSubscriptionByUnitsConcurrentSameRequestHasSingleWrite$' -count=1
go test ./model ./service -run 'Test(CreditValuationAnonymousSubscriptionDeltasAreForbidden|TimedSubscriptionAnonymousDeltasRemainCompatible|PostConsumeQuotaCreditUsesStableRequestTarget)$' -count=10
go test -race ./model ./service -run 'Test(CreditValuationAnonymousSubscriptionDeltasAreForbidden|PostConsumeQuotaCreditUsesStableRequestTarget)$' -count=1
```

结果：四条命令均 PASS，依次为 `go test: 1 packages ok`、`1 packages ok`、`2 packages ok`、`2 packages ok`。

请求领域与真实 SQLite 链路：

```text
go test ./model ./service -run 'Test(CreditValuationRequest|CreditRequestTargetCoalescer|CreditTask|LegacyCreditTask|CleanupSubscriptionPreConsumeRecords|PreviewSubscriptionPreConsumeCleanup|SubscriptionBillingReserveDoesNotDoubleCountCompatibilityFields|PostConsumeQuotaCreditUsesStableRequestTarget)' -count=1
go test ./model ./service ./controller -run 'Test(CreditValuationRequestPreConsumeRemovesMovingAverageCost|CreditValuationRequestFinalizesSameTargetIdempotently|CreditBillingSessionRefundUsesStableRequestTarget|CreditTaskInitialSettlementPersistsNonFinalRequestIdentity|CreditTaskFailureRefundReusesInitialBillingRequestIdentity|CreditValuationFiveAnalyticsPanels|SubscriptionKyrenCreditWebhookCompletesFromSnapshotWithoutInvitation|ExternalCreditPurchaseWebhookAndRefundLifecycle|PostConsumeQuotaCreditUsesStableRequestTarget)' -count=1
```

结果：分别 PASS，`go test: 2 packages ok` 与 `go test: 3 packages ok`。后者通过公开领域/Service/Controller 入口覆盖 request_id 预扣、追加、少结算/退款、Task 重放、Kyren 冻结来源、800 available / 32,000,000 micros CNY 与五接口一致性。

宽回归与相关窄 race：

```text
go test ./model ./service ./controller -count=1
go test -race ./model ./service -run 'Test(CreditRequestTargetCoalescer|CleanupSubscriptionPreConsumeRecordsSerializesWithTerminalTaskReplays|CreditValuationAnonymousSubscriptionDeltasAreForbidden|PostConsumeQuotaCreditUsesStableRequestTarget|CreditTaskInitialSettlementPersistsNonFinalRequestIdentity|CreditTaskFailureRefundReusesInitialBillingRequestIdentity)' -count=1
git diff --check
```

结果：三包宽回归 PASS（`go test: 3 packages ok`）；窄 race PASS（`go test: 2 packages ok`）；`git diff --check` 无输出。修改 Go 文件已执行 `gofmt`。

三包回归前曾精确发现旧测试假定 Credit 匿名 helper 可成功、timed helper 零读取或 Credit 夹具缺 ready state；仅迁移这些测试到冻结 request-aware/entitlement-guard 合同，生产门禁未放宽。输出中允许出现测试预期的 GORM record-not-found/唯一约束诊断和 Redis closed 日志，最终包结果均为 PASS。

### Issue #23 acceptance 映射

- AC1：F1 指纹与预扣请求记录同事务；相同参数无写入重放，四类冲突/缺指纹稳定拒绝，双连接仅一次写。
- AC2–6：累计目标增加/减少、原快照恢复、债务与 absorbed/unknown、逐请求 coalescer 聚焦门禁 PASS。
- AC7：Credit 匿名 helper 统一拒绝；`PostConsumeQuota` 复用 `request_id + original subscription + pre_consumed + delta` 的稳定累计目标，相同调用重放严格无写入；timed/converted 兼容 PASS。
- AC8–12：Task request identity、legacy Task、conversion routing seam、cleanup、稳定错误与并发/race 聚焦门禁 PASS。
- #22 冻结 tracer：三包宽回归 PASS；未修改 ingress、移动平均、analytics DTO/current_only 或 32 CNY 合同。

未运行真实 MySQL/PostgreSQL、全项目套件、部署；不冒充 #27/#28 验收。#26 继续消费既有 `valuation_subscription_id` 路由 seam，本修复未实现 conversion FX/单位价值/虚拟快照。
