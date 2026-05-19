# API Help 动态配置重设计实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法跟踪进度。当前用户要求直接在主工作区 `C:/Users/34404/source/repos/new-api` 开发，不使用额外 worktree。

**目标：** 将 API Help 的 OpenCode / OMP 自动配置改为后端单一事实源，按当前 API key 可用模型裁剪输出，并移除 provider-native `web_search` / `image_generation` / provider-tools 自动配置路径。

**架构：** 后端新增共享 config-guide token 校验与 effective model resolver，`/api/token/opencode/openai-models` 和 `/config-guides/...` 复用该 resolver。OpenCode / OMP 配置只由后端端点渲染；前端只选择 API key、构造 manifest instruction、拉取并展示后端 artifacts。OMP 成功路径只保留普通 OpenAI Responses provider 配置，不读取 provider-tools npm metadata，不输出 plugin / image-generator / `-Sys`。

**技术栈：** Go 1.22、Gin、GORM、SQLite 测试库、React 19、TypeScript、TanStack Query、Bun、i18next。

---

## 参考规格

- 规格：`docs/superpowers/specs/2026-05-19-api-help-dynamic-config-redesign.md`
- 主工作区：`C:/Users/34404/source/repos/new-api`
- 重要约束：
  - 不提交真实 API key、私有参考仓库路径、私有仓库名或私有产品语义。
  - 不修改受保护项目/组织标识。
  - 新 UI 文案必须走 `t()` 并同步 `web/default/src/i18n/locales/{en,zh,fr,ja,ru,vi}.json`。
  - Go 业务 JSON 序列化使用 `common.Marshal` / `common.Unmarshal` / `common.DecodeJson`。
  - 子代理不运行项目级验证、格式化、build、typecheck、lint 或 test；主代理最终统一运行。

## 当前差距

- `controller/config_guide.go`：public config guide 只校验 query/base_url，不校验 API key 真实性、归属、状态、用户状态、group、AllowIps，也没有按 key 交集裁剪；OMP 仍调用 provider-tools metadata 并输出 plugin / image-generator / image provider / `-Sys`。
- `controller/token.go:GetOpenCodeOpenAIModels`：无 `token_id` 参数校验，直接返回 raw `models` 和 `omp_openai_provider_tools`。
- `router/config-guide-router.go`：仍注册 `/config-guides/omp-openai/plugin.txt` 和 `/image-generator.md` 成功路由。
- `web/default/src/features/api-help/lib/usage-config.ts`：仍包含前端 OpenCode / OMP 成功配置 renderer、provider-tools、image-provider、`imageGeneration` 文案。
- `web/default/src/features/api-help/components/api-usage-help-dialog.tsx`：metadata queryKey 只含 userId，未选 key 仍请求，切 key 复用旧 ready 状态，OMP tab 仍显示 plugin/image-generator。

## 文件职责

### 后端

- `controller/config_guide.go`
  - 新增 config-guide token 校验与 effective model resolver。
  - OpenCode / OMP handler 改为先校验 token，再用 effective models 渲染。
  - 移除 OMP provider-tools / plugin / image-generator 成功路径。
  - OpenCode renderer 对 options 做 denylist，固定默认模型 `gpt-5` 和小模型 `gpt-5-mini`。
- `controller/config_guide_test.go`
  - 覆盖 public key 校验、effective set、OpenCode denylist、OMP cleanup、no-store header、secret 不回显。
- `controller/token.go`
  - `GetOpenCodeOpenAIModels` 改为 `token_id` 入口，返回 effective `models`，不返回 raw provider-tools metadata。
  - 登录态 `api_key` 兼容参数直接返回 400，避免额外泄露面。
- `controller/token_test.go`
  - 覆盖登录态 metadata endpoint 授权、状态码、effective set、无 provider-tools metadata。
- `router/config-guide-router.go`
  - 注销 OMP `plugin.txt` / `image-generator.md` 路由。
- `router/config_guide_route_test.go`
  - 锁定 config-guide 路由优先于 SPA fallback，旧 OMP image/plugin 路由不再成功。
- `service/opencode_metadata.go` / `service/opencode_metadata_test.go`
  - 保留 models.dev mirror、Fast materialization、stale fallback；本次不让 API Help OMP 成功路径调用 `GetOMPProviderToolsMetadata`。

### 前端

- `web/default/src/features/keys/api.ts`
  - `getOpenCodeOpenAIModels(tokenId)` 序列化 `token_id`。
  - 增加 `fetchAgentConfigArtifact(path)` 拉取后端 artifacts。
- `web/default/src/features/api-help/lib/usage-config.ts`
  - 保留 base URL、API key normalization、manifest URL/instruction、Generic/Common Apps 手动片段。
  - 删除前端 OpenCode / OMP 成功 renderer、provider-tools、image-provider、`-Sys` helper。
  - 增加 query key、artifact path、ready gating、config section 纯 helper。
- `web/default/src/features/api-help/lib/usage-config.test.ts`
  - 覆盖 helper 行为和源码 smoke：不含 provider-native image/provider-tools helpers。
- `web/default/src/features/api-help/api-key-loading.test.ts`
  - 源码 smoke：queryKey 含 selected key，未选 key不发 metadata，Dialog 使用后端 artifacts，布局 shell 正确。
- `web/default/src/features/api-help/components/api-usage-help-dialog.tsx`
  - OpenCode tab：可见一句话 instruction + 一个 `opencode.json` artifact block。
  - OMP tab：可见一句话 instruction + `models.yml` / `config.yml` 两个 artifact blocks。
  - loading/unavailable/missing required model 时不显示成功 instruction 和成功 config blocks。
- `web/default/src/i18n/locales/{en,zh,fr,ja,ru,vi}.json`
  - 补齐新增/修改文案。

---

## 任务 1：后端共享 token 校验与 effective model resolver

**允许修改：**

- `controller/config_guide.go`
- `controller/config_guide_test.go`
- `controller/token.go`
- `controller/token_test.go`

**禁止修改：** 前端文件、router 文件、service metadata 文件。

### 步骤

- [ ] **步骤 1：新增后端失败测试 helper**

在 `controller/config_guide_test.go` 扩展测试基础设施：

```go
func setupConfigGuideTestDB(t *testing.T) *gorm.DB {
    t.Helper()
    db := setupTokenControllerTestDB(t)
    if err := db.AutoMigrate(&model.User{}, &model.Ability{}); err != nil {
        t.Fatalf("failed to migrate config guide test tables: %v", err)
    }
    originalGroupRatio := ratio_setting.GroupRatio2JSONString()
    originalModelRatio := ratio_setting.ModelRatio2JSONString()
    t.Cleanup(func() {
        _ = ratio_setting.UpdateGroupRatioByJSONString(originalGroupRatio)
        _ = ratio_setting.UpdateModelRatioByJSONString(originalModelRatio)
    })
    require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"paid":1}`))
    require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"gpt-5":1,"gpt-5-mini":1,"gpt-5-fast":1}`))
    return db
}

func seedConfigGuideUser(t *testing.T, db *gorm.DB, id int, group string, status int) *model.User {
    t.Helper()
    user := &model.User{Id: id, Username: fmt.Sprintf("cg-user-%d", id), Password: "password123", Group: group, Status: status}
    require.NoError(t, db.Create(user).Error)
    return user
}

func seedConfigGuideToken(t *testing.T, db *gorm.DB, userID int, rawKey string, status int, expiredTime int64, group string, unlimited bool, modelLimits string, allowIps *string) *model.Token {
    t.Helper()
    token := &model.Token{UserId: userID, Name: rawKey, Key: rawKey, Status: status, CreatedTime: 1, AccessedTime: 1, ExpiredTime: expiredTime, RemainQuota: 100, UnlimitedQuota: unlimited, Group: group, ModelLimits: modelLimits, ModelLimitsEnabled: modelLimits != "", AllowIps: allowIps}
    require.NoError(t, db.Create(token).Error)
    return token
}

func seedConfigGuideAbility(t *testing.T, db *gorm.DB, group string, modelName string) {
    t.Helper()
    require.NoError(t, db.Create(&model.Ability{Group: group, Model: modelName, ChannelId: len(modelName) + len(group), Enabled: true}).Error)
}
```

如当前测试包没有 `require` / `gorm` imports，添加 `github.com/stretchr/testify/require`、`gorm.io/gorm`、`github.com/QuantumNous/new-api/model`、`github.com/QuantumNous/new-api/setting/ratio_setting`。

- [ ] **步骤 2：写 public API key 校验失败测试**

在 `controller/config_guide_test.go` 新增：

```go
func TestConfigGuidePublicAPIKeyValidationFailures(t *testing.T) {
    cases := []struct {
        name string
        tokenStatus int
        userStatus int
        expiredTime int64
        group string
        allowIps *string
        target string
        wantStatus int
    }{
        {name: "missing", target: "/config-guides/opencode-openai/manifest.json", wantStatus: http.StatusBadRequest},
        {name: "unknown", target: "/config-guides/opencode-openai/manifest.json?api_key=sk-missing", wantStatus: http.StatusUnauthorized},
        {name: "disabled", tokenStatus: common.TokenStatusDisabled, userStatus: common.UserStatusEnabled, expiredTime: -1, group: "default", target: "/config-guides/opencode-openai/manifest.json?api_key=sk-livetoken", wantStatus: http.StatusForbidden},
        {name: "expired status", tokenStatus: common.TokenStatusExpired, userStatus: common.UserStatusEnabled, expiredTime: -1, group: "default", target: "/config-guides/opencode-openai/manifest.json?api_key=sk-livetoken", wantStatus: http.StatusForbidden},
        {name: "expired time", tokenStatus: common.TokenStatusEnabled, userStatus: common.UserStatusEnabled, expiredTime: 1, group: "default", target: "/config-guides/opencode-openai/manifest.json?api_key=sk-livetoken", wantStatus: http.StatusForbidden},
        {name: "user disabled", tokenStatus: common.TokenStatusEnabled, userStatus: common.UserStatusDisabled, expiredTime: -1, group: "default", target: "/config-guides/opencode-openai/manifest.json?api_key=sk-livetoken", wantStatus: http.StatusForbidden},
        {name: "exhausted", tokenStatus: common.TokenStatusExhausted, userStatus: common.UserStatusEnabled, expiredTime: -1, group: "default", target: "/config-guides/opencode-openai/manifest.json?api_key=sk-livetoken", wantStatus: http.StatusTooManyRequests},
        {name: "deprecated token group", tokenStatus: common.TokenStatusEnabled, userStatus: common.UserStatusEnabled, expiredTime: -1, group: "gone", target: "/config-guides/opencode-openai/manifest.json?api_key=sk-livetoken", wantStatus: http.StatusForbidden},
        {name: "ip denied", tokenStatus: common.TokenStatusEnabled, userStatus: common.UserStatusEnabled, expiredTime: -1, group: "default", allowIps: common.GetPointer("10.0.0.0/8"), target: "/config-guides/opencode-openai/manifest.json?api_key=sk-livetoken", wantStatus: http.StatusForbidden},
        {name: "control character", target: "/config-guides/opencode-openai/manifest.json?api_key=sk-live%0A-token", wantStatus: http.StatusBadRequest},
        {name: "suffix key accepted like TokenAuth", tokenStatus: common.TokenStatusEnabled, userStatus: common.UserStatusEnabled, expiredTime: -1, group: "default", target: "/config-guides/opencode-openai/manifest.json?api_key=sk-livetoken-extra-suffix", wantStatus: http.StatusOK},
        {name: "deprecated user group", tokenStatus: common.TokenStatusEnabled, userStatus: common.UserStatusEnabled, expiredTime: -1, group: "", target: "/config-guides/opencode-openai/manifest.json?api_key=sk-livetoken", wantStatus: http.StatusForbidden},
    }
    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            db := setupConfigGuideTestDB(t)
            if tc.tokenStatus != 0 {
                userGroup := "default"
                if tc.name == "deprecated user group" {
                    userGroup = "gone"
                }
                seedConfigGuideUser(t, db, 10, userGroup, tc.userStatus)
                seedConfigGuideToken(t, db, 10, "livetoken", tc.tokenStatus, tc.expiredTime, tc.group, true, "", tc.allowIps)
                seedConfigGuideAbility(t, db, "default", "gpt-5")
                seedConfigGuideAbility(t, db, "default", "gpt-5-mini")
            withStubOpenCodeMetadataProvider(t, stubOpenCodeMetadataProvider{models: configGuideTestModels()})
            ctx, recorder := newAuthenticatedContext(t, http.MethodGet, tc.target, nil, 0)
            GetOpenCodeConfigGuideManifest(ctx)
            require.Equal(t, tc.wantStatus, recorder.Code, recorder.Body.String())
            require.NotContains(t, recorder.Body.String(), "livetoken")
        })
    }
}
```

- [ ] **步骤 3：写 effective set 成功和交集测试**

新增测试：

```go
func TestConfigGuidePublicAPIKeyUsesEffectiveModels(t *testing.T) {
    db := setupConfigGuideTestDB(t)
    seedConfigGuideUser(t, db, 10, "default", common.UserStatusEnabled)
    seedConfigGuideToken(t, db, 10, "livetoken", common.TokenStatusEnabled, -1, "default", true, "", nil)
    seedConfigGuideAbility(t, db, "default", "gpt-5")
    seedConfigGuideAbility(t, db, "default", "gpt-5-mini")
    seedConfigGuideAbility(t, db, "default", "gpt-5-Sys")
    seedConfigGuideAbility(t, db, "default", "not-in-metadata")
    withStubOpenCodeMetadataProvider(t, stubOpenCodeMetadataProvider{models: configGuideTestModels()})

    ctx, recorder := newAuthenticatedContext(t, http.MethodGet, "/config-guides/opencode-openai/opencode.json?api_key=sk-livetoken&base_url=https://api.example.com/v1", nil, 0)
    GetOpenCodeConfigGuideJSON(ctx)

    require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
    require.Equal(t, "no-store", recorder.Header().Get("Cache-Control"))
    require.Equal(t, "no-cache", recorder.Header().Get("Pragma"))
    var cfg map[string]any
    require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &cfg))
    models := cfg["provider"].(map[string]any)["new-api"].(map[string]any)["models"].(map[string]any)
    require.Contains(t, models, "gpt-5")
    require.Contains(t, models, "gpt-5-mini")
    require.Contains(t, models, "gpt-5-fast")
    require.NotContains(t, models, "not-in-metadata")
    for id := range models {
        require.NotContains(t, id, "-Sys")
    }
}
```

- [ ] **步骤 4：写 resolver 表驱动测试**

新增纯 helper 测试，目标函数签名：

```go
func buildConfigGuideEffectiveModels(input configGuideEffectiveModelsInput) (map[string]service.OpenCodeOpenAIModel, error)
```

测试覆盖 token limits、auto group、OMP 不合成 fast、缺小模型 fail-closed。测试可直接传 `availableModels`，避免每个 case 建 DB。

- [ ] **步骤 5：写 token endpoint 失败测试**

在 `controller/token_test.go` 新增：

```go
func TestGetOpenCodeOpenAIModelsRequiresOwnedTokenID(t *testing.T) {
    db := setupTokenControllerTestDB(t)
    require.NoError(t, db.AutoMigrate(&model.User{}, &model.Ability{}))
    require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))
    require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"gpt-5":1,"gpt-5-mini":1,"gpt-5-fast":1}`))
    seedConfigGuideUser(t, db, 1, "default", common.UserStatusEnabled)
    seedConfigGuideUser(t, db, 2, "default", common.UserStatusEnabled)
    owned := seedConfigGuideToken(t, db, 1, "owned-token", common.TokenStatusEnabled, -1, "default", true, "", nil)
    foreign := seedConfigGuideToken(t, db, 2, "foreign-token", common.TokenStatusEnabled, -1, "default", true, "", nil)
    seedConfigGuideAbility(t, db, "default", "gpt-5")
    seedConfigGuideAbility(t, db, "default", "gpt-5-mini")
    withStubOpenCodeMetadataProvider(t, stubOpenCodeMetadataProvider{models: configGuideTestModels()})

    ctx, recorder := newAuthenticatedContext(t, http.MethodGet, fmt.Sprintf("/api/token/opencode/openai-models?token_id=%d", owned.Id), nil, 1)
    GetOpenCodeOpenAIModels(ctx)
    require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
    response := decodeAPIResponse(t, recorder)
    require.True(t, response.Success)
    require.Contains(t, string(response.Data), "gpt-5")
    require.NotContains(t, string(response.Data), "omp_openai_provider_tools")

    ctx, recorder = newAuthenticatedContext(t, http.MethodGet, fmt.Sprintf("/api/token/opencode/openai-models?token_id=%d", foreign.Id), nil, 1)
    GetOpenCodeOpenAIModels(ctx)
    require.Equal(t, http.StatusUnauthorized, recorder.Code)
    require.NotContains(t, recorder.Body.String(), "foreign-token")
}

func TestGetOpenCodeOpenAIModelsRejectsAPIKeyQueryCompatibility(t *testing.T) {
    ctx, recorder := newAuthenticatedContext(t, http.MethodGet, "/api/token/opencode/openai-models?api_key=sk-anything", nil, 1)
    GetOpenCodeOpenAIModels(ctx)
    require.Equal(t, http.StatusBadRequest, recorder.Code)
    require.NotContains(t, recorder.Body.String(), "sk-anything")
}
```

- [ ] **步骤 5a：运行 token/effective 红灯测试**

实现任何生产代码前运行：

```bash
go test ./controller -run 'Test(ConfigGuidePublicAPIKeyValidation|ConfigGuidePublicAPIKeyUsesEffectiveModels|BuildConfigGuideEffectiveModels|GetOpenCodeOpenAIModels)' -count=1
```

预期：FAIL，原因应是缺少真实 token 校验、effective resolver 或 `token_id` endpoint 行为。

- [ ] **步骤 6：实现共享校验和 resolver**

在 `controller/config_guide.go` 中新增类型：

```go
type configGuideClient string
const (
    configGuideClientOpenCode configGuideClient = "opencode"
    configGuideClientOMP configGuideClient = "omp"
)

type configGuideEffectiveModelsInput struct {
    Client configGuideClient
    Metadata map[string]service.OpenCodeOpenAIModel
    AvailableModels []string
}
```

新增 helper：

- `parseConfigGuideTokenKey(raw string)`：复用 TokenAuth key 解析边界，trim、拒绝控制字符、去掉 `Bearer ` / `sk-` / 重复 `sk-sk-`，再按 `-` 截取首段；错误响应不得回显原始 key。
- `loadConfigGuideTokenByPublicKey(c, apiKey)`：先调用 `parseConfigGuideTokenKey`，再 `model.GetTokenByKey`，not found -> 401，DB -> 500。
- `loadConfigGuideTokenByID(c, tokenID, userID)`：`model.GetTokenByIds`，not found -> 401；读取后仍必须调用同一个 `validateConfigGuideTokenUsability`。
- `validateConfigGuideTokenUsability(c, token)`：
  - `TokenStatusExhausted` -> 429。
  - 非 enabled、expired status、expired time -> 403。
  - `model.GetUserCache` 或 DB 用户读取，user 非 enabled -> 403。
  - IP allowlist 用 `c.ClientIP()` + `common.IsIpInCIDRList`；解析失败/不匹配 -> 403。
  - 先计算最终 using group：token group 非空用 token group，否则用 user group；token group 非空时必须属于 `service.GetUserUsableGroups(user.Group)`；最终 using group 非 `auto` 时必须 `ratio_setting.ContainsGroupRatio(usingGroup)`，包括 token group 为空且用户分组已废弃的场景。
- `availableConfigGuideModelsForToken(token, user)`：按规格复用 `ListModels` 语义。
- `buildConfigGuideEffectiveModels(input)`：strip `-Sys`，按 metadata 交集；OpenCode 合成 `<base>-fast`；OMP 不合成 fast；必须包含 `gpt-5` / `gpt-5-mini`。
- `requireConfigGuideEffectiveModels(c, params, client)`：public config guide 统一入口。

- [ ] **步骤 7：更新 token endpoint**

在 `controller/token.go:GetOpenCodeOpenAIModels`：

- 有 `api_key` 参数直接 400。
- 缺 `token_id` 400。
- 解析 token id，调用 `loadConfigGuideTokenByID`，然后必须调用共享 `validateConfigGuideTokenUsability`；disabled、expired、exhausted、用户禁用、group/IP 不可用状态码必须与 public config guide 一致。
- 调 metadata provider + resolver。
- 响应只返回 `models`。

- [ ] **步骤 8：替换 config guide handlers 的 raw metadata 使用**

OpenCode manifest/json、OMP manifest/models/config 均调用 `requireConfigGuideEffectiveModels`。`GetOMPConfigGuideConfig` 也必须校验 key 和 required models 后才返回 config，避免无效 key 成功。

同步改造现有成功测试（例如 `TestOpenCodeConfigGuideManifestReturnsJSONNotWebFallback`、`TestOpenCodeConfigGuideJSONReturnsRenderableConfig`、`TestConfigGuideDerivesBaseURLFromRequest`、`TestOMPConfigGuideManifestAndFiles`、`TestOMPConfigGuideModelsQuotesYAMLScalars`）：所有会期望 200 的 config-guide handler 测试都必须调用统一 valid fixture，seed enabled user/token/abilities/ratio 后再请求 `api_key=sk-livetoken`。

- [ ] **步骤 9：运行绿灯测试**

运行：

```bash
go test ./controller -run 'Test(ConfigGuidePublicAPIKeyValidation|ConfigGuidePublicAPIKeyUsesEffectiveModels|BuildConfigGuideEffectiveModels|GetOpenCodeOpenAIModels)' -count=1
```

预期：PASS。

---

## 任务 2：后端 OpenCode renderer 和 OMP cleanup

**允许修改：**

- `controller/config_guide.go`
- `controller/config_guide_test.go`
- `router/config-guide-router.go`
- `router/config_guide_route_test.go`

**禁止修改：** `controller/token.go`、前端文件。

### 步骤

- [ ] **步骤 1：写 OpenCode denylist 失败测试**

扩展 `configGuideTestModels()`，给 `gpt-5` 或 `gpt-5-fast` 加入污染 options：

```go
Options: map[string]any{
    "metadata": map[string]any{"builtin_tools": map[string]any{"web_search": true}},
    "builtin_tools": map[string]any{"image_generation": true},
    "web_search": true,
    "imageGeneration": true,
    "serviceTier": "priority",
},
```

新增测试：

```go
func TestOpenCodeConfigGuideJSONDoesNotEmitProviderNativeTools(t *testing.T) {
    // seed valid token + abilities + stub metadata
    ctx, recorder := newAuthenticatedContext(t, http.MethodGet, "/config-guides/opencode-openai/opencode.json?api_key=sk-livetoken&base_url=https://api.example.com/v1", nil, 0)
    GetOpenCodeConfigGuideJSON(ctx)
    require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
    body := recorder.Body.String()
    for _, forbidden := range []string{"builtin_tools", "web_search", "image_generation", "imageGeneration", "metadata", "agent.image", "structured_output"} {
        require.NotContains(t, body, forbidden)
    }
    require.Contains(t, body, `"store":false`)
    require.Contains(t, body, `"serviceTier":"priority"`)
}
```

- [ ] **步骤 2：替换 small model fallback 测试**

将 `TestOpenCodeConfigGuideJSONFallsBackWhenSmallModelMissing` 改为：

```go
func TestOpenCodeConfigGuideJSONFailsWhenSmallModelMissing(t *testing.T) {
    models := configGuideTestModels()
    delete(models, "gpt-5-mini")
    // seed valid token and abilities
    // expected: http.StatusServiceUnavailable
}
```

- [ ] **步骤 3：写 OMP provider-tools 不得调用测试**

扩展 stub：

```go
type stubOpenCodeMetadataProvider struct {
    models map[string]service.OpenCodeOpenAIModel
    plugin service.OMPProviderToolsMetadata
    err error
    failOnOMPProviderTools bool
}
func (p stubOpenCodeMetadataProvider) GetOMPProviderToolsMetadata(context.Context) service.OMPProviderToolsMetadata {
    if p.failOnOMPProviderTools { panic("OMP provider-tools metadata must not be called") }
    return p.plugin
}
```

新增测试：

```go
func TestOMPConfigGuideDoesNotRequireProviderToolsMetadata(t *testing.T) {
    withStubOpenCodeMetadataProvider(t, stubOpenCodeMetadataProvider{models: configGuideTestModels(), failOnOMPProviderTools: true})
    // seed valid token and abilities
    targets := []string{
        "/config-guides/omp-openai/manifest.json?api_key=sk-livetoken&base_url=https://api.example.com/v1",
        "/config-guides/omp-openai/models.yml?api_key=sk-livetoken&base_url=https://api.example.com/v1",
        "/config-guides/omp-openai/config.yml?api_key=sk-livetoken&base_url=https://api.example.com/v1",
    }
    for _, target := range targets {
        ctx, recorder := newAuthenticatedContext(t, http.MethodGet, target, nil, 0)
        switch {
        case strings.Contains(target, "manifest.json"):
            GetOMPConfigGuideManifest(ctx)
        case strings.Contains(target, "models.yml"):
            GetOMPConfigGuideModels(ctx)
        default:
            GetOMPConfigGuideConfig(ctx)
        }
        require.Equal(t, http.StatusOK, recorder.Code, target+recorder.Body.String())
        for _, forbidden := range []string{"plugin", "image-generator", "openaiProviderTools", "new-api-image", "imageGeneration", "-Sys"} {
            require.NotContains(t, recorder.Body.String(), forbidden)
        }
    }
}
```

- [ ] **步骤 4：更新 OMP manifest 测试**

`TestOMPConfigGuideManifestAndFiles` 改为期望 `len(manifest.Items)==2`，只包含 `models` 和 `config`。`models.yml` 不包含 provider-tools / image provider；`config.yml` 不含 `-Sys`。

- [ ] **步骤 5：写旧路由不成功测试**

在 `router/config_guide_route_test.go` 新增：

```go
func TestOMPConfigGuidePluginAndImageGeneratorRoutesAreNotSuccessful(t *testing.T) {
    gin.SetMode(gin.TestMode)
    engine := gin.New()
    SetConfigGuideRouter(engine)
    for _, path := range []string{"/config-guides/omp-openai/plugin.txt", "/config-guides/omp-openai/image-generator.md"} {
        recorder := httptest.NewRecorder()
        req := httptest.NewRequest(http.MethodGet, path+"?api_key=sk-livetoken", nil)
        engine.ServeHTTP(recorder, req)
        require.NotEqual(t, http.StatusOK, recorder.Code)
    }
}
```

- [ ] **步骤 6：实现 OpenCode sanitize**

在 `mergeConfigGuideOpenCodeModelOptions` 中递归删除 forbidden keys：`metadata`、`builtin_tools`、`web_search`、`image_generation`、`imageGeneration`。保留 `store:false` 和 `serviceTier` 等普通 options。`renderConfigGuideOpenCode` 必须要求 `gpt-5` 和 `gpt-5-mini` 均为有效 text model，否则返回 error。

- [ ] **步骤 7：实现 OMP cleanup**

在 `controller/config_guide.go`：

- 删除 handler 成功路径：`GetOMPConfigGuidePlugin` / `GetOMPConfigGuideImageGenerator` 可保留为 410，但不再注册路由。
- 删除 `requireConfigGuideOMPProviderToolsVersion` 调用。
- `GetOMPConfigGuideManifest` items 只保留 `models.yml`、`config.yml`。
- `renderConfigGuideOMPModels(baseURL, apiKey, models)` 只输出 provider `new-api`。
- 删除/不调用 `withConfigGuideOMPSysVariants`。
- `renderConfigGuideOMPSettings` 使用 `new-api/gpt-5` 和 `new-api/gpt-5-mini`，不含 `-Sys`。

- [ ] **步骤 8：注销旧路由**

在 `router/config-guide-router.go` 删除：

```go
ompRoute.GET("/plugin.txt", controller.GetOMPConfigGuidePlugin)
ompRoute.GET("/image-generator.md", controller.GetOMPConfigGuideImageGenerator)
```

- [ ] **步骤 9：主代理验证命令**

主代理运行：

```bash
go test ./controller ./router -run 'Test(OpenCodeConfigGuideJSONDoesNotEmitProviderNativeTools|OpenCodeConfigGuideJSONFailsWhenSmallModelMissing|OMPConfigGuide|ConfigGuideRouteReturnsJSONBeforeWebFallback|OMPConfigGuidePluginAndImageGeneratorRoutesAreNotSuccessful)' -count=1
```

预期：PASS。

---

## 任务 3：前端 API 与 usage-config helper

**允许修改：**

- `web/default/src/features/keys/api.ts`
- `web/default/src/features/api-help/lib/usage-config.ts`
- `web/default/src/features/api-help/lib/usage-config.test.ts`
- `web/default/src/features/api-help/api-key-loading.test.ts`

**禁止修改：** Dialog TSX、locale JSON、后端文件。

### 步骤

- [ ] **步骤 1：写 `token_id` API smoke 测试**

在 `usage-config.test.ts` 或 `api-key-loading.test.ts` 中读取 `web/default/src/features/keys/api.ts`，断言：

```ts
assert.match(keysApiSource, /getOpenCodeOpenAIModels\(tokenId: number\)/)
assert.match(keysApiSource, /params\.set\('token_id', String\(tokenId\)\)/)
assert.doesNotMatch(keysApiSource, /api_key/)
```

- [ ] **步骤 2：写 usage-config 禁用 provider-native helper 测试**

在 `usage-config.test.ts` 新增源码 smoke：

```ts
test('usage config does not contain provider-native OMP image helpers', () => {
  const source = readFileSync(new URL('./usage-config.ts', import.meta.url), 'utf8')
  for (const forbidden of ['IMAGE_PROVIDER_ID', 'OMP_PROVIDER_TOOLS_PACKAGE', 'buildOmpPluginInstructions', 'buildOmpImageGeneratorConfig', 'openaiProviderTools', 'imageGeneration', 'new-api-image']) {
    assert.doesNotMatch(source, new RegExp(forbidden))
  }
})
```

- [ ] **步骤 3：写 query key / artifact gate helper 测试**

新增纯函数并测试：

```ts
buildOpenCodeMetadataQueryKey(42) // ['api-help','opencode-openai-models','token','42']
buildAgentConfigArtifactQueryKey({ client:'opencode', file:'opencode.json', selectedKeyId:'42', apiKey:'sk-live', serverAddress:'https://api.example.com' })
canFetchAgentConfigArtifacts({ selectedKeyId:'42', metadataReady:true, apiKey:'sk-live' }) // true
canFetchAgentConfigArtifacts({ selectedKeyId:'', metadataReady:true, apiKey:'sk-live' }) // false
```

- [ ] **步骤 4：写 config section helper 测试**

新增函数：

```ts
buildAgentConfigSections(input: { client: AgentConfigGuideClient; ready: boolean; artifacts: Partial<Record<AgentConfigArtifactFile, string>> }): ConfigFile[]
```

断言 ready=false 返回 `[]`；OpenCode ready 返回 `opencode.json`；OMP ready 返回 `~/.omp/agent/models.yml` 和 `~/.omp/agent/config.yml`，不返回 plugin/image-generator。

新增 artifact path helper 表驱动测试，断言：

```ts
assert.equal(buildAgentConfigArtifactPath('opencode', 'opencode.json', 'sk-live', 'https://api.example.com'), '/config-guides/opencode-openai/opencode.json?api_key=sk-live&base_url=https%3A%2F%2Fapi.example.com%2Fv1')
assert.equal(buildAgentConfigArtifactPath('omp', 'models.yml', 'live', 'https://api.example.com/v1'), '/config-guides/omp-openai/models.yml?api_key=sk-live&base_url=https%3A%2F%2Fapi.example.com%2Fv1')
assert.equal(buildAgentConfigArtifactPath('omp', 'config.yml', 'sk-live', ''), '/config-guides/omp-openai/config.yml?api_key=sk-live')
```

同时删除或重写 `usage-config.test.ts` 里对 `buildOpenCodeConfig`、`buildOmpModelsConfig`、`buildOmpSettingsConfig` 的 imports 和旧断言；这些导出会被移除，测试必须只覆盖 manifest/artifact/helper 行为。

- [ ] **步骤 4a：运行 usage-config 红灯测试**

实现 helper 前运行：

```bash
cd web/default && bunx tsx --test src/features/api-help/lib/usage-config.test.ts src/features/api-help/api-key-loading.test.ts
```

预期：FAIL，原因应是旧 renderer 仍存在或新 helper 尚未实现。

- [ ] **步骤 5：实现 `keys/api.ts`**

将：

```ts
export async function getOpenCodeOpenAIModels(): Promise<...>
```

改为：

```ts
export async function getOpenCodeOpenAIModels(tokenId: number): Promise<...> {
  const params = new URLSearchParams()
  params.set('token_id', String(tokenId))
  const res = await api.get(`/api/token/opencode/openai-models?${params.toString()}`)
  return res.data
}

export async function fetchAgentConfigArtifact(path: string): Promise<string> {
  const res = await api.get(path, { responseType: 'text' })
  return typeof res.data === 'string' ? res.data : JSON.stringify(res.data, null, 2)
}
```

- [ ] **步骤 6：瘦身 `usage-config.ts`**

删除：

- `IMAGE_PROVIDER_ID`
- `OMP_PROVIDER_TOOLS_PACKAGE`
- `buildOpenCodeConfig`
- `buildOmpModelsConfig`
- `buildOmpSettingsConfig`
- `buildOmpPluginInstructions`
- `buildOmpImageGeneratorConfig`
- provider-native web_search/imageGeneration 相关文案和 helper

保留 generic/common apps helpers，新增：

```ts
export type AgentConfigArtifactFile = 'opencode.json' | 'models.yml' | 'config.yml'
export type ConfigFile = { path: string; content: string; hint?: string }
export function buildOpenCodeMetadataQueryKey(selectedKeyId: string | number | undefined): readonly unknown[]
export function buildAgentConfigArtifactPath(client: AgentConfigGuideClient, file: AgentConfigArtifactFile, apiKey: string, serverAddress: string): string
export function buildAgentConfigArtifactQueryKey(input: AgentConfigArtifactQueryInput): readonly unknown[]
export function canFetchAgentConfigArtifacts(input: { selectedKeyId?: string; metadataReady: boolean; apiKey: string }): boolean
export function buildAgentConfigSections(input: { client: AgentConfigGuideClient; ready: boolean; artifacts: Partial<Record<AgentConfigArtifactFile, string>> }): ConfigFile[]
```

`buildAgentConfigArtifactPath` 必须复用 `buildOpenAIBaseUrl`、`buildAgentConfigGuidePath` 的安全规则；manifest URL 一句话仍通过 `buildAgentConfigGuideInstruction` 生成。

- [ ] **步骤 7：运行绿灯测试**

运行：

```bash
cd web/default && bunx tsx --test src/features/api-help/lib/usage-config.test.ts src/features/api-help/api-key-loading.test.ts
```

预期：PASS。

---

## 任务 4：前端 Dialog 改为后端 artifact 展示并修复布局/i18n

**允许修改：**

- `web/default/src/features/api-help/components/api-usage-help-dialog.tsx`
- `web/default/src/features/api-help/api-key-loading.test.ts`
- `web/default/src/features/api-help/lib/usage-config.test.ts`
- `web/default/src/i18n/static-keys.ts`（仅当新增动态 key 无法被 t() 扫描时）
- `web/default/src/i18n/locales/en.json`
- `web/default/src/i18n/locales/zh.json`
- `web/default/src/i18n/locales/fr.json`
- `web/default/src/i18n/locales/ja.json`
- `web/default/src/i18n/locales/ru.json`
- `web/default/src/i18n/locales/vi.json`

**禁止修改：** 后端文件、`keys/api.ts`、`usage-config.ts` 生产 helper。

### 步骤

- [ ] **步骤 1：写 Dialog 使用后端 artifacts 的源码 smoke 测试**

在 `api-key-loading.test.ts` 新增：

```ts
test('api help dialog uses backend artifacts instead of frontend OpenCode or OMP renderers', () => {
  assert.doesNotMatch(source, /buildOpenCodeConfig/)
  assert.doesNotMatch(source, /buildOmpModelsConfig/)
  assert.doesNotMatch(source, /buildOmpSettingsConfig/)
  assert.doesNotMatch(source, /buildOmpPluginInstructions/)
  assert.doesNotMatch(source, /buildOmpImageGeneratorConfig/)
  assert.match(source, /fetchAgentConfigArtifact/)
  assert.match(source, /opencode\.json/)
  assert.match(source, /models\.yml/)
  assert.match(source, /config\.yml/)
})
```

- [ ] **步骤 2：写 metadata queryKey / enabled gate smoke 测试**

test('metadata query is keyed by selected API key and disabled without a key', () => {
  assert.match(source, /const selectedApiKey = selectedKeyId \? apiKeys\.find/)
  assert.match(source, /buildOpenCodeMetadataQueryKey\(selectedKeyId\)/)
  assert.match(source, /enabled:\s*open\s*&&\s*Boolean\(selectedKeyId\)/)
  assert.match(source, /getOpenCodeOpenAIModels\(selectedApiKey\.id\)/)
})
```

- [ ] **步骤 3：写布局 smoke 测试**

```ts
test('dialog uses fixed flex shell with body wrapper and footer margin reset', () => {
  assert.match(source, /DialogContent className='flex max-h-\[92vh\][^']*flex-col[^']*overflow-hidden/)
  assert.match(source, /<div className='min-h-0 flex-1 overflow-hidden'>/)
  assert.match(source, /<ScrollArea className='h-full min-h-0'>/)
  assert.match(source, /DialogFooter className='mx-0 mb-0 shrink-0/)
  assert.doesNotMatch(source, /grid-rows-none/)
})
```

- [ ] **步骤 3a：运行 Dialog 红灯测试**

实现 Dialog 前运行：

```bash
cd web/default && bunx tsx --test src/features/api-help/lib/usage-config.test.ts src/features/api-help/api-key-loading.test.ts
```

预期：FAIL，原因应是 Dialog 仍导入旧 renderer、queryKey/gating 或布局 smoke 未满足。

- [ ] **步骤 4：改造 Dialog 数据流**

在 `api-usage-help-dialog.tsx`：

- 移除旧导入：`buildOpenCodeConfig`、`buildOmpModelsConfig`、`buildOmpSettingsConfig`、`buildOmpPluginInstructions`、`buildOmpImageGeneratorConfig`。
- 新增导入：`fetchAgentConfigArtifact`、`buildOpenCodeMetadataQueryKey`、`buildAgentConfigArtifactPath`、`buildAgentConfigArtifactQueryKey`、`canFetchAgentConfigArtifacts`、`buildAgentConfigSections`。
- 先计算 `apiKeys` 和显式 selected key，再声明 metadata/artifact queries，避免在声明前使用变量：

```tsx
const apiKeys = apiKeysQuery.data ?? []
const selectedApiKey = selectedKeyId
  ? apiKeys.find((item) => String(item.id) === selectedKeyId)
  : undefined
const currentSelectedKeyId = selectedApiKey ? String(selectedApiKey.id) : ''
```

- 不再用 `apiKeys[0]` 作为隐式 selected key。可以在 key 列表加载后显式初始化 `selectedKeyId`，但初始化前不得请求 metadata/artifacts。
- `metadataQuery` 改为 selected key 维度，并把响应包装上当前 key，防 stale ready：

```tsx
const metadataQuery = useQuery({
  queryKey: buildOpenCodeMetadataQueryKey(selectedKeyId),
  queryFn: async () => {
    if (!selectedApiKey) return undefined
    const result = await getOpenCodeOpenAIModels(selectedApiKey.id)
    return result.success ? { selectedKeyId: String(selectedApiKey.id), data: result.data } : undefined
  },
  enabled: open && Boolean(selectedKeyId) && Boolean(selectedApiKey),
  staleTime: 15 * 60 * 1000,
})
const metadata = metadataQuery.data?.selectedKeyId === currentSelectedKeyId ? metadataQuery.data.data : undefined
```

- selected key 变化时，ready 由当前 key guard 后的 metadata 决定；未选 key 时 state 为 unavailable 或 loading，但不发请求、不显示成功 instruction。
- OMP ready 不依赖 `omp_openai_provider_tools`。
- 增加 artifact queries：
  - opencode: `opencode.json`
  - omp: `models.yml`
  - omp: `config.yml`
- artifact queryKey 必须包含 selected key id、client、file、serverAddress/baseUrl。
- artifact queryFn 也必须返回 `{ selectedKeyId, content }`，读取数据时只接受 `selectedKeyId === currentSelectedKeyId` 的结果，防止切 key 后旧 artifact 短暂显示。
- artifact enabled 必须同时满足 `selectedKeyId`、`selectedApiKey`、apiKey、metadata ready。
- `opencodeFiles` 和 `ompFiles` 只来自 `buildAgentConfigSections`，not ready 或 current-key guard 不通过时为空。

- [ ] **步骤 5：让一句话配置可见且 copy 一致**

修改 `AutoConfigCard`：

- ready 时显示完整 instruction 文本：

```tsx
<p className='break-all font-mono text-xs'>{instruction}</p>
```

- `CopyButton value={instruction}`；复制值必须等于可见文本。
- not ready 时不生成真实 key fallback success instruction。

- [ ] **步骤 6：修复布局 shell**

修改结构：

```tsx
<DialogContent className='flex max-h-[92vh] min-h-0 max-w-[calc(100%-1rem)] flex-col gap-0 overflow-hidden p-0 sm:max-w-5xl'>
  <DialogHeader className='shrink-0 p-4 pb-3 sm:p-5 sm:pb-4'>...</DialogHeader>
  <div className='shrink-0 border-y px-4 py-3 sm:px-5'>...</div>
  <div className='min-h-0 flex-1 overflow-hidden'>
    <ScrollArea className='h-full min-h-0'>...</ScrollArea>
  </div>
  <DialogFooter className='mx-0 mb-0 shrink-0 gap-2 p-4 pt-3 sm:p-5 sm:pt-4'>...</DialogFooter>
</DialogContent>
```

Key selector 外层改为 `w-full space-y-1.5 md:w-64`，避免窄屏撑破。

- [ ] **步骤 7：i18n 文案同步**

新增或修改文案至少包括：

- `Copy this visible instruction into your AI agent; it will fetch the manifest and configuration files directly from this gateway.`
- `Configuration is loaded from the backend after the selected API key is verified.`
- `Configuration file is loading...`
- `Configuration file is unavailable.`

为 `en/zh/fr/ja/ru/vi` 添加翻译。若文案以 `t('...')` 字面量出现，不需要 static keys；若作为动态 label 进入 helper，补 `static-keys.ts`。

- [ ] **步骤 8：主代理验证命令**

主代理运行：

```bash
cd web/default && bunx tsx --test src/features/api-help/lib/usage-config.test.ts src/features/api-help/api-key-loading.test.ts
cd web/default && bun run i18n:sync
cd web/default && bun run typecheck
```

预期：PASS；i18n sync report missing/extras 为 0。

---

## 任务 5：最终验证、审查与收口

**允许修改：** 仅修复前四个任务遗留的失败，不新增范围。

### 步骤

- [ ] **步骤 1：运行后端定向测试**

```bash
go test ./service -run 'TestOpenCodeMetadataServiceExtractsModels(AndProviderTools|FromProviderCatalog)' -count=1
go test ./controller -run 'Test(ConfigGuide|OpenCodeConfigGuide|OMPConfigGuide|GetOpenCodeOpenAIModels|BuildConfigGuideEffectiveModels)' -count=1
go test ./router -run 'Test(ConfigGuideRouteReturnsJSONBeforeWebFallback|OMPConfigGuidePluginAndImageGeneratorRoutesAreNotSuccessful)' -count=1
```

- [ ] **步骤 2：运行前端定向测试与类型检查**

```bash
cd web/default && bunx tsx --test src/features/api-help/lib/usage-config.test.ts src/features/api-help/api-key-loading.test.ts src/features/playground/playground-api-help.test.ts
cd web/default && bun run i18n:sync
cd web/default && bun run typecheck
```

- [ ] **步骤 3：运行生产构建**

```bash
cd web/default && bun run build
```

- [ ] **步骤 4：静态搜索安全检查**

使用专用搜索工具检查：

- 参考仓库路径或私有仓库名不得出现在提交文件中。
- `web/default/src/features/api-help` 不得出现 `IMAGE_PROVIDER_ID`、`OMP_PROVIDER_TOOLS_PACKAGE`、`buildOmpPluginInstructions`、`buildOmpImageGeneratorConfig`、`imageGeneration`、`openaiProviderTools`。
- OpenCode 输出测试覆盖 `builtin_tools`、`web_search`、`image_generation` denylist。

- [ ] **步骤 5：最终代码审查**

并发派发至少 3 个只读 reviewer：后端安全/正确性、前端 UI/gating、测试/i18n。审查者禁止修改文件、禁止运行项目级验证。所有 Critical/Important 必须修复后复审。

- [ ] **步骤 6：工作区检查与提交**

```bash
git diff --check
git status --short --branch --untracked-files=all
```

若验证和审查通过，提交：

```text
fix(api-help): 重构动态配置并移除自动工具注入
```
