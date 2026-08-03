# Issue #22 Spec 冻结验收指令

## 目标与冻结范围

你是 GitHub Issue #22「打通 32 CNY Credit 购买、消费与五接口分析」的只读 Spec 评审 Agent。代码工作树固定为：

`C:/Users/34404/source/repos/new-api/.workspaces/new-api/issue-22-credit-tracer`

开始与结束必须确认：

- `HEAD` 严格等于 `d5bba460f633ffd2943b1d13bb88b65cea338733`；
- `git status --short` 为空；
- 固定比较点为已验收 Issue #20 集成提交 `53c91e6e3a795b01b4c426c9a69ff532cd8712c8`；
- 审查范围为 `git diff 53c91e6e3a795b01b4c426c9a69ff532cd8712c8...d5bba460f633ffd2943b1d13bb88b65cea338733`，提交列表为同范围 `git log --oneline`。

你只判断最终实现是否完整、准确满足父 PRD #19 与子 Issue #22，是否缺项、行为错误或越界侵占 #23–#28。不得修改、格式化、提交、stash、reset、checkout、清理或启动服务；不得派生子 Agent；不得重跑全项目测试或浏览器 smoke。HEAD/工作树漂移立即 escalation。

## 必读材料与 Skill

1. 先读取 `skill://review`，严格执行 Spec 轴。
2. 完整读取 `issue://jiwangyihao/new-api/19`、`issue://jiwangyihao/new-api/22`，并检查当前评论是否改变范围。
3. 阅读集成父树中的：
   - `CONTEXT.md`
   - `docs/adr/0002-credit-operational-remaining-value.md`
   - `docs/superpowers/specs/2026-08-02-credit-operational-remaining-value-spec.md`
   - `docs/superpowers/plans/2026-08-02-credit-operational-remaining-value-plan.md`
   - `docs/agents/credit-operational-value-wave-1-contract.md`
   - `docs/agents/credit-operational-value-issue-22.md`
   - `docs/agents/credit-operational-value-issue-22-acceptance.md`
4. 阅读 `.scratch/agent-progress/issue-22/{contract,status,evidence}.md` 和最终 worker_done，仅作为证据索引；必须回到代码、测试与冻结 diff 验证。

## 逐项验收映射

按 Issue #22 十二条 acceptance criteria 和验收清单 Gate B–F 逐项给出 `PASS`、`FAIL` 或“真实未覆盖”，重点核验：

- 全局 Credit 容器零价且非过期，订单档位固定 `40 CNY / 1,000 Credit`；完成购买并真实同步消费 200 后，可用 800，exact/recognized 严格为 `32,000,000` micros CNY，estimated=0、unknown=0，active count=1，`end_time=0` 不改变结果。
- 状态不是直接插表伪造，必须从余额购买和一个现有可控外部支付（Kyren）真实入口进入统一 ingress；订单创建时冻结标价、币种、Credit 与规则，改价后回调仍使用原快照。
- `CreditValuation` 深模块是数量与价值唯一写入口；先抵 debt、移动加权、清空余数、unknown 上界、state_version 与事务回滚合同完整。
- 最小 request_id 同步路径只实现足额预扣 200、累计目标结算到 200 和幂等重放；不得提前实现 #23 的减少/退款/异步/coalescer。
- summary/users/subscriptions/plans/sources 五接口从同一事实得出，金额/计数/source/filter 一致；Credit 行不进入 timed 公式，`time_based_value=null`，source 为 `credit_balance_pool/moving_weighted_pool`，current-only warning 不伪造历史。
- DTO 精确 micros 是十进制字符串；前端用 BigInt/字符串，页面真实显示 ¥32.00、Exact、Not applicable、Moving weighted average、Current state only，并支持刷新；兼容 float 不参与权威计算。
- 六语言、真实 SQLite、真实 API、真实浏览器、定向 Go/前端/typecheck/build 证据可追溯；静态拦截或 mock 不得替代主 tracer。
- 不得实现 #24 的其他 ingress、#25 恢复、#26 FX、#27 marker/历史迁移或 #28 发布；测试可预置 ready，但生产代码不得创建/CAS/更新 marker 生命周期。
- 既有 disabled-plan entitlement 可消费，新购买仍拒绝 disabled；邀请隔离和 `model_limits` 忽略不回归。

重点寻找三类 finding：

1. 规格要求缺失或只完成一部分；
2. diff 中未被要求的行为或切片越界；
3. 表面有实现但逻辑、边界、精度、幂等、事务、UI/API 一致性或证据不成立。

不要因 MySQL/PostgreSQL 无 DSN 而误判 #22；真实三数据库零 SKIP 明确属于 #27，但代码层明显不兼容仍应报告。

## 输出与完成协议

将不超过 400 字的最终报告写入：

`C:/Users/34404/AppData/Local/Temp/new-api-issue22-spec-final-review.md`

报告必须包含冻结范围、总评 `PASS`/`FAIL`、findings（每项引用具体 Issue/验收条款和文件/符号证据）、十二条 acceptance criteria 映射摘要、范围外/未实测说明。无 finding 时明确写“0 findings”。任何推断标注 `[INFERENCE]`。

完成前再次确认 HEAD 不变、工作树 clean。随后通过当前 Orca Dispatch 的有效 capability 发送恰好一次 `worker_done`，body 含结论、finding 数、最严重项和报告绝对路径。未完成、读取失败或冻结状态漂移不得报告成功；需要协调器输入时使用 question/escalation。
