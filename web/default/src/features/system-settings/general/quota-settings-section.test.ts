import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { describe, test } from 'node:test'

describe('billing quota settings', () => {
  test('does not expose manual monthly invitation reward plan configuration', () => {
    const billingDefaults = readFileSync(
      'src/features/system-settings/billing/index.tsx',
      'utf8'
    )
    const billingRegistry = readFileSync(
      'src/features/system-settings/billing/section-registry.tsx',
      'utf8'
    )
    const quotaSection = readFileSync(
      'src/features/system-settings/general/quota-settings-section.tsx',
      'utf8'
    )

    assert.doesNotMatch(billingDefaults, /MonthlyInvitationRewardPlanCode/)
    assert.doesNotMatch(billingRegistry, /MonthlyInvitationRewardPlanCode/)
    assert.doesNotMatch(quotaSection, /MonthlyInvitationRewardPlanCode/)
    assert.doesNotMatch(quotaSection, /Monthly Invitation Reward Plan Code/)
  })
})
