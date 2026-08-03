# Issue #25 独立验收门禁

## Gate A：Worker 交付与可恢复性

- [ ] Orca Dispatch 只发送一次 `worker_done`；记录最终 Worker 状态、共同基线、Worker HEAD、`merge-base`、终端 ID 与失败计数。
- [ ] Worker 源自已验收并集成 #20、#22、#23、#24 的共同基线；不得从 `origin/main`、生产提交或旧 Worker 分支开工。
- [ ] `.scratch/agent-progress/issue-25/{status,evidence,contract}.md` 全部提交，且状态、锁序、终态优先级、幂等指纹、RED/GREEN 命令和遗留风险与代码一致。
- [ ] 每个可验证小步均为 Conventional Commit；工作树干净，关键事务、并发、API、browser 与分析证据不只存在于终端或临时脚本。
- [ ] 修改导出符号前的 LSP references、现有 recovery/adjustment/支付终态入口清单及最终调用迁移清单可追溯。

## Gate B：切片所有权与非目标

- [ ] #25 只负责管理员 Credit decrease、Credit 订单退款、拒付、财务恢复、低频破坏性 outflow、来源终态、邀请取消及相应 API/UI/i18n。
- [ ] 只消费 #23 的 request snapshot/restore 合同和 #24 的 adjustment/ledger/ingress 合同；不复制移动平均核心，不直接在 controller/service 修改数量或 `CreditValuationState`。
- [ ] 低频 outflow 不读取、重算或改写活动 `SubscriptionPreConsumeRecord` 成本快照；请求退款仍由 #23 按请求自己的冻结快照恢复。
- [ ] 未实现 #26 的 timed→Credit conversion、运行时 FX、转换期间虚拟快照，未实现 #27 的历史迁移/ready，也未执行 #28 的发布。
- [ ] 未为 timed 退款增加服务撤销或 grant reversal；已禁用套餐的既有权益仍可消费，已授权 recovery 不因套餐后来 disabled 而失效。

## Gate C：混合池破坏性 outflow

- [ ] 公开领域入口在同一事务及固定锁序下更新目标 `UserSubscription`、`CreditValuationState`、结构化 ledger 与来源终态；任一故障注入均证明整笔回滚。
- [ ] 操作前以 `A=max(token_limit-token_used,0)`、`C=min(Q,A)` 计算，只对 `C` 按池平均分别 floor 移除 exact、estimated、unknown；`C=A` 时吸收所有舍入余数。
- [ ] `Q-A` 只形成 settlement debt；余额为零、部分不足和完全不足时成本始终非负，不凭空移除 exact/estimated/unknown。
- [ ] 管理员 decrease、退款、拒付和财务恢复都不按订单原价、支付额、退款额、充值档位或来源批次撤值；来源事实仅用于身份、审计、终态和邀请取消。
- [ ] outflow 与请求 settle/refund 并发遵守统一锁序；结果属于明确列出的合法串行化集合，活动请求快照在低频 outflow 前后保持不变。
- [ ] 五个运营分析接口在 outflow 后立即反映 available、exhausted、debt、exact/estimated/unknown 与 active paid count；零可用量或仅有 debt 不计 active paid count。

## Gate D：来源终态、幂等与邀请隔离

- [ ] ledger 结构化保存 source type/id/key、operation、gross Credit、consumed available、debt formed、removed exact/estimated/unknown、currency、rule/state version、参数指纹与终态；JSON 仅作补充。
- [ ] 相同来源身份和完全相同参数重放返回原结果且不增加状态版本；相同 key/来源但 operation、数量、目标权益、终态、规则版本或指纹不同稳定拒绝。
- [ ] 同一订单的 refund/chargeback/financial recovery 采用仓库既有终态优先级，重复 webhook、并发回调和进程重启至多发生一次实际回收。
- [ ] 唯一冲突后只可读取并重放完全一致的持久化结果；不得覆盖已提交来源事实或依赖进程内布尔值。
- [ ] Credit recovery 不产生邀请收益且不进入邀请付费统计；需取消既有错误奖励时，取消、来源终态、数量和价值同事务且幂等。
- [ ] disabled-plan recovery 边界有正反测试：恢复已授权订单允许，任何借 recovery 新建分配仍拒绝。

## Gate E：管理员 decrease API 与 UI

- [ ] 复用 #24 adjustment API；`decrease` 要求正 `amount`、非空 reason 和稳定 `idempotency_key`，请求携带任何 `plan_id` 都返回稳定 code 并原子回滚。
- [ ] 响应以字符串返回精确 micros，并包含 gross Credit、consumed available、debt formed、removed exact/estimated/unknown、currency 与 `state_version_after`；兼容浮点字段不参与算术。
- [ ] 同 key/同参数重放原结果；operation、amount、reason 或其他指纹字段变化后必须使用新 key，错误复用稳定拒绝。
- [ ] UI 从 increase 切换到 decrease 时隐藏并清空 plan、价格及预览；实际 payload 不含 `plan_id`，切回 increase 也不泄漏旧选择。
- [ ] 可控失败后的同参数重试复用同一 key，成功或业务参数变化后生成新 key；浏览器证据记录实际请求与响应。
- [ ] 文案准确区分运营剩余价值、回收、欠额和成本未知，不称为现金退款或负债；en、zh、fr、ru、ja、vi 无 missing/extras。

## Gate F：真实入口、数据库并发与 race

- [ ] 真实 SQLite tracer 通过管理员 adjustment API 和至少一个真实订单 recovery 入口执行 outflow，再读取目标状态、ledger、来源终态和五个分析 API；不得直接插表冒充主路径。
- [ ] 定向测试覆盖普通比例、混合 confidence、完全清空余数、部分/完全欠额、零可用量、算术溢出、稳定错误及每个事务失败点。
- [ ] 并发覆盖 refund+chargeback、同 key 重放/冲突、outflow+request settle、outflow+request refund；断言数量、成本、版本、终态和 ledger 只属于合法串行结果。
- [ ] 相关定向包的 Go `-race` 通过，但 race 结果不替代真实数据库并发证明。
- [ ] 启动真实应用/API 与真实浏览器完成 increase→decrease 切换、无 plan payload、成功或受控失败重试和分析刷新；静态资源拦截不能替代 API 行为。
- [ ] SQL、命名唯一约束和锁语义完成跨库静态审查；未把 GORM DryRun 声称为 MySQL/PostgreSQL 验收，完整三库零 SKIP 保留给 #27。

## Gate G：集成前回归与放行

- [ ] 运行受影响 Go 包、真实 SQLite tracer、API/browser smoke、定向并发/race、前端测试、typecheck/build、六语言检查和 `git diff --check`。
- [ ] #20 精确价格、#22 冻结 32 CNY Credit、#23 request restore、#24 redemption/increase 与 debt offset 全部保持通过；#21 timed 多币种不因 Credit recovery 改变。
- [ ] 完成报告逐条映射 GitHub Issue #25 acceptance criteria，列出提交 SHA、领域/API/UI 合同、命令、关键输出、未运行范围与 #26 共享文件风险。
- [ ] 协调器只有在所有门禁有证据后才能 non-ff merge；失败项返回原 Worker 修复，不得在协调器分支临时重写领域逻辑。

## 不放行条件

- 按订单原价、实收或退款额撤回 Credit 池价值；
- controller/service 先改数量，再异步或尽力补写估值；
- outflow 回写活动请求快照，导致请求退款使用当前池平均或恢复后来 ingress 成本；
- refund/chargeback 并发可重复回收，或终态只存在进程内；
- decrease 接受/泄漏 `plan_id`，失败重试生成新幂等键，或前端解析错误文本；
- timed 退款被错误实现为服务/grant 撤销；
- 只有 mock、直接插表或 DryRun，没有真实 SQLite、API/browser 和并发证据；
- 越界实现 #26–#28，或削弱 #20–#24 已集成合同。
