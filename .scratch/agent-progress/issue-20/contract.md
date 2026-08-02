# Issue #20 消费合同

## 金额合同

- `1 currency unit = 1,000,000 micros`。
- `SubscriptionPlan.price_amount_micros` 是前向估值的权威 `int64` 持久化字段；JSON 请求/响应使用十进制字符串，避免 JSON number 精度损失。
- 现有 `price_amount` 仅用于兼容展示和既有支付路径；管理员写入由原始十进制输入严格解析 micros，再派生兼容值。
- 有价套餐拒绝缺失精确值、负数、超过六位小数、`int64` 溢出及精确/兼容字段不一致，并返回稳定错误码。
- 无价套餐的精确字段为字符串 `"0"`；历史新增列仍为待迁移值，不从旧浮点反推。

## Credit 估值币种合同

- 全局 `credit_balance` 套餐首次配置只接受 `CNY` 或 `USD`。
- 存在任一 Credit 权益、`CreditValuationState` 或估值账本后，普通套餐接口不得改写币种；拒绝与套餐写入处于同一原子事务并使用稳定错误码。

## 附加式 schema 合同

- `CreditValuationState`：每份 Credit 权益一行；subscription 主键、user 唯一。
- `CreditValuationMigration`：版本化 marker 结构；#20 只注册结构，不写状态、不切换 `ready`。
- `TimedSubscriptionValuationGrant`：不可变计时授予；幂等键和来源组合使用稳定命名唯一索引。
- 扩展现有请求预扣、低频 Credit 账本、转换与权益快照结构，为后续切片提供整数金额、请求扣除/恢复和 FX 快照字段。
- SQLite、MySQL >= 5.7.8、PostgreSQL >= 9.6 使用相同 GORM 语义；MySQL 不依赖 `CHECK`。

## 只读诊断合同

- 从数据库原始十进制/SQLite 数值表示严格判断历史 `price_amount` 能否精确转换 micros。
- 输出按稳定套餐 ID 排序的 `plan_id` 和稳定 reason。
- 不写 `price_amount_micros`、不修改 marker、不决定或阻止 `ready`。

## 整数比例合同

- 提供无浮点、无按请求 `big.Int` 分配的非负 `floor(a × b / d)`。
- 覆盖普通值、余数向下取整、完全清空吸收余数、分母为零、中间乘积超过 64 位但结果有效、结果溢出。

## UI 往返合同

- 管理员表单保留用户原始十进制文本并据此生成 micros 字符串；不得从 JavaScript `Number` 反推。
- 创建、编辑、刷新同时保留旧展示和精确字段；既有购买、disabled plan、模型范围忽略行为不变。

## 下游消费接口

- #21：消费计时 grant 模型和精确套餐标价字段；不在 #20 写 grant。
- #22：消费 Credit 状态模型、币种合同、整数比例和低频快照字段；不在 #20 启用数量/估值双写。
- #27：独占历史价格回填、估值重建、marker 状态变化、非法历史行阻止 `ready` 和强制双写切换。

## #20 明确非所有权

不回填历史套餐价格；不重建历史 Credit/计时估值；不修改 migration marker 状态；不自行阻止 `ready`；不启用 Credit 数量/估值强制双写。
