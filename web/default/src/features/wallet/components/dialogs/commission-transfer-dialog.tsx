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
import {
  accountBalanceCentsToCnyAmount,
  accountBalanceCnyToCents,
  formatAccountBalanceForPlanPurchase,
} from '@/features/subscriptions/lib'

interface CommissionTransferDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  availableCents: number
  minimumTransferCents: number
  transferring: boolean
  onConfirm: (amountCents: number) => Promise<boolean>
}

export function CommissionTransferDialog(props: CommissionTransferDialogProps) {
  const { t } = useTranslation()
  const [amountCny, setAmountCny] = useState(0)

  useEffect(() => {
    if (props.open) {
      const minimumCny = accountBalanceCentsToCnyAmount(
        props.minimumTransferCents
      )
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setAmountCny(minimumCny)
    }
  }, [props.open, props.minimumTransferCents])

  const availableCny = accountBalanceCentsToCnyAmount(props.availableCents)
  const minimumTransferDisplay = formatAccountBalanceForPlanPurchase(
    props.minimumTransferCents
  )

  const handleConfirm = async () => {
    const success = await props.onConfirm(accountBalanceCnyToCents(amountCny))
    if (success) props.onOpenChange(false)
  }

  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent className='max-sm:w-[calc(100vw-1.5rem)] sm:max-w-md'>
        <DialogHeader>
          <DialogTitle>{t('Transfer to balance')}</DialogTitle>
          <DialogDescription>
            {t(
              'Move available commission to your account balance immediately.'
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
              htmlFor='commission-transfer-amount'
              className='text-muted-foreground text-xs font-medium tracking-wider uppercase'
            >
              {t('Transfer amount')}
            </Label>
            <Input
              id='commission-transfer-amount'
              type='number'
              value={amountCny}
              onChange={(event) => setAmountCny(Number(event.target.value))}
              min={accountBalanceCentsToCnyAmount(props.minimumTransferCents)}
              max={availableCny}
              step={0.01}
              className='font-mono text-lg'
            />
            <p className='text-muted-foreground text-xs'>
              {t('Minimum:')} {minimumTransferDisplay}
            </p>
          </div>
        </div>

        <DialogFooter className='grid grid-cols-2 gap-2 sm:flex'>
          <Button
            variant='outline'
            onClick={() => props.onOpenChange(false)}
            disabled={props.transferring}
          >
            {t('Cancel')}
          </Button>
          <Button onClick={handleConfirm} disabled={props.transferring}>
            {props.transferring && (
              <Loader2 className='mr-2 h-4 w-4 animate-spin' />
            )}
            {t('Transfer to balance')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
