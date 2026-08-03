# Issue #20 验证证据

## 约束基线

- 父 PRD：`issue://jiwangyihao/new-api/19`
- 当前切片：`issue://jiwangyihao/new-api/20`
- 生产行为基线：`f446a1569c2ced54a3fe438b5c4575659a59241d`
- 共享执行协议：`docs/agents/credit-operational-value-execution.md`

## 已执行

- 阅读并确认 Issue #20 验收标准。
- 阅读 `CONTEXT.md`、ADR 0001、ADR 0002。
- 阅读规格 4.1–5.7 与实施计划任务 1–2。
- 加载 `skill://tdd`：后续严格采用单个可观察行为的 RED → GREEN 循环。

## RED/GREEN 记录

- RED：`go test ./model -run TestCreditValuationMathMulDivFloorOrdinaryValue -count=1`，编译失败 `undefined: mulDivFloor`。
- GREEN：最小整数实现后普通值 `40,000,000 × 800 / 1,000 = 32,000,000` 通过。
- RED：零分母缺少稳定 sentinel；补充 `ErrCreditValuationDivisionByZero` 后通过。
- RED：`MaxInt64 × MaxInt64 / MaxInt64` 旧 `int64` 中间乘法得到 0；改用 `math/bits.Mul64/Div64` 后通过。
- RED：`MaxInt64 × 2 / 1` 缺少结果溢出检测；补充 `credit_valuation_overflow` 后通过。
- RED：`ParseDecimalAmountMicros` 不存在；实现严格十进制文本解析后六位小数通过。
- RED：负数、七位小数、`MaxInt64` 边界与越界输入缺少稳定错误；补充非负、精度和防溢出检查后通过。
- GREEN：`go test ./model -run 'Test(ParseDecimalAmountMicros|CreditValuationMathMulDivFloor)' -count=1`，目标包通过。
- RED：`go test ./controller -run TestAdminCreateSubscriptionPlanRoundTripsExactPriceMicros -count=1` 返回成功但响应缺少 `price_amount_micros`，SQLite 查询报 `no such column: price_amount_micros`。
- GREEN：`SubscriptionPlan` 增加 nullable `BIGINT` 权威字段并以 JSON 字符串序列化；创建定向测试通过，数据库值与响应均为 `40123456`。
- RED：旧套餐迁移后查询 `price_amount_micros` 报列不存在；SQLite 附加迁移增加 nullable 列后，`TestSubscriptionPlanPriceMicrosMigrationLeavesLegacyRowPending` 证明旧有价行仍为 `NULL`，未填充虚假精确值。

当前实现成本：比例热路径只执行 `math/bits` 128 位乘积/除法和常数次分支；不使用浮点或 `big.Int`，无设计上的堆分配。
## 最终验证待办

- 精确金额解析、序列化和稳定错误码。
- 套餐创建、编辑、读取往返。
- Credit 估值币种首次配置与冻结。
- 附加 migration/schema 可重复与唯一约束。
- 只读非法价格诊断不写数据库。
- 防溢出整数比例边界。
- 管理 UI 真实浏览器请求 payload 与刷新结果。
- 修改文件格式化、`git diff --check`、工作树清洁度。
