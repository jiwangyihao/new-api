# Issue #23 最终 Spec F1/F2 收敛修复指令

## 目标与冻结现场

你负责关闭父 PRD GitHub #19、子 Issue #23「完成 request_id 同步与异步可逆结算」最终 Spec 复评确认的两个 blocker。必须复用现有 Orca 工作树：

`C:/Users/34404/source/repos/new-api/.workspaces/new-api/issue-23-request-settlement`

启动时必须确认：

- 工作树分支为 `jiwangyihao/issue-23-request-settlement`；
- HEAD 为 `8cdfd4acb78b502af4c0232460baf7df852b7b2c`；
- `git status --short` 无输出；
- 与已验收集成基线的 merge-base 为 `ec1858fec89509bdec9a90a230a8496047c5becd`。

此前 Spec 报告 `C:/Users/34404/AppData/Local/Temp/new-api-issue23-spec-final-rereview.md` 结论为 FAIL。旧 Standards attempt 未落有效报告且 Dispatch 已失败，不能当作通过证据。你只修复下面 F1/F2，不重新设计 #23，不修改 #24–#28。

## 必读材料与 Skill

按顺序读取并服从：

1. 仓库和全局 `AGENTS.md`；
2. `issue://jiwangyihao/new-api/19`、`issue://jiwangyihao/new-api/23`，只为依赖合同读取 #22；
3. `docs/agents/credit-operational-value-execution.md`；
4. `docs/agents/credit-operational-value-wave-2-contract.md`；
5. `docs/agents/credit-operational-value-issue-23.md`；
6. `docs/agents/credit-operational-value-issue-23-acceptance.md`；
7. `docs/agents/credit-operational-value-issue-23-spec-final-rereview.md` 与上述最终 Spec 报告；
8. `.scratch/agent-progress/issue-23/{contract,status,evidence}.md`、cleanup、double-count 等现有证据；
9. `CONTEXT.md`、ADR 0002、2026-08-02 spec/plan 中 request_id、累计目标、匿名 delta、Task 与 cleanup 合同。

必须使用 `skill://diagnosing-bugs` 稳定复现；必须按 `skill://tdd` 做 RED→最小 GREEN→回归；接口边界需要调整时读取 `skill://codebase-design`。本修复不涉及 UI，不加载 shadcn-ui/i18n，不改 locale。

第一项实际改动先创建并提交：

- `.scratch/agent-progress/issue-23/final-spec-fix-contract.md`
- `.scratch/agent-progress/issue-23/final-spec-fix-status.md`
- `.scratch/agent-progress/issue-23/final-spec-fix-evidence.md`

每个可编译、可验证的小步立即 Conventional Commit。不要把成果只留在终端或未提交 diff。

## F1：request_id 必须绑定完整规范化不可变请求指纹

### 已确认缺陷

`model/subscription.go::preConsumeUserSubscriptionByUnits` 在找到既有 `request_id` 时，只拒绝 `refunded`，随后直接返回旧结果。它没有比较本次调用与已提交请求的不可变参数，因此同一 `request_id` 可用不同 user、model、quota_type 或 distributor amount 被静默当作成功重放。

### 必须先写的 RED

通过公开 `PreConsumeUserSubscriptionByUnits` 与真实 SQLite 构造至少以下冲突，证明旧实现错误返回成功且不应再次写入：

1. 同 request_id、不同 user；
2. 同 request_id、不同规范化 model；
3. 同 request_id、不同 quota_type；
4. 同 request_id、不同 distributor amount；
5. 同一完整参数重放仍返回原结果且不重复扣除；
6. 冲突发生时请求记录、权益数量、估值状态、版本与审计均零写入。

### 最小 GREEN 合同

- 为 `SubscriptionPreConsumeRecord` 持久化版本化、确定性、碰撞安全的规范化请求指纹；至少覆盖 `user_id`、规范化 `model_name`、`quota_type`、`distributor_amount`。若采用 hash，输入编码必须无分隔符歧义，不能依赖 Go map 顺序、时间、随机数、浮点或进程状态。
- 指纹必须与 request record 在同一事务创建；已有 request_id 分支必须在读取旧业务结果前比较指纹。
- 相同指纹重放返回已提交结果且严格无写入；任何字段不同返回导出的稳定 sentinel（建议专用 `ErrSubscriptionPreConsumeRequestConflict` 或同等清晰命名），所有调用层使用 `errors.Is`，不得解析文本。
- 历史/旁路记录缺少足以证明一致性的指纹时必须 fail-closed，不能在热路径根据当前调用补造“可信”指纹，也不能交给 #27 之前的运行时猜测。
- schema 必须是附加式、SQLite/MySQL 5.7/PostgreSQL 9.6 静态兼容；不得切换 migration marker、回填历史或声称 MySQL/PostgreSQL 实测。
- 保留 request_id 唯一键、现有请求快照、cleanup 与 Task 引用合同。

完成 F1 后运行定向单次、`-count=10`、必要的真实 SQLite 并发/故障注入与窄 `-race`，更新 evidence，提交 clean 安全点；再进入 F2。

## F2：Credit 禁止所有匿名 delta，quota 必须走 request identity + 累计目标

### 已确认缺陷

- `service/quota.go::PostConsumeQuota` 对 subscription 仍直接调用 `model.PostConsumeUserSubscriptionTokenDelta` / `AmountDelta`；
- 两个导出匿名 helper 自身没有拒绝 `credit_balance`，因此其他 service/controller/relay/Task 调用点仍可能绕过 request snapshot 与 CreditValuation 深模块。

### 必须先写的 RED

1. 对 Credit 权益直接调用 token/amount 匿名 helper，应稳定失败，但旧实现会修改数量或入队；
2. `PostConsumeQuota` 对 Credit 的成功最终结算与失败退款必须使用 `RelayInfo.RequestId`、原 `SubscriptionId` 和累计目标，旧实现匿名 delta 应使测试失败；
3. request_id 缺失、目标为负、映射冲突或终态冲突须稳定失败并零写入；
4. timed 与 converted 的合法匿名兼容路径继续工作；
5. 搜索并分类所有匿名 helper 调用点，证明没有 Credit service/controller/relay/异步绕路。

### 最小 GREEN 合同

- 新增导出的稳定 sentinel（例如 `ErrCreditValuationAnonymousDeltaForbidden`），`PostConsumeUserSubscriptionTokenDelta`、`PostConsumeUserSubscriptionAmountDelta` 及任何别名必须在写入/入 coalescer 前识别目标权益；目标为 `credit_balance` 时返回该 sentinel，调用层以 `errors.Is` 判断。
- 不得破坏 timed 正常结算；converted source 既有合法转换映射路径保持兼容，但不能因此允许直接对 Credit target 匿名写入。
- `PostConsumeQuota` 必须对 Credit 使用稳定 `RelayInfo.RequestId`、原 `SubscriptionId` 和累计目标调用现有 request-aware 领域入口；成功 final、少结算与失败退款均复用同一 request identity。目标累计量必须有溢出/负数检查，不能恢复匿名 delta。
- 优先复用已实现的 `SubscriptionFunding`/`BillingSession` request-aware 接缝；禁止复制 Credit 移动平均、退款快照或 coalescer 算法。
- 若某个旧 fallback 不能证明 request identity，必须 fail-closed，而不是临时生成 request_id。
- 迁移完使用 LSP references（若可用）或完整结构化调用点清单证明：Credit 不再通过匿名 helper；保留的匿名调用仅限 timed/converted 明确边界。

完成 F2 后更新 evidence，提交 clean 安全点。

## 必须保留的既有合同

- #22 的 Credit ingress、移动加权平均、32 CNY tracer、current_only、权威 micros 与 analytics DTO 不改；
- #23 已完成的请求活动快照、target increase/decrease、absorbed/unknown restore、original subscription identity、同步 BillingSession、共享事务 coalescer、Task identity、cleanup、兼容字段双计数修复不回归；
- #24 管理员/兑换 ingress、#25 destructive recovery、#26 FX/conversion/virtual snapshot、#27 migration/ready、#28 release 均不得实现；
- 不新增 UI、前端、locale、文档功能、第二份高频 ledger 或新迁移 CLI；
- MySQL/PostgreSQL 实机零 SKIP 留给 #27，不能冒充通过。

## 验证与完成条件

至少运行并记录：

1. F1 完整指纹相同重放与四类冲突，单次、`-count=10`、原子回滚；
2. F2 Credit token/amount 匿名 helper 稳定拒绝，timed/converted 正常；
3. `PostConsumeQuota` Credit request-aware 成功 final、少结算/退款、重放、错误隔离；
4. #23 请求领域、BillingSession、coalescer、Task identity、cleanup、double-count 聚焦回归；
5. 真实 SQLite Controller/Service 请求链或现有 Kyren tracer，证明 request_id、累计目标、800 available/32 CNY 与重放版本不漂移；
6. 相关 model/service/controller 测试与窄 `-race`；
7. `go test ./model ./service ./controller -count=1`；
8. `gofmt` 仅作用于修改 Go 文件，`git diff --check`，最终 staged/unstaged/untracked 全零。

若宽回归出现独立已知噪声，必须先单独重复验证并在 evidence 诚实分类；不得把红灯写成 PASS。不要运行全项目测试、真实 MySQL/PostgreSQL 或部署。

结束时逐条映射 Issue #23 acceptance，列出提交 SHA、RED/GREEN、调用点迁移、未运行范围和后续 #26 seam。随后使用当前 Dispatch 注入的 capability 发送恰好一次有效 `worker_done --outcome succeeded`；若仍有 blocker，发送 failed/escalation，不得虚假成功。不要关闭 Issue、合并、部署或回收工作树。
