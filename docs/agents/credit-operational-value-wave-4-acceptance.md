# 第四执行波次协调器派发、迁移与发布交接清单（Issues #27、#28）

本波次严格串行：先完成全部前向 writer，再迁移并建立 `ready`，最后用同一不可变 digest 发布。等待时长、Agent 自报或单库通过都不能跨越依赖门槛。

## #27 派发前门禁

- [ ] #21–#26 均已收到 `worker_done`、通过独立及组合验收、non-ff merge 到 `jiwangyihao/credit-operational-value-integration`，GitHub Issues 按实际证据关闭。
- [ ] 最新集成树干净；记录完整基线 SHA，确认包含 #20 精确价格、#21 timed grant、#22 CreditValuation、#23 request settlement、#24 ingress、#25 recovery、#26 conversion/FX 的稳定合同。
- [ ] `.scratch/agent-progress/issue-20` 至 `issue-26` 可读，所有 writer、来源快照、request identity、FX、conversion 和 recovery 接缝均有合同与证据。
- [ ] #27 指令、wave-4 共享合同和 `credit-operational-value-issue-27-acceptance.md` 已提交到共同基线。
- [ ] 从该精确基线显式创建 #27 隔离子工作树；Orca parent 必须是集成工作树，不能是主树、`origin/main` 或任一 Worker。
- [ ] 创建独立 Orca Task，任务正文包含父 PRD #19、Issue #27、技能、恢复文件、真实三库要求、非所有权和验收文档。
- [ ] Worker 为 ready/input_accepted、terminal connected/running 后才进入等待；输入停在 Paste 时只提交现有输入，不重派。

## #27 观测、故障恢复与验收

- [ ] 使用 `orchestration check --wait` 消费 question/escalation/worker_done；超时仅表示无事件，不表示失败。
- [ ] 每数轮检查 `worker-show`、有限终端 cursor 和 `.scratch/agent-progress/issue-27`；只有明确 failed/stopped 才使用 `--retry-of`。
- [ ] 模型不可用、Paste 未提交、终端停滞但工作树/进程可恢复时，先读取现场并原地唤醒，不重复探索或重跑 apply。
- [ ] 若 Agent 在迁移 apply、marker 或真实数据库矩阵中断，先保存当前 version、marker、最后稳定主键、checksum、数据库状态和未提交文件，再决定恢复。
- [ ] question/escalation 涉及上游 writer 缺失时，暂停 #27 所有变通实现；按符号、调用点和缺失合同返回对应已集成切片修复，不能在迁移内复制 writer。
- [ ] 收到 worker_done 后按 `credit-operational-value-issue-27-acceptance.md` 完整验收；失败项返回原 Worker 在原工作树修复。
- [ ] 通过后 non-ff merge #27，并在集成树复跑历史价格、SQLite 迁移、marker/fail-closed、32 CNY 五接口与差异检查。
- [ ] 合并后才关闭 #27、停止 Worker 并回收本 Run 创建的 #27 子工作树；保留 `release-handoff.md` 给 #28。

## #27 → #28 强制交接门槛

- [ ] #27 已在真实 SQLite、MySQL 5.7.44、PostgreSQL 9.6.24 运行同一矩阵，三者 PASS 且零 SKIP；证据记录版本但不泄露 DSN。
- [ ] `release-handoff.md` 已由协调器审阅：迁移版本、CLI argv/退出码、marker/CAS/批次/checksum、stop-write blocker、三库矩阵、32 CNY fixture、repair/suspend 和 legacy Task 合同完整。
- [ ] 历史套餐价格从原始十进制文本回填；非法行阻止 ready；Credit/timed 历史只标 estimated/unknown，不存在 #20/#27 所有权重叠。
- [ ] 最新集成提交上 dry-run 两次稳定、apply/verify/重放/blocked ready/repair/suspend 和 ready fail-closed 均通过。
- [ ] frozen 32 CNY、timed 多币种、request restore、positive ingress、recovery、conversion/FX、disabled-plan 和邀请隔离组合回归通过。
- [ ] 集成树干净，无迁移进程、临时数据库服务、未提交 fixture 或敏感输出。
- [ ] #28 指令、wave-4 共享合同和 `credit-operational-value-issue-28-acceptance.md` 已包含在最新基线。
- [ ] 只有上述条件全部满足，才从最新集成提交显式创建 #28 子工作树并派发；不得从 #27 Worker HEAD 直接开工。

## #28 派发与不可逆动作决策门

- [ ] 创建独立 Orca Task，正文包含父 PRD #19、Issue #28、#27 `release-handoff.md`、既定跳板、技能、恢复协议、生产安全边界和验收清单。
- [ ] Worker 首先只读探测生产并持久化状态；在本地全量门禁、镜像 digest、脚本审阅和两次在线 dry-run 通过前，不允许生产写变更。
- [ ] 服务器脚本必须先落盘、审阅、提交，再传输；使用服务器本地 `flock`/`trap` 和显式状态机，禁止临时大段 shell。
- [ ] **Decision 1 — stop writes**：确认 digest、dry-run checksum、blocker 与维护窗口后，协调器检查状态记录；条件不全则不进入停写。
- [ ] **Decision 2 — apply**：外部写、后台任务、非终态预扣、异步回调、旧可写会话清零且一致备份/SHA-256 完成后，才允许 apply。
- [ ] **Decision 3 — start closed**：apply+verify 与 marker ready 全部通过后，才允许同 digest 封闭启动；失败时保持写关闭。
- [ ] **Decision 4 — open writes**：健康、业务、32 CNY、disabled-plan、五接口、真实前端及 fail-closed 探针全部通过后，才允许原子开放写。
- [ ] 开放写时间一经记录，即进入禁止 image-only rollback 边界；后续故障优先向前修复，回滚必须 stop→suspend→新版本重建。

## #28 观测与故障恢复

- [ ] 继续使用 `orchestration check --wait` 接收关键事件；协调器同时只读核对生产安全状态，不能只看 Worker 终端。
- [ ] 超时不重派；每数轮检查 `worker-show`、终端增量、progress/runbook 和生产只读状态。只有 failed/stopped 且现场已保存才 retry。
- [ ] Agent/终端中断时先查服务器锁、容器/进程、流量、marker、备份和迁移日志；状态不明确时不重跑 apply、不开放写、不启动旧镜像。
- [ ] dry-run/checksum 不稳定、blocker 非零、apply/verify 失败、启动/探针失败或开放后异常均进入专门故障步骤；使用 diagnosing-bugs 固化复现和安全状态。
- [ ] 不因“服务恢复 200”结束故障步骤；必须解释根因、数据状态、marker、digest、流量与后续恢复合同。
- [ ] Worker 完成不代表发布验收完成；收到 worker_done 后按 `credit-operational-value-issue-28-acceptance.md` 独立复核本地、服务器、数据库、API、浏览器、观察与回滚证据。

## 最终集成与全量回归

1. 记录 #28 共同基线、Worker HEAD、`merge-base`、提交列表、镜像构建 SHA/digest 与工作树清洁度。
2. 对可修复的代码/脚本问题返回原 Worker；对生产状态问题先保持安全状态并原地恢复，不机械重派。
3. Gate 全部通过后以 non-ff merge 集成 #28，记录 Worker SHA、最终 merge SHA 与生产 digest 的对应关系。
4. 在集成树执行后端全套、前端全套、i18n、build/copyright、三库代表性复验、32 CNY、真实浏览器和 `git diff --check`。
5. 独立只读确认生产当前 release/digest、marker、健康、写流量、观察窗口和备份仍与 evidence/runbook 一致。
6. 任一父 PRD 合同回归失败，#28/#19 保持 OPEN，并将最窄修复返回拥有该合同的原切片/Worker；协调器不暗中重写大段领域逻辑。

## Issue 关闭与资源回收

- [ ] #28 仅在独立 Gate、最终全量回归和生产状态确认通过后关闭；关闭评论列出集成 SHA、digest、备份/SHA-256、migration version/checksum、探针、观察和回滚结论。
- [ ] 核对 #20–#27 均已按各自验收证据关闭，且没有因 #28 发现的回归需要重新打开。
- [ ] 父 PRD #19 的 76 条 User Stories 与九个垂直切片均有可追溯证据后才关闭；父评论链接切片、最终集成 SHA、生产 release 和剩余风险。
- [ ] 停止/释放 #28 Worker，只回收本 Run 创建的 #28 工作树；按阶段确认其他本 Run 子工作树均已在对应 Issue 关闭后回收。
- [ ] 使用 Orca 原生命令删除节点，再运行原生 `git worktree prune`；不触碰主树、集成树（交付前）、`account`、`disk` 或其他会话工作树。
- [ ] 最终确认没有遗留 waiter、运行中 Agent、服务器发布锁、临时隧道、调试服务或测试数据库进程。
- [ ] 集成树最终状态、提交历史和生产证据完整后，按用户要求决定是否合入父工作树；未经明确合同不擅自覆盖主树未提交改动。

## 整体不放行条件

- #27 在任一前向 writer 未验收集成时派发，或 #28 在 #27 未合并/三库非零 SKIP时派发；
- 子工作树父节点错误，或 Worker 从 `origin/main`、生产提交、兄弟 Worker 分支启动；
- 等待超时被当作失败，导致重复 Agent/重复迁移探索；
- 历史价格浮点化、历史结果伪 exact、marker 部分 ready 或 ready 热路径自修；
- 发布使用漂移 tag、不同 digest、无一致备份、无停写 blocker 清零或无封闭启动；
- 生产插入临时数据、静态 interception 冒充 API，或隔离克隆证据冒充生产；
- 双写接受流量后 image-only rollback，或未知状态下重跑 apply/开放写；
- 三库、32 CNY、disabled-plan、request/conversion/recovery、前端/i18n、观察窗口任一缺失；
- 工作树不干净、恢复/交接证据未提交、包含敏感信息，或 Issues/父 PRD 提前关闭。
