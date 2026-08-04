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

### 管理员同币种 exact ingress

- RED：`go test ./model -run TestAdminCreditBalanceIncreaseUsesSelectedPlanExactIngress -count=1`。
- RED 结果：编译失败，确认 `CreditBalanceAdjustmentRequest.PlanId`、结构化 ledger 字段和精确响应字段均缺失；代表公开领域合同尚不存在。
- GREEN：同一命令通过，`go test: 1 packages ok`，耗时约 13.91 秒。
- 行为：真实 SQLite ready marker 下，选择 `40 CNY / 1,000 Credit` 档位售后授予 800 Credit，得到 exact `32,000,000` micros CNY、state version 1，并结构化写入 plan、毛/净 Credit、源价格/分母、来源 key/status、指纹与 1:1 FX。

### 管理员 increase 资格矩阵

- GREEN：`go test ./model -run TestAdminCreditBalanceIncreaseRejectsIneligiblePlansAtomically -count=1` → `go test: 1 packages ok`，约 18.51 秒。
- 行为：缺少 plan、disabled、trial、invite trial、零/缺失精确价格、零 Credit 分母、未开放不限时购买、非 timed、EUR 均返回稳定 sentinel；真实 SQLite 中 adjustment、ledger、state、subscription、邀请事件全部保持 0，证明原子拒绝。
## 实际数据库/API/浏览器范围

- SQLite：已执行首个管理员领域纵切，真实内存 SQLite + GORM migration + ready marker。
- MySQL/PostgreSQL：本切片只做静态兼容审查；真实矩阵归 #27。
- API：尚未执行；当前仅 model 公开领域入口。
- 浏览器：尚未执行。
