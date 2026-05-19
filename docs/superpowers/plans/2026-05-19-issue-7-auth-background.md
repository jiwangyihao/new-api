# Issue #7 注册页背景图替换实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 将 default 前端注册页替换为带无明显版权风险二次元女生风格背景图的注册界面，并保持桌面端和移动端表单可读。

**架构：** 使用项目内原创 SVG 作为静态背景资产；`AuthLayout` 增加最小可选 `backgroundImageSrc` prop，并将背景图、遮罩和半透明卡片样式限制在 `hasBackground` 分支；`SignUp` 独占传入背景图，登录和其他认证页不变。通过 source-level TDD 测试锁定背景授权记录、无外链、注册页 wiring、共享布局默认不启用背景视觉，并通过 Rsbuild 生产构建验证 SVG 打包。

**技术栈：** React 19、TypeScript、TanStack Router、i18next、Tailwind CSS、Rsbuild、Bun。

---

## 0. 执行约束

- 实现 worktree：`C:/Users/34404/source/repos/new-api/.worktrees/issue-7-auth-background`。
- 主工作区：`C:/Users/34404/source/repos/new-api`，当前存在与 Issue #7 无关的用户改动；不得删除、移动、格式化、暂存或提交这些改动。
- 规格文件：`C:/Users/34404/source/repos/new-api/.worktrees/issue-7-auth-background/docs/superpowers/specs/2026-05-19-issue-7-auth-background-spec.md`。
- 本计划文件：`C:/Users/34404/source/repos/new-api/.worktrees/issue-7-auth-background/docs/superpowers/plans/2026-05-19-issue-7-auth-background.md`。
- 实现前必须读取并遵守根目录 `AGENTS.md` 与 `web/default/AGENTS.md`。
- 不修改 classic 前端。
- 不新增依赖。
- 不修改注册 API、OAuth、微信验证码、Turnstile、邀请码、条款同意或其他注册业务逻辑。
- 不修改或删除受保护品牌、版权、归属信息，例如 `new-api`、`QuantumNous`、AGPL 版权头。
- `web/default/src/env.d.ts` 已通过 `@rsbuild/core/types` 获得 `*.svg` 声明；不得重复添加本地 `declare module '*.svg'`。
- 子代理执行实现时不要运行项目级 build/test/lint/typecheck/formatter；主控在合并后统一运行验证。
- 每次提交前必须执行 `git status --short --untracked-files=all`、`git diff -- <本任务文件>`、`git diff --cached -- <本任务文件>` 和 `git diff --cached --name-only`，确认只包含 Issue #7 文件。
- 最终必须合并回主分支并清理 `.worktrees/issue-7-auth-background`。

## 1. 文件结构

- 创建：`web/default/src/features/auth/auth-layout-background.test.ts`
  - Source-level 契约测试，先红灯后绿灯。
  - 覆盖 `AuthLayout` 可选背景、背景专用样式分支、`SignUp` wiring、`SignIn` 不受影响、SVG 授权和无外部资源。
- 创建：`web/default/src/features/auth/assets/sign-up-anime-girl.svg`
  - 自包含原创二次元女生风格 SVG 背景资产。
  - 文件头包含 AI 生成和授权记录。
- 修改：`web/default/src/features/auth/auth-layout.tsx`
  - `AuthLayoutProps` 新增 `backgroundImageSrc?: string`。
  - 当且仅当 `backgroundImageSrc` 存在时渲染装饰背景图、遮罩层和注册页专用表单卡片视觉。
  - 默认认证页不启用背景图或注册页专用卡片视觉。
- 修改：`web/default/src/features/auth/sign-up/index.tsx`
  - 静态 import SVG。
  - `<AuthLayout backgroundImageSrc={signUpAnimeGirlBackground}>`。
- 保持不改：`web/default/src/env.d.ts`
  - Rsbuild 已内置 SVG import 类型；只在实测缺失时另行处理。

---

## 2. 任务 A：编写红灯契约测试

**文件：**
- 创建：`web/default/src/features/auth/auth-layout-background.test.ts`
- 只读参考：`web/default/src/features/auth/api.test.ts`
- 只读参考：`web/default/src/features/auth/auth-layout.tsx`
- 只读参考：`web/default/src/features/auth/sign-up/index.tsx`
- 只读参考：`web/default/src/features/auth/sign-in/index.tsx`

- [ ] **步骤 A1：创建失败测试文件**

创建 `web/default/src/features/auth/auth-layout-background.test.ts`，内容如下：

```ts
/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { describe, test } from 'node:test'

function readSource(relativePath: string): string {
  return readFileSync(new URL(relativePath, import.meta.url), 'utf8')
}

function assertIncludes(source: string, expected: string): void {
  assert.ok(source.includes(expected), `expected source to include ${expected}`)
}

function assertExcludes(source: string, unexpected: string): void {
  assert.ok(
    !source.includes(unexpected),
    `expected source not to include ${unexpected}`
  )
}

const authLayoutSource = readSource('./auth-layout.tsx')
const signUpSource = readSource('./sign-up/index.tsx')
const signInSource = readSource('./sign-in/index.tsx')

describe('auth layout sign-up background', () => {
  test('keeps background rendering optional and decorative', () => {
    assert.match(authLayoutSource, /backgroundImageSrc\?:\s*string/)
    assert.match(
      authLayoutSource,
      /const\s+hasBackground\s*=\s*Boolean\(props\.backgroundImageSrc\)/
    )
    assert.match(authLayoutSource, /hasBackground\s*&&\s*\(/)
    assert.match(authLayoutSource, /src=\{props\.backgroundImageSrc\}/)
    assert.match(authLayoutSource, /aria-hidden=['"]true['"]/)
    assert.match(authLayoutSource, /alt=['"]{2}/)
    assertIncludes(authLayoutSource, 'object-cover')
    assertIncludes(authLayoutSource, 'bg-background/80')
    assertIncludes(authLayoutSource, 'lg:bg-background/45')
  })

  test('keeps sign-up card readability styles behind the background branch', () => {
    assert.match(
      authLayoutSource,
      /hasBackground\s*\?\s*['"][^'"]*rounded-3xl[^'"]*bg-background\/90[^'"]*shadow-2xl[^'"]*backdrop-blur-xl/s
    )
    assertIncludes(
      authLayoutSource,
      'mx-auto flex w-full flex-col justify-center space-y-2 px-4 py-8 sm:w-[480px] sm:p-8'
    )
    assert.match(authLayoutSource, /:\s*'items-center pt-16 sm:pt-0'/)
  })

  test('wires the anime girl background only into sign-up', () => {
    assert.match(signUpSource, /sign-up-anime-girl\.svg/)
    assert.match(signUpSource, /backgroundImageSrc=\{signUpAnimeGirlBackground\}/)
    assert.doesNotMatch(signInSource, /backgroundImageSrc/)
  })

  test('documents the generated SVG source and excludes external assets', () => {
    const svgSource = readSource('./assets/sign-up-anime-girl.svg')

    assertIncludes(svgSource, 'AI-generated original illustration')
    assertIncludes(svgSource, 'No external image source')
    assertIncludes(svgSource, 'Intended for commercial use')
    assert.match(svgSource, /<svg\b/)
    assert.match(svgSource, /<(path|circle|ellipse|linearGradient)\b/)

    for (const forbidden of [
      'data:',
      'base64',
      '<image',
      '<foreignObject',
      '<script',
      '@import',
      'xlink:href',
      'href="http',
      "href='http",
      'src="http',
      "src='http",
      'url(http',
      'url(//',
    ]) {
      assertExcludes(svgSource, forbidden)
    }
  })
})
```

- [ ] **步骤 A2：运行测试验证红灯**

运行：

```bash
cd web/default
bunx tsx --test src/features/auth/auth-layout-background.test.ts src/features/auth/api.test.ts
```

预期：命令失败。`auth-layout-background.test.ts` 的失败原因应包含缺少 `backgroundImageSrc`、`hasBackground`、`sign-up-anime-girl.svg` 或 SVG 文件，不应是 TypeScript 语法错误、路径错误之外的测试文件自身错误。

- [ ] **步骤 A3：记录红灯结果**

在执行记录中保存红灯输出要点：失败测试名称、失败断言和退出码。不得因为测试红灯而修改测试期望来适配旧代码。

---

## 3. 任务 B：新增 SVG 背景资产

**文件：**
- 创建：`web/default/src/features/auth/assets/sign-up-anime-girl.svg`

- [ ] **步骤 B1：创建资产目录和 SVG 文件**

创建 `web/default/src/features/auth/assets/sign-up-anime-girl.svg`，内容如下。该 SVG 自包含，使用矢量图形绘制原创二次元女生、柔和光效和抽象科技背景：

```xml
<!--
AI-generated original illustration for new-api Issue #7.
No external image source, stock asset, character IP, logo, or copyrighted work was used.
Intended for commercial use in this project.
-->
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1600 1200" role="img" aria-label="Original anime girl illustration background">
  <defs>
    <linearGradient id="unusedSkyGradient" x1="0" y1="0" x2="1" y2="1">
      <stop offset="0" stop-color="#fff7fb"/>
      <stop offset="1" stop-color="#dfe8ff"/>
    </linearGradient>
  </defs>
  <rect width="1600" height="1200" fill="#f7f0ff"/>
  <circle cx="1130" cy="310" r="430" fill="#dfe8ff" opacity="0.82"/>
  <circle cx="1260" cy="420" r="280" fill="#f6c8eb" opacity="0.46"/>
  <path d="M0 934c260-118 501-150 736-96 235 55 402 59 864-116v478H0Z" fill="#ffffff" opacity="0.58"/>
  <path d="M0 1013c255-83 507-96 756-38 259 60 482 64 844-54v279H0Z" fill="#d7e2ff" opacity="0.72"/>
  <g opacity="0.28" stroke="#7c8ee8" stroke-width="2">
    <path d="M135 183h466M92 320h545M1006 191h381M1032 344h450M105 1054h525M980 1010h455"/>
    <path d="M220 120v348M397 96v412M1196 118v388M1354 166v284"/>
  </g>
  <g fill="#ffffff" opacity="0.78">
    <circle cx="192" cy="217" r="7"/>
    <circle cx="363" cy="112" r="5"/>
    <circle cx="1316" cy="236" r="8"/>
    <circle cx="1435" cy="411" r="5"/>
    <circle cx="1116" cy="137" r="4"/>
    <circle cx="524" cy="274" r="6"/>
  </g>
  <g>
    <ellipse cx="1040" cy="1060" rx="250" ry="44" fill="#8b7bd9" opacity="0.22"/>
    <path d="M835 433c-82 66-120 188-97 320 22 128 116 248 300 252 172 4 289-90 321-225 35-148-10-300-112-371-103-72-301-62-412 24Z" fill="#6e63bc"/>
    <path d="M806 592c-64 52-89 150-73 251 14 87 60 155 138 195-34-111-37-245 14-352 23-48 12-94-79-94Z" fill="#4b5599" opacity="0.9"/>
    <path d="M1298 580c81 66 88 223 30 335-29 56-70 97-123 121 35-117 24-259-36-360-30-51 17-95 129-96Z" fill="#d88dcc" opacity="0.86"/>
    <path d="M897 734c-89 56-144 161-168 310h628c-24-150-80-255-170-310-72 64-216 64-290 0Z" fill="#c5d2ff"/>
    <path d="M900 742c78 74 209 76 287 0 17 39 32 82 44 128H856c11-46 26-89 44-128Z" fill="#eef4ff"/>
    <path d="M921 478c-42 48-62 111-58 180 7 115 80 193 181 193 102 0 176-78 183-193 5-72-17-138-63-186-68-70-178-67-243 6Z" fill="#ffe7d8"/>
    <path d="M906 589c-35-8-64 17-60 61 4 43 36 68 72 54Zm276 0c36-8 65 17 60 61-4 43-36 68-72 54Z" fill="#ffd4c3"/>
    <path d="M864 556c99-20 205-83 255-164 54 76 91 121 154 151-5-67-42-136-100-177-82-58-218-54-295 12-54 46-83 115-84 185 22-1 45-3 70-7Z" fill="#7061bf"/>
    <path d="M963 662c19 17 48 17 67 0M1082 662c18 17 47 17 66 0" stroke="#47517f" stroke-width="14" stroke-linecap="round" fill="none"/>
    <circle cx="996" cy="690" r="9" fill="#f2a1ac" opacity="0.6"/>
    <circle cx="1117" cy="690" r="9" fill="#f2a1ac" opacity="0.6"/>
    <path d="M1030 746c29 20 60 20 89 0" stroke="#d4778a" stroke-width="9" stroke-linecap="round" fill="none"/>
    <path d="M1002 833c29 31 78 31 107 0" stroke="#8fa0ff" stroke-width="7" stroke-linecap="round" fill="none" opacity="0.7"/>
    <path d="M763 928c-64 36-122 93-168 171h177c22-48 50-88 84-121Z" fill="#cbd7ff"/>
    <path d="M1324 928c64 36 122 93 168 171h-177c-22-48-50-88-84-121Z" fill="#cbd7ff"/>
  </g>
  <g opacity="0.55" fill="none" stroke-linecap="round">
    <path d="M688 274c66 39 128 54 190 45" stroke="#8aa7ff" stroke-width="6"/>
    <path d="M1212 264c-77 43-151 60-222 51" stroke="#d79cff" stroke-width="6"/>
    <path d="M704 357c59 18 105 20 139 8" stroke="#ffffff" stroke-width="5"/>
    <path d="M1198 354c-60 20-107 23-143 10" stroke="#ffffff" stroke-width="5"/>
  </g>
</svg>
```

- [ ] **步骤 B2：确认资产没有外部资源**

运行定向搜索（使用专用 search 工具或后续测试），确认该 SVG 不包含：`data:`、`base64`、`<image`、`<foreignObject`、`<script`、`@import`、`xlink:href`、`href="http`、`href='http`、`src="http`、`src='http`、`url(http`、`url(//`。

---

## 4. 任务 C：实现注册页背景布局

**文件：**
- 修改：`web/default/src/features/auth/auth-layout.tsx`
- 修改：`web/default/src/features/auth/sign-up/index.tsx`
- 不修改：`web/default/src/env.d.ts`

- [ ] **步骤 C1：更新 `AuthLayoutProps` 与背景分支**

将 `web/default/src/features/auth/auth-layout.tsx` 修改为以下结构。保留文件头和现有 imports，新增 `cn` import：

```tsx
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { useSystemConfig } from '@/hooks/use-system-config'
import { cn } from '@/lib/utils'
import { Skeleton } from '@/components/ui/skeleton'

type AuthLayoutProps = {
  children: React.ReactNode
  backgroundImageSrc?: string
}

export function AuthLayout(props: AuthLayoutProps) {
  const { t } = useTranslation()
  const { systemName, logo, loading } = useSystemConfig()
  const hasBackground = Boolean(props.backgroundImageSrc)

  return (
    <div className='relative grid min-h-svh max-w-none overflow-hidden'>
      {hasBackground && (
        <div className='absolute inset-0' aria-hidden='true'>
          <img
            src={props.backgroundImageSrc}
            alt=''
            aria-hidden='true'
            className='absolute inset-0 h-full w-full object-cover object-center'
          />
          <div className='bg-background/80 absolute inset-0 backdrop-blur-[1px] sm:bg-background/70 lg:bg-background/45' />
          <div className='absolute inset-0 bg-[radial-gradient(circle_at_72%_38%,transparent_0,transparent_28%,var(--background)_78%)] opacity-95' />
        </div>
      )}

      <Link
        to='/'
        className='absolute top-4 left-4 z-10 flex items-center gap-2 transition-opacity hover:opacity-80 sm:top-8 sm:left-8'
      >
        <div className='relative h-8 w-8'>
          {loading ? (
            <Skeleton className='absolute inset-0 rounded-full' />
          ) : (
            <img
              src={logo}
              alt={t('Logo')}
              className='h-8 w-8 rounded-full object-cover'
            />
          )}
        </div>
        {loading ? (
          <Skeleton className='h-6 w-24' />
        ) : (
          <h1 className='text-xl font-medium'>{systemName}</h1>
        )}
      </Link>

      <div
        className={cn(
          'container relative z-10 flex',
          hasBackground
            ? 'items-start py-24 sm:items-center sm:py-0'
            : 'items-center pt-16 sm:pt-0'
        )}
      >
        <div
          className={cn(
            'mx-auto flex w-full flex-col justify-center space-y-2 px-4 py-8 sm:w-[480px] sm:p-8',
            hasBackground
              ? 'rounded-3xl border bg-background/90 shadow-2xl backdrop-blur-xl'
              : ''
          )}
        >
          {props.children}
        </div>
      </div>
    </div>
  )
}
```

关键点：

- 使用 `min-h-svh`，避免移动端长注册表单在小屏 `h-svh` 中被裁切。
- 背景图和遮罩层只在 `hasBackground` 为真时渲染。
- 根内容容器的移动端 `items-start py-24` 只在 `hasBackground` 分支启用；默认分支保留原有 `items-center pt-16 sm:pt-0`，避免影响登录、找回密码、OTP、OAuth 等其他认证页。
- `bg-background/90`、`shadow-2xl`、`backdrop-blur-xl`、`rounded-3xl` 只在 `hasBackground ? ... : ''` 分支。
- 背景 `<img>` 作为装饰，`alt=''` 且 `aria-hidden='true'`。
- 不新增 `backgroundImageAlt` prop；组件 props 按 `web/default/AGENTS.md` 要求使用 `props.xxx`，不解构组件 props。

- [ ] **步骤 C2：注册页传入背景资产**

修改 `web/default/src/features/auth/sign-up/index.tsx` imports：

```tsx
import signUpAnimeGirlBackground from '../assets/sign-up-anime-girl.svg'
```

将：

```tsx
<AuthLayout>
```

改为：

```tsx
<AuthLayout backgroundImageSrc={signUpAnimeGirlBackground}>
```

不得修改 `SignUpForm`、`TermsFooter`、注册表单字段或注册业务逻辑。

- [ ] **步骤 C3：确认不修改 SVG 类型声明**

读取 `web/default/src/env.d.ts`，确认仍保留 `/// <reference types="@rsbuild/core/types" />`，且没有新增重复的 `declare module '*.svg'`。

---

## 5. 任务 D：绿灯验证与提交实现

**文件：**
- `web/default/src/features/auth/auth-layout-background.test.ts`
- `web/default/src/features/auth/assets/sign-up-anime-girl.svg`
- `web/default/src/features/auth/auth-layout.tsx`
- `web/default/src/features/auth/sign-up/index.tsx`

- [ ] **步骤 D1：运行定向测试验证绿灯**

运行：

```bash
cd web/default
bunx tsx --test src/features/auth/auth-layout-background.test.ts src/features/auth/api.test.ts
```

预期：全部通过，`auth-layout-background.test.ts` 和既有 `api.test.ts` 无失败。

- [ ] **步骤 D2：运行 i18n 同步**

运行：

```bash
cd web/default
bun run i18n:sync
```

预期：命令退出码为 0。本次不新增用户可见文案，原则上不应产生 locale diff。若 locale 文件出现 diff，必须确认是否由 Issue #7 直接引起；否则不得提交 locale 变更。

- [ ] **步骤 D3：运行类型检查**

运行：

```bash
cd web/default
bun run typecheck
```

预期：`tsc -b` 退出码为 0。

- [ ] **步骤 D4：运行生产构建验证 SVG 打包**

运行：

```bash
cd web/default
bun run build
```

预期：Rsbuild 生产构建退出码为 0，证明 SVG 静态 import 可打包。

- [ ] **步骤 D5：检查 diff 范围**

运行：

```bash
git status --short --untracked-files=all
git diff -- web/default/src/features/auth/auth-layout.tsx web/default/src/features/auth/sign-up/index.tsx web/default/src/features/auth/auth-layout-background.test.ts web/default/src/features/auth/assets/sign-up-anime-girl.svg
git diff --cached -- web/default/src/features/auth/auth-layout.tsx web/default/src/features/auth/sign-up/index.tsx web/default/src/features/auth/auth-layout-background.test.ts web/default/src/features/auth/assets/sign-up-anime-girl.svg
git diff --cached --name-only
```

预期：只有 Issue #7 文件出现变更；`web/default/src/env.d.ts` 不应出现 diff；locale 文件不应出现 diff，除非有明确 Issue #7 原因。

- [ ] **步骤 D6：提交实现**

暂存并提交：

```bash
git add web/default/src/features/auth/auth-layout.tsx \
  web/default/src/features/auth/sign-up/index.tsx \
  web/default/src/features/auth/auth-layout-background.test.ts \
  web/default/src/features/auth/assets/sign-up-anime-girl.svg
git diff --cached --check
git diff --cached --name-only
git commit -m "feat(auth): 替换注册页背景图"
```

提交前再次确认暂存区不包含主工作区或其他 Issue 的文件。

---

## 6. 任务 E：最终验证、合并回主分支并清理 worktree

**文件：**
- Git 分支和 worktree 状态。
- 不直接修改业务文件。

- [ ] **步骤 E1：实现审查通过后执行最终验证**

在 `C:/Users/34404/source/repos/new-api/.worktrees/issue-7-auth-background` 运行：

```bash
cd web/default
bunx tsx --test src/features/auth/auth-layout-background.test.ts src/features/auth/api.test.ts
bun run i18n:sync
bun run typecheck
bun run build
```

预期全部退出码为 0。

- [ ] **步骤 E2：确认 worktree 分支干净**

运行：

```bash
git status -sb --untracked-files=all
git log --oneline --decorate -5
git diff
git diff --cached
```

预期：工作树干净；最新提交包含规格和实现提交。

- [ ] **步骤 E3：切回主工作区并保护无关改动**

在 `C:/Users/34404/source/repos/new-api` 运行：

```bash
git status -sb --untracked-files=all
git diff --cached --name-only
git diff --name-only
git worktree list
```

预期：当前分支必须是 `main`。暂存区必须为空；若 `git diff --cached --name-only` 有任何输出，停止并要求用户处理，禁止自动 merge。未暂存 tracked diff 可以存在，但必须确认不包含 Issue #7 将修改的文件：`docs/superpowers/specs/2026-05-19-issue-7-auth-background-spec.md`、`docs/superpowers/plans/2026-05-19-issue-7-auth-background.md`、`web/default/src/features/auth/auth-layout.tsx`、`web/default/src/features/auth/sign-up/index.tsx`、`web/default/src/features/auth/auth-layout-background.test.ts`、`web/default/src/features/auth/assets/sign-up-anime-girl.svg`。不得清理、stash、reset 或提交这些无关改动。合并前记录它们，后续只合并 worktree 分支提交。

- [ ] **步骤 E4：合并分支到 main**

在主工作区确认 `main`、暂存区为空且 tracked diff 不与 Issue #7 文件重叠后运行：

```bash
git merge --no-ff --no-edit worktree/issue-7-auth-background
```

若发生冲突：

- 只处理 Issue #7 文件冲突。
- 不覆盖主工作区无关改动。
- 逐项审阅冲突，保留本次 Issue #7 规格、计划、测试、SVG、AuthLayout 和 SignUp 变更。
- 解决后运行 `git status --short --untracked-files=all`、`git diff`、`git diff --cached`，再提交 merge。

- [ ] **步骤 E5：推送 main 到 fork 远端**

如果用户仍要求沿用先前模式并更新 fork 远端，且确认 `deploy` 指向 `https://github.com/jiwangyihao/new-api.git`，运行：

```bash
git push deploy main:main
```

预期：远端 `deploy/main` 更新到包含 Issue #7 merge commit 的提交。若用户只要求本地合并，不执行推送。

- [ ] **步骤 E6：清理 worktree 和分支**

在主工作区运行：

```bash
git worktree remove .worktrees/issue-7-auth-background
git branch -d worktree/issue-7-auth-background
git worktree list
git status -sb --untracked-files=all
```

预期：`.worktrees/issue-7-auth-background` 已删除，`worktree/issue-7-auth-background` 已删除。`git worktree list` 只显示应保留的 worktree。主工作区中与 Issue #7 无关的用户改动仍保持原样，未被清理或提交。
