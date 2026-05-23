# 全面移除业务分组概念实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。当前用户最新要求使用隔离 worktree 开发；实现工作树为 `C:/Users/34404/source/repos/new-api/.worktrees/remove-business-groups`，分支为 `worktree/remove-business-groups`。所有实现子代理必须收到完整规格/计划路径和 2000 字以上任务提示。子代理只改自己任务列出的文件，不运行项目级 build/lint/typecheck/i18n/build，不格式化全仓；主代理在批次后统一验证。

**目标：** 从产品、后端运行时、前端 UI/API 类型、OpenAPI、i18n 与配置中移除所有业务分组概念，包括渠道分组、token 分组、用户 vip/svip/default 分组、订阅升级分组、分组倍率、分组限流、分组亲和、日志/指标/分析 group 维度，同时保留非业务 `group` 概念。

**架构：** 第一阶段采用逻辑删除 + 兼容读取：保留历史 DB 列和跨库迁移安全性，但业务代码不再读取 group 值做决策，旧 payload/option/endpoint 通过 no-op shim 兼容。渠道能力从 `group + model + channel` 降级为 `model + channel`，计费倍率移除外部 group ratio 合同，订阅与充值不再更新用户分组。default 与 classic 前端同步删除业务 group UI、类型、i18n 文案，OpenAPI/README 与测试矩阵锁定无业务 group 合同。

**技术栈：** Go 1.22+、Gin、GORM、SQLite/MySQL/PostgreSQL；React 19/TypeScript/Rsbuild/Bun；go-i18n；node:test/tsx。

**规格：** `C:/Users/34404/source/repos/new-api/.worktrees/remove-business-groups/docs/superpowers/specs/2026-05-21-remove-ai-channel-groups-design.md`
**计划：** `C:/Users/34404/source/repos/new-api/.worktrees/remove-business-groups/docs/superpowers/plans/2026-05-21-remove-business-groups.md`

---

## 文件职责与批次边界

### 后端核心批次

- `controller/group.go`、`router/api-router.go`：旧 group endpoint no-op shim。
- `controller/option.go`、`model/option.go`、`setting/ratio_setting/group_ratio.go`、`setting/auto_group.go`、`setting/user_usable_group.go`、`common/topup-ratio.go`、`setting/rate_limit.go`：旧 group option no-op 与运行时分组配置退役。
- `controller/token.go`、`model/token.go`、`controller/token_test.go`：旧 token `group` / `cross_group_retry` payload 兼容忽略，响应不暴露业务 group。
- `model/ability.go`、`model/channel.go`、`model/channel_cache.go`、`model/channel_satisfy.go`、`controller/channel.go`、`controller/channel_upstream_update.go`、`service/channel_select.go`：无 group 渠道路由矩阵与渠道管理 API 兼容。
- `middleware/auth.go`、`middleware/distributor.go`、`middleware/model-rate-limit.go`、`relay/common/relay_info.go`、`relay/relay.go`、`controller/config_guide.go`、`controller/model.go`、`controller/user.go`、`controller/misc.go`、`controller/playground.go`、`dto/playground.go`、`service/channel_affinity.go`、`controller/channel_affinity_cache.go`、`setting/operation_setting/channel_affinity_setting.go`：请求上下文、模型列表、config guide、status、playground、模型限流与 channel affinity 不再依赖 group。
- `model/user.go`、`model/user_cache.go`、`controller/user.go`、`model/subscription.go`、`controller/subscription.go`、`model/redemption.go`、`controller/subscription_payment_balance.go`、`service/invitation_reward.go`：用户/订阅/兑换码分组退役。
- `relay/helper/price.go`、`types/price_data.go`、`service/log_info_generate.go`、`service/task_billing.go`、`service/quota.go`、`service/text_quota.go`、`service/violation_fee.go`、`controller/channel-test.go`、`model/task.go`、`dto/task.go`、`controller/task_video.go`、`relay/relay_task.go`、`controller/topup.go`、`controller/topup_stripe.go`、`controller/topup_waffo.go`、`controller/topup_waffo_pancake.go`、`controller/topup_creem.go`：计费、充值、任务 group ratio 与新日志写入退役。
- `controller/pricing.go`、`model/pricing.go`、`model/model_extra.go`、`model/model_meta.go`、`controller/model_meta.go`、`model/log.go`、`controller/log.go`、`model/perf_metric.go`、`controller/perf_metrics.go`、`pkg/perf_metrics/types.go`、`pkg/perf_metrics/metrics.go`、`pkg/perf_metrics/flush.go`、`dto/usage_analytics.go`、`model/usage_analytics.go`、`controller/usage_analytics.go`、`dto/admin_analytics.go`、`model/admin_analytics.go`、`model/admin_analytics_usage.go`、`model/admin_analytics_drilldown.go`、`model/admin_analytics_risk.go`、`controller/admin_analytics.go`：展示、日志、指标、分析 group 维度退役。

### 前端批次

- `web/default/src/features/keys/*`：API key 类型残留清理。
- `web/default/src/features/users/*`、`web/default/src/routes/_authenticated/users/index.tsx`、`web/default/src/components/profile-dropdown.tsx`、`web/default/src/components/layout/components/mobile-drawer.tsx`、`web/default/src/features/profile/components/profile-header.tsx`、`web/default/src/lib/api.ts`：用户 group UI/type/API 清理。
- 任务 6 文件清单中列出的 default 前端路径：渠道、模型、系统设置、订阅、钱包、playground、pricing、usage logs、performance metrics、usage analytics、admin analytics 的业务 group UI/type/API 清理。
- 任务 7 文件清单中列出的 classic 前端路径：token、用户、个人信息、渠道、设置、订阅、topup、playground、pricing、usage logs、models 的业务 group UI/type/API 清理。
- `web/default/src/i18n/static-keys.ts`、`web/default/src/i18n/locales/*.json`、`web/classic/src/i18n/locales/*.json`：业务 group i18n 清理，保留 allowlist 非业务 group。

### 文档与合约批次

- `docs/openapi/api.json`、README/配置说明、后端 `i18n/locales/*.yaml`：业务 group API/字段/文案删除，保留 `/api/prefill_group/`。

## 子代理执行顺序与冲突控制

- 串行执行任务 1 → 2 → 3 → 4 → 5，因为这些任务共享后端核心类型、函数签名和调用链。
- 任务 6 与任务 7 可以在任务 1-5 通过定向编译后并发执行；二者分别修改 default 与 classic，冲突较低。
- 任务 8 必须在任务 6/7 完成后执行，因为它会清理 i18n/static keys/OpenAPI，避免与前端任务争抢同一文案文件。
- 任务 9 只由主代理执行，负责统一验证、最终搜索、提交和最终并发审查。
- 任务 6、任务 7、任务 8 都会在 `web/default` 下新增或运行测试；实现子代理进入该目录前必须读取 `C:/Users/34404/source/repos/new-api/.worktrees/remove-business-groups/web/default/AGENTS.md` 并遵守 Bun、类型与 i18n 规则。
- 每个实现子代理只修改任务列出的文件；如果发现必须触碰未列出的文件，先停止并向主代理说明。
- 每个实现任务完成后必须至少进行规格合规审查和代码质量审查；审查失败则由实现子代理修复并复审。任务 9 还要额外并发派发至少 3 个最终只读 reviewer。

---

## 任务 1：后端兼容 shim 与配置 no-op

**文件：**
- 修改：`controller/group.go`
- 修改：`router/api-router.go`
- 修改：`controller/option.go`
- 修改：`model/option.go`
- 修改：`constant/context_key.go`
- 修改：`constant/cache_key.go`
- 测试：`controller/group_compat_test.go`（新建）
- 测试：`controller/option_group_compat_test.go`（新建或并入现有 option 测试）

测试 helper 必须自包含：`setupOptionTestDB`、`newAuthenticatedContext` 等 helper 要么复用同包现有 helper，要么在新测试文件中定义；不得引用未定义 helper。

- [ ] **步骤 1：编写旧 group endpoint no-op 测试**

在 `controller/group_compat_test.go` 添加测试，覆盖 `/api/group`、`/api/group/`、`/api/user/groups`、`/api/user/self/groups` 的控制器行为。若路由测试难以复用现有 router，可直接调用 `GetGroups` / `GetUserGroups` 并补一条 router 路径断言。

```go
func TestGroupEndpointsReturnNoopCompatibilityPayloads(t *testing.T) {
    setupOptionTestDB(t)

    groupCtx, groupRecorder := newAuthenticatedContext(t, http.MethodGet, "/api/group/", nil, 1)
    GetGroups(groupCtx)
    var groupResp struct {
        Success bool     `json:"success"`
        Data    []string `json:"data"`
    }
    require.NoError(t, common.Unmarshal(groupRecorder.Body.Bytes(), &groupResp))
    require.True(t, groupResp.Success)
    require.Empty(t, groupResp.Data)

    userCtx, userRecorder := newAuthenticatedContext(t, http.MethodGet, "/api/user/self/groups", nil, 1)
    GetUserGroups(userCtx)
    var userResp struct {
        Success bool                   `json:"success"`
        Data    map[string]interface{} `json:"data"`
    }
    require.NoError(t, common.Unmarshal(userRecorder.Body.Bytes(), &userResp))
    require.True(t, userResp.Success)
    require.Empty(t, userResp.Data)
}
```

- [ ] **步骤 2：编写旧 group option no-op 测试**

覆盖 group option 写入返回成功但不改变运行时状态。至少覆盖：`GroupRatio`、`GroupGroupRatio`、`AutoGroups`、`DefaultUseAutoGroup`、`UserUsableGroups`、`TopupGroupRatio`、`ModelRequestRateLimitGroup`、`group_ratio_setting.group_special_usable_group`。

```go
func TestGroupOptionsAreAcceptedAsNoopCompatibilityWrites(t *testing.T) {
    setupOptionTestDB(t)
    originalGroupRatio := ratio_setting.GroupRatio2JSONString()
    originalAutoGroups := setting.AutoGroups2JsonString()
    originalTopupRatio := common.TopupGroupRatio2JSONString()
    originalGroupGroupRatio := ratio_setting.GroupGroupRatio2JSONString()
    originalDefaultUseAutoGroup := setting.DefaultUseAutoGroup
    originalUserUsableGroups := setting.UserUsableGroups2JSONString()
    originalModelRateLimitGroup := setting.ModelRequestRateLimitGroup2JSONString()
    originalSpecialUsable := ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup.String()
    t.Cleanup(func() {
        require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(originalGroupRatio))
        require.NoError(t, setting.UpdateAutoGroupsByJsonString(originalAutoGroups))
        require.NoError(t, common.UpdateTopupGroupRatioByJSONString(originalTopupRatio))
        require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(originalGroupGroupRatio))
        require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(originalUserUsableGroups))
        require.NoError(t, setting.UpdateModelRequestRateLimitGroupByJSONString(originalModelRateLimitGroup))
        setting.DefaultUseAutoGroup = originalDefaultUseAutoGroup
    })

    for _, tc := range []struct{ key, value string }{
        {"GroupRatio", `{"vip":9}`},
        {"GroupGroupRatio", `{"vip":{"svip":9}}`},
        {"AutoGroups", `["vip"]`},
        {"DefaultUseAutoGroup", `true`},
        {"UserUsableGroups", `{"vip":"VIP"}`},
        {"TopupGroupRatio", `{"vip":9}`},
        {"ModelRequestRateLimitGroup", `{"vip":{"gpt":1}}`},
        {"group_ratio_setting.group_special_usable_group", `{"vip":{"special":"Special"}}`},
    } {
        require.NoError(t, model.UpdateOption(tc.key, tc.value), tc.key)
        require.NoError(t, updateOptionThroughControllerForGroupRemovalTest(t, tc.key, tc.value), tc.key)
    }

    require.JSONEq(t, originalGroupRatio, ratio_setting.GroupRatio2JSONString())
    require.JSONEq(t, originalAutoGroups, setting.AutoGroups2JsonString())
    require.JSONEq(t, originalTopupRatio, common.TopupGroupRatio2JSONString())
    require.JSONEq(t, originalGroupGroupRatio, ratio_setting.GroupGroupRatio2JSONString())
    require.Equal(t, originalDefaultUseAutoGroup, setting.DefaultUseAutoGroup)
    require.JSONEq(t, originalUserUsableGroups, setting.UserUsableGroups2JSONString())
    require.JSONEq(t, originalModelRateLimitGroup, setting.ModelRequestRateLimitGroup2JSONString())
    require.Equal(t, originalSpecialUsable, ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup.String())
}
```

- [ ] **步骤 3：运行测试确认失败**

运行：

```bash
go test ./controller -run 'TestGroupEndpointsReturnNoopCompatibilityPayloads|TestGroupOptionsAreAcceptedAsNoopCompatibilityWrites' -count=1
```

预期：FAIL，现有 group endpoint 返回真实分组，旧 option 会更新运行时配置或触发旧校验。

- [ ] **步骤 4：实现 no-op endpoint**

修改 `controller/group.go`：

```go
func GetGroups(c *gin.Context) {
    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "message": "",
        "data":    []string{},
    })
}

func GetUserGroups(c *gin.Context) {
    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "message": "",
        "data":    map[string]interface{}{},
    })
}
```

保留路由，不删除 `router/api-router.go` 里的 group 路径；如果 `/api/group` 无尾斜杠当前不可达，在 router 中补兼容路由到同一 handler。

- [ ] **步骤 5：实现 group option no-op**

在 `model/option.go` 新增 helper：

```go
func isDeprecatedBusinessGroupOption(key string) bool {
    switch key {
    case "GroupRatio", "GroupGroupRatio", "AutoGroups", "DefaultUseAutoGroup", "UserUsableGroups", "TopupGroupRatio", "ModelRequestRateLimitGroup", "group_ratio_setting.group_special_usable_group":
        return true
    default:
        return strings.HasPrefix(key, "group_ratio_setting.")
    }
}
```

在 `updateOptionMap` 写入 `common.OptionMap[key] = value` 后、进入 `handleConfigUpdate` / legacy switch 前：

```go
if isDeprecatedBusinessGroupOption(key) {
    return nil
}
```

在 `controller/option.go` 的 group option 校验分支中同步跳过旧 group option 校验，确保旧管理端保存返回成功；测试中必须通过 controller 更新路径覆盖一次，不能只直接调用 `model.UpdateOption`。
如果 `isDeprecatedBusinessGroupOption` 保持在 `model` 包内不可导出，`controller/option.go` 必须定义本地等价 helper 或改为导出共享 helper；不得通过字符串散落实现两套不一致列表。
`constant/context_key.go` / `constant/cache_key.go`：旧 ContextKey*Group、UserGroupKeyFmt、TokenFieldGroup 等若调用点全部清理后不再需要，应删除；若仍被 legacy shim 编译引用，只保留最小兼容常量并在最终搜索中证明不会驱动业务分组。

- [ ] **步骤 6：运行测试验证通过**

运行：

```bash
go test ./controller -run 'TestGroupEndpointsReturnNoopCompatibilityPayloads|TestGroupOptionsAreAcceptedAsNoopCompatibilityWrites' -count=1
```

预期：PASS。

---

## 任务 2：后端渠道能力与路由去分组

**文件：**
- 修改：`model/ability.go`
- 修改：`model/channel.go`
- 修改：`model/channel_cache.go`
- 修改：`model/channel_satisfy.go`
- 修改：`service/channel_select.go`
- 修改：`controller/channel.go`
- 修改：`controller/channel_upstream_update.go`
- 修改：`controller/token.go`
- 修改：`model/token.go`
- 测试：`controller/token_test.go`
- 修改：`controller/relay.go`
- 修改：`controller/relay-text.go`
- 测试：`model/channel_group_removal_test.go`（新建或扩展现有 channel/ability 测试）
- 测试：`controller/channel_group_removal_test.go`（新建或扩展 channel controller 测试）

- [ ] **步骤 1：编写无分组 ability 与 routing 测试**

测试应证明：同一 channel 即使旧 `Group` 中有 `default,vip`，更新能力后只按模型生成一条有效 ability；查询按 model/priority/weight 选出渠道，不需要 group；旧 group 数据不会重复命中。
测试文件必须自包含地提供测试 DB helper；可复用同包已有 `setup*TestDB`，若不存在则在 `model/channel_group_removal_test.go` 中创建 SQLite 内存 DB、设置 `model.DB`、迁移 `Channel` 和 `Ability`。不得引用未定义 helper。
后端 token 兼容测试同步更新：`controller/token_test.go` 中新增/调整测试，旧客户端提交 `group` / `cross_group_retry` 时创建/更新返回成功但忽略业务状态，列表/详情/更新响应不暴露这些字段；旧的“显式更新 group 生效”测试删除或改为“显式旧字段被忽略”。
控制器测试必须覆盖：旧 `group` 查询被忽略且不影响 channel/tag 搜索结果；tag edit `groups` payload 不写入业务状态；upstream update 不再允许或传播 `group` 字段；Add/Update channel 响应不暴露 group，且旧 payload 不更新业务状态。
同时显式 seed 同一 `model + channel_id` 的多条 legacy ability（例如 `Group=default` 与 `Group=vip`），验证新查询按 `model + channel_id` 去重，不依赖 group，也不会因旧复合主键数据重复命中。

```go
func TestChannelAbilitiesIgnoreBusinessGroups(t *testing.T) {
    db := setupChannelGroupRemovalTestDB(t)
    channel := &model.Channel{Id: 1001, Type: constant.ChannelTypeOpenAI, Key: "sk-test", Status: common.ChannelStatusEnabled, Name: "groupless", Models: "gpt-test", Group: "default,vip"}
    require.NoError(t, db.Create(channel).Error)
    channel.Keys = []string{"sk-test"}
    require.NoError(t, channel.UpdateAbilities(nil))

    var abilities []model.Ability
    require.NoError(t, db.Where("channel_id = ? AND model = ?", channel.Id, "gpt-test").Find(&abilities).Error)
    require.Len(t, abilities, 1)

    selected, err := model.GetRandomSatisfiedChannel("gpt-test", 0)
    require.NoError(t, err)
    require.NotNil(t, selected)
    require.Equal(t, channel.Id, selected.Id)
}
```

- [ ] **步骤 2：运行测试确认失败**

运行：

```bash
go test ./model -run 'TestChannelAbilitiesIgnoreBusinessGroups' -count=1
```

预期：FAIL，现有 `UpdateAbilities` 会按 group 拆多条，选择函数仍需要 group。

- [ ] **步骤 3：调整 Ability 写入和查询 API**

在 `model/ability.go` 保留 `Ability.Group` legacy 字段，但新增固定兼容值：

```go
const legacyAbilityGroup = ""
```

新增/改造函数：

```go
func GetEnabledModels() []string
func GetChannel(model string, retry int) (*Channel, error)
func GetRandomSatisfiedChannel(model string, retry int) (*Channel, error)
```

将旧 `getPriority(group, model, retry)` 改为 `getPriority(model, retry)`，where 条件移除 group。查询 ability 时按 model/channel_id 去重；如 DB 不易 distinct 多列，先查询 abilities 后在 Go 中按 channel_id 去重。

`AddAbilities` / `UpdateAbilities` 不再 split `channel.Group`，仅对 `channel.Models` 生成 ability：

```go
ability := Ability{
    Group:     legacyAbilityGroup,
    Model:     modelName,
    ChannelId: channel.Id,
    Enabled:   channel.Status == common.ChannelStatusEnabled,
    Priority:  channel.Priority,
    Weight:    uint(channel.GetWeight()),
    Tag:       channel.Tag,
}
```

- [ ] **步骤 4：调整 channel cache / satisfy**

`model/channel_cache.go` 将缓存结构从 group->model->channels 改为 model->channels；对外旧签名如暂时保留 group 参数，则忽略该参数并转调新函数。`model/channel_satisfy.go` 不再判断 group，仅按 model/status/其他已有非分组条件判断。补充缓存测试：开启 `common.MemoryCacheEnabled` 后初始化 channel cache，验证旧签名 `IsChannelEnabledForGroupModel` / `IsChannelEnabledForAnyGroupModel` 如保留也忽略 group，禁用渠道会从 model->channels 缓存移除。

- [ ] **步骤 5：调整 service channel select**

`service/channel_select.go` 中 `RetryParam` 删除或忽略 `TokenGroup`；`CacheGetRandomSatisfiedChannel` 不再处理 `auto` / cross-group retry，直接：

```go
channel, err := model.GetRandomSatisfiedChannel(param.ModelName, param.GetRetry())
return channel, "", err
```

如调用方仍需要第二返回值，返回空字符串或移除字段需与后续任务协调。

`controller/channel.go`、`controller/channel_upstream_update.go`：旧 `group` / `groups` 查询和 payload 兼容忽略；`SearchChannels` / `SearchTags` 不按 group 过滤；`ChannelTag.Groups` 不再传入业务更新；Add/Update channel 不保存请求中的业务 group；upstream update 不允许继续把 group 作为可更新业务字段；响应不暴露业务 group。旧客户端提交这些字段必须返回成功或原有非分组校验错误，但不能更新业务状态。

`controller/token.go`、`model/token.go`：移除 `defaultTokenGroup()` 业务路径；AddToken/UpdateToken 忽略旧 payload 中的 `group` / `cross_group_retry`；响应 DTO 或 JSON 不暴露 `group` / `cross_group_retry`。legacy DB 列保留但不参与新业务决策。

`controller/relay.go`、`controller/relay-text.go`：不再读取或设置 token/user/group 上下文；`ContextKeyUsingGroup` 相关写入删除，relay 只根据 token、model、channel 等非分组条件执行。

- [ ] **步骤 6：运行定向测试**

运行：

```bash
go test ./model -run 'TestChannelAbilitiesIgnoreBusinessGroups' -count=1
go test ./service -run Channel -count=1
```

预期：PASS 或 service 无匹配测试；编译错误必须修复到相关包通过。

---

## 任务 3：后端 auth / relay / model list / config guide 去分组

**文件：**
- 修改：`middleware/auth.go`
- 修改：`middleware/distributor.go`
- 修改：`middleware/model-rate-limit.go`
- 修改：`relay/common/relay_info.go`
- 修改：`relay/relay.go`
- 修改：`relay/relay-image.go`
- 修改：`relay/relay-mj.go`
- 修改：`controller/model.go`
- 修改：`controller/config_guide.go`
- 修改：`controller/user.go`
- 修改：`controller/misc.go`
- 修改：`controller/playground.go`
- 修改：`dto/playground.go`
- 修改：`service/channel_affinity.go`
- 修改：`controller/channel_affinity_cache.go`
- 修改：`setting/operation_setting/channel_affinity_setting.go`
- 测试：`controller/model_list_test.go`
- 测试：`controller/config_guide_test.go`

- [ ] **步骤 1：编写 model list / config guide 无分组测试**

更新或新增测试，证明用户/token/channel 的旧 group 值为 `gone` 也不导致模型列表/config guide 403，只要 token/user 状态、quota、model limits、channel ability 非分组条件满足。

```go
func TestModelListIgnoresUserAndTokenGroups(t *testing.T) {
    db := setupModelListTestDB(t)
    require.NoError(t, db.Create(&model.User{Id: 901, Username: "groupless", Password: "password", Group: "gone", Status: common.UserStatusEnabled, AffCode: "groupless"}).Error)
    require.NoError(t, db.Create(&model.Token{UserId: 901, Name: "token", Key: "sk-groupless", Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true, Group: "gone"}).Error)
    seedEnabledAbilityWithoutBusinessGroupForModelListTest(t, db, "gpt-groupless", 1001)

    recorder := httptest.NewRecorder()
    c, _ := gin.CreateTestContext(recorder)
    c.Set("id", 901)
    c.Set("token_id", 1)
    callModelListHandlerUsedByExistingTests(c)

    require.Equal(t, http.StatusOK, recorder.Code)
    require.Contains(t, recorder.Body.String(), "gpt-groupless")
}
```

测试 helper 要么复用 `controller/model_list_test.go` 和 `controller/config_guide_test.go` 已有 helper，要么在相同测试文件中定义；计划中的 `seedEnabledAbilityWithoutBusinessGroupForModelListTest` 和 `callModelListHandlerUsedByExistingTests` 是需要实现的测试辅助函数名，不是外部依赖。

- [ ] **步骤 2：运行测试确认失败**

运行：

```bash
go test ./controller -run 'TestModelListIgnoresUserAndTokenGroups|TestConfigGuide.*Group' -count=1
```

预期：FAIL，当前 group 校验或 `GetGroupEnabledModels` 会失败。

- [ ] **步骤 3：移除 auth/context group 注入与校验**

在 `middleware/auth.go`：
- `SetupContextForToken` 不再校验 token group 是否在用户可用组。
- 不再设置 `ContextKeyTokenGroup`、`ContextKeyTokenCrossGroupRetry`、`ContextKeyUsingGroup`、`ContextKeyUserGroup` 用于业务逻辑。
- 保留用户 ID、token ID、token quota、model limits、allow_ips 等非分组 context。

在 `middleware/model-rate-limit.go` / `setting/rate_limit.go`：旧 `ModelRequestRateLimitGroup` option 仅兼容保存，不再按 `ContextKeyTokenGroup` / `ContextKeyUserGroup` 应用 group 限流。

`relay/relay-image.go`、`relay/relay-mj.go` 与 `relay/relay.go` 保持一致：不再从 context/meta 读取业务 group；MJ/图片等非文本 relay 的 quota/log meta 不写 group。

在 `controller/misc.go:GetStatus`：响应不再返回 `default_use_auto_group`。

在 `controller/playground.go` / `dto/playground.go`：旧 playground request `group` 字段兼容忽略；不再构造 `playground-<group>` 临时 token 名称或写 token Group。

在 `service/channel_affinity.go`、`controller/channel_affinity_cache.go`、`setting/operation_setting/channel_affinity_setting.go`：`include_using_group` 兼容忽略；缓存 key、admin_info、usage stats、preferred-channel 查询和 mark-used 都不再包含 `using_group`。

在 `middleware/distributor.go`：
- playground/group、auto group、using group 相关分支删除或忽略。
- channel affinity 调用不传 using group。

- [ ] **步骤 4：调整 RelayInfo**

在 `relay/common/relay_info.go`：
- 移除或 legacy ignore `TokenGroup`、`UsingGroup`、`UserGroup` 字段的业务赋值。
- `InitChannelMeta` 不再写 group。
- 任何需要日志名称的地方不要构造 `playground-<group>`。

在 `relay/relay.go`：
- `RetryParam` 调用不传 TokenGroup。
- 记录日志时 group 字段传空或删除参数（与任务 6 协调）。

- [ ] **步骤 5：调整模型列表与 config guide**

`controller/model.go`、`controller/user.go:GetUserModels`、`controller/config_guide.go`：
- 使用 `model.GetEnabledModels()` 或新的无 group enabled-model helper。
- 不再调用 `model.GetUserGroup`、`service.GetUserUsableGroups`、`service.GetUserAutoGroup`、`model.GetGroupEnabledModels`。
- config guide token usability 不再因 token/user group 不存在返回 403。

- [ ] **步骤 6：运行定向测试**

运行：

```bash
go test ./controller -run 'TestModelList|TestConfigGuide|TestGetOpenCodeOpenAIModels' -count=1
```

预期：PASS。

---

## 任务 4：后端用户、订阅、兑换码与充值去分组

**文件：**
- 修改：`model/user.go`
- 修改：`model/user_cache.go`
- 修改：`controller/user.go`
- 修改：`service/group.go`
- 修改：`model/subscription.go`
- 修改：`controller/subscription.go`
- 修改：`controller/subscription_payment_balance.go`
- 修改：`controller/subscription_payment_stripe.go`
- 修改：`controller/subscription_payment_epay.go`
- 修改：`controller/subscription_payment_creem.go`
- 修改：`controller/subscription_payment_completion.go`
- 修改：`service/invitation_reward.go`
- 修改：`model/redemption.go`
- 修改：`controller/topup.go`
- 修改：`controller/topup_stripe.go`
- 修改：`controller/topup_waffo.go`
- 修改：`controller/topup_waffo_pancake.go`
- 修改：`controller/topup_creem.go`
- 修改：`common/topup-ratio.go`
- 测试：`controller/subscription_group_removal_test.go`（新建或扩展 subscription 测试）
- 测试：`controller/topup_group_ratio_test.go`（新建或扩展 topup 测试）
- 测试：`controller/user_group_removal_test.go`（新建或扩展现有 user 测试）

测试 helper 必须自包含：`setupSubscriptionControllerTestDB`、`seedSubscriptionUser`、`seedSubscriptionPlan` 等要么复用同包现有 helper，要么在新测试文件中定义；不得引用未定义 helper。
用户测试必须覆盖：创建/编辑用户旧 payload 中的 `group` 兼容忽略且不写业务状态；搜索 query `group` / `user_group` 被忽略且不影响结果；用户详情、列表、登录响应不暴露业务 group；`GetUserModels` 不依赖 user group。`service/group.go` 的 `GetUserUsableGroups` / `GetUserAutoGroup` / `GetUserGroupRatio` / `GroupInUserUsableGroups` 删除或变成固定 no-op 兼容 helper，不能继续读取 ratio_setting/auto/user usable group。

- [ ] **步骤 1：编写订阅不更新用户分组测试**

```go
func TestSubscriptionPurchaseDoesNotUpdateUserGroup(t *testing.T) {
    db := setupSubscriptionControllerTestDB(t)
    user := seedSubscriptionUser(t, db, 101, "vip")
    plan := seedSubscriptionPlan(t, db, model.SubscriptionPlan{Title: "No group", UpgradeGroup: "svip", TotalAmount: 100})

    _, _, err := createBalanceSubscriptionOrder(101, plan, "trade-no-group", plan.PriceAmount)
    require.NoError(t, err)

    var updated model.User
    require.NoError(t, db.First(&updated, user.Id).Error)
    require.Equal(t, "vip", updated.Group)
}
```

- [ ] **步骤 2：编写充值金额不按用户分组测试**

```go
func TestTopupAmountIgnoresGroupRatio(t *testing.T) {
    original := common.TopupGroupRatio2JSONString()
    t.Cleanup(func() { require.NoError(t, common.UpdateTopupGroupRatioByJSONString(original)) })
    require.NoError(t, common.UpdateTopupGroupRatioByJSONString(`{"vip":9,"default":1}`))

    vipAmount := getPayMoney(10, "vip")
    defaultAmount := getPayMoney(10, "default")
    require.Equal(t, defaultAmount, vipAmount)
}
```

- [ ] **步骤 3：运行测试确认失败**

运行：

```bash
go test ./controller -run 'TestSubscriptionPurchaseDoesNotUpdateUserGroup|TestTopupAmountIgnoresGroupRatio' -count=1
```
订阅支付测试还应覆盖 balance、stripe、epay、creem、completion 最终完成路径：旧 `upgrade_group` / `prev_user_group` 不改变用户 group、不写新的订阅 group 响应字段，第三方回调最终走 `model.CompleteSubscriptionOrder` 时也不回退/升级用户 group。

预期：FAIL，当前购买订阅会更新 group，充值按 group ratio。

- [ ] **步骤 4：用户模型兼容字段退役**

`model/user.go`：保留 `Group` legacy 字段用于旧 DB，但 `ToBaseUser` 不再填业务 Group；`SearchUsers` 忽略 group 过滤；`Edit` / `Update` 不再写 group。

`model/user_cache.go`：`UserBase.Group` 如果暂时保留，标记 legacy，`WriteContext` 不再写业务 group；`UpdateUserGroupCache` 调用点移除或变成 no-op 兼容。

`controller/user.go`：创建/编辑/搜索/详情/登录响应不暴露 group；注册默认 token 不再根据 `DefaultUseAutoGroup` 写 auto group。

- [ ] **步骤 5：订阅、兑换码与 loadtest seed 去分组**

`controller/subscription.go`：创建/更新 plan 忽略 `UpgradeGroup`，不校验 group ratio；`GetSubscriptionPlans` 等响应不暴露 `upgrade_group`。

`model/subscription.go`：`CreateUserSubscriptionFromPlanTx`、`ExpireDueSubscriptions`、downgrade 相关函数不写/不回退用户 group；保留旧列但不读写；`PublicUserSubscription` 不输出 `upgrade_group` / `prev_user_group`。

`service/invitation_reward.go` 不再把 `plan.UpgradeGroup` 写入 `UserSubscription.UpgradeGroup`。

`controller/subscription_payment_balance.go`、其他支付回调路径、`model/redemption.go`：删除 `UpdateUserGroupCache` 调用。

`pkg/loadtest/config/config.go`、`pkg/loadtest/config/config_test.go`、`pkg/loadtest/seed/seed.go`、`pkg/loadtest/seed/seed_test.go`、`pkg/loadtest/artifact/artifact.go`、`pkg/loadtest/artifact/artifact_test.go`、`cmd/loadtest-seed/main.go`、`config.loadtest.yaml`：loadtest 配置与 seed 不再要求或输出业务 `group`；不再写 `GroupRatio` option，不再用 group 隔离 route；旧配置中的 `loadtest.group` 兼容忽略，`artifact.SeedOutput` 删除或 legacy-ignore `Group json:"group"`。

- [ ] **步骤 6：充值去 group ratio**

`controller/topup.go`、`controller/topup_stripe.go`、`controller/topup_waffo.go`、`controller/topup_waffo_pancake.go`、`controller/topup_creem.go`：`getPayMoney`、`getStripePayMoney`、Waffo/Pancake 金额计算不读取 `model.GetUserGroup` 或 `common.GetTopupGroupRatio`。函数签名如保留 group 参数，参数不使用并命名 `_ string`。
同时覆盖 `getStripePayMoney`、`RequestPay/GetChargedAmount`、Waffo、Waffo-Pancake 每条金额路径；所有路径都不得按用户 group 或 `TopupGroupRatio` 改金额。
`controller/topup_group_ratio_test.go` 必须新增具体断言覆盖：`getPayMoney`、`getStripePayMoney`、Creem/Stripe/Epay charged amount helper、Waffo `RequestPay` / `GetChargedAmount`、Waffo-Pancake 金额路径在用户旧 group 为 `vip` 且 `TopupGroupRatio={"vip":9}` 时仍按倍率 1 计算；如路径依赖外部签名，使用本包 helper 构造 request，不引入网络 mock。
`controller/topup_creem.go` 当前无业务 group ratio 时可只做审计说明；若新增支付金额逻辑，不得读取用户 group。

`common/topup-ratio.go` 可保留旧 option 兼容函数，但运行时返回 1 或 no-op。

- [ ] **步骤 7：运行定向测试**

运行：

```bash
go test ./controller -run 'TestSubscription|TestTopup|TestWaffo|TestPancake' -count=1
go test ./model -run 'TestSubscription|TestRedemption' -count=1
go test ./pkg/loadtest/config ./pkg/loadtest/seed ./pkg/loadtest/artifact ./cmd/loadtest-seed -count=1
go test ./controller -run 'TestUser.*Group|TestUserGroupRemoval|TestSearchUsersIgnoresGroup|TestLoginResponseOmitsGroup|TestGetUserModelsIgnoresGroup' -count=1
```

预期：PASS。

---

## 任务 5：后端计费、任务、价格、日志、指标、分析去分组

**文件：**
- 修改：`relay/helper/price.go`
- 修改：`service/log_info_generate.go`
- 修改：`service/task_billing.go`
- 修改：`service/quota.go`
- 修改：`service/text_quota.go`
- 修改：`service/violation_fee.go`
- 修改：`controller/channel-test.go`
- 修改：`relay/mjproxy_handler.go`
- 修改：`model/task.go`
- 修改：`dto/task.go`
- 修改：`types/price_data.go`
- 修改：`controller/task_video.go`
- 修改：`relay/relay_task.go`
- 修改：`controller/pricing.go`
- 修改：`model/pricing.go`
- 修改：`model/model_extra.go`
- 修改：`model/model_meta.go`
- 修改：`controller/model_meta.go`
- 修改：`model/log.go`
- 修改：`controller/log.go`
- 修改：`model/perf_metric.go`
- 修改：`controller/perf_metrics.go`
- 修改：`pkg/perf_metrics/types.go`
- 修改：`pkg/perf_metrics/metrics.go`
- 修改：`pkg/perf_metrics/flush.go`
- 修改：`types/request_meta.go`
- 修改：`dto/usage_analytics.go`
- 修改：`model/usage_analytics.go`
- 修改：`controller/usage_analytics.go`
- 修改：`dto/admin_analytics.go`
- 修改：`model/admin_analytics.go`
- 修改：`model/admin_analytics_usage.go`
- 修改：`model/admin_analytics_drilldown.go`
- 修改：`model/admin_analytics_risk.go`
- 修改：`controller/admin_analytics.go`
- 测试：`controller/pricing_directory_test.go`、`service/log_info_generate_test.go`、`service/task_billing_test.go`、`controller/usage_analytics_test.go`、`model/usage_analytics_test.go`、`controller/admin_analytics_test.go`、`model/admin_analytics_test.go`、`model/admin_analytics_usage_test.go`、`model/perf_metric_test.go`
- 测试：`model/task_group_removal_test.go`（新建或扩展任务/模型计费测试）
- 测试：`model/pricing_group_removal_test.go`（新建或扩展价格倍率测试）
- 测试：`model/log_group_removal_test.go`（新建或扩展日志/分析测试）
- 测试：`pkg/perf_metrics/group_removal_test.go`（新建或扩展指标测试）
- 测试：`relay/relay_task_subscription_billing_test.go`（扩展 task 创建/响应 group 退役断言）

- [ ] **步骤 1：编写 pricing/log other 无 group ratio 测试**

扩展 `controller/pricing_directory_test.go`：管理员和普通用户响应都不包含 `group_ratio`、`auto_groups`、`enable_groups`。

扩展 `service/log_info_generate_test.go`：`GenerateTextOtherInfo` 不输出 `group_ratio` / `user_group_ratio` / `group_group_ratio`。
扩展 `model/pricing_group_removal_test.go` 或现有 pricing 测试：`GroupRatio` / `GroupGroupRatio` legacy option 不影响模型倍率、文本 quota、violation fee 与 channel-test 费用计算；`types.PriceData` 不再公开 `GroupRatioInfo` 业务语义。

- [ ] **步骤 2：编写 task billing 不读 group 测试**

在 `service/task_billing_test.go` 添加：旧 task.Group/user.Group 为 `vip` 且 group ratio 为 9 时，重算结果仍按倍率 1；同时断言任务日志不写业务 group。MJ/Midjourney 日志测试或源码可见性断言必须覆盖 `relay/mjproxy_handler.go` 不再输出 `分组倍率`、不读取 `priceData.GroupRatioInfo.GroupRatio`，也不写 `Group: info.UsingGroup` / `Group: relayInfo.UsingGroup`。
同时覆盖 `model/task.go` / `dto/task.go` / `controller/task_video.go` / `relay/relay_task.go`：旧 task.Group 可兼容读历史行，但新建、更新、返回、计费上下文都不写或不暴露业务 group。
`relay/relay_task_subscription_billing_test.go` 或等价 relay 任务测试必须覆盖 `TaskModel2Dto` / `RelayTaskFetch` / 新任务创建：旧 `Task.Group` 可存在于 DB，但返回 DTO 不包含 `group`，新建 task 不写 `relayInfo.UsingGroup`。

- [ ] **步骤 3：编写 analytics group 兼容测试**

用户侧：`group_by=group` 返回既定 400 或回退默认；`groups` filter 忽略且不 500。管理端：`user_group` / `request_group` group-by 和 filters 同样按既定行为测试。计划执行时必须在任务内固定每个端点行为，推荐：业务 group-by 返回 400，业务 group filters 忽略。
后台性能指标测试必须覆盖 `pkg/perf_metrics`：Sample/QueryParams/GroupResult/bucketKey 不再公开或按业务 group 聚合；旧 group query 兼容忽略，响应结构不返回 `groups`。

- [ ] **步骤 4：运行测试确认失败**

运行：

```bash
go test ./controller -run 'TestGetPricing|TestUsageAnalytics|TestAdminAnalytics|TestPerfMetrics|TestLog' -count=1
go test ./service -run 'TestGenerateTextOtherInfo|TestTaskBilling' -count=1
go test ./model -run 'TestTask|TestPricing|TestLog|TestPerfMetric|TestAdminAnalytics|TestUsageAnalytics' -count=1
go test ./pkg/perf_metrics -run Group -count=1
go test ./relay -run 'Test.*Task.*Group|TestRelayTask|TestTaskModel2Dto' -count=1
```

预期：FAIL，现有响应/日志/analytics 仍含 group。

- [ ] **步骤 5：计费结构去 group ratio 外部暴露**

`relay/helper/price.go`：`HandleGroupRatio` 删除业务逻辑或固定 1；外部错误文案移除 “Group & Model Pricing”。

`types/price_data.go`：如保留内部 `GroupRatio` 字段，仅允许作为固定 1 的内部兼容值；`ToSetting`、调试日志、other 信息、OpenAPI 和前端类型都不得输出 `group_ratio` / `user_group_ratio` / `group_group_ratio`。

`service/log_info_generate.go`：删除 `group_ratio`、`user_group_ratio`、`group_special_ratio` 注入。

`service/quota.go`、`service/text_quota.go`、`service/violation_fee.go`、`controller/channel-test.go`：扣费、违规扣费、text/audio/wss/channel-test 新日志写入不再读取 `relayInfo.PriceData.GroupRatioInfo` 做业务决策，也不向 `RecordConsumeLog` / error log 写业务 Group。

保留模型倍率、补全倍率、缓存倍率、模型价格、表达式计费。

- [ ] **步骤 6：任务去 group**

`model/task.go` 保留 legacy Group 字段但新任务不写；`TaskBillingContext.GroupRatio` 删除或固定 1。

`service/task_billing.go` 不回读 `User.Group` 或 ratio_setting group ratio；任务日志不写 group。

`relay/mjproxy_handler.go`：MJ 消费日志、补扣/退费和 other 信息不再输出或依赖 group ratio，不写业务 Group；保留非分组价格、补全/模型/表达式计费信息。

`dto/task.go`、`controller/task_video.go`、`relay/relay_task.go` 响应不输出业务 group。

- [ ] **步骤 7：pricing/model meta 去 group**

`controller/pricing.go` 不再返回 `group_ratio`、`auto_groups`，不按 usable group 过滤 pricing；管理员和普通用户响应都不能暴露 `enable_groups`。

`model/pricing.go`、`model/model_extra.go`、`model/model_meta.go`、`controller/model_meta.go` 删除或内部化 `EnableGroup` / `GetModelEnableGroups`；不再从 `Ability.Group` 构建 pricing/model meta 输出。

- [ ] **步骤 8：logs/perf/analytics 去 group**

`controller/log.go` 忽略 group query；`model/log.go` 的 filters 不再应用 Group；`RecordConsumeLog`、`RecordErrorLog`、`RecordTaskBillingLog` 新写入 Group 固定空值或不对外暴露。
`types/request_meta.go`：删除 `UserUsingGroup json:"user_using_group"` 或改为内部 legacy 忽略字段；不得继续作为日志、指标、analytics 或 API contract 输出。

`model/perf_metric.go`、`pkg/perf_metrics/types.go`、`pkg/perf_metrics/metrics.go`、`pkg/perf_metrics/flush.go` 聚合唯一键改业务逻辑为 model + bucket；若旧 DB 仍有 group 列，写入固定空字符串，不暴露/不过滤。

`dto/usage_analytics.go` 删除业务 `UsageAnalyticsGroupByGroup` 和 `Groups` filter；保留通用 group_by 机制。

`model/usage_analytics.go`、`controller/usage_analytics.go` 对 `group_by=group` 返回 400 或回退默认，按步骤 3 固定。

`dto/admin_analytics.go`、`model/admin_analytics.go`、`model/admin_analytics_usage.go`、`model/admin_analytics_drilldown.go`、`model/admin_analytics_risk.go`、`controller/admin_analytics.go` 删除或忽略 `AdminAnalyticsQuery.UserGroups` / `RequestGroups`、`AdminAnalyticsDrilldownFilter.UserGroup`、`AdminUsageGroupByUserGroup` / `AdminUsageGroupByRequestGroup` 等 filters、group-by 和输出；旧 filter 忽略，旧 group_by 推荐返回 400。

- [ ] **步骤 9：运行定向测试**

运行：

```bash
go test ./controller -run 'TestGetPricing|TestUsageAnalytics|TestAdminAnalytics|TestPerfMetrics|TestLog' -count=1
go test ./service -run 'TestGenerateTextOtherInfo|TestTaskBilling' -count=1
go test ./model -run 'TestTask|TestPricing|TestLog|TestPerfMetric|TestAdminAnalytics|TestUsageAnalytics' -count=1
go test ./pkg/perf_metrics -run Group -count=1
go test ./relay -run 'Test.*Task.*Group|TestRelayTask|TestTaskModel2Dto' -count=1
```

预期：PASS。

---

## 任务 6：前端 default 业务 group 清理

**文件：**
- 修改：`web/default/src/features/keys/types.ts`
- 修改：`web/default/src/features/users/api.ts`、`types.ts`、`lib/user-form.ts`、用户表格/抽屉组件
- 修改：`web/default/src/routes/_authenticated/users/index.tsx`
- 修改：`web/default/src/lib/api.ts`
- 修改：`web/default/src/components/profile-dropdown.tsx`
- 修改：`web/default/src/components/layout/components/mobile-drawer.tsx`
- 修改：`web/default/src/features/profile/components/profile-header.tsx`
- 删除或改为非业务用途：`web/default/src/components/group-badge.tsx`
- 删除或拆分：`web/default/src/components/model-group-selector.tsx`（移除业务 group selector；若仍被非分组 model selector 使用，保留/迁移 model-only 组件）
- 删除：`web/default/src/features/keys/components/api-key-group-combobox.tsx`
- 修改：`web/default/src/features/channels/api.ts`、`web/default/src/features/channels/types.ts`、`web/default/src/features/channels/lib/channel-form.ts`、`web/default/src/features/channels/components/drawers/channel-mutate-drawer.tsx`、`web/default/src/features/channels/components/channels-columns.tsx`、`web/default/src/features/channels/components/dialogs/edit-tag-dialog.tsx`、`web/default/src/features/channels/components/dialogs/upstream-update-dialog.tsx`
- 修改：`web/default/src/routes/_authenticated/channels/index.tsx`
- 修改：`web/default/src/features/models/types.ts`、`web/default/src/features/models/lib/model-form.ts`、`web/default/src/features/models/components/models-columns.tsx`、`web/default/src/features/models/components/dialogs/upstream-conflict-dialog.tsx`、`web/default/src/features/models/components/drawers/model-mutate-drawer.tsx`
- 修改：`web/default/src/features/system-settings/types.ts`、`web/default/src/features/system-settings/billing/index.tsx`、`web/default/src/features/system-settings/billing/section-registry.tsx`、`web/default/src/features/system-settings/models/index.tsx`、`web/default/src/features/system-settings/models/ratio-settings-card.tsx`、`web/default/src/features/system-settings/models/group-ratio-form.tsx`、`web/default/src/features/system-settings/models/group-ratio-visual-editor.tsx`、`web/default/src/features/system-settings/models/group-special-usable-editor.tsx`、`web/default/src/features/system-settings/request-limits/rate-limit-section.tsx`、`web/default/src/features/system-settings/security/index.tsx`、`web/default/src/features/system-settings/security/section-registry.tsx`、`web/default/src/features/system-settings/general/channel-affinity/types.ts`、`web/default/src/features/system-settings/general/channel-affinity/api.ts`、`web/default/src/features/system-settings/general/channel-affinity/constants.ts`、`web/default/src/features/system-settings/general/channel-affinity/index.tsx`、`web/default/src/features/system-settings/general/channel-affinity/rule-editor-dialog.tsx`、`web/default/src/features/system-settings/general/channel-affinity/cache-stats-dialog.tsx`
- 修改：`web/default/src/features/subscriptions/types.ts`、`web/default/src/features/subscriptions/api.ts`、`web/default/src/features/subscriptions/lib/plan-form.ts`、`web/default/src/features/subscriptions/components/subscriptions-columns.tsx`、`web/default/src/features/subscriptions/components/subscriptions-mutate-drawer.tsx`、`web/default/src/features/subscriptions/components/dialogs/subscription-purchase-dialog.tsx`
- 修改：`web/default/src/features/wallet/components/subscription-plans-card.tsx`
- 修改：`web/default/src/features/playground/api.ts`、`web/default/src/features/playground/constants.ts`、`web/default/src/features/playground/types.ts`、`web/default/src/features/playground/index.tsx`、`web/default/src/features/playground/components/playground-input.tsx`、`web/default/src/features/playground/hooks/use-playground-state.ts`、`web/default/src/features/playground/hooks/use-chat-handler.ts`、`web/default/src/features/playground/lib/payload-builder.ts`
- 修改：`web/default/src/features/pricing/index.tsx`、`web/default/src/features/pricing/types.ts`、`api.ts`、`columns.ts`、`search.ts`、`hooks/use-pricing-data.ts`、`hooks/use-filters.ts`、`lib/filters.ts`、`lib/price.ts`、`lib/dynamic-price.ts`、`lib/model-helpers.ts`、`lib/mock-stats.ts`、`components/pricing-sidebar.tsx`、`components/pricing-toolbar.tsx`、`components/pricing-columns.tsx`、`components/model-card.tsx`、`components/model-details.tsx`、`components/model-details-api.tsx`、`components/model-details-performance.tsx`
- 修改：`web/default/src/routes/pricing/index.tsx`、`web/default/src/routes/pricing/$modelId/index.tsx`
- 修改：`web/default/src/features/usage-logs/types.ts`、`api.ts`、`index.tsx`、`lib/format.ts`、`components/common-logs-filter-bar.tsx`、`components/usage-logs-table.tsx`、`components/columns/common-logs-columns.tsx`、`components/dialogs/details-dialog.tsx`、`components/dialogs/user-info-dialog.tsx`
- 修改：`web/default/src/routes/_authenticated/usage-logs/$section.tsx`
- 修改：`web/default/src/features/performance-metrics/types.ts`、`api.ts`、`lib/format.ts`
- 修改：`web/default/src/features/usage-analytics/types.ts`、`api.ts`、`constants.ts`、`index.tsx`、`components/usage-analytics-filter-bar.tsx`、`components/usage-ranking-table.tsx`、`components/usage-breakdown-chart.tsx`、`components/usage-trend-chart.tsx`、`lib/filters.ts`、`lib/chart-data.ts`、`lib/page-contract.ts`、`lib/filters.test.ts`、`lib/chart-data.test.ts`、`lib/page-contract.test.ts`
- 修改：`web/default/src/features/admin-analytics/types.ts`、`api.ts`、`constants.ts`、`index.tsx`、`lib/filters.ts`、`lib/drilldown.ts`、`lib/chart-data.ts`、`lib/page-contract.ts`、`lib/filters.test.ts`、`lib/drilldown.test.ts`、`lib/chart-data.test.ts`、`lib/page-contract.test.ts`
- 测试：`web/default/src/features/business-groups-removal.test.ts`（新建源码可见性测试）

- [ ] **步骤 1：编写 default 源码可见性测试**

新建 `web/default/src/features/business-groups-removal.test.ts`，读取关键文件并断言业务 group 文案/字段不存在，同时保留 allowlist 非业务 group。该测试仅扫描 default，不扫描 locale JSON；locale 由任务 8 和主验证处理。

```ts
import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { existsSync, readFileSync } from 'node:fs'
import { join } from 'node:path'

const root = process.cwd()
const read = (path: string) => readFileSync(join(root, path), 'utf8')
const readIfExists = (path: string) => existsSync(join(root, path)) ? read(path) : ''
const optionalFiles = new Set([
  'src/components/group-badge.tsx',
  'src/components/model-group-selector.tsx',
  'src/features/keys/components/api-key-group-combobox.tsx',
])

const businessForbidden = [
  'cross_group_retry',
  'default_use_auto_group',
  'GroupRatio',
  'GroupGroupRatio',
  'UserUsableGroups',
  'TopupGroupRatio',
  'AutoGroups',
  'ModelRequestRateLimitGroup',
  'include_using_group',
  'using_group',
  'upgrade_group',
  'enable_groups',
  'user_group',
  'request_group',
  '/api/group',
  '/api/user/self/groups',
  "group_by: 'group'",
  "groupBy: 'group'",
  'group_ratio',
  'user_group_ratio',
  'Groups filter',
  'Pricing by Group',
  '分组倍率',
  '升级分组',
]

describe('business group removal in default frontend', () => {
  it('removes business group contracts from feature source', () => {
    const files = [
      'src/features/keys/types.ts',
      'src/features/users/api.ts',
      'src/features/users/types.ts',
      'src/features/users/lib/user-form.ts',
      'src/routes/_authenticated/users/index.tsx',
      'src/lib/api.ts',
      'src/components/profile-dropdown.tsx',
      'src/components/layout/components/mobile-drawer.tsx',
      'src/features/profile/components/profile-header.tsx',
      'src/components/group-badge.tsx',
      'src/components/model-group-selector.tsx',
      'src/features/keys/components/api-key-group-combobox.tsx',
      'src/features/channels/api.ts',
      'src/features/channels/types.ts',
      'src/features/channels/lib/channel-form.ts',
      'src/features/channels/components/drawers/channel-mutate-drawer.tsx',
      'src/features/channels/components/channels-columns.tsx',
      'src/features/channels/components/dialogs/edit-tag-dialog.tsx',
      'src/features/channels/components/dialogs/upstream-update-dialog.tsx',
      'src/routes/_authenticated/channels/index.tsx',
      'src/features/models/types.ts',
      'src/features/models/lib/model-form.ts',
      'src/features/models/components/models-columns.tsx',
      'src/features/models/components/dialogs/upstream-conflict-dialog.tsx',
      'src/features/models/components/drawers/model-mutate-drawer.tsx',
      'src/features/system-settings/types.ts',
      'src/features/system-settings/billing/index.tsx',
      'src/features/system-settings/billing/section-registry.tsx',
      'src/features/system-settings/models/index.tsx',
      'src/features/system-settings/models/ratio-settings-card.tsx',
      'src/features/system-settings/models/group-ratio-form.tsx',
      'src/features/system-settings/models/group-ratio-visual-editor.tsx',
      'src/features/system-settings/models/group-special-usable-editor.tsx',
      'src/features/system-settings/request-limits/rate-limit-section.tsx',
      'src/features/system-settings/security/index.tsx',
      'src/features/system-settings/security/section-registry.tsx',
      'src/features/system-settings/general/channel-affinity/types.ts',
      'src/features/system-settings/general/channel-affinity/api.ts',
      'src/features/system-settings/general/channel-affinity/constants.ts',
      'src/features/system-settings/general/channel-affinity/index.tsx',
      'src/features/system-settings/general/channel-affinity/rule-editor-dialog.tsx',
      'src/features/system-settings/general/channel-affinity/cache-stats-dialog.tsx',
      'src/features/subscriptions/types.ts',
      'src/features/subscriptions/api.ts',
      'src/features/subscriptions/lib/plan-form.ts',
      'src/features/subscriptions/components/subscriptions-columns.tsx',
      'src/features/subscriptions/components/subscriptions-mutate-drawer.tsx',
      'src/features/subscriptions/components/dialogs/subscription-purchase-dialog.tsx',
      'src/features/wallet/components/subscription-plans-card.tsx',
      'src/features/playground/api.ts',
      'src/features/playground/constants.ts',
      'src/features/playground/types.ts',
      'src/features/playground/index.tsx',
      'src/features/playground/components/playground-input.tsx',
      'src/features/playground/hooks/use-playground-state.ts',
      'src/features/playground/hooks/use-chat-handler.ts',
      'src/features/playground/lib/payload-builder.ts',
      'src/features/pricing/index.tsx',
      'src/features/pricing/types.ts',
      'src/features/pricing/api.ts',
      'src/features/pricing/columns.ts',
      'src/features/pricing/search.ts',
      'src/features/pricing/hooks/use-pricing-data.ts',
      'src/features/pricing/hooks/use-filters.ts',
      'src/features/pricing/lib/filters.ts',
      'src/features/pricing/lib/price.ts',
      'src/features/pricing/lib/dynamic-price.ts',
      'src/features/pricing/lib/model-helpers.ts',
      'src/features/pricing/lib/mock-stats.ts',
      'src/features/pricing/components/pricing-sidebar.tsx',
      'src/features/pricing/components/pricing-toolbar.tsx',
      'src/features/pricing/components/pricing-columns.tsx',
      'src/features/pricing/components/model-card.tsx',
      'src/features/pricing/components/model-details.tsx',
      'src/features/pricing/components/model-details-api.tsx',
      'src/features/pricing/components/model-details-performance.tsx',
      'src/routes/pricing/index.tsx',
      'src/routes/pricing/$modelId/index.tsx',
      'src/features/usage-logs/types.ts',
      'src/features/usage-logs/api.ts',
      'src/features/usage-logs/index.tsx',
      'src/features/usage-logs/lib/format.ts',
      'src/features/usage-logs/components/common-logs-filter-bar.tsx',
      'src/features/usage-logs/components/usage-logs-table.tsx',
      'src/features/usage-logs/components/columns/common-logs-columns.tsx',
      'src/features/usage-logs/components/dialogs/details-dialog.tsx',
      'src/features/usage-logs/components/dialogs/user-info-dialog.tsx',
      'src/routes/_authenticated/usage-logs/$section.tsx',
      'src/features/performance-metrics/types.ts',
      'src/features/performance-metrics/api.ts',
      'src/features/performance-metrics/lib/format.ts',
      'src/features/usage-analytics/types.ts',
      'src/features/usage-analytics/api.ts',
      'src/features/usage-analytics/constants.ts',
      'src/features/usage-analytics/index.tsx',
      'src/features/usage-analytics/components/usage-analytics-filter-bar.tsx',
      'src/features/usage-analytics/components/usage-ranking-table.tsx',
      'src/features/usage-analytics/components/usage-breakdown-chart.tsx',
      'src/features/usage-analytics/components/usage-trend-chart.tsx',
      'src/features/usage-analytics/lib/filters.ts',
      'src/features/usage-analytics/lib/chart-data.ts',
      'src/features/usage-analytics/lib/page-contract.ts',
      'src/features/admin-analytics/types.ts',
      'src/features/admin-analytics/api.ts',
      'src/features/admin-analytics/constants.ts',
      'src/features/admin-analytics/index.tsx',
      'src/features/admin-analytics/lib/filters.ts',
      'src/features/admin-analytics/lib/drilldown.ts',
      'src/features/admin-analytics/lib/chart-data.ts',
      'src/features/admin-analytics/lib/page-contract.ts',
    ]
    for (const path of files) {
      if (!optionalFiles.has(path)) assert.equal(existsSync(join(root, path)), true, `${path} must exist and be scanned`)
      const source = optionalFiles.has(path) ? readIfExists(path) : read(path)
      for (const term of businessForbidden) {
        assert.equal(source.includes(term), false, `${path} still contains ${term}`)
      }
    }

    for (const removedPath of [
      'src/features/keys/components/api-key-group-combobox.tsx',
    ]) {
      assert.equal(existsSync(join(root, removedPath)), false, `${removedPath} should be deleted`)
    }
  })
})
```

- [ ] **步骤 2：运行测试确认失败**

运行：

```bash
cd web/default && bunx tsx --test src/features/business-groups-removal.test.ts
```

预期：FAIL，现有 default 前端仍有大量业务 group。

- [ ] **步骤 3：清理 users/profile/API key**

删除 `ApiKey` schema/type 中 `group`、`cross_group_retry`。

用户 API/search/form/types 删除 group 查询、字段、默认值、表格列、编辑控件；`routes/_authenticated/users/index.tsx` 删除 group search schema/query；profile dropdown/mobile drawer/profile header 删除 group 展示。

旧 `/api/group` / `/api/user/self/groups` client 函数从 `web/default/src/lib/api.ts` 和 feature API 中删除，除非兼容调用已无使用。

- [ ] **步骤 4：清理 channels/models/system settings/subscriptions**

channel form schema/defaults/payload 删除 group/groups；channel UI 不再请求 group list；tag edit/upstream update 不再发送 groups；`routes/_authenticated/channels/index.tsx` 删除 group search schema/query。

models types/forms/columns/upstream conflict 删除 `enable_groups`。

system settings 删除 group ratio form、TopupGroupRatio、ModelRequestRateLimitGroup group editor、channel affinity include_using_group/context key/cache stats using_group。

subscriptions plan form/API/types 和 `features/wallet/components/subscription-plans-card.tsx` 删除 `upgrade_group` 展示。

- [ ] **步骤 5：清理 playground/pricing/logs/perf/analytics/admin analytics**

playground 不再加载 user groups 或显示 group selector。

pricing sidebar/toolbar/search schema/columns/model details/price helpers 删除 `group_ratio`、`auto_groups`、`enable_groups`、Groups filter、Pricing by Group 等；同步清理 `routes/pricing/index.tsx` 和 `routes/pricing/$modelId/index.tsx` 的 group search schema。

usage logs 删除 group filters/types 和 `routes/_authenticated/usage-logs/$section.tsx` group search schema；performance metrics 使用实际路径 `web/default/src/features/performance-metrics/types.ts`、`api.ts`、`lib/format.ts`，删除 `PerformanceGroup.group` 类型和 pricing model details 的 per-group performance 表格。

usage analytics 删除业务 `group` group-by、Groups filter/drilldown；admin analytics 删除 user_group/request_group filters/group-by/types。

- [ ] **步骤 6：记录 static keys/locales 待任务 8 清理**

任务 6 不直接修改 `web/default/src/i18n/static-keys.ts` 或 `web/default/src/i18n/locales/*.json`，避免与任务 8 冲突；删除源码引用后，如留下无用 key，由任务 8 统一清理并运行 `bun run i18n:sync`。

- [ ] **步骤 7：运行 default 定向测试**

运行：

```bash
cd web/default && bunx tsx --test src/features/business-groups-removal.test.ts src/features/keys/api-key-form-visibility.test.ts
```

预期：PASS。

---

## 任务 7：前端 classic 业务 group 清理

**文件：**
- 修改：`web/classic/src/components/table/tokens/modals/EditTokenModal.jsx`
- 修改：`web/classic/src/components/table/tokens/TokensColumnDefs.jsx`
- 修改：`web/classic/src/components/table/users/modals/EditUserModal.jsx`
- 修改：`web/classic/src/components/settings/personal/components/UserInfoHeader.jsx`
- 修改：`web/classic/src/components/table/users/UsersColumnDefs.jsx`
- 修改：`web/classic/src/components/table/users/UsersFilters.jsx`
- 修改：`web/classic/src/components/table/channels/modals/EditChannelModal.jsx`
- 修改：`web/classic/src/components/table/channels/modals/EditTagModal.jsx`
- 修改：`web/classic/src/components/table/channels/ChannelsFilters.jsx`
- 修改：`web/classic/src/components/table/channels/ChannelsColumnDefs.jsx`
- 修改：`web/classic/src/hooks/channels/useChannelsData.jsx`
- 修改：`web/classic/src/pages/Setting/Ratio/GroupRatioSettings.jsx`
- 修改：`web/classic/src/components/settings/RatioSetting.jsx`
- 修改：`web/classic/src/pages/Setting/RateLimit/SettingsRequestRateLimit.jsx`
- 修改：`web/classic/src/components/settings/RateLimitSetting.jsx`
- 修改：`web/classic/src/pages/Setting/Operation/SettingsChannelAffinity.jsx`
- 修改：`web/classic/src/pages/Setting/Payment/SettingsGeneralPayment.jsx`
- 修改：`web/classic/src/components/settings/PaymentSetting.jsx`
- 修改：`web/classic/src/components/settings/SystemSetting.jsx`
- 修改：`web/classic/src/components/table/subscriptions/modals/AddEditSubscriptionModal.jsx`
- 修改：`web/classic/src/components/table/subscriptions/SubscriptionsColumnDefs.jsx`
- 修改：`web/classic/src/components/topup/SubscriptionPlansCard.jsx`
- 修改：`web/classic/src/components/topup/modals/SubscriptionPurchaseModal.jsx`
- 修改：`web/classic/src/components/playground/SettingsPanel.jsx`
- 修改：`web/classic/src/components/playground/OptimizedComponents.js`
- 修改：`web/classic/src/components/table/model-pricing/filter/PricingGroups.jsx`
- 修改：`web/classic/src/hooks/model-pricing/useModelPricingData.jsx`
- 修改：`web/classic/src/hooks/model-pricing/usePricingFilterCounts.js`
- 修改：`web/classic/src/helpers/render.jsx`
- 修改：`web/classic/src/helpers/utils.jsx`
- 修改：`web/classic/src/components/table/model-pricing/layout/PricingSidebar.jsx`
- 修改：`web/classic/src/components/table/model-pricing/layout/PricingPage.jsx`
- 修改：`web/classic/src/components/table/model-pricing/layout/content/PricingContent.jsx`
- 修改：`web/classic/src/components/table/model-pricing/view/card/PricingCardView.jsx`
- 修改：`web/classic/src/components/table/model-pricing/view/table/PricingTableColumns.jsx`
- 修改：`web/classic/src/components/table/model-pricing/modal/components/ModelPricingTable.jsx`
- 修改：`web/classic/src/components/table/model-pricing/modal/components/DynamicPricingBreakdown.jsx`
- 修改：`web/classic/src/components/table/model-pricing/modal/components/FilterModalContent.jsx`
- 修改：`web/classic/src/components/table/model-pricing/modal/ModelDetailSideSheet.jsx`
- 修改：`web/classic/src/components/table/model-pricing/modal/PricingFilterModal.jsx`
- 修改：`web/classic/src/components/table/usage-logs/UsageLogsFilters.jsx`
- 修改：`web/classic/src/components/table/usage-logs/UsageLogsColumnDefs.jsx`
- 修改：`web/classic/src/components/table/usage-logs/modals/ChannelAffinityUsageCacheModal.jsx`
- 修改：`web/classic/src/components/table/usage-logs/modals/UserInfoModal.jsx`
- 修改：`web/classic/src/components/table/models/ModelsColumnDefs.jsx`
- 修改：`web/classic/src/constants/playground.constants.js`
- 修改：`web/classic/src/hooks/playground/useDataLoader.js`
- 修改：`web/classic/src/constants/channel-affinity-template.constants.js`
- 修改：`web/classic/src/hooks/tokens/useTokensData.jsx`
- 修改：`web/classic/src/hooks/users/useUsersData.jsx`
- 修改：`web/classic/src/hooks/usage-logs/useUsageLogsData.jsx`
- 测试：`web/default/src/features/business-groups-removal-classic.test.ts`（新建，源码扫描 classic）

- [x] **步骤 1：编写 classic 源码可见性测试**

在 default 测试目录创建 node:test，读取 classic 源码关键文件，禁止业务 group 字段/文案，保留 allowlist 非业务 group。该测试只扫描 classic 源码，不修改 classic locale。

```ts
import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { existsSync, readFileSync } from 'node:fs'
import { resolve } from 'node:path'

const repoRoot = resolve(process.cwd(), '../..')
const read = (path: string) => readFileSync(resolve(repoRoot, path), 'utf8')
const readIfExists = (path: string) => existsSync(resolve(repoRoot, path)) ? read(path) : ''
const optionalFiles = new Set([
  'web/classic/src/pages/Setting/Ratio/GroupRatioSettings.jsx',
  'web/classic/src/components/table/model-pricing/filter/PricingGroups.jsx',
])

const forbidden = ['cross_group_retry', 'default_use_auto_group', 'upgrade_group', 'GroupRatio', 'UserUsableGroups', 'TopupGroupRatio', 'AutoGroups', 'ModelRequestRateLimitGroup', 'include_using_group', 'using_group', 'enable_groups', '/api/group', 'group_ratio', 'user_group_ratio', '分组倍率', '升级分组']

describe('business group removal in classic frontend', () => {
  it('removes classic business group UI contracts', () => {
    for (const path of [
      'web/classic/src/components/playground/SettingsPanel.jsx',
      'web/classic/src/components/playground/OptimizedComponents.js',
      'web/classic/src/components/table/tokens/modals/EditTokenModal.jsx',
      'web/classic/src/components/table/tokens/TokensColumnDefs.jsx',
      'web/classic/src/components/table/users/modals/EditUserModal.jsx',
      'web/classic/src/components/settings/personal/components/UserInfoHeader.jsx',
      'web/classic/src/components/table/users/UsersColumnDefs.jsx',
      'web/classic/src/components/table/users/UsersFilters.jsx',
      'web/classic/src/components/table/channels/modals/EditChannelModal.jsx',
      'web/classic/src/components/table/channels/modals/EditTagModal.jsx',
      'web/classic/src/components/table/channels/ChannelsColumnDefs.jsx',
      'web/classic/src/components/table/channels/ChannelsFilters.jsx',
      'web/classic/src/hooks/channels/useChannelsData.jsx',
      'web/classic/src/components/table/subscriptions/modals/AddEditSubscriptionModal.jsx',
      'web/classic/src/components/table/subscriptions/SubscriptionsColumnDefs.jsx',
      'web/classic/src/components/topup/SubscriptionPlansCard.jsx',
      'web/classic/src/components/table/model-pricing/filter/PricingGroups.jsx',
      'web/classic/src/hooks/model-pricing/useModelPricingData.jsx',
      'web/classic/src/hooks/model-pricing/usePricingFilterCounts.js',
      'web/classic/src/helpers/render.jsx',
      'web/classic/src/helpers/utils.jsx',
      'web/classic/src/components/table/model-pricing/layout/PricingSidebar.jsx',
      'web/classic/src/components/table/model-pricing/layout/PricingPage.jsx',
      'web/classic/src/components/table/model-pricing/view/card/PricingCardView.jsx',
      'web/classic/src/components/table/model-pricing/view/table/PricingTableColumns.jsx',
      'web/classic/src/components/table/model-pricing/modal/components/ModelPricingTable.jsx',
      'web/classic/src/components/table/model-pricing/layout/content/PricingContent.jsx',
      'web/classic/src/components/table/model-pricing/modal/components/DynamicPricingBreakdown.jsx',
      'web/classic/src/components/table/model-pricing/modal/components/FilterModalContent.jsx',
      'web/classic/src/components/table/model-pricing/modal/ModelDetailSideSheet.jsx',
      'web/classic/src/components/table/model-pricing/modal/PricingFilterModal.jsx',
      'web/classic/src/components/table/usage-logs/UsageLogsFilters.jsx',
      'web/classic/src/components/table/usage-logs/UsageLogsColumnDefs.jsx',
      'web/classic/src/components/table/usage-logs/modals/ChannelAffinityUsageCacheModal.jsx',
      'web/classic/src/components/table/usage-logs/modals/UserInfoModal.jsx',
      'web/classic/src/hooks/usage-logs/useUsageLogsData.jsx',
      'web/classic/src/components/topup/modals/SubscriptionPurchaseModal.jsx',
      'web/classic/src/pages/Setting/Ratio/GroupRatioSettings.jsx',
      'web/classic/src/components/settings/RatioSetting.jsx',
      'web/classic/src/components/settings/RateLimitSetting.jsx',
      'web/classic/src/pages/Setting/RateLimit/SettingsRequestRateLimit.jsx',
      'web/classic/src/pages/Setting/Operation/SettingsChannelAffinity.jsx',
      'web/classic/src/pages/Setting/Payment/SettingsGeneralPayment.jsx',
      'web/classic/src/components/settings/PaymentSetting.jsx',
      'web/classic/src/components/settings/SystemSetting.jsx',
      'web/classic/src/components/table/models/ModelsColumnDefs.jsx',
      'web/classic/src/constants/playground.constants.js',
      'web/classic/src/hooks/playground/useDataLoader.js',
      'web/classic/src/constants/channel-affinity-template.constants.js',
      'web/classic/src/hooks/tokens/useTokensData.jsx',
      'web/classic/src/hooks/users/useUsersData.jsx',
      'web/classic/src/pages/Pricing/index.jsx',
    ]) {
      if (!optionalFiles.has(path)) assert.equal(existsSync(resolve(repoRoot, path)), true, `${path} must exist and be scanned`)
      const source = optionalFiles.has(path) ? readIfExists(path) : read(path)
      for (const term of forbidden) {
        assert.equal(source.includes(term), false, `${path} still contains ${term}`)
      }
    }
  })
})
```

任务 7 不直接修改 `web/default/src/i18n/static-keys.ts` 或 locale JSON；如删除 UI 后留下无用 key，由任务 8 统一清理。

- [x] **步骤 2：运行测试确认失败**

运行：

```bash
cd web/default && bunx tsx --test src/features/business-groups-removal-classic.test.ts
```

预期：FAIL。

- [x] **步骤 3：清理 classic token/user/profile/channel**

删除 token group selector、auto group 默认、cross-group retry、token group column。

删除 user edit group 字段和 personal user group 展示。

删除 channel groups select、fetch `/api/group/`、tag edit groups、batch groups payload；同步清理 channel 列表列、filters、`useChannelsData` 中的 group option 拉取与 group query。

- [x] **步骤 4：清理 classic settings/subscription/playground/pricing/logs/models**

删除 RatioSetting 的分组 tab、GroupRatioSettings 入口或组件引用。

删除 RateLimit group 设置、channel affinity include_using_group/context key/cache stats using_group。

删除 Payment/System setting 中 `TopupGroupRatio` 设置和展示。

删除 subscription upgrade_group、topup 订阅卡片/购买弹窗/订阅表格中的升级分组展示、playground group selector、playground data loader group 拉取、pricing group filter、pricing hooks/sidebar/modal/view/helper 中的 group ratio/enable_groups 计算展示、usage logs group filter/column、ChannelAffinityUsageCacheModal using_group、models enable_groups column、token/user hooks 的 group loader/query。

- [x] **步骤 5：运行 classic 源码测试**

运行：

```bash
cd web/default && bunx tsx --test src/features/business-groups-removal-classic.test.ts
```

预期：PASS。

---

## 任务 8：OpenAPI、后端 i18n、前端 i18n allowlist 清理

**文件：**
- 修改：`docs/openapi/api.json`
- 修改：`i18n/locales/en.yaml`
- 修改：`i18n/locales/zh-CN.yaml`
- 修改：`i18n/locales/zh-TW.yaml`
- 修改：`web/default/src/i18n/static-keys.ts`
- 修改：`web/default/src/i18n/locales/*.json`（由 i18n sync 更新）
- 修改：`web/classic/src/i18n/locales/*.json`
- 修改：`README.md`、`README.en.md`、`README.zh_CN.md`、`README.zh_TW.md`、`docs/channel/other_setting.md` 中如存在业务 group option 文案则限定编辑；不得修改 protected project branding。
- 测试：`web/default/src/features/business-group-contract-removal.test.ts`（或等价源码/文档可见性测试，覆盖 OpenAPI、README、docs、后端 i18n、default static keys、default locales、classic locales）

- [x] **步骤 1：编写 contract 源码测试**

测试读取 OpenAPI、README、docs、后端 i18n、default static keys、default locales、classic locales，禁止业务 group contract，允许 `/api/prefill_group/`、OAuth claim groups、Uptime Kuma groups、CSS/Tailwind group、SQL `GROUP BY` 等 allowlist。

```ts
import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

const repoRoot = resolve(process.cwd(), '../..')
const read = (path: string) => readFileSync(resolve(repoRoot, path), 'utf8')

describe('business group public contracts are removed', () => {
  it('removes business group endpoints and schemas from OpenAPI', () => {
    const api = read('docs/openapi/api.json')
    for (const term of ['cross_group_retry', 'upgrade_group', 'enable_groups', 'group_ratio']) {
      assert.equal(api.includes(term), false, `OpenAPI still contains ${term}`)
    }
    assert.equal(api.includes('/api/prefill_group/'), true)
    // 旧 no-op shim 若仍在 OpenAPI 中出现，必须标记 legacy/no-op 且 schema 不暴露业务字段；也可从公开 OpenAPI 中删除这些旧 endpoint。
  })

  it('removes business group phrases from i18n contracts', () => {
    for (const path of [
      'i18n/locales/en.yaml',
      'i18n/locales/zh-CN.yaml',
      'i18n/locales/zh-TW.yaml',
      'web/default/src/i18n/static-keys.ts',
      'web/default/src/i18n/locales/en.json',
      'web/default/src/i18n/locales/zh.json',
      'web/default/src/i18n/locales/fr.json',
      'web/default/src/i18n/locales/ja.json',
      'web/default/src/i18n/locales/ru.json',
      'web/default/src/i18n/locales/vi.json',
      'web/classic/src/i18n/locales/en.json',
      'web/classic/src/i18n/locales/zh-CN.json',
      'web/classic/src/i18n/locales/zh.json',
      'web/classic/src/i18n/locales/zh-TW.json',
      'web/classic/src/i18n/locales/fr.json',
      'web/classic/src/i18n/locales/ja.json',
      'web/classic/src/i18n/locales/ru.json',
      'web/classic/src/i18n/locales/vi.json',
    ]) {
      const source = read(path)
      for (const term of ['GroupRatio', 'TopupGroupRatio', 'DefaultUseAutoGroup', 'cross_group_retry', 'upgrade_group', 'enable_groups', 'using_group', 'group_access_denied', 'group_not_exists', 'under group', '当前分组', '分组 {{.Group}}', '分组倍率', '升级分组']) {
        assert.equal(source.includes(term), false, `${path} still contains ${term}`)
      }
    }
  })
})
```

- [x] **步骤 2：运行测试确认失败**

运行：

```bash
cd web/default && bunx tsx --test src/features/business-group-contract-removal.test.ts
```

预期：FAIL。

- [x] **步骤 3：清理 OpenAPI**

删除业务 group endpoints/schema fields：`/api/user/groups`、`/api/user/self/groups`、`/api/group/`、User.group、Token.group/cross_group_retry、Channel.group/groups、Subscription upgrade_group、pricing group_ratio/auto_groups/enable_groups、analytics business group filters。保留 `/api/prefill_group/`。

- [x] **步骤 4：清理后端 i18n 和 README/配置说明**

搜索业务 group 文案和配置项：`GroupRatio`、`TopupGroupRatio`、`AutoGroups`、`UserUsableGroups`、`DefaultUseAutoGroup`、`cross_group_retry`、`upgrade_group`、`enable_groups`、`using_group`、`分组倍率`、`自动分组`、`升级分组`。删除或改写为非业务描述。同步清理 default static keys/locales 和 classic `web/classic/src/i18n/locales/*.json`；不得删除 protected project branding。
任务 8 基于任务 6/7 已删除的源码 key 做最终 prune/sync；`web/default/src/i18n/static-keys.ts` 由任务 8 统一清理，任务 6/7 不并发修改 locale JSON。

- [x] **步骤 5：运行 contract 测试**

运行：

```bash
cd web/default && bunx tsx --test src/features/business-group-contract-removal.test.ts
```

预期：PASS。

---

## 任务 9：集成验证与收口

**文件：**
- 修改：本计划任务 1-8 已列出的源码与测试文件；若最终搜索发现业务残留，先把该文件归入对应任务边界后再修复并复审。
- 不创建新文档，除非上一任务需要更新现有 README/配置说明

- [ ] **步骤 1：运行后端定向测试矩阵**

运行：

```bash
go test ./controller -run 'Test(GroupEndpointsReturnNoopCompatibilityPayloads|GroupOptionsAreAcceptedAsNoopCompatibilityWrites|Token|Channel.*Group|ConfigGuide|ModelList|Status|Playground|ChannelAffinity|GetPricing|UsageAnalytics|AdminAnalytics|PerfMetrics|Log|Subscription|Topup|Waffo|Pancake)' -count=1
go test ./controller -run 'Test(User.*Group|UserGroupRemoval|SearchUsersIgnoresGroup|LoginResponseOmitsGroup|GetUserModelsIgnoresGroup)' -count=1
go test ./model -run 'Test(ChannelAbilitiesIgnoreBusinessGroups|ChannelCache.*Group|Subscription|Redemption|Task|Pricing|Log|UsageAnalytics|AdminAnalytics|PerfMetric)' -count=1
go test ./service -run 'Test(GenerateTextOtherInfo|TaskBilling|Channel|Quota|ViolationFee|ChannelAffinity)' -count=1
go test ./pkg/perf_metrics -run Group -count=1
go test ./pkg/loadtest/config ./pkg/loadtest/seed ./pkg/loadtest/artifact ./cmd/loadtest-seed -count=1
go test ./relay -run 'Test.*Task.*Group|TestRelayTask|TestTaskModel2Dto' -count=1
```

预期：全部 PASS。若某个包没有匹配测试，Go 会报告 no tests to run 但包必须编译通过。

- [ ] **步骤 2：运行前端定向测试矩阵**

运行：

```bash
cd web/default && bunx tsx --test \
  src/features/business-groups-removal.test.ts \
  src/features/business-groups-removal-classic.test.ts \
  src/features/business-group-contract-removal.test.ts \
  src/features/keys/api-key-form-visibility.test.ts
```

预期：全部 PASS。

- [ ] **步骤 3：运行 i18n、类型检查、构建**

运行：

```bash
cd web/default && bun run i18n:sync && bun run typecheck && bun run build
```

预期：i18n sync 完成，`tsc -b` 成功，`rsbuild build` 成功。

- [ ] **步骤 4：运行三库迁移 smoke test 或等价验证**

SQLite 至少运行现有迁移/模型初始化相关测试；MySQL/PostgreSQL 如本地无服务，运行项目已有 DB compatibility 测试或记录不可运行原因，并确保所有 raw SQL 保留字引用仍通过 `commonGroupCol` / `logGroupCol`。

推荐命令：

```bash
go test ./model -run 'Test.*SQLite|Test.*Migration|Test.*Subscription' -count=1
```

如没有三库环境，最终报告必须明确实际验证范围，不能声称三库运行通过。
最小 legacy fixture 验证：在 SQLite 中 seed `users.group`、`tokens.group/cross_group_retry`、`channels.group`、`abilities.group`、`subscription_plans.upgrade_group`、`user_subscriptions.upgrade_group/prev_user_group`、`logs.group`、`perf_metrics.group`、`tasks.group` 旧值，运行本计划新增测试，确认旧列保留但不参与模型可见性、渠道选择、计费、充值、订阅、日志过滤或分析聚合。

- [ ] **步骤 5：最终源码可见性搜索**

使用 OMP `search` 工具而不是 shell grep，检查业务 group 残留。允许命中：`prefill_group`、OAuth claim groups、Uptime Kuma groups、UI layout group、SQL `GROUP BY`、CSS/Tailwind group、测试中 allowlist 说明。业务残留必须修复或在计划执行记录中说明为何非业务。

- [ ] **步骤 6：最终审查**

提交前分派至少 3 个只读 reviewer 子代理并发审查最终 diff：后端运行时、前端 UI/API、兼容与合同。所有 review pass 后才能提交；若 review fail，修复后重新运行受影响验证并复审。

- [ ] **步骤 7：提交**

主代理检查：

```bash
git status -sb --untracked-files=all
git diff --stat
```

确认只包含本任务文件后提交。优先使用 `git add -A`，但提交前必须先检查 `git status -sb --untracked-files=all` 和 `git diff --stat`，确认 worktree 中只有本任务相关文件；如出现无关文件，改用逐文件 `git add`。

```bash
git add -A
git commit -m "refactor(groups): 移除业务分组概念"
```

如果 notify 工具可用，最终 review 全部通过并提交后用 notify 报告；当前工具集中没有 notify 时，在最终回复中报告同等信息。
