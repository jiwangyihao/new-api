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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  Field,
  FieldContent,
  FieldDescription,
  FieldLabel,
  FieldLegend,
  FieldSet,
  FieldTitle,
} from '@/components/ui/field'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
import { Spinner } from '@/components/ui/spinner'
import type { RedemptionMode, RedemptionResult } from '@/features/wallet/types'

interface RedemptionDialogProps {
  open: boolean
  code: string
  redeeming: boolean
  onOpenChange: (open: boolean) => void
  onSubmit: (request: {
    key: string
    redemption_mode: RedemptionMode
  }) => Promise<RedemptionResult | null>
}

export function RedemptionDialog({
  open,
  code,
  redeeming,
  onOpenChange,
  onSubmit,
}: RedemptionDialogProps) {
  const { t } = useTranslation()
  const [mode, setMode] = useState<RedemptionMode | null>(null)
  const [error, setError] = useState('')
  const [receipt, setReceipt] = useState<RedemptionResult | null>(null)

  const submit = async () => {
    if (!mode || redeeming) return
    setError('')
    try {
      const result = await onSubmit({ key: code, redemption_mode: mode })
      if (result) setReceipt(result)
    } catch (submitError) {
      setError(
        submitError instanceof Error
          ? submitError.message
          : t('Redemption failed')
      )
    }
  }

  const handleOpenChange = (nextOpen: boolean) => {
    if (!nextOpen) {
      setMode(null)
      setError('')
      setReceipt(null)
    }
    onOpenChange(nextOpen)
  }

  const credit = receipt?.credit_balance

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className='sm:max-w-lg'>
        <DialogHeader>
          <DialogTitle>{t('Choose redemption mode')}</DialogTitle>
          <DialogDescription>
            {t(
              'Subscription codes require an explicit benefit mode. Account balance codes keep their existing value.'
            )}
          </DialogDescription>
        </DialogHeader>

        {receipt ? (
          <Alert>
            <AlertTitle>{t('Redemption receipt')}</AlertTitle>
            <AlertDescription className='flex flex-col gap-1'>
              {receipt.redemption_mode === 'credit_balance' && credit ? (
                <>
                  <span>
                    {t('Gross Credit')}: {credit.gross_credit}
                  </span>
                  <span>
                    {t('Debt offset')}: {credit.debt_offset}
                  </span>
                  <span>
                    {t('Available Credit balance')}: {credit.available_credit}
                  </span>
                </>
              ) : receipt.type === 'subscription' ? (
                <span>
                  {t('Timed subscription')}:{' '}
                  {receipt.plan?.title || t('Subscription plan')}
                </span>
              ) : (
                <span>{t('Account balance redemption completed')}</span>
              )}
            </AlertDescription>
          </Alert>
        ) : (
          <FieldSet>
            <FieldLegend variant='label'>{t('Redemption mode')}</FieldLegend>
            <FieldDescription>
              {t('Choose the benefit you want before redeeming.')}
            </FieldDescription>
            <RadioGroup
              value={mode || ''}
              onValueChange={(value) => {
                setMode(value as RedemptionMode)
                setError('')
              }}
              disabled={redeeming}
            >
              <FieldLabel htmlFor='redemption-mode-timed'>
                <Field orientation='horizontal'>
                  <RadioGroupItem id='redemption-mode-timed' value='timed' />
                  <FieldContent>
                    <FieldTitle>{t('Timed subscription')}</FieldTitle>
                    <FieldDescription>
                      {t(
                        'Keep the plan duration, reset cycle, and service limits.'
                      )}
                    </FieldDescription>
                  </FieldContent>
                </Field>
              </FieldLabel>
              <FieldLabel htmlFor='redemption-mode-credit-balance'>
                <Field orientation='horizontal'>
                  <RadioGroupItem
                    id='redemption-mode-credit-balance'
                    value='credit_balance'
                  />
                  <FieldContent>
                    <FieldTitle>{t('Credit balance')}</FieldTitle>
                    <FieldDescription>
                      {t(
                        'Add the latest monthly Credit to the non-expiring balance; service limits come from the Credit balance plan.'
                      )}
                    </FieldDescription>
                  </FieldContent>
                </Field>
              </FieldLabel>
            </RadioGroup>
          </FieldSet>
        )}

        {error && (
          <Alert variant='destructive'>
            <AlertTitle>{t('Redemption failed')}</AlertTitle>
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        )}

        <DialogFooter>
          {receipt ? (
            <Button type='button' onClick={() => handleOpenChange(false)}>
              {t('Close')}
            </Button>
          ) : (
            <Button
              type='button'
              onClick={submit}
              disabled={!mode || redeeming}
            >
              {redeeming && <Spinner data-icon='inline-start' />}
              {t('Confirm redemption')}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
