# Codex Pro 分组服务模式实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 为付费订阅用户新增 `Codex Pro` 三态服务模式，并只在上游确认 Pro 分组实际 serve 成功后对订阅 token 做 2x 结算。

**架构：** 用户模式保存在现有 `User.Setting` JSON 中，由订阅域 API 读写；relay 侧根据本次实际订阅 `BillingSession`、模型、模式和下游弱 intent 生成上游 Pro request marker。上游 response trailer `X-NewAPI-Pro-Served: codex-pro` 只作为候选 ack，handler 完整消费 body / stream 且成功完成后才置内部 served 状态；结算阶段只对订阅 token 乘 2，且过滤内部响应 Header / trailer。

**技术栈：** Go、Gin、GORM、React 19、TypeScript、TanStack Query、i18next、Bun、Go 单元测试、前端 typecheck。

---

## 规格与约束

- 规格文件：`docs/superpowers/specs/2026-06-09-codex-pro-group-mode-design.md`。
- 当前仓库不使用 worktree，直接在主分支开发；不要恢复、清理或提交其他开发者留下的无关改动。
- 保护项：不得删除、改名或替换 `nеw-аρi`、`QuаntumΝоuѕ` 相关品牌、版权、包路径或元数据。
- JSON marshal/unmarshal 必须使用 `common.Marshal` / `common.Unmarshal` 等项目包装函数。
- 数据库必须兼容 SQLite、MySQL、PostgreSQL；本功能复用用户 `setting` JSON，不新增数据库列和迁移。
- 前端改动位于 `web/default/` 时必须遵守 `web/default/AGENTS.md`；所有用户可见文案使用 i18n，语言为 `en`、`zh`、`fr`、`ja`、`ru`、`vi`。
- TDD：每个行为变更先写失败测试并运行确认失败，再实现最小代码，再运行测试确认通过。
- Header 常量固定：
  - 下游弱 intent：`X-NewAPI-Codex-Pro-Intent: codex-pro`。
  - 上游请求 marker：`X-NewAPI-Pro-Request: codex-pro`。
  - 上游响应 trailer ack：`X-NewAPI-Pro-Served: codex-pro`。
- 客户端和通道配置不能伪造、覆盖或删除内部 Pro request / served Header。
- `X-NewAPI-Pro-Served` 只能从上游 response trailer 读取，且不得暴露到下游响应 Header 或 trailer；普通 response Header 中同名字段必须忽略。
- 本期只支持 Codex adaptor 的 OpenAI Responses 非流式、Responses 流式、Responses compact 路径；其他路径不得发送 Pro request marker。
- GPT 系列 gating 必须复用 `common.IsOpenAITextModel` 或同等单一 helper。
- `codex_pro_eligible` 表示账号 / 订阅层资格，不受 `codex_pro_mode = "off"` 影响；`off` 模式选择器仍可切回。

## 文件结构

- 修改：`dto/user_settings.go`
  - 增加 `CodexProMode string json:"codex_pro_mode,omitempty"`。
- 修改：`common/str.go`
  - 增加 Codex Pro mode 常量和 `NormalizeCodexProMode` / `ValidateCodexProModeForUpdate`。
- 修改：`model/user.go`、`controller/user.go`
  - 确保所有写回用户 setting 的路径保留 `CodexProMode`。
- 修改：`model/subscription.go`
  - 订阅预扣结果补齐 plan price、invite trial、source / grant reason 等资格元数据，或提供同等只读资格 helper。
- 修改：`service/funding_source.go`、`service/billing_session.go`、`service/billing.go`、`service/text_quota.go`
  - 将实际订阅资格同步到 `RelayInfo`；结算阶段按内部 `CodexProServed` 对订阅 token 做 2x。
- 修改：`relay/common/relay_info.go`
  - 增加 Pro request marker、served candidate、served final、资格原因等内部字段。
- 修改：`relay/channel/api_request.go`
  - 增加 Pro Header 常量、最终化函数、下游响应过滤。
- 修改：`relay/channel/codex/adaptor.go`
  - 限定 Codex Responses 路径才允许 Pro marker。
- 修改：`relay/channel/openai/relay_responses.go`、`relay/channel/openai/relay_responses_compact.go`
  - 完整消费 body / stream 后读取 response trailer 候选 ack、过滤 Header / trailer、成功完成后置最终 served 状态。
- 修改：`controller/subscription.go`、`router/api-router.go`
  - `/api/subscription/self` 补充 Pro 字段；新增 `PUT /api/subscription/self/codex-pro-mode`。
- 修改：`controller/config_guide.go`
  - 在 Codex / Claude Code / OpenCode / OMP / Hermes Agent / OpenClaw 帮助中补充 intent Header 配置或限制说明。
- 修改：`web/default/src/features/subscriptions/api.ts`、`web/default/src/features/subscriptions/types.ts`
  - 增加 Codex Pro API 与类型。
- 修改：`web/default/src/features/wallet/components/subscription-plans-card.tsx` 或同目录相邻子组件
  - 增加三态控制、无资格原因展示、保存失败回滚。
- 修改：`web/default/src/i18n/locales/{en,zh,fr,ja,ru,vi}.json`
  - 补齐前端文案翻译。
- 新增或修改测试：
  - Go：`common`、`controller`、`model`、`service`、`relay/channel`、`relay/channel/openai`。
  - 前端：若现有测试基础可用，补充组件 / API 测试；否则至少通过 typecheck 和 i18n sync 验证。

---

## 任务 1：用户模式字段、API 与设置保留

**文件：**
- `dto/user_settings.go`
- `common/str.go`
- `model/user.go`
- `controller/user.go`
- `controller/subscription.go`
- `router/api-router.go`
- 对应 Go 测试文件

- [ ] **步骤 1：编写失败测试：mode 规范化与校验**

在 `common` 或现有字符串 helper 测试文件中补充测试：

- 空值规范化为 `flexible`。
- `all` / `flexible` / `off` 合法并原样返回。
- 历史脏值读取时规范化为 `flexible`。
- 更新接口校验收到非法值时返回错误。

运行：

```bash
go test -p 1 ./common -run 'CodexProMode|NormalizeCodexProMode|ValidateCodexProMode' -count=1
```

预期：失败，原因是 helper 尚不存在。

- [ ] **步骤 2：实现 mode 字段与 helper**

实现：

```go
const (
    CodexProModeAll      = "all"
    CodexProModeFlexible = "flexible"
    CodexProModeOff      = "off"
)

func NormalizeCodexProMode(mode string) string
func ValidateCodexProModeForUpdate(mode string) error
```

在 `dto.UserSetting` 增加 `CodexProMode` 字段。读取响应一律输出规范化值。

- [ ] **步骤 3：运行 helper 测试确认通过**

```bash
go test -p 1 ./common -run 'CodexProMode|NormalizeCodexProMode|ValidateCodexProMode' -count=1
```

- [ ] **步骤 4：编写失败测试：通用用户 setting 更新保留 CodexProMode**

覆盖 `controller/user.go::UpdateUserSetting` 的整包重建风险：先构造用户 setting 含 `codex_pro_mode: all`，再调用通知 / 安全 / 语言类通用设置更新，断言 DB setting 中仍保留 `codex_pro_mode: all`。

运行：

```bash
go test -p 1 ./controller -run 'UpdateUserSetting.*CodexProMode|CodexProMode.*Preserve' -count=1
```

预期：失败，原因是现有 `UpdateUserSetting` 重建 `dto.UserSetting` 时未携带新字段。

- [ ] **步骤 5：修复所有用户 setting 写回路径**

- `UpdateUserSetting` 必须先读取当前 setting，修改允许字段，保留 `CodexProMode`、`BillingPreference`、`ActiveSubscriptionId` 等不属于该接口的字段。
- `UpdateSubscriptionPreference`、`SetActiveSubscription` 和新增接口都必须读取当前 setting 后只改目标字段。
- 任何用户设置缓存 / Redis 同步沿用现有路径，不新增并行缓存机制。

- [ ] **步骤 6：编写失败测试：订阅 self API 与更新接口**

覆盖：

- `GET /api/subscription/self` 返回 `codex_pro_mode`、`codex_pro_eligible`、`codex_pro_unavailable_reason`。
- 缺失 setting 时 mode 为 `flexible`。
- `PUT /api/subscription/self/codex-pro-mode` 写入 `all` / `flexible` / `off` 成功。
- 非法 mode 返回现有参数错误风格，不写 DB。
- 无资格用户允许保存 mode，但 `codex_pro_eligible=false`；`off` 不影响 eligible。

运行：

```bash
go test -p 1 ./controller ./router -run 'CodexProMode|SubscriptionSelf' -count=1
```

预期：失败，原因是 API / 路由尚不存在。

- [ ] **步骤 7：实现订阅域 API 与路由**

在 `controller/subscription.go` 增加：

- request DTO：`{ Mode string json:"mode" }`。
- `UpdateCodexProMode` handler：校验 mode、读取当前用户 setting、只修改 `CodexProMode`、写回、返回规范化 mode 和资格结果。
- `GetSubscriptionSelf` 响应补充 `codex_pro_mode`、`codex_pro_eligible`、`codex_pro_unavailable_reason`。

在 `router/api-router.go` 增加：

```go
subscriptionRoute.PUT("/self/codex-pro-mode", controller.UpdateCodexProMode)
```

- [ ] **步骤 8：运行任务 1 测试确认通过**

```bash
go test -p 1 ./common -run 'CodexProMode|NormalizeCodexProMode|ValidateCodexProMode' -count=1
go test -p 1 ./controller ./router -run 'CodexProMode|SubscriptionSelf|UpdateUserSetting.*CodexProMode|CodexProMode.*Preserve' -count=1
```

- [ ] **步骤 9：提交任务 1**

```bash
git add dto/user_settings.go common/str.go model/user.go controller/user.go controller/subscription.go router/api-router.go controller/*codex*test.go router/*codex*test.go common/*test.go
git commit -m "feat(codex-pro): 新增用户分组服务模式接口"
```

提交前使用 `git status --short` 核对仅包含任务 1 相关文件。

---

## 任务 2：订阅资格、RelayInfo 和 Pro request Header 最终化

**文件：**
- `model/subscription.go`
- `service/funding_source.go`
- `service/billing_session.go`
- `relay/common/relay_info.go`
- `relay/channel/api_request.go`
- `relay/channel/codex/adaptor.go`
- 对应 Go 测试文件

- [ ] **步骤 1：编写失败测试：订阅资格判断**

覆盖：

- active、未过期、未耗尽、有价、非试用、非邀请试用、销售 / 兑换码 / 管理员售后分配的订阅可用。
- `is_trial`、`invite_trial`、`trial_code`、`invite_trial` 来源不可用。
- `monthly_invite_entitlement` 等邀请奖励不可用。
- 过期 / 耗尽 / 没有 active 订阅不可用。
- wallet-only 策略下返回 `wallet_only` 或不生成 Pro marker。

运行：

```bash
go test -p 1 ./model ./service -run 'CodexPro.*Eligible|PaidSubscription.*CodexPro|CodexPro.*Unavailable' -count=1
```

预期：失败。

- [ ] **步骤 2：实现资格元数据传播**

在订阅预扣结果和 `SubscriptionFunding` 中补充足以判断资格的字段：

- `UserSubscriptionId` / `PlanId` / `PlanTitle` 已有字段继续保留。
- 补齐 `PlanPriceAmount`、`PlanIsTrial`、`PlanInviteTrial`、`SubscriptionSource`、`SubscriptionGrantReason`、`SubscriptionStatus`、`SubscriptionEndTime`、`SubscriptionTokenRemaining` 或等价字段。
- 不用标题、business code、客户端 Header 判断付费资格。
- 在 `BillingSession.syncRelayInfo()` 中把本次实际订阅资格结果同步到 `RelayInfo`。

- [ ] **步骤 3：运行资格测试确认通过**

```bash
go test -p 1 ./model ./service -run 'CodexPro.*Eligible|PaidSubscription.*CodexPro|CodexPro.*Unavailable' -count=1
```

- [ ] **步骤 4：编写失败测试：Pro request marker 条件**

在 relay / channel 测试中覆盖：

- 有资格 + GPT 系列 + mode=`all` + Codex Responses 路径：最终发送 `X-NewAPI-Pro-Request: codex-pro`。
- 有资格 + GPT 系列 + mode=`flexible` + 下游 `X-NewAPI-Codex-Pro-Intent: codex-pro`：最终发送 marker。
- 有资格 + mode=`flexible` 但没有 intent：不发送 marker。
- mode=`off`：不发送 marker。
- 非 GPT 模型：不发送 marker。
- 非 Codex Responses / compact 路径：不发送 marker。
- 客户端或通道配置通过大小写变体、`*` / regex passthrough、`set_header`、`delete_header`、runtime override 伪造、覆盖或删除 `X-NewAPI-Pro-Request` 均无效。

运行：

```bash
go test -p 1 ./relay/channel ./relay/channel/codex -run 'CodexPro|ProRequest|FinalizePro' -count=1
```

预期：失败。

- [ ] **步骤 5：实现 RelayInfo 字段与 Header 最终化**

在 `relay/common/relay_info.go` 增加字段，例如：

```go
CodexProMode string
CodexProEligible bool
CodexProUnavailableReason string
CodexProRequestMarker string
CodexProServedCandidate bool
CodexProServed bool
```

在 `relay/channel/api_request.go` 增加：

```go
const (
    CodexProIntentHeaderName = "X-NewAPI-Codex-Pro-Intent"
    CodexProIntentHeaderValue = "codex-pro"
    CodexProRequestHeaderName = "X-NewAPI-Pro-Request"
    CodexProServedHeaderName = "X-NewAPI-Pro-Served"
    CodexProMarkerValue = "codex-pro"
)

func FinalizeCodexProRequestHeader(headers http.Header, info *relaycommon.RelayInfo)
func StripCodexProServedHeader(headers http.Header)
```

`doRequest` 最终发送前同时调用试用 marker 与 Pro marker 最终化；Pro marker 必须晚于所有 adaptor / override / passthrough。

- [ ] **步骤 6：实现 Codex 路径 gating**

在 Codex adaptor 或 relay 初始化阶段只允许以下路径设置 `CodexProRequestMarker`：

- `RelayModeResponses`
- `RelayModeResponsesCompact`

其他路径不设置 marker，即使用户模式为 `all`。

每次重试开始前重置 candidate / served / request marker，避免失败尝试污染下一次成功尝试。

- [ ] **步骤 7：运行任务 2 测试确认通过**

```bash
go test -p 1 ./model ./service -run 'CodexPro.*Eligible|PaidSubscription.*CodexPro|CodexPro.*Unavailable' -count=1
go test -p 1 ./relay/channel ./relay/channel/codex -run 'CodexPro|ProRequest|FinalizePro' -count=1
```

- [ ] **步骤 8：提交任务 2**

```bash
git add model/subscription.go service/funding_source.go service/billing_session.go relay/common/relay_info.go relay/channel/api_request.go relay/channel/codex/adaptor.go model/*test.go service/*test.go relay/channel/*test.go relay/channel/codex/*test.go
git commit -m "feat(codex-pro): 添加分组请求标记最终化"
```

---

## 任务 3：上游 served ack、响应过滤与 2x 订阅结算

**文件：**
- `relay/channel/openai/relay_responses.go`
- `relay/channel/openai/relay_responses_compact.go`
- `service/http.go`
- `service/text_quota.go`
- `service/billing.go`
- `service/billing_session.go`
- 对应 Go 测试文件

- [ ] **步骤 1：编写失败测试：ack 解析与过滤**

覆盖：

- 已发送 Pro request marker 且 response trailer `X-NewAPI-Pro-Served: codex-pro`：只设置 candidate。
- 普通 response Header `X-NewAPI-Pro-Served: codex-pro`：必须忽略，不设置 candidate。
- 缺失 trailer、值为 `pro` / `true` / `2x` / 空值：不设置 candidate。
- 非流式成功解析 usage 且无 upstream error 后设置 final served。
- compact 成功解析 usage 且无 upstream error 后设置 final served。
- 流式只有收到 `response.completed`、正常读到 EOF 且无流错误后设置 final served。
- upstream error、解析失败、流式中断、请求取消时不设置 final served。
- 下游响应 Header 和 trailer 不包含 `X-NewAPI-Pro-Served`。

运行：

```bash
go test -p 1 ./relay/channel/openai ./service -run 'CodexPro.*Ack|ProServed|Responses.*Header|Trailer' -count=1
```

预期：失败。

- [ ] **步骤 2：实现 ack 读取与过滤**

实现 helper：

```go
func (info *RelayInfo) MarkCodexProServedCandidateFromTrailers(trailers http.Header)
func (info *RelayInfo) ConfirmCodexProServed()
```

规则：

- 只有本次内部已发送 `X-NewAPI-Pro-Request: codex-pro` 时，才接受 candidate。
- candidate 只来自上游 response trailer。
- handler 成功解析 usage / completion 且流式正常读到 EOF 后才确认 final served。
- `service.ShouldCopyUpstreamHeader` 或调用处过滤 `X-NewAPI-Pro-Served`，并确保下游 response trailer 不暴露该字段。

- [ ] **步骤 3：运行 ack 测试确认通过**

```bash
go test -p 1 ./relay/channel/openai ./service -run 'CodexPro.*Ack|ProServed|Responses.*Header|Trailer' -count=1
```

- [ ] **步骤 4：编写失败测试：2x 订阅结算**

覆盖：

- `CodexProServed=true` 且实际使用订阅 token：`SubscriptionTokens` 按 2x 结算。
- `CodexProServed=false`：按 1x。
- 钱包 quota 不因 Pro served 翻倍。
- 免费模型 / 无订阅 / wallet-only 不翻倍。
- 预扣不足时使用现有 `SubscriptionPostDelta` 表达额外订阅扣减。
- 结算失败时按现有错误 / 退款路径处理。

运行：

```bash
go test -p 1 ./service -run 'CodexPro.*Settle|Subscription.*2x|Wallet.*CodexPro' -count=1
```

预期：失败。

- [ ] **步骤 5：实现 2x 订阅 token 结算**

在 `subscriptionTokensForTextSettle` 或进入 `BillingSettleInput.SubscriptionTokens` 前统一处理：

```go
if relayInfo.CodexProServed && subscriptionTokens > 0 {
    subscriptionTokens *= 2
}
```

不得修改 wallet quota、channel quota、模型价格倍率或预扣估算逻辑。消费日志 / admin 可见排查信息记录：mode、是否发送 Pro request marker、candidate ack、final served、倍率 2、request id / upstream request id。

- [ ] **步骤 6：运行任务 3 测试确认通过**

```bash
go test -p 1 ./relay/channel/openai ./service -run 'CodexPro.*Ack|ProServed|Responses.*Header|CodexPro.*Settle|Subscription.*2x|Wallet.*CodexPro' -count=1
```

- [ ] **步骤 7：提交任务 3**

```bash
git add relay/channel/openai/relay_responses.go relay/channel/openai/relay_responses_compact.go service/http.go service/text_quota.go service/billing.go service/billing_session.go relay/channel/openai/*test.go service/*test.go
git commit -m "feat(codex-pro): 按上游确认执行订阅双倍结算"
```

---

## 任务 4：前端三态控制、API Help 和 i18n

**文件：**
- `web/default/src/features/subscriptions/api.ts`
- `web/default/src/features/subscriptions/types.ts`
- `web/default/src/features/wallet/components/subscription-plans-card.tsx`
- 可选新建：`web/default/src/features/wallet/components/codex-pro-mode-control.tsx`
- `web/default/src/features/keys/components/dialogs/cc-switch-dialog.tsx`
- `web/default/src/i18n/locales/{en,zh,fr,ja,ru,vi}.json`
- 可能修改：`controller/config_guide.go` 及其测试

- [ ] **步骤 1：编写失败测试或 type-level 验证：前端 API 类型**

补充类型 / API：

```ts
export type CodexProMode = 'all' | 'flexible' | 'off'
export interface UpdateCodexProModeRequest { mode: CodexProMode }
export interface UpdateCodexProModeResponse {
  codex_pro_mode: CodexProMode
  codex_pro_eligible: boolean
  codex_pro_unavailable_reason: string
}
```

运行：

```bash
cd web/default && bun run typecheck
```

预期：失败，原因是 API / 类型尚未实现或引用缺失。

- [ ] **步骤 2：实现前端 API 与类型**

新增 `updateCodexProMode(data)` 调用：

```ts
api.put('/api/subscription/self/codex-pro-mode', data)
```

`SelfSubscriptionData` 补充三项返回字段。

- [ ] **步骤 3：实现订阅区域三态控制**

在 `subscription-plans-card.tsx` 或同目录相邻组件中实现：

- 有资格：展示 `全部` / `灵活` / `关闭` 三态选择器、当前 2x 说明、弱 intent Header 说明。
- 无资格：展示禁用态和行动导向原因，不直接显示枚举。
- `off` 但有资格：选择器可用，可切回。
- 切换后乐观更新；保存失败回滚并 `toast.error`。
- 刷新订阅信息后同步最新 mode / eligible / reason。

- [ ] **步骤 4：补充 API Help / 配置引导**

在 API 帮助中补充：

- Codex CLI：`http_headers = { "X-NewAPI-Codex-Pro-Intent" = "codex-pro" }`。
- Claude Code：`ANTHROPIC_CUSTOM_HEADERS="X-NewAPI-Codex-Pro-Intent: codex-pro"`。
- OpenCode：保留 provider id `new-api`、`@ai-sdk/openai`，仅在模型 `headers` 增加 intent Header。
- Oh My Pi / OMP：保留 provider id `new-api`，仅在 provider 下追加 `headers`。若无法确认当前 schema 支持，必须显示限制说明而不是伪造配置。
- Hermes Agent / OpenClaw：未核验自定义 Header 字段时只显示限制和建议改用 `全部` 模式；不得输出假配置。

若 `controller/config_guide.go` 生成 OpenCode / OMP artifact，则补充后端测试，确保 provider id、包名、文件结构不变且只追加 Header。

- [ ] **步骤 5：补齐 i18n 并同步**

新增文案覆盖 `en`、`zh`、`fr`、`ja`、`ru`、`vi`：

- `Codex Pro`
- `All`
- `Flexible`
- `Off`
- `Only eligible GPT-family requests can try Codex Pro.`
- `Only requests acknowledged by the upstream Codex Pro response trailer and completed successfully consume 2x subscription tokens.`
- `Fallback requests are billed at the normal rate.`
- `Please purchase an eligible paid subscription first.`
- `Trial subscriptions do not support Codex Pro.`
- `Invitation reward subscriptions do not support Codex Pro.`
- `Your current billing preference will not create a subscription billing session.`
- API Help 中所有新增说明。

运行：

```bash
cd web/default && bun run i18n:sync
```

- [ ] **步骤 6：运行前端验证**

```bash
cd web/default && bun run i18n:sync
cd web/default && bun run typecheck
```

- [ ] **步骤 7：运行配置引导后端测试**

```bash
go test -p 1 ./controller -run 'ConfigGuide|OpenCode|OMP|CodexPro' -count=1
```

- [ ] **步骤 8：提交任务 4**

```bash
git add controller/config_guide.go controller/*config*test.go web/default/src/features/subscriptions/api.ts web/default/src/features/subscriptions/types.ts web/default/src/features/wallet/components/subscription-plans-card.tsx web/default/src/features/wallet/components/codex-pro-mode-control.tsx web/default/src/features/keys/components/dialogs/cc-switch-dialog.tsx web/default/src/i18n/locales/en.json web/default/src/i18n/locales/zh.json web/default/src/i18n/locales/fr.json web/default/src/i18n/locales/ja.json web/default/src/i18n/locales/ru.json web/default/src/i18n/locales/vi.json
git commit -m "feat(codex-pro): 增加前端模式控制和配置引导"
```

---

## 任务 5：端到端审查、修复与最终验证

- [ ] **步骤 1：并发发起只读代码审查**

至少启动 3 个只读 review 子代理：

1. 后端 API / setting / 资格审查。
2. relay / Header / ack / 计费审查。
3. 前端 UX / i18n / harness 配置审查。

每个子代理必须收到完整仓库路径、规格路径、计划路径、相关 commit 范围、禁止事项和验收标准；提示词至少 2000 字。review 子代理不运行项目级 build/test/lint，不修改文件。

- [ ] **步骤 2：按审查意见修复**

- 对明确 bug、规格不符、测试缺口直接修复。
- 对审查意见存在歧义时使用 `receiving-code-review` 技能验证后再改。
- 修复后对相同方向重新发起 review，直到 3 个方向均 PASS。

- [ ] **步骤 3：运行最终后端验证**

```bash
go test -p 1 ./common ./model ./service ./controller ./router ./relay/channel ./relay/channel/codex ./relay/channel/openai -run 'CodexPro|SubscriptionSelf|ConfigGuide|OpenCode|OMP' -count=1
```

如果新增测试分布在其他包，追加对应包；不得只运行窄化后不覆盖改动的测试。

- [ ] **步骤 4：运行最终前端验证**

```bash
cd web/default && bun run i18n:sync
cd web/default && bun run typecheck
```

- [ ] **步骤 5：检查工作树与提交**

使用 `git status --short` 确认只剩本任务相关改动。若 review 修复还有未提交文件，提交：

```bash
git add <only-codex-pro-related-files>
git commit -m "fix(codex-pro): 修复分组服务审查问题"
```

- [ ] **步骤 6：最终验收确认**

确认以下事实均由测试或代码审查覆盖：

- 用户可以设置 `all` / `flexible` / `off`，默认 `flexible`。
- 只有付费等价订阅、实际订阅计费来源、GPT 系列、Codex Responses 支持路径才会尝试 Pro。
- `flexible` 必须有 `X-NewAPI-Codex-Pro-Intent: codex-pro`；`all` 不要求 intent；`off` 不触发。
- 客户端 / 通道 / runtime 不能伪造 `X-NewAPI-Pro-Request` 或 `X-NewAPI-Pro-Served`。
- `X-NewAPI-Pro-Served` 不暴露给终端响应 Header 或 trailer。
- 只有上游 response trailer ack 候选存在且 handler 成功完成后才对订阅 token 2x。
- 钱包 quota、试用、奖励、无订阅、失败、回退普通分组不 2x。
- 前端文案完成 6 语言翻译，i18n sync 通过。
- API Help 对 Codex CLI、Claude Code、OpenCode、OMP、Hermes Agent、OpenClaw 均给出可用 Header 配置或明确限制。

---

## 子代理开发拆分建议

实现阶段应使用并发子代理，但要降低冲突：

1. `BackendModeApiWorker`：负责任务 1，边界限于用户 setting、订阅 API、路由、相关测试。
2. `RelayMarkerBillingWorker`：负责任务 2 和任务 3 的后端 relay / billing，边界限于 `model`、`service`、`relay`、相关测试。
3. `FrontendHarnessWorker`：负责任务 4，边界限于 `web/default` 前端、`controller/config_guide.go` 和配置引导测试。

如果 `RelayMarkerBillingWorker` 与 `BackendModeApiWorker` 都需要修改 `service/billing_session.go` 或 `model/subscription.go`，由 `RelayMarkerBillingWorker` 主改资格元数据和 billing 同步；`BackendModeApiWorker` 只消费公开 helper，避免同一区域竞争。

## 最终完成标准

- 所有计划复选框完成或明确不再适用并记录原因。
- 所有 review 子代理 PASS。
- 所有相关 Go 测试、`bun run i18n:sync`、`bun run typecheck` 通过。
- 所有实现提交只包含 Codex Pro 相关文件。
- 最终回复必须列出变更文件、关键行为、验证命令和结果，不得声称未观察到的事实。
