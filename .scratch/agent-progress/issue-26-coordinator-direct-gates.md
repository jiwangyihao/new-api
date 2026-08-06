# Issue #26 协调器直接门禁记录

日期：2026-08-07
候选工作树：`C:/Users/34404/source/repos/new-api/.workspaces/new-api/issue-26-conversion-fx`
候选 HEAD：`0df808b2356c9a9cdffa07b1b9dae19f4b912e61`
固定分支点 / merge-base：`60e71da8d5be73816dd7c892b0d4f96768db98b3`
候选状态：`git status --short --branch` 显示 staged/unstaged/untracked 均为 0；`git diff --check` 无输出。

## 独立复评状态

Standards 与 Spec 两轴均未产出 verdict 或报告，不能计为 PASS：

- 首轮 Dispatch `ctx_538b08d59c2f` / `ctx_5ad670fff0ca`：Orca 报告终端仍为 PowerShell `>>` 状态，`worker-read` 为 `session_not_reported`，无报告、heartbeat 或 `worker_done`；随后停止。
- 重派 Dispatch `ctx_e4454b50bce1`：OMP 终端报 `kernel_unavailable: no serving account became available before timeout`，无 verdict；随后停止。
- 重派 Dispatch `ctx_3ecd78a0dfa2`：只停留在 `OMP ready`，无报告、heartbeat 或 `worker_done`；随后停止。
- 未继续重派、未改用自定义终端；Gate A 的独立双轴 verdict 仍未完成。

## 协调器直接验证

以下均从冻结候选实际执行，未运行浏览器以外的项目级全套件：

- `go test ./model -run "Test(ParseCreditFXRateSnapshot|CreditFXRateSnapshotConvertMicros|ConfirmTimedSubscriptionConversion|TimedReserveConversion|TimedConversionConcurrentWith|RecalculateTimedSubscriptionConversionQuoteFormulaBoundaries|TimedConversionPreservesCompletedLogOwnershipAndTargetsNewUsage)" -count=10`：PASS。
- `go test -race ./model -run "Test(CreditFXRateSnapshotConvertMicrosUsesOverflowSafeFloor|ConfirmTimedSubscriptionConversionConcurrentSameFactsWritesOnce|TimedReserveConversionRefundRestoresVirtualExactSnapshot|TimedConversionConcurrentWith(FinalSettle|FullRefund)UsesLegalSerialization)" -count=1`：PASS。
- `go test ./router -run "TestSubscriptionConversion(QuotesRouteIsAuthenticatedLiveAndReadOnly|RouteCommitsLatestQuoteAtomicallyAndReplays|RoutesExposeFrozenCrossCurrencyFactsAcrossHistoryAndAnalytics)" -count=1`：PASS；同一 `TestSubscriptionConversion` 路由集合 `-count=10`：PASS；`go test -race ./router -run "^TestSubscriptionConversion" -count=1`：PASS。
- `go test ./controller ./service -count=1`：PASS。
- `go test ./model -run "Test(CreditValuation(OrderIngressCreatesExactState|RequestPreConsumeRemovesMovingAverageCost|RequestFinalizesSameTargetIdempotently|FiveAnalyticsViewsAgreeOnThirtyTwoCNY)|PaidSubscriptionValueUsesTimedGrantTimelineAcrossFiveViews|AdminAnalyticsActiveScopeSeparatesTimedHistoryFromCreditBalance|CreditValuationAnonymousSubscriptionDeltasAreForbidden|TimedSubscriptionGrantMetadataMigrationSupportsFreshAndLegacySQLite)" -count=1`：PASS。
- `go test ./model -run "Test(CreditValuationSchemaSQLiteMigrationIsAdditiveAndRepeatable|CreditValuationSchemaSQLiteUniqueConstraints|TimedSubscriptionGrantMetadataMigrationSupportsFreshAndLegacySQLite)" -count=10`：PASS。
- `bun run i18n:sync`：完成；转换组件测试 `15 pass / 0 fail`；`bun run typecheck`：PASS；`bun run build`：Rsbuild `ready built`。
- 真实 SQLite/Chromium 证据已持久化于候选 `.scratch/agent-progress/issue-26/coordinator-browser-evidence.md`：quote→confirm→history HTTP 200；公式 `0 × 100 + 100 = 100`；source price `40000000`；CNY→USD FX `10/73`；gross/net cost `5479452`；unit value `4000000/73`；rule/state version `1`；将当前 FX `7.3→8.5`、Plan `40→55` 后历史仍冻结。

## 宽回归边界

`go test ./model ./router ./controller ./service -count=1` 在候选中复现 8 个 redemption route 失败；同一精确失败集在干净 integration 基线 `44e77a609663410591eebb202e29121a8bbc54a2` 中逐项复现，故当前证据将其标为既有基线失败而非 #26 回归。候选的 Issue #26 专属模型/路由/并发门禁仍通过。该宽回归不能被表述为全仓 PASS。

## 未运行范围与放行结论

- 未运行真实 MySQL 5.7 或 PostgreSQL 9.6；三数据库零 SKIP 属于 #27。
- 未执行生产部署；属于 #28。
- 未合并候选、未关闭 GitHub Issue #26、未回收候选工作树，因为 Gate A 的独立 Standards/Spec verdict 缺失，且宽回归仍有基线失败。
- 结论：协调器直接门禁提供了大量正向证据，但 Issue #26 不能宣布最终 PASS，也不能 non-ff 合入 integration。
