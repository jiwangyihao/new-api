# Issue #21 状态

## 当前阶段

Orca 1.4.167 重启后由当前 Dispatch `ctx_7d91bd847e54` 接管。已核对工作树为 clean HEAD `f812e77fcd6e3d2875ce7b973ccc49c87e612590`；该提交及其祖先中的 timed grant 领域写入、不可变性、真实调用点、五接口逐币种分析、缺口/重叠 warning 与实际 `end_time` 裁剪均作为已验收恢复基线，不重做。

当前只完成管理员 timed grant UI、跨币种 timed 展示、六语言、真实浏览器 smoke 与最终定向门禁。

## 已完成

- 统一 `GrantTimedSubscriptionTx` 与不可变 `TimedSubscriptionValuationGrant` 已接入订单、兑换和管理员真实入口。
- 重放、参数冲突、续期追加、改价/改币种冻结、disabled 新来源拒绝和 grant 更新/删除拒绝已有真实 SQLite 证据。
- summary/users/subscriptions/plans/sources 已统一读取 grant 时间线；跨币种 singular 为 null，`*_by_currency` 保留原币种。
- `missing_timed_grants`、`overlapping_grants` 与实际失效窗口裁剪已有定向 GREEN。
- 已读取 `skill://shadcn-ui` 与 `skill://i18n-translate`，UI 将复用现有 Base UI/shadcn 组合并维护 en/zh/fr/ru/ja/vi。

## 下一步

1. 以组件可观察行为写管理员 reason/idempotency/retry payload 的 UI RED，再最小实现。
2. 补 timed 三组 `*_by_currency` 与 nullable singular 的展示行为。
3. 补齐六语言并运行 i18n 同步检查。
4. 启动真实应用，完成管理员授予重试与跨币种浏览器 smoke；最后运行定向门禁和 `git diff --check`。

## 阻塞

- 当前没有外部阻塞。
- 明确不触碰 Credit 核心、FX、marker/ready、历史迁移或发布。

## 最近安全提交

- `f812e77fcd6e3d2875ce7b973ccc49c87e612590 test(analytics): 覆盖计时失效窗口裁剪`。
- 本接管记录尚待提交。
