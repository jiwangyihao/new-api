# Issue #21 实现 Agent 指令

## 目标与垂直交付

你负责父 PRD #19 的 GitHub Issue #21「固化计时权益 grant 时间线与多币种分析」。必须在 Orca 为你创建的隔离子工作树内交付一条可合并、可验证、工作树干净的永久实现：计时套餐每次真实获得都通过一个领域入口产生不可变估值 grant；购买、兑换、管理员售后授予和续期均可审计且幂等；运营分析按 grant 冻结事实计算逐币种时间/token/recognized 剩余价值；管理员能在现有 UI 中以原因和可重试幂等键授予计时权益，并正确查看跨币种结果。

这是垂直切片，不是只建表或只写 calculator。领域写入、调用点、分析 API、最小 UI、六语言、测试和浏览器证据必须闭环。但严格禁止提前实现 Credit 深模块、Credit 请求结算、计时转换 FX、历史回填、migration ready、发布部署等其他 Issue 所有权。

## 必读材料与 Skill

开始修改前按顺序阅读并服从：

1. 仓库及全局 `AGENTS.md`。
2. `issue://jiwangyihao/new-api/19` 与 `issue://jiwangyihao/new-api/21`；GitHub CLI 一律显式 `--repo jiwangyihao/new-api`。
3. `docs/agents/credit-operational-value-execution.md`。
4. `docs/agents/credit-operational-value-wave-1-contract.md`，特别是 #21 与 #22 的共享文件主改责任。
5. `.scratch/agent-progress/issue-20/contract.md` 以及 #20 已落地的精确价格、schema、模型和错误合同。若该文件或对应提交不存在，立即通过 Orca ask 报告，不能自行重做 #20。
6. `CONTEXT.md`、ADR 0001、ADR 0002。
7. 新规格第 5.3、6、8、9、10、12、13、14 节，以及实施计划任务 5、任务 8 的 timed 部分、任务 9 的管理员 timed/UI 部分。

这是永久 feature，必须先读取并执行 `skill://tdd`：先写会因合理缺陷失败的领域/API/UI 测试，再最小实现，再重构。修改 `web/default` 前读 `skill://shadcn-ui`；新增或改变任何可见文本前读 `skill://i18n-translate` 并维护 en、zh、fr、ru、ja、vi。遇到并发、事务、窗口算法或数据库失败无法直接解释时读 `skill://diagnosing-bugs`，先复现和定位；深模块 seam 不清楚时读 `skill://codebase-design`，但不得推翻 ADR/spec。只有实际触及动态计价表达式才读 `pkg/billingexpr/expr.md`。

## 必须实现的领域合同

- 定义窄而完整的 `TimedSubscriptionGrantRequest` 和 `GrantTimedSubscriptionTx(tx, request)`。调用者只提供稳定来源身份、用户/plan、#20 的权威整数标价、原币种、期限/重置与结构化来源事实；模块派生是否有价、置信度、grant 窗口和规则版本，调用方不能直接伪造 `exact`。
- 在同一事务中创建/续期计时权益并写 `TimedSubscriptionValuationGrant`。利用实际 `EventStartTime/EventEndTime`，不要根据当前时间或套餐再次推算。既有低层创建函数应降为受控内部 seam；购买完成、兑换履约、管理员绑定/授予等有价业务调用点迁移到统一入口。
- grant 使用确定性 `idempotency_key` 与 `(source_type, source_key)`。订单、兑换和管理员来源都有稳定身份；重复来源返回已提交结果且不得再次续期。相同幂等键参数变化必须稳定拒绝，而不是生成第二次服务窗口。
- 一次续期追加一条 grant，不覆盖已有 grant。冻结 plan ID、服务窗口、grant Credit、`source_price_micros`、原币种、期限/重置、规则版本和完整来源快照；同币种 FX 为 1/1。套餐以后改价、改币种或改 Credit 不回写历史 grant。
- 邀请奖励、邀请试用、试用码等继续按现有规则创建权益，但必须由结构化来源显式判为“不估值”，不得仅以价格是否大于 0 推断，也不得创建伪零价 grant。邀请付费统计保持 timed 且排除邀请/试用。
- 管理员计时授予请求必须带非空 reason 和可重试 idempotency key。失败重试保留原 key；成功后或任何业务参数变化后由 UI 生成新 key。disabled plan、trial 或其他现有不合格条件继续拒绝新授予；已持有 disabled-plan 权益消费语义不能受影响。
- `TimedSubscriptionValuationGrant` 必须真正不可变：更新/删除通过模型和领域边界拒绝；不要提供普通 HTTP 修改/删除接口。该切片不新增退款自动撤销服务或 reversal schema。现有管理员失效若缩短 `end_time` 或改变状态，分析只按实际可交付窗口裁剪。

## 必须实现的分析/API/UI 合同

- timed 分析只从 grant 时间线计算，绝不查询“当前 SubscriptionPlan 价格”补猜。按 `[snapshot_at, subscription.end_time)` 与每条 grant 窗口交集计算时间价值；当前额度周期再用真实 `max(token_limit-token_used,0)/token_limit` 折减，未来周期完整，且复用现有 reset 周期规则。
- 每个 grant 原币种独立计算 `time_based_value`、`token_based_value`、`recognized=min(time,token)`；禁止跨币种换算或相加。单币种权益保留兼容 singular MoneyAmount，跨币种权益 singular 必须为 `null`，并返回精确 `*_by_currency`。
- 多 grant 正常首尾相接。若窗口重叠，同一秒只能由最早创建 grant 计值；后续重叠部分披露稳定 `overlapping_grants` unknown/warning，不得重复计值。缺失或无法可靠建立时间线时披露 unknown/warning，不能回退当前套餐价格。
- summary、users、subscriptions、plans、sources 五接口中的 timed 数据须口径一致；source breakdown 按 grant source_type 聚合，混合来源权益使用 `source_attribution=mixed_grants`，不能把最后一次来源套给整条权益。
- 与并行 #22 的边界：#22 主改通用 micros DTO、通用 paid-row 分流骨架和通用前端精确金额/警告组件；你主改 timed calculator、timed 分支及 `*_by_currency`。尽量新增独立 timed 文件和窄调用 seam；必须触碰共享文件时只做必要增量并写入 contract.md。不得实现 Credit branch 或覆盖 #22 的通用骨架。
- 管理员计时授予 UI 收集 reason，维护可重试 key，并清晰显示失败/重试状态。计时分析 UI 按币种拆分，singular 为 null 时不得按当前套餐币种重新合并。文案必须使用准确的“运营剩余价值”等术语，不得称为退款、负债或实收。

## 崩溃恢复与提交纪律

第一项实际改动必须创建并提交：

- `.scratch/agent-progress/issue-21/status.md`：阶段、完成项、下一步、阻塞、最近安全提交。
- `.scratch/agent-progress/issue-21/evidence.md`：RED/GREEN 命令、关键输出、浏览器/数据库证据、失败根因。
- `.scratch/agent-progress/issue-21/contract.md`：grant schema、来源身份、领域接口、错误码、分析 DTO、UI payload、共享文件清单，以及明确非所有权。

频繁更新这三份文件。每个可编译、可验证小步立即使用 Conventional Commits 提交（英文 type/scope、简体中文 subject）；不要把关键代码只留在未提交工作树或大段临时脚本里。需要 #22 尚未提供的通用 seam 时，按共享合同先实现最窄 timed 侧接口并通过 Orca `orchestration ask` 报告，不要复制 Credit 实现或停掉所有可并行工作。

## 验证与完成条件

至少用定向测试证明：购买/兑换/管理员授予/续期均生成 grant；重复来源不续期；参数冲突拒绝；改价/改币种不回写；邀请/试用排除；跨币种 singular null；窗口重叠不重复；现有失效裁剪；五接口 timed 一致；管理员 UI key/reason 重试语义；六语言无 missing/extras。并发测试必须断言合法串行结果，不依赖 goroutine 调度。

运行真实 SQLite 领域/API tracer；三数据库完整零 SKIP 矩阵由 #27 统一负责，但你的 schema/SQL 必须三方兼容，不能拿 GORM DryRun 当验收。UI 改动必须启动应用并用真实浏览器完成管理员授予与跨币种展示 smoke，记录请求 payload、重试 key 和观察结果。只格式化明确修改文件，运行 `git diff --check`；不要运行项目级全量测试或部署生产。

完成前逐条复核 Issue #21 acceptance criteria，确保所有代码和恢复文件已提交、工作树干净。然后在当前 Dispatch 只发送一次 `worker_done`，列出提交 SHA、文件/接口合同、定向测试、SQLite/API/浏览器证据、共享文件、三数据库证据实际范围、遗留风险和进度目录；明确声明未实现 Credit 结算、转换 FX、历史迁移、ready 切换、生产发布。不要关闭 Issue、不要合并/回收工作树，等待协调器验收。