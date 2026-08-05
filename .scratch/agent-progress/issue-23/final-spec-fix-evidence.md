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
