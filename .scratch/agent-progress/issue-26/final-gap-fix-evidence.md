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
- stable-code/i18n RED/GREEN 提交：待创建。

## Finding C：轮询 quote 写放大

- RED 命令：待定位 `GetTimedConversionQuotes` 的真实 SQLite 行为接缝后记录。
- RED 结果：待运行；必须观察数据库记录增长，不使用源码文本断言。
- 根因：待 RED 后确认。
- GREEN：待实现。
- 提交：待创建。

## 预定验证边界

- 前端：受影响 component/API tests、`bun run typecheck`、`bun run i18n:sync`、`bun run build`；不运行全前端套件、formatter 或 linter。
- 后端：conversion model/controller/router 定向单次、关键 `-count=10`、必要窄 `-race`；不运行 project-wide 套件。
- 数据库：本 Dispatch 运行真实 SQLite；MySQL 5.7/PostgreSQL 9.6 只做静态兼容审查，实机矩阵留给 #27。
- 收尾：只格式化明确修改文件，执行 `git diff --check`，最终 staged/unstaged/untracked 全零。

## 当前提交与下一动作

- 安全文档：`8b390a7fb`；quote identity RED/GREEN：`e984c1eb7`、`e10d4bbd8`、`27c3552cb`；typed adapter：`979c43af2`。
- 下一动作：提交 stable-code/i18n GREEN，然后建立真实 SQLite quote 写放大 RED。
