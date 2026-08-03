# 第二并行波次协调器派发与集成清单（Issues #23、#24）

## 派发前门禁

- [ ] Issues #20、#21、#22 已各自收到 `worker_done`、通过独立及组合验收、以 non-ff merge 集成；GitHub Issues 已关闭。
- [ ] 集成树干净，记录共同基线完整 SHA；该基线可读取 `.scratch/agent-progress/issue-{20,21,22}/contract.md`、CreditValuation 深模块、timed grant、最小同步 request tracer、购买来源快照及五接口通用 DTO。
- [ ] #23/#24 指令、第二波共享合同与两份独立验收清单均已提交到共同基线。
- [ ] 分别创建独立 Orca Task；两个子工作树都必须显式从同一集成分支基线创建，Orca parent 必须是集成工作树而非主树。
- [ ] 每个 `worker-start` 指令包含父 PRD、当前 Issue、共享合同、技能、进度持久化、非所有权和验收文件；记录 Run/Task/Dispatch/terminal/worktree/branch。
- [ ] 两个 Worker 均为 ready/input_accepted、terminal connected/running 后才进入等待；输入停在 Paste 时只提交现有输入，不重派。

## 并行观测与接口协调

- [ ] 通过 `orchestration check --wait` 消费 question/escalation/worker_done；超时只表示没有事件，不表示失败。
- [ ] 每数轮检查 `worker-show`、有限终端 cursor 增量和 `.scratch/agent-progress/issue-<N>`；仅当 `worker-show` 明确 failed/stopped 才使用 `--retry-of`。
- [ ] #23 主改 request-aware 结算、预扣记录、合并器、Funding/Billing/quota/Task 身份传播和清理；#24 主改兑换、管理员 increase、低频 ingress/ledger 和 UI/i18n。
- [ ] 若共同触及 `model/credit_valuation.go`，#23 只能扩展 request 分支；#24 只能调用稳定 ingress。任何 seam 变更先冻结最窄接口并由协调器转发，不允许两边复制移动平均或互相重写。
- [ ] `model/credit_balance.go`、redemption、管理员 adjustment controller/UI 由 #24 主改；合并器、预扣记录、Task 私有数据和请求传播由 #23 主改。
- [ ] 一个 Worker 完成不停止另一个；立即保存其完成现场并独立验收。验收失败时让原 Worker 在原工作树修复，不因另一边仍运行而重派。
- [ ] 模型不可用、终端输入未提交等可原地恢复故障优先唤醒原 Worker；先读 progress/终端/工作树，禁止按运行时长重复探索。

## 独立验收与集成顺序

默认顺序固定为 **#23 request 核心先，#24 低频 ingress/UI 后**。

1. 记录共同基线、两个 Worker HEAD、各自 `merge-base` 与工作树清洁度。
2. 按 `credit-operational-value-issue-23-acceptance.md` 完整验收 #23；失败项返回原 #23 Worker。
3. #23 通过后以 non-ff merge 集成，提交信息使用 `feat(valuation): 集成请求级可逆 Credit 结算`；立即复跑其领域、SQLite、request smoke 与 race 门禁。
4. 按 `credit-operational-value-issue-24-acceptance.md` 在 #24 原分支完成独立验收；确认其只消费共同基线 ingress，不依赖未集成 #23 的私有实现。
5. 以 non-ff merge 集成 #24。若冲突，保留 #23 的 request/预扣/合并器/Task 合同，把 #24 的 redemption/admin ingress 与 ledger/UI 接入现有窄接口；禁止机械选择 ours/theirs。
6. 若冲突解决需要改变任一 observable contract，停止或中止当前 merge，向对应原 Worker发送最小修复要求；协调器不暗中重写大段领域逻辑。
7. 第二次 merge 后执行完整组合回归；若 #24 破坏已集成 #23，则两个 Issue 均保持 OPEN，直到原 Worker 修复并重新验收。

## 组合回归矩阵

- [ ] 真实 SQLite 按稳定顺序执行：购买 Credit → 请求预扣/追加 → 兑换或管理员 increase → 少结算/退款；请求恢复使用自身快照，低频 ingress 使用冻结档位事实。
- [ ] 组合覆盖 debt：请求追加形成 debt，后续兑换/increase 先抵债，随后请求退款只把可证明部分恢复，无法证明来源的新增可用量为 unknown。
- [ ] 合并器逐请求结果与同序逐条事务一致；并发 ingress+consume、consume+restore 结果属于合法串行化集合，数量、成本、版本和 ledger 无漂移。
- [ ] 相同 request target 与相同 low-frequency idempotency key 均严格无操作；冲突参数分别返回稳定错误，互不污染状态。
- [ ] 五个运营分析接口同时正确展示购买、请求出账/恢复、兑换和售后 increase 的 exact/estimated/unknown、available/debt、来源和币种。
- [ ] #21 timed 多币种仍按币种拆分；Credit `time_based_value` 仍为 null，32 CNY 冻结 tracer 仍得到 `32,000,000` micros CNY。
- [ ] disabled-plan 已有权益消费仍可用；新兑换/increase 拒绝 disabled 档位；模型范围忽略与邀请隔离仍成立。
- [ ] 管理员浏览器流程通过：选档、32 CNY 预览、失败重试同 key、成功/参数变化换 key、切换 decrease 后请求无 `plan_id`。
- [ ] 运行受影响 Go 包、真实 SQLite tracer、请求 smoke、定向 race、前端测试、typecheck/build、六语言检查和 `git diff --check`。
- [ ] 未将 #27 才负责的真实 MySQL/PostgreSQL 零 SKIP 或 #28 生产发布标成已完成。

## Issue 关闭与后续派发

- [ ] #23、#24 仅在各自门禁和组合回归均通过后关闭；关闭评论包含集成 SHA、关键验证及明确未运行范围。
- [ ] 最终 evidence 写入各自 progress 目录，记录独立验收、集成提交、组合回归和后续 seam。
- [ ] 关闭后停止/释放对应 Worker，仅回收本 Run 创建的 #23/#24 工作树；使用 Orca 原生命令，不触碰集成树、主树、`account`、`disk` 或其他会话工作树。
- [ ] `orca worktree rm` 后确认节点消失，再运行原生 `git worktree prune`；集成树必须保持干净。
- [ ] #25 只有在 #23+#24 均验收集成后可派发；#26 只有在 #21+#22+#23 验收集成后可派发。两者条件都满足时才可并行启动，且必须从同一最新集成基线派生。

## 组合不放行条件

- 两个 Worker 未从同一已验收 #20–#22 基线派生；
- request 分支和 ingress 分支各自复制一套移动平均、锁序或 DTO；
- 请求身份、低频来源身份或幂等结果仅保存在进程内；
- 合并冲突通过丢弃测试、弱化稳定错误或越界实现 #25–#28 解决；
- 32 CNY、timed 多币种、request restore、debt offset、邀请隔离或 disabled-plan 回归任一失败；
- 只有 mock/直接插表/DryRun，没有真实 SQLite、request/API/browser 和组合回归证据；
- 工作树不干净、恢复记录未提交或验收证据与实际命令不一致。
