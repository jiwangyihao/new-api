# Issue #22 窄验收修复证据

## 冻结基线
- `git rev-parse HEAD`：`d5bba460f633ffd2943b1d13bb88b65cea338733`。
- `git status --short`：空。
- 协调器 finding：四个 `recognized_remaining_value` 列表排序仍比较兼容 `float64 amount`；五个 paid-value panel response 均未传播结构化 current-only warning。

## Finding A：权威 micros 排序
- RED 命令：`go test ./model -run TestPaidSubscriptionValueRecognizedRemainingSortUsesAuthoritativeMicros -count=1`。
- RED 精确信号：四个子测试 `users/subscriptions/plans/sources` 全部 FAIL；相同兼容 `amount=9007199254.740992` 下，较高 `9007199254740993` micros 被当作 tie，升序实际分别出现 `low-a,high,low-c`（前三类）与 `high,low-a,low-c`（sources），而不是 `low-a,low-c,high`。
- GREEN 命令：`go test ./model -run 'TestPaidSubscriptionValue(RecognizedRemainingSortUsesAuthoritativeMicros|UsersDescSortUsesUserIDTieBreaker)' -count=1`。
- GREEN 精确信号：`go test: 1 packages ok`；共用测试的 `users/subscriptions/plans/sources` 四个子测试均验证 precision-boundary micros 的升/降序及既有确定性 tie-breaker，兼容 float 相同不再影响排序。严格解析不回退 float；格式错误返回 `ErrCreditValuationSourceInvalid`，范围溢出返回 `ErrCreditValuationOverflow`，共同 builder 传播错误。

## Finding B：五面板 current-only warning
- RED 命令：待执行。
- RED 精确信号：待记录。
- GREEN 命令：待执行。
- GREEN 精确信号：待记录。

## 回归与范围
- 32 CNY/paid-value 后端定向回归：待执行。
- 前端 format/panel/page 定向测试与 typecheck：待执行。
- MySQL 5.7/PostgreSQL 9.6：本窄修复不实测，三数据库矩阵仍归 Issue #27。
- Issue #23–#28：不实现。
