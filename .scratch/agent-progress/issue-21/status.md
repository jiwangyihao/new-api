# Issue #21 状态

## 当前阶段

管理员 timed grant UI 已完成首个 RED→GREEN：表单收集 reason、冻结精确套餐 micros/原币种，并对失败重试复用确定性 attempt key；业务事实改变后生成新 key。timed grant 领域写入、五接口逐币种分析及窗口 warning/裁剪继续以 clean 基线为准，不重做。

当前剩余：跨币种 timed 明细显示、六语言、真实浏览器 smoke 与最终定向门禁。

## 已完成

- 统一 `GrantTimedSubscriptionTx` 与不可变 `TimedSubscriptionValuationGrant` 已接入订单、兑换和管理员真实入口。
- 重放、参数冲突、续期追加、改价/改币种冻结、disabled 新来源拒绝和 grant 更新/删除拒绝已有真实 SQLite 证据。
- summary/users/subscriptions/plans/sources 已统一读取 grant 时间线；跨币种 singular 为 null，`*_by_currency` 保留原币种。
- `missing_timed_grants`、`overlapping_grants` 与实际失效窗口裁剪已有定向 GREEN。
- 已读取 `skill://shadcn-ui` 与 `skill://i18n-translate`，UI 将复用现有 Base UI/shadcn 组合并维护 en/zh/fr/ru/ja/vi。

## 下一步

1. 补 timed 明细的 nullable singular 与三组 `*_by_currency` 可见展示测试及实现。
2. 补齐管理员授予与 timed 分析新增文案的 en/zh/fr/ru/ja/vi 翻译并运行同步检查。
3. 启动真实应用，完成管理员授予重试与跨币种浏览器 smoke。
4. 运行 Issue #21 定向门禁、`git diff --check`，更新最终证据并保持工作树 clean。

## 阻塞

- 当前没有外部阻塞。
- 明确不触碰 Credit 核心、FX、marker/ready、历史迁移或发布。

## 最近安全提交

- `f812e77fcd6e3d2875ce7b973ccc49c87e612590 test(analytics): 覆盖计时失效窗口裁剪`。
- 本轮管理员 UI GREEN 待提交。
