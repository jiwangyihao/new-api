# Issue #28 可恢复发布 Runbook

> 本文件只记录脱敏命令形状、阶段、退出码、摘要和恢复点；禁止写入凭据、DSN、Cookie、dump 内容或用户数据。每个不可逆动作前后更新 `status.md` 与 `evidence.md`。

## 0. Read-back / preflight（只读）

- 候选分支：`jiwangyihao/issue-28-production-release`；发布前合并最新 `deploy/main`，最终以推送到远端 `main` 的确切 SHA 为唯一源码身份。
- #27：已合入提交 `e6ec10721` 的历史证据为 SQLite 3.50.4、MySQL 5.7.44、PostgreSQL 9.6.24 同一矩阵 36 阶段 PASS、`SKIP=0`；当前 shell 未提供 MySQL/PostgreSQL DSN，不能宣称当前候选三库重跑。
- 目标：仅 `ssh netcup-ows-migrate`；只读身份 hostname/vendor/product=`netcup-ows-migrate`/`netcup`/`KVM Server`。
- 最近只读生产基线：应用 digest=`sha256:62a5d95811923be881395265aaeddf5bb9176db55edc936a89722371ffd05976`、revision=`0a6995369c5f3755508567eaa2db5f363eb1d22f`，应用 healthy；部署前必须再次实测，禁止沿用文档中的旧 digest。
- 用户已授权推送 `main` 与 CI 构建；Orca 协调器生产写状态仍冻结/未授权，在该状态实际更新前不得执行生产远程写。

## 0.1 已完成本地门禁

- 默认前端 `bun test`：573 pass、0 fail；`bun run typecheck`、`bun run build:check`、`bun run copyright:check`、`bun run i18n:sync` 均通过。
- 经典前端 `bun run build` 通过；仅有既有非阻断警告。
- Go 代表性窄门禁含 `-race` 通过；外部矩阵 SQLite 3.50.4 阶段通过。缺少两条外部 DSN 的失败如实保留，不视为跳过后的 PASS。
- `server-release.test.sh` 的 `TEST_FILTER=full` 状态机合同通过；`server-release.sh` 与测试脚本语法检查通过。
- 用户明确取消本地 Linux/WSL 测试；严格 `0600` 权限合同不放宽，实际生产阶段由目标 Linux 满足。

## 1. 本地门禁

- `go test ./... -count=1`
- `go test ./model -run '<代表性门禁>' -race -count=1`
- `cd web/default && bun test`
- `cd web/default && bun run typecheck`
- `cd web/default && bun run i18n:sync`
- `cd web/default && bun run build:check`
- `cd web/default && bun run copyright:check`
- `git diff --check`
- CI 镜像：只接受推送到 `deploy/main` 的确切 SHA 对应的成功 `Build deployment image` run；从该 run 取得 `ghcr.io/jiwangyihao/new-api@sha256:<digest>`，记录 SHA、run ID、digest，禁止使用 `latest` 或漂移 tag。

## 2. 只读 dry-run

- 使用目标 digest 运行 `new-api credit-valuation-migrate --dry-run --version 1`
- 连续两次输出必须为单行 JSON，业务字段逐字节一致，checksum 相同
- 审阅 estimated/unknown、非法价格、unsupported currency、歧义和 blocker；任一 blocker 非零停止

## 3. 维护窗口

- 阶段前：协调器明确放行；写入 `status.md`
- `stop-writes`：关闭反代/API 写流量、停止后台任务；观察清零 HTTP writers、非终态预扣、异步 settlement、旧 writer sessions
- `backup`：PostgreSQL 一致备份；记录服务器绝对路径、大小、UTC、SHA-256；读取/恢复验证
- `apply`：同一 digest/version/checksum 执行 `--apply --version 1 --batch-size 100`
- `verify`：同一 digest/version 执行 `--verify --version 1`；checksum 与 apply 合同一致
- `start-closed`：所有实例同 digest 启动，写仍关闭；读取 marker/health/fail-closed
- `probe`：生产只读；32 CNY tracer 在隔离克隆执行
- `open-writes`：协调器显式放行后原子恢复，记录精确 UTC
- `observe`：记录固定窗口、聚合指标和结论

## 4. 回滚演练

A. ready 前：隔离环境旧镜像 image-only rollback，附加 schema 不变

B. ready 后、写未开放：停服回滚，保留 marker/附加 schema，重新迁移 verify

C. 双写接受流量后：禁止 image-only rollback；stop → suspend(reason) → 新备份 → 新版本重建/verify → 封闭启动 → 受控探针 → 放流

## 5. 中断恢复

若命令中断：先读取锁、容器、流量、marker、备份和迁移日志；状态不明时不重跑 apply、不开放流量、不启动旧镜像，向协调器 escalation。
## 6. 当前恢复入口

- 先完成候选提交、合并最新 `deploy/main`、解决冲突并复跑受影响门禁；随后将确切 SHA 推送到远端 `main`。
- 等待该 SHA 对应 CI run 成功并取得不可变 digest；CI 绿灯或 `latest` 标签都不能替代 digest 绑定。
- 生产远程写仅在 Orca 协调器授权状态实际更新后执行；按 dry-run→stop-writes→backup→apply/verify→start-closed→probe→open-writes→observe 顺序推进。
- 部署后同时核对容器镜像 digest、OCI revision、健康/API 探针与 `credit_valuation_migrations` 目标版本/状态；任一不一致保持写关闭并按合同恢复。
