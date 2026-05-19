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
