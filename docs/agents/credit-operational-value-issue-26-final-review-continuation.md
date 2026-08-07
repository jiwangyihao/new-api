# Issue #26 M1/M3/M2 最终续作 Agent 指令

## 目标与冻结现场

你负责完成 GitHub `jiwangyihao/new-api#26` 最终复评留下的 M1/M3/M2。工作树固定为：

`C:/Users/34404/source/repos/new-api/.workspaces/new-api/issue-26-final-review-fix`

开始时必须确认：

- 当前 HEAD 为 `44009213cb8e4a582de34f884deecd5a8d687b2c` 或仅包含本任务后续提交；
- 工作树 clean；
- Orca parent 严格指向 `credit-operational-value-integration`；
- `b8598f4b7add27ba237f30dec6ceae7968cc2aa3` 为 merge-base/祖先；
- 已完成的 H1 request→target 固定锁序提交 `3feb091159aef26731c1698647791acc03c29c0a` 仍在祖先链；
- 父树已经包含 #24 H2 跨币种 ingress 与路由夹具校准，禁止覆盖或回退。

先读取 `skill://diagnosing-bugs`、`skill://tdd`、`skill://codebase-design`；若修改可见 UI，再读取 `skill://shadcn-ui` 与 `skill://i18n-translate`。完整读取父 PRD #19、Issue #26、`CONTEXT.md`、ADR 0002、2026-08-02 spec/plan、`docs/agents/credit-operational-value-issue-26-acceptance.md`、`docs/agents/credit-operational-value-wave-3-contract.md`，以及报告：

- `C:/Users/34404/AppData/Local/Temp/new-api-issue26-final-integrated-standards-review.md`
- `C:/Users/34404/AppData/Local/Temp/new-api-issue26-final-integrated-spec-review.md`

立即创建并提交 `.scratch/agent-progress/issue-26/final-review-continuation-{contract,status,evidence}.md`。按阶段频繁小步提交；上下文达到 75% 时必须先提交 clean 恢复点，达到 85% 前必须 HANDOFF_READY 或完成，禁止把唯一成果留在未提交 diff。

## 所有权与禁止范围

只修 M1/M3 与 M2；H1 已完成，不得重新设计锁序。不得修改或复制 #24 redemption/admin increase 的 ingress、UI 或幂等合同；不得实现 #25 destructive recovery、#27 migration/ready、#28 release。保持 #20–#24 已集成的精确价格、timed grants、Credit moving-weighted state、request-aware settlement、current_only warning、BigInt/micros sorter 与 disabled-plan 消费边界。

## 阶段一：M1/M3——稳定错误与 committed unit value

必须先写可失败测试，再最小实现并独立提交。

1. 导出并使用稳定领域 sentinel：至少 `ErrConversionIneligible` 与 `ErrConversionQuoteStale`；`errors.Is` 可判。不得继续依赖 `strings.HasPrefix`、自由文本或 controller 自行猜原因。
2. controller/router 为资格失效、quote 过期/事实漂移、幂等冲突、FX 无效分别返回稳定 machine code；现有 UI 只按 code/reason_codes 分支，不能解析 message。
3. conversion confirm/history/analytics 返回的未舍入单位价值必须来自确认事务已经 committed 的结构化字段。禁止响应层以 `math/big`、当前 Plan、当前 Option 或兼容 float 重算。
4. 如现有 schema 无法保存未舍入单位价值，允许只增加附加式结构化列（整数 numerator/denominator 或等价权威 micros+basis），迁移必须兼容 SQLite/MySQL 5.7/PostgreSQL 9.6；不得引入 JSON-only 权威事实或动态回填。
5. 真实 SQLite quote→confirm→history/analytics tracer 必须证明：Plan/Option 后续变化不改变 source price、basis、unit value、FX、gross/net value、rule/state version；错误路径零写入。
6. 保留 `full_31_day_blocks × credit_basis + current_remaining_credit` 数量公式、31 天业务月、conversion 不是新增收款、邀请归因为零。

阶段一门禁：定向单次与 `-count=10`、必要窄 `-race`、router code 测试、前端现有 conversion card 测试、gofmt、`git diff --check`。提交 clean 安全点后才进入阶段二。

## 阶段二：M2——quote identity 与 stale 合同

必须先用真实 API/SQLite 测试证明旧 quote 可被错误重放或事实变化后仍确认的 RED，再最小 GREEN。

1. quote 返回稳定 `quote_id`、`created_at`、`expires_at` 及版本化 authoritative facts fingerprint；时间字段为服务端权威值。
2. fingerprint 至少覆盖 source entitlement、source Plan ID、source price micros/currency、credit basis、gross/net Credit、source/target currency、FX numerator/denominator/captured_at、rule/version、目标 Credit mapping，以及确认所需资格事实。规范化编码必须无歧义且不经过浮点。
3. Confirm 必须在同一事务、H1 request-first 锁序下重新锁定并重读权威事实；quote 过期、fingerprint 不匹配、资格/Plan/FX/目标映射变化均返回 `ErrConversionQuoteStale` 和稳定 code，整笔零写入。
4. 相同 quote 的完全相同确认/重放返回同一 committed conversion；不同 quote 或相同 id 但事实冲突不得覆盖既有 conversion。
5. 不允许只信任客户端回传 fingerprint；服务端必须能验证 identity、有效期与权威事实。
6. 兼容旧 API 的范围只能由现有明确测试证明；不得新增隐式宽松 fallback 绕过 stale 校验。

阶段二门禁：quote→confirm 成功、过期、Plan 改价、FX 改变、basis 改变、target mapping 改变、并发 confirm、重放与失败零写入，均须真实 SQLite；运行 `-count=10` 与窄 race。

## 最终验收

完成后运行并持久化：

- H1 request→target 锁序与 conversion↔settle/refund 双连接回归；
- FX parser/invalid/identity/reverse/floor/overflow；
- 同币种与 CNY↔USD conversion、幂等/冲突、in-flight settle/refund；
- quote/confirm/history/analytics router tracer；
- #20–#24 代表性后端合同；
- 受影响前端 tests、typecheck、i18n sync、production build；
- `go test ./model ./service ./controller ./router -count=1`；
- `git diff --check` 与 clean tree。

MySQL/PostgreSQL 实机零 SKIP 归 #27，不得冒充验证。不要运行部署或写生产数据。最终更新三份 progress，列出所有提交、RED/GREEN、未运行边界和风险，确认 staged/unstaged/untracked 全零，再从注入的 Task/Dispatch capability 发送一次且仅一次 `worker_done --outcome succeeded`。