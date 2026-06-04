# 试用套餐上游标记设计

## 背景

当前 `new-api` 的所有出站请求最终都会访问用户自建的 `sub2api` 服务。对方只需要区分「试用请求」与「非试用请求」，不需要为付费套餐维护一套可配置的优先级矩阵。也就是说，本期目标不是做一套通用的套餐优先级系统，而是给试用套餐提供一个稳定、可识别、不会被用户伪造的内部标记。

现有订阅模型里已经有 `SubscriptionPlan.is_trial`，这本身就是试用套餐的权威标记。问题不在于「如何再存一份试用配置」，而在于「如何把这个语义稳定地带到上游请求里」。

## 目标

1. 对所有使用试用套餐的请求，向上游 `sub2api` 注入一个稳定的内部标记。
2. 付费套餐不需要任何标记；上游应当把缺省值视为普通请求并忽略。
3. 标记必须由后端生成，不能由客户端或通道配置伪造。
4. 试用标记的生成规则应当直接复用现有的 `is_trial` 语义，避免引入新的运营配置入口。
5. 不新增数据库表，不新增前端可见配置项。

## 非目标

- 不实现通用的套餐优先级系统。
- 不给付费套餐增加等级、权重或排序字段。
- 不新增后台表单来配置该标记。
- 不修改试用套餐的发放、计费、过期或取消语义。
- 不把这个标记暴露给终端用户。
- 不要求 `sub2api` 侧理解复杂的套餐元数据；本期只需要一个稳定的试用标识符。

## 方案选择

### 方案 A：复用现有 `is_trial`，派生固定试用标记（采用）

当请求实际使用的订阅套餐 `is_trial = true` 时，后端注入固定标记 `trial`；当套餐不是试用套餐时，不注入任何标记。

优点：

- 复用现有语义，不增加新的业务配置面。
- 行为简单，稳定，容易解释。
- 付费套餐天然无标记，符合 `sub2api` 的忽略策略。
- 试用套餐在历史 active 订阅里也能按当前计划语义自动生效。

缺点：

- 只覆盖「试用 / 非试用」二分语义，暂不支持更细粒度的套餐分层。

### 方案 B：新增可配置字符串字段，用于任意优先级映射

给套餐增加一个可配置字符串字段，例如 `upstream_marker` 或 `upstream_priority`，由管理员手动填写。

优点：

- 未来可扩展到更细粒度的套餐分层。

缺点：

- 现在并不需要。
- 会引入新的后台配置面和更多校验逻辑。
- 容易把「试用标记」演变成一套新的运营配置系统。

结论：本期采用方案 A。

## 设计

### 标记语义

- 固定标记值：`trial`
- 含义：本次请求使用的是试用套餐。
- 付费套餐：不发送该标记。
- 钱包、无订阅、非试用订阅等未实际使用试用套餐的场景：不发送该标记。免费模型若仍经过订阅预扣并实际选中试用套餐，则按实际选中的 `SubscriptionPlan.is_trial` 打标；若未使用试用套餐，则不发送。

### 上游 Header 约定

新增一个内部 Header：

```http
X-NewAPI-Subscription-Marker: trial
```

约定如下：

- Header 名称固定为 `X-NewAPI-Subscription-Marker`；实现内部使用常量，比较和清理时按 HTTP Header 大小写不敏感语义处理。
- Header 值固定为小写 ASCII 字符串 `trial`。
- 仅当有效订阅套餐为试用套餐时发送，判断依据是本次请求实际选中的 `SubscriptionPlan.is_trial`。
- 付费套餐、无订阅、钱包等未实际使用试用套餐的场景都不发送；空值不是合法标记，必须表现为 Header 缺失。
- 上游 `sub2api` 可以把「Header 存在且值为 `trial`」视为试用流量，其余情况视为普通流量。
- 该 Header 属于内部标记，不面向客户端暴露，也不得携带用户 ID、订阅 ID、套餐 ID、套餐标题、`business_code` 等可关联用户或运营配置的信息。

### 数据流

1. `model.PreConsumeUserSubscriptionByUnits` 选出本次请求实际使用的 `UserSubscription` 与对应 `SubscriptionPlan`。
2. 订阅预扣结果在 `SubscriptionPreConsumeResult` 中补充一个运行时字段，记录当前计划是否为试用。该字段唯一来源是 `fillSubscriptionPreConsumeResult` 已拿到的 `plan *SubscriptionPlan`，填充值必须是 `plan != nil && plan.IsTrial`。
3. `SubscriptionFunding.PreConsume` 从预扣结果复制该布尔值；不得用 `grant_reason`、`source`、`business_code`、套餐标题或用户输入推导试用标记。
4. `BillingSession.syncRelayInfo` 将该布尔值转换为 `RelayInfo.SubscriptionTrialMarker`：试用时为 `trial`，非试用时为空；`clearRelayBillingState` 必须清空该 marker。
5. 在最终出站请求发送前，统一把 `X-NewAPI-Subscription-Marker: trial` 注入到最终 Header。
6. 付费套餐因为没有试用标记，Header 直接省略。

### 运行时字段

本期不新增数据库字段，只新增少量运行时字段用于跨层传递试用语义：

- `SubscriptionPreConsumeResult.PlanIsTrial bool`
- `SubscriptionFunding.PlanIsTrial bool`
- `RelayInfo.SubscriptionTrialMarker string`

其中：

- `PlanIsTrial` 只按 `plan != nil && plan.IsTrial` 填充。
- `PlanIsTrial = true` 时，`RelayInfo.SubscriptionTrialMarker = "trial"`。
- `PlanIsTrial = false` 时，`RelayInfo.SubscriptionTrialMarker = ""`。
- `clearRelayBillingState` 必须将 `RelayInfo.SubscriptionTrialMarker` 清空，避免无订阅、免费模型或后续降级路径复用旧状态。

这样做的原因是：

- `SubscriptionPlan.is_trial` 本来就是权威来源；
- 标记只影响出站请求，不影响账务、订单或套餐展示；
- 运行时字段足够，没必要把这个语义持久化到新的表结构里。

### 注入时机与优先级

`X-NewAPI-Subscription-Marker` 是保留内部 Header。最终出站前必须执行一次内部 Header 最终化，规则如下：

1. 先按大小写不敏感语义删除最终请求 Header 中任何来源的 `X-NewAPI-Subscription-Marker`。
2. 仅当 `RelayInfo.SubscriptionTrialMarker == "trial"` 时，再设置 `X-NewAPI-Subscription-Marker: trial`。
3. 其他任何值，包括空字符串、客户端传入的值、通道配置写入的值、runtime header override 写入的值，都不得出现在最终上游请求里。

该最终化步骤必须晚于 adaptor 默认 Header、通道 `header_override`、`param_override` 的 `set_header` / `delete_header` / `pass_headers`、runtime header override 和客户端 Header passthrough。也就是说，通道配置和客户端请求可以继续影响普通 Header，但不能覆盖、删除或伪造内部试用标记。

如果未来确实需要允许某个特殊通道屏蔽该标记，应当单独设计显式开关，不在本期做隐式删除逻辑。

### 作用范围

本期需要覆盖所有会实际发往上游、且支持 HTTP Header 的主请求出口路径：

- 普通 HTTP API 请求构造路径；
- 表单请求构造路径；
- 异步任务 / 非文本任务提交请求，例如 `DoTaskApiRequest` / `TaskAdaptor.BuildRequestHeader` 后的最终发送路径；
- 直接构造 `http.Request` 后调用 `channel.DoRequest` 的 provider 主请求路径；
- WebSocket 握手路径，包括 `DoWssRequest` 和 provider 自行发起的用户计费主请求 WebSocket Dial；
- AWS SDK / Bedrock 等不经 `channel.DoRequest` 的用户计费主请求，如果当前请求确实代表最终上游调用，也必须通过 SDK middleware、HTTP client wrapper 或等效机制注入同一内部 Header。

实现点应尽量集中：普通 HTTP 请求的最终化优先放在 `relay/channel/api_request.go` 的 `doRequest(c, req, info)` 中，以覆盖 `DoApiRequest`、`DoFormRequest`、`DoTaskApiRequest` 和直接调用 `channel.DoRequest` 的路径；WebSocket 在发起 Dial 前单独最终化，覆盖 `DoWssRequest` 以及 Xunfei、Volcengine TTS streaming 等 provider 自有 WebSocket 主请求；AWS SDK 主请求在 SDK 实际发送 HTTP 请求前最终化。

SSE 流式请求仍是普通 HTTP 请求，不需要单独握手逻辑。provider 辅助轮询、文件上传、获取临时 token、上传预签名地址、任务状态查询等不代表本次用户计费主请求的内部辅助请求，不属于本期标记范围；若后续发现某类用户计费主请求绕过上述出口，应补充同等最终化和测试，而不是让客户端或通道配置承担标记职责。

## 兼容性

- **数据库**：无迁移，无新增列。
- **前端**：无页面、无表单、无 i18n 变更。
- **订阅历史**：历史 active 试用订阅会在下次请求时自动带上试用标记，只要当前有效计划仍然是试用套餐。
- **付费套餐**：保持完全无标记，不影响现有请求。
- **上游容错**：`sub2api` 如果没有看到该 Header，就按普通请求处理。

## 实现边界

后端实现时应尽量保持改动局部化：

- 试用语义从订阅预扣结果一路传到 `RelayInfo`；
- Header 注入最好抽成一个很小的内部函数，避免散落在多个 adaptor 中；
- 不要把该标记塞进客户端可控的请求参数、通道参数或公开 DTO；
- 不要复用 `business_code`、套餐标题或用户输入作为试用标记。

## 测试

至少补充以下测试：

1. **试用标记传播测试**
   - 试用套餐的预扣结果按 `plan != nil && plan.IsTrial` 设置 `PlanIsTrial`，并能正确传到 `RelayInfo.SubscriptionTrialMarker`。
2. **Header 注入测试**
   - 试用套餐请求最终会带上 `X-NewAPI-Subscription-Marker: trial`。
3. **非试用路径测试**
   - 付费套餐、无订阅、钱包等未实际使用试用套餐的场景都不注入该 Header；免费模型按实际选中的套餐判断。
4. **客户端伪造防护测试**
   - 非试用请求即使客户端带 `X-NewAPI-Subscription-Marker: trial`，并通过 `*` / regex passthrough、`{client_header:X-NewAPI-Subscription-Marker}` 或 runtime `pass_headers` 尝试透传，最终上游也没有该 Header。
5. **通道覆盖防护测试**
   - 试用请求即使客户端或通道配置把同名 Header 写成其他值、空值或尝试删除，最终上游也只有 `trial`。
6. **任务、WebSocket 与 SDK 主请求路径测试**
   - 任务提交请求、WebSocket 握手路径和 AWS SDK / Bedrock 等不经 `channel.DoRequest` 的用户计费主请求遵循同样的最终化规则。
7. **回归测试**
   - 现有订阅计费、订阅过期、套餐展示和管理后台行为不受影响。

## 验收标准

- 试用套餐请求能够稳定携带 `X-NewAPI-Subscription-Marker: trial`。
- 付费套餐、无订阅、钱包等未实际使用试用套餐的非试用请求不携带该标记；免费模型按实际选中的套餐判断。
- 客户端、通道 `header_override`、runtime header override 或 Header passthrough 不能伪造、覆盖、删除最终内部试用标记。
- 不新增数据库迁移，不新增前端配置项。
- `sub2api` 可以仅依据这个标记区分试用流量与普通流量。
- 现有订阅账务和套餐管理行为不发生回归。
