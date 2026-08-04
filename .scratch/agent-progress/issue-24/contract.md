# Issue #24 实现合同

## 主改责任

本切片独占以下区域：

1. Credit 兑换成功事务中的不可变充值档位来源快照、来源身份、幂等与 ingress 调用。
2. 管理员 Credit `increase` 的 `plan_id`、reason、幂等指纹、档位资格、服务端精确预览与 ingress 调用。
3. `CreditBalanceLedger` 对兑换/售后来源的结构化 Credit、金额、币种、FX、规则版本、状态版本和来源终态。
4. 管理员 adjustment API、`AdminCreditBalancePanel`、六语言与真实 SQLite/API/browser tracer。

## 兑换来源身份

- `source_type = redemption`。
- `source_key` 使用兑换记录的稳定持久化身份，不使用进程内随机值。
- `source_id` 使用兑换记录主键，仅作追溯；幂等结果同时受来源唯一身份和 ledger 唯一约束保护。
- `idempotency_key` 由稳定兑换身份派生，重复兑换请求不得生成新的业务身份。
- Credit 估值来源只取兑换成功事务冻结的 `SubscriptionEntitlementSnapshot`：`plan_id`、`list_price_micros`、`list_price_currency`、`monthly_token_limit`、`valuation_rule_version`；禁止读取兼容 float、渠道实付或全局 Credit 容器价格。
- fulfillment snapshot、ledger 与估值 ingress 保存同一份不可变事实；套餐后续改价、改币种、改 Credit 不回写。

## 管理员 increase 请求与指纹

请求 payload：

```json
{
  "operation": "increase",
  "amount": 800,
  "plan_id": 123,
  "idempotency_key": "stable-retry-key",
  "reason": "售后补偿"
}
```

- `increase` 要求 `amount > 0`、`plan_id > 0`、非空 `reason`、非空稳定 `idempotency_key`。
- `decrease` 不得携带 `plan_id`；本切片只验证清除/拒绝泄漏，不实现其移动平均出账。
- 最终参数指纹包含：`user_id`、`operation`、`gross_credit`、`plan_id`、`source_price_micros`、`source_plan_credit`、`source_currency`、`valuation_currency`、FX numerator/denominator/captured_at、`rule_version`、标准化 reason；operator ID 作为审计字段，是否纳入指纹遵循现有重试合同并由测试冻结。
- 相同 user/key 和完全相同指纹重放原结果；任一指纹字段变化返回 `credit_valuation_idempotency_mismatch`，不得再次改余额或状态。

## 档位资格

兑换与管理员 increase 在事务内锁定档位并验证：

- `entitlement_type = timed`；
- `enabled = true`；
- `is_trial = false`，且不是邀请来源档位；
- `price_amount_micros > 0`；
- `monthly_token_limit > 0`；
- `unlimited_purchase_enabled = true`。

`model_limits` 不参与资格或消费判断。已有 disabled-plan 权益仍可消费，但 disabled 档位拒绝新兑换/increase。

## Ingress 合同

- 调用方只构造 #22 的 `CreditValuationSourceSnapshot`，由 `newForwardCreditValuationIngress` 派生 exact；不得声明 confidence 或直接写 `token_limit/token_used` / `CreditValuationState`。
- 固定锁序：来源行/档位 → 目标 `UserSubscription` → `CreditValuationState` → ledger/source terminal state。
- `gross_cost = floor(source_price_micros × gross_credit / source_plan_credit)`。
- ingress 先抵扣 settlement debt；只有 `net_credit` 与 `floor(gross_cost × net_credit / gross_credit)` 进入 exact 状态。
- 同币种 FX 固定 `1/1`。CNY/USD 跨币种等待 #26 的唯一 `CreditFXRateSnapshot` seam；#24 不复制 parser/provider。

## 结构化 ledger 字段

至少保存：

- `source_type/source_key/source_id`、`idempotency_key`、`parameter_fingerprint`；
- `plan_id`、`gross_credit`、`net_credit`；
- `source_price_micros`、`source_plan_credit`；
- `valuation_gross_cost_micros`、`valuation_net_cost_micros`；
- `valuation_currency`、`fx_source_currency`；
- `fx_rate_numerator/denominator/captured_at`；
- `valuation_confidence = exact`、`valuation_rule_version`、`valuation_state_version_after`；
- 来源终态与完整 `source_snapshot`（JSON 仅补充，不能替代结构化关键列）。

ledger、Credit 数量、估值状态和兑换/管理员来源终态必须在同一事务提交；任何失败全部回滚。

## API 响应与稳定错误

increase/兑换结果至少返回或可从现有响应中观察：

- `gross_credit`、`net_credit`；
- `gross_amount_micros`、`net_amount_micros`（十进制字符串）；
- `valuation_currency`、`source_currency`；
- `confidence`、FX numerator/denominator/captured_at；
- `rule_version`、`state_version_after`、`replayed`。

稳定错误：`credit_valuation_plan_required`、`credit_valuation_plan_ineligible`、`credit_valuation_unsupported_currency`、`credit_valuation_invalid_fx`、`credit_valuation_overflow`、`credit_valuation_state_missing`、`credit_valuation_state_mismatch`、`credit_valuation_idempotency_mismatch`、`credit_valuation_migration_not_ready`。前端不得解析错误文本决定业务分支。

## 服务端权威预览

- UI 不以 JavaScript 浮点作为最终金额来源。
- 预览调用后端与提交共用的资格/快照/整数 ingress 计算接缝，不写任何状态。
- 精确 micros 以字符串返回；UI 只做 BigInt/字符串格式化。

## UI 幂等键生命周期

- 首次有效 increase 参数集合生成一个 key。
- 可控失败后的重试保留原 key。
- 成功后生成新 key。
- operation、plan、amount 或其他业务参数变化后生成新 key，并清除旧预览。
- 切换到 decrease 时立即清空 `plan_id` 与预览，请求 payload 不含 `plan_id`。
- 切回 increase 不恢复旧档位、旧预览或旧 key。

## 共享文件

预计涉及：`model/credit_balance.go`、`model/credit_balance_adjustment.go`、`model/redemption.go`、相关 model/controller tests、管理员 adjustment controller/router、`web/default/src/features/subscriptions/{types,api}.ts`、`admin-credit-balance-panel.tsx` 及测试、六个 locale。

若需扩展 `model/credit_valuation.go`，只允许为 #24 消费既有 ingress 的窄适配或真实缺陷修复，并先报告协调器；不得改 request-aware 分支。

## 明确非所有权

- 不实现 #23 的 request settlement、coalescer、funding/session/task 身份。
- 不实现 #25 的 decrease outflow、订单退款、拒付或财务恢复。
- 不实现 #26 的 timed→Credit conversion、转换期间虚拟快照或独立 FX provider/parser。
- 不实现 #27 的历史回填、migration marker/ready 切换或三数据库最终矩阵。
- 不实现 #28 的发布、备份、镜像或生产操作。
