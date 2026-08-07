# Issue #26 最终复评续作合同

- 冻结 HEAD：`44009213cb8e4a582de34f884deecd5a8d687b2c`。
- Orca parent：`jiwangyihao/credit-operational-value-integration`；祖先 `b8598f4b7add27ba237f30dec6ceae7968cc2aa3` 与 H1 提交 `3feb091159aef26731c1698647791acc03c29c0a` 已确认。
- 当前 HEAD：`0f98f18ed`；M1/M3 已形成独立 GREEN 提交，提交后工作树 clean。
- 当前 phase：M2 quote identity/stale；最近安全提交 `0f98f18ed`。
- 当前未提交：仅本次三份 progress/evidence 校准。

## M1/M3 已交付

- `ErrConversionIneligible` 与 `ErrConversionQuoteStale` 已导出；资格拒绝以 `%w` 包装，controller 只以 `errors.Is` 映射稳定 code。
- Confirm/history 直接格式化 committed conversion unit-value numerator/denominator；响应层不再以 `math/big` 重算。
- RED `9ffade1ac`；GREEN `0f98f18ed`。不得回头重做或扩展前端。

## M2 冻结合同

- Quote 返回服务端权威 `quote_id`、`created_at`、`expires_at` 与版本化 `facts_fingerprint`；精确值只用整数/字符串，不经过浮点。
- Fingerprint 覆盖 user/source entitlement、source Plan ID、价格 micros/currency、duration/reset、credit basis、remaining/gross/net Credit、source/target currency、FX numerator/denominator/captured_at、rule/version、目标 Credit mapping与确认资格事实。
- 服务端验证 quote identity、所有者、source、有效期及权威 fingerprint；Confirm 不信任客户端回传 fingerprint。
- Confirm 沿 H1 request-first 锁序，在同一事务锁定并重读权威事实；过期、篡改或任一事实漂移均包装 `ErrConversionQuoteStale`，API code 为 `subscription_conversion_quote_stale`，整笔零写入。
- 同一 quote、同一幂等键、相同事实重放返回 committed conversion；不同 quote 或事实冲突不得覆盖既有 conversion。
- 不新增隐式旧 API fallback；兼容范围仅由现有明确测试决定。

## 固定不变量与范围

- 数量公式保持 `full_31_day_blocks × credit_basis + current_remaining_credit`；31 天业务月，部分周期不按秒折算，已预扣量不重复计入。
- conversion 是 exact 运营规则值，不是新增收款或邀请归因；后续 Plan/Option/FX 变化不重写 committed 事实。
- 不修改 #24 redemption/admin increase ingress/UI/幂等；不实现 #25、#27、#28。
- SQLite 是本任务行为证据；MySQL 5.7/PostgreSQL 9.6 实机零 SKIP 保留给 #27。

## 下一动作

- 立即提交本 progress 校准。
- 只写真实 SQLite API 的 M2 RED：quote identity/created/expires/fingerprint 存在；quote 后权威事实漂移再 confirm 必须 stale 且零写。
- 独立提交 RED 后做最小 GREEN；不做前端扩展或额外阅读。
