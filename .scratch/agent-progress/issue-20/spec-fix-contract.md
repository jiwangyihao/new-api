# Issue #20 Spec H1/M1 收敛修复合同

## H1：精确零值与历史 NULL

- `price_amount_micros` 的 nullable 合同由“字段是否显式提供”决定，而不是由数值真假决定。
- 前向创建或更新显式提交 `price_amount_micros: "0"` 时：严格解析成功，持久化为非 NULL 的整数 0，对外 JSON 为十进制字符串 `"0"`。
- 同时显式提供 `price_amount: 0` 时兼容展示值为 0，且与精确 micros 相等。
- 完全未提供价格字段的历史/无关更新不得合成零；已有 `price_amount_micros IS NULL` 必须保持 NULL。
- 零不是迁移完成标志，也不得触发历史回填。
- 既有负数、非法格式、超精度、溢出、显示与精确值不一致拒绝合同保持不变。

## M1：真实 SQLite 只读诊断

H1 提交前不推进 M1。之后范围严格收敛为：

1. 使用真实 SQLite NUMERIC/REAL 数据构造表面十进制文本可解析、但原始数值与规范 micros 重建值严格不等的 `roundtrip_mismatch`。
2. Go 应用层不得扫描或格式化 `float32/float64`，不得使用容差比较。
3. reason 使用稳定常量 `roundtrip_mismatch`，既有 `invalid_decimal`、`negative`、`precision_exceeds_six`、`overflow` 保持不变。
4. 结果按 plan ID 稳定排序，重复执行确定。
5. 诊断前后数据库相关内容快照完全相同，证明零写入。

不再扩大 SQLite/跨方言探索；真实 MySQL/PostgreSQL 矩阵不属于本次收敛执行。

## 明确非所有权

- 不回填历史 `price_amount_micros`。
- 不创建或更新 migration marker。
- 不判断、返回、阻止或切换 `ready`。
- 不重建历史 Credit/计时估值，不启用数量/估值强制双写。
- 不实现 #21–#26 的购买、结算、兑换、转换、恢复、FX、移动平均或分析业务路径。

## 恢复锚点

- 最近安全 HEAD：`79982d773d127779c9c3835c2e1c771b7a829268`。
- 当前未提交文件：三份 `spec-fix-*.md`；H1 测试/实现加入后由状态文件持续列出。
