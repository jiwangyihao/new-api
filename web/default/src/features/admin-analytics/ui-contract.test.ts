import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

function readSource(relativePath: string): string {
  return readFileSync(new URL(relativePath, import.meta.url), 'utf8')
}

const indexSource = readSource('./index.tsx')
const statisticsSource = readSource(
  '../system-settings/billing/statistics-section.tsx'
)

test('paid analytics exposes visible row controls instead of relying on hidden limit params', () => {
  assert.match(indexSource, /PaidAnalyticsRowControls/)
  assert.match(indexSource, /adminAnalytics\.pagination\.allRows/)
  assert.match(indexSource, /enableAdminAnalyticsAllRows/)
  assert.match(indexSource, /enableAdminAnalyticsPagedRows/)
})

test('paid analytics links directly to subscription statistics exclusions', () => {
  assert.match(indexSource, /to='\/system-settings\/billing\/\$section'/)
  assert.match(indexSource, /params=\{\{ section: 'statistics' \}\}/)
  assert.match(indexSource, /adminAnalytics\.filters\.manageExcludedUsers/)
})

test('subscription statistics section has a user search selector for exclusions', () => {
  assert.match(statisticsSource, /searchUsers/)
  assert.match(statisticsSource, /ExcludedUserSearchSelect/)
  assert.match(
    statisticsSource,
    /systemSettings\.billing\.statistics\.searchUsers/
  )
})
