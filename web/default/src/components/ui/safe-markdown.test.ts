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
import { isSafeHref, safeMarkdownUrlTransform } from './safe-markdown'

describe('safe markdown', () => {
  test('allows only safe href protocols', () => {
    for (const href of [
      'https://example.com',
      'http://example.com',
      'mailto:support@example.com',
      'tel:+123456789',
      '/docs',
      './relative',
      '../parent',
      'relative/path',
      '#section',
    ]) {
      assert.equal(isSafeHref(href), true, href)
    }
  })

  test('rejects unsafe href protocols and control characters', () => {
    for (const href of [
      undefined,
      '',
      '   ',
      'javascript:alert(1)',
      'JaVaScRiPt:alert(1)',
      'data:text/html,<script>alert(1)</script>',
      'vbscript:msgbox(1)',
      'file:///etc/passwd',
      'blob:https://example.com/id',
      '//evil.example.com',
      'https://example.com/\u0000path',
    ]) {
      assert.equal(isSafeHref(href), false, String(href))
    }
  })

  test('uses the same allowlist for markdown URL transform', () => {
    assert.equal(safeMarkdownUrlTransform(' tel:+123456789 '), 'tel:+123456789')
    assert.equal(safeMarkdownUrlTransform('javascript:alert(1)'), '')
  })

  test('does not enable raw HTML rendering', () => {
    const source = readFileSync('src/components/ui/safe-markdown.tsx', 'utf8')

    assert.doesNotMatch(source, /rehypeRaw/)
    assert.doesNotMatch(source, /dangerouslySetInnerHTML/)
    assert.match(source, /react-markdown/)
    assert.match(source, /remark-gfm/)
  })
})
