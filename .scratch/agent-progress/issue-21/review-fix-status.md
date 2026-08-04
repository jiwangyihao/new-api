# Issue #21 Standards 修复状态

## 基线

- 冻结实现 HEAD：`547512242578ec198034d322875c5485735b247a`。
- 父集成分支：`jiwangyihao/credit-operational-value-integration`。
- 父集成 HEAD：`2260cd2f6369d9cd9e1bea2ac93349b45c7b0ccc`。
- 当前修复分支：`jiwangyihao/issue-21-timed-grants`。
- 合并状态：父集成 `2260cd2f6369d9cd9e1bea2ac93349b45c7b0ccc` 已合并，merge commit 为 `9cee335ddb0638af7b5bb9229d5d2a03db5a0712`；冲突已全部解决，工作树 clean。

## 冲突决策

合并固定保留 #22 的通用 Credit DTO、Credit/current_only 分支、整数 micros accumulator/sorter 与前端 BigInt 骨架；只叠加 #21 的 timed grant、`*_by_currency`、timed warning/source 增量。禁止恢复 #21 旧 float accumulator、旧 DTO 或按 Plan 币种补猜。

## Findings

1. 并发同源重放串行化：`IN_PROGRESS`，待先写真实多连接 SQLite RED。
2. 权威整数 micros 聚合与排序：`PENDING`，待合并 #22 后补 precision-boundary 与 Credit+timed 组合 RED。
3. timed micros 加法溢出关闭：`PENDING`，待补 MaxInt64 与多 segment/source/五接口 RED。
4. 不可变 hook 稳定 sentinel：`PENDING`，待补 update/delete `errors.Is` RED。

## 恢复信息

- 最近安全提交：`9cee335ddb0638af7b5bb9229d5d2a03db5a0712`（父集成基线合并与 23 个冲突块收敛）。
- 未提交文件：仅本次 `review-fix-status.md` 与 `review-fix-evidence.md` 合并恢复记录，待立即提交。
- 下一条精确命令：`git add .scratch/agent-progress/issue-21/review-fix-status.md .scratch/agent-progress/issue-21/review-fix-evidence.md && git commit -m "docs(issue-21): 记录父集成合并恢复状态"`。
- 阻塞：无。
