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
import { type Row } from '@tanstack/react-table'
import {
  Trash2,
  Copy,
  Edit,
  Power,
  PowerOff,
  MoreHorizontal as DotsHorizontalIcon,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { copyToClipboard } from '@/lib/copy-to-clipboard'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuShortcut,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { getRedemptionsByBatch, updateRedemptionStatus } from '../api'
import { REDEMPTION_STATUS, SUCCESS_MESSAGES } from '../constants'
import {
  createFullRedemptionBatchRow,
  getRedemptionRowCopyCount,
  getRedemptionRowCopyText,
  isRedemptionBatchRow,
  isRedemptionExpired,
  type RedemptionBatchRow,
} from '../lib'
import { useRedemptions } from './redemptions-provider'

interface DataTableRowActionsProps<TData> {
  row: Row<TData>
}

export function DataTableRowActions<TData>({
  row,
}: DataTableRowActionsProps<TData>) {
  const { t } = useTranslation()
  const redemption = row.original as RedemptionBatchRow
  const { setOpen, setCurrentRow, triggerRefresh } = useRedemptions()
  const isEnabled = redemption.status === REDEMPTION_STATUS.ENABLED
  const isUsed = redemption.status === REDEMPTION_STATUS.USED
  const isExpired = isRedemptionExpired(
    redemption.expired_time,
    redemption.status
  )

  const handleToggleStatus = async () => {
    const newStatus = isEnabled
      ? REDEMPTION_STATUS.DISABLED
      : REDEMPTION_STATUS.ENABLED

    const result = await updateRedemptionStatus(redemption.id, newStatus)
    if (result.success) {
      const message = isEnabled
        ? t(SUCCESS_MESSAGES.REDEMPTION_DISABLED)
        : t(SUCCESS_MESSAGES.REDEMPTION_ENABLED)
      toast.success(message)
      triggerRefresh()
    }
  }

  const isBatchRow = isRedemptionBatchRow(redemption)

  const getActionRedemption = async (): Promise<RedemptionBatchRow> => {
    if (!isBatchRow || !redemption.batch_id) {
      return redemption
    }
    const result = await getRedemptionsByBatch(redemption.batch_id)
    if (!result.success || !result.data) {
      throw new Error(result.message || t('Failed to load redemption codes'))
    }
    return createFullRedemptionBatchRow(redemption, result.data)
  }

  const handleCopyCodes = async () => {
    let actionRedemption: RedemptionBatchRow
    try {
      actionRedemption = await getActionRedemption()
    } catch (_error) {
      toast.error(t('Failed to load redemption codes'))
      return
    }
    const ok = await copyToClipboard(getRedemptionRowCopyText(actionRedemption))
    if (ok) {
      toast.success(
        t('Copied {{count}} redemption codes', {
          count: getRedemptionRowCopyCount(actionRedemption),
        })
      )
    } else {
      toast.error(t('Copy failed'))
    }
  }
  const canEdit = !isBatchRow && isEnabled && !isExpired
  const canToggle = !isBatchRow && !isUsed && !isExpired

  return (
    <DropdownMenu modal={false}>
      <DropdownMenuTrigger
        render={
          <Button
            variant='ghost'
            className='data-popup-open:bg-muted flex h-8 w-8 p-0'
          />
        }
      >
        <DotsHorizontalIcon className='h-4 w-4' />
        <span className='sr-only'>{t('Open menu')}</span>
      </DropdownMenuTrigger>
      <DropdownMenuContent align='end' className='w-[180px]'>
        <DropdownMenuItem
          onClick={() => {
            setCurrentRow(redemption)
            setOpen('update')
          }}
          disabled={!canEdit}
        >
          {t('Edit')}
          <DropdownMenuShortcut>
            <Edit size={16} />
          </DropdownMenuShortcut>
        </DropdownMenuItem>
        <DropdownMenuItem onClick={handleCopyCodes}>
          {isBatchRow ? t('Copy batch codes') : t('Copy code')}
          <DropdownMenuShortcut>
            <Copy size={16} />
          </DropdownMenuShortcut>
        </DropdownMenuItem>
        {canToggle && (
          <DropdownMenuItem onClick={handleToggleStatus}>
            {isEnabled ? (
              <>
                {t('Disable')}
                <DropdownMenuShortcut>
                  <PowerOff size={16} />
                </DropdownMenuShortcut>
              </>
            ) : (
              <>
                {t('Enable')}
                <DropdownMenuShortcut>
                  <Power size={16} />
                </DropdownMenuShortcut>
              </>
            )}
          </DropdownMenuItem>
        )}
        <DropdownMenuSeparator />
        <DropdownMenuItem
          onClick={async () => {
            try {
              const actionRedemption = await getActionRedemption()
              setCurrentRow(actionRedemption)
              setOpen('delete')
            } catch (_error) {
              toast.error(t('Failed to load redemption codes'))
            }
          }}
          className='text-destructive focus:text-destructive'
        >
          {t('Delete')}
          <DropdownMenuShortcut>
            <Trash2 size={16} />
          </DropdownMenuShortcut>
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
