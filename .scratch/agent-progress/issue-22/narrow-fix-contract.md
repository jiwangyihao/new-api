# Issue #22 窄验收修复合同

## Finding A：权威 micros 比较器
- 输入：`AdminAnalyticsMoneyAmount.AmountMicros` / `AdminAnalyticsMoneyBreakdown.AmountMicros` 的十进制整数字符串；仅在规范化币种与查询币种一致时参与比较，币种不匹配按零 micros 处理。
- 权威性：比较器严格解析为固定宽度 `int64` 后比较；禁止先转换为 `float64`，禁止读取兼容 `amount` 回退。兼容 `amount` 只由 micros 最后派生供旧客户端展示。
- 错误：空值、非十进制整数、负值或 `int64` 溢出必须返回稳定 sentinel 并终止该 panel 构建；不得用解析错误文本分支，不得静默回退兼容 float。
- tie-breaker：相同 micros 时保持现有确定性：users 用 `user_id`，subscriptions 用 `subscription_id`，plans 用 `plan_id`，sources 用现有 `grant_reason`（再由 stable sort 保留确定顺序）；升降序继续沿用现有整体方向语义。
- 范围：只修复 users/subscriptions/plans/sources 的 `recognized_remaining_value`；summary 无列表排序，不扩张。

## Finding B：current-only panel warning
- 稳定形状：复用 `AdminAnalyticsAvailabilityWarning`，固定 `section=credit_valuation`、`reason=current_only`；`message` 为稳定非机器分支说明。
- 触发：任一返回事实行的 `snapshot_semantics=current_only` 时，summary/users/subscriptions/plans/sources 对应 `AdminAnalyticsPanelResponse.Warnings` 都包含该 warning。
- 去重与排序：同一 panel 多条 current-only 行仅生成一次；warning 顺序确定。没有 current-only 行时不返回该 warning。
- 传播：五个 panel 从完整未分页事实聚合 warning，避免分页或仅 subscriptions 明细导致提示丢失；明细继续保留最新 `valuation_state_version`、`valuation_updated_at` 与 `snapshot_semantics=current_only`。
- 非阻断：warning 不转成 error，不伪造历史快照。前端优先消费 panel warning，并保留旧响应的 subscription 明细推断作为兼容回退；不得重复显示提示。

## 严格非所有权
- 不修改人民币余额、Kyren、BillingSession/request_id、Credit 数量/估值深模块或 32 CNY tracer。
- 不实现 Issue #23 的 target 增减、退款、异步 identity、coalescer；不实现 #24/#25 正向入账或恢复；不实现 #26 FX；不实现 #27/#28 migration marker、ready、历史回填、发布或回滚。
