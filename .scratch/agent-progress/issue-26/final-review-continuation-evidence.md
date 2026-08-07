# Issue #26 最终复评续作证据

## 冻结现场

- 起始 HEAD `44009213cb8e4a582de34f884deecd5a8d687b2c`，起始工作树 clean。
- `b8598f4b7add27ba237f30dec6ceae7968cc2aa3` 与 H1 `3feb091159aef26731c1698647791acc03c29c0a` 均为祖先。
- Orca parent 为 `credit-operational-value-integration`；#24 H2 跨币种 ingress 与路由校准未覆盖或回退。

## M1/M3

- RED `9ffade1ac`：缺少稳定 sentinel；history 从其他 operand 重算 committed unit value。
- GREEN `0f98f18ed`：ineligible/stale sentinel、`errors.Is` machine code、committed unit-value 直读完成。
- 定向单次、`-count=10`、窄 `-race` 均为 3 packages ok。

## M2 RED

- RED 提交：`81b3f1d9d test(subscription): 固化转换报价身份失败合同`。
- 命令：`go test ./router -run '^(TestSubscriptionConversionQuotesRouteIsAuthenticatedLiveAndReadOnly|TestSubscriptionConversionRoutesExposeFrozenCrossCurrencyFactsAcrossHistoryAndAnalytics)$' -count=1`。
- 真实结果：FAIL。quote route 的 `quote_id`、`created_at`、`expires_at`、`facts_fingerprint` 均为空；时间无法解析且 `expires_at > created_at` 不成立。
- 真实 stale tracer：现有完整 SQLite quote→confirm fixture 在 Plan `price_amount_micros` 改价后携旧 quote identity；由于 identity 为空且 confirm 未消费它，响应仍为 success，`require.False(stale.Success)` 失败。该结果证明旧 quote 事实漂移未返回 `subscription_conversion_quote_stale`，也未满足失败零写入合同。
- 为让 stale 断言继续执行而产生的 RED 后 `require`→`assert` 重复编辑，经 `git diff 81b3f1d9d` 确认只有两行，并已恢复到 `81b3f1d9d`。
- 恢复后 `git status --short --branch`：staged/unstaged/untracked 均为 0。
## M2 GREEN

- GREEN 提交：`0e40a74fe fix(subscription): 固化转换报价身份与过期校验`。
- 服务端持久化 `SubscriptionConversionQuote`：随机 `quote_id`，DB 权威 `created_at`/`expires_at`，版本化 canonical authoritative facts snapshot 与 SHA-256 fingerprint；迁移已注册主路径与 valuation schema helper。
- Quote API 返回 `quote_id`、`created_at`、`expires_at`、`facts_fingerprint`；Confirm API 要求 `quote_id`，事务内锁定 user/source 对应记录，重新读取 source、source Plan、target mapping、valuation/FX 与资格事实并比对 fingerprint。
- 重放合同：同 idempotency key、source 与 quote 返回同一 committed conversion；不同 source 或 quote 返回稳定 `subscription_conversion_idempotency_conflict`。

## M2 GREEN 验证

- `go test ./router -run '^(TestSubscriptionConversionRouteRejectsExpiredQuoteWithoutWrites|TestSubscriptionConversionRouteRejectsAuthoritativeFactDriftWithoutWrites)$' -count=1 -timeout=120s`：PASS。真实 SQLite 分别将服务端报价时间置为已过期，以及在报价后改变 source `token_used` / `last_grant_credit_snapshot`；均返回 `subscription_conversion_quote_stale`，并断言 subscription 状态/余额、conversion、ledger、邀请归因零额外写入。
- `go test ./model ./controller ./router -run 'SubscriptionConversion' -count=1 -timeout=240s`：3 packages PASS。
- 核心 quote/confirm/stale/history/analytics/concurrent/eligibility route 集合 `-count=10`：PASS。
- 过期、remaining/basis 事实漂移与并发 confirm 的窄 `go test -race ./router ... -count=1`：PASS。
- `gofmt` 已覆盖全部 12 个实现/测试文件；实现提交前 `git diff --cached --check`：PASS。

## 边界

- MySQL/PostgreSQL 实机矩阵归 #27；本次仅声称真实 SQLite 事务与 stale 零写证据。
- 当前 Dispatch 明确限定补过期/事实漂移 SQLite 测试、定向回归、证据与提交，因此未运行全仓、前端、typecheck、i18n 或 production build。
- 未部署、未写生产数据，未触碰 #24 ingress/UI、#25、#27 或 #28。
