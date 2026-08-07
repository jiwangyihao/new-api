# Issue #24 进度状态

## 当前阶段

IN_PROGRESS：复评确认唯一 blocker 为 H2；严格只在管理员 increase 与 redemption 的既有事务中消费 #26 `CurrentCreditFXRateSnapshot`/`ParseCreditFXRateSnapshot` seam，先完成后端分组 GREEN 安全提交，再进入 API/UI/六语言/browser。

## 已完成

- 已确认工作树基线为 `ec1858fec89509bdec9a90a230a8496047c5becd`，初始工作树干净。
- 本轮冻结基线：当前子工作树 HEAD/baseRef/merge-base 均为 `6f865feca3cd517a3dd744e67ea1240d5001d2ed`；`git status --porcelain=v1` 无输出。
- Orca 子工作树 ID 为 `1bd24578-ec8b-4492-961c-108ab229f4e7::C:/Users/34404/source/repos/new-api/.workspaces/new-api/issue-24-final`，`parentWorktreeId` 为 `1bd24578-ec8b-4492-961c-108ab229f4e7::C:/Users/34404/source/repos/new-api`，lineage capture 为 `explicit-cli-flag`。
- 已读取父 PRD #19、Issue #24、`CONTEXT.md`、ADR 0001/0002、执行上下文、第二波次合同、规格目标章节和计划任务 4/9。
- 已读取 `skill://tdd` 与 `skill://codebase-design`。
- 已确认 #20 精确价格合同与 #22 `CreditValuation` ingress、状态、账本和五接口分析已集成。
- 已确认兑换和管理员 increase 应只消费 #22 的 `newForwardCreditValuationIngress` / `ApplyCreditValuationIngressTx`，不得重写移动平均或请求结算。
- 已完成管理员 increase 的 `plan_id`、权威档位事实、exact ingress、结构化 ledger 和精确响应首个真实 SQLite纵切。
- 已证明缺 plan、disabled/trial/invite、零/缺失精确价格、零分母、资格关闭、非 timed 和 unsupported currency 均原子拒绝，不留下 adjustment/ledger/state/subscription/邀请事件。
- 已证明部分/全额 debt offset 只把同比例净成本纳入 exact。
- 已证明完整指纹同 key 重放与参数/冻结价格变化冲突，重放不增加 state version。
- 已通过 SQLite ledger 故障注入证明 adjustment、ledger、state、subscription 同事务回滚。
- 已通过真实 `Redeem` 入口冻结兑换档位精确价格、Credit 分母、目标估值币种、规则版本与稳定来源身份；套餐后续改价不回写 ledger、fulfillment 或状态。
- 已证明兑换相同身份重放同一 ledger/state version；篡改冻结来源价格后复用来源身份返回稳定 `credit_valuation_idempotency_mismatch`，余额与估值状态不变。
- 已证明兑换部分/全额 settlement debt 优先抵扣：只把净新增 Credit 和同比例 floor 后净成本写入 exact 状态。
- 已通过 SQLite ledger trigger 证明兑换来源终态、余额、估值状态、ledger 与日志整笔回滚；兑换记录恢复为未使用且保留原始冻结快照。
- 已通过现有服务纵切证明 Credit 兑换不创建邀请奖励事件、佣金记录/账户，也不计入邀请付费资格。
- 已保持 #27 marker 未 ready 的历史基线：缺少精确估值事实时不在 #24 创建半可信状态；marker ready 后 `GrantCreditBalanceTx` 仍对 nil source 失败关闭。
- 已保留跨币种最小可执行合同：CNY 来源、USD 估值必须消费冻结的有理数 FX 快照并返回结构化 FX；当前因 #22 仅支持同币种而真实 RED，未在 #24 实现 parser/provider/Option。
- H2 兑换跨币种已 GREEN：CNY→USD、USD→CNY 均在已锁定兑换事务内消费 `CurrentCreditFXRateSnapshot`，冻结 FX 进入 fulfillment、完整指纹、结构化 ledger 与重放响应；Option 变化后重放仍观察首次快照。
- 缺失 FX 返回稳定 `credit_valuation_invalid_fx` 且整笔无写入；跨币种 ledger 故障证明兑换状态、fulfillment FX、Credit、估值状态与 ledger 同事务回滚。
- redemption H2 安全提交：`49b1ece48`（`feat(subscription): 冻结兑换跨币种估值快照`）；提交后定向复验再次 PASS，`gofmt`、`git diff --check` 通过且工作树 clean。

## 下一步

1. 完成 H2 后端：双向 FX 冻结、完整指纹、结构化 ledger/API/replay、invalid FX 与故障回滚。
2. 运行后端分组测试并创建安全提交；之后接通 API/UI、六语言与真实 browser 验收。
## 阻塞

- 无外部阻塞；禁止修改或复制 #26 parser/provider、Option 生命周期及 H1/M1/M2/M3，也不触碰 #25/#27/#28。
## 最近安全提交

- 起始安全提交：`ec1858fec89509bdec9a90a230a8496047c5becd`。
- 恢复合同安全提交：`4e0640e2f`。
- 管理员 exact ingress 安全提交：`b07addec3`。
- 资格矩阵安全提交：`09b8775d0`。
- 管理员 debt/幂等/回滚安全提交：`34b536821`。
- 兑换同币种冻结快照安全提交：`32d55638e`。
- 兑换幂等安全提交：`a41fcdcc8`。
- 兑换 debt offset 安全提交：`a33f6f012`。
- 兑换回滚/邀请隔离安全提交：`9345fd18a`。
- 跨币种最小 RED/接口需求安全提交：`91b5a8384`。
- 最终验证记录：本文件所在 HANDOFF_READY 提交。
