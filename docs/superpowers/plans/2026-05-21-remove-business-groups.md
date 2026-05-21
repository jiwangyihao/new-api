# 全面移除业务分组概念实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。当前用户明确要求不要使用 worktree，直接在主分支开发；所有实现子代理必须收到完整规格/计划路径和 2000 字以上任务提示。子代理只改自己任务列出的文件，不运行项目级 build/lint/typecheck/i18n/build，不格式化全仓；主代理在批次后统一验证。

**目标：** 从产品、后端运行时、前端 UI/API 类型、OpenAPI、i18n 与配置中移除所有业务分组概念，包括渠道分组、token 分组、用户 vip/svip/default 分组、订阅升级分组、分组倍率、分组限流、分组亲和、日志/指标/分析 group 维度，同时保留非业务 `group` 概念。

**架构：** 第一阶段采用逻辑删除 + 兼容读取：保留历史 DB 列和跨库迁移安全性，但业务代码不再读取 group 值做决策，旧 payload/option/endpoint 通过 no-op shim 兼容。渠道能力从 `group + model + channel` 降级为 `model + channel`，计费倍率移除外部 group ratio 合同，订阅与充值不再更新用户分组。default 与 classic 前端同步删除业务 group UI、类型、i18n 文案，OpenAPI/README 与测试矩阵锁定无业务 group 合同。

**技术栈：** Go 1.22+、Gin、GORM、SQLite/MySQL/PostgreSQL；React 19/TypeScript/Rsbuild/Bun；go-i18n；node:test/tsx。

**规格：** `C:/Users/34404/source/repos/new-api/docs/superpowers/specs/2026-05-21-remove-ai-channel-groups-design.md`

---

## 文件职责与批次边界

### 后端核心批次

- `controller/group.go`、`router/api-router.go`：旧 group endpoint no-op shim。
- `controller/option.go`、`model/option.go`、`setting/ratio_setting/group_ratio.go`、`setting/auto_group.go`、`setting/user_usable_group.go`、`common/topup-ratio.go`、`setting/rate_limit.go`：旧 group option no-op 与运行时分组配置退役。
- `model/ability.go`、`model/channel.go`、`model/channel_cache.go`、`model/channel_satisfy.go`、`service/channel_select.go`：无 group 渠道路由矩阵。
- `middleware/auth.go`、`middleware/distributor.go`、`relay/common/relay_info.go`、`relay/relay.go`、`controller/config_guide.go`、`controller/model.go`、`controller/user.go`：请求上下文、模型列表、config guide 不再依赖 group。
- `model/user.go`、`model/user_cache.go`、`controller/user.go`、`model/subscription.go`、`controller/subscription.go`、`model/redemption.go`、`controller/subscription_payment_balance.go`：用户/订阅/兑换码分组退役。
- `relay/helper/price.go`、`service/log_info_generate.go`、`service/task_billing.go`、`model/task.go`、`dto/task.go`、`controller/task_video.go`、`relay/relay_task.go`、`controller/topup*.go`：计费、充值、任务 group ratio 退役。
- `controller/pricing.go`、`model/model_meta.go`、`controller/model_meta.go`、`model/log.go`、`controller/log.go`、`model/perf_metric.go`、`controller/perf_metrics.go`、`dto/usage_analytics.go`、`model/usage_analytics.go`、`controller/usage_analytics.go`、`dto/admin_analytics.go`、`model/admin_analytics*.go`、`controller/admin_analytics.go`：展示、日志、指标、分析 group 维度退役。

### 前端批次

- `web/default/src/features/keys/*`：API key 类型残留清理。
- `web/default/src/features/users/*`、`web/default/src/components/profile-dropdown.tsx`、`web/default/src/components/layout/components/mobile-drawer.tsx`、`web/default/src/features/profile/components/profile-header.tsx`：用户 group UI/type/API 清理。
- `web/default/src/features/channels/*`、`web/default/src/features/models/*`、`web/default/src/features/system-settings/*`、`web/default/src/features/subscriptions/*`、`web/default/src/features/playground/*`、`web/default/src/features/usage-logs/*`、`web/default/src/features/perf-metrics/*`、`web/default/src/features/usage-analytics/*`、`web/default/src/features/admin-analytics/*`：default 业务 group UI/type/API 清理。
- `web/classic/src/components/table/tokens/*`、`web/classic/src/components/table/users/*`、`web/classic/src/components/settings/personal/*`、`web/classic/src/components/table/channels/*`、`web/classic/src/pages/Setting/*`、`web/classic/src/components/table/subscriptions/*`、`web/classic/src/components/playground/*`、`web/classic/src/components/table/model-pricing/*`、`web/classic/src/components/table/usage-logs/*`、`web/classic/src/components/table/models/*`：classic 业务 group UI/type/API 清理。
- `web/default/src/i18n/static-keys.ts`、`web/default/src/i18n/locales/*.json`、`web/classic/src/locales/*`：业务 group i18n 清理，保留 allowlist 非业务 group。

### 文档与合约批次

- `docs/openapi/api.json`、README/配置说明、后端 `i18n/locales/*.yaml`：业务 group API/字段/文案删除，保留 `/api/prefill_group/`。

---

## 任务 1：后端兼容 shim 与配置 no-op

**文件：**
- 修改：`controller/group.go`
- 修改：`router/api-router.go`
- 修改：`controller/option.go`
- 修改：`model/option.go`
- 测试：`controller/group_compat_test.go`（新建）
- 测试：`controller/option_group_compat_test.go`（新建或并入现有 option 测试）

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
    t.Cleanup(func() {
        require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(originalGroupRatio))
        require.NoError(t, setting.UpdateAutoGroupsByJsonString(originalAutoGroups))
        require.NoError(t, common.UpdateTopupGroupRatioByJSONString(originalTopupRatio))
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
    }

    require.JSONEq(t, originalGroupRatio, ratio_setting.GroupRatio2JSONString())
    require.JSONEq(t, originalAutoGroups, setting.AutoGroups2JsonString())
    require.JSONEq(t, originalTopupRatio, common.TopupGroupRatio2JSONString())
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

在 `controller/option.go` 的 group option 校验分支中同步跳过旧 group option 校验，确保旧管理端保存返回成功。

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
- 测试：`model/channel_group_removal_test.go`（新建或扩展现有 channel/ability 测试）

- [ ] **步骤 1：编写无分组 ability 与 routing 测试**

测试应证明：同一 channel 即使旧 `Group` 中有 `default,vip`，更新能力后只按模型生成一条有效 ability；查询按 model/priority/weight 选出渠道，不需要 group；旧 group 数据不会重复命中。

```go
func TestChannelAbilitiesIgnoreBusinessGroups(t *testing.T) {
    db := setupModelTestDB(t)
    channel := &model.Channel{Id: 1001, Type: constant.ChannelTypeOpenAI, Key: "sk-test", Status: common.ChannelStatusEnabled, Name: "groupless", Models: "gpt-test", Group: "default,vip"}
    require.NoError(t, db.Create(channel).Error)
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

`model/channel_cache.go` 将缓存结构从 group->model->channels 改为 model->channels；对外旧签名如暂时保留 group 参数，则忽略该参数并转调新函数。`model/channel_satisfy.go` 不再判断 group，仅按 model/status/其他已有非分组条件判断。

- [ ] **步骤 5：调整 service channel select**

`service/channel_select.go` 中 `RetryParam` 删除或忽略 `TokenGroup`；`CacheGetRandomSatisfiedChannel` 不再处理 `auto` / cross-group retry，直接：

```go
channel, err := model.GetRandomSatisfiedChannel(param.ModelName, param.GetRetry())
return channel, "", err
```

如调用方仍需要第二返回值，返回空字符串或移除字段需与后续任务协调。

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
- 修改：`relay/common/relay_info.go`
- 修改：`relay/relay.go`
- 修改：`controller/model.go`
- 修改：`controller/config_guide.go`
- 修改：`controller/user.go`
- 测试：`controller/model_list_test.go`
- 测试：`controller/config_guide_test.go`

- [ ] **步骤 1：编写 model list / config guide 无分组测试**

更新或新增测试，证明用户/token/channel 的旧 group 值为 `gone` 也不导致模型列表/config guide 403，只要 token/user 状态、quota、model limits、channel ability 非分组条件满足。

```go
func TestModelListIgnoresUserAndTokenGroups(t *testing.T) {
    db := setupModelListTestDB(t)
    require.NoError(t, db.Create(&model.User{Id: 901, Username: "groupless", Password: "password", Group: "gone", Status: common.UserStatusEnabled, AffCode: "groupless"}).Error)
    require.NoError(t, db.Create(&model.Token{UserId: 901, Name: "token", Key: "sk-groupless", Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true, Group: "gone"}).Error)
    seedEnabledAbilityWithoutBusinessGroup(t, db, "gpt-groupless", 1001)

    recorder := httptest.NewRecorder()
    c, _ := gin.CreateTestContext(recorder)
    c.Set("id", 901)
    c.Set("token_id", 1)
    // call list handler used by existing tests

    require.Equal(t, http.StatusOK, recorder.Code)
    require.Contains(t, recorder.Body.String(), "gpt-groupless")
}
```

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
- 修改：`model/subscription.go`
- 修改：`controller/subscription.go`
- 修改：`controller/subscription_payment_balance.go`
- 修改：`model/redemption.go`
- 修改：`controller/topup.go`
- 修改：`controller/topup_stripe.go`
- 修改：`controller/topup_waffo.go`
- 修改：`controller/topup_waffo_pancake.go`
- 测试：`controller/subscription_group_removal_test.go`（新建或扩展 subscription 测试）
- 测试：`controller/topup_group_ratio_test.go`（新建或扩展 topup 测试）

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

预期：FAIL，当前购买订阅会更新 group，充值按 group ratio。

- [ ] **步骤 4：用户模型兼容字段退役**

`model/user.go`：保留 `Group` legacy 字段用于旧 DB，但 `ToBaseUser` 不再填业务 Group；`SearchUsers` 忽略 group 过滤；`Edit` / `Update` 不再写 group。

`model/user_cache.go`：`UserBase.Group` 如果暂时保留，标记 legacy，`WriteContext` 不再写业务 group；`UpdateUserGroupCache` 调用点移除或变成 no-op 兼容。

`controller/user.go`：创建/编辑/搜索/详情/登录响应不暴露 group；注册默认 token 不再根据 `DefaultUseAutoGroup` 写 auto group。

- [ ] **步骤 5：订阅与兑换码去分组**

`controller/subscription.go`：创建/更新 plan 忽略 `UpgradeGroup`，不校验 group ratio；响应不暴露 upgrade_group。

`model/subscription.go`：`CreateUserSubscriptionFromPlanTx`、`ExpireDueSubscriptions`、downgrade 相关函数不写/不回退用户 group；保留旧列但不读写。

`controller/subscription_payment_balance.go`、其他支付回调路径、`model/redemption.go`：删除 `UpdateUserGroupCache` 调用。

- [ ] **步骤 6：充值去 group ratio**

`controller/topup*.go`：`getPayMoney`、`getStripePayMoney`、Waffo/Pancake 金额计算不读取 `model.GetUserGroup` 或 `common.GetTopupGroupRatio`。函数签名如保留 group 参数，参数不使用并命名 `_ string`。

`common/topup-ratio.go` 可保留旧 option 兼容函数，但运行时返回 1 或 no-op。

- [ ] **步骤 7：运行定向测试**

运行：

```bash
go test ./controller -run 'TestSubscription|TestTopup|TestWaffo|TestPancake' -count=1
go test ./model -run 'TestSubscription|TestRedemption' -count=1
```

预期：PASS。

---

## 任务 5：后端计费、任务、价格、日志、指标、分析去分组

**文件：**
- 修改：`relay/helper/price.go`
- 修改：`service/log_info_generate.go`
- 修改：`service/task_billing.go`
- 修改：`model/task.go`
- 修改：`dto/task.go`
- 修改：`controller/task_video.go`
- 修改：`relay/relay_task.go`
- 修改：`controller/pricing.go`
- 修改：`model/model_meta.go`
- 修改：`controller/model_meta.go`
- 修改：`model/log.go`
- 修改：`controller/log.go`
- 修改：`model/perf_metric.go`
- 修改：`controller/perf_metrics.go`
- 修改：`dto/usage_analytics.go`
- 修改：`model/usage_analytics.go`
- 修改：`controller/usage_analytics.go`
- 修改：`dto/admin_analytics.go`
- 修改：`model/admin_analytics.go`
- 修改：`model/admin_analytics_usage.go`
- 修改：`model/admin_analytics_drilldown.go`
- 修改：`controller/admin_analytics.go`
- 测试：相关现有 pricing/log/perf/analytics/admin analytics/task billing 测试

- [ ] **步骤 1：编写 pricing/log other 无 group ratio 测试**

扩展 `controller/pricing_directory_test.go`：管理员和普通用户响应都不包含 `group_ratio`、`auto_groups`、`enable_groups`。

扩展 `service/log_info_generate_test.go`：`GenerateTextOtherInfo` 不输出 `group_ratio` / `user_group_ratio` / `group_group_ratio`。

- [ ] **步骤 2：编写 task billing 不读 group 测试**

在 `service/task_billing_test.go` 添加：旧 task.Group/user.Group 为 `vip` 且 group ratio 为 9 时，重算结果仍按倍率 1。

- [ ] **步骤 3：编写 analytics group 兼容测试**

用户侧：`group_by=group` 返回既定 400 或回退默认；`groups` filter 忽略且不 500。管理端：`user_group` / `request_group` group-by 和 filters 同样按既定行为测试。计划执行时必须在任务内固定每个端点行为，推荐：业务 group-by 返回 400，业务 group filters 忽略。

- [ ] **步骤 4：运行测试确认失败**

运行：

```bash
go test ./controller -run 'TestGetPricing|TestUsageAnalytics|TestAdminAnalytics|TestPerfMetrics|TestLog' -count=1
go test ./service -run 'TestGenerateTextOtherInfo|TestTaskBilling' -count=1
```

预期：FAIL，现有响应/日志/analytics 仍含 group。

- [ ] **步骤 5：计费结构去 group ratio 外部暴露**

`relay/helper/price.go`：`HandleGroupRatio` 删除业务逻辑或固定 1；外部错误文案移除 “Group & Model Pricing”。

`service/log_info_generate.go`：删除 `group_ratio`、`user_group_ratio`、`group_special_ratio` 注入。

保留模型倍率、补全倍率、缓存倍率、模型价格、表达式计费。

- [ ] **步骤 6：任务去 group**

`model/task.go` 保留 legacy Group 字段但新任务不写；`TaskBillingContext.GroupRatio` 删除或固定 1。

`service/task_billing.go` 不回读 `User.Group` 或 ratio_setting group ratio；任务日志不写 group。

`dto/task.go`、`controller/task_video.go`、`relay/relay_task.go` 响应不输出业务 group。

- [ ] **步骤 7：pricing/model meta 去 group**

`controller/pricing.go` 不再返回 `group_ratio`、`auto_groups`，不按 usable group 过滤 pricing。

`model/model_meta.go`、`controller/model_meta.go` 删除 `enable_groups` 输出或置为空并不暴露到 API/frontend。

- [ ] **步骤 8：logs/perf/analytics 去 group**

`controller/log.go` 忽略 group query；`model/log.go` 的 filters 不再应用 Group。

`model/perf_metric.go` 聚合唯一键改业务逻辑为 model + bucket；若旧 DB 仍有 group 列，写入固定空字符串，不暴露/不过滤。

`dto/usage_analytics.go` 删除业务 `UsageAnalyticsGroupByGroup` 和 `Groups` filter；保留通用 group_by 机制。

`model/usage_analytics.go`、`controller/usage_analytics.go` 对 `group_by=group` 返回 400 或回退默认，按步骤 3 固定。

`dto/admin_analytics.go`、`model/admin_analytics*.go`、`controller/admin_analytics.go` 删除/忽略 user_group/request_group filters 和 group-by/output。

- [ ] **步骤 9：运行定向测试**

运行：

```bash
go test ./controller -run 'TestGetPricing|TestUsageAnalytics|TestAdminAnalytics|TestPerfMetrics|TestLog' -count=1
go test ./service -run 'TestGenerateTextOtherInfo|TestTaskBilling' -count=1
```

预期：PASS。

---

## 任务 6：前端 default 业务 group 清理

**文件：**
- 修改：`web/default/src/features/keys/types.ts`
- 修改：`web/default/src/features/users/api.ts`、`types.ts`、`lib/user-form.ts`、用户表格/抽屉组件
- 修改：`web/default/src/components/profile-dropdown.tsx`
- 修改：`web/default/src/components/layout/components/mobile-drawer.tsx`
- 修改：`web/default/src/features/profile/components/profile-header.tsx`
- 修改：`web/default/src/features/channels/api.ts`、`types.ts`、`lib/channel-form.ts`、相关 channel components
- 修改：`web/default/src/features/models/*`
- 修改：`web/default/src/features/system-settings/*`
- 修改：`web/default/src/features/subscriptions/*`
- 修改：`web/default/src/features/playground/*`
- 修改：`web/default/src/features/usage-logs/*`
- 修改：`web/default/src/features/perf-metrics/*`
- 修改：`web/default/src/features/usage-analytics/*`
- 修改：`web/default/src/features/admin-analytics/*`
- 测试：`web/default/src/features/business-groups-removal.test.ts`（新建源码可见性测试）

- [ ] **步骤 1：编写 default 源码可见性测试**

新建 `web/default/src/features/business-groups-removal.test.ts`，读取关键文件并断言业务 group 文案/字段不存在，同时 allowlist 非业务 group。

```ts
import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { join } from 'node:path'

const root = process.cwd()
const read = (path: string) => readFileSync(join(root, path), 'utf8')

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
]

describe('business group removal in default frontend', () => {
  it('removes business group contracts from feature source', () => {
    for (const path of [
      'src/features/keys/types.ts',
      'src/features/users/api.ts',
      'src/features/users/types.ts',
      'src/features/channels/lib/channel-form.ts',
      'src/features/system-settings/types.ts',
      'src/features/subscriptions/lib/plan-form.ts',
      'src/features/usage-analytics/types.ts',
      'src/features/admin-analytics/types.ts',
      'src/i18n/static-keys.ts',
    ]) {
      const source = read(path)
      for (const term of businessForbidden) {
        assert.equal(source.includes(term), false, `${path} still contains ${term}`)
      }
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

用户 API/search/form/types 删除 group 查询、字段、默认值、表格列、编辑控件；profile dropdown/mobile drawer/profile header 删除 group 展示。

旧 `/api/group` / `/api/user/self/groups` client 函数删除，除非兼容调用已无使用。

- [ ] **步骤 4：清理 channels/models/system settings/subscriptions**

channel form schema/defaults/payload 删除 group/groups；channel UI 不再请求 group list；tag edit/upstream update 不再发送 groups。

models types/forms/columns/upstream conflict 删除 `enable_groups`。

system settings 删除 group ratio form、TopupGroupRatio、ModelRequestRateLimitGroup group editor、channel affinity include_using_group/context key/cache stats using_group。

subscriptions plan form/API/types 删除 `upgrade_group`。

- [ ] **步骤 5：清理 playground/pricing/logs/perf/analytics/admin analytics**

playground 不再加载 user groups 或显示 group selector。

pricing 不再处理 `group_ratio`、`auto_groups`、`enable_groups`。

usage logs/perf metrics 删除 group filters/types。

usage analytics 删除业务 `group` group-by、Groups filter/drilldown；admin analytics 删除 user_group/request_group filters/group-by/types。

- [ ] **步骤 6：清理 static keys/locales**

`web/default/src/i18n/static-keys.ts` 删除业务 group keys；保留 allowlist 非业务 group。

不要手工大范围编辑 locale JSON；修改源码后由主代理运行 `bun run i18n:sync`。

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
- 修改：`web/classic/src/components/table/channels/modals/EditChannelModal.jsx`
- 修改：`web/classic/src/components/table/channels/modals/EditTagModal.jsx`
- 修改：`web/classic/src/pages/Setting/Ratio/GroupRatioSettings.jsx`
- 修改：`web/classic/src/components/settings/RatioSetting.jsx`
- 修改：`web/classic/src/pages/Setting/RateLimit/SettingsRequestRateLimit.jsx`
- 修改：`web/classic/src/components/settings/RateLimitSetting.jsx`
- 修改：`web/classic/src/pages/Setting/Operation/SettingsChannelAffinity.jsx`
- 修改：`web/classic/src/components/table/subscriptions/modals/AddEditSubscriptionModal.jsx`
- 修改：`web/classic/src/components/playground/SettingsPanel.jsx`
- 修改：`web/classic/src/components/table/model-pricing/filter/PricingGroups.jsx`
- 修改：`web/classic/src/components/table/usage-logs/UsageLogsFilters.jsx`
- 修改：`web/classic/src/components/table/models/ModelsColumnDefs.jsx`
- 测试：`web/default/src/features/business-groups-removal-classic.test.ts`（新建，源码扫描 classic）

- [ ] **步骤 1：编写 classic 源码可见性测试**

在 default 测试目录创建 node:test，读取 classic 源码关键文件，禁止业务 group 字段/文案，allowlist 非业务 group。

```ts
import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

const repoRoot = resolve(process.cwd(), '../..')
const read = (path: string) => readFileSync(resolve(repoRoot, path), 'utf8')

const forbidden = ['cross_group_retry', 'default_use_auto_group', 'upgrade_group', 'GroupRatio', 'UserUsableGroups', 'TopupGroupRatio', 'AutoGroups', 'ModelRequestRateLimitGroup', 'include_using_group', 'using_group', 'enable_groups']

describe('business group removal in classic frontend', () => {
  it('removes classic business group UI contracts', () => {
    for (const path of [
      'web/classic/src/components/table/tokens/modals/EditTokenModal.jsx',
      'web/classic/src/components/table/users/modals/EditUserModal.jsx',
      'web/classic/src/components/table/channels/modals/EditChannelModal.jsx',
      'web/classic/src/components/table/subscriptions/modals/AddEditSubscriptionModal.jsx',
      'web/classic/src/pages/Setting/Ratio/GroupRatioSettings.jsx',
      'web/classic/src/pages/Setting/Operation/SettingsChannelAffinity.jsx',
    ]) {
      const source = read(path)
      for (const term of forbidden) {
        assert.equal(source.includes(term), false, `${path} still contains ${term}`)
      }
    }
  })
})
```

- [ ] **步骤 2：运行测试确认失败**

运行：

```bash
cd web/default && bunx tsx --test src/features/business-groups-removal-classic.test.ts
```

预期：FAIL。

- [ ] **步骤 3：清理 classic token/user/profile/channel**

删除 token group selector、auto group 默认、cross-group retry、token group column。

删除 user edit group 字段和 personal user group 展示。

删除 channel groups select、fetch `/api/group/`、tag edit groups、batch groups payload。

- [ ] **步骤 4：清理 classic settings/subscription/playground/pricing/logs/models**

删除 RatioSetting 的分组 tab、GroupRatioSettings 入口或组件引用。

删除 RateLimit group 设置、channel affinity include_using_group/context key/cache stats using_group。

删除 subscription upgrade_group、playground group selector、pricing group filter、usage logs group filter、models enable_groups column。

- [ ] **步骤 5：运行 classic 源码测试**

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
- 修改：`i18n/locales/zh.yaml`
- 修改：`web/default/src/i18n/static-keys.ts`
- 修改：`web/default/src/i18n/locales/*.json`（由 i18n sync 更新）
- 修改：README/配置说明中实际包含业务 group option 的文件（通过搜索定位后限定编辑）
- 测试：`docs/business-group-contract-removal.test.ts` 或 `web/default/src/features/business-group-contract-removal.test.ts`

- [ ] **步骤 1：编写 contract 源码测试**

测试读取 OpenAPI、后端 i18n、static keys，禁止业务 group contract，允许 `/api/prefill_group/` 和 allowlist。

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
    for (const term of ['/api/group/', '/api/user/groups', '/api/user/self/groups', 'cross_group_retry', 'upgrade_group', 'enable_groups', 'group_ratio']) {
      assert.equal(api.includes(term), false, `OpenAPI still contains ${term}`)
    }
    assert.equal(api.includes('/api/prefill_group/'), true)
  })
})
```

- [ ] **步骤 2：运行测试确认失败**

运行：

```bash
cd web/default && bunx tsx --test src/features/business-group-contract-removal.test.ts
```

预期：FAIL。

- [ ] **步骤 3：清理 OpenAPI**

删除业务 group endpoints/schema fields：`/api/user/groups`、`/api/user/self/groups`、`/api/group/`、User.group、Token.group/cross_group_retry、Channel.group/groups、Subscription upgrade_group、pricing group_ratio/auto_groups/enable_groups、analytics business group filters。保留 `/api/prefill_group/`。

- [ ] **步骤 4：清理后端 i18n 和 README/配置说明**

搜索业务 group 文案和配置项：`GroupRatio`、`TopupGroupRatio`、`AutoGroups`、`UserUsableGroups`、`DefaultUseAutoGroup`、`cross_group_retry`、`upgrade_group`、`分组倍率`、`自动分组`、`升级分组`。删除或改写为非业务描述。不得删除 protected project branding。

- [ ] **步骤 5：运行 contract 测试**

运行：

```bash
cd web/default && bunx tsx --test src/features/business-group-contract-removal.test.ts
```

预期：PASS。

---

## 任务 9：集成验证与收口

**文件：**
- 修改：必要时补充任何遗漏测试文件
- 不创建新文档，除非上一任务需要更新现有 README/配置说明

- [ ] **步骤 1：运行后端定向测试矩阵**

运行：

```bash
go test ./controller -run 'Test(GroupEndpointsReturnNoopCompatibilityPayloads|GroupOptionsAreAcceptedAsNoopCompatibilityWrites|Token|ConfigGuide|ModelList|GetPricing|UsageAnalytics|AdminAnalytics|PerfMetrics|Log|Subscription|Topup|Waffo|Pancake)' -count=1
go test ./model -run 'Test(ChannelAbilitiesIgnoreBusinessGroups|Subscription|Redemption|UsageAnalytics|AdminAnalytics)' -count=1
go test ./service -run 'Test(GenerateTextOtherInfo|TaskBilling|Channel)' -count=1
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

- [ ] **步骤 5：最终源码可见性搜索**

使用 OMP `search` 工具而不是 shell grep，检查业务 group 残留。允许命中：`prefill_group`、OAuth claim groups、Uptime Kuma groups、UI layout group、SQL `GROUP BY`、CSS/Tailwind group、测试中 allowlist 说明。业务残留必须修复或在计划执行记录中说明为何非业务。

- [ ] **步骤 6：提交**

主代理检查：

```bash
git status -sb --untracked-files=all
git diff --stat
```

确认只包含本任务文件后提交：

```bash
git add controller/group.go router/api-router.go controller/option.go model/option.go setting/ratio_setting/group_ratio.go setting/auto_group.go setting/user_usable_group.go common/topup-ratio.go setting/rate_limit.go model/ability.go model/channel.go model/channel_cache.go model/channel_satisfy.go service/channel_select.go middleware/auth.go middleware/distributor.go relay/common/relay_info.go relay/relay.go controller/model.go controller/config_guide.go controller/user.go model/user.go model/user_cache.go model/subscription.go controller/subscription.go controller/subscription_payment_balance.go model/redemption.go controller/topup.go controller/topup_stripe.go controller/topup_waffo.go controller/topup_waffo_pancake.go relay/helper/price.go service/log_info_generate.go service/task_billing.go model/task.go dto/task.go controller/task_video.go relay/relay_task.go controller/pricing.go model/model_meta.go controller/model_meta.go model/log.go controller/log.go model/perf_metric.go controller/perf_metrics.go dto/usage_analytics.go model/usage_analytics.go controller/usage_analytics.go dto/admin_analytics.go model/admin_analytics.go model/admin_analytics_usage.go model/admin_analytics_drilldown.go controller/admin_analytics.go docs/openapi/api.json i18n/locales/en.yaml i18n/locales/zh.yaml web/default/src web/classic/src docs/superpowers/specs/2026-05-21-remove-ai-channel-groups-design.md docs/superpowers/plans/2026-05-21-remove-business-groups.md
git commit -m "refactor(groups): 移除业务分组概念"
```

- [ ] **步骤 7：最终审查**

分派至少 3 个只读 reviewer 子代理并发审查最终 diff：后端运行时、前端 UI/API、兼容与合同。所有 review pass 后再推送或按用户后续要求收口。
