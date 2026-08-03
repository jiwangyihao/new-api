# Issue #21 状态

## 当前阶段

浏览器续作验收已完成：两个 production dist、端口 `31021` 隔离服务、真实 setup/login、管理员 timed grant 失败重试与 CNY/USD 跨币种分析均通过。真实浏览器未拦截或 mock API；隔离 SQLite 保存两条连续 immutable grant。

管理员 smoke：首个 CNY 请求在后端受控停止时失败，UI 展示 `Timed grant failed` 与“重试复用同一 key”；服务恢复后完整 payload 原样重放成功；套餐改为 USD、reason 改变后的下一次成功使用新 key。计划列表仅显示启用、非 trial/invite-trial、正 micros 的 timed 计划。

分析 smoke：同一 `snapshot_at=1785785996` 下 summary/users/subscriptions/plans/sources 五接口 recognized 均为 `CNY=39998102`、`USD=10000000` micros；subscription 三个 singular 均为 null，当前 Plan 价格只显示 `$10.00`，页面仍显示 `¥40.00, $10.00`、confidence exact、warnings `—`、unknown timed `0`。下一步执行最终定向窄门禁，然后清理服务、浏览器、目标 DB、dist 与依赖产物。

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

- 执行验收文档规定的 Go 领域/API tracer、两个前端定向测试、typecheck、六语言 sync、`git diff --check`。
- 清理浏览器/服务/隔离 SQLite/dist/依赖产物，更新 COMPLETE 证据并保持工作树 clean。

## 阻塞

- 无产品阻塞；真实浏览器两条 smoke 已通过。
- 明确不触碰其他数据库、Credit 核心、FX、marker/ready、历史迁移或发布。

## 最近安全提交

- 领域/分析基线：`f812e77fcd6e3d2875ce7b973ccc49c87e612590`。
- 管理员 UI：`8e143ca77 feat(subscription): 完成计时售后授予交互`。
- 跨币种展示：`1809124c5 feat(analytics): 展示计时跨币种运营剩余价值`。
- 六语言：`5ea548998 feat(i18n): 补全计时授予六语言`。
- 强五接口 API tracer：`1481d4f97 test(analytics): 强化计时五接口金额追踪`。
- 浏览器续作恢复：`94c208a8b`、`b9f4d226f`、`c4aa6bb02`、`2ac7995ba`。
