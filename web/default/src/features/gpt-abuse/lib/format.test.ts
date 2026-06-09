import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import {
  extractRawWarning,
  formatFingerprintPrefix,
  formatGPTAbuseTimestamp,
  formatGPTAbuseChannel,
  formatRawWarning,
  formatRawWarningSummary,
} from './format'

describe('gpt abuse format helpers', () => {
  test('formats timestamps with second precision', () => {
    assert.match(formatGPTAbuseTimestamp(1700000000), /^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$/)
    assert.equal(formatGPTAbuseTimestamp(0), '—')
  })

  test('truncates raw warning detail for display', () => {
    const formatted = formatRawWarning('x'.repeat(1200))
    assert.equal(formatted.length, 1000)
    assert.equal(formatted.endsWith('…'), true)
  })

  test('summarizes raw warning detail without rendering full large JSON', () => {
    const summary = formatRawWarningSummary('x'.repeat(600))
    assert.equal(summary.length, 240)
    assert.equal(summary.endsWith('…'), true)
  })

  test('extracts raw_error from direct or upstream warning extra payloads', () => {
    assert.equal(extractRawWarning({ raw_error: 'direct' }), 'direct')
    assert.equal(
      extractRawWarning({ upstream_warning: { raw_error: 'nested' } }),
      'nested'
    )
  })

  test('shows only fingerprint prefixes and channel summaries', () => {
    assert.equal(formatFingerprintPrefix('  a1b2c3d4e5f6  '), 'a1b2c3d4e5f6')
    assert.equal(formatFingerprintPrefix(''), '—')
    assert.equal(formatGPTAbuseChannel(4, 'Sub2API'), 'Sub2API (#4)')
    assert.equal(formatGPTAbuseChannel(4, ''), '#4')
  })
})
