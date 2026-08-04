# Issue #22 Gate C 与 UI 收敛恢复指令

## 恢复目标

你接管父 PRD #19、GitHub Issue #22 的既有隔离工作树：

`C:/Users/34404/source/repos/new-api/.workspaces/new-api/issue-22-credit-tracer`

本次不是重新实现 Issue #22。开始时必须确认工作树 HEAD 为 `6d8d001867a6922eb1a8da9df08befa69a037d1b`、`git status --short` 为空，并完整读取：

1. 仓库与全局 `AGENTS.md`；
2. `issue://jiwangyihao/new-api/19`、`issue://jiwangyihao/new-api/22`；
3. `docs/agents/credit-operational-value-execution.md`；
4. `docs/agents/credit-operational-value-wave-1-contract.md`；
5. `docs/agents/credit-operational-value-issue-22.md`；
6. `.scratch/agent-progress/issue-22/{status,evidence,contract}.md`；
7. 本恢复指令。

既有领域与分析提交是恢复基线，不得重做：订单冻结来源、`CreditValuationState`、最小同步 `request_id` 预扣/同目标最终结算、32 CNY 五接口与通用 micros DTO 已分别由 `d6a493c75`、`e03e62905`、`06619f81b`、`452a75ccd` 等提交完成。UI WIP/RED 已由 `91df0bd08` 保存，交接说明由 `6d8d00186` 保存。

## 必须使用的 Skill 与工作方式

这是永久 feature。开始实现前读取并使用 `skill://tdd`，每个缺口必须有真实 RED，再做最小 GREEN。数据库、事务、支付回调或结算行为不符合预期时读取 `skill://diagnosing-bugs`；深模块边界不清时读取 `skill://codebase-design`，但不得重设计冻结合同。修改 `web/default` 前读取 `skill://shadcn-ui`；增加或修改可见文案前读取 `skill://i18n-translate`，维护 en、zh、fr、ru、ja、vi 六语言。

先更新 `.scratch/agent-progress/issue-22/status.md`，写明恢复 HEAD、当前阶段、下一条 RED 与禁止范围；每个 RED/GREEN 命令和关键响应写入 `evidence.md`；若发现接口边界变化，先写 `contract.md`。每完成一个可恢复、可验证的小步立即 Conventional Commit。不要把重要成果只放在终端、内存或一次性长脚本中。短脚本或临时输出放 `.scratch/agent-progress/issue-22/`，不得散落仓库。

## 第一优先级：Gate C 两个真实购买入口与 BillingSession

只补以下最小垂直闭环，不重新实现已完成后端 tracer。

### A. 人民币余额购买

从现有 HTTP/领域入口开始：`controller/subscription_payment_balance.go::SubscriptionRequestBalance` → `service.CreateBalanceSubscriptionOrder` → `model.CompleteSubscriptionOrderTx`。复用 `controller/subscription_balance_purchase_test.go` 的真实 Gin + SQLite 夹具，不得直接插入 `CreditValuationState`。

测试必须：

- 配置零价全局 Credit 容器与有价充值档位 `40 CNY / 1,000 Credit`；
- 使用前向 ready 前置（测试可显式预置，但生产代码不得创建/CAS/切换 migration marker）；
- 从人民币余额 HTTP 入口完成购买；
- 断言订单不可变快照含 `40,000,000` micros、1,000 Credit、CNY 和稳定来源身份；
- 断言完成后唯一 Credit entitlement 的 `available=1000`、exact=`40,000,000`、estimated=0、unknown=0；
- 断言重复/失败路径不重复入账且事务原子。

### B. 一个受控外部支付入口

使用仓库现有 Kyren fake checkout 与签名 webhook：`controller/subscription_payment_kyren_test.go`、`performSignedKyrenWebhook`。不得发真实外部网络请求，不得新增支付 API。

测试必须：

- 创建订单时冻结 `40 CNY / 1,000 Credit`；
- 订单创建后修改当前充值档位价格；
- 通过真实签名 webhook 完成订单；
- 断言入账仍使用订单创建时冻结的 `40,000,000` micros，而非改价后的计划或渠道实收金额；
- 断言重复 webhook 幂等；
- 保留 disabled-plan 边界：已授权订单仅可按既有不可变快照履约，新购买仍拒绝 disabled/不合格档位。

### C. BillingSession 同步消费 200

从上述任一真实购买结果继续，不另造状态。复用 `service.NewBillingSession`、`PreConsumeBilling`、`SettleBillingWithInput` 与 `SubscriptionFunding.PreConsume`，让真实 `relayInfo.RequestId` 传播到已有最小 request seam。

只验证 Issue #22 已冻结的单一场景：足额同步预扣 200，并以相同目标累计 200 最终结算；断言最终 available=800、exact=`32,000,000` micros CNY、estimated=0、unknown=0、状态版本符合一次有效扣除；重复相同目标幂等。不要实现目标增加/减少、少结算、退款、异步 task identity 或 coalescer，这些全部属于 #23。

Gate C 完成后立即更新 evidence/status 并提交一个安全点，再进入 UI。不要扩展支付提供商、重构 BillingSession 或增加新 API。

## 第二优先级：收敛既有 UI RED

从 `91df0bd08` 的前端现场继续：

- `amount_micros` 优先于兼容 float；超出 JS safe integer 的金额使用字符串/BigInt 格式化；
- Credit 明细显示 exact/estimated/unknown、时间值“不适用”、moving-weighted 术语和 `current_only` 非阻断提示；
- 修复 `paid-value-panel.test.tsx` 的 TanStack `RouterProvider` 测试夹具，不通过删除 Link、绕过页面或弱化断言解决；
- 保持 #21 的 timed 扩展 seam，不覆盖其逐币种 timed 实现；
- 所有新增可见文案补齐六语言并运行 i18n missing/extras 检查。

只运行明确相关的前端测试、typecheck 和 production build；不要运行全仓大套件。使用真实 default UI 和真实本地 SQLite/API 做浏览器 smoke，不得用静态资源拦截伪造通过。浏览器至少证明 32 CNY、Credit 时间值不适用、moving-weighted、confidence/current-only 字段在真实页面中可观察；记录 URL、关键 API 字段和 UI 文本，清理服务、标签与临时数据库。

## 严格非所有权

不得实现：

- #23 的通用 target 追加/减少、退款、异步 `subscription_request_id`、coalescer；
- #24 的兑换与管理员售后正向入账；
- #25 的退款、拒付、破坏性恢复；
- #26 的 FX、有理数汇率、转换中在途请求；
- #27 的历史回填、marker 创建/CAS/状态迁移、`ready/suspended` 切换或三数据库零 SKIP；
- #28 的生产发布。

生产代码对 marker 只能读取 predicate；测试可以构造必要前置，但不得把 marker 生命周期偷渡进本切片。运行时只覆盖同币种 CNY→CNY，不新增 FX。

## 完成条件

完成前逐条复核 GitHub #22 与 `docs/agents/credit-operational-value-issue-22-acceptance.md`。至少提供：

1. 人民币余额真实入口 RED/GREEN；
2. Kyren 签名 webhook 改价后仍用冻结快照的 RED/GREEN；
3. BillingSession 真实 `request_id` 预扣/相同目标 200 结算证据；
4. 32 CNY 五接口既有 tracer 回归；
5. 相关前端测试、typecheck、Rsbuild build 与六语言检查；
6. 真实 SQLite + 真实浏览器 smoke；
7. `git diff --check` 与 clean 工作树；
8. 明确 MySQL/PostgreSQL 实测范围，不得把无 DSN 的 SKIP 宣称为 PASS。

全部代码、测试和 progress 文件提交后，只发送一次有效 `worker_done`，包含最终 HEAD、提交列表、真实入口/SQLite/API/UI 证据、共享文件、未实现范围与风险。不要合并、关闭 Issue、部署或回收工作树。
