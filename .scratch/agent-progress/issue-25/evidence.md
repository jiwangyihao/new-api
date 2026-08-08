# Issue #25 验证证据

## 恢复现场

- `git status --short --branch && git rev-parse HEAD && git merge-base HEAD fe1901aaf7a769fe7057c6483e30b7b1491adcdc && git branch --show-current`
  - 分支：`jiwangyihao/issue-25-destructive-outflow`
  - staged / unstaged / untracked：均为 0
  - HEAD：`fe1901aaf7a769fe7057c6483e30b7b1491adcdc`
  - merge-base：`fe1901aaf7a769fe7057c6483e30b7b1491adcdc`
- `orca status --json && orca worktree current --json && orca orchestration dispatch-show --task task_685c1c42de63 --json`
  - Orca runtime：ready
  - parentWorktreeId：`credit-operational-value-integration`
  - baseRef：`jiwangyihao/credit-operational-value-integration`
  - Run：`run_59804e39b728`
  - Dispatch：`ctx_214c53d3471f`，状态 `dispatched`，failure_count=0

## RED / GREEN

- 尚未开始；每个垂直行为在此记录实际 RED 与 GREEN 命令和关键结果。

## 待收集证据

- 混合池比例 outflow、清空余数、欠额、零余额、溢出与成本非负。
- 事务故障注入和完整回滚。
- 幂等重放、指纹冲突、refund / chargeback 终态竞争。
- outflow 与 request settle / refund 的数据库并发及活动请求快照不变。
- 管理员 decrease API、真实订单 recovery、五个运营分析接口的真实 SQLite tracer。
- 相关 Go 包 `-race`。
- 管理员 increase→decrease 浏览器交互、真实 payload / response 与分析刷新。
- 前端定向测试、typecheck/build、六语言 missing/extras 与 `git diff --check`。

## 数据库实测边界

- 本切片必须真实执行 SQLite。
- MySQL 5.7.44 / PostgreSQL 9.6.24 完整零 SKIP 矩阵归 #27；本切片只做跨库静态审查，不把 DryRun 宣称为实测。
