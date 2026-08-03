# Issue #24 独立验收门禁

## Gate A：Worker 交付与可恢复性

- [ ] Orca Dispatch 已发送且只发送一次 `worker_done`；记录 Worker 最终状态、共同基线、Worker HEAD、`merge-base`、终端与失败计数。
- [ ] Worker 源自已经验收并集成 #20、#21、#22 的共同基线，且 `.scratch/agent-progress/issue-24/{status,evidence,contract}.md` 全部提交。
- [ ] `contract.md` 明确兑换来源身份、管理员 increase payload/fingerprint、档位资格、ingress/ledger/响应字段、UI 幂等键生命周期、共享文件和非所有权。
- [ ] 每个可验证小步均为 Conventional Commit；工作树干净，关键实现、RED/GREEN、API/browser 证据不只留在终端或临时脚本。

## Gate B：切片所有权与非目标

- [ ] #24 只负责 Credit 兑换和管理员售后 increase 两类正向 ingress、档位资格/预览、低频 ledger、对应 API/UI/i18n 与邀请隔离。
- [ ] 只调用 #22 的窄 ingress 和移动平均深模块，不在 controller/service 直接修改 `token_limit/token_used` 或 `CreditValuationState`，不重写通用 analytics。
- [ ] 未实现 #23 的请求结算、#25 的 decrease/订单退款/拒付/财务恢复、#26 的 timed→Credit 转换/转换 FX/在途请求、#27 的迁移/ready、#28 的发布。
- [ ] 跨 CNY/USD 仅使用 #22 提供的普通 Credit ingress-time 有理数 FX 接缝；没有复制 #26 的转换估值与虚拟快照算法。
- [ ] 已有 disabled-plan 权益继续可消费，但兑换和 increase 均拒绝 disabled/ineligible 档位；任何套餐继续忽略 `model_limits`。

## Gate C：Credit 兑换真实垂直链路

- [ ] 从现有兑换 API/领域入口开始，成功事务锁定所选充值档位并验证 enabled、非试用、正价格、正 Credit、允许不限时购买等资格。
- [ ] 来源快照冻结权威 `price_amount_micros`、档位 Credit 分母、原币种、入账时 FX、规则版本和稳定来源身份；后续改价、改币种或改 Credit 不回写既有价值。
- [ ] 兑换记录、来源终态、Credit 数量、估值状态和低频 ledger 同事务提交；每个故障注入点均证明整笔回滚。
- [ ] 相同来源/幂等键与相同参数重放原结果，不重复入账；同身份但数量、档位、价格、币种、FX 或规则不同返回稳定 idempotency mismatch。
- [ ] ingress 先抵扣 settlement debt；全额偿债时 `net_credit=0` 且价值不增加，部分偿债时只有净 Credit 与按整数 floor 分摊的净成本进入 exact 状态。
- [ ] Credit 兑换不产生邀请奖励，也不进入邀请付费统计；领域/API/分析测试均有证据。

## Gate D：管理员售后 increase API 合同

- [ ] 扩展现有 adjustment 接口：increase 必须提交正 `amount`、合格 `plan_id`、非空 reason 和稳定 `idempotency_key`；`plan_id` 进入完整参数指纹。
- [ ] 缺 plan、trial/disabled/零价/零 Credit/不允许购买档位、无效币种/FX、溢出、状态不一致均返回稳定 code 并原子回滚。
- [ ] 任意正 Credit 以整数 `floor(price_amount_micros × amount / plan_credit)` 计算毛成本；跨币种使用冻结有理数 FX，服务端 micros 字符串为权威结果，不依赖 JS/Go 二进制浮点。
- [ ] increase 响应返回毛/净 Credit、毛/净 `amount_micros`、估值币种、源币种、confidence、FX、规则版本和 `state_version_after`，精确 micros 使用字符串。
- [ ] 同 key/同参数重放原结果；同 key 但 operation、plan、amount、价格、FX、currency、规则版本或最终合同指定的其他指纹字段变化时稳定拒绝。
- [ ] increase 与兑换复用同一 ingress，数量、状态、来源终态和 ledger 同事务；全额/部分 debt offset 结果与兑换一致。
- [ ] decrease 请求不得携带 `plan_id`，但本切片没有实现其移动平均出账或破坏性恢复。

## Gate E：低频账本与分析可观察性

- [ ] `CreditBalanceLedger` 以结构化列保存来源类型/键/ID、plan ID、gross/net Credit、gross/net cost micros、估值/源币种、FX numerator/denominator/captured_at、confidence、规则版本、状态版本及参数指纹；JSON 只能补充。
- [ ] ledger/source 唯一约束与状态写入同事务；唯一冲突后的读取只能重放完全相同结果，不能覆盖已提交事实。
- [ ] 五个运营分析接口在兑换/increase 后显示正确 available、debt、exact、总值及来源；测试不能只断言 ledger 已插入。
- [ ] 32 CNY 示例通过真实管理员入口：`40 CNY / 1,000 Credit × 800 Credit = 32,000,000 micros CNY`；全额抵债案例显示价值零增长。
- [ ] unsupported currency、无效 FX 与 cross-currency floor 边界均有稳定错误/精确结果测试，现存状态不动态重估。

## Gate F：管理员 UI、六语言与真实浏览器

- [ ] increase 模式只加载并展示合格充值档位，要求选择档位，展示标价、档位 Credit、原币种与服务端权威运营价值预览；无档位不能提交。
- [ ] 800 Credit、40 CNY/1,000 Credit 的预览显示 32 CNY；请求保留原始精确整数/字符串，不因紧凑显示改变 payload。
- [ ] 可控失败后的重试复用同一幂等键；成功后，或 operation/plan/amount 等业务参数变化后生成新 key。
- [ ] 切换到 decrease 时隐藏并清除 plan/预览状态，真实请求 payload 不含 `plan_id`；切回 increase 不泄漏旧档位或旧预览。
- [ ] 文案准确使用“运营剩余价值”“售后授予”，不称为实收、退款额、负债或可退款余额；en、zh、fr、ru、ja、vi 无 missing/extras。
- [ ] 启动真实前端和 API，以真实浏览器完成选档、预览、提交或受控失败重试、切换 operation，并记录实际请求/响应和可见结果。静态资源拦截不能替代 API 行为。

## Gate G：数据库、回归与放行

- [ ] 真实 SQLite 领域/API tracer 分别通过真实兑换入口和管理员 adjustment API 入账，覆盖冻结事实、幂等冲突、事务回滚、debt offset、邀请隔离和分析明细。
- [ ] schema、命名唯一约束和事务/锁语义经跨库静态审查；未把 DryRun 声称为 MySQL/PostgreSQL 验收，完整三库矩阵明确归 #27。
- [ ] 运行受影响 Go/前端定向测试、真实 SQLite/API/browser smoke、typecheck/build、i18n 检查及 `git diff --check`。
- [ ] #20 精确价格、#21 timed 多币种、#22 购买/32 CNY 主 tracer 均保持通过；#23 若已集成，其 request 分支也不得被低频 ingress 改写。
- [ ] 完成报告逐条映射 GitHub Issue #24 acceptance criteria，列出提交 SHA、领域/API/UI 合同、命令、关键输出、未运行范围和风险。
- [ ] 协调器只有在全部证据成立后才能 non-ff merge；失败返回原 Worker 修复，不在协调器分支临时重写业务逻辑。

## 不放行条件

- controller 或 UI 自行计算/声明权威价值、confidence 或 FX；
- 使用兼容 float、当前 Credit 容器价格或实际支付额推导来源价值；
- 只写 ledger、不原子更新物化状态，或只测直接插表；
- disabled/trial/零价档位仍能兑换或 increase；
- 重试生成新幂等键，或参数变化仍复用旧结果；
- decrease 泄漏 `plan_id`，或越界实现其估值出账；
- 六语言、真实 SQLite、真实 API/browser 或分析明细证据缺失；
- 越界实现 #23、#25–#28。
