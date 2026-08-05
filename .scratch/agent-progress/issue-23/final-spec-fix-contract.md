# Issue #23 最终 Spec F1/F2 修复合同

## 冻结基线

- 分支：`jiwangyihao/issue-23-request-settlement`
- 起始 HEAD：`8cdfd4acb78b502af4c0232460baf7df852b7b2c`
- 集成基线与 merge-base：`ec1858fec89509bdec9a90a230a8496047c5becd`
- 范围：只关闭最终 Spec 复评的 F1、F2；不重新设计 #23，不实现 #24–#28。

## F1：预扣请求指纹

- `SubscriptionPreConsumeRecord` 附加持久化版本化、确定性、碰撞安全的规范化请求指纹。
- 指纹至少覆盖 `user_id`、规范化 `model_name`、`quota_type`、`distributor_amount`，并使用无分隔符歧义的固定字段编码。
- 指纹与请求记录在预扣事务中原子创建；既有 `request_id` 必须先比较指纹，再读取并返回旧业务结果。
- 相同指纹重放严格无写入；任一不可变字段变化返回导出稳定 sentinel，调用层使用 `errors.Is`。
- 缺失或无法证明一致性的历史/旁路指纹一律失败关闭，热路径不得补造可信指纹。
- schema 仅附加字段；保留 request_id 唯一键、请求快照、cleanup 与 Task 引用合同；不切换 migration marker、不回填历史。

## F2：禁止 Credit 匿名 delta

- 导出的 token/amount 匿名 delta helper 在任何写入或入 coalescer 前识别目标权益；`credit_balance` 返回稳定 sentinel。
- timed 与既有 converted source 的合法兼容路径继续工作，但不得允许直接对 Credit target 匿名写入。
- `PostConsumeQuota` 对 Credit 必须使用 `RelayInfo.RequestId`、原 `SubscriptionId` 与累计目标调用现有 request-aware 领域入口；成功 final、少结算、失败退款复用同一身份。
- request_id 缺失、负目标、溢出、映射冲突与终态冲突失败关闭并零写入；不得临时生成 request_id。
- 优先复用 `SubscriptionFunding` / `BillingSession` seam；不复制移动平均、请求快照、退款或 coalescer 算法。

## 验证合同

- F1：完整参数重放、四类冲突、零写入、`-count=10`、真实 SQLite 并发/故障注入、窄 `-race`。
- F2：Credit token/amount 匿名拒绝、timed/converted 兼容、`PostConsumeQuota` request-aware final/少结算/退款/重放/错误隔离、全调用点清单。
- 回归：#23 request、BillingSession、coalescer、Task identity、cleanup、double-count、Kyren 800 Credit / 32 CNY tracer。
- 宽验证：`go test ./model ./service ./controller -count=1`、修改 Go 文件 `gofmt`、`git diff --check`、最终工作树干净。
- 明确未运行：真实 MySQL/PostgreSQL、全项目测试、部署；这些仍属于 #27/#28。
