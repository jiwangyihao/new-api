# Issue #21 状态

## 当前阶段

浏览器续作恢复进行中：两个 production dist 已就绪，但受监督启动调用实际携带 `env={}`，因此前两次日志均明确显示默认 `http://localhost:3000/`，并在仓库根目录误建默认 `one-api.db`；这不是产品行为失败。当前错误实例也由空 env 调用产生，必须先停止。

固定恢复参数不变：端口 `31021`；临时 SQLite `.scratch/agent-progress/issue-21/browser/issue21-smoke.db`；受监督服务名 `issue21-backend-recovery`。下一次只允许由启动命令本身显式设置 `PORT=31021`、`SQLITE_PATH=.scratch/agent-progress/issue-21/browser/issue21-smoke.db`、隔离 `SESSION_SECRET`，不再依赖 hub 的空 `env` 字段。

下一步：停止当前错误实例；仅在路径确认后删除本续作误建的仓库根 `one-api.db`；按上述唯一命令重启，并同时以 `GET /api/status` 成功响应和目标 SQLite 文件存在证明 readiness。成功后直接进入既定浏览器 smoke，不探索其他启动方案。

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

- 真实浏览器管理员 timed grant（失败同 key 重试、成功后新 key）与跨币种展示 smoke。
- 浏览器完成后运行既定最终窄门禁、更新 evidence、`git diff --check` 并保持工作树 clean。

## 阻塞

- 启动编排错误：空 `env` 令应用监听默认 `3000` 并误建根目录 `one-api.db`；已定位，待按唯一显式命令恢复。
- 产品代码无阻塞；明确不触碰其他数据库、Credit 核心、FX、marker/ready、历史迁移或发布。

## 最近安全提交

- 领域/分析基线：`f812e77fcd6e3d2875ce7b973ccc49c87e612590`。
- 管理员 UI：`8e143ca77 feat(subscription): 完成计时售后授予交互`。
- 跨币种展示：`1809124c5 feat(analytics): 展示计时跨币种运营剩余价值`。
- 六语言：`5ea548998 feat(i18n): 补全计时授予六语言`。
- 强五接口 API tracer：`1481d4f97 test(analytics): 强化计时五接口金额追踪`。
