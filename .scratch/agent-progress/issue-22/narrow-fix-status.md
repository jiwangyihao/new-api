# Issue #22 窄验收修复状态

## 冻结基线
- 冻结 clean HEAD：`d5bba460f633ffd2943b1d13bb88b65cea338733`。
- 工作树：`jiwangyihao/issue-22-credit-tracer`。
- 启动时 `git status --short`：空。

## 最终状态
- 阶段：窄验收修复完成；业务实现 HEAD 为 `12d4f5fd5caa2c738faaccc72478f133d8aaa067`。
- Finding A 已由 `04e5611bd fix(analytics): 使用权威 micros 排序` 完成：users/subscriptions/plans/sources 的 `recognized_remaining_value` 严格按十进制 `amount_micros` 排序，兼容 `amount` 不参与权威比较，既有确定性 tie-breaker 保持不变。
- Finding B 已由 `12d4f5fd5 fix(analytics): 传播 current-only 面板警告` 完成：summary/users/subscriptions/plans/sources 统一传播唯一的 `section=credit_valuation`、`reason=current_only` 结构化 warning；无 current-only 行时为空，明细语义保持不变。
- 既有定向 PASS：`go test ./model -run 'TestPaidSubscriptionValue(RecognizedRemainingSortUsesAuthoritativeMicros|UsersDescSortUsesUserIDTieBreaker)' -count=1`。
- 既有定向 PASS：`go test ./model -run 'TestCreditValuationFiveAnalytics(PanelsReturnCurrentOnlyWarning|ViewsAgreeOnThirtyTwoCNY)' -count=1`。
- 本次收尾不修改业务代码、不重跑测试或大套件，仅提交 `narrow-fix-status.md` 与 `narrow-fix-evidence.md`；提交后执行 `git diff --check` 与 `git status --short`。
- 下一步：等待协调器重新验收。
- 阻塞：无。
- 最近业务安全提交：`12d4f5fd5caa2c738faaccc72478f133d8aaa067`。

## 范围边界
- 不重做 CreditValuation、人民币余额、Kyren、BillingSession/request_id、32 CNY tracer、六语言或浏览器 smoke。
- 不实现 Issue #23–#28，不写 migration marker，不切换 `ready`，不实现 FX。
- 仅运行 model/controller 与实际相关前端定向门禁，不运行全仓测试。
