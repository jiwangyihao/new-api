# Issue #4 登录欢迎公告弹窗规格

> 面向 AI 代理的工作者：本规格用于处理 GitHub fork 仓库 `jiwangyihao/new-api` 的 Issue #4「登录/注册后：新增欢迎公告弹窗」。实现前必须读取仓库根目录 `AGENTS.md` 与 `web/default/AGENTS.md`，并遵守 Go + Gin、React 19、TypeScript、Bun、i18n、Base UI、Tailwind CSS 以及项目受保护标识约束。

**目标：** 新增一个由管理员可配置的「进入系统欢迎公告」弹窗。用户完成认证并进入 `_authenticated` 系统区域后看到公告；管理员可在系统设置中启用、关闭、编辑文案和选择展示频率。

**架构：** 复用现有 `console_setting` 配置管理和系统设置内容页表单；新增受 `UserAuth` 保护的欢迎公告读取接口；前端在 `AuthenticatedLayout` 挂载 `WelcomePopupGate`，通过 React Query 获取登录用户可见配置，并用用户维度 localStorage + 当前 JS 会话内存兜底记录关闭状态。

**技术栈：** Go、Gin、GORM 配置表、React 19、TypeScript、TanStack Router、TanStack Query、React Hook Form、Zod、Base UI、Tailwind CSS、Bun。

---

## 1. 背景

GitHub Issue #4 要求：用户登录或注册进入系统后弹出欢迎公告，文案为：

```text
欢迎使用赔钱GPT！

欢迎邀请好友使用赔钱GPT，邀请两位好友每月付费订阅，即可免费享受“一瓶盖可乐”付费订阅！

填写邀请码或试用码可享用24小时GPT5.5畅用！

官方QQ群：1106020227
```

用户补充指出：还需要注意「设置中的可编辑性」。因此本功能不能只在前端硬编码文案，而必须作为运营配置进入系统设置。

当前密码注册流程不会自动登录：`sign-up-form.tsx` 注册成功后提示用户去登录，后端 `Register` 也不建立 session。为避免把注册语义扩大成自动登录，本功能保持现有注册流程不变：**密码注册用户首次登录并进入系统时展示欢迎公告**；OAuth、WeChat、Passkey、OTP 等流程只要最终进入 `_authenticated` 系统区域，就由同一入口展示。

## 2. 决策

1. **新增独立欢迎公告配置。** 不复用 dashboard 公告 `console_setting.announcements`，也不复用全局 Notice。欢迎公告有独立触发时机、关闭状态和展示频率。
2. **管理员可编辑。** 系统设置中新增「欢迎公告弹窗」小节，支持启用开关、内容编辑、展示频率选择和内容预览。
3. **默认内容使用 Issue 文案。** 新安装或无配置时，默认启用并展示 Issue 指定中文文案。
4. **默认频率为内容版本维度每用户一次。** 同一用户关闭某个内容版本后不再反复弹出；管理员修改内容后，所有用户会看到新版本。
5. **触发语义是进入认证系统区域，不是改造所有认证接口。** 弹窗挂载在 `AuthenticatedLayout`，覆盖所有进入 `_authenticated` 的路径；密码注册不自动登录，注册后首次登录时触发。
6. **欢迎公告内容只面向已登录用户接口返回。** 不把管理员可编辑内容放进公开 `/api/status`，避免未登录访问者读取仅面向系统内用户的运营公告。
7. **只支持安全 Markdown，不支持原始 HTML。** 欢迎公告展示和设置页预览都必须使用禁用 raw HTML 的渲染路径；后端危险片段检查只是额外约束，不是主要 XSS 防线。
8. **移动端可用。** 弹窗最大高度受限，正文区域滚动，所有关闭路径都清晰并持久化关闭状态。
9. **不修改受保护项目标识。** 本功能不触碰 README、版权头、项目品牌、组织归属等受保护内容。

## 3. 业务范围

### 3.1 必须满足

- 用户普通登录进入系统后能看到欢迎公告弹窗。
- 密码注册用户在注册成功后首次登录进入系统时能看到欢迎公告弹窗。
- OAuth、WeChat、Passkey、OTP 等最终进入 `_authenticated` 区域的路径也走同一触发逻辑。
- 公告默认文案与 Issue 文案一致。
- 管理员可启用或禁用欢迎公告弹窗。
- 管理员可编辑欢迎公告内容。
- 管理员可选择展示频率。
- 管理员可在设置页预览欢迎公告正文的渲染效果。
- 设置保存后必须刷新相关查询；当前会话再次读取时使用最新配置。
- localStorage 关闭状态必须按 userId 隔离。
- 弹窗有明确关闭按钮。
- 右上角关闭、底部按钮、Escape、`onOpenChange(false)` 等所有关闭路径都必须调用同一个关闭持久化流程。
- 移动端内容不溢出屏幕；长内容可滚动。
- 前端用户可见 UI 标签和说明遵守 i18n 规则。
- 更新 TypeScript / TSX 后必须运行 `bun run typecheck`。

### 3.2 非目标

- 不实现后端按用户记录已读状态；关闭状态保存在浏览器本地，并有当前 JS 会话内存兜底。
- 不实现富文本编辑器；本次使用 textarea。
- 不支持原始 HTML；配置内容按安全 Markdown 渲染，HTML 按文本处理或被 Markdown 渲染器忽略。
- 不改造现有通知中心、dashboard 公告列表或 Notice 弹窗语义。
- 不改造密码注册为自动登录。
- 不强制管理员必须配置后才能启用；默认配置即可满足 Issue。
- 不新增第三方依赖。
- 不实现远程推送、WebSocket 实时公告或多端已读同步。

## 4. 当前代码基线

已确认的关键文件和现有能力：

- `setting/console_setting/config.go`
  - 已有 `ConsoleSetting`，通过 `config.GlobalConfig.Register("console_setting", &consoleSetting)` 注册。
  - 已有 `ApiInfo`、`Announcements`、`FAQ`、`UptimeKumaGroups` 及对应启用开关。
- `setting/console_setting/validation.go`
  - 已有 `ValidateConsoleSettings`。
  - 已有危险内容检查 `checkDangerousContent`，但它不能作为 HTML/XSS 的唯一防线。
- `setting/config/config.go`、`model/option.go`
  - `config.GlobalConfig` 可按结构体 `json` tag 生成 `console_setting.<key>` 默认配置。
  - 本功能不新增表或列，不需要数据库 schema migration；缺省时依赖 `defaultConsoleSetting` 与 `ExportAllConfigs()` 暴露运行时默认值，管理员首次保存后写入 option 表。
- `router/api-router.go`
  - `/api/status` 是公开接口，不适合返回管理员可编辑的登录后公告正文。
  - `/api/user` 下已有 `selfRoute` 使用 `middleware.UserAuth()`，适合新增已登录用户读取欢迎公告配置接口。
- `controller/option.go`
  - 更新 `console_setting.*` 配置时已有集中校验入口。
- `web/default/src/features/system-settings/content/index.tsx`
  - 内容设置页集中读取 `system-options` 并组装 `ContentSettings`。
- `web/default/src/features/system-settings/content/section-registry.tsx`
  - 内容设置左侧分区注册在 `CONTENT_SECTIONS`。
- `web/default/src/features/system-settings/content/announcements-section.tsx`
  - 可作为系统设置内容编辑表单、启用开关、保存按钮的参考。
- `web/default/src/components/ui/markdown.tsx`
  - 现有 `Markdown` 使用 `rehypeRaw`，允许原始 HTML；欢迎公告不能直接复用这个组件作为安全渲染路径。
- `web/default/src/components/notification-dialog.tsx`
  - 已有 Base UI Dialog + ScrollArea 的通知弹窗实现。当前通知弹窗由按钮手动打开，不会自动与欢迎公告抢焦点。
- `web/default/src/components/layout/components/authenticated-layout.tsx`
  - 认证区域公共布局，是进入系统后统一触发欢迎公告的合适挂载点。
- `web/default/src/stores/auth-store.ts`
  - 可读取当前用户 ID，用于每用户关闭状态隔离。
- `web/default/src/features/auth/hooks/use-auth-redirect.ts`
  - 登录成功后会设置 auth store 并导航到目标页面。
- `web/default/src/features/auth/sign-up/components/sign-up-form.tsx`
  - 密码注册成功后跳转登录页，不自动进入系统。
- `web/default/package.json`
  - 没有单独 test script 和 React Testing Library 依赖；现有前端测试主要用 `node:test` / `bun test <file>` 验证纯逻辑或源代码 smoke。
- `web/default/src/i18n/static-keys.ts`
  - 动态 i18n key 需要在这里登记，不能只依赖 `bun run i18n:sync` 自动扫描。

## 5. 后端配置与接口设计

### 5.1 ConsoleSetting 新增字段

文件：`setting/console_setting/config.go`

新增字段：

```go
WelcomePopupContent   string `json:"welcome_popup_content"`
WelcomePopupEnabled   bool   `json:"welcome_popup_enabled"`
WelcomePopupFrequency string `json:"welcome_popup_frequency"`
```

默认配置：

```go
WelcomePopupContent: `欢迎使用赔钱GPT！

欢迎邀请好友使用赔钱GPT，邀请两位好友每月付费订阅，即可免费享受“一瓶盖可乐”付费订阅！

填写邀请码或试用码可享用24小时GPT5.5畅用！

官方QQ群：1106020227`,
WelcomePopupEnabled: true,
WelcomePopupFrequency: "once_per_version",
```

字段语义：

- `welcome_popup_content`：安全 Markdown 文本，存储欢迎公告正文。
- `welcome_popup_enabled`：是否启用进入系统欢迎公告。
- `welcome_popup_frequency`：展示频率。

允许的展示频率：

| 值 | 用户语义 | 关闭后 | 内容变化后 |
|---|---|---|---|
| `once_per_version` | 每个用户对同一内容版本只展示一次。默认值。 | 同一内容 hash 不再展示。 | 新内容 hash 重新展示。 |
| `once_per_day` | 每个用户每天最多展示一次，但新内容优先。 | 同一天同一内容不再展示。 | 即使同一天已关闭旧内容，新内容 hash 也重新展示。 |
| `every_session` | 每次进入一个新的认证系统会话最多展示一次。 | 同一 `AuthenticatedLayout` 挂载周期内不再自动展示。 | 同一挂载周期不因内容变化重新打开；刷新页面或重新进入系统后再按最新内容展示。 |

`every_session` 的可测试定义：用户完成认证并进入 `_authenticated` 区域后，`AuthenticatedLayout` 的一次挂载周期最多自动打开一次。普通 `_authenticated` 子路由切换不算新的系统会话；页面刷新、重新打开标签页、退出后重新登录并重新挂载认证布局算新的系统会话。

### 5.2 Go 常量与校验

文件：`setting/console_setting/validation.go`

新增常量和辅助函数，避免保存校验与接口兜底分叉：

```go
const (
    WelcomePopupFrequencyOncePerVersion = "once_per_version"
    WelcomePopupFrequencyOncePerDay     = "once_per_day"
    WelcomePopupFrequencyEverySession   = "every_session"
    DefaultWelcomePopupFrequency        = WelcomePopupFrequencyOncePerVersion
)

func NormalizeWelcomePopupFrequency(frequency string) string
func ValidateWelcomePopupContent(content string) error
func ValidateWelcomePopupFrequency(frequency string) error
```

规则：

- 内容为空允许保存；但前端触发时空内容不弹出。
- 内容长度不超过 2000 个 Unicode 字符；后端用 rune 计数，前端提示与后端保持一致。
- 内容不得包含现有 `dangerousChars` 中的危险片段；这是额外输入约束，不是主要 XSS 防线。
- 频率只能是 `once_per_version`、`once_per_day`、`every_session`。
- 非法频率保存失败，返回明确错误。
- 输出给用户端时，如果存量配置为空或非法，`NormalizeWelcomePopupFrequency` 兜底为 `once_per_version`。

`controller/option.go` 新增 key 分支：

- `console_setting.welcome_popup_content`
- `console_setting.welcome_popup_frequency`

布尔开关 `console_setting.welcome_popup_enabled` 按现有通用更新流程保存即可。

### 5.3 已登录用户读取接口

新增受保护接口：

```text
GET /api/user/welcome-popup
```

路由位置：`router/api-router.go` 的 `selfRoute` 组内，必须经过 `middleware.UserAuth()`。

响应示例：

```json
{
  "success": true,
  "message": "",
  "data": {
    "enabled": true,
    "content": "欢迎使用赔钱GPT！\n\n...",
    "frequency": "once_per_version"
  }
}
```

输出规则：

- 未登录用户不能调用成功。
- 始终返回 `enabled` 和归一化后的 `frequency`。
- 仅在启用且内容 trim 后非空时返回非空 `content`；禁用或内容为空时 `content` 返回空字符串。
- 接口不返回管理员配置以外的用户敏感信息。
- 不在公开 `/api/status` 中返回 `welcome_popup_content`。

### 5.4 系统设置读取

`/api/option` 读取配置时，`config.GlobalConfig` 会按 `console_setting.<json tag>` 暴露新增字段。前端内容设置页应将这些字段纳入 `ContentSettings` 默认值。

## 6. 前端设置页设计

### 6.1 新增内容设置分区

文件：`web/default/src/features/system-settings/content/section-registry.tsx`

新增分区：

- `id`: `welcome-popup`
- 标题 key：`Welcome Popup`
- 描述 key：`Configure login welcome announcement popup`

路由无需新增文件，现有 `/_authenticated/system-settings/content/$section` 会根据 `CONTENT_SECTION_IDS` 校验分区。本功能不新增 `src/routes` 文件时无需更新 `routeTree.gen.ts`；如果实现过程中新增或删除 file route，则必须重新生成并提交 `routeTree.gen.ts`。

### 6.2 新增设置组件与纯逻辑模块

新增文件：

- `web/default/src/features/system-settings/content/welcome-popup-section.tsx`
- `web/default/src/features/system-settings/content/welcome-popup-form.ts`

组件职责：

- 展示启用开关。
- 编辑欢迎公告正文 textarea。
- 选择展示频率 select。
- 保存内容和频率。
- 必须提供预览区或预览按钮，按用户端安全 Markdown 规则渲染正文。
- 设置页说明必须明确：欢迎公告内容只会通过登录后的接口返回给已认证用户；内容支持 Markdown，不支持原始 HTML。

纯逻辑模块职责：

```ts
export const welcomePopupFormSchema = z.object({ ... })
export function buildWelcomePopupFormDefaults(options: WelcomePopupOptionValues): WelcomePopupFormValues
export function collectWelcomePopupSettingUpdates(values, defaults): UpdateOptionRequest[]
```

这些函数用于当前仓库可执行的 `node:test` / `bun test` 单元测试，避免依赖未安装的 React Testing Library。

表单字段：

```ts
type WelcomePopupFrequency = 'once_per_version' | 'once_per_day' | 'every_session'

type WelcomePopupFormValues = {
  content: string
  frequency: WelcomePopupFrequency
}
```

Zod 规则：

- content：`string().max(2000)`，提示与后端 Unicode 字符限制一致。
- frequency：枚举上述 3 个值。

保存规则：

- 启用开关变化时立即调用 `useUpdateOption` 保存 `console_setting.welcome_popup_enabled`。
- 点击保存时，通过 `collectWelcomePopupSettingUpdates` 只提交发生变化的 `console_setting.welcome_popup_content` 和 `console_setting.welcome_popup_frequency`。
- 任一保存失败时显示现有 toast 错误，不吞掉错误。
- 保存成功后刷新 `system-options`；如果当前登录用户读取过欢迎公告，也应使 `['welcome-popup']` 查询失效，保证当前会话下一次读取使用新配置。

### 6.3 ContentSettings 类型扩展

文件：`web/default/src/features/system-settings/types.ts`

新增字段：

```ts
export type WelcomePopupFrequency =
  | 'once_per_version'
  | 'once_per_day'
  | 'every_session'

'console_setting.welcome_popup_content': string
'console_setting.welcome_popup_enabled': boolean
'console_setting.welcome_popup_frequency': WelcomePopupFrequency
```

文件：`web/default/src/features/system-settings/content/index.tsx`

`defaultContentSettings` 新增默认值，必须与后端默认一致。

## 7. 用户端弹窗设计

### 7.1 安全 Markdown 渲染

新增文件：

`web/default/src/components/ui/safe-markdown.tsx`

或在现有 Markdown 组件中新增明确的安全模式。无论实现路径如何，欢迎公告必须满足：

- 使用 `react-markdown` + `remark-gfm`。
- 不使用 `rehypeRaw`。
- 不使用 `dangerouslySetInnerHTML`。
- 链接打开新窗口时加 `rel='noopener noreferrer'`。
- 链接协议只允许安全协议；至少禁止 `javascript:`、`data:` 等危险协议。
- 设置页预览和用户端弹窗使用同一个安全 Markdown 渲染路径。

### 7.2 新增弹窗组件

新增文件：

`web/default/src/features/welcome-popup/components/welcome-popup-dialog.tsx`

组件职责：

- 使用项目现有 `Dialog`、`DialogContent`、`DialogHeader`、`DialogTitle`、`DialogDescription` 或等价 aria 关联、`DialogFooter`、`Button`、`ScrollArea`。
- 标题显示 `Welcome` / `欢迎`。
- 正文渲染配置内容。
- 底部提供明确关闭按钮：`I know`。
- 移动端约束：`DialogContent` 包含 `max-h-[90vh]`，正文滚动区域包含受限高度，例如 `max-h-[60vh]`。
- 保留 Dialog 默认焦点管理和 Escape 关闭能力。
- 所有关闭路径必须调用传入的同一个 `onClose`：底部按钮、右上角关闭、Escape、外部导致的 `onOpenChange(false)`。

### 7.3 新增 API 与状态逻辑

新增文件：

- `web/default/src/features/welcome-popup/api.ts`
- `web/default/src/features/welcome-popup/types.ts`
- `web/default/src/features/welcome-popup/lib/welcome-popup-state.ts`
- `web/default/src/features/welcome-popup/hooks/use-welcome-popup.ts`

API：

```ts
export async function getWelcomePopup(): Promise<WelcomePopupConfig>
```

调用：`GET /api/user/welcome-popup`。

Hook 输入数据：

- 当前用户 ID：来自 `useAuthStore((state) => state.auth.user?.id)`。
- 欢迎公告配置：来自 React Query `['welcome-popup', userId]`。
- 查询必须等 userId 存在后才启用。
- 不基于公开 `/api/status` 的 localStorage placeholder 弹出，避免旧缓存导致过期公告先弹。

Hook 输出：

```ts
type UseWelcomePopupResult = {
  open: boolean
  content: string
  close: () => void
}
```

本地存储 key：

```text
welcome-popup-state:v1:<userId>
```

持久化内容：

```json
{
  "lastContentHash": "...",
  "lastClosedDate": "YYYY-MM-DD"
}
```

内存兜底：

- 新增模块级非持久化 Map，按 `userId + frequency + contentHash/date` 记录当前 JS 会话内已关闭状态。
- localStorage 读写失败、QuotaExceeded、隐私模式等异常必须吞掉，并写入内存兜底。
- 展示判定时，只要 localStorage 或内存兜底任一命中关闭状态，就不展示。

纯状态函数：

```ts
export function hashWelcomePopupContent(content: string): string
export function createWelcomePopupStorageKey(userId: number | string): string
export function getLocalDateKey(now: Date): string
export function shouldShowWelcomePopup(input: ShouldShowWelcomePopupInput): boolean
export function buildClosedState(input: BuildClosedStateInput): WelcomePopupClosedState
```

展示规则：

| 频率 | 展示条件 | 关闭写入 |
|---|---|---|
| `once_per_version` | 用户已登录、启用、内容非空，且关闭状态中的 `lastContentHash` 不等于当前内容 hash。 | 写入当前 hash 和本地日期。 |
| `once_per_day` | 用户已登录、启用、内容非空，且当前内容 hash 不同，或关闭日期不是浏览器本地今天。 | 写入当前 hash 和浏览器本地日期。 |
| `every_session` | 用户已登录、启用、内容非空，且当前 `AuthenticatedLayout` 挂载周期尚未自动展示过。 | 设置当前挂载周期已关闭/已展示标记；不写入永久阻止下次系统会话的状态。 |

同一 `AuthenticatedLayout` 挂载周期内，关闭后即使 React 重渲染、auth store 更新、`welcome-popup` 查询 refetch，也不得自动重新打开。实现可以使用 `shownThisMountRef` 或等价门禁。

浏览器日期口径：`once_per_day` 使用浏览器本地日期 `YYYY-MM-DD`，测试中必须通过可注入 `Date` 覆盖跨日判断。

### 7.4 挂载位置

新增文件：

`web/default/src/features/welcome-popup/index.tsx`

导出 `WelcomePopupGate`，内部调用 `useWelcomePopup()` 并渲染 `WelcomePopupDialog`。

修改文件：`web/default/src/components/layout/components/authenticated-layout.tsx`

在布局中挂载：

```tsx
<WelcomePopupGate />
```

挂载原则：

- 放在 `AuthenticatedLayout` 内部，确保所有认证页面覆盖。
- 不阻塞页面渲染，不影响 sidebar、header、main 内容加载。
- 子路由切换不得导致同一挂载周期重复弹出。

## 8. i18n 设计

默认公告内容属于后端运营配置默认值，不通过 `t()` 翻译，不加入 `static-keys.ts`，不作为 UI key 强制翻译。非中文 locale 下仍展示管理员配置原文；管理员可自行编辑为目标语言。

以下用户可见 UI key 必须同步到所有 locale，并按使用方式登记：

- 若 key 通过 `t('...')` 字面量使用，i18n 工具可扫描。
- 若 key 通过配置、常量、`titleKey`、`descriptionKey` 或动态 label 传入 `t()`，必须加入 `web/default/src/i18n/static-keys.ts`。

必须覆盖的 key：

- `Welcome Popup`
- `Configure login welcome announcement popup`
- `Welcome announcement content`
- `Display frequency`
- `Show once per content update`
- `Show once per day`
- `Show every system session`
- `Save welcome popup`
- `Welcome`
- `I know`
- `Preview`
- `This popup appears after users enter the authenticated system area.`
- `Markdown is supported. Raw HTML is not supported.`
- `This content is returned only to authenticated users.`
- `Welcome popup content must be at most 2000 characters.`

实现后从 `web/default/` 运行：

```bash
bun run i18n:sync
```

`bun run i18n:sync` 不会自动发现所有源码 key；实现者必须手动检查并补齐：

- `web/default/src/i18n/static-keys.ts`
- `web/default/src/i18n/locales/en.json`
- `web/default/src/i18n/locales/zh.json`
- `web/default/src/i18n/locales/fr.json`
- `web/default/src/i18n/locales/ja.json`
- `web/default/src/i18n/locales/ru.json`
- `web/default/src/i18n/locales/vi.json`

## 9. 测试设计

### 9.1 前端纯逻辑测试

新增测试：

`web/default/src/features/welcome-popup/lib/welcome-popup-state.test.ts`

使用 `node:test` / `bun test`，覆盖：

- 禁用时不展示。
- 内容为空时不展示。
- 未登录 / 无 userId 不展示。
- `once_per_version` 首次展示。
- `once_per_version` 同内容关闭后不展示。
- `once_per_version` 内容变更后重新展示。
- `once_per_day` 同一天同内容关闭后不展示。
- `once_per_day` 次日重新展示。
- `once_per_day` 同一天内容变更后重新展示。
- `every_session` 在同一挂载周期只自动展示一次。
- 不同 userId 的关闭状态互不影响。
- localStorage get/set 抛错后，内存兜底能保证关闭后同一 JS 会话不再重复展示。
- 日期使用可注入 `Date`，不依赖测试运行当天。

### 9.2 设置页纯逻辑与 smoke 测试

新增测试：

`web/default/src/features/system-settings/content/welcome-popup-form.test.ts`

覆盖：

- 构建默认内容和默认频率。
- 收集内容变化时提交 `console_setting.welcome_popup_content`。
- 收集频率变化时提交 `console_setting.welcome_popup_frequency`。
- 未变化时不提交无关更新。
- 超过 2000 字符时 schema 拒绝。
- 非法频率时 schema 拒绝。

新增或扩展源代码 smoke 测试：

- 断言 `content/section-registry.tsx` 包含 `welcome-popup` 分区。
- 断言 `content/index.tsx` 的默认设置包含 3 个欢迎公告配置 key。
- 断言 `use-update-option.ts` 或等价失效逻辑包含欢迎公告相关刷新处理。
- 断言 `welcome-popup-section.tsx` 使用安全 Markdown 预览，而不是 raw HTML Markdown。

不要求引入 React Testing Library、jsdom、happy-dom 或 Vitest。

### 9.3 认证布局挂载与弹窗 smoke 测试

新增测试：

`web/default/src/features/welcome-popup/welcome-popup-layout.test.ts`

使用源代码 smoke + 纯逻辑测试覆盖：

- `authenticated-layout.tsx` 导入并渲染 `WelcomePopupGate`。
- `WelcomePopupGate` 使用 `WelcomePopupDialog`。
- `WelcomePopupDialog` 的 `DialogContent` 包含 `max-h-[90vh]`。
- 正文区域使用 `ScrollArea` 或等价受限滚动容器。
- `onOpenChange(false)` 调用统一关闭处理。
- 底部关闭按钮调用统一关闭处理。
- 安全 Markdown 渲染路径不包含 `rehypeRaw`。

这些 smoke 测试用于弥补当前仓库没有 DOM 组件测试工具链的限制。

### 9.4 后端测试

新增或扩展 Go 测试：

- `setting/console_setting/validation_test.go`
- `controller/welcome_popup_test.go`
- 必要时扩展 `controller/option_test.go`

覆盖场景：

- 默认配置文案与 Issue #4 完全一致。
- 合法频率通过校验。
- 非法频率被拒绝。
- 危险内容被拒绝。
- 内容长度按 Unicode 字符计数。
- `NormalizeWelcomePopupFrequency` 对空值或非法存量值兜底为 `once_per_version`。
- `GET /api/user/welcome-popup` 未登录时不能成功。
- `GET /api/user/welcome-popup` 登录后返回启用状态、内容和频率。
- 禁用时接口返回空内容。
- 空内容时接口返回空内容。
- `UpdateOption` 对非法 `console_setting.welcome_popup_frequency` 返回失败且不更新。
- 配置导出包含 `console_setting.welcome_popup_content`、`console_setting.welcome_popup_enabled`、`console_setting.welcome_popup_frequency`。

### 9.5 验证命令

实现完成后至少运行：

```bash
# 后端
# 具体 -run 范围可按实际测试名收窄，但必须覆盖 console_setting 和 welcome popup controller。
go test ./setting/console_setting ./controller

# 前端，在 web/default 下运行。
bun test src/features/welcome-popup/lib/welcome-popup-state.test.ts \
  src/features/system-settings/content/welcome-popup-form.test.ts \
  src/features/welcome-popup/welcome-popup-layout.test.ts
bun run i18n:sync
bun run typecheck
```

如果新增或修改了其他测试文件，最终命令必须包含这些文件。

## 10. 验收标准

- 登录后进入系统能看到欢迎公告弹窗。
- 密码注册成功后不改变现有跳转登录行为；该用户首次登录进入系统时能看到欢迎公告弹窗。
- OAuth、WeChat、Passkey、OTP 等进入认证系统区域后能看到欢迎公告弹窗。
- 默认公告文案与 Issue #4 完全一致。
- 欢迎公告内容不通过公开 `/api/status` 返回，未登录用户不能成功读取受保护欢迎公告接口。
- 管理员可以在系统设置中启用或禁用欢迎公告弹窗。
- 管理员可以在系统设置中编辑欢迎公告内容。
- 管理员可以在系统设置中选择展示频率。
- 管理员可以预览欢迎公告正文。
- 设置保存后刷新相关查询；后续读取使用最新配置。
- 默认展示频率为 `once_per_version`：每个用户每次内容更新后显示一次。
- `once_per_day` 在同一天同内容关闭后不再展示，但内容更新后重新展示。
- `every_session` 在同一个认证布局挂载周期最多展示一次，关闭后不因 status refetch、auth store 更新或子路由切换重新打开。
- 所有关闭路径都会持久化或写入内存兜底关闭状态，点击关闭后弹窗关闭并按频率规则生效。
- localStorage 不可用时，关闭后同一 JS 会话内不重复弹出。
- 移动端弹窗内容不溢出屏幕，正文可滚动，关闭按钮可见。
- 欢迎公告按安全 Markdown 渲染，不支持 raw HTML，不使用 `rehypeRaw` 或 `dangerouslySetInnerHTML`。
- localStorage 关闭状态按 userId 隔离。
- TypeScript 类型检查通过。
- 相关前端单元 / smoke 测试通过。
- 相关后端校验 / controller 测试通过。

## 11. 实施顺序建议

1. 后端新增 `console_setting` 字段、默认值、频率常量、校验和受保护读取接口。
2. 前端扩展 `SystemStatus` 以外的必要类型、`ContentSettings`、设置页默认值和系统设置分区。
3. 新增系统设置欢迎公告配置组件和纯逻辑表单模块。
4. 新增安全 Markdown 渲染路径。
5. 新增欢迎公告 API、状态判定纯函数、Hook、Dialog、Gate，并挂载到 `AuthenticatedLayout`。
6. 补齐 i18n static keys 和所有 locale。
7. 编写并运行前后端测试。
8. 运行 `web/default` 下的 `bun run typecheck`。

## 12. 多子代理实施边界建议

后续实现计划应按文件边界降低冲突：

| 任务 | 主要文件 | 依赖关系 |
|---|---|---|
| 后端配置与接口 | `setting/console_setting/*`、`controller/*welcome*`、`controller/option.go`、`router/api-router.go` | 可先行。 |
| 设置页 | `web/default/src/features/system-settings/content/*`、`web/default/src/features/system-settings/types.ts`、`web/default/src/features/system-settings/hooks/use-update-option.ts` | 依赖频率枚举和配置 key。 |
| 用户端弹窗 | `web/default/src/features/welcome-popup/*`、`web/default/src/components/ui/safe-markdown.tsx`、`authenticated-layout.tsx` | 依赖后端接口路径和类型。 |
| i18n 与 smoke 测试 | `web/default/src/i18n/*`、新增测试文件 | 应在 UI 文案稳定后执行，避免多代理反复改 locale。 |

## 13. 风险与边界

- localStorage 是浏览器维度，不是跨设备已读状态；这是本次明确非目标。
- 默认文案为中文运营内容，非中文 locale 下仍会展示管理员配置的原文；管理员可自行编辑为目标语言。
- `every_session` 是进入认证系统区域的会话语义，不是严格服务器登录事件计数。
- 当前欢迎公告可独立挂载，因为现有通知弹窗仅手动打开；如果后续新增自动通知弹窗，需要统一弹窗优先级或队列。
- Markdown 渲染必须使用安全路径，不得复用启用 raw HTML 的 `Markdown` 组件。
- 欢迎公告配置变化后需要刷新相关查询，否则当前会话可能看不到最新配置。

## 14. 规格自检

- 无 TODO 或待定字段。
- 范围聚焦在 Issue #4：欢迎公告弹窗与设置可编辑性。
- 明确密码注册不自动登录，注册用户首次登录进入系统时展示。
- 明确不通过公开 `/api/status` 暴露管理员可编辑欢迎公告内容。
- 明确安全 Markdown 策略，不支持 raw HTML。
- 明确 `once_per_version`、`once_per_day`、`every_session` 的关闭和内容更新矩阵。
- 明确所有 Dialog 关闭路径都必须持久化或写入内存兜底。
- 明确当前仓库可执行的前端测试方式，不要求新增 DOM 测试依赖。
- 后端、前端、i18n、测试和验收均有明确路径。
- 未要求修改受保护项目标识。
