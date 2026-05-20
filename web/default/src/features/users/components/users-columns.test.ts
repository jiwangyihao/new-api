import assert from 'node:assert/strict'
import { test } from 'node:test'
import {
  getInvitationDisplayState,
  getUserQuotaDisplayState,
} from './users-columns'

test('user quota display treats used quota without remaining quota as no balance', () => {
  const state = getUserQuotaDisplayState({ quota: 0, used_quota: 3659607 })

  assert.equal(state.hasQuota, false)
  assert.equal(state.remaining, 0)
  assert.equal(state.total, 0)
  assert.equal(state.percentage, 0)
})

test('user quota display keeps remaining quota as available balance', () => {
  const state = getUserQuotaDisplayState({ quota: 500000, used_quota: 250000 })

  assert.equal(state.hasQuota, true)
  assert.equal(state.remaining, 500000)
  assert.equal(state.total, 750000)
  assert.equal(state.percentage, (500000 / 750000) * 100)
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
