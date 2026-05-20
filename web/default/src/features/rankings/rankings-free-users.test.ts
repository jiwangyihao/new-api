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
})

test('profile settings expose editable rankings display name field', () => {
  const rankingsDisplayTab = readRankingsDisplayTabSource()
  assert.match(rankingsDisplayTab, /rankings_display_name/)
  assert.match(rankingsDisplayTab, /Ranking display name/)

  const profileTypes = readProfileTypesSource()
  assert.match(profileTypes, /rankings_display_name\?: string/)
})
