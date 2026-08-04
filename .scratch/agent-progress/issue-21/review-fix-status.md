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

1. 并发同源重放串行化：`COMPLETE`。真实文件型 SQLite、4 连接、callback/barrier RED 已证明旧锁前 replay read 导致 `SQLITE_BUSY`；计划行现先通过现有 `conversion_guard_version` 写 guard 串行化，再权威查重。定向、`-count=10` 与窄 `-race` 均通过。
2. 权威整数 micros 聚合与排序：`COMPLETE`。#22 的四列表 `amount_micros` sorter 保持不变；两个 non-timed row 累加分支改用 `Value.*Micros`，precision-boundary 与 Credit 32 CNY + timed CNY/USD 组合验证通过。
3. timed micros 加法溢出关闭：`COMPLETE`。`adminCalculateTimedSubscriptionValue` 的 token/current+future、currency time/token、source time/token 累加均改用现有 `checkedAddInt64`；`MaxInt64` 成功，下一 micros 通过五接口稳定返回 `ErrCreditValuationOverflow` 与空响应。
4. 不可变 hook 稳定 sentinel：`COMPLETE`。update/delete 及重复 hook 调用均命中同一包级 `ErrTimedSubscriptionGrantImmutable`；真实 SQLite 证明失败后原 grant 未变化，无普通 mutation API 需要额外 code 映射。

## 恢复信息

- 最近安全提交：`e6ffde16b`（Finding 3 独立测试/证据 follow-up；Finding 4 提交后更新）。
- 未提交文件：Finding 4 sentinel、`errors.Is` 行为测试与本 status/evidence，待独立提交。
- 下一条精确命令：`go test ./model -run '^TestTimedSubscriptionValuationGrant' -count=1 && git diff --check`。
- 阻塞：无。
