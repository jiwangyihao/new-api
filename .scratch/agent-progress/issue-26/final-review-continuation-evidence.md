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
- M2 GREEN 未开始；未新增 schema、token、签名或兼容 fallback。

## 边界

- 未运行 M2 GREEN、前端、完整包级或全仓门禁。
- MySQL/PostgreSQL 实机矩阵属于 #27；未部署、未写生产数据。
