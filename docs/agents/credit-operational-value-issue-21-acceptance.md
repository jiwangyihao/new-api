# Issue #21 协调器验收清单

## 用途与基线

本清单供协调器在父 PRD #19 的 Issue #21 Worker 发出 `worker_done` 后使用。只有 Issue #20 已独立验收并集成到 `jiwangyihao/credit-operational-value-integration`，且 #21 子工作树从该提交派生，才可开始验收。

验收必须同时服从 Issue #21、ADR 0002、2026-08-02 规格/计划、`credit-operational-value-execution.md` 和 `credit-operational-value-wave-1-contract.md`。失败项交还原 Worker 在原工作树修复；不得降低断言、伪造数据库证据或把垂直交付拆给后续 Issue。

#21 只拥有：不可变 timed grant、统一计时授予入口、timed 分析算法与逐币种投影、管理员计时授予 UI。Credit 深模块、请求结算、转换 FX、历史迁移和发布不属于本切片。

## Gate A：Worker 与提交完整性

- [ ] 当前 Dispatch 恰好收到一次 `worker_done`；记录 Run、Task、Dispatch、终端、Worker 分支、父工作树和最终 HEAD。
- [ ] `worker_done` 列出提交 SHA、领域/API/UI 改动、测试命令、真实 SQLite 与浏览器证据、共享文件及未决风险。
- [ ] `.scratch/agent-progress/issue-21/{status,evidence,contract}.md` 已提交且内容与 HEAD 一致；`status.md` 为完成状态。
- [ ] Worker 工作树无 staged、unstaged、untracked 文件；成果不只存在于终端、stash、备份或临时脚本。
- [ ] `git merge-base` 证明分支从已验收 #20 集成基线派生；其提交链仅包含 #21 和恢复记录。
- [ ] 完整差异通过 `git diff --check`；Conventional Commit 使用英文 type/scope、简体中文 subject。
- [ ] 未修改受保护项目身份、凭据、部署配置或用户无关文件；未越界实现 #22–#28。

## Gate B：领域授予合同

- [ ] 存在单一窄入口 `GrantTimedSubscriptionTx`（或经审阅等价接口），由真实购买、兑换、管理员授予和续期调用。
- [ ] 权益创建/续期与 grant 在同一事务完成；任一失败均整体回滚，不留下无 grant 的有价服务窗口或无权益的 grant。
- [ ] 调用者只提交稳定来源事实；模块派生有价性、置信度、窗口和规则版本，不能由调用者直接声明 `exact`。
- [ ] grant 冻结实际服务起止、精确标价 micros、原币种、Credit、期限/重置、规则版本与结构化来源；后续改 plan 不回写。
- [ ] 订单、兑换、管理员来源均有确定性 `idempotency_key` 和 `(source_type, source_key)`；相同参数重放不续期，参数冲突返回稳定错误并回滚。
- [ ] 每次有效续期追加 grant，不覆盖旧行；唯一约束由真实 SQLite 数据库证明，而非只检查 GORM tag。
- [ ] grant 更新/删除在普通模型和领域边界均被拒绝；未新增普通 HTTP 修改/删除入口或 timed reversal schema。
- [ ] 邀请奖励、邀请试用和试用码明确标记不估值，不创建伪零价 grant；邀请付费统计仍排除这些来源。
- [ ] disabled/trial/不合格计划继续拒绝新授予；既有 disabled-plan 权益消费行为保持不变。
- [ ] 管理员授予必填 reason 与幂等键；失败重试复用原键，成功或参数改变生成新键。

## Gate C：timed 计算与五接口一致性

- [ ] timed 行只读取不可变 grant；代码不存在用查询时当前 plan 价格补猜的 fallback。
- [ ] 每条 grant 按 `[snapshot_at, subscription.end_time)` 与其服务窗口求交；已失效/缩短的权益按实际状态和 `end_time` 裁剪。
- [ ] 当前周期按真实剩余 Credit 比例折减，未来周期完整，并复用仓库既有 reset 周期语义；零额度、边界时刻和跨周期均有测试。
- [ ] 每个原币种独立计算 time/token/recognized，recognized 为同币种两口径较小值；禁止跨币种动态换汇或求和。
- [ ] 单币种保留兼容 singular MoneyAmount；多币种 singular 为 `null`，精确值只通过 `*_by_currency` 返回。
- [ ] 首尾相接 grant 不丢秒、不重计；窗口重叠只计最早创建 grant，后续重叠披露稳定 `overlapping_grants` unknown/warning。
- [ ] 缺失或歧义时间线披露 unknown/warning，不回退当前套餐价格；warning 使用稳定 code，不靠文本分支。
- [ ] summary、users、subscriptions、plans、sources 从同一 timed row 事实聚合，筛选、排序和 totals 一致。
- [ ] source breakdown 按 grant 来源聚合；混合来源使用 `mixed_grants`，不得把最后一次来源归给整个权益。
- [ ] #21 仅实现 timed calculator/row 分支与 timed DTO 增量；通用 micros DTO、Credit 分流和 Credit UI 仍归 #22。

## Gate D：API、管理员 UI 与六语言

- [ ] 管理员授予 API 接受 reason/idempotency key，稳定区分缺失、冲突、disabled/trial 与重复成功。
- [ ] UI 在失败重试时复用同一 key，成功或业务参数变化后生成新 key；组件测试断言真实 payload，而非只测按钮文案。
- [ ] timed 分析 UI 按币种拆分；singular 为 `null` 时不按 plan 币种或浏览器汇率重新合并。
- [ ] unknown、overlap、mixed source 与实际失效裁剪在页面可见，术语为“运营剩余价值”，不称为退款、负债、实收或会计递延收入。
- [ ] 所有新增用户可见文字使用 `t(...)`；en、zh、fr、ru、ja、vi 无 missing/extras。
- [ ] 运行相关组件测试、`bun run typecheck` 与 `bun run build`；只格式化实际修改文件。
- [ ] 启动受监督开发服务并用真实浏览器完成管理员授予、失败重试、成功后新 key、单币种与跨币种展示 smoke；记录 payload、响应和可见结果，结束后关闭 tab/服务。

## Gate E：测试与数据库证据

- [ ] 真实 SQLite tracer 覆盖购买、兑换、管理员授予和续期，且均从现有领域入口产生 grant；禁止直接插入 grant 代替主证明。
- [ ] 覆盖重复来源、参数冲突、事务回滚、改价/改币种不回写、邀请/试用排除、失效裁剪、重叠窗口和跨币种 singular null。
- [ ] 并发测试只接受合法串行化结果，不依赖 goroutine 调度或 sleep。
- [ ] 受影响 Go 包定向测试通过；测试必须对观察合同失败，而不是断言源码文本或实现细节。
- [ ] schema/SQL 审阅兼容 SQLite、MySQL 5.7.8+、PostgreSQL 9.6；DryRun 不得标成真实数据库 PASS，三库零 SKIP 总门禁保留给 #27。
- [ ] `git diff --check`、工作树清洁度及 Issue #21 十一条 acceptance criteria 的证据映射全部通过。

## 不放行条件

出现任一项则保持 #21 OPEN 并让原 Worker 修复：

- 有价服务窗口可以在没有不可变 grant 的情况下提交；
- 幂等冲突再次续期，或普通业务路径可更新/删除 grant；
- timed 价值读取当前 plan 价格或跨币种求和；
- overlap 被重复计值，unknown 被静默变成 exact；
- 侵入 Credit 深模块、request settlement、conversion FX、历史迁移或 ready marker；
- 缺少真实领域/SQLite/browser 证据、六语言、构建或清洁工作树。
