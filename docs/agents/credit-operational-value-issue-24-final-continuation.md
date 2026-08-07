# Issue #24 API/UI/六语言/浏览器最终续作 Agent 指令

## 目标与冻结现场

你负责完成 GitHub `jiwangyihao/new-api#24` 尚缺的管理员 API、服务端预览、UI、六语言、分析与真实浏览器垂直链路。工作树固定为：

`C:/Users/34404/source/repos/new-api/.workspaces/new-api/issue-24-final`

开始时必须确认：

- 当前 HEAD 为 `c7c983d02f2161f52a9a815a452dc7d950f692fc` 或仅包含本任务后续提交；
- 工作树 clean；
- Orca parent 严格指向 `credit-operational-value-integration`；
- `b8598f4b7add27ba237f30dec6ceae7968cc2aa3` 为 merge-base/祖先；
- H2 领域提交 `49b1ece48`、`79f3f221e` 仍在祖先链；
- 父树的 #26 H1 request→target 锁序及路由夹具校准已吸收，禁止回退。

先读取 `skill://diagnosing-bugs`、`skill://tdd`、`skill://codebase-design`、`skill://shadcn-ui`、`skill://i18n-translate`。完整读取父 PRD #19、Issue #24、`CONTEXT.md`、ADR 0002、2026-08-02 spec/plan、`docs/agents/credit-operational-value-issue-24.md`、`docs/agents/credit-operational-value-issue-24-acceptance.md`、wave-2 contract/acceptance，以及 `.scratch/agent-progress/issue-24/{contract,status,evidence}.md`。

立即创建或更新并提交 `.scratch/agent-progress/issue-24/final-continuation-{contract,status,evidence}.md`。每个可验证阶段小步提交；上下文 75% 先提交 clean 恢复点，85% 前完成或 HANDOFF_READY。

## 所有权与禁止范围

#24 只负责 redemption 与管理员 after-sales increase 正向 ingress、档位资格/预览、低频 ledger、对应 API/UI/i18n、分析可观察性和邀请隔离。只消费 #26 唯一 `CurrentCreditFXRateSnapshot` / `CreditFXRateSnapshot` seam；禁止实现或复制 FX parser/provider/Option 生命周期、conversion、virtual request snapshot。禁止 #25 decrease/refund/chargeback/recovery、#27 migration/ready、#28 release。

## 阶段一：管理员 API 与服务端权威预览

先写 router/controller/service RED，再最小 GREEN并独立提交。

1. 扩展现有 adjustment DTO：increase 必须传正 `amount`、合格 `plan_id`、非空 reason、稳定 `idempotency_key`；decrease 禁止携带 `plan_id`。
2. controller 只转发意图，不计算价格、FX、confidence 或 value；领域层在同事务锁定 Plan，验证 enabled、非 trial/invite-trial、正 `price_amount_micros`、正 plan Credit、可购买、支持币种。
3. 提供服务端权威预览 API/动作，输入 operation/plan_id/amount，返回 gross/net Credit、gross/net `amount_micros` 字符串、source/valuation currency、FX numerator/denominator/captured_at/direction、confidence、rule version、debt offset 和预期 state version；不能写数据库。
4. 正式 increase API 返回与 preview 同结构的 committed 结果；40 CNY/1000 Credit × 800 = 32,000,000 micros CNY。同币种 1/1 与 CNY↔USD 使用冻结有理数 FX和整数 floor。
5. `plan_id`、amount、operation、reason、price/basis、currency、FX、rule/version 等进入完整幂等指纹；同 key 同事实重放原结果，任一事实变化稳定冲突，零写入。
6. 缺 plan、disabled/trial/zero/invalid FX/unsupported currency/overflow/state mismatch 返回稳定 machine code，controller/UI 不解析文本。
7. redemption 与 increase 的 structured ledger/source snapshot/fingerprint/replay 必须保留 H2 已通过合同，Option/Plan 后续变化不动态重估。

## 阶段二：分析可观察性

用真实 redemption API 与 admin adjustment API 纵切证明五个运营分析接口更新 available/debt/exact/value/source；不能直接插状态冒充。覆盖：

- 32 CNY 主例；
- 全额 debt offset，`net_credit=0` 且价值零增长；
- 部分 debt offset，仅净 Credit 与同比例 floor 净成本入池；
- CNY↔USD source/valuation currency 与 frozen FX；
- redemption/increase 不产生 invitation reward、commission 或 paid referral attribution。

保持 #22 current_only/BigInt/micros sorter、#23 request 分支和 #26 conversion analytics 不变。

## 阶段三：管理员 UI 与六语言

1. 在现有 adjustment 面板中，increase 模式只加载/显示合格充值档位，必须选择档位；展示标价、档位 Credit、源币种与服务端权威运营价值预览。
2. 无档位不得提交；原始整数/十进制字符串请求不得被紧凑显示改写。
3. 受控失败重试复用同一 idempotency key；成功或 operation/plan/amount 等事实变化后生成新 key。
4. 切换 decrease 时隐藏并清除 plan/preview，payload 不含 `plan_id`；切回 increase 不泄漏旧档位或预览。
5. UI 明确使用“运营剩余价值”“售后授予”，不得称为实收、退款额、负债或可退款余额。
6. en、zh、fr、ru、ja、vi 全部翻译，`bun run i18n:sync` 无 missing/extras。
7. 使用现有 shadcn/Base UI 组合和项目样式，不新建第二套管理页面。

## 阶段四：真实浏览器与最终门禁

构建当前分支前端并用隔离 SQLite 启动真实应用，使用真实 Chromium 完成：

- increase 选档→服务端 preview→提交；
- 800 Credit / 40 CNY / 1000 Credit 显示 32 CNY且实际请求保留精确字段；
- 一次可控失败后同 key 重试；成功后新事实生成新 key；
- CNY↔USD preview/提交，刷新后冻结 FX/历史不变；
- increase→decrease→increase 切换，验证 plan_id/preview 清理；
- 五接口与低频 ledger 可见结果；
- redemption 与 invitation isolation 代表性路径。

静态资源拦截不能替代 API。完成后关闭 tab、停止服务、删除临时 DB/WAL/SHM与验收构建残留。

最终运行：真实 SQLite model/service/controller/router 定向、count=10/必要窄 race；`go test ./model ./service ./controller ./router -count=1`；受影响前端 tests、typecheck、i18n sync、production build；#20–#23 与 #26 代表性回归；gofmt、`git diff --check`、clean tree。完整三数据库实机归 #27，不能冒充。

最终更新 progress，逐条映射 Issue #24 acceptance，列出提交、API/UI合同、真实请求/响应、未运行范围和风险；确认 staged/unstaged/untracked 全零，再发送一次且仅一次有效 `worker_done --outcome succeeded`。