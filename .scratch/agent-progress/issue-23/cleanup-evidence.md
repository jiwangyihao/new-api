# Issue #23 请求记录清理证据

## 2026-08-05 恢复安全点

### 基线核验
命令：
```text
git branch --show-current && git rev-parse HEAD && git status --short
```
关键输出：
```text
jiwangyihao/issue-23-request-settlement
d9e620191f8ca02c237859cc0250f98209749016
M service/billing_session.go
M service/task_billing_test.go
```
结论：分支和 HEAD 与协调器指定值严格一致；工作树并非干净，存在两处继承的 Task 兼容改动，因此在裁决前不进行清理生产代码写入。

### 收敛判断
- `service/task_billing_test.go` 的差异把 legacy 匿名 Credit Task 夹具迁移到持久主键身份，并补充成功、失败与重放断言。
- `service/billing_session.go` 的唯一差异把 Task relay 的 Credit `SettleWithInput` 从 `final=false` 改为 `final=true`。
- 该生产差异只允许由现有新持久 identity 成功终态定向测试证明；没有明确 RED 即撤销，不继续探索 Task 路径。

### 下一条 RED/GREEN
待运行现有 `TestCreditTaskSuccessFinalAndReplayReusePersistedRequestID`（及直接相关持久 identity 用例），分别在当前差异与撤销该一行差异的状态下验证是否只有 `final=true` 能得到 `settled` 终态。
