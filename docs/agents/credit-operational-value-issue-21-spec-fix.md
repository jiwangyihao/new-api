# Issue #21 AC2 / Gate B 收敛修复 Agent 指令

## 任务目标与冻结现场

你负责关闭父 PRD #19、GitHub Issue #21「固化计时权益 grant 时间线与多币种分析」最终 Spec 复评中唯一剩余的 AC2 / Gate B blocker。工作目录必须复用现有 Orca 工作树：

`C:/Users/34404/source/repos/new-api/.workspaces/new-api/issue-21-timed-grants`

冻结且已验收 Standards 的 HEAD 为：

`763b0f40bdc8fb7d5c11bc69f46749fd40a8763b`

该分支已经合入父集成基线中的 Issue #22 通用 Credit/current_only/权威 micros/BigInt 骨架，且已经完成四项 Standards 修复。不得重新合并父分支、重做冲突解决、改写已通过实现、关闭 Issue、部署生产或回收工作树；最终集成由协调器负责。

唯一 blocker：`controller/subscription.go` 的管理员 timed grant 路径信任客户端提交的价格和币种，`model/timed_subscription_valuation.go` 的 `GrantTimedSubscriptionTx` 没有在既有计划行 guard 与同一事务内重新读取并冻结权威 Plan 事实。现状允许针对一个权威价格 `40 CNY` 的 timed Plan 构造客户端请求并写入 `25 USD` 的 exact grant，这是必须从根因关闭的权限边界错误。

## 必读资料与 Skill

修改前依次读取并服从：

1. 仓库与全局 `AGENTS.md`。
2. `issue://jiwangyihao/new-api/19`、`issue://jiwangyihao/new-api/21`、已关闭的 `issue://jiwangyihao/new-api/22`。
3. `docs/agents/credit-operational-value-execution.md`。
4. `docs/agents/credit-operational-value-wave-1-contract.md`、`credit-operational-value-wave-1-acceptance.md`。
5. `docs/agents/credit-operational-value-issue-21.md`、`credit-operational-value-issue-21-acceptance.md`。
6. `docs/agents/credit-operational-value-issue-21-review-fix.md` 及 `.scratch/agent-progress/issue-21/review-fix-*`。
7. `C:/Users/34404/AppData/Local/Temp/new-api-issue21-standards-rereview-final.md` 与 `C:/Users/34404/AppData/Local/Temp/new-api-issue21-spec-rereview-final.md`。
8. `CONTEXT.md`、ADR 0002、2026-08-02 specification/plan 中 timed grant、计划快照、整数金额、幂等和事务章节。

必须使用 `skill://diagnosing-bugs` 先复现越权输入，使用 `skill://tdd` 完成 RED→最小 GREEN，使用 `skill://codebase-design` 保持 Plan 权威读取、grant 深模块和调用层边界。若必须修改 `web/default` 的管理员 grant 表单或请求构造，先读取 `skill://shadcn-ui`；若新增或改变可见文本，再读取 `skill://i18n-translate` 并维护六语言。修改导出符号前使用 LSP references。

## 可恢复进度与提交纪律

第一项实际变更必须创建并提交：

- `.scratch/agent-progress/issue-21/spec-fix-status.md`
- `.scratch/agent-progress/issue-21/spec-fix-evidence.md`
- `.scratch/agent-progress/issue-21/spec-fix-contract.md`

记录冻结 HEAD、唯一 blocker、权威事实字段、事务/锁序、RED/GREEN 命令、最近安全提交、未提交文件、下一步与阻塞。每完成一个可验证小步立即更新并提交；上下文约 80% 时必须先形成 clean 或诚实 WIP 的 HANDOFF_READY。提交使用 Conventional Commits，英文 type/scope、简体中文 subject。不得把关键成果只留在终端、stash、临时脚本或未提交大 diff。

## 固定实现合同

### 1. 调用方只能表达意图，不能提供估值事实

管理员创建 timed grant 的业务输入只允许表达：目标用户、`plan_id`、`reason` 和 `idempotency_key`（以及既有鉴权/路由必需字段）。价格、币种、Credit 数量、持续时间、reset 策略、billing rule、窗口价值或 confidence 都不是调用方可控制的事实。

如果当前 controller DTO 或前端请求仍携带 `price_amount`、`price_amount_micros`、`currency`、`credit_amount`、duration/reset/rule 等估值字段，应做干净切换：从管理员表单/请求构造中删除，或在兼容解码不可避免时明确忽略并证明其不能影响结果。不得把客户端值传进领域层再与 Plan 比较后采用；领域层必须自行取得权威事实。

### 2. 在同一事务和同一线性化 guard 下读取权威 Plan

复用 Finding 1 已通过的数据库计划行 guard 与固定锁序。`GrantTimedSubscriptionTx` 或其唯一权威深模块入口必须在同一事务内：

1. 锁定/线性化目标 Plan；
2. 从数据库重新读取当前权威 Plan，而不是信任 controller 的预读对象或旧快照；
3. 验证它仍是请求指定的 plan identity、类型为 timed、可用于新 allocation 且 `enabled=true`；
4. 从该行精确读取并冻结 `price_amount_micros`、`currency`、Credit/token 数量、duration/window、reset policy、billing/pricing rule 与设计要求的 source identity；
5. 在同一事务内写 entitlement/window/grant，失败整体回滚。

不得通过二进制浮点反推 micros；不得用当前全局设置替代 Plan 冻结事实；不得新增仅进程内 mutex 作为正确性来源。

### 3. 幂等与 disabled-plan 边界保持

- 已成功提交的同一 source/idempotency identity 重放，必须返回同一 grant/窗口事实；即使 Plan 后来 disabled，重放也不得生成新 allocation 或改写已冻结事实。
- disabled Plan 的新 idempotency key / 新 allocation 必须稳定拒绝且零写入。
- identity 相同但请求目标或事实冲突，继续使用既有稳定 mismatch sentinel/code，不能泄漏方言 DB 错误或依赖错误文本。
- 既有 disabled-plan entitlement 的消费行为不受本修复影响。

## 必须先证明的 RED

至少用真实 SQLite 事务/API 测试证明旧实现失败：

1. 数据库中的 timed Plan 权威事实为 `40.000000 CNY`；管理员请求伪造 `25.000000 USD`（以及不同 Credit/duration/reset/rule，如当前 DTO 可表达）。旧实现写入错误的 25 USD exact grant；测试必须在修复前稳定 FAIL。
2. controller/API payload 即使包含伪造价币字段，最终数据库 grant 只能等于 Plan 权威快照；如果干净 DTO 拒绝/忽略字段，断言可观察合同和稳定响应。
3. controller 预读后、领域写入前 Plan 被修改或停用时，领域层必须以 guard 内重读事实为准；不得冻结过期价币。
4. 权威 Plan 缺失精确 micros、币种非法、类型不符或 disabled 的新 grant，返回稳定错误并证明 entitlement/window/grant 全部零写入。

## 最小 GREEN 与边界证据

完成最小实现后，至少提供：

- 40 CNY Plan 对 25 USD 攻击被消除，数据库和 API 返回的 grant 严格是 `40,000,000` micros CNY；客户端其他伪造 Plan 事实同样不能生效。
- 同一事务内权威重读、锁序和 source identity 的代码/测试证据。
- 成功 grant 后同 key 重放不新增行、不续期两次、不改变冻结快照；新 key 在 disabled 后拒绝。
- 失败路径原子性：用户权益、window、grant、ledger/相关状态无部分写入。
- 边界秒：grant 生效起点/结束点遵循现有 spec 的闭开区间或既定窗口语义，恰好边界不重复计值、不丢一秒；不得发明新时间规则。
- 零额度：按 Issue/spec 的现有合法性规则给出真实证据。若 zero Credit Plan 对新 grant 非法，则稳定拒绝并零写入；若既有规范允许，则证明其冻结/分析行为，不得自行选择更方便的语义。
- `go test` 定向重复 `-count=10`；若改动并发接缝，运行窄范围 `-race`。
- #22 的 32 CNY Credit/current_only/权威 micros/BigInt 合同与 #21 timed CNY/USD 五接口组合定向测试保持通过。

## 禁止事项

- 不改动或回退 #22 的 CreditValuation、current_only warning、权威 micros sorter、BigInt DTO/UI。
- 不重做已通过的四项 Standards 修复、timed 五接口、六语言或浏览器流程；只有实际修改管理员 UI 时才运行对应窄 UI/browser 验证。
- 不实现 #23 request settlement、#24 正向入账、#25 恢复、#26 FX/转换、#27 migration marker/ready、#28 发布。
- 不做 locale/report 全量重排，不改锁文件，不新增通用 retry/telemetry/框架。
- 不把 MySQL/PostgreSQL 未运行写成 PASS；真实三库零 SKIP 仍归 #27。

## 完成条件

完成前必须：

1. 更新 `spec-fix-*` 为 COMPLETE，列出有效 RED、最小 GREEN、提交 SHA、权威 Plan 字段、锁序和非目标。
2. 运行受影响的 model/controller/router 或 service 定向测试、必要的管理员请求测试、组合 Credit+timed 回归、`git diff --check`。
3. 若修改前端，运行对应定向测试、typecheck；若改变用户可见流程，执行真实浏览器 smoke 并清理 tab/服务/临时 DB。
4. 工作树 staged/unstaged/untracked 全部为 0。
5. 使用当前 Dispatch 注入 capability 发送且只发送一次 `worker_done`，正文包含最终 HEAD、提交列表、RED/GREEN 命令与结果、边界/原子性/组合证据、未运行项和范围声明。

不要合并父分支、关闭 Issue、部署生产或回收工作树。
