# Issue #25 恢复状态

## 当前阶段

- 阶段：HANDOFF_READY（C 组最小公开路径测试意外 GREEN；D/E PENDING）
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
2. **D 邀请取消隔离（PENDING）**：从公开 `RecoverSubscriptionOrder` 接缝构造 Credit 订单及错误/既有 `InvitationRewardEvent`，断言 recovery、订单终态、奖励取消同事务且幂等；同时断言 Credit 不新增邀请收益、不进入邀请付费统计。
3. **E SQLite 并发原子性（PENDING）**：在真实文件型 SQLite WAL 接缝覆盖 refund + chargeback、refund + admin decrease、outflow + request settle、outflow + request refund；断言合法串行结果集合、单一 recovery ledger、数量/状态版本一致及活动 request snapshot 不变，并运行相关窄 `-race`。

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
