# Issue #28 发布合同

## 目标与边界

从已集成的 Credit 运营估值候选发布到生产；生产行为基线为 `f446a1569c2ced54a3fe438b5c4575659a59241d`。#27 的历史迁移算法、verify、marker 状态机和三库矩阵已验收，#28 负责集成最新 `deploy/main`、候选门禁、不可变镜像、生产停写切换、探针、观察和回滚。

## 部署目标

- SSH：仅 `netcup-ows-migrate`
- 工作目录：`/opt/new-api`
- Compose：`compose.yml` + `compose.release.yml`，按需要叠加 `compose.primary.yml`、`compose.replica.yml`、`compose.network.yml`
- 服务：`new-api`；依赖：`new-api-postgres`、`new-api-redis`
- 监听：`127.0.0.1:13080`、`127.0.0.1:13081`
- 镜像：GHCR immutable digest；禁止 `latest`、deploy tag 或服务器本机构建
- 目标身份：远端 hostname `netcup-ows-migrate`、vendor `netcup`、product `KVM Server`；旧 RackNerd/AutoDLChen 文字仅作冲突审计，不是目标
## 当前执行状态

- 用户已明确授权将候选推送到 `jiwangyihao/new-api` 的远端 `main` 并通过 CI 构建镜像。
- Orca 协调器的生产写操作能力仍显示冻结/未授权；推送与 CI 可继续，但在该状态实际更新前，禁止在 `netcup-ows-migrate` 执行 pull/up、停写、备份、迁移或其他远端变更。
- 现网外部写流量未核验；未执行 stop-writes，不能记录为关闭。
- 尚未发生生产远程写：未创建/传输脚本、未获取 flock、未备份、未 pull 镜像、未修改 compose、未 apply/verify、未重启、未执行生产写探针、未开放流量。
- 最近一次只读实测生产镜像为 `sha256:62a5d95811923be881395265aaeddf5bb9176db55edc936a89722371ffd05976`，revision 为 `0a6995369c5f3755508567eaa2db5f363eb1d22f`，容器 healthy；部署前仍须再次只读核验。


## 状态机

1. `preflight`：核验目标/当前 immutable digest、revision、当前容器和受管 Nginx gate；旧 runtime 仅用只读 SQL/健康检查形成 bootstrap 证据，不伪造 runtime/drain 能力
2. `stage-runtime`：写仍开放时切换到目标 digest；目标 runtime 启动后再验证带 token 的 runtime stats 与全 writer drain 能力
3. `read-only-dry-run`：写开放时连续执行两次兼容性预演；逐份验证完整合同、blocker 和诊断，但允许真实业务写入导致业务快照变化
4. `stop-writes`：反代/应用关闭外部写流量，停止后台任务；确认 HTTP 写、非终态预扣、异步结算和旧 writer 会话清零
5. `frozen-dry-run`：停写且全 writer drain 后连续执行两次；除 `fx.captured_at` 外规范化业务 JSON 必须相同，checksum 必须相同并冻结为审批/apply 输入
6. `backup`：冻结预演通过后创建 PostgreSQL 一致备份；记录绝对路径、UTC、大小、SHA-256 和可读/可恢复检查
7. `apply`：同一 digest、version、固定 batch size 100 和冻结 checksum 执行 marker CAS
8. `verify`：同一 digest/version/checksum 原子验证；失败保持写关闭
9. `start-closed`：所有实例同 digest 启动，外部写仍关闭；检查 ready、health、版本和 fail-closed
10. `probe`：生产只读健康/业务探针；无授权账号时隔离克隆完成 32 CNY 行为链路
11. `open-writes`：仅所有前置门禁通过后原子恢复外部写，记录 UTC、digest、marker
12. `observe`：记录健康、HTTP 错误率、state missing/mismatch、unknown、unsupported FX、结算延迟/replay、batch pending、PostgreSQL 锁 gauge/连接/写负载和资源

## 退出码

- `0`：阶段成功并满足合同
- `1`：运行时/数据库/迁移/健康/探针失败；保持写关闭并保留证据
- `2`：参数或状态合同错误；不执行不可逆动作
- `3`：安全阻断（身份、digest、marker、blocker、checksum、实例不一致）

## 不变量

- 同一源码提交构建出的所有应用实例使用同一 immutable digest
- 在线 dry-run/verify 均只读；在线两次预演各自满足兼容合同，停写后的两次冻结预演经规范化后业务 JSON 与 checksum 一致
- apply/verify 使用同一 migration version、digest、固定 batch size 100 和停写后冻结的输入 checksum
- marker 只允许合法 CAS 转换；verify 原子通过后才 ready
- 每份 Credit 权益恰有一行状态，available 与 token 数量一致，金额/未知量非负且 unknown 不超过 available
- 迁移前 blocker 必须为零：非终态预扣、缺稳定身份的活动异步任务、旧 writer 会话、Credit 计划缺失/歧义、估值币种/FX 无效
- 生产不插入临时用户、plan、order、subscription 或 entitlement
- static interception、health 200、容器 running 均不能冒充 API/DB/业务证明

## 备份与写入

- 所有远端变更先经协调器明确放行
- 服务器本地脚本必须已提交、已审阅，使用 `flock` 和 `trap`
- 失败默认保持写关闭，不自动开放流量、不删备份/日志、不启动旧镜像
- 备份 dump 不进入仓库；仅记录绝对路径/SHA-256/可恢复性摘要

## 回滚

- ready 前：旧镜像可回滚
- ready 后、开放写前：先停服再回滚，保留附加 schema/marker，重新迁移验证
- 双写接受流量后：禁止 image-only rollback；stop all writes → 原子 `suspend --reason` → 新一致备份 → 新 migration version 重建/verify → 重新封闭启动与探针
- 任一回滚不得删除 valuation state、immutable grant、ledger、request snapshot，不得把 marker 伪装为 pending
## 当前门禁结论

- 默认前端 `bun test` 为 573 pass、0 fail；`bun run typecheck`、`bun run build:check`、`bun run copyright:check`、`bun run i18n:sync` 均通过。
- 经典前端 `bun run build` 通过；仅有既有 Browserslist、第三方 `eval` 与 chunk 大小警告。
- Go 窄门禁（含 `-race`）通过；`TestCreditValuationExternalMatrix` 的 SQLite 3.50.4 阶段通过。#27 已有 MySQL 5.7.44/PostgreSQL 9.6.24 同一矩阵 36 阶段 PASS、`SKIP=0` 的历史证据，但当前 shell 未提供两条 DSN，不能冒充当前候选三库重跑。
- `server-release.test.sh` 的 `TEST_FILTER=full` 完整状态机合同通过，脚本语法检查通过。用户明确取消本地 Linux/WSL 权限测试；严格 `0600` 合同保持不变，由获授权后的目标 Linux 发布流程满足。
- 当前允许完成候选集成、推送和 CI 镜像构建；生产远程写仍须等待 Orca 协调器授权状态实际更新，并严格按本合同状态机执行。
