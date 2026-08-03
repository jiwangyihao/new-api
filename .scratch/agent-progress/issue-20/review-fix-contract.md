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

- 最终接口与锁顺序待入口清单和 RED 测试确认。
- 普通 Credit 套餐更新与所有现存 Credit allocation/grant 路径必须在各自数据库事务中经过同一个 model/service 深模块 guard。
- guard 必须重新读取并锁定权威全局 Credit plan，不信任未锁的 `TargetPlanSnapshot`。
- 合法串行结果仅为：币种先提交、首个权益随后使用新币种；或权益先提交、普通币种修改稳定拒绝。
- disabled-plan 既有消费与新 grant 拒绝语义不变。

## 明确非所有权

不回填历史价格；不重建历史 Credit/计时估值；不修改 migration marker 状态；不决定或阻止 `ready`；不启用 Credit 数量/估值强制双写；不实现 #21–#28 的购买、结算、转换、恢复、FX 或移动平均估值功能。
