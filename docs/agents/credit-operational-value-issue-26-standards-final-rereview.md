# Issue #26 Standards 最终冻结复评指令

## 目标与冻结现场

你是父 PRD GitHub #19、子 Issue #26「固化转换估值、FX 与在途请求结算」的最终 Standards 只读复评 Agent。只读审查以下冻结工作树：

`C:/Users/34404/source/repos/new-api/.workspaces/new-api/issue-26-conversion-fx`

开始与结束时必须核对：

- `HEAD` 严格等于 `0df808b2356c9a9cdffa07b1b9dae19f4b912e61`；
- `git status --short` 无输出；
- 分支点/merge-base 为 `60e71da8d5be73816dd7c892b0d4f96768db98b3`；
- 固定审查命令为 `git diff 60e71da8d5be73816dd7c892b0d4f96768db98b3...0df808b2356c9a9cdffa07b1b9dae19f4b912e61`；
- 提交列表只来自 `git log 60e71da8d5be73816dd7c892b0d4f96768db98b3..0df808b2356c9a9cdffa07b1b9dae19f4b912e61 --oneline`。

禁止编辑、格式化、提交、stash、reset、切换分支、启动服务、写数据库、运行项目级大套件或派生 Agent。冻结状态漂移必须 escalation。现有 worker/协调器 evidence 只是索引，结论必须回到最终代码、测试与仓库标准。

## 必读材料

1. `skill://review`，只执行 Standards 轴。
2. 自动注入的项目/全局 `AGENTS.md`。
3. `issue://jiwangyihao/new-api/19`、`issue://jiwangyihao/new-api/26`。
4. 集成父树 `C:/Users/34404/source/repos/new-api/.workspaces/new-api/credit-operational-value-integration` 下：
   - `docs/agents/credit-operational-value-execution.md`
   - `docs/agents/credit-operational-value-wave-3-contract.md`
   - `docs/agents/credit-operational-value-issue-26.md`
   - `docs/agents/credit-operational-value-issue-26-acceptance.md`
   - `CONTEXT.md`
   - `docs/adr/0002-credit-operational-remaining-value.md`
   - `docs/superpowers/specs/2026-08-02-credit-operational-remaining-value-spec.md`
5. 冻结树 `.scratch/agent-progress/issue-26/{contract,status,evidence}.md` 与 `coordinator-browser-evidence.md`。

## Standards 审查主题

逐项给出直接证据：

1. **整数金额与唯一 FX seam**：Option 原始十进制是否完全避开 `float64`；CNY/USD 有理数解析、约分、方向、floor、overflow 是否采用有界整数；热路径是否无 `big.Int` 分配；是否存在第二套 parser/provider、动态重估或兼容价格反推。
2. **深模块与复用边界**：conversion 是否只消费 #21 timed grant、#22 Credit ingress/移动平均、#23 request-aware 入口；是否复制状态机、匿名 Credit delta、无界扫描、N+1、无必要副本或旁路 DB 写；Service/Controller 是否越过 Model 深模块直接操作 GORM。
3. **事务、锁序、幂等和并发**：conversion、source、target、ledger、valuation state、活动接替、在途 request snapshot 是否同事务；锁序是否符合来源→request→目标；同事实重放、事实冲突、双连接同时确认、conversion 与 final/refund 是否返回稳定领域结果而非泄漏 SQLite/GORM 错误。
4. **在途请求正确性**：原 `subscription_id`、窗口、冻结成本/FX/rounding 是否保留；首次 settle/refund 不二次扣目标池；转换后新请求才进入 Credit；重放不增长实体或版本；没有进程内临时状态代替持久事实。
5. **API/UI 与精度**：所有 micros、Credit、numerator/denominator/captured_at/version 是否以字符串穿过 API/TypeScript；前端是否使用 string/BigInt 而非 `Number`；稳定 sentinel/code 是否在 adapter 映射；六语言不是复制英语；未新增第二套 UI。
6. **仓库规则与范围**：JSON 使用项目 wrapper；SQLite/MySQL 5.7/PostgreSQL 9.6 静态语义合理；无受保护标识修改；没有越界实现 #24 ingress、#25 recovery、#27 migration/marker、#28 release；未把未实测三数据库冒充通过。
7. **已知证据复核**：浏览器证据必须来自真实 30261 隔离应用而非静态拦截；冻结历史在当前 FX/Plan 更新后仍保持 40,000,000、10/73、5,479,452；coalescer 全包阈值波动若为既有基线，只能作为诚实风险，不得吞掉真实回归。

## 输出与完成

将不超过 800 字的报告写到：

`C:/Users/34404/AppData/Local/Temp/new-api-issue26-standards-final-rereview.md`

必须包含：冻结范围、总评 `PASS`/`FAIL`、上述 1–7 逐项结论、findings（严重度、文件/符号、直接证据、影响、最小修复）、未实测范围及边界。推断标 `[INFERENCE]`；无 finding 明写 `0 findings`。工具强制执行的格式问题可跳过，但不得跳过架构、精度、事务、性能或数据库兼容问题。

结束前再次确认 frozen HEAD 与 clean tree；随后用当前 Dispatch 注入 capability 发送恰好一次有效 `worker_done`，正文包含 PASS/FAIL、finding 数、最严重项、边界结论和报告绝对路径。进程完成不等于复评 PASS。
