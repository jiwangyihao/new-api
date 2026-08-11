# Issue #28 发布证据

## 证据边界

本文件只记录可复核命令、脱敏摘要、校验和与状态；不记录 DSN、令牌、Cookie、私钥、数据库 dump、完整生产日志或可识别用户数据。生产结论必须标注“生产”或“隔离克隆”，静态资源渲染不能替代已认证 API/数据库证据。

## 硬阻断：Issue #27

- 集成 HEAD：`0d85b9f14a8b2170f6c769b64602068105fe6184`
- #27 验收提交：`e6ec1072104a826a7a572dd55cf9c0422f2b3d8d`
- 集成验证：`git merge-base --is-ancestor e6ec1072104a826a7a572dd55cf9c0422f2b3d8d HEAD` 通过
- 已合入 #27 交接证据：SQLite `3.50.4`、MySQL `5.7.44`、PostgreSQL `9.6.24` 同一矩阵 36 阶段 PASS、`SKIP=0`；该结果来自已合入交接记录，不能替代当前候选重跑
- 当前候选命令：`go test ./model -run 'TestCreditValuationExternalMatrix$' -count=1 -v -timeout 60m`（在 Go 全套中实际触发）；退出码 `1`，MySQL/PostgreSQL 因 `TEST_MYSQL_DSN` / `TEST_POSTGRES_DSN` 缺失而 Fatal，Gate F 未完成
- 冻结业务结果（仅 #27 已合入证据）：40 CNY / 1,000 Credit，消费 200，剩余 800，`exact_cost_micros=32000000` CNY，五分析接口一致，`active_paid_subscription_count=1`，estimated=0，unknown=0

## 强制 Read-back 与主机身份

- HEAD：`0d85b9f14a8b2170f6c769b64602068105fe6184`
- merge-base：`f446a1569c2ced54a3fe438b5c4575659a59241d`
- 工作树：clean
- SSH 别名：`netcup-ows-migrate`
- 远端原始只读输出：`hostname=netcup-ows-migrate`、`sys_vendor=netcup`、`product_name=KVM Server`
- 目标裁决：协调器已明确接受 Netcup 为现行生产目标；旧 RackNerd/AutoDLChen 目标禁止访问
- 冲突审计：提交 `737a6b02c` 将既有 RackNerd/AutoDLChen 访问约定更正为 `netcup-ows-migrate`；该更正与远端身份输出一致。发布前后不得省略这一冲突历史。

## 本地门禁（最终结果）

- Go 全套：`go test ./... -count=1`，退出码 `1`；原始日志 `artifact://33`。root setup 因 `web/classic/dist` 缺失失败；model 包最终 FAIL，且外部矩阵 DSN 缺失。
- Go 定向 race：`go test ./model -run 'Test(CreditValuationMath|CreditValuationDeltaCoalescer|CreditValuationMigration|CreditValuationRequest|SubscriptionDeltaCoalescer)' -race -count=1 -timeout 30m`，退出码 `0`；仅为窄门禁。
- 默认前端：cwd=`web/default` 的 `bun test` 退出码 `1`，`0 pass / 105 fail`，共同错误 `Cannot find package 'happy-dom'`，原始日志 `artifact://29`。
- 默认前端 typecheck：cwd=`web/default` 的 `bun run typecheck` 退出码 `1`，`tsc` 不存在。
- 经典依赖：cwd=`web/classic` 的 `bun install --frozen-lockfile` 退出码 `1`，`lockfile had changes, but lockfile is frozen`。默认冻结安装没有完成结果，挂起任务取消；`--no-save` 未获得执行结果。
- 默认 build/build:check、六语言 i18n、copyright、生产 build 未执行；未生成或提交 dist/lockfile。
- `git diff --check` 退出码 `0`，但工作树含本任务未提交 `.scratch/agent-progress/issue-28/`，因此最终树不 clean。

## 生产只读预检

- 目标：仅 SSH `netcup-ows-migrate`；hostname/vendor/product 原始只读输出为 `netcup-ows-migrate` / `netcup` / `KVM Server`。历史 RackNerd/AutoDLChen 文字冲突已由 `737a6b02c` 更正并保留审计。
- 当前生产：`new-api` digest=`sha256:45f0ae2bb003a08ffa2beffdea60506b89251db4b24931bf344087b6a7395a09`，revision=`d13efc82f796ca5f78f826f0f96e89d3812a48ae`，应用、PostgreSQL、Redis healthy；13080/13081 `/api/status` 均 `success=true`。
- PostgreSQL 只读：版本 `18.4`，`new_api|public`；现有业务表可读，新增估值表 `credit_valuation_migrations`、`credit_valuation_states`、`timed_subscription_valuation_grants` 均 `absent`，关键附加列未出现。该 `table_absent` 是预迁移基线，不是 ready/成功证据。
- 现网外部写流量：未核验；协调器生产写操作授权：冻结/未授权。不得记录为“写已关闭”。
- 未执行：远程脚本、flock、stop-writes、备份、镜像 pull、compose 修改、apply、verify、重启、生产写探针、open-writes。

## 业务、浏览器与监控

- 生产只读健康已观察到；真实认证前端/Chromium、隔离克隆 32 CNY 行为证明、disabled-plan 探针、五接口生产证据均未执行或未完成。
- 开放流量观察窗口未执行；mismatch/missing、unknown、unsupported FX、settlement latency、DB lock wait、error/write load 未形成发布窗口证据。
- 静态资源或健康 200 未被冒充为 API/DB/业务证明。

## 回滚演练

- ready 前旧镜像回滚、ready 后未开放写停服回滚、双写接受流量后禁止 image-only rollback 三阶段均未实际演练；仅保留合同，不宣称通过。

## 最终结论

- 状态：`blocked/failed`。
- 阻断：classic dist 缺失、当前候选 Go 全套/model 失败、当前候选外部 DSN 不可达、前端依赖/冻结 lockfile 不一致、前端全套/typecheck/build/i18n 未完成。
- 未发生生产写操作：未停写、未备份、未拉取镜像、未迁移、未重启、未执行写探针、未开放流量。
- 本次不得发送 succeeded、不得构建/发布、不得关闭 Issue #28 或父 #19。
