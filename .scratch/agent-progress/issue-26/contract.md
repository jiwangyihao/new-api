# Issue #26 固化合同

## 范围与不变量

本文件是 Issue #26 的恢复与跨模块合同。实现只覆盖 timed → Credit 转换、唯一运行时 FX seam，以及转换时已经在途请求的精确估值与后续结算；任何偏离都必须先在 `status.md` 和 `evidence.md` 记录原因与验证。

## 转换数量公式

- `full_31_day_blocks = floor(remaining_duration_seconds / (31 × 24 × 60 × 60))`。
- `gross_credit = full_31_day_blocks × source_credit_basis + current_remaining_credit`。
- 不按秒折算不足 31 天的部分周期。
- `current_remaining_credit` 只加一次，不得同时从 grant、plan 或请求余额重复计入。
- Quote 与 Confirm 使用同一整数公式；Confirm 必须从锁定后的权威数据重算并与 Quote 冻结指纹一致。

## FX 方向与整数换算

唯一方向定义：`1 USD = numerator / denominator CNY`。

`CreditFXRateSnapshot` 的结构化字段冻结为：

- `source_currency`：`CNY` 或 `USD`；
- `valuation_currency`：`CNY` 或 `USD`；
- `numerator`：正整数十进制字符串，约分后分子；
- `denominator`：正整数十进制字符串，约分后分母；
- `captured_at`：正的 Unix 毫秒时间戳十进制字符串；
- `direction`：固定字面值 `USD_TO_CNY`，同币种快照为 `IDENTITY`。

同币种永远使用 `1/1`，不依赖 Option。跨币种仅支持 CNY/USD：

- USD → CNY：`floor(source_micros × numerator / denominator)`；
- CNY → USD：`floor(source_micros × denominator / numerator)`；
- 同币种：原值不变。

所有金额与比率运算必须为 overflow-safe 全整数运算，最终向下取整；禁止读取、运算或反推 `float64 USDExchangeRate`。Option 原始十进制文本只在唯一 parser/provider 中解析：拒绝缺失、空白、非规范十进制、超过允许精度、零、负数、方向不匹配、不支持币种和中间值/结果溢出；解析后以最大公约数约分并原子发布不可变快照。普通 Credit ingress 只能携带该窄类型，不得拥有第二套 parser/provider。

## 稳定错误合同

Go 导出 sentinel 与结构化 code 一一对应，调用者可使用 `errors.Is` 或 code 判断：

| Sentinel | Code |
| --- | --- |
| `ErrCreditFXRateMissing` | `credit_fx_rate_missing` |
| `ErrCreditFXRateEmpty` | `credit_fx_rate_empty` |
| `ErrCreditFXInvalidDecimal` | `credit_fx_invalid_decimal` |
| `ErrCreditFXPrecisionExceeded` | `credit_fx_precision_exceeded` |
| `ErrCreditFXNonPositive` | `credit_fx_non_positive` |
| `ErrCreditFXDirectionMismatch` | `credit_fx_direction_mismatch` |
| `ErrCreditFXUnsupportedCurrency` | `credit_fx_unsupported_currency` |
| `ErrCreditFXOverflow` | `credit_fx_overflow` |
| `ErrConversionIneligible` | `subscription_conversion_ineligible` |
| `ErrConversionQuoteStale` | `subscription_conversion_quote_stale` |
| `ErrConversionIdempotencyConflict` | `subscription_conversion_idempotency_conflict` |

API 错误响应必须保留稳定 `code`；显示文案由六语言 i18n 映射，不得依赖解析自由文本。

## Quote 与 Confirm 冻结字段

Quote 和最终 conversion 都冻结以下结构化数据：

- source entitlement/subscription/plan/grant 的 ID 与权威版本或更新时间；
- source tier 的单位价值 micros、currency、credit basis；
- duration、reset、rule 以及完整 31 日块数量；
- `current_remaining_credit`、`gross_credit`；
- 转换规则版本；
- 完整 `CreditFXRateSnapshot`；
- source value micros、target value micros；
- quote 创建/过期时间；
- 创建者 user ID 与目标 Credit currency。

转换属于价值搬运，不得标记为新增收款、邀请收入或退款价值。

## 幂等指纹

Confirm 的唯一幂等域为 `(operation = timed_to_credit, user_id, idempotency_key)`。指纹为固定字段顺序的 canonical struct 经项目 JSON wrapper 序列化后取 SHA-256，输入至少包括：`quote_id`、source subscription/plan/grant 身份及权威版本、冻结 source tier/value/currency、duration/reset/rule、`current_remaining_credit`、`gross_credit`、目标 currency、完整 FX snapshot、source/target micros 和转换规则版本。

同一幂等域且指纹相同必须返回原 conversion/ledger/target Credit 结果；同一幂等域但任一参数不同必须返回 `subscription_conversion_idempotency_conflict`，不得产生第二笔写入。

## 事务与锁序

Confirm 的固定锁序：

1. 以唯一幂等域锁定或原子保留 conversion/idempotency 行；
2. 锁定 quote；
3. 锁定 source entitlement/subscription；
4. 锁定 source plan；
5. 按主键升序锁定 source grants；
6. 按 request identity 升序锁定关联在途请求扣除快照；
7. 锁定目标活动/Credit entitlement；
8. 写入目标 Credit ingress、低频 ledger、conversion 状态和活动接替。

每一步都在同一数据库事务中完成；锁后重读权威状态并重算资格、数量、价值与指纹。disabled、trial、invitation 或不合格 plan 拒绝新的转换；既有 disabled-plan entitlement 的消费合同不改变。任一注入故障、stale quote、冲突或写失败全部回滚，不得留下 ingress、ledger、conversion 或活动状态的部分结果。

## 在途请求状态机

复用 Issue #23 的 request identity、累计目标、request deduction snapshot 与 cleanup，不创建匿名 delta：

1. `source_preconsumed`：请求已从 timed source 预扣，保留原 `subscription_id` 和 request attribution；
2. 转换事务内附加不可变 `conversion_virtual_exact` 估值快照，状态进入 `converted_inflight`，但 request identity 与原 source attribution 不变；
3. 少结算、追加扣除或退款都通过原 request-aware 入口，以累计目标相对已结算累计值计算，并只使用该虚拟 exact snapshot；
4. 首次终态完成 Issue #23 cleanup，进入 `terminal`；重复终态返回同结果或 no-op，不得二次改变数量或价值。

后续 Credit ingress、Option FX 更新或活动订阅变化不得重估此请求。禁止直接改数量再补写估值状态。

## API 与 UI DTO

Issue #26 自有 API：quote、confirm、conversion history；不暴露 Issue #24 管理 increase/redemption 生命周期。DTO 中所有可能超过 JavaScript 安全整数或要求十进制精确的字段均为字符串：`*_micros`、`numerator`、`denominator`、`captured_at`、credit 数量及累计值。前端只允许 string/BigInt 算术，禁止 `Number` 转换金额或 FX。Quote/Confirm/History 必须展示冻结的 source/target currency、micros、`numerator/denominator`、方向与 captured_at，并用稳定错误 code 映射六语言文案。

## 文件所有权

Issue #26 可拥有：

- 唯一 Credit FX parser/provider/snapshot 与其定向测试；
- timed → Credit quote/confirm/history 的 model/service/controller/router 与定向测试；
- Issue #23 request-aware seam 所需的最小扩展；
- `web/default` 中转换入口、quote/confirm/history UI 与六语言翻译；
- `.scratch/agent-progress/issue-26/**` 和 Issue #26 验收证据。

共享文件只做最小追加：Option 变更发布 hook、路由注册、既有 subscription service/model、公共 DTO、六语言 locale。不得建立第二套约定或重写无关区域。

## 明确非所有权

- 不修改 Issue #24 admin increase/redemption 业务、API、UI 或其幂等生命周期；只满足其已冻结跨币种消费者合同。
- 不实现 Issue #25 decrease/refund/chargeback/recovery。
- 不实现 Issue #27 历史回填、迁移 CLI、marker/ready/failed/suspended；MySQL/PostgreSQL 实机验收也不归本 Issue。
- 不实现 Issue #28 镜像、备份、部署或生产操作。
- 不恢复 `model_limits`，不动态重估历史，不使用浮点金额，不创建第二套 FX 类型，不绕过 request-aware 入口写匿名 Credit delta。
