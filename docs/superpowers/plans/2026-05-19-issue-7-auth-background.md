# Issue #7 注册页背景图替换修正计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 将 default 前端注册页替换为带无明显版权风险二次元萌系可爱美图背景的注册界面，并保持桌面端和移动端表单可读。

**架构：** 使用 `image_generator` 子代理逐张生成的 20 张原生 16:9 PNG 作为本地静态候选资产；`AuthLayout` 保持最小可选 `backgroundImageSrc` prop，并将背景图、遮罩和半透明卡片样式限制在 `hasBackground` 分支；`SignUp` 独占传入 `sign-up-backgrounds/signup-moe-native-13.png`，登录和其他认证页不变。通过 source-level TDD 测试锁定 20 张 PNG 资产、无专门来源标注、注册页 wiring、共享布局默认不启用背景视觉，并通过 Rsbuild 生产构建验证 PNG 打包。

**技术栈：** React 19、TypeScript、TanStack Router、i18next、Tailwind CSS、Rsbuild、Bun。

---

## 0. 执行约束

- 实现 worktree：`C:/Users/34404/source/repos/new-api/.worktrees/issue-7-twenty-backgrounds`。
- 主工作区：`C:/Users/34404/source/repos/new-api`，可能存在与 Issue #7 无关的用户改动；不得删除、移动、格式化、暂存或提交这些改动。
- 规格文件：`C:/Users/34404/source/repos/new-api/.worktrees/issue-7-twenty-backgrounds/docs/superpowers/specs/2026-05-19-issue-7-auth-background-spec.md`。
- 本计划文件：`C:/Users/34404/source/repos/new-api/.worktrees/issue-7-twenty-backgrounds/docs/superpowers/plans/2026-05-19-issue-7-auth-background.md`。
- 实现前必须读取并遵守根目录 `AGENTS.md` 与 `web/default/AGENTS.md`。
- 不修改 classic 前端。
- 不新增依赖。
- 不修改注册 API、OAuth、微信验证码、Turnstile、邀请码、条款同意或其他注册业务逻辑。
- 不修改或删除受保护品牌、版权、归属信息，例如 `new-api`、`QuantumNous`、AGPL 版权头。
- 用户明确要求不要在图片或用户可见资源中专门标注「AI 生成」。
- `web/default/src/env.d.ts` 已通过 `@rsbuild/core/types` 获得常见静态资源声明；不得主动添加本地 `declare module '*.png'`，除非 typecheck 证明缺失。
- 子代理执行实现审查时不要运行项目级 build/test/lint/typecheck/formatter；主控统一运行验证。
- 每次提交前必须执行 `git status --short --untracked-files=all`、`git diff`、`git diff --cached` 和 `git diff --cached --name-only`，确认只包含 Issue #7 文件。
- 最终必须合并回主分支并清理 `.worktrees/issue-7-twenty-backgrounds`。

## 1. 文件结构

- 修改：`web/default/src/features/auth/auth-layout-background.test.ts`
  - Source-level 契约测试，按红灯后绿灯验证。
  - 覆盖 `AuthLayout` 可选背景、背景专用样式分支、`SignUp` wiring、`SignIn` 不受影响、20 张 PNG 资产存在且不含专门来源标注或脚本入口。
- 创建目录：`web/default/src/features/auth/assets/sign-up-backgrounds/`
  - 新增 `signup-moe-native-01.png` 到 `signup-moe-native-20.png`。
  - 20 张图均由 `image_generator` 子代理使用 `image_gen.imagegen` 逐张生成，原生 16:9，萌系可爱，全年龄安全。
- 删除：`web/default/src/features/auth/assets/sign-up-anime-girl.jpg`
  - 删除先前单张旧资产，避免继续引用。
- 修改：`web/default/src/features/auth/sign-up/index.tsx`
  - 静态 import `../assets/sign-up-backgrounds/signup-moe-native-13.png`。
  - `<AuthLayout backgroundImageSrc={signUpAnimeGirlBackground}>`。
- 保持：`web/default/src/features/auth/auth-layout.tsx`
  - 保留 `backgroundImageSrc?: string`、装饰性 `<img>`、遮罩、`z-20` 首页链接和背景分支表单卡片样式。
- 保持不改：`web/default/src/env.d.ts`
  - Rsbuild 类型若能覆盖 PNG import，不额外声明。

---

## 2. 任务 A：更新红灯契约测试

**文件：**

- 修改：`web/default/src/features/auth/auth-layout-background.test.ts`
- 只读参考：`web/default/src/features/auth/api.test.ts`
- 只读参考：`web/default/src/features/auth/auth-layout.tsx`
- 只读参考：`web/default/src/features/auth/sign-up/index.tsx`
- 只读参考：`web/default/src/features/auth/sign-in/index.tsx`

- [x] **步骤 A1：把测试从单张 JPG 契约改为 20 张 PNG 契约**

测试文件保留项目版权头、`node:test` 和 source-level 模式。测试要求：

- `SignUp` wiring 精确匹配 `sign-up-backgrounds/signup-moe-native-13.png`。
- `SignUp` 不再引用 `sign-up-anime-girl.svg` 或 `sign-up-anime-girl.jpg`。
- 资产目录正好包含 `signup-moe-native-01.png` 到 `signup-moe-native-20.png`。
- 每张图片是 PNG 或 JPEG raster 图片。
- 图片二进制不包含 `AI-generated`、`No external image source`、`Intended for commercial use`、`Pollinations`、`<script`、`<foreignObject`、`<iframe`、`base64` 等风险标记。

- [x] **步骤 A2：运行测试验证红灯**

在未放入 `sign-up-backgrounds/` 资产前运行：

```bash
cd web/default
bunx tsx --test src/features/auth/auth-layout-background.test.ts src/features/auth/api.test.ts
```

预期：命令失败。失败原因来自缺少 `sign-up-backgrounds/` 目录或 20 张 PNG 资产，不应是 TypeScript 语法错误。

- [x] **步骤 A3：记录红灯结果**

红灯结果：`auth-layout-background.test.ts` 因 `ENOENT` 无法扫描 `assets/sign-up-backgrounds/` 目录失败；`api.test.ts` 通过。

---

## 3. 任务 B：生成并接入 20 张 PNG 背景资产

**文件：**

- 创建：`web/default/src/features/auth/assets/sign-up-backgrounds/signup-moe-native-01.png` 到 `signup-moe-native-20.png`
- 删除：`web/default/src/features/auth/assets/sign-up-anime-girl.jpg`
- 修改：`web/default/src/features/auth/sign-up/index.tsx`

- [x] **步骤 B1：使用 `image_generator` 子代理逐张产出候选图**

按用户要求：每个 `image_generator` 子代理只生成 1 张图。每个任务明确要求：

- 调用 `image_gen.imagegen` 或等价 provider-native 生图工具。
- 原生请求 16:9 横图，不能拉伸、裁剪、resize 或 fit。
- 原创二次元萌系可爱风格，适合注册页背景。
- 全年龄、非性化、穿着得体。
- 不模仿现有动漫角色、IP、Logo、商标、水印或签名。
- 不在图片中写任何文字，不标注「AI 生成」。
- 主体不要铺满右侧和中心，给注册表单留出可读空间。

- [x] **步骤 B2：保存最终 20 张 PNG**

将 provider 生成结果复制到：

`web/default/src/features/auth/assets/sign-up-backgrounds/`

文件：

- `signup-moe-native-01.png`
- `signup-moe-native-02.png`
- `signup-moe-native-03.png`
- `signup-moe-native-04.png`
- `signup-moe-native-05.png`
- `signup-moe-native-06.png`
- `signup-moe-native-07.png`
- `signup-moe-native-08.png`
- `signup-moe-native-09.png`
- `signup-moe-native-10.png`
- `signup-moe-native-11.png`
- `signup-moe-native-12.png`
- `signup-moe-native-13.png`
- `signup-moe-native-14.png`
- `signup-moe-native-15.png`
- `signup-moe-native-16.png`
- `signup-moe-native-17.png`
- `signup-moe-native-18.png`
- `signup-moe-native-19.png`
- `signup-moe-native-20.png`

要求：

- 每个文件前 8 字节是 PNG magic bytes `89 50 4E 47 0D 0A 1A 0A`。
- PIL 识别格式为 PNG。
- 尺寸为 `1672x941`，宽高比约 1.7768，接近 16:9。
- 不提交 `.generated-backgrounds/`、候选预览 contact sheet 或 provider session 目录。

- [x] **步骤 B3：删除旧单图并切换注册页 import**

删除：

`web/default/src/features/auth/assets/sign-up-anime-girl.jpg`

修改 `web/default/src/features/auth/sign-up/index.tsx`：

```ts
import signUpAnimeGirlBackground from '../assets/sign-up-backgrounds/signup-moe-native-13.png'
```

保留：

```tsx
<AuthLayout backgroundImageSrc={signUpAnimeGirlBackground}>
```

不得修改注册表单、服务条款或业务逻辑。

---

## 4. 任务 C：确认布局能力保持正确

**文件：**

- 只读或最小修改：`web/default/src/features/auth/auth-layout.tsx`

- [x] **步骤 C1：确认 `AuthLayoutProps` 与 `hasBackground`**

确认存在：

```ts
type AuthLayoutProps = {
  children: React.ReactNode
  backgroundImageSrc?: string
}

const hasBackground = Boolean(props.backgroundImageSrc)
```

遵守 `web/default/AGENTS.md`：组件 props 非必要不要解构，继续使用 `props.backgroundImageSrc`。

- [x] **步骤 C2：确认装饰性背景与层级**

确认背景层仅在 `hasBackground` 时渲染，并包含：

- `<img src={props.backgroundImageSrc} alt='' aria-hidden='true' ... />`
- `object-cover object-center`
- 遮罩 `bg-background/80`、`lg:bg-background/45`
- 内容层 `relative z-10`
- 首页 Logo / 系统名链接 `z-20`

- [x] **步骤 C3：确认表单可读性分支**

确认表单容器在 `hasBackground` 分支中包含：

- `rounded-3xl`
- `border`
- `bg-background/90`
- `shadow-2xl`
- `backdrop-blur-xl`

未传背景时保留默认认证页布局，例如 `items-center pt-16 sm:pt-0`。

---

## 5. 任务 D：本地验证

- [x] **步骤 D1：运行定向测试**

```bash
cd web/default
bunx tsx --test src/features/auth/auth-layout-background.test.ts src/features/auth/api.test.ts
```

最终结果：`tests 8`、`pass 8`、`fail 0`。

- [x] **步骤 D2：运行 i18n 同步**

```bash
cd web/default
bun run i18n:sync
```

结果：命令完成，未产生需要提交的 locale 变更。

- [x] **步骤 D3：运行 TypeScript 检查**

```bash
cd web/default
bun run typecheck
```

结果：`tsc -b` 通过。

- [x] **步骤 D4：运行生产构建**

```bash
cd web/default
bun run build
```

结果：Rsbuild 通过，产物包含 `dist/static/image/signup-moe-native-13...png`。

---

## 6. 任务 E：并发只读审查

完成实现和本地验证后，派发至少 3 个并发只读审查子代理。所有子代理不得修改文件、不得运行项目级 build/test/lint/formatter。

- [x] **审查 1：需求与资产审查**
  - 检查是否真正使用 PNG 美图资产，而不是继续使用旧 SVG/JPG。
  - 检查 20 张图片是否纳入暂存、格式真实、尺寸约 16:9。
  - 检查是否未提交候选图缓存、预览图、临时生成目录。

- [x] **审查 2：前端布局审查**
  - 检查 `AuthLayout` 层级、遮罩、表单卡片可读性和 Logo 链接可点击性。
  - 检查只有 `SignUp` 传入背景，其他认证页不受影响。

- [x] **审查 3：测试与构建契约审查**
  - 检查测试是否覆盖 20 张 PNG 资产、wiring、默认页不受影响和可读性样式。
  - 检查测试不依赖与当前 TS target 不兼容的 API。

如果任一审查返回 fail 或 important 阻塞项，先验证反馈，再修复，并重新进行只读复审，直到全部 pass。

复审结果：资产暂存复审、测试精确复审、PNG 格式复核均通过。

---

## 7. 任务 F：提交、合并、推送、清理

- [ ] **步骤 F1：提交前检查工作树**

在修正 worktree 根目录运行：

```bash
git status --short --untracked-files=all
git diff
git diff --cached
git diff --cached --name-only
```

确认仅包含 Issue #7 文件：

- `docs/superpowers/specs/2026-05-19-issue-7-auth-background-spec.md`
- `docs/superpowers/plans/2026-05-19-issue-7-auth-background.md`
- `web/default/src/features/auth/assets/sign-up-anime-girl.jpg`（删除）
- `web/default/src/features/auth/assets/sign-up-backgrounds/signup-moe-native-01.png` 到 `signup-moe-native-20.png`
- `web/default/src/features/auth/auth-layout-background.test.ts`
- `web/default/src/features/auth/sign-up/index.tsx`

不得提交 `.generated-backgrounds/`、候选 contact sheet、provider session 目录、`node_modules/`、无关未跟踪文件或主工作区无关文件。

- [ ] **步骤 F2：提交修正分支**

暂存 Issue #7 文件并提交。提交信息遵循中文 Conventional Commit，例如：

```text
fix(auth): 使用20张生成图片完善注册页背景
```

- [ ] **步骤 F3：合并回主分支并推送**

回到主工作区，确认主分支状态，合并修正分支。若主分支有用户本地改动或远端差异，必须保留并正常合并，不得重写历史或覆盖用户改动。合并后推送到 `deploy/main`。

- [ ] **步骤 F4：清理 worktree 与分支**

合并完成后清理：

```bash
git worktree remove .worktrees/issue-7-twenty-backgrounds
git branch -d worktree/issue-7-twenty-backgrounds
```

- [ ] **步骤 F5：最终状态确认**

在主工作区运行：

```bash
git status -sb --untracked-files=all
git worktree list
git log --oneline --decorate -8
```

确认：

- 修正提交已进入 `main`。
- `deploy/main` 已包含修正提交。
- 修正 worktree 已清理。
- 主工作区中与 Issue #7 无关的文件仍保留且未被提交。
