import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'

import { processChartData, processUserChartData } from './charts'
import { calculateDashboardStats } from './stats'

function readDashboardIndexSource(): string {
  return readFileSync(new URL('../index.tsx', import.meta.url), 'utf8')
}

test('processChartData token totals and token trend use token_used instead of quota', () => {
  const data = [
    {
      created_at: 1_700_000_000,
      model_name: 'expensive-low-token',
      quota: 100_000,
      token_used: 10,
      count: 100,
    },
    {
      created_at: 1_700_000_000,
      model_name: 'cheap-high-token',
      quota: 1,
      token_used: 1000,
      count: 1,
    },
  ]

  const result = processChartData(data, 'day', (x) => x)

  assert.equal(result.totalTokensDisplay, '1,010')
  const trendValues = result.spec_line.data[0].values as Array<{ Tokens: number }>
  assert.equal(
    trendValues.reduce((sum, item) => sum + item.Tokens, 0),
    1010
  )
  const rankValues = result.spec_rank_bar.data[0].values as Array<{
    Model: string
    Count: number
  }>
  assert.equal(rankValues[0].Model, 'expensive-low-token')
})

test('calculateDashboardStats totals by token_used instead of quota', () => {
  const data = [
    {
      created_at: 1_700_000_000,
      model_name: 'expensive-low-token',
      quota: 100_000,
      token_used: 10,
      count: 1,
    },
    {
      created_at: 1_700_000_000,
      model_name: 'cheap-high-token',
      quota: 1,
      token_used: 1000,
      count: 1,
    },
  ]

  const stats = calculateDashboardStats(data)

  assert.equal(stats.totalTokens, 1010)
  assert.ok(!('totalQuota' in stats))
})

test('processUserChartData ranks users by token_used instead of quota', () => {
  const data = [
    {
      created_at: 1_700_000_000,
      username: 'alice',
      quota: 100_000,
      token_used: 10,
      count: 1,
    },
    {
      created_at: 1_700_000_000,
      username: 'bob',
      quota: 1,
      token_used: 1000,
      count: 1,
    },
  ]

  const result = processUserChartData(data, 'day', (x) => x)
  const rankValues = result.spec_user_rank.data[0].values as Array<{
    User: string
  }>

  assert.equal(rankValues[0].User, 'bob')
})

test('dashboard users section is admin-only at mount point', () => {
  const source = readDashboardIndexSource()

  assert.match(source, /activeSection === 'users' && isAdmin/)
  assert.doesNotMatch(
    source,
    /activeSection === 'users' &&\s*\([\s\S]*?<LazyUserCharts[\s\S]*?\/\>/
  )
})
