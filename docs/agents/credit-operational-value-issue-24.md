# Issue #24 实现 Agent 指令

## 目标与垂直交付

你负责父 PRD #19 的 GitHub Issue #24「覆盖兑换与 Credit 售后正向入账」。必须在 Orca 为你创建的隔离子工作树中，把两条真实低频正向来源完整接入 #22 的 CreditValuation 深模块：Credit 兑换成功事务冻结所选充值档位的精确价格与来源事实；管理员售后 increase 通过真实 API/UI 选择合格档位、预览任意正 Credit 的运营价值并以可重试幂等键入账。两条路径都必须先抵扣 settlement debt，只把净新增可用量和同比例净成本计入状态，并在分析明细中可观察。

这是永久 feature，不是只加 `plan_id` 字段或只插 ledger。你必须闭环领域入口、兑换/管理员调用点、事务与幂等、结构化低频账本、API、管理员 UI、六语言、真实 SQLite/API/浏览器验收。严格禁止越界：购买、通用深模块和五接口骨架属于 #22；请求退款/异步任务属于 #23；decrease、订单退款/拒付和财务恢复属于 #25；计时转换估值、跨币种转换 FX 和在途请求属于 #26；历史迁移、ready 门禁、发布属于 #27/#28。

## 必读材料与 Skill

修改前依次阅读并服从：

1. 仓库及全局 `AGENTS.md`。
2. `issue://jiwangyihao/new-api/19` 与 `issue://jiwangyihao/new-api/24`；GitHub CLI 始终显式传 `--repo jiwangyihao/new-api`。
3. `docs/agents/credit-operational-value-execution.md`。
4. `docs/agents/credit-operational-value-wave-2-contract.md`，你是兑换、管理员 increase、低频来源账本和对应 UI 的主改者。
5. `.scratch/agent-progress/issue-20/contract.md`、`issue-22/contract.md` 与已集成实现；确认精确 `price_amount_micros`、CNY/USD 币种/FX 快照合同、Credit ingress 构造器、分析明细和稳定错误均存在。缺失时立即 Orca `orchestration ask`，不得自行重做 #20/#22。
6. `CONTEXT.md`、ADR 0001、ADR 0002。
7. 新规格第 5.6、6、7.2、7.6、8、9、12.4、13、14 节，以及实施计划任务 4 中仅“兑换与管理员 increase”部分、任务 9 中管理员 Credit increase UI 部分。不要实现任务 4 的 conversion/recovery/decrease。

必须先读取并执行 `skill://tdd`，从真实领域/API/UI 会失败的行为测试开始。修改 `web/default` 前必须读 `skill://shadcn-ui`；新增或改变可见文案前读 `skill://i18n-translate`，维护 en、zh、fr、ru、ja、vi。事务、幂等、FX 或数据库错误难以定位时读 `skill://diagnosing-bugs`，先复现再修；深模块调用 seam 不清楚时读 `skill://codebase-design`，但不得改变 ADR/spec。只有实际触及动态计价表达式才读 `pkg/billingexpr/expr.md`。

## Credit 兑换领域合同

- 从现有 Credit 兑换真实 API/领域入口开始，不新增平行的“估值专用兑换”。兑换成功事务必须读取并锁定所选充值档位，验证其仍满足 enabled、非试用、正价格、正 Credit、允许不限时购买等现有/规格资格；disabled 或不合格档位拒绝新兑换。
- 使用档位权威 `price_amount_micros`、档位 Credit 分母、原币种、规则版本和入账时 CNY/USD 有理数 FX 构造 #22 的不可伪造 forward ingress。禁止从 `float64 price_amount`、当前全局 Credit 容器价格、渠道实收金额或 controller 提交的 confidence 推导价值。
- 兑换来源使用稳定 `source_type/source_key/source_id` 与 idempotency key。兑换记录的 fulfillment/source snapshot 与 ledger 保存相同不可变事实；后续档位改价、改币种或改 Credit 不能回写已完成兑换价值。
- 兑换来源终态、Credit 数量、估值状态和 `CreditBalanceLedger` 必须在同一事务提交；任一步失败全部回滚。唯一键冲突后只重放已提交的相同参数结果；同身份但价格、币种、FX、档位、数量或规则不同返回稳定幂等冲突。
- ingress 先抵扣已有 settlement debt。`gross_credit` 全部偿债时 `net_credit=0`、运营剩余价值不增加；部分偿债时只有净 Credit 与同比例 floor 后的净成本进入 exact 状态。账本关键字段必须结构化保存，不能只写 JSON。

## 管理员售后 increase API/UI 合同

- 扩展现有管理员 Credit adjustment 请求，而不是新增旁路接口：`increase` 必须提交正 `amount`、合格 `plan_id`、非空 reason 和稳定 `idempotency_key`；`plan_id` 进入完整参数指纹。缺 plan、档位不合格、币种/FX 无效、溢出或状态不一致返回稳定错误并回滚。
- 任意正 Credit 数量按所选档位单位价值以整数 `floor(price_amount_micros × amount / plan_credit)` 计算毛成本；跨 CNY/USD 再使用冻结有理数 FX。服务端结果是权威合同，UI 预览必须调用或复用同一后端精确计算接缝，不能以 JS 浮点作为最终值。
- increase 调用与兑换相同的 Credit ingress，原子更新数量、状态、来源终态和低频 ledger。响应返回毛/净 Credit、毛/净 `amount_micros`、币种、confidence、FX、规则版本和 `state_version_after`，以字符串传精确 micros。
- 相同幂等键和相同参数重放原结果，不再次增加数量或价值；同键但 operation、plan、amount、price、FX、currency、reason（若现有指纹合同包含 reason）或规则版本变化时稳定拒绝。先查仓库现有幂等指纹惯例，并把最终字段列表写入 contract.md。
- 管理员 UI 在 operation 为 increase 时加载现有管理员套餐列表并只展示合格充值档位，要求选择档位、显示标价/档位 Credit/原币种与本次数量的精确运营价值预览。失败重试保留原幂等键；成功后，或 operation/plan/amount 等业务参数变化后生成新 key。
- 切换到 decrease 时必须隐藏且清除 `plan_id` 和预览状态，请求不得发送 `plan_id`。不要实现 decrease 的移动平均出账；其完整行为归 #25。不得让 increase 档位状态泄漏到其他管理员操作。
- 文案使用“运营剩余价值”“售后授予”等准确术语，不得称为实收、退款额、负债或可退款余额。所有新文案补齐 en、zh、fr、ru、ja、vi，并通过 i18n missing/extras 检查。

## 共同不变量与非所有权

- 两条来源只调用 #22 窄 ingress 接口，不在 controller/service 自行修改 `token_limit/token_used` 或 `CreditValuationState`。固定锁序和同事务不变量不能改变。
- 所有失败必须是稳定 code/sentinel；前端/API 不解析错误文本决定业务分支。使用 `common/json.go` 包装 JSON。
- 低频 ledger 至少结构化保存来源身份、plan ID、gross/net Credit、gross/net cost micros、估值币种、源币种、FX numerator/denominator/captured_at、confidence、规则版本、状态版本和参数指纹；完整来源 JSON 只是补充。
- marker 非 ready 时服从 #20/#27 的生产基线与前向快照规则，不自行创建半可信状态、回填历史或切换 ready。若 #22 已提供运行态门禁 seam，严格复用。
- Credit 兑换与售后 increase 不产生邀请奖励、不进入邀请付费统计；任何套餐继续忽略 `model_limits`。已有 disabled-plan Credit 权益仍可消费，但新兑换/increase 必须拒绝 disabled 档位。
- 本切片不实现 timed→Credit conversion，即使现有“兑换”命名邻近 conversion 代码也必须保持边界；转换价值/FX 与在途请求全部归 #26。

## 崩溃恢复与提交纪律

第一项实际改动必须创建并提交：

- `.scratch/agent-progress/issue-24/status.md`：阶段、完成项、下一步、阻塞、最近安全提交；
- `.scratch/agent-progress/issue-24/evidence.md`：RED/GREEN 命令、API/浏览器/数据库证据、失败根因；
- `.scratch/agent-progress/issue-24/contract.md`：兑换来源身份、increase payload/fingerprint、资格规则、ingress/ledger 字段、响应、UI key 生命周期、共享文件和明确非所有权。

频繁更新并在每个可编译、可验证小步使用 Conventional Commits 提交。不要把关键实现只留在未提交工作树或一次性脚本。需要 #23 的请求入口时继续不相关部分并 Orca ask，不得复制 request settlement。修改导出符号前使用 LSP references。

## 验证与完成条件

至少以定向测试证明：兑换冻结档位事实且改价不回写；兑换重复/冲突幂等；事务任一步失败全回滚；全额/部分 debt offset；管理员 arbitrary positive Credit 比例估值；缺失/disabled/trial/零价/零 Credit/不允许购买档位拒绝；跨 CNY/USD 快照与 unsupported currency 拒绝；increase 重放/冲突；operation 切换清除 plan；邀请隔离；分析明细显示 exact/available/debt 和正确来源，而非只断言 ledger 插入。

运行真实 SQLite 领域/API tracer，分别通过真实兑换入口和管理员 adjustment API 入账并读取运营分析明细。启动应用并用真实浏览器完成管理员 increase：选择档位、输入 800 Credit、以 `40 CNY / 1000 Credit` 预览 32 CNY、提交或在可控失败后重试同一 key、切换 decrease 后确认 payload 不含 `plan_id`。记录实际请求/响应关键字段和 UI 观察；静态拦截只能证明渲染，不能替代 API 行为。

完整 MySQL/PostgreSQL 矩阵由 #27 负责，但 schema/SQL 必须跨库；DryRun 不是验收。只运行本切片定向测试和必要 smoke，格式化明确修改文件，执行 `git diff --check`；不要运行全仓测试或部署生产。完成前逐条复核 Issue #24 acceptance criteria，提交所有代码/恢复记录并保持工作树干净。随后在当前 Dispatch 只发送一次 `worker_done`，列出提交 SHA、领域/API/UI 合同、测试、SQLite/API/浏览器证据、共享文件、三数据库实际范围、风险和进度目录；明确声明未实现 #23、#25–#28。不要关闭 Issue、合并或回收工作树，等待协调器验收。
