# Issue #22 协调器验收清单

## 用途与基线

本清单供协调器在父 PRD #19 的 Issue #22 Worker 发出 `worker_done` 后使用。只有 Issue #20 已独立验收并集成到 `jiwangyihao/credit-operational-value-integration`，且 #22 子工作树从该提交派生，才可开始验收。

验收必须同时满足 Issue #22、ADR 0002、2026-08-02 规格/计划、`credit-operational-value-execution.md` 和 `credit-operational-value-wave-1-contract.md`。#22 是通用 analytics DTO、Credit 分流及 CreditValuation 深模块的主改者；不得用直接插表、mock API 或静态资源拦截替代 32 CNY 真实 tracer。

## Gate A：Worker 与提交完整性

- [ ] 当前 Dispatch 恰好收到一次 `worker_done`；记录 Run、Task、Dispatch、终端、Worker 分支、父工作树和最终 HEAD。
- [ ] `worker_done` 列出提交 SHA、深模块合同、入账/消费入口、五接口/UI、测试、SQLite tracer、浏览器证据、共享文件和风险。
- [ ] `.scratch/agent-progress/issue-22/{status,evidence,contract}.md` 已提交且与 HEAD 一致；`contract.md` 明确锁序、不变量、来源快照、DTO 和非所有权。
- [ ] Worker 工作树完全干净；关键成果不存在于 stash、临时副本、一次性脚本或未提交测试中。
- [ ] `git merge-base` 证明分支从已验收 #20 基线派生；提交链仅包含 #22。
- [ ] 完整差异通过 `git diff --check`；提交符合 Conventional Commits 与中文 subject 规则。
- [ ] 未修改受保护项目身份、凭据、生产配置或无关用户文件；未越界实现 #23–#28。

## Gate B：CreditValuation 深模块与不变量

- [ ] 每份 Credit 权益由数据库唯一约束保证恰有一行 `CreditValuationState`；重复状态在真实 SQLite 被拒绝。
- [ ] 唯一深模块同时写 `token_limit/token_used` 与估值状态；锁序固定为已锁权益→估值状态→本 tracer 请求记录/ledger，事务由调用者管理。
- [ ] 调用者只能提交结构化来源事实，不能直接声明 confidence、成本或当前 plan 推断值。
- [ ] 状态保存前验证可用量一致、成本非负、unknown 不超过可用 Credit、币种一致、版本单调；有效变更才增加 `state_version`。
- [ ] 入账使用 #20 固定宽度 floor helper；先抵扣 settlement debt，仅净新增可用 Credit 及同比例净成本进入池，`net_credit=0` 不增值。
- [ ] 出账使用操作前移动加权比例移除 exact/estimated/unknown；完全清空吸收全部 micros 余数，超量仅形成 debt。
- [ ] 热路径不使用 float 或按请求分配 `big.Int`；所有失败返回稳定 sentinel/code 并回滚完整事务。
- [ ] marker 未 ready/历史缺状态遵循 #20/#27 冻结合同；不按当前 plan 价格热修、不回填历史、不切换 marker。
- [ ] 既有 disabled-plan Credit 可消费；新购买仍检查 enabled/eligible；`model_limits` 保持忽略，邀请逻辑仍排除 Credit。

## Gate C：真实入账与最小同步 request tracer

- [ ] 订单创建冻结充值档位 `price_amount_micros`、币种、档位 Credit、规则版本和稳定来源；完成回调只消费该快照。
- [ ] 人民币余额购买与一个仓库现有、测试可控的外部支付完成入口均调用同一 ingress；不得使用渠道实收金额或回调时当前 plan 价格。
- [ ] 套餐在订单创建后改价，回调仍按原快照入账；快照缺失、币种不支持、档位不合格、溢出或状态失败使订单事务整体回滚。
- [ ] 相同来源/完成回调重放返回同一结果，不重复增加 Credit 或价值；冲突参数返回稳定错误。
- [ ] 最小同步链路使用真实 `request_id` 预扣 200，记录必要身份/扣除事实，并最终结算到累计目标 200。
- [ ] 该预扣要求足额且不形成 debt；重复同一同步目标幂等。
- [ ] #22 未实现 target 减少、通用追加、退款、异步 task identity 或 coalescer；这些仍由 #23 承担。
- [ ] 真实 tracer 不直接创建 `CreditValuationState`，必须从购买/支付领域入口开始。

## Gate D：冻结 32 CNY 五接口验收

- [ ] 真实数据库夹具包含零价全局 Credit 容器和 `40 CNY / 1,000 Credit` 有价充值档位，`end_time=0` 不影响识别。
- [ ] 完成购买并消费 200 后，权益可用量严格为 800，exact 为 `32,000,000` micros CNY，estimated=0、unknown=0。
- [ ] summary、users、subscriptions、plans、sources 五接口由同一状态事实返回一致金额和计数，`active_paid_subscription_count=1`。
- [ ] Credit paid-row 不检查全局容器正价格、不进入 timed 公式；`time_based_value=null`，token=recognized=exact+estimated。
- [ ] source 固定为 `credit_balance_pool / moving_weighted_pool`；不得按 `(user_id, plan_id)` 猜订单，plan filter 只匹配全局 Credit plan。
- [ ] 正可用且成本全 unknown 的 Credit 仍计 active；exhausted/debt 保留明细、金额零且不计 active。
- [ ] `snapshot_at` 早于状态更新时间时返回最新版本及 `current_only` warning，不伪造历史。
- [ ] `amount_micros` 为十进制字符串并驱动后端汇总/排序；兼容 float 只在最后派生，不参与权威计算。
- [ ] 不同价格两次入账、先消费后入账、debt 抵扣、清空余数、改价后回调、重放、筛选/排序均有行为测试。

## Gate E：API、UI、i18n 与浏览器

- [ ] 通用 DTO 暴露 exact/estimated/unknown、state version、更新时间、nullable time、snapshot semantics 和精确 micros 字符串。
- [ ] 前端优先用 BigInt/字符串解析 micros；仅旧响应缺精确字段时回退兼容 float。
- [ ] 页面显示 32 CNY、exact/estimated/unknown、Credit 时间值“不适用”、moving-weighted 归属与 current-only 非阻断刷新提示。
- [ ] 术语明确为“运营剩余价值”，不称为实收、退款、负债、可退金额或递延收入。
- [ ] 新增文案全部使用 `t(...)`，en、zh、fr、ru、ja、vi 无 missing/extras。
- [ ] 相关组件测试、`bun run typecheck` 与 `bun run build` 通过；只格式化实际修改文件。
- [ ] 使用受监督服务和真实浏览器观察五接口页面、32 CNY、null time、warning 与刷新；静态拦截只可辅助渲染，不能代替真实 API/领域 tracer。结束后关闭 tab/服务。

## Gate F：回归、并发与证据

- [ ] 主 seam 使用真实 SQLite 和现有购买/支付/request/API 入口；不直接插估值状态、不以 calculator 单测代替。
- [ ] 必要低层测试覆盖算术不变量、事务回滚、幂等、固定锁序和合法并发串行化集合。
- [ ] 受影响 Go 包定向测试通过；现有购买、disabled entitlement 消费、邀请隔离与 `model_limits` 忽略无回归。
- [ ] schema/SQL 兼容 SQLite、MySQL 5.7.8+、PostgreSQL 9.6；DryRun 不是真实数据库 PASS，三库零 SKIP 由 #27 总验收。
- [ ] 五接口响应关键字段、请求 payload、数据库状态与浏览器观察均记录在 evidence；命令可由协调器在同 HEAD 重现。
- [ ] Issue #22 十二条 acceptance criteria 逐条映射 PASS/FAIL/真实未覆盖；不得把推断标为 PASS。
- [ ] 工作树干净、`git diff --check` 通过、无失败测试、无未解释警告或会破坏 #21 timed 扩展 seam 的模糊点。

## 不放行条件

出现任一项则保持 #22 OPEN 并让原 Worker修复：

- 直接插入估值状态伪造 tracer，或只测 calculator/mock API；
- 从当前 plan、支付实收或 float 反推 Credit 成本；
- Credit 数量与价值不在同一锁序/事务，或失败后部分写入；
- 五接口金额、活动计数、source/filter 不一致，32 CNY 信号不成立；
- 提前实现 #23 的通用结算、#24–#26 的其他来源/恢复/FX、#27 marker 或历史迁移；
- 缺少真实数据库/浏览器证据、六语言、构建或清洁工作树。
