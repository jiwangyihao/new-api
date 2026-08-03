# Issue #21 状态

## 当前阶段

HANDOFF_READY：代码与定向 SQLite/API 验收安全点为 `1481d4f97 test(analytics): 强化计时五接口金额追踪`。管理员 timed grant UI、跨币种运营剩余价值展示、六语言与强五接口 API tracer 均已提交；停止新增实现与测试。

真实浏览器 smoke 尚未完成。首次受监督启动 `go run .` 在编译期失败：`main.go:77:12: pattern web/classic/dist: no matching files found`，原因是当前工作树缺少 Go embed 所需的 `web/default/dist` 与 `web/classic/dist`，尚未进入数据库、登录或 UI 行为。

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
