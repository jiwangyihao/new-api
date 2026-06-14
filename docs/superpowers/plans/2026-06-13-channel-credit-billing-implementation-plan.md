# 渠道 Credit 计费与动态倍率实现计划

> 本计划用于隔离工作树 `C:/Users/34404/source/repos/new-api/.worktrees/credit-billing`。实现必须以规格 `docs/superpowers/specs/2026-06-13-channel-fixed-request-dynamic-billing-spec.md` 为准。

## 目标

实现渠道级 credit billing profile：

- `usage_tokens`：按可信 usage token 结算 credit。
- `fixed_request`：每个可计费请求固定扣 `fixed_request_credits`，但仍保持「无可信 usage 不扣费」。
- 上游动态倍率：仅当渠道显式启用 `dynamic_billing_multiplier_enabled` 时，由 adapter/响应标准化层写入请求级 settlement metadata，结算层只消费标准化数值。
- 全面把通用额度池产品语义迁移为 credit；真实模型 usage token、max/context token、tokenizer、OpenAI 兼容协议字段仍保留 token。
- 订阅扣费和 API Key cap 共享同一次 credit billing 结果，不再分叉计算。
- Realtime/WebSocket fixed request 即使出现多次 usage 增量，也只能按请求扣一次 fixed credits。

## 总体原则

1. 不改 legacy quota / 资金成本口径：`logs.quota`、`users.used_quota`、`channels.used_quota`、`PriceData.Quota`、`QuotaToPreConsume` 仍是旧 quota/成本口径。
2. 不把 fixed request 或动态倍率塞进 `pkg/billingexpr`。
3. JSON marshal/unmarshal 使用 `common.*` 包装。
4. 数据库迁移、更新逻辑兼容 SQLite/MySQL/PostgreSQL；更新显式 false/0 必须落库。
5. 新增或触达 API/DTO 优先使用 `credit_*` 字段；旧 `token_*` 仅作为兼容字段或内部存储细节。
6. 子代理不得运行项目范围格式化；每个任务只运行自身定向测试。最终由主会话统一运行验收命令。

## 低冲突任务拆分

### A. `pkg/creditbilling` 统一 helper

**主责文件**

- `pkg/creditbilling/creditbilling.go`
- `pkg/creditbilling/creditbilling_test.go`

**实现内容**

- 新建 `CreditBillingInput` / `CreditBillingResult`。
- 支持 `ModeUsageTokens = "usage_tokens"`、`ModeFixedRequest = "fixed_request"`。
- 输出 `Chargeable`、`HasTrustedUsage`、`BaseCredits`、`APIKeyCredits`、`SubscriptionCredits`、`DynamicBillingMultiplier`、`DynamicBillingMultiplierSource`、`ZeroReason`。
- 取整规则：decimal half-up / half-away-from-zero；正输入最小扣 1；0 输入为 0；非法倍率返回错误。
- `Chargeable=false` 或 `HasTrustedUsage=false` 标准零扣费。
- usage mode：base = raw metered tokens × channel token billing multiplier。
- fixed request mode：base = fixed_request_credits；raw usage 仅决定是否可信，不作为 base。
- final subscription credits = base × dynamic multiplier。
- API Key credits 与 subscription credits 使用同一一次性结果，除非规格明确保留基础口径字段用于审计。

**测试**

- usage tokens 基础倍率。
- fixed request 基础扣费。
- no trusted usage 0。
- trusted usage 且 total/raw 为 0：usage mode 0；fixed request 扣固定值。
- 动态倍率启用后的最终 credits。
- 正输入最小 1。
- 非法倍率拒绝。
- non-chargeable 0。

### B. 渠道模型、API、上下文、retry billing profile

**主责文件**

- `model/channel.go`
- `controller/channel.go`
- `middleware/distributor.go`
- `constant/context_key.go`
- `model/ability.go`
- `model/channel_cache.go`
- `controller/relay.go`
- `controller/channel_credit_billing_profile_test.go` 或扩展现有 channel token multiplier tests

**实现内容**

- `model.Channel` 新增：
  - `CreditBillingMode string json:"credit_billing_mode" gorm:"not null;default:'usage_tokens'"`
  - `FixedRequestCredits int64 json:"fixed_request_credits" gorm:"not null;default:0"`
  - `DynamicBillingMultiplierEnabled bool json:"dynamic_billing_multiplier_enabled" gorm:"not null;default:false"`
- 默认值：usage_tokens / 0 / false。
- 校验：mode 只能 usage_tokens/fixed_request；fixed_request 必须 `fixed_request_credits > 0`；usage_tokens 可把 fixed_request_credits 归零。
- controller add/update 使用 raw presence；显式 false/0 能落库。GORM 更新使用 map 或 `Select`，不要因零值被忽略。
- distributor 写入 context：credit billing mode、fixed request credits、dynamic multiplier enabled。
- retry 只允许同 billing profile：mode、fixed credits、token multiplier、dynamic enabled 都兼容。

**测试**

- 创建默认 profile。
- fixed request 缺少 credits 拒绝。
- update 保留未传字段。
- update 显式 `false` / `0` 落库。
- retry 不跨 profile。

### C. RelayInfo 与 service 结算收敛

**主责文件**

- `relay/common/relay_info.go`
- `service/billing_session.go`
- `service/text_quota.go`
- `service/quota.go`
- `service/billing.go`
- `service/token_limit_session.go`
- `service/*credit*billing*_test.go`

**实现内容**

- `RelayInfo` 冻结请求级 billing profile：mode、fixed credits、dynamic enabled、dynamic multiplier、dynamic source、has trusted usage。
- `clearRelayBillingState` 清理新增字段。
- 预扣阶段：usage mode 仍按估算 token；fixed request 预扣一次 fixed credits。
- 实际结算阶段统一调用 `pkg/creditbilling.Calculate`。
- `HasTrustedUsage` 不再由 `TotalTokens > 0` 反推；adapter/响应解析层或 settlement metadata 显式设置。文本兼容路径可把「收到可信 usage 对象」作为 trusted，但不能要求 total 大于 0。
- 保持 no usage 不扣费：无可信 usage 时 refund/pre-consume delta 回 0。
- Codex Pro 不再因为 `X-NewAPI-Pro-Served` 写死 2x；无 numeric dynamic multiplier 时为 1。
- Realtime/WebSocket fixed request 对同一请求使用请求级幂等标记，只结算一次 fixed credits；多次 usage 增量只累计审计 usage，不重复产生 base credits。
- API Key cap、订阅扣费和审计日志消费同一个 result。

**测试**

- text fixed request 正常扣一次。
- text no trusted usage 不扣费。
- trusted zero total：usage mode 0，fixed request 扣固定。
- dynamic disabled 忽略上游 multiplier。
- dynamic enabled 生效。
- WSS fixed request 多 usage 增量只扣一次。
- Codex Pro legacy header 不再隐式 2x。

### D. 动态倍率标准化层

**主责文件**

- `relay/common/relay_info.go` 的动态倍率 setter/getter（如需修改需与 C 保持字段兼容）
- `relay/channel/openai/*`、`relay/common/*`、`dto/*` 中实际解析上游响应 usage 的位置
- 对应 relay tests

**实现内容**

- 动态倍率只能在渠道 `dynamic_billing_multiplier_enabled=true` 时读取。
- adapter/响应解析层从上游 body/SSE/header/trailer 中提取明确 numeric multiplier，并写入标准化 metadata；service 不解析 provider JSON。
- 非 numeric、NaN、Inf、<=0、超过上限的 multiplier 拒绝或忽略，并记录 source/zero reason 可审计。
- 默认 multiplier = 1，source = `default`。
- 保留 `X-NewAPI-Pro-Served` 作为日志/标记，不作为隐式扣费倍率。

**测试**

- disabled 忽略 numeric multiplier。
- enabled 接受合法 multiplier。
- 非法 multiplier 回退或报错符合规格。
- Codex Pro served header 不导致 2x。

### E. subscription channel credit equivalents API

**主责文件**

- `model/subscription_channel_equivalent.go`
- `model/subscription.go`
- `controller/subscription.go`
- `controller/subscription_channel_equivalents_test.go`
- `router/subscription_public_plans_route_test.go`

**实现内容**

- 旧 `channel_token_equivalents` 产品语义迁移为 `channel_credit_equivalents`。
- Go 类型改为 credit：`PlanChannelCreditEquivalent`、`SubscriptionChannelCreditEquivalent`。
- discriminated fields：
  - `kind`: `usage_tokens` / `fixed_request` / `unlimited`
  - `value_type`: `single` / `range` / `unlimited`，必填。
  - usage tokens 展示等价 token 量。
  - fixed request 展示请求次数。
- usage mode：按 channel token multiplier 计算 plan/subscription credit 等价 token。
- fixed request：按 fixed_request_credits 计算可用请求次数。
- unlimited：`value_type=unlimited`。
- API 可保留旧字段兼容，但新字段必须返回并由前端消费。

**测试**

- usage token single/range/unlimited。
- fixed request single/range/unlimited。
- `value_type` 必填。
- public plans route 返回 `channel_credit_equivalents`。

### F. default 前端渠道表单

**主责文件**

- `web/default/src/features/channels/types.ts`
- `web/default/src/features/channels/lib/channel-form.ts`
- `web/default/src/features/channels/components/drawers/channel-mutate-drawer.tsx`
- 相关 channels 测试（如存在）

**实现内容**

- 类型新增 `credit_billing_mode`、`fixed_request_credits`、`dynamic_billing_multiplier_enabled`。
- 表单默认值：usage_tokens / 0 / false。
- Zod 校验：fixed_request 必须 fixed credits > 0；usage_tokens 可为 0。
- 编辑渠道时正确回填；提交 payload 包含显式 false/0。
- UI 增加 billing profile 控件：mode select、fixed credits input、dynamic multiplier enabled switch。
- 文案使用 `t()`，不要裸英文。

**验证**

- `cd web/default && bun run typecheck` 最终统一跑；任务内可只跑相关 TS 测试/类型检查片段。

### G. default 前端 credit 展示与 i18n

**主责文件**

- `web/default/src/features/subscriptions/types.ts`
- `web/default/src/features/wallet/lib/subscription-display.ts`
- `web/default/src/features/wallet/**`
- `web/default/src/features/home/**`
- `web/default/src/features/keys/**`
- `web/default/src/features/usage-logs/**`
- `web/default/src/features/usage-analytics/**`
- `web/default/src/features/admin-analytics/**`
- `web/default/src/features/dashboard/**`
- `web/default/src/i18n/static-keys.ts`
- `web/default/src/i18n/locales/{en,zh,fr,ja,ru,vi}.json`

**实现内容**

- 只迁移通用额度池/订阅额度/API Key cap/扣费/已用/剩余/预扣语义为 credit。
- 保留真实 usage token 文案：prompt/completion/total tokens、model token limits、tokenizer、credential API token。
- `channel_credit_equivalents` TS 使用 `kind + value_type` discriminated union。
- 套餐卡：usage_tokens 渠道展示 token 量；fixed_request 渠道展示请求次数。
- wallet/home/subscriptions/keys/logs/analytics/dashboard 中用户可见通用额度统一 credit。
- 更新六语言 locale 和 `static-keys.ts`；最终运行 `bun run i18n:sync` 并检查 `_sync-report.json`。

### H. 日志、审计、导出核对

**主责文件**

- `service/log_info_generate.go`
- `model/log.go`
- `controller/log.go`
- 相关 log/export tests

**实现内容**

- 新日志信息暴露 credit billing mode、base credits、API key credits、subscription credits、dynamic multiplier source。
- legacy `quota` 仍是 legacy quota，不改含义。
- 导出/接口触达字段使用 credit 命名；兼容字段保留但前端新代码不依赖。
- 日志展示「Consumption Multiplier」保留或扩展为动态倍率与渠道倍率可解释。

---

## 审查修改循环

开发完成后至少启动 3 个只读 review 子代理：

1. **Spec compliance review**：逐条核对规格 §13/§14，重点 fixed request、dynamic multiplier、no usage、WSS once-only、credit wording。
2. **Backend correctness review**：检查 cross-DB、GORM zero-value、retry profile、settlement consistency、legacy quota 边界。
3. **Frontend/i18n review**：检查 default 前端 credit 文案、真实 token 保留、六语言 locale、`static-keys.ts`、type union。

所有 review PASS 后才进入最终验证。若有 finding，修复后重复 review。

## 最终验证命令

在 `.worktrees/credit-billing` 运行：

```bash
go test ./pkg/creditbilling ./pkg/tokenbilling ./service ./controller ./model ./router -count=1
```

在 `.worktrees/credit-billing/web/default` 运行：

```bash
bun run i18n:sync
bun run typecheck
```

如某个命令失败，必须定位根因并修复；不能通过跳过测试、删断言或降低验收范围完成。
