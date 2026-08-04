# Issue #21 邀请身份重放修复状态

状态：HANDOFF_READY

## 冻结现场

- 工作树：`C:/Users/34404/source/repos/new-api/.workspaces/new-api/issue-21-wide-model-fix`
- 分支：`jiwangyihao/issue-21-wide-model-fix`
- 起始 HEAD：`86b49a724e32b1dfea3b43a25f73e03efb8584b7`
- 稳定 RED 提交：`7b9e0038e`
- clean handoff 证据提交：`86b49a724`
- 进度基线提交：`d5ad009c4`
- 生产修复提交：`1786fd9b015dac7213efbb999cd7035c29398cc4`
- 最近安全提交：`1786fd9b0`（恢复成功订单重放邀请身份）。

## 完成结果

- 在 `subscriptionOrderCompletionResultFromTimedGrantTx` 已恢复 immutable timed grant 与 subscription/window 后，按订单唯一来源身份读取既有 `InvitationRewardEvent`。
- 唯一 event 同时匹配 `SourceOrderId`、订单 `FulfilledSubscriptionID`、invitee 与有效 inviter 时，只把持久化 `InviterId` 合入返回结果。
- event 不存在时保持 `InviterId=0`；多行、来源不匹配或非法身份使用既有稳定 `ErrTimedSubscriptionGrantInvalid` fail closed。
- 重放路径不创建或更新 event、subscription、timed grant、ledger、订单或授权快照，也不读取 current Plan。
- 首次履约、邀请事件创建、佣金计算及 #21/#22 既有合同未修改。

## 修改文件

- `model/subscription.go`
- `.scratch/agent-progress/issue-21/invitation-replay-fix-status.md`
- `.scratch/agent-progress/issue-21/invitation-replay-fix-evidence.md`
- `.scratch/agent-progress/issue-21/invitation-replay-fix-contract.md`

未修改测试断言；复用 `7b9e0038e` 已提交的两个稳定 RED 与既有无 event 重放测试。

## 验证结果

- 两个成功重放测试单次：PASS。
- 两个成功重放测试 `-count=10`：PASS。
- 无 invitation event 的 paid timed 成功重放：PASS；event 仍为 0，subscription/grant 数量与 immutable fulfillment 结果不变。
- 九项 invitation fixture 组合：PASS。
- `go test ./model -count=1`：PASS。
- 三项重放接缝 `go test -race`：PASS。
- `gofmt -w model/subscription.go`：完成。
- `git diff --check`：PASS。

## 未运行与范围声明

- 未运行 MySQL/PostgreSQL 实机；该零 SKIP 矩阵属于 #27。
- 未运行 service/controller/frontend/i18n；本修复未触及这些范围。
- 无剩余实现项或已知阻塞；最终证据提交后工作树必须 clean。
