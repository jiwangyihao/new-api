# Issue #22 实现合同

## 主改责任
本切片主改：
- `CreditValuation` 深模块：每份 Credit 权益唯一 `CreditValuationState`，与 `token_limit/token_used` 同事务写入。
- Credit 前向来源快照及人民币余额/受控外部支付完成入口。
- 冻结 tracer 所需最小同步 `request_id` 预扣与一次最终结算。
- 通用 analytics micros DTO、exact/estimated/unknown、状态版本、更新时间、nullable `time_based_value`、`snapshot_semantics`。
- 五个 paid-value API 的 Credit 分流、筛选/排序和混合池来源。
- 默认前端精确金额格式化、BigInt、置信度/current-only 与最小 Credit 展示骨架；可见文案覆盖六语言。

## 深模块接口与锁序
调用方只传结构化来源事实或目标累计用量；模块派生 confidence、比例/舍入、债务抵扣和 state_version。锁序：已锁定目标 `UserSubscription` → `CreditValuationState` → 本 tracer 请求记录/ledger；模块不提交事务、不清缓存、不发业务日志。#22 仅接受来源币种已等于池币种的同币种事实，不定义 FX 接口。

## 状态不变量
`available_credit=max(token_limit-token_used,0)`；成本非负；`unknown_credit<=available_credit`；币种为全局 Credit 估值币种；`state_version` 单调递增且幂等重放不递增；清空池吸收全部微单位余数；超量只形成 debt。

## 来源快照
订单创建冻结充值档位的 `price_amount_micros`、档位 Credit、币种、规则版本、目标池估值币种和来源身份。marker ready 时完成/回调只消费快照，不读当前套餐价格或渠道实收金额补猜；marker 非 ready 时保留既有数量写路径。重复回调不重复入账。

## 请求 seam
只实现足额同步预扣 200 与目标累计 200 的一次最终结算；同一 `request_id`/目标重放幂等。禁止实现 #23 的通用追加、少结算、退款、异步任务与 coalescer。

## API/UI
金额字段 `amount_micros` 为十进制字符串；兼容 float 仅最后派生。Credit 行 `token=recognized=exact+estimated`、`time_based_value=null`、basis=`credit_moving_weighted_average`，source=`credit_balance_pool/moving_weighted_pool`。当前状态早于 `snapshot_at` 时返回 `current_only` warning；summary/users/subscriptions/plans/sources 共享同一事实。

## 共享文件与边界
预计共享文件：`model/subscription.go`、`model/admin_analytics_paid_subscription.go`、`dto/admin_analytics.go`、`controller/admin_analytics.go`、默认前端 analytics 类型/格式化与 locale。#22 主改通用/Credit 区域；为 #21 timed 分支保留窄 seam，不重写其实现。

## 明确非所有权
不实现 #23–#28 的追加/少结算/退款/异步任务/coalescer、转换/售后、恢复、跨币种 FX、有理数汇率、历史迁移/marker 生命周期、三数据库矩阵和发布切换。#22 对 marker 仅只读。

## 五接口安全点收敛
协调器要求本安全点只完成现有 summary/users/subscriptions/plans/sources 的真实 SQLite tracer：冻结 `40 CNY / 1,000 Credit`，消费 200 后五个视图一致返回 `32,000,000` micros CNY、活动数 1、estimated 0、unknown 0。此安全点不重构查询签名、不扩展 warning 设计；通过后立即提交，再进入默认前端与六语言。FX 和 migration marker 生命周期仍明确排除。
