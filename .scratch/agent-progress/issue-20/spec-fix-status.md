# Issue #20 Spec H1/M1 收敛修复状态

## 基线

- 冻结候选 HEAD：`79982d773d127779c9c3835c2e1c771b7a829268`。
- 分支：`issue-20-valuation-foundation`。
- 开始状态：工作树 clean。
- 最近安全 HEAD：`79982d773d127779c9c3835c2e1c771b7a829268`。

## 当前阶段

- H1：model、controller/API 创建与更新显式零值均已 GREEN；历史 NULL、非零与拒绝路径回归通过，准备小步提交。
- M1：冻结等待 H1 提交；之后仅做真实 SQLite `roundtrip_mismatch` 与诊断前后数据库快照相同，不再扩展跨方言探索。

## RED / GREEN

| 项目 | RED | GREEN | 状态 |
| --- | --- | --- | --- |
| H1 显式精确零值保真 | model 指针为 nil；API 数据库 `sql.NullInt64.Valid=false` | model/create/update/GET 与历史 NULL 回归通过 | GREEN，待提交 |
| M1 SQLite 数值往返诊断 | H1 提交后执行 | 待实现 | 暂停 |

## 已确认根因

- H1：`NormalizeSubscriptionPlanPrice` 以 `exactMicros == 0` 判断“未提供”，导致显式字符串 `"0"` 返回 `AmountMicros=nil`，controller 随后持久化 `NULL`。
- M1：诊断只解析数据库 `CAST` 后的表面文本，未识别数据库原始数值与规范 micros 重建值不相等的情况。

## 当前未提交文件

- `.scratch/agent-progress/issue-20/spec-fix-status.md`
- `.scratch/agent-progress/issue-20/spec-fix-evidence.md`
- `.scratch/agent-progress/issue-20/spec-fix-contract.md`

- `model/credit_valuation_money_test.go`
- `controller/subscription_exact_price_test.go`
- `model/credit_valuation_money.go`

H1 实现落盘后继续在此列出。

## 下一条命令

格式化 H1 Go 文件，重跑窄范围测试与 `git diff --check`，提交 H1 可恢复小提交。

## 阻塞

无。

## 非所有权

不实施 #27 的历史回填、migration marker 写入或 `ready` 裁决；不启用 Credit 数量/估值强制双写；不实现 #21–#26 业务路径。
