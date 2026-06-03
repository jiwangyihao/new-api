import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { describe, test } from 'node:test'
import { buildQuotaSettingsOptionUpdates } from './quota-settings-section'

function optionValue(
  updates: Array<{ key: string; value: string | number | boolean }>,
  key: string
): string | number | boolean | undefined {
  return updates.find((update) => update.key === key)?.value
}

describe('billing quota settings', () => {
  test('saves account balance rewards as cents without changing pre-consumed quota', () => {
    const updates = buildQuotaSettingsOptionUpdates({
      QuotaForNewUser: 10,
      QuotaForInviter: 5,
      QuotaForInvitee: 3,
      PreConsumedQuota: 1000,
      TopUpLink: 'https://example.com/topup',
      general_setting: {
        docs_link: 'https://docs.example.com',
      },
      quota_setting: {
        enable_free_model_pre_consume: true,
      },
    })

    assert.equal(optionValue(updates, 'QuotaForNewUser'), '1000')
    assert.equal(optionValue(updates, 'QuotaForInviter'), '500')
    assert.equal(optionValue(updates, 'QuotaForInvitee'), '300')
    assert.match(String(optionValue(updates, 'PreConsumedQuota')), /^1000$/)
  })

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
