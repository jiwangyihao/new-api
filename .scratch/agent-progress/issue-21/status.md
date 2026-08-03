# Issue #21 状态

## 当前阶段

管理员 timed grant UI、timed 跨币种运营剩余价值展示与六语言均已 GREEN：reason/精确 micros/原币种进入 payload，失败重试复用 key；跨币种 singular null 时显示三组原币种明细；新增文案在 en/zh/fr/ru/ja/vi 均有真实翻译。

当前剩余：真实浏览器 smoke 与最终定向门禁。

## 已完成

- 统一 `GrantTimedSubscriptionTx` 与不可变 `TimedSubscriptionValuationGrant` 已接入订单、兑换和管理员真实入口。
- 重放、参数冲突、续期追加、改价/改币种冻结、disabled 新来源拒绝和 grant 更新/删除拒绝已有真实 SQLite 证据。
- summary/users/subscriptions/plans/sources 已统一读取 grant 时间线；跨币种 singular 为 null，`*_by_currency` 保留原币种。
- `missing_timed_grants`、`overlapping_grants` 与实际失效窗口裁剪已有定向 GREEN。
- 已读取 `skill://shadcn-ui` 与 `skill://i18n-translate`，UI 将复用现有 Base UI/shadcn 组合并维护 en/zh/fr/ru/ja/vi。

## 下一步

1. 启动真实应用，完成管理员授予重试与跨币种浏览器 smoke。
2. 运行 Issue #21 定向门禁、`git diff --check`，更新最终证据并保持工作树 clean。

## 阻塞

- 当前没有外部阻塞。
- 明确不触碰 Credit 核心、FX、marker/ready、历史迁移或发布。

## 最近安全提交

- `f812e77fcd6e3d2875ce7b973ccc49c87e612590 test(analytics): 覆盖计时失效窗口裁剪`。
- 管理员 UI：`8e143ca77 feat(subscription): 完成计时售后授予交互`。
- 跨币种展示：`1809124c5 feat(analytics): 展示计时跨币种运营剩余价值`。
- 六语言 GREEN 待提交。
