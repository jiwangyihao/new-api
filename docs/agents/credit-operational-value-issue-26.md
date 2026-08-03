# Issue #26 实现 Agent 指令

## 目标与垂直交付

你负责父 PRD #19 的 GitHub Issue #26「固化转换估值、FX 与在途请求结算」。必须在 Orca 为你创建的隔离子工作树中，把现有 timed→Credit 转换完整接入运营估值，同时保持已经发布的转换数量、冷却期、宽限期和逐项不可逆语义。确认事务重新校验并冻结源 timed 档位的精确价格、与数量公式相同的 `credit_basis`、毛/净 Credit 与成本、规则版本和必要的 CNY/USD 有理数 FX；转换期间已经预扣的请求在转换后继续沿 #23 的持久 request identity 正确少结算、追加或退款。

这是永久 feature，不是只在 `SubscriptionConversion` 加字段。你必须贯通运行时 FX 配置快照、转换报价/确认、Credit ingress、源/目标权益和 ledger 原子性、转换前后请求桥接、API/UI 六语言以及真实 SQLite/API/浏览器行为。严格禁止越界：#21 的 timed grant 计算不重写；#22 的通用 Credit 混合池不复制；#23 的一般 request settlement 不重构；#24/#25 的兑换、管理员调整和 recovery 不触碰；#27/#28 的历史迁移、门禁与生产发布不实现。

## 必读材料与 Skill

修改前依次阅读并服从：

1. 仓库及全局 `AGENTS.md`。
2. `issue://jiwangyihao/new-api/19` 与 `issue://jiwangyihao/new-api/26`；GitHub CLI 始终显式传 `--repo jiwangyihao/new-api`。
3. `docs/agents/credit-operational-value-execution.md`。
4. `docs/agents/credit-operational-value-wave-3-contract.md`；你是 conversion、运行时 FX 与转换在途请求分支的主改者。
5. 已集成 `.scratch/agent-progress/issue-20`、`issue-21`、`issue-22`、`issue-23` 合同和最终实现。确认精确套餐价格、timed grant、Credit ingress、request target 入口、`valuation_subscription_id` 接缝和稳定错误确实存在。缺失时立即 Orca `orchestration ask`，不得复制依赖切片。
6. `CONTEXT.md`、ADR 0001、ADR 0002。
7. 新规格第 5.6、6、7.1–7.2、7.5–7.6、8–10、11.3、12–14 节；实施计划任务 4 中仅转换部分、任务 6 中仅转换期间在途请求部分，以及任务 9 中转换提示部分。

必须先读取并执行 `skill://tdd`，每个数量、FX、事务和在途请求合同从可观察失败开始。转换/并发/幂等难以定位时读 `skill://diagnosing-bugs` 并先复现；跨模块 seam 需要调整时读 `skill://codebase-design`，但不得改变 ADR/spec。修改 `web/default` 前读 `skill://shadcn-ui`，新增/改变可见文案前读 `skill://i18n-translate` 并维护 en、zh、fr、ru、ja、vi。若实际触及 tiered/dynamic billing 表达式，先读 `pkg/billingexpr/expr.md`；否则不要引入 billing expression 范围。

## 转换数量与冻结估值

- 复用现有报价和 `ConfirmTimedSubscriptionConversion` 真实事务，不新增平行转换入口。确认时重新锁定并校验源权益、源档位、冷却期、宽限期、试用/邀请/disabled 资格、转换规则和目标全局 Credit 权益。
- 毛 Credit 严格保持 `full_31_day_blocks × credit_basis + current_remaining_credit`。31 天是业务月；不得按秒折算部分周期。用于成本的分母必须是同一份冻结 `credit_basis`，不能重新读取变化后的规则。
- 冻结源档位权威 `price_amount_micros`、source/target currency、`credit_basis`、gross/net Credit、gross/net cost micros、规则版本和 FX。来源价格不得从 `float64 PriceAmount`、订单实收额或兼容 DTO 反推。
- 将未舍入单位价值保存为 `valuation_source_price_micros / valuation_credit_basis`，供在途请求恢复；不能只存 floor 后的每 Credit 值。后续套餐改价、Credit basis 或汇率变化不得回写旧转换。
- 转换是 exact 规则价值但不是新增收款，不产生邀请奖励、不进入邀请付费统计。历史明细保留源权益/转换审计；当前剩余价值只进入目标 Credit 混合池，不按源订单伪造剩余来源。

## 运行时 FX 合同

- 同币种固定 1/1。只支持 CNY 和 USD；声明为 `1 USD = numerator / denominator CNY`。USD→CNY 用 floor 乘 numerator/denominator，CNY→USD 反向。非正汇率、溢出、无效原始文本或其他异币种整笔拒绝。
- 运行时不得读取或运算 `float64 USDExchangeRate`。从 `operation_setting`/Option 的原始十进制字符串用严格解析器生成约分、有界、正整数 `CreditFXRateSnapshot`；初始化和配置更新必须先完整验证，再原子替换只读快照。
- 每次异币种转换把当前快照的 numerator、denominator、source currency、captured_at 固化到 conversion/ledger；事务开始后配置变化不能改变结果。禁止动态重估已有状态。
- 若 #20 已提供通用严格 decimal→rational helper，应复用；若接口不足，通过 Orca ask 请求窄扩展，不复制另一套解析惯例。

## 原子转换、幂等与失败

- 同一事务提交转换记录、结构化估值/FX 字段、Credit ingress/ledger、源权益 converted 状态和活动权益接替。任一失败注入全部回滚，不留下部分 converted、重复 Credit 或孤立 ledger。
- 转换低频身份、参数指纹与现有不可逆步骤保持稳定。相同身份/参数重放返回原结果；源权益、数量、价格、basis、FX、currency、规则版本或目标映射变化时返回稳定冲突，不再次转换。
- conversion ledger/record 关键字段必须结构化保存，完整 JSON 只能补充。所有业务分支错误使用稳定 code/sentinel；API/UI 不解析错误文本。JSON 使用 `common/json.go` 包装。
- 新转换必须拒绝 disabled、trial、invitation 或不合格 timed 计划；该检查不改变已有 disabled-plan 权益继续消费的边界，也不恢复 `model_limits`。

## 转换期间在途请求

- 识别“请求在源 timed 权益上预扣、转换确认后首次 settle/refund”的持久化状态。请求日志保留原 `subscription_id`；预扣记录通过转换映射把 `valuation_subscription_id` 指向目标 Credit 状态。
- 已预扣量不在 `current_remaining_credit`，因此转换 ingress 不包含它。转换后首次结算读取原预扣记录、转换记录和目标状态，以冻结单位价值为原 `applied_credit` 建立虚拟 exact 扣除快照，但绝不再次增加目标 `token_used` 或减少目标池。
- 最终目标小于预扣时，差额按虚拟转换快照恢复目标 Credit；清空时带走舍入余数。目标大于预扣时，超出部分按目标池追加当时的 exact/estimated/unknown 移动平均出账，并可按 #23 规则形成 settlement debt。
- 重复相同 settle/refund 是无操作；冲突目标、转换映射不一致、状态缺失、并发转换/结算和溢出必须稳定失败或落入合法串行顺序，不能重复创建虚拟快照。
- 只实现 #23 明确预留的转换分支，不改变普通 Credit 请求和旧 Task 的一般算法。若 #23 接缝不足，先写失败测试并 Orca ask，列出最窄签名变化。

## API、报价与 UI

- 转换 quote/confirm 响应保留既有数量、冷却与 blocker 合同，并新增精确字符串 micros、source/target currency、FX、有理数单位价值/规则版本和明确估值提示。原始 request 数量继续使用整数，不因展示压缩改变 payload。
- UI 明确说明“31 天业务月”“转换规则确值但不是新增收款”“后续改价/汇率不回写”。无效 FX、不支持币种、资格丢失、幂等冲突等使用稳定错误映射。
- 新文案补齐 en、zh、fr、ru、ja、vi；不得改变已发布钱包两列布局、Credit 激活入口、充值隐藏策略或 conversion 既有主要 blocker 展示原则。

## 崩溃恢复与提交纪律

第一项实际改动必须创建并提交：

- `.scratch/agent-progress/issue-26/status.md`：阶段、完成项、下一步、阻塞、最近安全提交；
- `.scratch/agent-progress/issue-26/evidence.md`：RED/GREEN、FX 向量、事务失败、并发、API/浏览器和分析证据；
- `.scratch/agent-progress/issue-26/contract.md`：数量公式、转换 snapshot/ledger 字段、FX 解析与方向、幂等指纹、锁序、在途请求状态机、稳定错误、UI DTO、共享文件和明确非所有权。

每个可编译、可验证小步立即 Conventional Commit 并更新恢复文件。关键工作不得只留在终端、大段脚本或未提交工作树。修改导出符号前使用 LSP references。不需要等待 #24/#25；不得复制其 ingress/recovery 逻辑。

## 验证与完成条件

至少以定向行为测试证明：0/1/多个完整 31 天区块与当前剩余 Credit 的原数量公式；不按部分周期秒数折算；同一 basis 参与数量/价值；同币种、CNY→USD、USD→CNY、约分、floor、原始小数精度、无效/非正 FX、不支持币种与溢出；改价/改汇率不回写；转换重放/冲突；每个事务失败点全回滚；并发转换；在途少结算、追加、退款、重复结算及请求日志/目标估值身份；邀请隔离和 disabled 资格。

运行真实 SQLite 端到端 tracer，通过真实 conversion quote/confirm 入口完成至少一条转换，再读取目标 Credit 估值和五个运营分析 API。使用本地真实应用与浏览器验证报价、确认、精确估值/FX 提示及稳定错误；再以可控 mock-upstream 或现有请求入口证明一条转换期间在途请求完成少结算或追加。静态拦截只能证明渲染。对转换/结算并发运行定向 Go `-race`；完整 MySQL/PostgreSQL 矩阵由 #27 负责，DryRun 不是验收。

只运行本切片定向测试和必要 smoke，格式化明确修改文件并执行 `git diff --check`；不要运行全仓测试或部署。完成前逐条复核 Issue #26 acceptance criteria，提交全部代码/恢复记录并保持工作树干净。随后在当前 Dispatch 只发送一次 `worker_done`，列出提交 SHA、领域/API/UI/FX 合同、定向测试、SQLite/API/浏览器/在途/race 证据、共享文件、三数据库实际范围、风险和进度目录；明确声明未实现 #24/#25/#27/#28。不要关闭 Issue、合并或回收工作树，等待协调器验收。
