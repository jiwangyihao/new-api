# Issue #7 注册页背景图替换修正计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 将 default 前端注册页替换为带无明显版权风险二次元女生风格美图背景的注册界面，并保持桌面端和移动端表单可读。

**架构：** 使用生成式图片流程产出的本地 JPG 作为静态背景资产；`AuthLayout` 保持最小可选 `backgroundImageSrc` prop，并将背景图、遮罩和半透明卡片样式限制在 `hasBackground` 分支；`SignUp` 独占传入背景图，登录和其他认证页不变。通过 source-level TDD 测试锁定 JPG 资产、无专门来源标注、注册页 wiring、共享布局默认不启用背景视觉，并通过 Rsbuild 生产构建验证 JPG 打包。

**技术栈：** React 19、TypeScript、TanStack Router、i18next、Tailwind CSS、Rsbuild、Bun。

---

## 0. 执行约束

- 实现 worktree：`C:/Users/34404/source/repos/new-api/.worktrees/issue-7-generated-background`。
- 主工作区：`C:/Users/34404/source/repos/new-api`，可能存在与 Issue #7 无关的用户改动；不得删除、移动、格式化、暂存或提交这些改动。
- 规格文件：`C:/Users/34404/source/repos/new-api/.worktrees/issue-7-generated-background/docs/superpowers/specs/2026-05-19-issue-7-auth-background-spec.md`。
- 本计划文件：`C:/Users/34404/source/repos/new-api/.worktrees/issue-7-generated-background/docs/superpowers/plans/2026-05-19-issue-7-auth-background.md`。
- 实现前必须读取并遵守根目录 `AGENTS.md` 与 `web/default/AGENTS.md`。
- 不修改 classic 前端。
- 不新增依赖。
- 不修改注册 API、OAuth、微信验证码、Turnstile、邀请码、条款同意或其他注册业务逻辑。
- 不修改或删除受保护品牌、版权、归属信息，例如 `new-api`、`QuantumNous`、AGPL 版权头。
- 用户明确要求不要在图片或用户可见资源中专门标注「AI 生成」。
- `web/default/src/env.d.ts` 已通过 `@rsbuild/core/types` 获得常见静态资源声明；不得主动添加本地 `declare module '*.jpg'`，除非 typecheck 证明缺失。
- 子代理执行实现审查时不要运行项目级 build/test/lint/typecheck/formatter；主控统一运行验证。
- 每次提交前必须执行 `git status --short --untracked-files=all`、`git diff`、`git diff --cached` 和 `git diff --cached --name-only`，确认只包含 Issue #7 文件。
- 最终必须合并回主分支并清理 `.worktrees/issue-7-generated-background`。

## 1. 文件结构

- 修改：`web/default/src/features/auth/auth-layout-background.test.ts`
  - Source-level 契约测试，按红灯后绿灯验证。
  - 覆盖 `AuthLayout` 可选背景、背景专用样式分支、`SignUp` wiring、`SignIn` 不受影响、JPG 资产存在且不含专门来源标注或 SVG/XML 入口。
- 创建：`web/default/src/features/auth/assets/sign-up-anime-girl.jpg`
  - 生成式图片流程产出的原创二次元女生风格 JPG 背景资产。
  - 文件本身不包含文字、水印、签名、Logo、商标或专门来源标注。
- 删除：`web/default/src/features/auth/assets/sign-up-anime-girl.svg`
  - 删除先前错误方向的手写 SVG 资产，避免继续引用。
- 修改：`web/default/src/features/auth/sign-up/index.tsx`
  - 静态 import JPG。
  - `<AuthLayout backgroundImageSrc={signUpAnimeGirlBackground}>`。
- 保持：`web/default/src/features/auth/auth-layout.tsx`
  - 保留 `backgroundImageSrc?: string`、装饰性 `<img>`、遮罩、`z-20` 首页链接和背景分支表单卡片样式。
- 保持不改：`web/default/src/env.d.ts`
  - Rsbuild 类型若能覆盖 JPG import，不额外声明。

---

## 2. 任务 A：更新红灯契约测试

**文件：**

- 修改：`web/default/src/features/auth/auth-layout-background.test.ts`
- 只读参考：`web/default/src/features/auth/api.test.ts`
- 只读参考：`web/default/src/features/auth/auth-layout.tsx`
- 只读参考：`web/default/src/features/auth/sign-up/index.tsx`
- 只读参考：`web/default/src/features/auth/sign-in/index.tsx`

- [ ] **步骤 A1：把测试从 SVG 契约改为 JPG 契约**

测试文件保留项目版权头、`node:test` 和 source-level 模式。新增 `readBinary()`，并将背景资产断言改为：

```ts
function readBinary(relativePath: string): Buffer {
  return readFileSync(new URL(relativePath, import.meta.url))
}
```

`SignUp` wiring 测试要求：

```ts
test('wires the generated JPG background only into sign-up', () => {
  assert.match(signUpSource, /sign-up-anime-girl\.jpg/)
  assert.doesNotMatch(signUpSource, /sign-up-anime-girl\.svg/)
  assert.match(signUpSource, /backgroundImageSrc=\{signUpAnimeGirlBackground\}/)
  assert.doesNotMatch(signInSource, /backgroundImageSrc/)
})
```

本地图片资产测试要求：

```ts
test('keeps the generated image as an unlabelled local asset', () => {
  const imageBytes = readBinary('./assets/sign-up-anime-girl.jpg')

  assert.equal(imageBytes[0], 0xff)
  assert.equal(imageBytes[1], 0xd8)
  assert.equal(imageBytes[2], 0xff)

  const imageText = imageBytes.toString('latin1')
  for (const forbidden of [
    'AI-generated',
    'No external image source',
    'Intended for commercial use',
    'Pollinations',
    '<svg',
    '<image',
    '<script',
    'data:',
    'base64',
  ]) {
    assertExcludes(imageText, forbidden)
  }
})
```

- [ ] **步骤 A2：运行测试验证红灯**

在未修改生产 import 且未放入 JPG 资产前运行：

```bash
cd web/default
bunx tsx --test src/features/auth/auth-layout-background.test.ts src/features/auth/api.test.ts
```

预期：命令失败。`auth-layout-background.test.ts` 的失败原因应包含仍引用 `sign-up-anime-girl.svg` 或缺少 `sign-up-anime-girl.jpg`，不应是 TypeScript 语法错误。

- [ ] **步骤 A3：记录红灯结果**

在执行记录中保存红灯输出要点：失败测试名称、失败断言和退出码。不得因为测试红灯而修改测试期望来适配旧代码。

---

## 3. 任务 B：生成并接入 JPG 背景资产

**文件：**

- 创建：`web/default/src/features/auth/assets/sign-up-anime-girl.jpg`
- 删除：`web/default/src/features/auth/assets/sign-up-anime-girl.svg`
- 修改：`web/default/src/features/auth/sign-up/index.tsx`

- [ ] **步骤 B1：使用图片生成流程产出候选图**

首选 `image_generator` 子代理生成候选图。提示词必须包含本计划与规格完整路径，并明确：

- 原创二次元女生风格美图，适合注册页背景。
- 可包含萝莉、少女、御姐等二次元人物风格，但不得模仿或复刻现有动漫角色、IP、Logo、商标、水印或签名。
- 不要在图片中写任何文字，不要标注「AI 生成」。
- 画面适合 `object-cover` 全屏背景，表单区域可读。
- 输出为本地图片文件，最终可保存为 JPG。

如果专用子代理报告缺少实际图片生成工具，则必须记录该事实，并使用可访问的生成式图片服务生成原创候选图，仍需满足上述约束。

- [ ] **步骤 B2：选择并保存最终 JPG**

从候选图中选择最适合注册页背景的一张，保存为：

`web/default/src/features/auth/assets/sign-up-anime-girl.jpg`

要求：

- 文件前三个字节是 JPEG magic bytes `FF D8 FF`。
- 不包含文字、水印、签名、Logo、商标或专门来源标注。
- 不提交候选图缓存、预览图或临时生成目录。

- [ ] **步骤 B3：删除旧 SVG 并切换注册页 import**

删除：

`web/default/src/features/auth/assets/sign-up-anime-girl.svg`

修改 `web/default/src/features/auth/sign-up/index.tsx`：

```ts
import signUpAnimeGirlBackground from '../assets/sign-up-anime-girl.jpg'
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

- [ ] **步骤 C1：确认 `AuthLayoutProps` 与 `hasBackground`**

确认存在：

```ts
type AuthLayoutProps = {
  children: React.ReactNode
  backgroundImageSrc?: string
}

const hasBackground = Boolean(props.backgroundImageSrc)
```

遵守 `web/default/AGENTS.md`：组件 props 非必要不要解构，继续使用 `props.backgroundImageSrc`。

- [ ] **步骤 C2：确认装饰性背景与层级**

确认背景层仅在 `hasBackground` 时渲染，并包含：

- `<img src={props.backgroundImageSrc} alt='' aria-hidden='true' ... />`
- `object-cover object-center`
- 遮罩 `bg-background/80`、`lg:bg-background/45`
- 内容层 `relative z-10`
- 首页 Logo / 系统名链接 `z-20`

- [ ] **步骤 C3：确认表单可读性分支**

确认表单容器在 `hasBackground` 分支中包含：

- `rounded-3xl`
- `border`
- `bg-background/90`
- `shadow-2xl`
- `backdrop-blur-xl`

未传背景时保留默认认证页布局，例如 `items-center pt-16 sm:pt-0`。

---

## 5. 任务 D：本地验证

- [ ] **步骤 D1：运行定向测试**

```bash
cd web/default
bunx tsx --test src/features/auth/auth-layout-background.test.ts src/features/auth/api.test.ts
```

预期：`tests 7`、`pass 7`、`fail 0`。

- [ ] **步骤 D2：运行 i18n 同步**

```bash
cd web/default
bun run i18n:sync
```

预期：命令完成。若产生 locale diff，确认是否由本任务直接引起；本任务不新增用户文案，原则上不应提交 locale 变更。

- [ ] **步骤 D3：运行 TypeScript 检查**

```bash
cd web/default
bun run typecheck
```

预期：退出码为 0。若 JPG import 类型缺失，先确认 `@rsbuild/core/types` 是否覆盖；只有确实缺失时才最小修改 `env.d.ts`。

- [ ] **步骤 D4：运行生产构建**

```bash
cd web/default
bun run build
```

预期：退出码为 0，Rsbuild 能打包 JPG 静态资源。

---

## 6. 任务 E：并发只读审查

完成实现和本地验证后，派发至少 3 个并发只读审查子代理。所有子代理不得修改文件、不得运行项目级 build/test/lint/formatter。

- [ ] **审查 1：需求与资产审查**
  - 检查是否真正使用 JPG 美图资产，而不是继续使用 SVG。
  - 检查图片是否不含明显水印、签名、商标、文字或专门来源标注。
  - 检查是否未提交候选图、预览图、临时生成目录。

- [ ] **审查 2：前端布局审查**
  - 检查 `AuthLayout` 层级、遮罩、表单卡片可读性和 Logo 链接可点击性。
  - 检查只有 `SignUp` 传入背景，其他认证页不受影响。

- [ ] **审查 3：测试与构建契约审查**
  - 检查测试是否覆盖 JPG 资产、wiring、默认页不受影响和可读性样式。
  - 检查测试不依赖脆弱或错误的实现细节。

如果任一审查返回 fail 或 important 阻塞项，先验证反馈，再修复，并重新进行至少 3 个并发只读复审，直到全部 pass。

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
- `web/default/src/features/auth/assets/sign-up-anime-girl.jpg`
- `web/default/src/features/auth/assets/sign-up-anime-girl.svg`（删除）
- `web/default/src/features/auth/auth-layout-background.test.ts`
- `web/default/src/features/auth/sign-up/index.tsx`
- 以及必要时 `web/default/src/features/auth/auth-layout.tsx`

不得提交 `.generated-backgrounds/`、候选图、预览图、`node_modules/`、无关未跟踪文件或主工作区无关文件。

- [ ] **步骤 F2：提交修正分支**

暂存 Issue #7 文件并提交。提交信息遵循中文 Conventional Commit，例如：

```text
fix(auth): 使用生成图片修正注册页背景
```

- [ ] **步骤 F3：合并回主分支并推送**

回到主工作区，确认主分支状态，合并修正分支。若主分支有用户本地提交或远端差异，必须保留并正常合并，不得重写历史或覆盖用户改动。合并后推送到 `deploy/main`。

- [ ] **步骤 F4：清理 worktree 与分支**

合并完成后清理：

```bash
git worktree remove .worktrees/issue-7-generated-background
git branch -d worktree/issue-7-generated-background
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
- 主工作区中与 Issue #7 无关的未跟踪文件仍保留且未被提交。
