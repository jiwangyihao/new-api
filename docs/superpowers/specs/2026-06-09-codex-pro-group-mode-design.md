# Codex Pro 分组服务模式设计

## 背景

当前 `new-api` 已支持套餐订阅、钱包余额和多上游 relay。用户希望新增一个 `Codex Pro` 分组服务模式：付费套餐用户可以在控制台手动启用，符合条件的 GPT 系列请求可以尝试由上游 `sub2api` 的 Pro 分组处理；只有请求确实由 Pro 分组 serve 成功后，本次订阅 token 消耗才按 2 倍结算。

这个需求有 3 个关键边界：

1. **控制权在用户控制台**：用户只选择服务模式，不直接传计费倍率。
2. **上游只证明 serve 事实**：`sub2api` 返回「是否实际由 Pro 分组 serve」的 ack，不决定结算倍率。
3. **计费权在 `new-api`**：`new-api` 根据可信 ack 在结算阶段解释为 2x，不允许客户端或通道配置伪造。

现有试用套餐上游标记已采用服务端最终化 Header 的模式：最终出站前先剥离客户端同名 Header，再由 `RelayInfo` 内部状态决定是否写入。Codex Pro 应复用这个安全边界，而不是新增客户端可控参数。

## 目标

1. 新增用户级 `Codex Pro` 分组服务模式，三态为：`全部`、`灵活`、`关闭`。
2. 默认值为 `灵活`。
3. 仅付费套餐用户可启用或使用该能力；无有效付费套餐、试用套餐、钱包-only 场景不享受 Pro 分组。
4. 仅 GPT 系列模型参与 Pro 分组判断，非 GPT 系列模型不打 Pro 请求标记，也不 2x。
5. 只在上游明确返回「实际由 Pro 分组 serve 成功」的可信 ack 后，才按 2 倍订阅 token 结算。
6. 请求失败、上游回退普通分组、ack 缺失或 ack 值不匹配时，一律按普通倍率结算。
7. 补齐下游请求意图、上游请求标记、上游 serve ack 的协议边界。
8. 在控制台合适位置提供用户开关，并在 API 帮助中给出 Codex、Claude Code、OpenCode、Oh-My-Pi、Hermes Agent、OpenClaw 的 Header 配置方式。

## 非目标

- 不实现通用优先级、通用倍率或多等级计费系统。
- 不允许上游返回任意倍率，例如 `1.5`、`2`、`3`。
- 不把 Pro 模式配置放进管理员通道配置或套餐定义里。
- 不改变现有套餐购买、发放、过期、重置或钱包计费语义。
- 不允许客户端通过请求 Header 直接决定是否 2x 计费。
- 不强行修改 OpenAI Responses 的下游响应 body；上游 ack 只用于 `new-api` 内部结算状态。

## 方案选择

### 方案 A：按可信 serve ack 结算（采用）

`new-api` 在满足用户资格、用户模式和模型类型后，向上游发送 Pro 请求标记。`sub2api` 只有在实际由 Pro 分组成功 serve 时返回 ack。`new-api` 只在看到可信 ack 后把本次订阅 token 消耗按 2 倍结算。

优点：

- 符合「只有实际由 Pro 分组 serve 成功才 2x」的业务要求。
- 回退普通分组不会误扣。
- 上游只返回 serve 事实，不参与计费策略。
- 后续倍率从 2x 调整为其他值时，只改 `new-api` 计费策略，不改协议。

缺点：

- 需要 `sub2api` 配合返回 ack。
- 结算逻辑需要记录上游实际 serve 状态。

### 方案 B：按下游 intent 直接 2x 计费

用户启用 Pro 模式后，只要请求符合模型条件并发出 Pro intent，就按 2 倍结算，不依赖上游 ack。

优点：

- 实现简单。
- 不依赖上游响应协议。

缺点：

- 上游回退普通分组时也会 2x，违背用户要求。
- 用户会为未实际享受 Pro 服务的请求付费。

### 方案 C：先接入标记，不接入 2x 结算

先落地用户开关和上下游 Header，只记录 Pro 请求与 ack，暂不改变计费。

优点：

- 风险最低。
- 可先观察上游 ack 稳定性。

缺点：

- 无法完成 2x 结算目标。
- 后续仍要再改计费链路。

结论：本期采用方案 A。

## 用户模式

新增用户级字段 `codex_pro_mode`，枚举值如下：

| 值 | 中文展示 | 语义 |
| --- | --- | --- |
| `all` | 全部 | 符合资格的 GPT 系列请求都发送 Pro 请求 marker，不要求下游 intent Header。 |
| `flexible` | 灵活 | 默认模式。只有符合资格且下游请求带 `X-NewAPI-Codex-Pro-Intent: codex-pro` 的 GPT 系列请求才发送 Pro 请求 marker。 |
| `off` | 关闭 | 不发送 Pro 请求标记，不触发 Pro 2x 结算。 |

默认值为 `flexible`。

### 模式资格

生成 Pro 请求 marker 必须同时满足：

1. 用户当前有有效付费套餐。
2. 实际计费来源是订阅，而不是纯钱包。
3. 套餐不是试用套餐。
4. 请求模型属于 GPT 系列文本模型。
5. 用户模式不是 `off`。
6. 当前请求路径支持读取上游 serve ack。
7. 模式为 `all`；或模式为 `flexible` 且下游请求带 `X-NewAPI-Codex-Pro-Intent: codex-pro`。

其中「付费套餐」以有效订阅关联的 `SubscriptionPlan.price_amount > 0` 且 `is_trial = false` 为基础判断。不能只依赖套餐名称、兑换码来源或客户端声明。`flexible` 模式中的下游 intent 只是弱意图，不能绕过前 6 条资格判断。

## 协议设计

### 下游请求 intent（用户 / harness → new-api）

用户是否允许使用 Pro 模式由控制台持久化设置决定。下游 harness 只能传入弱 intent，不能传入计费倍率，也不能直接传「已由 Pro serve」这类结果字段。

固定下游 intent Header：

```http
X-NewAPI-Codex-Pro-Intent: codex-pro
```

该 Header 的唯一作用是让默认 `flexible` 模式具备按 harness / 按模型配置启用 Pro 的能力。它不能绕过用户开关、付费资格、模型限制，也不能触发 2x 结算。

判定规则：

1. 用户模式为 `off`：忽略该 Header，不发送上游 Pro 请求 marker。
2. 用户模式为 `flexible`：只有该 Header 存在且值为 `codex-pro` 时，才允许进入上游 Pro 请求 marker 判断。
3. 用户模式为 `all`：不要求该 Header；即使没有该 Header，也允许符合资格的 GPT 系列请求尝试 Pro。
4. 客户端传入 `X-NewAPI-Pro-Request` 或 `X-NewAPI-Pro-Served` 一律无效，必须被剥离或忽略。

### 上游请求 marker（new-api → sub2api）

固定上游请求 Header：

```http
X-NewAPI-Pro-Request: codex-pro
```

语义：

- `new-api` 已完成用户资格、用户模式、模型类型和请求路径判断。
- 本次请求希望 `sub2api` 尝试使用 Codex Pro 分组。
- 该 Header 只是请求 intent，不触发 2x 结算。
- Header 名称和值必须由 `new-api` 服务端最终化。
- 客户端、通道 `header_override`、runtime Header override 或 Header passthrough 不能伪造、覆盖或删除最终值。

最终化规则：

1. 最终出站前先按大小写不敏感语义删除任何来源的 `X-NewAPI-Pro-Request`。
2. 仅当内部 `RelayInfo` 判定本次请求允许尝试 Pro 时，写入 `X-NewAPI-Pro-Request: codex-pro`。
3. 其他情况保持 Header 缺失。

### 上游响应 ack（sub2api → new-api）

固定上游响应 Header：

```http
X-NewAPI-Pro-Served: codex-pro
```

语义：

- `sub2api` 确认本次请求实际由 Codex Pro 分组 serve 成功。
- 只有该 Header 存在且值精确等于 `codex-pro` 时，`new-api` 才能把本次订阅 token 消耗解释为 Pro serve。
- 请求回退普通分组、未命中 Pro 分组、失败、超时、取消或未完成 Pro 分组 serve 时，`sub2api` 不得返回该 Header。
- `sub2api` 不返回倍率，不返回金额，不返回用户信息。

### 上游 body ack

本期不使用 body ack。唯一可信的上游 serve ack 是响应 Header：

```http
X-NewAPI-Pro-Served: codex-pro
```

原因：

- Header 不需要修改 OpenAI Responses 的下游 body，兼容非流式、流式和 compact 路径。
- ack 是 `new-api` 与 `sub2api` 的内部结算信号，不面向终端用户展示。
- 如果未来出现无法稳定读取响应 Header 的上游路径，需要单独设计 body ack；不能在本期实现中隐式接受任意 body 字段作为结算凭据。

## 计费设计

### 计费时机

现有流程是：

1. `PreConsumeBilling` 创建 `BillingSession` 并预扣。
2. relay 执行上游请求。
3. handler 解析 usage。
4. `SettleBillingWithInput` / `BillingSession` 根据实际 usage 做结算和退款。

Pro 2x 应放在结算阶段，而不是预扣阶段直接把所有请求按 2x 固定扣费。

结算规则：

```text
settlement_subscription_tokens = actual_subscription_tokens
if pro_served_ack == true:
  settlement_subscription_tokens = actual_subscription_tokens * 2
```

只对订阅 token 生效。钱包 quota、免费模型、无订阅请求和失败退款路径不套用 Pro 2x。

### 预扣与不足处理

预扣阶段保持现有普通倍率，减少误扣。结算阶段若 ack 表明需要 2x，但预扣订阅 token 不足以覆盖最终差额，应使用现有 `SubscriptionPostDelta` 追加订阅扣减；不得把 Pro 订阅差额转嫁到钱包 quota，除非用户的既有扣费策略本来就允许钱包参与补足且现有结算代码已经支持该路径。任何无法完成的差额扣减都必须按现有账务规则返回明确错误或退款，不得静默少扣。

实现计划阶段需要明确验证：

- 订阅 token 预扣不足时，`SubscriptionPostDelta` 能正确表达额外扣减。
- 钱包 quota 不会因为 Pro ack 被错误套用 2x。
- 上游成功但结算失败时，日志与用户可见错误符合现有账务处理惯例。

### 回退与失败

以下场景不 2x：

- 用户模式为 `off`。
- 用户没有有效付费套餐。
- 当前实际使用的是试用套餐。
- 请求模型不是 GPT 系列文本模型。
- 未发送 `X-NewAPI-Pro-Request: codex-pro`。
- 上游没有返回 `X-NewAPI-Pro-Served: codex-pro`。
- 上游返回其他值，例如 `pro`、`true`、`2x`。
- 请求失败、超时、取消、流式未完成。
- 上游回退到普通分组。

## 数据与 API 设计

### 用户设置字段

在 `dto.UserSetting` 中新增字段：

```go
CodexProMode string `json:"codex_pro_mode,omitempty"`
```

持久化继续使用现有用户 `setting` JSON，不新增数据库列。读取设置时按以下规则规范化：

- 空值：`flexible`。
- `all`、`flexible`、`off`：原样保留。
- 其他历史脏值：读取时按 `flexible` 处理，更新接口收到非法值时返回参数错误。

该字段不放进通用 `UpdateUserSettingRequest`。新增订阅域接口：

```http
PUT /api/subscription/self/codex-pro-mode
```

请求体：

```json
{
  "mode": "flexible"
}
```

响应体返回规范化后的模式和资格状态。

### 资格返回

`/api/subscription/self` 响应补充：

```json
{
  "codex_pro_mode": "flexible",
  "codex_pro_eligible": true,
  "codex_pro_unavailable_reason": ""
}
```

`codex_pro_unavailable_reason` 使用固定枚举：

- `no_paid_subscription`
- `trial_subscription`
- `wallet_only`
- `disabled`

前端展示中文时通过 i18n 翻译，不直接展示枚举原文。

## 前端设计

### 主入口

主入口放在 `web/default/src/features/wallet/components/subscription-plans-card.tsx` 或其相邻订阅控制组件中。

理由：

- 该区域已经读取 `getSelfSubscriptionFull` 和 active subscription。
- `Codex Pro` 只对付费套餐用户开放，本质是订阅权益控制。
- `ProfileSettingsCard` 主要是账号绑定、通知、排行榜，不适合作为套餐权益开关主入口。

### 展示规则

- 有资格用户：展示三态选择器和 2x 说明。
- 无资格用户：展示禁用态和原因，例如「仅付费套餐用户可用」。
- 默认选择 `灵活`。
- 切换后立即调用订阅域 API 保存，失败则回滚 UI 状态并提示。

### i18n

新增 UI 文案必须补齐：

- `en`
- `zh`
- `fr`
- `ja`
- `ru`
- `vi`

文案需要明确：

- Pro 分组只对符合条件的 GPT 系列请求生效。
- 只有实际由 Pro 分组 serve 的请求才会产生 2 倍订阅 token 消耗。
- 回退普通分组不加倍。

## Harness 配置引导

API 帮助弹窗应补充以下配置说明，目标是让用户能把 `X-NewAPI-Codex-Pro-Intent: codex-pro` 传给 `new-api`。该 Header 只让默认 `flexible` 模式按 harness 启用 Pro，不是计费凭据。
### Codex CLI

Codex 官方配置支持：

- `model_providers.<id>.base_url`
- `model_providers.<id>.wire_api = "responses"`
- `model_providers.<id>.http_headers`
- `model_providers.<id>.env_http_headers`

示例：

```toml
model = "gpt-5"
model_provider = "newapi"

[model_providers.newapi]
name = "new-api"
base_url = "https://example.com/v1"
wire_api = "responses"
http_headers = { "X-NewAPI-Codex-Pro-Intent" = "codex-pro" }
env_key = "NEWAPI_API_KEY"
```

### Claude Code

Claude Code 官方环境变量支持：

- `ANTHROPIC_BASE_URL`
- `ANTHROPIC_AUTH_TOKEN`
- `ANTHROPIC_CUSTOM_HEADERS`

示例：

```bash
export ANTHROPIC_BASE_URL="https://example.com"
export ANTHROPIC_AUTH_TOKEN="sk-..."
export ANTHROPIC_CUSTOM_HEADERS="X-NewAPI-Codex-Pro-Intent: codex-pro"
```

### OpenCode

OpenCode provider 配置支持 `options.baseURL`，现有生成器也能在模型元数据中注入 `headers`。

示例：

```json
{
  "$schema": "https://opencode.ai/config.json",
  "provider": {
    "newapi": {
      "npm": "@ai-sdk/openai-compatible",
      "name": "new-api",
      "options": {
        "baseURL": "https://example.com/v1",
        "apiKey": "sk-..."
      },
      "models": {
        "gpt-5": {
          "name": "gpt-5",
          "headers": {
            "X-NewAPI-Codex-Pro-Intent": "codex-pro"
          }
        }
      }
    }
  }
}
```

### Oh-My-Pi

现有配置引导生成 `models.yml` / `config.yml`，provider 已有 `baseUrl` / `apiKey`。本期需要扩展 Oh-My-Pi 配置引导，使 provider 或模型级配置支持 headers：

```yaml
providers:
  newapi:
    baseUrl: https://example.com/v1
    apiKey: sk-...
    headers:
      X-NewAPI-Codex-Pro-Intent: codex-pro
```

如果目标 Oh-My-Pi 版本的配置 schema 尚不支持 `headers`，API 帮助必须明确标注「当前版本不支持按配置注入 Header，无法在 `flexible` 模式下按 harness 触发 Pro；可在控制台改用 `全部` 模式」。不得生成看似可用但实际不会生效的配置。

### Hermes Agent

Hermes Agent 官方配置目前稳定支持 `base_url` / `api_key`，未在已核验配置页中提供通用自定义 headers 字段。因此 API 帮助中的 Hermes Agent 项必须按保守策略展示：

- 若当前 Hermes Agent 版本支持主模型 provider 自定义 headers，则给出实际字段名和 `X-NewAPI-Codex-Pro-Intent: codex-pro` 示例。
- 若不支持，则明确提示 `flexible` 模式无法通过 Hermes Agent 配置 Header 触发 Pro，用户可在控制台改用 `全部` 模式。

### OpenClaw

OpenClaw 自定义 provider 支持 `models.providers.*.headers`，也支持 `models.providers.*.request.headers` 作为请求传输覆盖。API 帮助应优先生成 provider 级静态 Header：

```json5
{
  models: {
    providers: {
      "newapi": {
        baseUrl: "https://example.com/v1",
        apiKey: "sk-...",
        api: "openai-responses",
        headers: {
          "X-NewAPI-Codex-Pro-Intent": "codex-pro"
        }
      }
    }
  }
}
```

## 后端实现边界

实现时应保持改动集中：

1. 用户模式枚举和校验函数。
2. 订阅域 API 读取和更新模式。
3. relay 运行时字段，例如：
   - `RelayInfo.CodexProRequestMarker string`
   - `RelayInfo.CodexProServed bool`
4. Header 最终化函数，例如：
   - `FinalizeProRequestHeader(header http.Header, marker string)`
5. Codex Responses handler 解析上游 ack 并写回 `BillingSession` 或等效结算输入。
6. 结算阶段只根据内部 `CodexProServed` 状态对订阅 token 做 2x。

不要把 `X-NewAPI-Pro-Served` 从客户端透传为可信输入。它只应来自上游响应。

## 安全性

- 所有 Pro 内部 Header 都是保留 Header。
- 下游客户端传入 `X-NewAPI-Pro-Request`、`X-NewAPI-Pro-Served` 必须被删除或忽略。
- 通道配置不能覆盖最终内部 Header。
- 上游 ack 不携带用户 ID、订阅 ID、套餐 ID、金额或倍率。
- 结算日志可记录是否 Pro served，但不应把敏感 Header 原样暴露给普通用户。

## 兼容性

- **数据库**：复用现有用户 `setting` JSON，不新增列，不新增迁移。
- **现有用户**：空值和历史缺失值按 `flexible` 读取。
- **现有请求**：没有上游 ack 时保持普通计费。
- **试用套餐**：不参与 Pro 分组和 2x。
- **钱包-only 用户**：不参与 Pro 分组和 2x。
- **非 GPT 模型**：不参与 Pro 分组和 2x。
- **sub2api 未升级**：不会返回 `X-NewAPI-Pro-Served: codex-pro`，因此不会 2x。

## 测试

至少补充以下测试：

1. **用户模式校验测试**
   - `all`、`flexible`、`off` 合法。
   - 空值规范化为 `flexible`。
   - 更新接口收到非法值会返回参数错误；读取历史脏值时按 `flexible` 处理。
2. **资格判断测试**
   - 有效付费套餐可用。
   - 试用套餐、无套餐、钱包-only 不可用。
3. **模型 gating 测试**
   - `gpt-*`、`o1`、`o3`、`o4`、`chatgpt` 类模型可参与。
   - 非 GPT 系列模型不参与。
4. **上游请求 Header 最终化测试**
   - 符合条件时最终写入 `X-NewAPI-Pro-Request: codex-pro`。
   - 不符合条件时缺失。
   - 客户端或通道配置无法伪造、覆盖或删除。
5. **上游 ack 解析测试**
   - 响应 Header 为 `X-NewAPI-Pro-Served: codex-pro` 时记录 Pro served。
   - 缺失、大小写异常值、其他值都不记录。
   - 流式请求只有在 `response.completed` 后才可记录成功 serve。
6. **2x 结算测试**
   - ack 成功时订阅 token 按 2x 结算。
   - ack 缺失、失败、回退普通分组时按 1x。
   - 钱包 quota 不被错误 2x。
   - 订阅预扣不足时补扣 / 退款行为符合现有账务规则。
7. **前端测试**
   - 有资格用户可切换三态。
   - 无资格用户看到禁用态和原因。
   - 保存失败会回滚 UI。
8. **配置引导测试**
   - Codex、Claude Code、OpenCode、Oh-My-Pi、Hermes Agent、OpenClaw 帮助文案覆盖对应 Header 能力或明确限制。
   - i18n 同步后 6 种语言均无缺失 key。

## 验收标准

- 用户可以在订阅 / 钱包区域设置 `Codex Pro` 模式：`全部`、`灵活`、`关闭`。
- 默认模式为 `灵活`。
- 付费套餐用户发起 GPT 系列请求时，只有 `all` 模式，或 `flexible` 模式且下游带 `X-NewAPI-Codex-Pro-Intent: codex-pro` 时，`new-api` 才向上游发送 `X-NewAPI-Pro-Request: codex-pro`。
- 客户端和通道配置不能伪造 `X-NewAPI-Pro-Request` 或 `X-NewAPI-Pro-Served`。
- `sub2api` 返回 `X-NewAPI-Pro-Served: codex-pro` 时，本次订阅 token 消耗按 2x 结算。
- ack 缺失、请求失败、回退普通分组或非 GPT 模型时不 2x。
- 钱包 quota、试用套餐、无订阅请求不被 Pro 2x 影响。
- API 帮助中提供各 harness 的 Header 配置方式，并明确 Header 只是 intent，不是计费凭据。
- 所有新增前端文案完成 `en`、`zh`、`fr`、`ja`、`ru`、`vi` 翻译。
