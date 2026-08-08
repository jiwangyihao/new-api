# Issue #26 最终缺口修复证据

## 基线证据

- 命令：`git rev-parse HEAD && git branch --show-current && git status --porcelain=v1 --untracked-files=all && git merge-base HEAD c8a46e7c5de5cdfa6f94a41723d1396b57c7f2cd`。
- 结果：HEAD `c8a46e7c5de5cdfa6f94a41723d1396b57c7f2cd`；分支 `jiwangyihao/issue-26-final-gap-fix`；status 无输出；merge-base `c8a46e7c5de5cdfa6f94a41723d1396b57c7f2cd`。
- `orca worktree current --json`：当前 worktree HEAD 同上，`baseRef=jiwangyihao/credit-operational-value-integration`，`parentWorktreeId` 指向 `credit-operational-value-integration`，lineage capture 为 `explicit-cli-flag`。
- `orca status --json`：运行时 `ready`，Orca `1.4.176`。
- 跨会话 `agent://Issue26SpecReview`、`agent://Issue26StandardsReview` 返回 `No artifacts directory found`；未据此缩减范围，严格采用本 Dispatch 的三项权威 finding。

## Finding A：前端 quote identity

- RED 命令：`cd web/default && bun test src/features/subscription-conversion/components/timed-subscription-conversion-quotes-card.test.tsx`。
- 环境准备：子工作树初始缺少 `node_modules`，先运行 `bun install --frozen-lockfile`；锁文件未改变。依赖安装完成后同一命令进入行为断言。
- 首次 RED：`15 pass / 1 fail`，证明 confirm payload 丢失服务端 `quote_id`。审查随后指出首次测试错误允许最终刷新切换到新 identity 后直接确认，因此未以该错误合同进入 GREEN。
- 修正后 RED：`15 pass / 2 fail`。同 identity/fingerprint 的最终刷新实际 payload 缺少 `quote_id=quote-stable`；identity/fingerprint 变化时实际 confirm 请求数为 1，期望为 0。
- 根因：前端 `SubscriptionConversionQuote` 与 `SubscriptionConversionConfirmRequest` 未声明服务端 identity 字符串；`handleConfirm` 未传 `latest.quote_id`，也未在最终刷新发现重新报价时停下要求用户再次确认。
- GREEN：报价 DTO 精确保留 `quote_id`、`created_at`、`expires_at`、`facts_fingerprint`；confirm request 必须携 `quote_id`。最终刷新 identity/fingerprint 未变时提交 `latest.quote_id`；变化时只刷新预览并显示再次确认提示，confirm 请求数为 0。受控失败对同一 quote/fingerprint 复用 key，重新报价后第二次显式确认轮换 key，成功后清除该 source 的 attempt。
- GREEN 命令：`cd web/default && bun test src/features/subscription-conversion/components/timed-subscription-conversion-quotes-card.test.tsx`；结果 `18 pass / 0 fail`。
- RED 提交：`e984c1eb7 test(subscription): 固化前端报价身份失败合同`、`e10d4bbd8 test(subscription): 拒绝自动确认刷新报价`；GREEN 提交：`27c3552cb fix(subscription): 提交服务端转换报价身份`。

## Finding B：稳定错误 code 与六语言

- typed adapter RED：`cd web/default && bun test src/features/subscription-conversion/api.test.ts` 为 `0 pass / 1 fail`，模块缺少 `SubscriptionConversionRequestError`；既有 adapter 把 `{success:false, code, message}` 直接包装为普通 `Error(message)`。
- typed adapter GREEN：建立 feature 内最小 `SubscriptionConversionRequestError` 与成功/错误响应 union；adapter 保留 `code` 且不把 `message` 作为错误合同。提交 `979c43af2 fix(subscription): 保留转换 API 稳定错误码`。
- 稳定映射 RED：`bun test src/features/subscription-conversion/errors.test.ts` 为 `1 pass / 1 fail`；`subscription_conversion_ineligible` 实际落入通用 fallback。组件 stale RED 为 `18 pass / 1 fail`；stale code 未映射明确再次确认文案。
- 根因：缺少集中稳定 code→i18n key 映射；全局 axios interceptor 在 conversion 请求未设置 `skipBusinessError` 时仍会 toast 服务端自由文本。
- GREEN：集中穷举 conversion stale/ineligible/idempotency conflict 与 `credit_fx_rate_missing`、`credit_fx_rate_empty`、`credit_fx_invalid_decimal`、`credit_fx_precision_exceeded`、`credit_fx_non_positive`、`credit_fx_direction_mismatch`、`credit_fx_unsupported_currency`、`credit_fx_overflow`；unknown/untyped 只用本地化通用 fallback。quote/confirm 均显式 `skipBusinessError: true` 并由 API 行为测试断言。stale 只刷新预览，不自动发第二次 confirm。
- GREEN 命令：`bun test src/features/subscription-conversion/api.test.ts src/features/subscription-conversion/errors.test.ts src/features/subscription-conversion/components/timed-subscription-conversion-quotes-card.test.tsx`；结果 `23 pass / 0 fail`。
- 六语言：新增 5 个合并后的可见消息键并分别翻译 en/zh/fr/ja/ru/vi；`bun run i18n:sync` 成功，六语言 `missingCount=0`、`extrasCount=0`。既有全库 untranslated 计数未在本 Dispatch 清理。
- stable-code/i18n GREEN 提交：`c8aaf557f fix(subscription): 本地化转换稳定错误码`。

## Finding C：轮询 quote 写放大

- RED 命令：`go test ./model -run '^TestListTimedSubscriptionConversionQuotesReusesActiveQuoteForUnchangedFacts$' -count=1 -timeout=120s`。
- RED 结果：真实 SQLite 连续两次调用 `ListTimedSubscriptionConversionQuotes`，相同 user/source/facts 得到不同 UUID，持久 quote 记录数期望 1、实际 2。
- 根因：旧 `issueTimedSubscriptionConversionQuoteTx` 每次轮询无条件 `Create`；候选订阅无上限；source plan、target plan、Credit debt/valuation/FX 在每项重复查询；无有界过期清理。
- RED 提交：`3a2d081b8 test(subscription): 固化报价轮询写放大`。
- GREEN 设计：报价 facts 仍保存完整权威 fingerprint；同币种的数据库时间型 `fx_captured_at` 只从复用 identity key 中规范化排除，跨币种冻结 FX snapshot 仍参与 key。持久化 nullable 唯一 `reuse_key`，`INSERT ... ON CONFLICT DO NOTHING`/方言等价语义使并发同事实只保留一个有效 identity；复用时重新验证 owner/source、完整 snapshot fingerprint 与 reuse key。
- GREEN 行为：相同事实且未过期返回相同 `quote_id`/fingerprint/created/expires，记录数保持 1；token usage 等权威事实变化产生新 identity，旧 identity Confirm 返回 `ErrConversionQuoteStale` 且 conversion 记录数 0；过期 identity 被替换，新 identity 可见，旧 identity stale。
- 查询边界：候选订阅 `id asc LIMIT 100`；source plans 通过 `IN` 批量加载；Credit plan、target mapping/debt、valuation-ready 各在列表快照中只读一次；conversion history 保持 `id desc LIMIT 100`。
- 清理边界：每次按 `(user_id, expires_at, quote_id)` 稳定选择并删除至多 32 条已过期 identity；不删除未过期报价。并发 Confirm 仍按主键锁 quote，MySQL/PostgreSQL 删除等待锁的语义留给 #27 实机验证。
- GREEN 提交：`913c5c930 fix(subscription): 复用有效转换报价`。

## Finding D：报价 facts 与批量持久化原子性

- RED 提交：`a8aee625f test(subscription): 固化报价事实原子性`。
- RED 1：在旧读事务结束、identity 写入开始前把 source `token_used` 从 25 改为 26；接口仍返回/持久化 75 remaining 与旧 source facts，证明报价生成即 stale。
- RED 2：两条报价在第二条持久化前注入错误；旧实现返回失败但数据库已遗留第一条 quote，证明批量部分写。
- GREEN：`c4e89aabf fix(subscription): 原子持久化转换报价事实`。候选读取、facts 计算、测试接缝触发后的同事务重读、所有 quote identity 写入、过期清理和 conversion history 读取同属一个 GORM 事务；失败统一回滚。
- SQLite 并发：只识别实际 `github.com/glebarez/go-sqlite.Error` 的 `SQLITE_BUSY/SQLITE_LOCKED` 基础码，在完整事务边界最多 8 次有界重试；不解析错误文本，不吞掉业务/约束错误，不为 MySQL/PostgreSQL 添加方言重试。
- GREEN 结果：事实变化测试返回并持久化 `source_token_used=26`、`current_remaining_credit=74`；第二条注入失败后该 user quote count 为 0。
- 语义边界：提交后外部事实继续允许变化；Confirm 对变化返回 stale 是正确合同。本修复保证的是提交时返回计算与持久化 facts 一致，以及批量全成或全回滚，不扩展全局 Option/FX/计划锁图。

## 最终验证

- `go test ./model -run '^TestListTimedSubscriptionConversionQuotes' -count=10 -timeout=600s`：通过，包含事实变化与批量回滚用例。
- `go test ./model -run '^(TestListTimedSubscriptionConversionQuotes|TestRecalculateTimedSubscriptionConversionQuote|TestTimedSubscriptionConversionQuote|TestConfirmTimedSubscriptionConversion)' -count=10 -timeout=600s`：通过。
- `go test -race ./model -run '^TestListTimedSubscriptionConversionQuotesConcurrentSameFactsWritesOnce$' -count=1 -timeout=300s`：通过。
- `go test ./controller ./router -run 'SubscriptionConversion|TimedConversion' -count=1 -timeout=300s`：两个包通过。
- `bun test src/features/subscription-conversion/api.test.ts src/features/subscription-conversion/errors.test.ts src/features/subscription-conversion/components/timed-subscription-conversion-quotes-card.test.tsx`：23 pass / 0 fail。
- `bun run typecheck && bun run i18n:sync && bun run build`：通过；i18n 报告生成，Rsbuild production build 成功。
- `go test ./model ./service ./controller -count=1 -timeout=1200s`：model、service 通过；controller 仅 `TestCreditBalanceGrantRejectsReplayWithMismatchedIdentity` 失败，其旧断言要求自由文本 `idempotency key mismatch`，实际稳定 code 为 `credit_valuation_idempotency_mismatch`。未将该组合命令误报为通过；父集成需校准旧断言并重跑。
- `git diff --check`：通过；最新代码提交为 `c4e89aabf`。

## 验证边界与交接

- 本 Dispatch 使用真实 SQLite；MySQL 5.7/PostgreSQL 9.6 未运行，三库零 SKIP 归 Issue #27。
- 未修改 #24 管理员正向入账、#25 destructive recovery、#27 migration/ready 或 #28 release。
- 安全文档与实现链：`8b390a7fb`、`e984c1eb7`、`e10d4bbd8`、`27c3552cb`、`979c43af2`、`c8aaf557f`、`3a2d081b8`、`913c5c930`、`a8aee625f`、`c4e89aabf`。
- 父协调器下一步：集成本分支，校准一个既有 controller 文本断言，执行组合门禁；不得把 MySQL/PostgreSQL 标记为已实测。
