# Issue #26 最终复评续作合同

- 冻结 HEAD：`44009213cb8e4a582de34f884deecd5a8d687b2c`。
- Orca parent：`jiwangyihao/credit-operational-value-integration`；merge-base/祖先 `b8598f4b7add27ba237f30dec6ceae7968cc2aa3` 已确认。
- H1 固定锁序提交 `3feb091159aef26731c1698647791acc03c29c0a` 保持在祖先链；当前树同时保留 #24 H2 跨币种 ingress 与路由夹具校准，不覆盖、不回退。
- 当前 phase：M1/M3（稳定领域错误、稳定 machine code、committed unit value），随后为 M2（quote identity/stale）。
- 最近安全提交：`44009213cb8e4a582de34f884deecd5a8d687b2c`。
- 创建前未提交文件：无；本提交只新增本目录下三份 `final-review-continuation-*` 恢复文档。

## M1/M3 合同

- 导出 `ErrConversionIneligible` 与 `ErrConversionQuoteStale`，所有对应分支以 `%w` 包装，Go 调用者可用 `errors.Is` 判定。
- Controller/router 仅按 sentinel 映射稳定 machine code；不得解析自由文本、前缀或 message。保留既有 idempotency 与 FX sentinel/code。
- Confirm/history/analytics 的未舍入单位价值只读已提交 `SubscriptionConversion.ValuationUnitValueNumeratorMicros` 与 `ValuationUnitValueDenominator`；响应层不得以 `math/big`、当前 Plan/Option 或兼容 float 重算。
- 结构化字段非法时 fail closed；不得用重算掩盖持久化异常。
- 现有 conversion card 只按结构化 `code`/`reason_codes` 分支，未知 code 使用本地化 fallback；如需改可见 UI，先加载 shadcn-ui 与 i18n-translate，并维护 en/zh/fr/ru/ja/vi。

## M2 合同

- Quote 返回服务端权威 `quote_id`、`created_at`、`expires_at` 与版本化 `facts_fingerprint`；所有精确值使用整数/字符串，不经浮点。
- Fingerprint 至少覆盖 user/source entitlement、source Plan ID、权威价格 micros/currency、duration/reset、credit basis、remaining/gross/net Credit、source/target currency、FX numerator/denominator/captured_at、rule/version、目标 Credit mapping及资格事实。
- 服务端持久化或等价不可伪造方式验证 quote identity；Confirm 不信任客户端提交的 fingerprint。
- Confirm 沿既有 H1 request-first 锁序，在同一事务锁定并重读权威事实；过期、身份篡改或任一事实漂移统一包装 `ErrConversionQuoteStale`，稳定 code 为 `subscription_conversion_quote_stale`，且零写入。
- 同一 quote、同一幂等键、相同事实重放返回同一 committed conversion；不同 quote 或相同 identity 的事实冲突不得覆盖既有 conversion。

## 固定不变量与范围

- 数量公式保持 `full_31_day_blocks × credit_basis + current_remaining_credit`；31 天为业务月，部分周期不按秒折算，已预扣量不重复计入。
- conversion 是 exact 运营规则值，不是新增收款，不产生邀请归因；后续 Plan/Option/FX 变化不重写 committed 事实。
- 不修改或复制 #24 redemption/admin increase ingress/UI/幂等合同；不实现 #25 destructive recovery、#27 migration/ready、#28 release。
- 保留 #20–#24 精确价格、timed grants、Credit moving-weighted state、request-aware settlement、current_only warning、BigInt/micros sorter 与 disabled-plan 消费边界。
- SQLite 是本任务行为证据；MySQL 5.7/PostgreSQL 9.6 实机零 SKIP 保留给 #27，不冒充已验证。

## RED/GREEN 与下一动作

- RED：尚未运行；下一命令将由 M1 最小行为测试确定，先证明 ineligible 无法 `errors.Is`、controller 仍依赖文本、API 未读取 committed unit-value 字段。
- GREEN：尚未运行。
- 阻塞：无。
- 下一动作：读取相关 model/controller/router/frontend 测试接缝，逐个完成 M1/M3 的 RED→GREEN tracer，再提交 clean 安全点。
