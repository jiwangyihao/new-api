# API Help 动态配置重设计规格

## 背景

当前 API Help 的 OpenCode / OMP 动态配置来自上一轮增量补丁，已经暴露出 3 类根本问题：

1. 动态配置没有按当前 API key 的可用模型做交集裁剪，导致生成的配置可能包含用户实际不可用的模型。
2. Fast、reasoning variants、provider options、headers 等 `models.dev` 机制没有形成单一最终合同，前后端各自渲染，容易再次分叉。
3. OpenCode / OMP / image-generation 语义混在同一个帮助面板里，且面板布局和「一句话配置」缺少可测试的 UI 契约。

本次重做参考本地私有参考实现中已经验证过的动态配置链，但只迁移通用技术机制。当前用户约束优先：OpenCode 不做任何 `builtin_tools` 相关配置，特别是不做 `image_generation`。

## 目标

- 后端成为 OpenCode / OMP 动态配置的单一事实源。
- 完整支持 `models.dev` 的 Fast materialization、reasoning variants、provider options、headers、cost / limit 输出。
- 按当前 API key / group 的可用 OpenAI 模型与 metadata 做交集裁剪。
- 提供真实 `/config-guides/...` manifest 和配置文件端点，供 AI agent 通过「一句话配置」自动拉取。
- 修复 API Help 面板布局，保证 header、key selector、footer 稳定，内容区独立滚动。
- OpenCode 输出中不出现 `builtin_tools`、`web_search`、`image_generation`、`variants.image`、`agent.image`。

## 非目标

- 不实现 OpenCode provider-native image generation。
- 不在 Responses relay 中新增 `metadata.builtin_tools` 自动注入。
- 不把 OMP provider-tools / image-generator 语义混入 OpenCode。
- 不重写 Playground 主流程、API key 管理页或订阅逻辑。

## 公开仓库安全边界

- 本规格可在当前开发会话中参考本地私有仓库；后续实现、测试、UI 文案、manifest notes、提交信息不得写入参考仓库绝对路径、私有仓库名或私有产品语义。
- 测试 fixture 只能使用假域名、假 key、假用户、假模型和公开中性的 provider id。
- 不得输出或提交真实 API key。

## 参考实现要点

需要迁移的机制：

- `models.dev` 元数据 15 分钟 TTL，并在上游失败时返回 stale cache。
- `experimental.modes` 物化为 `baseID-mode`，例如 `gpt-5.5-fast`。
- mode 的 `provider.body` 转 camelCase 后进入 model options；`provider.headers` 进入 headers；mode cost 覆盖 / 叠加基础 cost。
- `reasoningLevels` 根据模型 ID 和 `release_date` 生成 `none`、`minimal`、`low`、`medium`、`high`、`xhigh`。
- config guide 根据 API key / group 可用 OpenAI 模型做交集；OpenCode 对指定 base 模型扩展 fast alias，OMP 不扩展。
- manifest 是 AI agent 的入口；真正配置通过 manifest items 拉取。

需要排除或隔离的机制：

- OpenCode 不输出 `metadata.builtin_tools` 或 `builtin_tools`。
- OpenCode 不输出 `variants.image` 或 `agent.image`。
- OMP provider-tools / `sub2api-openai-image` / `image-generator.md` 若后续恢复，必须严格限制在 OMP；本次实现先从 API Help 成功路径移除 OMP image-generation 子树，只保留普通 OMP Responses provider 配置。

## 架构设计

### 1. Metadata mirror 层

文件：`service/opencode_metadata.go`

职责：只镜像外部 metadata，不处理当前用户权限。

保留 / 增强：

- `GetOpenAIModels(ctx)`：读取 `models.dev/api.json`。
- `extractOpenCodeOpenAIModels(payload)`：
  - 优先读取 `openai.models`。
  - 如不存在，遍历 provider catalog，识别 `openai/` 前缀、`gpt-*` 与 codex 模型。
  - 过滤 `gpt-5-chat-latest`、`alpha`、`deprecated`。
  - 物化 `experimental.modes`。
- 这一层可以保留原始 metadata 中不会进入最终配置的字段，但最终 renderer 必须做 allowlist / denylist。
- `GetOMPProviderToolsMetadata(ctx)` 本次不参与 API Help 成功路径；若暂时保留，只能作为未使用的后续扩展能力。

### 2. Effective-model composition 层

新增纯 helper / resolver，职责是把 metadata 裁剪成当前 API key 可用集合。该层是后端单一事实源，供 `/api/token/opencode/openai-models` 和 `/config-guides/...` 共同调用。

输入：

- 当前 API key 或当前登录用户拥有的 token id。
- 当前用户 / group 可用 OpenAI 模型。
- metadata models。
- client 类型：`opencode` / `omp`。

API key 校验：

- 登录态 metadata API 推荐使用 `token_id`：`GET /api/token/opencode/openai-models?token_id=<id>`，后端验证 token 属于当前登录用户，并从数据库读取真实 token 字段。
- 如实现保留 `api_key` 参数，也必须验证该 key 属于当前登录用户；不得返回其他用户 token 的 effective set。
- public `/config-guides/...` 端点必须独立校验 query `api_key`，不能只依赖登录态。
- 三条入口（登录态 token_id、登录态 api_key、public api_key）必须复用同一套 config-guide token 可用性校验：归属、状态、过期、用户状态、token group 可用性 / 废弃分组、AllowIps 边界。
- AllowIps 默认按真实请求来源校验；若实现因代理链无法可靠判断而选择不校验，必须在实现计划中写明理由并补安全测试，确保不会比现有 TokenAuth 更宽。
- group 不可用、group 不存在或不属于用户可用分组时 fail-closed，不得用该 group 推导 effective models。
- invalid key 返回 401；disabled、user inactive、expired、group 不可用、IP 不允许返回 403；metadata unavailable 返回 503。
- quota exhausted 判定只基于可在 config-guide 阶段确定的 token 状态：`TokenStatusExhausted` 返回 429；`TokenStatusEnabled` 即使 `RemainQuota=0` 也保持兼容并允许生成配置，不在本阶段推断钱包 / 订阅额度。
- API key 校验失败时 fail-closed，不返回 raw metadata，不返回成功 manifest/config。

有效模型推导：

- 若 token 启用 `ModelLimitsEnabled`，优先使用 token `ModelLimits`。
- 否则使用 token `Group`；token group 为空时使用用户 group。
- 若最终 group 为 `auto`，按用户可用自动分组并集取模型；否则取该 group 启用模型。
- 可用模型按现有 `ListModels` 语义过滤未配置计费的模型，除非用户设置允许 unset ratio model。
- 默认模型选择固定优先级：默认模型优先 `gpt-5.5`，小模型优先 `gpt-5.4-mini`；若小模型缺失则回退到默认模型；默认模型不在 effective set 时 fail-closed，不用任意第一个模型静默替代。
- normalize available IDs：trim、strip `-Sys`、去重、排序。
- 对 metadata 模型做交集匹配时也 strip `-Sys`。
- OpenCode：若 available 包含 fast alias base，则扩展允许 `base-fast`。
- OMP：本次不合成 fast alias；若 OMP schema 不能无损表达 per-model provider options / headers，则明确剔除 `-fast` 并用测试锁定，不保留无输出路径的 fast 常量。
- `-Sys` 只作为历史 / 参考实现输入兼容被剥离，不得出现在 OpenCode effective set 或 OpenCode 输出；普通 OMP 配置也不生成虚拟 `-Sys` role。
- 缺少必需模型时 fail-closed，不生成成功配置。

测试 seam：

- 定义可替换的 available-model resolver 或纯 helper，并提供 `Set...ForTest` 测试 hook。
- 表驱动覆盖 token model limits、token group 覆盖用户 group、auto group 并集、group ability、unset-ratio 过滤、`-Sys` 归一化、OpenCode fast 扩展、OMP 不扩展 fast、必需模型 fail-closed。

### 3. OpenCode renderer

文件：`controller/config_guide.go`。

OpenCode 动态配置内容只由后端端点渲染。`web/default/src/features/api-help/lib/usage-config.ts` 不再作为 OpenCode / OMP 成功路径 renderer，只保留 manifest URL、普通手动片段等与后端动态配置无关的 helper。

最终输出：

- `$schema: https://opencode.ai/config.json`
- `provider.new-api.npm: @ai-sdk/openai`
- `provider.new-api.options.baseURL`
- `provider.new-api.options.apiKey`
- `provider.new-api.models`
- top-level `model`
- top-level `small_model`
- `agent.build.options.store=false`
- `agent.plan.options.store=false`

模型输出 allowlist：

- `id`
- `name`
- `family`
- `attachment`
- `reasoning`
- `tool_call`
- `temperature`
- `knowledge`
- `interleaved`
- `modalities`
- `cost`
- `limit`
- `release_date`
- sanitized `options`
- `headers`
- `variants`

明确不输出：

- `structured_output`（当前 OpenCode schema 不接受）。
- `experimental`
- `provider`
- `tools`
- `builtin_tools`
- `options.builtin_tools`
- `metadata`
- `metadata.builtin_tools`
- `web_search`
- `imageGeneration`
- `variants.image`
- `agent.image`
- `image_generation`

Fast 输出规则：

- map key 是 `gpt-*-fast`。
- model `id` 是去掉 `-fast` 的 base id。
- `options` 包含 sanitized materialized mode options + `store:false`。
- `headers` 保留 materialized mode headers。

Reasoning variants：

- `gpt-5-pro`：无 variants。
- Codex 特例：按参考实现生成固定 levels。
- 普通 reasoning 模型：默认 `low`、`medium`、`high`。
- `gpt-5` 族追加 `minimal`。
- `release_date >= 2025-11-13` 追加 `none`。
- `release_date >= 2025-12-04` 追加 `xhigh`。

### 4. OMP renderer

本次 API Help 只保留普通 OMP Responses provider 配置，内容只由后端端点渲染：

- `models.yml`
- `config.yml`

OMP 模型集合规则：

- 使用后端 effective set。
- 不在 OMP renderer 中合成 fast alias。
- 如果 effective set 中已有 `-fast`，只有在 OMP 配置 schema 能保留该模型所需 options / headers 时才输出；否则明确剔除并补测试说明这是有意行为。
- 不保留无输出路径的 `configGuideFastModelID` / `FAST_AGENT_MODEL_ID` 常量。
- 普通 OMP 配置不生成虚拟 `-Sys` role。

暂不在 API Help 成功路径输出：

- `plugin.txt`
- `image-generator.md`
- `sub2api-openai-image`
- `compat.openaiProviderTools.enabled`
- `compat.openaiProviderTools.imageGeneration`
- `configure-image-agent`

现有 `plugin.txt` / `image-generator.md` handler 和 route 必须删除 / 注销，或改为 404 / 410 非成功响应，并补路由测试锁定。普通 OMP 成功路径不得读取 npm latest / provider-tools metadata，也不得因 provider-tools metadata 不可用而失败。

如果后续明确恢复 OMP 图像能力，应作为 OMP 专属扩展重新加入，并保持与 OpenCode 完全隔离。

### 5. Manifest 和一句话配置

Manifest 端点：

- `GET /config-guides/opencode-openai/manifest.json`
- `GET /config-guides/opencode-openai/opencode.json`
- `GET /config-guides/omp-openai/manifest.json`
- `GET /config-guides/omp-openai/models.yml`
- `GET /config-guides/omp-openai/config.yml`

规则：

- 所有响应设置 `Cache-Control: no-store` 和 `Pragma: no-cache`。
- `api_key` 必填。
- `base_url` 可选；只允许 http / https，不允许 userinfo、query、fragment、控制字符。
- 未显式传 `base_url` 时，从 request host + `X-Forwarded-Proto` 推导 `/v1`。
- manifest items 中不要冗余带 `base_url`，除非用户显式提供。
- OpenCode manifest 只包含 `opencode.json` item；OMP manifest 只包含 `models.yml` 和 `config.yml` item。

一句话配置：

```text
Use this manifest to configure OpenCode: https://example.com/config-guides/opencode-openai/manifest.json?api_key=sk-...
```

UI 必须完整显示这句和复制按钮，不内嵌 manifest 内容。复制按钮的 value 必须与可见文本完全一致。

### 6. 前端面板布局

文件：`web/default/src/features/api-help/components/api-usage-help-dialog.tsx`

布局模型：

```text
DialogContent: flex column, max-h 92vh, overflow hidden
  DialogHeader: shrink-0
  Base URL / API key selector: shrink-0
  Body wrapper: min-h-0 flex-1 overflow-hidden
    ScrollArea root: h-full min-h-0
      Tabs
      Config blocks
  DialogFooter: shrink-0
```

约束：

- 不混用 grid override 和 flex override。
- 必须有 body wrapper，不能直接让长内容决定 DialogContent 高度。
- TabsList 外层允许横向滚动或 wrap，但不得撑宽弹窗。
- Key selector 窄屏占满宽度，不能用固定最小宽度撑破布局。
- code block 内部 `max-height` + `overflow:auto`。
- OpenCode tab：一句话配置 + 一个从后端 `opencode.json` 端点加载的只读 block。
- OMP tab：一句话配置 + 从后端 `models.yml` / `config.yml` 端点加载的两个 block。

Metadata gating：

- React Query `queryKey` 必须包含 selected token id 或 normalized API key。
- 未选中 API key 时不发 metadata 请求。
- API key 变化时必须进入 loading / unavailable 状态，不得继续展示上一个 key 的 ready 一句话或配置 block。
- 过期 in-flight 响应不得覆盖当前 key。
- loading、unavailable、missing required model 时不得渲染成功配置 block，也不得用真实 API key 生成 fallback 成功配置；只能显示加载 / 错误说明。

## 测试策略

### 后端测试

- `service/opencode_metadata_test.go`
  - fast mode materialization。
  - provider body camelCase。
  - provider headers 保留。
  - stale cache fallback。
- `controller/token_test.go`
  - `/api/token/opencode/openai-models` missing token_id、其他用户 token_id、disabled、expired、exhausted、metadata unavailable。
  - 成功响应返回 effective set，不返回 raw metadata，也不返回 provider-tools metadata。
  - endpoint 必须复用 config-guide token 可用性校验和状态码映射。
  - 若实现保留登录态 `api_key` 兼容参数，必须同样覆盖 missing/foreign/disabled/expired/exhausted/metadata unavailable；若不保留该参数，必须测试它返回 400 且不泄露 effective set。
- `controller/config_guide_test.go`
  - API key 校验失败 fail-closed：invalid=401、disabled/user inactive/expired/group 不可用/IP 不允许=403、TokenStatusExhausted=429。
  - `TokenStatusEnabled` 且 `RemainQuota=0` 仍可生成配置，保持现有订阅 / 钱包兼容。
  - API key 可用模型交集裁剪。
  - token model limits、token group 覆盖用户 group、auto group 并集、group ability、unset-ratio 过滤、`-Sys` 归一化。
  - OpenCode fast alias 扩展。
  - OMP 不扩展 fast alias。
  - OpenCode 输出无 `-Sys`。
  - OpenCode 序列化结果不包含 `web_search`、`metadata`、`builtin_tools`、`image_generation`、`variants.image`、`agent.image`。
  - OMP manifest / models.yml / config.yml 在 provider-tools metadata unavailable 或被调用即失败的 stub 下仍成功。
  - OMP manifest 不再包含 plugin / image-generator 成功项；OMP 输出不包含 `plugin`、`image-generator`、`openaiProviderTools`、`new-api-image`、`imageGeneration`。
  - invalid `base_url` 拒绝。
  - manifest、opencode.json、models.yml、config.yml 都断言 `Cache-Control: no-store` 与 `Pragma: no-cache`。
  - 表驱动覆盖 foreign key、control-character key、`sk-sk-` 正常归一化、错误响应不回显 secret。
- `router/config_guide_route_test.go`
  - `/config-guides/...` 在 SPA fallback 前返回 JSON / YAML 错误。
  - `plugin.txt` / `image-generator.md` 不再作为成功路由返回。

### 前端测试

- `web/default/src/features/keys/api.ts` / 相关测试
  - `getOpenCodeOpenAIModels(tokenId)` 必须序列化 `token_id`。
- `web/default/src/features/api-help/lib/usage-config.test.ts`
  - manifest URL 安全生成。
  - 普通手动片段仍可生成，但不作为 OpenCode / OMP 动态配置成功路径。
- 动态配置后端 / 前端契约测试
  - 使用稳定小 fixture（default、small、fast headers/options、禁用字段）。
  - 后端断言 provider / models / model / small_model / agent 子集。
  - 前端断言显示的是后端 artifact 与同一句 manifest instruction。
  - 不比较 `generated_at`、完整 YAML 文本或全量 JSON 字符串。
- 新增或扩展组件 / helper 测试：
  - 不新增 DOM 测试依赖时，从 `api-usage-help-dialog.tsx` 抽出纯 helper / hook：selected key 派生、metadata query key 构造、artifact query key 构造、metadata state reducer、stale response guard、一句话 instruction / block 组装。
  - 用 `node:test` / `bunx tsx --test` 覆盖这些 helper；源码 smoke 仅锁定 body wrapper / ScrollArea / footer class，不做脆弱 DOM 全量快照。
  - 如实现计划决定新增 React Testing Library / jsdom / happy-dom，必须明确新增依赖、测试文件和命令，并仍保持测试只断言行为契约。
  - OpenCode tab 显示可见的一句话配置，复制值等于可见文本，并展示一个 `opencode.json` block。
  - OMP tab 显示可见的一句话配置，复制值等于可见文本，并展示两个 block。
  - metadata 缺失时不显示成功的一句话配置或成功配置 block。
  - 切换 API key 后重新请求 metadata，旧 key 的 ready 状态和 in-flight 响应不能覆盖当前 key。
  - artifact fetch 的 queryKey 必须包含 selected token id 或 normalized API key、client、file path / base_url，且只在当前 key metadata ready 时启用。
  - 窄屏下 footer 不被滚动内容挤出；结构断言锁定 body wrapper / ScrollArea 高度类；若使用 `DialogContent p-0 overflow-hidden`，`DialogFooter` 需覆盖默认负 margin 或使用匹配默认 padding 的容器。

### i18n 验证

- 所有新增用户可见文案必须通过 `t()`。
- 更新 `web/default/src/i18n/locales/{en,zh,fr,ja,ru,vi}.json`。
- 如新增静态 key 需要扫描登记，更新对应 static keys 文件。
- `bun run i18n:sync` 后所有 locale `missingCount=0`、`extrasCount=0`。
- 本次新增 / 修改的 key 在六种 locale 中不得新增 untranslated；既有 untranslated 计数可保持不变。

## 实现计划拆分约束

- 计划阶段按低冲突边界拆分：后端 token/effective resolver、后端 OpenCode renderer、后端 OMP cleanup/routes、前端 API/helper、前端 dialog/layout/i18n、最终审查。
- 共享文件 `controller/config_guide.go`、`controller/token.go`、`router/config-guide-router.go`、`web/default/src/features/api-help/components/api-usage-help-dialog.tsx` 不得由多个子代理并发编辑。
- 子代理不运行项目级验证；主代理最终统一运行定向 Go 测试、前端 node:test / typecheck / build / i18n sync。
- 提交前增加静态搜索检查：不得出现参考仓库路径、私有 repo 名、真实 key、OpenCode `builtin_tools` / `image_generation` 旧语义。

## 验收标准

- OpenCode 动态配置只包含当前 API key 可用模型与对应 fast alias。
- OpenCode 配置可被 OpenCode 解析，并使用 `@ai-sdk/openai` provider。
- OpenCode 配置不包含 `builtin_tools`、`web_search`、`metadata`、`image_generation`、`variants.image`、`agent.image`。
- OMP API Help 成功路径不再输出 plugin / image-generator / image provider / provider-tools compat。
- 一句话配置可见且可复制；复制值等于可见文本；AI agent 拉到真实 manifest / config 文件，不会拿到 SPA HTML。
- 弹窗在窄屏和长配置内容下 footer 可见，内容区正常滚动。
- i18n locale 无 missing / extras；本次新增 / 修改 key 无新增 untranslated。
