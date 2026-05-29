# 管理员运维监控面板设计

## 背景

当前项目已经有多块可观测能力，但它们分散在不同页面和接口中：

- `/api/status` 返回站点公开状态与前端配置；
- `/api/status/test` 只做 DB ping 和活跃 HTTP 连接数；
- `/api/performance/stats` 返回内存、GC、goroutine、磁盘缓存、磁盘空间和性能配置，但当前权限是 Root；
- `/api/perf-metrics/summary` 与 `/api/perf-metrics` 返回模型级平均延迟、成功率、TPS；底层 `perf_metrics` 已存储 TTFT 汇总字段，首版运维面板会补齐 `QuerySummaryAll` 的模型级平均 TTFT 输出；
- `/api/log/stat`、`/api/log` 支持日志汇总和明细；
- `/api/channel/*` 能查看渠道状态、测试结果、余额和响应时间；
- `service/subscription_concurrency.go` 已实现订阅分发并发槽位和排队队列。

参考实现 `C:/Users/34404/Documents/GitHub/workbench/repos/sub2api/backend` 里的 Ops Dashboard 将这些信号收敛为独立运维层：`/api/v1/admin/ops/dashboard/snapshot-v2`、`/api/v1/admin/ops/concurrency`、`/api/v1/admin/ops/realtime-traffic`、`/api/v1/admin/ops/account-availability` 等接口，并在前端提供独立运维驾驶舱。

`new-api` 缺少同类管理员站点实时状态监控面板。新增能力应服务于管理员排障和值守，而不是运营分析或普通用户概览。

## 目标

1. 新增管理员运维监控页面 `/admin-ops`，用于统一查看站点实时健康状态。
2. 新增后端只读 API `/api/admin-ops/snapshot`，一次返回首屏核心快照。
3. 新增后端只读 API `/api/admin-ops/concurrency`，返回订阅分发并发槽位和排队队列指标。
4. 将并发槽位、排队队列、队列拒绝和 Redis 并发限制可用性纳入整体健康分级。
5. 复用当前已有系统状态、日志、性能指标、渠道状态和订阅并发实现，不引入不必要的新存储表。
6. 保持接口只读，不做告警配置、自动处置、渠道禁用、GC、缓存清理等写操作。
7. 保持数据库兼容 SQLite、MySQL、PostgreSQL。
8. 前端遵循 `web/default` 现有 React 19、TanStack Router、React Query、Base UI/Tailwind、i18n 约定。

## 非目标

- 不实现 WebSocket 或 SSE 实时推送；第一阶段使用 React Query 轮询。
- 不实现告警规则、告警事件、邮件通知或静默窗口。
- 不实现系统日志索引、日志清理、磁盘缓存清理、强制 GC 等写操作。
- 不采集或展示 API Key、Authorization header、请求体、响应体、prompt、图片 base64 或上游账号密钥。
- 不承诺 p95/p99 延迟；当前 `perf_metrics` 只有聚合平均值。
- 不把 `Log.UseTime` 秒级字段当成精确延迟分位数。
- 不把 Redis 未启用默认判为故障；Redis 是否必要由 `SubscriptionConcurrencyRequireRedis` 和实际并发模式决定。
- 不照搬参考实现里的 OpenAI sticky、hedge、attempt、account slot 归因；当前项目没有同等域模型。

## 方案选择

### 方案 A：扩展现有 Dashboard Overview

在 `/dashboard/overview` 现有管理员可见的 `PerformanceHealthPanel` 上继续增加卡片。

优点：改动少，入口已有。

缺点：`dashboard` 是用户工作台；运维排障信息密度高，包含并发、队列、依赖健康、错误和渠道异常，放在用户概览里会混淆场景。

结论：不采用。

### 方案 B：扩展 Admin Analytics

把运维状态作为 `admin-analytics` 新 tab。

优点：同为管理员页面，有 `useQueries` 和过滤器模式可复用。

缺点：`admin-analytics` 是运营分析，关注订阅、用户、邀请、配额、风险；运维监控是实时排障，刷新频率和指标语义不同。

结论：不采用。

### 方案 C：新增独立 Admin Ops 模块（采用）

新增 `/admin-ops` 页面与 `/api/admin-ops/*` 后端路由组。首版提供系统健康、依赖健康、并发与排队、实时流量、渠道健康、性能摘要、最近错误。

优点：职责清晰；可沿用参考实现的“独立 ops 层”边界；后续可扩展告警、趋势、provider-specific 视角。

缺点：需要新增前后端模块和导航配置。

结论：采用。

## 信息架构

```text
/admin-ops 管理员运维监控
├─ Header：整体健康、刷新时间、自动刷新状态、手动刷新
├─ HealthSummaryCards：系统与依赖健康
│  ├─ DB
│  ├─ Redis
│  ├─ CPU / Memory / Disk
│  └─ Active HTTP Connections / Goroutines
├─ ConcurrencyQueueCard：订阅分发并发与排队
│  ├─ Active slots
│  ├─ Queued requests
│  ├─ Saturated users
│  └─ Queue full / unavailable rejections
├─ RealtimeTrafficCard：最近窗口请求、错误、RPM、TPM
├─ ChannelHealthCard：渠道启用、禁用、自动禁用、慢响应、测试过期
├─ PerformanceModelsCard：模型平均延迟、平均 TTFT、成功率、TPS
└─ RecentErrorsCard：最近错误日志摘要和跳转
```

## 后端 API

### `GET /api/admin-ops/snapshot`

权限：`middleware.AdminAuth()`。

用途：返回运维页面首屏核心快照。

查询参数：

| 参数 | 类型 | 默认 | 说明 |
|---|---|---:|---|
| `window_seconds` | int | `300` | 流量统计窗口，允许 60、300、900、3600；非法值回退 300。 |
| `top` | int | `5` | Top 模型、Top 渠道、最近错误数量，范围 1–20。 |

响应格式：

```json
{
  "success": true,
  "data": {
    "generated_at": 1780000000,
    "health": {
      "status": "degraded",
      "score": 82,
      "reasons": ["concurrency_queue_not_empty"]
    },
    "runtime": {
      "version": "...",
      "start_time": 1779990000,
      "uptime_seconds": 10000,
      "node_name": "...",
      "active_connections": 12,
      "goroutines": 88
    },
    "system": {
      "cpu_usage": 12.3,
      "memory_usage": 61.2,
      "disk_usage": 70.1
    },
    "dependencies": {
      "database": { "enabled": true, "status": "healthy", "latency_ms": 3, "message": "" },
      "redis": { "enabled": true, "status": "healthy", "latency_ms": 2, "message": "" }
    },
    "concurrency": {
      "mode": "redis",
      "enabled": true,
      "summary": {
        "total_active": 18,
        "total_queued": 7,
        "active_users": 6,
        "queued_users": 3,
        "saturated_users": 2,
        "queue_pressure": 0.42
      },
      "config": {
        "ttl_seconds": 600,
        "default_queue_capacity": 10,
        "require_redis": false,
        "fail_open": false
      },
      "counters": {
        "acquired_total": 10293,
        "queued_total": 420,
        "queue_full_rejections_total": 12,
        "unavailable_rejections_total": 0,
        "redis_errors_total": 0
      },
      "users": []
    },
    "traffic": {
      "window_seconds": 300,
      "requests": 1200,
      "errors": 12,
      "rpm": 240,
      "tpm": 90000,
      "error_rate": 0.01
    },
    "channels": {
      "total": 50,
      "enabled": 42,
      "manual_disabled": 3,
      "auto_disabled": 5,
      "slow_count": 2,
      "stale_test_count": 4
    },
    "performance": {
      "models": []
    },
    "recent_errors": []
  }
}
```

### `GET /api/admin-ops/concurrency`

权限：`middleware.AdminAuth()`。

用途：返回订阅并发槽位和排队队列详情。`snapshot` 内嵌同一结构，但独立接口允许更高刷新频率。

查询参数：

| 参数 | 类型 | 默认 | 说明 |
|---|---|---:|---|
| `limit` | int | `20` | 返回热点用户数量，范围 1–100。 |
| `include_users` | bool | `true` | 是否返回并补充 `users` 明细中的 username；summary/health 始终为全部 active/queued 用户补充 limit 和 queue_capacity。 |
| `min_active_or_queued` | int | `1` | 过滤无活跃且无排队用户。 |

响应格式：

```json
{
  "success": true,
  "data": {
    "mode": "redis",
    "generated_at": 1780000000,
    "enabled": true,
    "summary": {
      "total_active": 18,
      "total_queued": 7,
      "active_users": 6,
      "queued_users": 3,
      "saturated_users": 2,
      "queue_pressure": 0.42
    },
    "config": {
      "ttl_seconds": 600,
      "default_queue_capacity": 10,
      "require_redis": false,
      "fail_open": false
    },
    "counters": {
      "acquired_total": 10293,
      "queued_total": 420,
      "queue_full_rejections_total": 12,
      "unavailable_rejections_total": 0,
      "redis_errors_total": 0
    },
    "users": [
      {
        "user_id": 123,
        "username": "alice",
        "active": 5,
        "limit": 5,
        "queued": 3,
        "queue_capacity": 10,
        "oldest_queued_seconds": 14,
        "utilization": 1.0,
        "queue_utilization": 0.3,
        "status": "saturated"
      }
    ]
  }
}
```

## 并发与排队采集设计

当前订阅并发限制只作用于订阅分发计费路径：`service.AcquireSubscriptionConcurrency` 在 `relay/relay.go` 中被调用。

### Redis 模式

当前 Redis key：

```text
subscription:concurrency:user:{userId}
subscription:concurrency:user:{userId}:queue
```

新增轻量索引：

```text
subscription:concurrency:users
```

类型：ZSET。

写入时机：在 `AcquireUserConcurrencyWithQueueCapacity` 的 Redis 成功或排队路径中调用，只记录 userId 与最近观测时间。

查询流程：

1. 清理索引中过期 score：`ZREMRANGEBYSCORE subscription:concurrency:users -inf now-ttl`。
2. 分批遍历索引中的全部有效用户用于计算全量 summary 和 health。`limit` 只裁剪最终 `users` 明细，不能影响 `total_active`、`total_queued`、`queue_pressure` 或健康分级。
3. 对每个索引用户读取：
   - `ZCARD subscription:concurrency:user:{userId}`；
   - `ZCARD subscription:concurrency:user:{userId}:queue`；
   - `ZRANGE subscription:concurrency:user:{userId}:queue 0 0 WITHSCORES` 计算最老排队时长。
4. 全量 runtime rows 只丢弃 active 和 queued 都为 0 的用户；`min_active_or_queued` 只用于最终 `users` 明细过滤，不能影响 summary 或 health。
5. 对全部 active/queued runtime rows 从 DB 批量查询当前运行时并发上限和队列容量：并发上限按 plan 优先、plan 缺失回退订阅实例；队列容量仅在 `plan.QueueCapacity > 0` 时使用 plan 值，否则回退运行时默认 `common.SubscriptionConcurrencyQueueCapacity`，不得回退 0；`include_users=false` 只能省略 username 和明细展示，不能省略 summary 所需的 limit / queue_capacity 查询。
6. 对富化后的全量 rows 计算 summary 和 health，然后按 `min_active_or_queued`、`limit` 裁剪最终 `users` 明细。
7. 按 `active + queued`、`queued`、`active` 降序返回热点用户。
8. 第一阶段不设置 Redis snapshot 扫描上限；如果后续为了保护超大部署增加上限，必须在 DTO 中新增 `partial`/`truncated` 字段，并且不得用样本汇总驱动整体健康。

不使用 Redis 全库 `SCAN` 作为主方案，避免 key 多时成本不可控。

### 内存模式

给 `memorySubscriptionConcurrencyLimiter` 增加只读 snapshot 方法。读取时加锁并复制：

- userId；
- active count；
- queued count；
- oldest queued age。

不暴露内部 map、slice、channel。

### 累计计数器

新增进程内原子计数器：

- `acquired_total`：成功获取槽位次数；
- `queued_total`：进入排队状态次数；
- `queue_full_rejections_total`：队列满或无队列容量导致的拒绝次数；
- `unavailable_rejections_total`：Redis 必需但不可用或限流器不可用导致的拒绝次数；
- `redis_errors_total`：Redis 执行错误次数。

计数位置：

- Redis 脚本返回 allowed：`acquired_total++`；
- Redis 脚本首次把 request_id 加入队列：`queued_total++`；脚本再次发现同一 request_id 已在队列时不能重复计数。
- Redis 脚本返回 rejected：`queue_full_rejections_total++`；
- 内存 limiter 返回 `ErrSubscriptionConcurrencyExceeded`：`queue_full_rejections_total++`；
- `handleSubscriptionConcurrencyRedisError` fail-closed：`redis_errors_total++` 和 `unavailable_rejections_total++`；
- fail-open：`redis_errors_total++`，但不增加 unavailable rejection。

计数器为进程级累计值，首版不宣称跨节点全局累计。

## 健康分级

后端统一计算健康状态，前端只展示。

### Critical

- DB ping 失败。
- `SubscriptionConcurrencyRequireRedis=true` 且 Redis 不可用、并且 fail-closed。
- `unavailable_rejections_total > 0`。
- `queue_full_rejections_total > 0`。

### Degraded

- `total_queued > 0`。
- `saturated_users > 0`。
- `queue_pressure >= 0.5`。
- CPU / Memory / Disk 超过 `common.GetPerformanceMonitorConfig()` 阈值。
- Redis 未启用但订阅并发要求 Redis 未开启：显示 disabled，不降级；如果 require Redis 则 critical。
- 渠道自动禁用数 > 0。
- 最近窗口错误率超过 5%。

### Healthy

无 critical / degraded reason。

健康分数初始规则：从 100 开始扣分；critical reason 每项扣 30，degraded reason 每项扣 10，最低 0。`status` 由最严重 reason 决定。

## 前端设计

### 新增文件

```text
web/default/src/routes/_authenticated/admin-ops/index.tsx
web/default/src/features/admin-ops/index.tsx
web/default/src/features/admin-ops/api.ts
web/default/src/features/admin-ops/types.ts
web/default/src/features/admin-ops/lib/format.ts
web/default/src/features/admin-ops/lib/health.ts
web/default/src/features/admin-ops/components/admin-ops-header.tsx
web/default/src/features/admin-ops/components/health-summary-cards.tsx
web/default/src/features/admin-ops/components/concurrency-queue-card.tsx
web/default/src/features/admin-ops/components/realtime-traffic-card.tsx
web/default/src/features/admin-ops/components/channel-health-card.tsx
web/default/src/features/admin-ops/components/performance-models-card.tsx
web/default/src/features/admin-ops/components/recent-errors-card.tsx
```

### 修改文件

```text
router/api-router.go
controller/admin_ops.go
service/admin_ops.go
service/subscription_concurrency.go
dto/admin_ops.go
model/admin_ops.go
web/default/src/hooks/use-sidebar-data.ts
web/default/src/hooks/use-sidebar-config.ts
web/default/src/features/system-settings/maintenance/config.ts
web/default/src/features/system-settings/maintenance/sidebar-modules-section.tsx
web/default/src/features/profile/components/sidebar-modules-card.tsx
web/default/src/routeTree.gen.ts
pkg/perf_metrics/types.go
pkg/perf_metrics/metrics.go
pkg/perf_metrics/*_test.go
model/perf_metric_test.go
web/default/src/i18n/static-keys.ts
web/default/src/i18n/locales/en.json
web/default/src/i18n/locales/zh.json
web/default/src/i18n/locales/fr.json
web/default/src/i18n/locales/ja.json
web/default/src/i18n/locales/ru.json
web/default/src/i18n/locales/vi.json
```

### 页面刷新

- `snapshot`：30 秒轮询。
- `concurrency`：5 秒轮询。
- 浏览器标签不可见时暂停自动轮询。
- 手动刷新同时 invalidate snapshot 和 concurrency query。

### 导航

在 Admin 分组中加入：

- 标题：`adminOps.title`
- 路由：`/admin-ops`
- 图标：`Activity` 或 `Gauge`

同时更新：

- `DEFAULT_SIDEBAR_MODULES.admin.ops = true`
- `URL_TO_CONFIG_MAP['/admin-ops'] = { section: 'admin', module: 'ops' }`

## 测试策略

必须遵循 TDD。

后端测试：

1. `service/subscription_concurrency_test.go`
   - 内存 limiter snapshot 能返回 active、queued、oldest queued seconds。
   - 队列满拒绝增加计数。
   - Redis fail-closed 增加 redis error 和 unavailable rejection。
2. `service/admin_ops_test.go`
   - 健康分级覆盖 DB failure、queue pressure、queue full rejection、healthy。
   - 并发 summary 正确计算 total active、total queued、saturated users、queue pressure。
3. `controller/admin_ops_test.go`
   - 非 admin 由 middleware 覆盖；controller 测试只验证参数归一化和响应结构。

前端测试：

1. `web/default/src/features/admin-ops/lib/health.test.ts`
   - `healthy/degraded/critical` tone 映射。
   - 并发状态 `normal/queued/saturated/queue_full_risk` 映射。
2. `web/default/src/features/admin-ops/lib/format.test.ts`
   - 百分比、时长、RPM/TPM、字节格式。

验证命令：

```bash
go test ./service ./controller ./model ./pkg/perf_metrics -run 'TestAdminOps|TestParseAdminOps|Test.*SubscriptionConcurrency|TestGetAdminOpsUserConcurrencyLimitsPrefersPlanValues|TestQuerySummaryAllIncludesAvgTtftMs|TestQuerySummaryAllIncludesRedisActiveBucketTtft' -count=1
(cd web/default && bun run build)
(cd web/default && bun run typecheck)
(cd web/default && bunx tsx --test src/features/admin-ops/lib/health.test.ts src/features/admin-ops/lib/format.test.ts src/features/admin-ops/lib/i18n-keys.test.ts src/hooks/use-sidebar-config.test.ts)
```

前端纯函数测试必须使用当前仓库已有的 `node:test` + `node:assert/strict` 风格，并通过 `bunx tsx --test` 执行；`typecheck` 不能替代这些纯函数测试。`bun run build` 用于触发 TanStack Router 生成并更新 `web/default/src/routeTree.gen.ts`。

## 风险与约束

- Redis 索引写入在请求热路径上，必须保持单次轻量命令，失败不得影响请求结果。
- 并发指标在 Redis 模式下可以跨节点统计槽位；进程内累计计数器不是跨节点全局值，UI 必须避免误导文案。
- 内存模式只代表当前进程，不代表多节点全局状态。
- 所有 DB 聚合必须使用 GORM 或跨库 SQL；不能使用 PostgreSQL-only JSONB/window 特性。
- `encoding/json` 只能用于类型引用或底层兼容场景，业务 marshal/unmarshal 必须使用 `common.*` 包装。
- 不修改受保护的 `new-api` / `QuantumNous` 品牌信息。
