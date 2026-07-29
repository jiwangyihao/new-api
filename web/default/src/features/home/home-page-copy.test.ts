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

function readOptionalSource(relativePath: string): string {
  try {
    return readFileSync(new URL(relativePath, import.meta.url), 'utf8')
  } catch {
    return ''
  }
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

describe('Issue #6 home page copy source contract', () => {
  test('preserves custom home page content before default landing sections', () => {
    const homeSource = readSource('./index.tsx')

    assertIncludes(homeSource, 'useHomePageContent')
    assertIncludes(homeSource, 'if (content)')
    assertIncludes(homeSource, 'isUrl ? (')
    assertIncludes(homeSource, '<iframe')
    assertIncludes(homeSource, 'Markdown')

    const customContentIndex = homeSource.indexOf('if (content)')
    const plansPreviewIndex = homeSource.indexOf('<PlansPreview />')
    assert.ok(
      customContentIndex >= 0 && customContentIndex < plansPreviewIndex,
      'custom home content branch must run before the default PlansPreview section'
    )
  })

  test('uses plans preview instead of default stats and features sections', () => {
    const homeSource = readSource('./index.tsx')
    const componentsSource = readSource('./components/index.ts')

    assertIncludes(homeSource, '<PlansPreview />')
    assertExcludes(homeSource, '<Stats />')
    assertExcludes(homeSource, '<Features />')

    assert.match(
      componentsSource,
      /export\s+\{\s*PlansPreview\s*\}\s+from\s+['"]\.\/sections\/plans-preview['"]/
    )
    assert.doesNotMatch(componentsSource, /from\s+['"]\.\/sections\/stats['"]/)
    assert.doesNotMatch(
      componentsSource,
      /from\s+['"]\.\/sections\/features['"]/
    )
  })

  test('updates hero title while preserving dashboard and model directory entries', () => {
    const heroSource = readSource('./components/sections/hero.tsx')

    assertIncludes(heroSource, 'Affordable, low-cost, high-speed GPT')
    assertIncludes(heroSource, '/dashboard')
    assertIncludes(heroSource, 'Go to Dashboard')
    assertIncludes(heroSource, 'Browse Models')
    assertExcludes(heroSource, 'View Pricing')
    assertExcludes(heroSource, 'Review model rates')
  })

  test('keeps terminal API demo focused on chat and responses', () => {
    const terminalDemoSource = readSource('./components/hero-terminal-demo.tsx')

    assertIncludes(terminalDemoSource, "id: 'gpt-chat'")
    assertIncludes(terminalDemoSource, "id: 'responses'")
    assertExcludes(terminalDemoSource, "id: 'claude'")
    assertExcludes(terminalDemoSource, 'Claude message routed')
    assertExcludes(terminalDemoSource, '<in>')
    assertExcludes(terminalDemoSource, '<out>')
    assertExcludes(terminalDemoSource, "id: 'gemini'")
    assertExcludes(terminalDemoSource, 'Gemini request served')
  })

  test('wires plans preview to quiet public plan data and wallet CTA copy', () => {
    const plansPreviewSource = readOptionalSource(
      './components/sections/plans-preview.tsx'
    )

    assert.match(plansPreviewSource, /getHomePublicPlansQuiet/)
    assert.doesNotMatch(plansPreviewSource, /getPublicPlans\(/)
    assert.match(plansPreviewSource, /formatPlanPrice/)
    assert.match(plansPreviewSource, /formatDuration/)
    assert.match(plansPreviewSource, /formatCreditLimit/)
    assert.match(plansPreviewSource, /formatConcurrencyLimit/)
    assert.match(plansPreviewSource, /t\('Choose a plan'\)/)
    assert.match(plansPreviewSource, /t\('View all plans'\)/)
    assert.match(plansPreviewSource, /hasMoreHomePlans/)
    assertIncludes(plansPreviewSource, '/wallet')
    assertExcludes(plansPreviewSource, 'View Pricing')
    assertExcludes(plansPreviewSource, 'Choose Plan')
    assertExcludes(plansPreviewSource, 'Review model rates')
  })

  test('defines required home page plan copy in every locale', () => {
    const localePaths = ['en', 'zh', 'fr', 'ja', 'ru', 'vi'] as const
    const requiredKeys = [
      'Affordable, low-cost, high-speed GPT',
      'Pick a plan that fits your GPT usage.',
      'View all plans',
    ]

    for (const locale of localePaths) {
      const localeSource = readSource(`../../i18n/locales/${locale}.json`)
      const localeJson = JSON.parse(localeSource) as {
        translation?: Record<string, unknown>
      }

      for (const key of requiredKeys) {
        const value = localeJson.translation?.[key]
        assert.ok(
          typeof value === 'string' && value.trim().length > 0,
          `${locale} translation for ${key} must be a non-empty string`
        )
      }
    }

    const zhSource = readSource('../../i18n/locales/zh.json')
    const zhJson = JSON.parse(zhSource) as {
      translation?: Record<string, unknown>
    }
    assert.equal(
      zhJson.translation?.['Affordable, low-cost, high-speed GPT'],
      '超便宜低价高速的GPT'
    )
  })
})
