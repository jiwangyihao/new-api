# Issue #21 夹具迁移 B 状态

状态：VERIFYING

## 冻结现场

- 工作树：`C:/Users/34404/source/repos/new-api/.workspaces/new-api/issue-21-fixture-b-service`
- 冻结基线：`774b35740c1879b285537031410731317d0142fc`
- 起始工作树：clean
- 最近安全提交：`1866aa042`（持久化最小 RED）
- 当前未提交文件：`service/invitation_commission_test.go` 与本次 GREEN 证据更新
## 已完成

- 已读取调试、TDD、Orca、父 PRD #19、Issue #21/#22、共享迁移合同、执行协议、Issue #21 验收、ADR/spec 相关合同与冻结 `final-spec-fix-*`。
- 已确认当前 HEAD 精确等于冻结基线，且工作树起始 clean。
- 已运行未经修改的 `go test ./service -count=1`，复现邀请 Credit 兑换夹具失败。
- 已将 Credit redemption 夹具从 `model.DB.Create` 迁移到 `Redemption.Insert`，并显式断言 `FulfillmentSnapshot` 非空。
- 已保留真实 `Redeem`、邀请处理和原有 Credit 邀请隔离断言。

## 当前阶段

- GREEN：最小 Credit redemption 子测试通过。
- GREEN：`go test ./service -count=1` 包级通过。
- 下一步：提交实现与证据，执行 diff/clean 检查并发送 `worker_done`。
## 阻塞

无。
