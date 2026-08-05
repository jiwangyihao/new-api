# Issue #23 实现合同

## 当前定位与基线
- 工作树：`C:/Users/34404/source/repos/new-api/.workspaces/new-api/issue-23-request-settlement`。
- 分支：`jiwangyihao/issue-23-request-settlement`。
- 最近安全 HEAD：`ec1858fec89509bdec9a90a230a8496047c5becd`。
- 已验证 `merge-base HEAD ec1858fec89509bdec9a90a230a8496047c5becd` 等于该提交；工作树起始状态干净。
- #22 合同已存在，明确交付 `CreditValuation` 深模块、购买来源快照、最小同步 request tracer 与五接口通用 DTO；#23 只深化 request 分支。

## #23 主改所有权
1. `SubscriptionPreConsumeRecord` 的请求活动快照、累计目标、终态、稳定重放与清理语义。
2. `CreditValuation` request 分支的原子预扣、追加、少结算、退款恢复、absorbed restore、restored unknown 与稳定错误。
3. 唯一 Credit 请求结算入口：稳定 `request_id + original_subscription_id + target_applied_credit + final`；调用方不得提交匿名 Credit delta。
4. `subscriptionTokenDeltaCoalescer` 保留 request identity、目标累计量、稳定入队顺序及逐请求结果。
5. `SubscriptionFunding`、`BillingSession`、quota/relay 同步、流式、失败退款和重算链路传播 `request_id` 与当前累计目标。
6. `TaskPrivateData.subscription_request_id`，新 Task 持久化、旧 Task 基于持久化主键的确定性兼容身份，以及安全保留/清理。

## 深模块接口与不变量
- 外部 seam 固定为请求身份和目标累计量；平均值、舍入、欠额、活动快照、版本和状态机都隐藏在深模块实现内。
- 预扣以 `request_id` 唯一，必须足额，不得形成欠额；相同不可变参数重放返回原结果，冲突返回稳定 sentinel/code 并整笔回滚。
- 请求记录保存：`applied_credit`、`deducted_available_credit`、`debt_formed_credit`、`valuation_subscription_id`、三类活动扣除快照、三类 absorbed restore、`restored_unknown_credit`、规则/结算/状态版本、终态与时间。
- 目标增加按追加时池的移动平均出账，超出可用量只形成 `debt_formed_credit`；目标相同严格无操作，不增加状态或结算版本。
- 目标减少先撤销本请求欠额，再按本请求活动快照比例恢复；清空活动快照带走所有余数，不读取退款时的新池平均。
- 只把退款前后 `newly_available` 恢复到物化状态；仍被其他欠额吸收的份额进入 absorbed audit；后来 ingress 已抵扣的请求欠额退款重新形成可用量时标 `unknown`。
- `final=true` 后相同目标幂等；不同目标只允许退款/纠正，非法增加稳定拒绝。

## 锁序与原子性
- 普通 Credit 请求：目标 `UserSubscription` → `CreditValuationState` → `SubscriptionPreConsumeRecord`。
- 转换后路由接缝：先按稳定 ID 取得原请求/映射，再按目标 `UserSubscription` → `CreditValuationState` → 请求记录完成写入；不得从状态反向锁权益。
- 数量、估值状态和请求记录必须在同一事务更新；状态缺失、不一致、映射冲突、目标冲突和算术溢出全部回滚。

## 状态机与稳定错误
- 请求状态：`consumed`（非终态）→ `settled`（final 且目标大于 0）或 `refunded`（final 且目标为 0）。
- 相同目标重放不变；终态后增加非法；终态后的明确减少用于纠正/退款。
- 复用并扩展稳定估值错误体系：state missing、state mismatch、overflow、idempotency mismatch；新增请求记录缺失、目标非法/冲突、finalized conflict、映射冲突时必须使用可判断 sentinel/code，调用方不得解析错误文本。

## 调用点迁移合同
- 使用 LSP references 与文本调用清单定位所有导出符号和 `PostConsumeUserSubscriptionTokenDelta` 调用点。
- controller/service/relay/异步 Credit 路径一律迁移到请求目标入口；若匿名 helper 保留，只能服务明确的 timed 兼容路径。
- `SubscriptionFunding` 保存 request ID 与累计目标；`Settle(delta)` 先转换为新目标，`Refund()` 以目标 0 调用同一入口。
- 合并器可共享事务，但逐请求验证、舍入、写回；禁止先按 subscription 求和。

## Task 身份与清理边界
- 新 Task 持久化 `subscription_request_id`，创建、轮询、重算、最终结算和失败退款复用同一 ID。
- 旧 Task 缺字段时仅由持久化 Task 主键生成确定性兼容身份；不得使用时间、随机数或进程内布尔值。
- 清理只删除 `settled/refunded` 且早于“最大异步任务生命周期 + 运维保留窗口”的记录；非终态永不因固定天数删除。保留参数遵循现有配置惯例并提供只读诊断。

## #26 接缝
- #23 只保存/使用 `valuation_subscription_id` 和目标权益路由，使转换前创建、转换后结束的请求能够找到目标 Credit 池，同时原请求日志保留原 `subscription_id`。
- 不计算转换单位价值、FX，不建立虚拟扣除成本快照，不实现转换价值恢复；这些严格属于 #26。

## 预计共享文件
- `model/subscription.go`
- `model/credit_valuation.go`
- `model/subscription_delta_coalescer.go`
- `service/funding_source.go`
- `service/billing_session.go`
- `service/quota.go`
- `model/task.go`
- 对应 model/service/异步任务定向测试

## 明确非所有权
- 不重写 #22 的购买 ingress、移动平均通用核心、低频来源构造器、订单来源快照、analytics DTO 或 32 CNY 五接口基线。
- 不实现 #24 的兑换或管理员 increase/API/UI/i18n。
- 不实现 #25 的管理员 decrease、订单退款、拒付或财务恢复。
- 不实现 #26 的转换单位价值、跨币种 FX、虚拟请求快照或转换价值恢复。
- 不实现 #27 的历史迁移、marker 生命周期、`ready` 切换或三数据库最终矩阵。
- 不实现 #28 的镜像、备份、部署或发布。
- 不关闭 Issue、不合并父树、不切换分支、不操作其他工作树。
