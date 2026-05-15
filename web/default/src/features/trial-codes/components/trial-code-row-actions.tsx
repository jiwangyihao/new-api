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
import { Edit, MoreHorizontal, Power, PowerOff, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuShortcut,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { updateTrialCodeStatus } from '../api'
import { trialCodeSchema, type TrialCode } from '../types'
import { useTrialCodes } from './trial-codes-provider'

type TrialCodeRowActionsProps<TData> = {
  row: Row<TData>
}

export function TrialCodeRowActions<TData>(
  props: TrialCodeRowActionsProps<TData>
) {
  const { t } = useTranslation()
  const trialCode = trialCodeSchema.parse(props.row.original)
  const { setOpen, setCurrentRow, triggerRefresh } = useTrialCodes()

  const handleToggleStatus = async () => {
    const res = await updateTrialCodeStatus(trialCode.id, !trialCode.enabled)
    if (res.success) {
      toast.success(
        trialCode.enabled
          ? t('Trial code disabled successfully')
          : t('Trial code enabled successfully')
      )
      triggerRefresh()
    }
  }

  const openDialog = (dialog: 'update' | 'delete', row: TrialCode) => {
    setCurrentRow(row)
    setOpen(dialog)
  }

  return (
    <DropdownMenu modal={false}>
      <DropdownMenuTrigger
        render={<Button variant='ghost' className='h-8 w-8 p-0' />}
      >
        <MoreHorizontal className='h-4 w-4' />
        <span className='sr-only'>{t('Open menu')}</span>
      </DropdownMenuTrigger>
      <DropdownMenuContent align='end' className='w-[160px]'>
        <DropdownMenuItem onClick={() => openDialog('update', trialCode)}>
          {t('Edit')}
          <DropdownMenuShortcut>
            <Edit size={16} />
          </DropdownMenuShortcut>
        </DropdownMenuItem>
        <DropdownMenuItem onClick={handleToggleStatus}>
          {trialCode.enabled ? (
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
        <DropdownMenuSeparator />
        <DropdownMenuItem
          onClick={() => openDialog('delete', trialCode)}
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
