# Issue #27 实现 Agent 指令

## 目标与垂直交付

你负责父 PRD #19 的 GitHub Issue #27「完成历史迁移、三数据库验证与门禁就绪」。必须在 Orca 为你创建的隔离子工作树中，将 #20–#26 已集成的前向精确价格、Credit/timed 估值、request settlement、正向/破坏性入账、conversion 和 FX 合同收束为可预演、可重跑、可验证的历史迁移与 fail-closed 门禁。

这是永久 feature，不是只写一个 SQL 脚本或让某个 fixture 通过。你必须贯通维护子命令、无副作用数据库初始化、历史价格精确回填、Credit/计时历史重建、marker 状态机、ready 后所有写路径门禁、稳定可观测错误以及真实 SQLite/MySQL 5.7/PostgreSQL 9.6 零 SKIP 验收。严格禁止越界：不得部署生产、构建发布镜像、切换外部流量或执行 #28；不得回到 #20 修改历史所有权，也不得重新实现 #21–#26 的运行时业务逻辑。

## 必读材料与 Skill

修改前依次阅读并服从：

1. 仓库及全局 `AGENTS.md`。
2. `issue://jiwangyihao/new-api/19` 与 `issue://jiwangyihao/new-api/27`；GitHub CLI 始终显式传 `--repo jiwangyihao/new-api`。
3. `docs/agents/credit-operational-value-execution.md`。
4. `docs/agents/credit-operational-value-wave-4-contract.md`；你是历史迁移、真实三数据库矩阵和 marker 门禁的唯一主改者。
5. 已集成 `.scratch/agent-progress/issue-20` 至 `issue-26` 的 `contract.md`、`evidence.md` 和最终代码。先确认所有数量 writer、来源快照、request identity、FX、conversion 与 recovery 接缝已存在；缺失时立即 Orca `orchestration ask`，不得复制上游切片。
6. `CONTEXT.md`、ADR 0001、ADR 0002。
7. 新规格第 11、13、14、15 节，以及第 7–10 节中迁移需要理解的来源和状态不变量。
8. 实施计划任务 7、任务 10，以及任务 2 中只属于历史价格文本回填/门禁的步骤。注意：旧计划若把历史回填写在任务 2，以修正后的 Issue #20/#27 所有权为准，历史回填只属于本 Issue。

必须先读取并执行 `skill://tdd`：从真实命令、数据库状态和业务门禁的可观察失败开始。迁移重跑、方言、锁或并发异常难以定位时读取 `skill://diagnosing-bugs`，先建立最小真实复现，不用日志猜测。维护入口或 CreditValuation 深模块边界需要改变时读取 `skill://codebase-design`，但不得改写 ADR/spec。若实际触及 tiered/dynamic billing expression，先读 `pkg/billingexpr/expr.md`；否则不要扩大范围。若必须修改用户可见 warning/UI，先读 `skill://shadcn-ui` 与 `skill://i18n-translate` 并维护 en、zh、fr、ru、ja、vi。

## 历史精确价格：本 Issue 的专属所有权

- #20 只增加精确列、前向写合同和只读非法值诊断；它明确不回填历史、不修改 marker、不决定 ready。你必须完整接管历史价格迁移。
- 按稳定套餐 ID 从数据库原始 DECIMAL/NUMERIC/SQLite 数值文本读取旧价格。不得先扫描到 `float32/float64`，不得从 JSON 兼容展示值或格式化字符串反推。
- 严格接受最多六位小数且可精确表示为 int64 micros 的非负值；检测负数、超精度、指数/特殊值、溢出、无法恢复和数值往返不一致，并输出稳定 plan ID/reason。
- dry-run 只诊断；apply 按稳定主键批次写 `price_amount_micros`；verify 要求所有需估值的历史有价套餐均已精确回填。不得舍入、截断、静默写零或自行猜值。
- 任一非法历史价格必须让迁移保持 failed/非 ready。运维只能通过 #20 前向精确价格接口显式修正，再重新 dry-run/apply/verify；`repair-missing-as-unknown` 不能修补价格。
- 为 SQLite、MySQL 5.7.44 和 PostgreSQL 9.6.24 使用明确、可测试的方言读取方式；保留数据库兼容规则，不依赖 PostgreSQL-only 或 MySQL-only 语法而无 fallback。

## Credit 与 timed 历史重建

- 迁移在写流量停止时按 `user_subscription_id` 稳定排序。合并 ledger、订单履约快照、兑换 fulfillment、conversion 和管理员来源时只按结构化 `(source_type, source_id/source_key)` 去重，禁止 `(user_id,plan_id)` 最近订单猜测。
- 对每份 Credit 权益计算最终 `A=max(token_limit-token_used,0)` 与债务。对可证明数量和成本的净正向来源求 `K/C`，数量可证明但成本未知的来源求 `U`，令 `T=K+U`、`R=min(A,T)`；若 `T>0`，迁移成本为 `floor(C×R/T)` 且全部进入 estimated，unknown 为同比分配的 U 加 `A-T` 正部。
- 无法证明可靠总分母时，写 `available_credit=A`、金额 0、`unknown_credit=A`。不能只用已知来源放大成本，不能重放请求日志、低频出账顺序或套餐改价历史。
- 历史异币种只支持 CNY/USD，冻结迁移启动时同一份有理数 FX；其他币种对应 Credit 进入 unknown。即使来源看似完整，历史 Credit 仍是 estimated，不得伪装 exact。
- timed grant 恢复优先级固定为订单 `FulfilledSubscriptionID + EntitlementSnapshot`、兑换 fulfillment、明确管理员记录、能唯一证明来源与服务窗口的事件；其余 unknown。一对多歧义不得选择最近订单。
- 已由前向版本写入且来源/窗口完整的 exact timed grant 不被覆盖；迁移只补缺失历史 estimated/unknown。

## 维护命令与初始化

实现根二进制早期子命令：

```text
/new-api credit-valuation-migrate (--dry-run|--apply|--verify|--repair-missing-as-unknown|--suspend) --version N [--batch-size N] [--reason TEXT]
```

- 五种模式互斥；`--batch-size` 只适用于 apply；`--reason` 只适用于 suspend 且必填；无效组合返回稳定非零退出码和结构化错误。
- 从 `InitDB` 抽出仅建立 DB/Option 所需连接、且不运行无关 migration 的维护初始化。命令结束后退出，不能启动 HTTP、Redis、定时器、同步器、队列或后台轮询。
- dry-run 与 verify 完全只读。用防写测试/数据库权限或事务证据证明；不能以“代码看起来没有写”代替。
- 启动时只读取一次 Option 原始 `USDExchangeRate`，复用 #26 严格 parser，冻结到 marker/输出；不得由可变 float 反推或在批次间重读。
- 输出稳定 JSON：版本、Credit 币种、FX、套餐价格诊断、exact/estimated/unknown 行数/金额/Credit、歧义原因、blocker、稳定批次边界和 checksum。运行时间等非业务字段不进入 checksum；同一快照两次输出的业务 JSON/checksum 必须相同。

## marker、重跑与 fail-closed

- marker 使用明确 `pending/running/ready/failed/suspended` 状态和版本 CAS。apply 按稳定主键批次、可重跑 upsert；只覆盖同版本且未被前向写改变的迁移行。
- 同版本 ready 重放为无操作；running/failed 只有在写流量停止、checksum 与预演一致时才能从最后稳定主键继续。ready 后发生前向写，旧版本不得覆盖。
- apply/verify 在进入 ready 前必须确认没有非终态预扣、仍会回调的订阅资金异步 Task 或可写旧进程会话；每类 blocker 返回稳定 reason，不能猜测在途成本。
- verify 原子检查：历史价格已精确回填；每份 Credit 权益一行状态；可用量、币种、非负、unknown 上界、状态版本一致；无重复/missing 来源与 timed grant；checksum 匹配。任一失败保持 failed，不允许部分 ready。
- 只有完全不存在套餐权益、成功订单、已兑换套餐和管理员授予历史的全新数据库，才能在 HTTP 启动前同事务自动创建 ready。不能把“没有 Credit”误当“没有 timed 历史”。
- ready 后每个 Credit 数量 writer 必须由深模块同锁同事务更新估值；状态缺失/数量不一致整笔失败，禁止热路径补 unknown、自建状态或回退旧 delta。
- ready 后历史 Task 缺 request ID 时，只能复用 #23 的持久 Task 主键 legacy identity：追加按当前平均值，退款新可用量 unknown，且仍同事务更新数量和状态。
- repair 只允许停写维护窗口、显式新 migration version 下补缺失 Credit 状态为 unknown，记录 critical 审计并要求重新 verify；不能修价格、覆盖现有状态或成为 HTTP 降级。
- suspend 只允许停写维护下从 ready 携带 reason 原子进入；suspended 时正常 HTTP 写保持关闭，只允许只读验证、修复或新版本迁移。

## 真实三数据库与并发矩阵

同一验收矩阵必须在真实 SQLite、MySQL 5.7.44 和 PostgreSQL 9.6.24 上运行，最终证据三者 PASS 且零 SKIP。DryRun、mock driver 或仅 schema 反射不能替代：

- schema、BIGINT、命名唯一约束、不可变 hook、行锁和方言文本读取；
- 历史价格合法/非法/边界回填、两次 dry-run checksum、apply 中断续跑、幂等重放、verify 和 repair/suspend；
- purchase/redemption/admin/conversion grant、consume、request settle/refund、recovery 和五分析接口；
- grant+grant、grant+consume、consume+restore、conversion+settlement、refund+admin decrease 的合法串行化集合；
- ready 后 state missing/mismatch fail-closed 与 legacy Task 幂等；
- frozen `40 CNY / 1,000 Credit`、消费 200、`end_time=0`，五接口均为 32,000,000 micros CNY、active count 1、estimated 0、unknown 0；
- 定向 Go `-race` 覆盖算术、合并器和门禁缓存，但不能代替数据库并发。

复用项目 `TEST_MYSQL_DSN`/`TEST_POSTGRES_DSN` 惯例。测试本身在开发机环境缺失时可以明确 SKIP，但本 Issue 的完成证据必须提供真实三库零 SKIP；不得伪造输出或把容器版本替代目标版本。服务型数据库进程使用监督式进程工具，不用失控后台 shell。DSN/密码不得写入提交或 Orca 消息。

## 崩溃恢复与交接

第一项实际改动必须创建并提交：

- `.scratch/agent-progress/issue-27/status.md`：阶段、已完成项、下一步、阻塞、最近安全提交、当前 migration version；
- `.scratch/agent-progress/issue-27/evidence.md`：RED/GREEN、命令模式、checksum、blocker、三库版本/PASS、并发/race 结果和失败修复；
- `.scratch/agent-progress/issue-27/contract.md`：schema、CLI argv/退出码、marker 状态机、批次/CAS、价格 parser、历史公式、锁序和稳定错误；
- `.scratch/agent-progress/issue-27/release-handoff.md`：严格按 wave-4 合同整理给 #28 的脱敏运行合同与证据索引。

每个可编译、可验证小步立即 Conventional Commit 并更新恢复文件。迁移输出和夹具尽快落盘，不能只留在终端或一段大脚本。修改导出符号前使用 LSP references。意外中断前尽可能记录当前 marker、已完成稳定主键、checksum、未提交文件和下一条命令。

## 验证与完成条件

只运行本切片定向测试、真实三数据库矩阵及必要 CLI/API smoke；不要运行项目级全套、前端全套或部署，它们由 #28/最终阶段统一执行。至少实际执行固定 SQLite fixture 的 dry-run 两次、apply、verify、重放、非法历史价格、blocked ready、repair、suspend 和恢复路径；对 MySQL/PostgreSQL 执行同一矩阵且零 SKIP。

格式化明确修改文件并执行 `git diff --check`。完成前逐条复核 Issue #27 的 17 条 acceptance criteria，提交全部代码与恢复/交接记录并保持工作树干净。随后在当前 Dispatch 只发送一次 `worker_done`，列出提交 SHA、CLI/schema/marker/历史算法合同、SQLite/MySQL/PostgreSQL 精确版本和零 SKIP、并发/race、32 CNY 五接口、共享文件、风险及证据路径；明确声明未部署生产、未实现 #28。不要关闭 Issue、合并或回收工作树，等待协调器验收。
