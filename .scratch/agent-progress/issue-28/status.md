# Issue #28 发布状态

## 当前阶段

- 阶段：发布阻断（本地门禁与外部依赖未闭环）
- 协调器生产写操作授权：冻结/未授权
- 现网外部写流量：未核验（只读预检未证明关闭；不得记录为关闭）
- 最近安全提交：`0d85b9f14a8b2170f6c769b64602068105fe6184`
- 下一动作：仅保存阻断证据并发送 `worker_done --outcome failed`；不得继续依赖安装、代码/锁文件修复或生产动作

## Read-back

- 实际 Worker HEAD：`0d85b9f14a8b2170f6c769b64602068105fe6184`
- 生产行为基线：`f446a1569c2ced54a3fe438b5c4575659a59241d`
- `merge-base(HEAD, production baseline)`：`f446a1569c2ced54a3fe438b5c4575659a59241d`
- Issue #27 验收提交：`e6ec1072104a826a7a572dd55cf9c0422f2b3d8d`
- #27 集成关系：`e6ec10721` 是当前 HEAD 祖先；当前 HEAD 是集成提交 `0d85b9f14`
- 读取时工作树：clean（`git status --porcelain=v1 --branch` 仅显示分支行）
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

## 服务器安全状态

- 只读实测：当前应用 digest=`ghcr.io/jiwangyihao/new-api@sha256:45f0ae2bb003a08ffa2beffdea60506b89251db4b24931bf344087b6a7395a09`，image ID 同 digest，revision=`d13efc82f796ca5f78f826f0f96e89d3812a48ae`，`new-api`/PostgreSQL/Redis 均 healthy；API 13080/13081 返回 `success=true`，现网版本 `deploy-20260810-d13efc8`
- 只读实测 PostgreSQL：`18.4`，数据库 `new_api`、schema `public`；既有业务表存在，但 `credit_valuation_migrations`、`credit_valuation_states`、`timed_subscription_valuation_grants` 均 absent，关键附加列尚 absent；这是预迁移基线，不是 ready 证据
- 主机只读身份：SSH alias=`netcup-ows-migrate`，hostname=`netcup-ows-migrate`，vendor=`netcup`，product=`KVM Server`
- 未执行：远程脚本创建/传输、flock、停写、备份、`docker compose pull`、修改 compose、apply、verify、重启、写探针、开放流量

## 故障恢复规则

- 任一状态不明、checksum 漂移、blocker 非零、apply/verify/启动/探针失败：保持写关闭，不自动放流、不启动旧镜像、不删除证据
- ready 前：允许旧镜像回滚
- ready 后且外部写未开放：先停服，保留附加 schema/marker 后回滚
- 强制双写接受流量后：禁止 image-only rollback；必须 stop writes → 原子 `suspend --reason` → 新一致备份 → 新 migration version 重建/verify → 重新受控开放

## 最终阻断证据

- Go 全套命令：`go test ./... -count=1`；退出码 `1`；原始日志 `artifact://33`。根因证据包括 `main.go:77:12: pattern web/classic/dist: no matching files found`（root setup failure）；`TestCreditValuationExternalMatrix/mysql` 与 `/postgres` 均以 `TEST_MYSQL_DSN is required` / `TEST_POSTGRES_DSN is required` 失败，明确 Gate F 禁止 SKIP；`github.com/QuantumNous/new-api/model` 最终 `FAIL`。未修复、未重跑为通过。
- Go 定向 race 命令：`go test ./model -run 'Test(CreditValuationMath|CreditValuationDeltaCoalescer|CreditValuationMigration|CreditValuationRequest|SubscriptionDeltaCoalescer)' -race -count=1 -timeout 30m`；退出码 `0`。这只是窄门禁，不能替代 Go 全套或三数据库零 SKIP。
- 默认前端全套命令（cwd=`web/default`）：`bun test`；退出码 `1`；`0 pass / 105 fail`；原始日志 `artifact://29`，共同错误为无法从 `src/test/setup.ts` 找到 `happy-dom`，不是测试通过。
- 默认前端 typecheck（cwd=`web/default`）：`bun run typecheck`；退出码 `1`；`tsc -b` 后 `bun: command not found: tsc`。
- 经典前端依赖命令（cwd=`web/classic`）：`bun install --frozen-lockfile`；退出码 `1`；日志为 `lockfile had changes, but lockfile is frozen`。默认前端冻结安装没有获得完成结果，后续挂起任务已取消；不得将其当作成功或失败替代证据。`bun install --no-save` 未获得执行结果，不计门禁。
- 默认前端生产 build、`build:check`、六语言 `i18n:sync`、版权检查未执行；classic dist 未生成；不得创建/提交 dist 或 lockfile 掩盖环境阻断。
- 本地镜像工具探测：`docker version --format '{{.Client.Version}}'`；退出码 `127`，`docker: command not found`；未构建镜像、未取得候选 digest。
- #27 继承证据仍是已合入提交中的 SQLite 3.50.4/MySQL 5.7.44/PostgreSQL 9.6.24、36 阶段 PASS、`SKIP=0`；但当前候选重跑因外部 DSN 缺失退出 `1`，不能把当前全套结果宣称三库通过。
- 结论：本次发布 `blocked/failed`。未执行镜像、备份、迁移、停写、重启、生产写探针或切流量；不得关闭 #20–#28 或父 #19。