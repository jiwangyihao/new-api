# Issue #20 Spec H1/M1 收敛修复状态

## 基线

- 冻结候选 HEAD：`79982d773d127779c9c3835c2e1c771b7a829268`。
- 分支：`issue-20-valuation-foundation`。
- 开始状态：工作树 clean。
- 最近安全 HEAD：`cf2b743b84ac74977d654d63dab52ecd8bb0d9fb`（H1 显式零值修复）。

## 当前阶段

- H1：已完成 RED→GREEN 并提交 `cf2b743b8`；显式零值创建、更新、数据库非 NULL 0 与 GET `"0"` 均受保护。
- M1：真实 SQLite NUMERIC/REAL `roundtrip_mismatch` 已 GREEN；稳定排序、重复调用与诊断前后完整夹具快照相同均通过，进入既定窄回归与提交。

## RED / GREEN

| 项目 | RED | GREEN | 状态 |
| --- | --- | --- | --- |
| H1 显式精确零值保真 | model 指针为 nil；API 数据库 `sql.NullInt64.Valid=false` | model/create/update/GET 与历史 NULL 回归通过 | GREEN，`cf2b743b8` |
| M1 SQLite 数值往返诊断 | 表面 `40.123456` 可解析但原始 REAL 严格不等，plan 6 被漏报 | plan 6 稳定返回 `roundtrip_mismatch`；排序、重复调用、零写入通过 | GREEN，待提交 |

## 已确认根因

- H1：`NormalizeSubscriptionPlanPrice` 以 `exactMicros == 0` 判断“未提供”，导致显式字符串 `"0"` 返回 `AmountMicros=nil`，controller 随后持久化 `NULL`。
- M1：诊断只解析数据库 `CAST` 后的表面文本，未识别数据库原始数值与规范 micros 重建值不相等的情况。

## 当前未提交文件

- `.scratch/agent-progress/issue-20/spec-fix-status.md`
- `.scratch/agent-progress/issue-20/spec-fix-evidence.md`
- `.scratch/agent-progress/issue-20/spec-fix-contract.md`
- `model/subscription_price_diagnostic.go`
- `model/subscription_price_diagnostic_test.go`

## 下一条命令

运行既定 H1/M1 窄 Go 回归、plan-form 13 tests、tsc 与 `git diff --check`，然后提交 M1。

## 阻塞

无。

## 非所有权

不实施 #27 的历史回填、migration marker 写入或 `ready` 裁决；不启用 Credit 数量/估值强制双写；不实现 #21–#26 业务路径。
