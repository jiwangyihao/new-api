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
import { useEffect, useState } from 'react'
import { Loader2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Textarea } from '@/components/ui/textarea'
import {
  accountBalanceCentsToCnyAmount,
  accountBalanceCnyToCents,
  formatAccountBalanceForPlanPurchase,
} from '@/features/subscriptions/lib'
import type {
  InvitationCommissionContact,
  InvitationCommissionWithdrawalPayload,
} from '../../types'

interface CommissionWithdrawalDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  availableCents: number
  minimumWithdrawalCents: number
  submitting: boolean
  onConfirm: (
    payload: InvitationCommissionWithdrawalPayload
  ) => Promise<boolean>
}

export function CommissionWithdrawalDialog(
  props: CommissionWithdrawalDialogProps
) {
  const { t } = useTranslation()
  const [amountCny, setAmountCny] = useState(0)
  const [contactType, setContactType] =
    useState<InvitationCommissionContact['type']>('wechat')
  const [contactValue, setContactValue] = useState('')
  const [remark, setRemark] = useState('')

  useEffect(() => {
    if (props.open) {
      const minimumCny = accountBalanceCentsToCnyAmount(
        props.minimumWithdrawalCents
      )
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setAmountCny(minimumCny)
      setContactType('wechat')
      setContactValue('')
      setRemark('')
    }
  }, [props.open, props.minimumWithdrawalCents])

  const availableCny = accountBalanceCentsToCnyAmount(props.availableCents)
  const minimumWithdrawalDisplay = formatAccountBalanceForPlanPurchase(
    props.minimumWithdrawalCents
  )

  const handleConfirm = async () => {
    const payload: InvitationCommissionWithdrawalPayload = {
      amount_cents: accountBalanceCnyToCents(amountCny),
      contact: {
        type: contactType,
        value: contactValue.trim(),
      },
    }
    const trimmedRemark = remark.trim()
    if (trimmedRemark) payload.remark = trimmedRemark

    const success = await props.onConfirm(payload)
    if (success) props.onOpenChange(false)
  }

  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent className='max-sm:w-[calc(100vw-1.5rem)] sm:max-w-lg'>
        <DialogHeader>
          <DialogTitle>{t('Request manual cashback')}</DialogTitle>
          <DialogDescription>
            {t(
              'Submit your contact details for a manual cashback request. This is not an automatic payout.'
            )}
          </DialogDescription>
        </DialogHeader>

        <div className='space-y-4 py-3'>
          <div className='space-y-2'>
            <Label className='text-muted-foreground text-xs font-medium tracking-wider uppercase'>
              {t('Available commission balance')}
            </Label>
            <div className='text-2xl font-semibold'>
              {formatAccountBalanceForPlanPurchase(props.availableCents)}
            </div>
          </div>

          <div className='space-y-3'>
            <Label
              htmlFor='commission-withdrawal-amount'
              className='text-muted-foreground text-xs font-medium tracking-wider uppercase'
            >
              {t('Manual cashback amount')}
            </Label>
            <Input
              id='commission-withdrawal-amount'
              type='number'
              value={amountCny}
              onChange={(event) => setAmountCny(Number(event.target.value))}
              min={accountBalanceCentsToCnyAmount(props.minimumWithdrawalCents)}
              max={availableCny}
              step={0.01}
              className='font-mono text-lg'
            />
            <p className='text-muted-foreground text-xs'>
              {t('Minimum:')} {minimumWithdrawalDisplay}
            </p>
          </div>

          <div className='grid gap-3 sm:grid-cols-[150px_1fr]'>
            <div className='space-y-2'>
              <Label htmlFor='commission-contact-type'>
                {t('Contact type')}
              </Label>
              <NativeSelect
                id='commission-contact-type'
                value={contactType}
                onChange={(event) =>
                  setContactType(
                    event.target.value as InvitationCommissionContact['type']
                  )
                }
                className='w-full'
              >
                <NativeSelectOption value='wechat'>
                  {t('WeChat')}
                </NativeSelectOption>
                <NativeSelectOption value='telegram'>
                  {t('Telegram')}
                </NativeSelectOption>
                <NativeSelectOption value='email'>
                  {t('Email')}
                </NativeSelectOption>
                <NativeSelectOption value='other'>
                  {t('Other')}
                </NativeSelectOption>
              </NativeSelect>
            </div>

            <div className='space-y-2'>
              <Label htmlFor='commission-contact-value'>{t('Contact')}</Label>
              <Input
                id='commission-contact-value'
                value={contactValue}
                onChange={(event) => setContactValue(event.target.value)}
                placeholder={t('Enter manual cashback contact')}
              />
            </div>
          </div>

          <div className='space-y-2'>
            <Label htmlFor='commission-withdrawal-remark'>{t('Remark')}</Label>
            <Textarea
              id='commission-withdrawal-remark'
              value={remark}
              onChange={(event) => setRemark(event.target.value)}
              placeholder={t('Optional manual cashback note')}
              rows={3}
            />
          </div>
        </div>

        <DialogFooter className='grid grid-cols-2 gap-2 sm:flex'>
          <Button
            variant='outline'
            onClick={() => props.onOpenChange(false)}
            disabled={props.submitting}
          >
            {t('Cancel')}
          </Button>
          <Button onClick={handleConfirm} disabled={props.submitting}>
            {props.submitting && (
              <Loader2 className='mr-2 h-4 w-4 animate-spin' />
            )}
            {t('Request manual cashback')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
