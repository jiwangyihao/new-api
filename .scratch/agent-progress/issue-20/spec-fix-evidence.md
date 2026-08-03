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

## M1 真实 SQLite RED / GREEN

### 真实夹具

测试表使用 `price_amount NUMERIC`，plan 6 通过 SQLite 自身执行 `CAST('40.12345600000001' AS REAL)` 写入真实数值。夹具断言：

- `CAST(price_amount AS TEXT)` 为可被严格 micros 解析的 `40.123456`；
- 原始 `price_amount` 与 `CAST('40.123456' AS NUMERIC)` 严格数值比较不相等。

因此这是 SQLite 原始数值到规范六位 micros 十进制的真实往返不一致，不是注入伪字符串。

### RED

命令：`go test ./model -run TestDiagnosePendingSubscriptionPlanPricesIsReadOnlyAndDeterministic -count=1`

关键输出：

```text
expected: plan 2 negative, plan 6 roundtrip_mismatch, plan 7 invalid_decimal, plan 9 precision_exceeds_six
actual:   plan 2 negative, plan 7 invalid_decimal, plan 9 precision_exceeds_six
FAIL github.com/QuantumNous/new-api/model
```

冻结实现稳定漏报 plan 6，准确复现 M1。测试已同时要求稳定 plan ID 排序、重复调用结果一致，以及包含 `total_changes()`、SQLite 存储类型/字面值、micros 的诊断前后完整快照相同。

### GREEN

- `go test ./model -run TestDiagnosePendingSubscriptionPlanPricesIsReadOnlyAndDeterministic -count=1`：PASS。
- `go test ./model -run TestDiagnosePendingSubscriptionPlanPricesIsReadOnlyAndDeterministic -count=10`：PASS，证明稳定排序与重复执行确定。
- SQLite 专属比较将严格解析所得 `int64` micros 用整数运算重建为规范六位十进制字符串，再由 SQLite `CAST(? AS NUMERIC)` 与原始 `price_amount` 严格比较；Go 未扫描、格式化或比较任何 `float32/float64`。
- 表面文本解析成功但严格比较不等时返回稳定常量 `SubscriptionPlanPriceDiagnosticRoundtripMismatch = "roundtrip_mismatch"`。
- 每次测试内部连续调用诊断两次，结果均严格等于 plan ID `2, 6, 7, 9` 的有序列表。
- 诊断前后快照严格相等；快照包含 SQLite `total_changes()` 以及全部套餐行的 ID、`typeof(price_amount)`、`quote(price_amount)`、`price_amount_micros`，证明零写入。
- 非 SQLite 查询、错误和诊断行为保持原样；本轮未新增跨库语义或测试，真实跨库历史迁移仍属于 #27。

### 最终窄回归

- `go test ./model -run 'Test(ParseDecimalAmountMicros|NormalizeSubscriptionPlanPrice|DiagnosePendingSubscriptionPlanPricesIsReadOnlyAndDeterministic)$' -count=1`：PASS。
- `go test ./controller -run 'TestAdmin(CreateSubscriptionPlanPreservesExplicitZeroPriceMicros|UpdateSubscriptionPlanPreservesExplicitZeroPriceMicros|UpdateSubscriptionPlanPreservesLegacyPriceWhenPriceFieldsAreAbsent|CreateSubscriptionPlanRejectsInvalidExactPricesAtomically|CreateSubscriptionPlanRoundTripsExactPriceMicros|UpdateSubscriptionPlanRoundTripsExactPriceMicros)$' -count=1`：PASS。
- `go test ./model -run TestSubscriptionPlanPriceDiagnosticQuerySupportsAllDialects -count=1`：PASS，确认非 SQLite 查询与 unsupported 行为未改变。
- `bun test src/features/subscriptions/lib/plan-form.test.ts`：`13 pass / 0 fail`。
- `bun run typecheck`：`tsc -b` PASS。
- `git diff --check`：PASS。

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

- 最近安全 HEAD：`cf2b743b84ac74977d654d63dab52ecd8bb0d9fb`（H1）。
- 当前未提交：三份 `spec-fix-*.md`、`model/subscription_price_diagnostic.go`、`model/subscription_price_diagnostic_test.go`。
