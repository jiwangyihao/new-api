# Issue #20 Standards 评审修复合同

## Finding 1：精确价格权威性

- `price_amount_micros` 的 nullable 公开 DTO 合同继续保留。
- 历史 `NULL` 表示“没有权威精确文本”，不能从兼容 `price_amount` 的 JavaScript `number`、格式化文本或容差比较反推。
- 表单必须显式区分：存在权威精确值、仅供显示的历史兼容值、用户已明确修改价格。
- 只有用户明确输入原始十进制文本，或创建新的有价套餐，才生成并提交 micros；历史套餐的非价格编辑必须让精确列继续为 NULL。
- `0`、大整数边界及易受二进制浮点影响的十进制值遵循同一规则，不设置 Number 反推旁路。

## Finding 2：schema 启动合同

- 先证明旧兼容列 `price_amount` 是否需要扩宽；权威 `BIGINT price_amount_micros` 已承载前向精确范围时，优先不迁移旧兼容列，消除非必要启动风险。
- 若仍有不可删除的关键 schema 变化，其错误必须返回并由 `migrateDB` fail-closed；不得只记录 warning 后继续。
- 历史 `price_amount_micros` 始终保持 NULL；不做回填，不切换 marker。

## Finding 3：共享计划级线性化接缝

- `AcquireCreditBalancePlanGuardTx(tx)` 是 allocation 与 valuation_currency 更新唯一共享线性化接缝：MySQL/PostgreSQL 锁定唯一全局 Credit plan 行，SQLite 通过同一行的事务写取得单写 guard。
- `GuardCreditValuationCurrencyUpdateTx(tx, currency)` 在计划 guard 内判断权益、估值状态与 ledger；controller 不再拥有私有冻结规则。
- `GrantCreditBalanceTx` 的共享锁序固定为全局 Credit plan → 用户 → Credit 权益/ledger；它必须先获取当前全局计划 guard，并按 `TargetPlanId` 校验权威计划身份。
- 新购买、兑换、转换和管理员授予读取当前计划状态，停用计划拒绝新的 allocation。已经由不可变订单快照授权、等待支付回调的订单仍按其 `TargetPlanSnapshot` 履约；快照不能跳过计划 guard 或改变全局计划身份。
- 合法串行结果仅为：币种先提交，后续首个 grant 在 guard 后使用相应授权事实；或首个 grant 先提交，随后币种更新返回 `ErrCreditValuationCurrencyLocked`。幂等重放与已有权益消费保持可用。

## 明确非所有权

不回填历史价格；不重建历史 Credit/计时估值；不修改 migration marker 状态；不决定或阻止 `ready`；不启用 Credit 数量/估值强制双写；不实现 #21–#28 的购买、结算、转换、恢复、FX 或移动平均估值功能。
