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
import { toast } from 'sonner'
import {
  accountBalanceCnyToCents,
  formatAccountBalanceForPlanPurchase,
} from '@/features/subscriptions/lib'
import { cn } from '@/lib/utils'
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
import { adjustUserQuota } from '../api'
import type { QuotaAdjustMode } from '../types'

interface UserQuotaDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  userId: number
  currentBalanceCents?: number
  legacyQuota?: number
  onSuccess: () => void
}

export function accountBalanceAdjustmentInputToCents(
  input: string | number
): number {
  const amount = typeof input === 'number' ? input : Number.parseFloat(input)
  return accountBalanceCnyToCents(amount)
}

export function formatUserAccountBalanceForDialog(cents: number): string {
  return formatAccountBalanceForPlanPurchase(cents)
}

export function getUserQuotaAdjustmentPreview(input: {
  currentBalanceCents: number
  mode: QuotaAdjustMode
  input: string | number
}): { valueCents: number; nextBalanceCents: number; text: string } {
  const valueCents = accountBalanceAdjustmentInputToCents(input.input)
  const currentText = formatUserAccountBalanceForDialog(
    input.currentBalanceCents
  )

  if (input.mode === 'add') {
    const nextBalanceCents = input.currentBalanceCents + valueCents
    const valueText = formatUserAccountBalanceForDialog(valueCents)
    return {
      valueCents,
      nextBalanceCents,
      text: `${currentText}  +${valueText} = ${formatUserAccountBalanceForDialog(nextBalanceCents)}`,
    }
  }

  if (input.mode === 'subtract') {
    const nextBalanceCents = input.currentBalanceCents - valueCents
    const valueText = formatUserAccountBalanceForDialog(valueCents)
    return {
      valueCents,
      nextBalanceCents,
      text: `${currentText}  -${valueText} = ${formatUserAccountBalanceForDialog(nextBalanceCents)}`,
    }
  }

  return {
    valueCents,
    nextBalanceCents: valueCents,
    text: `${currentText} → ${formatUserAccountBalanceForDialog(valueCents)}`,
  }
}

export function UserQuotaDialog(props: UserQuotaDialogProps) {
  const { t } = useTranslation()
  const [mode, setMode] = useState<QuotaAdjustMode>('add')
  const [amount, setAmount] = useState('')
  const [loading, setLoading] = useState(false)

  const currentBalanceCents = props.currentBalanceCents ?? props.legacyQuota ?? 0
  const preview = getUserQuotaAdjustmentPreview({
    currentBalanceCents,
    mode,
    input: amount,
  })
  const getPreviewText = () => {
    return `${t('Current balance')}: ${preview.text}`
  }

  const handleConfirm = async () => {
    if (!amount && mode !== 'override') return
    if (preview.valueCents <= 0 && mode !== 'override') return

    setLoading(true)
    try {
      const result = await adjustUserQuota({
        id: props.userId,
        action: 'add_quota',
        mode,
        value: preview.valueCents,
      })
      if (result.success) {
        toast.success(t('Quota adjusted successfully'))
        setAmount('')
        setMode('add')
        props.onOpenChange(false)
        props.onSuccess()
      } else {
        toast.error(result.message || t('Failed to adjust quota'))
      }
    } catch (e: unknown) {
      toast.error(e instanceof Error ? e.message : t('Failed to adjust quota'))
    } finally {
      setLoading(false)
    }
  }

  const handleCancel = () => {
    setAmount('')
    setMode('add')
    props.onOpenChange(false)
  }

  const placeholder = t('Enter amount in CNY')

  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('Adjust Account Balance')}</DialogTitle>
          <DialogDescription>
            {t('Select an operation mode and enter the CNY amount')}
          </DialogDescription>
        </DialogHeader>
        <div className='space-y-4'>
          <div className='text-muted-foreground text-sm'>
            {getPreviewText()}
          </div>

          <div className='space-y-2'>
            <Label>{t('Mode')}</Label>
            <div className='flex gap-1'>
              {(['add', 'subtract', 'override'] as const).map((m) => (
                <Button
                  key={m}
                  type='button'
                  variant='outline'
                  size='sm'
                  className={cn(
                    mode === m &&
                      'bg-primary text-primary-foreground hover:bg-primary/90 hover:text-primary-foreground'
                  )}
                  onClick={() => {
                    setMode(m)
                    setAmount('')
                  }}
                >
                  {m === 'add'
                    ? t('Add')
                    : m === 'subtract'
                      ? t('Subtract')
                      : t('Override')}
                </Button>
              ))}
            </div>
          </div>

          <div className='space-y-2'>
            <Label>{t('Amount (CNY)')}</Label>
            <Input
              type='number'
              step={0.01}
              min={0}
              placeholder={placeholder}
              value={amount}
              onChange={(e) => setAmount(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter') handleConfirm()
              }}
            />
          </div>
        </div>
        <DialogFooter>
          <Button variant='outline' onClick={handleCancel}>
            {t('Cancel')}
          </Button>
          <Button onClick={handleConfirm} disabled={loading}>
            {loading ? t('Processing...') : t('Confirm')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
