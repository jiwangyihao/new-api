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

import type { JSX } from 'react'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import {
  GPT_ABUSE_EMPTY_DISPLAY,
  formatGPTAbuseChannel,
  formatGPTAbuseNumber,
  formatGPTAbuseTimestamp,
} from '../lib/format'
import {
  gptAbuseKindLabelKey,
  gptAbuseSeverityLabelKey,
  gptAbuseSuspensionLabelKey,
} from '../lib/filters'
import type { GPTAbuseUserListItem } from '../types'

type GPTAbuseUserTableProps = {
  users: GPTAbuseUserListItem[]
  onViewDetails: (user: GPTAbuseUserListItem) => void
  onClearSuspension: (user: GPTAbuseUserListItem) => void
  onResetWarnings: (user: GPTAbuseUserListItem) => void
}

export function GPTAbuseUserTable(props: GPTAbuseUserTableProps): JSX.Element {
  const { t } = useTranslation()

  return (
    <div className='overflow-x-auto'>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>{t('gptAbuse.table.user')}</TableHead>
            <TableHead>{t('gptAbuse.table.counts')}</TableHead>
            <TableHead>{t('gptAbuse.table.limit')}</TableHead>
            <TableHead>{t('gptAbuse.table.severity')}</TableHead>
            <TableHead>{t('gptAbuse.table.latestWarning')}</TableHead>
            <TableHead>{t('gptAbuse.table.suspension')}</TableHead>
            <TableHead>{t('gptAbuse.table.repeatBlocks')}</TableHead>
            <TableHead className='text-right'>{t('gptAbuse.table.actions')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {props.users.map((user) => (
            <TableRow key={user.user_id}>
              <TableCell className='min-w-48 align-top'>
                <div className='font-medium'>{user.username || `#${user.user_id}`}</div>
                <div className='text-muted-foreground text-xs'>#{user.user_id}</div>
                <div className='text-muted-foreground text-xs'>{user.user_email || GPT_ABUSE_EMPTY_DISPLAY}</div>
              </TableCell>
              <TableCell className='align-top'>
                <div className='text-sm'>
                  {t('gptAbuse.table.rawEffective', {
                    raw: formatGPTAbuseNumber(user.warning_count),
                    effective: formatGPTAbuseNumber(user.effective_warning_count),
                  })}
                </div>
                <div className='text-muted-foreground text-xs'>
                  {t('gptAbuse.table.highMedium', {
                    high: formatGPTAbuseNumber(user.high_count),
                    medium: formatGPTAbuseNumber(user.medium_count),
                  })}
                </div>
              </TableCell>
              <TableCell className='align-top'>
                <div>{formatGPTAbuseNumber(user.daily_limit)}</div>
                <div className='text-muted-foreground text-xs'>
                  {t('gptAbuse.table.remaining', {
                    count: formatGPTAbuseNumber(user.remaining_warning_count),
                  })}
                </div>
              </TableCell>
              <TableCell className='align-top'>
                <SeverityBadge severity={user.max_severity} />
                <div className='text-muted-foreground mt-1 text-xs'>
                  {user.latest_kind ? t(gptAbuseKindLabelKey(user.latest_kind)) : GPT_ABUSE_EMPTY_DISPLAY}
                </div>
              </TableCell>
              <TableCell className='min-w-52 align-top'>
                <div>{formatGPTAbuseTimestamp(user.latest_warning_at)}</div>
                <div className='text-muted-foreground text-xs'>{user.latest_requested_model || GPT_ABUSE_EMPTY_DISPLAY}</div>
                <div className='text-muted-foreground text-xs'>
                  {formatGPTAbuseChannel(user.latest_channel_id, user.latest_channel_name)}
                </div>
              </TableCell>
              <TableCell className='align-top'>
                <SuspensionBadge status={user.suspension_status} />
                {user.active_suspension ? (
                  <div className='text-muted-foreground mt-1 text-xs'>
                    {formatGPTAbuseTimestamp(user.active_suspension.suspended_until)}
                  </div>
                ) : null}
              </TableCell>
              <TableCell className='align-top'>
                <div>{formatGPTAbuseNumber(user.repeat_block_count)}</div>
                <div className='text-muted-foreground text-xs'>
                  {formatGPTAbuseTimestamp(user.latest_repeat_block_at)}
                </div>
              </TableCell>
              <TableCell className='align-top'>
                <div className='flex justify-end gap-2'>
                  <Button
                    type='button'
                    variant='outline'
                    size='sm'
                    onClick={() => props.onViewDetails(user)}
                  >
                    {t('gptAbuse.actions.viewDetails')}
                  </Button>
                  <Button
                    type='button'
                    variant='outline'
                    size='sm'
                    disabled={user.suspension_status !== 'active'}
                    onClick={() => props.onClearSuspension(user)}
                  >
                    {t('gptAbuse.actions.clearSuspension')}
                  </Button>
                  <Button
                    type='button'
                    variant='destructive'
                    size='sm'
                    onClick={() => props.onResetWarnings(user)}
                  >
                    {t('gptAbuse.actions.resetWarnings')}
                  </Button>
                </div>
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  )
}

function SeverityBadge(props: { severity: string }): JSX.Element {
  const { t } = useTranslation()
  const severity = props.severity || 'none'
  const variant = severity === 'high' ? 'destructive' : severity === 'medium' ? 'secondary' : 'outline'
  return <Badge variant={variant}>{t(gptAbuseSeverityLabelKey(severity))}</Badge>
}

function SuspensionBadge(props: { status: string }): JSX.Element {
  const { t } = useTranslation()
  const active = props.status === 'active'
  return (
    <Badge variant={active ? 'destructive' : 'outline'}>
      {t(gptAbuseSuspensionLabelKey(props.status))}
    </Badge>
  )
}
