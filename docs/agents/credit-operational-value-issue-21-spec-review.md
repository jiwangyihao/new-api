# Issue #21 Spec 冻结验收与 #22 组合合同评审指令

## 目标、冻结现场与只读边界

你是 GitHub Issue #21「固化计时权益 grant 时间线与多币种分析」的只读 Spec 评审 Agent。代码工作树固定为：

`C:/Users/34404/source/repos/new-api/.workspaces/new-api/issue-21-timed-grants`

开始与结束时必须确认：

- `HEAD` 严格等于 `547512242578ec198034d322875c5485735b247a`；
- `git status --short` 为空；
- #21 固定共同基线为已验收 #20 集成提交 `53c91e6e3a795b01b4c426c9a69ff532cd8712c8`；
- 当前集成父树已先合并 #22，HEAD 为 `ac830971a32e24f5b88c42b312d62fffd4229e21`；
- #21 自身范围为 `git diff 53c91e6e3a795b01b4c426c9a69ff532cd8712c8...547512242578ec198034d322875c5485735b247a`，提交列表为同范围 `git log --oneline`；
- 对共享 DTO/analytics/UI 的最终组合判断必须对照已集成 #22 HEAD，但不得修改、merge、rebase、checkout 或提交。

你只判断最终实现是否完整、准确满足父 PRD #19 和子 Issue #21，是否存在缺项、错误、证据不成立或越界侵占 #22–#28，并判断它是否能在保留 #22 通用 Credit 骨架的前提下组合。不得编辑、格式化、提交、stash、reset、清理、启动服务、写数据库或派生 Agent；不得重跑大套件或浏览器 smoke。HEAD/工作树漂移时立即 escalation。

## 必读材料与 Skill

1. 先读取 `skill://review`，严格执行 Spec 轴。
2. 完整读取 `issue://jiwangyihao/new-api/19` 与 `issue://jiwangyihao/new-api/21`，检查评论是否改变范围；读取已关闭 #22 仅用于共享合同边界。
3. 阅读集成父树中的：
   - `CONTEXT.md`
   - `docs/adr/0002-credit-operational-remaining-value.md`
   - `docs/superpowers/specs/2026-08-02-credit-operational-remaining-value-spec.md`
   - `docs/superpowers/plans/2026-08-02-credit-operational-remaining-value-plan.md`
   - `docs/agents/credit-operational-value-wave-1-contract.md`
   - `docs/agents/credit-operational-value-wave-1-acceptance.md`
   - `docs/agents/credit-operational-value-issue-21.md`
   - `docs/agents/credit-operational-value-issue-21-acceptance.md`
4. 阅读 `.scratch/agent-progress/issue-21/{contract,status,evidence}.md` 与最终 worker_done，只作证据索引；必须回到冻结 diff、实现和测试核验。
5. 对共享文件用只读 `git show ac830971a32e24f5b88c42b312d62fffd4229e21:<path>` 对照 #22 最终通用 seam。

## 逐项 Spec 验收映射

按 GitHub #21 的十一条 acceptance criteria 和协调器 Gate B–E 逐项给出 `PASS`、`FAIL` 或“真实未覆盖”，至少核验以下合同：

### 领域授予闭环

- 购买、订单完成、兑换、管理员售后授予和续期是否都经一个真实领域入口，在同一事务创建/续期权益并追加不可变 grant；不得只在测试直接插 grant 作为主证明。
- grant 是否冻结实际服务窗口、精确标价 micros、原币种、Credit、期限/重置、规则版本与结构化来源；后续 Plan 改价/改币种不得回写。
- `(source_type, source_key)` 与 idempotency key 是否确定、重放不续期、参数冲突稳定拒绝；续期追加而非覆盖。
- grant update/delete 是否在普通模型/领域路径拒绝；不得新增普通 HTTP 修改/删除或 reversal schema。
- 试用、邀请试用、邀请码明确不估值且不创建伪零价 grant；disabled 新授予拒绝，既有 disabled 权益消费不受影响。
- 管理员 API/UI 必须 reason + retryable key；同事实失败重试复用 key，成功或业务事实变化后产生新 key。

### timed 算法、五接口与多币种

- timed 估值只能读取不可变 grant；绝不按查询时当前 Plan 价格或币种补猜。
- 每条 grant 与 `[snapshot_at, subscription.end_time)` 以及真实服务窗口求交；当前周期按真实剩余 Credit 比例，未来周期完整；边界、零额度、失效裁剪有行为证明。
- 重叠秒只计最早 grant，后续重叠披露稳定 `overlapping_grants`；缺失/歧义披露 warning/unknown，不静默变 exact。
- 各原币种独立计算 time/token/recognized；recognized 为同币种两口径较小值。跨币种 singular 必须 `null`，通过精确 `*_by_currency` 返回。
- summary/users/subscriptions/plans/sources 必须从同一 timed row 对账；sources 按 grant 来源聚合，混合权益为 `mixed_grants`。
- API/SQLite 强 tracer 必须真实驱动购买/兑换/管理员/续期和五接口，不能靠 mock/静态拦截替代。

### 管理员 UI、六语言与真实浏览器

- UI 资格过滤只展示 enabled、非试用、非邀请试用、timed 且有正精确 micros 的计划；提交冻结价格/币种、reason、key。
- 真实浏览器证据必须证明一次受控失败捕获 payload/key、同事实同 key 重试成功、后续新业务事实产生新 key。
- CNY/USD 同权益在五接口和 UI 分币种显示；当前 Plan USD 不能改写历史 CNY，也不能补造 singular。
- unknown/overlap/mixed source/失效裁剪和 confidence/warning 在 UI 可见；术语必须是运营剩余价值，不得称退款、负债、实收。
- 所有新增文案覆盖 en、zh、fr、ru、ja、vi；组件测试、typecheck、build、真实 browser 与清理证据可追溯。

### 与已集成 #22 的组合合同

- #22 的通用 micros DTO、CreditValuation、Credit paid row、权威 amount_micros 排序、current_only warning 与通用 BigInt UI 归 #22；#21 只能增量接入 timed calculator、timed `*_by_currency`、timed warning/unknown 和 grant source。
- 组合后必须同时保留 #22 冻结 32 CNY：recognized/exact=`32,000,000` micros CNY、available=800、active count=1、Credit `time_based_value=null`；也必须保留 #21 timed CNY/USD、跨币种 singular null 与 `timed_grant_timeline`。
- 若冻结 #21 基于旧 DTO 形状，明确指出需要保留的 #22 字段和 #21 增量；可按所有权机械解决的字段冲突不是功能 finding，任何语义冲突或会破坏可观察合同的冲突必须报告 blocker。

### 明确范围边界

不得实现 Credit 深模块、request settlement、其他 Credit ingress/outflow、conversion FX/在途请求、历史回填、marker/ready、三数据库发布门禁或生产部署。MySQL/PostgreSQL 无 DSN 不应误判为 #21 缺陷；真实三数据库零 SKIP 属于 #27，但明显不兼容代码仍应 finding。

## 输出与完成协议

将不超过 500 字的最终报告写入：

`C:/Users/34404/AppData/Local/Temp/new-api-issue21-spec-final-review.md`

报告必须包含：冻结范围、总评 `PASS`/`FAIL`、按十一条 acceptance/Gate B–E 的映射摘要、findings（引用具体 Issue 条款和文件/符号证据）、与 #22 的组合结论、范围外/未实测说明。无 finding 时明确写“0 findings”。任何推断标为 `[INFERENCE]`。

完成前再次确认 HEAD 不变、工作树 clean。随后使用当前 Orca Dispatch 注入的有效 capability 发送恰好一次 `worker_done`，body 包含 PASS/FAIL、finding 数、最严重项、组合结论和报告绝对路径。未完成、读取失败或冻结状态漂移不得报告成功；需要协调器输入时发送 question/escalation。