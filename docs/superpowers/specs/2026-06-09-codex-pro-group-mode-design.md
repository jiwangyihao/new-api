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
8. 在控制台合适位置提供用户开关，并在 API 帮助中给出 Codex、Claude Code、OpenCode、Oh My Pi / OMP、Hermes Agent、OpenClaw 的 Header 配置方式。

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

1. 当前请求实际创建的是订阅 `BillingSession`，不是纯钱包或免费请求。
2. 该 `BillingSession` 选中的 `UserSubscription` 是当前 active、未过期、未耗尽的可计费订阅。
3. 关联 `SubscriptionPlan.price_amount > 0`，且 `is_trial = false`、`invite_trial = false`。
4. `UserSubscription.grant_reason` / `source` 不属于 `trial_code`、`invite_trial`、`monthly_invite_entitlement` 等非销售赠送来源。
5. 请求模型属于 GPT 系列文本模型。
6. 用户模式不是 `off`。
7. 当前请求路径支持读取上游 serve ack。
8. 模式为 `all`；或模式为 `flexible` 且下游请求带 `X-NewAPI-Codex-Pro-Intent: codex-pro`。

其中「付费套餐资格」必须落在本次请求实际选中的订阅实例上，不能只按套餐定义、套餐名称、兑换码来源或客户端声明判断。实现应复用订阅预扣 / 主计费订阅选择逻辑，或抽出与其等价的只读 helper，确保资格判断与实际计费来源一致。管理员售后分配的有价订阅按付费等价处理；试用、邀请试用、邀请奖励等非销售赠送权益不算付费等价。`flexible` 模式中的下游 intent 只是弱意图，不能绕过前 7 条资格判断。

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
2. 最终化必须晚于 adaptor 默认 Header、通道 `header_override`、`param_override` 的 `set_header` / `delete_header` / `pass_headers`、runtime Header override 和客户端 Header passthrough。
3. 仅当内部 `RelayInfo` 判定本次请求允许尝试 Pro 时，写入 `X-NewAPI-Pro-Request: codex-pro`。
4. 其他情况保持 Header 缺失。

### 上游响应 ack（sub2api → new-api）

固定上游响应 trailer：

```http
X-NewAPI-Pro-Served: codex-pro
```

语义：

- `sub2api` 用该 response trailer 声明本次响应由 Codex Pro 分组 serve。
- `new-api` 只能在完整消费 body / stream 到 EOF 后读取 `resp.Trailer`；普通 response Header 中的 `X-NewAPI-Pro-Served` 必须忽略，不能作为 ack。
- 最终状态机固定为 `CodexProServed = proRequestSent && handlerSuccess && trailerAck`。
- 非流式 / compact 请求必须成功解析 usage 且没有 upstream error；流式请求必须收到 `response.completed`、正常读到 EOF，且流状态没有错误。
- 请求回退普通分组、未命中 Pro 分组、失败、超时、取消、客户端中断、流式未完成、上游 error 或最终结算失败时，都不得按 Pro served 处理。
- `sub2api` 不返回倍率，不返回金额，不返回用户信息。
- `new-api` 发送 `X-NewAPI-Pro-Request: codex-pro` 时需要请求上游保留 response trailer，例如 HTTP/1.1 场景设置 `TE: trailers`；部署链路中的反向代理也必须保留 response trailer。

### 上游 body ack

本期不使用 body ack。唯一可作为候选信号的上游 serve ack 是响应 trailer：

```http
X-NewAPI-Pro-Served: codex-pro
```

原因：

- Trailer 在响应体结束后才可可靠读取，天然匹配「完整消费 body / stream 后才能确认」的结算语义。
- 普通 response Header 到达时间早于 `response.completed` 和 EOF，不能证明请求成功完成，因此不再作为候选 ack。
- ack 是 `new-api` 与 `sub2api` 的内部结算信号，不面向终端用户展示。
- 如果未来需要 body ack，必须单独设计；不能在本期实现中隐式接受任意 body 字段作为结算凭据。

### ack 响应过滤

`X-NewAPI-Pro-Served` 是内部结算 trailer。`new-api` 读取 trailer ack 后，必须确保写给终端用户的响应 Header 和 trailer 都不包含该字段。

该过滤必须覆盖：

- Codex Responses 非流式路径；
- Codex Responses 流式 SSE 路径；
- Codex Responses compact 路径。

终端用户不应从响应 Header 或 trailer 中看到 `X-NewAPI-Pro-Served`，也不能依赖该字段判断计费结果。

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
- 当前实际使用的是试用套餐或非销售赠送权益。
- 请求模型不是 GPT 系列文本模型。
- 未发送 `X-NewAPI-Pro-Request: codex-pro`。
- 上游 response trailer 没有返回 `X-NewAPI-Pro-Served: codex-pro`。
- 上游 response trailer 返回其他值，例如 `pro`、`true`、`2x`。
- 请求失败、超时、取消、流式未完成。
- 上游回退到普通分组。
- 上游请求重试后由普通分组成功；失败尝试中的候选 ack 不得污染最终结算。

每次上游尝试开始前，都必须重置 Pro request marker 候选、served ack 候选和最终 billed 标记。

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

实现必须提供统一 helper，例如：

- `NormalizeCodexProMode(mode string) string`：读取和响应规范化用。
- `ValidateCodexProModeForUpdate(mode string) error`：更新接口校验用。

所有写回用户 `setting` JSON 的路径都必须保留 `CodexProMode` 原值。现有通用通知设置更新、语言、侧栏、排行榜展示名、订阅扣费偏好、激活订阅等接口不能因为整包重写 `dto.UserSetting` 而擦掉该字段。新增订阅域接口也必须读取当前 setting 后只修改 `CodexProMode`，再按现有用户设置缓存策略同步 Redis / user cache。

该字段不放进通用 `UpdateUserSettingRequest`。新增订阅域接口，与现有 `/api/subscription/self/preference`、`/api/subscription/self/active` 同属 `UserAuth` 保护的订阅路由：

```http
PUT /api/subscription/self/codex-pro-mode
```

请求体：

```json
{
  "mode": "flexible"
}
```

非法 `mode` 返回现有参数错误风格。成功响应固定返回：

```json
{
  "codex_pro_mode": "flexible",
  "codex_pro_eligible": true,
  "codex_pro_unavailable_reason": ""
}
```

### 资格返回

`/api/subscription/self` 响应补充：

```json
{
  "codex_pro_mode": "flexible",
  "codex_pro_eligible": true,
  "codex_pro_unavailable_reason": ""
}
```

`codex_pro_eligible` 表示账号 / 订阅层是否有资格配置和使用 Codex Pro，不包含当前请求的模型、下游 intent、上游 ack 路径，也不受当前 `codex_pro_mode = "off"` 影响。用户主动选择 `off` 只由 `codex_pro_mode` 表达，不能把选择器禁用，用户必须能再切回 `all` 或 `flexible`。

`codex_pro_unavailable_reason` 使用固定枚举，并按以下优先级返回第一个命中的原因：

1. `feature_disabled`：系统级功能关闭。本期若不实现系统级开关，则不返回该值。
2. `wallet_only`：当前扣费策略或实际可用来源导致请求不会创建订阅 `BillingSession`。
3. `trial_subscription`：当前可计费订阅是 `is_trial`、`invite_trial`、`trial_code` 或 `invite_trial` 来源。
4. `reward_subscription`：当前权益来自 `monthly_invite_entitlement` 等邀请奖励 / 非销售赠送来源。
5. `no_paid_subscription`：没有 active、未过期、未耗尽的付费等价订阅。

前端展示中文时通过 i18n 翻译，不直接展示枚举原文。

## 前端设计

### 主入口

主入口放在 Dashboard 用量概览的统计卡片下方，由 `web/default/src/features/dashboard/components/models/log-stat-cards.tsx` 读取 `getSelfSubscriptionFull` 后渲染 `web/default/src/features/subscriptions/components/codex-pro-mode-control.tsx`。不得散落到充值卡片、可购买套餐卡片或 `ProfileSettingsCard`。

理由：

- Dashboard 用量概览已经是用户查看当前消耗、套餐状态和请求行为的入口，模式切换放在这里更贴近「本次请求是否尝试 Pro 分组」的使用场景。
- `LogStatCards` 已读取当前订阅信息并统一处理保存状态，复用 `CodexProModeControl` 可以避免钱包页与订阅页出现两套并行控制。
- `ProfileSettingsCard` 主要是账号绑定、通知、排行榜，不适合作为套餐权益开关主入口。

### 展示规则

- 有资格用户：展示三态选择器和 2x 说明。
- 无资格用户：展示禁用态和行动导向原因，例如「请先购买有效付费套餐」「试用套餐不支持 Codex Pro」「当前钱包优先策略不会创建订阅计费会话」。
- 当前模式为 `off` 但账号仍有资格时，选择器保持可用，用户必须能切回 `all` 或 `flexible`。
- 默认选择 `灵活`。
- 切换后立即调用订阅域 API 保存，失败则回滚 UI 状态并提示。

每个选项必须有用户可见说明：

| 选项 | 用户可见说明 |
| --- | --- |
| 全部 | 符合资格的 GPT 系列请求无需额外 Header 即尝试 Pro；只有实际上游 Pro serve 成功才会 2x。 |
| 灵活 | 默认。只有带 `X-NewAPI-Codex-Pro-Intent: codex-pro` 的请求才尝试 Pro；未配置 Header 的请求按普通模式处理。 |
| 关闭 | 不尝试 Pro，不发送 Pro 请求 marker。 |

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
- `X-NewAPI-Codex-Pro-Intent` 只是弱 intent，不保证命中 Pro，也不保证 2x。
- 只有实际由 Pro 分组 serve、上游 response trailer 返回 `X-NewAPI-Pro-Served: codex-pro`，且请求成功完成的请求，才会产生 2 倍订阅 token 消耗。
- 回退普通分组不加倍。

实现完成后必须在 `web/default` 下运行 `bun run i18n:sync`，并确认 6 种语言无缺失 key。若新增动态翻译 key，需要同步现有静态 key 收集文件。

## Harness 配置引导

API 帮助弹窗应补充以下配置说明，目标是让用户能把 `X-NewAPI-Codex-Pro-Intent: codex-pro` 传给 `new-api`。该 Header 只让默认 `flexible` 模式按 harness 启用 Pro，不是计费凭据；实际 2x 只在上游 response trailer 返回 `X-NewAPI-Pro-Served: codex-pro` 且请求成功结算后发生。

### Codex CLI

Codex 官方配置支持：

- `model_providers.<id>.base_url`
- `model_providers.<id>.wire_api = "responses"`
- `model_providers.<id>.http_headers`
- `model_providers.<id>.env_http_headers`

示例：

```toml
model = "gpt-5"
model_provider = "new-api"

[model_providers.new-api]
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

OpenCode 生成器必须保留现有 provider id、`npm` 包和文件结构，只追加 Header 配置；不得把 `new-api` 改成 `newapi`，也不得把 `@ai-sdk/openai` 改成其他包。

示例：

```json
{
  "$schema": "https://opencode.ai/config.json",
  "provider": {
    "new-api": {
      "npm": "@ai-sdk/openai",
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

### Oh My Pi / OMP

现有配置引导生成 `models.yml` / `config.yml`，provider id 为 `new-api`，provider 已有 `baseUrl` / `apiKey`。本期只在现有 artifact 上追加 Header，不改变 provider id、文件结构或基础字段。

```yaml
providers:
  new-api:
    api: openai-responses
    baseUrl: https://example.com/v1
    apiKey: sk-...
    headers:
      X-NewAPI-Codex-Pro-Intent: codex-pro
```

如果目标 Oh My Pi / OMP 版本的配置 schema 尚不支持 `headers`，API 帮助必须明确标注「当前版本不支持按配置注入 Header，无法在 `flexible` 模式下按 harness 触发 Pro；可在控制台改用 `全部` 模式」。不得生成看似可用但实际不会生效的配置。

### Hermes Agent

Hermes Agent 官方配置目前稳定支持 `base_url` / `api_key`，未在已核验配置页中提供通用自定义 headers 字段。因此 API 帮助中的 Hermes Agent 项必须按保守策略展示：

- 若当前 Hermes Agent 版本支持主模型 provider 自定义 headers，则给出实际字段名和 `X-NewAPI-Codex-Pro-Intent: codex-pro` 示例。
- 若不支持，则明确提示 `flexible` 模式无法通过 Hermes Agent 配置 Header 触发 Pro，用户可在控制台改用 `全部` 模式。

### OpenClaw

OpenClaw 项也必须采用保守策略：只有完成当前版本配置字段核验后，才能输出带 `X-NewAPI-Codex-Pro-Intent: codex-pro` 的配置示例。未核验字段时，API 帮助必须明确提示 `flexible` 模式无法通过 OpenClaw 配置 Header 触发 Pro，用户可在控制台改用 `全部` 模式；不得生成未经验证的假配置。

## 后端实现边界

实现时应保持改动集中：

1. 用户模式枚举、读取规范化和更新校验函数。
2. 订阅域 API 读取和更新模式，并保留其他用户 setting 字段。
3. 订阅资格 helper，结果必须与本次请求实际订阅计费来源一致。
4. relay 运行时字段，例如：
   - `RelayInfo.CodexProRequestMarker string`
   - `RelayInfo.CodexProServed bool`
   - `RelayInfo.CodexProServedCandidate bool`
5. Header 最终化函数，例如：
   - `FinalizeProRequestHeader(header http.Header, marker string)`
6. Codex Responses handler 在完整消费 body / stream 后读取上游 response trailer ack，过滤下游响应 Header / trailer，并在请求成功完成后写回 `BillingSession` 或等效结算输入。
7. 结算阶段只根据内部 `CodexProServed` 状态对订阅 token 做 2x。

不要把 `X-NewAPI-Pro-Served` 从客户端或普通上游 response Header 透传为可信输入。它只应来自 `sub2api` 的上游 response trailer。

本期支持读取 ack 的请求路径固定为 Codex adaptor 的 OpenAI Responses 非流式、Responses 流式和 Responses compact handler。其他 chat completions、realtime、audio、task、WebSocket、SDK 等路径即使模型是 GPT 且用户模式为 `all`，也不得发送 Pro request marker，除非同一实现计划补齐对应 ack 捕获和结算测试。

GPT 系列 gating 必须复用现有 `common.IsOpenAITextModel` 或同等单一 helper，避免另写一套模型前缀判断。

消费日志或 admin 可见排查信息中应记录：`codex_pro_mode`、是否发送 Pro request marker、是否看到候选 served ack、最终是否按 Pro 结算、固定倍率 `2`、request id / upstream request id。日志不得包含用户传入的敏感 Header 原文。

## 安全性

- 所有 Pro 内部 Header 都是保留 Header。
- 下游客户端传入 `X-NewAPI-Pro-Request`、`X-NewAPI-Pro-Served` 必须被删除或忽略。
- 上游 response trailer 中的 `X-NewAPI-Pro-Served` 读取后不得透传到终端响应 Header 或 trailer；普通 response Header 中同名字段必须忽略。
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
- **sub2api 未升级**：不会通过 response trailer 返回 `X-NewAPI-Pro-Served: codex-pro`，因此不会 2x。

## 测试

至少补充以下测试：

1. **用户模式校验测试**
   - `all`、`flexible`、`off` 合法。
   - 空值规范化为 `flexible`。
   - 更新接口收到非法值会返回参数错误；读取历史脏值时按 `flexible` 处理。
2. **用户设置保留与缓存测试**
   - 通用用户设置更新、语言、侧栏、排行榜、订阅偏好、激活订阅写回后保留 `CodexProMode`。
   - 更新 Codex Pro 模式后，DB setting 与 Redis / user cache 中的 setting 保持一致。
3. **资格判断测试**
   - active、未过期、未耗尽的付费等价订阅可用。
   - 试用套餐、`invite_trial`、`trial_code`、邀请奖励、无套餐、过期 / 耗尽订阅、wallet-only 不可用。
   - `ActiveSubscriptionId` 指向试用 / 奖励 / 付费订阅时，资格与实际预扣选择一致。
4. **模型 gating 测试**
   - `gpt-*`、`o1`、`o3`、`o4`、`chatgpt` 类模型可参与。
   - 非 GPT 系列模型不参与。
   - gating 复用统一 helper，不能出现另一套前缀判断。
5. **上游请求 Header 最终化测试**
   - 符合条件时最终写入 `X-NewAPI-Pro-Request: codex-pro`。
   - 不符合条件时缺失。
   - 客户端或通道配置无法通过 `*` / regex passthrough、`{client_header:...}`、runtime `pass_headers`、`set_header`、`delete_header` 伪造、覆盖或删除。
6. **上游 ack 解析与过滤测试**
   - response trailer 为 `X-NewAPI-Pro-Served: codex-pro` 时只记录候选 ack。
   - 普通 response Header 为 `X-NewAPI-Pro-Served: codex-pro` 时必须忽略。
   - 缺失、大小写异常值、其他值都不记录。
   - 非流式 / compact 请求成功解析 usage 且无 upstream error 后才记录最终 Pro served。
   - 流式请求只有在 `response.completed`、正常读到 EOF 且无流错误后才可记录最终 Pro served。
   - 下游响应 Header 和 trailer 都不包含 `X-NewAPI-Pro-Served`。
7. **2x 结算测试**
   - ack 成功且请求成功完成时订阅 token 按 2x 结算。
   - ack 缺失、失败、回退普通分组、流式未完成、重试后普通分组成功时按 1x。
   - 钱包 quota、渠道 quota、模型价格倍率、非订阅 token 不被错误 2x。
   - 订阅预扣不足时补扣 / 退款行为符合现有账务规则。
8. **前端测试**
   - 有资格用户可切换三态。
   - `off` 模式不禁用选择器。
   - 无资格用户看到行动导向原因。
   - 保存失败会回滚 UI。
9. **配置引导测试**
   - Codex、Claude Code、OpenCode、Oh My Pi / OMP、Hermes Agent、OpenClaw 帮助文案覆盖对应 Header 能力或明确限制。
   - OpenCode / OMP 生成器保留现有 provider id、包名和文件结构，只追加 Header。
   - i18n 同步后 6 种语言均无缺失 key。

## 验收标准

- 用户可以在 Dashboard 用量概览统计卡片下方设置 `Codex Pro` 模式：`全部`、`灵活`、`关闭`。
- 默认模式为 `灵活`。
- 付费套餐用户发起 GPT 系列请求时，只有 `all` 模式，或 `flexible` 模式且下游带 `X-NewAPI-Codex-Pro-Intent: codex-pro` 时，`new-api` 才向上游发送 `X-NewAPI-Pro-Request: codex-pro`。
- 客户端和通道配置不能伪造 `X-NewAPI-Pro-Request` 或 `X-NewAPI-Pro-Served`。
- `X-NewAPI-Pro-Served` 不会暴露给终端用户响应。
- `sub2api` 返回 `X-NewAPI-Pro-Served: codex-pro` 且请求成功完成时，本次订阅 token 消耗按 2x 结算。
- ack 缺失、请求失败、回退普通分组、流式未完成、非 GPT 模型或非销售赠送权益时不 2x。
- 钱包 quota、试用套餐、无订阅请求不被 Pro 2x 影响。
- API 帮助中提供各 harness 的 Header 配置方式或明确限制，并明确 Header 只是 intent，不是计费凭据。
- 所有新增前端文案完成 `en`、`zh`、`fr`、`ja`、`ru`、`vi` 翻译，并通过 `bun run i18n:sync` 检查。
