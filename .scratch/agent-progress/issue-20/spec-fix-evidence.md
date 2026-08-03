# Issue #20 Spec H1/M1 收敛修复证据

## 基线与边界

- 冻结候选：`79982d773d127779c9c3835c2e1c771b7a829268`；初始工作树 clean。
- 当前只推进 H1 RED→GREEN 与小步提交；H1 提交前不继续 M1。
- H1 完成后，M1 只覆盖真实 SQLite `roundtrip_mismatch` 与诊断前后数据库快照相同。

## H1 调查证据

### 可观察问题

`model/credit_valuation_money.go` 中，输入同时显式提供兼容展示 `"0"` 与精确 micros `"0"` 时，解析结果虽为零，但 `exactMicros == 0` 分支返回空 `SubscriptionPlanPrice{}`。因此公开 API 的显式精确零值在持久化时折叠为 `NULL`，无法与历史待迁移 `NULL` 区分。

### 根因

代码使用“数值是否非零”代替“字段是否提供”判断 nullable 语义。

### H1 反证合同

- model：显式 `"0"` 必须产生非 nil `AmountMicros` 且值为 0，兼容展示值为 0。
- create API：数据库值必须是非 NULL 0，响应/读取 JSON 必须是字符串 `"0"`。
- update API：显式 `"0"` 必须覆盖为非 NULL 0。
- 完全缺失价格字段的无关编辑必须继续保留历史 NULL。
- 既有拒绝路径与非零路径保持不变。

## M1 已知事实（冻结至 H1 提交后）

- 冻结诊断仅将 `price_amount` 转为文本并调用 `ParseDecimalAmountMicros`，没有数值域往返不一致 reason。
- 真实 SQLite NUMERIC/REAL 值可出现表面文本能解析、但原始数值不等于规范六位十进制重建值的情况。
- 后续仅以真实 SQLite 夹具验证 `roundtrip_mismatch`，并比较诊断前后数据库快照证明零写入；不继续扩大跨方言调查。

## H1 RED / GREEN 命令记录

### RED：model 显式零值

命令：`go test ./model -run TestNormalizeSubscriptionPlanPricePreservesExplicitZero -count=1`

关键输出：

```text
--- FAIL: TestNormalizeSubscriptionPlanPricePreservesExplicitZero
credit_valuation_money_test.go:53: Expected value not to be nil.
FAIL github.com/QuantumNous/new-api/model
```

### RED：controller/API 创建、持久化与读取

命令：`go test ./controller -run TestAdminCreateSubscriptionPlanPreservesExplicitZeroPriceMicros -count=1`

关键输出：

```text
--- FAIL: TestAdminCreateSubscriptionPlanPreservesExplicitZeroPriceMicros
subscription_exact_price_test.go:43: Should be true
FAIL github.com/QuantumNous/new-api/controller
```

失败断言是数据库 `sql.NullInt64.Valid == false`：显式 `"0"` 被持久化为 NULL，准确复现 H1。

### GREEN

- `go test ./model -run TestNormalizeSubscriptionPlanPricePreservesExplicitZero -count=1`：PASS。
- `go test ./controller -run TestAdminCreateSubscriptionPlanPreservesExplicitZeroPriceMicros -count=1`：PASS。
- `go test ./controller -run "TestAdmin(CreateSubscriptionPlanPreservesExplicitZeroPriceMicros|UpdateSubscriptionPlanPreservesExplicitZeroPriceMicros|UpdateSubscriptionPlanPreservesLegacyPriceWhenPriceFieldsAreAbsent|CreateSubscriptionPlanRejectsInvalidExactPricesAtomically|CreateSubscriptionPlanRoundTripsExactPriceMicros|UpdateSubscriptionPlanRoundTripsExactPriceMicros)$" -count=1`：PASS；覆盖创建零、更新零、历史 NULL、既有非零与拒绝路径。
- `go test ./model -run "Test(ParseDecimalAmountMicros|NormalizeSubscriptionPlanPrice)" -count=1`：PASS。

最小源头修复仅删除 `exactMicros == 0` 的“未提供”旁路；字段存在性仍由 `AmountMicrosProvided` 决定，未引入默认零或历史写入。

## 最近安全 HEAD 与未提交文件

- 最近安全 HEAD：`79982d773d127779c9c3835c2e1c771b7a829268`。
- 当前未提交：三份 `spec-fix-*.md`、`model/credit_valuation_money.go`、`model/credit_valuation_money_test.go`、`controller/subscription_exact_price_test.go`。
