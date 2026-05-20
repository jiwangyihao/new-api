# Issue #7 注册页背景图替换规格

> 面向 AI 代理的工作者：本规格用于处理 GitHub fork 仓库 `jiwangyihao/new-api` 的 Issue #7「注册页：替换背景图为不侵权的二次元女生图片」。实现前必须读取仓库根目录 `AGENTS.md` 与 `web/default/AGENTS.md`，并遵守 React 19、TypeScript、TanStack Router、i18n、Tailwind CSS、Bun、AGPL 版权头以及项目受保护标识约束。

**工作区要求：** 本次修正在隔离工作区 `C:/Users/34404/source/repos/new-api/.worktrees/issue-7-generated-background` 的 `worktree/issue-7-generated-background` 分支完成、验证、提交，再合并回主工作区 `main`。不得删除、移动、格式化或提交主工作区中与 Issue #7 无关的未跟踪或已修改文件。

**目标：** 为 default 前端注册页增加无明显版权风险的二次元女生风格美图背景，并保证桌面端和移动端注册表单保持可读。

**架构：** 不引入第三方运行时依赖，不从外部 URL 热链图片。使用生成式图片流程产出一张本仓库内的 JPG 背景资产，保存到 `web/default/src/features/auth/assets/`，在注册页专用路径中通过静态 import 使用。通用 `AuthLayout` 增加可选视觉配置，默认登录、找回密码、OAuth 等认证页保持现状，`SignUp` 仅传入注册页背景配置。

**技术栈：** React 19、TypeScript、TanStack Router、i18next、Tailwind CSS、Rsbuild、Bun。

---

## 1. 背景

Issue #7 要求将注册页面背景图替换为不侵权的二次元女生图片。用户进一步纠正：手写 SVG 矢量插画不符合「二次元美图」预期，必须使用图片生成流程产出真正的二次元美图资源，再接入注册页。

验收项：

- 注册页面背景图已替换为二次元女生风格美图。
- 图片不存在明显版权风险。
- 桌面端和移动端显示正常，不影响注册表单可读性。

## 2. 当前代码基线

已确认的关键文件和现有能力：

- `web/default/src/routes/(auth)/sign-up.tsx`
  - TanStack Router 注册页入口，渲染 `SignUp`。
  - 支持 `aff` 搜索参数。
- `web/default/src/features/auth/sign-up/index.tsx`
  - 当前通过 `<AuthLayout backgroundImageSrc={...}>` 包裹注册表单和服务条款。
  - 标题为 `Create an account`，已有 i18n。
- `web/default/src/features/auth/auth-layout.tsx`
  - 所有认证页共享布局。
  - 已具备可选 `backgroundImageSrc?: string`、装饰性背景 `<img>`、遮罩层和背景分支卡片可读性样式。
  - 左上角 Logo / 系统名链接使用 `z-20`，高于内容层 `z-10`。
- `web/default/src/features/auth/sign-in/index.tsx`、`forgot-password/index.tsx`、`otp/index.tsx`、`reset-password-confirm/index.tsx`、`components/oauth-callback-screen.tsx`、`oauth-onboarding/components/oauth-onboarding-form.tsx`
  - 均复用 `AuthLayout`，本次不得被迫显示注册页背景图。
- `web/default/src/env.d.ts`
  - 当前引用 `@rsbuild/core/types`，Rsbuild 已内置常见图片静态资源声明；实现应保留该引用，不需要新增本地 JPG module 声明，除非 typecheck 证明缺失。
- `web/default/src/features/auth/api.test.ts`
  - 现有 auth 测试使用 `node:test` + source-level 断言模式；新增布局契约测试应沿用该模式。
- `web/default/package.json`
  - 定向测试可用 `bunx tsx --test <test files>`。
  - TypeScript 验证命令为 `bun run typecheck`。

## 3. 决策

1. **只改 default 注册页，不改 classic。** Issue 指向当前站点注册页面；default 前端是当前默认主题，classic 没有被点名且不应扩大范围。
2. **使用生成式图片产出 JPG 美图资产。** 背景必须是真实 raster 图片资源，而不是手写 SVG 矢量插画；画面为原创二次元女生风格，不引用现有角色、IP、摄影作品、图库 URL 或外部素材。
3. **不在图片或用户可见资源中标注生成来源。** 用户明确要求不要专门标注「AI 生成」；图片文件不得包含水印、签名、Logo、商标、`AI-generated`、`Pollinations` 等专门标注或来源文字。
4. **不引入新依赖。** 背景使用 JPG 静态 import + CSS/Tailwind 完成；不增加图片优化库、动画库或远程图片服务运行时依赖。
5. **`AuthLayout` 增加可选背景配置，默认关闭。** 使用最小 prop `backgroundImageSrc?: string`；未传入时输出结构与现有认证页语义保持一致，不显示背景 `<img>`，也不启用注册页专用卡片视觉。
6. **只有 `SignUp` 传入背景。** 登录、忘记密码、OTP、OAuth 回调、OAuth onboarding、重置密码等页面不展示该背景，避免 Issue #7 的注册页变更影响其他认证流程。
7. **桌面端图像展示，移动端降低干扰。** 背景图覆盖全屏并使用 `object-cover`。移动端使用更强遮罩和表单卡片背景；桌面端保留图像氛围，但表单仍保持可读。
8. **表单放在半透明卡片上。** 注册表单内容区域使用 `bg-background/90`、边框、阴影和 backdrop blur，确保文字、输入框、按钮在背景上仍符合可读性要求。
9. **背景图片作为装饰。** `<img>` 使用空 `alt` 并 `aria-hidden="true"`；本次不暴露背景 alt 配置，避免屏幕阅读器重复朗读装饰图，也避免新增未使用 API。
10. **不改注册表单业务逻辑。** 不修改注册 API、OAuth、微信验证码、Turnstile、条款勾选、邀请码逻辑。
11. **不改受保护品牌和版权头。** 保留所有源文件现有 AGPL / QuantumNous 版权头；新增 TS/TSX 测试文件按项目习惯添加版权头。

## 4. 业务范围

### 4.1 必须满足

- 注册页面渲染一张二次元女生风格美图背景。
- 背景图为本仓库内静态资产，不依赖外部 URL。
- 背景图是 JPG raster 图片，不再使用先前手写 SVG 资产。
- 背景图不得包含水印、签名、Logo、商标、专门来源标注或现有动漫角色特征。
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

创建文件：`web/default/src/features/auth/assets/sign-up-anime-girl.jpg`

要求：

- 使用图片生成流程产出原创二次元女生风格美图，主体适合注册页背景。
- 不包含可识别的现有动漫角色、商标、Logo、文字、水印、签名或第三方作品元素。
- 不包含 `AI-generated`、`No external image source`、`Intended for commercial use`、`Pollinations` 等专门标注。
- 文件为本地 JPG，二进制以 JPEG magic bytes `FF D8 FF` 开头。
- 画面构图适合全屏 `object-cover`：主体可偏左或偏背景侧，注册表单区域即使在桌面和移动裁切下也不依赖图像文字表达信息。
- 删除旧的 `web/default/src/features/auth/assets/sign-up-anime-girl.svg`，避免继续引用错误方向资产。

### 5.2 图片类型声明

文件：`web/default/src/env.d.ts`

当前文件已通过 `/// <reference types="@rsbuild/core/types" />` 引入 Rsbuild 类型。实现不应主动新增本地 `*.jpg` module 声明；只有当 `bun run typecheck` 证明类型缺失时，才以最小变更补充声明。

### 5.3 `AuthLayout` 可选背景

文件：`web/default/src/features/auth/auth-layout.tsx`

`AuthLayoutProps` 保持：

```ts
type AuthLayoutProps = {
  children: React.ReactNode
  backgroundImageSrc?: string
}
```

渲染设计：

- 根节点是全屏相对容器，并允许内容高度超过视口时正常滚动。
- 当 `backgroundImageSrc` 存在时，在内容前渲染绝对定位装饰层：
  - `<img src={props.backgroundImageSrc} alt="" aria-hidden="true" className="absolute inset-0 h-full w-full object-cover object-center" />`
  - 遮罩层：`absolute inset-0 bg-background/80 backdrop-blur-[1px] sm:bg-background/70 lg:bg-background/45`
  - 渐变层增强表单侧可读性：`absolute inset-0 bg-[radial-gradient(...)]` 或等效 Tailwind arbitrary value。
- 内容层使用 `relative z-10`。
- 首页 Logo / 系统名链接必须高于内容层，例如 `z-20`，确保仍可点击。
- 表单容器在有背景时增加卡片样式：`rounded-3xl border bg-background/90 shadow-2xl backdrop-blur-xl`，移动端保留足够 padding。
- 未传入 `backgroundImageSrc` 时，不渲染背景 `<img>`，表单容器保持现有简洁布局或仅增加不影响视觉的基础类。

### 5.4 `SignUp` 使用背景

文件：`web/default/src/features/auth/sign-up/index.tsx`

- 导入背景：

```ts
import signUpAnimeGirlBackground from '../assets/sign-up-anime-girl.jpg'
```

- 将 `<AuthLayout>` 改为：

```tsx
<AuthLayout backgroundImageSrc={signUpAnimeGirlBackground}>
```

不得修改注册表单、服务条款或注册业务逻辑。

## 6. 测试设计

### 6.0 TDD 红灯要求

修正必须按红-绿顺序执行：先更新 `auth-layout-background.test.ts` 的 JPG 契约，不修改生产引用和资产，运行定向测试并观察到失败。失败原因应来自仍引用 `.svg`、缺少 `.jpg` 资产、装饰性背景或可读性样式缺失，而不是测试语法错误。确认红灯后，再替换资产并修改注册页 import，最后复跑同一命令确认绿灯。

### 6.1 Auth layout source-level 契约测试

文件：`web/default/src/features/auth/auth-layout-background.test.ts`

使用 `node:test` + `readFileSync`，避免引入 DOM 测试环境。测试必须覆盖：

- `AuthLayoutProps` 包含 `backgroundImageSrc?: string`。
- `AuthLayout` 仅在 `backgroundImageSrc` 存在时渲染背景 `<img>`。
- 背景 `<img>` 使用 `aria-hidden='true'` 或 `aria-hidden="true"`，`alt=''` 或 `alt=""`。
- 布局包含 `object-cover`、遮罩层、`bg-background/90`、`backdrop-blur`、`shadow-2xl` 等保证可读性的类。
- 背景专用类 `bg-background/90`、`shadow-2xl`、`backdrop-blur`、`rounded-3xl`、背景遮罩层等必须位于 `backgroundImageSrc` / `hasBackground` 控制的分支或 class 组合中；未传入背景时默认认证页不得启用注册页专用卡片视觉。
- `SignUp` import `sign-up-anime-girl.jpg` 并向 `AuthLayout` 传入 `backgroundImageSrc`，且不再引用 `sign-up-anime-girl.svg`。
- `SignIn` 源码不传入 `backgroundImageSrc`。
- JPG 文件存在，前三个字节为 `FF D8 FF`。
- JPG 二进制文本中不包含 `AI-generated`、`No external image source`、`Intended for commercial use`、`Pollinations`、`<svg`、`<image`、`<script`、`data:`、`base64` 等专门标注、SVG/XML 或外部内嵌资源入口。

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

生产构建验证 JPG 静态资源打包：

```bash
cd web/default
bun run build
```

`bun run typecheck` 不能替代构建验证；新增 JPG 静态 import 后必须至少执行一次生产构建。

## 7. 验收清单

- [ ] `web/default/src/features/auth/assets/sign-up-anime-girl.jpg` 存在，且是本地 JPG 图片。
- [ ] 旧的 `web/default/src/features/auth/assets/sign-up-anime-girl.svg` 已删除且不再被引用。
- [ ] JPG 不包含水印、商标、专门来源标注、SVG/XML、外部 URL 或 base64 内嵌资源入口。
- [ ] `SignUp` 传入背景图，注册页有二次元女生风格美图背景。
- [ ] `SignIn` 和其他认证页未默认显示注册页背景图。
- [ ] 注册页表单容器具备背景遮罩、边框、阴影和 blur，保证桌面/移动可读。
- [ ] 注册表单业务逻辑未改变。
- [ ] 定向 auth 测试通过。
- [ ] `bun run i18n:sync` 执行完成且未产生无关 locale diff。
- [ ] `bun run typecheck` 通过。
- [ ] `bun run build` 通过。
- [ ] 提交前确认 `git status --short --untracked-files=all`、`git diff`、`git diff --cached` 和 `git diff --cached --name-only` 不包含 Issue #7 以外的变更。
- [ ] 合并回主分支并清理 `.worktrees/issue-7-generated-background`。
