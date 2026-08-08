# Issue #25 实现契约

## 所有权

本切片只实现管理员 Credit decrease、Credit 订单 refund、chargeback、financial recovery、低频破坏性 outflow、来源终态、邀请取消和相应 API / UI / i18n。消费 #23 的请求级冻结快照和 #24 的 adjustment / ingress / ledger 合同，不复制它们的算法。

明确不实现 #26 的 timed→Credit conversion、运行时 FX、转换期间虚拟快照；不实现 #27 的历史迁移、ready 门禁、三数据库矩阵；不实现 #28 的发布、备份或回滚；不为 timed refund 新增服务撤销或 grant reversal。

## Outflow 输入

调用方只提供结构化来源事实与目标回收数量：

- 目标 `UserSubscription` / 用户身份；
- `source_type`、`source_id`、稳定 `source_key`；
- `operation`：admin decrease / refund / chargeback / financial recovery；
- 正数 `gross_credit`；
- 目标来源终态；
- 规则版本与稳定参数指纹；
- 审计原因及必要订单事实，但不得提供或决定池成本。

管理员 decrease 必须有正 `amount`、非空 `reason`、稳定 `idempotency_key`，且携带任何 `plan_id` 都以稳定 code 原子拒绝。

## Outflow 结果与算术

操作前：`A=max(token_limit-token_used,0)`，请求量 `Q`，实际消耗可用量 `C=min(Q,A)`。

- 对 exact、estimated、unknown 分别移除 `floor(component × C / A)`。
- `C=A` 时直接移除组件全部余数。
- `Q-A` 仅形成 `settlement_debt`。
- 零可用量不移除成本；所有成本结果必须保持非负。
- 禁止按订单原价、实付、退款额、充值档位或来源批次撤值。
- 返回 gross Credit、consumed available、debt formed、removed exact / estimated / unknown、currency、rule version、`state_version_after` 和来源终态；精确 micros 通过字符串进入 API。

## 事务与锁序

来源行可以先锁；进入 CreditValuation 深模块后的固定顺序为：

1. 目标 `UserSubscription`；
2. `CreditValuationState`；
3. 必要的请求记录或低频 ledger 结果。

同一数据库事务必须提交权益数量、估值状态、结构化 `CreditBalanceLedger`、订单/恢复来源终态和邀请奖励取消。任一稳定故障注入失败时全部回滚。低频 outflow 不读取、重算或改写活动 `SubscriptionPreConsumeRecord` 的 exact / estimated / unknown 扣除快照；与 request settle / refund 并发只允许固定锁序产生的合法串行结果。

## 来源身份、终态优先级与幂等

- 重放身份由稳定来源类型、来源 ID / key、operation 和参数指纹持久化确定，不使用内存标志。
- 指纹至少覆盖目标权益、gross Credit、operation、目标终态、reason、规则版本及调用合同要求的来源事实。
- 同来源身份且完全同参数：返回既有提交结果，不改变余额、成本或状态版本。
- 同 key / 来源但 operation、数量、目标权益、终态、规则版本或指纹不同：稳定 idempotency / terminal conflict，整笔回滚。
- refund、chargeback、financial recovery 遵循仓库既有支付终态优先级；同一订单最多一次实际 Credit 回收。唯一冲突后只能读取并验证持久化结果，不能覆盖原事实。
- 已授权订单的 financial recovery 不因充值档位后来 disabled 而失效；该例外不得借机创建新权益。已有 disabled-plan 权益继续可消费。

## 结构化 Ledger

关键列至少包含：`source_type`、`source_id`、`source_key`、`operation`、`gross_credit`、`consumed_available_credit`、`settlement_debt_formed`、`removed_exact_cost_micros`、`removed_estimated_cost_micros`、`removed_unknown_credit`、`currency`、`rule_version`、`state_version_after`、`parameter_fingerprint`、`terminal_state`。JSON 只可补充审计上下文，不能承载唯一关键字段。

## 管理员 Payload 与 Key 生命周期

- decrease 切换时隐藏并清空 increase 的 plan、价格和 preview 状态；最终 payload 不出现 `plan_id`。
- 同业务参数的可控失败重试复用原 key。
- amount、operation、reason 或其他指纹参数变化后生成新 key。
- 成功后生成下一次操作的新 key。
- 前端根据稳定 error code 分支，不解析错误文本。

## 邀请合同

Credit recovery 不产生邀请收益、不进入邀请付费统计。若原 Credit 订单已有错误或历史奖励，取消动作与 outflow、来源终态同事务并保持幂等。计时订单现金退款若不缩短实际服务，不修改 timed grant 或运营剩余价值。

## 分析生命周期

提交后五个运营分析接口必须立即反映 available / exhausted / debt、exact / estimated / unknown 和新状态版本；零可用量或仅有 debt 的 Credit 明细仍可见，但不计 `active_paid_subscription_count`。

## 预计共享文件

预计窄改 `model/credit_valuation.go`、`model/subscription.go`、低频 ledger / order / recovery 相关模型、管理员 adjustment controller/service/router，以及 `web/default` 现有 adjustment UI 和六语言 locale。只做附加字段或窄 helper，不修改 #23 请求快照算法及 #26 转换 / FX 接缝。
