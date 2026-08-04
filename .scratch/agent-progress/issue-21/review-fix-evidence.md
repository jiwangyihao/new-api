# Issue #21 Standards 修复证据

## 冻结输入

- Standards 评审：`C:/Users/34404/AppData/Local/Temp/new-api-issue21-standards-final-review.md`，结论为 Findings 1–4 阻塞。
- 冻结实现 HEAD：`547512242578ec198034d322875c5485735b247a`，初始工作树 staged/unstaged/untracked 均为 0。
- 父集成 HEAD：`2260cd2f6369d9cd9e1bea2ac93349b45c7b0ccc`，父集成工作树 staged/unstaged/untracked 均为 0。
- #22 集成提交说明：Issue #22 记录 `ac830971a32e24f5b88c42b312d62fffd4229e21`；当前父集成 HEAD 已在其后继续前进。

## 父集成合并恢复安全点

- 合并提交：`9cee335ddb0638af7b5bb9229d5d2a03db5a0712`，父集成 HEAD `2260cd2f6369d9cd9e1bea2ac93349b45c7b0ccc`。
- 冲突：23 个冲突块按冻结所有权解决；保留 #22 的通用整数 `adminMoneyAccumulator`、Credit/current_only DTO/状态/排序与前端 BigInt 语义，叠加 #21 的 timed `*_by_currency`、calculator、warning/source 和现有 timed UI。
- 后端验证：#22 权威 micros 排序与 #21 timed 五接口 tracer 同批通过；nullable recognized singular 在跨币种时从 by-currency 读取权威 micros。
- 前端验证：`panel-fields` 11/11 PASS；`bun run typecheck` PASS；Credit 不适用与 timed 跨币种展示语义均保留。
- 清洁度：冲突 0，staged 0、unstaged 0、untracked 0；cached diff check 通过。

## 已读取合同

- Issue #19、#21、#22。
- `docs/agents/credit-operational-value-execution.md`。
- Wave 1 contract/acceptance、Issue #21 instruction/acceptance。
- `.scratch/agent-progress/issue-21/{status,evidence,contract}.md`。
- `CONTEXT.md`、ADR 0002、2026-08-02 spec/plan 的金额、timed、锁/幂等、错误与测试章节。
- `skill://diagnosing-bugs`、`skill://tdd`、`skill://codebase-design`、Orca orchestration/CLI 实时指南。
- 父集成 #22 的 `model/admin_analytics_paid_subscription.go`、`dto/admin_analytics.go`、权威 micros 排序与 current_only 相关测试接缝。

## RED / GREEN 证据

尚未开始。每个 finding 完成有效 RED、最小 GREEN、定向验证后立即追加：精确命令、失败症状、根因、通过结果与提交 SHA。

## 数据库范围

- SQLite：待运行真实文件型或共享多连接并发证明。
- MySQL 5.7：未运行，三库零 SKIP 归 Issue #27。
- PostgreSQL 9.6：未运行，三库零 SKIP 归 Issue #27。
