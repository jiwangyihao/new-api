# PRD #9 生产钱包静态冒烟边界记录

日期：2026-08-06

## 已验证

- 当前生产实例位于 `netcup-ows-migrate`；容器 `3008e47b8444` 为 healthy，容器端口 `3000/tcp` 映射到主机 `127.0.0.1:13080` 与 `127.0.0.1:13081`。
- 容器镜像为 `ghcr.io/jiwangyihao/new-api@sha256:ddce80eb2ada9d5f52b014fb2ed2c0e8032f74afd62265ba7881f94fb505aec7`。
- 通过临时 SSH 隧道读取 `GET /api/status` 成功，返回 `success=true`，版本为 `deploy-20260802-c51ee86`。
- 本地 Git 验证 `f446a1569c2ced54a3fe438b5c4575659a59241d` 是生产报告版本 `c51ee86a33d87c30f080567d3d59b801f064ba5b` 的祖先，因此生产提交包含目标钱包改动。
- Chromium 请求 `/wallet` 能加载生产 HTML、JavaScript 与 CSS，并被正常重定向到登录页；页面标题为 `PQ API`。这只证明钱包入口和生产静态资源可达。

## 未验证与失败边界

- 未持有可用于本次只读验收的真实生产认证会话，因此没有验证认证后的真实钱包页面。
- 按协调器设计尝试用请求拦截构造仅前端静态夹具，但该路径先后出现：`document is not defined`、两次 `wait(predicate)` 30 秒超时，以及认证路由回退到登录页并显示多次 `Session expired!`。
- 上述 mock 路径不能增加生产后端置信度，已按阻断指令停止；不得据此宣称以下验收通过：`Add Account Balance` 不可见、Credit 激活真实发出 `PUT /api/subscription/self/active`、`1,500,000` 显示为 `1.5M`。
- 本次没有向生产发出任何写请求，也没有修改生产数据。

## 资源清理

- 浏览器标签 `prd9-wallet-smoke` 已关闭。
- SSH 隧道进程 `prd9-production-wallet-tunnel` 已停止。

## 结论

生产状态端点、钱包入口和静态资源可达，且生产提交包含目标钱包提交；真实认证钱包流程仍未验证。该 TODO 保持 blocked，不能记为完成。
