# 2026-08-10 Responses 流式并发过载事故报告

## 结论

本次故障不是 `new-api` 后端发布回归，也不是 Nginx、PostgreSQL、Redis 或 HTTP 回环重试造成的流量放大。

直接根因是生产环境 11 个订阅套餐的 `concurrency_limit` 与 `queue_capacity` 均为 `0`，而运行时将 `concurrency_limit <= 0` 解释为无限并发。两个高流量用户在 15 分钟内分别发起约 2,900 个以流式为主的 `/v1/responses` 请求，绕过了订阅并发保护；同机 `ows-shell` 长时间持有大量 detached semantic-budget 请求并承受持续 CPU、内存压力，最终触发 `new-api` 超时与 `ows-shell` 重启，重启窗口对应 Nginx 502 突发。

另发现一个独立的长期流请求缺陷：Redis 活跃租约默认 TTL 为 600 秒，原实现只在获取租约时写入一次；超过 TTL 的请求会被后续获取脚本清除，从而绕过并发占位。该缺陷由提交 `d13efc82f` 修复并已部署。

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

### 配置根因

故障时全部生产套餐均为：

```text
concurrency_limit=0
queue_capacity=0
```

运行时明确将 `concurrency_limit <= 0` 当作无限并发，因此该值没有提供保护。

## 应急修复

2026-08-10 23:29 UTC 事务化应用以下分档：

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

### 套餐分档后的 10 分钟窗口

2026-08-10 23:34:32–23:43:57 UTC：

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

## 回滚

### 套餐分档

执行：

```text
/opt/new-api/audits/emergency-concurrency-20260810T232859Z-rollback.sql
```

随后失效两类套餐缓存，并使用正式四层 Compose 仅重建 `new-api`。

注意：回滚套餐为 `0/0` 会重新启用无限并发，仅用于紧急诊断，不应作为常规回滚目标。

### heartbeat 镜像

恢复：

```text
/opt/new-api/audits/compose.release-before-heartbeat-20260811T000303Z.yml
```

到 `/opt/new-api/compose.release.yml`，然后使用正式四层 Compose 仅重建 `new-api`。

## 防复发措施

1. 套餐创建/编辑流程必须显式配置 `concurrency_limit` 与 `queue_capacity`；生产套餐禁止无意设置为 `0/0`。
2. 发布和日常巡检加入订阅套餐并发值检查，发现公开或生效套餐为无限并发时告警。
3. 监控订阅并发 acquired、queued、queue-full rejection、Redis error，以及每用户 active/queued 运行快照。
4. 保留长请求租约 heartbeat 回归，防止 TTL 清理再次绕过并发占位。
5. 对 429 日志实施采样或聚合，避免高并发攻击下错误日志本身造成额外 I/O 压力；不能通过移除 429 保护来消除日志量。
6. 持续监控 `ows-shell` detached 请求数量、RSS、CPU 与重启；其高基线资源使用仍需独立治理，但不再通过无限并发直接压垮 `new-api`。
7. 保持生产变更的事务前置条件、自动回滚、revision/health/status 三重验证及审计文件。
