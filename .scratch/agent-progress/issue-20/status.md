# Issue #20 进度状态

## 当前阶段

收尾与交付：实现、定向门禁、真实 default UI create → read → edit → refresh 及 Credit 估值币种往返均已完成；正在提交最终证据并清理工作树。

## 已完成

- 固定 #20 所有权：前向精确价格、独立 Credit 估值币种门禁、附加式 schema、只读非法价格诊断、防溢出整数比例。
- 固定非所有权：不回填历史价格或估值；不修改 migration marker 状态；不决定或阻止 `ready`；不启用 Credit 数量/估值强制双写。
- 后端 model/controller/router 定向回归通过；router 公开套餐 DTO 已显式覆盖 nullable `price_amount_micros`。
- 前端 typecheck、两份定向测试（25 pass）、六语言 i18n 同步通过。
- 隔离 SQLite 实例使用 3100 与 `.scratch/agent-progress/issue-20/browser-smoke.db`；协调器以真实受控 Chromium 完成 default 管理 UI create → read → edit → refresh。
- 浏览器实测证明请求中的 `price_amount` 原始十进制字符串、`price_amount_micros` 精确字符串、响应、列表读取及 reload 后表单值一致；Credit `valuation_currency` CNY 与独立配置同样通过 PUT/GET/reload。
- 本终端的 Orca tab/桌面控制错误已持久化并发送 escalation，随后由协调器接管唯一浏览器验收项；未把 API 或组件测试冒充浏览器通过。

## 下一步

1. 提交协调器浏览器证据与最终状态。
2. 确认 TODO 全部完成、隔离服务停止、临时数据库删除、工作树 clean。
3. 发送一次 `worker_done` 并立即停止。

## 遗留风险

MySQL/PostgreSQL DSN 未提供，真实外部数据库测试明确跳过；最终证据只覆盖真实 SQLite、跨方言 SQL/迁移合同测试，不能宣称三数据库实际通过。

## 最近安全提交

`b1ca3541b`：`test(subscription): 覆盖公开套餐精确价格字段`；恢复/门禁证据提交为 `88450e1e1`，主实现安全点为 `4823efa37`。
