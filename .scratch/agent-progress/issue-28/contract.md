# Issue #28 发布合同

## 目标与边界

从已集成 `0d85b9f14a8b2170f6c769b64602068105fe6184` 发布 Credit 运营估值版本；生产行为基线为 `f446a1569c2ced54a3fe438b5c4575659a59241d`。#27 的历史迁移算法、verify、marker 状态机和三库矩阵已验收，#28 只负责本地门禁、不可变镜像、生产停写切换、探针、观察和回滚。

## 部署目标

- SSH：仅 `netcup-ows-migrate`
- 工作目录：`/opt/new-api`
- Compose：`compose.yml` + `compose.release.yml`，按需要叠加 `compose.primary.yml`、`compose.replica.yml`、`compose.network.yml`
- 服务：`new-api`；依赖：`new-api-postgres`、`new-api-redis`
- 监听：`127.0.0.1:13080`、`127.0.0.1:13081`
- 镜像：GHCR immutable digest；禁止 `latest`、deploy tag 或服务器本机构建
- 目标身份：远端 hostname `netcup-ows-migrate`、vendor `netcup`、product `KVM Server`；旧 RackNerd/AutoDLChen 文字仅作冲突审计，不是目标
## 当前执行状态（最终）

- 发布状态：`blocked/failed`；本文件中的状态机是目标合同，不表示任何阶段已执行或通过。
- 协调器生产写操作授权：冻结/未授权。
- 现网外部写流量：未核验；未执行 stop-writes，不能记录为关闭。
- 未发生远程写：未创建/传输脚本、未获取 flock、未备份、未 pull 镜像、未修改 compose、未 apply/verify、未重启、未执行生产写探针、未开放流量。
- 生产只读基线 marker 表 `credit_valuation_migrations` absent，属于预迁移状态，不是 ready 证据。


## 状态机

1. `preflight`：本地 HEAD/merge-base/status、#27 零 SKIP、目标主机身份、当前 release/digest/health/marker
2. `read-only-dry-run`：同一目标 digest 连续两次维护 dry-run；完整业务 JSON/checksum 相同
3. `stop-writes`：反代/应用关闭外部写流量，停止后台任务；确认 HTTP 写、非终态预扣、异步结算和旧 writer 会话清零
4. `backup`：PostgreSQL 一致备份；绝对路径、UTC、大小、SHA-256、可读/可恢复检查
5. `apply`：同一 digest、冻结 version、稳定批次、marker CAS
6. `verify`：同一 digest/version/checksum 原子验证；失败保持写关闭
7. `start-closed`：所有实例同 digest 启动，外部写仍关闭；检查 ready、health、版本和 fail-closed
8. `probe`：生产只读健康/业务探针；无授权账号时隔离克隆完成 32 CNY 行为链路
9. `open-writes`：仅所有前置门禁通过后原子恢复外部写，记录 UTC、digest、marker
10. `observe`：记录健康、错误率、state missing/mismatch、unknown、unsupported FX、结算延迟、coalescer、DB 锁/连接/写负载、资源

## 退出码

- `0`：阶段成功并满足合同
- `1`：运行时/数据库/迁移/健康/探针失败；保持写关闭并保留证据
- `2`：参数或状态合同错误；不执行不可逆动作
- `3`：安全阻断（身份、digest、marker、blocker、checksum、实例不一致）

## 不变量

- 同一源码提交构建出的所有应用实例使用同一 immutable digest
- dry-run/verify 只读；两次 dry-run 业务 JSON 与 checksum 字节一致
- apply/verify 使用同一 migration version、digest 和冻结输入 checksum
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

- `go test ./... -count=1` 退出 `1`：classic dist 缺失、model 外部矩阵 DSN 缺失并最终 FAIL。
- cwd=`web/default` 的 `bun test` 退出 `1`（0/105，缺 `happy-dom`）；`bun run typecheck` 退出 `1`（`tsc` 不存在）。
- cwd=`web/classic` 的 `bun install --frozen-lockfile` 退出 `1`（lockfile changes）；默认冻结安装未获得完成结果，`--no-save` 未获得执行结果。
- 因此不得进入镜像、备份、迁移、停写或流量阶段；不得将未执行的安装/修复当作门禁证据。
