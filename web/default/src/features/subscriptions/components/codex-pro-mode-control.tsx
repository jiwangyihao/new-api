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
import { HelpCircle } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { StatusBadge } from '@/components/status-badge'
import type {
  CodexProMode,
  CodexProUnavailableReason,
  SelfSubscriptionData,
} from '@/features/subscriptions/types'

export const CODEX_PRO_MODE_TITLE_KEY = 'Codex Pro'

export const CODEX_PRO_MODE_OPTIONS = [
  {
    value: 'all',
    labelKey: 'All',
    descriptionKey:
      'All eligible GPT-family Responses requests try Codex Pro without requiring the intent header.',
  },
  {
    value: 'flexible',
    labelKey: 'Flexible',
    descriptionKey:
      'Only requests with X-NewAPI-Codex-Pro-Intent: codex-pro try Codex Pro in flexible mode.',
  },
  {
    value: 'off',
    labelKey: 'Off',
    descriptionKey:
      'Codex Pro is disabled; eligible requests stay on the normal group.',
  },
] as const satisfies ReadonlyArray<{
  value: CodexProMode
  labelKey: string
  descriptionKey: string
}>

export const CODEX_PRO_HELP_GUIDANCE_ITEMS = [
  'All mode tries Codex Pro for every eligible GPT-family Responses request without requiring a header.',
  'Flexible mode tries Codex Pro only when the downstream harness sends X-NewAPI-Codex-Pro-Intent: codex-pro.',
  'Off mode keeps eligible requests on normal routing and billing.',
  'Downstream relay guide: send X-NewAPI-Codex-Pro-Intent: codex-pro in Flexible mode; do not send internal X-NewAPI-Pro-Request.',
  'Do not rely on X-NewAPI-Pro-Served; it is an internal upstream trailer and is not passed to downstream clients.',
  'Read the response body top-level newapi_billing object for metered_tokens, billable_tokens, billing_multiplier, and Codex Pro served state.',
] as const

function CodexProHelpDialog() {
  const { t } = useTranslation()

  return (
    <Dialog>
      <DialogTrigger
        render={
          <Button
            type='button'
            variant='ghost'
            size='icon-sm'
            aria-label={t('Codex Pro help')}
          />
        }
      >
        <HelpCircle className='size-4' aria-hidden='true' />
      </DialogTrigger>
      <DialogContent className='sm:max-w-xl'>
        <DialogHeader>
          <DialogTitle>{t('Codex Pro help')}</DialogTitle>
          <DialogDescription>
            {t(
              'How Codex Pro mode affects routing, harness headers, and billing visibility.'
            )}
          </DialogDescription>
        </DialogHeader>
        <div className='space-y-4 text-sm'>
          <div className='grid gap-2'>
            {CODEX_PRO_MODE_OPTIONS.map((option) => (
              <div key={option.value} className='rounded-lg border p-3'>
                <div className='font-medium'>{t(option.labelKey)}</div>
                <p className='text-muted-foreground mt-1 text-xs leading-relaxed'>
                  {t(option.descriptionKey)}
                </p>
              </div>
            ))}
          </div>
          <div className='space-y-2 rounded-lg border p-3'>
            <div className='font-medium'>
              {t('Harness and downstream relay guide')}
            </div>
            <ul className='text-muted-foreground list-disc space-y-1 pl-4 text-xs leading-relaxed'>
              {CODEX_PRO_HELP_GUIDANCE_ITEMS.slice(3).map((item) => (
                <li key={item}>{t(item)}</li>
              ))}
            </ul>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  )
}
type CodexProModeControlData = Pick<
  SelfSubscriptionData,
  'codex_pro_mode' | 'codex_pro_eligible' | 'codex_pro_unavailable_reason'
>

export interface CodexProModeControlProps {
  data: CodexProModeControlData
  saving?: boolean
  onModeChange: (mode: CodexProMode) => void
}

export function canUseCodexProModeControl<
  T extends {
    codex_pro_eligible?: boolean
  },
>(data: T): boolean {
  return data.codex_pro_eligible === true
}

export function normalizeCodexProUnavailableReason(
  reason: string | undefined
): CodexProUnavailableReason {
  switch (reason) {
    case '':
    case 'wallet_only':
    case 'trial_subscription':
    case 'reward_subscription':
    case 'no_paid_subscription':
      return reason
    default:
      return 'no_paid_subscription'
  }
}

export function getCodexProUnavailableMessageKey(
  reason: CodexProUnavailableReason
): string
export function getCodexProUnavailableMessageKey(reason: string): string
export function getCodexProUnavailableMessageKey(reason: string): string {
  switch (reason) {
    case 'wallet_only':
      return 'Your current billing preference will not create a subscription billing session.'
    case 'trial_subscription':
      return 'Trial subscriptions do not support Codex Pro.'
    case 'reward_subscription':
      return 'Invitation reward subscriptions do not support Codex Pro.'
    case 'no_paid_subscription':
    default:
      return 'Please purchase an eligible paid subscription first.'
  }
}

export function getCodexProModeFailureRollback(input: {
  previousMode: CodexProMode
  requestedMode: CodexProMode
}): { mode: CodexProMode; messageKey: string } {
  return {
    mode:
      input.previousMode === input.requestedMode
        ? input.requestedMode
        : input.previousMode,
    messageKey: 'Request failed',
  }
}

export function normalizeCodexProMode(mode: string | undefined): CodexProMode {
  if (mode === 'all' || mode === 'flexible' || mode === 'off') return mode
  return 'flexible'
}

export function CodexProModeControl(props: CodexProModeControlProps) {
  const { t } = useTranslation()
  const available = canUseCodexProModeControl(props.data)
  const disabled = !available || props.saving === true
  const unavailableMessageKey = getCodexProUnavailableMessageKey(
    props.data.codex_pro_unavailable_reason ?? ''
  )

  return (
    <div className='space-y-3'>
      <div className='flex flex-wrap items-center gap-2'>
        <span className='text-sm font-medium'>
          {t(CODEX_PRO_MODE_TITLE_KEY)}
        </span>
        <CodexProHelpDialog />
        <StatusBadge
          label={available ? t('Available') : t('Not available')}
          variant={available ? 'success' : 'neutral'}
          copyable={false}
        />
      </div>

      <div className='text-muted-foreground space-y-1 text-xs'>
        <p>{t('Only eligible GPT-family requests can try Codex Pro.')}</p>
        <p>
          {t(
            'Codex Pro markers are logging signals only; credits use a numeric upstream multiplier only when the channel enables dynamic billing.'
          )}
        </p>
		<p>{t('Without a valid dynamic multiplier, requests are billed at the normal credit rate.')}</p>
        <p>
          <code className='bg-muted text-foreground rounded px-1 py-0.5 font-mono text-[11px]'>
            X-NewAPI-Codex-Pro-Intent: codex-pro
          </code>
        </p>
        {!available && <p>{t(unavailableMessageKey)}</p>}
      </div>

      <div
        role='group'
        aria-label={t(CODEX_PRO_MODE_TITLE_KEY)}
        className='bg-muted/40 grid grid-cols-3 rounded-lg border p-1'
      >
        {CODEX_PRO_MODE_OPTIONS.map((option) => {
          const selected = props.data.codex_pro_mode === option.value
          return (
            <button
              key={option.value}
              type='button'
              aria-pressed={selected}
              className={cn(
                'min-w-0 rounded-md px-2 py-2 text-left text-xs transition-colors disabled:pointer-events-none disabled:opacity-50',
                selected
                  ? 'bg-background text-foreground shadow-sm'
                  : 'text-muted-foreground hover:text-foreground'
              )}
              onClick={() => {
                if (selected || disabled) return
                props.onModeChange(option.value)
              }}
              disabled={disabled}
            >
              <span className='block font-medium'>{t(option.labelKey)}</span>
              <span className='text-muted-foreground mt-1 block text-[11px] leading-snug whitespace-normal'>
                {t(option.descriptionKey)}
              </span>
            </button>
          )
        })}
      </div>
    </div>
  )
}
