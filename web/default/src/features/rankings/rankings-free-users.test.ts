import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'

function readRankingsIndexSource(): string {
  return readFileSync('src/features/rankings/index.tsx', 'utf8')
}

function readRankingsTypesSource(): string {
  return readFileSync('src/features/rankings/types.ts', 'utf8')
}

function readRankingsComponentsIndexSource(): string {
  return readFileSync('src/features/rankings/components/index.ts', 'utf8')
}

function readFreeUsersSectionSource(): string {
  return readFileSync('src/features/rankings/components/free-users-section.tsx', 'utf8')
}

function readFreeUsersBarChartSource(): string {
  return readFileSync('src/features/rankings/components/free-users-bar-chart.tsx', 'utf8')
}

function readFreeUsersLineChartSource(): string {
  return readFileSync('src/features/rankings/components/free-users-line-chart.tsx', 'utf8')
}

const chartI18nKeys = [
  'Bar chart',
  '24-hour trend',
  'Hourly usage',
  'Cumulative usage',
  'Usage after free-plan activation',
  'Compare each ranked user within their first 24 hours of free-plan access',
  'No free-plan trend data available',
  'Rank #{{rank}}',
] as const

function readLocale(locale: string): Record<string, string> {
  const content = readFileSync(`src/i18n/locales/${locale}.json`, 'utf8')
  return JSON.parse(content).translation as Record<string, string>
}

function readRankingsDisplayTabSource(): string {
  return readFileSync(
    'src/features/profile/components/tabs/rankings-display-tab.tsx',
    'utf8'
  )
}

function readProfileTypesSource(): string {
  return readFileSync('src/features/profile/types.ts', 'utf8')
}

test('rankings page shows free-plan token leaderboard and removes market share plus trend cards', () => {
  const source = readRankingsIndexSource()
  assert.match(source, /FreeUsersSection/)
  assert.doesNotMatch(source, /MarketShareSection/)
  assert.doesNotMatch(source, /PulseSection/)

  const componentExports = readRankingsComponentsIndexSource()
  assert.match(componentExports, /free-users-section/)
  assert.doesNotMatch(componentExports, /market-share-section/)
  assert.doesNotMatch(componentExports, /pulse-section/)

  const types = readRankingsTypesSource()
  assert.match(types, /FreeUserRanking/)
  assert.match(types, /free_users: FreeUserRanking\[\]/)
  assert.match(types, /free_user_total_tokens: number/)
  assert.doesNotMatch(types, /VendorRanking/)
  assert.doesNotMatch(types, /RankingMover/)
  assert.doesNotMatch(types, /vendor_share_history/)
  assert.doesNotMatch(types, /top_movers/)
  assert.doesNotMatch(types, /top_droppers/)
})

test('rankings snapshot exposes free-user history for chart views', () => {
  const types = readRankingsTypesSource()
  assert.match(types, /FreeUserHistoryPoint/)
  assert.match(types, /FreeUserHistorySeries/)
  assert.match(types, /free_user_history: FreeUserHistorySeries/)
  assert.match(types, /series_label: string/)
  assert.match(types, /hour_label: string/)
  assert.match(types, /cumulative_tokens: number/)

  const page = readRankingsIndexSource()
  assert.match(page, /history=\{snapshot\.free_user_history\}/)
})

test('free-user section wires bar and line chart views', () => {
  const section = readFreeUsersSectionSource()
  assert.match(section, /history: FreeUserHistorySeries/)
  assert.match(section, /FreeUsersBarChart/)
  assert.match(section, /FreeUsersLineChart/)
  assert.match(section, /Bar chart/)
  assert.match(section, /24-hour trend/)
  assert.match(section, /Hourly usage/)
  assert.match(section, /Cumulative usage/)
})

test('free-user bar chart is horizontal with user labels on the y axis', () => {
  const source = readFreeUsersBarChartSource()
  assert.match(source, /direction:\s*'horizontal'/)
  assert.match(source, /series_label/)
  assert.match(source, /rank/)
  assert.match(source, /display_name/)
  assert.match(source, /xField:\s*'total_tokens'/)
  assert.match(source, /yField:\s*'series_label'/)
  assert.match(source, /#\$\{row\.rank\} · \$\{row\.display_name\}/)
})

test('free-user line chart supports hourly and cumulative modes', () => {
  const source = readFreeUsersLineChartSource()
  assert.match(source, /FreeUserTrendMode/)
  assert.match(source, /mode === 'hourly' \? 'tokens' : 'cumulative_tokens'/)
  assert.match(source, /hour_label/)
  assert.match(source, /series_label/)
  assert.match(source, /No free-plan trend data available/)
})

test('free-user chart i18n keys exist in all supported locales', () => {
  for (const locale of ['en', 'zh', 'fr', 'ja', 'ru', 'vi']) {
    const translation = readLocale(locale)
    for (const key of chartI18nKeys) {
      assert.equal(typeof translation[key], 'string', `${locale} missing ${key}`)
      assert.notEqual(translation[key].trim(), '', `${locale} empty ${key}`)
      if (locale !== 'en') {
        assert.ok(
          translation[key].includes('{{rank}}') === key.includes('{{rank}}'),
          `${locale} placeholder mismatch ${key}`
        )
        assert.notEqual(translation[key], key, `${locale} untranslated ${key}`)
      }
    }
  }
})

test('profile settings expose editable rankings display name field', () => {
  const rankingsDisplayTab = readRankingsDisplayTabSource()
  assert.match(rankingsDisplayTab, /rankings_display_name/)
  assert.match(rankingsDisplayTab, /Ranking display name/)

  const profileTypes = readProfileTypesSource()
  assert.match(profileTypes, /rankings_display_name\?: string/)
})
