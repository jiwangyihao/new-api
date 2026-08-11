# Issue #28 可恢复发布 Runbook

> 本文件只记录脱敏命令形状、阶段、退出码、摘要和恢复点；禁止写入凭据、DSN、Cookie、dump 内容或用户数据。每个不可逆动作前后更新 `status.md` 与 `evidence.md`。

## 0. Read-back / preflight（只读）

- 本地实际：HEAD=`0d85b9f14a8b2170f6c769b64602068105fe6184`，merge-base=`f446a1569c2ced54a3fe438b5c4575659a59241d`；工作树含未提交 `.scratch/agent-progress/issue-28/`。
- #27：已合入提交 `e6ec10721` 的历史证据为三库 36 阶段 PASS、`SKIP=0`；当前候选重跑因 MySQL/PostgreSQL DSN 缺失退出 `1`，不能宣称当前候选三库通过。
- 目标：仅 `ssh netcup-ows-migrate`；只读身份 hostname/vendor/product=`netcup-ows-migrate`/`netcup`/`KVM Server`。
- 生产只读基线：应用旧 digest=`sha256:45f0ae2bb003a08ffa2beffdea60506b89251db4b24931bf344087b6a7395a09`、revision=`d13efc82f796ca5f78f826f0f96e89d3812a48ae`，依赖和应用 healthy，PostgreSQL `18.4`；估值新增表均 absent，属于预迁移基线。
- 执行状态：协调器生产写操作授权冻结/未授权；现网外部写流量未核验；未执行任何远程写。
## 0.1 本地门禁失败记录

- `go test ./... -count=1`：退出码 `1`；`main.go:77:12` 缺少 `web/classic/dist`，`TestCreditValuationExternalMatrix/mysql` 与 `/postgres` 因缺 DSN Fatal，model 最终 FAIL；日志 `artifact://33`。
- cwd=`web/default` `bun test`：退出码 `1`，`0 pass / 105 fail`，缺 `happy-dom`；日志 `artifact://29`。
- cwd=`web/default` `bun run typecheck`：退出码 `1`，`tsc` 不存在。
- cwd=`web/classic` `bun install --frozen-lockfile`：退出码 `1`，lockfile changes；默认冻结安装无完成结果，挂起任务已取消；`bun install --no-save` 未获得执行结果。
- Go 窄 race 退出码 `0`，但不能替代失败的全套门禁；build、build:check、i18n、copyright 未执行。

## 1. 本地门禁

- `go test ./... -count=1`
- `go test ./model -run '<代表性门禁>' -race -count=1`
- `cd web/default && bun test`
- `cd web/default && bun run typecheck`
- `cd web/default && bun run i18n:sync`
- `cd web/default && bun run build:check`
- `cd web/default && bun run copyright:check`
- `git diff --check`
- 生产镜像 smoke/build：记录源码 SHA、workflow run、image ID、immutable digest

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
## 6. 最终阻断收尾

- 状态：`blocked/failed`。
- 未执行：镜像构建/拉取、服务器脚本、flock、stop-writes、PostgreSQL 备份、dry-run/apply/verify、marker ready、重启、生产写探针、Chromium 认证 API 验证、open-writes、观察窗口、回滚演练。
- 不能发送 succeeded 或宣称发布完成；恢复入口是先取得可用的 classic dist/前端锁定依赖和真实 MySQL 5.7.44/PostgreSQL 9.6.24 DSN，再从当前精确候选重跑全套门禁。
