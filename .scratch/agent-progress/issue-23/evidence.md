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
尚未运行。首个循环将使用真实 SQLite、公开 request 领域入口，证明 `request_id + target_applied_credit` 的目标变化合同；精确命令和关键失败输出将在执行后追加。

## 范围声明
- 尚未运行项目级全量测试、格式化器或 lint；按 Dispatch 合同由协调器最终统一运行。
- 本切片不新增 UI 或可见文案，因此当前不需要浏览器或 i18n 技能。
- MySQL/PostgreSQL 实测矩阵属于 #27；本切片仅做跨库静态语义审查和真实 SQLite 证明。
