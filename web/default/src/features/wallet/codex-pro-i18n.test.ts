import en from '@/i18n/locales/en.json'
import fr from '@/i18n/locales/fr.json'
import ja from '@/i18n/locales/ja.json'
import ru from '@/i18n/locales/ru.json'
import vi from '@/i18n/locales/vi.json'
import zh from '@/i18n/locales/zh.json'
import { STATIC_I18N_KEYS } from '@/i18n/static-keys'
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import {
  CODEX_PRO_MODE_OPTIONS,
  CODEX_PRO_MODE_TITLE_KEY,
} from '@/features/subscriptions/components/codex-pro-mode-control'

type Expect<T extends true> = T

type StaticI18nKey = (typeof STATIC_I18N_KEYS)[number]

const requiredCodexProKeys = [
  CODEX_PRO_MODE_TITLE_KEY,
  ...CODEX_PRO_MODE_OPTIONS.map((option) => option.labelKey),
  ...CODEX_PRO_MODE_OPTIONS.map((option) => option.descriptionKey),
  'Only eligible GPT-family requests can try Codex Pro.',
  'Codex Pro markers are logging signals only; credits use a numeric upstream multiplier only when the channel enables dynamic billing.',
  'Without a valid dynamic multiplier, requests are billed at the normal credit rate.',
  'Please purchase an eligible paid subscription first.',
  'Trial subscriptions do not support Codex Pro.',
  'Invitation reward subscriptions do not support Codex Pro.',
  'Your current billing preference will not create a subscription billing session.',
  'Codex CLI',
  'Claude Code',
  'Hermes Agent',
  'OpenClaw',
  'Use Codex Pro in flexible mode by adding the intent header where the client supports custom headers.',
  'The Codex Pro intent header only enables flexible-mode routing; it does not guarantee Pro service and is not a billing credential.',
  'This client has no verified custom header field yet. Flexible mode cannot trigger Codex Pro from this harness; use All mode in the console if needed.',
] as const

type CodexProI18nKey = (typeof requiredCodexProKeys)[number]
type MissingStaticCodexProKeys = Exclude<CodexProI18nKey, StaticI18nKey>

const staticCodexProKeyCoverage: Expect<
  MissingStaticCodexProKeys extends never ? true : false
> = true

function requireCodexProTranslations<T extends Record<CodexProI18nKey, string>>(
  translation: T
): T {
  return translation
}

const localeTranslations = {
  en: requireCodexProTranslations(en.translation),
  zh: requireCodexProTranslations(zh.translation),
  fr: requireCodexProTranslations(fr.translation),
  ja: requireCodexProTranslations(ja.translation),
  ru: requireCodexProTranslations(ru.translation),
  vi: requireCodexProTranslations(vi.translation),
} as const

void staticCodexProKeyCoverage

describe('Codex Pro i18n contract', () => {
  for (const [localeName, translation] of Object.entries(localeTranslations)) {
    test(`${localeName} includes Codex Pro user-facing copy`, () => {
      for (const key of requiredCodexProKeys) {
        assert.equal(typeof translation[key], 'string', `${localeName}: ${key}`)
        assert.notEqual(translation[key].trim(), '', `${localeName}: ${key}`)
      }
    })
  }
})
