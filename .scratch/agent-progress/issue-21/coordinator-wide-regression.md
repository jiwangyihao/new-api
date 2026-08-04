# Issue #21 宽回归集成证据

## 集成结果

- #21 父工作树起始基线：`de6c6bbe912294e802b25a5e9bbcc37e8d9194d7`。
- Model 二次修复冻结 HEAD：`8a7144a7af1a4fc3b32203bb7c53cf8a51d0a646`；以 non-ff merge `f6a5c8aed` 合入。
- Controller 二次修复冻结 HEAD：`ebc15d9f9aeb8ead95835775490878d5b4d36955`；以 non-ff merge `68fa0f997` 合入。
- 六语言同步删除重复 `adminAnalytics.fields.valuationConfidence` 键并保留既有译文，提交 `b41364036`。
- 最终冻结父树 HEAD：以本文件提交后的 HEAD 为准；提交前 staged/unstaged/untracked 均为 0。

## 后端验证

- `go test ./model -run "Invitation|PaymentMethod|PaidSubscription" -count=1`：PASS。
- `go test ./model -count=1`：PASS。
- `go test ./controller -run "KyrenCreditWebhookCompletesFromSnapshotWithoutInvitation" -count=25`：PASS。
- `go test ./model ./service ./controller -count=1`：三包 PASS。
- 七项 Spec、四项 Standards、32 CNY Credit、timed CNY/USD、邀请身份重放的 model 聚焦组合：PASS。
- 管理员 timed grant、五接口、Redemption 与 Kyren 缓存隔离的 controller 聚焦组合：PASS。

## 前端与 i18n 验证

- `bun test`：admin analytics format、panel-fields、paid-value-panel 与 user-subscriptions-dialog 共 20/20 PASS。
- `bun run typecheck`：PASS。
- `bun run i18n:sync`：PASS；同步结果已提交，再次检查工作树 clean。
- `bun run build`：Rsbuild production build PASS。
- `git diff --check`：PASS。

## 边界

- 未运行 MySQL 5.7/PostgreSQL 9.6 实机矩阵；三数据库零 SKIP 仍由 Issue #27 独占。
- 未执行生产部署；由后续 Issue #28 独占。
- 未放宽无授权快照、非法 timed Plan、disabled Plan 或 Redemption fail-closed 合同。
