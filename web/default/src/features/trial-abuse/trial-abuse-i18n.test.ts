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
import { describe, test } from 'node:test'

type LocaleFile = {
  translation?: Record<string, unknown>
}

const localeNames = ['en', 'zh', 'fr', 'ja', 'ru', 'vi'] as const

const riskReasonIds = [
  'sameRegistrationIpCluster',
  'sameRegistrationIpSelfInviteChain',
  'inviterLowPaidConversion',
  'managedInviterDisplayOnly',
  'registrationIpUnavailable',
  'logUnavailable',
  'candidateLimitExceeded',
  'logLimitExceeded',
] as const

const warningReasonIds = [
  'log_unavailable',
  'registration_ip_unavailable',
  'candidate_limit_exceeded',
  'log_limit_exceeded',
] as const

const sectionIds = [
  'overview',
  'usage_distribution',
  'risk_users',
  'risk_counts',
  'ip_clusters',
  'inviter_clusters',
  'self_invite_chains',
] as const

const criticalPageKeys = [
  'trialAbuse.title',
  'trialAbuse.description',
  'trialAbuse.readOnlyNotice',
  'trialAbuse.query',
  'trialAbuse.reset',
  'trialAbuse.refreshCurrentResult',
  'trialAbuse.empty.title',
  'trialAbuse.empty.description',
  'trialAbuse.partialResult',
  'trialAbuse.riskLevel.high',
  'trialAbuse.riskLevel.medium',
  'trialAbuse.riskLevel.low',
  'trialAbuse.riskParticipation.risk',
  'trialAbuse.riskParticipation.displayOnly',
  'trialAbuse.field.trialEndStart',
  'trialAbuse.field.trialEndEnd',
  'trialAbuse.field.registeredStart',
  'trialAbuse.field.registeredEnd',
  'trialAbuse.field.minConsumeCount',
  'trialAbuse.field.minClusterSize',
  'trialAbuse.field.generatedAt',
  'trialAbuse.field.riskUsers',
  'trialAbuse.field.ipClusters',
  'trialAbuse.field.inviterClusters',
  'trialAbuse.field.selfInviteChains',
  'trialAbuse.field.consumeCount',
  'trialAbuse.field.quota',
  'trialAbuse.field.tokens',
  'trialAbuse.field.observedIp',
  'trialAbuse.field.ipSource',
  'trialAbuse.field.riskScore',
  'trialAbuse.field.riskReasons',
  'trialAbuse.field.userId',
  'trialAbuse.field.username',
  'trialAbuse.field.inviter',
  'trialAbuse.field.trialSource',
  'trialAbuse.field.trialEnd',
  'trialAbuse.field.usedQuota',
  'trialAbuse.field.meteredTokens',
  'trialAbuse.field.riskLevel',
  'trialAbuse.manualQueryNotice',
  'trialAbuse.error.title',
  'trialAbuse.error.description',
  'trialAbuse.ipUnavailableNotice',
  'trialAbuse.validation.trialEndRangeRequired',
  'trialAbuse.validation.trialEndRangeTooLarge',
  'trialAbuse.validation.registeredRangeInvalid',
  'trialAbuse.validation.minConsumeCountRange',
  'trialAbuse.validation.minClusterSizeRange',
] as const

const requiredLocaleKeys = [
  ...criticalPageKeys,
  ...riskReasonIds.map((reason) => `trialAbuse.riskReason.${reason}`),
  ...warningReasonIds.map((reason) => `trialAbuse.warningReason.${reason}`),
  ...sectionIds.map((section) => `trialAbuse.section.${section}`),
] as const

const requiredStaticKeys = [
  'trialAbuse.title',
  'trialAbuse.description',
  ...riskReasonIds.map((reason) => `trialAbuse.riskReason.${reason}`),
  ...warningReasonIds.map((reason) => `trialAbuse.warningReason.${reason}`),
  ...sectionIds.map((section) => `trialAbuse.section.${section}`),
] as const

function readJson<T>(relativePath: string): T {
  return JSON.parse(
    readFileSync(new URL(relativePath, import.meta.url), 'utf8')
  )
}

function readSource(relativePath: string): string {
  return readFileSync(new URL(relativePath, import.meta.url), 'utf8')
}

function placeholders(value: string): string[] {
  return [...value.matchAll(/{{[^{}]+}}/g)].map((match) => match[0]).sort()
}

describe('trial abuse i18n keys', () => {
  test('exist in every supported locale with non-empty translations', () => {
    const english = readJson<LocaleFile>('../../i18n/locales/en.json')
    assert.ok(
      english.translation && typeof english.translation === 'object',
      'en.json must contain a translation object'
    )

    for (const localeName of localeNames) {
      const locale = readJson<LocaleFile>(
        `../../i18n/locales/${localeName}.json`
      )
      assert.ok(
        locale.translation && typeof locale.translation === 'object',
        `${localeName}.json must contain a translation object`
      )

      for (const key of requiredLocaleKeys) {
        assert.equal(
          Object.prototype.hasOwnProperty.call(locale.translation, key),
          true,
          `${localeName}.json is missing ${key}`
        )
        const value: unknown = locale.translation[key]
        assert.equal(
          typeof value,
          'string',
          `${localeName}.${key} must be string`
        )
        assert.notEqual(
          (value as string).trim(),
          '',
          `${localeName}.${key} must not be empty`
        )
        assert.deepEqual(
          placeholders(value as string),
          placeholders(english.translation[key] as string),
          `${localeName}.${key} placeholders must match en`
        )
      }
    }
  })

  test('static keys include dynamic trial abuse labels', () => {
    const source = readSource('../../i18n/static-keys.ts')

    for (const key of requiredStaticKeys) {
      assert.ok(
        source.includes(`'${key}'`) || source.includes(`"${key}"`),
        `static keys missing ${key}`
      )
    }
  })
})
