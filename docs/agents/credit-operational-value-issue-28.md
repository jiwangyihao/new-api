# Issue #28 发布 Agent 指令

## 目标与垂直交付

你负责父 PRD #19 的 GitHub Issue #28「安全切换生产并验证强制双写回滚边界」。必须在 Orca 为你创建的隔离子工作树中，从协调器已经验收并集成 #20–#27 的提交开始，完成本地最终门禁、不可变镜像构建、生产 dry-run/维护迁移/切换、真实业务与浏览器验收、观察窗口和回滚边界证据。

这是生产发布任务，不是“镜像能启动”或“静态页面能渲染”。你必须证明同一 digest、同一迁移版本、受控停写和可恢复的完整状态机，并留下可审计证据。禁止越界：不重新设计 #27 迁移算法，不降低 verify，不改 marker 伪造 ready，不在生产插入临时用户/套餐/订阅/权益，不在服务器热改源码或二进制，不使用漂移 tag，不在强制双写接受流量后 image-only rollback。

## 必读材料与 Skill

执行前依次阅读并服从：

1. 仓库及全局 `AGENTS.md`。
2. `issue://jiwangyihao/new-api/19` 与 `issue://jiwangyihao/new-api/28`；GitHub CLI 始终显式传 `--repo jiwangyihao/new-api`。
3. `docs/agents/credit-operational-value-execution.md`。
4. `docs/agents/credit-operational-value-wave-4-contract.md`；你是发布、生产证据和回滚边界的唯一主改者。
5. 已集成 `.scratch/agent-progress/issue-20` 至 `issue-27`，尤其 `issue-27/release-handoff.md`、三数据库矩阵、维护命令和 frozen fixture。任何依赖证据缺失先 Orca `orchestration ask`，不得在生产猜测。
6. `CONTEXT.md`、ADR 0001、ADR 0002、新规格第 13–15 节和实施计划任务 11。
7. 仓库现有 Docker/CI/部署配置及服务器现行 1Panel、Compose、OpenResty、PostgreSQL、Redis 约定；先只读探测再操作。

发布故障、性能回退、数据库 blocker 或探针异常时必须先读取 `skill://diagnosing-bugs`，复现并定位，不得靠反复重启掩盖。若为修复发布脚本或永久代码新增行为，先读取并执行 `skill://tdd`；若触及 `web/default`，先读 `skill://shadcn-ui`，新增/改变可见文本再读 `skill://i18n-translate` 并维护六语言。真实浏览器验收使用浏览器工具；若必须操作 Orca/桌面浏览器窗口，先读 `skill://computer-use`。不要因为本任务在 Orca worker 中运行而自行创建/回收其他工作树。

## 生产访问与安全边界

- RackNerd 访问必须遵守既有跳板：先到 `AutoDLChen`，再从跳板使用服务器既有密钥连接 `root@107.173.87.253`。不得改成未经验证的本机直连，不复制私钥，不把凭据写入仓库、`.scratch`、Orca 消息或日志。
- 开始时只读记录当前 release/commit/image digest、容器、端口、反代、数据库/Redis、磁盘/内存、marker（如存在）和健康状态。不得把根工作树最新 HEAD 当生产证据；已知生产行为基线是 `f446a1569c2ced54a3fe438b5c4575659a59241d`，以服务器实测为准。
- 保留所有用户自有工作树和主树 `CLAUDE.md`。不得修改受保护的 nеw-аρi/QuаntumΝоuѕ 标识、许可证或归属。
- 生产验证默认只读。只有已有明确授权的受控账号才能执行有写行为的业务探针；否则使用最新一致备份的隔离克隆完成 32 CNY 行为证明，生产库只做只读不变量和真实健康检查。

## 可恢复发布记录

第一项实际改动必须创建并提交：

- `.scratch/agent-progress/issue-28/status.md`：当前阶段、服务器状态、marker、流量、镜像、备份、最近安全提交、下一不可逆动作；
- `.scratch/agent-progress/issue-28/evidence.md`：本地门禁、构建 digest、迁移 checksum、探针、浏览器、监控和回滚演练证据；
- `.scratch/agent-progress/issue-28/contract.md`：部署拓扑、一次性脚本状态机、锁/cleanup、健康/业务探针、流量切换与三阶段回滚合同；
- `.scratch/agent-progress/issue-28/release-runbook.md`：可按步骤恢复的脱敏执行记录，只保存命令形状、摘要、路径、时间和状态，不保存密钥；
- `.scratch/agent-progress/issue-28/server-release.sh` 或仓库已有更合适的脚本位置：先在文件系统中编写、审阅和提交服务器本地脚本，再传输执行，禁止把不可恢复的大段 shell 临时粘进终端。

每个阶段、每个不可逆动作之前和之后立即更新状态并 Conventional Commit。远端长时间服务/日志用监督工具，不用失控后台 shell。意外中断时先检查服务器 `flock`、进程/容器、外部写流量、marker、备份和迁移日志；在不确定状态下绝不重跑 apply、开放写或启动旧镜像，先向协调器 escalation。

## 本地最终门禁与镜像

在任何生产变更前完成并记录：

- 后端全套 `go test ./... -count=1`；
- 前端全套 `bun test`、`bun run i18n:sync`（missing/extras=0）、`bun run build:check`、`bun run copyright:check`；
- `git diff --check`，无临时回归文件、空文件、测试账户/计划种子或未提交改动；
- #27 提供的真实 SQLite/MySQL 5.7.44/PostgreSQL 9.6.24 零 SKIP矩阵及并发/race 门禁，必要时在当前已集成提交重跑确认；
- frozen 32 CNY fixture 和五接口一致性。

从精确验收提交构建镜像，记录源码提交、构建命令、镜像 ID 与不可变 digest。部署只使用 digest，不使用 `latest` 或其他可漂移 tag。先证明新镜像内维护子命令、应用二进制和前端资源来自同一提交。若镜像未能重现本地门禁，停止发布并诊断。

## 服务器本地一次性流程

脚本必须使用服务器本地文件、`flock` 防并发和 `trap` 做清理/失败保守处置；显式状态机至少包含 preflight、read-only-dry-run、stop-writes、backup、apply、verify、start-closed、probe、open-writes、observe。脚本不得在失败时自动开放流量或删除证据。

1. **在线预演**：在不改生产数据的前提下，使用目标 digest 的 `credit-valuation-migrate --dry-run` 连续两次；记录脱敏稳定 JSON、相同 checksum、estimated/unknown、非法价格、unsupported currency、歧义和 blocker。输出为空或异常狭窄必须调查。
2. **停写门槛**：通过反代/应用层关闭所有外部写流量并停止后台任务；确认 HTTP 写、非终态预扣、异步结算和旧进程 DB 写会话清零。健康只读入口可按 runbook 保留，但不能有旁路 writer。
3. **一致备份**：按现有 PostgreSQL 惯例创建一致备份，记录服务器绝对路径、大小、时间和 SHA-256，并做可读取/可恢复性检查；不得把 dump 拉入仓库。
4. **迁移**：用同一个镜像 digest 和冻结 migration version 运行 apply，再运行 verify。任一 marker、数量、币种、非负、unknown 上界、来源唯一性、checksum 或 blocker 失败时保持写关闭，不启动业务。
5. **封闭启动**：marker ready 后，让所有实例以同一 digest 重启并在外部写仍关闭时读取门禁。确认版本、健康、数据库连接、旧分析兼容和 fail-closed 行为。
6. **受控探针**：仅有授权账号时才执行购买、消费、少结算/退款和 disabled-plan 既有权益消费；否则在备份隔离克隆执行写入 tracer。生产始终验证只读不变量和真实业务健康。
7. **开放流量**：所有受控门禁通过后才原子恢复外部写流量，记录准确时间；开放后即进入禁止 image-only rollback 阶段。

## 32 CNY、前端与边界验收

最终行为证据必须同时满足：充值层级 `40 CNY / 1,000 Credit`，消费 200，`end_time=0`，可用 800，`exact_cost_micros=32,000,000 CNY`，`active_paid_subscription_count=1`，estimated=0，unknown=0；summary/users/subscriptions/plans/sources 五个分析接口一致。

- 如果生产已有授权受控账号，以真实 API/DB 链路执行并保留脱敏证据；否则在最新生产备份隔离克隆运行完全相同版本和 fixture。必须明确证据来自生产还是隔离克隆，不能混写。
- 真实生产前端必须通过已认证 API 展示 32 CNY、exact/estimated/unknown、Credit 时间价值“不适用”和必要 warning。静态资源 interception 只能证明渲染，不能证明 API 激活或数据库状态。
- 复核 `duchuanbo` 边界：已有 disabled-plan entitlement 继续消费；新购买、兑换、转换、管理员 grant 拒绝 disabled；所有套餐继续忽略 model scope。
- 不创建临时生产数据。若缺授权条件，不能为了“完成验收”绕过此边界。

## 观察窗口与回滚边界

开放流量后持续观察并记录：健康/错误率、`credit_valuation_state_missing/mismatch`、unknown 增长、unsupported FX、请求结算重放/延迟、coalescer batch、PostgreSQL 锁等待、连接/写负载、CPU/内存/磁盘和用户可见错误。只记录脱敏聚合，不记录用户名、密钥或支付载荷。异常不能用重启清零后宣称通过；先诊断根因。

在隔离环境或开放写之前实际演练并记录三阶段合同：

- ready 前：旧镜像可忽略附加结构，允许 image-only rollback；
- ready 后但外部写未开放：停服后可回滚，保留附加结构和 marker，随后重新迁移；
- 强制双写接受流量后：禁止 image-only rollback。优先向前修复；必须回滚时先停止所有写，通过维护命令把 ready 原子置为 suspended 并记录原因，再回滚镜像；恢复服务前必须使用新 migration version 从新备份重建并 verify。

任何回滚均不得删除估值表、grant、ledger、请求快照，不得覆盖 immutable 历史，不得把 marker 伪装为 pending。

## 故障处理

- dry-run/checksum 不稳定：停止，使用 diagnosing-bugs 找非确定字段或漂移写入；不得“取第二次为准”。
- blocker 非零：保持写关闭或不进入维护，等待终态/停止旧 writer；不得删除预扣或 Task 伪造清零。
- apply/verify 失败：保持服务和写流量关闭，保存 marker/日志/备份，判断可重跑边界；不自动恢复旧镜像。
- 启动/探针失败：外部写保持关闭；从同 digest 定位，不用不同镜像试错。
- 开放后失败：默认向前修复；若必须回滚，严格 stop→suspend→新版本重建合同，并先 escalation。
- Agent/终端中断：协调器只在 worker 明确 failed/stopped 后重派；你必须让状态文件足够下一 worker从当前服务器状态继续，而不是重复探索或重做不可逆步骤。

## 完成条件

逐条复核 Issue #28 的 13 条 acceptance criteria。完成记录必须包含：精确源码提交和镜像 digest；备份绝对路径/SHA-256；两次 dry-run 和 apply/verify checksum；marker/迁移版本；三数据库零 SKIP；本地全量门禁；健康/业务/disabled-plan/五接口/浏览器证据；生产或隔离克隆的明确归属；开放流量时间与观察窗口；三阶段回滚演练结论；当前生产 release 状态和遗留风险。

完成前提交全部代码、脚本和脱敏恢复记录，保持工作树干净；不要提交 dump、凭据、Cookie 或完整敏感日志。随后在当前 Dispatch 只发送一次 `worker_done`，列出提交 SHA、镜像 digest、备份路径/SHA-256、迁移/checksum、探针、观察、回滚结论、风险和进度目录。不要自行关闭 #28/#19、合并或回收工作树，等待协调器验收。
