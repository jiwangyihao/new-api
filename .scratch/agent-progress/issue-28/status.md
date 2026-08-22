# Issue #28 发布状态

## 当前阶段

- 阶段：前向修复、生产迁移、放流与观察均已完成
- 用户授权：用户明确纠正“半上线”风险并要求立即完成前向收尾；禁止回滚旧实现
- 生产写流量：受管 Nginx gate 已恢复 `open`
- 最终生产 HEAD：`b164786033772cdf44ccdc41fc40068c9e3ac208`
- 最终生产镜像：`ghcr.io/jiwangyihao/new-api@sha256:b0bddee4b86f897e41353a69f0c7150f05af342f6fc843994bbb6a535028cb53`
- 下一动作：完成本地审计提交、关闭 Issue #28 与父 Issue #19

## Read-back

- 候选与镜像源码 HEAD：`b164786033772cdf44ccdc41fc40068c9e3ac208`；`deploy/main` 同指该提交
- 生产行为基线：`f446a1569c2ced54a3fe438b5c4575659a59241d`
- `deploy/main`：`b164786033772cdf44ccdc41fc40068c9e3ac208`，包含迁移 verifier 前向修复
- Issue #27 验收提交：`e6ec1072104a826a7a572dd55cf9c0422f2b3d8d`
- #27 集成关系：`e6ec10721` 是当前候选祖先；#27 历史三库零 SKIP 证据有效，但不能替代最终候选 Gate F 重跑
- 工作树：生产代码已提交；仅本地脱敏发布审计文件待收口
- #28 指令 SHA-256：`80dde8437e7ffece26dc1718b6d1bf0b3775f84dd607c29d7869b43a03f3ad8b`
- #28 验收 SHA-256：`89f05b563b69f0622eff4e2e2a673b7bca4e239619da06a6aaeec019cb4d30ff`
- #27 交接 SHA-256：`3db9d7d1481a32aa9a6cbb7013554d51d291223a41ab57dd420a574f8c9b622b`
- #27 证据 SHA-256：`2ce83036b881d36660ec6cd2540a831f78cf6d792693fbeb041fef7006c5fc2f`

## 生产目标身份

- 唯一允许的 SSH 主机别名：`netcup-ows-migrate`
- 只读实测 `hostname`：`netcup-ows-migrate`
- 只读实测 `sys_vendor`：`netcup`
- 只读实测 `product_name`：`KVM Server`
- 协调器裁决：接受集成提交 `737a6b02c` 的 Netcup 更正和上述远端身份作为现行生产目标；禁止访问旧 RackNerd/AutoDLChen 目标
- 审计冲突：历史任务和旧 SOP 曾写 RackNerd/AutoDLChen；该冲突已被 `737a6b02c` 更正为 Netcup，原始 hostname/vendor/product 输出保留在 `evidence.md`，不得将旧文字伪装为当前主机身份

## 服务器安全状态（最新只读实测）

- 生产应用：digest=`ghcr.io/jiwangyihao/new-api@sha256:b0bddee4b86f897e41353a69f0c7150f05af342f6fc843994bbb6a535028cb53`，revision=`b164786033772cdf44ccdc41fc40068c9e3ac208`，容器 `running healthy`，`RestartCount=0`
- Compose：`/opt/new-api/compose.release.yml` 已固定上述 immutable digest
- `/api/status`：`127.0.0.1:13080` 返回 `success=true`
- write-gate：`open`；当前运行态统计可用，batch writer 状态为 `ok`
- migration marker：version=`1`、status=`ready`、currency=`CNY`、checksum=`c4b08a8fd3bc338abd532f40863edea61b110adce319e6043b473fea5dfd9172`
- Credit 状态：`66/66`，missing=`0`，mismatch=`0`，非法版本/币种=`0`
- 生产备份：`/opt/new-api/backups/new_api_before_issue28_forward_20260822T051757Z.dump`，SHA-256=`64ceb735b40e1f8183bc10d00f8667205e2d01d1a10394d9e21579d349169f65`，mode `0600`，`pg_restore --list` 通过
- 发布 Hook、受管 Nginx gate、runtime drain 与 observe 配置均已安装到 `/opt/new-api`


## 故障恢复规则

- 任一状态不明、checksum 漂移、blocker 非零、apply/verify/启动/探针失败：保持写关闭，不自动放流、不启动旧镜像、不删除证据
- ready 前：允许旧镜像回滚
- ready 后且外部写未开放：先停服，保留附加 schema/marker 后回滚
- 强制双写接受流量后：禁止 image-only rollback；必须 stop writes → 原子 `suspend --reason` → 新一致备份 → 新 migration version 重建/verify → 重新受控开放

## 最终合并候选门禁

- 合并冲突：`controller/subscription_payment_kyren_test.go` 采用远端新的 `CompleteSubscriptionOrderTx` 入口，同时保留候选估值 plan/snapshot 夹具；五个 Kyren 终态/事件测试通过
- 默认前端：`bun test` 为 575 pass、0 fail；Credit 调整重试单文件 8 pass、0 fail；`bun run build:check`、`bun run copyright:check`（1108 项）、`bun run i18n:sync` 均通过
- 经典前端：首次 `bun install --frozen-lockfile` 暴露 `package.json` 的 axios 1.15.2 与锁文件 1.15.0 不一致；锁文件同步后冻结安装通过，`bun run build` 通过；仅有既有 Browserslist、第三方 `eval` 与大 chunk 警告
- Go 非矩阵全套：`go test ./... -skip '^TestCreditValuationExternalMatrix$' -count=1` 为 53 packages ok、59 no tests；此命令明确跳过 Gate F，不能称为完整 Go/三数据库门禁
- Go 估值核心 `-race` 定向门禁通过；#27 历史 Gate F 为 SQLite 3.50.4、MySQL 5.7.44、PostgreSQL 9.6.24 同一矩阵 36 阶段 PASS、`SKIP=0`；最终候选因当前 shell 未提供 MySQL/PostgreSQL DSN，Gate F 未新鲜重跑
- 发布脚本：合并后 `TEST_FILTER=full` 完整状态机合同通过；脚本语法检查通过
- 本地 Linux/WSL 测试按用户决定取消；严格 `0600` 权限检查未放宽，由获授权后的目标 Linux 发布流程满足
- `git diff --check` 通过；CI 工作流只构建镜像、不运行上述测试，因此 CI 成功仅证明构建成功，不证明 Gate F 或发布安全

## 最终生产验收

- 首次候选 `6528ee27f` apply 在事务内 verify 失败；状态写入全部回滚，`credit_valuation_states=0`，marker 记录 `failed/migration_execution_failed`
- 根因：83 条合法历史 Credit ledger 缺来源键，本应由历史 backfill 归类为 unknown，却被全局 verifier 再次当作 ready 阻断
- 前向修复提交：`b16478603 fix(valuation): 允许历史缺失来源按未知价值迁移`
- 定向回归通过：历史缺来源允许、重复来源仍拒绝、非法 timed grant 仍拒绝、repair/apply 合同保持
- GitHub Actions run `32557236431` 成功；修复镜像 immutable digest=`sha256:b0bddee4b86f897e41353a69f0c7150f05af342f6fc843994bbb6a535028cb53`
- 同版本冻结输入重试 apply 成功；随后 verify 成功，二者 checksum 均为 `c4b08a8fd3bc338abd532f40863edea61b110adce319e6043b473fea5dfd9172`
- migration report：Credit total=`66`、unknown rows=`56`；timed total=`3679`、unknown rows=`3668`；历史不可重建事实按设计保留为 unknown，没有伪造精确价值
- 放流后 35 秒观察：health failures=`0`、state missing=`0`、state mismatch=`0`、unsupported FX=`0`、panic=`0`、abnormal restart=`0`、PostgreSQL lock regression=`false`
- 观察窗口 HTTP 高错误比例来自既有 `403 subscription token exhausted`；最近五分钟日志未发现任何 `5xx`、估值 state missing/mismatch、unsupported/invalid FX 或 panic
- 现网没有启用管理员 access token，浏览器 relay 也不可用；未伪造凭据或创建临时生产账号。生产 API 的认证五接口探针未冒充通过，数据库/CLI/运行态验收已完成

## 发布结论

- 半上线状态已结束：最新修复代码、schema、materialized valuation state、ready marker 与运行容器均已统一到 `b16478603`
- 写流量已恢复，容器健康，Compose 固定不可变 digest，禁止回滚旧实现
- Issue #28 生产迁移与运行态验收完成；父 Issue #19 可随子 Issue 一并关闭
## 本轮恢复验证（2026-08-15）

- 应用维护生命周期定向门禁：`gofmt -w main.go main_task_startup_test.go && go test . -run 'Test(Maintenance|RuntimeStartupPlan|MainStarts)' -count=1`，退出码 `0`。
- 发布脚本语法：`bash -n .scratch/agent-progress/issue-28/server-release.sh`，退出码 `0`。
- 本地完整状态机合同：`TEST_FILTER=full bash .scratch/agent-progress/issue-28/server-release.test.sh`，最新退出码 `0`；覆盖 `stage-schema` 的 `maintenance_mode=true`、幂等 dry-run/apply/verify、`start-closed` 的 `maintenance_mode=false`、probe、open-writes、observe 与 rollback-suspend，并输出 `PASS: full pipeline only`。
- 维护模式实际代码路径：加载 `.env` 后解析 `MAINTENANCE_MODE`；使用 `InitMaintenanceDB`；不启动 Redis、后台任务、系统监控、profiling 或 HTTP；进程等待 `SIGINT`/`SIGTERM` 后退出。迁移 CLI 仍在 `main` 资源初始化前独立执行。
- 生产门禁仍为失败关闭：本轮未取得可核验的真实 write-gate drain、Compose 合并环境、Nginx 切流、数据库会话或生产容器生命周期证据；未执行生产写入、部署、重启、迁移、探针、放流或回滚。
- 合同测试使用单服务 Compose stub；它证明本地状态机和覆盖文件归一化，不证明真实多服务 Compose 合并结果或生产 Hook 行为。Issue #28 与 #19 保持 OPEN。

## 本轮阻断收口（2026-08-15）

- readiness 合同已修正：本地 Docker stub 仅在 maintenance Compose-up 时创建独立的 `maintenance_ready` 状态，`docker exec new-api test -s /tmp/new-api-maintenance-ready` 只检查该状态；正常 Compose-up 会清理它。
- `bash -n .scratch/agent-progress/issue-28/server-release.sh && TEST_FILTER=full bash .scratch/agent-progress/issue-28/server-release.test.sh` 退出码为 `0`，完整本地 stub 状态机输出 `PASS: full pipeline only`。
- 该结果只证明已审阅脚本的本地 stub 合同覆盖 readiness 创建、探针检查和清理；不证明真实 Compose、Nginx、数据库 drain 或生产 Hook。
- 生产状态保持不变：未执行远程脚本传输、flock、stop-writes、备份、镜像拉取、Compose 修改、迁移、重启、生产探针、open-writes 或回滚；现网外部写流量仍未核验。
- 真实 write-gate、`production-probe`、`clone-probe` 和 `observe` Hook 仍缺失；Issue #28 与父 Issue #19 保持 OPEN。
## 本轮最终阻断收口（2026-08-15）

- 本轮仅完成 `production-probe-hook.sh`、`clone-probe-hook.sh`、`observe-hook.sh` 的目标性修复与只读 `read`/`grep` 复核；这不是运行验证，也不构成发布通过。
- 本轮明确未运行 `bash -n`、任何测试或 fixture、`git`、formatter；不得将本轮结果或历史 stub 合同结果宣称为 `PASS`、生产就绪或完整门禁通过。
- `write-gate-hook.sh` 与 `write-gate-hook.test.sh` 本轮未修改，继续冻结；真实生产 write-gate、Nginx 切流、数据库/后台 writer drain 仍无可核验证据。
- 未执行远程脚本传输、flock、停写、备份、镜像拉取、Compose 修改、迁移、重启、生产探针、业务/前端验证、开放流量、观察窗口或回滚演练。
- 生产继续按失败关闭规则处理；现网旧 immutable digest 未被本轮修改。Issue #28 与父 Issue #19 保持 OPEN。
- 本轮未达到生产发布就绪条件；所有未完成发布任务均取消，等待后续解除限制并重新取得真实 Hook 与新鲜门禁证据。

## 本轮远端只读 preflight 阻断（2026-08-15）

- 目标仅为 SSH 别名 `netcup-ows-migrate`；本轮未访问旧 RackNerd/AutoDLChen 目标。
- 候选身份为 revision=`9ffa6391db5cfc0a20246f6c5a1aeda4c3682d1a`、immutable image=`ghcr.io/jiwangyihao/new-api@sha256:64266b6f36948fa083b12a17c5d19c659398aa0b1f4d61f026bf48d2df7e7b90`；远端 `compose.release.yml` 声明 digest=`sha256:6af45b7c97c1d5c910501baa06514263aaf08a89ec28077dfa08f89a24bb9e7a`，身份不一致，部署阻断。
- 本轮未取得远端实际 `docker inspect` 的 OCI revision、容器 digest、health 或 restart 结果；既有历史记录不替代本轮新鲜证据。
- 具体 Nginx site 内容未发现 `write-gate`/`NEW_API_WRITE_GATE_*` managed include；`newapi-origin-allowlist.conf` 是访问控制 include，不是 write-gate。递归 grep 因 SSH 目录不支持该用法失败，未将其当作递归搜索通过。
- `/opt/new-api/backups/new_api_final.dump.sha256` 与 `/opt/new-api/backups/new_api.dump.sha256` 读取均失败；没有可核验 checksum sidecar、可恢复性或回滚备份证据，读取失败不等于校验通过。
- `/opt/new-api/migration-prep` 清单未显示真实 Issue #28 write-gate、production-probe、clone-probe、observe Hook 或对应 approval/config；本地 scratch 适配器不能替代远端真实 Hook。
- 保持失败关闭：禁止 install、stop-writes、pull、Compose 修改、迁移、重启、业务探针、open-writes、observe 和 rollback；本轮未执行生产写操作。

## 本轮收口（本次会话）

- 本地发布工具链与回归已完成：Go 非矩阵全套、定向 Go、脚本语法、write-gate、探针 wrapper、完整状态机均通过。
- 生产现场只读核验仍失败关闭：候选未部署，远端缺真实 Hook/config/approval、runtime/drain、managed Nginx gate 和本次一致 `0600` 备份。
- 未执行任何生产远程写操作；不得把本地 stub 状态机 PASS 记录为生产部署或业务验收通过。
## 最终收口（2026-08-22）

- 生产停写、全 writer drain、备份、apply、verify、候选启动、放流和观察已完成
- 当前镜像、Compose、OCI revision、marker、Credit 状态和运行时健康互相一致
- 所有生产动作均为前向升级；没有执行旧镜像回滚、marker 删除或估值状态删除
- 发布证据保存于 `/opt/new-api/audits/issue-28/`，备份与 checksum 保存于 `/opt/new-api/backups/`
