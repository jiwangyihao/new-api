# 第一并行波次协调器派发与集成清单（Issues #21、#22）

## 派发前门禁

- [ ] Issue #20 已收到 `worker_done`、通过独立验收、以 non-ff merge 集成并在集成树复验。
- [ ] GitHub Issue #20 已关闭；集成分支和工作树干净，记录作为共同基线的完整 SHA。
- [ ] `.scratch/agent-progress/issue-20/contract.md` 与 #20 最终实现可从共同基线读取。
- [ ] #21/#22 指令、共享合同及两份验收清单均已提交到共同基线。
- [ ] 为 #21、#22 各创建独立 Orca Task；子工作树必须显式从集成分支共同基线创建，Orca parent 必须是集成工作树而非主树。
- [ ] 每个 `worker-start` 指令包含父 PRD、当前 Issue、共享合同、技能、进度持久化和禁止越界范围；记录 Run/Task/Dispatch/terminal/worktree/branch。
- [ ] 两个 Worker 均确认 ready/input_accepted、终端 connected/running 后才进入等待；输入停在 Paste 时只提交现有输入，不重派。

## 并行观测与协调

- [ ] 通过 `orchestration check --wait` 消费 question/escalation/worker_done；超时仅作活性检查，不表示失败。
- [ ] 每数轮检查 `worker-show`、有限终端 cursor 增量和 `.scratch/agent-progress/issue-<N>`；只有明确 failed/stopped 才使用 `--retry-of`。
- [ ] #22 主改 CreditValuation、通用 analytics DTO/Credit 分流和通用 micros UI；#21 主改 timed grant/calculator/逐币种 timed UI。
- [ ] 需要对方 seam 时，通过协调器传递冻结接口；不得让两个 Agent 竞争式重写同一 DTO、row builder 或前端格式化器。
- [ ] Agent 中断时先保留已提交/未提交现场、读取进度文件和终端错误；可原地唤醒则复用原 Worker，不按运行时长重开。
- [ ] 一个 Worker 完成不要求停止另一个。先独立验收已完成分支；未满足其自身所有验收门禁则返回原 Worker 修复。

## 集成顺序与冲突归属

默认集成顺序固定为 **#22 通用骨架优先，#21 timed 增量随后**。即使 #21 先完成，也先完成其独立验收并保持分支，等待 #22 骨架验收/集成。

1. 在集成树确认干净并记录共同基线、两个 Worker HEAD、各自 `merge-base`。
2. 按 `credit-operational-value-issue-22-acceptance.md` 完整验收 #22；失败时让原 #22 Worker 修复。
3. #22 通过后以 non-ff merge 集成，提交信息为 `feat(valuation): 集成 Credit 运营剩余价值主链路`；立即重跑 #22 定向 Go/前端/SQLite/browser 门禁。
4. 按 `credit-operational-value-issue-21-acceptance.md` 在其原分支完成独立验收；验证其 timed seam 可映射到已集成 #22 通用 DTO。
5. 以 non-ff merge 集成 #21。若产生冲突，按所有权解决：保留 #22 的通用 DTO、Credit 分支、精确金额格式化器；把 #21 的 timed calculator、`*_by_currency`、grant source 与 UI 增量接入这些 seam。禁止机械选择 ours/theirs。
6. 冲突解决若需要实质改动任一 observable contract，停止 merge 或中止当前尝试，把最小修复要求发给原 Worker；不要由协调器偷偷重写大段实现。
7. 合并后运行组合回归：#21/#22 受影响 Go 包、真实 SQLite 两条 tracer、五接口 timed+Credit 组合、前端定向测试、typecheck/build、i18n、真实浏览器与 `git diff --check`。
8. 组合验收必须同时证明 Credit 冻结 32 CNY 不受 timed 分支影响，timed 多币种不被 Credit singular/币种逻辑合并。

## Issue 关闭与资源回收

- [ ] #22 只有在自身门禁和组合回归均通过后关闭；#21 同理。若第二次 merge 破坏已集成 #22，两个 Issue 均保持 OPEN 直到修复。
- [ ] 每个 Issue 的最终 evidence 记录集成提交、组合测试与实际未运行范围；不把 #27 才负责的三库零 SKIP 标成已完成。
- [ ] 关闭 Issue 后停止/释放对应 Worker，并仅回收本 Run 创建的 #21/#22 子工作树；使用 Orca 原生命令，不删除集成树、主树、`account` 或 `disk`。
- [ ] `orca worktree rm` 后确认节点消失；再做原生 `git worktree prune`，不回收非本会话创建的工作树。
- [ ] 集成树干净后，才按 DAG 并行派发 #23/#24。

## 组合不放行条件

- 两个 Worker 不是从同一已验收 #20 基线派生；
- 通用 DTO 出现两套并行概念或 Credit/timed 由当前 plan 价格混算；
- 跨币种 singular 被错误相加，Credit `time_based_value` 不为 null；
- 32 CNY 五接口 tracer 或 timed grant tracer 只能靠直接插表/mock 通过；
- 合并冲突通过丢弃一方测试、弱化错误码或扩大后续 Issue 范围解决；
- 组合构建、浏览器、SQLite 或工作树清洁度不通过。
