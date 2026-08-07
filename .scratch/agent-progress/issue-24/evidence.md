# Issue #24 验证证据

## 基线证据

- `git rev-parse HEAD` → `ec1858fec89509bdec9a90a230a8496047c5becd`。
- `git status --short` → 无输出，初始工作树干净。
- `.scratch/agent-progress/issue-20/contract.md`：确认 `price_amount_micros`、Credit 估值币种和整数比例合同存在。
- `.scratch/agent-progress/issue-22/contract.md`：确认窄 ingress、固定锁序、同事务数量/状态/ledger 与五接口 Credit 分流存在。

## 最终集成基线核验（2026-08-07）

- `git rev-parse HEAD` → `6f865feca3cd517a3dd744e67ea1240d5001d2ed`。
- `git status --porcelain=v1` → 无输出，开工工作树干净。
- `git merge-base HEAD 6f865feca3cd517a3dd744e67ea1240d5001d2ed` → `6f865feca3cd517a3dd744e67ea1240d5001d2ed`。
- `orca worktree current --json` → `baseRef` 与 `git.head` 均为 `6f865feca3cd517a3dd744e67ea1240d5001d2ed`；`parentWorktreeId` 为 `1bd24578-ec8b-4492-961c-108ab229f4e7::C:/Users/34404/source/repos/new-api`，lineage capture source 为 `explicit-cli-flag`。
- 父工作树核对时 HEAD 为 `73c658daa8e7954cb6f229348aac80287253391c`；本子工作树保持协调器冻结的较早集成基线，不从旧 `issue-24-positive-ingress` 工作树继续开发。
- 复评裁决：#24 唯一 blocker 为 H2。实现边界收敛为管理员 increase 与 redemption 消费 #26 唯一 FX seam；后端分组 GREEN 后必须先创建安全提交。
- RED：`go test ./model -run TestRedemptionCreditBalanceCrossCurrencyRequiresFrozenFXSnapshot -count=1` 编译失败，缺少 `fx_direction`、grant 级 `replayed` 与 fulfillment 冻结 FX 快照字段，证明 H2 公开合同未完整接通。
- GREEN：`go test ./model -run 'TestRedemptionCreditBalance(CrossCurrencyRequiresFrozenFXSnapshot|SupportsUSDtoCNYFrozenFXSnapshot|RejectsInvalidFXAtomically)$' -count=1` → package PASS（约 14.82 秒）。覆盖 CNY→USD `10/73`、USD→CNY `73/10`、`captured_at`/direction、Option 变化冻结重放与缺失 FX 原子拒绝。
- GREEN：`go test ./model -run 'TestRedemptionCreditBalance(CrossCurrencyRequiresFrozenFXSnapshot|SupportsUSDtoCNYFrozenFXSnapshot|RejectsInvalidFXAtomically|LedgerFailureRollsBackEverything)$' -count=1` → package PASS（约 21.88 秒）。跨币种 SQLite trigger 故障后兑换恢复 enabled，fulfillment 不残留 `credit_fx_rate_snapshot`，subscription/state/ledger/log 均为零。
- `git diff -- model/credit_fx_rate.go model/credit_valuation.go` → 无输出，确认未修改 #26 parser/provider、Option 或 ingress 深模块。

## 已核验实现事实

- #22 提供 `CreditValuationSourceSnapshot`、`newForwardCreditValuationIngress`、`ApplyCreditValuationIngressTx`。
- #22 ingress 负责毛成本、settlement debt 抵扣、净 Credit/净成本、exact 状态和 `state_version`，调用方不得自行重复计算状态。
- 当前 ingress 只接受同币种；跨币种普通 Credit 来源没有可消费的权威运行时 FX snapshot seam。
- 兑换现有事务锁定来源、完成 grant、写 fulfillment 并标记 redeemed；Credit 模式尚未传估值来源事实。
- 管理员 adjustment 现有指纹未包含 plan 与权威价格/币种/FX/规则快照。

## RED / GREEN 记录

### 管理员同币种 exact ingress

- RED：`go test ./model -run TestAdminCreditBalanceIncreaseUsesSelectedPlanExactIngress -count=1`。
- RED 结果：编译失败，确认 `CreditBalanceAdjustmentRequest.PlanId`、结构化 ledger 字段和精确响应字段均缺失；代表公开领域合同尚不存在。
- GREEN：同一命令通过，`go test: 1 packages ok`，耗时约 13.91 秒。
- 行为：真实 SQLite ready marker 下，选择 `40 CNY / 1,000 Credit` 档位售后授予 800 Credit，得到 exact `32,000,000` micros CNY、state version 1，并结构化写入 plan、毛/净 Credit、源价格/分母、来源 key/status、指纹与 1:1 FX。

### 管理员 increase 资格矩阵

- GREEN：`go test ./model -run TestAdminCreditBalanceIncreaseRejectsIneligiblePlansAtomically -count=1` → `go test: 1 packages ok`，约 18.51 秒。
- 行为：缺少 plan、disabled、trial、invite trial、零/缺失精确价格、零 Credit 分母、未开放不限时购买、非 timed、EUR 均返回稳定 sentinel；真实 SQLite 中 adjustment、ledger、state、subscription、邀请事件全部保持 0，证明原子拒绝。

### 管理员 debt、幂等与事务回滚

- GREEN：`go test ./model -run 'TestAdminCreditBalanceIncrease(OffsetsDebtBeforeExactValue|IdempotencyBindsCompleteSnapshot|LedgerFailureRollsBackEverything)' -count=1` → `go test: 1 packages ok`，约 13.58 秒。
- debt：300 欠额 + 800 Credit 只形成净 500 / `20,000,000` micros；900 欠额 + 800 Credit 净 Credit/净成本均为 0，毛成本仍为 `32,000,000` micros。
- 幂等：同 key/完整同参数重放原 ledger/state version；amount、plan、operation、reason、operator 或冻结价格变化均返回 `credit_valuation_idempotency_mismatch`，余额与 state version 不再增加。
- 回滚：SQLite trigger 注入 admin ledger 创建失败后，adjustment、ledger、state、subscription 均为 0。
## 实际数据库/API/浏览器范围

- SQLite：真实内存 SQLite + GORM migration + ready marker；已通过管理员与兑换公开领域入口验证 exact ingress、幂等、debt offset 和事务回滚，并通过服务入口验证邀请隔离。
- MySQL/PostgreSQL：未运行真实数据库；仅沿用 GORM/结构化列，无新增数据库专用 SQL，完整矩阵仍归 #27。
- API/browser：恢复指令将本次续作限定为兑换同币种领域范围，未运行 API/browser，不能声明相关验收。
- 跨币种：真实 RED 后显式 SKIP，等待 #26；不是 MySQL/PostgreSQL 或 FX 验收通过。

### 兑换同币种冻结快照

- RED：`go test ./model -run TestRedemptionCreditBalanceFreezesExactTierSnapshot -count=1`。
- RED 结果：真实 `Redeem` 事务返回 `credit_valuation_source_required`（外层稳定 `redeem.failed`），证明兑换尚未把冻结来源事实交给 #22 ingress。
- GREEN：同一命令通过，`go test: 1 packages ok`，约 14.43 秒。
- 行为：`40 CNY / 1,000 Credit` 兑换经真实 SQLite 入口形成 1,000 gross/net Credit、`40,000,000` gross/net micros CNY、exact state version 1；ledger 结构化保存 `redemption:93004`、plan、价格、Credit 分母、1:1 FX、规则版本和参数指纹，档位改为 80 CNY 后历史 fulfillment/ledger/state 仍为 40 CNY。

### 兑换重放与来源冲突

- RED：`go test ./model -run TestRedemptionCreditBalanceReplaysExactResultAndRejectsSourceConflict -count=1`。
- RED 结果：底层已经发现 `credit_valuation_idempotency_mismatch`，但 `Redeem` 把它折叠成 `redeem.failed`，违反稳定 code/sentinel 合同。
- GREEN：同一命令通过，`go test: 1 packages ok`，约 15.03 秒。
- 行为：同一兑换重放复用原 ledger ID 与 state version；将已冻结来源价格从 40 CNY 篡改为 80 CNY 并重开来源后返回稳定 mismatch，ledger 仍仅一行、state version 仍为 1、exact cost 仍为 `40,000,000`、余额仍为 1,000。

### 兑换 settlement debt 优先抵扣

- GREEN：`go test ./model -run TestRedemptionCreditBalanceOffsetsDebtBeforeExactValue -count=1` → `go test: 1 packages ok`，约 16.98 秒；#22 深模块已实现该不变量，本组新增真实兑换入口证明，无需额外生产改动。
- 部分抵债：300 debt + 1,000 Credit 得到 net 700、`28,000,000` micros exact、debt 归零。
- 全额抵债：1,200 debt + 1,000 Credit 得到 net 0、净成本 0、剩余 debt 200；毛成本仍冻结为 `40,000,000` micros。
- 两例均保持 estimated/unknown 为 0，ledger 的 `debt_offset`、`net_credit`、`valuation_net_cost_micros` 与状态一致。

### 兑换事务回滚与邀请隔离

- GREEN：`go test ./model -run TestRedemptionCreditBalanceLedgerFailureRollsBackEverything -count=1` → `go test: 1 packages ok`，约 13.78 秒。
- 回滚：SQLite trigger 拒绝 redemption ledger 插入后，`Redeem` 返回稳定外层 `redeem.failed`；兑换记录恢复 enabled/未使用，冻结 source snapshot 未被完成态覆盖，subscription/state/ledger/invitation event/log 均为 0。
- 初次邀请隔离 RED：`go test ./service -run 'TestCreditFulfillmentPathsDoNotCreateInvitationBenefits/Credit_redemption' -count=1` 因历史非-ready Credit 池未设置 valuation currency 而返回 `credit_valuation_currency_required`，暴露 #24 不应提前改变 #27 marker 前基线。
- GREEN：兼容非-ready 快照后，`go test ./model -run 'TestRedemptionCreditBalance(FreezesExactTierSnapshot|ReplaysExactResultAndRejectsSourceConflict|OffsetsDebtBeforeExactValue)' -count=1 && go test ./service -run 'TestCreditFulfillmentPathsDoNotCreateInvitationBenefits/Credit_redemption' -count=1` → 两个 package 均通过，约 38.20 秒。
- 隔离行为：Credit 兑换后 invitation reward event、commission record、commission account 均为 0；邀请月度资格只统计独立 timed paid control，不把 Credit 兑换计入 qualified paid。

### 跨币种最小 RED（等待 #26）

- RED：`go test ./model -run TestRedemptionCreditBalanceCrossCurrencyRequiresFrozenFXSnapshot -count=1`。
- RED 结果：CNY 来源档位、USD Credit 估值池经真实 `Redeem` 入口命中 `credit_valuation_unsupported_currency`（外层 `redeem.failed`）；证明 #22 当前没有可消费的冻结 FX 快照接缝。
- 期望合同：成功响应必须保留 source=`CNY`、valuation=`USD` 及正 numerator/denominator/captured_at；测试以显式 `t.Skip` 保留，待 #26 提供唯一 `CreditFXRateSnapshot` 后移除跳过并 GREEN。
- 本切片未实现 FX parser/provider/Option、动态汇率或 conversion 生命周期，也未把该 RED 误报为通过。

### 最终定向回归

- `go test ./model -run 'TestRedemptionCreditBalance(FreezesExactTierSnapshot|ReplaysExactResultAndRejectsSourceConflict|OffsetsDebtBeforeExactValue|LedgerFailureRollsBackEverything|CrossCurrencyRequiresFrozenFXSnapshot)$|TestRedeemCreditBalanceConcurrentClaimPersistsOneGrantAndOneReplay$' -count=1 -v` → 5 个兑换行为/并发测试 PASS，跨币种合同 1 个 SKIP，package PASS（约 15.79 秒）。
- `go test ./service -run 'TestCreditFulfillmentPathsDoNotCreateInvitationBenefits/Credit_redemption$' -count=1 -v` → Credit redemption 邀请隔离 PASS。
- `go test ./model -run '^(TestRedeem|TestRedemptionCreditBalance)' -count=1 && go test ./service -run '^TestCreditFulfillmentPathsDoNotCreateInvitationBenefits$' -count=1` → 两个 package 均 `go test: 1 packages ok`，覆盖全部现有 Redeem/Credit redemption 定向用例及 purchase/redemption 邀请隔离。
- 未运行格式化器之外的 lint、项目级全量测试、真实 MySQL/PostgreSQL、API 或 browser；协调器与 #27 按各自所有权继续验证。
