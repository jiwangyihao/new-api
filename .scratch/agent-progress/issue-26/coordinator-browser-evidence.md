# Issue #26 协调器真实浏览器验收证据

日期：2026-08-06
候选提交（验收前）：`533bb7729b4db5f83f2fac3fe14555ac5380e320`

## 隔离启动

- 工作树：`C:/Users/34404/source/repos/new-api/.workspaces/new-api/issue-26-conversion-fx`
- 进程由 Hub 以命令内显式环境启动：
  - `cmd.exe /d /s /c set PORT=30261&& set SQLITE_PATH=C:\Users\34404\AppData\Local\Temp\new-api-issue26-browser.sqlite&& set SESSION_SECRET=issue26-browser-session-20260806&& set GIN_MODE=release&& go run .`
- Hub `describe` 确认上述完整命令；readiness 同时确认日志 `Local: http://localhost:30261` 与 TCP 端口 `30261`。
- 数据库为专用 SQLite fixture；真实 Chromium 访问 `http://127.0.0.1:30261/wallet`，未拦截或伪造 API。

## quote → confirm → history

真实钱包 UI 的 `TimedSubscriptionConversionQuotesCard`：

1. 展示可转换的 CNY timed subscription `#26902`。
2. 预览显示规则公式 `0 × 100 + 100 = 100`，其中完整 31 天块为 0、当前周期剩余 Credit 为 100。
3. UI 明确提示 conversion 是 rules-based valuation，`not a new payment`。
4. 点击 `Submit conversion` 后，真实 `POST /api/subscription/self/conversions` 返回 HTTP 200。
5. 返回和页面历史均显示：
   - `source_price_micros = 40000000`
   - `source_currency = CNY`
   - `target_currency = USD`
   - `fx_numerator/fx_denominator = 10/73`
   - `fx_direction = CNY_TO_USD`
   - `gross_cost_micros = 5479452`
   - `net_cost_micros = 5479452`
   - `unit_value_numerator_micros/unit_value_denominator = 4000000/73`
   - `rule_version = 1`
   - `state_version_after = 1`
   - gross/net/target Credit 均为 100。
6. `GET /api/subscription/self/conversion-quotes`、`GET /api/subscription/self` 与 `GET /api/subscription/self/credit-balance/ledger` 均返回 HTTP 200；conversion、Credit state、ledger 与 converted history 一致且各只有一份结果。

## 冻结事实复验

conversion 提交后，协调器直接修改隔离数据库中的当前配置：

- `USDExchangeRate: 7.3 → 8.5`
- timed Plan 当前展示价格：`40 CNY / 40000000 micros → 55 CNY / 55000000 micros`

完整刷新钱包后：

- 当前 Plan 卡片显示 `¥55.00`，证明当前配置确已变化；
- conversion history 仍显示原 `source_price_micros = 40000000`；
- frozen FX 仍为 `10/73`，方向仍为 `CNY_TO_USD`；
- gross/net cost 仍为 `5479452` micros；
- rule/state version 与 `fx_captured_at` 均保持转换时事实。

这证明历史不按当前 Option 或 Plan 价格动态重估。

## 清理

- Chromium 标签 `issue26-browser` 已关闭。
- Hub 服务 `issue26-browser-cmd` 已停止。
- 隔离 SQLite、WAL、SHM 与错误启动产生的 `one-api.db*` 均已删除。
- 候选工作树经 `git status --short --branch`、`git diff --check` 验证为 staged 0 / unstaged 0 / untracked 0（写入本证据文件前）。

结论：真实 Chromium 的 quote→confirm→history 主路径、跨币种冻结 FX、规则确值非新增收款、以及当前配置变化后历史事实不变均通过。
