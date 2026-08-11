# Issue #28 协调器验收矩阵：生产切换与强制双写回滚边界

本清单由协调器在 Issue #28 Worker 发出 `worker_done` 后执行。生产发布证据必须来自实际执行和可恢复记录；“镜像能启动”、静态页面渲染或 Agent 自报不能替代验收。任何不可逆步骤状态不明确时，保持写关闭并先诊断，不盲目重跑迁移、开放流量或启动旧镜像。

## Gate A：Worker、基线与发布边界

- [ ] Orca Dispatch 收到唯一一次 `worker_done`；记录 Task、Dispatch、终端、子工作树完整 ID、父工作树 ID、共同基线 SHA 和 Worker HEAD。
- [ ] #28 子工作树的 Orca parent 是集成工作树，`merge-base` 包含已验收集成的 #20–#27；不能从 `origin/main`、生产基线或 #27 Worker 分支直接派生。
- [ ] Worker 工作树干净；`.scratch/agent-progress/issue-28/{status,evidence,contract,release-runbook}.md` 与审阅后的服务器脚本均已提交，记录的最近安全提交与实际 HEAD 一致。
- [ ] 逐条复核 Issue #28 的 13 条 acceptance criteria；每条有命令、digest、数据库、API、浏览器或监控证据，不能以计划/TODO 代替。
- [ ] 未重新设计 #27 迁移算法、弱化 verify、伪造 marker、热改服务器源码/二进制、使用漂移 tag、插入临时生产用户/套餐/订阅/权益或复制凭据。
- [ ] 所有生产访问均通过已配置的 `netcup-ows-migrate` SSH 主机别名；证据和提交不含私钥、DSN、令牌、Cookie、dump 或可识别用户数据。
- [ ] 主树 `CLAUDE.md`、用户工作树、受保护项目标识和许可证未被修改。

## Gate B：本地最终门禁与不可变镜像

- [ ] 精确验收提交上 `go test ./... -count=1` 全部通过；无基于临时删除测试、复制旧前端 dist 或缩小包范围的伪通过。
- [ ] `web/default` 的 `bun test`、`bun run i18n:sync`（missing/extras=0）、`bun run build:check`、`bun run copyright:check` 全部通过。
- [ ] #27 的真实 SQLite/MySQL 5.7.44/PostgreSQL 9.6.24 零 SKIP、并发/race 和 frozen 32 CNY 门禁在当前集成提交上可追溯；必要的代表性路径已重跑。
- [ ] `git diff --check` 通过，无临时回归文件、空文件、测试账户/计划种子、敏感输出或未提交改动。
- [ ] 从该精确提交构建镜像，记录源码 SHA、构建命令、镜像 ID 和不可变 digest；生产部署只引用 digest，不引用 `latest` 或漂移 tag。
- [ ] 镜像内维护子命令、Go 二进制版本和前端资源均可证明来自同一源码提交；不存在本机二进制+旧资源或镜像外脚本漂移。
- [ ] 镜像环境中维护 CLI 与应用启动 smoke 通过；若和本地门禁不一致，发布在生产变更前停止并诊断。

## Gate C：只读生产基线与可恢复脚本

- [ ] 任何生产写操作前，只读记录当前 release/commit/image digest、容器、端口、反代、PostgreSQL/Redis、磁盘/内存、marker 和健康状态；生产实测优先于根工作树 HEAD。
- [ ] 服务器脚本先作为文件写入、审阅、提交并传输；脚本使用 `flock` 防并发、`trap` 保守清理，禁止在终端临时拼接不可恢复大段 shell。
- [ ] 脚本显式状态机至少包含 preflight、read-only-dry-run、stop-writes、backup、apply、verify、start-closed、probe、open-writes、observe。
- [ ] 失败路径默认保持外部写关闭并保留证据，不自动开放流量、不删除备份/日志、不启动旧镜像。
- [ ] 每个不可逆动作前后，`status.md` 和 runbook 记录服务、锁、marker、流量、digest、备份、迁移状态与下一动作；中断后可据此判定恢复点。
- [ ] 在线使用目标 digest 连续执行两次 dry-run，业务 JSON/checksum 相同；输出包含 estimated/unknown、非法价格、unsupported currency、歧义和 blocker，空或异常狭窄结果已调查。
- [ ] 未将生产静态资源 interception、健康 200 或容器 running 冒充迁移/API/数据库行为证明。

## Gate D：停写、备份、迁移与封闭启动

- [ ] 通过反代/应用层关闭所有外部写流量，并停止后台任务；HTTP 写、非终态预扣、异步结算及旧进程可写 DB 会话均有可观察的清零证据。
- [ ] 按现有 PostgreSQL 惯例创建一致备份，记录服务器绝对路径、大小、UTC 时间和 SHA-256，并验证备份可读取/可恢复；dump 不进入仓库。
- [ ] apply 与 verify 使用构建阶段记录的同一镜像 digest、同一 migration version 和冻结 checksum 合同。
- [ ] apply/verify 任一失败时，外部写与业务启动保持关闭；marker、日志、备份和稳定重跑边界已保存，不自动回退旧镜像。
- [ ] verify 原子通过后 marker 为预期 ready；数量、币种、非负、unknown 上界、来源唯一性、状态版本和 checksum 均符合 #27 交接。
- [ ] 所有实例以同一 digest 在外部写仍关闭时启动；确认版本、健康、数据库连接、旧分析兼容、ready 读取和 state missing/mismatch fail-closed。
- [ ] 没有旁路旧实例、旧定时任务或可写旧会话在封闭启动后继续写数据库。

## Gate E：业务、32 CNY 与真实前端探针

- [ ] 生产探针默认只读；只有已存在且明确授权的受控账号才允许生产写探针，否则在最新一致备份的隔离克隆执行相同版本和写入 fixture。
- [ ] 证据明确标注“生产”或“隔离克隆”，不混写；生产未为验收创建临时用户、计划、订单、订阅或权益。
- [ ] `40 CNY / 1,000 Credit`、消费 200、`end_time=0` 的完整领域/API 链路得到可用 800、`exact_cost_micros=32,000,000 CNY`、active count 1、estimated 0、unknown 0。
- [ ] summary/users/subscriptions/plans/sources 五个分析接口对同一 fixture 一致；不是直接插最终估值状态或只断言内部 helper。
- [ ] 真实生产前端经已认证 API 展示 32 CNY、exact/estimated/unknown、Credit 时间价值“不适用”和必要 warning；浏览器证据来自实际 bundle/API，静态 interception 只可作为渲染补充。
- [ ] 复核 `duchuanbo` 边界：既有 disabled-plan entitlement 继续消费；新购买、兑换、转换、管理员 grant 拒绝 disabled；模型范围仍被忽略。
- [ ] 健康、登录/鉴权（在授权范围内）、旧 API 兼容、前端资源和核心只读业务探针均通过，无隐式错误或错误码降级。

## Gate F：开放流量与观察窗口

- [ ] 仅在 Gate A–E 全部通过后原子恢复外部写流量，并记录准确 UTC 时间、digest、marker 和批准依据。
- [ ] 开放后所有实例仍为同一 digest，强制数量/估值双写已实际接受流量；从这一时刻开始明确禁止 image-only rollback。
- [ ] 观察窗口覆盖健康、错误率、`credit_valuation_state_missing/mismatch`、unknown 增长、unsupported FX、请求结算重放/延迟、coalescer batch、PostgreSQL 锁等待/连接/写负载、CPU/内存/磁盘。
- [ ] 观察证据为脱敏聚合，不记录用户名、密钥或支付载荷；异常不会通过重启清零后宣称通过。
- [ ] 观察期间出现异常时使用 diagnosing-bugs 固化复现和根因；不可逆状态不明时保持或重新关闭写并 escalation。
- [ ] 观察窗口长度、开始/结束时间、关键指标基线与结论写入 evidence/runbook；“看起来正常”不是证据。

## Gate G：三阶段回滚边界

- [ ] 在隔离环境或生产开放写之前实际演练 ready 前回滚：旧镜像可忽略附加结构，允许 image-only rollback，数据和 marker 不被删除或伪造。
- [ ] 实际演练 ready 后但外部写未开放：先停服再回滚，保留附加结构/marker，随后重新迁移并 verify。
- [ ] 实际证明强制双写接受流量后禁止 image-only rollback；不能通过仅写 runbook 或口头声明满足。
- [ ] 双写后必须回滚时的合同是 stop all writes → 原子 suspend（携带 reason）→ 回滚镜像 → 从新一致备份使用新 migration version 重建 → verify 后恢复。
- [ ] 任一回滚路径都不删除估值状态、immutable grant、ledger、请求快照，不覆盖不可变历史，不把 marker 伪装为 pending。
- [ ] 发布脚本和 runbook 对 dry-run 不稳定、blocker 非零、apply/verify 失败、启动/探针失败、开放后失败分别给出保守动作；不存在失败自动放流。

## Gate H：证据、当前状态与资源清理

- [ ] 完成记录包含精确源码 SHA、镜像 digest、备份绝对路径/SHA-256、两次 dry-run 与 apply/verify checksum、marker/migration version。
- [ ] 完成记录包含三数据库零 SKIP、本地全量门禁、健康/业务/disabled-plan/五接口/浏览器、生产或隔离克隆归属、开放流量时间和观察窗口。
- [ ] 完成记录包含三阶段回滚演练、当前生产 release/digest/marker/流量状态、已知风险和恢复入口。
- [ ] 所有本地脚本、恢复记录和脱敏证据已提交；生产日志、dump、凭据和敏感值未提交。
- [ ] 协调器独立复查服务器只读状态与关键证据，确认 Agent 结束后服务处于预期健康 release，而非迁移中、锁持有、写关闭或旧镜像状态。
- [ ] GitHub Issue #28 和父 PRD #19 在协调器完成最终全量验收前保持 OPEN；Worker 未自行关闭 Issue 或回收工作树。

## 集成、关闭与交付

1. 记录集成树基线、Worker HEAD、`merge-base`、提交列表和工作树清洁度。
2. 按 Gate A–H 验收；可修复代码/脚本问题返回原 Worker，生产状态问题优先保持安全状态并原地恢复，不轻率重派。
3. 代码与证据通过后以 non-ff merge 集成，提交信息使用 `feat(release): 集成 Credit 估值生产发布`；若发布过程中已需使用精确 worker SHA，记录其与最终 merge SHA 的关系。
4. 在集成树执行最终全量回归、`git diff --check` 和证据一致性检查；独立只读确认生产 digest、健康、marker、流量和观察结论。
5. 所有父 PRD acceptance criteria 与 #20–#28 组合合同均通过后，关闭 #28；评论包含集成 SHA、生产 digest、备份路径/SHA-256、migration version/checksum、探针、观察与回滚结论。
6. 逐项确认 #20–#27 已按证据关闭，再关闭父 PRD #19；父评论链接九个切片、最终集成 SHA、生产 release 和未隐藏风险。
7. 停止/释放 #28 Worker，仅回收本 Run 创建的 #28 工作树；随后按阶段清理本 Run 已完成子工作树并 `git worktree prune`，保留主树、集成树（直至最终交付）、`account`、`disk` 和其他会话资源。

## 不放行条件

- #28 未从包含已验收 #27 的干净集成基线派生；
- 本地全量测试、三库零 SKIP、32 CNY 或前端/i18n 任一门禁失败；
- 使用漂移 tag、不同 digest 执行迁移与服务，或镜像内容不能追溯同一提交；
- 无 `flock`/`trap`、大段临时 shell、无一致备份/SHA-256 或失败自动放流；
- blocker 未清零便 apply/ready，verify 被弱化，或旧 writer/实例仍可写；
- 为生产验证插入临时数据，或把隔离克隆、静态 interception 冒充生产 API 证据；
- 32 CNY、disabled-plan、五接口或真实前端任一未通过；
- 双写接受流量后执行或计划 image-only rollback；
- 观察窗口缺失、异常被重启掩盖、生产结束状态不明确；
- 恢复记录/证据未提交、工作树不干净、含敏感材料或父 PRD 提前关闭。
