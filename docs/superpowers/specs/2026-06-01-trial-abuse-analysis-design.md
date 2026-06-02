# 注册风控与试用滥用分析独立页面设计

## 背景

当前站点对通过试用码注册、邀请链接注册的用户赠送一天单并发不限量试用。线上排查后确认：整体滥用规模不严重，处置不应基于“未付费”本身，而应关注“试用已结束、未获得有价套餐权益、多账号聚类、高用量”的组合信号。

该能力不属于实时运维指标，不应放入 Admin Ops 实时刷新页面。新功能应是一个独立的后台分析页面，默认不自动刷新，管理员按需查询，用于观察趋势和决定是否需要后续处置策略。

## 目标

- 新增独立“注册风控 / 试用滥用分析”页面。
- 提供只读分析，不自动封禁、不自动改用户状态、不自动撤销试用或套餐。
- 默认手动查询，避免实时轮询和额外刷新压力。
- 使用当前确认的风险口径：
  - 试用已结束；
  - 未获得有价套餐权益；
  - 试用期间有明显用量，账号累计用量仅作为参考展示；
  - 存在同邀请人聚类，或在结构化注册 IP 可用时存在同 IP、自邀请链等多账号聚类信号。
- 支持阈值参数，方便后续根据真实情况调整，而不是写死运营判断。

## 非目标

- 不加入 Admin Ops 实时监控快照。
- 不提供自动处置、批量封禁、批量禁用邀请链接。
- 不把低用量未付费用户视为异常。
- 不把试用尚未结束的用户视为异常。
- 不把兑换码、管理员手动授予、订单、月度邀请奖励等有价套餐权益用户视为未付费用户。
- 不做长期审计表或异步任务调度；本期按需查询即可。

## 导航与权限

- 页面路径：`/trial-abuse`，放在已认证后台路由下。
- 前端路由文件：`web/default/src/routes/_authenticated/trial-abuse/index.tsx`。
- 页面模块：`web/default/src/features/trial-abuse/`，包含 `api.ts`、`types.ts`、`lib/filters.ts`、页面组件与测试。
- 入口名称：`注册风控`，放入后台侧边栏 Admin 分组。侧边栏模块键固定为 `admin.trial_abuse`；实现时必须同步更新以下分发点：`model/user.go` 的默认 sidebar 配置，`web/default/src/hooks/use-sidebar-config.ts` 的 `DEFAULT_SIDEBAR_MODULES` 与 `URL_TO_CONFIG_MAP`，`web/default/src/features/profile/components/sidebar-modules-card.tsx` 的 Admin `sectionDefs`，`web/default/src/hooks/use-sidebar-data.ts` 的菜单项，`web/default/src/features/system-settings/maintenance/config.ts` 的 `SIDEBAR_MODULES_DEFAULT` / parse 默认值，`web/default/src/features/system-settings/maintenance/sidebar-modules-section.tsx` 的 Admin module metadata，以及对应 sidebar 配置测试。
- 权限：管理员权限，与用户管理、订阅管理同级，不使用普通用户权限。
- 页面顶部明确展示：`只读分析，不会自动处置账号`。

## 后端 API

新增只读接口：

`GET /api/trial-abuse/summary`

建议查询参数：

- `trial_end_start` / `trial_end_end`：试用结束时间窗口，Unix 秒；默认最近 14 天，最大跨度 90 天。
- `registered_start` / `registered_end`：可选注册时间辅助筛选；不作为默认主窗口。
- `snapshot_at`：分析快照时间；默认当前时间，用于判断试用是否已结束。
- `min_consume_count`：高用量阈值；默认 `500`，最小 `1`，最大 `100000`。
- `min_cluster_size`：聚类最小账号数；默认 `2`，范围 `2..100`。
- `risk_limit`：返回风险用户数量；默认 `50`，最大 `200`。
- `group_limit`：返回 IP / 邀请人聚类数量；默认 `20`，最大 `100`。
- 非法参数返回 400；超过上限的 limit 类参数按上限截断，时间范围超过最大跨度时返回 400。

响应结构包含：

- `generated_at`：生成时间。
- `criteria`：本次使用的阈值与时间窗口，包括默认值归一化结果。
- `warnings`：日志不可用、IP 不可用、候选数量被截断等非致命降级信息。warning 字段为 `section`、`reason`、`message`；`reason` 使用稳定枚举：`log_unavailable`、`registration_ip_unavailable`、`candidate_limit_exceeded`、`log_limit_exceeded`；`section` 使用稳定枚举：`overview`、`usage_distribution`、`risk_users`、`risk_counts`、`ip_clusters`、`inviter_clusters`、`self_invite_chains`。
- `partial_sections`：机器可读的部分结果标记，类型为 `{ [section: string]: string[] }`；key 使用上述 section 枚举，value 为触发该 section 部分结果的 warning reason 列表。
- `overview`：总体统计，字段包括 `total_trial_users`、`active_trial_users`、`expired_trial_users`、`expired_unpaid_trial_users`、`high_usage_candidate_users`、`risk_user_count`、`high_risk_user_count`、`medium_risk_user_count`、`low_risk_user_count`、`managed_inviter_cluster_count`、`partial`、`partial_reasons`。
- `risk_counts`：高 / 中 / 低风险计数，字段包括 `high`、`medium`、`low`、`partial`、`partial_reasons`。
- `usage_distribution`：未付费已过期试用用户的用量分布，字段包括 `sample_size`、`zero_usage_count`、`above_threshold_count`、`p50`、`p75`、`p90`、`p95`、`p99`、`partial`、`partial_reasons`。
- `ip_clusters`：观察 IP 聚类；仅在结构化 IP 数据可用时参与强风险判定。
- `inviter_clusters`：邀请人聚类。
- `self_invite_chains`：同 IP 自邀请链聚类；仅在结构化注册 IP 可用时启用。
- `risk_users`：风险用户明细。

`risk_users` 最小字段集：`user_id`、`username`、`created_at`、`trial_source`、`trial_start_time`、`trial_end_time`、`inviter_id`、`inviter_username`、`consume_count`、`used_quota`、`metered_tokens`、`observed_ip`、`ip_source`、`registration_ip_available`、`risk_level`、`risk_score`、`risk_reasons`、`paid_entitlement_excluded`、`partial`、`partial_reasons`。

`ip_clusters` 最小字段集：`observed_ip`、`ip_source`、`registration_ip_available`、`candidate_count`、`expired_unpaid_trial_count`、`paid_entitlement_count`、`total_consume_count`、`sample_user_ids`、`partial`、`partial_reasons`。

`inviter_clusters` 最小字段集：`inviter_id`、`inviter_username`、`managed`、`candidate_count`、`expired_trial_invitee_count`、`expired_unpaid_trial_count`、`paid_entitlement_count`、`paid_conversion_rate`、`total_consume_count`、`risk_participation`、`sample_user_ids`、`partial`、`partial_reasons`。

`self_invite_chains` 最小字段集：`chain_id`、`registration_ip_available`、`registration_ip`、`candidate_count`、`total_consume_count`、`nodes`、`partial`、`partial_reasons`；`nodes` 包含 `user_id`、`username`、`inviter_id`。

## 风险口径

### 基础候选

用户必须同时满足：

1. `user_subscriptions.grant_reason IN ('trial_code', 'invite_trial')`；
2. `user_subscriptions.end_time <= snapshot_at`，且位于 `trial_end_start..trial_end_end` 观察窗口内；
3. 历史上不存在任何 `subscription_plans.price_amount > 0` 且归一化来源为 `order`、`redemption`、`admin`、`monthly_invite_entitlement` 的有价套餐权益。首版按“历史有价权益”排除，不要求该权益当前仍有效，避免把已购买后流失用户重新计入试用滥用；
4. 有价权益来源按归一化后的 `grant_reason/source` 判断。归一化规则：先 trim `grant_reason`；非空时使用 `grant_reason`；为空时 fallback 到 trim 后的 `source`。非空 `trial_code` / `invite_trial` 不被 `source` 覆盖；
5. `consume_count >= min_consume_count`。

`consume_count` 来自 `logs.type = LogTypeConsume` 的计数，默认统计试用订阅窗口内的消费日志：`logs.created_at BETWEEN trial.start_time AND trial.end_time`。响应同时返回 `used_quota`、`metered_tokens` 作为参考字段，但首版不把它们作为强制候选条件。若消费日志关闭、缺失或被清理，接口必须返回 `warnings`，不能静默把用量当作 0。

### 聚类信号

候选用户还需要命中至少一种聚类信号：

- 结构化注册 IP 可用时，同注册 IP 下候选账号数达到 `min_cluster_size`；
- 同邀请人下候选账号数达到阈值，且有价权益转化较弱；
- 结构化注册 IP 可用时，邀请人与被邀请人使用同一注册 IP，形成自邀请链；
- 结构化注册 IP 可用时，同 IP 自邀请链中候选账号数达到 `min_cluster_size`。

邀请人转化口径使用两个集合：

- 分母 `expired_trial_invitees`：该邀请人在观察窗口内通过邀请获得试用、且试用已结束的 invitee 总数，不应用高用量阈值，也不排除已获得历史有价权益的 invitee；
- 分子 `paid_invitees`：上述分母集合中历史上获得 `price_amount > 0` 有价套餐权益的 invitee 数。

首版邀请人风险阈值为高用量未付费候选数 `>= 10` 且 `paid_invitees / expired_trial_invitees < 10%`。候选数较小的邀请人只展示，不参与风险等级。

托管邀请渠道保护：若邀请人是 root/admin 角色用户，或后续被配置为销售、售后、官方推广等 managed inviter，则邀请人聚类默认只展示，不单独参与 `medium/high` 风险升级。托管邀请渠道中的账号只有同时命中独立账号级强信号时才进入风险，例如结构化注册 IP 自邀请链、同结构化注册 IP 大簇；当前缺少结构化注册 IP 时，托管邀请渠道只用于展示趋势。

### 风险等级

建议规则：

- `high`：结构化注册 IP 可用且同 IP 自邀请链中至少 3 个候选账号，或同 IP 至少 5 个候选账号。
- `medium`：结构化注册 IP 可用且同 IP 至少 3 个候选账号，或非托管邀请人下至少 10 个候选账号且有价权益转化率 `< 10%`。
- `low`：达到候选条件并命中弱聚类信号；托管邀请渠道仅可产生展示型低置信提示，不进入高/中风险。

风险等级只用于排序与展示，不触发自动动作。

## 数据查询设计

后端查询应优先使用 GORM 与 Go 层合并，避免单库 SQL 方言。若使用 raw SQL，必须为 PostgreSQL、MySQL、SQLite 三种数据库兼容性留出实现分支。

需要聚合的数据：

- `users`：`id`、`username`、`created_at`、`inviter_id`。
- `user_subscriptions`：试用权益、有价权益、`grant_reason`、`source`、`status`、`start_time`、`end_time`。
- `subscription_plans`：`price_amount`、`is_trial`、`invite_trial`、`business_code`。
- `trial_redemptions`：仅用于展示试用码明细，不参与候选过滤。
- `logs`：消费次数、首次消费、最后消费、`quota`、`metered_tokens`。

查询策略：先用 `user_subscriptions.end_time` 观察窗口取有限候选 user_id；批量查询这些用户的历史有价权益；再用 `LOG_DB` 按候选 user_id、`type=LogTypeConsume`、试用时间窗口聚合日志；最后在 Go 层构建聚类和风险等级。禁止逐用户查询日志，避免 N+1。

硬上限：`candidate_limit=5000`，`log_scan_limit=200000`，`log_aggregate_user_limit=5000`。触发候选截断时返回 `candidate_limit_exceeded`，此时 overview、usage_distribution、risk_users 均标记为部分结果；触发日志扫描截断或日志聚合用户截断时返回 `log_limit_exceeded`，此时用量分布和基于用量的风险等级标记为部分结果。实现可使用内部常量，不需要暴露为查询参数。

索引建议：评估是否需要 `logs(user_id, type, created_at)` 或 `logs(type, created_at, user_id)`，以及 `user_subscriptions(user_id, grant_reason, end_time)`。如果不新增索引，必须遵守时间窗口和硬上限，避免全表扫描。

注册 IP 当前没有稳定结构化字段。消费日志 IP 只有用户开启 `record_ip_log` 时才记录，且它是消费 IP，不是注册 IP。本期不得把消费 IP 命名为注册 IP，也不得在缺少结构化注册 IP 时启用“同注册 IP自邀请链”的强风险规则。页面可展示 `observed_ip` / `ip_source=consume_log` 作为弱观察信号，并通过 `registration_ip_available=false` 和 `warnings` 说明 IP 维度受记录配置影响。若后续新增结构化注册 IP 字段，再启用注册 IP 强规则。本期可把 IP 风险分类提取为纯函数，并用构造输入测试 `registration_ip_available=true` 的未来兼容逻辑；生产路径在没有结构化注册 IP 时必须降级为不可用。

## 前端页面

页面结构：

1. 顶部筛选区：
   - 试用结束时间范围，默认最近 14 天，最大 90 天；
   - 可选注册时间范围；
   - 高用量阈值，默认 `500`；
   - 聚类最小账号数，默认 `2`；
   - 查询按钮；
   - 重置按钮；
   - 最近生成时间。
2. 概览卡片：
   - 试用用户数；
   - 试用未结束数；
   - 已结束未付费试用数；
   - 高用量候选数；
   - 高 / 中 / 低风险数。
3. 用量分布：
   - `consume_count` 分位数；
   - 大于阈值人数。
4. 聚类列表：
   - 观察 IP 聚类；
   - 邀请人聚类；
   - 自邀请链，只有 `registration_ip_available=true` 时展示强风险结果，否则展示不可用说明。
5. 风险用户表：
   - 用户 ID；
   - 用户名；
   - 邀请人；
   - 试用来源；
   - 试用结束时间；
   - 消费次数；
   - 观察 IP 与 IP 来源；
   - 风险等级；
   - 风险原因。

页面默认不自动刷新。管理员点击“查询”才请求数据。React Query 必须使用已提交参数作为查询条件：首屏无已提交参数时 `enabled=false` 或等效门控；修改筛选草稿不触发请求；“刷新当前结果”只 refetch 上一次提交的筛选条件。不得设置 `refetchInterval`，并关闭窗口聚焦和网络恢复触发的自动 refetch。queryKey 使用 `['trial-abuse', 'summary', submittedCriteria]`，不得放入 `admin-ops` 或复用 Admin Ops 的自动刷新状态、轮询常量、invalidate 链路。

交互状态：查询中禁用查询按钮；参数非法时前端展示校验错误且不请求；后端错误展示可重试错误态；无风险数据时展示空状态；重置按钮恢复默认条件但不自动请求。

## i18n

新增前端文案必须使用 `t()`，统一放在 `trialAbuse.*` key 命名空间下，并进入 `web/default/src/i18n/locales/*.json`。风险等级、风险原因、表格列、侧边栏入口等动态 key 必须登记到 `web/default/src/i18n/static-keys.ts`，或以字面量 `t('...')` 形式出现以便扫描。中文、英文、法文、日文、俄文、越南文均需补齐，并运行 `bun run i18n:sync`。

风险原因 key 必须使用稳定枚举，首版枚举为：`sameRegistrationIpCluster`、`sameRegistrationIpSelfInviteChain`、`inviterLowPaidConversion`、`managedInviterDisplayOnly`、`registrationIpUnavailable`、`logUnavailable`、`candidateLimitExceeded`、`logLimitExceeded`。后端返回 reason key，前端映射为 `trialAbuse.riskReason.<key>`；不得返回需要直接展示给用户的自由文本作为唯一原因。

## 测试

后端：

- 模型 / 服务层测试风险口径：
  - 试用未结束用户被排除；
  - 历史有价套餐权益用户被排除，包括 `order`、`redemption`、`admin`、`monthly_invite_entitlement`；
  - `grant_reason/source` 归一化符合“非空 grant_reason 优先，空 grant_reason fallback source”；
  - 低用量未付费用户被排除；
  - 试用结束窗口按 `user_subscriptions.end_time` 过滤，而不是默认按注册时间过滤；
  - 邀请人转化率分母使用观察窗口内已结束的邀请试用 invitee 总数，分子使用其中历史有价权益 invitee 数；
  - root/admin 或 managed inviter 的邀请人聚类默认只展示，不单独进入高/中风险；
  - 日志不可用时返回 warning，不静默归零；
  - 结构化注册 IP 不可用时不启用同注册 IP 强风险规则；
  - 本期生产路径在缺少结构化注册 IP 时返回 `registration_ip_unavailable`；IP 可用时的同 IP 和自邀请链规则通过纯风险分类函数测试覆盖；
  - 硬上限触发时返回稳定 warning reason，并标记部分结果。
- 控制器测试参数规范化、非法参数 400、权限路径。

前端：

- API 参数构造测试。
- 首屏不调用 API，点击查询后才请求。
- 修改输入不触发请求，刷新当前结果只使用上一次提交条件。
- 阈值和时间范围非法时展示校验错误且不请求。
- 风险等级、阈值、空状态、错误状态展示测试。
- 页面不设置 `refetchInterval`，不复用 Admin Ops queryKey 或自动刷新状态。
- i18n key 覆盖测试。

验证命令：

- 后端目标包测试：`go test -p 1 ./model ./service ./controller -run 'TrialAbuse|AdminRisk|Risk' -count=1`
- 前端相关测试：按新增测试文件运行 `bun test ...`
- TypeScript：`bun run typecheck`
- i18n：`bun run i18n:sync`

## 交付验收

- 独立页面可由管理员打开，并手动查询结果。
- 页面不会进入 Admin Ops 实时刷新链路。
- API 不修改任何用户、订阅、兑换码或日志数据。
- 风险统计符合当前确认口径，不把正常流失用户计入风险名单。
- 对兑换码销售、管理员售后授予、有价套餐权益用户保持排除。
- 试用未结束用户保持排除。
