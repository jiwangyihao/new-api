# Issue #26 证据

## 2026-08-05 — Lineage 与 Git 基线自检

### Git 分支与工作树

命令：`git status --short --branch`

观测：

- branch：`jiwangyihao/issue-26-conversion-fx`
- staged：`0`
- unstaged：`0`
- untracked：`0`

命令：`git rev-parse HEAD`

观测：`60e71da8d5be73816dd7c892b0d4f96768db98b3`

### 已验收实现祖先

命令：`git merge-base HEAD fd4d4683bc3b3b2cdd78c8e5c851c58263e61971`

观测：`fd4d4683bc3b3b2cdd78c8e5c851c58263e61971`

### 基线后的提交范围

命令：`git log --oneline fd4d4683bc3b3b2cdd78c8e5c851c58263e61971..HEAD`

观测仅有两条调度/Agent 文档提交：

- `60e71da8d docs(agents): 修正 Issue 26 开工基线规则`
- `5409da885 docs(agents): 补充 Issue 26 调度与恢复边界`

### Orca 原生 lineage

命令：`orca status --json`

观测：runtime `ready`，runtimeId `ce771a98-c24e-477f-b2bd-fcccd853bd5b`，Orca `1.4.170`。

命令：`orca worktree current --json`

观测：

- worktree id：`1bd24578-ec8b-4492-961c-108ab229f4e7::C:/Users/34404/source/repos/new-api/.workspaces/new-api/issue-26-conversion-fx`
- head：`60e71da8d5be73816dd7c892b0d4f96768db98b3`
- branch：`refs/heads/jiwangyihao/issue-26-conversion-fx`
- parentWorktreeId：`1bd24578-ec8b-4492-961c-108ab229f4e7::C:/Users/34404/source/repos/new-api/.workspaces/new-api/credit-operational-value-integration`
- parent capture source：`explicit-cli-flag`
- parent confidence：`explicit`
- baseRef：`jiwangyihao/credit-operational-value-integration`

结论：五项开工前置条件全部符合；允许创建 Issue #26 的首批持久化文件。此时尚未修改运行时代码。

## TDD 记录

首个周期已进入 RED；以下记录测试、失败命令和可恢复的最小 GREEN 边界。

## 2026-08-05 — FX parser 首个 RED

新增公共行为测试 `TestParseCreditFXRateSnapshotCanonicalizesUSDtoCNY`，输入 `7.300000`、source `USD`、valuation `CNY`、正 captured_at，期望不可变快照规范化为 `73/10` 且方向为 `USD_TO_CNY`。

命令：`go test ./model -run TestParseCreditFXRateSnapshotCanonicalizesUSDtoCNY -count=1`

真实结果：`FAIL`（build failed），原因精确为尚不存在的公共 seam：

- `undefined: ParseCreditFXRateSnapshot`
- `undefined: CreditFXRateSnapshotInput`
- `undefined: CreditFXRateSnapshot`
- `undefined: CreditFXDirectionUSDtoCNY`

结论：RED 对预期缺失行为敏感，未因无关测试或环境失败。下一步最小 GREEN 只实现该测试要求的结构化类型、方向常量、规范十进制解析与最大公约数约分。

## 2026-08-05 — FX parser 首个 GREEN

最小实现新增 `model/credit_fx_rate.go`，公开范围严格限于首个 RED 编译和行为所需的 `CreditFXRateSnapshotInput`、`CreditFXRateSnapshot`、`CreditFXDirectionUSDtoCNY`、`ErrCreditFXRateInvalid` 与 `ParseCreditFXRateSnapshot`。实现以整数逐位解析规范正十进制，不读取或运算 `float64 USDExchangeRate`，并将 `7.300000` 约分为 `73/10`。

命令：`go test ./model -run TestParseCreditFXRateSnapshotCanonicalizesUSDtoCNY -count=1`

真实结果：`ok github.com/QuantumNous/new-api/model`。

格式化：`gofmt -w model/credit_fx_rate.go` 成功且无输出。当前周期未扩展非法矩阵、identity、反向换算、floor、overflow、conversion、API 或 UI。

## 2026-08-05 — 首个 GREEN 安全点校准

- 独立 RED：`58866ae7b`（`test(issue-26): 固化 FX 快照解析首个 RED`）。
- 独立 GREEN：`bb399d868`（`feat(issue-26): 实现 FX 快照规范解析`）。
- GREEN 提交后 `git status --short --branch` 观测 staged/unstaged/untracked 均为 0。
- 下一步按协调器收敛顺序执行：A 非法输入 → B identity/反向 → C floor/overflow；每组独立 RED→GREEN，期间暂停 conversion、request、API/UI。

## 2026-08-05 — A 组非法输入 RED

新增公共行为测试 `TestParseCreditFXRateSnapshotRejectsInvalidInputsWithStableErrors`，逐项要求缺失、空值、非法十进制、超精度、零/负值、不支持币种和声明方向不匹配返回可由 `errors.Is` 判断的稳定 sentinel。

命令：`go test ./model -run TestParseCreditFXRateSnapshotRejectsInvalidInputsWithStableErrors -count=1`

真实结果：`FAIL`（build failed）。编译器精确报告 `ErrCreditFXRateMissing`、`ErrCreditFXRateEmpty`、`ErrCreditFXInvalidDecimal`、`ErrCreditFXPrecisionExceeded`、`ErrCreditFXNonPositive`、`ErrCreditFXUnsupportedCurrency`、`ErrCreditFXDirectionMismatch` 以及 `CreditFXRateSnapshotInput.Direction` 尚不存在。

结论：A 组 RED 对稳定错误分类与显式方向合同敏感；下一步只实现这些缺失行为，不涉及 identity、反向换算、floor 或 overflow。

## 2026-08-05 — A 组非法输入 GREEN

最小实现新增并分类 `ErrCreditFXRateMissing`、`ErrCreditFXRateEmpty`、`ErrCreditFXInvalidDecimal`、`ErrCreditFXPrecisionExceeded`、`ErrCreditFXNonPositive`、`ErrCreditFXUnsupportedCurrency`、`ErrCreditFXDirectionMismatch`，并增加可选显式 `Direction` 输入校验。未实现 B 组 identity/反向或 C 组 floor/overflow。

命令：`go test ./model -run "TestParseCreditFXRateSnapshot(CanonicalizesUSDtoCNY|RejectsInvalidInputsWithStableErrors)" -count=1`

真实结果：`ok github.com/QuantumNous/new-api/model`。随后 `gofmt -w model/credit_fx_rate.go` 与 `git diff --check` 均成功；按 Go 1.22 规则将纯计数循环改为 `range len(part)` 后再次运行同一测试，仍为 GREEN。

## 2026-08-05 — A 组 GREEN 安全点校准

- A 组独立 RED：`cb398810e`（`test(issue-26): 固化 FX 非法输入 RED`）。
- A 组独立 GREEN：`2c3685f11`（`feat(issue-26): 分类 FX 非法输入错误`）。
- GREEN 提交命令串包含 `git diff --check`；提交后 `git status --short --branch` 观测 staged/unstaged/untracked 均为 0。
- 下一步只做 B 组 identity/反向、确定性与快照冻结；B 组提交前禁止 C 组或 conversion。

## 2026-08-05 — B 组 identity/反向 RED

新增 table-driven 公共行为测试 `TestParseCreditFXRateSnapshotFreezesDirectionalRatios`，覆盖 CNY/CNY 与 USD/USD 固定 `1/1`、USD→CNY `73/10`、CNY→USD 严格倒数 `10/73`、相同 input + captured_at 的确定性，以及已返回值快照不随后续 Option 原始文本变化。

命令：`go test ./model -run TestParseCreditFXRateSnapshotFreezesDirectionalRatios -count=1`

真实结果：`FAIL`。USD→CNY 子例已通过；`CNY_identity`、`USD_identity` 与 `CNY_to_USD` 子例均在 parser 返回非预期错误处失败，证明现有 seam 缺少 identity 与反向分支，而非环境或无关测试失败。

结论：B 组 RED 对指定的方向比率行为敏感；下一步只增加 identity 与反向最小分支并复验确定性/冻结断言。

## 2026-08-05 — B 组 identity/反向 GREEN

最小实现增加 `IDENTITY` 与 `CNY_TO_USD` 方向：同币种不读取 Option rate，固定冻结 `1/1`；USD→CNY 冻结已约分 ratio；CNY→USD 严格交换分子/分母。`CreditFXRateSnapshot` 是值类型，相同 input + captured_at 产生相同值；返回后的值不随后续 Option 原始文本变量变化。

命令：`go test ./model -run TestParseCreditFXRateSnapshotFreezesDirectionalRatios -count=10`

真实结果：`ok github.com/QuantumNous/new-api/model`。

回归命令：`go test ./model -run "TestParseCreditFXRateSnapshot(CanonicalizesUSDtoCNY|RejectsInvalidInputsWithStableErrors|FreezesDirectionalRatios)" -count=1`

真实结果：`ok github.com/QuantumNous/new-api/model`。`gofmt -w model/credit_fx_rate.go` 和 `git diff --check` 同时通过；未进入 C 组或 conversion。

## 2026-08-05 — B 组 GREEN 安全点校准

- B 组独立 RED：`b9b3098c9`（`test(issue-26): 固化 FX 方向冻结 RED`）。
- B 组独立 GREEN：`c4d419e0e`（`feat(issue-26): 冻结 FX 双向比率快照`）。
- GREEN 提交后 `git status --short --branch` 观测 staged/unstaged/untracked 均为 0。
- 到达的“继续 B 组”指令晚于上述提交，因此未重复制造同一 RED；按其禁止范围保持不进入 C 组或 conversion。

## 2026-08-05 — C 组整数 floor/overflow RED

新增 table-driven 公共行为测试 `TestCreditFXRateSnapshotConvertMicrosUsesOverflowSafeFloor`，覆盖 `floor(amount × numerator / denominator)`、`MaxInt64` 宽中间乘积、结果溢出、分母零与小于 1 micros 的余数清空。

命令：`go test ./model -run TestCreditFXRateSnapshotConvertMicrosUsesOverflowSafeFloor -count=1`

真实结果：`FAIL`（build failed）。编译器精确报告 `ErrCreditFXOverflow` 与 `CreditFXRateSnapshot.ConvertMicros` 尚不存在。

结论：C 组 RED 对要求的整数换算接口与稳定 overflow sentinel 敏感；下一步只使用定宽整数实现，不引入 `float64` 或大整数热路径分配。

## 2026-08-05 — C 组整数 floor/overflow GREEN

最小实现增加 `CreditFXRateSnapshot.ConvertMicros` 与稳定 `ErrCreditFXOverflow`。换算复用现有无分配定宽整数 `mulDivFloor`（`bits.Mul64`/`bits.Div64`），严格计算 `floor(amountMicros × numerator / denominator)`；非法/非正 ratio 与分母零返回稳定 invalid sentinel，最终结果超出 `int64` 返回 overflow sentinel。

命令：`go test ./model -run TestCreditFXRateSnapshotConvertMicrosUsesOverflowSafeFloor -count=10`

真实结果：`ok github.com/QuantumNous/new-api/model`。

窄竞态命令：`go test -race ./model -run TestCreditFXRateSnapshotConvertMicrosUsesOverflowSafeFloor -count=1`

真实结果：`ok github.com/QuantumNous/new-api/model`。

联合回归命令：`go test ./model -run "Test(ParseCreditFXRateSnapshot|CreditFXRateSnapshotConvertMicros)" -count=1`

真实结果：`ok github.com/QuantumNous/new-api/model`。`gofmt -w model/credit_fx_rate.go` 与 `git diff --check` 同时通过；C 组期间未进入 conversion/request/API/UI。

## 2026-08-05 — C 组 GREEN 安全点校准

- C 组独立 RED：`a783ff3c1`（`test(issue-26): 固化 FX 整数换算 RED`）。
- C 组独立 GREEN：`5318e5cc2`（`feat(issue-26): 实现 FX 整数安全换算`）。
- GREEN 提交后 `git status --short --branch` 观测 staged/unstaged/untracked 均为 0。
- FX A/B/C 已形成完整定向 seam；下一条行为周期进入 timed conversion Quote 冻结估值，暂不展开 Confirm、request、API 或 UI。

## 2026-08-05 — FX 向量交接就绪

- FX A/B/C 已全部完成：非法输入稳定错误、同币种 `1/1`、USD↔CNY 严格倒数、确定性/冻结快照，以及 overflow-safe 整数 floor。
- 最后业务 GREEN：`5318e5cc2`；随后安全点校准：`fd6d316f7`。
- 协调器明确撤销继续探索 conversion Quote 的方向；此前仅做定向查看，未修改 conversion、request、API 或 UI 文件。
- 当前恢复阶段固定为 `FX_VECTORS_HANDOFF_READY`；提交本校准并确认 clean 后停止，等待新的显式派发。

## 2026-08-05 — Conversion 同币种冻结估值 RED

新增真实 SQLite tracer `TestConfirmTimedSubscriptionConversionFreezesSameCurrencyValuation`：迁移 valuation schema、置 marker ready、创建 CNY source timed plan 与 CNY Credit target，按 `1 × 100 + 25 = 125` 验证 conversion、ledger、source 状态和目标 valuation state 原子冻结 `40,000,000 × 125 / 100 = 50,000,000` micros，以及 FX `1/1`。

命令：`go test ./model -run TestConfirmTimedSubscriptionConversionFreezesSameCurrencyValuation -count=1`

真实结果：`FAIL`。`ConfirmTimedSubscriptionConversion` 返回稳定 `credit_valuation_source_required`，精确表明现有 conversion 调用 `GrantCreditBalanceTx` 时未提供 `CreditValuationSourceSnapshot`；事务因此 fail-closed，未产生部分写入。

结论：RED 到达真实 Confirm → Grant ingress seam；下一步最小 GREEN 只连接同币种 source plan 精确价格/currency、credit basis/gross credit 与目标 valuation currency，并冻结现有 conversion/ledger 字段。

### RED 范围收敛

- tracer 只断言已有 schema 可持久化的 `SubscriptionConversion`、`CreditBalanceLedger`、`CreditValuationState` 字段及 source subscription converted mapping。
- FX 仅覆盖同币种 CNY → CNY 的冻结 `1/1`；未添加或假设新的 conversion schema。
- `gofmt -w model/subscription_conversion_valuation_test.go` 成功；`git diff --check` 成功且无输出。
- 当前只提交 RED 与进度证据，不实现 GREEN、跨币种、在途 request 或 API/UI。

## 2026-08-05 — Conversion 同币种冻结估值 GREEN

最小实现仅修改 `model/subscription_conversion.go` 与 `model/credit_balance.go`：Confirm 在既有事务中从锁定后的 source plan 读取权威 `price_amount_micros`/currency，以 quote 的 `credit_basis`/`gross_credit` 和目标 plan valuation currency 构造 `CreditValuationSourceSnapshot`，沿既有 `GrantCreditBalanceTx` ingress 写入 conversion、ledger 与 valuation state。同币种 conversion ledger 冻结 FX `1/1`，未新增 schema。

单测命令：`go test ./model -run TestConfirmTimedSubscriptionConversionFreezesSameCurrencyValuation -count=1`

真实结果：`ok github.com/QuantumNous/new-api/model`；`1 × 100 + 25 = 125` 与 `40,000,000 × 125 / 100 = 50,000,000` micros 全部断言通过。

重复命令：`go test ./model -run TestConfirmTimedSubscriptionConversionFreezesSameCurrencyValuation -count=10`

真实结果：`ok github.com/QuantumNous/new-api/model`。

首次联合回归发现 legacy marker-not-ready conversion 在 ledger FX 写入分支解引用 nil `ValuationSource`。根因是 FX ledger 冻结分支只按 source type 判断，未跟随 valuation runtime gate；修复为同时要求 `valuationReady`，保持 legacy path 不读取 absent valuation source。

最终定向回归：`go test ./model -run "Test(ConfirmTimedSubscriptionConversionFreezesSameCurrencyValuation|CreditValuationOrderIngressCreatesExactState|RecalculateTimedSubscriptionConversionQuoteFormulaBoundaries|TimedConversionPreservesCompletedLogOwnershipAndTargetsNewUsage)" -count=1`

真实结果：`ok github.com/QuantumNous/new-api/model`。`gofmt -w model/subscription_conversion.go model/credit_balance.go` 与 `git diff --check` 均成功；未进入跨币种、并发、在途 request 或 API/UI。

## 2026-08-05 — Conversion 跨币种冻结与重放 RED

新增真实 SQLite table-driven tracer `TestConfirmTimedSubscriptionConversionFreezesCrossCurrencyValuationAndReplay`，覆盖 CNY→USD reciprocal floor、USD→CNY forward floor、冻结 FX numerator/denominator/captured_at、Option 原始文本变化后同 source 同事实重放不重估，以及同 idempotency key 更换 source 的冲突零写入。

命令：`go test ./model -run TestConfirmTimedSubscriptionConversionFreezesCrossCurrencyValuationAndReplay -count=1`

真实结果：`FAIL`。CNY→USD 与 USD→CNY 两个子例均由真实 Confirm 返回稳定 `credit_valuation_unsupported_currency`；失败发生在当前同币种 guard，证明跨币种 snapshot 与整数换算尚未连接，同时保持事务 fail-closed。

结论：RED 已覆盖跨币种 conversion、冻结快照、重放和冲突零写入的最终公共路径；下一步最小 GREEN 只复用唯一 `CreditFXRateSnapshot` seam 与现有 conversion/ledger/state/idempotency 事务。
