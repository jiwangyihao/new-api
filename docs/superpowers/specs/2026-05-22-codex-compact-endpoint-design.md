# Codex compact 端点支持修复设计

## 背景

用户反馈当前站点不支持 Codex compact 端点：

- `GET /v1/models` 中目标模型只显示 `supported_endpoint_types=["openai-response"]`，没有 `openai-response-compact`。
- `POST /v1/responses`，`model=gpt-5.5` 返回 `200 OK`。
- `POST /v1/responses/compact`，`model=gpt-5.5` 返回 `503`。
- 错误信息为：`No available channel for model gpt-5.5-openai-compact under group default (distributor)`。

本设计对比 `~/Documents/GitHub/workbench` 中相关处理，修复当前仓库中 compact 路由、模型能力曝光、渠道选择和计费模型之间的边界错误。

## 当前证据

当前仓库已有 compact relay 能力：

- `router/relay-router.go` 将 `POST /v1/responses/compact` 路由到 `RelayFormatOpenAIResponsesCompaction`。
- `relay/responses_handler.go` 允许 `APITypeOpenAI` 和 `APITypeCodex` 使用 compact relay mode。
- `relay/channel/codex/adaptor.go` 在 `RelayModeResponsesCompact` 下会请求 `/backend-api/codex/responses/compact`。

失败发生在 relay/adaptor 之前。`middleware/distributor.go` 会在 compact 路径上提前改写模型名：

```go
if strings.HasPrefix(c.Request.URL.Path, "/v1/responses/compact") && modelRequest.Model != "" {
    modelRequest.Model = ratio_setting.WithCompactModelSuffix(modelRequest.Model)
}
```

因此客户端请求 `POST /v1/responses/compact`，`model=gpt-5.5` 会在渠道选择前变成 `gpt-5.5-openai-compact`。`model.GetRandomSatisfiedChannel()` 再按这个模型名查 `abilities` / channel cache。线上能力表只有 `gpt-5.5`，没有 `gpt-5.5-openai-compact`，所以 distributor 返回 `503`。

`GET /v1/models` 的 `supported_endpoint_types` 来自 `model/pricing.go` 对启用能力的聚合。该逻辑调用 `common.GetEndpointTypesByChannelType(ability.ChannelType, ability.Model)`，而当前 `common/endpoint_type.go` 没有为 Codex 渠道返回 `openai-response-compact`，因此模型能力曝光也缺少 compact 端点。

还需要注意 `model/pricing.go` 的覆盖规则：先聚合默认 endpoint，然后读取 `models.endpoints`；如果模型元数据 endpoints 非空，会替换默认 endpoint，不做合并。因此已有模型元数据若显式只配置 `openai-response`，仅修改 `common.GetEndpointTypesByChannelType()` 不会自动让该模型暴露 compact。

## workbench 对比

`workbench/repos/sub2api-private-reference/backend` 的边界是：

- compact 是请求路径能力，不是对外模型名能力。
- `internal/handler/endpoint.go` 保留 `/v1/responses/compact` 子路径。
- `internal/handler/openai_gateway_handler.go` 用 `isOpenAIRemoteCompactPath(c)` 得到 `requireCompact`，再把它传给调度。
- `internal/service/openai_model_mapping.go` 只在 compact 请求转发上游前调用 `resolveOpenAICompactForwardModel()`。

也就是：客户端模型名保持稳定；路径决定是否需要 compact；调度按原始模型和 compact capability 选账号/渠道；上游转发前才做 compact 专用模型映射。

### 本次不做完整 workbench parity

本次是最小可靠修复，不引入完整账号级 compact 能力系统：

- 不实现 `openai_compact_supported` 探测。
- 不新增 scheduler 级 compact capability filter。
- 不新增独立 `compact_model_mapping` 配置，继续复用当前仓库已有 `model_mapping`，但只能在 relay compact mode 下影响上游模型。
- 验收口径是：请求不再在 distributor 阶段因 `*-openai-compact` 伪模型失败；若所选上游不支持 compact，错误应来自 relay 或上游，而不是 distributor 选不到渠道。

## 根因

当前仓库混用了三种语义：

1. `openai-response-compact` 是 endpoint type，表示模型可通过 `/v1/responses/compact` 调用。
2. `-openai-compact` 是 compact 计费或上游映射使用的内部模型后缀。
3. `original_model` / `RelayInfo.OriginModelName` 当前同时参与渠道选择、token model limit、重试、日志、param override 和计费。

直接 503 根因是：内部 compact 后缀模型名过早泄漏到 distributor，导致渠道选择按 `gpt-5.5-openai-compact` 查找能力表。

另一个必须同时处理的风险是计费。`controller.Relay` 在调用 `relay.ResponsesHelper` 之前会先执行 `helper.ModelPriceHelper()` 和 `service.PreConsumeBilling()`。如果只删除 distributor 的提前后缀，首次预扣会按普通 `gpt-5.5` 定价，而不是 compact 价格。现有 `relay/helper/model_mapped.go` 在 relay 阶段才把 `info.OriginModelName` 改成 `WithCompactModelSuffix(finalUpstreamModelName)`，该时机晚于首次预扣。

## 设计原则

后续实现必须区分以下模型名：

| 名称 | 含义 | 使用范围 |
| --- | --- | --- |
| 客户端模型 | 请求体中的 `model`，例如 `gpt-5.5` | token model limit、distributor 选渠、channel affinity、retry 选渠、面向客户端的日志和错误文案 |
| 上游模型 | 实际发往 Codex/OpenAI 的 `model` | `ModelMappedHelper` 和 adaptor 转换后的请求体 |
| compact 计费模型 | `gpt-5.5-openai-compact` 或 `<mapped>-openai-compact` | `ModelPriceHelper`、预扣、结算、billing snapshot |
| endpoint type | `openai-response-compact` | `/v1/models`、pricing API、前端/客户端能力发现 |

约束：

- `ContextKeyOriginalModel` / Gin context 中的 `original_model` 应表示客户端模型。
- `RelayInfo.UpstreamModelName` 表示最终上游模型。
- compact 计费不能依赖永久污染 `ContextKeyOriginalModel`。
- 如果短期继续复用 `RelayInfo.OriginModelName` 作为 `ModelPriceHelper()` 输入，必须严格限定临时切换作用域，并覆盖错误返回、重试和恢复路径。

## 目标

- `/v1/responses/compact` 使用客户端模型名选择渠道。
- token model limit、channel affinity、retry 选渠均使用客户端模型名。
- compact 请求首次预扣和最终结算使用明确的 compact billing model。
- Codex 渠道模型在 `/v1/models` 中暴露 `openai-response-compact`。
- Codex 默认公开模型列表不再默认生成 `*-openai-compact` 伪模型。
- 普通 `/v1/responses` 行为不变。
- 不要求管理员为同一个模型额外配置 `-openai-compact` 伪模型能力。

## 非目标

- 不重构完整调度系统。
- 不引入账号探测任务或后台能力扫描。
- 不调整 OpenAI/Codex OAuth key 格式。
- 不改变普通 `/v1/chat/completions`、`/v1/responses`、图片、音频、embedding 等端点行为。
- 不删除现有 `-openai-compact` 定价后缀机制；只修正使用边界。
- 不承诺 OpenAI、Azure、XAI 等非 Codex 渠道在本次统一暴露 compact endpoint。当前仓库缺少账号级 compact 探测；但本次必须补 endpoint-aware channel filter，确保已暴露 compact 的模型不会随机选到不支持 compact 的渠道。

## 推荐方案

### 1. 渠道选择使用原始模型名

删除 `middleware/distributor.go` 中针对 `/v1/responses/compact` 的模型名后缀追加逻辑。

### 2. compact 计费用独立 billing model

实现必须在首次 `ModelPriceHelper()` / `PreConsumeBilling()` 前解析 compact billing model，不再依赖 distributor 改写模型名。

推荐策略：

1. compact 请求的选渠模型始终是客户端模型。
2. compact 请求的 billing model 使用 `WithCompactModelSuffix(...)`：如果可在预扣前安全解析 channel model mapping，则使用 `WithCompactModelSuffix(mappedUpstreamModel)`；否则使用 `WithCompactModelSuffix(clientModel)` 作为预扣 billing model。
3. relay/adaptor 阶段如果 mapping 后上游模型不同，最终结算应使用最终 compact billing model，并按现有 billing session 语义补差或结算。
4. 普通 `/v1/responses` 不使用 compact billing model。
5. 缺少 compact price 且不允许未配置模型价格时，应在预扣阶段给出明确错误，不应先转发上游再在结算阶段失败。

### 3. Codex 渠道声明 compact endpoint

在 `common.GetEndpointTypesByChannelType()` 中为 `constant.ChannelTypeCodex` 增加显式分支：

```go
case constant.ChannelTypeCodex:
    endpointTypes = []constant.EndpointType{
        constant.EndpointTypeOpenAIResponse,
        constant.EndpointTypeOpenAIResponseCompact,
    }
```

非 Codex 渠道保持现有规则，尤其：XAI 仍为 `openai` + `openai-response`；普通 OpenAI 文本模型仍默认 `openai`；response-only 模型仍默认 `openai-response`；图片模型仍前置 `image-generation`。

### 4. 保留 `models.endpoints` 覆盖语义

本次不改变 `models.endpoints` 非空时替换默认 endpoint 的现有语义：

- 无自定义 endpoints 的 Codex 模型：由 `ChannelTypeCodex` 默认规则暴露 `openai-response-compact`。
- 有自定义 endpoints 的模型：管理员配置必须显式包含 `openai-response-compact`，否则仍按自定义配置输出。

### 5. 收敛 Codex 默认公开模型列表

`relay/channel/codex/constants.go` 当前 `ModelList = withCompactModelSuffix(baseModelList)`，会把每个 Codex 默认模型扩展出 `*-openai-compact`。这与新边界冲突。

推荐修改：

- `codex.ModelList` 默认只包含客户端模型名。
- 删除默认 `withCompactModelSuffix()` 扩展，或仅保留为内部 legacy helper，不能用于公开 `ModelList`。
- 不自动删除线上已存在的 `*-openai-compact` ability，但不得继续由默认列表生成新的 suffixed 模型。
- `/v1/models` 默认不应把 `*-openai-compact` 作为普通模型暴露，除非管理员显式保留 legacy 配置。

### 6. channel test 对齐新边界

`controller/channel-test.go` 当前 compact 测试路径也会提前把测试模型改成 `WithCompactModelSuffix(testModel)`。需要调整：

- 显式 `endpoint_type=openai-response-compact` 或请求路径为 `/v1/responses/compact` 时，请求路径使用 compact endpoint，但客户端模型保持原始模型名。
- channel test 不应默认把 `gpt-5.5` 改成 `gpt-5.5-openai-compact` 再调用 `SetupContextForSelectedChannel()`。
- legacy 输入本身已经带 `-openai-compact` 时，可先归一化为客户端模型用于选渠，再在 billing/upstream 阶段按 compact 规则处理。

### 7. endpoint-aware channel filter

必须在渠道选择中加入 endpoint-aware 过滤，而不是只在 `/v1/models` 暴露 compact：

- compact 请求的 required endpoint type 为 `openai-response-compact`。
- distributor 初选、preferred channel / affinity 命中校验、auto group、retry 选渠都必须只接受明确支持该 endpoint type 的渠道。
- 支持关系以 `common.GetEndpointTypesByChannelType(channel.Type, clientModel)` 和模型元数据 endpoints 覆盖后的有效 endpoint 为准。
- DB direct path 和 memory cache path 都必须应用同一过滤语义。
- 同一 group 同一模型同时存在 Codex 和非 compact 渠道时，compact 请求只能选择 Codex（或其他明确支持 compact 的渠道），不能随机落到 relay 会拒绝的 API type。
- 如果某个用户可用 group 下没有支持 compact 的渠道，则不要向该用户暴露该模型的 `openai-response-compact`；如果仍发起 compact 请求，应返回明确的无 compact 可用渠道错误，而不是选到普通 responses 渠道后再由 relay 拒绝。

### 8. 兼容性选择

默认不加入 legacy fallback。若后续确认线上只配置了 `gpt-5.5-openai-compact` 能力，可补短期 fallback：先按原始模型选渠，找不到再按 `WithCompactModelSuffix(model)` 选渠；该 fallback 必须有注释，且不能改变 token model limit 的主授权边界。

## 数据流

```text
客户端
  POST /v1/responses/compact model=gpt-5.5
    ↓
distributor
  选渠模型 = gpt-5.5
  token model limit = gpt-5.5
    ↓
channel cache / abilities
  查找 default + gpt-5.5
    ↓
选中 Codex 渠道
    ↓
controller.Relay
  RelayModeResponsesCompact
  billing model = gpt-5.5-openai-compact 或 <mapped>-openai-compact
    ↓
relay / ModelMappedHelper / adaptor
  UpstreamModelName = 映射后的上游模型
  request path = /backend-api/codex/responses/compact
```

## 测试计划

### 1. endpoint type 单元测试

为 `common.GetEndpointTypesByChannelType()` 增加表驱动测试：

- `ChannelTypeCodex + gpt-5.5` 包含且只包含 `openai-response`、`openai-response-compact`，不包含 `openai`。
- `ChannelTypeXai + 任意模型` 仍为 `openai` + `openai-response`。
- 普通 OpenAI 文本模型仍为 `openai`。
- `o3-pro` 等 response-only 模型仍为 `openai-response`。
- 图片模型仍前置 `image-generation`。

### 2. distributor 选渠测试

构造 Gin router：`Distribute()` 后接测试 handler，返回 `original_model`、`channel_id`、`channel_type`。

测试数据：SQLite 中只创建 enabled Codex channel，只创建 `default + gpt-5.5` ability，明确不创建 `gpt-5.5-openai-compact` ability。

子测试：

- `common.MemoryCacheEnabled=false`：`POST /v1/responses/compact` + `{"model":"gpt-5.5"}` 不返回 503，选中 Codex channel，`original_model == "gpt-5.5"`，响应/错误不包含 `gpt-5.5-openai-compact`。
- `common.MemoryCacheEnabled=true`：调用 `model.InitChannelCache()` 后重复同样断言。
- token model limit：limit 只允许 `gpt-5.5` 时 compact 请求通过；limit 只允许 `gpt-5.5-openai-compact` 时请求被拒绝。

### 3. retry / affinity / param override 边界测试

构造 compact 请求并触发一次可重试 channel error，验证：retry 选渠仍按 `gpt-5.5`；context `original_model` 保持 `gpt-5.5`；param override 上下文中的 `original_model` 不出现 `-openai-compact`。若完整上游 retry 难以低成本构造，至少对 `controller.getChannel()` / `SetupContextForSelectedChannel()` 做单元级边界测试。

### 4. `/v1/models` 真实展示路径测试

在 `controller/model_list_test.go` 或同等位置测试，可复用 `setupModelListControllerTestDB()`：

- 创建 enabled Codex channel + `model=gpt-5.5` ability。
- 调用 `model.RefreshPricing()` 或 `model.GetPricing()` 初始化 pricing cache。
- 普通分组路径：`ListModels` 响应中 `gpt-5.5.supported_endpoint_types` 包含 `openai-response` 和 `openai-response-compact`。
- token model limit 路径：`ContextKeyTokenModelLimitEnabled=true` 且 limit 只含 `gpt-5.5` 时，响应同样包含 compact。
- endpoints 覆盖路径：无自定义 endpoints 时默认暴露 compact；自定义 endpoints 只含 `openai-response` 时按覆盖语义不自动补 compact；自定义 endpoints 显式包含 compact 时输出包含 compact。

### 5. Codex 默认模型列表测试

为 `relay/channel/codex` 增加测试：`ModelList` 包含 base model（如 `gpt-5.4`），默认不包含 `gpt-5.4-openai-compact`。

### 6. channel test compact 路径测试

为 `controller/channel-test.go` 增加测试：Codex channel 测试模型 `gpt-5.5` 且显式 `endpoint_type=openai-response-compact` 时，请求路径为 `/v1/responses/compact`，但传给 `SetupContextForSelectedChannel()` / `GenRelayInfo()` 的客户端模型保持 `gpt-5.5`。

### 7. compact 计费边界测试

配置普通模型和 compact wildcard / exact suffixed price 不同，覆盖：

- compact 请求首次预扣命中 compact price，而不是普通 `gpt-5.5` price。
- 普通 `/v1/responses` 不命中 compact price。
- `ModelMappedHelper` 后 `UpstreamModelName` 为上游真实模型；最终结算使用 compact billing model，并在错误路径恢复原始上下文。
- 缺少 compact price 且不允许未配置价格时，在预扣阶段失败。

### 8. endpoint-aware 多渠道选择测试

覆盖同一模型存在混合渠道的情况：

- 同一 group 内同时存在 Codex channel 和普通 OpenAI/XAI/其他非 compact channel，均声明 `gpt-5.5` ability；`/v1/responses/compact` 必须选中 Codex 或其他明确支持 compact 的渠道，不能选到 relay 会拒绝的 API type。
- 跨 group 场景：只有部分 group 有 Codex compact 可用渠道时，`/v1/models` 和 `/api/pricing` 对用户可用 group 的 compact 曝光必须与该用户实际可选渠道一致。
- DB direct path 与 memory cache path 都要覆盖。
- preferred channel / affinity 命中普通 responses 渠道时，compact 请求必须跳过或拒绝该命中，不能绕过 endpoint-aware filter。

### 9. `/api/pricing` endpoint map 测试

覆盖 pricing API 的两个输出面：

- 创建 Codex `gpt-5.5` ability 后调用 pricing controller，断言模型项的 `supported_endpoint_types` 包含 `openai-response-compact`。
- 断言顶层 `supported_endpoint["openai-response-compact"]` 为 `{"path":"/v1/responses/compact","method":"POST"}`。
- 自定义 `models.endpoints` 只含 `openai-response` 时，不自动补 compact。
- 自定义 endpoints 显式包含 `openai-response-compact` 时，`supported_endpoint` 使用该自定义 path/method；如果自定义值只给 endpoint key 而未给 method，则 method 按现有默认 `POST` 语义。

## 手动验证

修复后使用同一个可用 Codex 渠道验证：

```text
POST /v1/responses
model = gpt-5.5
```

预期：保持 `200 OK`。

```text
POST /v1/responses/compact
model = gpt-5.5
```

预期：不再返回 distributor 的 `No available channel for model gpt-5.5-openai-compact`。如果上游支持 compact，应返回 `200 OK`；如果上游不支持，错误来自 relay 或上游。

```text
GET /v1/models
```

预期：无自定义 endpoints 覆盖的 Codex `gpt-5.5.supported_endpoint_types` 至少包含 `openai-response` 和 `openai-response-compact`。

`/v1/models.supported_endpoint_types` 是模型可用端点类型信号。客户端可将 `openai-response-compact` 映射到内置 `/v1/responses/compact`，或通过 `/api/pricing.supported_endpoint` 获取 path/method。该字段是模型级聚合能力，不是某个具体渠道的承诺，因此服务端必须保证渠道选择不会把 compact 请求发给不支持 compact 的渠道。

## 风险与缓解

### 风险 1：预扣计费回退到普通模型

删除 distributor 后缀后，首次 `ModelPriceHelper()` 可能按普通模型计价。

缓解：实现 compact billing model 解析，并用自动化测试断言首次预扣命中 compact price。

### 风险 2：模型元数据 endpoints 覆盖默认能力

已有 `models.endpoints` 可能只配置 `openai-response`。

缓解：保留覆盖语义并在测试中覆盖；生产配置需要显式加入 `openai-response-compact`。

### 风险 3：Codex 默认模型列表继续泄漏 suffixed 模型

`ModelList` 当前默认生成 `*-openai-compact`。

缓解：收敛默认列表，并测试默认不包含 suffixed 模型。

### 风险 4：多渠道同模型选到不支持 compact 的渠道

当前选渠按 group + model，不按 endpoint type 过滤。

缓解：本次必须实现 endpoint-aware channel filter，并用 DB/cache、同 group、跨 group、affinity 命中测试覆盖，确保 compact 请求不会落到不支持 compact 的渠道。

### 风险 5：pricing API 漏掉 compact path/method

`/api/pricing` 同时返回模型项的 `supported_endpoint_types` 和顶层 `supported_endpoint` path/method map。

缓解：新增 pricing controller 测试，覆盖默认 compact endpoint map 和自定义 endpoints 覆盖语义。

## 验收标准

- `/v1/responses/compact model=gpt-5.5` 不再在 distributor 阶段查找 `gpt-5.5-openai-compact`。
- token model limit、retry、affinity 和 param override 使用客户端模型名。
- compact 请求首次预扣和最终结算使用 compact billing model。
- 已配置 `gpt-5.5` 的 Codex 渠道可以承接 compact 请求。
- `/v1/models` 对无自定义 endpoints 覆盖的 Codex 支持模型返回 `openai-response-compact`。
- Codex 默认 `ModelList` 不再公开 `*-openai-compact` 伪模型。
- `/v1/responses model=gpt-5.5` 行为保持不变。
- 新增测试覆盖 endpoint type、distributor DB/cache、token model limit、`/v1/models`、`/api/pricing`、channel test、compact billing、Codex 模型列表、endpoint-aware channel filter 和多渠道能力边界。
