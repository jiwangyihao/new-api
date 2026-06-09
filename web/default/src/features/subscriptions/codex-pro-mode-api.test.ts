import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { describe, test } from 'node:test'

import type {
  ApiResponse,
  CodexProMode,
  CodexProUnavailableReason,
  SelfSubscriptionData,
  UpdateCodexProModeRequest,
  UpdateCodexProModeResponse,
} from './types'
import type { updateCodexProMode } from './api'

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends <T>() => T extends B ? 1 : 2
    ? true
    : false

type Expect<T extends true> = T

const apiSource = readFileSync(new URL('./api.ts', import.meta.url), 'utf8')
const codexProModes = ['all', 'flexible', 'off'] as const satisfies readonly CodexProMode[]
const codexProModeContract: Expect<
  Equal<CodexProMode, (typeof codexProModes)[number]>
> = true
const codexProUnavailableReasons = [
  '',
  'wallet_only',
  'trial_subscription',
  'reward_subscription',
  'no_paid_subscription',
] as const satisfies readonly CodexProUnavailableReason[]
const codexProUnavailableReasonContract: Expect<
  Equal<CodexProUnavailableReason, (typeof codexProUnavailableReasons)[number]>
> = true
const updateRequest: UpdateCodexProModeRequest = { mode: 'flexible' }
const updateResponse: UpdateCodexProModeResponse = {
  codex_pro_mode: 'off',
  codex_pro_eligible: true,
  codex_pro_unavailable_reason: '',
}
const selfSubscriptionData: SelfSubscriptionData = {
  billing_preference: 'subscription_first',
  subscriptions: [],
  all_subscriptions: [],
  summary: {
    active_count: 0,
    token_limit: 0,
    token_used: 0,
    token_remaining: 0,
    token_unlimited: false,
    concurrency_limit: 0,
    gpt_abuse_warning_limit: 0,
    gpt_abuse_warning_count: 0,
    gpt_abuse_warning_remaining: 0,
    gpt_abuse_limit_enabled: false,
  },
  codex_pro_mode: 'flexible',
  codex_pro_eligible: false,
  codex_pro_unavailable_reason: 'no_paid_subscription',
}
const responseContract: ApiResponse<UpdateCodexProModeResponse> = {
  success: true,
  data: updateResponse,
}

type UpdateCodexProModeHelper = typeof updateCodexProMode
const helperContract: Expect<
  Equal<
    UpdateCodexProModeHelper,
    (data: UpdateCodexProModeRequest) => Promise<ApiResponse<UpdateCodexProModeResponse>>
  >
> = true

void codexProModeContract
void codexProUnavailableReasonContract
void updateRequest
void selfSubscriptionData
void responseContract
void helperContract

describe('Codex Pro subscription API contract', () => {
  test('exports the mode update helper on the subscription self route', () => {
    assert.match(apiSource, /export async function updateCodexProMode/)
    assert.match(apiSource, /api\.put\(['"]\/api\/subscription\/self\/codex-pro-mode['"]\s*,\s*data/)
  })
})
