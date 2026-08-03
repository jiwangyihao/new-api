# 第四执行波次共享合同（Issues #27 与 #28）

## 串行位置与派发门槛

本合同约束父 PRD #19 的历史迁移门禁和生产发布两个最终切片。它们严格串行，任何等待时长都不能替代依赖验收：

- Issue #27 只有在 #21、#22、#23、#24、#25、#26 全部由协调器验收并集成到 `jiwangyihao/credit-operational-value-integration` 后才能派发。它从该已集成提交创建隔离子工作树。
- Issue #28 只有在 #27 的代码、真实三数据库零 SKIP 矩阵、迁移命令、`ready/failed/suspended` 状态机和发布交接证据全部验收并集成后才能派发。它不得从 #27 worker 分支、`origin/main` 或生产提交直接开工。
- #27 不接触生产服务器，不构建或部署生产镜像，不关闭外部写流量；#28 不重新设计历史迁移算法，不绕过、不改弱 #27 的 verify 或门禁。
- 两者均不得关闭 GitHub Issue、父 PRD 或回收父工作树。只有协调器完成验收、集成和资源清理。

开工前必须读取已集成的 `.scratch/agent-progress/issue-20` 至 `issue-26` 合同与最终实现。缺少任一 writer、结构化来源、稳定 request identity、FX、转换或 recovery 接缝时，立即通过 Orca `orchestration ask` 报告精确符号、缺失合同和可继续范围；不得在 #27/#28 内复制上游实现。

## 所有权切分

### Issue #27：历史迁移与门禁主改者

#27 独占：

1. 从原始数据库 DECIMAL/SQLite 数值文本严格回填历史 `price_amount_micros`；非法历史行的稳定诊断和阻止 `ready`；
2. Credit 历史来源收集、稳定身份去重、`K/U/T/C/A/R` 保守比例重建以及 estimated/unknown 归类；
3. 历史 timed grant 的确定性恢复优先级和歧义 unknown；
4. `/new-api credit-valuation-migrate` 的 dry-run/apply/verify/repair/suspend 模式、独立维护初始化、稳定 JSON/checksum、CAS 与批次续跑；
5. `pending/running/ready/failed/suspended` 状态机、在途 blocker、全新空库自动 ready、ready 后 fail-closed、suspended 维护语义；
6. SQLite、MySQL 5.7.44、PostgreSQL 9.6.24 的同一 schema/迁移/运行时/并发矩阵和零 SKIP 证据。

#27 只能消费 #20 的附加列和只读非法值诊断；#20 不拥有历史回填或 ready 决策。#27 不得舍入、截断、静默写零或按当前套餐价格猜历史价格，不得重放高频请求日志猜消费顺序，不得把历史结果伪装为 exact。

### Issue #28：发布与生产证据主改者

#28 独占：

1. 最终全量门禁、不可变镜像构建与 digest 记录；
2. 服务器本地、带 `flock`/`trap` 的一次性发布流程，写流量/后台任务停止和恢复；
3. 一致数据库备份、SHA-256、同一 digest 的 dry-run/apply/verify、marker 检查和实例重启；
4. 健康、业务、停用计划边界、真实前端与五分析接口探针；
5. 强制双写开放后的观察窗口与 mismatch/missing/unknown/FX/结算/锁/负载证据；
6. ready 前、ready 后未开放写、强制双写已接受流量三阶段回滚边界，以及必须停写+suspend+新版本重建的演练结论。

#28 不得修改 migration marker 绕过 verify，不得把失败检查降级为 warning，不得在生产插入临时用户、套餐、订阅或权益，不得用静态资源拦截冒充 API/数据库行为证明。若真实运行暴露代码缺陷，先持久化复现、停止不可逆步骤并向协调器 escalation；不得在生产服务器上直接热改二进制或源码。

## #27 → #28 交接合同

#27 完成时除常规 `status.md`、`evidence.md`、`contract.md` 外，还要持久化 `.scratch/agent-progress/issue-27/release-handoff.md`，至少列出：

- 已验收提交和迁移版本；
- 维护命令完整 argv 合同、合法/非法模式组合和退出码；
- 独立数据库初始化方式，确认不会启动 HTTP、Redis、定时器或后台轮询；
- marker 表/状态、CAS、批次续跑、checksum 输入边界和 verify 不变量；
- 写流量、非终态预扣、异步 Task、旧进程会话 blocker 的稳定 reason；
- 三数据库版本、同一矩阵命令、PASS/零 SKIP 结果及脱敏证据路径；
- frozen 32 CNY fixture 的建立方式与五接口期望；
- ready 后强制双写、legacy Task、repair/suspend 的边界；
- #28 必须在生产重新观察、不能仅复用本地结论的项目。

不得在交接文件中保存 DSN、令牌、Cookie、私钥、数据库转储或其他密钥材料。

## 迁移状态与回滚不变量

- marker 非 ready：生产基线数量写继续运行，只捕获完整前向来源；不得创建半可信 Credit 状态，分析返回 `migration_not_ready`。
- dry-run/verify：完全只读，同一快照产生相同业务 JSON/checksum；运行时间等非确定字段不进入 checksum。
- apply：只在停写且 blocker 清零后运行；稳定主键批次、版本 CAS、可重跑 upsert，不覆盖已发生前向变更的状态。
- ready：每份 Credit 权益恰有一行匹配状态；以后所有数量写与估值同锁同事务，缺失/不一致时整笔失败。
- suspended：只允许停写维护下从 ready 携带原因进入；HTTP 写保持关闭，只允许验证、修复或新版本迁移。
- 强制双写接受流量后，旧镜像会制造数量/估值分叉，因此禁止 image-only rollback。必须先停写、suspend，再用新迁移版本重建并 verify。
- 附加 schema、immutable grant、ledger 和快照在任何常规回滚中都不得删除或改写。

## 证据与故障恢复

两个 worker 的第一项实际改动均须创建并提交 `.scratch/agent-progress/issue-<N>/{status,evidence,contract}.md`。每完成一个可编译或可验证小步即更新并 Conventional Commit。测试夹具、迁移输出摘要、checksum、发布动作和回滚状态必须尽快落盘，不能只存在终端滚屏或一次性大脚本里。

#28 每个不可逆动作前都要先在 `status.md` 记录当前服务、marker、镜像 digest、备份、流量和下一动作；将服务器脚本先保存为本地文件、审阅后再传到服务器，不在终端临时拼接大段脚本。意外中断后先检查服务、锁、marker、备份和运行中的迁移，再决定恢复；绝不盲目重跑 apply、开放写流量或启动旧镜像。

等待超时不是失败。协调器只在 Orca `worker-show` 明确证明 failed/stopped 后使用 `--retry-of`；可唤醒的模型/终端停滞应原地恢复。完成后 worker 只发送一次 `worker_done`，列出提交、验证、实际环境、未完成风险和证据路径。
