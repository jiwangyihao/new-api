# 渠道分组功能实现计划

规格：`docs/superpowers/specs/2026-06-27-channel-groups-spec.md`
分支：`feature/channel-groups`（worktree `.worktrees/channel-groups`）

## 阶段 A：数据模型与迁移

### A1. ChannelGroup 模型 + 关联表
- 新文件 `model/channel_group.go`：
  - `ChannelGroup` struct（照 spec）。
  - `ChannelGroupChannel`、`TokenGroupBinding` struct。
  - 常量 `DefaultChannelGroupName = "__default__"`。
  - `func normalizeGroupCreditBillingMode(mode string) string`：仅校验 `""`/`usage_tokens`/`fixed_request`，**空串保持空串**。
  - `func (g *ChannelGroup) BillingProfileOrInherit() (ChannelBillingProfile, bool)`：返回 (profile, overrides)；`CreditBillingMode==""` → overrides=false。
  - CRUD：`Insert/Update/DeleteChannelGroupByID/GetAllChannelGroups/GetChannelGroupByID/GetChannelGroupByName/IsChannelGroupNameDuplicated`。
  - 成员：`SetChannelGroupChannels(groupId, channelIds)`、`GetChannelIdsByGroup(groupId)`、`GetGroupIdsByChannel(channelId)`、`GetGroupNamesByChannel(channelId)`。
  - token 绑定：`SetTokenGroupBindings(tokenId, groupIds)`、`GetGroupIdsByToken(tokenId)`、`GetGroupNamesByToken(tokenId)`。
  - `ensureDefaultChannelGroup()`：保证 `__default__` 行存在，inherit。
- `model/main.go`：`migrateDB` + `migrateDBFast` 的 `AutoMigrate` 加入 3 个新模型；迁移后调用 `ensureDefaultChannelGroup()`。
- 测试 `model/channel_group_test.go`：normalize 空串保持、BillingProfileOrInherit inherit/override、name 唯一、默认分组保证。
- commit：`feat(channel-group): 新增分组实体与关联表模型`

### A2. 计费回落 resolver
- `model/channel_group.go`：`func ResolveEffectiveBillingProfile(group *ChannelGroup, channel *Channel) ChannelBillingProfile`：group nil 或 inherit → channel.BillingProfile()；否则分组 profile。
- 测试：inherit 回落、override 覆盖、group nil。
- commit：`feat(channel-group): 计费 profile 分组覆盖与回落渠道`

## 阶段 B：渠道选择按分组过滤

### B1. ability 重建带分组名
- `model/ability.go`：`AddAbilities`/`UpdateAbilities` 改为遍历该渠道分组集合（`GetGroupNamesByChannel` + 默认分组特判：默认分组无显式成员时所有启用渠道都属于它）× model 写行。`FixAbility` 同步。
- 复活 `IsChannelEnabledForGroupModel(group, model, channelId)`、`GetGroupEnabledModels(group)` 真正按 group 过滤。
- commit：`feat(channel-group): abilities 按分组重建`

### B2. cache 按 (group,model)
- `model/channel_cache.go`：`groupModel2channels map[string]map[string][]int`；`InitChannelCache` 构建；`GetRandomSatisfiedChannelForEndpointWithRetryConstraints` 支持分组集合并集。
- 新增多分组入口 `...WithGroups(groups []string, model, ...)`，保留单 group 兼容签名。
- DB 路径 `ability.go` 按 `group IN (?)` 过滤。
- 测试 `model/channel_group_selection_test.go`：单分组过滤、多分组并集、默认分组=全渠道、priority/weight 行为不变。
- commit：`feat(channel-group): 渠道选择按分组并集过滤`

### B3. RetryParam / service 接入
- `service/channel_select.go`：`RetryParam.TokenGroups []string`；`CacheGetRandomSatisfiedChannel` 用并集入口。
- commit：`feat(channel-group): 选择参数支持多分组`

## 阶段 C：distributor 与计费接入

### C1. token 分组解析进 context
- `constant/context_key.go`：`ContextKeyTokenGroups`。
- middleware（auth 或 distributor）：加载 token 绑定分组（空→默认分组名），写 context。
- `middleware/distributor.go` `Distribute()`：用分组集合选 channel；affinity 校验按分组。
- commit：`feat(channel-group): distributor 按 API Key 分组选择渠道`

### C2. 生效分组与计费 profile 写 context
- `SetupContextForSelectedChannel`：确定生效分组（选中渠道所属 ∩ token 分组，非默认优先再 id 升序）；`ResolveEffectiveBillingProfile` 写 billing context keys。
- retry：`controller/relay.go` `relayInfoBillingProfile` / `FrozenBillingProfile` 用生效 profile。
- 测试 `service`/`controller`：inherit 回落、override 覆盖、retry 同 profile。
- commit：`feat(channel-group): 结算使用分组生效计费 profile`

## 阶段 D：controller 与路由

### D1. ChannelGroup CRUD controller
- 新文件 `controller/channel_group.go`：admin CRUD + 成员设置 + 默认分组保护（拒删/拒改名/拒禁用）。
- 用户侧 `GetAvailableChannelGroups`：脱敏（id/name/description）。
- commit：`feat(channel-group): 分组管理 controller`

### D2. 路由
- `router/api-router.go`：`/channel_group` admin 组 + `/channel_group/available` user。
- commit：`feat(channel-group): 分组管理路由`

### D3. token 绑定 API
- `controller/token.go`：`tokenPayload.GroupIds []int`；`AddToken`/`UpdateToken` 持久化绑定；`tokenResponse` 返回 `group_ids`/`group_names`（脱敏）。
- 测试 `controller/token_group_test.go`：增改读绑定、脱敏不含渠道。
- commit：`feat(channel-group): API Key 绑定分组`

## 阶段 E：前端

### E1. 分组管理页（admin）
- 新 feature `web/default/src/features/channel-groups/`：列表/CRUD/成员多选/计费 profile（含 inherit 选项）/默认分组保护。
- commit：`feat(channel-group): 前端分组管理页`

### E2. API Key 表单多选分组
- `api-keys-mutate-drawer.tsx` + types/form：分组多选（数据源 `/channel_group/available`，仅 name/description）。
- commit：`feat(channel-group): API Key 表单选择分组`

### E3. 用户侧脱敏
- keys/用量/日志用户视图只显示分组名，隐藏渠道。
- commit：`feat(channel-group): 用户侧隐藏上游渠道`

### E4. i18n
- `static-keys.ts` + 六语言 locale；`bun run i18n:sync`。
- commit：`feat(channel-group): 分组功能 i18n`

## 阶段 F：验证

- 后端：`go test ./model ./service ./controller ./relay/common ./relay/channel/openai ./router -count=1`（拆包，必要时 service 用 `-timeout 5m`）。
- 前端：`bun run i18n:sync`、`bun run typecheck`、定向测试。
- 只读多子代理 review（后端选择/计费回落、契约脱敏、前端 i18n）。
- 主会话最终验证读取命令输出。

## 核心不可退让行为
- inherit 空串绝不被归一成 usage_tokens。
- 用户侧任何响应都不泄露渠道身份。
- 默认分组无显式成员=全部渠道；有显式成员=按成员。
- 多分组=并集选择。
- 升级后无绑定 token 行为与升级前一致。
