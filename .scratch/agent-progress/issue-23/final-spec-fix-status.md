# Issue #23 最终 Spec F1/F2 修复状态

## 当前阶段

F1/F2 已实现、验证并提交；最终门禁、SQLite 请求链、三包宽回归与窄 `-race` 全部通过。本文件记录最终可交付状态，提交后向协调者报告。

## 已完成

- F1：版本 1 SHA-256 不可变请求指纹覆盖 user、规范化 model、quota_type、distributor amount；异参/缺失指纹稳定失败关闭，相同参数无写入重放；双连接、`-count=10` 与窄 `-race` 通过。
- F2：匿名 token/amount helper 对 `credit_balance` 返回 `ErrCreditValuationAnonymousDeltaForbidden` 且零写入；`PostConsumeQuota` 使用稳定 `RequestId`、原 `SubscriptionId` 和不可变 `pre_consumed + delta` 调用 request-aware 累计目标入口，相同调用重放无写入；timed/converted 保持兼容。
- 生产提交：`2bb68e770`（禁止 Credit 匿名结算）、`45b9d64f4`（保持同步结算重放幂等）。
- 回归夹具提交：`dc333c928`；仅把旧 Credit 匿名写入夹具迁移到 request-aware/entitlement-guard 合同。
- 最终聚焦 request/coalescer/Task/cleanup/double-count 正则、真实 SQLite 请求链、F1/F2 `-count=10`、相关窄 `-race` 均 PASS。
- `go test ./model ./service ./controller -count=1` PASS；修改 Go 文件已 `gofmt`；`git diff --check` PASS。

## 验收映射

- Issue #23 AC1：预扣数量/估值/请求指纹同事务；四类冲突、缺指纹、重放和双连接验证通过。
- AC2–6：既有累计 target、快照恢复、欠额、absorbed、coalescer 聚焦回归通过。
- AC7：Credit 匿名 helper 失败关闭；`PostConsumeQuota` request-aware；timed/converted 明确兼容。
- AC8–12：Task identity、conversion seam、cleanup、错误回滚和并发/race 聚焦回归通过。
- #22 冻结 800 available / 32 CNY tracer 与五接口仍由三包宽回归覆盖通过；未实现 #24–#28。

## 未运行范围

- 未运行真实 MySQL 5.7/PostgreSQL 9.6、全项目测试或部署；三数据库零 SKIP 留给 #27，发布留给 #28。
- #26 seam 保持 `original_subscription_id + valuation_subscription_id + request_id`，未实现转换单位价值、FX 或虚拟快照。

## 最近安全提交

- `07801e667`：F1 版本化不可变请求指纹与验证。
- `0ba04adfb`：记录 F1 clean 安全点。
- `2bb68e770`：禁止 Credit 匿名结算并迁移 `PostConsumeQuota`。
- `45b9d64f4`：修复 `PostConsumeQuota` 相同调用重放累加。
- `dc333c928`：迁移 Credit 结算回归夹具。
- 最终状态与证据：本文件所在提交。
