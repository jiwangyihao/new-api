# Issue #7 注册页背景图替换规格

> 面向 AI 代理的工作者：本规格用于处理 GitHub fork 仓库 `jiwangyihao/new-api` 的 Issue #7「注册页：替换背景图为不侵权的二次元女生图片」。实现前必须读取仓库根目录 `AGENTS.md` 与 `web/default/AGENTS.md`，并遵守 React 19、TypeScript、TanStack Router、i18n、Tailwind CSS、Bun、AGPL 版权头以及项目受保护标识约束。

**工作区要求：** 本规格位于隔离工作区 `C:/Users/34404/source/repos/new-api/.worktrees/issue-7-auth-background/docs/superpowers/specs/2026-05-19-issue-7-auth-background-spec.md`。实现应先在该 worktree 的 `worktree/issue-7-auth-background` 分支完成、验证、提交，再按用户要求合并回主工作区 `main` 并清理该 worktree。不得删除、移动、格式化或提交主工作区中与 Issue #7 无关的未跟踪或已修改文件。

**目标：** 为 default 前端注册页增加无明显版权风险的二次元女生风格背景图，并保证桌面端和移动端注册表单保持可读。

**架构：** 不引入第三方运行时依赖，不从外部 URL 热链图片。将一张项目内生成并保存的 SVG 背景资产放入 `web/default/src/features/auth/assets/`，在注册页专用的布局路径中通过静态 import 使用；通用 `AuthLayout` 增加可选视觉配置，默认登录、找回密码、OAuth 等认证页保持现状，`SignUp` 仅传入注册页背景配置。

**技术栈：** React 19、TypeScript、TanStack Router、i18next、Tailwind CSS、Rsbuild、Bun。

---

## 1. 背景

Issue #7 要求将注册页面背景图替换为不侵权的二次元女生图片。当前 default 前端注册页实际没有独立图片背景：`SignUp` 通过 `AuthLayout` 居中渲染表单，页面背景依赖全局 `bg-background`。本次需要在注册页建立明确、可追溯、无外部版权风险的背景资产与渲染路径。

验收项：

- 注册页面背景图已替换为二次元女生风格图片。
- 图片不存在明显版权风险。
- 桌面端和移动端显示正常，不影响注册表单可读性。

## 2. 当前代码基线

已确认的关键文件和现有能力：

- `web/default/src/routes/(auth)/sign-up.tsx`
  - TanStack Router 注册页入口，渲染 `SignUp`。
  - 支持 `aff` 搜索参数。
- `web/default/src/features/auth/sign-up/index.tsx`
  - 当前直接渲染 `<AuthLayout>` 包裹注册表单和服务条款。
  - 标题为 `Create an account`，已有 i18n。
- `web/default/src/features/auth/auth-layout.tsx`
  - 所有认证页共享布局。
  - 当前根节点为 `relative grid h-svh max-w-none`，没有背景图片槽位。
  - 左上角 Logo / 系统名在根层绝对定位，表单容器居中。
- `web/default/src/features/auth/sign-in/index.tsx`、`forgot-password/index.tsx`、`otp/index.tsx`、`reset-password-confirm/index.tsx`、`components/oauth-callback-screen.tsx`、`oauth-onboarding/components/oauth-onboarding-form.tsx`
  - 均复用 `AuthLayout`，本次不得被迫显示注册页背景图。
- `web/default/src/env.d.ts`
  - 当前引用 `@rsbuild/core/types`，Rsbuild 已内置 `*.svg` 静态资源声明；实现应保留该引用，不需要新增本地 SVG module 声明。
- `web/default/src/features/auth/api.test.ts`
  - 现有 auth 测试使用 `node:test` + source-level 断言模式；可追加布局契约测试或新建 `auth-layout-background.test.ts`。
- `web/default/package.json`
  - 定向测试可用 `bunx tsx --test <test files>`。
  - TypeScript 验证命令为 `bun run typecheck`。

## 3. 决策

1. **只改 default 注册页，不改 classic。** Issue 指向当前站点注册页面；default 前端是当前默认主题，classic 没有被点名且不应扩大范围。
2. **使用项目内 AI 生成 SVG 资产。** SVG 由本次实现直接创建，描述抽象原创二次元女生插画，不引用现有角色、IP、摄影作品、图库 URL 或外部素材；随仓库提交，避免热链和授权漂移。
3. **保存授权记录。** 在 SVG 文件头部用 XML 注释记录：`AI-generated original illustration for Issue #7`、`No external image source`、`Intended for commercial use in this project`，满足来源和授权记录要求。
4. **不引入新依赖。** 背景使用 SVG + CSS/Tailwind 完成；不增加图片优化库、动画库或远程图片服务。
5. **`AuthLayout` 增加可选背景配置，默认关闭。** 新增最小 prop `backgroundImageSrc?: string`；未传入时输出结构与现有认证页语义保持一致，不显示背景 `<img>`，也不启用注册页专用卡片视觉。
6. **只有 `SignUp` 传入背景。** 登录、忘记密码、OTP、OAuth 回调、OAuth onboarding、重置密码等页面不展示该背景，避免 Issue #7 的注册页变更影响其他认证流程。
7. **桌面端图像展示，移动端降低干扰。** 背景图覆盖全屏并使用 `object-cover`。移动端使用更强遮罩和表单卡片背景；桌面端可在 `lg` 以上增加左右两栏视觉节奏，但表单仍保持可读。
8. **表单放在半透明卡片上。** 注册表单内容区域使用 `bg-background/90`、边框、阴影和 backdrop blur，确保文字、输入框、按钮在背景上仍符合可读性要求。
9. **背景图片作为装饰。** `<img>` 使用空 `alt` 并 `aria-hidden="true"`；本次不暴露背景 alt 配置，避免屏幕阅读器重复朗读装饰图，也避免新增未使用 API。
10. **不改注册表单业务逻辑。** 不修改注册 API、OAuth、微信验证码、Turnstile、条款勾选、邀请码逻辑。
11. **不改受保护品牌和版权头。** 保留所有源文件现有 AGPL / QuantumNous 版权头；新增 TS/TSX 测试文件按项目习惯添加版权头，SVG 内授权记录不得替换项目归属信息。

## 4. 业务范围

### 4.1 必须满足

- 注册页面渲染一张二次元女生风格背景图。
- 背景图为本仓库内静态资产，不依赖外部 URL。
- 背景图来源和授权记录随资产文件提交。
- `SignUp` 使用注册页背景；其他认证页默认不显示该背景。
- 桌面端注册页表单在背景之上有足够对比度，不被图像遮挡。
- 移动端注册页表单可读，背景不干扰输入、按钮和条款文本。
- 不改变注册表单字段、校验、OAuth、微信登录、Turnstile、邀请码和条款逻辑。
- 不新增依赖。
- 修改 TypeScript / TSX 后执行 `bun run typecheck`。

### 4.2 非目标

- 不修改 classic 前端。
- 不新增可配置后台上传背景图能力。
- 不新增图片管理、CDN、远程加载或图片压缩流水线。
- 不替换登录页、找回密码页、OAuth 页背景。
- 不改注册接口、用户模型、OAuth 或微信注册逻辑。
- 不修复全仓既有 lint 问题。
- 不部署远端服务，除非用户后续明确要求。

## 5. 前端设计

### 5.1 背景资产

创建文件：`web/default/src/features/auth/assets/sign-up-anime-girl.svg`

要求：

- SVG 自包含，不引用 `<image href="...">`、远程 URL、base64 外部图片或字体文件。
- 画面为原创二次元女生风格：柔和色彩、科技感光效、抽象网格或星光背景，不能出现可识别的现有动漫角色、商标、Logo 或第三方作品元素。
- 视图比例适合全屏 cover，建议 `viewBox="0 0 1600 1200"`。
- 文件头部包含授权记录注释，例如：

```xml
<!--
AI-generated original illustration for new-api Issue #7.
No external image source, stock asset, character IP, logo, or copyrighted work was used.
Intended for commercial use in this project.
-->
```

### 5.2 SVG 类型声明

文件：`web/default/src/env.d.ts`

当前文件已通过 `/// <reference types="@rsbuild/core/types" />` 引入 Rsbuild 类型，而 `@rsbuild/core/types` 已声明 `declare module '*.svg'`。本次不得重复添加本地 `*.svg` module 声明；只需保留现有 Rsbuild reference。若后续实测类型缺失，再以最小变更补充声明。

### 5.3 `AuthLayout` 可选背景

修改文件：`web/default/src/features/auth/auth-layout.tsx`

`AuthLayoutProps` 增加：

```ts
type AuthLayoutProps = {
  children: React.ReactNode
  backgroundImageSrc?: string
}
```

渲染设计：

- 根节点继续是全屏相对容器。
- 当 `backgroundImageSrc` 存在时，在内容前渲染绝对定位装饰层：
  - `<img src={backgroundImageSrc} alt="" aria-hidden="true" className="absolute inset-0 h-full w-full object-cover" />`
  - 深色/浅色遮罩层：`absolute inset-0 bg-background/80 backdrop-blur-[1px] sm:bg-background/70 lg:bg-background/45`
  - 渐变层增强表单侧可读性：`absolute inset-0 bg-[radial-gradient(...)]` 或等效 Tailwind arbitrary value。
- 内容层使用 `relative z-10`。
- 表单容器在有背景时增加卡片样式：`rounded-3xl border bg-background/90 shadow-2xl backdrop-blur-xl`，移动端保留足够 padding。
- 未传入 `backgroundImageSrc` 时，不渲染背景 `<img>`，表单容器保持现有简洁布局或仅增加不影响视觉的基础类。

### 5.4 `SignUp` 使用背景

修改文件：`web/default/src/features/auth/sign-up/index.tsx`

- 导入背景：

```ts
import signUpAnimeGirlBackground from '../assets/sign-up-anime-girl.svg'
```

- 将 `<AuthLayout>` 改为：

```tsx
<AuthLayout backgroundImageSrc={signUpAnimeGirlBackground}>
```

不得修改注册表单、服务条款或注册业务逻辑。

## 6. 测试设计

### 6.0 TDD 红灯要求

实现必须按红-绿顺序执行：先只新增 `auth-layout-background.test.ts`，不修改生产代码和 SVG 资产，运行定向测试并观察到失败。失败原因应来自缺少 `backgroundImageSrc`、注册页 SVG import / 传参、装饰性背景 `<img>`、可读性样式或 SVG 授权记录，而不是测试语法错误。确认红灯后，再创建 SVG、修改 `AuthLayout` 和注册页传参，最后复跑同一命令确认绿灯。

### 6.1 Auth layout source-level 契约测试

创建文件：`web/default/src/features/auth/auth-layout-background.test.ts`

使用 `node:test` + `readFileSync`，避免引入 DOM 测试环境。测试必须覆盖：

- `AuthLayoutProps` 包含 `backgroundImageSrc?: string`。
- `AuthLayout` 仅在 `backgroundImageSrc` 存在时渲染背景 `<img>`。
- 背景 `<img>` 使用 `aria-hidden='true'` 或 `aria-hidden="true"`，`alt=''` 或 `alt=""`。
- 布局包含 `object-cover`、遮罩层、`bg-background/90`、`backdrop-blur`、`shadow-2xl` 等保证可读性的类。
- 背景专用类 `bg-background/90`、`shadow-2xl`、`backdrop-blur`、`rounded-3xl`、背景遮罩层等必须位于 `backgroundImageSrc` / `hasBackground` 控制的分支或 class 组合中；未传入背景时默认认证页不得启用注册页专用卡片视觉。
- `SignUp` import `sign-up-anime-girl.svg` 并向 `AuthLayout` 传入 `backgroundImageSrc`。
- `SignIn` 源码不传入 `backgroundImageSrc`。
- SVG 文件包含 `AI-generated original illustration`、`No external image source`、`Intended for commercial use`，且不包含 `http://`、`https://`、`data:`、`base64`、`<image`、`<foreignObject`、`<script`、`@import`、`url(` 等外部资源或脚本入口；同时应包含矢量绘制元素，例如 `<path`、`<circle`、`<ellipse`、`<linearGradient`。

### 6.2 定向验证命令

前端定向测试：

```bash
cd web/default
bunx tsx --test src/features/auth/auth-layout-background.test.ts src/features/auth/api.test.ts
```

i18n 同步：

```bash
cd web/default
bun run i18n:sync
```

本次不新增用户可见文案；`i18n:sync` 原则上不应产生 locale 变更。若出现 locale diff，必须确认是否由 Issue #7 直接引起；否则不得提交。

TypeScript：

```bash
cd web/default
bun run typecheck
```

生产构建验证 SVG 静态资源打包：

```bash
cd web/default
bun run build
```

`bun run typecheck` 不能替代构建验证；新增 SVG 静态 import 后必须至少执行一次生产构建。

## 7. 验收清单

- [ ] `web/default/src/features/auth/assets/sign-up-anime-girl.svg` 存在，且包含来源/授权记录。
- [ ] SVG 不引用远程 URL、外部图片或第三方素材。
- [ ] `SignUp` 传入背景图，注册页有二次元女生风格视觉背景。
- [ ] `SignIn` 和其他认证页未默认显示注册页背景图。
- [ ] 注册页表单容器具备背景遮罩、边框、阴影和 blur，保证桌面/移动可读。
- [ ] 注册表单业务逻辑未改变。
- [ ] 定向 auth 测试通过。
- [ ] `bun run i18n:sync` 执行完成。
- [ ] `bun run typecheck` 通过。
- [ ] 提交前确认 `git status --short`、`git diff`、`git diff --cached` 不包含 Issue #7 以外的变更。
- [ ] 合并回主分支并清理 `.worktrees/issue-7-auth-background`。
