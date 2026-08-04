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

### Controller/API：伪造估值事实覆盖权威 Plan

- 命令：`go test ./controller -run '^TestAdminCreateTimedSubscriptionUsesAuthoritativePlanSnapshot$' -count=1 -v`。
- 真实 SQLite/API 夹具：数据库 Plan 为 `40,000,000` micros CNY；请求额外携带伪造 `25,000,000` micros USD。
- 旧实现稳定 FAIL：`timed_subscription_grant_test.go:45` 期望 `40000000`，实际写入 `25000000`；测试因此 FAIL，证明 controller 提交的攻击值进入 exact grant。
- 该 RED 同时证明兼容 payload 中的旧估值字段当前未被忽略，权限边界漏洞可由管理员 API 真实触发。

### Model：旧快照不能覆盖 guard 内权威 Plan

- 命令：`gofmt -w model/timed_subscription_valuation_test.go && go test ./model -run '^TestTimedSubscriptionValuationGrantUsesAuthoritativePlanSnapshot$' -count=1 -v`。
- 真实 SQLite 夹具：数据库 Plan 为 `40,000,000` micros CNY、1,000 Credit、3,600 秒、never reset；调用方传入旧/伪造 Plan 为 `25,000,000` micros USD、250 Credit、7,200 秒、daily reset。
- 旧实现稳定 FAIL：`timed_subscription_valuation_test.go:123` 期望 `40000000`，实际 grant 为 `25000000`；证明 guard 后虽重读 Plan，但 normalize 仍采用调用方价币。
- 同一测试还将锁定权威 Credit、duration、reset 与 source snapshot；修复必须使实际窗口为 3,600 秒并冻结数据库 Plan 全部事实。

## GREEN：权威 Plan 快照

- 最小实现：`TimedSubscriptionGrantRequest` 删除 `Plan`、`SourcePriceMicros`、`SourceCurrency`，调用方仅传 `UserId`、`PlanId` 与 source identity/reason/key；订单、兑换、管理员生产调用均已迁移。
- 事务路径：`SubscriptionPlan` guard → 已提交 source/idempotency identity 重放 → 同事务重新读取当前 Plan → 新 allocation 校验 timed/enabled/非 trial/权威 micros/币种/Credit → 创建 entitlement/window/grant。
- Controller/API RED：`go test ./controller -run '^TestAdminCreateTimedSubscriptionUsesAuthoritativePlanSnapshot$' -count=1 -v` 在旧实现中 FAIL，权威 `40,000,000 CNY` 被伪造 payload 写成 `25,000,000 USD`。
- Controller/API GREEN：`go test ./controller -run '^TestAdminCreateTimedSubscriptionUsesAuthoritativePlanSnapshot$' -count=1`；PASS（1 package）。旧估值字段不再进入管理员业务 DTO，数据库 grant 严格为 Plan 的 `40,000,000 CNY`。
- Model RED：`go test ./model -run '^TestTimedSubscriptionValuationGrantUsesAuthoritativePlanSnapshot$' -count=1 -v` 在旧实现中 FAIL，调用侧旧 Plan/价币覆盖数据库事实。
- Model GREEN：`go test ./model -run '^TestTimedSubscriptionValuationGrantUsesAuthoritativePlanSnapshot$' -count=1`；PASS（1 package）。数据库 Plan 的 40 CNY、1,000 Credit、3,600 秒、never reset 决定 grant 和 source snapshot。
- 调用迁移：其余 `TimedSubscriptionGrantRequest` 生产与测试 struct literal 已切换为 `PlanId`，不再传递 Plan 对象或价币估值事实；两条 GREEN 命令均完整编译对应测试包。

## GREEN：现有 timed grant 窄回归

- `go test ./model -run '^TestTimedSubscriptionValuationGrant' -count=1`：PASS（1 package）；覆盖创建/重放、管理员权威快照、身份冲突、续期、订单快照、trial、兑换快照、disabled Plan 与不可变 grant。
- 直接调用领域入口的非订单测试已改用 admin source，以免伪造不存在的订单 source；真实 order source 仅由 `CompleteSubscriptionOrderTx` 使用持久化订单快照进入。
- 订单和兑换都是已授权来源：前者从持久化 `SubscriptionOrder.EntitlementSnapshot`，后者从兑换创建时持久化的 `Redemption.FulfillmentSnapshot` 冻结事实；只有管理员新 allocation 使用 guard 内当前 enabled Plan。

## RED→GREEN：订单不可变履约快照

- 纠正范围：管理员新 grant 的权威事实来自 guard 内当前 enabled Plan；已授权订单的履约事实来自持久化 `SubscriptionOrder.EntitlementSnapshot`，不能被购买后 Plan 改价或停用重写。
- RED：`go test ./model -run '^TestTimedSubscriptionValuationGrantOrderCompletionUsesImmutableSnapshotAfterPlanChanges$' -count=1`；订单快照为 `40,000,000 CNY`、1,000 Credit、3,600 秒、never reset，创建订单后 Plan 改为 `25,000,000 USD`、250 Credit、7,200 秒、daily reset 并 disabled；57aab92c5 返回 `ErrTimedSubscriptionGrantInvalid`，无法履约。
- GREEN：同一命令 PASS（1 package）。`GrantTimedSubscriptionTx` 仍先执行 Plan 行 guard；order source 随后在同一事务锁定已成功订单，验证 user/plan/source identity 与持久化快照，再从快照构造 plan 并冻结 40 CNY、1,000 Credit、3,600 秒、never reset；Plan 后续 disabled 不撤销已授权订单。
- 管理员路径没有接受快照参数，仍从 guard 内当前数据库 Plan 重读并要求 enabled，因此客户端无法借用订单快照通道。

## RED→GREEN：兑换创建快照

- RED：`go test ./model -run '^TestTimedSubscriptionValuationGrantRedemptionCreatesAndReplaysGrant$' -count=1`。兑换创建时 Plan 为 `80,000,000 CNY`，随后改为 `50,000,000 USD`；旧路径在兑换时从后来 Plan 重建履约快照，grant 实际为 50 USD，违反兑换创建授权事实。
- GREEN：`Redemption.Insert` 在同一事务锁定 Plan 并持久化 entitlement source snapshot；兑换时锁定兑换记录和当前 Plan 资格，但从已持久化 snapshot 构造 timed grant 事实。相同命令 PASS（1 package），grant 为原 `80,000,000 CNY`，重放不续期。
- 兑换 `FulfillmentSnapshot` 仍在首次履约事务补齐事件时间与 fulfillment subscription，源 entitlement 部分不再被后来 Plan 改价重写。

## 数据库范围

- SQLite：权威快照真实事务/API 定向测试 PASS。
- MySQL 5.7：未运行；三库零 SKIP 归 Issue #27。
- PostgreSQL 9.6：未运行；三库零 SKIP 归 Issue #27。
