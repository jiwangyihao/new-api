# new-api 线上性能快速收益改造计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 在不关闭功能、不改变计费语义、不牺牲审计能力的前提下，消除线上已确认的高成本日志、失败指标写入和订阅结算热路径写放大。

**架构：** 先处理确定性浪费：保留消费日志入库但去掉 stdout 完整 JSON；修复 PostgreSQL `perf_metrics` upsert 失败，使性能指标可以重新开启；将订阅 token 用量更新从整行 `Save` 改为窄字段原子更新，缩短事务和锁持有时间。所有改动都保持现有接口、配置项和业务行为不变。

**技术栈：** Go 1.22+、GORM v2、PostgreSQL/MySQL/SQLite 兼容、Gin、项目内 `common` JSON 封装。

---

## 背景与约束

线上已确认：

- `model/log.go:214` 每次消费日志都会把完整 `RecordConsumeLogParams` 序列化到 stdout，长上下文请求会生成很大的 JSON 日志；但消费日志入库本身必须保留。
- `model/perf_metric.go:39-46` 在 PostgreSQL `ON CONFLICT DO UPDATE` 中使用未限定列名，例如 `generation_ms + ?`，线上报错 `column reference "generation_ms" is ambiguous`；临时已经关闭 `perf_metrics_setting.enabled=false`。
- `model/subscription.go:1531-1565` 的订阅结算会在 `FOR UPDATE` 后 `tx.Save(&sub)`，会更新整行；热路径只需要更新 `token_used` 或 `amount_used`。
- 用户不同意关闭 `DataExportEnabled`，不同意关闭订阅并发队列；这些配置不得作为代码改造方案。

## 文件结构

- 修改：`model/log.go`
  - 保留 `RecordConsumeLog` 入库行为。
  - 将完整 params stdout 改为短摘要日志，避免序列化整个 `params.Other`。
- 修改：`model/log_stat_token_test.go`
  - 增加消费日志不打印 `params=` / 大字段的回归测试。
- 修改：`model/perf_metric.go`
  - 使用跨库安全的 `gorm.Expr("? + ?", clause.Column{Name: ...}, delta)` 生成限定列更新表达式。
- 创建：`model/perf_metric_test.go`
  - 增加 upsert 累加语义测试，覆盖 `generation_ms` 等字段。
- 修改：`model/subscription.go`
  - 将 `postConsumeUserSubscriptionDeltaTx` 的整行 `Save` 改为 `Updates` 窄字段更新。
- 修改：`model/subscription_distributor_test.go`
  - 增加结算后订阅非计数字段不被覆盖的回归测试，覆盖分销 token 订阅路径。

---

## 任务 1：消费日志 stdout 改短摘要，保留入库

**文件：**

- 修改：`model/log.go:210-218`
- 修改：`model/log_stat_token_test.go`

### 目标

消除每次请求的完整 `params` JSON stdout 输出，避免长上下文请求重复序列化 `params.Other` 和写 Docker 日志。保留：

- `logs` 表完整入库；
- `logs.other` JSON；
- `RequestId` / `UpstreamRequestId`；
- data export；
- 审计查询能力。

### 步骤

- [ ] **步骤 1：编写失败测试，证明消费日志不会输出完整 params**

在 `model/log_stat_token_test.go` 追加测试函数。使用 `gin.DefaultWriter` 捕获 stdout，并构造 `Other` 中的大字段；调用 `RecordConsumeLog` 后断言输出不包含 `params=` 和大字段内容，但包含短摘要中的关键字段。

```go
func TestRecordConsumeLogStdoutSummaryDoesNotSerializeFullParams(t *testing.T) {
    resetLogStatTokenTestData(t)

    oldLogConsumeEnabled := common.LogConsumeEnabled
    oldDataExportEnabled := common.DataExportEnabled
    common.LogConsumeEnabled = true
    common.DataExportEnabled = false
    t.Cleanup(func() {
        common.LogConsumeEnabled = oldLogConsumeEnabled
        common.DataExportEnabled = oldDataExportEnabled
    })

    var buf bytes.Buffer
    oldWriter := gin.DefaultWriter
    gin.DefaultWriter = &buf
    t.Cleanup(func() { gin.DefaultWriter = oldWriter })

    ctx := testRecordConsumeLogContext(t, "perf-user")
    RecordConsumeLog(ctx, 4001, RecordConsumeLogParams{
        ChannelId:        7,
        PromptTokens:     123,
        CompletionTokens: 45,
        ModelName:        "gpt-5.5",
        TokenName:        "perf-token",
        Quota:            999,
        TokenId:          9,
        UseTimeSeconds:   3,
        IsStream:         true,
        Group:            "default",
        Other: map[string]interface{}{
            "large_payload": strings.Repeat("x", 8192),
        },
    })

    out := buf.String()
    if strings.Contains(out, "params=") {
        t.Fatalf("consume log stdout should not include full params JSON: %s", out)
    }
    if strings.Contains(out, "large_payload") || strings.Contains(out, strings.Repeat("x", 128)) {
        t.Fatalf("consume log stdout should not include large Other payload: %s", out)
    }
    for _, want := range []string{"record consume log", "userId=4001", "model=gpt-5.5", "quota=999", "prompt=123", "completion=45"} {
        if !strings.Contains(out, want) {
            t.Fatalf("consume log stdout missing %q in %s", want, out)
        }
    }
}
```

如果文件尚未导入 `bytes`、`strings`、`github.com/gin-gonic/gin`，补充 imports。不得引入 `encoding/json`。

- [ ] **步骤 2：运行测试验证失败**

运行：

```bash
go test ./model -run TestRecordConsumeLogStdoutSummaryDoesNotSerializeFullParams -count=1
```

预期：失败，原因是当前输出包含 `params=` 和 `large_payload`。

- [ ] **步骤 3：修改 `RecordConsumeLog` 的 stdout 日志**

将 `model/log.go` 中：

```go
logger.LogInfo(c, fmt.Sprintf("record consume log: userId=%d, params=%s", userId, common.GetJsonString(params)))
```

替换为短摘要：

```go
logger.LogInfo(c, fmt.Sprintf(
    "record consume log: userId=%d, model=%s, tokenId=%d, channelId=%d, quota=%d, prompt=%d, completion=%d, metered=%d, useTime=%d, stream=%t",
    userId,
    params.ModelName,
    params.TokenId,
    params.ChannelId,
    params.Quota,
    params.PromptTokens,
    params.CompletionTokens,
    meteredTokens,
    params.UseTimeSeconds,
    params.IsStream,
))
```

注意：`meteredTokens` 目前在日志行之后计算。实现时要先把：

```go
meteredTokens := normalizedMeteredTokens(params.PromptTokens, params.CompletionTokens, params.Other)
```

移动到短摘要日志之前。不要改变 `otherStr := common.MapToJsonStr(params.Other)` 和 `LOG_DB.Create(log)` 行为。

- [ ] **步骤 4：运行定向测试验证通过**

运行：

```bash
go test ./model -run 'TestRecordConsumeLogStdoutSummaryDoesNotSerializeFullParams|TestRecordConsumeLogStatToken' -count=1
```

预期：通过。

- [ ] **步骤 5：检查无直接 JSON 违规**

确认没有新增 `encoding/json` marshal/unmarshal 调用；业务代码仍使用 `common.*` JSON 封装。

- [ ] **步骤 6：Commit**

```bash
git add model/log.go model/log_stat_token_test.go
git commit -m "fix(log): 精简消费日志标准输出"
```

---

## 任务 2：修复 `perf_metrics` PostgreSQL upsert 歧义

**文件：**

- 修改：`model/perf_metric.go:29-49`
- 创建：`model/perf_metric_test.go`

### 目标

让 `perf_metrics_setting.enabled` 可以重新开启；消除线上 `generation_ms is ambiguous` 错误；保持指标累加语义不变。

### 步骤

- [ ] **步骤 1：编写 upsert 累加测试**

创建 `model/perf_metric_test.go`：

```go
package model

import "testing"

func TestUpsertPerfMetricAccumulatesCounters(t *testing.T) {
    setupTestDB(t)

    first := &PerfMetric{
        ModelName:      "gpt-5.5",
        Group:          "default",
        BucketTs:       1779190000,
        RequestCount:   2,
        SuccessCount:   1,
        TotalLatencyMs: 300,
        TtftSumMs:      50,
        TtftCount:      1,
        OutputTokens:   20,
        GenerationMs:   250,
    }
    second := &PerfMetric{
        ModelName:      "gpt-5.5",
        Group:          "default",
        BucketTs:       1779190000,
        RequestCount:   3,
        SuccessCount:   2,
        TotalLatencyMs: 700,
        TtftSumMs:      90,
        TtftCount:      2,
        OutputTokens:   80,
        GenerationMs:   600,
    }

    if err := UpsertPerfMetric(first); err != nil {
        t.Fatalf("first upsert failed: %v", err)
    }
    if err := UpsertPerfMetric(second); err != nil {
        t.Fatalf("second upsert failed: %v", err)
    }

    var got PerfMetric
    if err := DB.Where("model_name = ? AND \"group\" = ? AND bucket_ts = ?", "gpt-5.5", "default", int64(1779190000)).First(&got).Error; err != nil {
        t.Fatalf("query metric failed: %v", err)
    }

    if got.RequestCount != 5 || got.SuccessCount != 3 || got.TotalLatencyMs != 1000 || got.TtftSumMs != 140 || got.TtftCount != 3 || got.OutputTokens != 100 || got.GenerationMs != 850 {
        t.Fatalf("unexpected accumulated metric: %+v", got)
    }
}
```

如果项目测试辅助函数不是 `setupTestDB(t)`，先在同包测试中查找实际 helper 名称并替换；不要新建并行测试数据库模式。

- [ ] **步骤 2：运行测试验证当前失败或至少覆盖行为**

运行：

```bash
go test ./model -run TestUpsertPerfMetricAccumulatesCounters -count=1
```

在 SQLite 测试库上可能通过，因为线上错误是 PostgreSQL 方言歧义；即使通过，也保留该测试作为累加行为保护。后续需要在 PostgreSQL 集成环境跑一次完整验证。

- [ ] **步骤 3：修改 upsert 表达式，限定被更新列**

在 `model/perf_metric.go` 中增加 helper，避免重复字符串拼接：

```go
func incrementPerfMetricColumn(name string, delta int64) gorm.Expr {
    return gorm.Expr("? + ?", clause.Column{Name: name}, delta)
}
```

将 `DoUpdates` 改为：

```go
DoUpdates: clause.Assignments(map[string]interface{}{
    "request_count":    incrementPerfMetricColumn("request_count", metric.RequestCount),
    "success_count":    incrementPerfMetricColumn("success_count", metric.SuccessCount),
    "total_latency_ms": incrementPerfMetricColumn("total_latency_ms", metric.TotalLatencyMs),
    "ttft_sum_ms":      incrementPerfMetricColumn("ttft_sum_ms", metric.TtftSumMs),
    "ttft_count":       incrementPerfMetricColumn("ttft_count", metric.TtftCount),
    "output_tokens":    incrementPerfMetricColumn("output_tokens", metric.OutputTokens),
    "generation_ms":    incrementPerfMetricColumn("generation_ms", metric.GenerationMs),
}),
```

预期 SQL 语义：`perf_metrics.generation_ms + excluded/given value`，而不是未限定的 `generation_ms + ?`。

- [ ] **步骤 4：运行定向测试**

```bash
go test ./model -run TestUpsertPerfMetricAccumulatesCounters -count=1
```

预期：通过。

- [ ] **步骤 5：在 PostgreSQL 环境验证**

如果本地没有 PostgreSQL 测试环境，使用线上前必须在 staging 或临时 PostgreSQL 上执行一次：

```sql
insert into perf_metrics (model_name, "group", bucket_ts, request_count, success_count, total_latency_ms, ttft_sum_ms, ttft_count, output_tokens, generation_ms)
values ('probe-model', 'default', 1779190000, 1, 1, 10, 2, 1, 5, 8)
on conflict (model_name, "group", bucket_ts) do update set generation_ms = perf_metrics.generation_ms + 8;
```

或部署后重新开启 `perf_metrics_setting.enabled=true` 并观察 10 分钟，确认不再出现：

```text
column reference "generation_ms" is ambiguous
failed to flush perf metric bucket
```

- [ ] **步骤 6：Commit**

```bash
git add model/perf_metric.go model/perf_metric_test.go
git commit -m "fix(perf): 修复指标 upsert 列歧义"
```

---

## 任务 3：订阅用量结算改为窄字段更新

**文件：**

- 修改：`model/subscription.go:1531-1565`
- 修改：`model/subscription_distributor_test.go`

### 目标

保持订阅计费完全一致，但减少热路径写放大：`postConsumeUserSubscriptionDeltaTx` 只更新实际变化字段和 `updated_at`，不再 `Save(&sub)` 写整行。

### 步骤

- [ ] **步骤 1：编写失败测试，防止结算覆盖非计数字段**

在 `model/subscription_distributor_test.go` 添加测试。测试思路：创建一个分销 token 订阅，设置 `token_used=10`、`token_limit=0`、`status=active`、`grant_reason` 等字段；调用 `PostConsumeUserSubscriptionDelta(sub.Id, 7)`；断言 `token_used=17`，并且 `status`、`grant_reason`、`source`、`token_limit` 等字段不变。

示例代码骨架：

```go
func TestPostConsumeUserSubscriptionDeltaOnlyChangesTokenUsed(t *testing.T) {
    setupSubscriptionDistributorTestDB(t)

    plan := SubscriptionPlan{
        Title:             "distributor plan",
        Type:              SubscriptionPlanTypeTokens,
        Status:            "active",
        TokenLimit:        0,
        DistributorEnabled: true,
    }
    if err := DB.Create(&plan).Error; err != nil {
        t.Fatalf("create plan failed: %v", err)
    }

    sub := UserSubscription{
        UserId:      1001,
        PlanId:      plan.Id,
        Status:      "active",
        TokenLimit:  0,
        TokenUsed:   10,
        GrantReason: "trial_code",
        Source:      "trial_code",
        StartTime:   1779190000,
        EndTime:     1779276400,
    }
    if err := DB.Create(&sub).Error; err != nil {
        t.Fatalf("create subscription failed: %v", err)
    }

    if err := PostConsumeUserSubscriptionDelta(sub.Id, 7); err != nil {
        t.Fatalf("post consume failed: %v", err)
    }

    var got UserSubscription
    if err := DB.First(&got, sub.Id).Error; err != nil {
        t.Fatalf("query subscription failed: %v", err)
    }
    if got.TokenUsed != 17 {
        t.Fatalf("token_used = %d, want 17", got.TokenUsed)
    }
    if got.Status != sub.Status || got.GrantReason != sub.GrantReason || got.Source != sub.Source || got.TokenLimit != sub.TokenLimit {
        t.Fatalf("non-counter fields changed: got=%+v want status=%s grant=%s source=%s limit=%d", got, sub.Status, sub.GrantReason, sub.Source, sub.TokenLimit)
    }
}
```

如果现有测试 helper 或字段名不同，以当前文件已有 fixture 为准；不要引入新表结构。

- [ ] **步骤 2：运行测试验证当前行为**

```bash
go test ./model -run TestPostConsumeUserSubscriptionDeltaOnlyChangesTokenUsed -count=1
```

当前代码可能通过，因为 `Save` 不一定覆盖错误字段；该测试用于保护改造后语义不变。

- [ ] **步骤 3：将整行 Save 改为窄字段 Updates**

在 `postConsumeUserSubscriptionDeltaTx` 中，分销订阅分支：

```go
sub.TokenUsed = newUsed
return tx.Save(&sub).Error
```

替换为：

```go
return tx.Model(&UserSubscription{}).
    Where("id = ?", userSubscriptionId).
    Updates(map[string]interface{}{
        "token_used": newUsed,
        "updated_at": GetDBTimestamp(),
    }).Error
```

非分销订阅分支：

```go
sub.AmountUsed = newUsed
return tx.Save(&sub).Error
```

替换为：

```go
return tx.Model(&UserSubscription{}).
    Where("id = ?", userSubscriptionId).
    Updates(map[string]interface{}{
        "amount_used": newUsed,
        "updated_at": GetDBTimestamp(),
    }).Error
```

保持前面的 `FOR UPDATE` 查询、限额校验和 plan 判断不变。不要改计费公式，不要改并发队列。

- [ ] **步骤 4：运行订阅相关定向测试**

```bash
go test ./model -run 'TestPostConsumeUserSubscriptionDeltaOnlyChangesTokenUsed|TestSubscription' -count=1
```

再运行服务层订阅计费测试：

```bash
go test ./service -run 'TestSubscription.*Billing|Test.*Subscription' -count=1
```

预期：通过。

- [ ] **步骤 5：Commit**

```bash
git add model/subscription.go model/subscription_distributor_test.go
git commit -m "refactor(subscription): 缩小用量结算更新范围"
```

---

## 任务 4：集成验证与部署前检查

**文件：**

- 不新增文件。
- 检查前三个任务修改文件。

### 步骤

- [ ] **步骤 1：运行 Go 定向测试**

```bash
go test ./model -run 'TestRecordConsumeLogStdoutSummaryDoesNotSerializeFullParams|TestRecordConsumeLogStatToken|TestUpsertPerfMetricAccumulatesCounters|TestPostConsumeUserSubscriptionDeltaOnlyChangesTokenUsed|TestSubscription' -count=1
```

预期：通过。

- [ ] **步骤 2：运行服务层订阅计费测试**

```bash
go test ./service -run 'TestSubscription.*Billing|Test.*Subscription' -count=1
```

预期：通过。

- [ ] **步骤 3：运行 relay 订阅计费定向测试**

```bash
go test ./relay -run 'Test.*Subscription.*Billing' -count=1
```

预期：通过。

- [ ] **步骤 4：运行格式化**

```bash
gofmt -w model/log.go model/log_stat_token_test.go model/perf_metric.go model/perf_metric_test.go model/subscription.go model/subscription_distributor_test.go
```

- [ ] **步骤 5：最终 diff 检查**

确认只包含：

- 消费日志 stdout 短摘要；
- perf metrics upsert 修复；
- 订阅结算窄字段更新；
- 对应测试。

不得包含：

- 关闭 `DataExportEnabled`；
- 修改订阅并发队列；
- 关闭 `LogConsumeEnabled`；
- 任何 `sub2api` 内容；
- 受保护项名称/品牌修改。

- [ ] **步骤 6：部署后验证**

部署到线上后执行：

```bash
docker logs --since 10m new-api
```

确认：

- 不再出现 `record consume log: ... params={`；
- 消费日志仍然入库；
- `DataExportEnabled` 仍工作；
- 未出现 `generation_ms is ambiguous`。

如果重新开启 `perf_metrics_setting.enabled=true`，至少观察一个 flush 周期（默认 5 分钟）再确认没有 `failed to flush perf metric bucket`。

- [ ] **步骤 7：Commit 汇总（如前三个任务未单独 commit）**

```bash
git add model/log.go model/log_stat_token_test.go model/perf_metric.go model/perf_metric_test.go model/subscription.go model/subscription_distributor_test.go
git commit -m "fix(perf): 降低请求热路径日志和数据库开销"
```

---

## 暂不纳入本轮的事项

- 不关闭 `DataExportEnabled`。
- 不关闭或缩短订阅并发队列。
- 不关闭 `LogConsumeEnabled`。
- 不改 Redis 拆分与任何 `sub2api` 相关内容。
- 不做消费日志异步队列化；这是更大改造，需要单独设计失败补偿、进程退出 flush 和背压策略。
- 不做 `quota_data` upsert 重构；收益明确但涉及迁移/唯一索引和三库兼容，放入下一轮。
- 不做 PostgreSQL 参数调优；本轮只改代码路径。
