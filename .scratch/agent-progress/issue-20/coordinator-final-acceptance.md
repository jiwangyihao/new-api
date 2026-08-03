# Issue #20 合并 HEAD 最终验收证据

- 集成实现提交：`a8c779fa75cf66af2db45d0618cade49d3b18105`（non-ff）。
- 当前步骤完整审计报告：`C:/Users/34404/AppData/Local/Temp/new-api-issue20-final-acceptance.md`。
- 后端：`go test ./model ./controller ./router -count=1`，3 packages PASS。
- 前端：相关两文件 30/30 PASS；`bun run typecheck` PASS；`bun run build` PASS；六语言 i18n missing/extras 均为 0 且同步后 git diff 为空。
- SQLite：重复附加迁移、历史 NULL 保持、唯一约束真实拒绝、只读诊断、roundtrip_mismatch、零写入均 PASS。
- 当前集成 HEAD 真实 Chromium：精确价格 create→edit→full reload；`/wallet` 购买入口和 `Timed subscription` 对话框；账户余额购买成功；disabled plan 不出现在公开列表且新购买拒绝；含遗留 `model_limits` 的 Credit 配置请求在响应和 SQLite 中被清空/忽略。
- 浏览器服务从集成工作树启动，端口 3101，临时 SQLite `new-api-issue20-integration-browser-smoke-2.db`；结束后浏览器、服务与临时数据库均已清理。
- classic 锁文件存在既有 `axios` 声明漂移，`--frozen-lockfile` 如实失败；未改锁文件，使用同一集成树 `bun install --no-save && bun run build` 生成真实 classic 产物，工作树保持 clean。
- Standards 最终 PASS；H1/M1 由提交 `cf2b743b8`、`c3b3f6848` 修复并经协调器 `-count=10` 复验。失败/停止复评 `ctx_10127da09c7e` 未作为通过证据。
- MySQL 5.7.44 / PostgreSQL 9.6.24 未配置 DSN，明确为 2 SKIP；真实三库零 SKIP 门禁仍归 #27，未冒充 #20 PASS。
