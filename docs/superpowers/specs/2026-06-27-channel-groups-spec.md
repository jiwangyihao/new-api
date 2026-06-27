# 渠道分组（Channel Group）功能规格说明

## 背景与目标

当前 new-api 的渠道选择按 `model` 直接匹配所有启用渠道（`model2channels`），用户（API Key）无法控制自己的请求由哪些上游渠道服务；计费模式（按 token / 按请求固定扣费 / 动态倍率）只能配置在单个渠道上。

本功能引入**渠道分组**实体，实现：

1. 管理员可以把若干渠道打包进一个**分组**。
2. 用户为 API Key 选择**一个或多个分组**，从而自行决定请求由哪些渠道服务。
3. **用户只能看到分组**（id + 名称 + 描述），不能知道分组背后的真实上游渠道。
4. 计费模式配置**移动到分组**上；分组未配置计费方式时**回落到具体渠道**（分组计费方式无默认值）。
5. 用户未给 API Key 选择分组时，显式落到一个**默认分组**（默认允许所有渠道，管理员可在网站中手动配置默认分组的设置）。

## 术语

- **渠道（Channel）**：上游 provider 实例，已有实体。
- **分组（ChannelGroup）**：本功能新增实体。一个分组包含多个渠道（N:N）；一个渠道可属于多个分组。
- **默认分组（Default Group）**：固定存在的一行 ChannelGroup，`name = __default__`，不可删除、不可改名。语义为“允许所有渠道”，但管理员可编辑其计费 profile。
- **计费 profile**：`credit_billing_mode` + `fixed_request_credits` + `dynamic_billing_multiplier_enabled` + `token_billing_multiplier`，已存在于渠道（credit-billing 功能）。
- **inherit（继承/未配置）**：分组计费 `credit_billing_mode` 的第三态（空串），表示该分组不覆盖计费方式，结算时回落到所选中渠道自身的计费 profile。

## 设计决策（已与用户确认）

| 决策点 | 选择 |
|---|---|
| 渠道↔分组、API Key↔分组 关系存储 | 新建 `ChannelGroup` 实体 + 两张关联表 `channel_group_channels`、`token_group_bindings`，分组用稳定 ID 引用 |
| 默认分组 | 固定 DB 行（`name = __default__`），admin 可改其计费 profile 与成员 |
| 分组计费 inherit | `credit_billing_mode` 新增空串/`inherit` 第三态；非空时分组 profile 覆盖渠道，空时回落渠道 |
| 多分组选择范围 | 所选多个分组的渠道**并集**，按现有 priority + weight 随机选 |

## 数据模型

### ChannelGroup（新表 `channel_groups`）

照 `PrefillGroup` 范式：

```go
type ChannelGroup struct {
    Id          int            `json:"id"`
    Name        string         `json:"name" gorm:"size:64;not null;uniqueIndex:uk_channel_group_name,where:deleted_at IS NULL"`
    Description string         `json:"description" gorm:"type:varchar(255)"`
    Enabled     bool           `json:"enabled" gorm:"not null;default:true"`

    // 计费 profile（inherit 语义：CreditBillingMode 为空串表示不覆盖，回落渠道）
    CreditBillingMode               string  `json:"credit_billing_mode" gorm:"type:varchar(32);not null;default:''"`
    FixedRequestCredits             int64   `json:"fixed_request_credits" gorm:"not null;default:0"`
    DynamicBillingMultiplierEnabled bool    `json:"dynamic_billing_multiplier_enabled" gorm:"not null;default:false"`
    TokenBillingMultiplier          float64 `json:"token_billing_multiplier" gorm:"not null;default:1"`

    CreatedTime int64          `json:"created_time" gorm:"bigint"`
    UpdatedTime int64          `json:"updated_time" gorm:"bigint"`
    DeletedAt   gorm.DeletedAt `json:"-" gorm:"index"`
}
```

- `credit_billing_mode` 取值：`""`（inherit）、`usage_tokens`、`fixed_request`。
- **关键约束**：`""` 不得被 `normalizeCreditBillingMode` 归一成 `usage_tokens`；分组层的 normalize 必须保留空串语义。
- 默认分组 `__default__` 在迁移后用 `ensureDefaultChannelGroup()` 保证存在，初始 `CreditBillingMode=""`（inherit）。

### 关联表 channel_group_channels（新表）

```go
type ChannelGroupChannel struct {
    ChannelGroupId int `gorm:"primaryKey;autoIncrement:false;index"`
    ChannelId      int `gorm:"primaryKey;autoIncrement:false;index"`
}
```

- 渠道与分组多对多。
- 默认分组**不在此表显式列出全部渠道**；其“允许所有渠道”语义在选择路径与 ability 重建处特判（见下）。但管理员可手动收窄默认分组成员（一旦默认分组有显式成员行，则按显式成员处理；为空则=全部渠道）。

### 关联表 token_group_bindings（新表）

```go
type TokenGroupBinding struct {
    TokenId        int `gorm:"primaryKey;autoIncrement:false;index"`
    ChannelGroupId int `gorm:"primaryKey;autoIncrement:false;index"`
}
```

- API Key 与分组多对多。
- Token 未绑定任何分组 → 视为绑定默认分组。

### Ability 复活 group 维度

- `Ability.Group`（已存在，复合主键的一部分）从 legacy 空串改为存**分组名**（或分组 id 字符串；采用**分组名**以贴合 `Ability.Group varchar(64)` 现状与 group 字符串语义）。
  - 决策细化：`Ability.Group` 存 `ChannelGroup.Name`。重命名分组时需同步刷新 abilities（重命名属低频管理操作，重建 abilities 可接受）。
- 渠道重建 abilities（`AddAbilities` / `UpdateAbilities` / `FixAbility` / `InitChannelCache`）时：对该渠道所属的**每个分组**（由 `channel_group_channels` 决定）× 每个 model，写一行 `(group_name, model, channel_id)`。
- 默认分组：每个**启用渠道**都隐式属于默认分组（除非默认分组被管理员配置了显式成员），因此重建时为默认分组名也写 ability 行。

## 渠道选择路径改造

### 缓存结构

`model2channels map[string][]int`（仅 model 键）改为按 `(group, model)` 维度：

```go
var groupModel2channels map[string]map[string][]int // group -> model -> channelIds
```

- `InitChannelCache` 遍历每个启用渠道的分组集合（含默认分组特判），构建 `groupModel2channels[group][model]`。
- 保持 per-(group,model) 内的 priority 排序。

### 选择函数

`GetRandomSatisfiedChannelForEndpointWithRetryConstraints(group, model, ...)` 的 `group` 参数**复活**：

- 单分组：在 `groupModel2channels[group][model]` 内选择。
- 多分组（API Key 绑定多个分组）：取所有所选分组的 `groupModel2channels[g][model]` **并集去重**后，统一走现有 `selectCachedChannelByPriority`（priority 分桶 + weight 平滑随机）。
- DB 路径 `GetChannelForEndpointWithRetryConstraints` 同步按 `abilities.group IN (?)` 过滤。

### distributor 接入

`middleware/distributor.go`：

- 解析 token 绑定的分组集合（`token_group_bindings`，空 → `[__default__]`）。
- 写入 context（新 `ContextKeyTokenGroups []string` 或复用 `ContextKeyTokenGroup`）。
- `Distribute()` 选 channel 时把分组集合传入 `RetryParam.TokenGroups`（`service/channel_select.go` 的 `RetryParam` 增加 `TokenGroups []string`，保留 `TokenGroup` 兼容）。
- channel affinity 路径同样按分组集合校验（`IsChannelEnabledForGroupModel` 复活按分组判断）。

## 计费回落链

选中渠道后（`SetupContextForSelectedChannel`）：

1. 确定本次请求命中的**分组**（多分组时，命中渠道所属、且在 token 所选分组集合内的那个分组；若命中多个，取确定性优先：按分组 id 升序第一个，或默认分组优先级最低）。
   - 决策细化：记录“**生效分组**”= 选中渠道所属分组 ∩ token 所选分组，取其中**非默认分组优先、再按 id 升序**的第一个；若只剩默认分组则用默认分组。
2. 解析**生效计费 profile**：
   - 生效分组 `credit_billing_mode != ""`（非 inherit）→ 用分组 profile（mode/fixed_credits/dynamic_enabled/token_multiplier）。
   - 生效分组 `credit_billing_mode == ""`（inherit）→ 回落到渠道 `BillingProfile()`。
3. 把生效 profile 写入现有 billing context keys（`ContextKeyChannelCreditBillingMode` 等），下游 `FreezeChannelTokenBillingSnapshot` / 结算逻辑**无需改动**。

新增解析函数（`model` 或 `service` 层）：

```go
func ResolveEffectiveBillingProfile(group *ChannelGroup, channel *Channel) ChannelBillingProfile
```

- `group == nil || group.CreditBillingMode == ""` → `channel.BillingProfile()`。
- 否则 → 分组 profile（经分组层 Normalize，但 inherit 不参与本分支）。

retry 一致性：`RequireSameBillingProfile` 已存在；生效 profile 作为 `FrozenBillingProfile` 传入，保证重试命中相同计费 profile 的渠道。

## 控制器与路由

### Admin（`middleware.AdminAuth()`）

`apiRouter.Group("/channel_group")`：

- `GET /` 列表（含成员渠道 id、计费 profile）。
- `POST /` 创建。
- `PUT /` 更新（默认分组只能改 profile/成员/描述，不能改名/删除/禁用）。
- `DELETE /:id` 删除（默认分组拒绝删除）。
- `PUT /:id/channels` 设置成员渠道。

### User（`middleware.UserAuth()`）

- `GET /channel_group/available`：返回当前用户可选分组的**脱敏视图**（仅 `id`、`name`、`description`），**绝不含渠道信息**。

### Token 接入

- `tokenPayload` 增加 `GroupIds []int`（绑定分组 id 列表）。
- `AddToken` / `UpdateToken` 持久化 `token_group_bindings`。
- `tokenResponse` 返回 `group_ids []int` + `group_names []string`（脱敏，不含渠道）。

## 前端

遵循 `web/default/AGENTS.md`。

1. **分组管理页（admin）**：`web/default/src/features/channel-groups/`（新 feature）或并入 channels feature。CRUD + 成员渠道多选 + 计费 profile 配置（含 inherit 选项）。默认分组特殊标记、禁止删除/改名。
2. **API Key 表单多选分组**：`web/default/src/features/keys/components/api-keys-mutate-drawer.tsx` 增加分组多选（数据源 `/channel_group/available`，仅显示 name/description）。
3. **用户侧不可见上游**：用户视图（keys、用量、日志）只显示分组名，绝不显示渠道名/id。日志若原本显示 channel，对非 admin 用户改为显示生效分组名或脱敏。
4. **i18n**：六语言（en/zh/fr/ja/ru/vi），更新 `static-keys.ts`，运行 `bun run i18n:sync`。

## 兼容性

- `Token.Group` / `Token.CrossGroupRetry` legacy 单串字段保留，不再使用；新逻辑走 `token_group_bindings`。
- `Channel.Group` CSV legacy 字段保留兼容（不再作为分组成员来源；成员来源为 `channel_group_channels`）。
- 现有渠道计费 profile 不变；分组 inherit 时行为与改造前完全一致（回落渠道）。
- 迁移：新表 `channel_groups` / `channel_group_channels` / `token_group_bindings` 加入 `migrateDB()` 与 `migrateDBFast()` 的 `AutoMigrate`；`ensureDefaultChannelGroup()` 在迁移后执行。
- 现有部署升级后：所有 token 无绑定 → 默认分组 → 默认分组无显式成员 → 全部渠道 → 行为与升级前一致。

## 约束

- Go：JSON 用 `common.Marshal`/`common.Unmarshal`；SQLite/MySQL/PostgreSQL 兼容；不把分组计费塞进 `pkg/billingexpr`。
- `logs.quota`、`users.used_quota`、`channels.used_quota` 等 legacy quota 口径不变。
- 计费 inherit 第三态空串绝不被归一成 `usage_tokens`（分组层 normalize 独立于渠道层）。
- 测试针对 resolver/选择/回落**行为**，非 bundled 数据；禁止 source-grep 测试。

## 验收标准

1. 管理员可创建/编辑/删除分组，配置成员渠道与计费 profile（含 inherit）。
2. 默认分组固定存在，不可删除/改名，可改 profile/成员；无显式成员时=全部渠道。
3. 用户为 API Key 选择多个分组后，请求只由这些分组并集内的渠道服务。
4. 用户未选分组 → 落默认分组。
5. 用户侧任何视图都看不到上游渠道名/id，只看到分组 name/description。
6. 分组 `credit_billing_mode` 非空 → 按分组计费方式扣费；为空（inherit）→ 回落选中渠道计费方式。
7. 多分组并集选择 + retry 一致性（同计费 profile）正确。
8. SQLite/MySQL/PG 迁移与默认分组初始化成功。
9. 后端 targeted + 相关全包测试通过；前端 typecheck + i18n sync + 定向测试通过。
