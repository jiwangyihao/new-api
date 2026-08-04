# Issue #21 邀请身份重放修复状态

状态：INVESTIGATING

## 冻结现场

- 工作树：`C:/Users/34404/source/repos/new-api/.workspaces/new-api/issue-21-wide-model-fix`
- 分支：`jiwangyihao/issue-21-wide-model-fix`
- 起始 HEAD：`86b49a724e32b1dfea3b43a25f73e03efb8584b7`
- 稳定 RED 提交：`7b9e0038e`
- clean handoff 证据提交：`86b49a724`
- 起始工作树：clean。
- 最近安全提交：`86b49a724`。

## 唯一实现范围

- 生产代码：`model/subscription.go` 中成功订单重放结果恢复链及确有必要的同目录私有 helper。
- 测试：`model/invitation_commission_test.go` 及必要的同目录 test-only helper。
- 进度：本目录 `invitation-replay-fix-{status,evidence,contract}.md`。
- 禁止修改 `service`、`controller`、前端、i18n、schema、迁移门禁、FX、CreditValuation 或 #23–#28。

## 当前阶段

- 已确认冻结 HEAD 与 clean 工作树。
- 已读取父 PRD #19、Issue #21、已关闭 #22、执行/夹具合同、宽回归现场、验收清单、ADR 0002、2026-08-02 规格以及指定 Skills。
- 两个稳定 RED 尚待本任务原位复现；不得修改断言制造 GREEN。

## 当前修改文件

- `.scratch/agent-progress/issue-21/invitation-replay-fix-status.md`
- `.scratch/agent-progress/issue-21/invitation-replay-fix-evidence.md`
- `.scratch/agent-progress/issue-21/invitation-replay-fix-contract.md`

## 未提交文件

- 上述三份新进度文件；基线提交后应恢复为零。

## 下一步

1. 提交三份进度基线。
2. 单独运行两个成功重放测试，记录首次结果、持久化 event/source identity、错误重放结果与计数不变量。
3. 仅在现有 fulfillment-result 恢复事务内合入持久化 invitation identity。
