# Issue #26 独立验收门禁

## Gate A：Worker 交付与可恢复性

- [ ] Orca Dispatch 只发送一次 `worker_done`；记录最终 Worker 状态、共同基线、Worker HEAD、`merge-base`、终端 ID 与失败计数。
- [ ] Worker 源自已验收并集成 #20、#21、#22、#23 的共同基线；不得从 `origin/main`、生产提交或旧 Worker 分支开工。
- [ ] `.scratch/agent-progress/issue-26/{status,evidence,contract}.md` 全部提交，且数量公式、FX 方向、锁序、转换/请求状态机、RED/GREEN 命令和遗留风险与代码一致。
- [ ] 每个可验证小步均为 Conventional Commit；工作树干净，关键事务、并发、API、browser、FX 向量与分析证据不只存在于终端或临时脚本。
- [ ] 修改导出符号前的 LSP references、现有 quote/confirm/request settlement 调用点及最终迁移清单可追溯。

## Gate B：切片所有权与非目标

- [ ] #26 只负责 timed→Credit 转换估值、运行时 CNY/USD 有理数 FX 快照和转换期间在途请求的虚拟扣除/后续结算。
- [ ] 复用 #21 timed grant、#22 Credit ingress/移动平均、#23 request target 入口；不复制或重构这些通用模块。
- [ ] 未实现 #24 的 redemption/admin increase、#25 的 decrease/refund/chargeback/recovery、#27 的历史迁移/ready 或 #28 的发布。
- [ ] 转换不作为新收款、不产生邀请收入、不进入邀请付费统计；没有恢复 `model_limits` 或改变既有 disabled-plan 消费边界。
- [ ] 未动态重估既有转换或 Credit 状态，未支持 CNY/USD 之外的异币种 ingress。

## Gate C：转换数量与冻结估值

- [ ] quote 与 `ConfirmTimedSubscriptionConversion` 保持既有数量合同：`full_31_day_blocks × credit_basis + current_remaining_credit`，31 天为业务月，部分周期不按秒折算。
- [ ] 价值与数量使用同一份冻结 `credit_basis`；保存未舍入单位价值 `valuation_source_price_micros / valuation_credit_basis`，而不是仅保存 floor 后单位价。
- [ ] 确认事务重新锁定并校验源权益、源档位、冷却期、宽限期、trial/invitation/disabled 资格、规则版本和目标全局 Credit 权益。
- [ ] 冻结权威 `price_amount_micros`、source/target currency、basis、gross/net Credit、gross/net cost micros、规则版本和 FX；禁止从 float、兼容 DTO 或实际支付额反推。
- [ ] 后续套餐改价、basis 或 FX 更新不回写既有 conversion、ledger 或目标池成本；历史明细保留源权益和转换审计。
- [ ] 0、1、多个完整 31 天区块及 current remaining Credit 的边界均有行为测试，且已预扣量不被重复计入 `current_remaining_credit`。

## Gate D：运行时 FX 快照

- [ ] 同币种严格使用 1/1；仅支持 CNY/USD，方向固定为 `1 USD = numerator / denominator CNY`，正反转换均以 overflow-safe integer floor 计算。
- [ ] 从持久化 Option 原始十进制字符串严格解析、约分并有界验证；运行时不读取或运算 `float64 USDExchangeRate`，也不从 float 反推。
- [ ] 非正、空值、超精度、溢出、非法文本及不支持币种均返回稳定 code 并整笔拒绝；业务分支不解析错误文本。
- [ ] 初始化与配置更新必须先完整验证，再原子替换只读 `CreditFXRateSnapshot`；并发读取不能观察到半更新状态。
- [ ] 异币种转换在事务内冻结 numerator、denominator、source/target currency 与 `captured_at`；事务开始后的配置变化不影响本次结果。
- [ ] FX 测试向量覆盖 CNY→USD、USD→CNY、约分、floor、原始小数精度、边界乘积和结果溢出。

## Gate E：原子转换、幂等与可观察性

- [ ] 同一事务提交 conversion 结构化估值/FX 字段、Credit ingress/ledger、源权益 converted 状态和活动权益接替；每个故障注入点均证明全回滚。
- [ ] 相同稳定身份和完全相同参数重放原结果；源权益、数量、价格、basis、FX、currency、规则版本或目标映射变化稳定冲突且不重复转换。
- [ ] conversion/ledger 的价格、basis、单位价值、gross/net Credit/成本、FX、规则/状态版本和参数指纹均为结构化字段；JSON 只能补充。
- [ ] 五个运营分析 API 在转换后只把价值计入目标 Credit 当前池，源 timed 历史仍可追溯；不得把转换算作新增实收或邀请收入。
- [ ] quote/confirm API 保留既有数量、冷却与主要 blocker 合同，并新增字符串 micros、币种、FX、有理数单位价值和规则版本。
- [ ] disabled、trial、invitation 或其他不合格的新转换稳定拒绝；已有 disabled-plan 权益继续消费的回归保持通过。

## Gate F：转换期间在途请求

- [ ] 持久识别“源 timed 已预扣、转换已确认、首次 settle/refund 尚未映射”的状态；请求日志保留原 `subscription_id`，估值路由使用目标 `valuation_subscription_id`。
- [ ] 转换 ingress 排除已预扣量；转换后首次结算以冻结单位价值建立虚拟 exact 扣除快照，但绝不再次增加目标 `token_used` 或减少目标池。
- [ ] 最终目标小于预扣时按虚拟快照恢复，清空时吸收舍入余数；目标大于预扣时只有增量按目标池当时 exact/estimated/unknown 平均出账并可形成 debt。
- [ ] 相同 settle/refund 重放为无操作；目标冲突、转换映射不一致、状态缺失、溢出均返回稳定错误并原子回滚。
- [ ] 并发 conversion+settlement 只产生合法串行化结果，不重复创建虚拟快照、不重复扣目标池、不遗失请求终态。
- [ ] 普通 Credit 请求和旧 Task 的 #23 算法保持不变；转换分支只消费预留 seam，没有匿名 delta 绕路。

## Gate G：API/UI、真实链路与回归

- [ ] UI 明确展示“31 天业务月”“规则确值但不是新增收款”“后续改价/汇率不回写”，且原始数量/micros 请求不因紧凑显示改变。
- [ ] 无效 FX、不支持币种、资格丢失和幂等冲突通过稳定错误映射；新增文案覆盖 en、zh、fr、ru、ja、vi 且无 missing/extras。
- [ ] 钱包两列布局、Credit 激活入口、充值隐藏策略和 conversion 单一主要 blocker 展示原则保持不变。
- [ ] 真实 SQLite tracer 经真实 quote/confirm 完成转换，随后读取 conversion、目标状态、ledger 和五个分析 API；不得直接插表冒充主路径。
- [ ] 本地真实应用/API 与浏览器验证 quote、confirm、精确估值/FX 提示及受控稳定错误；再经可控 upstream/现有请求入口证明一条在途少结算或追加。
- [ ] 定向数据库并发与 Go `-race` 覆盖 conversion+settlement；跨库锁/schema 完成静态审查，完整三库零 SKIP 保留给 #27。
- [ ] 运行受影响 Go/前端定向测试、真实 SQLite/API/browser smoke、typecheck/build、六语言检查和 `git diff --check`。
- [ ] #20 精确价格、#21 timed grant、多币种分析、#22 32 CNY Credit 与 #23 普通 request restore 均保持通过；若 #24/#25 已集成，其 ingress/outflow 也不得受转换分支影响。
- [ ] 完成报告逐条映射 GitHub Issue #26 acceptance criteria，列出提交 SHA、领域/API/UI/FX 合同、命令、关键输出、未运行范围和共享文件风险。
- [ ] 协调器只有在全部门禁有证据后才能 non-ff merge；失败项返回原 Worker 修复，不得在协调器分支临时重写领域逻辑。

## 不放行条件

- 改变既有转换数量公式、按秒折算部分周期，或数量与价值使用不同 basis；
- 从 float、兼容价格或实际支付额推导价格/FX，或动态重估历史状态；
- 转换 ingress 重复包含已预扣量，首次 settle 再次扣目标池；
- 只保存 floor 后单位价，导致少结算无法按原转换快照恢复；
- 请求日志丢失原 subscription 身份，或重放依赖进程内状态；
- conversion、Credit ingress、源 converted 状态或活动接替不在同一事务；
- 只有 mock、直接插表或 DryRun，没有真实 SQLite、API/browser、在途请求和并发证据；
- 越界实现 #24/#25/#27/#28，或削弱 #20–#23 已集成合同。
