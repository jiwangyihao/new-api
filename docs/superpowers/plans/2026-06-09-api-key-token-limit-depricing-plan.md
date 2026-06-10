# API Key token 限额去价格化实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。实现必须遵守 TDD：先写失败测试，确认失败，再写最少生产代码。本文计划用于当前主分支直接开发，不使用 worktree。

**目标：** 将 API Key 限额从旧余额 / quota 口径切换为新 token cap 口径；历史 API Key 新 token cap 全部置空为未启用，旧字段保留兼容但不再作为 default 前端和新运行时的限额来源。

**架构：** 后端在 `model.Token` 上新增 `token_limit_enabled`、`token_limit`、`token_used`，新增 token cap 预扣记录和服务；订阅 token 仍是唯一请求资金来源，API Key token cap 只作为单 key guard。前端 default 主题移除 `remain_quota_dollars`、货币换算和 `formatQuota()` API Key 限额展示，改用 token 字段、token 表单和 i18n 文案。

**技术栈：** Go 1.25.1、Gin、GORM v2、SQLite / MySQL / PostgreSQL、React 19、TypeScript、React Hook Form、Zod、TanStack Query、Bun、i18next。

**规格来源：** `C:/Users/34404/source/repos/new-api/docs/superpowers/specs/2026-06-09-api-key-token-limit-depricing-spec.md`。实现前必须完整阅读该规格。任务 1 的模型导出 API 必须包含 `model.ConsumeTokenLimitIncrement`，用于 Realtime / WSS 增量 key cap。

---

## 计划说明

- 本计划按文件边界拆分，减少并发写入冲突。
- 所有子代理都必须先写测试，确认测试失败，再实现。
- 子代理不得运行项目级 build / lint / 全量测试 / 格式化；每个任务只运行自己新增或修改的定向测试。最终由主代理统一运行验证。
- 所有 Go JSON 编解码必须使用 `common.Marshal` / `common.Unmarshal` 等项目封装，不能直接调用 `encoding/json` 的 marshal / unmarshal。
- 所有数据库逻辑必须兼容 SQLite、MySQL、PostgreSQL；优先 GORM，避免数据库专属 SQL。
- 受保护的项目名称、组织名称、版权头、模块路径和品牌信息不得修改。
- 前端 API Key 定向测试必须从 `web/default/` 使用 Bun 执行 `src/features/keys/api-key-form-visibility.test.ts`，因为测试会直接 import TypeScript 模块，不能使用 Node 内置 test runner。

---

## 文件结构

### 后端模型与 token cap 服务

- 修改：`model/token.go`
  - 新增 `TokenLimitEnabled`、`TokenLimit`、`TokenUsed` 字段。
  - 新增 `TokenLimitRemaining()`、`TokenLimitUnlimited()`、`BuildTokenLimitView()` 之类的派生辅助。
  - 新增 token cap 原子预扣、结算、退款、重置用量函数。
  - 更新 `Update()` 的 Select 字段，确保新字段可保存并刷新缓存。
- 修改：`model/main.go`
  - `migrateDB()` 的 `DB.AutoMigrate(...)` 列表和 `migrateDBFast()` 的 `migrations` 切片都必须加入 `TokenLimitPreConsumeRecord`。
- 新增或修改：`model/token_limit_preconsume.go`
  - 定义 `TokenLimitPreConsumeRecord`、状态常量、幂等预扣 / settle / refund / reset 导出 API。
  - 所有修改 `token_used` 的导出 API 统一做 Redis token cache 失效。
- 修改：`model/token_validation_test.go`
  - 覆盖历史旧 quota 不影响鉴权，新字段默认未启用。
- 修改：`controller/token_test.go`
  - 覆盖 token 表迁移新增字段不迁移旧值，以及 token API 新字段契约。

### 后端控制器和路由

- 修改：`controller/token.go`
  - 构造 API Key 响应 DTO，包含新 token cap 字段和 legacy 字段。
  - 创建 / 更新 API Key 使用 `token_limit_enabled` / `token_limit` 校验，不再用 `QuotaPerUnit` 校验 API Key token limit。
  - 新增 `ResetTokenUsage` handler。
  - `GetTokenStatus()` / `GetTokenUsage()` 返回新 token 字段并保留 legacy credit 字段。
- 修改：`router/api-router.go`
  - 增加 `POST /api/token/:id/reset-token-usage`，注意放在 `GET /:id` 附近且不与 `/:id` 路由冲突。
- 修改：`controller/config_guide.go`
  - 配置向导不再因 legacy `TokenStatusExhausted` 直接拒绝可用 key。
  - 不再写入旧 `token_quota` 作为可用性依据；保留 token model limit / IP / 用户状态校验。
- 修改：`controller/config_guide_test.go` 和 `controller/token_test.go`
  - 更新 exhausted 场景，保证与主鉴权语义一致。

### 后端运行时生命周期

- 新增：`service/token_limit_session.go`
  - 定义 `TokenLimitSession`，提供 `PreConsume`、`Settle`、`Refund`、`Noop`、状态查询。
  - 错误使用 OpenAI-compatible `api_key_token_limit_exhausted`，HTTP 429。
  - 预扣失败不增加 `TokenUsed`。
  - settle 使用订阅结算同一 metered token 数。
- 修改：`service/billing.go`
  - `BillingSettleInput` 增加 `ApiKeyTokens int64` 和 `ResponseStarted bool`，用于区分普通响应和已发送响应后的审计结算。
  - `SettleBillingWithInput` 必须按固定顺序处理 key cap 与订阅 settle，并在失败时执行跨账本补偿。
- 修改：`service/billing_session.go`
  - 保留订阅-only 语义；为 key cap 拒绝后的订阅回滚提供同步退款包装，例如 `RefundBillingAfterTokenLimitReject`。
- 修改：`relay/common/billing.go`
  - 新增 `TokenLimitSettler` 接口，避免 `relay/common` 直接 import `service` 产生循环依赖。
- 修改：`relay/common/relay_info.go`
  - 在 `RelayInfo` 上新增 `TokenLimit TokenLimitSettler` 字段，并提供读取订阅预扣 token 的 helper。
- 修改：`controller/relay.go`
  - 在订阅预扣和订阅并发租约成功后执行 API Key cap 预扣。
  - 如果 key cap 预扣失败：释放并发租约、同步退款订阅预扣，返回 `api_key_token_limit_exhausted`。
  - 如果后续失败：现有 billing refund 之外，也要 refund key cap。
- 修改：`service/text_quota.go`
  - 在 `PostTextConsumeQuota()` 计算出 `subscriptionTokens` 后，将同一值用于 key cap settle。
  - 不直接使用 `usage.TotalTokens`、`prompt+completion` 或旧 quota 结算 key cap。
- 修改：`service/task_billing.go`
  - 停止 `RefundTaskQuota()` / `RecalculateTaskQuota()` 调整旧 API Key quota。
  - 如果任务不支持订阅 token cap，不接入 key cap；本次至少保证旧 `remain_quota` / `used_quota` 不再被任务退款 / 重算修改。
- 修改：`service/subscription_only_billing_test.go` 或新增 `service/token_limit_session_test.go`
  - 覆盖运行时 lifecycle：预扣、退款、settle delta、旧 quota 不参与、订阅回滚。
- 修改：`service/subscription_billing_test.go`
  - 覆盖 `PostTextConsumeQuota()` 使用订阅 metered token 同步 key cap。
- 修改：`service/task_billing_test.go`
  - 覆盖任务退款 / 重算不再改旧 token quota。

### 前端 default API Key

- 修改：`web/default/src/features/keys/types.ts`
  - `ApiKey` schema 增加 `token_limit_enabled`、`token_limit`、`token_used`、`token_remaining`、`token_unlimited`。
  - `ApiKeyFormData` 改为提交 `token_limit_enabled` / `token_limit`，不再提交 `remain_quota` / `unlimited_quota` 作为限额配置。
- 修改：`web/default/src/features/keys/lib/api-key-form.ts`
  - 删除 `parseQuotaFromDollars` / `quotaUnitsToDollars` import。
  - 删除 `remain_quota_dollars`。
  - 新增 Zod `superRefine`：启用 token limit 时 `token_limit > 0`。
  - 历史 key 不从旧 quota fallback；新字段缺失时默认未启用。
  - payload 显式构造，不展开临时字段。
- 修改：`web/default/src/features/keys/components/api-keys-mutate-drawer.tsx`
  - 删除 `getCurrencyDisplay` / `getCurrencyLabel` 和 `WalletCards` 限额图标依赖。
  - UI 改为 `API Key Token Limit`、`No token limit for this API key`、`Token limit`。
  - 文案说明只限制该 key，请求仍消耗订阅 token。
  - 默认关闭 token limit。
- 修改：`web/default/src/features/keys/components/api-keys-columns.tsx`
  - 删除 API Key limit 的 `formatQuota()` 使用。
  - 展示 `formatTokens(token_used) / formatTokens(token_limit)`、`token_remaining`，remaining 为 0 显示明确 0。
- 修改：`web/default/src/features/keys/components/api-keys-table.tsx`
  - 移动端卡片同步 token cap 展示。
- 修改：`web/default/src/features/keys/api.ts`
  - 新增 `resetApiKeyTokenUsage()`。
- 修改：`web/default/src/i18n/locales/{en,zh,fr,ja,ru,vi}.json`
  - 补齐新增文案。
- 修改：`web/default/src/i18n/static-keys.ts`
  - 如状态 / 常量间接调用 `t(config.label)`，登记新 key。
- 修改：`web/default/src/features/keys/api-key-form-visibility.test.ts`
  - 增加静态回归：不再出现 `remain_quota_dollars`、不再导入旧 quota 转换、不再在 API Key 表单 / 列表使用 `formatQuota()`。

---

## 任务 1：后端模型和 token cap 记录

**文件：**
- 修改：`model/token.go`
- 新增：`model/token_limit_preconsume.go`
- 修改：`model/main.go`
- 修改：`model/token_validation_test.go`
- 修改：`controller/token_test.go`

- [x] **步骤 1：编写失败测试：历史旧 quota 不迁移到新 token cap**

在 `controller/token_test.go` 新增或扩展迁移测试。使用现有 `legacyToken` 创建旧结构数据，然后 `AutoMigrate(&model.Token{}, &model.TokenLimitPreConsumeRecord{})`。断言：

```go
func TestTokenLimitFieldsDefaultEmptyForLegacyTokens(t *testing.T) {
    db := setupTokenControllerTestDB(t)
    require.NoError(t, db.Migrator().DropTable(&model.Token{}))
    require.NoError(t, db.AutoMigrate(&legacyToken{}))
    require.NoError(t, db.Create(&legacyToken{
        Id:             91001,
        UserId:         91002,
        Key:            "sk-legacy-limit",
        Status:         common.TokenStatusEnabled,
        Name:           "legacy-limit",
        ExpiredTime:    -1,
        RemainQuota:    123456,
        UsedQuota:      654321,
        UnlimitedQuota: false,
    }).Error)

    require.NoError(t, db.AutoMigrate(&model.Token{}, &model.TokenLimitPreConsumeRecord{}))

    var token model.Token
    require.NoError(t, db.First(&token, 91001).Error)
    require.False(t, token.TokenLimitEnabled)
    require.Equal(t, int64(0), token.TokenLimit)
    require.Equal(t, int64(0), token.TokenUsed)
    require.Equal(t, 123456, token.RemainQuota)
    require.Equal(t, 654321, token.UsedQuota)
    require.False(t, token.UnlimitedQuota)
}
```

- [x] **步骤 2：运行测试验证失败**

运行：

```bash
go test ./controller -run TestTokenLimitFieldsDefaultEmptyForLegacyTokens
```

预期：失败，原因是 `model.TokenLimitPreConsumeRecord` 未定义或 `model.Token` 未包含新字段。

- [x] **步骤 3：编写模型最少实现**

在 `model/token.go` 的 `Token` 中新增字段：

```go
TokenLimitEnabled bool  `json:"token_limit_enabled" gorm:"not null;default:false"`
TokenLimit        int64 `json:"token_limit" gorm:"type:bigint;not null;default:0"`
TokenUsed         int64 `json:"token_used" gorm:"type:bigint;not null;default:0"`
```

新增辅助方法：

```go
func (token *Token) TokenLimitUnlimited() bool {
    return token == nil || !token.TokenLimitEnabled
}

func (token *Token) TokenLimitRemaining() int64 {
    if token == nil || !token.TokenLimitEnabled {
        return 0
    }
    remaining := token.TokenLimit - token.TokenUsed
    if remaining < 0 {
        return 0
    }
    return remaining
}
```

在 `model/token_limit_preconsume.go` 新增：

```go
package model

const (
    TokenLimitPreConsumeStatusConsumed     = "consumed"
    TokenLimitPreConsumeStatusRefunded     = "refunded"
    TokenLimitPreConsumeStatusSettled      = "settled"
    TokenLimitPreConsumeStatusSettleFailed = "settle_failed"
)

type TokenLimitPreConsumeRecord struct {
    Id                int    `json:"id"`
    RequestId         string `json:"request_id" gorm:"type:varchar(64);uniqueIndex;not null"`
    UserId            int    `json:"user_id" gorm:"index;not null"`
    TokenId           int    `json:"token_id" gorm:"index;not null"`
    PreConsumedTokens int64  `json:"pre_consumed_tokens" gorm:"type:bigint;not null;default:0"`
    ActualTokens      int64  `json:"actual_tokens" gorm:"type:bigint;not null;default:0"`
    DeltaTokens       int64  `json:"delta_tokens" gorm:"type:bigint;not null;default:0"`
    FailureCode       string `json:"failure_code" gorm:"type:varchar(64);not null;default:''"`
    Status            string `json:"status" gorm:"type:varchar(16);not null;default:'consumed'"`
    CreatedAt         int64  `json:"created_at" gorm:"bigint"`
    UpdatedAt         int64  `json:"updated_at" gorm:"bigint"`
}
```

在 `model/main.go` 同时更新两条迁移路径：

```go
// migrateDB(): DB.AutoMigrate(...) 参数中加入
&TokenLimitPreConsumeRecord{},

// migrateDBFast(): migrations 切片中加入
{&TokenLimitPreConsumeRecord{}, "TokenLimitPreConsumeRecord"},
```

迁移测试除断言 `tokens` 新字段默认值外，还要断言 `token_limit_pre_consume_records` 表存在。

更新 `Token.Update()` 的 Select 字段，加入：

```go
"token_limit_enabled", "token_limit", "token_used"
```

- [x] **步骤 4：运行测试验证通过**

运行：

```bash
go test ./controller -run TestTokenLimitFieldsDefaultEmptyForLegacyTokens
```

预期：PASS。

- [x] **步骤 5：编写失败测试：原子预扣幂等且不读旧 quota**

在 `model/token_validation_test.go` 或新增 `model/token_limit_test.go` 写测试：

```go
func TestTokenLimitPreConsumeIgnoresLegacyQuotaAndIsIdempotent(t *testing.T) {
    setupTokenValidationTestDB(t)
    require.NoError(t, DB.AutoMigrate(&TokenLimitPreConsumeRecord{}))
    require.NoError(t, DB.Create(&Token{
        Id:                92001,
        UserId:            92002,
        Key:               "sk-token-limit",
        Status:            common.TokenStatusEnabled,
        ExpiredTime:       -1,
        RemainQuota:       0,
        UsedQuota:         999999,
        UnlimitedQuota:    false,
        TokenLimitEnabled: true,
        TokenLimit:        100,
        TokenUsed:         90,
    }).Error)

    ok, err := PreConsumeTokenLimit(92001, 92002, "req-token-limit", 10)
    require.NoError(t, err)
    require.True(t, ok)

    ok, err = PreConsumeTokenLimit(92001, 92002, "req-token-limit", 10)
    require.NoError(t, err)
    require.True(t, ok, "same request_id is idempotent")

    var token Token
    require.NoError(t, DB.First(&token, 92001).Error)
    require.Equal(t, int64(100), token.TokenUsed)
    require.Equal(t, 0, token.RemainQuota)
    require.Equal(t, 999999, token.UsedQuota)

    ok, err = PreConsumeTokenLimit(92001, 92002, "req-token-limit-over", 1)
    require.NoError(t, err)
    require.False(t, ok)
}
```

同一文件还必须新增 settle / refund / reset 幂等测试，覆盖重复调用、reset 后旧在途记录不污染新值：

```go
func TestTokenLimitSettleRefundResetAndCacheInvalidation(t *testing.T) {
    setupTokenValidationTestDB(t)
    require.NoError(t, DB.AutoMigrate(&TokenLimitPreConsumeRecord{}))
    require.NoError(t, DB.Create(&Token{
        Id:                92011,
        UserId:            92012,
        Key:               "sk-token-limit-cache",
        Status:            common.TokenStatusEnabled,
        ExpiredTime:       -1,
        TokenLimitEnabled: true,
        TokenLimit:        100,
        TokenUsed:         50,
    }).Error)

    ok, err := PreConsumeTokenLimit(92011, 92012, "req-token-limit-delta", 20)
    require.NoError(t, err)
    require.True(t, ok)
    require.NoError(t, SettleTokenLimitPreConsume("req-token-limit-delta", 10))
    require.NoError(t, SettleTokenLimitPreConsume("req-token-limit-delta", 10))

    var token Token
    require.NoError(t, DB.First(&token, 92011).Error)
    require.Equal(t, int64(60), token.TokenUsed)
    var settled TokenLimitPreConsumeRecord
    require.NoError(t, DB.Where("request_id = ?", "req-token-limit-delta").First(&settled).Error)
    require.Equal(t, TokenLimitPreConsumeStatusSettled, settled.Status)
    require.Equal(t, int64(10), settled.ActualTokens)
    require.Equal(t, int64(-10), settled.DeltaTokens)

    ok, err = PreConsumeTokenLimit(92011, 92012, "req-token-limit-reset", 30)
    require.NoError(t, err)
    require.True(t, ok)
    before, err := ResetTokenUsage(92011, 92012)
    require.NoError(t, err)
    require.Equal(t, int64(90), before)
    require.NoError(t, RefundTokenLimitPreConsume("req-token-limit-reset", "usage_reset_after_refund"))
    require.NoError(t, SettleTokenLimitPreConsume("req-token-limit-reset", 30))

    require.NoError(t, DB.First(&token, 92011).Error)
    require.Equal(t, int64(0), token.TokenUsed, "old in-flight record must not restore or subtract after reset")
}
```

必须新增独立 Redis 缓存一致性测试 `TestTokenLimitWritesInvalidateTokenCache`。该测试必须使用项目已有 `miniredis` 测试模式，并做成表驱动红灯测试，逐一覆盖所有会写入 `tokens.token_used` / `tokens.token_limit_enabled` / `tokens.token_limit` 的路径：`PreConsumeTokenLimit`、`SettleTokenLimitPreConsume`（actual 小于预扣的负 delta 和 actual 大于预扣的正 delta 各一例）、`RefundTokenLimitPreConsume`、`ConsumeTokenLimitIncrement`、`ResetTokenUsage`、`Token.Update()` 修改 `token_limit_enabled`、`Token.Update()` 修改 `token_limit`、`Token.Update()` 修改 `token_used`。每个 case 都必须先同步调用 `cacheSetToken(oldToken)`，再调用 `cacheGetTokenByKey(key)` 断言旧缓存命中且值为旧的 `token_limit_enabled` / `token_limit` / `token_used`；然后执行写操作，最后调用 `GetTokenByKey(key, false)` 断言返回数据库最新值。不得只依赖 `GetTokenByKey(key, true)` 的异步 cache 写入，因为当前源码通过 `gopool.Go` 异步写 Redis，可能导致测试假阳性。不得只覆盖其中任一写路径。

- [x] **步骤 6：运行测试验证失败**

运行：

```bash
go test ./model -run 'TestTokenLimitPreConsumeIgnoresLegacyQuotaAndIsIdempotent|TestTokenLimitSettleRefundResetAndCacheInvalidation|TestTokenLimitWritesInvalidateTokenCache|TestValidateUserTokenAllows'
```

预期：失败，原因是 `PreConsumeTokenLimit` 和预扣记录模型 API 未定义。

- [x] **步骤 7：实现模型原子操作和导出 API**

在 `model/token_limit_preconsume.go` 增加导出 API，`service` 包只能通过这些函数操作 key cap，不得重复写表更新逻辑：

```go
func PreConsumeTokenLimit(tokenId int, userId int, requestId string, tokens int64) (bool, error)
func SettleTokenLimitPreConsume(requestId string, actualTokens int64) error
func RefundTokenLimitPreConsume(requestId string, failureCode string) error
func ConsumeTokenLimitIncrement(tokenId int, userId int, idempotencyKey string, tokens int64) (bool, error)
func MarkTokenLimitSettleFailed(requestId string, actualTokens int64, failureCode string) error
func ResetTokenUsage(tokenId int, userId int) (before int64, err error)
```

`PreConsumeTokenLimit` 必须使用 `DB.Transaction` 保证记录和 `tokens.token_used` 原子一致：

1. `tokens <= 0` 或 token limit 未启用时 no-op 成功。
2. 在事务内先按 `request_id` 查询 `TokenLimitPreConsumeRecord`；已存在 `consumed` / `settled` 记录时直接返回成功，不再次增加 `token_used`；已存在 `refunded` 记录时返回 `ok=false`，不得重复扣减。
3. 新请求必须在同一事务中先创建 `consumed` 记录，再执行条件更新 `tokens.token_used`；条件更新失败时返回错误触发事务回滚，不能留下记录。
4. 条件更新使用 `RowsAffected` 判断 cap 是否不足；不足时整个事务回滚，不留下记录且返回 `ok=false`。
5. 唯一索引冲突只能重新读取同一 `request_id` 的记录并按状态幂等处理；禁止用“创建失败后盲目 refund delta”补偿，因为会错误退回已有请求。
6. 成功、settle、refund、reset 以及修改 `token_limit_enabled` / `token_limit` 的 `Token.Update()` 都必须调用同一个缓存失效 helper。

缓存失效 helper 要按 token id 查询 key，然后删除或刷新 Redis token cache：

```go
func invalidateTokenCacheById(tokenId int) error
```

`ConsumeTokenLimitIncrement` 用于 Realtime / WSS 增量扣减：`tokens <= 0` 或 token limit 未启用时 no-op 成功；新 `idempotencyKey` 必须在一个事务内创建状态为 `settled` 的记录并执行同一条件更新；重复 `idempotencyKey` 必须幂等返回，不得重复增加 `token_used`；cap 不足时事务回滚，不留下记录。
`RefundTokenLimitPreConsume` 和 `SettleTokenLimitPreConsume` 必须幂等：同一个 `request_id` 多次 refund / settle 不得重复调整 `token_used`。`ResetTokenUsage` 必须在事务中读取重置前 `token_used`、把 `token_used` 置 0，并把该 token 当前 `consumed` 状态的在途记录标记为 `refunded` 且 `failure_code = "usage_reset"`，避免旧请求后续 settle / refund 恢复或扣减 reset 后的新值。

`refundTokenLimitDelta` 使用 GORM 表达式：


```go
"token_used": gorm.Expr("CASE WHEN token_used >= ? THEN token_used - ? ELSE 0 END", tokens, tokens)
```

该 `CASE WHEN` 是标准 SQL，可用于 SQLite / MySQL / PostgreSQL。

- [x] **步骤 8：运行模型测试**

运行：

```bash
go test ./model -run 'TestTokenLimitPreConsumeIgnoresLegacyQuotaAndIsIdempotent|TestTokenLimitSettleRefundResetAndCacheInvalidation|TestTokenLimitWritesInvalidateTokenCache|TestValidateUserTokenAllows'
```

预期：PASS。

---

## 任务 2：后端 token API 契约

**文件：**
- 修改：`controller/token.go`
- 修改：`router/api-router.go`
- 修改：`controller/token_test.go`
- 修改：`controller/config_guide.go`
- 修改：`controller/config_guide_test.go`

- [x] **步骤 1：编写失败测试：创建、详情和列表返回新 token cap 字段**

在 `controller/token_test.go` 新增测试，`AddToken` 必须同时验证数据库字段和响应 DTO：

```go
func TestAddTokenAcceptsTokenLimitFields(t *testing.T) {
    setupTokenControllerTestDB(t)
    ctx, recorder := newAuthenticatedContext(t, http.MethodPost, "/api/token/", map[string]any{
        "name":                "limited",
        "expired_time":        -1,
        "token_limit_enabled": true,
        "token_limit":         1000,
    }, 94001)

    AddToken(ctx)

    require.Equal(t, http.StatusOK, recorder.Code)
    var body struct {
        Success bool            `json:"success"`
        Data    json.RawMessage `json:"data"`
    }
    require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &body))
    require.True(t, body.Success)
    var data map[string]any
    require.NoError(t, common.Unmarshal(body.Data, &data))
    require.Equal(t, true, data["token_limit_enabled"])
    require.Equal(t, float64(1000), data["token_limit"])
    require.Equal(t, float64(0), data["token_used"])
    require.Equal(t, float64(1000), data["token_remaining"])
    require.Equal(t, false, data["token_unlimited"])
    require.Contains(t, data, "remain_quota")
    require.Contains(t, data, "used_quota")
    require.Contains(t, data, "unlimited_quota")

    var token model.Token
    require.NoError(t, model.DB.Where("user_id = ? AND name = ?", 94001, "limited").First(&token).Error)
    require.True(t, token.TokenLimitEnabled)
    require.Equal(t, int64(1000), token.TokenLimit)
    require.Equal(t, int64(0), token.TokenUsed)
}
```

再写 `GetAllTokens` 或 `GetToken` 响应测试，断言 JSON 中包含：

```json
"token_limit_enabled": true,
"token_limit": 1000,
"token_used": 0,
"token_remaining": 1000,
"token_unlimited": false
```

同时新增状态切换和参数错误测试：

```go
func TestUpdateTokenLimitValidationAndStateSwitch(t *testing.T) {
    setupTokenControllerTestDB(t)
    token := seedToken(t, model.DB, 94501, "switch", "sk-switch")
    require.NoError(t, model.DB.Model(token).Updates(map[string]any{"token_used": int64(123), "used_quota": 456}).Error)

    badCtx, badRecorder := newAuthenticatedContext(t, http.MethodPut, "/api/token/", map[string]any{
        "id": token.Id, "name": "switch", "expired_time": -1, "token_limit_enabled": true, "token_limit": 0,
    }, 94501)
    UpdateToken(badCtx)
    require.NotEqual(t, http.StatusOK, badRecorder.Code)

    offCtx, offRecorder := newAuthenticatedContext(t, http.MethodPut, "/api/token/", map[string]any{
        "id": token.Id, "name": "switch", "expired_time": -1, "token_limit_enabled": false, "token_limit": 999999,
    }, 94501)
    UpdateToken(offCtx)
    require.Equal(t, http.StatusOK, offRecorder.Code)

    var got model.Token
    require.NoError(t, model.DB.First(&got, token.Id).Error)
    require.False(t, got.TokenLimitEnabled)
    require.Equal(t, int64(0), got.TokenLimit)
    require.Equal(t, int64(123), got.TokenUsed, "disabling limit must not reset usage")
    require.Equal(t, 456, got.UsedQuota, "new token cap must not read legacy used_quota")
}
```

同一红灯批次还必须先写 reset、status / usage 和 config guide 测试：使用步骤 4、5、6 中列出的测试正文和断言，全部写入后再执行步骤 2 的失败命令。不得先运行命令再补测试，也不得把这些测试留到实现之后补写。

- [x] **步骤 2：运行控制器测试验证失败**

运行：

```bash
go test ./controller -run 'TestAddTokenAcceptsTokenLimitFields|TestGetTokenReturnsTokenLimitFields|TestUpdateTokenLimitValidationAndStateSwitch|TestResetTokenUsageClearsNewTokenUsedOnlyAndRecordsAudit|TestResetTokenUsageRejectsForeignToken|TestGetToken(Status|Usage).*TokenLimit|ConfigGuide'
```

预期：失败，原因是字段未绑定、响应缺失、reset handler 未定义、status / usage 新字段缺失或 config guide 仍使用 legacy exhausted / token_quota。

- [x] **步骤 3：实现 token 响应 DTO 和创建 / 更新校验**

在 `controller/token.go` 新增 DTO：

```go
type tokenResponse struct {
    *model.Token
    TokenLimitEnabled bool  `json:"token_limit_enabled"`
    TokenLimit        int64 `json:"token_limit"`
    TokenUsed         int64 `json:"token_used"`
    TokenRemaining    int64 `json:"token_remaining"`
    TokenUnlimited    bool  `json:"token_unlimited"`
}
```

`buildMaskedTokenResponse` 返回 `tokenResponse`，并调用 `token.Clean()` 或复制后清理 key。

创建 / 更新校验：

```go
const maxAPIKeyTokenLimit int64 = 10_000_000_000_000

func normalizeTokenLimitFields(token *model.Token) error {
    if !token.TokenLimitEnabled {
        token.TokenLimit = 0
        return nil
    }
    if token.TokenLimit <= 0 {
        return errors.New("token limit must be greater than 0")
    }
    if token.TokenLimit > maxAPIKeyTokenLimit {
        return fmt.Errorf("token limit exceeds max: %d", maxAPIKeyTokenLimit)
    }
    return nil
}
```

`AddToken` 的 `cleanToken` 设置新字段：

```go
TokenLimitEnabled: token.TokenLimitEnabled,
TokenLimit:        token.TokenLimit,
TokenUsed:         0,
```

不要再用 `common.QuotaPerUnit` 校验 API Key 新 token limit。旧 `RemainQuota` / `UnlimitedQuota` 可以继续接受，但 default 前端不提交。

`UpdateToken` 非 `status_only` 时更新新字段，不隐式重置 `TokenUsed`。

更新 `controller/token_test.go` 的 `migrateTokenControllerTestDB`，至少迁移 `model.Token`、`model.Log` 和 `model.TokenLimitPreConsumeRecord`；reset 审计测试开始前要能向 `model.LOG_DB` 写入 / 查询 `model.Log`，不能因为审计表缺失把测试失败误判为 handler 行为。

`AddToken` 插入成功后不能只返回空 success；必须返回 `data: buildMaskedTokenResponse(&cleanToken)` 或等价 DTO，响应同时包含新 token cap 字段和 legacy 兼容字段。

- [x] **步骤 4：实现 reset-token-usage handler**

测试：

```go
func TestResetTokenUsageClearsNewTokenUsedOnlyAndRecordsAudit(t *testing.T) {
    setupTokenControllerTestDB(t)
    token := seedToken(t, model.DB, 95001, "reset", "sk-reset")
    require.NoError(t, model.DB.Model(token).Updates(map[string]any{
        "token_limit_enabled": true,
        "token_limit":         int64(1000),
        "token_used":          int64(900),
        "remain_quota":        123,
        "used_quota":          456,
    }).Error)

    ctx, recorder := newAuthenticatedContext(t, http.MethodPost, fmt.Sprintf("/api/token/%d/reset-token-usage", token.Id), nil, 95001)
    ctx.Params = gin.Params{{Key: "id", Value: strconv.Itoa(token.Id)}}

    ResetTokenUsage(ctx)

    require.Equal(t, http.StatusOK, recorder.Code)
    var got model.Token
    require.NoError(t, model.DB.First(&got, token.Id).Error)
    require.Equal(t, int64(0), got.TokenUsed)
    require.Equal(t, 123, got.RemainQuota)
    require.Equal(t, 456, got.UsedQuota)

    var audit model.Log
    require.NoError(t, model.LOG_DB.Where("user_id = ?", 95001).Order("id desc").First(&audit).Error)
    require.Contains(t, audit.Content, "reset token usage")
    require.Contains(t, audit.Other, "\"token_id\":")
    require.Contains(t, audit.Other, "\"before_token_used\":900")
    require.Contains(t, audit.Other, "\"after_token_used\":0")
}
```

同一文件必须新增跨用户拒绝测试，防止 handler 只按 token id 重置他人 API Key：

```go
func TestResetTokenUsageRejectsForeignToken(t *testing.T) {
    setupTokenControllerTestDB(t)
    token := seedToken(t, model.DB, 95011, "foreign", "sk-foreign-reset")
    require.NoError(t, model.DB.Model(token).Updates(map[string]any{
        "user_id":             95012,
        "token_limit_enabled": true,
        "token_limit":         int64(1000),
        "token_used":          int64(700),
    }).Error)

    ctx, recorder := newAuthenticatedContext(t, http.MethodPost, fmt.Sprintf("/api/token/%d/reset-token-usage", token.Id), nil, 95001)
    ctx.Params = gin.Params{{Key: "id", Value: strconv.Itoa(token.Id)}}

    ResetTokenUsage(ctx)

    require.NotEqual(t, http.StatusOK, recorder.Code)
    var got model.Token
    require.NoError(t, model.DB.First(&got, token.Id).Error)
    require.Equal(t, int64(700), got.TokenUsed)

    var count int64
    require.NoError(t, model.LOG_DB.Model(&model.Log{}).Where("user_id = ? AND content LIKE ?", 95001, "%reset token usage%").Count(&count).Error)
    require.Equal(t, int64(0), count)
}
```

实现 `ResetTokenUsage(c)`，只允许当前用户自己的 token。上述 reset 测试必须已经在步骤 2 前写入并失败。handler 必须调用 `model.ResetTokenUsage(tokenId, userId)`，只把 `token_used=0`，不修改旧字段；必须写审计日志，至少包含 `token_id`、操作者用户 ID、重置前 `token_used`、重置后 `0`、当前时间。可使用 `model.RecordLogWithAdminInfo` 或新增明确 helper，但不得降级为注释。

在途请求语义由任务 1 的 `model.ResetTokenUsage` 负责：重置时将该 token 当前 `consumed` 状态的 `token_limit_pre_consume_records` 标记为 `refunded` / `usage_reset`，后续旧 request 的 settle / refund 必须 no-op，不得恢复或扣减 reset 后的新值。

在 `router/api-router.go` 添加：

```go
tokenRoute.POST("/:id/reset-token-usage", controller.ResetTokenUsage)
```

放在 `GET /:id` 附近。Gin 静态段和参数段通常按注册顺序匹配；此路由包含两段，放在 `/:id` 附近即可。

- [x] **步骤 5：实现 status / usage 契约**

步骤 2 前必须给 `GetTokenUsage` 和 `GetTokenStatus` 写测试，断言新字段存在：

```go
"token_limit_enabled": true
"token_limit": 1000
"token_used": 250
"token_remaining": 750
"token_unlimited": false
```

旧 `total_granted` / `total_used` / `total_available` 保留。

- [x] **步骤 6：实现配置向导 exhausted 行为**

步骤 2 前必须更新 `controller/config_guide_test.go` 中 exhausted case：历史 exhausted key 不再返回 429，只要用户 enabled、未过期、IP 通过，应按主鉴权语义通过。对应 `controller/token_test.go` 中配置向导相关测试也更新。

- [x] **步骤 7：运行控制器定向测试**

运行：

```bash
go test ./controller -run 'TestAddTokenAcceptsTokenLimitFields|TestGetTokenReturnsTokenLimitFields|TestUpdateTokenLimitValidationAndStateSwitch|TestResetTokenUsageClearsNewTokenUsedOnlyAndRecordsAudit|TestResetTokenUsageRejectsForeignToken|TestGetToken(Status|Usage).*TokenLimit|ConfigGuide'
```

预期：PASS。

任务 2 的 TDD 顺序固定为：步骤 1、4、5、6 的测试全部写入后，执行步骤 2 的失败命令；再执行步骤 3 和步骤 4/5/6 对应实现；最后执行步骤 7 的通过命令。



---

## 任务 3：后端运行时 token cap 生命周期

**文件：**
- 新增：`service/token_limit_session.go`
- 修改：`service/billing.go`
- 修改：`service/billing_session.go`
- 修改：`service/text_quota.go`
- 修改：`service/quota.go`
- 修改：`service/log_info_generate.go`
- 修改：`types/error.go`
- 修改：`controller/relay.go`
- 修改：`relay/common/billing.go`
- 修改：`relay/common/relay_info.go`
- 修改：`relay/channel/openai/relay-openai.go`
- 修改：`relay/websocket.go`
- 修改：`service/subscription_only_billing_test.go`
- 修改：`service/subscription_billing_test.go`
- 修改：`service/quota_test.go`
- 修改：`relay/channel/openai/relay_realtime_test.go`（如果现有测试文件名不同，放在同包现有 realtime 测试文件）

本任务依赖任务 1 完成：`model.Token` 新字段、`TokenLimitPreConsumeRecord`、`model.PreConsumeTokenLimit`、`model.SettleTokenLimitPreConsume`、`model.RefundTokenLimitPreConsume`、`model.MarkTokenLimitSettleFailed`、`model.ConsumeTokenLimitIncrement` 必须已存在。`setupSubscriptionOnlyBillingTestDB(t)` 或 service 测试 `TestMain` 必须统一迁移 `model.TokenLimitPreConsumeRecord`，避免每个测试重复漏迁移。

- [x] **步骤 1：编写失败测试：session 基础行为**

在 `service/subscription_only_billing_test.go` 或新增 `service/token_limit_session_test.go` 写入：

```go
func TestTokenLimitSessionNoopWhenDisabled(t *testing.T) {
    setupSubscriptionOnlyBillingTestDB(t)
    require.NoError(t, model.DB.Create(&model.Token{Id: 96001, UserId: 96002, Key: "sk-no-cap", Status: common.TokenStatusEnabled, TokenLimitEnabled: false}).Error)
    relayInfo := subscriptionOnlyRelayInfo(96002, 96001, "sk-no-cap", "subscription_only")

    session := NewTokenLimitSession(relayInfo)
    apiErr := session.PreConsume(100)

    require.Nil(t, apiErr)
    require.Equal(t, int64(0), getTokenUsed(t, 96001))
}

func TestTokenLimitSessionRejectsWhenLimitExhausted(t *testing.T) {
    setupSubscriptionOnlyBillingTestDB(t)
    require.NoError(t, model.DB.Create(&model.Token{Id: 96011, UserId: 96012, Key: "sk-cap", Status: common.TokenStatusEnabled, TokenLimitEnabled: true, TokenLimit: 100, TokenUsed: 95}).Error)
    relayInfo := subscriptionOnlyRelayInfo(96012, 96011, "sk-cap", "subscription_only")

    session := NewTokenLimitSession(relayInfo)
    apiErr := session.PreConsume(10)

    require.NotNil(t, apiErr)
    require.Equal(t, types.ErrorCodeAPIKeyTokenLimitExhausted, apiErr.GetErrorCode())
    require.Equal(t, int64(95), getTokenUsed(t, 96011))
}
```

运行：

```bash
go test ./service -run 'TestTokenLimitSessionNoopWhenDisabled|TestTokenLimitSessionRejectsWhenLimitExhausted'
```

预期：失败，原因是 `NewTokenLimitSession` 或 `types.ErrorCodeAPIKeyTokenLimitExhausted` 未定义。

- [x] **步骤 2：实现 `TokenLimitSession` 和 relay/common 接口边界**

在 `relay/common/billing.go` 增加接口，避免 `relay/common` import `service` 造成循环依赖：

```go
type TokenLimitSettler interface {
    PreConsume(tokens int64) *types.NewAPIError
    Settle(actualTokens int64) error
    MarkSettleFailed(actualTokens int64, reason string) error
    Refund(reason string)
    ConsumeIncrement(tokens int64) (int64, *types.NewAPIError)
    RefundIncrement(sequence int64, reason string)
    PreConsumedTokens() int64
}
```

在 `relay/common/relay_info.go` 增加字段和 helper：

```go
TokenLimit TokenLimitSettler

func (info *RelayInfo) SubscriptionPreConsumedTokens() int64 { return info.SubscriptionPreConsumed }
```

`service/token_limit_session.go` 固定导出：

```go
type TokenLimitSession struct {
    relayInfo    *relaycommon.RelayInfo
    requestId    string
    tokenId      int
    userId       int
    enabled      bool
    preConsumed  int64
    incrementSeq int64
    settled      bool
    refunded     bool
    mu           sync.Mutex
}

func NewTokenLimitSession(relayInfo *relaycommon.RelayInfo) *TokenLimitSession
func (s *TokenLimitSession) PreConsume(tokens int64) *types.NewAPIError
func (s *TokenLimitSession) Settle(actualTokens int64) error
func (s *TokenLimitSession) MarkSettleFailed(actualTokens int64, reason string) error
func (s *TokenLimitSession) Refund(reason string)
func (s *TokenLimitSession) ConsumeIncrement(tokens int64) (int64, *types.NewAPIError)
func (s *TokenLimitSession) RefundIncrement(sequence int64, reason string)
func (s *TokenLimitSession) PreConsumedTokens() int64
```

`TokenLimitSession` 只能调用任务 1 的 `model.*TokenLimit*` 导出 API。`ConsumeIncrement` 在锁内递增 `incrementSeq`，生成幂等键 `requestId + ":realtime:" + strconv.FormatInt(sequence, 10)`，调用 `model.ConsumeTokenLimitIncrement(tokenId, userId, idempotencyKey, tokens)` 原子创建 settled 增量记录并增加 `token_used`。`RefundIncrement` 使用同一幂等键调用 `model.RefundTokenLimitPreConsume(idempotencyKey, reason)`。service 包不得重复写 `tokens` 或 `token_limit_pre_consume_records`。

- [x] **步骤 3：编写失败测试：key cap 拒绝回滚订阅预扣和并发租约**

新增 `TestTokenLimitRejectRefundsSubscriptionPreConsume` 和 `TestTokenLimitRejectReleasesSubscriptionConcurrencyLease`。核心断言：订阅预扣先成功，key cap 预扣拒绝后调用 `RefundBillingAfterTokenLimitReject(relayInfo.Billing)`，`user_subscriptions.token_used` 回到请求前，`SubscriptionPreConsumeRecord.Status = "refunded"`，API Key `TokenUsed` 保持拒绝前值，并发租约释放后可再次 `AcquireSubscriptionConcurrency(context.Background(), relayInfo)` 成功。

运行：

```bash
go test ./service -run 'TestTokenLimitRejectRefundsSubscriptionPreConsume|TestTokenLimitRejectReleasesSubscriptionConcurrencyLease'
```

预期：失败，原因是回滚辅助和 key cap 接入未定义。

- [x] **步骤 4：实现 relay 预扣接入、响应状态 helper 和回滚辅助**

在 `service/billing_session.go` 增加同步退款包装：

```go
func RefundBillingAfterTokenLimitReject(billing relaycommon.BillingSettler)
```

在 `service/token_limit_session.go` 增加退出 helper：

```go
func RefundTokenLimitOnRelayFailure(relayInfo *relaycommon.RelayInfo, reason string)
func MarkTokenLimitAfterResponseFailure(relayInfo *relaycommon.RelayInfo, reason string)
```

在 `service/billing.go` 增加两个 helper。清理 / panic / client-gone 只能使用真实写出状态；settle 阶段才把 streaming 归入审计分支：

```go
func ResponseAlreadyWritten(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, explicit bool) bool {
    relayStarted := relayInfo != nil && relayInfo.HasSendResponse()
    ginStarted := ctx != nil && ctx.Writer != nil && ctx.Writer.Written()
    return explicit || relayStarted || ginStarted
}

func ShouldAuditTokenLimitSettle(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, explicit bool) bool {
    return ResponseAlreadyWritten(ctx, relayInfo, explicit) || (relayInfo != nil && relayInfo.IsStream)
}
```

在 `types/error.go` 增加固定错误码，供 service、relay 和 WebSocket 出口统一使用：

```go
ErrorCodeAPIKeyTokenLimitExhausted ErrorCode = "api_key_token_limit_exhausted"
```

OpenAI-compatible 错误响应固定为 HTTP 429，不能映射成 `subscription_token_exhausted`；错误 type 使用项目中现有 quota / rate-limit 类错误约定。

在 `controller/relay.go` 的顺序固定为：订阅预扣成功 → 订阅并发租约成功 → `relayInfo.TokenLimit = service.NewTokenLimitSession(relayInfo)` → `relayInfo.TokenLimit.PreConsume(relayInfo.SubscriptionPreConsumedTokens())`。key cap 预扣拒绝时必须释放并发租约、调用 `RefundBillingAfterTokenLimitReject(relayInfo.Billing)`，返回 `api_key_token_limit_exhausted`。

- [x] **步骤 5：编写失败测试：后续失败、panic 和客户端中断清理 key cap**

测试正文必须使用真实 helper 和数据库断言：

- `TestRelayFailureAfterTokenLimitPreConsumeRefundsKeyCap`：seed 用户、订阅和启用 token cap 的 API Key；完成订阅预扣和 key cap 预扣后调用未写出响应清理路径；断言 `TokenUsed` 回到预扣前，`TokenLimitPreConsumeRecord.Status = refunded`，`FailureCode = "channel_error"`，订阅预扣已 refunded，调用方原错误码不被覆盖。
- `TestRelayErrorAfterResponseAuditsWithoutRefundOrSecondWrite`：先写出响应或调用 `relayInfo.SetFirstResponseTime()`，再设置 `newAPIError`；执行统一退出清理；断言不调用 `relayInfo.Billing.Refund(c)`、不调用 `RefundTokenLimitOnRelayFailure`、不写普通 JSON / WSS 错误响应，订阅预扣保持已提交或可由现有失败提交语义处理，key cap record 标记 `settle_failed`，`FailureCode = "error_after_response"`。
- `TestRelayPanicAfterTokenLimitPreConsumeRefundsKeyCap`：通过测试 helper 执行与 `controller/relay.go` 同等的统一 defer；在 key cap 预扣后、响应未写出前触发 panic；外层 recover 后断言 key cap record 已 `refunded`，未发送响应的订阅预扣 record 已 `refunded`，然后确认 helper 没有吞掉 panic。
- `TestRelayPanicAfterResponseAuditsWithoutRefund`：先写出响应或调用 `relayInfo.SetFirstResponseTime()`，再触发 panic；外层 recover 后断言不回滚已发送响应对应订阅预扣，不 refund key cap，key cap record 标记 `settle_failed`，`FailureCode = "panic_after_response"`，并确认 panic 仍被重新抛出。
- `TestRelayClientGoneBeforeResponseRefundsKeyCap`：取消 request context，保持 recorder 未写出；执行统一退出清理；断言 key cap 和订阅预扣都 `refunded`。
- `TestRelayStreamingClientGoneBeforeFirstChunkRefundsKeyCap`：设置 `relayInfo.IsStream = true`，但不调用 `SetFirstResponseTime()` 且 recorder 未写出；取消 request context 后执行统一退出清理；断言仍按 before-response 路径 refund key cap 和订阅预扣。
- `TestRelayClientGoneAfterResponseAuditsKeyCap`：取消 request context 前先 `recorder.WriteHeader(http.StatusOK)` 或调用 `relayInfo.SetFirstResponseTime()`；执行统一退出清理；断言不尝试写普通 429、不回滚已发送响应对应订阅扣费，key cap record 标记 `settle_failed`，`FailureCode = "client_gone_after_response"`。

运行：

```bash
go test ./service ./controller -run 'TestRelayFailureAfterTokenLimitPreConsumeRefundsKeyCap|TestRelayErrorAfterResponseAuditsWithoutRefundOrSecondWrite|TestRelayPanicAfterTokenLimitPreConsumeRefundsKeyCap|TestRelayPanicAfterResponseAuditsWithoutRefund|TestRelayClientGoneBeforeResponseRefundsKeyCap|TestRelayStreamingClientGoneBeforeFirstChunkRefundsKeyCap|TestRelayClientGoneAfterResponseAuditsKeyCap'
```

预期：失败，原因是统一退出 helper 未实现。

- [x] **步骤 6：实现统一退出清理**

替换 `controller/relay.go` 现有失败 defer，统一覆盖普通错误、panic 和客户端提前中断：

```go
defer func() {
    recovered := recover()
    if recovered != nil {
        if service.ResponseAlreadyWritten(c, relayInfo, false) {
            service.MarkTokenLimitAfterResponseFailure(relayInfo, "panic_after_response")
        } else {
            service.RefundTokenLimitOnRelayFailure(relayInfo, "panic")
            if relayInfo.Billing != nil {
                relayInfo.Billing.Refund(c)
            }
        }
        panic(recovered)
    }
    if newAPIError != nil {
        newAPIError = service.NormalizeViolationFeeError(newAPIError)
        if service.ResponseAlreadyWritten(c, relayInfo, false) {
            service.MarkTokenLimitAfterResponseFailure(relayInfo, "error_after_response")
            newAPIError = nil
            return
        }
        if relayInfo.Billing != nil {
            relayInfo.Billing.Refund(c)
        }
        service.RefundTokenLimitOnRelayFailure(relayInfo, string(newAPIError.GetErrorCode()))
        service.ChargeViolationFeeIfNeeded(c, relayInfo, newAPIError)
        return
    }
    if c.Request.Context().Err() != nil && relayInfo.TokenLimit != nil {
        if service.ResponseAlreadyWritten(c, relayInfo, false) {
            service.MarkTokenLimitAfterResponseFailure(relayInfo, "client_gone_after_response")
        } else {
            service.RefundTokenLimitOnRelayFailure(relayInfo, "client_gone_before_response")
            if relayInfo.Billing != nil {
                relayInfo.Billing.Refund(c)
            }
        }
    }
}()
```

该 defer 不得吞掉 panic；退款后必须重新 panic，让 Gin recovery 保持现有行为。流式请求在首包前 `IsStream=true` 但 `HasSendResponse=false`，仍必须走 before-response refund。任何 `ResponseAlreadyWritten=true` 的错误、panic 或 client-gone 出口都不得调用 `relayInfo.Billing.Refund(c)` 或 `RefundTokenLimitOnRelayFailure`；只能审计 / 标记 key cap 失败。对于已写出响应后的 `newAPIError`，统一清理 defer 必须设置 `newAPIError = nil`，或者同步修改 `controller/relay.go` 最外层错误写出 defer 为 `newAPIError != nil && !service.ResponseAlreadyWritten(c, relayInfo, false)` 才写 JSON / WSS / Claude 错误，避免二次写普通错误响应；测试 `TestRelayErrorAfterResponseAuditsWithoutRefundOrSecondWrite` 必须覆盖现有最外层 defer。

- [x] **步骤 7：编写失败测试：文本 settle 使用订阅 metered token 且错误码不被改写**

在 `service/subscription_billing_test.go` 添加区分口径的测试。用 Anthropic 语义构造 `SubscriptionMeteredTokens(usage)` 与 `usage.TotalTokens`、`PromptTokens + CompletionTokens` 不相等：

```go
usage := &dto.Usage{
    PromptTokens:                 100,
    CompletionTokens:             20,
    TotalTokens:                  50,
    UsageSemantic:                "anthropic",
    PromptTokensDetails:          dto.PromptTokensDetails{CachedTokens: 10},
    ClaudeCacheCreation5mTokens:  30,
    ClaudeCacheCreation1hTokens:  40,
}
expectedTokens := SubscriptionMeteredTokens(usage)
require.Equal(t, int64(200), expectedTokens)
require.NotEqual(t, int64(usage.TotalTokens), expectedTokens)
require.NotEqual(t, int64(usage.PromptTokens+usage.CompletionTokens), expectedTokens)
```

完整测试 `TestPostTextConsumeQuotaSettlesApiKeyLimitWithSubscriptionMeteredTokens` 必须先预扣 key cap，再调用 `PostTextConsumeQuota(ctx, relayInfo, usage, nil)`，断言订阅 `token_used` 和 API Key `token_used` 都等于 `subscriptionTokensForTextSettle(relayInfo, expectedTokens, summary.Quota)` 的结果，且 `RemainQuota` / `UsedQuota` 不变。

新增 `TestPostSettleErrorToOpenAIErrorPreservesAPIKeyTokenLimitError`：构造 API Key token cap strict settle 错误，调用 `PostSettleErrorToOpenAIError(relayInfo, err)`，断言返回 `types.ErrorCodeAPIKeyTokenLimitExhausted`、HTTP 429，不调用 `relayInfo.Billing.CommitPreConsumedOnFailure()`，不改写成 `types.ErrorCodeSubscriptionTokenExhausted`。真正的订阅 settle failure 仍保持订阅错误映射。

运行：

```bash
go test ./service -run 'TestPostTextConsumeQuotaSettlesApiKeyLimitWithSubscriptionMeteredTokens|TestPostSettleErrorToOpenAIErrorPreservesAPIKeyTokenLimitError'
```

预期：失败，原因是 key cap settle 尚未接入或 settle 错误 mapper 仍改写错误码。

- [x] **步骤 8：实现 settle 集成、strict 分支、审计分支和错误 mapper**

在 `service/billing.go` 的 `BillingSettleInput` 增加：

```go
ApiKeyTokens    int64
ResponseStarted bool
```

`SettleBillingWithInput(ctx, relayInfo, input)` 必须先计算：

```go
apiKeyTokens := input.ApiKeyTokens
if apiKeyTokens == 0 {
    apiKeyTokens = input.SubscriptionTokens
}
auditSettle := ShouldAuditTokenLimitSettle(ctx, relayInfo, input.ResponseStarted)
```

`auditSettle == false` 时使用 strict settle：先 `relayInfo.TokenLimit.Settle(apiKeyTokens)`；key cap settle 拒绝时必须退回该 request 的 key cap 预扣，再调用 `RefundBillingAfterTokenLimitReject(relayInfo.Billing)` 并返回 `api_key_token_limit_exhausted`。实现方式二选一且必须由测试锁定：`TokenLimitSession.Settle` 在返回拒绝错误前原子 refund 该 request 的 key cap 预扣，或 `SettleBillingWithInput` 在该分支显式调用 `relayInfo.TokenLimit.Refund("api_key_token_limit_exhausted")` 后再回滚订阅。订阅与 key cap 都必须回到请求前；订阅 settle 失败时调用 `relayInfo.TokenLimit.Refund(errorCode)` 退回 key cap。

`auditSettle == true` 时使用审计 settle：订阅仍按实际 `SubscriptionTokens` 结算；key cap 不得超过 `token_limit`，追加会超限时调用 `relayInfo.TokenLimit.MarkSettleFailed(apiKeyTokens, reason)`，不得返回普通 429，不得回滚已发送响应对应订阅扣费。当前非流式 `DoResponse` 多数路径已经在 `PostTextConsumeQuota` 前写出响应，必须依赖 `ctx.Writer.Written()` / `ResponseAlreadyWritten` 进入该审计分支；streaming 即使尚未写出也在 settle 阶段进入审计分支，但 cleanup 阶段仍必须用 `ResponseAlreadyWritten` 判断是否可退款。

在 `service/text_quota.go` 调用 `SettleBillingWithInput` 时设置：

```go
ApiKeyTokens:    subscriptionTokens,
ResponseStarted: ResponseAlreadyWritten(ctx, relayInfo, false),
```

`PostSettleErrorToOpenAIError` 必须识别 API Key token cap 错误：返回 `types.ErrorCodeAPIKeyTokenLimitExhausted` / HTTP 429，不调用 `CommitPreConsumedOnFailure()`；只有真实订阅 settle failure 才提交预扣并映射为订阅错误。

`service/log_info_generate.go` 必须把审计失败写入 log `other`：`api_key_token_limit_settle_failed=true`、`api_key_token_limit_actual_tokens`、`api_key_token_limit_pre_consumed`、`api_key_token_limit_failure_code`。

新增测试：

- `TestSettleBillingRefundsSubscriptionWhenApiKeySettleRejects`：`ResponseStarted=false`，`relayInfo.IsStream=false`，actual key tokens 超过 key cap；断言返回 `api_key_token_limit_exhausted`，订阅预扣 record `refunded`，订阅 `token_used` 和 key `TokenUsed` 都回到请求前。
- `TestSettleBillingRefundsApiKeyWhenSubscriptionSettleFails`：key settle 成功后让订阅 settle 失败；断言 key cap record `refunded`，key `TokenUsed` 回到请求前，并保留订阅错误。
- `TestSettleBillingUsesAuditWhenGinWriterAlreadyWritten`：使用 gin recorder 先 `WriteHeader(http.StatusOK)`，再让 actual key tokens 超 cap；断言函数不返回普通 429，订阅按实际 token 结算，key `TokenUsed` 不超过 `TokenLimit`，record 为 `settle_failed`。
- `TestStreamingSettleUsesAuditButStreamingCleanupBeforeFirstChunkRefunds`：`relayInfo.IsStream=true`，actual key tokens 超 cap；settle 阶段断言 record 为 `settle_failed`；另用未首包 client-gone 测试证明 cleanup 不把 `IsStream` 当已写出。
- `TestStreamingTokenLimitSettleFailureIsAuditedWithoutOverLimit`：`relayInfo.IsStream=true`，actual key tokens 超 cap；断言 record 为 `settle_failed`，并读取最后一条 consume log / audit log 的 `Other`，确认包含审计字段。

- [x] **步骤 9：编写失败测试：Realtime 增量和 WebSocket 错误出口**

新增 `TestPreWssConsumeQuotaSettlesApiKeyLimitIncrementally`：第一次增量 7 同时增加订阅和 key cap，第二次增量 4 因 key cap 拒绝，返回 `types.ErrorCodeAPIKeyTokenLimitExhausted`，订阅和 key cap 都保持第一次成功值。

新增 `TestPreWssConsumeQuotaRefundsApiKeyIncrementWhenSubscriptionIncrementFails`：构造订阅剩余不足但 key cap 足够；`PreWssConsumeQuota` 返回订阅错误后，API Key `token_used` 回到增量前。

在 `relay/channel/openai` 同包新增 WebSocket 出口测试：让 `preConsumeUsage()` 或同等路径返回 `*types.NewAPIError` 且 `GetErrorCode() == types.ErrorCodeAPIKeyTokenLimitExhausted`，断言 `realtimeErrorFromErrChan()` 和 OpenAI realtime 收尾 `preConsumeUsage` 包装保留 `api_key_token_limit_exhausted`，不改写为 `do_request_failed` 或 `subscription_token_exhausted`。如需通过 `WssHelper` 测试，则断言下游 WebSocket 错误 payload / code 为 `api_key_token_limit_exhausted`。

在 `relay` 包新增 WSS final settle 出口测试：让 `service.PostWssConsumeQuota` 或等价可注入路径返回 `*types.NewAPIError`，且 `GetErrorCode() == types.ErrorCodeAPIKeyTokenLimitExhausted`；调用 `WssHelper` 或覆盖 `relay/websocket.go` 中 `PostWssConsumeQuota` 错误包装分支，断言 WebSocket 返回的错误 payload / code 仍为 `api_key_token_limit_exhausted`，HTTP / OpenAI-compatible status 为 429，不被统一包装成 `subscription_token_exhausted` / 403。测试名固定为 `TestWssHelperPreservesAPIKeyTokenLimitExhaustedFromPostSettle`。

运行：

```bash
go test ./service ./relay ./relay/channel/openai -run 'TestPreWssConsumeQuotaSettlesApiKeyLimitIncrementally|TestPreWssConsumeQuotaRefundsApiKeyIncrementWhenSubscriptionIncrementFails|TestWssHelperPreservesAPIKeyTokenLimitExhaustedFromPostSettle|TestRealtime.*APIKeyTokenLimitExhausted|TestOpenaiRealtime.*APIKeyTokenLimitExhausted'
```

预期：失败，原因是 Realtime key cap 增量或 WebSocket 错误保留尚未实现。

- [x] **步骤 10：实现 Realtime 增量和错误保留**

`PreWssConsumeQuota()` 顺序固定为：计算本次增量 token → `relayInfo.TokenLimit.ConsumeIncrement(tokens)` 成功 → `BillingSession.SettleSubscriptionIncrement(tokens)` 成功。订阅增量失败时调用 `relayInfo.TokenLimit.RefundIncrement(sequence, "subscription_increment_failed")`，保证 key cap 不留下增量。key cap 增量拒绝时直接返回 `api_key_token_limit_exhausted`，不得先扣订阅。

修改 `relay/channel/openai/relay-openai.go` 的 `realtimeErrorFromErrChan` 和收尾 `preConsumeUsage` 包装：如果 `errors.As(err, &types.NewAPIError)`，且错误码是 `types.ErrorCodeAPIKeyTokenLimitExhausted`，必须原样返回对应 429 错误；订阅错误仍返回订阅耗尽；其他错误仍按现有 `do_request_failed`。同步修改 `relay/websocket.go` 的 `PostWssConsumeQuota` 错误包装：如果返回值已经是 `*types.NewAPIError` 且错误码是 `types.ErrorCodeAPIKeyTokenLimitExhausted`，直接返回该错误，不得统一 `types.NewOpenAIError(err, types.ErrorCodeSubscriptionTokenExhausted, 403, ...)`。

- [x] **步骤 11：运行运行时定向测试**

运行：

```bash
go test ./service ./controller ./relay ./relay/channel/openai -run 'TokenLimit|SubscriptionBillingDoesNotConsumeTokenKeyQuota|PostTextConsumeQuota|PreWssConsumeQuota|RelayFailureAfterTokenLimit|RelayPanicAfterTokenLimit|RelayClientGone|WssHelperPreservesAPIKeyTokenLimitExhaustedFromPostSettle|Realtime.*APIKeyTokenLimitExhausted|OpenaiRealtime.*APIKeyTokenLimitExhausted|PostSettleErrorToOpenAIError'
```

预期：PASS。

---

## 任务 4：异步任务旧 token quota 清理

**文件：**
- 修改：`service/task_billing.go`
- 修改：`service/task_billing_test.go`

- [x] **步骤 1：编写失败测试：订阅任务退款不改旧 token quota**

当前 wallet 任务路径会因 `ErrLegacyWalletFundingDisabled` 提前 return，不能作为红灯用例。必须使用 `BillingSourceSubscription` 且 seed 非 distributor 的 legacy subscription，让 `taskAdjustFunding` 成功后命中当前 `taskAdjustTokenQuota` 调用。

在 `service/task_billing_test.go` 新增或扩展：

```go
func TestRefundTaskQuotaDoesNotAdjustLegacyTokenQuota(t *testing.T) {
    truncate(t)
    ctx := context.Background()
    const userID, tokenID, channelID, subID = 97001, 97002, 97003, 97004
    const preConsumed = 10

    seedUser(t, userID, 0)
    seedToken(t, tokenID, userID, "sk-task-refund-no-legacy", 100)
    require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", tokenID).Update("used_quota", 50).Error)
    seedChannel(t, channelID)
    seedSubscription(t, subID, userID, 1000, 500)

    task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceSubscription, subID)
    RefundTaskQuota(ctx, task, "task failed")

    require.Equal(t, int64(500-preConsumed), getSubscriptionUsed(t, subID))
    require.Equal(t, 100, getTokenRemainQuota(t, tokenID))
    require.Equal(t, 50, getTokenUsedQuota(t, tokenID))
}
```

- [x] **步骤 2：编写失败测试：订阅任务重算不改旧 token quota**

同文件新增：

```go
func TestRecalculateTaskQuotaDoesNotAdjustLegacyTokenQuota(t *testing.T) {
    truncate(t)
    ctx := context.Background()
    const userID, tokenID, channelID, subID = 97011, 97012, 97013, 97014
    const preConsumed = 10
    const actualQuota = 15

    seedUser(t, userID, 0)
    seedToken(t, tokenID, userID, "sk-task-recalc-no-legacy", 100)
    require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", tokenID).Update("used_quota", 50).Error)
    seedChannel(t, channelID)
    seedSubscription(t, subID, userID, 1000, 500)

    task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceSubscription, subID)
    RecalculateTaskQuota(ctx, task, actualQuota, "task settle")

    require.Equal(t, int64(500+actualQuota-preConsumed), getSubscriptionUsed(t, subID))
    require.Equal(t, 100, getTokenRemainQuota(t, tokenID))
    require.Equal(t, 50, getTokenUsedQuota(t, tokenID))
}
```

同步更新现有 `TestRefundTaskQuota_Subscription` 和 `TestRecalculate_Subscription_NegativeDelta`：删除「Token refunded」旧期望，改为断言 `remain_quota` / `used_quota` 不变。

- [x] **步骤 3：运行测试验证失败**

运行：

```bash
go test ./service -run 'TestRefundTaskQuotaDoesNotAdjustLegacyTokenQuota|TestRecalculateTaskQuotaDoesNotAdjustLegacyTokenQuota|TestRefundTaskQuota_Subscription|TestRecalculate_Subscription_NegativeDelta'
```

预期：失败，原因是当前 subscription 任务路径仍调用 `taskAdjustTokenQuota()` 修改旧 `remain_quota` / `used_quota`。

- [x] **步骤 4：删除任务路径旧 quota 调整**

在 `service/task_billing.go`：

- `RefundTaskQuota()` 删除 `taskAdjustTokenQuota(ctx, task, -quota)`。
- `RecalculateTaskQuota()` 删除 `taskAdjustTokenQuota(ctx, task, quotaDelta)`。
- 删除无人调用的 `taskAdjustTokenQuota` 函数；若仍有编译引用，逐一移除引用。
- 更新注释：任务资金来源只调整 subscription 资金来源；API Key 旧 quota 不参与请求和任务结算。

禁止把任务 token cap 半接入为只读日志聚合。本次任务只要求异步任务不再改旧 `remain_quota` / `used_quota`。

- [x] **步骤 5：运行任务定向测试**

运行：

```bash
go test ./service -run 'TaskQuota|TaskBilling|DoesNotAdjustLegacyTokenQuota'
```

预期：PASS。

---

## 任务 5：前端 API Key 表单和展示

**文件：**
- 修改：`web/default/src/features/keys/types.ts`
- 修改：`web/default/src/features/keys/lib/api-key-form.ts`
- 修改：`web/default/src/features/keys/components/api-keys-mutate-drawer.tsx`
- 修改：`web/default/src/features/keys/components/api-keys-columns.tsx`
- 修改：`web/default/src/features/keys/components/api-keys-table.tsx`
- 修改：`web/default/src/features/keys/api.ts`
- 修改：`web/default/src/features/keys/components/data-table-row-actions.tsx`
- 新增：`web/default/src/features/keys/lib/api-key-token-display.ts`
- 修改：`web/default/src/features/keys/api-key-form-visibility.test.ts`

测试运行命令切到 Bun，但测试文件继续保留 Node 兼容的 `node:test` 和 `node:assert/strict` import。当前 `bun test src/features/keys/api-key-form-visibility.test.ts` 已能执行现有 Node 风格测试；不要改用 Bun 专属 test import，否则 `bun run typecheck` 会因为项目未配置 Bun 类型而失败。

- [x] **步骤 1：编写失败测试：静态禁止旧 quota 表单和 formatter**

扩展 `web/default/src/features/keys/api-key-form-visibility.test.ts`。先在文件顶部新增相关源码读取；后续该测试会 direct import TypeScript 模块，不能再用 Node 内置 test runner 执行。

```ts
const tableSource = readKeysSource('./components/api-keys-table.tsx')
const apiSource = readKeysSource('./api.ts')
const rowActionsSource = readKeysSource('./components/data-table-row-actions.tsx')
```

新增静态测试：

```ts
test('does not expose legacy quota currency fields for API key limits', () => {
  assert.doesNotMatch(formSource, /remain_quota_dollars/)
  assert.doesNotMatch(formSource, /parseQuotaFromDollars|quotaUnitsToDollars/)
  assert.doesNotMatch(drawerSource, /getCurrencyDisplay|getCurrencyLabel|Quota \(\{\{currency\}\}\)|WalletCards/)
  assert.doesNotMatch(columnsSource, /formatQuota\(/)
  assert.doesNotMatch(tableSource, /formatQuota\(/)
})
```

该测试必须先失败：当前源码仍包含旧字段、旧 formatter 或移动端旧展示。

- [x] **步骤 2：编写失败测试：新 token 表单默认值、校验和 payload**

同一测试文件直接 import 表单 schema 和转换函数：

```ts
import {
  API_KEY_FORM_DEFAULT_VALUES,
  apiKeyFormSchema,
  transformApiKeyToFormDefaults,
  transformFormDataToPayload,
} from './lib/api-key-form'
import { formatApiKeyTokenCount } from './lib/api-key-token-display'

test('defaults to no token limit and validates enabled token limits', () => {
  assert.equal(API_KEY_FORM_DEFAULT_VALUES.token_limit_enabled, false)
  assert.equal(apiKeyFormSchema.safeParse({ ...API_KEY_FORM_DEFAULT_VALUES, token_limit_enabled: true, token_limit: 0 }).success, false)
  assert.equal(apiKeyFormSchema.safeParse({ ...API_KEY_FORM_DEFAULT_VALUES, token_limit_enabled: true, token_limit: undefined }).success, false)
  assert.equal(apiKeyFormSchema.safeParse({ ...API_KEY_FORM_DEFAULT_VALUES, token_limit_enabled: true, token_limit: 1.5 }).success, false)
  assert.equal(apiKeyFormSchema.safeParse({ ...API_KEY_FORM_DEFAULT_VALUES, token_limit_enabled: true, token_limit: 1000 }).success, true)
})

test('does not migrate historical quota fields into token limit defaults', () => {
  const defaults = transformApiKeyToFormDefaults({
    id: 1,
    name: 'legacy',
    key: 'sk-***',
    status: 1,
    remain_quota: 999999,
    used_quota: 888888,
    unlimited_quota: false,
    expired_time: -1,
    created_time: 1,
    accessed_time: 1,
    model_limits_enabled: false,
    model_limits: '',
    allow_ips: '',
    token_limit_enabled: false,
    token_limit: 0,
    token_used: 0,
    token_remaining: 0,
    token_unlimited: true,
  })
  assert.equal(defaults.token_limit_enabled, false)
  assert.equal(defaults.token_limit, undefined)
})

test('submits token limit fields without legacy quota limit fields', () => {
  const limitedPayload = transformFormDataToPayload({
    ...API_KEY_FORM_DEFAULT_VALUES,
    name: 'limited',
    token_limit_enabled: true,
    token_limit: 1000,
  })
  assert.equal(limitedPayload.token_limit_enabled, true)
  assert.equal(limitedPayload.token_limit, 1000)
  assert.equal('remain_quota' in limitedPayload, false)
  assert.equal('unlimited_quota' in limitedPayload, false)
  assert.equal('remain_quota_dollars' in limitedPayload, false)

  const unlimitedPayload = transformFormDataToPayload({
    ...API_KEY_FORM_DEFAULT_VALUES,
    name: 'unlimited',
    token_limit_enabled: false,
    token_limit: undefined,
  })
  assert.equal(unlimitedPayload.token_limit_enabled, false)
  assert.equal(unlimitedPayload.token_limit, 0)
  assert.equal('remain_quota' in unlimitedPayload, false)
  assert.equal('unlimited_quota' in unlimitedPayload, false)
  assert.equal('remain_quota_dollars' in unlimitedPayload, false)
})

test('formats zero remaining API key tokens explicitly', () => {
  assert.equal(formatApiKeyTokenCount(0), '0 tokens')
  assert.equal(formatApiKeyTokenCount(42), '42 tokens')
})

test('exposes reset token usage action and API client', () => {
  assert.match(apiSource, /resetApiKeyTokenUsage/)
  assert.match(apiSource, /\/api\/token\/\$\{id\}\/reset-token-usage/)
  assert.match(rowActionsSource, /Reset token usage/)
  assert.match(rowActionsSource, /resetApiKeyTokenUsage\(apiKey\.id\)/)
  assert.match(rowActionsSource, /triggerRefresh\(\)/)
  assert.match(rowActionsSource, /toast\.success/)
  assert.match(rowActionsSource, /toast\.error/)
  assert.match(rowActionsSource, /isResettingTokenUsage|resettingTokenUsageId/)
})
```

这些测试必须先失败：表单 schema / payload 尚未提供新 token limit 行为。

- [x] **步骤 3：运行测试验证失败**

运行：

```bash
cd web/default && bun test src/features/keys/api-key-form-visibility.test.ts
```

预期：失败。

- [x] **步骤 4：更新类型和表单转换**

`types.ts`：

```ts
token_limit_enabled: z.boolean().default(false),
token_limit: z.number().default(0),
token_used: z.number().default(0),
token_remaining: z.number().default(0),
token_unlimited: z.boolean().default(true),
```

`ApiKeyFormData`：

```ts
export interface ApiKeyFormData {
  name: string
  expired_time: number
  token_limit_enabled: boolean
  token_limit?: number
  model_limits_enabled: boolean
  model_limits: string
  allow_ips: string
}
```

`api-key-form.ts`：

- 删除旧 quota import。
- schema 改为 `token_limit_enabled` / `token_limit`。
- `superRefine` 或 schema 校验启用时必须为正整数；使用 `z.number().int().positive()` 或等价整数校验，拒绝 `0`、`undefined`、小数和负数。输入 onChange 与 `transformFormDataToPayload` 也必须保证启用时只提交正整数。
- 默认值：`token_limit_enabled: false`，`token_limit: undefined`。
- `transformFormDataToPayload` 显式返回字段，不用 `...data` 透传临时字段。
- `transformApiKeyToFormDefaults` 不从旧 `remain_quota` fallback；`apiKey.token_limit_enabled ?? false`。

- [x] **步骤 5：更新抽屉 UI**

`api-keys-mutate-drawer.tsx`：

- 删除 `getCurrencyDisplay`、`getCurrencyLabel`、`WalletCards`。
- `unlimitedQuota` 改为 `tokenLimitEnabled` 或 `noTokenLimit`。
- 表单 Section 文案：
  - title：`API Key Token Limit`
  - description：`Limits only this API key. Requests still consume subscription tokens.`
- Switch：`No token limit for this API key` 或反向开关 `Enable token limit`；保持逻辑清晰。
- Input：`Token limit` / `Enter token limit`。
- 历史 key 提示：`This API key uses the new token limit model. Historical quota limits were not migrated.`。

- [x] **步骤 6：更新桌面列和移动卡片**

`api-keys-columns.tsx` 与 `api-keys-table.tsx`：

- 列标题 / 卡片标签从旧 `Quota` 改为 `Token limit` 或 `API Key Token Limit`。
- 创建 `lib/api-key-token-display.ts`，导出 `formatApiKeyTokenCount(tokens: number): string`；该 helper 内部可以复用 `formatTokens`，但必须对 `0` 显式返回 `0 tokens`，不能裸用当前 `formatTokens(0)`，因为 `formatTokens(0)` 返回 `-`。
- 桌面列和移动卡片都使用 `formatApiKeyTokenCount`，不要使用 `formatQuota`。
- 未启用：`Unlimited` / `No key limit`。
- 启用：显示 `formatApiKeyTokenCount(token_used) / formatApiKeyTokenCount(token_limit)`，remaining 为 0 时显式显示 `0 tokens`。

- [x] **步骤 7：新增 reset token usage API client 和用户操作**

`api.ts` 新增：

```ts
export async function resetApiKeyTokenUsage(id: number): Promise<ApiResponse<ApiKey>> {
  const response = await api.post(`/api/token/${id}/reset-token-usage`)
  return response.data
}
```

`data-table-row-actions.tsx` 增加用户可见操作，不能只新增 API client：

- import 并调用 `resetApiKeyTokenUsage(apiKey.id)`。
- 在行操作菜单中加入 `Reset token usage`，建议放在 Edit 附近、Delete 之前。
- 点击时设置 `isResettingTokenUsage` 或 `resettingTokenUsageId`，loading 期间禁用该菜单项，避免重复提交。
- 成功时 `toast.success(t('API key token usage reset'))`，调用 `triggerRefresh()` 刷新列表；如果当前行仍在编辑或详情上下文中，必须同步刷新 / 重新拉取当前 key 数据，不能让 `token_used` 继续显示旧值。
- 失败时 `toast.error(result.message || t(ERROR_MESSAGES.UNEXPECTED))`；catch 分支同样提示错误；finally 清理 loading。
- 不得使用旧 `remain_quota` / `used_quota` / 钱包文案表达 reset。

如果后端返回 envelope 结构与现有 API client 不同，按现有 `createApiKey` / `updateApiKey` 模式处理。

- [x] **步骤 8：运行前端定向测试**

运行：

```bash
cd web/default && bun test src/features/keys/api-key-form-visibility.test.ts
```

预期：PASS。

---

## 任务 6：前端 i18n 同步

**文件：**
- 修改：`web/default/src/features/keys/api-key-form-visibility.test.ts`
- 修改：`web/default/src/i18n/locales/en.json`
- 修改：`web/default/src/i18n/locales/zh.json`
- 修改：`web/default/src/i18n/locales/fr.json`
- 修改：`web/default/src/i18n/locales/ja.json`
- 修改：`web/default/src/i18n/locales/ru.json`
- 修改：`web/default/src/i18n/locales/vi.json`
- 修改：`web/default/src/i18n/static-keys.ts`（仅当动态 key 需要）

任务 6 依赖任务 5 的 UI 文案和测试完成；如果用子代理执行，优先把任务 5 和任务 6 交给同一个前端子代理串行完成，避免两个子代理同时编辑 `api-key-form-visibility.test.ts`。

- [x] **步骤 1：编写失败检查：新增 i18n key 不存在**

在 `web/default/src/features/keys/api-key-form-visibility.test.ts` 中增加读取 locale JSON 的断言，确保以下 key 存在于 6 个 locale：

```ts
const requiredI18nKeys = [
  'API Key Token Limit',
  'No token limit for this API key',
  'Token limit',
  'Enter token limit',
  'Limits only this API key. Requests still consume subscription tokens.',
  'Token Limit Reached',
  'Reset token usage',
  'API key token usage reset',
  'This API key uses the new token limit model. Historical quota limits were not migrated.',
]

type LocaleFile = { translation?: Record<string, unknown> }

for (const locale of ['en', 'zh', 'fr', 'ja', 'ru', 'vi']) {
  test(`api key token limit i18n keys exist in ${locale}`, () => {
    const source = readKeysSource(`../../i18n/locales/${locale}.json`)
    const localeFile = JSON.parse(source) as LocaleFile
    const translations = localeFile.translation ?? {}
    for (const key of requiredI18nKeys) {
      const value = translations[key]
      assert.equal(typeof value, 'string', `${locale} missing ${key}`)
      const text = value as string
      assert.notEqual(text.trim(), '', `${locale} has empty ${key}`)
    }
  })
}
```

- [x] **步骤 2：运行测试验证失败**

运行：

```bash
cd web/default && bun test src/features/keys/api-key-form-visibility.test.ts
```

预期：失败，locale 缺少新 key。

- [x] **步骤 3：补齐 i18n**

手动补齐 6 个 locale 的 key。翻译要自然，不要机翻味。

本任务不要运行 `bun run i18n:sync`；该命令由任务 7 主代理最终统一运行，避免子代理间重复同步造成冲突。

如果新增状态常量通过 `t(config.label)` 间接使用，在 `static-keys.ts` 添加 key。

- [x] **步骤 4：运行 i18n 静态测试**

运行：

```bash
cd web/default && bun test src/features/keys/api-key-form-visibility.test.ts
```

预期：PASS。

---

## 任务 7：最终验证与修复

**文件：**
- 可能修改：前面任务遗留失败对应文件。

- [x] **步骤 1：运行后端定向测试**

运行：

```bash
go test ./model ./controller ./service ./relay/... -run 'Token|Billing|Subscription|ConfigGuide|Relay'
```

预期：PASS。

如果失败：修复生产代码或测试。不得跳过失败测试，不得扩大到全量无关重构。

- [x] **步骤 2：运行前端 i18n 和类型检查**

从 `web/default/` 运行：

```bash
bun run i18n:sync
bun run typecheck
```

预期：PASS。

- [x] **步骤 3：运行前端静态测试**

运行：

```bash
cd web/default && bun test src/features/keys/api-key-form-visibility.test.ts
```

预期：PASS。

- [x] **步骤 4：审查关键禁止项**

使用内置搜索确认：

- default API Key 路径不再出现 `remain_quota_dollars`。
- `api-key-form.ts` 不再导入 `parseQuotaFromDollars` 或 `quotaUnitsToDollars`。
- API Key columns / mobile card 不再用 `formatQuota()` 展示 key limit。
- 后端新 token cap 不使用 `common.QuotaPerUnit`。
- 新订阅 token-only 请求、relay、配置向导和异步任务路径不再调用 `model.IncreaseTokenQuota()` / `model.DecreaseTokenQuota()` 修改旧 API Key quota。

- [x] **步骤 5：请求最终代码审查**

使用 3 个 reviewer 子代理并发审查：后端生命周期、前端契约、端到端验收。所有 Critical / Important 必须修复并复审通过。
