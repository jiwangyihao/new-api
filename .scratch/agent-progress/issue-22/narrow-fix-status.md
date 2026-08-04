# Issue #22 窄验收修复状态

## 冻结基线
- 冻结 clean HEAD：`d5bba460f633ffd2943b1d13bb88b65cea338733`。
- 工作树：`jiwangyihao/issue-22-credit-tracer`。
- 启动时 `git status --short`：空。

## 当前阶段
- 阶段：准备完成，开始 finding A 的排序行为 RED。
- 已完成：读取 Issue #19/#22、执行合同、Wave 1 合同、Issue #22 实现/验收说明、协调器窄审查、既有 progress、`CONTEXT.md`、ADR 0002 与 2026-08-02 规格对应章节；确认只修复 micros 排序与五面板 current-only warning。
- 下一步：为 users/subscriptions/plans/sources 的升降序与稳定 tie-breaker 写 precision-boundary 行为测试并取得 RED。
- 阻塞：无。
- 最近安全提交：`d5bba460f633ffd2943b1d13bb88b65cea338733`（本文件提交后更新）。

## 范围边界
- 不重做 CreditValuation、人民币余额、Kyren、BillingSession/request_id、32 CNY tracer、六语言或浏览器 smoke。
- 不实现 Issue #23–#28，不写 migration marker，不切换 `ready`，不实现 FX。
- 仅运行 model/controller 与实际相关前端定向门禁，不运行全仓测试。
