# Issue #22 窄验收修复证据

## 冻结基线
- `git rev-parse HEAD`：`d5bba460f633ffd2943b1d13bb88b65cea338733`。
- `git status --short`：空。
- 协调器 finding：四个 `recognized_remaining_value` 列表排序仍比较兼容 `float64 amount`；五个 paid-value panel response 均未传播结构化 current-only warning。

## Finding A：权威 micros 排序
- RED 命令：待执行。
- RED 精确信号：待记录。
- GREEN 命令：待执行。
- GREEN 精确信号：待记录。

## Finding B：五面板 current-only warning
- RED 命令：待执行。
- RED 精确信号：待记录。
- GREEN 命令：待执行。
- GREEN 精确信号：待记录。

## 回归与范围
- 32 CNY/paid-value 后端定向回归：待执行。
- 前端 format/panel/page 定向测试与 typecheck：待执行。
- MySQL 5.7/PostgreSQL 9.6：本窄修复不实测，三数据库矩阵仍归 Issue #27。
- Issue #23–#28：不实现。
