# Issue #24 最终续作证据

## 冻结现场（2026-08-07）

- `git rev-parse HEAD` → `c7c983d02f2161f52a9a815a452dc7d950f692fc`。
- `git status --porcelain=v1` → 无输出。
- 当前分支 → `jiwangyihao/issue-24-final`。
- `git merge-base --is-ancestor b8598f4b7add27ba237f30dec6ceae7968cc2aa3 HEAD` → 成功。
- `git merge-base --is-ancestor 49b1ece48 HEAD` → 成功。
- `git merge-base --is-ancestor 79f3f221e HEAD` → 成功。
- `orca worktree current --json` → `parentWorktreeId` 为 `.../.workspaces/new-api/credit-operational-value-integration`，当前 head 与 Git 一致。
- 近期祖先包含：`b8598f4b7` 路由冻结合同、`5a2c12698` request→target 锁序、`88fc07a02` 锁序回归、`49b1ece48` redemption H2、`79f3f221e` admin increase H2。

## 已读取权威资料

- `issue://jiwangyihao/new-api/19`、`issue://jiwangyihao/new-api/24`。
- `CONTEXT.md`。
- `docs/adr/0001-credit-balance-entitlement.md`、`0002-credit-operational-remaining-value.md`。
- `docs/superpowers/specs/2026-08-02-credit-operational-remaining-value-spec.md` 全文。
- `docs/superpowers/plans/2026-08-02-credit-operational-remaining-value-plan.md` 全文。
- `docs/agents/credit-operational-value-execution.md`。
- `docs/agents/credit-operational-value-issue-24.md`、`credit-operational-value-issue-24-acceptance.md`。
- `docs/agents/credit-operational-value-wave-2-contract.md`、`credit-operational-value-wave-2-acceptance.md`。
- `.scratch/agent-progress/issue-20/contract.md`、`issue-22/contract.md`。
- `.scratch/agent-progress/issue-24/{contract,status,evidence}.md`。

## 已确认的既有 H2 证据

- redemption H2：CNY→USD、USD→CNY、Option 变化冻结重放、invalid FX、ledger failure 原子回滚均已在既有 evidence 中记录为 GREEN；安全提交 `49b1ece48`。
- admin increase H2：双向 FX、Option 变化冻结重放、invalid FX、ledger failure 回滚均已 GREEN；安全提交 `79f3f221e`。
- 既有 `-count=10` H2 稳定组通过；同币种管理员全组通过；未修改 #26 `credit_fx_rate.go`、`credit_valuation.go`、Option 生命周期。
- 本续作不得把这些历史记录冒充新的 API/browser 证据；新证据必须来自本轮实际命令和真实请求。

## 实际范围声明

- 本文件前段记录的是续作初始冻结时的阶段性范围；本轮后续 API、analytics、frontend、i18n 与 browser 证据已在下方章节实际记录并完成收尾。
- MySQL/PostgreSQL 实机验收不属于 #24，完整矩阵归 #27；本轮已观察真实 SQLite。
- 本轮跨币种 Chromium 只观察 USD→CNY；CNY→USD 仅由既有 H2 定向测试覆盖，测试证据未冒充 Chromium 观察。

## 管理员 preview/commit 真实 HTTP RED

- 命令：`go test ./router -run 'TestAdminCreditAdjustment(PreviewRouteReturnsAuthoritativeMicrosWithoutWrites|CommitRouteForwardsPlanAndReturnsAuthoritativeResult)$' -count=1 -v`。
- 结果：FAIL，`github.com/QuantumNous/new-api/router`，约 5.59 秒；两条测试均通过真实 Gin router、AdminAuth 与内存 SQLite。
- preview 精确失败：`POST /api/subscription/admin/users/9962/credit-balance/adjustments/preview` 得 HTTP `404`，而合同要求 HTTP `200` 与 `success:true`；证明 preview 路由/controller/service 尚不存在。
- commit 精确失败：`POST /api/subscription/admin/users/9962/credit-balance/adjustments` 得 HTTP `200`、响应 `{"message":"credit_valuation_plan_required","success":false}`；请求已包含 `plan_id=9965`，证明现有 controller DTO/转发丢失 `plan_id`。
- commit 零写入：断言 `plan_id=9965` 的 `CreditBalanceAdjustment` 与 `CreditBalanceLedger` 均为 `0`；失败未留下半提交状态。
- preview 无写入合同尚未到达 handler，因 404 先失败；测试保留 adjustment/ledger/subscription 均为 0 的断言，供 GREEN 证明。
- 夹具事实：9965 是 `40 CNY / 1,000 Credit` 的 source plan；9963 是全局 Credit 余额 plan，`valuation_currency=CNY` 正确属于 9963。不得把估值币种错误写入 source plan。
- 本 RED 未修改任何生产代码、#26 parser/provider、migration 生命周期、analytics、UI、i18n 或 browser。

## 管理员 preview/commit 真实 HTTP GREEN

- 生产改动严格限于现有 adjustment seam：`model/credit_balance_adjustment.go` 增加只读 preview；service 仅做类型别名/转发；controller DTO 增加 `plan_id` 并转发；router 注册 preview POST。
- preview 复用既有档位资格、`CreditValuationSourceSnapshot`、`newForwardCreditValuationIngress`、冻结 `CreditFXRateSnapshot` 与整数 `prorateFloor`；未修改 #26 parser/provider/Option 生命周期。
- 命令：`gofmt -w model/credit_balance_adjustment.go controller/subscription.go service/subscription_financial_recovery.go router/api-router.go router/subscription_credit_adjustment_route_test.go && go test ./router -run 'TestAdminCreditAdjustment(PreviewRouteReturnsAuthoritativeMicrosWithoutWrites|CommitRouteForwardsPlanAndReturnsAuthoritativeResult)$' -count=10 -v`。
- 结果：`go test: 1 packages ok`，约 15.86 秒；两条真实 Gin/AdminAuth/SQLite route 行为连续 10 次通过。
- preview 可观察合同：`plan_id=9965`、gross Credit 800、gross/net `amount_micros="32000000"`、source/valuation currency CNY、confidence exact、`preview=true`；adjustment/ledger/subscription 计数均保持 0。
- commit 可观察合同：请求中的 `plan_id=9965` 穿过 controller/service；响应含 gross Credit 800、gross/net `32000000`、`state_version_after=1`、`replayed=false`；数据库有且仅有一条对应 adjustment 与 ledger。

## 管理员稳定错误码与幂等 HTTP 合同

- RED：`go test ./router -run TestAdminCreditAdjustmentRoutesExposeStableCodesAndReplayCommittedResult -count=1 -v` → FAIL；缺 plan 与冲突分别只返回 `message=credit_valuation_plan_required` / `message=credit_valuation_idempotency_mismatch`，没有稳定 `code` 字段。
- GREEN：controller 的 adjustment/preview 专用错误 writer 同时返回 `message` 与 `code=err.Error()`；不解析或映射错误文本。
- 稳定验证：`gofmt -w controller/subscription.go router/subscription_credit_adjustment_route_test.go && go test ./router -run 'TestAdminCreditAdjustment(PreviewRouteReturnsAuthoritativeMicrosWithoutWrites|CommitRouteForwardsPlanAndReturnsAuthoritativeResult|RoutesExposeStableCodesAndReplayCommittedResult)$' -count=10 -v && git diff --check` → `go test: 1 packages ok`，约 10.62 秒，diff-check 无输出。
- 行为：missing plan preview 返回 HTTP 200、`success=false`、`code=credit_valuation_plan_required`；首次提交 `replayed=false`；同 key/同事实重放 `replayed=true`、`gross_amount_micros="32000000"`、`state_version_after=1`；同 key/amount 801 返回 `code=credit_valuation_idempotency_mismatch`。
- 原子性：重放与冲突后对应 idempotency key 的 adjustment/ledger 仍各一条。

## 后端合同修复：事务快照、只读 preview、稳定 code、冻结 replay

- 修复范围：`model/credit_balance_adjustment.go`、`model/errors.go`、`controller/subscription.go`、`router/subscription_credit_adjustment_route_test.go`；未修改 #26 parser/provider/Option 生命周期、analytics、UI、i18n 或 browser。
- preview 现在在同一个 `DB.Transaction` 内先调用 `CreditValuationRuntimeReadyTx(tx)`；缺失 marker 或非 `ready` 返回 `credit_valuation_migration_not_ready`。
- preview 的 SQLite 查询只做事务内一致读取，不执行 `AcquireCreditBalancePlanGuardTx` 的 SQLite guard UPDATE；MySQL/PostgreSQL 读取使用 `FOR UPDATE`。余额与估值 state 读取同一事务，非 SQLite 使用锁读。
- preview route 测试注册 GORM create/update/delete callbacks；请求成功后三个 callback 计数均为 0，证明 preview 没有 INSERT/UPDATE/DELETE，而非仅业务表计数未变。
- 结果 DTO 共享 `CreditBalanceAdjustmentAuthoritativeResult`；preview 与 commit 均返回 plan、gross/net Credit、gross/net micros、source/valuation currency、confidence、FX、rule、state version、debt offset、余额和 replay/preview 标志。
- controller sentinel 映射固定 code：plan required/ineligible、unsupported currency、invalid FX、overflow、state missing/mismatch、idempotency mismatch、migration not ready；未知错误统一 `internal_error`，不再把任意 `err.Error()` 暴露为 machine code。
- replay lookup 先按 idempotency key 锁定既有 adjustment，再从已提交 ledger 的 `source_snapshot` 重建完整 valuation facts；不先读取、资格校验或锁定当前 source Plan。后续 Plan 改价、禁用、改 Credit、改币种及 Credit valuation currency 后，同 key replay 仍返回首次冻结字段。
- 定向命令：`gofmt -w model/credit_balance_adjustment.go model/errors.go controller/subscription.go router/subscription_credit_adjustment_route_test.go && go test ./router -run 'TestAdminCreditAdjustment(PreviewRouteReturnsAuthoritativeMicrosWithoutWrites|CommitRouteForwardsPlanAndReturnsAuthoritativeResult|RoutesExposeStableCodesAndReplayCommittedResult|ReplayUsesFrozenFactsAfterPlanChanges|PreviewRequiresReadyValuationMarker)$' -count=10 -v && git diff --check`。
- 结果：`go test: 1 packages ok`，约 10.67 秒；五条 route 行为每条连续 10 次通过，包含 SQL write callback 断言、missing/pending marker 稳定 code、冻结 replay 逐字段比较和冲突零重复写入。

## API_HANDOFF_READY 最终门禁

- `idempotency_key` 已作为规范化请求字段进入 `creditBalanceAdjustmentFingerprint` payload；指纹同时绑定 user、operation、amount、plan、operator、reason 与完整 valuation/FX/rule facts。
- `gofmt -w controller/subscription.go model/credit_balance_adjustment.go model/errors.go model/credit_positive_ingress_test.go router/subscription_credit_adjustment_route_test.go` 完成。
- `go test ./model -run '^TestAdminCreditBalanceIncrease' -count=1` → package PASS。
- `go test ./router -run 'TestAdminCreditAdjustment(PreviewRouteReturnsAuthoritativeMicrosWithoutWrites|CommitRouteForwardsPlanAndReturnsAuthoritativeResult|RoutesExposeStableCodesAndReplayCommittedResult|ReplayUsesFrozenFactsAfterPlanChanges|PreviewRequiresReadyValuationMarker)$' -count=10` → package PASS。
- `git diff --check` → 无输出。
- 未运行 UI、i18n、browser、analytics 扩展或最终包级全量；这些明确交给新续作 Worker。


## 管理员售后授予 UI、六语言与真实浏览器安全点

- 开工核对：`git rev-parse HEAD` 为 `c4a056227ddd7d782163bf435765f076674da187`；本轮所有生产改动均建立在已交付 API 安全点之上，未修改 #26 FX parser/provider 或并行创建 API client/估值器。
- `AdminCreditBalancePanel` 复用 `UserSubscriptionsDialog` 已有 `getAdminPlans()` 结果；increase 仅展示 enabled、timed、非 trial/invite-trial、精确正微单位价格、正 Credit 且 `unlimited_purchase_enabled=true` 的档位，全局零价 Credit 余额套餐未进入列表。
- increase 要求档位、正整数 Credit 与非空原因；preview/commit 都保留原始十进制字符串。decrease 请求不携 `plan_id`。operation、plan、amount、reason 变化清空旧 preview 并换 key；失败保留 key，成功清空表单并换 key。
- UI 只消费服务端稳定 `code`，覆盖 plan required/ineligible、unsupported currency、invalid FX、overflow、state missing/mismatch、migration not ready、idempotency mismatch 与安全通用回退；不解析服务端 `message`。
- 组件定向测试已实际通过，覆盖：无合格档位不可提交；40 CNY / 1,000 Credit × 800 的权威 preview 显示 ¥32.00 与 `32,000,000 micros`；increase payload 含 `plan_id`；失败重试复用 key；amount/plan/operation/success 后换 key并清 preview；decrease 不含 `plan_id`；原始整数字符串不受显示格式污染。
- `bun run typecheck`、`bun run i18n:sync`、`bun run build` 已实际通过；同步报告显示 en/zh/fr/ja/ru/vi 六语言 `missingCount=0`、`extrasCount=0`。报告中的既有 untranslated 计数不属于缺键或多余键，本轮新增 UI 文案均已提供六语言自然翻译。
- Issue #24 model/controller/router 定向门禁及五条 adjustment route `-count=10` 已实际通过；`git diff --check` 无输出。
- 真实现场：后端 `127.0.0.1:3024`、default 前端 `127.0.0.1:3025`、SQLite `.scratch/agent-progress/issue-24/vertical-e2e.db`，均为真实运行服务与真实 Chromium，不是 mock 或静态 HTML。
- Chromium 对 `issue24user` 观察到 increase 档位列表只有 `40 CNY / 1,000 Credit`；提交 preview 的真实 payload 为 `{"operation":"increase","amount":"800","plan_id":2}`。
- Chromium 权威 preview 实际显示：档位标价 ¥40.00、档位 Credit 1000、source/valuation currency CNY、gross/net Credit 800、gross/net ¥32.00（`32,000,000 micros`）、debt offset 0、FX `1/1 IDENTITY`、confidence exact、rule/state version `1/1`。
- 首次计划作为“可控失败”的提交实际由服务端成功提交，因此没有冒充失败证据：网络捕获 payload 含 `plan_id=2`、原始 `amount="800"` 与 key `admin-credit-2-aef43c1d-49e2-40a4-8724-aa50ee024dbe`；SQLite 仅有一条 adjustment、ledger 与一份 800 Credit 余额。
- 临时根目录 Cookie 文件 `.scratchagent-progressissue-24vertical-e2e.cookies` 已删除且不会进入提交。
- 安全提交：`f56242f8f4b658d67cb2c4c3e49dbdbfa996c91e`（`feat(subscription): 完成管理员售后 Credit 授予界面`），提交时根目录临时 Cookie 已删除且工作树 clean。

## 真实失败同键重试、跨币种与 operation 生命周期

- 通过既有 API 创建隔离用户 `issue24retry`（ID 3），不直接插入用户或业务入口数据；真实 Chromium 仍只看到合格售后档位。
- 在 SQLite 上创建仅命中 `user_id=3` 且 `source_type=admin_adjustment` 的临时 `BEFORE INSERT` ledger 失败触发器；这是受控本地失败注入，不是生产代码或静态拦截。
- Chromium 第一次提交 payload：`operation=increase`、原始 `amount="125"`、`plan_id=2`、reason `Issue 24 same-key retry`、key `admin-credit-3-bea32605-69e1-4008-8689-e7a6353f6df8`；UI 只显示本地化安全回退“无法安全完成本次售后授予”。
- 失败后 SQLite 计数为 adjustment=0、ledger=0、subscription=0，证明原子回滚；随后删除临时 trigger，数据库无残留 trigger。
- 不改变任何表单事实再次点击提交；第二次真实 payload 的 user/operation/amount/plan/reason/key 与第一次逐项相同。成功后 SQLite 仅有一条 adjustment/ledger，Credit=125，运营价值 `5,000,000` micros CNY，state version=1。
- 成功后表单清空并关闭，组件合同已覆盖下一事实生成新 key；amount/plan/reason/operation 变化与 decrease 无 `plan_id`、切回 increase 不恢复旧 plan/preview 均由真实可观察组件测试通过。未通过 #24 浏览器执行 decrease 写入，避免越界实现 #25。
- 通过既有 root `PUT /api/option/` 将唯一 `USDExchangeRate` seam 设为 `7.3`，并通过既有管理员套餐 API 创建本地 `10 USD / 1,000 Credit` 验收档位；未复制或修改 FX parser/provider。
- Chromium 选择 USD 档位并预览原始 `amount="1000"`，权威结果显示 gross/net Credit 1000、gross/net ¥73.00（`73,000,000 micros`）、source USD、valuation CNY、冻结 FX `73/10 USD_TO_CNY @ 1786135678`、confidence exact、rule/state `1/2`。该 preview 只读，未提交 USD 调整。

## 真实 ledger、五个运营分析接口与邀请隔离

- 同一个已登录真实 Chromium 依次读取 `GET /api/subscription/admin/users/3/credit-balance/ledger` 及 `/api/admin-analytics/paid-subscription-value/{summary,users,subscriptions,breakdown/plans,breakdown/sources}`；六个请求均 HTTP 200、`success=true`。
- ledger 精确返回一条 ID 2：source `admin_adjustment`、plan 2、key `admin-credit-3-bea32605-69e1-4008-8689-e7a6353f6df8`、gross/net Credit 125、gross/net `5,000,000` micros CNY、balance 0→125、debt offset 0。
- summary 返回 active paid subscriptions=2、active paid users=2、recognized/exact/token-based value `5,000,000` micros CNY；另有 `credit_valuation_state_missing_count=1`，对应先前 ID 2 的现场夹具未建 state，未隐瞒该真实边界。
- users 面板把 `issue24retry`（ID 3）归入 recognized `5,000,000` micros CNY；subscriptions 面板列出真实 Credit pool；plans 面板把相同价值归入 `Credit 余额套餐`（plan 1）；sources 面板把同一入口归因到真实 admin adjustment 来源。
- `GET /api/admin-analytics/invitation-paid-subscriptions/summary` 返回 inviter/invitee/paid subscription 均 0；SQLite 中 `invitation_reward_events`、`invitation_commission_accounts`、`invitation_commission_ledgers`、`invitation_commission_records`、`invitation_monthly_entitlements` 均为 0。售后授予未增加邀请奖励、commission 或 paid referral attribution。
- 未实测边界：MySQL/PostgreSQL 实机属于 #27；本轮跨币种浏览器只验证 USD→CNY 方向，反向 CNY→USD 已由既有 H2 定向测试覆盖；未将测试证据冒充本轮 Chromium 观察。