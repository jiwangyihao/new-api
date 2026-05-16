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

function readSource(file: string): string {
  return readFileSync(file, 'utf8')
}

describe('welcome popup integration smoke', () => {
  test('authenticated layout mounts the welcome popup gate', () => {
    const source = readSource(
      'src/components/layout/components/authenticated-layout.tsx'
    )

    assert.match(source, /WelcomePopupGate/)
    assert.match(source, /<WelcomePopupGate\s*\/>/)
  })

  test('welcome popup gate renders dialog from hook state', () => {
    const source = readSource('src/features/welcome-popup/index.tsx')

    assert.match(source, /useWelcomePopup/)
    assert.match(source, /WelcomePopupDialog/)
  })

  test('welcome popup hook uses protected query and not public status placeholder', () => {
    const source = readSource(
      'src/features/welcome-popup/hooks/use-welcome-popup.ts'
    )
    const apiSource = readSource('src/features/welcome-popup/api.ts')

    assert.match(source, /queryKey:\s*\['welcome-popup',\s*userId\]/)
    assert.match(source, /enabled:\s*Boolean\(userId\)/)
    assert.match(apiSource, /\/api\/user\/welcome-popup/)
    assert.doesNotMatch(source, /useStatus/)
    assert.doesNotMatch(source, /localStorage\.getItem\('status'\)/)
  })

  test('dialog has mobile-safe height and unified close paths', () => {
    const source = readSource(
      'src/features/welcome-popup/components/welcome-popup-dialog.tsx'
    )

    assert.match(source, /max-h-\[90vh\]/)
    assert.match(source, /ScrollArea/)
    assert.match(source, /max-h-\[60vh\]/)
    assert.match(source, /handleClose/)
    assert.match(source, /onOpenChange=\{handleOpenChange\}/)
    assert.match(source, /onClick=\{handleClose\}/)
  })

  test('safe markdown does not enable raw HTML rendering', () => {
    const source = readSource('src/components/ui/safe-markdown.tsx')

    assert.doesNotMatch(source, /rehypeRaw/)
    assert.doesNotMatch(source, /dangerouslySetInnerHTML/)
    assert.match(source, /react-markdown/)
    assert.match(source, /remark-gfm/)
    assert.match(source, /SAFE_HREF_PROTOCOLS/)
  })

  test('settings preview and dialog both use SafeMarkdown', () => {
    const sectionSource = readSource(
      'src/features/system-settings/content/welcome-popup-section.tsx'
    )
    const dialogSource = readSource(
      'src/features/welcome-popup/components/welcome-popup-dialog.tsx'
    )

    assert.match(sectionSource, /SafeMarkdown/)
    assert.match(dialogSource, /SafeMarkdown/)
    assert.doesNotMatch(sectionSource, /components\/ui\/markdown/)
    assert.doesNotMatch(dialogSource, /components\/ui\/markdown/)
  })

  test('password sign-up still redirects to login instead of authenticating', () => {
    const source = readSource(
      'src/features/auth/sign-up/components/sign-up-form.tsx'
    )

    assert.match(source, /Account created! Please sign in[\s\S]*redirectToLogin\(\)/)
    assert.doesNotMatch(
      source,
      /res\?\.success\)\s*\{[\s\S]{0,120}handleLoginSuccess/
    )
  })
})
