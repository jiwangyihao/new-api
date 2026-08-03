# 第三执行波次共享合同（Issues #25 与 #26）

## 调度位置与派发门槛

本合同约束父 PRD #19 的破坏性恢复与转换估值切片。两个 Agent 都必须从协调器指定的、已经验收并集成其真实依赖的 `jiwangyihao/credit-operational-value-integration` 提交创建隔离子工作树，不得从 `origin/main`、生产提交或旧 worker 分支自行开工。

- Issue #25 的硬依赖是 #23 与 #24；派发前还应存在已验收的 #20/#22 基础合同。它负责管理员 decrease、Credit 订单退款、拒付和财务恢复的低频破坏性 outflow。
- Issue #26 的硬依赖是 #21、#22 与 #23；它不依赖 #24。它负责 timed→Credit 转换估值、运行时 CNY/USD 有理数 FX 快照，以及转换期间在途请求的虚拟扣除快照和后续结算。
- 若两者在同一时间窗口执行，协调器可并行派发，但每个 worker 只依据自己已满足的依赖开工。#26 不得因为 #24 尚未完成而复制管理员/兑换 ingress；#25 不得因为 #26 尚未完成而实现转换 outflow 或 FX。
- 两者都不得实现 #27 的历史回填、迁移命令、`ready/failed/suspended` 切换与三数据库发布门禁，也不得执行 #28 的镜像构建、生产部署、备份、生产验证或回滚。

开工前必须读取已集成的 `.scratch/agent-progress/issue-20`、`issue-21`、`issue-22`、`issue-23`，以及 #25 需要的 `issue-24` 合同。若预期接口或稳定错误不存在，先通过 Orca `orchestration ask` 报告具体缺口和可继续工作；不得在切片中复制上游实现。

## 主改所有权

### Issue #25：破坏性低频 outflow 主改者

#25 独占以下行为和接缝：

1. `ApplyCreditValuationOutflowTx` 的低频破坏性调用合同，以及 `RecoverCreditBalanceTx` 对数量、exact/estimated/unknown 成本和 settlement debt 的原子更新；
2. 管理员 Credit decrease 的后端请求校验、幂等指纹、领域调用、响应和 UI operation 切换行为；
3. Credit 订单退款、拒付、财务恢复的来源终态优先级、唯一身份、重放/冲突语义；
4. 破坏性 `CreditBalanceLedger` 结构化字段、邀请奖励取消与来源终态同事务；
5. outflow 与请求结算并发时的锁序、合法串行化结果及分析生命周期更新。

#25 只能消费 #23 的请求级恢复合同，绝不能改写活动请求的扣除快照，也不能按订单批次、订单价格或实际退款额撤回池成本。它不得为 timed 订单增加服务撤销或 grant reversal schema。

### Issue #26：转换、FX 与在途请求主改者

#26 独占以下行为和接缝：

1. `SubscriptionConversion` 的精确估值、单位价值有理数、CNY/USD FX、规则版本和冻结快照字段；
2. `ConfirmTimedSubscriptionConversion` 确认事务中的重新校验、数量公式、转换 ingress、源权益 converted 状态和活动权益接替；
3. 从 `operation_setting` 原始十进制配置解析并原子发布只读 `CreditFXRateSnapshot` 的运行时合同；
4. #23 预留的“转换前预扣、转换后结算”接缝：建立虚拟扣除快照而不二次扣减目标池，并完成少结算、追加、退款与幂等；
5. 转换报价/分析中转换估值与稳定错误的 API/UI/六语言展示。

#26 不得更改 #21 的 timed grant 计值模型，不得把转换当成新收款或邀请收入，不得修改管理员 increase/decrease、订单退款/拒付或兑换来源。动态汇率重估和 CNY/USD 以外 Credit 入账均禁止。

## 共享深模块与文件冲突规则

`model/credit_valuation.go`、`model/subscription.go`、预扣记录类型和低频 ledger 类型可能被两个切片间接触及，但没有任何一个 Agent 拥有整个文件：

- #25 只扩展低频 outflow/recovery 分支；不得修改 request 快照算法、转换映射或 FX 构造器。
- #26 只扩展 conversion ingress、FX 与转换请求桥接分支；不得修改通用 recovery 终态、管理员 decrease 或退款/拒付路由。
- 共同数据结构只能做向后兼容的附加字段或窄 helper。若需要改变 #22/#23 的既有接口，必须先发送 Orca ask，列出符号、调用点、所需新合同和未改部分。
- 固定锁序保持：领域来源行可先锁；进入 Credit 模块后为目标 `UserSubscription` → `CreditValuationState` → 请求记录或 ledger 结果。转换后结算先按稳定 ID 锁原预扣记录与转换映射，再锁目标权益与状态。禁止反向锁。
- 两者都必须通过 CreditValuation 深模块同时更新 `token_limit/token_used` 和估值状态；controller/service 不得先改数量再补价值。

协调器集成时按可观察合同合并，不机械选择一侧。若同批均完成，优先验收独立领域测试；默认可先集成 #25 的低频 outflow，再集成 #26 的 conversion/request 分支并解决纯附加字段冲突。该顺序不改变 GitHub blocker 关系。

## 冻结业务不变量

- 破坏性出账统一按操作前混合池：`C=min(Q,A)`，分别 floor 移除 exact、estimated、unknown；`C=A` 时带走全部舍入余数，`Q-A` 只形成 settlement debt。成本永不为负。
- 管理员 decrease、Credit 退款、拒付、财务恢复不得按原订单价格、支付额或来源批次撤值。来源资料只用于审计、幂等和终态优先级。
- 活动请求快照属于 #23：低频 outflow 不回写它。之后请求退款仍按请求原快照恢复，并由同一锁序与低频 outflow 串行化。
- 转换数量严格为 `full_31_day_blocks × credit_basis + current_remaining_credit`；31 天为业务月，不对部分周期按秒折算。估值使用同一份冻结 `credit_basis`。
- 转换单位价值保存未舍入的 `valuation_source_price_micros / valuation_credit_basis`。同币种 FX 为 1/1；CNY/USD 使用 ingress 时从配置原始十进制文本解析、约分并冻结的正有理数；禁止由 `float64` 反推。
- 转换时已预扣量不包含在 `current_remaining_credit`，不得重复入账。转换后首次结算只建立虚拟成本快照，不再次扣目标 Credit；少用按转换快照恢复，多用按目标池当时平均出账。
- 请求日志继续保留原 `subscription_id`，估值记录可指向目标 Credit `valuation_subscription_id`。所有重放依据持久化请求/转换记录，不依赖进程内布尔值。
- 新分配动作继续拒绝 disabled/ineligible 套餐；已有 disabled-plan 权益保持可消费。所有套餐继续忽略 `model_limits`。Credit 转换和回收不产生邀请收入。
- marker 非 ready 时严格服从 #20/#27 的前向快照合同；不得创建半可信历史状态、回填历史或切换门禁。

## 并行、持久化与恢复

每个 worker 的第一项实际改动必须创建并提交 `.scratch/agent-progress/issue-<N>/{status,evidence,contract}.md`。`contract.md` 先写明本文件中的所有权、预计共享文件、锁序、幂等身份、稳定错误和明确非所有权。每个可编译、可验证小步立即使用 Conventional Commits 提交；关键实现、调查结论和失败根因不得只留在终端、大段临时脚本或未提交工作树。

发生异常时，worker 应先把当前阶段、最近安全提交、未提交文件和下一条 RED/GREEN 命令写入 status/evidence，再发送 question/escalation。协调器只在 `worker-show` 明确证明 failed/stopped 后才用 `--retry-of` 补发；运行时间长或 `check --wait` 超时不是重派理由。

完成后各 worker 只发送一次 `worker_done`，列出提交 SHA、领域/API/UI 合同、定向测试、真实 SQLite/API/浏览器证据、并发或 race 证据、共享文件、明确未实现范围和进度目录。Agent 不关闭 Issue、不合并、不部署、不回收工作树。协调器分别验收后再做联合回归。
