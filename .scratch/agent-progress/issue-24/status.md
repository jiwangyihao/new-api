# Issue #24 进度状态

## 当前阶段

同币种管理员 increase exact ingress 与完整资格矩阵已 GREEN，按协调器收敛进入 debt offset、完整指纹重放/冲突和事务回滚。

## 已完成

- 已确认工作树基线为 `ec1858fec89509bdec9a90a230a8496047c5becd`，初始工作树干净。
- 已读取父 PRD #19、Issue #24、`CONTEXT.md`、ADR 0001/0002、执行上下文、第二波次合同、规格目标章节和计划任务 4/9。
- 已读取 `skill://tdd` 与 `skill://codebase-design`。
- 已确认 #20 精确价格合同与 #22 `CreditValuation` ingress、状态、账本和五接口分析已集成。
- 已确认兑换和管理员 increase 应只消费 #22 的 `newForwardCreditValuationIngress` / `ApplyCreditValuationIngressTx`，不得重写移动平均或请求结算。
- 已完成管理员 increase 的 `plan_id`、权威档位事实、exact ingress、结构化 ledger 和精确响应首个真实 SQLite纵切。
- 已证明缺 plan、disabled/trial/invite、零/缺失精确价格、零分母、资格关闭、非 timed 和 unsupported currency 均原子拒绝，不留下 adjustment/ledger/state/subscription/邀请事件。

## 下一步

1. 证明管理员 increase 全额/部分 debt offset。
2. 证明完整参数指纹同 key 重放/冲突。
3. 证明 ledger/终态故障整笔回滚。
4. 完成兑换同币种冻结快照、幂等、debt offset 与邀请隔离。

## 阻塞

- #22 的 ingress 当前只支持来源币种等于 Credit 池估值币种；`model/credit_valuation.go` 明确将普通 Credit 跨币种 FX 接缝留给 #26。
- 协调器裁决：#24 不复制 FX parser/provider；先完成全部同币种行为，并保留跨币种最小 RED 与接口需求。跨币种 Gate D/E 等待 #26 提供唯一 `CreditFXRateSnapshot` seam。

## 最近安全提交

- 起始安全提交：`ec1858fec89509bdec9a90a230a8496047c5becd`。
- 恢复合同安全提交：`4e0640e2f`。
- 管理员 exact ingress 安全提交：`b07addec3`。
- 资格矩阵 GREEN 待提交。
