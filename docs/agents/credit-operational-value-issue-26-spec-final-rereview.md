# Issue #26 Spec 最终冻结复评指令

## 目标与冻结现场

你是父 PRD GitHub #19、子 Issue #26「固化转换估值、FX 与在途请求结算」的最终 Spec 只读复评 Agent。只读审查：

`C:/Users/34404/source/repos/new-api/.workspaces/new-api/issue-26-conversion-fx`

开始与结束时必须核对：

- `HEAD` 严格等于 `0df808b2356c9a9cdffa07b1b9dae19f4b912e61`；
- `git status --short` 无输出；
- merge-base 为 `60e71da8d5be73816dd7c892b0d4f96768db98b3`；
- 固定 diff：`git diff 60e71da8d5be73816dd7c892b0d4f96768db98b3...0df808b2356c9a9cdffa07b1b9dae19f4b912e61`；
- 固定提交列表：`git log 60e71da8d5be73816dd7c892b0d4f96768db98b3..0df808b2356c9a9cdffa07b1b9dae19f4b912e61 --oneline`。

禁止编辑、格式化、提交、stash、reset、切换分支、启动服务、写数据库、运行大套件或派生 Agent。冻结状态漂移必须 escalation。不要因已有测试/evidence 就假定行为正确；逐项回到最终代码、公开入口与持久字段。

## 必读材料

1. `skill://review`，只执行 Spec 轴。
2. `issue://jiwangyihao/new-api/19` 与 `issue://jiwangyihao/new-api/26`。
3. 集成父树下：
   - `docs/agents/credit-operational-value-wave-3-contract.md`
   - `docs/agents/credit-operational-value-issue-26.md`
   - `docs/agents/credit-operational-value-issue-26-acceptance.md`
   - `docs/superpowers/specs/2026-08-02-credit-operational-remaining-value-spec.md`
   - `docs/adr/0002-credit-operational-remaining-value.md`
   - `CONTEXT.md`
4. 冻结树 `.scratch/agent-progress/issue-26/{contract,status,evidence}.md` 与 `coordinator-browser-evidence.md`。

## Spec 逐项裁决

对以下每项明确 `PASS`/`FAIL` 并引用 Issue/acceptance 条款及代码/测试位置：

1. **数量公式**：`full_31_day_blocks × credit_basis + current_remaining_credit`；31 天业务月；不足 31 天不按秒折算；已预扣量不重复计入；数量与价值使用同一 basis。
2. **冻结 source 事实**：Confirm 锁后重读资格，并冻结 `price_amount_micros`、source/target currency、credit basis、gross/net Credit、gross/net cost、未舍入单位价值有理数、duration/reset/rule、rule/state version；后续改价不回写。
3. **FX**：同币种 1/1；CNY↔USD 从持久 Option 原始十进制解析为约分正有理数；方向 `1 USD = numerator/denominator CNY`；两向整数 floor；invalid/unsupported/overflow 稳定拒绝；配置变化不重估历史；无 float 反推。
4. **原子性与幂等**：conversion、Credit ingress、ledger、source converted、活动接替、valuation state 同事务；同事实重放返回原结果；source/price/basis/FX/rule/target 变化稳定冲突且零写入；同 source 双连接只写一次。
5. **运营语义**：conversion exact 是规则确值而非新增收款、邀请收入或可退款现金；历史保留原权益/来源；当前价值只进入目标 Credit 混合池；debt offset 不产生剩余价值。
6. **在途请求**：原 request 保留 source `subscription_id`/window/attribution；虚拟 exact snapshot 不二次扣目标池；少结算/全退款按冻结 snapshot 恢复，追加仅按目标池当时状态；转换后新请求才进 Credit；settle/refund 重放幂等；conversion 与 final/refund 并发合法串行化。
7. **资格边界**：disabled、trial、invitation、缺少资格拒绝新 conversion；已有 disabled-plan 权益消费、模型范围忽略、邀请隔离不回归；legacy valuation-not-ready 路径保持兼容且不伪造估值。
8. **API**：quote/confirm/history/analytics 返回精确字符串 micros、source/target currency、FX numerator/denominator/direction/captured_at、未舍入单位价值、rule/state version和稳定错误 code；改 Option/Plan 后 history 仍返回冻结事实；不新增 #24 生命周期。
9. **UI/i18n**：现有 wallet conversion 卡片显示 31 天公式、规则确值不是新增收款、冻结 price/currency/FX/micros/rule-state；所有大整数用 string/BigInt；六语言 en/zh/fr/ru/ja/vi 完整；布局/Credit 激活/充值隐藏/主要 blocker 原合同不变。
10. **真实验收**：真实 SQLite quote→confirm→history/analytics 主路径，而非直接插表冒充；真实 30261 应用和 Chromium 显示 CNY→USD 10/73、source 40,000,000、gross/net 5,479,452，后将当前 FX 7.3→8.5、Plan 40→55 后历史仍冻结；转换期间 request settle/refund 与双连接并发有行为证据。
11. **回归和范围**：#20 精确价、#21 timed grant、多币种分析、#22 32 CNY、#23 request restore 保持；没有实现 #24 admin increase/redemption、#25 recovery、#27 migration/ready、#28 release；MySQL/PostgreSQL 零 SKIP、全项目与部署未运行须诚实保留。
12. **Issue #26 十二条 acceptance**：逐条映射 GitHub #26 行 26–37，任何部分实现必须判 FAIL，不得用“后续 #24/#27”覆盖本 Issue 自有合同。

特别审查潜在缺口：冻结 conversion 的结构化持久字段是否足够支撑 API/history，不得只存在 JSON；`captured_at`/version/单位价值是否真实来自 committed ledger/state；在途追加目标大于 reserve 是否确实按目标池当前平均值而非旧 source 值；浏览器证据是否与最终 HEAD 一致。

## 输出与完成

将不超过 900 字的报告写到：

`C:/Users/34404/AppData/Local/Temp/new-api-issue26-spec-final-rereview.md`

必须包含冻结范围、总评 `PASS`/`FAIL`、上述 1–12 逐项结论、findings（严重度、对应条款、文件/符号、直接证据、影响、最小修复）、未实测范围与范围边界。推断标 `[INFERENCE]`；无 finding 明写 `0 findings`。不得把协调器浏览器证据、Worker 声明或测试名称本身当作实现证明。

结束前再次确认 frozen HEAD 与 clean tree；随后用当前 Dispatch 注入 capability 发送恰好一次有效 `worker_done`，正文包含 PASS/FAIL、finding 数、最严重项、边界结论和报告绝对路径。进程完成不等于复评 PASS。
