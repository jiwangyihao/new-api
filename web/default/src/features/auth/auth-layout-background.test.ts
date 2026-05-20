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
import { readdirSync, readFileSync } from 'node:fs'
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

function isPng(imageBytes: Buffer): boolean {
  return imageBytes.subarray(0, 8).equals(PNG_SIGNATURE)
}

function isJpeg(imageBytes: Buffer): boolean {
  return imageBytes[0] === 0xff && imageBytes[1] === 0xd8 && imageBytes[2] === 0xff
}

const authLayoutSource = readSource('./auth-layout.tsx')
const signUpSource = readSource('./sign-up/index.tsx')
const signInSource = readSource('./sign-in/index.tsx')
const PNG_SIGNATURE = Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a])
const generatedBackgroundFiles = readdirSync(
  new URL('./assets/sign-up-backgrounds/', import.meta.url)
).filter((fileName) => /^signup-moe-native-\d{2}\.(png|jpe?g)$/u.test(fileName))
const expectedBackgroundFiles = Array.from(
  { length: 20 },
  (_, index) => `signup-moe-native-${String(index + 1).padStart(2, '0')}.png`
)
const forbiddenImageMarkers = [
  'AI-generated',
  'No external image source',
  'Intended for commercial use',
  'Pollinations',
  '<script',
  '<foreignObject',
  '<iframe',
  'base64',
]

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

  test('wires the generated background pool only into sign-up', () => {
    assertIncludes(signUpSource, 'const signUpAnimeGirlBackgrounds = [')
    assertIncludes(signUpSource, 'Math.floor(Math.random() * signUpAnimeGirlBackgrounds.length)')
    assert.match(signUpSource, /const\s+signUpAnimeGirlBackground\s*=\s*signUpAnimeGirlBackgrounds\[/)
    for (const fileName of expectedBackgroundFiles) {
      assert.match(signUpSource, new RegExp(`sign-up-backgrounds/${fileName}`))
    }
    assert.doesNotMatch(signUpSource, /sign-up-anime-girl\.(svg|jpg)/)
    assert.match(signUpSource, /backgroundImageSrc=\{signUpAnimeGirlBackground\}/)
    assert.doesNotMatch(signInSource, /backgroundImageSrc/)
  })

  test('keeps exactly twenty generated local background assets', () => {
    assert.deepEqual([...generatedBackgroundFiles].sort(), expectedBackgroundFiles)
  })

  test('keeps generated images as unlabelled local raster assets', () => {
    for (const fileName of generatedBackgroundFiles) {
      const imageBytes = readBinary(`./assets/sign-up-backgrounds/${fileName}`)
      assert.ok(isPng(imageBytes) || isJpeg(imageBytes), `${fileName} must be PNG or JPEG`)

      const imageText = imageBytes.toString('latin1')
      for (const forbidden of forbiddenImageMarkers) {
        assertExcludes(imageText, forbidden)
      }
    }
  })
})
