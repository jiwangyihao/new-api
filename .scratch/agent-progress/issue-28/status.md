# Issue #28 发布状态

## 当前阶段

- 阶段：候选集成、推送与 CI 构建
- 用户授权：已明确授权推送 `jiwangyihao/new-api` 远端 `main` 并通过 CI 构建，目标生产实例为 `netcup-ows-migrate`
- Orca 生产写操作授权：仍显示冻结/未授权；在该状态实际更新前，禁止生产 pull/up、停写、备份、迁移或其他远端变更
- 现网外部写流量：未核验（只读预检未证明关闭；不得记录为关闭）
- 当前候选提交：`f1434499b`（Credit 管理员调整重试幂等修复）；待提交发布脚本/证据并合并最新 `deploy/main`
- 下一动作：提交发布合同与脚本，合并最新远端主线，完成受影响门禁并将确切 SHA 推送到 `main`

## Read-back

- 当前 Worker HEAD：`f1434499bf3ab4669b741d9bc6ff12a442f977bb`
- 生产行为基线：`f446a1569c2ced54a3fe438b5c4575659a59241d`
- 候选与 `deploy/main` 的共同祖先：`73c658daa8e7954cb6f229348aac80287253391c`
- Issue #27 验收提交：`e6ec1072104a826a7a572dd55cf9c0422f2b3d8d`
- #27 集成关系：`e6ec10721` 是当前候选祖先；#27 历史三库零 SKIP 证据有效，但不能替代合并后候选的门禁
- 工作树：含待提交的 Issue #28 合同、runbook、状态、证据与发布脚本；Credit UI 修复已单独提交，不能记录为 clean
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

## 最新候选门禁结果

- 默认前端：`bun test` 为 573 pass、0 fail；`bun run typecheck`、`bun run build:check`、`bun run copyright:check`、`bun run i18n:sync` 均通过
- 经典前端：`bun run build` 通过；仅有既有 Browserslist、第三方 `eval` 与大 chunk 警告
- Go 窄门禁：估值迁移/请求/结算相关 `-race` 定向命令通过；外部矩阵的 SQLite 3.50.4 阶段通过
- #27 历史 Gate F：SQLite 3.50.4、MySQL 5.7.44、PostgreSQL 9.6.24 同一矩阵 36 阶段 PASS、`SKIP=0`；当前 shell 未提供 MySQL/PostgreSQL DSN，不能冒充当前候选三库重跑
- 发布脚本：`TEST_FILTER=full bash .scratch/agent-progress/issue-28/server-release.test.sh` 完整状态机合同通过；`server-release.sh` 与测试脚本语法检查通过
- 本地 Linux/WSL 测试：按用户决定取消，不再作为发布阻断；严格 `0600` 权限检查未放宽，由获授权后的目标 Linux 发布流程满足
- `git diff --check` 通过；最终合并 `deploy/main` 后仍需复跑受影响门禁

## 发布结论

- 候选推送与 CI 构建：已获用户授权，可以继续
- CI 产物必须绑定最终推送到 `main` 的确切 SHA；仅接受该 SHA 对应成功 run 产生的 GHCR immutable digest，禁止以 `latest` 或 CI 绿灯替代 digest 绑定
- 生产远程写：Orca 协调器授权状态仍冻结/未授权；状态更新前禁止生产 pull/up、停写、备份、迁移、重启或开放流量
- 获授权后严格按 dry-run→stop-writes→backup→apply/verify→start-closed→probe→open-writes→observe 执行；部署后核对 digest、OCI revision、健康/API 与 migration version/status
- 尚未发生任何生产写操作
