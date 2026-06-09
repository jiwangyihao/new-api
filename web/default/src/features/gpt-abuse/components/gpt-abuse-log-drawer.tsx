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

import { useState, type JSX } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyTitle,
} from '@/components/ui/empty'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { getGPTAbuseRepeatBlocks, getGPTAbuseUserLogs } from '../api'
import { GPTAbuseRepeatBlockTable } from './gpt-abuse-repeat-block-table'
import {
  GPT_ABUSE_EMPTY_DISPLAY,
  extractRawWarning,
  formatBooleanBadge,
  formatGPTAbuseChannel,
  formatGPTAbuseTimestamp,
  formatRawWarning,
  formatRawWarningSummary,
} from '../lib/format'
import {
  gptAbuseKindLabelKey,
  gptAbuseSeverityLabelKey,
  gptAbuseSourceLabelKey,
} from '../lib/filters'
import type {
  GPTAbuseLogSearch,
  GPTAbuseRepeatBlockSearch,
  GPTAbuseSignalLogItem,
  GPTAbuseUserListItem,
} from '../types'

type GPTAbuseLogDrawerProps = {
  open: boolean
  user: GPTAbuseUserListItem | null
  logsSearch: GPTAbuseLogSearch
  repeatBlockSearch: GPTAbuseRepeatBlockSearch
  onOpenChange: (open: boolean) => void
}

export function GPTAbuseLogDrawer(props: GPTAbuseLogDrawerProps): JSX.Element {
  const { t } = useTranslation()
  const userId = props.user?.user_id ?? 0
  const logsQuery = useQuery({
    queryKey: ['gpt-abuse', 'logs', userId, props.logsSearch],
    queryFn: () => getGPTAbuseUserLogs(userId, props.logsSearch),
    enabled: props.open && userId > 0,
  })
  const repeatBlocksQuery = useQuery({
    queryKey: ['gpt-abuse', 'repeat-blocks', userId, props.repeatBlockSearch],
    queryFn: () => getGPTAbuseRepeatBlocks(userId, props.repeatBlockSearch),
    enabled: props.open && userId > 0,
  })
  const logs = logsQuery.data?.success === true ? logsQuery.data.data?.items ?? [] : []
  const repeatBlocks =
    repeatBlocksQuery.data?.success === true
      ? repeatBlocksQuery.data.data?.items ?? []
      : []

  return (
    <Sheet open={props.open} onOpenChange={props.onOpenChange}>
      <SheetContent className='flex h-dvh w-full flex-col overflow-hidden sm:max-w-6xl'>
        <SheetHeader>
          <SheetTitle>{t('gptAbuse.details.title')}</SheetTitle>
          <SheetDescription>
            {props.user
              ? t('gptAbuse.details.description', {
                  user: props.user.username || `#${props.user.user_id}`,
                })
              : t('gptAbuse.details.noUser')}
          </SheetDescription>
        </SheetHeader>
        <Tabs defaultValue='logs' className='min-h-0 flex-1'>
          <TabsList>
            <TabsTrigger value='logs'>{t('gptAbuse.details.warningLogs')}</TabsTrigger>
            <TabsTrigger value='repeatBlocks'>
              {t('gptAbuse.details.repeatBlocks')}
            </TabsTrigger>
          </TabsList>
          <TabsContent value='logs' className='min-h-0 overflow-auto'>
            {logsQuery.isLoading ? (
              <DetailState title={t('gptAbuse.details.loadingLogs')} />
            ) : logs.length === 0 ? (
              <DetailState title={t('gptAbuse.details.emptyLogs')} />
            ) : (
              <WarningLogsTable logs={logs} />
            )}
          </TabsContent>
          <TabsContent value='repeatBlocks' className='min-h-0 overflow-auto'>
            <p className='text-muted-foreground mb-3 text-sm'>
              {t('gptAbuse.repeatBlock.notCountedNotice')}
            </p>
            {repeatBlocksQuery.isLoading ? (
              <DetailState title={t('gptAbuse.details.loadingRepeatBlocks')} />
            ) : repeatBlocks.length === 0 ? (
              <DetailState title={t('gptAbuse.details.emptyRepeatBlocks')} />
            ) : (
              <GPTAbuseRepeatBlockTable repeatBlocks={repeatBlocks} />
            )}
          </TabsContent>
        </Tabs>
      </SheetContent>
    </Sheet>
  )
}

function WarningLogsTable(props: { logs: GPTAbuseSignalLogItem[] }): JSX.Element {
  const { t } = useTranslation()

  return (
    <div className='overflow-x-auto'>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>{t('gptAbuse.details.time')}</TableHead>
            <TableHead>{t('gptAbuse.details.request')}</TableHead>
            <TableHead>{t('gptAbuse.details.endpoint')}</TableHead>
            <TableHead>{t('gptAbuse.details.model')}</TableHead>
            <TableHead>{t('gptAbuse.details.channel')}</TableHead>
            <TableHead>{t('gptAbuse.details.classification')}</TableHead>
            <TableHead>{t('gptAbuse.details.countEligible')}</TableHead>
            <TableHead>{t('gptAbuse.details.extra')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {props.logs.map((log) => (
            <WarningLogRow key={log.id} log={log} />
          ))}
        </TableBody>
      </Table>
    </div>
  )
}

function WarningLogRow(props: { log: GPTAbuseSignalLogItem }): JSX.Element {
  const { t } = useTranslation()
  const [expanded, setExpanded] = useState(false)
  const rawWarning = extractRawWarning(props.log.extra)

  return (
    <TableRow>
      <TableCell className='min-w-36 align-top'>
        {formatGPTAbuseTimestamp(props.log.created_at)}
      </TableCell>
      <TableCell className='min-w-56 align-top'>
        <div>{props.log.request_id || GPT_ABUSE_EMPTY_DISPLAY}</div>
        <div className='text-muted-foreground text-xs'>
          {props.log.upstream_request_id || GPT_ABUSE_EMPTY_DISPLAY}
        </div>
      </TableCell>
      <TableCell className='min-w-40 align-top'>
        <div>{props.log.endpoint || GPT_ABUSE_EMPTY_DISPLAY}</div>
        <div className='text-muted-foreground text-xs'>{props.log.status_code || GPT_ABUSE_EMPTY_DISPLAY}</div>
        <div className='text-muted-foreground text-xs'>{props.log.error_code || GPT_ABUSE_EMPTY_DISPLAY}</div>
      </TableCell>
      <TableCell className='min-w-44 align-top'>
        <div>{props.log.requested_model || GPT_ABUSE_EMPTY_DISPLAY}</div>
        <div className='text-muted-foreground text-xs'>{props.log.upstream_model || GPT_ABUSE_EMPTY_DISPLAY}</div>
      </TableCell>
      <TableCell className='align-top'>
        {formatGPTAbuseChannel(props.log.channel_id, props.log.channel_name)}
      </TableCell>
      <TableCell className='min-w-44 align-top'>
        <div>{props.log.source ? t(gptAbuseSourceLabelKey(props.log.source)) : GPT_ABUSE_EMPTY_DISPLAY}</div>
        <div className='text-muted-foreground text-xs'>
          {props.log.kind ? t(gptAbuseKindLabelKey(props.log.kind)) : GPT_ABUSE_EMPTY_DISPLAY}
        </div>
        <Badge variant={props.log.severity === 'high' ? 'destructive' : 'secondary'}>
          {props.log.severity ? t(gptAbuseSeverityLabelKey(props.log.severity)) : GPT_ABUSE_EMPTY_DISPLAY}
        </Badge>
      </TableCell>
      <TableCell className='align-top'>
        {t(formatBooleanBadge(props.log.count_eligible))}
      </TableCell>
      <TableCell className='min-w-80 max-w-96 align-top'>
        <div className='text-muted-foreground text-xs whitespace-pre-wrap'>
          {formatRawWarningSummary(rawWarning)}
        </div>
        {expanded ? (
          <pre className='bg-muted mt-2 max-h-64 overflow-auto rounded-md p-2 text-xs whitespace-pre-wrap'>
            {formatRawWarning(rawWarning)}
          </pre>
        ) : null}
        <Button
          type='button'
          variant='ghost'
          size='sm'
          className='mt-1'
          onClick={() => setExpanded((value) => !value)}
        >
          {expanded ? t('gptAbuse.details.collapseExtra') : t('gptAbuse.details.expandExtra')}
        </Button>
      </TableCell>
    </TableRow>
  )
}

function DetailState(props: { title: string }): JSX.Element {
  return (
    <Empty>
      <EmptyHeader>
        <EmptyTitle>{props.title}</EmptyTitle>
        <EmptyDescription>—</EmptyDescription>
      </EmptyHeader>
    </Empty>
  )
}
