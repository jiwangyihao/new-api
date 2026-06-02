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
import { describe, test } from 'node:test'
import {
  TRIAL_ABUSE_DEFAULT_GROUP_LIMIT,
  TRIAL_ABUSE_DEFAULT_MIN_CLUSTER_SIZE,
  TRIAL_ABUSE_DEFAULT_MIN_CONSUME_COUNT,
  TRIAL_ABUSE_DEFAULT_RISK_LIMIT,
  TRIAL_ABUSE_DEFAULT_WINDOW_SECONDS,
  TRIAL_ABUSE_MAX_WINDOW_SECONDS,
  buildTrialAbuseSummaryParams,
  dateTimeInputToUnixSeconds,
  createDefaultTrialAbuseDraftFilters,
  trialAbuseRiskReasonI18nKey,
  unixSecondsToDateTimeInput,
  updateTrialAbuseDraftFilter,
  validateTrialAbuseDraftFilters,
} from './filters'

const fixedNow = 1_800_000_000

describe('trial abuse filter helpers', () => {
  test('builds default draft without submitted params', () => {
    const draft = createDefaultTrialAbuseDraftFilters(fixedNow)

    assert.equal(draft.trialEndStart, fixedNow - TRIAL_ABUSE_DEFAULT_WINDOW_SECONDS)
    assert.equal(draft.trialEndEnd, fixedNow)
    assert.equal(draft.registeredStart, 0)
    assert.equal(draft.registeredEnd, 0)
    assert.equal(draft.minConsumeCount, TRIAL_ABUSE_DEFAULT_MIN_CONSUME_COUNT)
    assert.equal(draft.minClusterSize, TRIAL_ABUSE_DEFAULT_MIN_CLUSTER_SIZE)
    assert.equal(draft.riskLimit, TRIAL_ABUSE_DEFAULT_RISK_LIMIT)
    assert.equal(draft.groupLimit, TRIAL_ABUSE_DEFAULT_GROUP_LIMIT)
  })

  test('rejects trial end ranges wider than ninety days', () => {
    const draft = createDefaultTrialAbuseDraftFilters(fixedNow)
    const validation = validateTrialAbuseDraftFilters({
      ...draft,
      trialEndStart: fixedNow - TRIAL_ABUSE_MAX_WINDOW_SECONDS - 1,
    })

    assert.equal(validation.valid, false)
    assert.ok(
      validation.errors.includes('trialAbuse.validation.trialEndRangeTooLarge')
    )
  })

  test('clamps limit params through the API contract helper', () => {
    const draft = createDefaultTrialAbuseDraftFilters(fixedNow)
    const params = buildTrialAbuseSummaryParams({
      ...draft,
      minConsumeCount: 200_000,
      minClusterSize: 500,
      riskLimit: 999,
      groupLimit: 999,
    })

    assert.equal(params.min_consume_count, 100000)
    assert.equal(params.min_cluster_size, 100)
    assert.equal(params.risk_limit, 200)
    assert.equal(params.group_limit, 100)
  })

  test('maps risk reason ids to trial abuse i18n keys', () => {
    assert.equal(
      trialAbuseRiskReasonI18nKey('inviterLowPaidConversion'),
      'trialAbuse.riskReason.inviterLowPaidConversion'
    )
  })


  test('round trips local datetime input without timezone drift', () => {
    const unixSeconds = 1_700_000_000
    const input = unixSecondsToDateTimeInput(unixSeconds)

    assert.equal(dateTimeInputToUnixSeconds(input), Math.floor(unixSeconds / 60) * 60)
  })
  test('keeps submitted filters separate from draft filters', () => {
    const draft = createDefaultTrialAbuseDraftFilters(fixedNow)
    const submitted = buildTrialAbuseSummaryParams(draft)
    const nextDraft = updateTrialAbuseDraftFilter(draft, 'minConsumeCount', 900)

    assert.equal(nextDraft.minConsumeCount, 900)
    assert.equal(submitted.min_consume_count, TRIAL_ABUSE_DEFAULT_MIN_CONSUME_COUNT)
  })
})
