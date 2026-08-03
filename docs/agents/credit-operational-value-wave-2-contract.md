# 第二并行波次共享合同（Issues #23 与 #24）

## 基线、依赖与派发条件

本合同只供父 PRD #19 的第二并行波次使用。两个 Agent 必须从已经验收并集成 Issues #20、#21、#22 的 `jiwangyihao/credit-operational-value-integration` 最新提交创建隔离子工作树。派发前协调器必须确认 `.scratch/agent-progress/issue-22/contract.md`、`CreditValuation` 深模块、订单来源快照、最小同步 request tracer 和五接口通用 DTO 已在父树；任一缺失均通过 Orca `orchestration ask` 报告，不得在旧基线上复制或重做 #22。

- Issue #23：把 #22 的最小同步请求 tracer 深化为完整 `request_id + target_applied_credit` 同步/实时/异步可逆结算，负责请求快照、合并器、资金会话与 Task 身份传播。
- Issue #24：把兑换与管理员售后 increase 两类低频正向来源接入 #22 的 Credit ingress，负责档位选择、预览、低频账本、API/UI 与六语言。
- 两个切片都不得提前实现 #25 的 decrease/订单退款/拒付/财务恢复、#26 的计时转换估值/跨币种 FX/转换期间虚拟快照、#27 的历史迁移与 ready 切换、#28 的生产发布。

## 主改责任与冲突边界

### Issue #23 主改

#23 是以下共享区域的主改者：

1. `SubscriptionPreConsumeRecord` 的请求活动快照、累计目标、终态与清理语义；
2. `CreditValuation` 中 request-aware 预扣、追加、少结算、退款恢复和稳定错误接缝；
3. `SettleUserSubscriptionRequestTarget` 领域入口及所有 Credit 匿名 delta 调用点的迁移；
4. `subscriptionTokenDeltaCoalescer` 的逐请求稳定顺序和逐请求结果回写；
5. `SubscriptionFunding`、`BillingSession`、quota/relay 调用链中的 `request_id` 与目标累计量传播；
6. `TaskPrivateData.subscription_request_id`、异步创建/轮询/重算/失败退款的稳定身份，以及预扣记录安全保留策略。

#23 可扩展 `model/credit_valuation.go` 的请求结算部分，但不得改变 #22 已验收的低频 ingress、购买来源快照或 analytics DTO。它只为“转换前创建、转换后结束”的请求保留目标权益路由和 `valuation_subscription_id` 接缝；转换的冻结单位价值、FX、虚拟扣除快照和少结算恢复算法严格留给 #26。

### Issue #24 主改

#24 是以下共享区域的主改者：

1. Credit 兑换成功事务中的结构化来源快照、幂等来源身份和 ingress 调用；
2. 管理员 Credit increase 的 `plan_id`、reason、idempotency fingerprint、档位资格校验和服务端精确预览；
3. `CreditBalanceLedger` 对兑换/售后来源的毛/净金额、FX、规则版本、状态版本与来源终态记录；
4. 管理员 adjustment API、`AdminCreditBalancePanel` 的 increase 档位选择/预览/重试语义，以及 en、zh、fr、ru、ja、vi 文案；
5. 兑换与售后 increase 的领域/API/浏览器 tracer 和邀请隔离验证。

#24 应只调用 #22 已建立的窄 ingress 构造器和 `ApplyCreditValuationIngressTx`；除非发现真实缺陷并先通过 Orca ask 报告，不得重写移动平均、请求退款或通用 analytics。`model/credit_balance.go`、管理员 adjustment controller/frontend 和 `model/redemption.go` 由 #24 主改；#23 不应触碰这些区域。

## 冻结接口与行为合同

两个 Agent 无权重新设计以下合同：

- Credit 数量与估值状态始终由同一深模块在同一事务、固定锁序下写入：目标 `UserSubscription` → `CreditValuationState` → 请求记录或低频 ledger 结果。
- 请求结算只接受稳定 `request_id` 和目标累计 Credit，不接受匿名总 delta。相同目标重放为无操作；目标增加按追加当时移动平均出账；目标减少先撤销本请求欠额，再按请求活动快照恢复。
- 预扣必须足额且不形成欠额。只有已执行请求的追加结算可形成 `debt_formed_credit`。少结算只恢复 `newly_available`；被其他欠额吸收的成本进入 absorbed audit；后来入账已抵扣的请求欠额退款重新形成可用量时只能标 unknown。
- 合并器可以共享一个事务，但必须保留稳定入队顺序，逐请求舍入、校验和回写，结果等同相同顺序逐条事务；禁止按 subscription 汇总匿名 delta。
- 新异步 Task 必须持久化 `subscription_request_id`。旧 Task 缺失时使用持久化 Task 主键构造确定性兼容身份；不得用进程内布尔值作为幂等来源。
- #24 的兑换与管理员 increase 都使用档位 `price_amount_micros`、档位 Credit、原币种、冻结 FX、规则版本与来源快照构造 exact ingress；禁止用当前全局 Credit 容器价格、兼容 float、实际支付额或 controller 声明 confidence。
- increase 必须提供 `plan_id`，且档位为 enabled、非试用、正价格、正 Credit、允许不限时购买；decrease 不得携带 `plan_id`，但其完整行为属于 #25。UI 切换操作时不得泄漏旧档位状态。
- 入账先抵扣 settlement debt，只有净新增可用 Credit 和同比例净成本进入状态。相同幂等键相同参数重放原结果；参数变化返回稳定 idempotency mismatch。
- 两切片均不得切换 migration marker、回填历史或在非 ready 时创建半可信状态；严格服从 #20/#27 门禁合同。
- 已有 disabled-plan 权益继续可消费；兑换和管理员 increase 必须拒绝 disabled/ineligible 档位。任何套餐继续忽略 `model_limits`；Credit 不产生邀请奖励或进入邀请付费统计。

## 并行协作与恢复

两个 Agent 启动后立即创建并提交 `.scratch/agent-progress/issue-<N>/{status,evidence,contract}.md`。`contract.md` 必须先写明本文件的主改责任、预计共享文件、稳定接口和明确非所有权。每个可编译、可验证小步都立即 Conventional Commit；关键成果不得只留在终端、大段临时脚本或未提交工作树。

需要对方尚未完成的接口时，先按本合同建立最窄调用 seam 和失败测试，再用 Orca `orchestration ask` 告知协调器所需接口、文件、阻塞范围及可继续部分；不得复制对方领域逻辑。若同时触及 `model/credit_valuation.go`，#23 只改 request 分支，#24 只消费 ingress 接口。协调器集成顺序默认为 #23 request 核心先、#24 低频 ingress/UI 后；冲突时保留双方可观察合同，不机械选择一边。

完成后两个 Agent 各发送一次 `worker_done`。协调器分别验收定向行为和真实 SQLite/API/UI 证据，再集成并运行本波次联合回归；Agent 不自行关闭 Issue、合并、部署或回收工作树。
