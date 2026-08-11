# Issue #27 当前状态

- 基线 HEAD：`b45bc8694e7e7a2b15be9e2447b46e140090191f`
- 工作树：`C:/Users/34404/source/repos/new-api/.workspaces/new-api/issue-27-migration-final`
- 初始状态：clean
- 当前阶段：Gate A–H 的 Issue #27 实现、三数据库 Gate F、请求/Task 补充回归、定向 race、本地后端全套与交接材料均已收敛；正在提交并从干净 Worker HEAD 做协调器集成验收，尚未部署。
- 已核对并保留：基线 HEAD/merge-base `b45bc8694e7e7a2b15be9e2447b46e140090191f`，原工作树及全部既有目标改动；未 reset、clean、stash、rebase、重建或触碰 retry/其他工作树。
- Gate B PASS；Gate C/D/E 的 SQLite 与合同窄测 PASS；Gate C–F 的真实三数据库行为由唯一共享矩阵补齐。
- Gate F PASS：同一 `TestCreditValuationExternalMatrix` 顺序通过 SQLite `3.50.4`、MySQL `5.7.44`、PostgreSQL `9.6.24`，三库共 36 个行为子阶段全部 PASS，`SKIP=0`，总退出码 `0`。
- Gate F 覆盖生产 schema/唯一约束、真实锁、原始价格 dry-run/apply/verify、历史迁移重放、生命周期、转换、破坏性恢复、32 CNY 五分析接口，以及 grant+grant、grant+consume、consume+restore、conversion+settlement、refund+admin decrease 五组并发。
- MySQL 首轮在 consume+restore 暴露缓存候选陈旧问题；修复为缓存只提供候选 ID，写事务始终按 ID 重新查询并 `FOR UPDATE`，再校验 status、时间、余额与 requiredTokens。MySQL 定向复跑和完整三库入口均已转绿。
- 已删除不应提交的临时探针源码、两个源码归档和原始运行日志；保留脱敏 Markdown 结论，不保留 DSN、凭据、dump 或完整数据库输出。
- Issue #27 剩余动作：提交全部目标代码、测试与脱敏恢复记录，确认 Worker 工作树 clean，再合入干净集成工作树并复核提交谱系。
- Issue #28 发布前仍须独立闭环：前端 `admin-credit-balance-panel.test.tsx` 的既有 OOM、与干净基线一致的版权头门禁、生产镜像/备份/迁移/浏览器/监控/回滚证据。完成这些前不得部署或关闭 #27/#28/#19。
