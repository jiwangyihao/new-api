# new-api 本地压测报告模板

## 基本信息

- 角色：`baseline` / `candidate`
- Commit：
- 场景：
- 路径：
- Token profile：
- Mock hash：
- Config hash：

## 并发扫描结果

| 并发 | 是否通过 | P95 latency | P95 TTFT | 成功/总数 | 备注 |
|---:|:---:|---:|---:|---:|---|

## 资源与不变量

- `first_failed_concurrency`：
- `highest_passed_concurrency`：
- runtime / PostgreSQL / Redis unavailable 项必须写明 reason，不得用 0 代替。

## 结论

- 是否可作为基线：
- 是否存在回归：
- 必须修复项：
