# Issue #23 验证证据

## 2026-08-05 基线与材料核验

### 起始工作树
命令：
```text
git status --short --branch && git merge-base HEAD ec1858fec89509bdec9a90a230a8496047c5becd && git rev-parse HEAD
```
关键输出：
```text
branch jiwangyihao/issue-23-request-settlement
staged 0, unstaged 0, untracked 0
ec1858fec89509bdec9a90a230a8496047c5becd
ec1858fec89509bdec9a90a230a8496047c5becd
```
结论：Worker 从协调器指定的已验收 #20/#21/#22 集成提交创建，且工作树起始干净。

### 必读材料
已读取：
- `AGENTS.md` 与自动注入全局规则
- `issue://jiwangyihao/new-api/19`
- `issue://jiwangyihao/new-api/23`
- `docs/agents/credit-operational-value-execution.md`
- `docs/agents/credit-operational-value-wave-2-contract.md`
- `.scratch/agent-progress/issue-20/contract.md`
- `.scratch/agent-progress/issue-22/contract.md`
- `CONTEXT.md`
- `docs/adr/0001-credit-balance-entitlement.md`
- `docs/adr/0002-credit-operational-remaining-value.md`
- 规格 5.4、6、7.3–7.5、9、11.3、13、14
- 实施计划任务 3 和任务 6
- `skill://tdd`、`skill://diagnosing-bugs`、`skill://codebase-design`

### #22 依赖结论
`.scratch/agent-progress/issue-22/contract.md` 明确声明已交付：
- `CreditValuation` 深模块；
- 购买来源快照；
- 最小同步 request tracer（只支持足额预扣及相同目标最终重放）；
- 通用 analytics DTO 与五接口 Credit 分流。

因此 #23 可在当前基线上深化 request 分支，不复制 #22 逻辑。

## RED/GREEN 记录

### 循环 1：目标累计量增加按当前池出账
RED 命令：
```text
go test ./model -run '^TestCreditValuationRequestTargetIncreaseUsesCurrentPool$' -count=1
```
关键输出：
```text
--- FAIL: TestCreditValuationRequestTargetIncreaseUsesCurrentPool
credit_valuation_request_test.go:19: Received unexpected error
FAIL github.com/QuantumNous/new-api/model
```
根因：#22 的 `SettleCreditRequestTargetTx` 明确只允许冻结 tracer 的相同目标，`200 -> 300` 返回 `ErrCreditValuationTargetConflict`。

GREEN 命令：
```text
go test ./model -run '^TestCreditValuationRequestTargetIncreaseUsesCurrentPool$' -count=1
```
关键输出：
```text
go test: 1 packages ok
```
行为：真实 SQLite 经购买 ingress 建立 1,000 Credit / 40 CNY 状态，预扣 200 后提交目标 300；最终 `token_used=300`、可用 700、exact `28,000,000` micros、请求活动快照 `12,000,000` micros、`settlement_version=2`。

### 导出符号引用证据
修改 `SettleCreditRequestTarget` 前使用 LSP references，找到 6 处：定义、`service/funding_source.go` 以及 `model/credit_valuation_tracer_test.go` 的 4 个调用。修改包内 `SettleCreditRequestTargetTx` 前使用 LSP references，只有定义和公开 wrapper 两处。

## 范围声明
- 尚未运行项目级全量测试、格式化器或 lint；按 Dispatch 合同由协调器最终统一运行。
- 本切片不新增 UI 或可见文案，因此当前不需要浏览器或 i18n 技能。
- MySQL/PostgreSQL 实测矩阵属于 #27；本切片仅做跨库静态语义审查和真实 SQLite 证明。
