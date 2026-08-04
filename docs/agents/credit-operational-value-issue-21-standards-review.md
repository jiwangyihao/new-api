# Issue #21 Standards 冻结验收与 #22 兼容性评审指令

## 目标、冻结现场与只读边界

你是 GitHub Issue #21「固化计时权益 grant 时间线与多币种分析」的只读 Standards 评审 Agent。代码工作树固定为：

`C:/Users/34404/source/repos/new-api/.workspaces/new-api/issue-21-timed-grants`

开始与结束时都必须确认：

- `HEAD` 严格等于 `547512242578ec198034d322875c5485735b247a`；
- `git status --short` 为空；
- #21 的共同派生基线为已验收 #20 集成提交 `53c91e6e3a795b01b4c426c9a69ff532cd8712c8`；
- 当前集成父树已先合并 #22，冻结 HEAD 为 `ac830971a32e24f5b88c42b312d62fffd4229e21`；
- #21 自身审查范围固定为 `git diff 53c91e6e3a795b01b4c426c9a69ff532cd8712c8...547512242578ec198034d322875c5485735b247a`，提交列表使用同一范围的 `git log --oneline`；
- 共享 DTO/analytics 兼容性必须额外对照 `ac830971a32e24f5b88c42b312d62fffd4229e21` 中的最终 #22 通用 Credit 骨架，但不得修改、merge、rebase、checkout 或生成提交。

你只判断最终 diff 是否违反仓库记录的工程规范，以及它能否安全接入已集成 #22 的通用 DTO/analytics seam。不得编辑、格式化、提交、stash、reset、清理、启动服务、写数据库或派生子 Agent；不得重复运行大套件或浏览器 smoke。HEAD 或工作树漂移时立即 escalation，禁止继续在非冻结状态评审。

## 必读材料与 Skill

1. 首先读取 `skill://review`，只执行 Standards 轴。
2. 阅读自动注入的项目级和全局 `AGENTS.md`，重点遵守：`common/json.go`、SQLite/MySQL/PostgreSQL 同时兼容、显式零值、Bun、六语言 i18n、受保护身份、验证证据不得夸大。
3. 阅读集成父树中的：
   - `docs/agents/credit-operational-value-execution.md`
   - `docs/agents/credit-operational-value-wave-1-contract.md`
   - `docs/agents/credit-operational-value-wave-1-acceptance.md`
   - `docs/agents/credit-operational-value-issue-21.md`
   - `docs/agents/credit-operational-value-issue-21-acceptance.md`
   - `CONTEXT.md`
   - `docs/adr/0002-credit-operational-remaining-value.md`
   - `docs/superpowers/specs/2026-08-02-credit-operational-remaining-value-spec.md`
4. 阅读当前 #21 工作树的 `.scratch/agent-progress/issue-21/{contract,status,evidence}.md`，但这些只是证据索引，不得替代代码审查。
5. 对共享文件，使用只读 `git show ac830971a32e24f5b88c42b312d62fffd4229e21:<path>` 对照已集成 #22 版本；不得机械建议 ours/theirs，必须按所有权判断语义。

## Standards 审查重点

逐文件、逐 hunk 审查，至少覆盖以下方面：

### 领域与数据库

- `TimedSubscriptionValuationGrant` 的唯一约束、不可变 update/delete hook、来源身份和字段类型是否兼容 SQLite、MySQL 5.7.8+、PostgreSQL 9.6；不得把 DryRun 或无 DSN SKIP 当真实三库 PASS。
- `GrantTimedSubscriptionTx` 是否是唯一窄入口；权益创建/续期与 grant 是否同事务；幂等重放、参数冲突、disabled/trial、邀请排除是否原子且稳定。
- 锁序、重试和失败回滚是否安全；不得引入数据竞争、sleep 驱动并发测试、文本错误码分支、浮点金额或按当前 Plan 价格补猜。
- 订单、兑换、管理员来源是否使用不可变来源事实；显式 micros、币种与零值是否保真；普通路径不能更新/删除 grant。
- 既有 disabled-plan entitlement 消费、邀请隔离、`model_limits` 忽略、已授权订单履约不得回归。

### timed 算法与五接口

- 计算是否只读 grant 时间线，使用整数 micros 与防溢出 helper；实际 `subscription.end_time` 裁剪、当前周期 Credit 比例、未来周期、零额度、边界秒是否遵循既有 reset 语义。
- 重叠窗口必须按稳定最早 grant 去重，缺口/歧义必须产生稳定 warning/unknown；不得静默变 exact。
- CNY/USD 等原币种独立聚合；跨币种 singular 必须为 `null`；source breakdown/mixed_grants、五接口 totals/filter/sort 必须从同一 paid row 事实派生。
- 代码不得越权重写 #22 已集成的权威 `amount_micros` 排序、Credit current_only warning、Credit paid row 或通用 BigInt 格式化器。

### 与已集成 #22 的共享 seam

重点检查 `dto/admin_analytics.go`、`model/admin_analytics_paid_subscription.go`、前端 analytics types/panel fields 与 locale 文件：

- #22 的通用 DTO、Credit 分支、权威 micros、current_only warning、32 CNY 结果必须保留；#21 只增加 timed calculator、`*_by_currency`、timed warnings/unknown/source attribution。
- 若 #21 分支基于旧 DTO 形状，指出具体冲突和应保留双方哪些字段/语义；不要把可机械合并的字段顺序差异误报为 blocker。
- 任何会使 #22 的 `recognized=32,000,000` micros CNY、`time_based_value=null`、active count=1 或 current_only warning 失真的共享改动都属于 blocker。

### 前端、i18n、测试与证据

- 管理员失败重试是否复用相同 key，成功/业务事实变化是否生成新 key；payload 必须冻结 micros/币种并要求 reason。
- 前端跨币种只读字符串/BigInt 和 `*_by_currency`，不能用 Number/兼容 float 参与权威计算或按当前 Plan 币种补猜。
- 所有新增可见文字必须 `t(...)`，en/zh/fr/ru/ja/vi 结构一致且不是复制英文占位。
- 测试必须防守可观察合同，真实 SQLite/API/browser 证据与代码一致；MySQL/PostgreSQL 未实测应作为范围说明，不是本切片 finding。
- 查找无谓分配、宽泛异常吞噬、未清理临时资源、弱化断言、源码文本测试或越界实现 #22–#28。

## 输出与完成协议

将不超过 500 字的最终报告写入：

`C:/Users/34404/AppData/Local/Temp/new-api-issue21-standards-final-review.md`

报告必须包含：冻结 HEAD/比较点/已集成 #22 HEAD、总评 `PASS` 或 `FAIL`、按严重度排序的 findings。每项 finding 必须包含文件/行或符号、直接证据、违反的规范来源和影响；推断标为 `[INFERENCE]`。若无 finding，明确写“0 findings”。单独给出“共享 seam 结论”，说明 #21 能否按 #22 通用骨架 + #21 timed 增量安全集成。明确 MySQL/PostgreSQL 未实测，三库零 SKIP 归 #27。

完成前再次确认冻结 HEAD 不变且工作树 clean。随后使用当前 Orca Dispatch 注入的有效 capability 发送恰好一次 `worker_done`，body 包含 PASS/FAIL、finding 数、最严重项、共享 seam 结论和报告绝对路径。未完成、读取失败或冻结状态漂移不得报告成功；需要协调器输入时发送 question/escalation。