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
  formatFingerprintPrefix,
  formatGPTAbuseChannel,
  formatGPTAbuseTimestamp,
} from '../lib/format'
import { gptAbuseKindLabelKey, gptAbuseSeverityLabelKey } from '../lib/filters'
import type { GPTAbuseRepeatBlockItem } from '../types'

type GPTAbuseRepeatBlockTableProps = {
  repeatBlocks: GPTAbuseRepeatBlockItem[]
}

export function GPTAbuseRepeatBlockTable(
  props: GPTAbuseRepeatBlockTableProps
): JSX.Element {
  const { t } = useTranslation()

  return (
    <div className='overflow-x-auto'>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>{t('gptAbuse.repeatBlock.time')}</TableHead>
            <TableHead>{t('gptAbuse.repeatBlock.request')}</TableHead>
            <TableHead>{t('gptAbuse.repeatBlock.fingerprint')}</TableHead>
            <TableHead>{t('gptAbuse.repeatBlock.firstWarning')}</TableHead>
            <TableHead>{t('gptAbuse.repeatBlock.channel')}</TableHead>
            <TableHead>{t('gptAbuse.repeatBlock.token')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {props.repeatBlocks.map((item) => (
            <TableRow key={item.id}>
              <TableCell className='min-w-36 align-top'>
                {formatGPTAbuseTimestamp(item.created_at)}
              </TableCell>
              <TableCell className='min-w-56 align-top'>
                <div>{item.request_id || GPT_ABUSE_EMPTY_DISPLAY}</div>
                <div className='text-muted-foreground text-xs'>{item.endpoint || GPT_ABUSE_EMPTY_DISPLAY}</div>
                <div className='text-muted-foreground text-xs'>{item.requested_model || GPT_ABUSE_EMPTY_DISPLAY}</div>
              </TableCell>
              <TableCell className='font-mono text-xs align-top'>
                {formatFingerprintPrefix(item.body_fingerprint_prefix)}
              </TableCell>
              <TableCell className='min-w-56 align-top'>
                <div>#{item.first_warning_log_id}</div>
                <div className='text-muted-foreground text-xs'>
                  {formatGPTAbuseTimestamp(item.first_warning_at)}
                </div>
                <div className='text-muted-foreground text-xs'>
                  {item.first_warning_kind ? t(gptAbuseKindLabelKey(item.first_warning_kind)) : GPT_ABUSE_EMPTY_DISPLAY} / {item.first_warning_severity ? t(gptAbuseSeverityLabelKey(item.first_warning_severity)) : GPT_ABUSE_EMPTY_DISPLAY}
                </div>
              </TableCell>
              <TableCell className='align-top'>
                {formatGPTAbuseChannel(item.channel_id, item.channel_name)}
              </TableCell>
              <TableCell className='align-top'>
                <div>{item.token_name || GPT_ABUSE_EMPTY_DISPLAY}</div>
                <div className='text-muted-foreground text-xs'>#{item.token_id}</div>
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  )
}
