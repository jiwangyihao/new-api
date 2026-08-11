# Issue #27 验收证据

## 2026-08-11 timed historical safe point

- 状态：定向回归 PASS；尚未据此宣称 Gate D 或任何 Gate B–H 完成。
- 命令：`gofmt -w model/timed_subscription_valuation_backfill_test.go model/timed_subscription_valuation_backfill.go && go test ./model -run 'TestTimedHistoricalBackfill(LegacyAdminSubscriptionWithoutAuthoritativeWindowRemainsUnknown|RetainsExistingForwardAdminGrantWithoutLegacyWindow|RejectsEntireSubscriptionWhenRenewalWindowsOverlap)$' -count=1`
- 退出码：`0`
- 结果：`ok github.com/QuantumNous/new-api/model 6.418s`
- 覆盖：无权威管理员历史窗口时稳定 unknown；完整既有 exact forward admin grant 保留且 `rows_skipped_existing=1`；同一 subscription 的重叠 renewal 窗口整份 fail-closed、零部分写入。
- 修复文件：`model/timed_subscription_valuation_backfill.go`、`model/timed_subscription_valuation_backfill_test.go`。
- 当时尚未完成：timed historical 全部窄测结果仍待后台命令返回；Gate B–H 尚待逐 Gate 实测；Gate F 的 MySQL 5.7.44/PostgreSQL 9.6.24 新鲜零 SKIP 矩阵当时尚未通过。该历史 RED 后续已由本文件“完整三数据库共享矩阵（PASS）”章节取代。

## 2026-08-11 timed historical 全量窄测

- 命令：`gofmt -w model/timed_subscription_valuation_backfill_test.go && go test ./model -run TestTimedHistoricalBackfill -count=1`
- 退出码：`0`
- 结果：`ok github.com/QuantumNous/new-api/model 6.652s`。
- 先前失败（已保留事实）：同一命令曾因两个旧 exact forward fixture 缺失 `SourceSnapshot`/`FxCapturedAt` 及冻结字段而退出 `1`；仅补齐 fixture 后重跑 GREEN，未放宽生产校验。
- 结论：timed historical 现有窄测试全部通过；这只是 Gate D 的一部分证据，尚未宣称 Gate D PASS。

## Gate B：命令解析与早期分派（部分 PASS）

- 命令：`go test . -run 'Test(ParseCreditValuationCommandArgs|CreditValuationCommand|RunCreditValuationCommand)' -count=1`
- 退出码：`0`
- 结果：`ok github.com/QuantumNous/new-api 0.157s`。
- 已覆盖：五种互斥模式、版本/批大小/reason/未知参数稳定 machine-readable code、参数错误发生在数据库打开前、迁移失败报告与关闭失败报告、根命令早期分派。
- Gate B 尚未完成：真实 CLI smoke、HTTP/Redis/worker 零副作用证明、dry-run/verify 写保护、确定性标准化 JSON/SHA-256、CAS/resume/replay 行为仍须分别实测。

- 真实 CLI 首轮（FAIL，已修根因后待重跑）：在 `.scratch/agent-progress/issue-27/gate-b-cli.sqlite` 运行两次 `go run . credit-valuation-migrate --dry-run --version 1`，均退出 `1`，稳定 code 为 `credit_valuation_command_migration_failed`，根因 `no such table: subscription_plans`；SQLite SHA-256 前/中/后均为 `de91b21cd76e7178cc4de23c909df47ea1b21eb13fb4ac315de2086a502cb7fc`，证明失败路径零写。
- 同轮非法请求 `go run . credit-valuation-migrate --dry-run` 返回 `credit_valuation_command_version_required`；`go run` 包装器把程序退出 `2` 映射为外层退出 `1`。
- 首轮还发现数据库选择日志写入 stdout；已在 `model/main.go` 抽出静默维护连接并将 GORM logger 设为 silent，待补齐同一临时库的必要 schema 后重跑真实 CLI。

- 生产 schema fixture 准备事实：
  - 第一次独立 `model.InitDB()` 准备命令退出 `1`：`common.IsMasterNode` 默认 false，未执行 `migrateDB`，随后测试 helper 调用 `CloseDB` 因 `LOG_DB=nil` panic；未计 PASS。
  - 第二次设 `common.IsMasterNode=true` 后退出 `1`：先前手写 `credit_valuation_migrations` 表不完整，生产迁移报 `failed to look up field status from DDL ...`；未计 PASS。
  - 删除仅用于首轮 smoke 的两张手写表后，在同一 SQLite 文件运行独立生产 `InitDB`/`migrateDB` 准备命令，退出 `0`，stdout 包含 `database migration started` 与 `production schema migration complete`，stderr 为空；fixture SHA-256 为 `44a3952b37c9109f3dd91d4d1b48a23087ca25eb72fcc83b0cf768a5926a80e3`。
  - 当前恢复命令：针对现有 `.scratch/agent-progress/issue-27/gate-b-cli.sqlite` 运行 `credit-valuation-migrate --dry-run --version 1` 与 `--verify --version 1`；维护命令自身不得调用 `migrateDB` 或执行 schema 写。
- provider/socket blocker：一次 `git status --short --branch` 在执行前因 socket closed 中断，未产生文件系统效果；当前恢复后尚未重复该无关检查。

## Gate B：真实 CLI、只读与确定性报告（PASS）

- fixture 准备：独立临时 Go 入口设置 `common.SQLitePath`、`common.IsMasterNode=true` 后调用生产 `model.InitDB()`/`migrateDB`，退出 `0`；该准备发生在维护命令之前，维护命令自身未执行 DDL。
- fixture：`.scratch/agent-progress/issue-27/gate-b-cli.sqlite`；smoke 前 SHA-256：`44a3952b37c9109f3dd91d4d1b48a23087ca25eb72fcc83b0cf768a5926a80e3`。
- 环境：移除 `SQL_DSN`，设置 `SQLITE_PATH` 为上述 fixture、`REDIS_CONN_STRING=redis://127.0.0.1:1/0`、`PORT=65531`、`GIN_MODE=release`；以进程树与 TCP 连接监视三次命令。
- 命令 1/2：`go run . credit-valuation-migrate --dry-run --version 1`；两次退出码均为 `0`，stderr 均为空，stdout 均为单行可解析 JSON 且逐字节相同；`success=true`、`mode=dry_run`、`read_only=true`、`changed=false`、`ready=true`。
- 命令 3：`go run . credit-valuation-migrate --verify --version 1`；退出码 `0`，stderr 为空，stdout 为单行可解析 JSON；`success=true`、`mode=verify`、`read_only=true`、`changed=false`、`ready=true`。
- 三次业务 checksum 均为 `477f71c3fc73e8161c382ee6b26c54ce9d8411f93221a7832cb2280c1b0c7ae1`；两次 dry-run 完整 stdout 相同。报告为稳定结构体/排序切片 JSON，checksum 为 64 位 SHA-256。
- 每次命令前后数据库 SHA-256 均为 `44a3952b37c9109f3dd91d4d1b48a23087ca25eb72fcc83b0cf768a5926a80e3`，证明 dry-run/verify 完全只读。
- 副作用监视：三次均未监听 `PORT=65531`，未连接 Redis 端口 `1`；进程树仅见 `compile.exe`、`link.exe`、命令进程 `new-api.exe`，无 HTTP 服务、定时器或业务 worker 子进程。
- 参数/CAS/resume/replay：命令解析与早期分派单测退出 `0`；active lease 冲突、expired lease 同 checksum 恢复、checksum drift 拒绝、failed retry 冻结 FX/checksum、旧 ready 被更高 non-ready 阻断等窄测退出 `0`（详见 Gate E 证据）。
- 结论：Gate B PASS。首次缺 schema/日志污染的 FAIL 事实仍保留在上文，修复后真实 smoke 已复跑通过。

## Gate C：精确历史价格迁移（SQLite 部分 PASS）

- 临时 Gate B fixture 已在取证后删除。
- 命令：`go test ./model -run 'Test(RunCreditValuationPlanPriceMigration|CreditValuationPlanPriceMigration)' -count=1`
- 退出码：`0`
- 结果：`ok github.com/QuantumNous/new-api/model 6.450s`。
- 已覆盖：SQLite 原始数值文本读取、精确 micros 回填、已有 exact 保留、零价、重复 dry-run 零写与稳定报告、负数/非法文本/超六位/溢出/roundtrip mismatch、非法 apply 零写、稳定 batch 边界、失败事务回滚、重跑幂等、三方言 CAST 生成合同。
- Gate C 尚未整体 PASS：MySQL 5.7.44 与 PostgreSQL 9.6.24 的真实执行并入 Gate F 新鲜共享矩阵后才能完成。

## Gate D：Credit 与 timed 历史重建（SQLite PASS）

- 命令：`go test ./model -run 'Test(CreditHistoricalBackfill|TimedHistoricalBackfill)' -count=1`
- 退出码：`0`
- 结果：`ok github.com/QuantumNous/new-api/model 0.475s`。
- Credit 覆盖：`A/K/U/T/C/R` 保守分配、known/unknown 来源、稳定 source identity 去重、outflow 排除、dry-run 零写、apply 幂等、existing state 保留、repair 版本约束与 persisted replay 汇总。
- timed 覆盖：权威唯一 order/redemption 服务窗口、缺失窗口 unknown、无独立 legacy admin 履约事实时禁止用 `UserSubscription.StartTime/EndTime` 猜测、完整 existing exact grant 校验/skip、candidate 与 forward 窗口冲突、renewal overlap 整份 subscription fail-closed 且零部分写、FX snapshot 与多币种汇总。
- Gate D SQLite 行为 PASS；真实 MySQL/PostgreSQL 执行仍属于 Gate F，方言生成测试不替代三库验收。

## Gate E：marker CAS、状态门禁与 fail-closed（部分 PASS）

- 命令：`go test ./model -run 'Test(ValidateCreditValuationMigrationRequestRequiresVersion|FreshCreditValuationDatabaseAutoReadiesThroughStartupMigration|VerifyCreditValuationMigrationRejectsEveryNonReadyStatus|HigherFailedMarkerBlocksLowerReadyApplyReplay|RunningMigrationLeaseActiveRejectsClaim|ExpiredRunningMigrationSameChecksumCanResume|ExpiredRunningMigrationChecksumDriftRejectsResume|FailedMigrationRetryPreservesFrozenFXAndChecksum|SuspendedMigrationRejectsGrantAndPreconsumeWithoutSideEffects|VerifyCreditValuationMigrationSourcesUsesCanonicalCreditSourceKey|MigrationSnapshotUsesFXDirectionForUSDValuation|RepairMissingAsUnknownCanBeAppliedWithSameVersion|VerifyCreditValuationMigrationSourcesRejectsInvalidTimedGrantFacts|SuspendTransitionsHighestReadyMarkerAndKeepsReadOnlyViewsAvailable|FreshDatabaseDetectionIncludesTimedHistoryWithoutCredit)$' -count=1`
- 退出码：`0`
- 结果：`ok github.com/QuantumNous/new-api/model 0.523s`。
- 已覆盖：pending/running/failed/suspended 拒绝 verify、过期 lease resume 与 checksum drift、冻结 FX/checksum、suspended 写保护、source 唯一性与 timed grant 非法事实、repair/version、空库 auto-ready 与 timed 历史阻止 auto-ready。
- Gate E 尚未完成：真实服务启动/运行时热路径全量接缝、所有 ready 后数量状态一致性矩阵及三数据库验收仍须实测。

## Gate E：marker 与热路径接缝（SQLite PASS）

- 命令：`go test ./model -run 'Test(CreditValuationMigration|FreshCreditValuationDatabase|VerifyCreditValuationMigration|HigherFailed|RunningMigration|ExpiredRunning|FailedMigration|SuspendedMigration|MigrationSnapshot|RepairMissing|SuspendTransitions|FreshDatabaseDetection|CreditValuationOrderIngressPreservesLegacyPathWhenMarkerNotReady|CreditValuationRequestPreConsumeRemovesMovingAverageCost|CreditValuationRequestTargetStateMissingRollsBackAtomically|CreditValuationRequestTargetStateMismatchRollsBackAtomically|CreditBalancePreConsumeRejectsExhaustionAndAllowsSettlementDebt|CreditValuationOrderRecoveryUsesImmutablePurchaseFactsAndTerminalReplay|NativeCreditOrderRecoveryRejectsMissingPurchaseLedger)$' -count=1`
- 退出码：`0`
- 结果：`ok github.com/QuantumNous/new-api/model 0.539s`。
- 覆盖：marker 生命周期/CAS/lease/checksum/repair/suspend、空库 auto-ready、ready 前 legacy 路径、ready 后移动平均预消费、state missing/mismatch 原子回滚、余额耗尽与 settlement debt、订单恢复不可变事实及缺 ledger fail-closed。
- Gate E SQLite 行为 PASS；真实 MySQL/PostgreSQL 与并发合法串行化集合仍属于 Gate F。

## Gate F：最终矩阵前的历史 blocker（已由最终 PASS 取代）

- 环境探测命令：`printenv TEST_MYSQL_DSN && printenv TEST_POSTGRES_DSN`。
- 探测退出码：`1`；无输出，当前 shell 未提供真实 MySQL/PostgreSQL 测试 DSN。
- 服务盘点命令：`hub op=ps`（当前项目范围仅返回 `omp.lsp.mux`，无 MySQL/PostgreSQL 服务）；结果未发现可用持久数据库服务。
- 现有可复核 FAIL：`.scratch/agent-progress/issue-27/external-migration-matrix-20260810-final.txt` 显示 MySQL 5.7.44 并发阶段 `invalid connection`，PostgreSQL 9.6.24 `127.0.0.1:35443` connection refused；该文件明确是 FAIL，不能计作 PASS。
- 当时结论：Gate F 保持 `FAIL/blocker`；当时尚未运行新鲜 SQLite/MySQL 5.7.44/PostgreSQL 9.6.24 同一共享矩阵，不能声称 `SKIP=0` 或三库 PASS。最终状态见本文件后续“完整三数据库共享矩阵（PASS）”章节。
- 当时的恢复动作：恢复持久 MySQL 5.7.44/PostgreSQL 9.6.24 服务与稳定 DSN，确认 `SELECT VERSION()`，再运行同一 `TestCreditValuationExternalMatrix`。该动作已完成；旧缺 DSN/连接失败只作为历史 RED，不计最终证据。

## Gate F：历史隧道连接与版本探针（当时矩阵仍 RED）

- 持久隧道：`127.0.0.1:33317 -> RackNerd6C6G issue27-mysql57-final2:3306`；`127.0.0.1:35443 -> RackNerd6C6G issue27-postgres96-final2:5432`。Orca hub 进程名分别为 `issue27-mysql57-tunnel`、`issue27-postgres96-tunnel`，TCP readiness 均通过。
- 本地 Go/GORM 探针使用与生产测试相同驱动执行 `Ping` 与 `SELECT VERSION()`；退出码 `0`。
- MySQL 结果：`mysql|connected=true|version=5.7.44`。
- PostgreSQL 结果：`postgres|connected=true|version=PostgreSQL 9.6.24 on x86_64-pc-linux-gnu (Debian 9.6.24-1.pgdg90+1)`。
- 连接配置：MySQL 与 PostgreSQL 专用测试 DSN 仅存放于仓库外受限环境文件；证据只记录 `set/unset`、长度、目标版本和脱敏端口，不记录账号或凭据。
- 当时仅目标版本环境 blocker 解除；最终共享矩阵后来已在 SQLite/MySQL/PostgreSQL 全部 PASS 且 `SKIP=0`，见后续最终章节。

## Gate F：共享矩阵调研结论与落盘结构（实现前）

- 唯一入口将新增为 `model/credit_valuation_external_matrix_test.go` 中的 `TestCreditValuationExternalMatrix`；顶层只包含 `sqlite`、`mysql`、`postgres` 三个顺序子测试。由于 `model.DB`、`model.LOG_DB`、数据库方言标志及缓存是包级状态，禁止 `t.Parallel()`。
- 外部 DSN 缺失必须用 `t.Fatalf` 使矩阵失败，绝不 `Skip`；SQLite 使用独立临时文件，MySQL/PostgreSQL 使用本任务 final2 容器的专用 `issue27` 数据库。每个子测试先核对实际 `SELECT VERSION()`，MySQL 必须为 `5.7.44`，PostgreSQL 必须为 `9.6.24`。
- 每个 dialect 走同一生产 `migrateDB()` 与同一业务断言；仅数据库打开、测试库复位及版本匹配允许方言分支。测试依次覆盖：生产 schema/命名唯一约束与真实行锁；原始 DECIMAL/NUMERIC/SQLite 文本价格的 dry-run 重放只读、apply、verify；migration marker ready；Credit purchase ingress、consume、request refund、timed conversion、order recovery；summary/users/subscriptions/plans/sources 五分析接口的 32 CNY 一致结果；grant+grant、grant+consume、consume+restore、conversion+settlement、refund+admin decrease 五组并发合法串行化与数量/估值/ledger 不变量。
- TDD 切片顺序：先落盘三 dialect/版本/生产 schema/价格/marker tracer 并运行唯一测试取得真实 RED；修至 GREEN 后逐一增加生命周期、分析、五组并发。每次 RED/PASS 都追加实际命令、退出码和首个失败到本文件。
- 现有 `TestCreditBalanceProductionMigrationExternalDatabases` 只覆盖 MySQL/PostgreSQL 的旧迁移与同幂等 grant，且 DSN 缺失会 Skip；它不能替代 Gate F 新共享矩阵，也不会被包装为 PASS。

## Gate F：完整三数据库共享矩阵（PASS）

- 唯一入口：`go test ./model -run 'TestCreditValuationExternalMatrix$' -count=1 -v -timeout 60m`。
- 外部数据库 DSN 仅从仓库外受限环境注入；测试入口在缺失 DSN 时 `Fatal`，不允许 `Skip`。证据未记录 DSN、账号或凭据。
- 退出码：`0`；结果：`ok github.com/QuantumNous/new-api/model 2437.133s`；输出中无 `SKIP`。
- 真实版本：SQLite `3.50.4`；MySQL `5.7.44`；PostgreSQL `9.6.24`。
- 三库各自顺序通过 12 个相同行为阶段：schema、row_lock、price_migration、migration_engine、lifecycle、conversion、recovery、concurrent_grant_grant、concurrent_grant_consume、concurrent_consume_restore、concurrent_conversion_settlement、concurrent_refund_admin_decrease；合计 36 个行为子阶段 PASS。
- `migration_engine` 在三库均执行第二次 dry-run、apply、verify 与 ready 重放，确认 dry-run checksum/批次稳定、apply 后 verify checksum 一致、同版本 ready replay 不改变状态/grant/marker 数量。
- lifecycle 从真实购买入口建立永久 Credit，消费 200 后保留 800，并由 summary/users/subscriptions/plans/sources 五个分析读取同一 `32,000,000` micros CNY、active count 1、estimated 0、unknown 0。
- 首轮 MySQL 完整矩阵在 concurrent consume+restore 返回 `credit_valuation_state_mismatch`。根因是 `forUpdate=true` 的单活动权益缓存命中仍可能携带陈旧对象；修复后缓存仅提供候选 ID，当前写事务按 ID 重查并 `FOR UPDATE`，再校验 status、时间、余额和 requiredTokens。
- 修复后的定向命令 `go test ./model -run 'TestCreditValuationExternalMatrix/mysql/concurrent_consume_restore$' -count=1 -v -timeout 15m` 在 MySQL 5.7.44 退出 `0`，结果 `ok ... 413.443s`；随后 MySQL 完整子矩阵、PostgreSQL 完整子矩阵和上述唯一三库入口均退出 `0`。
- Gate F 结论：PASS，`SKIP=0`。定向 Go `-race` 是独立补充门禁，仍须在提交前记录，不能由本矩阵替代。

## Gate A/H：临时产物清理（PASS）

- 已逐项检查并删除仅用于诊断/传输的 `mysql-process-inspect.go`、`db-version-probe.go`、`issue27-go-test-src.tar.gz`、`issue27-remote-src.tar.gz`、五份 `.log` 和两份旧矩阵 `.txt`。
- 两个 tar.gz 分别约 1.7 MB 与 42.7 MB，包含源码副本而非交付物；原始日志包含数据库进程/连接现场，既非最终 PASS 证据也不适合提交。
- 最终证据只保留结构化、脱敏 Markdown；仓库中不保留 DSN、凭据、数据库 dump、完整敏感日志或重复源码归档。
- `release-handoff.md` 单独记录 #28 所需命令、marker/CAS、稳定 blocker、三库矩阵、32 CNY fixture 与生产重验边界。


## 2026-08-12 提交前最终回归

- 免费模型零资金活动回归最初稳定失败：`TestSettleBillingWithInputFreeModelIgnoresNonZeroSubscriptionTokens` 返回 `credit_valuation_request_not_found`。根因是免费模型只验证活动订阅、不创建预扣记录，但结算会话无条件进入 request-aware 路由。
- 修复仅在 `ErrCreditValuationRequestNotFound`、`creditValuationTracked=false`、目标/预扣/已提交增量/资金来源目标均为零时接受无操作；非零目标、已有预扣或已跟踪 Credit 请求仍 fail-closed。
- 新增 `TestSettleBillingWithInputMissingRequestStillFailsForSubscriptionFundingActivity`，覆盖非零目标和已有预扣两类缺请求记录场景；两者均保持 `ErrCreditValuationRequestNotFound` 且数量不变。
- 新增 `TestCreditBillingSessionFinalizesMappedZeroTargetAfterRestart`，证明已映射但当前目标为零的请求仍进入深模块并原子写入 `status=refunded` 与 `finalized_at`，不会被免费模型无操作边界绕过。
- 三项零目标边界单次与 `-count=5` 通过；对应 `-race` 通过：`ok github.com/QuantumNous/new-api/service 8.474s`。
- 请求累计、转换后退款与 Task 身份行为集普通窄测通过；定向 `-race` 通过：`ok github.com/QuantumNous/new-api/model 11.853s`、`ok github.com/QuantumNous/new-api/service 5.719s`。最终加入零目标边界后的窄 `-race` 再次通过：model `5.537s`、service `4.649s`。
- 本地后端全套以 `go test ./... -skip '^TestCreditValuationExternalMatrix$' -count=1` 运行，最终结果为 52 个 package PASS、60 个 package 无测试。外部矩阵不在该命令中重复执行；其 SQLite/MySQL 5.7.44/PostgreSQL 9.6.24 零 SKIP 证据仍以本文件 Gate F 章节的独立完整运行结果为准。
- 一次本地后端全套曾在 SQLite 并发路由测试 `TestSubscriptionConversionRouteConcurrentDifferentKeysConvertsSourceOnce` 返回通用错误码；同测试随后 `-count=10` 全部通过，完整本地后端全套重跑也通过。该次失败保留为环境竞争事实，不冒充首次全套成功。
- 前端依赖使用 `bun install --frozen-lockfile` 恢复；`bun run typecheck` 与 `bun run build` 均通过。Issue #27 相对基线没有任何 `web/default` 文件差异。
- 前端全套实际结果为 571 PASS、1 FAIL；唯一失败 `admin-credit-balance-panel.test.tsx` 在全套和单文件运行中都于约 5 分钟后 `RangeError: Out of memory`。该失败不是 Issue #27 引入，但在 Issue #28 发布前仍必须闭环，不能记为前端全套 PASS。
- `bun run copyright:check` 在 Worker 与干净集成基线均返回完全相同的 83 个既有待更新文件；Issue #27 无前端差异。该基线门禁仍由 Issue #28 发布前处理，未批量修改项目品牌、归属或元数据。
- `gofmt` 已作用于本 Issue 修改的 Go 文件；最终定向命令/迁移/model/service 回归通过；`git diff --check` 无输出。