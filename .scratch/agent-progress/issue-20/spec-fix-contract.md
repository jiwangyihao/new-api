# Issue #20 Spec H1/M1 收敛修复合同

## H1：精确零值与历史 NULL

- `price_amount_micros` 的 nullable 合同由“字段是否显式提供”决定，而不是由数值真假决定。
- 前向创建或更新显式提交 `price_amount_micros: "0"` 时：严格解析成功，持久化为非 NULL 的整数 0，对外 JSON 为十进制字符串 `"0"`。
- 同时显式提供 `price_amount: 0` 时兼容展示值为 0，且与精确 micros 相等。
- 完全未提供价格字段的历史/无关更新不得合成零；已有 `price_amount_micros IS NULL` 必须保持 NULL。
- 零不是迁移完成标志，也不得触发历史回填。
- 既有负数、非法格式、超精度、溢出、显示与精确值不一致拒绝合同保持不变。

## M1：真实 SQLite 只读诊断

M1 范围严格收敛为 SQLite 专属诊断：

1. 真实 SQLite NUMERIC/REAL 行的表面文本先由现有严格解析器转成 `int64` micros。
2. 仅 SQLite 将 micros 用整数运算重建为规范六位十进制字符串，并在数据库内用 `CAST(? AS NUMERIC)` 与原始 `price_amount` 严格比较。
3. Go 应用层不得扫描、格式化或比较 `float32/float64`，不得使用容差。
4. 表面文本可解析但严格往返不等时返回稳定常量 `roundtrip_mismatch`；既有 `invalid_decimal`、`negative`、`precision_exceeds_six`、`overflow` 保持不变。
5. 结果按 plan ID 稳定排序，重复执行确定；诊断前后完整夹具快照相同，证明零写入。
6. 非 SQLite 查询、错误和诊断语义保持原样；不新增 MySQL/PostgreSQL 合同或测试，真实跨库历史迁移属于 #27。

## 明确非所有权

- 不回填历史 `price_amount_micros`。
- 不创建或更新 migration marker。
- 不判断、返回、阻止或切换 `ready`。
- 不重建历史 Credit/计时估值，不启用数量/估值强制双写。
- 不实现 #21–#26 的购买、结算、兑换、转换、恢复、FX、移动平均或分析业务路径。

## 最终提交与收尾检查

- H1 实现提交：`cf2b743b84ac74977d654d63dab52ecd8bb0d9fb`。
- M1 实现提交：`c3b3f6848ad5cb3dca4bdce3385499f74875c208`。
- 三份 spec-fix 文档完成一致更新后统一提交；提交后 `git status --short` 必须无输出。
- 本合同确认：不实施 #27 历史回填、migration marker 写入、`ready` 裁决或 Credit 数量/估值强制双写。
