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
