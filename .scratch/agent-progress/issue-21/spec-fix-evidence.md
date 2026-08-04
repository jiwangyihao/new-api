# Issue #21 AC2 / Gate B Spec 修复证据

## 冻结输入

- HEAD：`763b0f40bdc8fb7d5c11bc69f46749fd40a8763b`。
- 初始工作树：staged 0、unstaged 0、untracked 0。
- Standards 最终复评：PASS，0 findings。
- Spec 最终复评：FAIL，唯一 finding 为 AC2 / Gate B 权限边界；`controller/subscription.go:AdminBindSubscription` 信任客户端价币，`model/timed_subscription_valuation.go:GrantTimedSubscriptionTx` 未从同一 guard 内的权威 Plan 冻结价币。
- 已读取：Issues #19/#21/#22、执行/Wave/Issue 合同与验收、review-fix 三文件、`CONTEXT.md`、ADR 0002、2026-08-02 specification/plan timed/整数/事务/幂等章节、强制技能 `diagnosing-bugs`/`tdd`/`codebase-design`、Orca CLI/orchestration 实时指南。

## 旧实现观察

- `AdminBindSubscriptionRequest` 暴露 `source_price_micros` 与 `source_currency`，并原样传给 model。
- `TimedSubscriptionGrantRequest` 暴露 `Plan *SubscriptionPlan`、`SourcePriceMicros`、`SourceCurrency`。
- `GrantTimedSubscriptionTx` 已先执行 `SubscriptionPlan.conversion_guard_version` 写 guard，并在同一事务重读计划资格；但重读后再次 normalize 时仍使用请求中的价币，因此 Plan 的权威 `price_amount_micros/currency` 没有决定 exact grant。
- 现有 controller 测试正把 `25,000,000 USD` 作为成功事实断言，构成权限漏洞的可重复接缝。

## 预定 RED / GREEN 命令

- API RED：`go test ./controller -run '^TestAdminCreateTimedSubscriptionUsesAuthoritativePlanSnapshot$' -count=1 -v`。
- 领域 stale/资格/原子性 RED：`go test ./model -run '^TestTimedSubscriptionValuationGrant(UsesAuthoritativePlanSnapshot|RereadsPlanInsideGuard|RejectsInvalidAuthoritativePlanAtomically)$' -count=1 -v`。
- GREEN 重复：相同定向集合 `-count=10`。
- 若触及并发接缝：窄 `go test -race ./model -run '<exact concurrency pattern>' -count=1`。
- 最终组合：#22 32 CNY Credit/current_only/权威 micros/BigInt 后端合同与 #21 timed CNY/USD 五接口定向测试。

## RED

- 尚未运行；下一安全步先提交恢复现场，再新增一个真实 API/SQLite tracer。

## GREEN

- 尚未实现。

## 数据库范围

- SQLite：待运行真实事务/API 测试。
- MySQL 5.7：未运行；三库零 SKIP 归 Issue #27。
- PostgreSQL 9.6：未运行；三库零 SKIP 归 Issue #27。
