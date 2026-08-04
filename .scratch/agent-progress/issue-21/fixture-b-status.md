# Issue #21 夹具迁移 B 状态

状态：COMPLETE

## 冻结现场

- 工作树：`C:/Users/34404/source/repos/new-api/.workspaces/new-api/issue-21-fixture-b-service`
- 冻结基线：`774b35740c1879b285537031410731317d0142fc`
- 起始工作树：clean
- 最近安全提交：`934d7ba10`（收口兑换夹具迁移实跑证据）
- 当前未提交文件：仅本状态校准；最终提交后为 0
## 已完成

- 已读取调试、TDD、Orca、父 PRD #19、Issue #21/#22、共享迁移合同、执行协议、Issue #21 验收、ADR/spec 相关合同与冻结 `final-spec-fix-*`。
- 已确认当前 HEAD 精确等于冻结基线，且工作树起始 clean。
- 已运行未经修改的 `go test ./service -count=1`，复现邀请 Credit 兑换夹具失败。
- 已将 Credit redemption 夹具从 `model.DB.Create` 迁移到 `Redemption.Insert`，并显式断言 `FulfillmentSnapshot` 非空。
- 已保留真实 `Redeem`、邀请处理和原有 Credit 邀请隔离断言。

## 完成结果

- RED 已持久化于 `1866aa042`；Credit redemption 测试旁路 `Redemption.Insert`，缺少 `FulfillmentSnapshot` 后被生产路径稳定 fail-closed。
- GREEN 已持久化于 `df6531cf6`；合法 option Plan 使用权威整数 micros，Redemption 经 `Insert` 冻结快照，再走真实 Redeem/邀请处理调用链。
- 迁移后最小子测试与 `go test ./service -count=1` 已再次实际运行并通过；原邀请隔离断言完整保留。
- 下一步：提交本状态校准，通过最终 diff/clean 检查后发送 `worker_done`。
## 阻塞

无。
