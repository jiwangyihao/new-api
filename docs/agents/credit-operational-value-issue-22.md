# Issue #22 实现 Agent 指令

## 目标与垂直交付

你负责父 PRD #19 的 GitHub Issue #22「打通 32 CNY Credit 购买、消费与五接口分析」。必须在 Orca 隔离子工作树中完成不限时 Credit 运营剩余价值的第一条真实 tracer：订单创建冻结 `40 CNY / 1,000 Credit` 的档位标价；人民币余额购买和一个现有受控外部支付完成入口用该快照入账；真实 `request_id` 同步预扣并最终结算 200 Credit；运营分析 summary、users、subscriptions、plans、sources 和最小管理 UI 一致显示可用 800、exact `32,000,000` micros CNY、活动有价权益数 1。

这是永久 feature，不能只让 calculator 测试转绿，也不能直接插入估值状态伪造结果。持久化、深模块、两个真实入账入口、最小同步消费、五接口、UI、六语言和行为测试必须闭环。但严格限制为 Issue #22：通用追加/少结算/退款、异步任务与合并器归 #23；兑换/转换/管理员售后正向入账归 #24；管理员减少、订单退款/拒付与恢复归 #25；转换 FX 和在途请求归 #26；历史迁移/ready 门禁归 #27。

## 必读材料与 Skill

改代码前依次阅读：

1. 仓库和全局 `AGENTS.md`。
2. `issue://jiwangyihao/new-api/19` 与 `issue://jiwangyihao/new-api/22`；GitHub CLI 始终传 `--repo jiwangyihao/new-api`。
3. `docs/agents/credit-operational-value-execution.md`。
4. `docs/agents/credit-operational-value-wave-1-contract.md`，你是通用 analytics DTO/Credit 分支的主改者。
5. `.scratch/agent-progress/issue-20/contract.md` 和 #20 的精确价格、schema、币种与整数算术实现。若父树不含 #20 最终合同，立即 Orca ask，不得复制或重做 #20。
6. `CONTEXT.md`、ADR 0001/0002。
7. 新规格第 4–9、12–14 节，以及实施计划任务 3、任务 4 中仅购买/支付相关部分、任务 8、任务 9 中 Credit 分析部分。不要照搬计划中属于 #23–#27 的水平步骤。

必须先读并使用 `skill://tdd`，从会失败的真实行为测试开始。修改 `web/default` 前读 `skill://shadcn-ui`；新增可见文字前读 `skill://i18n-translate`，维护 en、zh、fr、ru、ja、vi。并发、事务、数据库或计费选择失败不能直接解释时读 `skill://diagnosing-bugs`；深模块 seam 需要深化时读 `skill://codebase-design`，但 ADR/spec 决策不可擅改。只有真正触及 tiered/dynamic billing expression 时读 `pkg/billingexpr/expr.md`。

## CreditValuation 深模块合同

- `model/credit_valuation.go` 必须成为 Credit `token_limit/token_used` 与每份权益唯一 `CreditValuationState` 的同一事务写入者。锁序固定为已锁定目标 `UserSubscription` → `CreditValuationState` → 本 tracer 的请求记录/ledger；模块不自行提交事务、不清缓存、不发业务日志。
- 调用者只能提交不可伪造的结构化来源事实。`newForwardCreditValuationIngress` 从档位 `price_amount_micros`、档位 Credit、订单创建时币种/规则/来源快照派生 exact；调用方不能直接声明 confidence，也不能在支付回调读取当前套餐价格或渠道实收金额补猜。
- 入账公式严格为整数 floor：毛成本按来源标价×毛 Credit/档位 Credit；先抵扣 `settlement_debt`，只有净新增可用 Credit 和同比例净成本进入状态。`net_credit=0` 不增加剩余成本。使用 #20 防溢出 helper，不用 float/big.Int 热路径。
- 出账按操作前池的移动加权比例分别移除 exact、estimated、unknown。完全消耗可用池时移除全部微单位余数；超量部分只形成欠额。所有保存前验证数量一致、成本非负、unknown 上界、币种和单调 `state_version`。
- marker/迁移语义必须服从 #20/#27：本切片不回填历史、不把半可信历史建成 exact、不切换 ready。只实现前向 tracer 在设计允许状态下的运行时行为和稳定错误接缝；不得热路径按当前套餐价格“修复”缺失状态。
- 所有失败返回稳定 sentinel/code，不能让 controller/frontend 解析错误文本分支。保持已持有 disabled-plan Credit 可消费；所有新购买继续检查档位 enabled/eligible；继续忽略 `model_limits`。

## 必须打通的真实领域入口

- 订单创建时把充值档位权威 `price_amount_micros`、币种、档位 Credit、规则版本写入现有 `EntitlementSnapshot`/结构化来源快照。人民币余额购买完成和一个仓库现有、测试可控的外部支付完成入口都必须使用订单创建快照调用 Credit ingress；回调时改价不能改变入账成本。
- 幂等重放只返回同一已提交结果，不再次增加 Credit 或价值；快照缺失、币种不支持、档位不合格、金额溢出、数量/状态失败必须让订单完成事务整体回滚。禁止用直接 `DB.Create(CreditValuationState)` 作为业务实现。
- 只实现冻结 tracer 所需的最小同步 request-aware 路径：真实 `request_id` 预扣 200，写入必要的 `SubscriptionPreConsumeRecord` 身份/扣除事实，并在一次同步最终结算到目标累计 200。预扣要求足额、不形成欠额；重复相同同步目标幂等。不要提前实现 target 减少、通用追加、退款、异步 task `subscription_request_id` 或 coalescer，它们属于 #23。
- 消费选择 Credit 时不能因全局 Credit 计划 disabled 而拒绝已有权益；新入账仍检查充值档位和功能开关。邀请付费/邀请奖励继续排除 Credit。

## 五接口与 UI 合同

- 你是本并行波次通用 analytics DTO 和 Credit 分支主改者。`AdminAnalyticsMoneyAmount/Breakdown` 增加精确 `amount_micros` 字符串；内部汇总/排序使用整数 micros，最后才派生兼容 float。通用响应增加 exact/estimated/unknown、状态版本、更新时间、nullable `time_based_value` 与 `snapshot_semantics`。
- paid-row builder 先按显式 `entitlement_type` 分流。Credit 不检查零价格全局容器、不看 `end_time=0`、不进入时间价值公式；`token=recognized=exact+estimated`，`time=null`，basis 为 moving weighted average。
- 正可用 Credit 即计为一条活动有价权益，即使全部成本 unknown；耗尽/欠额保留明细、金额零、不计 active。Credit source 固定为 `credit_balance_pool / moving_weighted_pool`，不再按 `(user_id, plan_id)` 猜订单；plan filter 只匹配全局 Credit plan。
- `snapshot_at` 早于状态更新时间时返回最新状态、版本和 `current_only` warning，不伪造历史。summary、users、subscriptions、plans、sources 必须由同一状态事实得到一致结果。
- 与并行 #21 的边界：保留稳定 timed 扩展 seam，不继续固化“当前 plan 价格”为新主口径；#21 主改 timed grant calculator、`*_by_currency` 和 timed UI。你可以定义通用 shape，但不得替 #21 实现 grant 时间线。共享文件冲突时 contract.md 明确列出你的主改内容。
- 最小管理 UI 必须优先解析 micros 字符串并用 BigInt/字符串格式化；仅旧响应缺字段时回退 float。显示 32 CNY、exact/estimated/unknown、Credit 时间值“不适用”、moving-weighted 术语和 current-only 非阻断刷新提示；不得称为退款、负债、实收。新增文案补齐六语言。

## 主验收 seam 与测试纪律

主测试必须使用真实数据库和现有领域入口：创建零价全局 Credit 容器与 `40 CNY / 1,000 Credit` 有价充值档位，创建订单并走人民币余额或受控支付完成，走真实 `request_id` 同步预扣/结算 200，再调用五个 paid-value API。禁止测试直接插入 `CreditValuationState` 或静态拦截 API 代替该主链路。

除主 seam 外，只为算术/不变量、事务回滚、幂等、锁序/并发和 controller 边界保留必要低层测试。至少覆盖：不同价格两次入账的移动平均、先消费后入账顺序差、欠额抵扣、清空余数、全 unknown 活动计数、exhausted/debt、current_only、改价后回调仍用旧快照、重复回调/重复 settle、disabled 既有消费与新购买拒绝、邀请隔离、五接口筛选/排序一致。

## 崩溃恢复与完成条件

第一项实际改动创建并提交：

- `.scratch/agent-progress/issue-22/status.md`：阶段、完成、下一步、阻塞、最近安全提交。
- `.scratch/agent-progress/issue-22/evidence.md`：RED/GREEN、真实 DB/API/UI smoke、失败根因和精确输出摘要。
- `.scratch/agent-progress/issue-22/contract.md`：深模块接口、状态不变量、来源快照、最小 request seam、DTO/UI 字段、稳定错误、共享文件与非所有权。

持续更新并小步 Conventional Commit；关键成果不能只留在终端或一次性脚本。需要 #21 的 timed seam 时按共享合同保留窄扩展点并 Orca ask，不要复制 timed 实现。

至少运行定向 Go/前端测试、真实 SQLite 主 tracer 和应用真实浏览器 smoke；记录五接口响应关键字段、请求 payload 和 UI 观察。完整 MySQL 5.7/PostgreSQL 9.6 零 SKIP 由 #27 负责，但 SQL/schema 必须三方兼容，DryRun 不是验收。只格式化修改文件，运行 `git diff --check`；不要运行全仓测试或生产部署。

完成前逐条复核 Issue #22 acceptance criteria，提交全部代码/恢复记录并保持工作树干净。然后只发送一次 `worker_done`，包含提交 SHA、修改文件/合同、定向测试、SQLite 五接口 tracer、浏览器证据、共享文件、三数据库实际范围、风险和进度目录；明确声明未实现 #23–#28。不要关闭 Issue、部署、合并或回收工作树，等待协调器验收。