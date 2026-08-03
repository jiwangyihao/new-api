# Issue #21 上下文溢出恢复指令

## 任务身份与基线

你是 GitHub Issue #21「固化计时权益 grant 时间线与多币种分析」的恢复实现 Agent。继续原 Task `task_7e8acb18c8b5`，原 attempt `ctx_2d4507566c7a` 因 `context_length_exceeded` 被协调器停止；这不是代码失败，也不是重新设计的许可。

工作树固定为：

`C:/Users/34404/source/repos/new-api/.workspaces/new-api/issue-21-timed-grants`

当前分支：`jiwangyihao/issue-21-timed-grants`。当前安全 HEAD：`8ce6a8d7c14b6654c08762c537ab22d14bcb00d0`。它包含此前 `ccd516aaa` 的真实业务 RED，以及一次明确标注的编译 RED 恢复提交。启动时工作树应为 clean；若不 clean，先读取差异，不得覆盖或丢弃。

父集成基线来自已验收并关闭的 #20；不要从 `origin/main` 重建，不要 reset、rebase、stash 或覆盖现有提交。

## 必读材料与技能

开始编辑前必须读取：

1. GitHub 父 PRD `#19` 与当前 Issue `#21`；
2. `docs/agents/credit-operational-value-execution.md`；
3. `docs/agents/credit-operational-value-wave-1-contract.md`；
4. `docs/agents/credit-operational-value-issue-21.md`；
5. `docs/agents/credit-operational-value-issue-21-acceptance.md`；
6. 当前工作树 `.scratch/agent-progress/issue-21/{contract,status,evidence}.md`；
7. `CONTEXT.md`、`docs/adr/0002-credit-operational-remaining-value.md`、2026-08-02 spec/plan 中 timed grant、analytics 和 UI 章节。

必须使用 `tdd` skill：保留 RED 事实，逐一推进 GREEN；使用 `codebase-design` skill 保持 grant 深模块与调用方边界。进入 UI 时读取 `shadcn-ui` 和 `i18n-translate`；只有遇到无法直接解释的失败才读取 `diagnosing-bugs`。不要启动新的子 Agent，不要重新做已经完成的广泛探索。

## 已完成且不得重做

以下安全提交和合同已经存在，应直接复用：

- `796d2217a`：公开 timed grant 领域入口；
- `9e5a78bf1`：幂等冲突与续期追加；
- `ff635938c`：订单履约和付费快照门禁；
- `6fc4cb813`：兑换来源冻结；
- `528673889`：管理员 timed 售后授予；
- `a99c919e7`：disabled-plan、试用排除和 grant 不可变性；
- `14361af41`：来源身份规范化；
- `ccd516aaa`：timed grant 时间线计算器和真实五接口业务 RED；
- `8ce6a8d7c`：编译 RED、恢复合同和最窄下一步。

订单、兑换、管理员 grant、幂等、续期、试用排除、disabled-plan 和不可变性已有真实 SQLite RED→GREEN。不要重构这些路径，除非最窄回归证明当前分析接线打破它们。

## 第一目标：把现有五接口 RED 收敛为 GREEN

第一步仅处理当前恢复合同，不得继续扩张 DTO：

1. 运行：
   `go test ./model -run '^TestPaidSubscriptionValueUsesTimedGrantTimelineAcrossFiveViews$' -count=1`
2. 修复 `model/admin_analytics_paid_subscription.go` 中 nullable singular 的值/指针兼容问题；当前已知位置约在旧行 855–856、999，必须以当前源码为准重新定位。
3. 保持 `dto/admin_analytics.go` 已冻结 shape：nullable singular、`*_by_currency`、`amount_micros`、timed confidence/warnings/unknown 字段；不要再次设计通用 Credit DTO。
4. 把 `model/timed_subscription_analytics.go` 的 grant-only 结果以最窄方式接入 paid row，使 summary/users/subscriptions/plans/sources 都复用同一 row 投影。
5. 真实 fixture 固定：当前 Plan=`999 EUR`，同一 timed 权益包含 `40 CNY` 与 `10 USD` 两条相邻 grant，当前 Credit 剩余 50%；五接口必须得到 recognized=`10 CNY + 5 USD`，不得出现 EUR。
6. 单币种才保留兼容 singular；跨币种 singular 必须为 `null`。不得把不同币种相加。
7. 缺 grant、非法 grant、重叠 grant 输出稳定 unknown/warning；不得回退查询时当前 Plan 价格。
8. 先保存测试从编译 RED 到业务 RED，再到 GREEN 的原始结果。GREEN 后立即更新三份 progress 并做一个小步 Conventional Commit。

## 第二目标：完成 #21 剩余垂直切片

第一目标提交后，再按现有 Issue 指令完成：

- timed 逐币种分析和 source attribution/mixed_grants；
- 管理员 timed grant UI 的 reason/idempotency 行为；
- admin analytics 对 timed 多币种值的可见展示；
- en、zh、fr、ru、ja、vi 六语言；
- 真实 SQLite/API 与真实浏览器 smoke；
- 必要定向测试、前端 typecheck/build、i18n missing/extras 检查。

不得为了等待 #22 而复制其通用 Credit 骨架。若共享文件需要 #22 通用合同，保留 timed 最小增量并在 `.scratch/agent-progress/issue-21/contract.md` 明确记录集成规则。

## 严格所有权边界

本恢复 Agent不得实现：

- CreditValuation 通用状态、Credit 购买/消费五接口（#22）；
- request_id 异步可逆结算（#23）；
- Credit 正向入账（#24）；
- 退款、拒付、破坏性恢复（#25）；
- FX、转换或在途转换（#26）；
- 历史回填、migration marker 状态变更、`ready/suspended` 切换或三数据库迁移门禁（#27）；
- 发布部署（#28）。

可以读取 marker predicate 或在测试夹具预置所需状态，但生产代码不得创建、CAS、更新或自动切换 marker。不得恢复 legacy `model_limits`，不得改变 timed→Credit 转换数量公式。

## 持久化与故障恢复

每个有意义小步后必须更新并提交：

- `.scratch/agent-progress/issue-21/status.md`：当前阶段、完成项、下一步、阻塞、最近安全提交；
- `.scratch/agent-progress/issue-21/evidence.md`：RED/GREEN 命令与原始结果、API/browser 证据；
- `.scratch/agent-progress/issue-21/contract.md`：实际共享文件与集成边界。

不要把大段脚本放在终端输入中。用项目文件或 `.scratch` 保存可恢复信息。发现上下文再次接近上限时，先提交 clean 恢复点、更新三份文件，再向协调器 escalation；不得继续无界探索。

## 完成条件

只有全部满足才发送 `worker_done succeeded`：

- Issue #21 的所有验收项完成；
- 真实 SQLite timed grant 与五接口测试通过，业务 RED 已变 GREEN；
- 管理员 API/UI、六语言和真实浏览器证据齐全；
- 定向 Go/前端门禁、typecheck/build、i18n 检查通过；
- `git diff --check` 通过且工作树 clean；
- 所有实现与 progress 文件均已小步提交；
- 明确列出未运行或 SKIP 的 MySQL/PostgreSQL，不得伪称实测。

完成消息必须包含最终 HEAD、提交列表、验证命令与结果、修改文件、浏览器证据路径、已知 SKIP/风险。若遇到无法自行解决的阻塞，发送带精确失败命令、错误和现场路径的 escalation，不要发送虚假成功。
