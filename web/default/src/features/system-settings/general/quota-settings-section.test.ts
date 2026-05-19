import { readFileSync } from 'node:fs'
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

describe('billing quota settings', () => {
  test('wires monthly invitation reward plan code into quota settings form', () => {
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

    assert.match(
      billingDefaults,
      /MonthlyInvitationRewardPlanCode:\s*'basic_monthly'/
    )
    assert.match(
      billingRegistry,
      /MonthlyInvitationRewardPlanCode:\s*settings\.MonthlyInvitationRewardPlanCode/
    )
    assert.match(quotaSection, /name='MonthlyInvitationRewardPlanCode'/)
    assert.match(quotaSection, /Monthly Invitation Reward Plan Code/)
    assert.match(
      quotaSection,
      /Business code of the subscription plan granted when an inviter has at least two qualified paid direct invitees\./
    )
  })
})
