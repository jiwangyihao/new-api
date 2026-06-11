# 渠道 token 扣费倍率实现计划

> **面向 AI 代理的工作者：** 本计划使用当前 OMP harness 的 `task` 子代理直接在主工作区执行，不使用 worktree。步骤使用复选框（`- [ ]`）语法跟踪进度。启动任何全新子代理时，必须提供本计划和规格的完整路径，并提供不少于 2000 字的完整任务提示词；review 通过后自动进入实现，无需再次征求用户确认，只用 `notify` 报告阶段切换。

**目标：** 实现渠道级 token 扣费倍率，并在套餐界面展示不同渠道下的等价可用 token。

**架构：** 后端在 `Channel` 上新增 `token_billing_multiplier`，请求预扣前冻结独立 billing snapshot，订阅与 API Key token cap 使用渠道计费 token，Codex Pro 额外倍率只作用订阅。后端派生 `channel_token_equivalents` 给套餐接口，前端用类型安全 union 展示等价 token，并在渠道表单中维护倍率。

**技术栈：** Go 1.25.1、Gin、GORM v2、SQLite / MySQL / PostgreSQL、React 19、TypeScript、React Hook Form、Zod、TanStack Query、Bun、i18next。

---

## 0. 规格与全局约束

规格文件：`C:/Users/34404/source/repos/new-api/docs/superpowers/specs/2026-06-10-channel-token-billing-multiplier-spec.md`

计划文件：`C:/Users/34404/source/repos/new-api/docs/superpowers/plans/2026-06-10-channel-token-billing-multiplier-plan.md`

必须遵守：

- 根规则：`C:/Users/34404/source/repos/new-api/AGENTS.md`。
- 前端规则：`C:/Users/34404/source/repos/new-api/web/default/AGENTS.md`。
- billing expression 规则：`C:/Users/34404/source/repos/new-api/pkg/billingexpr/expr.md`。
- 不使用 worktree；直接在当前主工作区开发。
- 使用当前 harness 的 `task` 子代理并发调度，不使用 `superpowers:subagent-driven-development` 的 worktree 流程。
- 子代理实现时不得运行项目级全量 build/test/lint；每个子代理只运行自己修改范围内的定向测试。主 Agent 最后统一运行必要验证。
- 所有 Go JSON marshal/unmarshal 调用遵守 `common` 包规则；业务代码不得新增直接 `encoding/json` marshal/unmarshal 调用。
- 数据库代码必须兼容 SQLite、MySQL、PostgreSQL。
- 不修改受保护项目名称、组织名称、版权、模块路径、品牌信息。
- i18n 由任务 8 串行统一处理；任务 5、任务 6、任务 7 只列出新增文案 key，不直接修改 locale JSON，避免并发写同一批文件。

---

## 1. 文件结构与职责

### 共享 token 倍率工具

- 创建：`pkg/tokenbilling/multiplier.go`
  - 纯函数包，不依赖 `model` / `service` / `controller`。
  - 提供默认值、最大值、倍率校验、扣费 token 换算、等价 token 换算、倍率比较 helper。
- 创建：`pkg/tokenbilling/multiplier_test.go`
  - 覆盖 decimal half-up、非法倍率、等价 token floor。

### 后端核心计费

- 修改：`model/channel.go`
  - 新增 `TokenBillingMultiplier` 字段。
  - 新增 `GetTokenBillingMultiplier()` 辅助方法。
- 修改：`controller/channel.go`
  - 创建 / 更新 / 复制 / 批量新增渠道时处理倍率默认值、显式非法值、旧客户端兼容。
  - 使用 `common.UnmarshalBodyReusable` 和指针 DTO / raw envelope 检测字段 presence。
- 修改：`middleware/distributor.go`
  - `SetupContextForSelectedChannel()` 写入 `ContextKeyChannelTokenBillingMultiplier`。
- 修改：`constant/context-key.go` 或实际 context key 文件
  - 新增 `ContextKeyChannelTokenBillingMultiplier`。
- 修改：`relay/common/relay_info.go`
  - 新增独立 billing snapshot 字段和 freeze helper。
  - `InitChannelMeta()` 不覆盖已冻结倍率。
- 修改：`controller/relay.go`
  - 在 `service.PreConsumeBilling()` 和 `TokenLimit.PreConsume()` 前冻结 billing snapshot。
  - retry 时按 frozen multiplier 过滤候选。
- 修改：`service/channel_select.go`
  - `RetryParam` 增加 frozen multiplier / used channel 过滤参数，调用统一 selector。
- 修改：`model/channel_cache.go`
  - 内存缓存路径按 frozen multiplier 和 used channels 过滤候选后再按 priority / weight 选择。
- 修改：`model/ability.go`
  - 非内存缓存 DB 路径执行同样的 frozen multiplier / used channels 过滤。
- 修改：`service/billing_session.go`
  - 预扣使用渠道计费 token，并使订阅预扣记录、`SubscriptionPreConsumed`、API Key preconsume 口径一致。
- 修改：`service/text_quota.go`
  - 实际结算分层计算 raw / channel billable / API Key billable / subscription billable。
- 修改：`service/quota.go`
  - Realtime / WSS 增量扣费使用渠道计费 token 且只乘一次。
- 修改：`service/log_info_generate.go`
  - 日志 other 追加倍率快照、raw token、channel billable、API Key billable、subscription billable、initial channel 审计字段。

### 后端套餐展示

- 修改：`model/subscription.go`
  - `SubscriptionPlan` 增加 `ChannelTokenEquivalents` 派生字段（`gorm:"-"`）。
  - `SelfSubscriptionSummary` 增加订阅摘要等价字段。
  - 定义 plan/self 等价 token DTO。
- 修改：`controller/subscription.go`
  - `GetSubscriptionPlans()`、`GetPublicSubscriptionPlans()`、`GetSubscriptionSelf()` 填充 `channel_token_equivalents`。
  - `PublicSubscriptionPlan` 增加字段。

### 前端渠道配置

- 修改：`web/default/src/features/channels/types.ts`
  - `Channel` schema 增加 `token_billing_multiplier`。
- 修改：`web/default/src/features/channels/lib/channel-form.ts`
  - Zod schema、默认值、API -> form、form -> create/update payload 增加倍率字段和校验。
  - 使用 `z.coerce.number()` 或显式 `Number()` onChange，避免 number input 传字符串导致校验失败。
- 修改：`web/default/src/features/channels/components/drawers/channel-mutate-drawer.tsx`
  - 表单 UI 增加「渠道扣费倍率」输入与说明。
  - 保存成功后刷新相关 React Query key。

### 前端套餐展示

- 修改：`web/default/src/features/subscriptions/types.ts`
  - 新增 `PlanChannelTokenEquivalent` / `SubscriptionChannelTokenEquivalent` discriminated union。
  - `SubscriptionPlan`、`PublicSubscriptionPlan`、`SelfSubscriptionSummary` 增加 `channel_token_equivalents`。
- 创建：`web/default/src/features/subscriptions/query-keys.ts`
  - 集中定义 subscription plans / public plans / self summary 相关 query keys。
- 修改：`web/default/src/features/subscriptions/lib/format.ts`
  - 新增不会把 `0` 当 unlimited 的 finite token formatter。
- 修改：`web/default/src/features/wallet/lib/subscription-display.ts`
  - 新增等价 token 展示 helper。
- 修改：`web/default/src/features/wallet/components/subscription-plans-card.tsx`
  - 迁移 plans/self summary 到 React Query 并复用集中 query keys，确保渠道保存后的 invalidation 能刷新当前页面。
  - 套餐卡片和当前订阅摘要展示等价 token。
- 修改：`web/default/src/features/home/components/sections/plans-preview.tsx`
  - 如果显示 token 额度，使用与 wallet 一致的简化等价提示；不保留会误导的旧文案。

### i18n 串行收尾

- 修改：`web/default/src/i18n/locales/{en,zh,fr,ru,ja,vi}.json`
- 修改：`web/default/src/i18n/static-keys.ts`
- 该任务串行执行，不与前端任务 5 / 6 / 7 并发写 locale。

### 测试

- 新增 / 修改：`controller/channel_token_multiplier_test.go` 或现有 channel controller 测试。
- 新增：`pkg/tokenbilling/multiplier_test.go`。
- 修改：`service/subscription_billing_test.go`。
- 修改：`service/quota` 相关 WSS 测试或新增 `service/channel_token_multiplier_realtime_test.go`。
- 修改：`controller/subscription_*_test.go` 或新增 `controller/subscription_channel_equivalents_test.go`。
- 修改 / 新增前端测试：
  - `web/default/src/features/channels/lib/channel-form.test.ts`
  - `web/default/src/features/subscriptions/lib/format.test.ts`
  - `web/default/src/features/wallet/components/subscription-plans-card.test.ts`

---

## 2. 任务拆分

### 任务 1：后端渠道倍率字段、纯计算 helper、渠道 API 兼容

**文件：**

- 创建：`pkg/tokenbilling/multiplier.go`
- 创建：`pkg/tokenbilling/multiplier_test.go`
- 修改：`model/channel.go`
- 修改：`controller/channel.go`
- 修改：`middleware/distributor.go`
- 修改：`constant/context-key.go` 或实际 context key 定义文件
- 测试：`controller/channel_token_multiplier_test.go` 或邻近 channel controller 测试

- [ ] **步骤 1：查找 context key 定义文件**

使用 `search` 搜索 `ContextKeyChannelId`，定位实际文件。新增：

```go
ContextKeyChannelTokenBillingMultiplier = "channel_token_billing_multiplier"
```

- [ ] **步骤 2：创建 `pkg/tokenbilling` 纯函数包**

创建 `pkg/tokenbilling/multiplier.go`：

```go
package tokenbilling

import (
    "fmt"
    "math"

    "github.com/shopspring/decimal"
)

const (
    DefaultMultiplier = 1.0
    MaxMultiplier     = 100.0
    Epsilon           = 1e-9
)

func ValidateMultiplier(multiplier float64) error {
    if math.IsNaN(multiplier) || math.IsInf(multiplier, 0) || multiplier <= 0 || multiplier > MaxMultiplier {
        return fmt.Errorf("渠道扣费倍率必须大于 0 且不超过 100")
    }
    return nil
}

func EffectiveMultiplier(multiplier float64) float64 {
    if multiplier <= 0 || math.IsNaN(multiplier) || math.IsInf(multiplier, 0) {
        return DefaultMultiplier
    }
    return multiplier
}

func SameMultiplier(a float64, b float64) bool {
    return math.Abs(a-b) <= Epsilon
}

func ApplyMultiplier(rawTokens int64, multiplier float64) (int64, error) {
    if rawTokens <= 0 {
        return 0, nil
    }
    if err := ValidateMultiplier(multiplier); err != nil {
        return 0, err
    }
    product := decimal.NewFromInt(rawTokens).Mul(decimal.NewFromFloat(multiplier))
    rounded := product.Round(0).IntPart()
    if rounded < 1 {
        return 1, nil
    }
    return rounded, nil
}

func EquivalentTokens(standardTokens int64, multiplier float64) (int64, error) {
    if standardTokens <= 0 {
        return 0, nil
    }
    if err := ValidateMultiplier(multiplier); err != nil {
        return 0, err
    }
    return decimal.NewFromInt(standardTokens).Div(decimal.NewFromFloat(multiplier)).Floor().IntPart(), nil
}
```

此包不得导入 `model`、`service`、`controller`，避免 import cycle。后端所有扣费和展示等价 token 均调用此包，禁止重复实现取整逻辑。

- [ ] **步骤 3：写纯 helper 测试**

创建 `pkg/tokenbilling/multiplier_test.go`，覆盖：

- `ApplyMultiplier(0, 2) == 0`。
- `ApplyMultiplier(1, 1) == 1`。
- `ApplyMultiplier(1, 1.5) == 2`。
- `ApplyMultiplier(3, 0.5) == 2`。
- `ApplyMultiplier(1, 0.1) == 1`。
- `ApplyMultiplier(10000, 2) == 20000`。
- `ValidateMultiplier(0)`、`-1`、`100.01`、`math.NaN()`、`math.Inf(1)` 返回 error。
- `EquivalentTokens(1, 2) == 0`。
- `EquivalentTokens(1000000, 2) == 500000`。

- [ ] **步骤 4：在 `model.Channel` 增加字段和 getter**

在 `model/channel.go` 的 `Channel` struct 中新增：

```go
TokenBillingMultiplier float64 `json:"token_billing_multiplier" gorm:"not null;default:1"`
```

新增方法：

```go
func (channel *Channel) GetTokenBillingMultiplier() float64 {
    if channel == nil {
        return tokenbilling.DefaultMultiplier
    }
    return tokenbilling.EffectiveMultiplier(channel.TokenBillingMultiplier)
}
```

要求：

- 新增字段随 `AutoMigrate(&Channel{})` 自动迁移。
- 不写数据库特有 migration SQL。

- [ ] **步骤 5：替换 Add/Update 的 JSON 绑定方式以检测 presence**

`controller.AddChannel()` 和 `UpdateChannel()` 当前使用 `ShouldBindJSON`，非指针 `float64` 无法区分未传、显式 `0`、显式 `null`。改为一次读取可复用 body，并同时解析业务 request 与 raw presence envelope。

建议 raw DTO 使用 `json.RawMessage` 类型保存原始字段，但所有解码必须通过 `common.UnmarshalBodyReusable` / `common.Unmarshal`，不得直接调用 `encoding/json.Unmarshal`：

```go
type addChannelRawRequest struct {
    Channel *struct {
        TokenBillingMultiplier json.RawMessage `json:"token_billing_multiplier"`
    } `json:"channel"`
}

type patchChannelRawRequest struct {
    TokenBillingMultiplier json.RawMessage `json:"token_billing_multiplier"`
}

func parseChannelTokenBillingMultiplier(raw json.RawMessage) (present bool, value float64, err error) {
    if len(raw) == 0 {
        return false, 0, nil
    }
    if common.GetJsonType(raw) != "number" {
        return true, 0, fmt.Errorf("渠道扣费倍率必须是数字")
    }
    if err := common.Unmarshal(raw, &value); err != nil {
        return true, 0, err
    }
    return true, value, tokenbilling.ValidateMultiplier(value)
}
```

实现要求：

- 允许引用 `encoding/json` 的 `json.RawMessage` 类型；不得新增直接 `encoding/json` marshal/unmarshal 调用。
- 不使用 `*float64` 作为唯一 presence 判断，因为 JSON 显式 `null` 会与 absent 混在一起。
- 不使用 `ShouldBindJSON` 先消耗 body 后再读取 body；统一使用 `common.UnmarshalBodyReusable` 或 `common.GetBodyStorage` + `common.Unmarshal`。
- absent：创建默认 `1`，更新保留原值。
- present 且类型为 `null`、`string`、`boolean`、`object`、`array`、`0`、负数、`>100`、`NaN`、`Inf`：拒绝。

- [ ] **步骤 6：渠道创建默认倍率**

在 `AddChannel()`：

- `addChannelRequest.Channel == nil` 仍按现有错误处理。
- 字段未传：`addChannelRequest.Channel.TokenBillingMultiplier = tokenbilling.DefaultMultiplier`。
- 字段存在：调用 `tokenbilling.ValidateMultiplier(value)`；非法则返回错误。
- 显式 `0` 永远不是默认值，必须拒绝。
- batch 模式中用值拷贝：

```go
localChannel := *addChannelRequest.Channel
localChannel.Key = key
channels = append(channels, localChannel)
```

不能复用同一个 `*model.Channel` 指针。

测试必须覆盖：旧客户端创建未传倍率成功且落库为 `1`；显式 `0` 创建失败；显式 `null` 创建失败；字符串/布尔等非数字类型创建失败；显式 `2` 创建成功；batch 未传默认 `1`。

- [ ] **步骤 7：渠道更新保留倍率**

在 `UpdateChannel()`：

- 先读取 `originChannel`。
- raw DTO 未传 `token_billing_multiplier`：`channel.TokenBillingMultiplier = originChannel.TokenBillingMultiplier`。
- raw DTO 显式传合法值：保存该值。
- raw DTO 显式传非法值：拒绝。
- `model.Channel.Update()` 如仍用 `DB.Model(channel).Updates(channel)`，必须确保合法倍率被写入。可改为 map update 或在 `Update()` 中对 `token_billing_multiplier` 使用 `Select`。

测试必须覆盖：旧客户端更新其它字段但未传倍率，原倍率不变；显式 `null` 更新失败；显式 `0` 更新失败；显式 `1.5` 更新成功。

- [ ] **步骤 8：复制和批量新增**

在 `CopyChannel()` 确认复制 struct 时继承 `TokenBillingMultiplier`。

在批量新增中每条 `localChannel` 均带合法倍率。

- [ ] **步骤 9：context 写入倍率**

在 `middleware.SetupContextForSelectedChannel()` 中设置：

```go
common.SetContextKey(c, constant.ContextKeyChannelTokenBillingMultiplier, channel.GetTokenBillingMultiplier())
```

- [ ] **步骤 10：定向测试**

运行：

```bash
go test ./pkg/tokenbilling -count=1
go test ./controller -run 'Test.*Channel.*Token.*Multiplier|Test.*UpdateChannel.*Multiplier|Test.*AddChannel.*Multiplier' -count=1
```

预期：新增测试 PASS。

---

### 任务 2：后端 relay billing snapshot、预扣、结算、retry 与日志

**文件：**

- 修改：`relay/common/relay_info.go`
- 修改：`controller/relay.go`
- 修改：`service/channel_select.go`
- 修改：`model/channel_cache.go`
- 修改：`model/ability.go`
- 修改：`service/billing_session.go`
- 修改：`service/text_quota.go`
- 修改：`service/quota.go`
- 修改：`service/log_info_generate.go`
- 测试：`service/subscription_billing_test.go`
- 测试：`service/channel_token_multiplier_test.go`
- 测试：`service/channel_token_multiplier_realtime_test.go` 或邻近 realtime 测试
- 测试：`model/channel_cache_test.go` / `model/ability_test.go` 或邻近 channel selector 测试

- [ ] **步骤 1：RelayInfo 新增独立 billing snapshot 字段**

在 `relay/common/relay_info.go` 的 `RelayInfo` 中新增：

```go
ChannelTokenBillingMultiplier float64
InitialChannelId              int
InitialChannelType            int
RawMeteredTokens              int64
ChannelBillableTokens         int64
SubscriptionBillableTokens    int64
ApiKeyBillableTokens          int64
EstimatedRawTokens            int64
```

新增方法：

```go
func (info *RelayInfo) FreezeChannelTokenBillingSnapshot(c *gin.Context) error
func (info *RelayInfo) FrozenChannelTokenBillingMultiplier() float64
```

`Freeze...` 从 Gin context 读取 channel id/type/multiplier，默认 multiplier 为 `1`，校验 `0 < multiplier <= 100`。

- [ ] **步骤 2：不要提前初始化 ChannelMeta**

`FreezeChannelTokenBillingSnapshot()` 只能写独立 snapshot，不调用 `InitChannelMeta()`。

`InitChannelMeta()` 可把 `ChannelTokenBillingMultiplier` 复制到 `ChannelMeta`（如果新增字段），但不能覆盖 `RelayInfo.ChannelTokenBillingMultiplier`。

- [ ] **步骤 3：预扣前冻结**

在 `controller.Relay()` 中，`helper.ModelPriceHelper()` 之后、`service.PreConsumeBilling()` 之前调用：

```go
if err := relayInfo.FreezeChannelTokenBillingSnapshot(c); err != nil {
    newAPIError = types.NewError(err, types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
    return
}
```

必须发生在 `service.PreConsumeBilling(...)`、`service.NewTokenLimitSession(...)`、`TokenLimit.PreConsume(...)` 之前。

- [ ] **步骤 4：预扣使用渠道计费 token，并同步所有请求级预扣字段**

必须保留现有 eligibility gate：只有 `distributorSubscriptionEligibleForBilling(relayInfo)` 通过且请求属于订阅 token 口径时，才进入渠道倍率预扣。`RelayFormatTask`、音频、图像、per-call、legacy quota 路径继续使用现有拒绝或旧结算语义；不得为了渠道倍率把这些路径降级成 `1` 个 token 预扣，也不得对它们调用 `tokenbilling.ApplyMultiplier`。

在 `service.NewBillingSession()` 中，gate 通过后定义单一变量 `preconsumeBillableTokens`：

```go
raw := int64(relayInfo.GetEstimatePromptTokens()) // 来自 controller.Relay 中 service.EstimateRequestToken 的 token 估算
if raw <= 0 {
    raw = 1 // token-billed eligible 请求的最小 token 保留；不得回退到 legacy quota / preConsumedQuota
}
preconsumeBillableTokens, err := tokenbilling.ApplyMultiplier(raw, relayInfo.FrozenChannelTokenBillingMultiplier())
relayInfo.EstimatedRawTokens = raw
```

要求：

- `SubscriptionFunding.distributorAmount = preconsumeBillableTokens`。
- `session.preConsume(c, preconsumeBillableTokens)`，不是 legacy `preConsumedQuota`。
- `BillingSession.preConsumedQuota`、订阅预扣记录、`relayInfo.SubscriptionPreConsumed`、`relayInfo.FinalPreConsumedQuota`、API Key `TokenLimit.PreConsume()` 都使用渠道计费 token。
- 后续差额结算用 `actualSubscriptionBillableTokens - preconsumeBillableTokens`。
- `preConsumedQuota` / `QuotaToPreConsume` / `ModelRatio` / `ModelPrice` / billing expression 产出的旧 quota 值不得作为 raw token 输入再次乘渠道倍率。
- 如果必须保留某个 legacy 字段用于 quota 日志，必须命名清楚，不允许再参与 token 差额扣费。

新增测试：估算 raw=10、倍率=2 时订阅预扣和 API Key 预扣均为 20；实际 usage=8、倍率=2 时结算 delta 按 16 与预扣 20 退款；调整 `ModelRatio`、`ModelPrice` 或 tiered billing expression 不改变 `preconsumeBillableTokens`；`RelayFormatTask` / per-call 任务预扣仍按现有 `PriceData.Quota` 拒绝或旧语义处理，不被改成最小 `1` token。

- [ ] **步骤 5：retry 同倍率候选过滤覆盖缓存和非缓存路径**

在 `service.RetryParam` 增加字段，例如：

```go
FrozenTokenBillingMultiplier float64
UsedChannelIds []int
RequireSameTokenBillingMultiplier bool
```

选择语义固定为：

1. 先按 endpoint/model/group 得到候选。
2. 排除 `UsedChannelIds`。
3. 如启用同倍率约束，使用 `tokenbilling.SameMultiplier(channel.GetTokenBillingMultiplier(), frozen)` 过滤。
4. 对剩余候选重新按现有 priority / weight 策略选择最高可用 priority；不要把 retry index 直接作为 priority 下标而跳过同 priority 未使用候选。
5. 若无同倍率候选，返回当前项目已有的无可用渠道错误，不降级到不同倍率渠道。

覆盖文件：

- `model/channel_cache.go`：内存缓存路径。
- `model/ability.go`：`common.MemoryCacheEnabled=false` 的 DB fallback 路径。
- `service/channel_select.go`：对 controller 暴露统一入口。

测试必须分别覆盖 memory cache on/off：初始渠道倍率 `2`，存在同 endpoint 但倍率 `1` 和 `2` 的候选，retry 只能选择倍率 `2` 且未使用的候选。

- [ ] **步骤 6：controller retry 使用过滤**

在 `controller.getChannel()` 或调用 `service.CacheGetRandomSatisfiedChannel()` 的位置传入 frozen multiplier 和已用渠道。

`addUsedChannel(c, channel.Id)` 已存在，复用当前 `use_channel` / used channel 记录时注意类型。consume log 的 `channel_id` / `channel_type` 继续归属最终实际上游渠道，初始渠道只写 snapshot 审计字段。

- [ ] **步骤 7：文本结算分层**

在 `service.PostTextConsumeQuota()`：

```go
rawMeteredTokens := SubscriptionMeteredTokens(usage)
channelBillableTokens, err := tokenbilling.ApplyMultiplier(rawMeteredTokens, relayInfo.FrozenChannelTokenBillingMultiplier())
apiKeyTokens := channelBillableTokens
subscriptionTokens := subscriptionTokensForTextSettle(relayInfo, channelBillableTokens, summary.Quota)
relayInfo.RawMeteredTokens = rawMeteredTokens
relayInfo.ChannelBillableTokens = channelBillableTokens
relayInfo.ApiKeyBillableTokens = apiKeyTokens
relayInfo.SubscriptionBillableTokens = subscriptionTokens
```

保持：

- `usageUnavailable` / `usageEstimated` / `TotalTokens == 0` 路径不扣费并退还预扣。
- 本地估算只写 `EstimatedRawTokens`，不作为 raw metered。
- `summary.Quota` 不乘渠道倍率。
- Codex Pro 只通过 `subscriptionTokensForTextSettle()` 影响订阅，不影响 API Key cap。

- [ ] **步骤 8：Realtime / WSS 增量扣费**

在 `service.PreWssConsumeQuota()`：

```go
raw := int64(tokens)
billable, err := tokenbilling.ApplyMultiplier(raw, relayInfo.FrozenChannelTokenBillingMultiplier())
relayInfo.RawMeteredTokens += raw
relayInfo.ChannelBillableTokens += billable
relayInfo.ApiKeyBillableTokens += billable
relayInfo.SubscriptionBillableTokens += billable
TokenLimit.ConsumeIncrement(billable)
session.SettleSubscriptionIncrement(billable)
```

`PostWssConsumeQuota()` 只做日志/最终对齐，不再用 raw `TotalTokens` 重复 settle，也不再二次乘倍率。

- [ ] **步骤 9：日志字段**

在 `service/log_info_generate.go` 的 billing info 中追加：

```go
other["channel_token_billing_multiplier"] = relayInfo.ChannelTokenBillingMultiplier
other["raw_metered_tokens"] = relayInfo.RawMeteredTokens
other["channel_billable_tokens"] = relayInfo.ChannelBillableTokens
other["api_key_billable_tokens"] = relayInfo.ApiKeyBillableTokens
other["subscription_billable_tokens"] = relayInfo.SubscriptionBillableTokens
other["initial_channel_id"] = relayInfo.InitialChannelId
other["initial_channel_type"] = relayInfo.InitialChannelType
```

`logs.metered_tokens` 保持最终订阅扣费 token，不改 `model.RecordConsumeLog()` 语义。

- [ ] **步骤 10：后端定向测试**

必须写出并运行覆盖以下语义的测试：

- 订阅预扣按渠道计费 token。
- API Key cap 预扣按同一渠道计费 token。
- Codex Pro + 渠道倍率：API Key 只扣 `channel_billable_tokens`，订阅扣 `subscription_billable_tokens`。
- `usageEstimated`、`usage == nil`、`TotalTokens == 0` 不扣费并退预扣。
- Realtime / WSS 增量只乘一次。
- 日志 other 字段齐全，且 `logs.metered_tokens == subscription_billable_tokens`。
- API Key settle audit 路径记录 `api_key_billable_tokens`。
- retry memory cache on/off 都只选择同倍率未使用候选；无同倍率候选时停止 retry。

运行：

```bash
go test ./model -run 'Test.*Channel.*Multiplier|Test.*Retry.*Multiplier|Test.*Ability.*Multiplier' -count=1
go test ./service -run 'Test.*Channel.*Multiplier|TestPostTextConsumeQuota.*Multiplier|TestPostTextConsumeQuotaCodexPro.*TokenLimit|Test.*Wss.*Multiplier|Test.*UsageEstimated.*Subscription' -count=1
go test ./controller -run 'Test.*Relay.*Multiplier|Test.*Retry.*Multiplier' -count=1
```

预期：新增和相关旧测试 PASS。

---

### 任务 3：后端套餐等价 token API

**文件：**

- 修改：`model/channel.go`
- 修改：`model/subscription.go`
- 修改：`controller/subscription.go`
- 测试：`controller/subscription_public_plans_route_test.go`
- 测试：`controller/subscription_self_summary_test.go`
- 测试：新增 `controller/subscription_channel_equivalents_test.go` 或相邻测试

- [ ] **步骤 1：定义等价 token DTO**

在 `model/subscription.go` 或新文件 `model/subscription_channel_equivalent.go` 定义 DTO。不要放到 `service`，避免 controller/model 循环依赖；token 计算调用 `pkg/tokenbilling`。

定义 plan 和 subscription 两套 DTO。有限数值字段使用 `*int64` / `*float64` 指针加 `omitempty`，确保 finite `0` 可以通过非 nil 指针序列化；`unlimited` 必须返回 `token_unlimited: true`。

```go
type PlanChannelTokenEquivalent struct {
    Kind                    string   `json:"kind"`
    ChannelType             int      `json:"channel_type"`
    ChannelTypeName         string   `json:"channel_type_name"`
    ChannelTypeLabelKey     string   `json:"channel_type_label_key,omitempty"`
    VariantCount            int      `json:"variant_count"`
    Multiplier              *float64 `json:"multiplier,omitempty"`
    MinMultiplier           *float64 `json:"min_multiplier,omitempty"`
    MaxMultiplier           *float64 `json:"max_multiplier,omitempty"`
    EquivalentTokenLimit    *int64   `json:"equivalent_token_limit,omitempty"`
    EquivalentTokenLimitMin *int64   `json:"equivalent_token_limit_min,omitempty"`
    EquivalentTokenLimitMax *int64   `json:"equivalent_token_limit_max,omitempty"`
    TokenUnlimited          bool     `json:"token_unlimited,omitempty"`
}

type SubscriptionChannelTokenEquivalent struct {
    Kind                         string   `json:"kind"`
    ChannelType                  int      `json:"channel_type"`
    ChannelTypeName              string   `json:"channel_type_name"`
    ChannelTypeLabelKey          string   `json:"channel_type_label_key,omitempty"`
    VariantCount                 int      `json:"variant_count"`
    Multiplier                   *float64 `json:"multiplier,omitempty"`
    MinMultiplier                *float64 `json:"min_multiplier,omitempty"`
    MaxMultiplier                *float64 `json:"max_multiplier,omitempty"`
    EquivalentTokenLimit         *int64   `json:"equivalent_token_limit,omitempty"`
    EquivalentTokenLimitMin      *int64   `json:"equivalent_token_limit_min,omitempty"`
    EquivalentTokenLimitMax      *int64   `json:"equivalent_token_limit_max,omitempty"`
    EquivalentTokenRemaining     *int64   `json:"equivalent_token_remaining,omitempty"`
    EquivalentTokenRemainingMin  *int64   `json:"equivalent_token_remaining_min,omitempty"`
    EquivalentTokenRemainingMax  *int64   `json:"equivalent_token_remaining_max,omitempty"`
    TokenUnlimited               bool     `json:"token_unlimited,omitempty"`
}
```

`Kind` 取值固定：`single`、`range`、`unlimited`。

- [ ] **步骤 2：SubscriptionPlan / SelfSubscriptionSummary 增加字段**

`SubscriptionPlan` / `PublicSubscriptionPlan` 使用：

```go
ChannelTokenEquivalents []PlanChannelTokenEquivalent `json:"channel_token_equivalents" gorm:"-"`
```

`SelfSubscriptionSummary` 使用：

```go
ChannelTokenEquivalents []SubscriptionChannelTokenEquivalent `json:"channel_token_equivalents" gorm:"-"`
```

- [ ] **步骤 3：查询启用渠道倍率分组**

在 `model/channel.go` 或 `controller/subscription.go` 增加查询函数：

- 只统计启用渠道。
- 按 `type` 分组。
- 对同类型多倍率返回 min/max multiplier、variant count。
- 无启用渠道时返回空数组，不返回默认 OpenAI=1。
- 即使无启用渠道，也必须显式初始化为空 slice，接口 JSON 必须返回 `"channel_token_equivalents":[]`，不能因 nil slice 或 `omitempty` 省略字段。
- 查询使用 GORM，避免 DB 专有 SQL；必要时拉取少量字段到 Go 内存分组。

- [ ] **步骤 4：构建 plan equivalents**

对 `MonthlyTokenLimit`：

- `MonthlyTokenLimit < 0` 或当前语义表示 unlimited：返回 `kind = "unlimited"`、`token_unlimited = true`；不得设置普通等价 token 数值字段。
- finite：调用 `tokenbilling.EquivalentTokens(monthlyTokenLimit, multiplier)`，用指针字段写入结果；即使结果为 `0` 也必须序列化该字段。
- `single`：只有一个 multiplier，`variant_count = 1`，设置 `multiplier` 和 `equivalent_token_limit`。
- `range`：同 channel type 多倍率，设置 `min_multiplier`、`max_multiplier`、`equivalent_token_limit_min`、`equivalent_token_limit_max`；注意 `multiplier` 越大，等价 token 越小，字段命名按 min/max equivalent 值输出。

- [ ] **步骤 5：构建 self summary equivalents**

对当前 active subscription：

- limit 使用 `summary.TokenLimit` 或订阅标准 token limit。
- remaining 使用 `max(0, summary.TokenLimit - summary.TokenUsed)`。
- finite remaining 为 `0` 时必须通过非 nil 指针返回 `0`，不能省略成 unlimited。
- unlimited 订阅返回 `kind = "unlimited"`、`token_unlimited = true`，不得返回普通等价 token 数值字段。

- [ ] **步骤 6：接口填充位置**

- `/api/subscription/plans`：每个 plan 填充 `plan.channel_token_equivalents`。
- `/api/subscription/public/plans`：每个 public plan 填充 `plan.channel_token_equivalents`。
- `/api/subscription/self`：`summary.channel_token_equivalents` 填充当前订阅 remaining equivalents；record 内 plan 只提供 limit equivalents。

- [ ] **步骤 7：后端测试**

测试：

- 倍率 `1` 与 `2` 时，`1,000,000` 标准 token 返回 `1,000,000` 与 `500,000`。
- 倍率 `0.5` 时，`1,000,000` 标准 token 返回 `2,000,000`。
- 同 channel type 倍率 `1.5` 与 `2` 返回 `range`，`variant_count = 2`，min/max multiplier 与 equivalent min/max 方向正确。
- 无启用渠道返回空 equivalents。
- unlimited plan 返回 `kind = "unlimited"` 且 `token_unlimited = true`。
- self summary remaining `0` 的 raw JSON 必须包含 `"equivalent_token_remaining":0`。
- public plans 与 admin plans shape 一致。

运行：

```bash
go test ./controller -run 'Test.*Subscription.*Channel.*Equivalent|TestPublicSubscriptionPlans|TestSubscriptionSelfSummary' -count=1
```

---

### 任务 4：后端集成验收场景

**文件：**

- 测试：`controller/subscription_channel_equivalents_test.go` 或新增 `service/channel_token_multiplier_e2e_test.go`
- 如需测试辅助：只修改邻近测试 helper 文件

此任务在任务 2、任务 3 合并后串行执行，专门覆盖跨 billing / subscription API / log 的端到端验收。任务 2 和任务 3 不负责这个跨任务测试，避免并发依赖。

- [ ] **步骤 1：编写端到端验收测试**

测试场景：

- 渠道 A 倍率 `1`，渠道 B 倍率 `2`。
- 套餐 `monthly_token_limit = 1,000,000`。
- 套餐 API 对渠道 B 返回等价 token `500,000`。
- 一次可信 usage `10,000` 走渠道 B，订阅 `token_used` 增加 `20,000`。
- API Key token cap / audit 记录 `20,000`。
- 日志记录 `raw_metered_tokens = 10000`、`channel_billable_tokens = 20000`、`api_key_billable_tokens = 20000`、`subscription_billable_tokens = 20000`、`channel_token_billing_multiplier = 2`。
- 修改渠道 B 倍率为 `1.5` 后，新的套餐展示约 `666,666`，旧日志仍保留倍率 `2`，不回算。

- [ ] **步骤 2：运行后端集成验收测试**

运行：

```bash
go test ./controller ./service -run 'Test.*Channel.*Multiplier.*EndToEnd|Test.*Channel.*Multiplier.*Snapshot' -count=1
```

---

### 任务 5：前端渠道倍率配置

**文件：**

- 修改：`web/default/src/features/channels/types.ts`
- 修改：`web/default/src/features/channels/lib/channel-form.ts`
- 修改：`web/default/src/features/channels/components/drawers/channel-mutate-drawer.tsx`
- 测试：`web/default/src/features/channels/lib/channel-form.test.ts`

此任务不修改 locale JSON 和 `static-keys.ts`；只在完成报告中列出需要任务 8 添加的文案 key。

- [ ] **步骤 1：类型 schema**

`ChannelSchema` 增加：

```ts
token_billing_multiplier: z.number().default(1)
```

同步 `ChannelFormData` interface。如果确认该 interface 无调用，可删除或保持与 schema 一致，禁止类型语义分叉。

- [ ] **步骤 2：表单 Zod 校验与 number 转换**

在 `channelFormSchema` 中增加：

```ts
token_billing_multiplier: z.coerce
  .number({ message: 'Channel token billing multiplier is required' })
  .gt(0, 'Channel token billing multiplier must be greater than 0')
  .max(100, 'Channel token billing multiplier must be less than or equal to 100')
```

如果项目当前 Zod 版本不支持该写法，则 UI 层必须使用 `onChange={(e) => field.onChange(Number(e.target.value))}`，并用 schema 拒绝空字符串 / 非数字。

- [ ] **步骤 3：默认值和转换**

`CHANNEL_FORM_DEFAULT_VALUES.token_billing_multiplier = 1`。

`transformChannelToFormDefaults()`：

```ts
token_billing_multiplier: channel.token_billing_multiplier ?? 1
```

不能用 `|| 1`，避免掩盖异常 `0` 导致前端无法显示后端脏数据。

`transformFormDataToCreatePayload()` 和 `transformFormDataToUpdatePayload()` payload 增加 `token_billing_multiplier`。

- [ ] **步骤 4：表单 UI**

在 `channel-mutate-drawer.tsx` 合适位置增加 number input：

- label：`t('Channel token billing multiplier')`
- helper：`t('1 means raw token deduction; 2 means each upstream token deducts 2 subscription tokens; 0.5 means each upstream token deducts 0.5 subscription tokens.')`
- step：`0.01`
- min：`0.01`
- max：`100`
- value / onChange 必须按步骤 2 转 number。

遵守 web/default props 不过度解构规则。

- [ ] **步骤 5：保存后缓存刷新**

该步骤在任务 7 执行。任务 5 只实现字段、表单 UI 与 payload，不导入任务 6 / 7 创建的 query key 文件，不修改跨 feature invalidation。

任务 5 完成报告必须列出渠道保存成功当前只刷新 `channelsQueryKeys.lists()`，由任务 7 在前端集成阶段补充套餐相关 query invalidation。

- [ ] **步骤 6：前端定向测试**

新增 / 修改 `channel-form.test.ts`：

- 默认倍率为 `1`。
- API channel 转 form 保留倍率。
- create/update payload 包含倍率。
- `0`、负数、`101`、非数字、空字符串校验失败。
- number input 正常输入 `2` 后进入 form state 的值是 number，不是 string。
- 本任务不测试渠道保存后的跨 feature cache invalidation；该测试归任务 7。
运行：

```bash
cd web/default && bun test src/features/channels/lib/channel-form.test.ts
```

---

### 任务 6：前端套餐等价 token 展示与查询

**文件：**

- 修改：`web/default/src/features/subscriptions/types.ts`
- 创建：`web/default/src/features/subscriptions/query-keys.ts`
- 修改：`web/default/src/features/subscriptions/lib/format.ts`
- 修改：`web/default/src/features/wallet/lib/subscription-display.ts`
- 修改：`web/default/src/features/wallet/components/subscription-plans-card.tsx`
- 修改：`web/default/src/features/home/components/sections/plans-preview.tsx`
- 测试：`web/default/src/features/subscriptions/lib/format.test.ts`
- 测试：`web/default/src/features/wallet/components/subscription-plans-card.test.ts`
- 测试：`web/default/src/features/home/components/sections/plans-preview.test.tsx` 或现有相邻 Home 预览测试

此任务不修改 locale JSON 和 `static-keys.ts`；只在完成报告中列出需要任务 8 添加的文案 key。

- [ ] **步骤 1：新增 discriminated union 类型**

在 `subscriptions/types.ts` 增加规格第 8.1 的 `PlanChannelTokenEquivalent` 和 `SubscriptionChannelTokenEquivalent`，用 `kind: 'single' | 'range' | 'unlimited'` 区分。

`subscriptionPlanSchema` 增加可选 `channel_token_equivalents`，使用 `z.discriminatedUnion('kind', [...])`。

`PublicSubscriptionPlan` 和 `SelfSubscriptionSummary` 增加字段。

- [ ] **步骤 2：集中 query keys，并迁移 Wallet 到 React Query**

创建 `features/subscriptions/query-keys.ts`。注意 Wallet 和 Home 使用不同 API 和返回类型，query key 必须拆开，禁止共用缓存：

```ts
export const subscriptionQueryKeys = {
  walletPlans: ['subscriptions', 'plans'] as const,
  homePublicPlans: ['home', 'subscription-public-plans'] as const,
  selfSummary: ['subscriptions', 'self', 'summary'] as const,
  dashboardSelfSubscriptions: ['dashboard', 'overview', 'self-subscriptions'] as const,
  adminPlans: ['admin-subscription-plans'] as const,
}
```

把 `subscription-plans-card.tsx` 中手动 `useState` + `fetchPlans()` / `fetchSelfSubscription()` 迁移到 `useQuery`：

- `getPublicPlans()` 使用 `subscriptionQueryKeys.walletPlans`。
- `getSelfSubscriptionFull()` 使用 `subscriptionQueryKeys.selfSummary`。
- Home `getHomePublicPlansQuiet()` 继续使用独立 `subscriptionQueryKeys.homePublicPlans`。

任务 6 不修改 `channel-mutate-drawer.tsx`；渠道保存后的 invalidation 由任务 7 统一接入。

测试必须证明 Wallet plans 和 Home public plans 使用不同 query key，不会共享缓存；并证明 `SubscriptionPlansCard` 使用集中 query keys，在 query data 更新后重新渲染 equivalents。

- [ ] **步骤 3：新增 finite token formatter 并替换所有 finite 展示点**

在 `subscriptions/lib/format.ts` 新增：

```ts
export function formatFiniteTokenCount(value: number, t: TranslationFn): string {
  if (!Number.isFinite(value) || value <= 0) return `0 ${t('tokens')}`
  return formatTokenLimit(value, t)
}
```

必须替换当前订阅摘要中所有 finite count / remaining 展示点，特别是 `subscription-plans-card.tsx` 当前 `formatTokenLimit(remainTokens, t)` 要改为 `formatFiniteTokenCount(remainTokens, t)`。

`SubscriptionChannelTokenEquivalent` 的 remaining 单值和区间也必须使用 finite formatter；只有 `kind = 'unlimited'` 才显示 unlimited。

- [ ] **步骤 4：新增等价展示 helper**

在 `wallet/lib/subscription-display.ts` 或新文件：

```ts
export function formatPlanChannelEquivalent(item, t): string
export function formatSubscriptionChannelEquivalent(item, t): string
export function getVisibleChannelEquivalents(items, limit = 3)
export function shouldShowChannelEquivalents(items): boolean
```

规则：

- `single`：`OpenAI: about 1M tokens`。
- `range`：`Claude: about 500K - 666K tokens`。
- `unlimited`：`OpenAI: Unlimited tokens`。
- channel label 使用前端现有静态 `CHANNEL_TYPE_OPTIONS` / channel type map 通过 `channel_type` 查找；找不到时回退 `channel_type_name`。
- 不直接渲染后端返回的动态 `channel_type_label_key`，避免 static key 不完整；如果保留该字段，仅作为后端兼容字段。
- 所有 visible items 均为 `single` 且 `multiplier === 1` 时，`shouldShowChannelEquivalents()` 返回 false，不重复列出等价 token。

- [ ] **步骤 5：套餐卡片展示**

在 available plans benefits 中，在 `Monthly Token Limit` 下增加等价 token 说明。最多显示 3 个，多余项显示 `+N more` 或 tooltip / 折叠。

当所有渠道倍率都是 `1` 时，不重复展示与标准额度完全相同的渠道列表，只保留标准额度和说明。

- [ ] **步骤 6：当前订阅摘要展示**

只给当前 active subscription 展示 `selfSubscriptionData.summary.channel_token_equivalents` 的 remaining equivalents；非当前 active / 历史记录只展示基础套餐信息，不展示 remaining equivalents。

finite remaining `0` 必须显示 `0 tokens`。

- [ ] **步骤 7：Home 预览**

如果 `plans-preview.tsx` 显示 token 额度，则展示与 wallet 相同的简化 equivalents（最多 2 项 + `+N more`）。如果该组件只展示价格而不展示 token，则不新增 equivalents，但必须删除或调整任何与 wallet 语义冲突的旧 token 文案。

新增或修改 Home 相关测试（例如 `web/default/src/features/home/components/sections/plans-preview.test.tsx` 或项目现有相邻测试），覆盖 Home 使用 `subscriptionQueryKeys.homePublicPlans`，不会与 Wallet 的 `walletPlans` query key 共享缓存；若显示 equivalents，则覆盖最多 2 项 + `+N more` 和全 `1.0` 不重复展示。

- [ ] **步骤 8：前端测试**

测试：

- `formatFiniteTokenCount(0)` 返回 `0 tokens`。
- `unlimited` 显示 unlimited。
- `range` 显示 min-max。
- 全 `1.0` 倍率时卡片不重复误导。
- 套餐卡片渲染 `channel_token_equivalents`。
- 当前订阅 summary remaining `0` 显示 `0 tokens`。
- Wallet plans 和 Home public plans 不共享 query key / cache。
- Query data 更新后 Wallet 重新渲染 equivalents。
运行：

```bash
cd web/default && bun test src/features/subscriptions/lib/format.test.ts src/features/wallet/components/subscription-plans-card.test.ts src/features/home/components/sections/plans-preview.test.tsx
```

---

### 任务 7：前端保存后刷新集成

**文件：**

- 修改：`web/default/src/features/channels/components/drawers/channel-mutate-drawer.tsx`
- 测试：`web/default/src/features/channels/components/drawers/channel-mutate-drawer.test.tsx` 或当前项目相邻保存成功测试

此任务在任务 5、任务 6 合并后串行执行。任务 6 已创建 `features/subscriptions/query-keys.ts`，本任务只导入使用，不修改 locale JSON 和 `static-keys.ts`。

- [ ] **步骤 1：导入集中 query keys**

在 `channel-mutate-drawer.tsx` 保存成功逻辑中导入任务 6 创建的 `subscriptionQueryKeys`。

- [ ] **步骤 2：保存成功后刷新相关数据源**

渠道保存成功后：

- 保留现有 `channelsQueryKeys.lists()` invalidate。
- invalidate `subscriptionQueryKeys.walletPlans`。
- invalidate `subscriptionQueryKeys.homePublicPlans`。
- invalidate `subscriptionQueryKeys.selfSummary`。
- invalidate `subscriptionQueryKeys.dashboardSelfSubscriptions` prefix；当前实际 key 形如 `['dashboard','overview','self-subscriptions', user?.id]`。
- invalidate `subscriptionQueryKeys.adminPlans` prefix；当前实际 key 形如 `['admin-subscription-plans', refreshTrigger]`。
- `admin-ops`、`redemption-codes` 若只用计划标题/价格且不展示 token 额度，不强制刷新；如果实现时发现它们展示 token 等价信息，必须一并刷新对应 key。

- [ ] **步骤 3：刷新测试**

测试保存成功会调用 query invalidation，至少覆盖 channels list、wallet plans、home public plans、self summary、dashboard self subscriptions、admin plans。若现有组件测试不易直接覆盖 drawer，可抽出 `invalidateChannelTokenMultiplierRelatedQueries(queryClient)` helper 并测试该 helper。

运行：

```bash
cd web/default && bun test src/features/channels/components/drawers/channel-mutate-drawer.test.tsx
```

---

### 任务 8：前端 i18n 串行集成

**文件：**

- 修改：`web/default/src/i18n/locales/en.json`
- 修改：`web/default/src/i18n/locales/zh.json`
- 修改：`web/default/src/i18n/locales/fr.json`
- 修改：`web/default/src/i18n/locales/ru.json`
- 修改：`web/default/src/i18n/locales/ja.json`
- 修改：`web/default/src/i18n/locales/vi.json`
- 修改：`web/default/src/i18n/static-keys.ts`

此任务在任务 5、任务 6、任务 7 合并后串行执行。

- [ ] **步骤 1：添加渠道表单文案**

补充 6 个 locale：

- `Channel token billing multiplier`
- `1 means raw token deduction; 2 means each upstream token deducts 2 subscription tokens; 0.5 means each upstream token deducts 0.5 subscription tokens.`
- `Channel token billing multiplier is required`
- `Channel token billing multiplier must be greater than 0`
- `Channel token billing multiplier must be less than or equal to 100`

- [ ] **步骤 2：添加套餐展示文案**

补充 6 个 locale：

- `Equivalent usable tokens`
- `Estimated by current channel multiplier. Actual deduction depends on the channel used.`
- `About {{value}} tokens`
- `About {{min}} - {{max}} tokens`
- `{{count}} more channels`
- `Standard tokens`
- `By channel`
- `tokens`

- [ ] **步骤 3：static keys**

因为任务 6 规定前端用 `channel_type` 查静态 channel type map，不直接调用 `t(item.channel_type_label_key)`，所以不新增后端动态 label key 到 `static-keys.ts`。如果实现中实际引入任何动态 key，必须在 `static-keys.ts` 明确列出全部可能 key，并补齐 6 语言 locale。

- [ ] **步骤 4：i18n sync**

运行：

```bash
cd web/default && bun run i18n:sync
```

该命令会写文件；它属于同步步骤，不是纯检查。运行后检查生成变更，只保留本功能所需 locale / report 变更，移除无关重排或额外噪声。

---

## 3. 子代理调度与并行执行策略

### 3.1 调度方式

使用当前 harness 的 `task` 工具直接派发可写子代理。不使用 worktree。每个新子代理的 assignment 必须包含：

- 完整仓库路径：`C:/Users/34404/source/repos/new-api`。
- 完整规格路径：`C:/Users/34404/source/repos/new-api/docs/superpowers/specs/2026-06-10-channel-token-billing-multiplier-spec.md`。
- 完整计划路径：`C:/Users/34404/source/repos/new-api/docs/superpowers/plans/2026-06-10-channel-token-billing-multiplier-plan.md`。
- 本任务可写文件清单和不可写边界。
- 本任务定向测试命令。
- 禁止运行项目级全量 build/test/lint。
- 禁止修改 locale JSON，除任务 8 外。
- 完成回报格式：修改文件列表、新增/修改测试列表、实际运行命令与结果、未覆盖风险。

### 3.2 执行批次

1. **先执行任务 1**：建立后端字段、纯 helper、渠道 API 基础。这是任务 2 / 3 依赖。
2. **并发执行任务 2 与任务 3**：
   - 任务 2 修改 relay / billing / logging / retry。
   - 任务 3 修改 subscription API / equivalents。
   - 两者只能调用任务 1 创建的 `pkg/tokenbilling`，不得修改该包；如确需扩展，先通过 IRC 与主 Agent 确认。
3. **串行执行任务 4**：在任务 2 / 3 合并后，补跨 billing / subscription API / log 的后端集成验收。
4. **并发执行任务 5 与任务 6**：
   - 任务 5 修改 channels feature，不导入 subscription query keys，不做跨 feature invalidation。
   - 任务 6 修改 subscriptions / wallet / home feature，并拥有 `features/subscriptions/query-keys.ts`。
   - 两者不得修改 locale JSON 或 `static-keys.ts`。
5. **串行执行任务 7**：在任务 5 / 6 合并后，修改渠道保存成功后的相关 query invalidation。
6. **串行执行任务 8**：统一 i18n locale / static keys / `bun run i18n:sync`。
7. **review 循环**：每个批次完成后并发派发至少 3 个只读 review 子代理。所有 review 返回 `PASS` 后自动进入下一批；任一返回 `NEEDS_CHANGES` 时，主 Agent 修正后重新并发 review。

### 3.3 shared context 模板

派发实现子代理时 shared context 使用以下内容并按任务追加细节：

```text
# Goal
实现渠道 token 扣费倍率：渠道配置倍率，后端按倍率扣订阅/API Key token cap，套餐 API 返回按渠道倍率换算的等价 token，前端展示并保持刷新。

# Constraints
直接在主工作区 C:/Users/34404/source/repos/new-api 开发，不使用 worktree。必须读取并遵守规格和计划完整路径。不得运行项目级全量 build/test/lint；只运行 assignment 指定定向测试。不得修改未分配文件。除任务 8 外不得修改 web/default/src/i18n/locales/*.json 或 static-keys.ts。Go JSON marshal/unmarshal 只能用 common 包。DB 兼容 SQLite/MySQL/PostgreSQL。不得修改受保护项目/组织品牌信息。

# Contract
共享 token 倍率计算只在 pkg/tokenbilling；任务 2/3 调用它，不重新实现取整。渠道倍率字段为 token_billing_multiplier，合法范围 0 < value <= 100，未传创建默认 1，显式 0 拒绝，更新未传保留。预扣和结算用渠道计费 token；Codex Pro 额外调整只作用订阅，不作用 API Key cap。前端 query keys 由任务 6 提供，任务 7 只导入使用。
```

### 3.4 子代理 assignment 要求

每个实现 assignment 不少于 2000 字，必须包含：

- `# Target`：精确文件和符号；明确非目标。
- `# Change`：逐步编辑要求、API、测试用例。
- `# Acceptance`：可观察结果、定向命令、禁止事项。
- 说明与其他并发任务关系：谁拥有共享文件，谁只能调用。

每个 review assignment 不少于 2000 字，必须包含：

- 只读边界；不得编辑文件。
- 审查对象路径和本批次修改范围。
- 输出 schema：`verdict: PASS | NEEDS_CHANGES`、`must_fix`、`should_fix`、`evidence`。
- 要求列出文件路径、行段、原因。

---

## 4. 最终验证

所有任务合并后，主 Agent 运行：

```bash
go test ./pkg/tokenbilling -count=1
go test ./model -run 'Test.*Channel.*Multiplier|Test.*Retry.*Multiplier|Test.*Ability.*Multiplier|Test.*Subscription.*Channel.*Equivalent' -count=1
go test ./service -run 'Test.*Channel.*Multiplier|TestPostTextConsumeQuota.*Multiplier|TestPostTextConsumeQuotaCodexPro.*TokenLimit|Test.*Wss.*Multiplier|TestApplyChannelTokenBillingMultiplier|TestEquivalentTokensForMultiplier|Test.*UsageEstimated.*Subscription' -count=1
go test ./controller -run 'Test.*Channel.*Multiplier|Test.*Relay.*Multiplier|Test.*Retry.*Multiplier|Test.*Subscription.*Channel.*Equivalent|TestPublicSubscriptionPlans|TestSubscriptionSelfSummary' -count=1
```

前端：

```bash
cd web/default && bun test src/features/channels/lib/channel-form.test.ts src/features/channels/components/drawers/channel-mutate-drawer.test.tsx src/features/subscriptions/lib/format.test.ts src/features/wallet/components/subscription-plans-card.test.ts src/features/home/components/sections/plans-preview.test.tsx
cd web/default && bun run typecheck
cd web/default && bun run i18n:sync
```

`bun run i18n:sync` 会写文件；运行后必须检查生成变更，只保留本功能需要的 locale / report 变更。

如果改动触发 broader compile risk，再补充：

```bash
go test ./controller ./service ./model -run 'Test.*Subscription|Test.*Billing|Test.*Channel' -count=1
```

---

## 5. 完成定义映射

实现完成必须证明：

- `Channel.token_billing_multiplier` 默认 `1` 且校验 `0 < multiplier <= 100`。
- 创建未传倍率默认 `1`；显式 `0` 拒绝。
- 旧客户端更新渠道未传倍率时保留原值。
- 预扣前冻结独立 billing snapshot，不依赖提前初始化 `ChannelMeta`。
- retry 同倍率过滤覆盖 memory cache on/off，过滤后继续沿用 priority / weight。
- consume log 归属最终上游渠道，初始渠道仅做审计字段。
- 订阅与 API Key token cap 使用渠道计费 token。
- `BillingSession.preConsumedQuota`、`relayInfo.SubscriptionPreConsumed`、`FinalPreConsumedQuota`、API Key preconsume 使用同一渠道计费 token 口径。
- Codex Pro 额外订阅调整不作用于 API Key cap。
- `usageEstimated` / 无可信 usage 仍不扣费并退预扣。
- WSS / Realtime 增量只乘一次，post 阶段不重复 settle。
- 日志包含倍率、raw、channel billable、API Key billable、subscription billable；`logs.metered_tokens == subscription_billable_tokens`。
- `/api/subscription/plans`、`/api/subscription/public/plans`、`/api/subscription/self` 返回等价 token；无启用渠道返回空 equivalents。
- 前端类型安全展示 single/range/unlimited。
- finite remaining `0` 显示 `0 tokens`。
- 渠道保存后 Wallet、Home、Dashboard self summary、Admin plans 相关数据源刷新。
- i18n 由任务 8 串行补齐并同步。
- 后端定向测试、前端定向测试、typecheck、i18n sync 通过。
