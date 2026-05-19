# 订阅邀请奖励与钱包激活实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 让邀请奖励套餐与购买套餐按用户选择和套餐级别正确消耗时间与配额，并在钱包面板提供激活选择、配额重置和完整邀请奖励规则说明。

**架构：** 后端以 `UserSubscription.grant_reason` 区分购买、邀请权益和试用；用户当前激活订阅保存在 `users.setting.active_subscription_id`，扣费选择统一走 `selectPrimaryBillableSubscriptionTx`。邀请权益有效期改为两个有效付费直属下级中较短的 `end_time`，钱包前端基于 `/api/subscription/self` 返回字段显示激活状态、可切换项、可重置项和邀请规则。

**技术栈：** Go 1.22+、Gin、GORM、SQLite/MySQL/PostgreSQL 兼容；React 19、TypeScript、TanStack Query/Axios、Base UI/Tailwind、i18next。

---

## 文件结构

- 修改：`dto/user_settings.go` —— 增加 `ActiveSubscriptionId` 用户偏好字段。
- 修改：`model/user.go` —— 将设置 JSON marshal/unmarshal 改为 `common.Marshal` / `common.Unmarshal`，并保留新增字段。
- 修改：`model/subscription.go` —— 增加订阅来源/层级 helper、激活订阅选择、同级邀请优先、手动配额重置、self summary 新字段。
- 修改：`service/invitation_reward.go` —— 邀请权益有效期改为两个最长有效付费直属下级的交集。
- 修改：`controller/subscription.go` —— 增加激活订阅选择和配额重置接口。
- 修改：`router/api-router.go` —— 注册 `PUT /api/subscription/self/active` 与 `POST /api/subscription/self/:id/reset-quota`。
- 修改：`model/subscription_distributor_test.go` —— 覆盖扣费选择、同级奖励优先、手动重置。
- 修改：`service/invitation_reward_test.go` —— 覆盖两个最长下级交集有效期。
- 修改/创建：`controller/subscription_active_reset_test.go` —— 覆盖用户 API。
- 修改：`web/default/src/features/subscriptions/types.ts` —— 扩展后端字段类型。
- 修改：`web/default/src/features/subscriptions/api.ts` —— 增加激活切换与配额重置 API helper。
- 修改：`web/default/src/features/wallet/components/subscription-plans-card.tsx` —— 增加激活按钮、重置按钮、来源标签和有效到期展示。
- 修改：`web/default/src/features/wallet/components/affiliate-rewards-card.tsx` —— 增加完整邀请奖励规则说明。
- 修改：`web/default/src/i18n/locales/{en,zh,fr,ja,ru,vi}.json` —— 新增 UI 文案翻译。
- 修改：`web/default/src/features/wallet/wallet-layout.test.ts` 或新增 `subscription-actions.test.ts` —— 用源码契约测试覆盖关键 UI/接口契约。

---

### 任务 1：后端订阅选择、激活偏好与配额重置

**文件：**
- 修改：`dto/user_settings.go`
- 修改：`model/user.go`
- 修改：`model/subscription.go`
- 修改：`model/subscription_distributor_test.go`

- [ ] **步骤 1：编写失败测试**

在 `model/subscription_distributor_test.go` 增加以下测试：

```go
func TestPreConsumeUserSubscriptionPrioritizesSameTierInviteRewardOverPaid(t *testing.T) {
    truncateTables(t)
    require.NoError(t, DB.Create(&User{Id: 7601, Username: "same_tier", Status: common.UserStatusEnabled, AffCode: "aff7601"}).Error)
    ensureSubscriptionPreConsumeRecordTableForTest(t)
    seedDistributorSubscriptionPlanForTest(t, 7602, "basic_monthly", 100)
    paidEnd := common.GetTimestamp() + 30*86400
    rewardEnd := common.GetTimestamp() + 3*86400
    require.NoError(t, DB.Create(&UserSubscription{Id: 7603, UserId: 7601, PlanId: 7602, Status: "active", TokenLimit: 100, TokenUsed: 0, EndTime: paidEnd, GrantReason: "order", Source: "order"}).Error)
    require.NoError(t, DB.Create(&UserSubscription{Id: 7604, UserId: 7601, PlanId: 7602, Status: "active", TokenLimit: 100, TokenUsed: 0, EndTime: rewardEnd, GrantReason: "monthly_invite_entitlement", Source: "monthly_invite_entitlement"}).Error)

    pre, err := PreConsumeUserSubscription("same-tier-reward", 7601, "gpt-4o", 0, 6)

    require.NoError(t, err)
    assert.Equal(t, 7604, pre.UserSubscriptionId)
    var paid UserSubscription
    require.NoError(t, DB.First(&paid, 7603).Error)
    assert.Equal(t, int64(0), paid.TokenUsed)
    var reward UserSubscription
    require.NoError(t, DB.First(&reward, 7604).Error)
    assert.Equal(t, int64(6), reward.TokenUsed)
}

func TestPreConsumeUserSubscriptionUsesSelectedDifferentTierSubscription(t *testing.T) {
    truncateTables(t)
    user := User{Id: 7611, Username: "selected_tier", Status: common.UserStatusEnabled, AffCode: "aff7611"}
    require.NoError(t, DB.Create(&user).Error)
    ensureSubscriptionPreConsumeRecordTableForTest(t)
    seedDistributorSubscriptionPlanForTest(t, 7612, "basic_monthly", 100)
    seedDistributorSubscriptionPlanForTest(t, 7613, "pro_monthly", 100)
    now := common.GetTimestamp()
    require.NoError(t, DB.Create(&UserSubscription{Id: 7614, UserId: 7611, PlanId: 7612, Status: "active", TokenLimit: 100, TokenUsed: 0, EndTime: now + 3*86400, GrantReason: "monthly_invite_entitlement", Source: "monthly_invite_entitlement"}).Error)
    require.NoError(t, DB.Create(&UserSubscription{Id: 7615, UserId: 7611, PlanId: 7613, Status: "active", TokenLimit: 100, TokenUsed: 0, EndTime: now + 30*86400, GrantReason: "order", Source: "order"}).Error)
    setting := user.GetSetting()
    setting.ActiveSubscriptionId = 7615
    user.SetSetting(setting)
    require.NoError(t, DB.Save(&user).Error)

    pre, err := PreConsumeUserSubscription("selected-paid", 7611, "gpt-4o", 0, 5)

    require.NoError(t, err)
    assert.Equal(t, 7615, pre.UserSubscriptionId)
    var reward UserSubscription
    require.NoError(t, DB.First(&reward, 7614).Error)
    assert.Equal(t, int64(0), reward.TokenUsed)
}

func TestResetUserSubscriptionQuotaConsumesOneMonthFromPaidSubscription(t *testing.T) {
    truncateTables(t)
    require.NoError(t, DB.Create(&User{Id: 7621, Username: "reset_quota", Status: common.UserStatusEnabled, AffCode: "aff7621"}).Error)
    seedDistributorSubscriptionPlanForTest(t, 7622, "basic_monthly", 100)
    now := common.GetTimestamp()
    paidEnd := now + 70*86400
    require.NoError(t, DB.Create(&UserSubscription{Id: 7623, UserId: 7621, PlanId: 7622, Status: "active", TokenLimit: 100, TokenUsed: 88, AmountUsed: 12, StartTime: now - 86400, EndTime: paidEnd, GrantReason: "order", Source: "order"}).Error)

    result, err := ResetUserSubscriptionQuota(7621, 7623)

    require.NoError(t, err)
    require.NotNil(t, result)
    var sub UserSubscription
    require.NoError(t, DB.First(&sub, 7623).Error)
    assert.Equal(t, int64(0), sub.TokenUsed)
    assert.Equal(t, int64(0), sub.AmountUsed)
    assert.InDelta(t, paidEnd-30*86400, sub.EndTime, 2)
    assert.NotZero(t, sub.LastResetTime)
}
```

- [ ] **步骤 2：运行测试验证失败**

运行：

```bash
go test ./model -run 'TestPreConsumeUserSubscription(PrioritizesSameTierInviteRewardOverPaid|UsesSelectedDifferentTierSubscription)|TestResetUserSubscriptionQuotaConsumesOneMonthFromPaidSubscription' -count=1
```

预期：FAIL，原因包含 `ActiveSubscriptionId undefined`、`ResetUserSubscriptionQuota undefined` 或断言当前选择仍按旧排序。

- [ ] **步骤 3：实现最少后端模型代码**

实现要求：

1. `dto.UserSetting` 增加：

```go
ActiveSubscriptionId int `json:"active_subscription_id,omitempty"`
```

2. `model/user.go` 中 `GetSetting` / `SetSetting` 改用 `common.Unmarshal` / `common.Marshal`。

3. `model/subscription.go` 增加 helper：

```go
const (
    SubscriptionGrantOrder = "order"
    SubscriptionGrantMonthlyInviteEntitlement = "monthly_invite_entitlement"
)

func isPaidSubscription(sub *UserSubscription) bool
func isInvitationRewardSubscription(sub *UserSubscription) bool
func subscriptionTierKey(plan *SubscriptionPlan) string
func oneMonthSecondsFrom(now int64) int64
```

4. 修改 `selectPrimaryBillableSubscriptionTx`：
   - 读取 `users.setting.active_subscription_id`；
   - 如果选中的订阅有效、可扣费、额度足够，则优先返回；
   - 否则在候选中如果存在同 tier 的购买订阅和邀请奖励订阅，邀请奖励订阅排在对应购买订阅前；
   - 保持非同 tier 默认排序稳定。

5. 新增：

```go
type SubscriptionResetResult struct {
    SubscriptionId int   `json:"subscription_id"`
    EndTime        int64 `json:"end_time"`
    NextResetTime  int64 `json:"next_reset_time,omitempty"`
}

func ResetUserSubscriptionQuota(userId int, subscriptionId int) (*SubscriptionResetResult, error)
func SetUserActiveSubscription(userId int, subscriptionId int) error
```

`ResetUserSubscriptionQuota` 必须：
- 只允许 reset 当前用户 active 且未过期订阅；
- 只允许用 `grant_reason == "order"` 的购买套餐支付一个月有效期；
- 如果目标是邀请奖励套餐，寻找同用户同 tier 的购买套餐支付一个月；
- 剩余有效期不足 30 天则报错；
- 将目标套餐 `token_used` / `amount_used` 清零；
- 将支付套餐 `end_time` 扣 30 天；
- 如果目标和支付套餐相同，清零后扣自己的 `end_time`。

- [ ] **步骤 4：运行测试验证通过**

运行同一步骤 2 命令，预期 PASS。

- [ ] **步骤 5：Commit**

```bash
git add dto/user_settings.go model/user.go model/subscription.go model/subscription_distributor_test.go
git commit -m "feat(subscription): 支持激活订阅选择与配额重置"
```

---

### 任务 2：邀请权益有效期改为两个最长付费下级交集

**文件：**
- 修改：`service/invitation_reward.go`
- 修改：`service/invitation_reward_test.go`

- [ ] **步骤 1：编写失败测试**

在 `service/invitation_reward_test.go` 增加：

```go
func TestMonthlyInvitationEntitlementUsesTopTwoPaidInviteeOverlapEndTime(t *testing.T) {
    truncate(t)
    require.NoError(t, model.DB.AutoMigrate(&model.InvitationMonthlyEntitlement{}))
    at := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
    seedInvitationRewardUsers(t, 1601, 1602, 1603, 1604)
    seedInvitationRewardPlan(t, 2601, "basic_monthly", true)
    paidPlan := seedInvitationRewardPlan(t, 2602, "standard_monthly", true)
    seedPaidInviteeSubscriptionWithEnd(t, 1602, paidPlan.Id, at, at.Add(10*24*time.Hour).Unix())
    seedPaidInviteeSubscriptionWithEnd(t, 1603, paidPlan.Id, at, at.Add(20*24*time.Hour).Unix())
    seedPaidInviteeSubscriptionWithEnd(t, 1604, paidPlan.Id, at, at.Add(30*24*time.Hour).Unix())

    status, err := EnsureMonthlyInvitationEntitlement(1601, at)

    require.NoError(t, err)
    require.True(t, status.Entitled)
    assert.Equal(t, at.Add(20*24*time.Hour).Unix(), status.EntitlementEndTime)
    var sub model.UserSubscription
    require.NoError(t, model.DB.First(&sub, status.RewardSubscriptionId).Error)
    assert.Equal(t, at.Add(20*24*time.Hour).Unix(), sub.EndTime)
}

func seedPaidInviteeSubscriptionWithEnd(t *testing.T, userId int, planId int, at time.Time, end int64) {
    t.Helper()
    start := at.Add(-24 * time.Hour).Unix()
    tradeNo := fmt.Sprintf("paid-order-%d-%d", userId, end)
    require.NoError(t, model.DB.Create(&model.SubscriptionOrder{UserId: userId, PlanId: planId, Money: 10, TradeNo: tradeNo, PaymentProvider: model.PaymentProviderEpay, PaymentMethod: "alipay", Status: common.TopUpStatusSuccess, CreateTime: start, CompleteTime: start}).Error)
    require.NoError(t, model.DB.Create(&model.UserSubscription{UserId: userId, PlanId: planId, Status: "active", StartTime: start, EndTime: end, GrantReason: "order", Source: "order"}).Error)
}
```

确保 `service/invitation_reward_test.go` import 增加 `fmt`。

- [ ] **步骤 2：运行测试验证失败**

```bash
go test ./service -run 'TestMonthlyInvitationEntitlement(UsesTopTwoPaidInviteeOverlapEndTime|grants current month Basic)' -count=1
```

预期：FAIL，新测试会得到自然月末而不是第二长下级 `end_time`。

- [ ] **步骤 3：实现交集有效期**

在 `service/invitation_reward.go`：

- 新增 `qualifiedInviteeEndTime` 结构。
- 将 `countQualifiedActiveInviteesTx` 保留给状态计数。
- 新增 `listQualifiedActiveInviteeEndTimesTx(tx, inviterId, now) ([]qualifiedInviteeEndTime, error)`，使用当前合格条件，按 `user_subscriptions.end_time desc` 查询，并保证同一 invitee 只取最长有效期。
- `EnsureMonthlyInvitationEntitlement` 中，合格后用 top two 中较短的 `EndTime` 作为 `endTime`。
- `GetInvitationEntitlementStatus` 的 `EntitlementEndTime` 优先读权益订阅真实 `end_time`，不要再用 `monthEndUnix(at)`。

- [ ] **步骤 4：运行测试验证通过**

运行步骤 2 命令，预期 PASS。

- [ ] **步骤 5：Commit**

```bash
git add service/invitation_reward.go service/invitation_reward_test.go
git commit -m "feat(invitation): 使用付费下级交集计算奖励有效期"
```

---

### 任务 3：订阅用户 API 与路由

**文件：**
- 修改：`controller/subscription.go`
- 修改：`router/api-router.go`
- 创建：`controller/subscription_active_reset_test.go`

- [ ] **步骤 1：编写失败测试**

创建 `controller/subscription_active_reset_test.go`：

```go
package controller

import (
    "bytes"
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/QuantumNous/new-api/common"
    "github.com/QuantumNous/new-api/model"
    "github.com/gin-gonic/gin"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func setupSubscriptionActiveResetTestDB(t *testing.T) {
    t.Helper()
    gin.SetMode(gin.TestMode)
    db := setupModelListControllerTestDB(t)
    require.NoError(t, db.AutoMigrate(&model.User{}, &model.SubscriptionPlan{}, &model.UserSubscription{}, &model.SubscriptionPreConsumeRecord{}))
}

func performSetActiveSubscriptionRequest(t *testing.T, userID int, body string) *httptest.ResponseRecorder {
    t.Helper()
    recorder := httptest.NewRecorder()
    ctx, _ := gin.CreateTestContext(recorder)
    ctx.Request = httptest.NewRequest(http.MethodPut, "/api/subscription/self/active", bytes.NewBufferString(body))
    ctx.Request.Header.Set("Content-Type", "application/json")
    ctx.Set("id", userID)
    SetActiveSubscription(ctx)
    return recorder
}

func performResetSubscriptionQuotaRequest(t *testing.T, userID int, subID string) *httptest.ResponseRecorder {
    t.Helper()
    recorder := httptest.NewRecorder()
    ctx, _ := gin.CreateTestContext(recorder)
    ctx.Request = httptest.NewRequest(http.MethodPost, "/api/subscription/self/"+subID+"/reset-quota", nil)
    ctx.Set("id", userID)
    ctx.Params = gin.Params{{Key: "id", Value: subID}}
    ResetSubscriptionQuota(ctx)
    return recorder
}

func TestSetActiveSubscriptionPersistsUserChoice(t *testing.T) {
    setupSubscriptionActiveResetTestDB(t)
    userID := 9701
    require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "active_user", Status: common.UserStatusEnabled}).Error)
    code := "pro_monthly"
    require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: 9702, Title: "Pro", Enabled: true, MonthlyTokenLimit: 100, ConcurrencyLimit: 1, BusinessCode: &code}).Error)
    require.NoError(t, model.DB.Create(&model.UserSubscription{Id: 9703, UserId: userID, PlanId: 9702, Status: "active", TokenLimit: 100, EndTime: common.GetTimestamp() + 86400, GrantReason: "order", Source: "order"}).Error)

    recorder := performSetActiveSubscriptionRequest(t, userID, `{"subscription_id":9703}`)

    require.Equal(t, http.StatusOK, recorder.Code)
    assert.Contains(t, recorder.Body.String(), `"active_subscription_id":9703`)
    user, err := model.GetUserById(userID, false)
    require.NoError(t, err)
    assert.Equal(t, 9703, user.GetSetting().ActiveSubscriptionId)
}

func TestResetSubscriptionQuotaEndpointResetsAndConsumesPaidMonth(t *testing.T) {
    setupSubscriptionActiveResetTestDB(t)
    userID := 9711
    require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "reset_user", Status: common.UserStatusEnabled}).Error)
    code := "basic_monthly"
    require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: 9712, Title: "Basic", Enabled: true, DurationUnit: model.SubscriptionDurationMonth, DurationValue: 1, MonthlyTokenLimit: 100, ConcurrencyLimit: 1, BusinessCode: &code}).Error)
    end := common.GetTimestamp() + 70*86400
    require.NoError(t, model.DB.Create(&model.UserSubscription{Id: 9713, UserId: userID, PlanId: 9712, Status: "active", TokenLimit: 100, TokenUsed: 90, EndTime: end, GrantReason: "order", Source: "order"}).Error)

    recorder := performResetSubscriptionQuotaRequest(t, userID, "9713")

    require.Equal(t, http.StatusOK, recorder.Code)
    assert.Contains(t, recorder.Body.String(), `"subscription_id":9713`)
    var sub model.UserSubscription
    require.NoError(t, model.DB.First(&sub, 9713).Error)
    assert.Equal(t, int64(0), sub.TokenUsed)
    assert.Less(t, sub.EndTime, end)
}
```

- [ ] **步骤 2：运行测试验证失败**

```bash
go test ./controller -run 'Test(SetActiveSubscriptionPersistsUserChoice|ResetSubscriptionQuotaEndpointResetsAndConsumesPaidMonth)' -count=1
```

预期：FAIL，controller 函数未定义。

- [ ] **步骤 3：实现 controller 与路由**

在 `controller/subscription.go` 增加：

```go
type ActiveSubscriptionRequest struct {
    SubscriptionId int `json:"subscription_id"`
}

func SetActiveSubscription(c *gin.Context)
func ResetSubscriptionQuota(c *gin.Context)
```

行为：
- 参数错误返回 `common.ApiErrorMsg(c, "参数错误")`。
- 调用 `model.SetUserActiveSubscription` / `model.ResetUserSubscriptionQuota`。
- 成功返回 `common.ApiSuccess`，包含 `active_subscription_id` 或 reset result。

在 `router/api-router.go` 的 subscriptionRoute 增加：

```go
subscriptionRoute.PUT("/self/active", controller.SetActiveSubscription)
subscriptionRoute.POST("/self/:id/reset-quota", controller.ResetSubscriptionQuota)
```

- [ ] **步骤 4：运行测试验证通过**

运行步骤 2 命令，预期 PASS。

- [ ] **步骤 5：Commit**

```bash
git add controller/subscription.go router/api-router.go controller/subscription_active_reset_test.go
git commit -m "feat(subscription): 增加用户激活订阅与重置接口"
```

---

### 任务 4：前端钱包激活选择、重置和邀请规则说明

**文件：**
- 修改：`web/default/src/features/subscriptions/types.ts`
- 修改：`web/default/src/features/subscriptions/api.ts`
- 修改：`web/default/src/features/wallet/components/subscription-plans-card.tsx`
- 修改：`web/default/src/features/wallet/components/affiliate-rewards-card.tsx`
- 修改：`web/default/src/i18n/locales/{en,zh,fr,ja,ru,vi}.json`
- 修改：`web/default/src/features/wallet/wallet-layout.test.ts`

- [ ] **步骤 1：编写失败测试**

扩展 `web/default/src/features/wallet/wallet-layout.test.ts`：

```ts
test('wallet subscriptions expose active selection and quota reset actions', () => {
  const source = readWalletSource()
  assert.match(source, /setActiveSubscription/)
  assert.match(source, /resetSubscriptionQuota/)
  assert.match(source, /Set as active/)
  assert.match(source, /Reset quota/)
})

test('affiliate card documents invitation reward rules near referral link', () => {
  const source = readFileSync('src/features/wallet/components/affiliate-rewards-card.tsx', 'utf8')
  assert.match(source, /Invitation reward rules/)
  assert.match(source, /two longest valid paid referrals/)
  assert.match(source, /same tier/)
})
```

- [ ] **步骤 2：运行测试验证失败**

```bash
cd web/default && bun test src/features/wallet/wallet-layout.test.ts
```

预期：FAIL，源码中没有这些 API 和文案。

- [ ] **步骤 3：实现 API 与类型**

在 `web/default/src/features/subscriptions/types.ts`：
- `UserSubscription` 增加 `effective_end_time?: number`、`is_active_selected?: boolean`、`can_reset_quota?: boolean`、`source_label?: string`。
- `SelfSubscriptionData` 增加 `active_subscription_id?: number`。

在 `web/default/src/features/subscriptions/api.ts` 增加：

```ts
export interface SetActiveSubscriptionRequest { subscription_id: number }
export async function setActiveSubscription(data: SetActiveSubscriptionRequest): Promise<ApiResponse<{ active_subscription_id: number }>>
export async function resetSubscriptionQuota(subscriptionId: number): Promise<ApiResponse<{ subscription_id: number; end_time: number; next_reset_time?: number }>>
```

- [ ] **步骤 4：实现钱包 UI**

在 `subscription-plans-card.tsx`：
- import 新 API helper。
- 保存 `activeSubscriptionId` 状态，读取 `res.data.active_subscription_id`。
- 每个 active 订阅显示：
  - 当前激活：`Current active` badge。
  - 非当前且 active：`Set as active` 按钮，调用 `setActiveSubscription` 后刷新。
  - 可重置：`Reset quota` 按钮，确认后调用 `resetSubscriptionQuota`。
- 来源显示：
  - `grant_reason === 'order'` → `Paid plan`
  - `grant_reason === 'monthly_invite_entitlement'` → `Invitation reward`
  - `grant_reason === 'trial_code' || 'invite_trial'` → `Trial`
- 文案全部用 `t()`。

在 `affiliate-rewards-card.tsx` 邀请链接附近增加规则说明区块，包含：
- `Invitation reward rules`
- `Invite at least two direct users with active paid subscriptions to receive a Basic reward plan.`
- `The reward is valid until the overlap end time of your two longest valid paid referrals.`
- `When the reward is the same tier as your paid plan, reward time is consumed first and paid time is preserved.`
- `When tiers differ, choose the active plan in Wallet. Reward usage does not consume paid plan time; paid plan usage lets both natural validity windows elapse.`
- `Quota reset consumes one month from a paid plan and cannot be paid by invitation rewards.`

- [ ] **步骤 5：补齐 i18n**

向 6 个 locale JSON 增加以上 key，英文 key/value 一致，其他语言给出自然翻译。

- [ ] **步骤 6：运行测试验证通过**

```bash
cd web/default && bun test src/features/wallet/wallet-layout.test.ts
```

预期 PASS。

- [ ] **步骤 7：Commit**

```bash
git add web/default/src/features/subscriptions/types.ts web/default/src/features/subscriptions/api.ts web/default/src/features/wallet/components/subscription-plans-card.tsx web/default/src/features/wallet/components/affiliate-rewards-card.tsx web/default/src/i18n/locales/en.json web/default/src/i18n/locales/zh.json web/default/src/i18n/locales/fr.json web/default/src/i18n/locales/ja.json web/default/src/i18n/locales/ru.json web/default/src/i18n/locales/vi.json web/default/src/features/wallet/wallet-layout.test.ts
git commit -m "feat(wallet): 支持订阅激活切换和邀请规则说明"
```

---

### 任务 5：最终验证

**文件：**
- 全部受影响文件

- [ ] **步骤 1：格式化受影响 Go 文件**

```bash
gofmt -w dto/user_settings.go model/user.go model/subscription.go service/invitation_reward.go controller/subscription.go router/api-router.go model/subscription_distributor_test.go service/invitation_reward_test.go controller/subscription_active_reset_test.go
```

- [ ] **步骤 2：运行后端定向测试**

```bash
go test ./model -run 'TestPreConsumeUserSubscription(PrioritizesSameTierInviteRewardOverPaid|UsesSelectedDifferentTierSubscription|_IgnoresAmountTotalForDistributorLimit|ByUnits_UsesUnitsForSelectedDistributor)|TestResetUserSubscriptionQuotaConsumesOneMonthFromPaidSubscription|TestGetSubscriptionSelf' -count=1
go test ./service -run 'TestMonthlyInvitationEntitlement' -count=1
go test ./controller -run 'Test(SetActiveSubscriptionPersistsUserChoice|ResetSubscriptionQuotaEndpointResetsAndConsumesPaidMonth|SubscriptionBalancePay)' -count=1
```

预期全部 PASS。

- [ ] **步骤 3：运行前端定向测试与类型检查**

```bash
cd web/default && bun test src/features/wallet/wallet-layout.test.ts src/features/subscriptions/api.test.ts
cd web/default && bun run typecheck
```

预期全部 PASS。

- [ ] **步骤 4：i18n 同步**

```bash
cd web/default && bun run i18n:sync
```

预期命令成功，新增 key 不缺失。

- [ ] **步骤 5：最终 diff 审查**

```bash
git diff --stat
git diff --check
```

预期无 whitespace error，diff 只包含计划文件中的受影响文件。
