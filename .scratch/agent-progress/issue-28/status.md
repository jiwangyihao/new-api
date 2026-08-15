# Issue #28 发布状态

## 当前阶段

- 阶段：生产发布未完成；候选与不可变镜像已完成，部署远端仅保留 `main`
- 用户授权：已明确要求立即直接部署并忽略 Orca 状态，同时要求部署远端仅保留 `main`
- Orca 状态：仅保留为历史审计，不再作为当前授权阻断；直接用户授权不等于可以伪造缺失的生产 Hook 或跳过发布合同
- 现网外部写流量：未核验（只读预检未证明关闭；不得记录为关闭）
- 最终候选 HEAD：`9ffa6391db5cfc0a20246f6c5a1aeda4c3682d1a`；`deploy/main` 仍指向该提交
- 下一动作：现有已审阅脚本缺真实 write-gate、生产/克隆探针和 observe Hook，无法执行安全状态机；保持生产不变

## Read-back

- 候选与镜像源码 HEAD：`9ffa6391db5cfc0a20246f6c5a1aeda4c3682d1a`；本次仅将脱敏阻断证据另行提交到本地发布分支，不推送、不改变已构建镜像内容
- 生产行为基线：`f446a1569c2ced54a3fe438b5c4575659a59241d`
- 已合并 `deploy/main`：`0a6995369c5f3755508567eaa2db5f363eb1d22f` 是当前候选祖先
- Issue #27 验收提交：`e6ec1072104a826a7a572dd55cf9c0422f2b3d8d`
- #27 集成关系：`e6ec10721` 是当前候选祖先；#27 历史三库零 SKIP 证据有效，但不能替代最终候选 Gate F 重跑
- 工作树：最终候选提交时 clean；本次仅更新脱敏状态与证据，不改变已构建镜像内容
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
- 目标镜像：CI run `31810007737` 成功，revision=`9ffa6391db5cfc0a20246f6c5a1aeda4c3682d1a`，digest=`sha256:64266b6f36948fa083b12a17c5d19c659398aa0b1f4d61f026bf48d2df7e7b90`；目标 digest 尚未拉到服务器
- 生产 PostgreSQL 尚无 `credit_valuation%` 表；符合旧镜像尚未执行 schema stage 的状态
- `/opt/new-api/migration-prep` 与仓库均缺真实 Issue #28 write-gate、production-probe、clone-probe、observe Hook 适配器；现有 `server-release.test.sh` 只提供 stub 合同


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

## 当前生产阻断

- 授权状态：用户已直接授权生产发布；Orca 不再作为当前阻断理由
- 能力阻断：现有 `server-release.sh` 强制依赖真实 write-gate、production-probe、clone-probe、observe Hook，但仓库与服务器均不存在这些适配器；不得用 stub 或临时脚本替代
- 安全阻断：现网 Nginx 没有 write-gate include，应用启用后台任务且 Compose 设置 `BATCH_UPDATE_ENABLED=true`，无法证明 HTTP/后台 writer 完整关闭与 drain
- 验收阻断：最新备份隔离克隆 32 CNY tracer、真实认证前端探针、开放写观察和三阶段回滚均没有可执行实现或证据
- 门禁边界：最终候选只完成显式跳过 external matrix 的 Go 非矩阵全套；只能引用 #27 历史三库 `SKIP=0`，不得宣称最终候选新鲜三库全套

## 发布结论

- 候选推送与 CI 构建已完成：`main`=`9ffa6391db5cfc0a20246f6c5a1aeda4c3682d1a`，run=`31810007737`，immutable digest=`sha256:64266b6f36948fa083b12a17c5d19c659398aa0b1f4d61f026bf48d2df7e7b90`
- 远端分支清理已完成：`deploy` 明确指向 `jiwangyihao/new-api`，默认分支为 `main`；删除四个 `prd9-*` 分支后，`git ls-remote --heads deploy` 只返回 `refs/heads/main`
- 生产远程写：直接用户授权已收到，但现有已审阅发布脚本因真实 Hook 缺失无法执行完整安全状态机；未部署
- 当前生产继续运行旧 digest `sha256:62a5d95811923be881395265aaeddf5bb9176db55edc936a89722371ffd05976`，健康且未被本次操作修改
- Issue #28 与父 #19 必须保持 OPEN；生产部署、业务验证、观察和回滚演练均未完成
- 尚未发生任何生产写操作
## 本次收口决定

- 用户已直接授权立即发布并要求忽略 Orca，但没有授权以 stub、伪造探针或不完整状态机冒充安全发布。
- 当前仓库没有已落地的真实 write-gate、生产只读探针、隔离克隆探针或 observe Hook；不得把状态机与 stub 合同测试当作生产能力。
- 本次只完成部署远端分支清理和脱敏证据更新；不传输脚本、不拉取目标镜像、不停写、不备份、不迁移、不重启、不开放流量。
- Issue #28 与父 #19 保持 OPEN；生产部署、业务验收、观察窗口和回滚演练均未完成。
