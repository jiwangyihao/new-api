# 全面移除业务分组概念设计

## 背景

当前项目中 `group` 已扩散为多个业务维度：

1. **AI 渠道路由分组**：Token、Channel、Ability、自动分组、渠道倍率、日志过滤、性能指标与前端渠道配置。
2. **用户等级/套餐等级分组**：`User.Group`、订阅套餐 `upgrade_group`、订阅到期回退、兑换码升级分组、充值分组倍率。
3. **展示与分析分组**：价格目录 `enable_groups`、用量分析按 group 维度聚合、日志与模型列表展示 group。

用户最新要求明确：**移除用户等级/套餐等级中的 vip 等分组概念。** 因此本次范围调整为：全面移除所有业务含义的分组，包括渠道分组、令牌分组、用户分组、套餐升级分组、充值分组倍率和由这些字段派生的展示/分析维度。

## 目标

- 用户端和管理端不再展示、配置或理解任何业务分组，例如 `default`、`vip`、`svip`、`auto`。
- API key / token 不再有 `group` 或 `cross_group_retry` 语义。
- 渠道选择只基于模型、渠道状态、优先级、权重、标签等非分组维度。
- 用户不再有 `Group` 等级字段参与权限、价格、充值、订阅或渠道选择。
- 订阅套餐不再包含 `upgrade_group`，购买套餐只授予套餐额度、并发、周期等非分组权益。
- 兑换码、订阅到期、余额购买、第三方支付不再更新用户分组或记录分组回退。
- 计费倍率不再包含 `group_ratio`、`group_group_ratio`、`TopupGroupRatio`、special usable group、auto group。
- 日志、性能指标、配置向导、价格展示、用量分析不再暴露 group 字段、过滤器或聚合维度。
- 对旧客户端或旧数据中的 group 字段兼容忽略，不因字段存在而 500。

## 非目标

- 不删除无关的 UI/代码结构概念，例如 Tailwind `group-hover`、按钮组、导航分组、统计分组、OAuth claim 中的 `groups`、Uptime Kuma 监控分组、模型预填组、可复用预填组 `prefill_group` 等非用户/渠道/套餐业务分组。
- 不在第一阶段跨库物理删除所有历史 DB 列；SQLite/MySQL/PostgreSQL 的破坏性 schema 清理由后续迁移单独处理。
- 不改变 token quota、订阅额度、用户状态、邀请奖励资格、支付金额基础公式等非分组逻辑。
- 不把“分组”替换成同义新概念继续承载 vip/svip 权益；移除后不能出现隐藏的 tier/level 变体来绕回同一功能。

## 当前关键耦合点

### 后端数据模型

- `model.User.Group`：用户等级/分组，默认 `default`，被缓存到 `UserBase.Group`。
- `model.Token.Group`、`model.Token.CrossGroupRetry`：token 级渠道分组和自动分组重试。
- `model.Channel.Group`：渠道可用分组，当前默认 `default`。
- `model.Ability.Group`：能力表主键的一部分，决定某个模型在某个分组下可用哪些渠道。
- `model.SubscriptionPlan.UpgradeGroup`：购买套餐后升级用户分组。
- `model.UserSubscription.UpgradeGroup`、`PrevUserGroup`：订阅生效和到期回退用户分组。
- `model.Log.Group`：请求日志记录使用分组，并支持按 group 过滤。
- `model.PerfMetric.Group`：性能指标以 model + group + bucket 聚合。
- `dto.Task.Group`、`dto.PlaygroundRequest.Group`、`dto.UsageAnalytics*Group*`：任务、playground、用量分析仍携带 group 字段。

### 后端运行时路径

- `controller/token.go`：创建/更新 token 时处理 group / cross_group_retry。
- `middleware` 与 relay 上下文：设置 user group、token group、using group、auto group。
- `service/channel_select.go`：基于 token group 或 auto group 选择渠道。
- `model/ability.go`：`GetGroupEnabledModels`、`GetChannel(group, model, retry)`、`AddAbilities`、`UpdateAbilities` 都以 group 为选择维度。
- `controller/config_guide.go`：校验 token/user group，并按 using group 返回模型能力。
- `service/group.go`、`controller/group.go`：用户可用分组、倍率、自动分组 API。
- `setting/ratio_setting/group_ratio.go`、`setting/auto_group.go`、`setting/user_usable_group.go`：分组倍率、自动分组、用户可用分组配置。
- `common/topup-ratio.go` 与多种充值控制器：按用户 group 应用充值倍率。
- `setting/rate_limit.go`：`ModelRequestRateLimitGroup` 仍按 group 配置模型请求限流。
- `controller/subscription.go`、`model/subscription.go`、`controller/subscription_payment_balance.go`、`model/redemption.go`：创建套餐时校验升级分组，购买/兑换/到期时更新或回退用户分组。
- `controller/pricing.go`：按用户可用分组过滤价格并返回 `group_ratio` / `auto_groups`。
- `controller/log.go`、`controller/perf_metrics.go`、`controller/usage_analytics.go`：按 group 过滤或聚合日志、指标、用量分析。
- `service/channel_affinity.go`、`controller/channel_affinity_cache.go`、`setting/operation_setting/channel_affinity_setting.go`：渠道亲和缓存 key/stats 仍包含 `using_group`。

### 前端暴露面

- `web/default`：用户管理仍可编辑/filter `group`；渠道表单要求 `group`；系统设置仍包含 `GroupRatio`、`TopupGroupRatio`、`UserUsableGroups`、`GroupGroupRatio`、`AutoGroups`、`DefaultUseAutoGroup`、special usable group；订阅表单仍有 `upgrade_group`；playground、pricing、usage analytics、static i18n keys 仍有 group。
- `web/classic`：token 编辑、token 列、用户编辑、渠道编辑、分组倍率设置、订阅 `upgrade_group`、playground 分组选择、pricing group filter、日志 group filter 都仍暴露 group。
- `GroupBadge` / group selector / `/api/group/` / `/api/user/self/groups` 是两个前端共享的高风险依赖入口。

## 目标架构

### 用户模型

用户不再有业务分组权益。

- 注册、导入、缓存、登录返回、用户管理不再设置或展示 `group`。
- `UserBase` 不再需要 `Group`。
- 旧数据库中的 users.group 仅作为历史兼容列保留，不进入业务逻辑。
- 删除 `GetUserGroup` / `UpdateUserGroupCache` 等业务用法，或将其调用点改为无分组逻辑。

### 订阅与兑换码

订阅套餐只授予非分组权益。

- `SubscriptionPlan` 创建/编辑不再接受 `upgrade_group`。
- `UserSubscription` 不再记录 `UpgradeGroup` / `PrevUserGroup` 作为业务状态。
- 购买订阅、余额支付、第三方支付、兑换码、邀请试用和到期处理不再更新用户 group。
- 原“用户分组将升级/回退到 X”的提示删除。
- 套餐权益保留：总额度、周期重置、月 token 限额、并发限制、购买限制、试用/邀请/奖励资格。

### 渠道能力表

渠道能力从“按分组 + 模型 + 渠道”降级为“按模型 + 渠道”。

- `Ability` 不再包含业务分组维度。
- `AddAbilities` / `UpdateAbilities` 只根据 `Channel.Models` 生成能力。
- 渠道选择函数改为 `GetChannel(model, retry)` / `GetRandomSatisfiedChannel(model, retry)`。
- 优先级与权重继续保留，作为重试与负载选择维度。
- 旧 Channel.Group / Ability.Group 数据不参与选择。
- 渠道亲和缓存不再包含 `using_group` 维度，相关 stats/API 不再展示 using group。

### Token 与请求上下文

- Token 创建/更新忽略输入中的 `group` 和 `cross_group_retry`。
- 请求上下文不再设置 user group、token group、using group、auto group。
- relay 日志和计费数据不再依赖 group。
- 对旧 API 客户端提交的 `group` 字段保持兼容忽略，不报错。

### 计费与充值倍率

- 删除 channel group ratio / group-group ratio / special usable group 对价格计算的影响。
- 删除 top-up group ratio 对充值金额的影响；充值金额只由基础价格、额度显示模式、预设折扣等非分组参数决定。
- `PriceData` 中 group ratio 信息退化为常量 1 或移除。
- 日志 `other` 中不再注入分组倍率字段。
- 保留模型倍率、补全倍率、缓存倍率、模型固定价格、表达式计费等非分组计费逻辑。

### 配置与 API

- 删除或废弃纯分组接口：`/api/group/`、`/api/user/self/groups`。
- `status` 不再返回 `default_use_auto_group`。
- `pricing` 不再返回 `group_ratio` / `auto_groups`，不再按用户分组过滤渠道价格。
- 配置保存时不再接受或校验 `GroupRatio`、`GroupGroupRatio`、`AutoGroups`、`DefaultUseAutoGroup`、`UserUsableGroups`、`TopupGroupRatio`、`group_special_usable_group` 等业务分组配置。
- `ModelRequestRateLimitGroup` 不再作为可配置项；模型请求限流只保留非分组维度。
- 旧配置项可在读取时忽略；不需要立即从数据库 options 表物理删除。

### 文档、OpenAPI 与 i18n

- `docs/openapi/api.json` 不再声明 `/api/user/groups`、`/api/user/self/groups`、`/api/group/` 这类业务分组接口，也不再在 User/Channel/Token/Subscription schema 中承诺业务 group 字段。
- `/api/prefill_group/` 保留为可复用预填模板接口，但文案和命名在后续清理中应避免与用户/渠道/套餐分组混淆。
- 后端 i18n 与前端 locales/static keys 删除业务分组文案，例如升级分组、分组倍率、自动分组、用户可用分组、cross-group retry。
- README 和配置说明不再描述 `GroupRatio`、`TopupGroupRatio`、`AutoGroups`、`UserUsableGroups` 等选项。

### 日志、指标与分析

- 新日志不再记录业务 group。
- 日志查询 API 忽略 group query 参数，前端移除 group filter。
- 性能指标聚合改为 model + bucket。
- 用量分析移除 `group` group-by 维度、过滤器和 drilldown 字段。
- 历史日志/指标中的 group 列可保留在 DB 中，但不再展示或作为过滤条件。

## 数据库兼容策略

本次推荐 **逻辑删除 + 兼容读取**，不立即物理删除所有 DB 字段。

原因：

- SQLite 不支持直接 drop column，跨 SQLite/MySQL/PostgreSQL 做物理删除成本高。
- 现有表中 `group` 是保留字字段，物理迁移风险高。
- 逻辑删除即可达成产品与运行时移除；后续可单独做破坏性 schema 清理。

具体策略：

- 第一阶段保留旧列，但业务代码不再读取其值做决策。
- 旧 API 请求中的 group 字段被忽略；旧响应逐步移除 group 字段，前端类型同步删除。
- `Ability` 查询与写入改为无 group 逻辑；旧 ability group 数据不参与新选择路径。
- `User.Group`、`SubscriptionPlan.UpgradeGroup`、`UserSubscription.PrevUserGroup` 等旧列暂留但业务不读写。

## 实施分解建议

1. **后端用户/订阅分组退役**：移除 `User.Group`、`upgrade_group`、`PrevUserGroup` 的业务读写；购买/兑换/过期不再更新用户分组；充值倍率不再按 group。
2. **后端渠道选择路径重构**：移除 service/model 层对 group 的渠道选择依赖，确保模型调用仍能选出可用渠道。
3. **后端 token/config/pricing/log/analytics API 清理**：忽略 token group、移除 group API、移除 pricing group 输出、移除日志/指标/用量分析 group 过滤或聚合。
4. **计费结构清理**：将 group ratio 固定为 1 或移除字段，更新日志 other 信息和测试。
5. **文档、OpenAPI 与 i18n 清理**：更新 OpenAPI、README/配置说明、后端 i18n、default/classic locale 与 static keys，删除业务 group 合同和文案。
6. **前端 default 清理**：删除用户、系统设置、渠道配置、订阅、价格、playground、日志、指标、用量分析中的业务 group UI/API/types/i18n。
7. **前端 classic 清理**：同步删除 classic 中的 token/user/channel group、分组设置、订阅升级分组、playground 分组、pricing/log filters。
8. **测试与兼容验证**：补充用户创建/编辑无 group、订阅购买不更新 group、创建/编辑渠道无 group、relay 选择渠道、pricing、日志/analytics、前端源码可见性测试。

## 验收标准

- 用户端 API key 创建/编辑/列表不出现 group 或 cross-group 概念。
- 管理端用户创建/编辑/列表不出现 `default`、`vip`、`svip` 或用户分组字段。
- 管理端订阅套餐创建/编辑不出现升级分组；购买套餐后不更新用户分组。
- 管理端渠道创建/编辑不出现分组选择；渠道保存后模型调用仍能路由到渠道。
- 系统设置中不再出现分组倍率、充值分组倍率、自动分组、用户可用分组设置。
- 价格展示不再返回或展示 `group_ratio` / `auto_groups` / `enable_groups`。
- 日志、性能指标、用量分析不再提供 group 过滤或 group-by；历史数据仍可正常查询。
- 旧客户端提交 user/token/channel/subscription 的 group 字段不会导致 500，字段被忽略。
- OpenAPI、README/配置说明、后端 i18n、前端 locales/static keys 不再承诺或展示业务分组接口、字段和文案。
- Go 定向测试、前端定向测试、`bun run typecheck`、`bun run i18n:sync`、`bun run build` 通过。

## 风险与缓解

- **渠道选择失败风险**：当前 `Ability.Group` 是主键维度，必须先设计无 group 的能力查询并补充 relay 选择测试。
- **订阅权益回归风险**：`upgrade_group` 当前会改变用户权益和过期回退；移除时必须证明套餐额度、并发、月限额、有效期仍照常生效。
- **充值金额回归风险**：`TopupGroupRatio` 会影响金额计算；移除后需要测试 vip/default 不再改变金额，折扣和基础价格仍生效。
- **计费回归风险**：group ratio 参与价格计算，需要用测试证明移除后默认倍率为 1，表达式计费不受影响。
- **跨数据库迁移风险**：避免第一阶段 drop column；采用逻辑删除和兼容忽略。
- **classic/default 双前端不一致风险**：计划中必须同时覆盖两个前端，不能只改 default。
- **误删无关 group 风险**：保留 UI layout group、OAuth claim groups、Uptime Kuma groups、通知 URL 参数 group、模型预填组、SQL `GROUP BY` 等非用户/渠道/套餐业务分组概念。
