# Issue #28 发布证据

## 证据边界

本文件只记录可复核命令、脱敏摘要、校验和与状态；不记录 DSN、令牌、Cookie、私钥、数据库 dump、完整生产日志或可识别用户数据。生产结论必须标注“生产”或“隔离克隆”，静态资源渲染不能替代已认证 API/数据库证据。

## 硬阻断：Issue #27

- 集成 HEAD：`0d85b9f14a8b2170f6c769b64602068105fe6184`
- #27 验收提交：`e6ec1072104a826a7a572dd55cf9c0422f2b3d8d`
- 集成验证：`git merge-base --is-ancestor e6ec1072104a826a7a572dd55cf9c0422f2b3d8d HEAD` 通过
- 已合入 #27 交接证据：SQLite `3.50.4`、MySQL `5.7.44`、PostgreSQL `9.6.24` 同一矩阵 36 阶段 PASS、`SKIP=0`；该结果来自已合入交接记录，不能替代当前候选重跑
- 早期候选命令：`go test ./model -run 'TestCreditValuationExternalMatrix$' -count=1 -v -timeout 60m`；退出码 `1`，MySQL/PostgreSQL 因 `TEST_MYSQL_DSN` / `TEST_POSTGRES_DSN` 未提供而 Fatal，Gate F 未完成。该记录由后续定向复跑细化，不能替代最新结果。
- 冻结业务结果（仅 #27 已合入证据）：40 CNY / 1,000 Credit，消费 200，剩余 800，`exact_cost_micros=32000000` CNY，五分析接口一致，`active_paid_subscription_count=1`，estimated=0，unknown=0

## 强制 Read-back 与主机身份

- 当前候选 HEAD：`f1434499bf3ab4669b741d9bc6ff12a442f977bb`（发布脚本与本证据尚待提交）
- 生产行为基线：`f446a1569c2ced54a3fe438b5c4575659a59241d`
- 候选与最新 `deploy/main` 的共同祖先：`73c658daa8e7954cb6f229348aac80287253391c`；合并完成后以最终推送 SHA 替换本值
- SSH 别名：`netcup-ows-migrate`
- 远端原始只读输出：`hostname=netcup-ows-migrate`、`sys_vendor=netcup`、`product_name=KVM Server`
- 目标裁决：协调器已明确接受 Netcup 为现行生产目标；旧 RackNerd/AutoDLChen 目标禁止访问
- 冲突审计：提交 `737a6b02c` 将既有 RackNerd/AutoDLChen 访问约定更正为 `netcup-ows-migrate`；该更正与远端身份输出一致。发布前后不得省略这一冲突历史。

## 本地候选门禁（合并前）

- Go 全套：`go test ./... -count=1` 的唯一实际失败为 `TestCreditValuationExternalMatrix/mysql` 与 `/postgres` 缺少外部 DSN；原始日志 `artifact://15257`。SQLite 3.50.4 阶段完成。
- 外部矩阵定向：`artifact://15263`；SQLite 3.50.4 的 schema、锁、迁移、生命周期、转换、恢复和并发阶段完成；MySQL/PostgreSQL 因 DSN 未提供而 Fatal，不能宣称当前候选三库通过。
- #27 历史 Gate F：提交 `e6ec10721` 已记录 SQLite 3.50.4、MySQL 5.7.44、PostgreSQL 9.6.24 同一矩阵 36 阶段 PASS、`SKIP=0`；该证据有效但不替代最终合并候选重跑。
- Go 窄门禁：估值迁移/请求/结算相关定向 `-race` 命令退出码 `0`。
- 默认前端：`bun test` 为 573 pass、0 fail；`bun run typecheck`、`bun run build:check`、`bun run copyright:check`、`bun run i18n:sync` 均通过。
- 经典前端：`bun run build` 通过；仅有既有 Browserslist、第三方 `eval` 与大 chunk 警告。
- 发布脚本：`env TEST_FILTER=full bash .scratch/agent-progress/issue-28/server-release.test.sh` 完整状态机合同通过；输出覆盖 preflight、stop-writes、backup、stage-schema、双 dry-run、apply、verify、start-closed、probe、open-writes、observe、rollback-suspend 及幂等重放。`bash -n server-release.sh` 与 `bash -u -n server-release.test.sh` 通过。
- 本地 Linux/WSL 测试按用户决定取消；严格 `0600` 权限合同未放宽，由获授权后的目标 Linux 发布流程满足。
- `git diff --check` 通过；合并最新 `deploy/main` 后必须复跑受影响门禁。

## 生产只读预检

- 目标：仅 SSH `netcup-ows-migrate`；最新只读身份输出为 `netcup-ows-migrate` / `netcup` / `KVM Server`。
- 当前生产：`new-api` digest=`sha256:62a5d95811923be881395265aaeddf5bb9176db55edc936a89722371ffd05976`，revision=`0a6995369c5f3755508567eaa2db5f363eb1d22f`，容器 `running healthy`，`RestartCount=0`；`127.0.0.1:13080/api/status` 返回 `success=true`，版本 `deploy-20260813-0a69953`。
- 当前 `/opt/new-api/compose.release.yml` 固定上述 immutable digest；生产目录存在基础/network/primary/replica/release Compose、audits、backups 与 migration-prep。
- 现网外部写流量未核验；Orca 生产写操作授权仍冻结/未授权。用户已授权推送与 CI，但这不等于 Orca 生产写放行。
- 未执行：远程脚本传输、flock、stop-writes、备份、镜像 pull、compose 修改、apply、verify、重启、生产写探针、open-writes。

## 业务、浏览器与监控

- 生产只读健康已观察到；真实认证前端/Chromium、隔离克隆 32 CNY 行为证明、disabled-plan 探针、五接口生产证据均未执行或未完成。
- 开放流量观察窗口未执行；mismatch/missing、unknown、unsupported FX、settlement latency、DB lock wait、error/write load 未形成发布窗口证据。
- 静态资源或健康 200 未被冒充为 API/DB/业务证明。

## 回滚演练

- ready 前旧镜像回滚、ready 后未开放写停服回滚、双写接受流量后禁止 image-only rollback 三阶段均未实际演练；仅保留合同，不宣称通过。

## 当前结论

- 候选推送与 CI 构建已获用户授权；生产远程写仍被 Orca 协调器授权状态冻结。
- 最终 CI 产物必须绑定推送到 `deploy/main` 的确切 SHA：等待该 SHA 对应 `Build deployment image` run 成功并取得 `ghcr.io/jiwangyihao/new-api@sha256:<digest>`；禁止使用 `latest` 或仅凭 CI 绿灯。
- 获生产写授权后，必须按 dry-run→stop-writes→backup→apply/verify→start-closed→probe→open-writes→observe 状态机执行，不能直接套通用 pull/up。
- 部署后必须同时验证容器 digest、OCI revision、健康/API 探针与 `credit_valuation_migrations` 目标 version/status；任一不一致保持写关闭。
- 当前尚未发生任何生产写操作，也未关闭 Issue #28 或父 #19。


## Windows 权限语义历史证据（本地 Linux 验收已取消）

- 当前 runner 为 `Windows_NT`；`stat -f -c '%T'` 对临时文件报告 `NTFS`。`mktemp -d` 返回 `C:\Users\34404\AppData\Local\Temp\...`，因此 `/tmp` 映射到 Windows 临时目录而非 POSIX 文件系统。
- 在同一临时目录创建 `state`、`state.sha256`、`approval`，分别执行 `chmod 0600` 后，实际 `stat -c '%a'` 均为 `644`；执行 `mv -f` 原子替换后，三者仍均为 `644`。该结果只证明 Windows runner 不能模拟 POSIX 权限，不再构成发布阻断。
- 用户明确要求不进行本地 Linux 测试；不再安装或探测 WSL/Linux，也不再要求本地原生 POSIX 验收。
- 严格权限合同未放宽：`verify_state_integrity` 和 `require_approval` 的 `0600` 检查保持不变，获授权后的目标 Linux 发布流程必须实际满足它们。
- 未执行任何生产写操作、部署或切流量。
## 权限合同双层复验（2026-08-13）

- Windows 定向命令：`env TEST_FILTER=permission bash .scratch/agent-progress/issue-28/server-release.test.sh`；实际输出包含 `PASS: simulated mode 600 accepted`、`PASS: permission override state rejected`、`PASS: permission override checksum rejected`、`PASS: permission override approval rejected`、`SKIP: POSIX mode semantics unavailable`、`PASS: permission contract only`、`DIRECT_RC=0`。
- 定向用例确认 `run_permission_override_case` 仍覆盖 state、checksum、approval 三种不安全权限；脚本函数定义位于 `TEST_FILTER=permission` 分支之前。
- POSIX 环境探测：`wsl.exe --list --verbose` 返回 WSL 安装提示并以退出码 `1` 结束，未提供可用 Linux 发行版；`test_real_posix_permission_contract` 因此输出 `SKIP`，该门禁未宣称通过。
- Shell 语法：`bash -u -n .scratch/agent-progress/issue-28/server-release.test.sh && bash -n .scratch/agent-progress/issue-28/server-release.sh` 无输出并成功退出。
- Windows 完整发布 stub 回归：独立命令 `timeout 900s bash .scratch/agent-progress/issue-28/server-release.test.sh` 的外层回执为 `RAW_RC=1`；`/tmp/issue28-full-rerun-20260813.log` 仅含 `started_at`、`finished_at ... rc=1`，没有可复核的阶段末尾输出。因此完整 stub 回归未通过，既有 `stop-writes` 权限语义阻断仍有效。
- 生产脚本仍严格要求 state、checksum sidecar、approval 为 `0600`；未为 Windows 放宽权限检查。
## 编排失败交接

- 已按 `worker_done --outcome failed` 尝试发送任务 `task_d53f2a82f939`，使用的 Dispatch 为 `ctx_b4eb0587374d`。
- Orca 明确拒绝：`dispatch_capability_invalid`，原因是该 Dispatch capability 已撤销；`dispatch-show` 同时确认 Dispatch 状态为 `failed`，`task-list --status dispatched` 为空。
- 因无有效 Dispatch 身份，无法发送被接受的 `worker_done`；该失败交接事实不视为成功，也不改变 `blocked/failed` 结论。

## 用户决定（2026-08-14）

- 用户明确要求不进行本地 Linux 测试；本地 POSIX `0600` 验收任务已取消，不再作为当前发布阻断。
- 不再轮询 WSL/Linux 环境；严格 `0600` 合同仍由获授权后的目标 Linux 发布流程执行。
- 生产写授权保持冻结。
- 实际未提交改动：`.scratch/agent-progress/issue-28/evidence.md`、`.scratch/agent-progress/issue-28/status.md`、`.scratch/agent-progress/issue-28/server-release.sh`、`.scratch/agent-progress/issue-28/server-release.test.sh`。未 commit、stash 或删除证据。