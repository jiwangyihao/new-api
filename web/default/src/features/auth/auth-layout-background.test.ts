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

function readBinary(relativePath: string): Buffer {
  return readFileSync(new URL(relativePath, import.meta.url))
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
      /hasBackground\s*\?\s*['"][^'"]*rounded-3xl[^'"]*border[^'"]*bg-background\/90[^'"]*shadow-2xl[^'"]*backdrop-blur-xl/s
    )
    assertIncludes(
      authLayoutSource,
      'mx-auto flex w-full flex-col justify-center space-y-2 px-4 py-8 sm:w-[480px] sm:p-8'
    )
    assert.match(authLayoutSource, /:\s*'items-center pt-16 sm:pt-0'/)
  })

  test('wires the generated JPG background only into sign-up', () => {
    assert.match(signUpSource, /sign-up-anime-girl\.jpg/)
    assert.doesNotMatch(signUpSource, /sign-up-anime-girl\.svg/)
    assert.match(signUpSource, /backgroundImageSrc=\{signUpAnimeGirlBackground\}/)
    assert.doesNotMatch(signInSource, /backgroundImageSrc/)
  })

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
      '<foreignObject',
      '<iframe',
      'data:',
      'base64',
      'http://',
      'https://',
    ]) {
      assertExcludes(imageText, forbidden)
    }
  })
})
