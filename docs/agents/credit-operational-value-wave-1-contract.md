# 第一并行波次共享合同（Issues #21 与 #22）

## 基线与并行边界

本合同只供父 PRD #19 的第一并行波次使用。两个 Agent 必须从已经验收并集成 Issue #20 的 `jiwangyihao/credit-operational-value-integration` 最新提交创建子工作树；若父树中没有 #20 的最终实现提交、`.scratch/agent-progress/issue-20/contract.md` 或精确价格/schema 合同，立即通过 Orca `orchestration ask` 报告，不得在旧基线上自行复制 #20。

- Issue #21：计时权益 grant 时间线、计时领域入口、计时多币种分析和管理员计时授予 UI。
- Issue #22：CreditValuation 深模块、人民币/受控外部支付来源、最小同步 request-aware 消费、Credit 五接口分析和共享精确金额展示骨架。
- 两个切片均不得提前实现 #23–#28。共同发现缺口时，记录到各自 `.scratch/agent-progress/issue-<N>/contract.md` 并向协调器提问；不得竞争式扩张所有权。

## 共享接口与主改责任

### Issue #22 主改

#22 是本波次以下共享区域的主改者：

1. `CreditValuationState` 的运行时深模块与 Credit 数量/价值原子更新；
2. `AdminAnalyticsMoneyAmount/Breakdown.amount_micros`、exact/estimated/unknown、nullable `time_based_value`、状态版本与 `snapshot_semantics` 等通用 DTO；
3. paid-value 五接口的显式 `entitlement_type` 分流骨架，以及 Credit 分支；
4. 前端精确 micros/BigInt 格式化、通用置信度/current-only warning 和 Credit 面板字段；
5. 冻结 `40 CNY / 1,000 Credit`、消费 200 后 32 CNY 的真实数据库端到端主验收。

#22 必须为计时分支保留稳定、窄的扩展接缝，不能把 timed 继续硬编码为“查询时套餐价格”。若 #21 尚未集成，#22 可以保持现有 timed 可用行为，但不得伪造 grant 数据、不得把 timed 当 Credit，也不得删除规格规定的 `*_by_currency` 扩展空间。

### Issue #21 主改

#21 是本波次以下区域的主改者：

1. `TimedSubscriptionGrantRequest`、`GrantTimedSubscriptionTx` 和不可变 grant 写入/幂等规则；
2. 订单、兑换、管理员售后授予、续期、邀请/试用排除的 timed 调用点；
3. 计时 grant 时间线投影、窗口去重、逐币种 time/token/recognized 计算；
4. timed 专用 `*_by_currency`、`mixed_grants`、overlap/unknown warnings 和 source breakdown；
5. 管理员计时授予 reason/idempotency UI、计时多币种展示及对应六语言文案。

#21 应优先把计时算法和行构建器放入独立、窄职责文件，由共享 paid-row 分流调用；避免重排 #22 主改的通用 DTO、通用格式化器和 Credit UI。确需改共享文件时只做最小增量，并在 contract.md 逐项列出，方便协调器按“#22 通用骨架优先、#21 timed 增量随后”集成。

## 冻结的数据合同

两个 Agent 无权重新设计以下合同：

- 所有权威金额持久化/传输使用十进制字符串 micros；兼容 `amount` 只在最后一步生成。
- `time_based_value` 对 Credit 为 `null`；timed 在单币种时保留旧 singular 字段，跨币种时 singular 为 `null`，只能使用 `*_by_currency`，禁止动态换汇后相加。
- paid-value 行必须先按显式 `entitlement_type` 分流。Credit 不看全局容器价格或 `end_time`；timed 只看不可变 grant，不看查询时套餐价格补猜。
- #22 的 Credit source 固定为 `credit_balance_pool / moving_weighted_pool`；#21 的 timed source 按 grant 来源聚合，混合来源为 `mixed_grants`。
- 邀请/试用不创建有价 timed grant；Credit 不进入邀请付费或邀请奖励。
- migration marker 非 ready 的具体历史回填和 ready 切换仍只属于 #27。两个切片只按 #20/#27 冻结合同实现前向写入和读取，不自行迁移历史。
- #22 只实现冻结 tracer 所需的最小同步 `request_id` 预扣/最终结算。通用追加、少结算、退款、异步任务、合并器由 #23；转换/售后正向入账由 #24；破坏性恢复由 #25；转换 FX/在途结算由 #26。
- #21 不改变 Credit 请求结算或计时→Credit 转换公式，不新增计时退款自动 reversal schema。

## 并行协作与恢复

两个 Agent 一开始创建并持续提交 `.scratch/agent-progress/issue-<N>/{status,evidence,contract}.md`。`contract.md` 必须首先写明本文件的主改责任、预计共享文件和不会实现的下游范围。每完成一个可编译/可验证的小步立即提交；不要把关键成果只留在未提交工作树或终端脚本中。

需要对方尚未完成的接口时，不猜测也不复制另一切片实现：先按本合同的冻结 shape 建立最窄本地接缝和行为测试，再用 Orca `orchestration ask` 告知协调器接口需求、文件和阻塞范围。可继续做独立部分时继续，不要把整个 Agent 无谓阻塞。

完成后两个 Agent 分别发送一次 `worker_done`。协调器按 #22 通用 DTO/Credit 骨架优先、#21 timed 增量随后集成；若文件冲突，保留双方可观察合同而不是机械选择一边。