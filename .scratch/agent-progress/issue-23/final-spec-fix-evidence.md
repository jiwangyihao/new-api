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
