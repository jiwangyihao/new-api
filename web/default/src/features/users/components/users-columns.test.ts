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
import { test } from 'node:test'
import {
  getInvitationDisplayState,
  getUserQuotaDisplayState,
} from './users-columns'

function readUsersColumnsSource(): string {
  return readFileSync(new URL('./users-columns.tsx', import.meta.url), 'utf8')
}

test('user quota column formats account balance cents with CNY helper while keeping used quota as usage', () => {
  const source = readUsersColumnsSource()

  assert.match(source, /formatAccountBalanceForPlanPurchase/)
  assert.doesNotMatch(source, /formatQuota\(quotaState\.remaining\)/)
  assert.doesNotMatch(source, /formatQuota\(quotaState\.total\)/)
  assert.match(source, /formatQuota\(quotaState\.used\)/)
})

test('user quota display treats used quota without remaining quota as no balance', () => {
  const state = getUserQuotaDisplayState({ quota: 0, used_quota: 3659607 })

  assert.equal(state.hasQuota, false)
  assert.equal(state.remaining, 0)
  assert.equal(state.total, 0)
  assert.equal(state.percentage, 0)
})

test('user quota display state keeps available account balance cents separate from used quota usage', () => {
  const state = getUserQuotaDisplayState({ quota: 4000, used_quota: 3659607 })

  assert.equal(state.hasQuota, true)
  assert.equal(state.remaining, 4000)
  assert.equal(state.used, 3659607)
  assert.equal(state.total, 4000)
  assert.equal(state.percentage, 100)
})

test('invitation display uses relationship counts instead of legacy quota rewards', () => {
  const state = getInvitationDisplayState({
    aff_count: 0,
    direct_invite_count: 7,
    qualified_paid_invite_count: 2,
    invitation_reward_status: 'qualified',
    invitation_reward_plan_title: '一瓶盖可乐',
    inviter_id: 206,
  })

  assert.equal(state.directInviteCount, 7)
  assert.equal(state.qualifiedPaidInviteCount, 2)
  assert.equal(state.rewardText, '一瓶盖可乐')
  assert.equal(state.inviterId, 206)
})

test('invitation display exposes commission account and switch estimate separately', () => {
  const commissionState = getInvitationDisplayState({
    direct_invite_count: 2,
    qualified_paid_invite_count: 2,
    invitation_reward_mode: 'commission',
    invitation_commission_available_cents: 10800,
    invitation_commission_earned_cents: 12800,
  })

  assert.equal(commissionState.rewardMode, 'commission')
  assert.equal(commissionState.showCommissionSummary, true)
  assert.equal(commissionState.commissionAvailableCents, 10800)
  assert.equal(commissionState.commissionEarnedCents, 12800)
  assert.equal(commissionState.showCommissionEstimate, false)

  const prospectState = getInvitationDisplayState({
    direct_invite_count: 28,
    qualified_paid_invite_count: 28,
    invitation_reward_mode: 'subscription',
    invitation_commission_estimated_cents: 20293,
    invitation_commission_estimated_event_count: 28,
  })

  assert.equal(prospectState.rewardMode, 'subscription')
  assert.equal(prospectState.showCommissionSummary, false)
  assert.equal(prospectState.showCommissionEstimate, true)
  assert.equal(prospectState.commissionEstimatedCents, 20293)
  assert.equal(prospectState.commissionEstimatedEventCount, 28)
})
