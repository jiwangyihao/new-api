# Issue #24 验证证据

## 基线证据

- `git rev-parse HEAD` → `ec1858fec89509bdec9a90a230a8496047c5becd`。
- `git status --short` → 无输出，初始工作树干净。
- `.scratch/agent-progress/issue-20/contract.md`：确认 `price_amount_micros`、Credit 估值币种和整数比例合同存在。
- `.scratch/agent-progress/issue-22/contract.md`：确认窄 ingress、固定锁序、同事务数量/状态/ledger 与五接口 Credit 分流存在。

## 已核验实现事实

- #22 提供 `CreditValuationSourceSnapshot`、`newForwardCreditValuationIngress`、`ApplyCreditValuationIngressTx`。
- #22 ingress 负责毛成本、settlement debt 抵扣、净 Credit/净成本、exact 状态和 `state_version`，调用方不得自行重复计算状态。
- 当前 ingress 只接受同币种；跨币种普通 Credit 来源没有可消费的权威运行时 FX snapshot seam。
- 兑换现有事务锁定来源、完成 grant、写 fulfillment 并标记 redeemed；Credit 模式尚未传估值来源事实。
- 管理员 adjustment 现有指纹未包含 plan 与权威价格/币种/FX/规则快照。

## RED / GREEN 记录

尚未执行生产行为测试。后续每个垂直切片在此记录精确命令、失败原因、实现提交和 GREEN 输出。

## 实际数据库/API/浏览器范围

- SQLite：尚未执行。
- MySQL/PostgreSQL：本切片只做静态兼容审查；真实矩阵归 #27。
- API：尚未执行。
- 浏览器：尚未执行。
