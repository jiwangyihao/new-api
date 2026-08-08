# Issue #25 恢复状态

## 当前阶段

- 阶段：A 组 outflow 核心已提交，进入 B 组订单终态
- Dispatch：`task_685c1c42de63` / `ctx_214c53d3471f`
- Worker 终端：`term_22a24ff9-059b-4d1a-b7d9-9da9f0e33a64`
- 分支：`jiwangyihao/issue-25-destructive-outflow`
- 冻结共同基线：`fe1901aaf7a769fe7057c6483e30b7b1491adcdc`
- A 组提交：`90e6f3c80 test(issue-25): 覆盖管理员减少边界与原子性`
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

## 下一步

1. B 组：订单退款 / 拒付复用 immutable purchase facts，终态幂等。
2. C 组：请求 deduction snapshot 恢复归因。
3. D/E 组：邀请隔离与 SQLite 并发原子性。

## 阻塞

- 当前无阻塞；A 组完成后才进入 B 组。

## 最近安全提交

- `90e6f3c80 test(issue-25): 覆盖管理员减少边界与原子性`
