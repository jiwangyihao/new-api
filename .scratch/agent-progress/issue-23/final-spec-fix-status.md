# Issue #23 最终 Spec F1/F2 修复状态

## 当前阶段

F1 已在 `07801e667` clean 提交；F2 已 GREEN 并通过单次、`-count=10`、窄 `-race`、timed/converted 兼容验证，待 clean 安全提交。

## 已完成

- 冻结现场、必读合同、最终 Spec FAIL 与历史证据已核验；诊断、TDD、模块设计与 Orca 调度约束已加载。
- F1：版本 1 SHA-256 不可变请求指纹覆盖 user、规范化 model、quota_type、distributor amount；异参/缺失指纹稳定失败关闭，相同参数无写入重放；双连接、`-count=10` 与窄 `-race` 通过。
- F2 RED：匿名 token/amount helper 对 Credit 返回成功；`PostConsumeQuota` 将 `token_used` 从 100 匿名增加却使 request record `applied_credit` 停留 100。
- F2 GREEN：两个导出匿名 helper 对 `credit_balance` 返回 `ErrCreditValuationAnonymousDeltaForbidden` 且零写入；timed 与 converted 原路径保持兼容。
- `PostConsumeQuota` 对 Credit 读取稳定 `RequestId` 的已提交 `applied_credit`，加本次 delta 得到累计 target，再调用 `SettleUserSubscriptionRequestTarget`；缺 request、映射冲突、负目标/溢出沿既有稳定错误失败关闭。
- F2 核心单次、`-count=10`、窄 `-race`、converted source 与 timed `PostConsumeQuota` 回归均通过；精确命令见 evidence。

## 下一步

1. `gofmt`、`git diff --check` 并提交 F2 clean 安全点。
2. 仅运行既定 Issue #23 聚焦回归与 `go test ./model ./service ./controller -count=1` 最终门禁。
3. 更新验收映射、确认 clean tree，报告协调者。

## 阻塞

无。

## 最近安全提交

- `07801e667`：F1 版本化不可变请求指纹与验证。
- `0ba04adfb`：记录 F1 clean 安全点。
- F2 GREEN 提交：待创建。
