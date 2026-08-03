# Issue #21 状态

## 当前阶段

浏览器续作恢复进行中：已在 clean HEAD `2f9701976282d1c53d7ce0914088a302498f6f32` 接管，不重做已完成领域/API/UI/i18n 实现。当前只生成 Go embed 产物、启动隔离应用、完成真实浏览器 smoke、执行最终窄门禁并清理交付。

固定恢复参数：后端端口 `31021`；临时 SQLite `.scratch/agent-progress/issue-21/browser/issue21-smoke.db`；受监督服务名 `issue21-backend-recovery`；前端构建命令分别为 `bun install --frozen-lockfile` 与 `bun run build`（`web/default`、`web/classic`）。下一步先构建两个 production dist，再以显式隔离 `SQLITE_PATH`、`PORT`、`SESSION_SECRET` 启动服务并通过健康端点确认 readiness。

前次 `go run .` 仅因 `web/default/dist` 与 `web/classic/dist` 缺失在 Go embed 编译期失败；尚未进入数据库、登录或 UI 行为。本续作严格禁止 Credit 核心、FX、marker/ready、历史迁移与发布范围。

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

- Go embed 前端产物缺失阻断真实后端启动；未把静态拦截或组件测试冒充浏览器证据。
- 明确不触碰 Credit 核心、FX、marker/ready、历史迁移或发布。

## 最近安全提交

- 领域/分析基线：`f812e77fcd6e3d2875ce7b973ccc49c87e612590`。
- 管理员 UI：`8e143ca77 feat(subscription): 完成计时售后授予交互`。
- 跨币种展示：`1809124c5 feat(analytics): 展示计时跨币种运营剩余价值`。
- 六语言：`5ea548998 feat(i18n): 补全计时授予六语言`。
- 强五接口 API tracer：`1481d4f97 test(analytics): 强化计时五接口金额追踪`。
