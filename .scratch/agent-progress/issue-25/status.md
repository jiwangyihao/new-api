# Issue #25 恢复状态

## 当前阶段

- 阶段：E_REFUND_ADMIN_DECREASE_HANDOFF_READY（第二场直接 GREEN；等待协调器）
- Dispatch：`task_c27d832fec9b` / `ctx_d1e85f528802`
- Worker 终端：`term_597e2278-5e48-4f44-aaa1-e7f25a04d8af`
- 分支：`jiwangyihao/issue-25-destructive-outflow`
- 冻结共同基线：`fe1901aaf7a769fe7057c6483e30b7b1491adcdc`
- A 组提交：`90e6f3c80 test(issue-25): 覆盖管理员减少边界与原子性`
- B 组提交：`d6fdcd45c fix(issue-25): 固化订单回收事实与终态幂等`
- merge-base：`fe1901aaf7a769fe7057c6483e30b7b1491adcdc`
- Orca 父工作树：`credit-operational-value-integration`

## 已完成

- 核对 HEAD、分支、clean 状态与冻结共同基线。
- 核对 Orca parent、Run、Task、Dispatch 与终端绑定。
- 阅读 TDD、故障诊断、深模块、shadcn/ui 与 i18n 工作流入口。
- 阅读父 PRD #19、Issue #25、执行总览、Wave 3 共享合同与领域词汇。
- 核对 #20、#22、#23、#24 已集成合同、ADR、规格与实施计划指定范围。
- 确认 `ApplyCreditValuationOutflowTx` 已提供混合池 outflow，但 `RecoverCreditBalanceTx` 仍只修改 `token_used`，是首个 RED 的预期失败点。
- 已通过公开 `AdjustCreditBalance(decrease)` 证明携带 `plan_id` 稳定拒绝且无 adjustment 副作用。
- 已复现无 `plan_id` decrease 只增加 `token_used`、未同步减少 mixed exact / estimated / unknown 状态的真实缺陷。
- `RecoverCreditBalanceTx` 在 valuation ready 时复用现有 `ApplyCreditValuationOutflowTx`，同事务同步更新数量与 mixed exact / estimated / unknown。
- 保留 marker 非 ready 的旧数量路径；未新增字段、接口或 ledger 结构。
- 定向测试单次及 `count=10` GREEN，`gofmt` 与 `git diff --check` 通过。
- 真实 SQLite 表驱动覆盖完全清空舍入余数、超出可用只形成 settlement debt 且成本保持非负。
- 同事实重放返回持久化结果；变化 amount 稳定 `ErrCreditValuationIdempotencyMismatch` 且零写入。
- ledger 插入失败时 adjustment、权益、估值状态与 ledger 整笔回滚。
- 首次 decrease 与重放统一从持久化 ledger 投影结果，状态版本一致。
- B 组唯一真实 SQLite 测试 GREEN：订单可变 Credit 字段 / snapshot 被篡改后仍从 immutable purchase ledger 恢复 1,000 Credit。
- recovery 按当前 mixed pool 移除 60,000,000 micros；ledger 复制原 purchase price / denominator / FX facts。
- refund 同事实重放复用 ledger；payload / reason 变化稳定冲突且 state / ledger 数量不变；refund→chargeback 终态晋升复用原 ledger，不重复 outflow。

## PENDING 与精确续作接缝

1. **C 请求快照恢复（MINIMAL PROBE GREEN / HANDOFF）**：新增唯一测试 `TestCreditRequestRefundRestoresFrozenAttributionAfterDebtAbsorption`，经公开 `PreConsumeUserSubscriptionByUnits` / `SettleUserSubscriptionRequestTarget` 与真实 SQLite 验证 deduction snapshot → later ingress absorbs debt → full refund。冻结 exact / estimated / unknown 按原 request 恢复，已吸收债务转为 `restored_unknown`，另一 active request 保持逐字段不变；现有生产实现直接 GREEN，未制造生产改动。首次运行仅因测试误引用不存在的结果字段编译失败，修正测试后行为测试 PASS。
2. **D 邀请取消隔离（GREEN / HANDOFF）**：新增表驱动真实 SQLite 测试 `TestCreditOrderRecoveryCancelsOnlyMatchingInvitationReward`，经公开 `RecoverSubscriptionOrder` 覆盖 refund / chargeback。Credit 订单完成时不新增 `InvitationRewardEvent` 或邀请收益；对预存错误/既有事件执行 recovery 时，仅稳定 identity 匹配目标订单的事件转为 `cancelled`，另一订单事件逐字段不变；同事实 replay 复用 recovery ledger 且全量状态零写入。现有生产实现直接 GREEN，未修改生产代码。
3. **E SQLite 并发原子性（SECOND GREEN / HANDOFF）**：首场 refund + chargeback 已由 `845758872` clean 提交。第二场新增 `TestConcurrentRefundAndAdminDecreaseUseLegalSerializations`，真实文件 SQLite WAL、两个独立连接与确定性 User query barrier 单次直接 GREEN；refund recovery 与 admin decrease 各执行一次，最终 token limit=500 / used=600 / debt=100，mixed pool 成本清零、state_version=3，单一 recovery ledger 与单一 admin decrease ledger；两种合法串行顺序的管理员撤值分别为 0 或 6,000,000 micros，总撤值固定 30,000,000 micros；异参 admin replay 稳定 `ErrCreditValuationIdempotencyMismatch` 且全量快照零写入，无 SQLite/GORM/唯一约束文本泄漏。第三、四场保持 PENDING，等待协调器。

## 阻塞

- 无技术阻塞；协调器硬截止要求本终端不进入 C/D/E，由下一续作继续。

## C 组最小测试交接

- 命令：`go test ./model -run '^TestCreditRequestRefundRestoresFrozenAttributionAfterDebtAbsorption$' -count=1`
- 结果：`go test: 1 packages ok`（行为意外 GREEN）。
- 范围：仅新增一个 request 测试并更新 progress；未修改生产代码，未进入 D/E/API/UI/i18n。

## 最近安全提交

- `d6fdcd45c fix(issue-25): 固化订单回收事实与终态幂等`
- `06d185c40 chore(issue-25): 记录 outflow 核心安全点`
- `90e6f3c80 test(issue-25): 覆盖管理员减少边界与原子性`

## D 组邀请取消隔离交接

- 唯一测试：`TestCreditOrderRecoveryCancelsOnlyMatchingInvitationReward`（refund / chargeback 表驱动）。
- 首次运行真实揭示 Credit 订单不会自动创建邀请事件；原测试夹具错误期待事件存在，因此在 recovery 前显式构造错误/既有事件后验证取消隔离。
- 行为结果：目标事件进入 `cancelled` 并记录 recovery reason；另一订单事件逐字段不变；同事实 replay 返回原 ledger 且订单、事件、Credit 数量/估值、ledger/event/commission 计数均不变。
- Credit 排除：两个 Credit 订单完成后 `InvitationRewardEvent` 与 `InvitationCommissionRecord` 均为 0；邀请付费统计的 paid invitee 与 recognized amount 均为 0。
- 范围：未修改生产代码，未进入 E/API/UI/i18n。
