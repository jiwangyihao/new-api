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
