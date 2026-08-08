# Issue #26 最终缺口修复状态

- 日期：2026-08-08。
- 当前 phase：`FINAL_GAP_FIX_COMPLETE`。
- 起始 HEAD：`c8a46e7c5de5cdfa6f94a41723d1396b57c7f2cd`；当前代码 HEAD：`c4e89aabf`。
- 分支：`jiwangyihao/issue-26-final-gap-fix`。
- 父工作树：`credit-operational-value-integration`；Orca 显式 parent lineage、`baseRef` 与共同起始 HEAD 已确认。
- 已完成 Finding A：前端最终刷新后携服务端 `quote_id`；identity/fingerprint 变化时停止自动确认并要求用户再次确认。
- 已完成 Finding B：转换 API 保留稳定 `code`，卡片穷举 stale/ineligible/idempotency/FX code，未知 code 使用本地化 fallback；en、zh、fr、ru、ja、vi 同步。
- 已完成 Finding C：相同权威事实复用有效报价；事实变化或过期产生新 identity；旧 identity stale 且零 conversion 写；候选按 `id asc` 限 100；source plan、Credit plan/debt/target/valuation-ready 批量或单次读取；每次清理至多 32 条过期报价；数据库唯一 `reuse_key` 串行化并发同事实发放。
- 已完成原子性补强：报价计算、同事务事实重读、所有 identity 持久化、过期清理与 history 读取位于同一事务；第 N 条持久化失败会回滚前 N-1 条；SQLite `BUSY/LOCKED` 只在整个事务边界有界重试，其他错误不重试。
- 原子性 RED：`a8aee625f test(subscription): 固化报价事实原子性`，证明旧实现读写间 source 变化后仍持久化旧 facts，且第二条失败时遗留第一条 quote。
- GREEN 代码提交：`913c5c930 fix(subscription): 复用有效转换报价`、`c4e89aabf fix(subscription): 原子持久化转换报价事实`。
- GREEN：报价列表定向 `-count=10`、转换报价/确认定向 `-count=10`、并发报价窄 `-race`、controller/router conversion 定向、前端 23 个定向测试、typecheck、i18n sync、生产 build 与 `git diff --check` 均通过。
- 包级门禁：`go test ./model ./service ./controller -count=1` 中 model、service 通过；controller 只有既有 `TestCreditBalanceGrantRejectsReplayWithMismatchedIdentity` 仍断言自由文本 `idempotency key mismatch`，实际为稳定 code `credit_valuation_idempotency_mismatch`。该测试不属于本 Finding 的转换路径，留给父集成校准后重跑，未把部分失败写成全绿。
- 数据库边界：本轮真实 SQLite；MySQL 5.7/PostgreSQL 9.6 未实测，归 Issue #27。
- 当前阻塞：无实现阻塞；父协调器需集成后校准上述旧 controller 断言并执行组合门禁。
- 最近安全提交：`c4e89aabf fix(subscription): 原子持久化转换报价事实`。
- 下一动作：提交本最终证据，保持工作树 clean，交由父协调器集成。
