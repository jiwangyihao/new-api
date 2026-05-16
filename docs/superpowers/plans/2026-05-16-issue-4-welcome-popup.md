# Issue #4 登录欢迎公告弹窗实现计划

> 面向 AI 代理：本计划用于主分支直接开发。实现子代理只修改指定文件，不运行项目级验证、不提交；主代理统一验证、审查和提交。

## 目标

实现 GitHub Issue #4「登录/注册后：新增欢迎公告弹窗」：已认证用户进入 `_authenticated` 系统区域后展示可配置欢迎公告。密码注册当前不会自动登录，因此密码注册用户在注册成功后仍跳转登录页，并在首次登录进入系统区域时展示弹窗。

默认公告正文必须与 Issue #4 一致：

```text
欢迎使用赔钱GPT！

欢迎邀请好友使用赔钱GPT，邀请两位好友每月付费订阅，即可免费享受“一瓶盖可乐”付费订阅！

填写邀请码或试用码可享用24小时GPT5.5畅用！

官方QQ群：1106020227
```

## 关键决策

- 新增独立 `console_setting` 配置，不复用 dashboard announcements，不复用全局 Notice。
- 管理员在系统设置中可启用/禁用、编辑正文、选择展示频率并预览。
- 欢迎公告正文不通过公开 `/api/status` 返回。
- 新增受保护接口 `GET /api/user/welcome-popup`，必须注册在 `selfRoute` 下并继承 `middleware.UserAuth()`。
- 展示频率只允许：
  - `once_per_version`：同一用户对同一内容版本关闭后不再自动展示；内容变更后重新展示。
  - `once_per_day`：同一用户同一内容同一天关闭后不再自动展示；隔天或内容变更后重新展示。
  - `every_session`：同一 `AuthenticatedLayout` 挂载周期最多自动展示一次；关闭写入本地状态，但挂载周期门禁只作用于 `every_session`。
- 前端关闭状态按 userId 使用 localStorage：`welcome-popup-state:v1:<userId>`。
- localStorage 不可用、JSON 损坏、get/set 抛错时，用当前 JS 会话内存 Map 兜底，并可通过纯函数测试验证。
- Markdown 必须安全：使用 `react-markdown` + `remark-gfm`，不使用 `rehypeRaw` 或 `dangerouslySetInnerHTML`；链接协议使用白名单，只允许相对链接、锚点、`http:`、`https:`、`mailto:`、`tel:`。
- 所有 Dialog 关闭路径必须调用同一个 `onClose`/`close()`。
- 默认中文运营公告正文不加入 i18n；所有新增 UI 文案、错误文案、labelKey 必须补齐 `static-keys.ts` 和 6 个 locale。

## 子代理分工

### 任务 1：后端配置、校验、接口

允许修改/创建：

- `setting/console_setting/config.go`
- `setting/console_setting/validation.go`
- `setting/console_setting/validation_test.go`
- `controller/welcome_popup.go`
- `controller/welcome_popup_test.go`
- `router/welcome_popup_route_test.go`
- `controller/option.go`
- `router/api-router.go`

实现要求：

- `ConsoleSetting` 新增：
  - `WelcomePopupContent string json:"welcome_popup_content"`
  - `WelcomePopupEnabled bool json:"welcome_popup_enabled"`
  - `WelcomePopupFrequency string json:"welcome_popup_frequency"`
- 默认值：默认正文为 Issue #4 原文，默认启用，默认频率 `once_per_version`。
- `NormalizeWelcomePopupFrequency` 可对输出兜底，但保存校验必须精确接受枚举字面量，拒绝首尾空白和 `every_login` 等非法值。
- `ValidateWelcomePopupContent` 按 Unicode rune 限制 2000 字符，允许空内容，拒绝既有 `checkDangerousContent` 能识别的危险内容。
- `GetWelcomePopup` 返回结构：
  - `success: true`
  - `data.enabled`
  - `data.content`
  - `data.frequency`
- 禁用或内容 trim 后为空时，`content` 返回空字符串；频率仍返回归一化值。
- 判断空内容时可 trim，但返回非空正文时应保留管理员原始正文。
- `controller/option.go` 保存 `console_setting.welcome_popup_content` 与 `console_setting.welcome_popup_frequency` 前必须校验。校验失败返回 `success:false`，不得写入 DB/GlobalConfig。
- `router/api-router.go` 在 `selfRoute` 下注册 `GET /welcome-popup`。

后端测试要求由主代理最终运行并修复：

- 默认文案、默认启用、默认频率。
- 合法/非法频率，包含拒绝 `" once_per_day "`。
- 危险内容、Unicode 长度边界。
- `config.GlobalConfig.ExportAllConfigs()` 导出三个 key 且值正确。
- `GetWelcomePopup` 未登录失败、登录成功、禁用隐藏内容、空内容隐藏。
- `/api/status` 不包含欢迎公告正文、配置 key 或测试泄露字符串。
- 非法 option 保存测试必须显式 `AutoMigrate(&model.Option{})`，先写入合法旧值，再断言非法保存后旧值未变。
- 生产路由级测试必须构造 `router.SetApiRouter`，断言未登录 `GET /api/user/welcome-popup` 返回未授权且不含默认正文，证明实际路由继承 `UserAuth`。

### 任务 2：前端共享安全 Markdown 与用户端弹窗

允许修改/创建：

- `web/default/src/components/ui/safe-markdown.tsx`
- `web/default/src/components/ui/safe-markdown.test.ts`
- `web/default/src/features/welcome-popup/types.ts`
- `web/default/src/features/welcome-popup/api.ts`
- `web/default/src/features/welcome-popup/lib/welcome-popup-state.ts`
- `web/default/src/features/welcome-popup/lib/welcome-popup-state.test.ts`
- `web/default/src/features/welcome-popup/lib/welcome-popup-storage.ts`
- `web/default/src/features/welcome-popup/lib/welcome-popup-storage.test.ts`
- `web/default/src/features/welcome-popup/hooks/use-welcome-popup.ts`
- `web/default/src/features/welcome-popup/components/welcome-popup-dialog.tsx`
- `web/default/src/features/welcome-popup/index.tsx`
- `web/default/src/features/welcome-popup/welcome-popup-layout.test.ts`
- `web/default/src/components/layout/components/authenticated-layout.tsx`

实现要求：

- `SafeMarkdown` 契约：`SafeMarkdown(props: { children: string; className?: string })`。
- 导出可测试的 `isSafeHref(href: string): boolean`。协议白名单：相对链接、锚点、`http:`、`https:`、`mailto:`、`tel:`；拒绝 `javascript:`、大小写/控制字符变体、`data:`、`vbscript:`、`file:`、`blob:`。
- `SafeMarkdown` 不得 import `rehypeRaw`，不得使用 `dangerouslySetInnerHTML`，不得复用旧 `Markdown` 组件。
- API Hook 使用 `queryKey: ['welcome-popup', userId]`，只有存在 userId 时 enabled，不得通过 `useStatus` 或 `localStorage.getItem('status')` 获取配置。
- `shouldShowWelcomePopup` 必须是纯逻辑，覆盖禁用/空内容/无用户、三种频率、内容更新、日期变化。
- `shownThisSession` 只对 `every_session` 生效；`once_per_version` 与 `once_per_day` 在同一挂载周期内遇到内容变更时必须允许重新展示。
- storage 读写必须抽成可测试模块，支持注入 storage，get/set 抛错和 JSON 损坏时用内存 Map 兜底。
- `WelcomePopupDialog` 的 overlay click、Esc、右上角关闭按钮、底部关闭按钮都必须走同一个 `onClose`。
- Dialog 内容区域移动端最大高度受限并可滚动。
- `WelcomePopupGate` 挂载到 `AuthenticatedLayout` 内部，使登录、OAuth、Passkey、OTP 等进入系统路径统一触发。
- 源码 smoke 测试必须证明密码注册仍是跳转登录/不自动登录：检查 `sign-up-form.tsx` 仍包含注册成功提示和 `redirectToLogin()`，且不写 auth store。

### 任务 3：前端系统设置页

允许修改/创建：

- `web/default/src/features/system-settings/types.ts`
- `web/default/src/features/system-settings/content/index.tsx`
- `web/default/src/features/system-settings/content/section-registry.tsx`
- `web/default/src/features/system-settings/content/welcome-popup-form.ts`
- `web/default/src/features/system-settings/content/welcome-popup-form.test.ts`
- `web/default/src/features/system-settings/content/welcome-popup-section.tsx`
- `web/default/src/features/system-settings/hooks/use-update-option.ts`

依赖契约：`@/components/ui/safe-markdown` 由任务 2 提供，设置页只 import 使用，不得自行创建替代 Markdown。

实现要求：

- 新增 `WelcomePopupFrequency = 'once_per_version' | 'once_per_day' | 'every_session'`。
- `ContentSettings` 增加三个欢迎公告 key。
- 默认内容设置加入三个欢迎公告 key。
- 内容设置分区新增 `welcome-popup`，标题 `Welcome Popup`，说明 `Configure login welcome announcement popup`。
- `welcome-popup-form.ts` 使用 schema factory，例如 `createWelcomePopupFormSchema(t)`，确保 `Welcome popup content must be at most 2000 characters.` 实际经过 `t()` 翻译；测试可传 identity translator。
- `collectWelcomePopupSettingUpdates` 只收集正文与频率变化；启用开关由独立 Switch 保存。
- `welcome-popup-section.tsx` 支持启用/禁用、正文 textarea、频率 select、安全 Markdown 预览。
- 保存正文/频率时必须检查每次 `updateOption.mutateAsync` 的返回值；任一返回 `success:false` 时停止后续保存，不显示成功 toast。
- 启用开关保存失败时不得乐观更新为新状态。
- `use-update-option.ts` 对欢迎公告相关 key 保存成功后 invalidate `['welcome-popup']`，不要加入 `STATUS_RELATED_KEYS`。
- 不必要时不要解构 props；组件中用 `props.defaultValues`。

### 任务 4：i18n 与 smoke 收口

允许修改/创建：

- `web/default/src/i18n/static-keys.ts`
- `web/default/src/i18n/locales/en.json`
- `web/default/src/i18n/locales/zh.json`
- `web/default/src/i18n/locales/fr.json`
- `web/default/src/i18n/locales/ja.json`
- `web/default/src/i18n/locales/ru.json`
- `web/default/src/i18n/locales/vi.json`
- 必要时补充 `web/default/src/features/welcome-popup/welcome-popup-layout.test.ts`

必须覆盖 UI key：

- `Welcome Popup`
- `Configure login welcome announcement popup`
- `This popup appears after users enter the authenticated system area.`
- `Enabled`
- `This content is returned only to authenticated users.`
- `Welcome announcement content`
- `Markdown is supported. Raw HTML is not supported.`
- `Display frequency`
- `Show once per content update`
- `Show once per day`
- `Show every system session`
- `Preview`
- `Save welcome popup`
- `Welcome popup saved`
- `Welcome popup content must be at most 2000 characters.`
- `Welcome announcement`
- `Close welcome popup`
- `I understand`

默认运营公告正文不得进入 `static-keys.ts` 或任何 locale 文件。

Smoke 测试要求：

- 枚举欢迎公告 UI key，检查 `static-keys.ts` 和 6 个 locale 均包含。
- 检查默认运营公告正文未进入 static keys/locales。
- 检查 `welcome-popup-section.tsx` 和 `welcome-popup-dialog.tsx` 均导入 `SafeMarkdown`，且不导入旧 `Markdown`。
- 检查 `safe-markdown.tsx` 不包含 `rehypeRaw` 或 `dangerouslySetInnerHTML`。
- 检查 `authenticated-layout.tsx` 挂载 `WelcomePopupGate`。
- 检查 `api.ts` 请求 `/api/user/welcome-popup`，不读取 `/api/status`。
- 检查 `sign-up-form.tsx` 注册成功后仍调用 `redirectToLogin()`。

## 主代理最终验证清单

主代理在所有实现任务结束后运行并读取输出：

- `go test ./setting/console_setting ./controller ./router -run 'Test(DefaultWelcomePopup|WelcomePopup|GetWelcomePopup|StatusDoesNotExposeWelcomePopup|UpdateOptionRejectsInvalidWelcomePopup)' -count=1`
- `cd web/default && bun test src/components/ui/safe-markdown.test.ts src/features/system-settings/content/welcome-popup-form.test.ts src/features/welcome-popup/lib/welcome-popup-state.test.ts src/features/welcome-popup/lib/welcome-popup-storage.test.ts src/features/welcome-popup/welcome-popup-layout.test.ts`
- `cd web/default && bun run i18n:sync`
- `cd web/default && bun run typecheck`

最终人工核对：

- `/api/status` 未泄露欢迎公告。
- `/api/user/welcome-popup` 未登录不可读，登录可读。
- 管理员可在设置中编辑欢迎公告，保存失败不提示成功。
- 弹窗在进入系统后出现，关闭后按频率规则不重复；内容更新会重新展示。
- 移动端弹窗内容可滚动，关闭按钮可见。
- 工作区没有无关 lockfile、格式化或 protected identifier 改动。
