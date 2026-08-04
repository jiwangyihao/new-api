# Issue #21 状态

## 当前阶段

COMPLETE：Issue #21 浏览器续作与最终交付已完成。真实管理员 timed grant 失败重试、成功后新 key、CNY/USD 跨币种五接口与 UI smoke 均通过；最终定向门禁通过；临时资源已清理。

最终门禁：领域 SQLite tracer PASS；管理员 grant controller 与强五接口 API tracer PASS；`user-subscriptions-dialog` 2/2、`panel-fields` 10/10 PASS；`bun run typecheck` PASS；en/zh/fr/ru/ja/vi missing/extras 全为 0 且 sync 无 diff；production build 已用于真实 embed smoke；`git diff --check` PASS。

清理完成：浏览器 tab 数为 0；受监督服务 `issue21-backend-recovery` 为 exited；目标 SQLite、两个 dist、classic 临时 `node_modules`、误建 `one-api.db` 与临时截图均已删除。只保留已提交的小型文本证据；未触碰其他数据库、Credit、FX、marker/ready、历史迁移或发布。

## 已完成

- 统一 `GrantTimedSubscriptionTx` 与不可变 `TimedSubscriptionValuationGrant` 已接入订单、兑换和管理员真实入口。
- 重放、参数冲突、续期追加、改价/改币种冻结、disabled 新来源拒绝和 grant 更新/删除拒绝已有真实 SQLite 证据。
- summary/users/subscriptions/plans/sources 已统一读取 grant 时间线；强 API tracer 逐端点断言 CNY/USD、跨币种 singular null 与 `mixed_grants`。
- 管理员 UI 已收集 reason、冻结 micros/原币种，失败重试复用 key；跨币种明细、confidence、warning 与 unknown timed 已展示。
- en/zh/fr/ru/ja/vi 新增文案已翻译，六语言 missing/extras 均为 0。

## 浏览器恢复接缝

1. 先生成两个 Go embed 前端目录，再以隔离 `SQLITE_PATH`、`PORT=31021` 启动后端；前端 dev server 使用 `VITE_REACT_APP_SERVER_URL=http://127.0.0.1:31021`。
2. 新隔离库通过 `POST /api/setup` 创建 root，再由 `/api/user/login` 建立浏览器 session。
3. 管理员授予路径：`/users` → 用户行操作 → User Subscription Management；真实请求为 `POST /api/subscription/admin/users/:id/subscriptions`。
4. 跨币种展示路径：`/admin-analytics`，选择 paid-subscription-value；五个请求均位于 `/api/admin-analytics/paid-subscription-value/**`。
5. smoke 必须记录：首次人为制造失败及同 payload/key 重试、成功后新 key，以及 CNY/USD 同卡显示且三个 singular 不被 Plan 币种补猜。

## 剩余门禁

- 无。

## 阻塞

- 无。

## 最近安全提交

- 领域/分析基线：`f812e77fcd6e3d2875ce7b973ccc49c87e612590`。
- 管理员 UI：`8e143ca77 feat(subscription): 完成计时售后授予交互`。
- 跨币种展示：`1809124c5 feat(analytics): 展示计时跨币种运营剩余价值`。
- 六语言：`5ea548998 feat(i18n): 补全计时授予六语言`。
- 强五接口 API tracer：`1481d4f97 test(analytics): 强化计时五接口金额追踪`。
- 浏览器续作恢复：`94c208a8b`、`b9f4d226f`、`c4aa6bb02`、`2ac7995ba`。
