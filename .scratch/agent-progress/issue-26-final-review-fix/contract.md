# #26 Final Review Fix 合同

- 冻结实现祖先：`6f865feca3cd517a3dd744e67ea1240d5001d2ed`
- 当前子树 HEAD：`91fad13207b7b7ee916705fe1b8c8ccfd829aaa1`；该 HEAD 上的协调文档提交必须保留。
- 当前 phase：H1 request → target 固定锁序。
- 当前范围：仅验证并修复 Confirm 在任何目标 Credit entitlement、valuation state、ledger 或 conversion 写入前，按稳定 request identity 顺序锁定并验证 source 的在途 request。
- 固定顺序合同：conversion/idempotency → quote → source subscription/plan/grants → in-flight requests（request ID 升序）→ target Credit/valuation/ledger → conversion/source/history/activity writes。
- 必须先用真实 SQLite 双连接及确定性交错行为测试建立 RED；不得使用源码文本断言替代行为证据。
- 最小 GREEN 只处理 H1 锁序，不修改错误映射、quote identity、committed unit value、前端或其他 finding。
- 禁止触碰 #24 adjustment/redemption、#25、#27、#28。
- 未提交文件：安全提交前为本目录下的 `contract.md`、`status.md`、`evidence.md` 三份 progress 文件；无代码改动。
- provider 阻塞解除前不得开始 RED/GREEN；SQLite 结果也不冒充 MySQL/PostgreSQL 行锁验证。
