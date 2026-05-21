# 全面移除业务分组概念设计

## 背景

当前项目中 `group` 已扩散为多个业务维度：

1. **AI 渠道路由分组**：Token、Channel、Ability、自动分组、渠道倍率、渠道亲和缓存、模型请求限流、日志过滤、性能指标与前端渠道配置。
2. **用户等级/套餐等级分组**：`User.Group`、订阅套餐 `upgrade_group`、订阅到期回退、兑换码升级分组、充值分组倍率。
3. **展示与分析分组**：价格目录 `enable_groups`、用量分析和管理端分析里的业务 group 维度、日志与模型列表展示 group。

用户最新要求明确：**移除用户等级/套餐等级中的 `vip` 等分组概念。** 因此本次范围为：全面移除所有业务含义的分组，包括渠道分组、令牌分组、用户分组、套餐升级分组、充值分组倍率、分组限流、分组亲和缓存，以及由这些字段派生的展示/分析维度。

## 目标

- 用户端和管理端不再展示、配置或理解任何业务分组，例如 `default`、`vip`、`svip`、`auto`。
- API key / token 不再有 `group` 或 `cross_group_retry` 语义。
- 渠道选择只基于模型、渠道状态、优先级、权重、标签等非分组维度。
- 用户不再有 `Group` 等级字段参与权限、价格、充值、订阅、模型可见性或渠道选择。
- 订阅套餐不再包含 `upgrade_group`，购买套餐只授予套餐额度、并发、周期等非分组权益。
- 兑换码、订阅到期、余额购买、第三方支付、邀请试用不再更新用户分组或记录分组回退。
- 计费倍率不再包含 `group_ratio`、`user_group_ratio`、`group_group_ratio`、`TopupGroupRatio`、special usable group、auto group。
- 日志、性能指标、配置向导、价格展示、模型列表、用量分析、管理端分析不再暴露业务 group 字段、过滤器或聚合维度。
- 对旧客户端或旧数据中的 group 字段兼容忽略，不因字段存在而 500。

## 非目标

- 不删除无关的 UI/代码结构概念，例如 Tailwind `group-hover`、按钮组、导航分组、统计卡片分组、OAuth claim 中的 `groups`、Uptime Kuma 监控分组、模型预填组、可复用预填组 `prefill_group`、SQL `GROUP BY` 等非用户/渠道/套餐业务分组。
- 不在第一阶段跨库物理删除所有历史 DB 列；SQLite/MySQL/PostgreSQL 的破坏性 schema 清理由后续迁移单独处理。
- 不改变 token quota、订阅额度、用户状态、邀请奖励资格、支付金额基础公式等非分组逻辑。
- 不把“分组”替换成同义新概念继续承载 vip/svip 权益；移除后不能出现隐藏的 tier/level/plan group 变体来绕回同一功能。

## 当前关键耦合点

### 后端数据模型

- `model.User.Group`：用户等级/分组，默认 `default`，被缓存到 `UserBase.Group`。
- `model.Token.Group`、`model.Token.CrossGroupRetry`：token 级渠道分组和自动分组重试。
- `model.Channel.Group`：渠道可用分组，当前默认 `default`。
- `model.Ability.Group`：能力表复合主键的一部分，决定某个模型在某个分组下可用哪些渠道。
- `model.SubscriptionPlan.UpgradeGroup`：购买套餐后升级用户分组。
- `model.UserSubscription.UpgradeGroup`、`PrevUserGroup`：订阅生效和到期回退用户分组。
- `model.Task.Group`、`dto.Task.Group`、`TaskBillingContext.GroupRatio`：异步任务、视频/MJ 任务补扣和重算计费用的分组维度。
- `model.Log.Group`：请求日志记录使用分组，并支持按 group 过滤。
- `model.PerfMetric.Group`：性能指标以 model + group + bucket 聚合。
- `dto.PlaygroundRequest.Group`、`dto.UsageAnalytics*Group*`、`dto.AdminAnalytics*Group*`：playground、用户侧用量分析、管理端分析仍携带业务 group 字段。

### 后端运行时路径

- `controller/token.go`：创建/更新 token 时处理 group / cross_group_retry。
- `middleware/auth.go`、`middleware/distributor.go`、relay 上下文：设置 user group、token group、using group、auto group，并将 group 带入 playground、分发与亲和逻辑。
- `relay/common/relay_info.go`：`RelayInfo` 携带 `TokenGroup`、`UsingGroup`、`UserGroup`。
- `service/channel_select.go`：基于 token group 或 auto group 选择渠道。
- `model/ability.go`：`GetGroupEnabledModels`、`GetChannel(group, model, retry)`、`AddAbilities`、`UpdateAbilities` 都以 group 为选择维度。
- `model/channel_cache.go`、`model/channel_satisfy.go`：内存缓存和能力判断仍是 group -> model -> channel 结构。
- `model/channel.go`、`controller/channel.go`、`controller/channel_upstream_update.go`：渠道搜索、批量 tag 编辑、上游更新仍接受/返回 group 或 groups。
- `controller/config_guide.go`：校验 token/user group，并按 using group 返回模型能力。
- `controller/model.go`、`controller/user.go:GetUserModels`、`controller/model_meta.go`、`model/model_meta.go`：模型列表、用户模型、模型元数据仍按 group 过滤或暴露 `enable_groups`。
- `service/group.go`、`controller/group.go`：用户可用分组、倍率、自动分组 API。
- `setting/ratio_setting/group_ratio.go`、`setting/auto_group.go`、`setting/user_usable_group.go`：分组倍率、自动分组、用户可用分组配置。
- `setting/rate_limit.go`：`ModelRequestRateLimitGroup` 仍按 group 配置模型请求限流。
- `common/topup-ratio.go` 与多种充值控制器：按用户 group 应用充值倍率。
- `controller/subscription.go`、`model/subscription.go`、`controller/subscription_payment_balance.go`、`model/redemption.go`：创建套餐时校验升级分组，购买/兑换/到期时更新或回退用户分组。
- `relay/helper/price.go`、`service/log_info_generate.go`、`service/task_billing.go`、`controller/task_video.go`：普通 relay、日志 other、异步任务重算和视频/MJ 补扣仍应用 group ratio 或输出 group。
- `controller/pricing.go`：按用户可用分组过滤价格并返回 `group_ratio` / `auto_groups`。
- `controller/log.go`、`controller/perf_metrics.go`、`controller/usage_analytics.go`：按 group 过滤或聚合日志、指标、用户侧用量分析。
- `controller/admin_analytics.go`、`model/admin_analytics*.go`、`dto/admin_analytics.go`：管理端分析仍解析 `user_groups`、`request_groups`、`group_by=user_group/request_group`，并返回用户分组分布、drilldown 的 `UserGroup` / `RequestGroup`。
- `service/channel_affinity.go`、`controller/channel_affinity_cache.go`、`setting/operation_setting/channel_affinity_setting.go`：渠道亲和缓存 key/stats 仍包含 `using_group`，规则上下文仍有业务 group key。
- `model/option.go`、`controller/option.go`：启动和配置保存仍加载、校验、更新多种 group option。

### 前端暴露面

- `web/default`：用户管理、个人资料、profile dropdown、mobile drawer 仍显示/编辑/filter `group`；渠道表单要求 `group`；系统设置仍包含 `GroupRatio`、`TopupGroupRatio`、`UserUsableGroups`、`GroupGroupRatio`、`AutoGroups`、`DefaultUseAutoGroup`、`ModelRequestRateLimitGroup`、channel affinity 的 `include_using_group` / `using_group`、special usable group；订阅表单仍有 `upgrade_group`；playground、pricing、usage logs、perf metrics、usage analytics、admin analytics、models `enable_groups`、static i18n keys 仍有业务 group。
- `web/classic`：token 编辑、token 列、用户编辑/用户资料、渠道编辑/tag 编辑、分组倍率设置、请求限流分组设置、channel affinity `include_using_group` / `using_group`、订阅 `upgrade_group`、playground 分组选择、pricing group filter、logs/admin analytics group filter 都仍暴露业务 group。
- `GroupBadge` / group selector / `/api/group` / `/api/group/` / `/api/user/groups` / `/api/user/self/groups` 是两个前端共享的高风险依赖入口。
- default/classic 模型管理页、模型类型、模型表单、上游冲突对比中仍暴露或依赖 `enable_groups`。

## 目标架构

### 用户模型

用户不再有业务分组权益。

- 注册、导入、缓存、登录返回、用户管理、个人资料、全局壳层不再设置或展示 `group`。
- `UserBase` 不再需要业务 `Group` 字段进入认证上下文或响应 DTO。
- 旧数据库中的 users.group 仅作为历史兼容列保留，不进入业务逻辑。
- 删除 `GetUserGroup` / `UpdateUserGroupCache` 等业务用法，或将其调用点改为无分组逻辑。
- 用户搜索不再接受 group 过滤；旧 group 查询参数兼容忽略。

### 订阅与兑换码

订阅套餐只授予非分组权益。

- `SubscriptionPlan` 创建/编辑不再接受 `upgrade_group`。
- `UserSubscription` 不再把 `UpgradeGroup` / `PrevUserGroup` 作为业务状态。
- 购买订阅、余额支付、第三方支付、兑换码、邀请试用、邀请奖励、到期处理不再更新用户 group。
- 原“用户分组将升级/回退到 X”的提示删除。
- 套餐权益保留：总额度、周期重置、月 token 限额、并发限制、购买限制、试用/邀请/奖励资格。

### 渠道能力表、缓存与亲和

渠道能力从“按分组 + 模型 + 渠道”降级为“按模型 + 渠道”。

- `Ability` 业务上不再包含分组维度。
- 第一阶段保留物理 `abilities.group` 列和旧复合主键，不依赖 AutoMigrate drop/rebuild。
- 新写入的 ability 使用固定兼容值空字符串，仅用于满足旧复合主键；该值不得对外展示或用于业务选择。
- `UpdateAbilities` 按 channel 清理旧行后仅按模型写入一条有效能力，避免旧多 group 行重复。
- 新查询按 `model/channel_id` 去重并忽略旧 group 值；旧 ability group 数据不参与选择。
- `AddAbilities` / `UpdateAbilities` 只根据 `Channel.Models` 生成能力。
- 渠道选择函数改为 `GetChannel(model, retry)` / `GetRandomSatisfiedChannel(model, retry)`。
- `model/channel_cache.go` 的缓存结构改为 model -> channel availability；`model/channel_satisfy.go` 不再判断 group。
- `EditChannelByTag`、渠道状态更新、ability status 同步、批量上游更新不再接受或传播 group/groups。
- 优先级与权重继续保留，作为重试与负载选择维度。
- 渠道亲和缓存不再包含 `using_group` 维度，规则上下文不再提供业务 group key，stats/API 不再展示 using group。

### Token、relay 与模型列表

- Token 创建/更新忽略输入中的 `group` 和 `cross_group_retry`。
- 请求上下文不再设置 user group、token group、using group、auto group。
- `RelayInfo` 不再向计费、日志、重试、channel affinity 提供业务 group。
- 模型列表 API、用户模型 API、config guide 模型列表改为无分组的 enabled-model 查询，不再依赖 `GetUserGroup`、token group、auto group 或 user usable group 聚合。
- 对旧 API 客户端提交的 `group` / `cross_group_retry` 字段保持兼容忽略，不报错。

### 计费、充值与异步任务

- 删除 channel group ratio / group-group ratio / special usable group 对价格计算的影响。
- 删除 top-up group ratio 对充值金额的影响；充值金额只由基础价格、额度显示模式、预设折扣等非分组参数决定。
- `PriceData` 内部可临时保留常量 1 以减少改动，但所有 response、日志 other、OpenAPI、前端 types 不得暴露 `group_ratio`、`user_group_ratio`、`group_group_ratio`。
- 日志 `other` 中不再注入分组倍率字段。
- 新异步任务不再写入或展示 `model.Task.Group`；旧列仅历史兼容。
- `TaskBillingContext.GroupRatio` 移除或固定为 1；`service/task_billing.go` 重算不得回读 `User.Group` 或 group ratio。
- 任务/MJ/video DTO、任务日志、补扣/退费路径不再输出或依赖 group。
- 保留模型倍率、补全倍率、缓存倍率、模型固定价格、表达式计费等非分组计费逻辑。

### 配置与 API

- 第一阶段保留 `/api/group`、`/api/group/`、`/api/user/groups`、`/api/user/self/groups` 路由作为兼容 shim，但 OpenAPI 不再声明为业务能力。
- 兼容 shim 行为：返回 `success: true`；`/api/group` 与 `/api/group/` 返回空数组 `data: []`；用户分组接口返回空对象 `data: {}`；不返回 ratio、desc、auto 等业务信息。
- `status` 不再返回 `default_use_auto_group`。
- `pricing` 不再返回 `group_ratio` / `auto_groups`，不再按用户分组过滤渠道价格。
- `ModelRequestRateLimitGroup` 不再作为可配置项；模型请求限流只保留非分组维度。
- 配置保存兼容旧 option，但业务 no-op：`GroupRatio`、`GroupGroupRatio`、`AutoGroups`、`DefaultUseAutoGroup`、`UserUsableGroups`、`TopupGroupRatio`、`group_special_usable_group`、`ModelRequestRateLimitGroup` 的写入请求返回成功但不更新业务内存状态。
- 旧配置项可在读取时忽略；不需要立即从数据库 options 表物理删除。

### 旧输入兼容矩阵

- token payload：`group`、`cross_group_retry` 忽略，响应不暴露业务 group。
- user payload/query：`group` 忽略，搜索 group 过滤忽略，响应不暴露业务 group。
- channel payload/query/tag edit/upstream update：`group`、`groups` 忽略，搜索 group 过滤忽略，响应不暴露业务 group。
- subscription payload：`upgrade_group`、`prev_user_group` 忽略，响应不暴露业务 group。
- playground/task payload：`group` 忽略，按无分组模型/渠道选择执行。
- log/perf/admin/user analytics query：group filters 忽略；业务 group-by 值按端点既有校验机制返回 400 或回退到默认非分组维度，但不得 500；实现计划需为每个端点固定具体行为并测试。
- group option keys：保存请求 no-op 成功，不改变运行时业务设置。

### 文档、OpenAPI 与 i18n

- `docs/openapi/api.json` 不再声明 `/api/user/groups`、`/api/user/self/groups`、`/api/group/` 这类业务分组接口，也不再在 User/Channel/Token/Subscription schema 中承诺业务 group 字段。
- `/api/prefill_group/` 保留为可复用预填模板接口，但文案和命名在后续清理中应避免与用户/渠道/套餐分组混淆。
- 后端 i18n、前端 locales/static keys、代码内硬编码错误文案/日志文案删除业务分组文案，例如升级分组、分组倍率、自动分组、用户可用分组、cross-group retry、Group & Model Pricing。
- README 和配置说明不再描述 `GroupRatio`、`TopupGroupRatio`、`AutoGroups`、`UserUsableGroups` 等选项。
- 文档/i18n/OpenAPI 清理使用 allowlist，保留 `/api/prefill_group/`、OAuth claim groups、Uptime Kuma groups、UI/表格/导航/统计分组、SQL `GROUP BY`、CSS/Tailwind group 等非业务 group。

### 日志、指标与分析

- 新日志不再记录业务 group。
- 日志查询 API 忽略 group query 参数，前端移除 group filter。
- 性能指标聚合改为 model + bucket。
- 用户侧用量分析保留通用 `group_by` 机制，但删除业务值 `group`，删除 `Groups` filter、drilldown group 字段和 `group_by=group` 前端入口。
- 管理端分析删除/忽略 `user_groups`、`request_groups` filters，删除 `user_group` / `request_group` group-by 维度，删除生命周期/分布/drilldown 中的 `UserGroup` / `RequestGroup` 输出。
- 历史日志/指标中的 group 列可保留在 DB 中，但不再展示或作为业务过滤条件。

## 数据库兼容策略

本次推荐 **逻辑删除 + 兼容读取**，不立即物理删除所有 DB 字段。

原因：

- SQLite 不支持直接 drop column，跨 SQLite/MySQL/PostgreSQL 做物理删除成本高。
- 现有表中 `group` 是保留字字段，物理迁移风险高。
- `abilities.group` 是复合主键的一部分，第一阶段重建主键风险高。
- 逻辑删除即可达成产品与运行时移除；后续可单独做破坏性 schema 清理。

具体策略：

- 第一阶段不得执行 DROP/ALTER 删除业务 group 历史列；新旧 SQLite/MySQL/PostgreSQL 的遗留列策略保持一致。
- Go struct 可保留 GORM legacy 字段以维持 AutoMigrate 和旧库启动，但 API DTO/response 不暴露这些字段。
- 业务代码不得 Select/Where/Update `User.Group`、`Token.Group`、`Channel.Group`、`SubscriptionPlan.UpgradeGroup`、`UserSubscription.PrevUserGroup` 等旧列做决策。
- 残留 raw SQL 如必须引用历史 `group` 列，仅限迁移/兼容路径，并继续使用 `commonGroupCol` / `logGroupCol` 保证跨库引用正确。
- 旧 API 请求中的 group 字段被忽略；旧响应按端点级规则立即移除或返回无业务信息的兼容空结构，不使用“逐步”作为验收口径。
- `Ability` 查询与写入改为无 group 逻辑；旧 ability group 数据不参与新选择路径。
- `model/main.go` 中 SQLite `ensureSubscriptionPlanTableSQLite` 继续创建/补齐 `upgrade_group` 属于兼容保留，不代表业务读写。
- 实现完成后需要 SQLite/MySQL/PostgreSQL 启动迁移 smoke test 或等价的三库迁移验证计划。

## 实施分解建议

1. **后端用户/订阅分组退役**：移除 `User.Group`、`upgrade_group`、`PrevUserGroup` 的业务读写；购买/兑换/过期不再更新用户分组；充值倍率不再按 group。
2. **后端渠道选择路径重构**：移除 service/model 层对 group 的渠道选择依赖，确保模型调用仍能选出可用渠道。
3. **后端 token/config/pricing/model/log/analytics API 清理**：忽略 token group、移除 group API 业务输出、移除 model/config-guide group 可见性、移除 pricing group 输出、移除日志/指标/用户侧和管理端分析 group 过滤或聚合。
4. **计费与任务结构清理**：将 group ratio 固定为 1 或移除字段，更新普通 relay、异步任务、视频/MJ、日志 other 信息和测试。
5. **兼容 shim 与 OpenAPI 清理**：保留旧 group endpoint no-op 空响应，更新 OpenAPI、README/配置说明、后端 i18n、default/classic locale 与 static keys，删除业务 group 合同和文案。
6. **前端 default 清理**：删除用户、个人资料、全局壳层、系统设置、请求限流、channel affinity、渠道配置、模型管理、订阅、价格、playground、日志、指标、用户侧和管理端分析中的业务 group UI/API/types/i18n。
7. **前端 classic 清理**：同步删除 classic 中的 token/user/profile/channel group、请求限流分组设置、channel affinity using_group、分组设置、订阅升级分组、playground 分组、pricing/log/admin analytics filters。
8. **测试与兼容验证**：补充用户创建/编辑无 group、订阅购买/兑换/过期不更新 group、充值不按 group 改金额、创建/编辑渠道无 group、relay 选择渠道、config guide、model list、pricing、日志/perf/用户侧与管理端 analytics、旧 endpoint shim、旧 option no-op、前端源码可见性测试。

## 验收标准

- 用户端 API key 创建/编辑/列表不出现 group 或 cross-group 概念。
- 用户端个人资料、profile dropdown、mobile drawer、classic personal settings 不出现 `default`、`vip`、`svip` 或用户分组字段。
- 管理端用户创建/编辑/列表不出现 `default`、`vip`、`svip` 或用户分组字段。
- 管理端订阅套餐创建/编辑不出现升级分组；购买、兑换、邀请试用、到期处理后不更新用户分组。
- 管理端渠道创建/编辑/tag 编辑/上游批量更新不出现分组选择；渠道保存后模型调用仍能路由到渠道。
- 系统设置中不再出现分组倍率、充值分组倍率、自动分组、用户可用分组、分组请求限流、channel affinity using group 设置。
- 模型列表、用户模型、config guide 不再按用户/token/channel group 限制可见模型。
- 价格展示不再返回或展示 `group_ratio` / `user_group_ratio` / `group_group_ratio` / `auto_groups` / `enable_groups`。
- 日志、性能指标、用户侧用量分析、管理端分析不再提供业务 group 过滤或 business group-by；历史数据仍可正常查询。
- 旧客户端提交 user/token/channel/subscription/playground/task 的 group/cross_group_retry/groups/upgrade_group/prev_user_group 字段不会导致 500，字段被忽略。
- 旧 group endpoint 仍返回 no-op 空结构，不再返回业务分组；旧 group option 写入 no-op 成功且不改变运行时业务状态。
- OpenAPI、README/配置说明、后端 i18n、代码硬编码文案、前端 locales/static keys 不再承诺或展示业务分组接口、字段和文案；allowlist 中的非业务 group 保持可用。
- 后端定向测试至少覆盖：token 旧字段忽略且响应不暴露；config guide/model list 无分组可见性；pricing 普通/管理员响应无业务 group；subscription 购买/兑换/过期不读写用户 group；topup stripe/epay/waffo/waffo-pancake 不按用户 group 改金额；channel routing/ability/cache/affinity 无 using_group 仍能选路；rate limit group option no-op；logs/perf/user analytics/admin analytics 旧 group 查询参数按既定兼容行为处理；旧 endpoint shim 和旧 option no-op。
- 前端定向测试至少覆盖 default/classic 业务 group 源码可见性边界，避免误删 allowlist 非业务 group。
- `go test` 定向测试、前端定向测试、`bun run i18n:sync`、`bun run typecheck`、`bun run build` 通过；三库迁移 smoke test 或等价验证完成。

## 风险与缓解

- **渠道选择失败风险**：当前 `Ability.Group` 是主键维度，必须先设计无 group 的能力查询并补充 relay 选择测试。
- **订阅权益回归风险**：`upgrade_group` 当前会改变用户权益和过期回退；移除时必须证明套餐额度、并发、月限额、有效期仍照常生效。
- **充值金额回归风险**：`TopupGroupRatio` 会影响金额计算；移除后需要测试 vip/default 不再改变金额，折扣和基础价格仍生效。
- **计费回归风险**：group ratio 参与价格计算，需要用测试证明外部响应和日志不再暴露 group ratio，内部临时常量 1 不改变非分组计费逻辑，表达式计费不受影响。
- **旧客户端兼容风险**：旧前端和外部客户端仍可能调用 group endpoints、提交 group payload 或保存 group options；通过 no-op shim、字段忽略和定向 API 测试缓解。
- **跨数据库迁移风险**：避免第一阶段 drop column；采用逻辑删除和兼容忽略；三库迁移验证覆盖旧列保留、`group` 保留字和 `abilities.group` 复合主键。
- **classic/default 双前端不一致风险**：计划中必须同时覆盖两个前端，不能只改 default。
- **误删无关 group 风险**：保留 UI layout group、OAuth claim groups、Uptime Kuma groups、通知 URL 参数 group、模型预填组、`prefill_group`、SQL `GROUP BY` 等非用户/渠道/套餐业务分组概念。
- **同义新权益风险**：不得新增 `tier`、`level`、`plan group` 等字段继续承载 vip/svip 权益；新增权益只能来自订阅额度、并发、月限额、有效期、business_code 等非分组维度。
