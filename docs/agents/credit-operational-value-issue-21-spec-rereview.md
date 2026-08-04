# Issue #21 Spec 最终短复评指令

## 冻结现场与只读边界

你是 GitHub Issue #21「固化计时权益 grant 时间线与多币种分析」的只读 Spec 复评 Agent。只读工作树：

`C:/Users/34404/source/repos/new-api/.workspaces/new-api/issue-21-timed-grants`

开始和结束必须确认：

- `HEAD` 严格等于 `763b0f40bdc8fb7d5c11bc69f46749fd40a8763b`；
- `git status --short` 为空；
- 已含 #22 的集成父基线是 `2260cd2f6369d9cd9e1bea2ac93349b45c7b0ccc`；
- merge-base 必须为该父基线；
- 最终复评范围严格为 `2260cd2f6369d9cd9e1bea2ac93349b45c7b0ccc...763b0f40bdc8fb7d5c11bc69f46749fd40a8763b`。

禁止编辑、格式化、提交、stash、reset、切换分支、启动服务、写数据库、运行项目大套件或派生子 Agent。冻结状态漂移必须 escalation。

## 必读材料与方法

1. 读取 `skill://review`，仅执行 Spec 轴。
2. 阅读父 PRD `issue://jiwangyihao/new-api/19`、子 Issue `issue://jiwangyihao/new-api/21` 及评论；读取已关闭 #22 仅用于组合合同。
3. 阅读集成父树中的：
   - `CONTEXT.md`
   - `docs/adr/0002-credit-operational-remaining-value.md`
   - `docs/superpowers/specs/2026-08-02-credit-operational-remaining-value-spec.md`
   - `docs/superpowers/plans/2026-08-02-credit-operational-remaining-value-plan.md`
   - `docs/agents/credit-operational-value-wave-1-contract.md`
   - `docs/agents/credit-operational-value-wave-1-acceptance.md`
   - `docs/agents/credit-operational-value-issue-21.md`
   - `docs/agents/credit-operational-value-issue-21-acceptance.md`
4. 阅读 `.scratch/agent-progress/issue-21/` 的最终 contract/status/evidence、review-fix 证据和有效 worker_done，但必须回到实现与测试核验。
5. 只做冻结 diff 与既有证据的短评审；不得因好奇扩展到 #23–#28。

## 十一条验收合同映射

对 GitHub #21 每条 acceptance criteria 给出 `PASS`、`FAIL` 或“真实未覆盖”，并至少核验：

### 领域 grant 闭环

- 购买、订单履约、兑换、管理员售后授予与续期都经统一真实领域入口，在同一事务内创建/续期权益并追加不可变 grant；
- grant 冻结真实服务窗口、精确标价 micros、原币种、Credit、期限/重置、规则版本和结构化来源；Plan 后续改价/改币种不回写；
- `(source_type, source_key)` 与幂等 key 确定，同参数重放不续期，参数冲突稳定拒绝；真实并发同源重放属于合法串行化；
- update/delete 普通路径以稳定 sentinel 拒绝；
- trial、邀请试用不估值，disabled 新授予拒绝，既有 disabled entitlement 消费不回归；
- 管理员 API/UI 要求 reason 与 retryable key：失败同事实同 key，成功或事实变化使用新 key。

### timed 算法与五接口

- 估值只读不可变 grant，不按查询时 Plan 补猜价格或币种；
- 服务窗口与 `[snapshot_at, subscription.end_time)` 正确求交，当前周期按剩余 Credit 比例，未来周期完整；失效、边界秒、零额度有行为证据；
- 重叠秒只计稳定最早 grant，并披露 `overlapping_grants`；缺失/歧义稳定 warning/unknown；
- CNY/USD 独立整数 micros 聚合，recognized 是同币种两口径较小值；跨币种 singular 为 null，`*_by_currency` 精确；
- summary/users/subscriptions/plans/sources 从同一 paid row 对账，source attribution 与 `mixed_grants` 正确；
- 所有权威 micros 累加 fail-closed，不得 float64 或静默 int64 溢出；
- 强 tracer 真实驱动领域入口与五个 HTTP API，不用 mock/静态拦截替代主证明。

### UI、六语言和浏览器

- 管理员只展示 enabled、非试用/邀请试用、timed 且有正精确 micros 的计划；payload 冻结价格/币种/reason/key；
- 真实浏览器证据证明断服失败捕获 payload/key、同事实同 key 重试 HTTP 200，业务事实变化产生新 key；
- CNY/USD 同权益在五接口和页面分币种显示，当前 Plan USD 不改写历史 CNY；跨币种 singular 仍为 null；
- unknown/overlap/mixed/失效裁剪/confidence/warning 可见；术语为运营剩余价值；
- en、zh、fr、ru、ja、vi 完整，组件测试/typecheck/build/browser/资源清理证据可追溯。

### 与已集成 #22 的组合合同

必须明确核验：

- #22 通用 micros DTO、CreditValuation、Credit paid row、权威 `amount_micros` 排序、current_only warning 和 BigInt UI 均保留；
- #21 只增加 timed calculator、timed `*_by_currency`、timed warning/unknown、grant source 与管理员 UI；
- 组合后 #22 冻结信号仍是 recognized/exact=`32,000,000` micros CNY、available=800、active count=1、Credit `time_based_value=null`；
- #21 timed CNY/USD、跨币种 singular null、`timed_grant_timeline` 同时成立；
- 父基线 merge 冲突的最终语义确实按上述所有权解决，而不是仅编译通过。

### 范围边界

不得实现 Credit request settlement、其他 ingress/outflow、conversion FX/在途请求、历史回填、marker/ready、三数据库发布门禁或生产部署。MySQL/PostgreSQL 无 DSN 不是 #21 finding；真实三数据库零 SKIP 属于 #27，但明显方言错误仍应报告。

## 输出与完成

把不超过 500 字的最终报告写入：

`C:/Users/34404/AppData/Local/Temp/new-api-issue21-spec-rereview-final.md`

报告必须包含冻结范围、总评 `PASS`/`FAIL`、十一条 acceptance/Gate B–E 映射摘要、findings（引用具体条款与文件/符号）、#22 组合结论、范围外/未实测说明。推断标 `[INFERENCE]`；无 finding 写“0 findings”。

结束前再次确认冻结 HEAD 和 clean tree。随后使用当前 Dispatch 注入 capability 发送恰好一次有效 `worker_done`，body 包含 PASS/FAIL、finding 数、最严重项、组合结论与报告绝对路径。未完成、读取失败或冻结状态漂移不得报告成功。