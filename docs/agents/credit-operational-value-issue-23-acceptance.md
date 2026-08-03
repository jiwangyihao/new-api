# Issue #23 独立验收门禁

## Gate A：Worker 交付与可恢复性

- [ ] Orca Dispatch 已发送且只发送一次 `worker_done`；`worker-show` 记录最终状态、终端和失败计数。
- [ ] Worker 分支源自已经验收并集成 #20、#21、#22 的共同基线；记录共同基线、Worker HEAD 和 `merge-base`。
- [ ] `.scratch/agent-progress/issue-23/{status,evidence,contract}.md` 已提交，状态、领域合同、RED/GREEN 命令、已运行范围和遗留风险与代码一致。
- [ ] 所有实现和测试均已 Conventional Commit；Worker 工作树干净，关键成果不只存在于终端、临时脚本或未提交文件。
- [ ] 修改导出符号前的 references 结果、匿名 Credit delta 调用点清单和最终迁移清单均可追溯。

## Gate B：切片所有权与非目标

- [ ] #23 只深化 #22 的最小同步 request tracer，主改 request-aware 预扣/结算、请求快照、调用链身份传播、合并器、异步 Task 与清理策略。
- [ ] 未复制或重写 #22 的购买 ingress、移动平均核心、低频来源构造器、通用 analytics DTO 或 32 CNY 基线。
- [ ] 未实现 #24 的兑换/管理员 increase、#25 的 decrease/退款/拒付/财务恢复、#26 的转换单位价值/FX/虚拟快照、#27 的历史迁移/ready、#28 的发布。
- [ ] 转换期间请求只留下稳定 `valuation_subscription_id` 与目标路由接缝；没有在本切片伪造转换成本或跨币种价值。
- [ ] Credit 请求不再从 controller/service/relay/异步路径绕过深模块；若旧匿名 helper 为 timed 保留，调用范围已明确限制并有引用证据。

## Gate C：请求级领域合同

- [ ] 真实预扣在同一事务、固定锁序下更新目标 `UserSubscription`、`CreditValuationState` 与以 `request_id` 唯一的记录；故障注入证明任一步失败全部回滚。
- [ ] 请求记录持久化 `applied_credit`、`deducted_available_credit`、`debt_formed_credit`、`valuation_subscription_id`、exact/estimated/unknown 活动扣除快照、absorbed restore、restored unknown、规则/结算/状态版本及终态。
- [ ] 预扣必须足额且不形成欠额；相同请求和相同不可变参数重放原结果，不重复扣除；订阅、数量或参数冲突返回稳定 code/sentinel 并整笔回滚。
- [ ] 唯一结算入口接受 `request_id + original_subscription_id + target_applied_credit + final`，不接受 Credit 匿名 delta。请求日志保留原订阅身份，估值可路由至目标 Credit 权益。
- [ ] 目标增加按追加当时混合池分别移除 exact/estimated/unknown；超出可用量只形成结算欠额，不产生虚构成本。
- [ ] 目标相同为严格无操作，不递增 `state_version` 或 `settlement_version`；终态相同目标可重放，终态后非法增加稳定拒绝。
- [ ] 目标减少先撤销本请求尚未撤销的欠额，再从本请求活动快照按比例恢复；清空快照时吸收全部舍入余数，不使用退款时的新池平均。
- [ ] 只将退款前后新增可用量写回物化价值；仍被其他欠额吸收的成本进入结构化 absorbed audit；后来入账已抵债后退款重新形成的可用量标记 unknown。
- [ ] 负目标、记录/状态缺失、数量或映射不一致、终态冲突和算术溢出均有稳定错误及原子回滚测试，业务分支不解析错误文本。

## Gate D：同步、流式、合并器与异步链路

- [ ] `SubscriptionFunding` 与 `BillingSession` 持久传播稳定请求 ID 和目标累计量；同步 quota、流式增量、重算、最终结算和失败退款全部调用同一领域入口。
- [ ] 正 delta 合并器保留逐请求身份、目标与稳定入队顺序；共享事务内逐条校验、舍入、写回，结果与同序逐条事务完全一致，禁止按 subscription 匿名求和。
- [ ] 两个请求交错 ingress/consume/restore 的测试只接受合法串行化结果集合，不依赖 goroutine 调度碰巧通过。
- [ ] 新 Task 持久化 `subscription_request_id`，创建、轮询、重算和失败退款复用同一 ID；进程重启后的重放不重复扣款或退款。
- [ ] 旧 Task 用持久化 Task 主键生成确定性兼容身份，不使用时间、随机数或进程内布尔值；兼容路径仍进入 Credit 深模块。
- [ ] 清理器只删除已终态且超过“最大异步生命周期 + 运维保留窗口”的记录；非终态永不因固定天数删除，保留参数和只读诊断可测试。

## Gate E：真实数据库、并发与 race 证据

- [ ] 真实 SQLite 领域测试通过公开领域入口覆盖预扣、追加、少结算、失败退款、终态重放、交错入账和原子回滚；不得直接插入请求快照冒充主路径。
- [ ] 定向数据库并发测试覆盖同 request 重放/冲突、两个 request 稳定顺序、consume+restore、合并器与逐条等价，并验证数量、成本、版本、欠额和审计无漂移。
- [ ] 合并器相关 Go `-race` 定向检查通过，且不以 race 结果代替真实数据库并发证明。
- [ ] SQL、索引和锁语义经跨库静态审查；未把 GORM DryRun 声称为 MySQL/PostgreSQL 验收。完整三库零 SKIP 明确保留给 #27。

## Gate F：真实请求链与可观察结果

- [ ] 启动本地真实应用或既有可控 mock upstream，至少走通“预扣 → 增量追加 → 少结算或失败退款”，记录实际 request ID、目标累计量、数量、估值和终态。
- [ ] 真实请求日志仍关联原 `subscription_id`；估值记录按合同关联 `valuation_subscription_id`，五个运营分析接口在结算前后显示一致剩余价值。
- [ ] 验证相同目标重放不变、不同目标按规则变化、错误响应暴露稳定 code；测试不依赖内部表直接构造主链事实。
- [ ] 若切片没有新增 UI，明确记录“不需要浏览器 UI 变更”的证据；若新增任何可见文案，则补齐 en、zh、fr、ru、ja、vi 并执行 i18n missing/extras 检查。

## Gate G：集成前回归与放行

- [ ] 格式化仅触及明确修改文件；运行受影响 Go 包测试、真实 SQLite tracer、请求链 smoke、定向 race 和 `git diff --check`。
- [ ] #20 精确价格、#21 timed grant、多币种分析及 #22 冻结 32 CNY Credit tracer 均保持通过；Credit `time_based_value` 仍为 null。
- [ ] 完成报告逐条映射 GitHub Issue #23 acceptance criteria，列出提交 SHA、命令、关键输出、未运行范围和后续 #26 接缝。
- [ ] 协调器只有在以上门禁全部有证据后才能 non-ff merge；失败项返回原 Worker 修复，不得在协调器分支偷偷补写大段实现。

## 不放行条件

- request ID 只存在于内存或日志，数据库无法在重启后重放；
- Credit 仍存在 controller/service 可调用的匿名 delta 绕路；
- 合并器先求和再结算，舍入或错误无法逐请求归属；
- 退款使用当前池平均、恢复后来 ingress 的成本，或把 absorbed restore 写成可用价值；
- 旧 Task 兼容身份依赖当前时间/随机数；
- 只有 mock/直接插表/单线程测试，没有真实 SQLite 请求与并发证据；
- 越界实现 #24–#28，或为了通过测试削弱 #20–#22 合同。
