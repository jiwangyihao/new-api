# Issue #21 夹具迁移 C 合同

## 所有权与非所有权

- 仅拥有 `controller` 余额购买、Kyren、Stripe、Epay/通用 payment webhook、邀请订单测试及必要的 controller 测试 setup/helper。
- 不修改 `model`、`service`、前端、locale、生产支付/履约代码；若真实可重复证据表明生产缺陷，先通过 Orca 向协调器提问并等待授权。
- 不实现 #22 CreditValuation、#23–#28、FX、migration marker/ready、三数据库实机或部署。

## 权威 Plan 夹具

用于新购买或从 Plan 冻结订单授权事实的 timed Plan 必须同时满足：

- `entitlement_type=timed`、`enabled=true`、非 trial/invite-trial；
- 稳定 `business_code`；
- 非 NULL 且为正的权威 `price_amount_micros`，币种仅 CNY/USD；兼容 `PriceAmount float64` 不作为权威来源；
- 正 `monthly_token_limit`；
- 受支持且正值的 duration，custom 时 `custom_seconds>0`；
- 受支持的 reset，custom 时 `quota_reset_custom_seconds>0`。

## 订单授权与履约

- 真实购买入口应自然冻结完整 `SubscriptionOrder.EntitlementSnapshot`；测试不得手写缺快照成功订单。
- 必须从已授权 provider order 开始的 webhook 测试，应复用已有合法入口/helper，从权威 Plan 生成并持久化完整 `SubscriptionEntitlementSnapshot`。
- 快照必须与订单授权时 Plan identity 一致，并冻结精确 micros、币种、Credit、duration/reset、规则版本；不得使用 `{}`、零值或仅绕过 nil 检查的数据。
- 成功回调只从不可变订单快照履约；当前 Plan 后续 disabled、改价或改币种不撤销已授权订单。
- 新购买 disabled Plan 继续拒绝。

## Provider 与重放合同

- 保留余额、Kyren、Stripe、Epay 原有签名、金额、provider 幂等和 HTTP 状态断言。
- webhook 重放返回同一 subscription 与 `[start,end)` 窗口，不新增 grant、不二次续期。
- 邀请订单的奖励与 entitlement 断言不得减少；邀请/试用不创建有价 timed grant。

## 测试隔离

- 每个相关 setup 保存并恢复全局 DB、Redis、setting 状态，不依赖测试顺序。
- 若本路 setup 缺少必要表，最小补充 `AutoMigrate`；不得吞缺表日志。
- 若 Redis panic 属于无关测试，保留精确证据并通过 Orca 交给协调器。

## 验收命令

- 各 provider/邀请最小 RED→GREEN。
- `go test ./controller -run 'Balance|Kyren|Stripe|Epay|Payment|Invitation|SubscriptionOrder' -count=1`
- 关键 provider/重放测试 `-count=10`。
- `go test ./controller -count=1`
- `git diff --check`，最终 staged/unstaged/untracked 全零。
