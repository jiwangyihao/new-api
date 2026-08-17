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

- 最终候选 HEAD：`9ffa6391db5cfc0a20246f6c5a1aeda4c3682d1a`
- 生产行为基线：`f446a1569c2ced54a3fe438b5c4575659a59241d`
- `deploy/main` 提交 `0a6995369c5f3755508567eaa2db5f363eb1d22f` 是最终候选祖先；最终候选已推送到远端 `main`
- SSH 别名：`netcup-ows-migrate`
- 远端原始只读输出：`hostname=netcup-ows-migrate`、`sys_vendor=netcup`、`product_name=KVM Server`
- 目标裁决：协调器已明确接受 Netcup 为现行生产目标；旧 RackNerd/AutoDLChen 目标禁止访问
- 冲突审计：提交 `737a6b02c` 将既有 RackNerd/AutoDLChen 访问约定更正为 `netcup-ows-migrate`；该更正与远端身份输出一致。发布前后不得省略这一冲突历史。

## 最终合并候选门禁

- 唯一合并冲突位于 `controller/subscription_payment_kyren_test.go`；采用远端新的 `CompleteSubscriptionOrderTx` 完成入口，并保留候选估值 plan/snapshot 夹具。五个 Kyren 终态/事件定向测试退出码 `0`。
- 默认前端全套：575 pass、0 fail；Credit 调整重试单文件：8 pass、0 fail。
- 默认前端构建门禁：`bun run build:check`、`bun run copyright:check`（1108 项）、`bun run i18n:sync` 均通过。
- 经典前端：首次 `bun install --frozen-lockfile` 失败，暴露 `package.json` 的 axios 1.15.2 与 `bun.lock` 的 1.15.0 不一致；运行常规 `bun install` 只更新锁文件 5 行，随后冻结安装通过，生产构建通过；仅有既有非阻断警告。
- Go 非矩阵全套：`go test ./... -skip '^TestCreditValuationExternalMatrix$' -count=1` 结果为 53 packages ok、59 no tests；原始输出 `artifact://15341`。该命令明确跳过 Gate F，不能称为完整 Go 或三数据库门禁。
- Go 估值核心合并门禁：迁移/请求/结算相关 `-race` 定向命令退出码 `0`。
- #27 历史 Gate F：提交 `e6ec10721` 已记录 SQLite 3.50.4、MySQL 5.7.44、PostgreSQL 9.6.24 同一矩阵 36 阶段 PASS、`SKIP=0`；当前 shell 未提供 MySQL/PostgreSQL DSN，最终合并候选没有新鲜 Gate F 结果。
- 发布脚本：合并后 `env TEST_FILTER=full bash .scratch/agent-progress/issue-28/server-release.test.sh` 完整状态机合同通过；输出覆盖所有正向、幂等与 rollback-suspend 阶段。
- 本地 Linux/WSL 测试按用户决定取消；严格 `0600` 权限合同未放宽，由获授权后的目标 Linux 发布流程满足。
- `git diff --check` 通过。`ghcr-deploy.yml` 仅构建/推送镜像，不运行测试；CI 成功只能证明该 SHA 镜像构建成功，不能证明 Gate F 或发布安全。

## 生产只读预检

- 目标：仅 SSH `netcup-ows-migrate`；最新只读身份输出为 `netcup-ows-migrate` / `netcup` / `KVM Server`
- 当前生产：`new-api` digest=`sha256:62a5d95811923be881395265aaeddf5bb9176db55edc936a89722371ffd05976`，revision=`0a6995369c5f3755508567eaa2db5f363eb1d22f`，容器 `running healthy`，`RestartCount=0`；`127.0.0.1:13080/api/status` 返回 `success=true`，版本 `deploy-20260813-0a69953`
- 当前 `/opt/new-api/compose.release.yml` 固定上述 immutable digest；生产目录存在基础/network/primary/replica/release Compose、audits、backups 与 migration-prep
- 用户已直接授权立即部署并要求忽略 Orca；Orca 失败状态仅保留为历史审计，不再作为当前授权阻断
- 未执行：远程脚本传输、flock、stop-writes、备份、镜像 pull、compose 修改、apply、verify、重启、生产写探针、open-writes
- 最终候选镜像：GitHub Actions run `31810007737` 成功；OCI revision=`9ffa6391db5cfc0a20246f6c5a1aeda4c3682d1a`；immutable digest=`sha256:64266b6f36948fa083b12a17c5d19c659398aa0b1f4d61f026bf48d2df7e7b90`；目标 digest 尚未拉到生产主机
- 生产 PostgreSQL 只读检查未发现 `credit_valuation%` 表，符合旧镜像尚未 stage schema；这不是迁移通过证据
- `/opt/new-api/migration-prep` 仅有旧 PRD9 脚本；没有 Issue #28 config、`server-release.sh`、write-gate、production-probe、clone-probe、observe Hook 或 approval 文件
- 仓库只有通用发布状态机与 stub 合同测试，没有真实 Hook adapter；现网 Nginx 直接反代 `127.0.0.1:13080`，Compose 启用 `BATCH_UPDATE_ENABLED=true`，因此尚不能证明 HTTP/后台 writer 完整关闭与 drain


## 业务、浏览器与监控

- 生产只读健康已观察到；真实认证前端/Chromium、隔离克隆 32 CNY 行为证明、disabled-plan 探针、五接口生产证据均未执行或未完成。
- 开放流量观察窗口未执行；mismatch/missing、unknown、unsupported FX、settlement latency、DB lock wait、error/write load 未形成发布窗口证据。
- 静态资源或健康 200 未被冒充为 API/DB/业务证明。

## 回滚演练

- ready 前旧镜像回滚、ready 后未开放写停服回滚、双写接受流量后禁止 image-only rollback 三阶段均未实际演练；仅保留合同，不宣称通过。

## 当前结论

- 候选推送与镜像构建已完成：`main`=`9ffa6391db5cfc0a20246f6c5a1aeda4c3682d1a`，run=`31810007737`，digest=`sha256:64266b6f36948fa083b12a17c5d19c659398aa0b1f4d61f026bf48d2df7e7b90`
- 用户直接授权已生效，但真实 write-gate / production-probe / clone-probe / observe Hook 未交付；现有已审阅脚本无法进入完整生产状态机
- 当前生产仍运行旧 digest，健康且未修改；目标 digest 未拉取，迁移 schema 未 stage
- Issue #28 与父 #19 保持 OPEN；生产部署、32 CNY 隔离克隆、认证前端、观察窗口和回滚演练均未完成
- 当前尚未发生任何生产写操作


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
## 编排历史交接

- 原续作 Dispatch `ctx_b4eb0587374d` 已失败并撤销 capability；该事实只作历史审计
- 用户随后明确要求直接发布并忽略 Orca，因此当前阻断不再归因于 Orca
- 当前实际阻断是现有已审阅脚本要求的四个真实生产 Hook 均不存在，不能用 stub 或临时方案替代

## 用户决定（2026-08-14）

- 用户明确要求不进行本地 Linux 测试；本地 POSIX `0600` 验收任务已取消，不再作为当前发布阻断。
- 不再轮询 WSL/Linux 环境；严格 `0600` 合同仍由获授权后的目标 Linux 发布流程执行。
- 用户随后直接授权立即部署并要求忽略 Orca；该授权不补齐真实生产 Hook，也不允许伪造状态机证据
- 最终候选 `9ffa6391d` 已推送并成功构建不可变镜像

## 本次收口决定

- 停止继续设计维护模式、迁移 CLI 或第三套临时发布方案；只允许使用现有已审阅发布脚本。
- 现有脚本依赖的 write-gate、生产只读探针、隔离克隆探针和 observe Hook 均未落地，因此不能执行生产状态机，也不记录适配器或发布干跑通过。
- 部署远端已清理为仅 `main`：删除 `prd9-credit-activation-fix`、`prd9-integration`、`prd9-layout-fix`、`prd9-ui-fixes` 后，远端只剩 `refs/heads/main` 指向 `9ffa6391d`。
- 生产仍运行旧 digest `sha256:62a5d95811923be881395265aaeddf5bb9176db55edc936a89722371ffd05976` 且只读健康；目标 digest 未拉取，部署、业务、观察和回滚验收均未完成。
## 本轮本地交付物与证据（2026-08-15）

- 修改文件：`main.go`、`main_task_startup_test.go`、`.scratch/agent-progress/issue-28/server-release.sh`、`.scratch/agent-progress/issue-28/server-release.test.sh`；`git diff --check` 通过。
- 应用生命周期：维护模式在数据库初始化后保持进程存活，等待 `SIGINT`/`SIGTERM`；定向 Go 测试退出码 `0`。该测试覆盖启动开关全部关闭及终止信号等待，不冒充真实容器检查。
- 发布脚本：`bash -n` 退出码 `0`；最新完整本地合同退出码 `0`，阶段输出包含 `stage-schema result=pass write_gate=closed maintenance_mode=true`、`start-closed result=pass maintenance_mode=false`、`probe`、`open-writes`、`observe`、`rollback-suspend` 与 `PASS: full pipeline only`。
- `MAINTENANCE_MODE=true` 已纳入脚本配置白名单与必填校验；stage-schema 覆盖只作用于 `new-api`，恢复路径从原始 Compose 备份完整恢复；迁移 CLI 通过显式环境变量运行。
- 证据边界：完整合同使用单服务 stub，未验证真实 Compose 多文件服务级环境归属；未验证真实 Nginx、PostgreSQL/Redis drain、容器退出/健康、生产 Hook 或远端迁移命令。
- 生产结论：保持失败关闭；没有真实远端 Hook/容器/数据库只读证据，不得执行或宣称生产发布就绪；当前生产旧 digest 未被本轮修改。
## 本轮阻断收口（2026-08-15）

- readiness 测试 stub 已从 `running_env` 判定改为独立状态：maintenance Compose-up 创建 `$STUB_STATE.maintenance_ready`，探针执行 `test -s /tmp/new-api-maintenance-ready` 检查该状态，normal Compose-up 删除该状态。
- `bash -n .scratch/agent-progress/issue-28/server-release.sh && TEST_FILTER=full bash .scratch/agent-progress/issue-28/server-release.test.sh` 退出码为 `0`，阶段覆盖 maintenance readiness、幂等 dry-run/apply/verify、封闭启动、probe、observe 与 rollback-suspend。
- 本地合同通过不构成生产发布证明；真实 write-gate、`production-probe`、`clone-probe`、`observe`、Nginx 切流和 PostgreSQL/Redis drain 仍未交付或验证。
- 按失败关闭规则未执行任何生产远程写操作；生产仍运行旧 immutable digest `sha256:62a5d95811923be881395265aaeddf5bb9176db55edc936a89722371ffd05976`，Issue #28 与父 Issue #19 保持 OPEN。
## 本轮最终阻断证据（2026-08-15）

- 证据范围：仅对 `production-probe-hook.sh`、`clone-probe-hook.sh`、`observe-hook.sh` 进行了目标性编辑后的 `read`/`grep` 复核；未执行脚本、fixture、测试或生产运行时验证。
- 明确未运行：`bash -n`、任何测试、fixture、`git`、formatter。因此不得宣称脚本语法通过、fixture PASS、完整门禁 PASS 或生产发布就绪。
- `write-gate-hook.sh` 与 `write-gate-hook.test.sh` 未修改；write-gate 适配器保持冻结。本地合同或历史记录不替代真实生产 Hook 证据。
- 明确未执行生产副作用：无远程写入、flock、停写、备份、镜像拉取、Compose/迁移修改、重启、业务或前端验证、开放写流量、观察窗口及回滚演练。
- 当前结论：Issue #28 发布阻断继续有效；生产保持失败关闭，Issue #28 与父 Issue #19 保持 OPEN。剩余工作需在解除限制后重新执行并取得可复核证据。
## 本轮远端只读 preflight 阻断证据（2026-08-15）

- 目标主机仅为 `netcup-ows-migrate`。本轮只读读取了远端 Compose、具体 Nginx site 文件、snippets、备份目录和 migration-prep 目录；没有执行远程写入。
- 候选与远端 release 配置不一致：候选 revision=`9ffa6391db5cfc0a20246f6c5a1aeda4c3682d1a` / digest=`sha256:64266b6f36948fa083b12a17c5d19c659398aa0b1f4d61f026bf48d2df7e7b90`；远端 `compose.release.yml` 声明 digest=`sha256:6af45b7c97c1d5c910501baa06514263aaf08a89ec28077dfa08f89a24bb9e7a`。这是安全阻断，不是部署成功或当前容器实际 digest 证据。
- 具体 Nginx site 读取结果：`api.pqapi.shop` 与 `newapi-direct-ip` 反代 `127.0.0.1:13080`，使用 `newapi-origin-allowlist.conf`；未看到 `write-gate` include 或 `NEW_API_WRITE_GATE_*` 标记。`aws-g.pqapi.shop` 与 `ows-router-internal` 指向其他 upstream。递归 grep 因 SSH 目录不支持而失败，未将其当作全树搜索通过。
- `/opt/new-api/backups/new_api_final.dump.sha256` 与 `/opt/new-api/backups/new_api.dump.sha256` 读取失败；没有 sidecar checksum，也没有本轮一致备份的可读/可恢复验证。不得把读取失败记为备份通过。
- `/opt/new-api/migration-prep` 清单未显示 Issue #28 的真实 Hook、write-gate 配置或 approval 文件；本地 `production-probe-hook.sh`、`clone-probe-hook.sh`、`observe-hook.sh` 只做过 read/grep 复核，未运行语法或 fixture 验证，且 `write-gate-hook.sh` 未修改。
- 本轮未执行 install、stop-writes、pull、Compose 修改、迁移、重启、业务/前端验证、open-writes、观察窗口或回滚；生产保持失败关闭，Issue #28 与父 Issue #19 保持 OPEN。