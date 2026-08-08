# Issue #24 最终续作合同

## 冻结现场

- 工作树：`C:/Users/34404/source/repos/new-api/.workspaces/new-api/issue-24-final`。
- 开工 HEAD：`c7c983d02f2161f52a9a815a452dc7d950f692fc`，工作树 clean。
- Orca parent：`credit-operational-value-integration`；`b8598f4b7add27ba237f30dec6ceae7968cc2aa3`、`49b1ece48`、`79f3f221e` 均在祖先链。
- 保留已吸收的 #26 H1 request→target 锁序与路由夹具校准；不得回退。
- 旧 `issue-24-positive-ingress` 仅可只读参考既有 progress 与 `e4ad9c32b`，不得在其上继续开发或覆盖任何 Agent 成果。

## 本次所有权

本续作只完成 Issue #24 仍缺的公开合同：

1. 现有管理员 Credit adjustment 的 `increase` HTTP API 与服务端权威预览；
2. 真实 redemption/admin adjustment 入口后的五接口分析可观察性与邀请隔离；
3. `AdminCreditBalancePanel` 的合格充值档位选择、服务端预览、幂等键生命周期和 operation 状态清理；
4. en、zh、fr、ru、ja、vi 六语言；
5. 真实 SQLite 应用、真实 Chromium 垂直链路及最终定向门禁。

## API 与领域接口

- 继续扩展既有 adjustment 路径，不创建平行售后入口。
- `increase` 请求必须包含：`operation=increase`、正整数 `amount`、正 `plan_id`、非空 `reason`、稳定 `idempotency_key`。
- `decrease` 不得携带 `plan_id`；本 Issue 只守住请求合同，不实现 #25 的 outflow/recovery 行为。
- controller 只解析/校验传输合同并转发意图；价格、FX、confidence、debt offset、净值与版本全部由 model 领域模块决定。
- 服务端 preview 输入 `operation/plan_id/amount`，与正式提交共用档位资格、冻结事实和整数估值接缝，但不得写 adjustment、ledger、subscription、state 或来源终态。
- preview 与 committed result 统一返回：gross/net Credit、gross/net `amount_micros` 十进制字符串、source/valuation currency、FX numerator/denominator/captured_at/direction、confidence、rule version、debt offset、预期/提交后 state version；正式提交另返回 `replayed`。
- 40 CNY / 1,000 Credit × 800 Credit 必须得到 `32,000,000` micros CNY；同币种为 1/1，CNY↔USD 只消费唯一 `CurrentCreditFXRateSnapshot` / `CreditFXRateSnapshot`，采用整数 floor。
- 缺 plan、plan 不合格、unsupported currency、invalid FX、overflow、state missing/mismatch、idempotency mismatch 均以稳定 machine code 暴露；controller/UI 不解析文本。

## 幂等与冻结事实

- 指纹包含 user、operation、amount、plan、标准化 reason、operator、权威 `source_price_micros`、plan Credit、source/valuation currency、FX numerator/denominator/captured_at/direction、rule version 及既有领域合同要求的完整事实。
- 相同 key 与完全相同事实重放首次 committed result，不增加 Credit、价值、ledger 或 state version。
- 同 key 的任一事实变化稳定冲突，零写入。
- redemption 与 increase 的 ledger、source snapshot、fingerprint/replay 继续保持既有 H2 合同；Plan/Option 后续变化不得动态重估历史。

## 分析可观察性

使用真实 redemption API 与 admin adjustment API 建立状态，再读取 summary/users/subscriptions/plans/sources 五个运营分析接口；不得直接插入估值状态冒充入口。

必须覆盖：

- 32 CNY 主例；
- 全额 debt offset：`net_credit=0`，价值零增长；
- 部分 debt offset：仅净 Credit 与同比例 floor 净成本入池；
- CNY↔USD 的 source/valuation currency 与冻结 FX；
- redemption/increase 不产生 invitation reward、commission 或 paid referral attribution；
- #22 current_only/BigInt/micros sorter、#23 request 分支和 #26 conversion analytics 不变。

## UI 与六语言

- 复用现有 shadcn/Base UI 与 `AdminCreditBalancePanel`，不新增第二套管理页。
- increase 仅加载并展示 enabled、timed、非 trial/invite-trial、正精确价格、正 Credit、允许不限时购买的档位；无选择不得提交。
- 展示档位标价、档位 Credit、源币种与服务端权威“运营剩余价值”预览；不得称为实收、退款额、负债或可退款余额；管理员动作称为“售后授予”。
- 原始整数/十进制字符串请求不得被紧凑显示改写。
- 可控失败重试保留 key；成功或 operation/plan/amount 等事实变化后生成新 key并清空旧预览。
- 切换 decrease 立即清除 plan/preview/key，payload 不含 `plan_id`；切回 increase 不恢复旧状态。
- 所有新文案补齐 en、zh、fr、ru、ja、vi；`bun run i18n:sync` 必须 missing/extras 为 0。

## 非所有权

- 不实现或复制 #26 FX parser/provider、Option 生命周期、conversion、virtual request snapshot。
- 不实现 #25 decrease/refund/chargeback/recovery。
- 不实现 #27 migration/ready/三数据库实机矩阵。
- 不实现 #28 镜像、备份、部署或生产操作。
- 不重写 #22 CreditValuation 移动平均深模块、#23 request-aware 分支或五接口通用骨架。

## 完成门禁

- 真实 SQLite model/service/controller/router 定向测试，关键不稳定边界 `-count=10`，必要窄 race。
- `go test ./model ./service ./controller ./router -count=1`。
- 受影响前端 tests、typecheck、i18n sync、production build。
- #20–#23 与 #26 代表性回归。
- 真实 Chromium 完成 increase、preview、提交、失败同 key 重试、成功后新事实换 key、CNY↔USD 冻结、operation 切换、五接口/ledger 与邀请隔离。
- 停止服务并删除临时 DB/WAL/SHM 与验收构建残留；gofmt、`git diff --check`、工作树 clean。
