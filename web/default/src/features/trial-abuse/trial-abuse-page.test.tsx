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
  reduceTrialAbusePageState,
  trialAbuseSummaryHasRisk,
  trialAbuseSummaryQueryEnabled,
  trialAbuseSummaryQueryKey,
  type TrialAbusePageState,
} from './index'
import {
  createDefaultTrialAbuseDraftFilters,
  updateTrialAbuseDraftFilter,
  validateTrialAbuseDraftFilters,
} from './lib/filters'
import type { TrialAbuseSummaryResponse } from './types'

const fixedNow = 1_800_000_000

function defaultPageState(): TrialAbusePageState {
  return {
    draft: createDefaultTrialAbuseDraftFilters(fixedNow),
    submittedCriteria: null,
  }
}

function summaryWithOnlyWeakConsumeIPCluster(): TrialAbuseSummaryResponse {
  return {
    generated_at: fixedNow,
    criteria: {
      trial_end_start: fixedNow - 86400,
      trial_end_end: fixedNow,
      snapshot_at: fixedNow,
      min_consume_count: 500,
      min_cluster_size: 2,
      risk_limit: 50,
      group_limit: 20,
    },
    warnings: [],
    partial_sections: {},
    overview: {
      total_trial_users: 2,
      active_trial_users: 0,
      expired_trial_users: 2,
      expired_unpaid_trial_users: 2,
      high_usage_candidate_users: 2,
      risk_user_count: 0,
      high_risk_user_count: 0,
      medium_risk_user_count: 0,
      low_risk_user_count: 0,
      managed_inviter_cluster_count: 0,
      partial: false,
      partial_reasons: [],
    },
    risk_counts: {
      high: 0,
      medium: 0,
      low: 0,
      partial: false,
      partial_reasons: [],
    },
    usage_distribution: {
      sample_size: 2,
      zero_usage_count: 0,
      above_threshold_count: 2,
      p50: 600,
      p75: 600,
      p90: 600,
      p95: 600,
      p99: 600,
      partial: false,
      partial_reasons: [],
    },
    ip_clusters: [
      {
        observed_ip: '203.0.113.9',
        ip_source: 'consume_log',
        registration_ip_available: false,
        candidate_count: 2,
        expired_unpaid_trial_count: 2,
        paid_entitlement_count: 0,
        total_consume_count: 1200,
        sample_user_ids: [1, 2],
        partial: false,
        partial_reasons: [],
      },
    ],
    inviter_clusters: [],
    self_invite_chains: [],
    risk_users: [],
  }
}

describe('trial abuse page query contract', () => {
  test('does not enable summary query before filters are submitted', () => {
    const state = defaultPageState()

    assert.equal(trialAbuseSummaryQueryEnabled(state.submittedCriteria), false)
    assert.deepEqual(trialAbuseSummaryQueryKey(state.submittedCriteria), [
      'trial-abuse',
      'summary',
      null,
    ])
  })

  test('submits current draft filters explicitly', () => {
    const submitted = reduceTrialAbusePageState(defaultPageState(), {
      type: 'submit',
    })

    assert.equal(trialAbuseSummaryQueryEnabled(submitted.submittedCriteria), true)
    assert.equal(
      submitted.submittedCriteria?.min_consume_count,
      submitted.draft.minConsumeCount
    )
    assert.deepEqual(trialAbuseSummaryQueryKey(submitted.submittedCriteria), [
      'trial-abuse',
      'summary',
      submitted.submittedCriteria,
    ])
  })

  test('draft edits do not mutate last submitted params', () => {
    const submitted = reduceTrialAbusePageState(defaultPageState(), {
      type: 'submit',
    })
    const editedDraft = updateTrialAbuseDraftFilter(
      submitted.draft,
      'minConsumeCount',
      900
    )
    const edited = reduceTrialAbusePageState(submitted, {
      type: 'draft',
      draft: editedDraft,
    })

    assert.equal(edited.draft.minConsumeCount, 900)
    assert.equal(submitted.submittedCriteria?.min_consume_count, 500)
    assert.deepEqual(edited.submittedCriteria, submitted.submittedCriteria)
  })

  test('invalid draft keeps the query disabled because submit is skipped', () => {
    const state = defaultPageState()
    const invalidDraft = updateTrialAbuseDraftFilter(
      state.draft,
      'trialEndStart',
      state.draft.trialEndEnd + 60
    )
    const edited = reduceTrialAbusePageState(state, {
      type: 'draft',
      draft: invalidDraft,
    })

    assert.equal(validateTrialAbuseDraftFilters(edited.draft).valid, false)
    assert.equal(trialAbuseSummaryQueryEnabled(edited.submittedCriteria), false)
  })

  test('reset restores draft but keeps the last submitted result refresh target', () => {
    const submitted = reduceTrialAbusePageState(defaultPageState(), {
      type: 'submit',
    })
    const resetDraft = createDefaultTrialAbuseDraftFilters(fixedNow + 60)
    const reset = reduceTrialAbusePageState(submitted, {
      type: 'reset',
      draft: resetDraft,
    })

    assert.equal(reset.draft.trialEndEnd, fixedNow + 60)
    assert.deepEqual(reset.submittedCriteria, submitted.submittedCriteria)
  })

  test('weak consume-ip clusters alone do not hide the no-risk empty state', () => {
    assert.equal(
      trialAbuseSummaryHasRisk(summaryWithOnlyWeakConsumeIPCluster()),
      false
    )
  })
})
