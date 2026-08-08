# Issue #25 C/D/E 核心续作 Agent 指令

## 恢复现场

你在唯一正确的 Orca 工作树 `C:/Users/34404/source/repos/new-api/.workspaces/new-api/issue-25-destructive-outflow` 中续作 GitHub `jiwangyihao/new-api#25`。开始时必须确认：分支为 `jiwangyihao/issue-25-destructive-outflow`；工作树 clean；当前 HEAD 为 `c76a3c9f0da80abf4c055c7ee87e67749e2d149c` 或仅包含本任务随后产生的提交；`fe1901aaf7a769fe7057c6483e30b7b1491adcdc` 是共同集成基线与 merge-base；Orca parent 严格指向 `credit-operational-value-integration`。禁止 reset、rebase、checkout、另建工作树、丢弃提交或覆盖 A/B 成果。

已完成且不得重做：管理员 decrease 混合池 outflow、完全清空余数、欠额、幂等/冲突与失败回滚；订单 refund/chargeback immutable purchase facts、终态优先级与同事实重放。关键提交：`92482861f`、`90e6f3c80`、`d6fdcd45c`，交接提交 `c76a3c9f0`。先完整阅读 `.scratch/agent-progress/issue-25/{contract,status,evidence}.md`、`docs/agents/credit-operational-value-issue-25.md`、`docs/agents/credit-operational-value-issue-25-acceptance.md`、Wave 3 合同、Issue #25 与父 PRD #19。

本任务只完成 C/D/E 三组核心合同。不要进入 API/UI/i18n/browser，也不要实现 #27 migration/marker 或 #28 release。必须使用 `skill://tdd`；难以稳定复现时使用 `skill://diagnosing-bugs`；共享锁序或模块边界不清时使用 `skill://codebase-design`。每组开始先更新 status，每组完成后立即补 evidence、gofmt、`git diff --check` 并用 Conventional Commit 提交，工作成果不得只留在终端。

## C：请求快照恢复

从公开 `SettleUserSubscriptionRequestTarget` / `SettleCreditRequestTargetTx` 与现有 #23 request record 接缝构造真实 SQLite 行为测试。场景必须包含：Credit request 已冻结 deduction snapshot；随后普通 ingress 抵消 settlement debt 或改变当前池组成；再对原 request 部分退款或全退款。恢复必须只依据该 request 的原冻结 exact/estimated/unknown、deducted available、debt formed、absorbed restore、rounding 与 original subscription/request attribution，不能按退款时当前池平均重算，不能把后来 ingress 成本错误归还，不能改写其他 active request 的 `SubscriptionPreConsumeRecord`。覆盖后来 ingress 已吸收债务时不能精确还原的部分转为 restored unknown；全退款吸收请求桶余数；同事实重放幂等，参数冲突返回稳定 sentinel 且零写入。先提交真实 RED，再做最小 GREEN；定向单次、`-count=10`、必要窄 `-race` 通过后形成独立 clean 提交。

## D：邀请取消隔离

从公开 `RecoverSubscriptionOrder` 构造真实 SQLite Credit 订单、immutable purchase ledger，以及错误或既有的 `InvitationRewardEvent`。refund/chargeback/financial recovery 只允许取消与当前订单稳定 identity 匹配的奖励；其他订单、其他用户和 timed 订单奖励不得受影响。Credit recovery 不得新增邀请收益，不得进入邀请付费统计。奖励取消、订单终态、Credit 数量、估值状态与 recovery ledger 必须在同一事务；同事实重复回调幂等，payload/reason/target/terminal 冲突稳定拒绝且零写入，refund→chargeback 晋升不重复 outflow。若既有生产接缝已经满足合同，测试可直接 GREEN，但必须诚实记录；不得制造无意义生产改动。完成真实 RED/GREEN、`-count=10` 与独立 clean 提交。

## E：SQLite 并发与原子性

使用真实文件型 SQLite WAL、两个独立连接和确定性 barrier，不用 sleep 猜时序。至少覆盖：refund 与 chargeback；refund 与管理员 decrease；低频 outflow 与 request final settle；低频 outflow 与 request refund。结果必须属于文档明确列出的合法串行化集合：最多一份实际 recovery/outflow、单一终态 ledger、数量/available/exact/estimated/unknown/debt/state_version 一致，活动 request snapshot 与 original attribution 不被低频 outflow 改写。不得泄漏 `SQLITE_BUSY`、唯一约束或内部 GORM 文本；稳定领域冲突应使用 sentinel/code。对可控中间失败验证整事务回滚。运行每项单次与 `-count=10`，并对相关 model 路径运行窄 `go test -race`。若当前实现直接 GREEN，记录证据而不制造生产改动。

## 完成与交接

C、D、E 全部形成独立可恢复提交后，运行三组联合定向测试、A/B 回归、#23 request restore 代表性测试、#24 ingress/debt 代表性测试、`gofmt`、`git diff --check`。更新 `.scratch/agent-progress/issue-25/status.md` 为 `CORE_CDE_HANDOFF_READY`，完整列出提交 SHA、RED/GREEN 命令、SQLite/race 证据、未运行 MySQL/PostgreSQL（归 #27）、以及仍未完成的 API/UI/i18n/browser。确认 staged/unstaged/untracked 全零。不要声称 Issue #25 全部完成，不要发送最终 `worker_done succeeded`；发送一次 escalation/交接消息或停在等待协调器，由后续最终交付 Worker接管。
