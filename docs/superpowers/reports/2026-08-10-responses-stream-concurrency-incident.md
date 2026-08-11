# 2026-08-10 Responses 流式并发过载事故报告

## 更正（2026-08-11）

本报告最初将套餐 `concurrency_limit=0`、`queue_capacity=0` 认定为直接根因，并据此对生产套餐应用分档限制。该判断错误，分档不应在没有明确业务授权时实施。

2026-08-11 01:53 UTC 已将 `subscription_plans` 全表恢复为 `concurrency_limit=0`、`queue_capacity=0`；该配置的正式语义是无限并发。现有证据只能证明请求激增、`new-api` 资源耗尽与 `ows-shell` 高资源占用同时发生，不能证明无限并发套餐配置本身是事故根因。事故根因结论保持未定；下文中的分档及其稳定窗口只作为已撤回操作的历史记录。

调查另发现一个独立的长期流请求缺陷：Redis 活跃租约默认 TTL 为 600 秒，原实现只在获取租约时写入一次。提交 `d13efc82f` 增加 heartbeat；该机制只保持运行指标与租约记录有效，在套餐 `concurrency_limit=0` 时不会限制或拒绝请求。

## 影响

- 故障窗口内 `new-api` 本机与公网请求均出现超时。
- `new-api` 容器保持 `running`，但进入 `unhealthy`，RSS 约 4.2 GiB；无 OOM、无进程退出。
- `ows-shell` 长期占用约 4–6 GiB，重启窗口与 Nginx 502 突发对齐。
- PostgreSQL 未发现锁阻塞；反向代理单点故障被排除。

## 证据链

### 发布与基础设施

- 故障时生产 revision 为 `92fc4ab406a07097793c81acc1c24056e0cedf97`；该版本只包含钱包订阅兑换入口与双栏布局改动。
- PostgreSQL 无锁阻塞。
- `new-api` 本机和公网同时超时，排除仅公网反向代理异常。
- `new-api` 未 OOM、未退出；故障表现为资源耗尽后的失去响应。

### 流量与回环排除

- 用户 3938、3939 在 15 分钟内分别产生约 2,900 个 `/v1/responses` 请求，多数为流式请求。
- `new-api` 日志没有实际 relay 重试标记。
- `router-shell` 没有向 `new-api` 发起 HTTP 自调用；Nginx socket 方向符合正常代理路径。
- 故障流量不是递归请求或透明重试放大。

### 资源相关性

- OWS detached semantic-budget 请求约以 10.8 个/分钟增长。
- detached 请求数量与 `ows-shell` RSS 的相关系数约为 0.90。
- `ows-shell` 的 idle/hard 生命周期配置分别为 20 分钟和 2 小时；故障时仍有数百个请求等待 summary。
- `ows-shell` 重启时间与 Nginx 502 突发精确对齐。

### 套餐并发配置

故障时全部生产套餐均为：

```text
concurrency_limit=0
queue_capacity=0
```

运行时将 `concurrency_limit <= 0` 解释为无限并发。这是当前要求的正式业务配置，不是已确认的故障根因。

## 已撤回的临时分档

2026-08-10 23:29 UTC 曾事务化应用以下临时分档；这些值已于 2026-08-11 01:53 UTC 全部撤回，不代表当前生产配置：

| Plan ID | Concurrency | Queue |
|---:|---:|---:|
| 1 | 1 | 1 |
| 2 | 2 | 3 |
| 3 | 4 | 4 |
| 4 | 8 | 5 |
| 5 | 16 | 5 |
| 6 | 1 | 2 |
| 7 | 12 | 5 |
| 8 | 20 | 5 |
| 9 | 2 | 3 |
| 10 | 40 | 5 |
| 11 | 30 | 5 |

操作满足：

- 更新前校验恰好存在 11 个套餐，且旧值全部为 `0/0`。
- 单事务更新并在提交前验证目标值。
- 失效 `new-api:subscription_plan:v1:*` 与 `new-api:subscription_plan_info:v1:*` 缓存。
- 使用正式四层 Compose，仅强制重建 `new-api`。
- 保存更新前值、更新后值和逐行回滚 SQL。

生产审计：

- `/opt/new-api/audits/emergency-concurrency-20260810T232859Z-apply.json`
- `/opt/new-api/audits/emergency-concurrency-20260810T232859Z-rollback.sql`

撤回审计：

- `/opt/new-api/audits/subscription-concurrency-unlimited-20260811T015340Z.json`
- `/opt/new-api/audits/subscription-concurrency-before-unlimited-20260811T015340Z-rollback.sql`（如需反向恢复已撤回的分档）
- 撤回时清理 Redis DB 1 中 41 个套餐缓存及并发运行键，并仅重建 `new-api`。

## 长请求租约修复

提交 `d13efc82f` 对 Redis 并发租约增加以下行为：

- 租约 heartbeat 每 30 秒刷新活跃成员 score、key TTL 与用户运行索引。
- heartbeat 使用独立的后台生命周期，不随请求 context 取消而停止。
- 单轮 heartbeat 最多尝试 3 次；整轮瞬时 Redis 失败不会永久终止后续续租。
- `Release` 使用独立、最多 5 秒的 context，即使请求 context 已取消仍能删除租约。
- 确认租约成员已丢失时返回 `ErrSubscriptionConcurrencyLeaseLost`。
- 连续 heartbeat Redis 错误计入订阅并发 Redis 错误指标。

回归覆盖：

- 长请求超过原 TTL 后仍保持并发占位和运行索引。
- 请求 context 取消后仍能释放租约。
- 单次和整轮 heartbeat 瞬时 Redis 失败后继续续租。
- 释放后下一请求可正常获取并发槽位。

验证命令：

```text
go test ./service -count=1
go test -race ./service -run '^(TestRedisSubscriptionConcurrencyLeaseHeartbeatKeepsLongRequestActive|TestRedisSubscriptionConcurrencyLeaseReleaseIgnoresCanceledRequestContext|TestRedisSubscriptionConcurrencyLeaseHeartbeatRetriesTransientFailure|Test.*SubscriptionConcurrency.*|TestRedisSubscriptionConcurrency.*|TestConcurrencyLease.*)$' -count=1
```

两项均通过。

部署审计：

- GitHub Actions run `31444004669` 成功。
- 镜像 digest：`sha256:45f0ae2bb003a08ffa2beffdea60506b89251db4b24931bf344087b6a7395a09`。
- `/opt/new-api/audits/subscription-concurrency-heartbeat-deploy-20260811T000303Z.json`

## 生产验证

### 已撤回分档期间的 10 分钟窗口

以下 2026-08-10 23:34:32–23:43:57 UTC 数据仅记录临时分档曾产生的效果，不代表当前生产目标：

- 10/10 样本：`new-api` 均为 `running/healthy`。
- 10/10 样本：`/api/status` 均返回 HTTP 200 且 `success=true`。
- `RestartCount` 全程为 0。
- 无 panic、fatal 或数据库错误。
- 该窗口没有新的 502；429 替代了资源耗尽。
- `/v1/responses`：3,392 个 200、46,472 个 429。
- 内部来源 `172.19.0.4`：1,710 个 `/v1/responses` 200、40 个 400、0 个 429/5xx，证明 429 来自外部高并发调用，不是内部回环。
- `new-api` 内存约 360–807 MiB；故障时约 4.2 GiB。
- 用户 3938、3939 运行态均被限制在 20 active + 5 queued。

窗口内 58 个 `/v1/responses` 500 均来自可识别的请求参数或媒体错误（非法 reasoning effort、无效图片、未知参数），不是资源或依赖故障。

### heartbeat 部署后

- revision：`d13efc82f796ca5f78f826f0f96e89d3812a48ae`。
- 容器持续 `running/healthy`，`RestartCount=0`。
- `/api/status` 持续成功。
- 60 秒采样中 12 个同一租约成员的 Redis score 按 30 秒周期前进，直接证明 heartbeat 在生产生效。
- 最近日志无 panic、fatal、heartbeat failure、Redis 并发错误或数据库错误。

## 当前生产状态

2026-08-11 01:58 UTC 完成全表与缓存复核：

- `subscription_plans` 共 11 行；`concurrency_limit <> 0` 为 0 行，`queue_capacity <> 0` 为 0 行。
- 11 个套餐逐行确认均为 `concurrency_limit=0`、`queue_capacity=0`。
- Redis DB 1 中重新生成的套餐缓存也全部为 `0/0`，不存在残留的非零缓存值。
- 清理旧并发运行键并仅重建 `new-api` 后，日志中 `subscription concurrency exceeded` 为 0。
- 连续 3 次验证均为 `running/healthy`、`RestartCount=0`，`/api/status` 均返回 HTTP 200 且 `success=true`。
- 当前 revision 为 `d13efc82f796ca5f78f826f0f96e89d3812a48ae`；heartbeat 不改变无限并发语义。

生产审计：

- `/opt/new-api/audits/subscription-concurrency-unlimited-20260811T015340Z.json`

### heartbeat 镜像回滚

如需单独回滚 heartbeat 镜像，将 `/opt/new-api/audits/compose.release-before-heartbeat-20260811T000303Z.yml` 恢复到 `/opt/new-api/compose.release.yml`，再使用正式四层 Compose 仅重建 `new-api`。该操作与套餐是否无限并发相互独立。

## 防复发措施

1. 将 `concurrency_limit=0` 明确定义并保留为无限并发；不得再把该值误判为缺失配置或事故根因。
2. 未经明确业务授权，不得修改生产套餐的并发或队列值；任何限流方案必须先单独确认产品语义与影响范围。
3. 资源和流量异常应通过请求来源、生命周期、CPU、内存、依赖状态与错误时间线继续定位，不能用套餐限流替代根因分析。
4. 保留长请求租约 heartbeat 回归；它用于保持运行记录有效，不改变无限并发套餐的放行语义。
5. 持续监控 `new-api` 与 `ows-shell` 的 RSS、CPU、detached 请求数量、健康状态和重启事件。
6. 所有生产变更继续保留事务前置条件、缓存失效、自动回滚、revision/health/status 验证与审计文件。
