# Issue #26 最终复评缺口修复 Agent 指令

## 任务目标

你负责父 PRD `jiwangyihao/new-api#19`、子 Issue `jiwangyihao/new-api#26`「固化转换估值、FX 与在途请求结算」在最终 Standards/Spec 复评中确认的剩余缺口。工作必须发生在协调器为本任务显式创建的 Orca 子工作树内，并以创建时的集成分支 HEAD `6f865feca3cd517a3dd744e67ea1240d5001d2ed` 为冻结基线。该基线已经集成 #20、#21、#22、#23、#24 同币种核心和 #26 既有实现；不得 reset、rebase、切换到 `origin/main`，也不得重写或丢弃既有历史。

本任务只修复最终复评报告中的 #26 自有问题：

1. **H1：固定 request → target 锁序。** Confirm 必须在任何目标 Credit entitlement、valuation state、ledger 或 conversion 写入之前，按稳定 request identity 顺序锁定并验证该 source 的在途 request；随后才锁定目标 Credit 并写入。不得以 SQLite 特判、死锁重试或忽略错误替代固定锁序。
2. **M1：稳定 sentinel/code。** 增加并正确包装导出的 `ErrConversionIneligible` 与 `ErrConversionQuoteStale`；Go 调用者必须可用 `errors.Is` 判断。Controller 只能依据 sentinel 映射稳定 code，不能解析错误文本或前缀。现有 `ErrConversionIdempotencyConflict` 与 FX sentinel 保持不变。
3. **M3：API 返回 committed unit value。** Confirm/history 必须直接格式化 `SubscriptionConversion.ValuationUnitValueNumeratorMicros` 与 `ValuationUnitValueDenominator`，不得在响应层用 `math/big` 从其他字段重算并掩盖持久化事实。
4. **M2：quote identity/stale 合同。** Quote 必须返回服务端不可伪造且可验证的 quote identity（或等价的持久化身份）、`created_at`、`expires_at` 与权威 facts fingerprint；Confirm 必须携带该身份，在锁内校验 user/source、过期时间和所有 authoritative frozen facts。过期、篡改或 facts 漂移必须返回 `ErrConversionQuoteStale` 和稳定 API code，且事务零写入。优先复用仓库现有签名/随机身份/存储惯例；不要发明第二套通用 token 框架。

Issue #24 的 H2（管理员 increase/redemption 消费唯一 FX seam）由正在运行的 #24 Worker 独占。你不得修改 `model/credit_balance_adjustment.go`、`model/redemption.go`、管理员正向入账 API/UI 或其测试来“顺手修复” H2。也不得实现 #25 destructive recovery、#27 migration/marker、#28 release。

## 开工自检与恢复协议

开工后第一时间完成并持久化以下内容：

- 读取：
  - `AGENTS.md` 与全局规则；
  - GitHub `jiwangyihao/new-api#19`、`#26`（必须显式 `--repo jiwangyihao/new-api`）；
  - `CONTEXT.md`；
  - `docs/adr/0002-credit-operational-remaining-value.md`；
  - `docs/superpowers/specs/2026-08-02-credit-operational-remaining-value-spec.md`；
  - `docs/superpowers/plans/2026-08-02-credit-operational-remaining-value-plan.md`；
  - `docs/agents/credit-operational-value-execution.md`；
  - `docs/agents/credit-operational-value-issue-26.md`；
  - `docs/agents/credit-operational-value-issue-26-acceptance.md`；
  - `C:/Users/34404/AppData/Local/Temp/new-api-issue26-final-integrated-standards-review.md`；
  - `C:/Users/34404/AppData/Local/Temp/new-api-issue26-final-integrated-spec-review.md`。
- 必须使用 `diagnosing-bugs`、`tdd` 与 `codebase-design` skill；不要把 review finding 当作未经复现的真理，先以最小测试证明根因。
- 创建并尽快提交：
  - `.scratch/agent-progress/issue-26-final-review-fix/contract.md`
  - `.scratch/agent-progress/issue-26-final-review-fix/status.md`
  - `.scratch/agent-progress/issue-26-final-review-fix/evidence.md`
- 每份文件写明冻结 HEAD、当前 phase、最近安全提交、未提交文件、RED/GREEN 命令与结果、阻塞和下一动作。上下文达到约 80% 前必须形成 clean HANDOFF_READY；禁止把大量未提交改动留给下一 Agent。
- 所有实现按小步提交：每个阶段独立 RED、独立 GREEN、证据/状态校准。提交消息遵循 Conventional Commits，subject 使用简体中文。

## 分阶段执行顺序

### 阶段 A：H1 request → target 固定锁序

只处理锁序，不同时修改错误映射、quote 或前端。

1. 在真实文件 SQLite/WAL 或现有可控 hook 中写确定性交错 RED，证明 Confirm 当前在锁定 request 前进入目标 Credit/ledger。测试必须观察真实顺序或合法串行化结果，不能检查源码文本。
2. 将现有 `freezeTimedConversionInFlightRequestsTx` 拆成清晰的两阶段深模块接口：
   - 目标写入前，按 request ID 升序查询、锁定、验证并捕获待冻结 rows；
   - 目标 ingress 完成后，只更新已经锁定并验证的 rows。
3. 固定顺序应为 conversion/idempotency → quote → source subscription/plan/grants → in-flight requests（稳定 identity 顺序）→ target Credit/valuation/ledger → conversion/source/history/activity writes。
4. 补相反入口（request-aware final settle/refund）与 Confirm 的交错测试，证明无内部 DB 错误泄漏、无重复扣减、无部分写。SQLite 不能证明 MySQL/PostgreSQL 行锁语义，但实现和测试必须明确固定顺序；真实三数据库仍由 #27 验收。
5. 运行定向单次、`-count=10` 与必要窄 `-race`，提交阶段 A clean 安全点。

### 阶段 B：M1 sentinel/code 与 M3 committed unit value

1. 先写行为 RED：
   - ineligible/stale 可由 `errors.Is` 判定；
   - controller 返回稳定 `subscription_conversion_ineligible` / `subscription_conversion_quote_stale` code；
   - Confirm/history API 即使其他冻结 operand 被测试夹具改成不一致值，也必须直接返回 conversion committed numerator/denominator。
2. 在 `model/errors.go` 定义稳定 sentinel；所有对应 model 分支使用 `%w` 包装，不拼接供机器解析的文本。
3. 删除 controller 的 `strings.HasPrefix` 分类和响应层 `math/big`/`big.Rat` 重算；用 `strconv.FormatInt` 直接返回 committed 字段。若 committed 字段非法，必须稳定 fail-closed，不能静默重算。
4. 前端 API 层必须保留结构化 `code`，现有 conversion card 使用 code → i18n key 的穷举映射；未知 code 使用本地化 fallback，不直接依赖服务端自由文本。只修改现有 conversion UI，不新建页面。
5. 新增文案同步 en、zh、fr、ru、ja、vi，运行 `bun run i18n:sync`，确保 missing/extras 为 0。
6. 提交阶段 B clean 安全点。

### 阶段 C：M2 quote identity/stale

1. 先写 quote→confirm 行为 RED，至少覆盖：合法 quote 成功；过期；签名/identity 篡改；user/source 不匹配；Plan price、duration/reset、basis、remaining、target currency、FX、rule/version 任一漂移；同一合法 quote + 同一 idempotency key 重放。
2. 设计一个深且最小的 quote seam：Quote 返回 `quote_id`（或 opaque token）、`created_at`、`expires_at`、`facts_fingerprint`。Confirm request 必须携带 quote identity。服务端不得信任客户端提交的 facts。
3. Confirm 在阶段 A 的锁序内验证 quote identity/expiry/fingerprint并锁后重算 authoritative facts；任一漂移返回 `ErrConversionQuoteStale`、稳定 code、零写入。
4. 采用仓库现有 JSON wrapper `common.*`，规范化 fingerprint 必须版本化、无歧义、全整数/字符串；不要使用浮点或临时当前 Option 反推历史。
5. 浏览器/前端应继续使用现有卡片；quote stale 时要求用户重新 quote，不能自动静默确认新事实。
6. 提交阶段 C clean 安全点。

## 验证门禁

阶段内只运行定向测试，避免在并行 Worker 期间阻塞全仓套件。全部实现完成后再运行一次最终门禁：

- H1 固定锁序、相反入口交错、转换在途 settle/refund、同 source 并发；
- sentinel/code/errors.Is；committed unit value；quote identity/expiry/fingerprint/stale/重放；
- 既有 FX parser/floor、同/跨币种 conversion、结构化 replay conflict、analytics route、32 CNY Credit 与 timed history 代表性回归；
- `go test ./model ./service ./controller -count=1`；
- 必要窄 `go test -race`；
- conversion 前端定向测试、`bun run typecheck`、`bun run i18n:sync`、`bun run build`；
- 真实 SQLite API quote→confirm→history；若现有 UI 受改动，使用当前分支真实应用与真实 Chromium 验证 stale 重新报价、稳定 code 本地化和 committed unit value，不能用静态拦截冒充；
- `git diff --check`，最终 `git status --short` 必须为空。

MySQL 5.7.44/PostgreSQL 9.6.24 零 SKIP 是 #27 门禁，本任务不得冒充已运行；但代码必须保持 GORM/三方言兼容。不要启动部署或生产操作。

## 完成与编排合同

完成时：

1. 更新三份 progress 文件，列出每个 finding 的 RED、根因、GREEN、提交与验证；
2. 确认工作树 clean；
3. 使用 Orca 注入的 task/dispatch/capability 发送一次且仅一次 `worker_done --outcome succeeded`，正文明确修改、证据、未实测边界和剩余工作；
4. 发送后停止工作并等待协调器。若无法完成，发送 `question` 或 `escalation`，不要伪造成功，也不要自行合并、关闭 Issue、删除 worktree或派发下游。
