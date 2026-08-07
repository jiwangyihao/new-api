# Issue #26 最终复评续作证据

## 冻结现场

- 起始 HEAD：`44009213cb8e4a582de34f884deecd5a8d687b2c`；起始工作树 clean。
- `b8598f4b7add27ba237f30dec6ceae7968cc2aa3` 与 H1 `3feb091159aef26731c1698647791acc03c29c0a` 均为祖先。
- Orca parent 为 `credit-operational-value-integration`；父树 #24 H2 跨币种 ingress 与路由校准保留。

## M1/M3 已完成

- RED `9ffade1ac`：缺少稳定 sentinel；真实 SQLite history 在 committed unit-value 与其他 operand 不一致时返回重算值。
- GREEN `0f98f18ed`：导出 ineligible/stale sentinel；资格拒绝 `%w`；controller 用 `errors.Is`；history/confirm 直读 committed unit-value。
- 单次与 `-count=10`：model/controller/router 定向集合均 3 packages ok。
- 窄 `-race -count=1`：同一定向集合 3 packages ok。
- `git log -1 --oneline`：`0f98f18ed fix(subscription): 稳定转换错误与已提交单位价值`。
- GREEN 后 `git status --short --branch`：staged/unstaged/untracked 均为 0。

## M2 RED/GREEN

- RED：尚未运行。
- GREEN：尚未运行。
- 下一行为：真实 SQLite API quote 必须返回非空 quote identity、created_at、expires_at、authoritative facts fingerprint；随后改变权威事实并携 quote confirm，必须返回 `subscription_conversion_quote_stale` 且 conversion/ledger/Credit/source 均零写入。
- RED 必须独立提交，再做最小 GREEN。

## 未实测边界

- 尚未运行 M2、前端、完整包级或全仓门禁。
- MySQL/PostgreSQL 实机矩阵属于 #27；未部署、未写生产数据。
