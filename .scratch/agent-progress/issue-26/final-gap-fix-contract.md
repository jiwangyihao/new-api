# Issue #26 最终缺口修复合同

## 冻结基线

- 日期：2026-08-08。
- 子工作树：`C:/Users/34404/source/repos/new-api/.workspaces/new-api/issue-26-final-gap-fix`。
- 分支：`jiwangyihao/issue-26-final-gap-fix`。
- 起始 HEAD：`c8a46e7c5de5cdfa6f94a41723d1396b57c7f2cd`。
- 集成父工作树：`C:/Users/34404/source/repos/new-api/.workspaces/new-api/credit-operational-value-integration`。
- Orca `baseRef`：`jiwangyihao/credit-operational-value-integration`；显式 parent lineage 已确认。
- 起始 HEAD、父集成 clean HEAD 和 merge-base 均为 `c8a46e7c5de5cdfa6f94a41723d1396b57c7f2cd`；起始 staged、unstaged、untracked 均为 0。

## 只修三项真实缺口

### A. 前端携带服务端 quote identity

- `SubscriptionConversionQuote` 精确保留服务端 `quote_id`、`created_at`、`expires_at`、`facts_fingerprint` 字符串。
- `SubscriptionConversionConfirmRequest` 必须携带 `quote_id`。
- `handleConfirm` 先执行既有最终刷新，再提交该次刷新返回的 `latest.quote_id`、`subscription_id` 与稳定 `idempotency_key`；客户端不得生成 quote identity。
- 同一 quote 与同一业务事实的受控失败重试复用同一 key；成功、重新报价或事实变化继续服从既有 key 生命周期。不得把旧 quote 与新 key 静默配对。
- stale 不自动替用户确认新事实；刷新后要求用户查看新报价并再次确认。

### B. 稳定错误 code 与六语言本地化

- API adapter 保留服务端稳定 `code`，不把响应自由文本降级成普通 `Error` 合同。
- conversion card 穷举映射 `subscription_conversion_quote_stale`、`subscription_conversion_ineligible`、`subscription_conversion_idempotency_conflict` 与既有 FX invalid/unsupported/overflow code。
- 未知 code 使用本地化通用 fallback；不得解析或直接展示服务端 `message`。
- stale 文案明确“报价已过期或权威事实变化，请查看刷新后的报价并再次确认”。
- 文案同步 `en`、`zh`、`fr`、`ru`、`ja`、`vi`，`bun run i18n:sync` 的 missing/extras 必须均为 0。

### C. 消除 5 秒轮询的无界 quote 写放大

- 相同 `user_id + source_subscription_id + authoritative facts_fingerprint` 且报价仍有效时，报价接口复用服务端已有有效 quote，不新增 identity 或记录。
- 事实变化或过期必须产生新服务端 identity；旧 identity 的 Confirm 继续返回 `subscription_conversion_quote_stale` 且零业务写入。
- quote identity 继续不可伪造并绑定 owner/source；`created_at`、`expires_at` 继续由数据库权威持久化。
- 候选订阅查询必须有明确上限与稳定排序，保持“可转换权益”语义。
- 批量/共享可安全复用的 source plan、目标 Credit mapping、debt、FX 等事实，避免逐项明显重复查询；不建立进程内权威缓存。
- 过期/失效 quote 的清理或替换必须有上限、有索引支撑且不破坏并发 Confirm。
- 所有数据库逻辑保持 SQLite、MySQL 5.7.8+、PostgreSQL 9.6+ 兼容，优先 GORM。

## 不可改变的不变量

- 不绕过服务端 stale/fingerprint 校验，不删除 quote identity。
- 不回退 H1 request→target 锁序、M1 sentinel/code、M2 服务端 quote identity/stale、M3 committed unit value。
- 不改变转换数量公式、31 天业务月、FX 方向、单位价值、请求虚拟 snapshot、邀请隔离或 disabled-plan 消费边界。
- 不扩展 analytics 为第二套逐条 committed snapshot DTO。
- 不修改 `model/credit_balance_adjustment.go`、`model/redemption.go` 或 #24 UI/进度。
- 不实现 #25 destructive recovery、#27 migration/ready、#28 release；不改生产。

## RED→GREEN 接缝与命令

1. 前端 quote identity：在现有 conversion card 组件测试中观察最终刷新请求与 confirm payload；RED 必须证明 `quote_id` 缺失，GREEN 断言提交最后一次刷新返回的身份。
2. 前端稳定错误：在同一组件/API adapter 公共行为接缝中，RED 证明 `code` 丢失或文案回退为服务端自由文本；GREEN 覆盖已知 code、本地化 unknown fallback、stale 不自动确认。
3. 后端写放大：真实 SQLite 通过 `GetTimedConversionQuotes` 连续获取相同事实；RED 断言 quote 记录数增长，GREEN 断言 identity 复用且记录数不增长。随后逐步覆盖事实变化、过期、旧 identity stale 零写、owner/source 篡改和并发串行化。
4. 精确测试命令在完成代码定位后写入 evidence；不运行 project-wide 套件、全前端套件、formatter 或 linter。

## 提交纪律

- 安全文档：`8b390a7fb docs(issue-26): 固化最终缺口修复合同`。
- 前端 quote identity RED/GREEN：`e984c1eb7`、`e10d4bbd8`、`27c3552cb`。
- 前端 stable-code/i18n GREEN：`979c43af2`、`c8aaf557f`。
- 后端 quote reuse 与原子性 RED/GREEN：`3a2d081b8`、`913c5c930`、`a8aee625f`、`c4e89aabf`。
- 每个小步均使用 Conventional Commit，英文 `type(scope)`、简体中文 subject。

## 交接

- A/B/C 三项合同已经实现；本工作树只剩最终证据提交。
- 父协调器集成后需校准一个既有 controller 自由文本断言，并执行组合门禁。
- MySQL 5.7/PostgreSQL 9.6 实机矩阵仍归 Issue #27，不得由本 SQLite 证据替代。
