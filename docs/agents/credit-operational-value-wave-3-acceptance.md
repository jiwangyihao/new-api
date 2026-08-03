# 第三执行波次协调器派发与集成清单（Issues #25、#26）

## 派发前门禁

- [ ] Issue #25 仅在 #23 与 #24 均收到 `worker_done`、通过独立/组合验收并 non-ff merge 后派发；共同基线同时包含已验收 #20/#22。
- [ ] Issue #26 仅在 #21、#22 与 #23 均收到 `worker_done`、通过独立/组合验收并 non-ff merge 后派发；#24/#25 不是其硬依赖。
- [ ] 若两者派发条件同时满足，可并行启动；两个子工作树必须显式从同一最新集成分支提交创建，Orca parent 必须是集成工作树而非主树。
- [ ] 集成树干净，记录共同基线完整 SHA；相关 `.scratch/agent-progress/issue-{20,21,22,23,24}` 合同与最终实现均可读取。
- [ ] #25/#26 指令、第三波共享合同和两份独立验收清单均已提交到共同基线。
- [ ] 分别创建 Orca Task；每个 `worker-start` 指令都包含父 PRD、当前 Issue、共享合同、技能、恢复文件、非所有权和验收文档。
- [ ] 两个 Worker 均为 ready/input_accepted、terminal connected/running 后才进入等待；输入停在 Paste 时只提交现有输入，不重派。

## 并行观测与接口协调

- [ ] 使用 `orchestration check --wait` 消费 question/escalation/worker_done；超时只表示没有事件，不表示失败。
- [ ] 每数轮检查 `worker-show`、有限终端 cursor 增量和 `.scratch/agent-progress/issue-<N>`；仅当 `worker-show` 明确 failed/stopped 才使用 `--retry-of`。
- [ ] #25 主改低频 outflow/recovery、管理员 decrease、refund/chargeback/financial terminal 与邀请取消；#26 主改 conversion ingress、运行时 FX 和转换 request bridge。
- [ ] `model/credit_valuation.go` 由分支而非整文件划界：#25 只扩展 outflow/recovery，#26 只扩展 conversion/FX/request bridge；双方均不得重写 #23 request 核心。
- [ ] `model/subscription.go`、预扣记录和 ledger 类型只能做向后兼容附加；需要改变已集成 observable contract 时，Worker 必须先 Orca ask 列出符号、调用点、最窄新签名与不改部分。
- [ ] 固定锁序保持：来源行 → 目标 `UserSubscription` → `CreditValuationState` → request record 或 ledger；转换后结算按稳定 ID 先锁预扣记录/转换映射，再锁目标权益/状态。
- [ ] 一个 Worker 完成不停止另一个；立即保存其完成现场并独立验收。失败项返回原 Worker 在原工作树修复，不因另一边仍运行而重派。
- [ ] 模型不可用、Paste 未提交或终端可原地恢复时优先唤醒原 Worker；先读 progress、终端和工作树，不按运行时长重复探索。

## 独立验收与集成顺序

默认顺序为 **#25 低频 outflow 先，#26 conversion/request 分支后**；若 #26 先满足硬依赖并完成，可先独立验收，但组合集成仍以可观察合同及实际共同基线为准。

1. 记录共同基线、两个 Worker HEAD、各自 `merge-base`、提交列表与工作树清洁度。
2. 按 `credit-operational-value-issue-25-acceptance.md` 完整验收 #25；失败项返回原 #25 Worker。
3. #25 通过后以 non-ff merge 集成，提交信息使用 `feat(valuation): 集成 Credit 破坏性恢复`；立即复跑 outflow/recovery、SQLite、API/browser 与并发门禁。
4. 按 `credit-operational-value-issue-26-acceptance.md` 在 #26 原分支完成独立验收；确认其只消费稳定 ingress/request seam，不依赖 #25 私有实现。
5. 以 non-ff merge 集成 #26。若冲突，保留 #25 的 recovery/terminal/admin decrease 合同，将 #26 的 conversion/FX/request bridge 接入现有窄接口；禁止机械选择 ours/theirs。
6. 若冲突解决要求改变任一 observable contract，中止或暂停 merge，向对应原 Worker 发最小修复要求；协调器不暗中重写大段领域逻辑。
7. 第二次 merge 后执行完整组合回归；任一切片破坏另一切片或上游合同，则相关 Issues 均保持 OPEN，直到原 Worker 修复并重新验收。

## 组合回归矩阵

- [ ] 真实 SQLite 按稳定顺序执行：购买 Credit → request 预扣 → timed→Credit conversion → request 少结算/追加 → 管理员 decrease 或订单 recovery；每一步数量、价值、状态版本与审计一致。
- [ ] 转换前已预扣量不重复进入 gross Credit，转换后首次 settle 不二次扣目标池；之后低频 outflow 不回写虚拟/活动 request snapshot。
- [ ] 混合 exact/estimated/unknown 池在 conversion ingress、request restore 和 recovery outflow 后仍遵守 floor/清空余数规则，成本永不为负。
- [ ] 并发覆盖 conversion+settlement、outflow+settlement、outflow+request refund 和重复 recovery；结果属于合法串行化集合，锁序无反转或死锁。
- [ ] 普通 ingress/request/outflow 与 conversion/FX 各自的幂等身份互不污染；同参数重放无状态版本增长，冲突参数稳定拒绝。
- [ ] CNY/USD 冻结 FX 后配置更新不回写 conversion 或现存 Credit；同币种 1/1，unsupported currency 稳定拒绝。
- [ ] 五个运营分析接口同时正确显示 Credit purchase/redemption/increase、request 出账恢复、recovery outflow 和 conversion 的 available/debt/exact/estimated/unknown/币种来源。
- [ ] 32 CNY 冻结 tracer 仍为 `32,000,000` micros CNY；timed 多币种仍分币种，Credit `time_based_value` 仍为 null。
- [ ] disabled-plan 既有权益消费与已授权 recovery 保持可用；新兑换/increase/conversion 仍拒绝 disabled/ineligible；模型范围忽略和邀请隔离成立。
- [ ] 管理员 decrease browser 流程、conversion quote/confirm browser 流程及一条转换期间真实 request smoke 均通过，payload 保留原始整数/字符串。
- [ ] 运行受影响 Go 包、真实 SQLite tracer、API/request smoke、定向并发/race、前端测试、typecheck/build、六语言检查和 `git diff --check`。
- [ ] 未把 #27 才拥有的历史回填、ready/suspended 或真实 MySQL/PostgreSQL 零 SKIP，以及 #28 的生产发布标成已完成。

## Issue 关闭、工作树回收与 #27 派发

- [ ] #25、#26 仅在各自门禁及组合回归通过后关闭；关闭评论包含集成 SHA、关键验证和明确未运行范围。
- [ ] 最终 evidence 写入各自 progress 目录，记录独立验收、集成提交、组合回归及交给 #27 的所有 writer/schema/marker seam。
- [ ] 关闭后停止/释放对应 Worker，仅回收本 Run 创建的 #25/#26 工作树；使用 Orca 原生命令，不触碰集成树、主树、`account`、`disk` 或其他会话工作树。
- [ ] `orca worktree rm` 后确认节点消失，再运行原生 `git worktree prune`；集成树保持干净。
- [ ] #27 只有在 #21–#26 全部独立验收、组合回归、non-ff merge 并关闭后才能派发，且必须从包含全部前向 writers 的最新集成提交创建子工作树。

## 组合不放行条件

- 任一 Worker 未从满足其硬依赖的已验收集成基线派生；
- outflow、conversion 或 request 分支复制移动平均、锁序、幂等或 DTO；
- conversion 首次 settle 二次扣池，或低频 outflow 改写 request snapshot；
- 退款按订单价格撤值、转换从 float/实收推值，或 FX 动态重估；
- 合并冲突通过丢弃测试、弱化稳定错误或越界实现 #27/#28 解决；
- 32 CNY、timed 多币种、request restore、debt offset、disabled-plan 或邀请隔离任一回归失败；
- 只有 mock、直接插表或 DryRun，没有真实 SQLite、API/browser、request 与并发证据；
- 工作树不干净、恢复记录未提交或验收证据与实际命令不一致。
