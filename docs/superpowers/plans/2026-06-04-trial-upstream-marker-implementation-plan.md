# 试用套餐上游标记实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 让实际使用试用套餐的上游主请求稳定携带 `X-NewAPI-Subscription-Marker: trial`，并确保客户端、通道配置或 runtime header override 不能伪造该标记。

**架构：** 运行时只复用 `SubscriptionPlan.is_trial`，不新增数据库字段和前端配置。订阅预扣结果把 `plan != nil && plan.IsTrial` 传到 `RelayInfo.SubscriptionTrialMarker`；所有上游主请求在最终发送前先清理同名 Header，再仅当 marker 为 `trial` 时写入固定 Header。

**技术栈：** Go、Gin、GORM、AWS SDK v2、gorilla/websocket、Go 单元测试。

---

## 规格与约束

- 规格文件：`docs/superpowers/specs/2026-06-04-trial-upstream-marker-design.md`。
- 不新增数据库迁移，不新增前端页面、表单、i18n 或配置项。
- Header 固定为 `X-NewAPI-Subscription-Marker: trial`。
- Header 名称清理必须按 HTTP Header 大小写不敏感语义处理。
- 非试用请求最终不得携带该 Header，即使客户端、通道 `header_override`、runtime header override 或 `param_override` 尝试设置。
- 试用请求最终必须携带固定值 `trial`，即使客户端或通道尝试写入其他值、空值或删除。
- 不发送用户 ID、订阅 ID、套餐 ID、套餐标题、`business_code` 等信息。
- 保留并适配现有未提交改动；不要恢复或覆盖其他开发者留下的修改。

## 文件结构

- 修改：`model/subscription.go`
  - 在 `SubscriptionPreConsumeResult` 增加 `PlanIsTrial bool`。
  - 在 `fillSubscriptionPreConsumeResult` 使用 `plan != nil && plan.IsTrial` 填充。
- 修改：`model/subscription_distributor_test.go`
  - 增加预扣结果试用标记测试，覆盖正常预扣与幂等重复请求。
- 修改：`service/funding_source.go`
  - 在 `SubscriptionFunding` 增加 `PlanIsTrial bool`，并从预扣结果复制。
- 修改：`service/billing_session.go`
  - 在 `syncRelayInfo` 中同步 `RelayInfo.SubscriptionTrialMarker`。
  - 在 `clearRelayBillingState` 中清空 marker。
- 修改：`service/subscription_billing_test.go`
  - 增加 billing session 到 `RelayInfo` 的 marker 同步测试，以及清空状态测试。
- 修改：`relay/common/relay_info.go`
  - 新增 `SubscriptionTrialMarker string` 字段。
- 修改：`relay/channel/api_request.go`
  - 增加 Header 常量与最终化函数。
  - 在 `doRequest` 发送前最终化 HTTP Header。
  - 在 `DoWssRequest` Dial 前最终化 WebSocket Header。
  - 暴露可被 provider 自有 WebSocket / AWS 路径复用的函数。
- 修改：`relay/channel/api_request_test.go`
  - 增加 Header 最终化单元测试，覆盖试用、非试用、客户端/通道/runtime 伪造、大小写变体和 WebSocket Header。
- 修改：`relay/channel/aws/relay-aws.go`
  - 在 AWS SDK 主请求发送前注入同一 Header，建议用 AWS SDK APIOptions middleware 或等效 HTTP client wrapper。
- 修改：`relay/channel/aws/relay_aws_test.go`
  - 增加 AWS SDK request Header 注入 / 清洗测试，至少覆盖非流式 `InvokeModelInput` 构建路径；若使用 HTTP client wrapper，则用 fake HTTP client 捕获 request。
- 修改：`relay/channel/xunfei/relay-xunfei.go`
  - 让 Xunfei provider 自有 WebSocket Dial 使用最终化 Header。
- 修改：`relay/channel/volcengine/tts.go`
  - 让 Volcengine TTS streaming 自有 WebSocket Dial 使用最终化 Header。
- 可选创建：`relay/channel/xunfei/relay_xunfei_test.go`、`relay/channel/volcengine/tts_test.go`
  - 如现有包内无合适测试文件，可新增只测 Header 构造 helper 的单元测试。

---

## 任务 1：传播试用套餐 marker 到 RelayInfo

**文件：**
- 修改：`model/subscription.go`
- 修改：`model/subscription_distributor_test.go`
- 修改：`service/funding_source.go`
- 修改：`service/billing_session.go`
- 修改：`service/subscription_billing_test.go`
- 修改：`relay/common/relay_info.go`

- [ ] **步骤 1：编写失败的模型测试**

在 `model/subscription_distributor_test.go` 增加测试，放在 `TestPreConsumeUserSubscriptionByUnitsReturnsPlanMetadata` 附近：

```go
func TestPreConsumeUserSubscriptionByUnitsReturnsPlanTrialMarker(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.Create(&User{Id: 7651, Username: "trial_marker_user", Status: common.UserStatusEnabled, AffCode: "aff7651"}).Error)
	ensureSubscriptionPreConsumeRecordTableForTest(t)
	trialCode := "trial-marker"
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: 7652, Title: "Trial Marker", Enabled: true, IsTrial: true, BusinessCode: &trialCode}).Error)
	seedUserSubscriptionForDistributorTest(t, 7653, 7651, 7652, 0, 0, 0, "trial_code")

	pre, err := PreConsumeUserSubscriptionByUnits("trial-marker-ok", 7651, "gpt-4o", 0, 0, 10)
	require.NoError(t, err)
	assert.True(t, pre.PlanIsTrial)

	repeat, err := PreConsumeUserSubscriptionByUnits("trial-marker-ok", 7651, "gpt-4o", 0, 0, 10)
	require.NoError(t, err)
	assert.True(t, repeat.PlanIsTrial)
}
```

- [ ] **步骤 2：运行模型测试验证失败**

运行：

```bash
go test -p 1 ./model -run 'PreConsumeUserSubscriptionByUnitsReturnsPlanTrialMarker' -count=1
```

预期：编译失败或测试失败，原因是 `SubscriptionPreConsumeResult` 尚无 `PlanIsTrial`。

- [ ] **步骤 3：实现模型字段传播**

在 `model/subscription.go` 的 `SubscriptionPreConsumeResult` 增加：

```go
PlanIsTrial bool
```

在 `fillSubscriptionPreConsumeResult` 中，在已有 `PlanId` / `PlanTitle` 赋值附近加入：

```go
result.PlanIsTrial = plan != nil && plan.IsTrial
```

不要用 `grant_reason`、`source`、`business_code` 或标题推导。

- [ ] **步骤 4：运行模型测试验证通过**

运行：

```bash
go test -p 1 ./model -run 'PreConsumeUserSubscriptionByUnitsReturnsPlanTrialMarker' -count=1
```

预期：PASS。

- [ ] **步骤 5：编写失败的 service 传播测试**

在 `service/subscription_billing_test.go` 增加测试，使用现有 `seedUser`、`seedToken`、`newBillingTestContext`、`newBillingTestRelayInfo` helpers：

```go
func TestPreConsumeBillingSyncsSubscriptionTrialMarker(t *testing.T) {
	truncate(t)
	const userID = 9061
	const tokenID = 9062
	const planID = 9063
	const subID = 9064
	seedUser(t, userID, 10_000)
	seedToken(t, tokenID, userID, "sk-trial-marker", 10_000)
	ensureSubscriptionBillingTables(t)
	trialCode := "trial-marker-billing"
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: planID, Title: "Trial Marker", Enabled: true, IsTrial: true, BusinessCode: &trialCode}).Error)
	require.NoError(t, model.DB.Create(&model.UserSubscription{Id: subID, UserId: userID, PlanId: planID, AmountTotal: 1, TokenLimit: 0, TokenUsed: 0, Status: "active", GrantReason: "trial_code", StartTime: time.Now().Unix(), EndTime: time.Now().Add(24 * time.Hour).Unix()}).Error)

	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-trial-marker", "req-trial-marker", "subscription_only")

	apiErr := PreConsumeBilling(ctx, 6, relayInfo)
	require.Nil(t, apiErr)
	assert.Equal(t, "trial", relayInfo.SubscriptionTrialMarker)
}

func TestClearRelayBillingStateClearsSubscriptionTrialMarker(t *testing.T) {
	info := &relaycommon.RelayInfo{SubscriptionTrialMarker: "trial"}

	clearRelayBillingState(info)

	assert.Empty(t, info.SubscriptionTrialMarker)
}
```

- [ ] **步骤 6：运行 service 测试验证失败**

运行：

```bash
go test -p 1 ./service -run 'PreConsumeBillingSyncsSubscriptionTrialMarker|ClearRelayBillingStateClearsSubscriptionTrialMarker' -count=1
```

预期：编译失败或测试失败，原因是 `RelayInfo.SubscriptionTrialMarker` / `SubscriptionFunding.PlanIsTrial` 尚未实现。

- [ ] **步骤 7：实现 service 与 RelayInfo 字段传播**

在 `relay/common/relay_info.go` 的订阅字段附近增加：

```go
// SubscriptionTrialMarker is set to "trial" when the actual billed subscription plan is trial.
SubscriptionTrialMarker string
```

在 `service/funding_source.go` 的 `SubscriptionFunding` 增加：

```go
PlanIsTrial bool
```

在 `SubscriptionFunding.PreConsume` 中复制：

```go
s.PlanIsTrial = res.PlanIsTrial
```

在 `service/billing_session.go` 的 `syncRelayInfo` 订阅分支设置：

```go
if sub.PlanIsTrial {
	info.SubscriptionTrialMarker = "trial"
} else {
	info.SubscriptionTrialMarker = ""
}
```

在非订阅分支和 `clearRelayBillingState` 中清空：

```go
info.SubscriptionTrialMarker = ""
```

- [ ] **步骤 8：运行任务 1 相关测试验证通过**

运行：

```bash
go test -p 1 ./model -run 'PreConsumeUserSubscriptionByUnitsReturnsPlanTrialMarker|PreConsumeUserSubscriptionByUnitsReturnsPlanMetadata' -count=1
go test -p 1 ./service -run 'PreConsumeBillingSyncsSubscriptionTrialMarker|ClearRelayBillingStateClearsSubscriptionTrialMarker' -count=1
```

预期：全部 PASS。

- [ ] **步骤 9：提交任务 1**

只提交本任务相关文件：

```bash
git add -- model/subscription.go model/subscription_distributor_test.go service/funding_source.go service/billing_session.go service/subscription_billing_test.go relay/common/relay_info.go
git commit -m "feat(subscription): 传播试用套餐上游标记"
```

---

## 任务 2：实现 HTTP / 通用 WebSocket Header 最终化

**文件：**
- 修改：`relay/channel/api_request.go`
- 修改：`relay/channel/api_request_test.go`

- [ ] **步骤 1：编写失败的 Header 最终化测试**

在 `relay/channel/api_request_test.go` 增加测试：

```go
func TestFinalizeSubscriptionMarkerHeaderSetsTrialMarker(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "https://example.com/v1/chat/completions", nil)
	req.Header.Set("x-newapi-subscription-marker", "spoofed")

	FinalizeSubscriptionMarkerHeader(req.Header, &relaycommon.RelayInfo{SubscriptionTrialMarker: "trial"})

	require.Equal(t, "trial", req.Header.Get(SubscriptionMarkerHeaderName))
}

func TestFinalizeSubscriptionMarkerHeaderRemovesSpoofedNonTrialMarker(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "https://example.com/v1/chat/completions", nil)
	req.Header.Set("x-newapi-subscription-marker", "trial")
	req.Header.Set("X-NewAPI-Subscription-Marker", "trial")

	FinalizeSubscriptionMarkerHeader(req.Header, &relaycommon.RelayInfo{})

	require.Empty(t, req.Header.Get(SubscriptionMarkerHeaderName))
	for key := range req.Header {
		require.NotEqual(t, strings.ToLower(SubscriptionMarkerHeaderName), strings.ToLower(key))
	}
}

func TestFinalizeSubscriptionMarkerHeaderIgnoresNonTrialMarkerValue(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "https://example.com/v1/chat/completions", nil)

	FinalizeSubscriptionMarkerHeader(req.Header, &relaycommon.RelayInfo{SubscriptionTrialMarker: "paid"})

	require.Empty(t, req.Header.Get(SubscriptionMarkerHeaderName))
}
```

需要为测试添加 `strings` import。

- [ ] **步骤 2：运行 Header 最终化测试验证失败**

运行：

```bash
go test -p 1 ./relay/channel -run 'FinalizeSubscriptionMarkerHeader' -count=1
```

预期：编译失败，原因是常量或函数尚未实现。

- [ ] **步骤 3：实现 Header 最终化函数**

在 `relay/channel/api_request.go` 增加常量与函数，放在 header override 常量附近：

```go
const (
	SubscriptionMarkerHeaderName  = "X-NewAPI-Subscription-Marker"
	SubscriptionTrialMarkerValue = "trial"
)

func FinalizeSubscriptionMarkerHeader(headers http.Header, info *common.RelayInfo) {
	if headers == nil {
		return
	}
	for key := range headers {
		if strings.EqualFold(key, SubscriptionMarkerHeaderName) {
			delete(headers, key)
		}
	}
	if info != nil && info.SubscriptionTrialMarker == SubscriptionTrialMarkerValue {
		headers.Set(SubscriptionMarkerHeaderName, SubscriptionTrialMarkerValue)
	}
}
```

注意：函数复用 `strings` 和 `http`，`api_request.go` 已经有这些 import。

- [ ] **步骤 4：运行 Header 最终化测试验证通过**

运行：

```bash
go test -p 1 ./relay/channel -run 'FinalizeSubscriptionMarkerHeader' -count=1
```

预期：PASS。

- [ ] **步骤 5：编写失败的 override 防伪测试**

在 `api_request_test.go` 增加测试，验证 runtime/header passthrough 后最终化仍清理：

```go
func TestFinalizeSubscriptionMarkerHeaderRemovesRuntimeOverrideSpoof(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Request.Header.Set("X-NewAPI-Subscription-Marker", "trial")

	info := &relaycommon.RelayInfo{
		IsChannelTest: false,
		ChannelMeta: &relaycommon.ChannelMeta{
			HeadersOverride: map[string]any{
				"*": "",
				"X-NewAPI-Subscription-Marker": "trial",
			},
		},
	}

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	upstreamReq := httptest.NewRequest(http.MethodPost, "https://example.com/v1/chat/completions", nil)
	applyHeaderOverrideToRequest(upstreamReq, headers)
	FinalizeSubscriptionMarkerHeader(upstreamReq.Header, info)

	require.Empty(t, upstreamReq.Header.Get(SubscriptionMarkerHeaderName))
}

func TestFinalizeSubscriptionMarkerHeaderKeepsTrialAfterOverrideDeletes(t *testing.T) {
	upstreamReq := httptest.NewRequest(http.MethodPost, "https://example.com/v1/chat/completions", nil)
	upstreamReq.Header.Set(SubscriptionMarkerHeaderName, "wrong")

	FinalizeSubscriptionMarkerHeader(upstreamReq.Header, &relaycommon.RelayInfo{SubscriptionTrialMarker: "trial"})

	require.Equal(t, "trial", upstreamReq.Header.Get(SubscriptionMarkerHeaderName))
}
```

- [ ] **步骤 6：运行 override 防伪测试验证通过**

运行：

```bash
go test -p 1 ./relay/channel -run 'FinalizeSubscriptionMarkerHeader|RemovesRuntimeOverrideSpoof|KeepsTrialAfterOverrideDeletes' -count=1
```

预期：PASS。

- [ ] **步骤 7：集成 HTTP 与 `DoWssRequest` 最终化**

在 `doRequest(c, req, info)` 的 `client.Do(req)` 前调用：

```go
FinalizeSubscriptionMarkerHeader(req.Header, info)
```

在 `DoWssRequest` 中，`websocket.DefaultDialer.Dial(fullRequestURL, targetHeader)` 前调用：

```go
FinalizeSubscriptionMarkerHeader(targetHeader, info)
```

- [ ] **步骤 8：运行 relay/channel 相关测试**

运行：

```bash
go test -p 1 ./relay/channel -run 'HeaderOverride|FinalizeSubscriptionMarkerHeader' -count=1
```

预期：PASS。

- [ ] **步骤 9：提交任务 2**

```bash
git add -- relay/channel/api_request.go relay/channel/api_request_test.go
git commit -m "feat(relay): 注入试用套餐上游标记"
```

---

## 任务 3：覆盖 AWS 与 provider 自有 WebSocket 主请求

**文件：**
- 修改：`relay/channel/aws/relay-aws.go`
- 修改：`relay/channel/aws/relay_aws_test.go`
- 修改：`relay/channel/xunfei/relay-xunfei.go`
- 修改或创建：`relay/channel/xunfei/relay_xunfei_test.go`
- 修改：`relay/channel/volcengine/tts.go`
- 修改或创建：`relay/channel/volcengine/tts_test.go`

- [ ] **步骤 1：编写失败的 Xunfei / Volcengine Header helper 测试**

如果直接测试 Dial 太重，先引入小 helper 并测试 helper。测试期望代码可以是：

`relay/channel/xunfei/relay_xunfei_test.go`：

```go
func TestBuildXunfeiDialHeaderFinalizesSubscriptionMarker(t *testing.T) {
	header := buildXunfeiDialHeader(&relaycommon.RelayInfo{SubscriptionTrialMarker: "trial"})
	require.Equal(t, "trial", header.Get(channel.SubscriptionMarkerHeaderName))
}

func TestBuildXunfeiDialHeaderRemovesNonTrialSpoof(t *testing.T) {
	header := buildXunfeiDialHeader(&relaycommon.RelayInfo{})
	require.Empty(t, header.Get(channel.SubscriptionMarkerHeaderName))
}
```

`relay/channel/volcengine/tts_test.go`：

```go
func TestBuildVolcengineTTSDialHeaderFinalizesSubscriptionMarker(t *testing.T) {
	header := buildVolcengineTTSDialHeader("token", &relaycommon.RelayInfo{SubscriptionTrialMarker: "trial"})
	require.Equal(t, "Bearer;token", header.Get("Authorization"))
	require.Equal(t, "trial", header.Get(channel.SubscriptionMarkerHeaderName))
}

func TestBuildVolcengineTTSDialHeaderRemovesNonTrialSpoof(t *testing.T) {
	header := buildVolcengineTTSDialHeader("token", &relaycommon.RelayInfo{})
	require.Empty(t, header.Get(channel.SubscriptionMarkerHeaderName))
}
```

- [ ] **步骤 2：运行 provider helper 测试验证失败**

运行：

```bash
go test -p 1 ./relay/channel/xunfei ./relay/channel/volcengine -run 'SubscriptionMarker|DialHeader' -count=1
```

预期：编译失败，helper 尚未实现。

- [ ] **步骤 3：实现 Xunfei / Volcengine WebSocket Header helper**

在 `relay/channel/xunfei/relay-xunfei.go`：

1. 引入 `net/http`、`github.com/QuantumNous/new-api/relay/channel` 和 `relaycommon`（如尚未存在）。
2. 新增：

```go
func buildXunfeiDialHeader(info *relaycommon.RelayInfo) http.Header {
	header := http.Header{}
	channel.FinalizeSubscriptionMarkerHeader(header, info)
	return header
}
```

3. 将 `xunfeiMakeRequest` 签名改为接收 `info *relaycommon.RelayInfo`，并在 `d.Dial(authUrl, buildXunfeiDialHeader(info))` 使用该 Header。
4. 更新 `xunfeiStreamHandler` / `xunfeiHandler` 签名与调用，把 `info` 传入。
5. 更新 `Adaptor.DoResponse` 调用。

在 `relay/channel/volcengine/tts.go`：

1. 引入 `github.com/QuantumNous/new-api/relay/channel`。
2. 新增：

```go
func buildVolcengineTTSDialHeader(token string, info *relaycommon.RelayInfo) http.Header {
	header := http.Header{}
	header.Set("Authorization", fmt.Sprintf("Bearer;%s", token))
	channel.FinalizeSubscriptionMarkerHeader(header, info)
	return header
}
```

3. 在 `handleTTSWebSocketResponse` 中用 helper 替代手写 Header。

- [ ] **步骤 4：运行 provider helper 测试验证通过**

运行：

```bash
go test -p 1 ./relay/channel/xunfei ./relay/channel/volcengine -run 'SubscriptionMarker|DialHeader' -count=1
```

预期：PASS。

- [ ] **步骤 5：编写失败的 AWS Header 注入测试**

在 `relay/channel/aws/relay_aws_test.go` 增加一个 fake HTTP client 捕获 AWS SDK 真实发送前的 request。若实现选择 middleware，也可以测试 middleware 对 smithy request 的效果。推荐测试结构：

```go
type captureHTTPClient struct {
	lastRequest *http.Request
}

func (c *captureHTTPClient) Do(req *http.Request) (*http.Response, error) {
	c.lastRequest = req
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"body":"{}"}`)),
		Request:    req,
	}, nil
}
```

如果 AWS response mock 太复杂，可以只测试 helper/middleware 级别，要求 helper 在 SDK options 上设置 APIOptions，并通过 smithy stack 执行 Build/Finalize middleware。验收必须能证明 AWS 主请求最终 HTTP Header 会包含或清理 marker。

- [ ] **步骤 6：运行 AWS 测试验证失败**

运行：

```bash
go test -p 1 ./relay/channel/aws -run 'SubscriptionMarker|AwsClientRequest' -count=1
```

预期：失败，AWS marker middleware 尚未实现。

- [ ] **步骤 7：实现 AWS SDK 主请求 marker 注入**

在 `relay/channel/aws/relay-aws.go` 中新增 helper，优先使用 AWS SDK `Options.APIOptions`，在 SDK 实际发送 HTTP 请求前对 smithy HTTP request Header 调用 `channel.FinalizeSubscriptionMarkerHeader`。

实现方向：

```go
func addSubscriptionMarkerMiddleware(info *relaycommon.RelayInfo) func(*bedrockruntime.Options) {
	return func(o *bedrockruntime.Options) {
		o.APIOptions = append(o.APIOptions, func(stack *middleware.Stack) error {
			return stack.Finalize.Add(middleware.FinalizeMiddlewareFunc("newApiSubscriptionMarker", func(ctx context.Context, in middleware.FinalizeInput, next middleware.FinalizeHandler) (out middleware.FinalizeOutput, metadata middleware.Metadata, err error) {
				if req, ok := in.Request.(*smithyhttp.Request); ok && req != nil {
					channel.FinalizeSubscriptionMarkerHeader(req.Header, info)
				}
				return next.HandleFinalize(ctx, in)
			}), middleware.Before)
		})
	}
}
```

需要按实际 AWS SDK 类型调整 import：`github.com/aws/smithy-go/middleware` 与 `github.com/aws/smithy-go/transport/http`。

在 `newAwsClient` 创建 `bedrockruntime.New` 时增加该 optFn。注意两个分支都要加，不能只覆盖 Bearer 或 AK/SK 之一。

- [ ] **步骤 8：运行 AWS 测试验证通过**

运行：

```bash
go test -p 1 ./relay/channel/aws -run 'SubscriptionMarker|AwsClientRequest' -count=1
```

预期：PASS。

- [ ] **步骤 9：运行 provider 相关测试**

运行：

```bash
go test -p 1 ./relay/channel/aws ./relay/channel/xunfei ./relay/channel/volcengine -run 'SubscriptionMarker|DialHeader|AwsClientRequest' -count=1
```

预期：PASS。

- [ ] **步骤 10：提交任务 3**

```bash
git add -- relay/channel/aws/relay-aws.go relay/channel/aws/relay_aws_test.go relay/channel/xunfei/relay-xunfei.go relay/channel/xunfei/relay_xunfei_test.go relay/channel/volcengine/tts.go relay/channel/volcengine/tts_test.go
git commit -m "feat(relay): 覆盖试用标记特殊上游路径"
```

---

## 任务 4：最终验证与审查准备

**文件：**
- 不新增生产文件；如测试或格式需要微调，仅修改本计划涉及文件。

- [ ] **步骤 1：运行目标 Go 测试**

运行：

```bash
go test -p 1 ./model -run 'PreConsumeUserSubscriptionByUnitsReturnsPlanTrialMarker|PreConsumeUserSubscriptionByUnitsReturnsPlanMetadata' -count=1
go test -p 1 ./service -run 'PreConsumeBillingSyncsSubscriptionTrialMarker|ClearRelayBillingStateClearsSubscriptionTrialMarker' -count=1
go test -p 1 ./relay/channel -run 'HeaderOverride|FinalizeSubscriptionMarkerHeader' -count=1
go test -p 1 ./relay/channel/aws ./relay/channel/xunfei ./relay/channel/volcengine -run 'SubscriptionMarker|DialHeader|AwsClientRequest' -count=1
```

预期：全部 PASS。

- [ ] **步骤 2：运行空白检查**

运行：

```bash
git diff --check HEAD~3..HEAD
```

如果任务提交数量不是 3，使用实际实现起点到当前 HEAD 的范围。预期：无输出。

- [ ] **步骤 3：检查提交范围**

运行：

```bash
git show --stat --oneline --no-renames HEAD~2..HEAD
```

预期：只包含本功能相关提交与文件；不得包含无关开发智能体留下的工作区变更。

- [ ] **步骤 4：请求最终代码审查**

并发派发至少 3 个只读 reviewer 子代理，分别从规格合规、后端路径、安全测试角度审查最新实现。审查必须引用：

- 规格：`docs/superpowers/specs/2026-06-04-trial-upstream-marker-design.md`
- 计划：`docs/superpowers/plans/2026-06-04-trial-upstream-marker-implementation-plan.md`
- 实现提交范围：从本计划第一个实现提交到当前 HEAD。

所有 reviewer 必须 PASS 后才能进入发布或部署阶段。
