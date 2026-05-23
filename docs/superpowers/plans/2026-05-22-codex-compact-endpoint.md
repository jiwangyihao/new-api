# Codex compact 端点支持修复实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 修复 Codex `/v1/responses/compact` 因 distributor 使用 `*-openai-compact` 伪模型选渠导致的 503，并补齐模型能力曝光、endpoint-aware 选渠、compact 计费和渠道测试边界。

**架构：** 将客户端模型、上游模型、compact billing model 和 endpoint type 分层。distributor、token model limit、retry、affinity 始终使用客户端模型；compact 计费在首次预扣前解析独立 billing model；渠道选择新增 endpoint-aware 过滤，避免已暴露 compact 的模型落到不支持 compact 的渠道。

**技术栈：** Go 1.22+、Gin、GORM、SQLite 测试库、项目 JSON 包装器 `common.Marshal` / `common.Unmarshal`。

---

## 规格来源

- 规格文件：`docs/superpowers/specs/2026-05-22-codex-compact-endpoint-design.md`
- 本计划直接在主分支工作，不使用 git worktree。
- 实现必须遵循 TDD：每个任务先写失败测试，验证红灯后再改生产代码。

## 相关文件与职责

- 修改：`common/endpoint_type.go` — 为 Codex 声明 `openai-response` 与 `openai-response-compact`，保留非 Codex 规则。
- 测试：`common/endpoint_type_test.go` — endpoint type 最小不变量测试。
- 修改：`model/ability.go` — DB direct path 的 endpoint-aware 选渠查询/过滤；先过滤有效 endpoint，再计算 priority/retry。
- 修改：`model/channel_cache.go` — memory cache path 的 endpoint-aware 选渠过滤。
- 修改：`service/channel_select.go` — `RetryParam` 携带 required endpoint type，并在 auto group、retry、普通 group 选渠时传递。
- 修改：`middleware/distributor.go` — 删除 compact 路径提前追加 `-openai-compact`；根据请求路径设置 required endpoint type；preferred channel / affinity 命中校验必须支持 endpoint-aware 过滤。
- 测试：新增 `middleware/distributor_compact_test.go` 或在现有 middleware 测试文件中增加测试 — 覆盖 DB/cache、token model limit、混合渠道与 affinity。
- 修改：`relay/channel/codex/constants.go` — `ModelList` 只暴露 base model，不再默认生成 `*-openai-compact`。
- 测试：`relay/channel/codex/constants_test.go` — Codex 默认模型列表不暴露 suffixed 模型。
- 修改：`controller/channel-test.go` — compact channel test 不再在选渠/context 阶段提前追加 suffix。
- 测试：`controller/channel_test_compact_test.go` 或现有 `controller/channel-test` 相关测试 — 覆盖显式 compact endpoint 测试模型保持客户端模型名。
- 修改：`relay/common/relay_info.go`、`relay/helper/price.go`、`relay/helper/model_mapped.go`、`controller/relay.go`、`service/text_quota.go`、`service/billing_session.go` — 在 `relay/common` 中增加独立 billing model 状态与 resolver，在首次预扣、tiered snapshot 和最终结算中使用，避免污染 client model context。
- 修改：`controller/relay.go`、`relay/responses_handler.go` — 预扣与最终结算使用 compact billing model；错误路径恢复上下文。
- 测试：新增/扩展 relay/helper 或 controller 测试 — 覆盖 compact 首次预扣、普通 responses 不命中 compact price、缺 compact price 预扣失败、错误恢复。
- 修改：`model/pricing.go`、`controller/model.go`、`controller/pricing.go`、`controller/model_meta.go` — 在模型能力曝光和管理员模型元数据展示中按用户可用 group/metadata 规则重算 endpoint types，保留 `models.endpoints` 覆盖语义，并确保 `/api/pricing.supported_endpoint` 只由当前响应中可见 endpoint types 派生。
- 测试：`controller/model_list_test.go`、新增 `controller/pricing_compact_test.go`、`controller/model_meta_compact_test.go` 或扩展现有 model_meta 测试 — 覆盖 `/v1/models`、`/api/pricing` 与 model metadata endpoints 展示。

---

### 任务 1：锁定 endpoint type 与 Codex 模型列表边界

**文件：**
- 修改：`common/endpoint_type.go`
- 创建或修改：`common/endpoint_type_test.go`
- 修改：`relay/channel/codex/constants.go`
- 创建：`relay/channel/codex/constants_test.go`

- [ ] **步骤 1：编写 endpoint type 失败测试**

在 `common/endpoint_type_test.go` 中添加表驱动测试：

```go
package common

import (
    "testing"

    "github.com/QuantumNous/new-api/constant"
    "github.com/stretchr/testify/assert"
)

func TestGetEndpointTypesByChannelType_CodexCompact(t *testing.T) {
    got := GetEndpointTypesByChannelType(constant.ChannelTypeCodex, "gpt-5.5")

    assert.ElementsMatch(t, []constant.EndpointType{
        constant.EndpointTypeOpenAIResponse,
        constant.EndpointTypeOpenAIResponseCompact,
    }, got)
    assert.NotContains(t, got, constant.EndpointTypeOpenAI)
}

func TestGetEndpointTypesByChannelType_ExistingRulesUnchanged(t *testing.T) {
    assert.ElementsMatch(t, []constant.EndpointType{
        constant.EndpointTypeOpenAI,
        constant.EndpointTypeOpenAIResponse,
    }, GetEndpointTypesByChannelType(constant.ChannelTypeXai, "grok-4"))

    assert.Equal(t, []constant.EndpointType{constant.EndpointTypeOpenAI},
        GetEndpointTypesByChannelType(constant.ChannelTypeOpenAI, "gpt-4o"))

    assert.Equal(t, []constant.EndpointType{constant.EndpointTypeOpenAIResponse},
        GetEndpointTypesByChannelType(constant.ChannelTypeOpenAI, "o3-pro"))

    imageEndpoints := GetEndpointTypesByChannelType(constant.ChannelTypeOpenAI, "gpt-image-1")
    assert.NotEmpty(t, imageEndpoints)
    assert.Equal(t, constant.EndpointTypeImageGeneration, imageEndpoints[0])
    assert.Contains(t, imageEndpoints, constant.EndpointTypeOpenAI)
}
```

- [ ] **步骤 2：运行 endpoint type 测试验证失败**

运行：

```bash
go test ./common -run 'TestGetEndpointTypesByChannelType' -count=1
```

预期：`TestGetEndpointTypesByChannelType_CodexCompact` 失败，因为当前 Codex 走默认逻辑，不返回 `openai-response-compact`。

- [ ] **步骤 3：实现 Codex endpoint type**

在 `common/endpoint_type.go` 的 `switch channelType` 中增加 Codex 分支：

```go
case constant.ChannelTypeCodex:
    endpointTypes = []constant.EndpointType{
        constant.EndpointTypeOpenAIResponse,
        constant.EndpointTypeOpenAIResponseCompact,
    }
```

不要修改 XAI、response-only、image 规则。

- [ ] **步骤 4：运行 endpoint type 测试验证通过**

运行：

```bash
go test ./common -run 'TestGetEndpointTypesByChannelType' -count=1
```

预期：PASS。

- [ ] **步骤 5：编写 Codex ModelList 失败测试**

创建 `relay/channel/codex/constants_test.go`：

```go
package codex

import (
    "testing"

    "github.com/QuantumNous/new-api/setting/ratio_setting"
    "github.com/stretchr/testify/assert"
)

func TestModelListDoesNotExposeCompactSuffixModels(t *testing.T) {
    assert.Contains(t, ModelList, "gpt-5.4")
    assert.NotContains(t, ModelList, ratio_setting.WithCompactModelSuffix("gpt-5.4"))
}
```

- [ ] **步骤 6：运行 Codex ModelList 测试验证失败**

运行：

```bash
go test ./relay/channel/codex -run TestModelListDoesNotExposeCompactSuffixModels -count=1
```

预期：FAIL，因为当前 `ModelList` 包含 `gpt-5.4-openai-compact`。

- [ ] **步骤 7：收敛 Codex ModelList**

修改 `relay/channel/codex/constants.go`：

```go
var ModelList = baseModelList
```

删除不再使用的 `ratio_setting` 与 `lo` import，以及 `withCompactModelSuffix` 函数。如果后续实现需要 legacy helper，保留未导出 helper 也可以，但不能用于 `ModelList`。

- [ ] **步骤 8：运行任务 1 测试**

运行：

```bash
go test ./common -run 'TestGetEndpointTypesByChannelType' -count=1
go test ./relay/channel/codex -run TestModelListDoesNotExposeCompactSuffixModels -count=1
```

预期：全部 PASS。

---

### 任务 2：实现 endpoint-aware 渠道选择

**文件：**
- 修改：`model/ability.go`
- 修改：`model/channel_cache.go`
- 修改：`service/channel_select.go`
- 修改：`middleware/distributor.go`
- 测试：`middleware/distributor_compact_test.go`（新建）

- [ ] **步骤 1：编写 distributor compact 失败测试基础设施**

创建 `middleware/distributor_compact_test.go`。测试使用 SQLite in-memory DB，迁移 `model.Channel`、`model.Ability`、`model.Model`。提供 helper：

```go
func setupDistributorCompactTestDB(t *testing.T) *gorm.DB
func insertCompactTestChannel(t *testing.T, id int, channelType int, models string, group string)
func performDistributedCompactRequest(t *testing.T, memoryCache bool, tokenLimit map[string]bool) *httptest.ResponseRecorder
```

helper 合同：`insertCompactTestChannel` 只插入 channel，不自动创建 ability，避免与测试手写 ability 主键冲突；`performDistributedCompactRequest(memoryCache=true)` 必须保存并恢复 `common.MemoryCacheEnabled`，发请求前调用 `model.InitChannelCache()`；每个测试使用独立 shared-memory SQLite DSN，`t.Cleanup` 关闭 DB、恢复 cache 开关并避免 channel cache 污染后续测试。

测试 router：

```go
r := gin.New()
r.Use(func(c *gin.Context) {
    common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
    common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
    if tokenLimit != nil {
        common.SetContextKey(c, constant.ContextKeyTokenModelLimitEnabled, true)
        common.SetContextKey(c, constant.ContextKeyTokenModelLimit, tokenLimit)
    }
})
r.Use(Distribute())
r.POST("/v1/responses/compact", func(c *gin.Context) {
    c.JSON(http.StatusOK, gin.H{
        "original_model": c.GetString("original_model"),
        "channel_id": c.GetInt("channel_id"),
        "channel_type": c.GetInt("channel_type"),
    })
})
```

- [ ] **步骤 2：编写 DB/cache 失败测试**

添加测试：

```go
func TestDistributeResponsesCompactUsesClientModelForChannelSelection(t *testing.T) {
    for _, memoryCache := range []bool{false, true} {
        t.Run(fmt.Sprintf("memory_cache_%t", memoryCache), func(t *testing.T) {
            db := setupDistributorCompactTestDB(t)
            insertCompactTestChannel(t, 101, constant.ChannelTypeCodex, "gpt-5.5", "default")
            require.NoError(t, db.Create(&model.Ability{Group: "default", Model: "gpt-5.5", ChannelId: 101, Enabled: true}).Error)
            // Do not create gpt-5.5-openai-compact ability.

            rec := performDistributedCompactRequest(t, memoryCache, nil)
            require.Equal(t, http.StatusOK, rec.Code)
            assert.Contains(t, rec.Body.String(), `"original_model":"gpt-5.5"`)
            assert.Contains(t, rec.Body.String(), `"channel_id":101`)
            assert.NotContains(t, rec.Body.String(), "gpt-5.5-openai-compact")
        })
    }
}
```

预期当前失败：因为 distributor 查 `gpt-5.5-openai-compact`。

同时添加自定义 endpoints 子测试：为 `gpt-5.5` 写入 `model.Model{Endpoints: {"openai-response":"/v1/responses"}}` 时，compact 请求应没有可用 compact 渠道；显式包含 `openai-response-compact` 后才允许选中 Codex。使用 `common.Marshal` 构造 endpoints JSON。

- [ ] **步骤 3：编写 token model limit 失败测试**

添加测试：

```go
func TestDistributeResponsesCompactTokenLimitUsesClientModel(t *testing.T) {
    setupDistributorCompactTestDB(t)
    insertCompactTestChannel(t, 101, constant.ChannelTypeCodex, "gpt-5.5", "default")
    require.NoError(t, model.DB.Create(&model.Ability{Group: "default", Model: "gpt-5.5", ChannelId: 101, Enabled: true}).Error)

    allowed := performDistributedCompactRequest(t, false, map[string]bool{"gpt-5.5": true})
    require.Equal(t, http.StatusOK, allowed.Code)

    denied := performDistributedCompactRequest(t, false, map[string]bool{ratio_setting.WithCompactModelSuffix("gpt-5.5"): true})
    require.Equal(t, http.StatusForbidden, denied.Code)
}
```

预期当前失败：允许原始模型时会被后缀改写导致拒绝或选渠失败。

- [ ] **步骤 4：编写混合渠道 endpoint-aware 失败测试**

添加测试：

```go
func TestDistributeResponsesCompactSkipsNonCompactChannels(t *testing.T) {
    for _, memoryCache := range []bool{false, true} {
        t.Run(fmt.Sprintf("memory_cache_%t", memoryCache), func(t *testing.T) {
            setupDistributorCompactTestDB(t)
            insertCompactTestChannel(t, 201, constant.ChannelTypeOpenAI, "gpt-5.5", "default")
            insertCompactTestChannel(t, 202, constant.ChannelTypeCodex, "gpt-5.5", "default")
            require.NoError(t, model.DB.Create(&[]model.Ability{
                {Group: "default", Model: "gpt-5.5", ChannelId: 201, Enabled: true},
                {Group: "default", Model: "gpt-5.5", ChannelId: 202, Enabled: true},
            }).Error)

            rec := performDistributedCompactRequest(t, memoryCache, nil)
            require.Equal(t, http.StatusOK, rec.Code)
            assert.Contains(t, rec.Body.String(), `"channel_id":202`)
        })
    }
}
```

设置 Codex 与非 compact 渠道同优先级同权重时，为避免随机性，测试 helper 可把非 compact 权重设高，仍应选 Codex；或通过 endpoint-aware filter 后只剩 Codex。另加子测试：非 compact 渠道 priority 高于 Codex compact 渠道时，compact 请求仍选中较低 priority 的 Codex，因为 priority/retry 必须在过滤后的候选集合上计算。

- [ ] **步骤 5：运行 distributor 测试验证失败**

运行：

```bash
go test ./middleware -run 'TestDistributeResponsesCompact' -count=1
```

预期：至少 DB/cache 基础测试失败。

- [ ] **步骤 6：扩展 RetryParam 携带 required endpoint type**

在 `service/channel_select.go` 的 `RetryParam` 增加字段：

```go
RequiredEndpointType constant.EndpointType
```

新增 helper：

```go
func (p *RetryParam) RequiredEndpoint() constant.EndpointType {
    if p == nil {
        return ""
    }
    return p.RequiredEndpointType
}
```

- [ ] **步骤 7：为 model 选渠增加 endpoint-aware API**

在 `model/model_meta.go` 或 `model/pricing.go` 附近先增加共享有效 endpoint helper，随后在 `model/ability.go` 和 `model/channel_cache.go` 增加新选渠函数，保留旧函数兼容其他调用：

```go
func GetRandomSatisfiedChannelWithEndpoint(group string, modelName string, retry int, endpointType constant.EndpointType) (*Channel, error)
func GetChannelWithEndpoint(group string, modelName string, retry int, endpointType constant.EndpointType) (*Channel, error)
```

endpoint 为空时行为等同旧函数。

有效 endpoint helper 必须应用 `models.endpoints` 覆盖语义，不是可选项：

```go
func GetEffectiveEndpointTypes(channelType int, modelName string) []constant.EndpointType {
    // 若模型元数据 endpoints 非空且有效：返回 endpoints 中的 key。
    // 否则：返回 common.GetEndpointTypesByChannelType(channelType, modelName)。
}

func ChannelSupportsEndpoint(channel *Channel, modelName string, endpointType constant.EndpointType) bool {
    if endpointType == "" {
        return true
    }
    return slices.Contains(GetEffectiveEndpointTypes(channel.Type, modelName), endpointType)
}
```

实现可在 pricing refresh / channel cache init 时预加载模型元数据，或使用已有 pricing cache；不要在热路径每次查 DB。`GetEffectiveEndpointTypes` 必须使用与 `updatePricing` 相同的模型元数据匹配语义：exact、prefix、suffix、contains、status 与有效 endpoints JSON 解析规则一致。DB direct、memory cache、preferred channel / affinity、retry、auto group、`/v1/models`、`/api/pricing` 必须使用同一有效 endpoint 语义。新增至少一个 prefix/suffix/contains metadata endpoints 覆盖测试，验证选渠与曝光一致。

- [ ] **步骤 8：memory cache path 过滤 endpoint**

在 `GetRandomSatisfiedChannelWithEndpoint` 的 memory cache 分支中，从 candidate channel IDs 中筛除不支持 required endpoint 的 channel。注意优先级计算应基于过滤后的候选。

- [ ] **步骤 9：DB direct path 过滤 endpoint**

在 `GetChannelWithEndpoint` 中先取得 group/model 的所有 enabled ability + channel 候选，再根据有效 endpoint 过滤，最后基于过滤后的候选计算 priority/retry 目标层和权重随机。不要先用旧 `getChannelQuery()` 选最高 priority 后再过滤，否则高优先级非 compact 渠道会把低优先级 Codex compact 渠道误删为“无渠道”。normalized model fallback 也必须走同一流程。不要用数据库特定 SQL join 做复杂条件，避免破坏 SQLite/MySQL/PostgreSQL 兼容。

- [ ] **步骤 10：service 层传递 required endpoint**

修改 `CacheGetRandomSatisfiedChannel`，调用：

```go
model.GetRandomSatisfiedChannelWithEndpoint(group, param.ModelName, retry, param.RequiredEndpoint())
```

包括 auto group 分支和普通 group 分支。

- [ ] **步骤 11：distributor 设置 required endpoint 并删除后缀改写**

在 `middleware/distributor.go` 删除 compact path 的：

```go
modelRequest.Model = ratio_setting.WithCompactModelSuffix(modelRequest.Model)
```

新增 path 到 endpoint helper：

```go
func requiredEndpointTypeForRequestPath(path string) constant.EndpointType {
    if strings.HasPrefix(path, "/v1/responses/compact") {
        return constant.EndpointTypeOpenAIResponseCompact
    }
    return ""
}
```

创建 `RetryParam` 时设置 `RequiredEndpointType`。

preferred channel / affinity 命中时，除了 enabled 和 group/model 判断，还必须检查该 channel 支持 required endpoint；不支持则跳过该 preferred channel。

- [ ] **步骤 12：运行任务 2 测试验证通过**

运行：

```bash
go test ./middleware -run 'TestDistributeResponsesCompact' -count=1
```

预期：PASS。

---

### 任务 3：修复 compact 计费模型边界

**前置阅读：** 本任务涉及 tiered/dynamic billing，开始前必须阅读 `pkg/billingexpr/expr.md`，并按其中“one expression, one truth”和 snapshot 设计维护 billing model 语义。

**文件：**
- 修改：`relay/common/relay_info.go`
- 修改：`relay/helper/price.go` 或新增 `relay/helper/billing_model.go`
- 修改：`relay/helper/model_mapped.go`
- 修改：`service/text_quota.go`
- 修改：`service/billing_session.go`
- 修改：`controller/relay.go`
- 修改：`relay/responses_handler.go`
- 测试：新增 `relay/helper/compact_billing_test.go`、`service/text_quota`/billing session 相关测试或相关 controller/relay 测试

- [ ] **步骤 1：编写 billing model helper 失败测试**

新增测试验证 helper 行为：

```go
func TestWithCompactBillingModelSuffix(t *testing.T) {
    assert.Equal(t, "gpt-5.5-openai-compact", relaycommon.WithCompactBillingModelSuffix("gpt-5.5"))
    assert.Equal(t, "gpt-5.5-openai-compact", relaycommon.WithCompactBillingModelSuffix("gpt-5.5-openai-compact"))
}

func TestResolveBillingModelDefaultsToOriginModel(t *testing.T) {
    info := &relaycommon.RelayInfo{OriginModelName: "gpt-5.5"}

    assert.Equal(t, "gpt-5.5", relaycommon.ResolveBillingModelName(info))
}

func TestResolveBillingModelForResponsesCompactUsesMappedUpstreamWhenSet(t *testing.T) {
    info := &relaycommon.RelayInfo{
        OriginModelName:  "gpt-5.5",
        BillingModelName: "upstream-gpt-openai-compact",
    }

    assert.Equal(t, "upstream-gpt-openai-compact", relaycommon.ResolveBillingModelName(info))
}

func TestResolveBillingModelForResponsesNormal(t *testing.T) {
    info := &relaycommon.RelayInfo{
        OriginModelName: "gpt-5.5",
        ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gpt-5.5"},
    }

    assert.Equal(t, "gpt-5.5", relaycommon.ResolveBillingModelName(info))
}
```

- [ ] **步骤 2：运行 helper 测试验证失败**

运行：

```bash
go test ./relay/common -run TestResolveBillingModel -count=1
```

预期：FAIL，函数不存在。

- [ ] **步骤 3：实现 billing model helper**

实现：

```go
const compactModelSuffix = "-openai-compact"

func WithCompactBillingModelSuffix(modelName string) string {
    if modelName == "" || strings.HasSuffix(modelName, compactModelSuffix) {
        return modelName
    }
    return modelName + compactModelSuffix
}

func ResolveBillingModelName(info *RelayInfo) string {
    if info == nil {
        return ""
    }
    if strings.TrimSpace(info.BillingModelName) != "" {
        return info.BillingModelName
    }
    return info.OriginModelName
}
```

在 `relay/common` 中实现 `WithCompactBillingModelSuffix` 和 `ResolveBillingModelName(info *RelayInfo) string` 以避免 import cycle，并新增 `RelayInfo.BillingModelName string`。依赖边界写死：`relay/common` 不得 import `setting/ratio_setting`、`relay/helper` 或高层 service/controller 包；compact suffix 在 `relay/common` 用本地常量 `"-openai-compact"`（或下沉到另一个无依赖包）实现，测试 `go test ./relay/common` 必须覆盖无 import cycle。预扣前由 controller 在 compact mode 下设置 `BillingModelName`，普通 responses 不设置；`ModelMappedHelper` 在 compact mode 下只设置 `UpstreamModelName` 和 mapped `BillingModelName`，不得再永久把 `OriginModelName` 改成 suffixed billing model。`relay/helper/price.go`、`service/text_quota.go`、`service/billing_session.go`、`relay/responses_handler.go` 全部调用 `relaycommon.ResolveBillingModelName`，不要从 service 反向 import `relay/helper`。

首次预扣 mapped model 合同：在 `controller.Relay` 完成 `GenRelayInfo` 并选中 channel 后、首次 `helper.ModelPriceHelper` / `service.PreConsumeBilling` 前，读取当前选中 channel 的 `ModelMapping`。从 `relay/helper/model_mapped.go` 提取纯解析函数（例如 `ResolveMappedModelName(modelName string, mapping string) (string, bool)`，放在不会形成循环依赖的位置；该函数不得修改 request、context 或 `OriginModelName`）。compact mode 下若能解析到 mapped upstream model，则设置 `info.BillingModelName = relaycommon.WithCompactBillingModelSuffix(mappedModel)`；否则设置 `info.BillingModelName = relaycommon.WithCompactBillingModelSuffix(info.OriginModelName)`。普通 responses 不设置 compact billing suffix。补测试：只配置 `upstream-gpt-openai-compact` 价格且 channel mapping `gpt-5.5:upstream-gpt` 时，首次预扣成功并命中 mapped compact price；只配置 `gpt-5.5-openai-compact` 时 fallback 成功；普通 responses 使用客户端/正常 mapped 计费模型，不带 compact suffix。

- [ ] **步骤 4：让 ModelPriceHelper 使用 billing model**

修改 `relay/helper/price.go`，不要直接多处使用 `info.OriginModelName` 查价格。最小安全改法：

```go
billingModelName := relaycommon.ResolveBillingModelName(info)
```

然后 `GetModelPrice`、`billing_setting.GetBillingMode`、`billing_setting.GetBillingExpr`、`GetModelRatio`、completion/cache/image/audio ratio 都使用 `billingModelName`。`modelPriceHelperTiered` 的 snapshot `ModelName` 也必须写 billing model。错误消息可包含 billing model，但 Gin context `original_model` 与 `RelayInfo.OriginModelName` 保持客户端模型。

- [ ] **步骤 5：编写 compact price 失败测试**

配置模型 ratio/price，使普通与 compact 不同。测试 `RelayModeResponsesCompact + OriginModelName=gpt-5.5 + BillingModelName=gpt-5.5-openai-compact` 调用 `ModelPriceHelper` 命中 compact price；`RelayModeResponses` 命中普通 price。必须使用统一 `snapshotCompactEndpointTestGlobals(t)` helper 保存并恢复 ratio/price/tiered/self-use/cache 等全局状态，并在 cleanup 中 `model.InvalidatePricingCache()`。

伪代码：

```go
func TestModelPriceHelperResponsesCompactUsesCompactBillingModel(t *testing.T) {
    restore := snapshotCompactEndpointTestGlobals(t)
    t.Cleanup(restore)
    require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"gpt-5.5":1,"*-openai-compact":9}`))
    require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{}`))

    c, _ := gin.CreateTestContext(httptest.NewRecorder())
    info := &relaycommon.RelayInfo{
        RelayMode: relayconstant.RelayModeResponsesCompact,
        OriginModelName: "gpt-5.5",
        BillingModelName: "gpt-5.5-openai-compact",
        ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gpt-5.5"},
    }

    priceData, err := ModelPriceHelper(c, info, 1000, &types.TokenCountMeta{})
    require.NoError(t, err)
    assert.Equal(t, 9.0, priceData.ModelRatio)
}
```

- [ ] **步骤 6：运行 compact price 测试验证失败/通过**

先运行看到失败，再完成 `ModelPriceHelper` 修改后运行：

```bash
go test ./relay/common -run TestResolveBillingModel -count=1
go test ./relay/helper -run TestModelPriceHelperResponsesCompact -count=1
go test ./service -run 'Test.*Compact.*Billing|Test.*BillingModel' -count=1
```

预期：PASS。

- [ ] **步骤 7：更新 ModelMappedHelper 与最终结算消费者**

修改 `relay/helper/model_mapped.go`：compact mode 下不得再执行 `info.OriginModelName = WithCompactModelSuffix(...)`。它应设置 `info.UpstreamModelName` 和 `info.BillingModelName = WithCompactModelSuffix(finalUpstreamModelName)`。

更新 `service/text_quota.go`、`service/billing_session.go`、tiered snapshot 使用方：计费、结算、subscription funding model 使用 billing model 或已冻结的 `PriceData` / `TieredBillingSnapshot`；日志/param override/token limit/retry 使用客户端模型 `OriginModelName`。

当前 `relay/responses_handler.go` compact 响应后会保存 `originModelName` 再调用 `ModelPriceHelper`。在 ModelPriceHelper 支持 billing model 后，避免把 `OriginModelName` 临时作为 billing model；错误路径仍需恢复 `info.PriceData`、`BillingModelName` 等计费状态。

- [ ] **步骤 8：补 tiered 与结算边界测试**

新增测试：

- compact tiered_expr 使用 suffixed billing model 的 expression，并写入 `TieredBillingSnapshot.ModelName`。
- `ModelMappedHelper` 后 `OriginModelName` 仍为 `gpt-5.5`，`UpstreamModelName` 为 mapped upstream，`BillingModelName` 为 `<mapped>-openai-compact`。
- `BuildParamOverrideContext` 中 `original_model` 为客户端模型，`upstream_model` 为上游模型。
- 缺少 compact price 且 `AcceptUnsetRatioModel=false` 时，预扣阶段失败，不用普通模型价格放行。

- [ ] **步骤 9：运行相关 relay 测试**

运行：

```bash
go test ./relay/common -run TestResolveBillingModel -count=1
go test ./relay/helper -run TestModelPriceHelperResponsesCompact -count=1
go test ./service -run 'Test.*Compact.*Billing|Test.*BillingModel' -count=1
go test ./relay -run ResponsesCompact -count=1
```

预期：PASS。如果 `./relay -run ResponsesCompact` 没有测试，记录 no tests 或 PASS 输出即可。

---

### 任务 4：修复 `/v1/models` 与 `/api/pricing` compact 能力曝光

**文件：**
- 修改：`model/pricing.go`
- 修改：`controller/model.go`
- 修改：`controller/pricing.go`（按用户过滤 supported_endpoint）
- 修改：`controller/model_meta.go`（JSON wrapper 与 endpoints 展示语义）
- 测试：`controller/model_list_test.go`
- 测试：新增 `controller/pricing_compact_test.go`
- 测试：新增 `controller/model_meta_compact_test.go` 或扩展现有 model_meta 测试

- [ ] **步骤 1：编写 `/v1/models` 失败测试**

在 `controller/model_list_test.go` 中添加测试：

```go
func TestListModelsIncludesCodexCompactEndpoint(t *testing.T) {
    db := setupModelListControllerTestDB(t)
    require.NoError(t, db.Create(&model.User{Id: 2001, Username: "codex_user", Group: "default", Status: common.UserStatusEnabled}).Error)
    require.NoError(t, db.Create(&model.Channel{Id: 2101, Type: constant.ChannelTypeCodex, Status: common.ChannelStatusEnabled, Models: "gpt-5.5", Group: "default", Name: "codex"}).Error)
    require.NoError(t, db.Create(&model.Ability{Group: "default", Model: "gpt-5.5", ChannelId: 2101, Enabled: true}).Error)
    restore := snapshotCompactEndpointTestGlobals(t)
    t.Cleanup(restore)
    require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"gpt-5.5":1}`))

    model.InvalidatePricingCache()
    model.GetPricing()

    rec := httptest.NewRecorder()
    c, _ := gin.CreateTestContext(rec)
    c.Set("id", 2001)
    ListModels(c, constant.ChannelTypeOpenAI)

    ids := decodeListModelsResponse(t, rec)
    require.Contains(t, ids, "gpt-5.5")
    var payload listModelsResponse
    require.NoError(t, common.Unmarshal(rec.Body.Bytes(), &payload))
    var endpoints []constant.EndpointType
    for _, item := range payload.Data {
        if item.Id == "gpt-5.5" {
            endpoints = item.SupportedEndpointTypes
            break
        }
    }
    assert.Contains(t, endpoints, constant.EndpointTypeOpenAIResponse)
    assert.Contains(t, endpoints, constant.EndpointTypeOpenAIResponseCompact)
}
```

同时添加 token model limit 子测试：limit 只包含 `gpt-5.5`，响应中同样包含 compact。

- [ ] **步骤 2：编写 endpoints 覆盖测试**

添加测试：

- `model.Model{ModelName:"gpt-5.5", Endpoints: {"openai-response":"/v1/responses"}}` 时不自动补 compact。
- Endpoints 显式包含 `openai-response-compact` 时输出包含 compact。

使用 `common.Marshal` 构造 JSON 字符串，禁止直接 `encoding/json.Marshal`。

- [ ] **步骤 3：运行 model list 测试验证失败**

运行：

```bash
go test ./controller -run 'TestListModels.*Compact|TestListModelsIncludesCodexCompactEndpoint' -count=1
```

预期：compact endpoint 缺失或覆盖测试失败。

- [ ] **步骤 4：调整 pricing/model exposure 逻辑**

在 `model/pricing.go` 中保留 endpoints 覆盖语义。Codex 默认能力应因任务 1 的 `GetEndpointTypesByChannelType` 自动出现 compact。若 token model limit 路径仍缺失，确保 `GetModelSupportEndpointTypes` 在 pricing cache 初始化后可返回。

必须按用户/group 做 endpoint-aware 过滤：新增 `GetModelSupportEndpointTypesForGroups(modelName, groups)` 或 controller 层等价逻辑。`/v1/models` 和 `/api/pricing` 返回前按当前用户可用 group 重新计算每个模型 endpoint types；顶层 `supported_endpoint` 由当前响应中实际出现的 endpoint types 派生，而不是全局 `model.GetSupportedEndpointMap()`。补跨 group 测试：default 只有 OpenAI、vip 有 Codex 时，default 用户不看到 compact，vip 或 auto 可用组包含 Codex 时才看到 compact。

- [ ] **步骤 5：编写 `/api/pricing` 失败测试**

新增 `controller/pricing_compact_test.go`，复用 `controller/pricing_directory_test.go` 中的 setup helper 或显式保存/恢复 ratio 配置。测试必须设置 `ratio_setting.UpdateGroupRatioByJSONString({"default":1})`、`ratio_setting.UpdateModelRatioByJSONString({"gpt-5.5":1})`，创建 enabled user/channel/ability，`model.InvalidatePricingCache(); model.GetPricing()` 后调用 controller。

```go
func TestGetPricingIncludesCodexCompactSupportedEndpoint(t *testing.T) {
    db := setupModelListControllerTestDB(t)
    restore := snapshotCompactEndpointTestGlobals(t)
    t.Cleanup(restore)
    require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))
    require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"gpt-5.5":1}`))

    const userID = 2201
    require.NoError(t, db.Create(&model.User{Id: userID, Username: "pricing_codex", Group: "default", Status: common.UserStatusEnabled}).Error)
    require.NoError(t, db.Create(&model.Channel{Id: 2202, Type: constant.ChannelTypeCodex, Status: common.ChannelStatusEnabled, Models: "gpt-5.5", Group: "default", Name: "codex"}).Error)
    require.NoError(t, db.Create(&model.Ability{Group: "default", Model: "gpt-5.5", ChannelId: 2202, Enabled: true}).Error)
    model.InvalidatePricingCache()
    model.GetPricing()

    rec := httptest.NewRecorder()
    c, _ := gin.CreateTestContext(rec)
    c.Set("id", userID)
    GetPricing(c)

    // Decode payload; find data item model_name == gpt-5.5 and assert supported_endpoint_types contains compact.
    // Decode supported_endpoint["openai-response-compact"].path/method and assert /v1/responses/compact + POST.
}
```

- [ ] **步骤 6：实现 `/api/pricing` endpoint map 保证**

不要用全局 `model.GetSupportedEndpointMap()` 的内容作为 controller 可见性验收。`common/endpoint_defaults.go` 可作为默认 endpoint 元信息来源；若自定义 endpoints 显式覆盖 compact path/method，保持现有 `model/pricing.go` 解析语义，但 `controller/pricing.go` 的顶层 `supported_endpoint` 必须从当前响应过滤后的 pricing items 收集 endpoint type 后生成。

- [ ] **步骤 7：修复 model metadata endpoints 展示与 JSON wrapper**

`controller/model_meta.go` 是管理员配置/展示模型元数据 endpoints 的入口，必须纳入本任务而不是只做最终人工检查：

- 业务路径中的 `json.Marshal` / `json.Unmarshal` 改为 `common.Marshal` / `common.Unmarshal`；仅允许保留 `json.RawMessage`、`json.Number` 等类型引用。
- exact model 与规则模型（prefix/suffix/contains）展示 endpoints 时，调用与选渠、pricing 同源的 effective endpoint helper 或等价封装，确保默认 Codex compact、自定义 `models.endpoints` 覆盖、status 过滤与 `updatePricing` 语义一致。
- 新增或扩展 controller/model_meta 测试：
  - exact Codex `gpt-5.5` 默认展示 response + compact；
  - 自定义 endpoints 只含 `openai-response` 时不展示 compact；
  - prefix/suffix/contains 规则模型在显式 endpoints 覆盖时展示与 pricing/选渠一致；
  - 测试使用 `common.Unmarshal` 解码响应，不新增 direct `encoding/json` marshal/unmarshal 调用。

- [ ] **步骤 8：运行任务 4 测试**

运行：

```bash
go test ./controller -run 'TestListModels.*Compact|TestGetPricingIncludesCodexCompact|TestModelMeta.*Compact' -count=1
```

预期：PASS。

---

### 任务 5：修复 channel test compact 路径

**文件：**
- 修改：`controller/channel-test.go`
- 测试：新增或修改 controller channel test 单元测试

- [ ] **步骤 1：编写 channel test 失败测试**

为 `testChannel` 的真实路径添加测试。不要只测试未接入的新 helper。若直接调用 `testChannel` 会发上游请求，先提取一个返回 `requestPath` 与 selection model 的小函数，并让 `testChannel` 实际使用它，例如：

```go
func normalizeCompactChannelTestModelForSelection(modelName string, endpointType string, requestPath string) string
```

测试：

```go
func TestChannelTestCompactEndpointKeepsClientModelForSelection(t *testing.T) {
    got := normalizeCompactChannelTestModelForSelection("gpt-5.5", string(constant.EndpointTypeOpenAIResponseCompact), "/v1/responses/compact")
    assert.Equal(t, "gpt-5.5", got)
}
```

测试必须覆盖该 helper 已接入 `testChannel` 的调用点；或通过 `httptest` / fake adaptor seam 覆盖 `SetupContextForSelectedChannel` 后 context model。显式 compact endpoint 和 legacy suffixed 输入都要覆盖。

- [ ] **步骤 2：运行 channel test 验证失败**

运行：

```bash
go test ./controller -run 'TestChannelTestCompactEndpointKeepsClientModel' -count=1
```

预期：FAIL 或 helper 不存在。

- [ ] **步骤 3：修改 channel-test compact 逻辑**

删除或限制以下提前改写：

```go
if strings.HasPrefix(requestPath, "/v1/responses/compact") {
    testModel = ratio_setting.WithCompactModelSuffix(testModel)
}
```

显式 compact endpoint 只改变 requestPath / relayFormat，不改变用于 `SetupContextForSelectedChannel` 和 `GenRelayInfo` 的客户端模型。legacy suffixed 输入可以先 trim suffix 作为 client model。

- [ ] **步骤 4：运行 channel test**

运行：

```bash
go test ./controller -run 'TestChannelTestCompactEndpointKeepsClientModel' -count=1
```

预期：PASS。

---

### 任务 6：补 retry、affinity 与多渠道集成边界

**文件：**
- 修改：`middleware/distributor.go`
- 修改：`controller/relay.go`
- 修改：`service/channel_select.go`
- 测试：`middleware/distributor_compact_test.go` 或相关 service/controller 测试

- [ ] **步骤 1：编写 affinity 命中非 compact 渠道测试**

在 distributor compact 测试中构造。测试 helper 还必须设置并清理 `ContextKeyTokenGroup`，因为 `GenRelayInfo` / relay retry 会从该 context 读取 token group：

- channel 301：OpenAI，model `gpt-5.5`。
- channel 302：Codex，model `gpt-5.5`。
- 通过 service affinity 机制或直接上下文模拟 preferred channel 为 301。

断言 compact 请求不会选 301；应选 302 或返回明确错误。

- [ ] **步骤 2：编写 retry 保持 client model 测试**

优先增加 controller/relay 层测试或可测试 seam：构造 `/v1/responses/compact` 的 `RelayInfo` 后触发一次可重试错误，断言第二次 `getChannel` / `CacheGetRandomSatisfiedChannel` 收到 `RequiredEndpointType=openai-response-compact` 且 `ModelName=gpt-5.5`。如果完整 HTTP relay 难构造，至少提取从 `GenRelayInfo` + relay mode 构造 retryParam 的 helper 并直接测试；不能只手写 service 层 `RetryParam`。

- [ ] **步骤 3：运行边界测试验证失败**

运行：

```bash
go test ./middleware -run 'TestDistributeResponsesCompact.*Affinity|TestDistributeResponsesCompact.*Retry' -count=1
```

预期：当前至少 affinity/required endpoint 未过滤。

- [ ] **步骤 4：完善 preferred channel / affinity endpoint 校验**

在 `middleware/distributor.go` 的 preferred channel 命中逻辑中，增加：

```go
if requiredEndpoint != "" && !model.ChannelSupportsEndpoint(preferred, modelRequest.Model, requiredEndpoint) {
    // skip preferred for this request
}
```

具体 helper 名称按任务 2 实现确定。

- [ ] **步骤 5：确保 retryParam 始终带 required endpoint**

在 `controller/relay.go` retry loop 中构造 `RetryParam` 时，从 `relayInfo.RelayMode` 或 context 取 required endpoint。新增 helper 并测试：

```go
RequiredEndpointType: relaycommon.RequiredEndpointTypeForRelayMode(relayInfo.RelayMode)
```

或复用 middleware 设置在 context 中的值。不要让 retry 退回普通模型选渠。

- [ ] **步骤 6：运行任务 6 测试**

运行：

```bash
go test ./middleware -run 'TestDistributeResponsesCompact' -count=1
go test ./service -run 'Test.*Compact.*Retry' -count=1
```

预期：相关测试 PASS。若 `./service` 无对应测试，确保 middleware 覆盖 retry/affinity 即可。

---

### 任务 7：最终聚合验证

**文件：**
- 无新增生产代码；只运行已修改包的聚合测试。

- [ ] **步骤 1：运行相关包测试**

运行：

```bash
go test ./common -run 'TestGetEndpointTypesByChannelType' -count=1
go test ./relay/channel/codex -run TestModelListDoesNotExposeCompactSuffixModels -count=1
go test ./middleware -run 'TestDistributeResponsesCompact' -count=1
go test ./controller -run 'TestListModels.*Compact|TestGetPricingIncludesCodexCompact|TestModelMeta.*Compact|TestChannelTestCompactEndpointKeepsClientModel' -count=1
go test ./relay/common -run TestResolveBillingModel -count=1
go test ./relay/helper -run TestModelPriceHelperResponsesCompact -count=1
go test ./service -run 'Test.*Compact.*Billing|Test.*BillingModel' -count=1
```

预期：全部 PASS。

- [ ] **步骤 2：检查没有直接使用 `encoding/json` 新增 marshal/unmarshal**

检查本次新增/修改代码：业务代码 JSON marshal/unmarshal 必须使用 `common.Marshal` / `common.Unmarshal`。测试里也优先使用 `common.Marshal`，避免引入项目规则违规。

- [ ] **步骤 3：人工检查关键不变量**

确认：

- `middleware/distributor.go` 不再在 compact path 下调用 `WithCompactModelSuffix` 改写选渠模型。
- `codex.ModelList` 不再默认包含 `*-openai-compact`。
- compact billing model 不污染 Gin context `original_model`。
- endpoint-aware filter 覆盖 DB/cache、preferred channel、retry。
- `/v1/models` 与 `/api/pricing` 均能暴露 compact endpoint，且遵守 `models.endpoints` 覆盖语义。

- [ ] **步骤 4：准备代码审查上下文**

记录实际变更文件、运行过的测试命令和结果，供后续请求代码审查。

- [ ] **步骤 5：检查 JSON wrapper 约定**

本次触碰 `model/pricing.go`、`relay/helper/model_mapped.go`、`controller/model_meta.go` 等业务文件时，若存在 `json.Unmarshal` / `json.Marshal` 实际调用，替换为 `common.Unmarshal` / `common.Marshal`。只允许保留 `json.RawMessage` 等类型引用。`controller/model_meta.go` 的 endpoints 展示/管理路径必须与新的 effective endpoint helper 同源或语义一致，避免管理员配置页看到与选渠/pricing 不一致的 endpoints。最终检查本次触碰业务文件中没有新增或保留违规 marshal/unmarshal 调用。
