# Issue #21 邀请身份重放修复状态

状态：RED_CONFIRMED

## 冻结现场

- 工作树：`C:/Users/34404/source/repos/new-api/.workspaces/new-api/issue-21-wide-model-fix`
- 分支：`jiwangyihao/issue-21-wide-model-fix`
- 起始 HEAD：`86b49a724e32b1dfea3b43a25f73e03efb8584b7`
- 稳定 RED 提交：`7b9e0038e`
- clean handoff 证据提交：`86b49a724`
- 起始工作树：clean。
- 最近安全提交：`d5ad009c4`（建立邀请身份重放修复现场）。

## 唯一实现范围

- 生产代码：`model/subscription.go` 中成功订单重放结果恢复链及确有必要的同目录私有 helper。
- 测试：复用已提交的 `model/invitation_commission_test.go` 与既有无 event 重放覆盖，不扩大测试文件修改。
- 进度：本目录 `invitation-replay-fix-{status,evidence,contract}.md`。
- 禁止修改 `service`、`controller`、前端、i18n、schema、迁移门禁、FX、CreditValuation 或 #23–#28。

## 当前阶段

- 已确认冻结 HEAD 与 clean 工作树。
- 已读取指定材料与 Skills；后续不再扩展探索。
- 两个稳定 RED 已按 evidence 中命令再次复现：首次 `InviterId` 分别为 `9201`、`9231`，成功重放均实际返回 `0`。
- 下一步只编辑 `subscriptionOrderCompletionResultFromExistingFulfillmentTx` → `subscriptionOrderCompletionResultFromTimedGrantTx` 结果恢复链，从匹配当前 order/immutable fulfillment identity 的既有 `InvitationRewardEvent` 恢复 `InviterId`。

## 当前修改文件

- `.scratch/agent-progress/issue-21/invitation-replay-fix-status.md`
- `.scratch/agent-progress/issue-21/invitation-replay-fix-evidence.md`

## 未提交文件

- 上述两份 RED 记录；实现后与生产修复一起形成 clean 安全提交。

## 下一步

1. 仅修改指定恢复链；不写实体、不读 current Plan。
2. 运行两个测试单次/十次、无 event=0 与计数不变、九项组合及完整 model 包。
3. 更新最终证据、提交并确认工作树 clean。
