# Issue #20 协调器真实浏览器验收证据

## 环境

- Worker 隔离工作树：`issue-20-valuation-foundation`
- 隔离服务：`http://127.0.0.1:3100`
- 数据库：`.scratch/agent-progress/issue-20/browser-smoke.db`
- 使用当前会话受控真实 Chromium 登录管理员账号后操作 default UI；未拦截或伪造 API。

## 管理员套餐创建

- 页面：`/subscriptions`
- 表单值：标题 `Issue 20 Browser Precision`，金额 `40.123456`，月 Credit `1000`。
- UI 发出 `POST /api/subscription/admin/plans`，HTTP 200。
- 请求同时包含：
  - `price_amount: "40.123456"`
  - `price_amount_micros: "40123456"`
  - `currency: "CNY"`
- 创建响应返回计划 ID `2`，并原样返回 `price_amount_micros: "40123456"`。
- 紧随其后的管理列表 GET 仍返回 `price_amount: 40.123456` 与 `price_amount_micros: "40123456"`。

## 编辑与刷新

- 从真实行操作菜单重新打开编辑表单，初始金额为 `40.123456`。
- 将金额改为 `41.654321`，UI 发出 `PUT /api/subscription/admin/plans/2`，HTTP 200。
- 请求同时包含：
  - `price_amount: "41.654321"`
  - `price_amount_micros: "41654321"`
  - `currency: "CNY"`
- 保存后的列表 GET 返回 `price_amount: 41.654321` 与 `price_amount_micros: "41654321"`。
- 完整页面 reload 后，行显示新副标题与价格；再次从行操作菜单打开编辑表单，`Actual Amount` 精确恢复为 `41.654321`，无精度漂移。

## Credit 估值币种往返

- UI 明确显示 `Credit valuation currency`，选择值为 `CNY`。
- 填写 concurrency `4`、queue `8`、Business Code `issue20-credit` 并确认配置。
- UI 发出 `PUT /api/subscription/admin/credit-balance-plan`，HTTP 200，请求包含：
  - `valuation_currency: "CNY"`
  - `configured: true`
  - `concurrency_limit: 4`
  - `queue_capacity: 8`
  - `business_code: "issue20-credit"`
- PUT 响应和刷新后的 GET 均返回 `valuation_currency: "CNY"` 及相同配置；页面 reload 后控件仍显示 CNY、4、8 与 `issue20-credit`。

## 结论

真实浏览器的 create → read → edit → refresh 与 Credit 估值币种往返通过。API payload 使用十进制字符串和精确 micros 字符串；服务端响应及刷新后的 UI 原值一致。
