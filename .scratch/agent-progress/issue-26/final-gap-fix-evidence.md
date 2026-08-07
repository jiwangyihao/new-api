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
- RED 提交：`e984c1eb7 test(subscription): 固化前端报价身份失败合同`、`e10d4bbd8 test(subscription): 拒绝自动确认刷新报价`；GREEN 提交待创建。

## Finding B：稳定错误 code 与六语言

- RED 命令：待 Finding A GREEN 后按垂直切片记录。
- RED 结果：待运行。
- 根因：待 RED 后确认。
- GREEN：待实现。
- 提交：待创建。

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

- 安全文档提交：待创建。
- 下一动作：提交本安全点，定位并执行 Finding A 的前端 RED。
