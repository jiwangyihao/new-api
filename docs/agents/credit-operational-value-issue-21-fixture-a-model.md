# Issue #21 旧夹具迁移 A：model paid-value analytics

## 任务目标

你负责修复冻结 Issue #21 分支中 `model` 包的旧 paid-value analytics 测试夹具，使其符合不可变 timed grant 时间线合同，并让 `go test ./model -count=1` 不再因这些旧夹具失败。只改 `model` 测试文件与必要的同目录 `_test.go` helper；不得修改生产代码，除非新的可重复失败证明生产缺陷且先向协调器 `question` 获得授权。

工作树由协调器创建为冻结 `issue-21-timed-grants` 的 Orca 子工作树，代码基线必须包含 `774b35740c1879b285537031410731317d0142fc`。共享合同位于：

`C:/Users/34404/source/repos/new-api/.workspaces/new-api/credit-operational-value-integration/docs/agents/credit-operational-value-issue-21-fixture-migration-contract.md`

## 必读材料与 Skills

按顺序读取：自动注入的 `AGENTS.md`；父 PRD `issue://jiwangyihao/new-api/19`；Issue `#21` 与已关闭 `#22`；共享合同；`docs/agents/credit-operational-value-execution.md`、Issue #21 acceptance、ADR 0002 与 2026-08-02 spec 中 timed grant/整数金额/历史 unknown 段；冻结树 `.scratch/agent-progress/issue-21/{status,evidence,contract}.md` 和 `final-spec-fix-*`。

必须使用 `skill://diagnosing-bugs` 复现，使用 `skill://tdd` RED→GREEN；需要整理 helper 边界时读取 `skill://codebase-design`。禁止子 Agent、项目全量 formatter/lint/前端套件。

## 精确所有权

主目标文件预计为：

- `model/admin_analytics_paid_subscription_test.go`
- `model/admin_analytics_test.go` 中仅与 paid-value fixture/migration 直接相关的测试 setup（若实际需要）
- 可在 `model` 下复用或新增一个窄 `_test.go` helper，但优先更新现有 helper，避免重复约定

不得改 `service/**`、`controller/**`、前端、locale、生产 analytics/timed 代码或 schema。

## 必须完成的行为

1. 先运行 `go test ./model -count=1`，把与旧 paid-value fixture 相关的失败测试名、错误、堆栈和数量写入 `fixture-a-evidence.md`。不要先改夹具。
2. 对每个仍按“当前 Plan 价格 + UserSubscription”构造 paid timed 价值的旧测试，补齐与其服务窗口一致的不可变 `TimedSubscriptionValuationGrant`。优先通过已存在的测试 helper；若只测试聚合算法而非领域入口，可直接插入 grant，但必须明确这是辅助夹具，主 tracer 已由 #21 真实领域入口覆盖。
3. grant 必须冻结与原测试预期一致的 `EventStartTime/EventEndTime`、`GrantCredit`、`SourcePriceMicros`、`SourceCurrency`、`ValuationAmountMicros`、`ValuationCurrency`、confidence、rule/version 与稳定 source identity。不得从 `PriceAmount float64` 反推 micros。
4. 原测试若预期“无 token 仅 time value”“超用归零”“never reset”“缩短窗口”“排序”等行为，应迁移事实，不得降低断言。若该行为在新合同下应产生 missing/unknown warning，应按 Issue #21 spec 修正断言并解释，而非伪造 exact。
5. 修复空指针或金额/排序失败的根因必须是合法 immutable grant fixture，不得在生产代码加 fallback 当前 Plan。
6. 运行相关最小测试 RED→GREEN，随后运行：
   - `go test ./model -run 'PaidSubscriptionValue|AdminPaidSubscription|TimedSubscription' -count=1`
   - 相同关键集合至少 `-count=10`
   - `go test ./model -count=1`
7. 若包级仍因 Redis 全局夹具、缺表 teardown 或其他目录旧夹具失败，记录精确测试名/错误并确认是否属于 A；属于 A 必须修复，不属于 A 用 `question` 交给协调器，不得忽略。

## 可恢复进度

第一项实际改动创建并提交：

- `.scratch/agent-progress/issue-21/fixture-a-status.md`
- `.scratch/agent-progress/issue-21/fixture-a-evidence.md`
- `.scratch/agent-progress/issue-21/fixture-a-contract.md`

状态必须列出每个失败测试的迁移结果。每完成一组夹具即提交，推荐 `test(analytics): 迁移计时价值不可变 grant 夹具`。不要把所有改动堆到一个未提交 diff。

## 验收与非目标

验收：旧实现测试在冻结基线可重复失败；迁移后原可观察业务断言保持或按新 warning 合同精确更新；model 定向与包级回归通过；diff-check 通过；工作树 clean；有效 worker_done。

非目标：生产代码、Credit 深模块、request settlement、兑换业务逻辑、前端/i18n、三数据库实机、部署。不得关闭 Issue 或合并父分支。
