# Issue #28 发布状态

## 当前阶段

- 阶段：最终候选提交与远端 `main` 推送
- 用户授权：已明确授权推送 `jiwangyihao/new-api` 远端 `main` 并通过 CI 构建，目标生产实例为 `netcup-ows-migrate`
- Orca 生产写操作授权：仍显示冻结/未授权；在该状态实际更新前，禁止生产 pull/up、停写、备份、迁移或其他远端变更
- 现网外部写流量：未核验（只读预检未证明关闭；不得记录为关闭）
- 合并候选 HEAD：`989d91d1e961fbeef27880fb57a3042f97588865`；已包含 `deploy/main` 的 `0a6995369c5f3755508567eaa2db5f363eb1d22f`
- 下一动作：提交合并后门禁证据与经典前端锁文件，记录最终 SHA，推送到远端 `main`

## Read-back

- 当前 Worker HEAD：`989d91d1e961fbeef27880fb57a3042f97588865`（合并后门禁证据与锁文件尚待提交）
- 生产行为基线：`f446a1569c2ced54a3fe438b5c4575659a59241d`
- 已合并 `deploy/main`：`0a6995369c5f3755508567eaa2db5f363eb1d22f` 是当前候选祖先
- Issue #27 验收提交：`e6ec1072104a826a7a572dd55cf9c0422f2b3d8d`
- #27 集成关系：`e6ec10721` 是当前候选祖先；#27 历史三库零 SKIP 证据有效，但不能替代最终候选 Gate F 重跑
- 工作树：仅含待提交的合并后证据与 `web/classic/bun.lock` 一致性修复；提交后必须 clean
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

- 生产应用：digest=`ghcr.io/jiwangyihao/new-api@sha256:62a5d95811923be881395265aaeddf5bb9176db55edc936a89722371ffd05976`，revision=`0a6995369c5f3755508567eaa2db5f363eb1d22f`，容器 `running healthy`，`RestartCount=0`
- Compose：`/opt/new-api/compose.yml` + `/opt/new-api/compose.release.yml`；release override 当前固定上述 immutable digest
- 主机只读身份：SSH alias=`netcup-ows-migrate`，hostname=`netcup-ows-migrate`，vendor=`netcup`，product=`KVM Server`
- `/api/status`：`127.0.0.1:13080` 返回 `success=true`，版本 `deploy-20260813-0a69953`
- 未执行：远程脚本创建/传输、flock、停写、备份、镜像 pull、修改 compose、apply、verify、重启、写探针、开放流量

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

## 发布结论

- 候选推送与 CI 构建：已获用户授权，可以继续
- CI 产物必须绑定最终推送到 `main` 的确切 SHA；仅接受该 SHA 对应成功 run 产生的 GHCR immutable digest，禁止以 `latest` 或 CI 绿灯替代 digest 绑定
- 生产远程写：Orca 协调器授权状态仍冻结/未授权；状态更新前禁止生产 pull/up、停写、备份、迁移、重启或开放流量
- 获授权后严格按 dry-run→stop-writes→backup→apply/verify→start-closed→probe→open-writes→observe 执行；部署后核对 digest、OCI revision、健康/API 与 migration version/status
- 尚未发生任何生产写操作
