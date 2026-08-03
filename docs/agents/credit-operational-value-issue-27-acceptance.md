# Issue #27 协调器验收矩阵：历史迁移、三数据库与门禁

本清单由协调器在 Issue #27 Worker 发出 `worker_done` 后执行。Worker 自报、单库测试或迁移脚本能运行都不能替代验收。所有失败项优先返回原 Worker 在原工作树修复；只有 Orca 明确显示 failed/stopped 且现场已保存，才允许以 `--retry-of` 恢复。

## Gate A：Worker、基线与切片所有权

- [ ] Orca Dispatch 收到唯一一次 `worker_done`；记录 Task、Dispatch、终端、子工作树完整 ID、父工作树 ID、共同基线 SHA 和 Worker HEAD。
- [ ] 子工作树的 Orca parent 是 `credit-operational-value-integration`；`merge-base` 包含已验收集成的 #20–#26，不能从 `origin/main`、生产基线或任一旧 Worker 分支派生。
- [ ] Worker 工作树干净；所有代码、测试、迁移夹具及 `.scratch/agent-progress/issue-27/{status,evidence,contract,release-handoff}.md` 均已提交，恢复记录中的最近安全提交与实际 HEAD 一致。
- [ ] 提交列表为 Conventional Commits，未混入主树 `CLAUDE.md`、用户工作树、凭据、DSN、数据库 dump、完整敏感日志或生产操作。
- [ ] 逐条复核 GitHub Issue #27 的 17 条 acceptance criteria；每条均有代码、真实命令或数据库结果证据，不能以计划、TODO 或推断代替。
- [ ] #27 只消费 #20 的附加列、前向精确价格和只读诊断；历史价格回填与 `ready` 决策只在 #27。未反向修改 #20 所有权，也未复制 #21–#26 的运行时 writer。
- [ ] 未构建或部署生产镜像、未连接或修改生产数据库、未切换流量、未实现 #28；完成声明明确这些未运行范围。

## Gate B：维护子命令与无副作用初始化

- [ ] 根二进制提供 `credit-valuation-migrate`，且 `--dry-run`、`--apply`、`--verify`、`--repair-missing-as-unknown`、`--suspend` 五种模式严格互斥。
- [ ] `--version`、`--batch-size`、`--reason` 的必填性和合法组合与合同一致；非法组合返回稳定非零退出码及结构化错误，不依赖解析英文错误文本。
- [ ] 维护入口只初始化数据库与必要 Option，不启动 HTTP、Redis、定时器、队列、同步器或业务轮询；进程完成后自行退出，无遗留服务。
- [ ] dry-run 与 verify 通过防写证据证明完全只读；同一数据库快照连续两次运行得到规范化后相同业务 JSON 和 SHA-256。
- [ ] 输出包含迁移版本、Credit 币种、冻结 FX、历史价格诊断、exact/estimated/unknown 汇总、歧义、blocker、稳定批次边界和 checksum；时间、耗时、临时路径等非确定字段不进入 checksum。
- [ ] apply 使用稳定主键批次、版本 marker CAS 和可重跑 upsert；断点恢复从已持久化稳定边界继续，不覆盖迁移开始后发生的前向变更。
- [ ] 同版本 `ready` 重放是可观察的无操作；不同参数、不同 checksum 或过期版本不会静默覆盖现存状态。

## Gate C：历史精确价格回填

- [ ] SQLite、MySQL 与 PostgreSQL 均从数据库原始 DECIMAL/NUMERIC/SQLite 数值文本读取旧价格；路径中不存在先扫描为 `float32/float64`、从兼容 JSON 展示值反推或格式化再解析。
- [ ] 合法的非负、最多六位小数值精确转换为 int64 micros，且 micros 回转十进制后与原数值相等。
- [ ] 负数、超过六位小数、指数/特殊值、溢出、无法恢复及往返不一致按稳定套餐 ID 和稳定 reason 报告。
- [ ] dry-run 只诊断；apply 才按稳定套餐 ID 回填 `price_amount_micros`；verify 要求所有需要估值的历史有价套餐完成精确回填。
- [ ] 任一非法历史价格会使 marker 保持非 `ready`；迁移不舍入、不截断、不静默写零、不按当前套餐价猜值。
- [ ] `repair-missing-as-unknown` 不能修补非法套餐价格；运维必须经 #20 前向精确价格合同显式修正后重新 dry-run/apply/verify。
- [ ] #20 创建的历史 NULL 行在 #27 apply 前保持 NULL；验收证明 #20 没有抢先回填，#27 才拥有写入行为。

## Gate D：Credit 与 timed 历史重建

- [ ] Credit 权益按 `user_subscription_id` 稳定排序，来源仅按结构化 `(source_type, source_id/source_key)` 去重，不使用 `(user_id, plan_id)` 最近订单猜测。
- [ ] 对每份权益正确计算 `A=max(token_limit-token_used,0)`；证明 `K/U/T/C/A/R` 和 `floor(C×R/T)` 的保守比例公式，包括 `T=0`、`A<T`、`A>T`、已耗尽、债务与 int64 边界。
- [ ] 可证明数量与成本的历史来源进入 estimated，而不是 exact；总分母、来源或支持币种不可证明时，未证明可用量进入 unknown，不放大已知成本。
- [ ] 历史 CNY/USD 使用迁移启动时冻结的一份严格有理数 FX；unsupported currency 对应 Credit 进入 unknown，不动态重估。
- [ ] timed grant 恢复优先级固定为订单履约快照、兑换 fulfillment、明确管理员记录、唯一可证明来源/服务窗口；一对多歧义保持 unknown，不选择最近订单。
- [ ] 已由前向 writer 写入且来源、窗口完整的 exact Credit/timed 数据不会被历史迁移覆盖；迁移仅补缺失的 estimated/unknown。
- [ ] 重复来源、重复 grant、断点重跑和批次边界不会重复数量或成本；同快照重跑状态版本不增长。

## Gate E：marker 状态机与 fail-closed

- [ ] `pending/running/ready/failed/suspended` 转移由版本 CAS 约束；非法转移、版本冲突和 checksum 漂移均稳定拒绝。
- [ ] apply/verify 在进入 ready 前检测非终态预扣、仍会回调的异步 Task、可写旧进程会话和其他旁路 writer；每类 blocker 有稳定 reason。
- [ ] verify 原子检查：历史价格完整、每份 Credit 权益恰有一行状态、可用量/币种/非负/unknown 上界/版本一致、来源和 timed grant 唯一、checksum 匹配；任一失败不允许部分 ready。
- [ ] 只有完全没有套餐权益、成功订单、兑换和管理员授予历史的全新数据库可在 HTTP 启动前原子自动 ready；“没有 Credit 但有 timed 历史”不能误判为空库。
- [ ] ready 后所有 Credit 数量 writer 与估值状态同锁同事务；状态缺失或数量不一致整笔失败，不热路径创建 unknown、不回退旧 delta。
- [ ] ready 后 legacy Task 使用 #23 持久 Task 主键身份；追加按当前平均值，退款新增可用量为 unknown，重复回调幂等。
- [ ] repair 仅在停写、显式新 migration version 下补缺失 Credit 状态为 unknown，记录 critical 审计并要求重新 verify；不覆盖现有状态。
- [ ] suspend 仅在停写维护窗口从 ready 携带 reason 原子进入；suspended 时正常 HTTP 写保持关闭，只允许验证、修复或新版本迁移。

## Gate F：真实三数据库与并发证据

- [ ] 同一验收矩阵在真实 SQLite、MySQL `5.7.44`、PostgreSQL `9.6.24` 上执行；证据记录真实版本、命令形状、PASS 数与 `SKIP=0`，不泄露 DSN。
- [ ] 三库均覆盖 schema、BIGINT、命名唯一约束、不可变 hook、行锁和原始十进制文本读取；DryRun/mock/schema 反射不能替代真实执行。
- [ ] 三库均覆盖合法/非法/边界价格、两次 dry-run checksum、apply、断点续跑、幂等重放、verify、blocked ready、repair 与 suspend。
- [ ] 三库均覆盖 purchase/redemption/admin/conversion grant、consume、request settle/refund、recovery 及五个分析接口。
- [ ] 并发矩阵覆盖 grant+grant、grant+consume、consume+restore、conversion+settlement、refund+admin decrease，结果属于定义的合法串行化集合且无死锁/锁序反转。
- [ ] 定向 Go `-race` 覆盖算术、合并器和门禁缓存；明确其只是补充，不能代替数据库并发。
- [ ] 所有数据库测试使用项目现有 `TEST_MYSQL_DSN`/`TEST_POSTGRES_DSN` 入口；证据中没有把“本机缺环境时 SKIP”误报为本 Issue 完成。

## Gate G：冻结 tracer、错误与兼容性

- [ ] 真实数据库领域级 tracer 通过现有购买/兑换/管理员/请求结算入口进入五个分析 API，而不是直接插最终估值表。
- [ ] 冻结 fixture `40 CNY / 1,000 Credit`、消费 200、`end_time=0` 得到可用 800、`32,000,000` micros CNY、active count 1、estimated 0、unknown 0；summary/users/subscriptions/plans/sources 一致。
- [ ] marker 非 ready、非法历史价格、estimated、unknown、current_only、unsupported FX、state missing/mismatch 都通过稳定 code 或结构化 warning 暴露，调用方不解析文本。
- [ ] disabled-plan 既有权益消费、模型范围忽略、邀请隔离、转换归属和 #20–#26 已验收合同均未回归。
- [ ] 受影响 Go 包、CLI smoke、定向前端测试（若有可见 warning）、六语言检查、明确格式化文件及 `git diff --check` 通过。
- [ ] 没有临时测试文件、fixture 数据库、后台数据库进程或未提交输出留在交付工作树。

## Gate H：#27 → #28 发布交接

- [ ] `release-handoff.md` 明确已验收提交、migration version、维护命令完整 argv/退出码、marker/CAS/批次/checksum 不变量和独立初始化方式。
- [ ] 交接列出所有 stop-write blocker 及稳定 reason、三库版本/同一矩阵/零 SKIP 证据路径、32 CNY fixture 建立方式、ready/legacy Task/repair/suspend 边界。
- [ ] 交接明确 #28 必须重新观察的生产项目，不能把本地迁移结论伪装为生产验证。
- [ ] 交接不包含 DSN、令牌、Cookie、私钥、dump 或可识别用户数据。
- [ ] 协调器在 merge 前独立审阅并执行最小代表性矩阵；Worker 自报和粘贴日志不能替代实际验收。

## 集成与放行

1. 记录集成树基线、Worker HEAD、`merge-base`、提交列表和工作树清洁度。
2. 在 Worker 分支完成 Gate A–H；失败项返回原 Worker 修复并重新 `worker_done`/验收。
3. 通过后以 non-ff merge 集成，提交信息使用 `feat(valuation): 集成历史估值迁移门禁`。
4. 在集成树立即重跑历史价格、SQLite 迁移、marker/fail-closed、32 CNY 五接口和 `git diff --check`；确认 merge 未引入冲突回归。
5. 仅在合并后验收全部通过，才关闭 #27，并在关闭评论中写集成 SHA、三库真实版本/零 SKIP、迁移 version/checksum、32 CNY 结果和明确“尚未部署生产”。
6. 停止/释放 #27 Worker，仅使用 Orca 原生命令回收本 Run 创建的 #27 子工作树，再执行 `git worktree prune`；不得触碰主树、集成树、`account`、`disk` 或其他会话工作树。
7. 只有此时才允许从最新干净集成提交创建 #28 子工作树并派发发布 Agent。

## 不放行条件

- #27 从缺少任一 #21–#26 writer 的基线开工；
- #20 仍执行历史回填，或 #27 只诊断但不拥有回填/ready；
- 历史价格经过浮点、舍入、当前套餐价猜测或静默零值；
- 历史 Credit 被标为 exact，或最近订单/请求日志被用于猜消费顺序；
- dry-run/verify 发生写入，checksum 不稳定，或 apply 无 CAS/稳定批次；
- ready 允许部分状态、热路径自修或旧 writer 绕过深模块；
- 三库存在 SKIP、使用错误数据库版本，或以 mock/DryRun 代替真实矩阵；
- 32 CNY、disabled-plan、request restore、conversion、recovery 或邀请隔离回归；
- 未提交恢复/交接证据、工作树不干净，或越界执行生产发布。
