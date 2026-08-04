# Issue #24 进度状态

## 当前阶段

管理员同币种 increase 资格、debt、完整参数指纹和事务回滚均已 GREEN，下一步只实现兑换同币种冻结快照、幂等、debt 与邀请隔离。

## 已完成

- 已确认工作树基线为 `ec1858fec89509bdec9a90a230a8496047c5becd`，初始工作树干净。
- 已读取父 PRD #19、Issue #24、`CONTEXT.md`、ADR 0001/0002、执行上下文、第二波次合同、规格目标章节和计划任务 4/9。
- 已读取 `skill://tdd` 与 `skill://codebase-design`。
- 已确认 #20 精确价格合同与 #22 `CreditValuation` ingress、状态、账本和五接口分析已集成。
- 已确认兑换和管理员 increase 应只消费 #22 的 `newForwardCreditValuationIngress` / `ApplyCreditValuationIngressTx`，不得重写移动平均或请求结算。
- 已完成管理员 increase 的 `plan_id`、权威档位事实、exact ingress、结构化 ledger 和精确响应首个真实 SQLite纵切。
- 已证明缺 plan、disabled/trial/invite、零/缺失精确价格、零分母、资格关闭、非 timed 和 unsupported currency 均原子拒绝，不留下 adjustment/ledger/state/subscription/邀请事件。
- 已证明部分/全额 debt offset 只把同比例净成本纳入 exact。
- 已证明完整指纹同 key 重放与参数/冻结价格变化冲突，重放不增加 state version。
- 已通过 SQLite ledger 故障注入证明 adjustment、ledger、state、subscription 同事务回滚。

## 下一步

1. 编写兑换同币种冻结快照 RED 并接入 #22 ingress。
2. 证明兑换重放/冲突、部分/全额 debt offset。
3. 证明兑换失败整笔回滚且不产生邀请奖励或邀请付费统计。
4. 写跨币种最小 RED/接口需求后 clean HANDOFF_READY。

## 阻塞

- #22 的 ingress 当前只支持来源币种等于 Credit 池估值币种；`model/credit_valuation.go` 明确将普通 Credit 跨币种 FX 接缝留给 #26。
- 协调器裁决：#24 不复制 FX parser/provider；先完成全部同币种行为，并保留跨币种最小 RED 与接口需求。跨币种 Gate D/E 等待 #26 提供唯一 `CreditFXRateSnapshot` seam。

## 最近安全提交

- 起始安全提交：`ec1858fec89509bdec9a90a230a8496047c5becd`。
- 恢复合同安全提交：`4e0640e2f`。
- 管理员 exact ingress 安全提交：`b07addec3`。
- 资格矩阵安全提交：`09b8775d0`。
- 管理员 debt/幂等/回滚 GREEN 待提交。
