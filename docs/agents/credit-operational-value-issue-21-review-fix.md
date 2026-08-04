# Issue #21 Standards 阻塞项修复 Agent 指令

## 任务目标与冻结基线

你负责修复父 PRD #19、GitHub Issue #21「固化计时权益 grant 时间线与多币种分析」在最终 Standards 评审中确认的四项阻塞。工作目录必须是现有 Orca 工作树：

`C:/Users/34404/source/repos/new-api/.workspaces/new-api/issue-21-timed-grants`

冻结实现 HEAD 是 `547512242578ec198034d322875c5485735b247a`；该 HEAD 已完成 timed grant 领域入口、五接口、管理员 UI、六语言和真实浏览器证据，但尚未集成。父集成分支已经包含 Issue #22 的通用 CreditValuation、权威 `amount_micros` DTO/排序、`current_only` warning 与 BigInt 前端合同；开始修复前必须读取父集成工作树当前 HEAD，并把最新 `jiwangyihao/credit-operational-value-integration` 合并进本子分支，在本工作树内解决共享 DTO/analytics 冲突。冲突决策固定为：保留 #22 的通用 Credit DTO、Credit/current_only、整数 micros accumulator/sorter 与前端通用骨架，只叠加 #21 的 timed grant、`*_by_currency`、timed warning/source 增量。不得把旧 float accumulator 或旧 DTO 覆盖回父实现。你不得向父分支合并、关闭 Issue 或回收工作树；协调器完成最终集成。

## 必读资料与执行 Skill

修改前按顺序读取并服从：

1. 仓库/全局 `AGENTS.md`。
2. `issue://jiwangyihao/new-api/19`、`issue://jiwangyihao/new-api/21`、已关闭的 `issue://jiwangyihao/new-api/22`。
3. `docs/agents/credit-operational-value-execution.md`。
4. `docs/agents/credit-operational-value-wave-1-contract.md`、`credit-operational-value-wave-1-acceptance.md`。
5. `docs/agents/credit-operational-value-issue-21.md` 与 `credit-operational-value-issue-21-acceptance.md`。
6. `C:/Users/34404/AppData/Local/Temp/new-api-issue21-standards-final-review.md`。
7. `.scratch/agent-progress/issue-21/{status,evidence,contract}.md`。
8. `CONTEXT.md`、ADR 0002、2026-08-02 specification/plan 的 timed、整数金额、错误合同与并发章节。
9. 父集成树中 #22 的 `model/admin_analytics_paid_subscription.go`、相关 DTO、排序测试和 current_only 测试。

必须使用 `skill://diagnosing-bugs` 逐项复现根因，使用 `skill://tdd` 完成 RED→最小 GREEN，使用 `skill://codebase-design` 保持 timed 深模块与 #22 通用骨架边界。若不改 UI 或可见文案，不得额外改前端/i18n；若因冲突不得不触碰 `web/default`，先读 `skill://shadcn-ui` 与 `skill://i18n-translate`，只保留既有合同，不新增产品范围。修改 exported symbol 前先使用 LSP references；不要用文本搜索代替可用的符号工具。

## 崩溃恢复与提交纪律

第一项实际变更必须创建并提交：

- `.scratch/agent-progress/issue-21/review-fix-status.md`
- `.scratch/agent-progress/issue-21/review-fix-evidence.md`
- `.scratch/agent-progress/issue-21/review-fix-contract.md`

其中记录：父集成 HEAD、冻结 HEAD、合并提交/冲突决策、四项 finding 状态、最近安全提交、未提交文件、下一条精确命令和阻塞。每个 finding 的有效 RED、最小 GREEN、定向验证完成后立即更新并提交。推荐每 10–20 分钟形成一个可恢复安全提交；禁止把关键成果只留在终端、stash、临时脚本或未提交大 diff。上下文达到约 80% 时必须先持久化 HANDOFF_READY 和 clean/诚实 WIP 提交，不得无界探索。

提交遵循 Conventional Commits，英文 type/scope、简体中文 subject。只格式化实际修改文件。不要运行项目级全量测试、生产部署或无关 formatter。

## 必须修复的四项 blocker

### Finding 1：并发同源重放的合法串行化

当前 `GrantTimedSubscriptionTx` 在取得权威计划行锁之前查重；两个真实数据库连接可能同时未查到来源，随后一个成功、另一个在 grant 唯一约束处泄漏方言相关原始 DB error，且可能先续期再撞唯一约束。必须先写确定性真实 SQLite RED：

- 使用文件型或共享 SQLite、多连接（不得 `SetMaxOpenConns(1)`）、显式 barrier/hook，不依赖 `sleep` 或 goroutine 运气；
- 两个并发事务提交完全相同的 `source_type/source_key/idempotency_key` 与相同事实；
- 合法结果只能是同一 entitlement/window/grant 的成功重放，最终一份权益、一条 grant、只续期一次；不得向调用者泄漏 `UNIQUE constraint` 文本；
- 相同 identity 但参数不同必须稳定返回 `ErrTimedSubscriptionGrantIdempotencyMismatch` 并整体回滚；
- disabled plan 已提交来源仍可重放，新来源仍拒绝。

最小实现应建立跨 SQLite/MySQL 5.7/PostgreSQL 9.6 均成立的线性化顺序。优先复用仓库现有计划行 guard/事务锁模式；锁序固定且写入 contract。唯一冲突仍可能发生时，必须映射到稳定 sentinel 并重新读取已提交事实，不能依赖解析数据库错误文本。不得新增仅进程内 mutex 作为正确性来源，不得用单连接测试伪造并发证明。至少重复运行并发用例 `-count=10`，并对窄包运行 `-race`；真实 MySQL/PostgreSQL 零 SKIP 仍归 #27，不得虚报。

### Finding 2：权威整数 micros 聚合与排序

当前 #21 分支的 `adminMoneyAccumulator` 同时保存 `float64` 和 micros，并把 micros 转回 float 做聚合/比较；这会破坏大整数精度，也会覆盖已集成 #22 的权威 `amount_micros` 排序。合并父集成基线后：

- 通用 Credit 路径完整保留 #22 已验收实现；
- timed `ByCurrency` / `BySourceCurrency` 只通过整数 micros 接入通用 accumulator；
- summary/users/subscriptions/plans/sources 的金额聚合与 recognized 排序以十进制 `amount_micros` 为唯一权威值；兼容 `Amount float64` 仅用于展示，不能参与精确值计算或排序；
- 保留 #22 的解析失败稳定错误、升降序与业务主键 tie-breaker；
- 单币种 singular 与多币种 nullable/by-currency 合同不变，禁止跨币种求和或按当前 Plan 币种补猜。

先补 precision-boundary RED（超过 JavaScript/float 安全精度但 int64 合法）以及 Credit 32 CNY + timed CNY/USD 组合测试，再做最小接线。不要设计第二套 sorter/DTO/accumulator。

### Finding 3：所有 timed micros 加法溢出关闭

`adminCalculateTimedSubscriptionValue` 和聚合器中的 `tokenMicros += futureMicros`、currency/source accumulator 等 `int64 +=` 必须使用已有 `checkedAddInt64` 或等价窄 helper，并稳定返回 `ErrCreditValuationOverflow`。必须写可观察 RED，至少覆盖：

- 单个 segment 合法，但同币种多 segment 累加超过 `math.MaxInt64`；
- source 聚合与五接口总计溢出；
- 临界 `MaxInt64` 可成功，下一单位稳定失败；
- 失败不能返回截断/负数/部分 totals，也不能静默降为 unknown。

不得使用 binary float、`big.Int` 热路径或按请求分配型抽象。错误必须沿五接口返回稳定分支，不解析文本。

### Finding 4：不可变 hook 的稳定 sentinel/code

`TimedSubscriptionValuationGrant.BeforeUpdate/BeforeDelete` 当前每次 `errors.New`。定义并复用一个稳定 sentinel（例如语义明确的 `ErrTimedSubscriptionGrantImmutable`，实际命名遵循仓库既有错误体系），两个 hook 返回同一 sentinel，领域/API 层通过 `errors.Is` 判断。补测试证明 update/delete 均 `errors.Is` 命中，重复调用错误身份稳定，原 grant 未变化。若 API 会暴露该错误，复用既有稳定 code 映射；不得新增依赖错误文本的分支。

## 非目标与禁止事项

- 不重做已完成的 timed 购买、兑换、管理员 grant、UI、六语言或浏览器流程。
- 不修改 CreditValuation 深模块、Credit request settlement、正向入账、退款/恢复、转换 FX、历史迁移、marker/ready、发布代码。
- 不实现 #23–#28，不新增 retry/telemetry/通用框架。
- 不全量重排六 locale 或 `_reports`；若合并冲突产生噪声，恢复 #22/父集成版本，仅保留 #21 现有 15 个翻译键和必要 timed 增量。
- 不修改受保护项目标识、用户无关文件、锁文件或部署配置。
- 不把 GORM DryRun、单连接 SQLite 或测试 SKIP 宣称为跨数据库通过。

## 验收与完成条件

至少提供并持久化以下证据：

1. 四项 finding 各自有效 RED、最小 GREEN、提交 SHA。
2. 真实多连接 SQLite 同源并发测试 `-count=10` 与窄范围 `go test -race`。
3. 不同参数冲突、disabled 已提交重放/新来源拒绝、事务回滚与唯一错误不泄漏。
4. `MaxInt64` 聚合边界、溢出稳定 sentinel、无部分响应。
5. 不可变 update/delete 的 `errors.Is` 稳定测试。
6. #22 32 CNY Credit tracer/current_only/权威 micros 排序与 #21 timed CNY/USD 五接口组合定向测试同时通过；Credit recognized 仍为 `32,000,000` micros、available 800、active count 1，timed 逐币种值/nullable singular 不回归。
7. 受影响 `model/service/controller` 定向测试；不需要重复真实浏览器，除非你修改了 UI。
8. `git diff --check`，工作树 staged/unstaged/untracked 全为 0。
9. MySQL/PostgreSQL 未运行时明确写“未运行”，三库零 SKIP 归 #27。

完成前逐条复核 Issue #21 acceptance 和 Standards 四项 finding，更新 `review-fix-*` 为 COMPLETE，列出合并父基线后的共享文件及冲突决策。然后在当前 Dispatch 使用注入 capability 发送且只发送一次 `worker_done`，包含最终 HEAD、提交列表、RED/GREEN 命令、SQLite/race/组合证据、未运行项与非所有权声明。不要合并父分支、关闭 Issue 或回收工作树。