# Issue #22 窄验收修复状态

## 冻结基线
- 冻结 clean HEAD：`d5bba460f633ffd2943b1d13bb88b65cea338733`。
- 工作树：`jiwangyihao/issue-22-credit-tracer`。
- 启动时 `git status --short`：空。

## 当前阶段
- 阶段：Finding A 已 GREEN，提交安全点后进入 Finding B RED。
- 已完成：users/subscriptions/plans/sources 共用 precision-boundary 测试覆盖升序、降序和现有业务主键 tie-breaker；四个列表均严格预解析目标币种 `amount_micros` 为 `int64`，不读取兼容 `amount`；解析失败复用既有稳定错误，四个 builder 传播错误。
- 下一步：提交 Finding A 安全点；随后为 summary/users/subscriptions/plans/sources 写 current-only panel warning 行为 RED。
- 阻塞：无。
- 最近安全提交：Finding A 本提交（提交后以 HEAD 为准）。

## 当前实现约束
- Finding A 只严格解析现有 `amount_micros`；禁止兼容 float 回退。
- 无效值只复用语义匹配的既有 `ErrCreditValuationSourceInvalid` / `ErrCreditValuationOverflow`，不新增 sentinel，不做 API 重构。
- Finding A 完成 RED→GREEN 并形成安全提交后，才处理 Finding B。

## 范围边界
- 不重做 CreditValuation、人民币余额、Kyren、BillingSession/request_id、32 CNY tracer、六语言或浏览器 smoke。
- 不实现 Issue #23–#28，不写 migration marker，不切换 `ready`，不实现 FX。
- 仅运行 model/controller 与实际相关前端定向门禁，不运行全仓测试。
