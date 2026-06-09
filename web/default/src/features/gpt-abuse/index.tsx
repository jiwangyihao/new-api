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

import { useMemo, useState, type JSX } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { AlertTriangle, RefreshCw } from 'lucide-react'
import { toast } from 'sonner'
import { useTranslation } from 'react-i18next'
import { ErrorState } from '@/components/error-state'
import { SectionPageLayout } from '@/components/layout'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyTitle,
} from '@/components/ui/empty'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { clearGPTAbuseSuspension, getGPTAbuseUsers, resetGPTAbuseWarnings } from './api'
import { GPTAbuseClearSuspensionDialog } from './components/gpt-abuse-clear-suspension-dialog'
import { GPTAbuseLogDrawer } from './components/gpt-abuse-log-drawer'
import { GPTAbuseResetDialog } from './components/gpt-abuse-reset-dialog'
import { GPTAbuseUserTable } from './components/gpt-abuse-user-table'
import {
  GPT_ABUSE_KIND_OPTIONS,
  GPT_ABUSE_SEVERITY_OPTIONS,
  GPT_ABUSE_SORT_BY_OPTIONS,
  GPT_ABUSE_SORT_ORDER_OPTIONS,
  GPT_ABUSE_SOURCE_OPTIONS,
  GPT_ABUSE_STATUS_OPTIONS,
} from './constants'
import {
  buildGPTAbuseLogSearch,
  buildGPTAbuseRepeatBlockSearch,
  dateTimeInputToUnixSeconds,
  gptAbuseKindLabelKey,
  gptAbuseSeverityLabelKey,
  gptAbuseSortByLabelKey,
  gptAbuseSortOrderLabelKey,
  gptAbuseSourceLabelKey,
  gptAbuseStatusLabelKey,
  unixSecondsToDateTimeInput,
  updateGPTAbuseSearchForFilterChange,
  updateGPTAbuseSearchForPagination,
  updateGPTAbuseSearchForSorting,
} from './lib/filters'
import type {
  GPTAbuseReasonPayload,
  GPTAbuseResetWarningsPayload,
  GPTAbuseUserListItem,
  GPTAbuseUserListSearch,
} from './types'

export type GPTAbusePageProps = {
  search: GPTAbuseUserListSearch
  onSearchChange: (next: GPTAbuseUserListSearch) => void
}

export const gptAbuseQueryKeys = {
  users: (search?: GPTAbuseUserListSearch) =>
    search ? (['gpt-abuse', 'users', search] as const) : (['gpt-abuse', 'users'] as const),
  logs: (userId: number) => ['gpt-abuse', 'logs', userId] as const,
  repeatBlocks: (userId: number) => ['gpt-abuse', 'repeat-blocks', userId] as const,
}

export async function invalidateGPTAbuseUserDetailQueries(
  queryClient: Pick<ReturnType<typeof useQueryClient>, 'invalidateQueries'>,
  userId: number
): Promise<void> {
  await Promise.all([
    queryClient.invalidateQueries({ queryKey: gptAbuseQueryKeys.users() }),
    queryClient.invalidateQueries({ queryKey: gptAbuseQueryKeys.logs(userId) }),
    queryClient.invalidateQueries({ queryKey: gptAbuseQueryKeys.repeatBlocks(userId) }),
  ])
}

export function GPTAbusePage(props: GPTAbusePageProps): JSX.Element {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [detailsUser, setDetailsUser] = useState<GPTAbuseUserListItem | null>(null)
  const [clearUser, setClearUser] = useState<GPTAbuseUserListItem | null>(null)
  const [resetUser, setResetUser] = useState<GPTAbuseUserListItem | null>(null)
  const userQuery = useQuery({
    queryKey: gptAbuseQueryKeys.users(props.search),
    queryFn: () => getGPTAbuseUsers(props.search),
  })
  const users = userQuery.data?.success === true ? userQuery.data.data?.items ?? [] : []
  const total = userQuery.data?.success === true ? userQuery.data.data?.total ?? 0 : 0
  const logsSearch = useMemo(() => buildGPTAbuseLogSearch(props.search), [props.search])
  const repeatBlockSearch = useMemo(
    () => buildGPTAbuseRepeatBlockSearch(props.search),
    [props.search]
  )

  const clearMutation = useMutation({
    mutationFn: (payload: { userId: number; body: GPTAbuseReasonPayload }) =>
      clearGPTAbuseSuspension(payload.userId, payload.body),
    onSuccess: async (response, variables) => {
      if (!response.success) {
        toast.error(response.message || t('gptAbuse.toast.clearFailed'))
        return
      }
      toast.success(t('gptAbuse.toast.clearSuccess'))
      setClearUser(null)
      await invalidateGPTAbuseUserDetailQueries(queryClient, variables.userId)
    },
    onError: () => {
      toast.error(t('gptAbuse.toast.clearFailed'))
    },
  })

  const resetMutation = useMutation({
    mutationFn: (payload: { userId: number; body: GPTAbuseResetWarningsPayload }) =>
      resetGPTAbuseWarnings(payload.userId, payload.body),
    onSuccess: async (response, variables) => {
      if (!response.success) {
        toast.error(response.message || t('gptAbuse.toast.resetFailed'))
        return
      }
      toast.success(t('gptAbuse.toast.resetSuccess'))
      setResetUser(null)
      await invalidateGPTAbuseUserDetailQueries(queryClient, variables.userId)
    },
    onError: () => {
      toast.error(t('gptAbuse.toast.resetFailed'))
    },
  })

  const canGoPrevious = props.search.offset > 0
  const canGoNext = props.search.offset + props.search.limit < total

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('gptAbuse.title')}</SectionPageLayout.Title>
      <SectionPageLayout.Description>
        {t('gptAbuse.description')}
      </SectionPageLayout.Description>
      <SectionPageLayout.Actions>
        <Button
          type='button'
          variant='outline'
          onClick={() => void userQuery.refetch()}
          disabled={userQuery.isFetching}
        >
          <RefreshCw className='size-4' aria-hidden='true' />
          {userQuery.isFetching ? t('gptAbuse.actions.refreshing') : t('gptAbuse.actions.refresh')}
        </Button>
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        <div className='space-y-4'>
          <Alert>
            <AlertTriangle className='size-4' aria-hidden='true' />
            <AlertTitle>{t('gptAbuse.notice.title')}</AlertTitle>
            <AlertDescription>{t('gptAbuse.notice.description')}</AlertDescription>
          </Alert>
          <GPTAbuseFilterCard search={props.search} onSearchChange={props.onSearchChange} />
          <Card>
            <CardHeader>
              <CardTitle>{t('gptAbuse.table.title')}</CardTitle>
            </CardHeader>
            <CardContent className='space-y-4'>
              {userQuery.isError ? (
                <ErrorState
                  title={t('gptAbuse.error.loadUsers')}
                  description={t('gptAbuse.error.loadUsersDescription')}
                  action={
                    <Button type='button' variant='outline' onClick={() => void userQuery.refetch()}>
                      {t('gptAbuse.actions.refresh')}
                    </Button>
                  }
                />
              ) : users.length === 0 ? (
                <Empty>
                  <EmptyHeader>
                    <EmptyTitle>{t('gptAbuse.empty.users')}</EmptyTitle>
                    <EmptyDescription>{t('gptAbuse.empty.usersDescription')}</EmptyDescription>
                  </EmptyHeader>
                </Empty>
              ) : (
                <GPTAbuseUserTable
                  users={users}
                  onViewDetails={setDetailsUser}
                  onClearSuspension={setClearUser}
                  onResetWarnings={setResetUser}
                />
              )}
              <div className='flex flex-col gap-2 border-t pt-4 text-sm sm:flex-row sm:items-center sm:justify-between'>
                <div className='text-muted-foreground'>
                  {t('gptAbuse.pagination.summary', {
                    total,
                    offset: props.search.offset,
                    limit: props.search.limit,
                  })}
                </div>
                <div className='flex items-center gap-2'>
                  <Button
                    type='button'
                    variant='outline'
                    size='sm'
                    disabled={!canGoPrevious}
                    onClick={() =>
                      props.onSearchChange(
                        updateGPTAbuseSearchForPagination(props.search, {
                          offset: Math.max(0, props.search.offset - props.search.limit),
                        })
                      )
                    }
                  >
                    {t('gptAbuse.pagination.previous')}
                  </Button>
                  <Button
                    type='button'
                    variant='outline'
                    size='sm'
                    disabled={!canGoNext}
                    onClick={() =>
                      props.onSearchChange(
                        updateGPTAbuseSearchForPagination(props.search, {
                          offset: props.search.offset + props.search.limit,
                        })
                      )
                    }
                  >
                    {t('gptAbuse.pagination.next')}
                  </Button>
                </div>
              </div>
            </CardContent>
          </Card>
        </div>
        <GPTAbuseLogDrawer
          open={detailsUser !== null}
          user={detailsUser}
          logsSearch={logsSearch}
          repeatBlockSearch={repeatBlockSearch}
          onOpenChange={(open) => {
            if (!open) setDetailsUser(null)
          }}
        />
        <GPTAbuseClearSuspensionDialog
          open={clearUser !== null}
          user={clearUser}
          loading={clearMutation.isPending}
          onOpenChange={(open) => {
            if (!open) setClearUser(null)
          }}
          onConfirm={(payload) => {
            if (clearUser) {
              clearMutation.mutate({ userId: clearUser.user_id, body: payload })
            }
          }}
        />
        <GPTAbuseResetDialog
          open={resetUser !== null}
          user={resetUser}
          loading={resetMutation.isPending}
          onOpenChange={(open) => {
            if (!open) setResetUser(null)
          }}
          onConfirm={(payload) => {
            if (resetUser) {
              resetMutation.mutate({ userId: resetUser.user_id, body: payload })
            }
          }}
        />
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}

function GPTAbuseFilterCard(props: GPTAbusePageProps): JSX.Element {
  const { t } = useTranslation()

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('gptAbuse.filters.title')}</CardTitle>
      </CardHeader>
      <CardContent>
        <div className='grid gap-4 md:grid-cols-2 xl:grid-cols-4'>
          <FilterField label={t('gptAbuse.filters.startTime')}>
            <Input
              type='datetime-local'
              value={unixSecondsToDateTimeInput(props.search.start_timestamp)}
              onChange={(event) =>
                props.onSearchChange(
                  updateGPTAbuseSearchForFilterChange(props.search, {
                    start_timestamp: dateTimeInputToUnixSeconds(event.target.value),
                  })
                )
              }
            />
          </FilterField>
          <FilterField label={t('gptAbuse.filters.endTime')}>
            <Input
              type='datetime-local'
              value={unixSecondsToDateTimeInput(props.search.end_timestamp)}
              onChange={(event) =>
                props.onSearchChange(
                  updateGPTAbuseSearchForFilterChange(props.search, {
                    end_timestamp: dateTimeInputToUnixSeconds(event.target.value),
                  })
                )
              }
            />
          </FilterField>
          <FilterField label={t('gptAbuse.filters.keyword')}>
            <Input
              value={props.search.keyword}
              onChange={(event) =>
                props.onSearchChange(
                  updateGPTAbuseSearchForFilterChange(props.search, {
                    keyword: event.target.value,
                  })
                )
              }
              placeholder={t('gptAbuse.filters.keywordPlaceholder')}
            />
          </FilterField>
          <FilterField label={t('gptAbuse.filters.userId')}>
            <Input
              type='number'
              min={1}
              value={props.search.user_id ?? ''}
              onChange={(event) =>
                props.onSearchChange(
                  updateGPTAbuseSearchForFilterChange(props.search, {
                    user_id: event.target.value === '' ? undefined : Number(event.target.value),
                  })
                )
              }
            />
          </FilterField>
          <FilterField label={t('gptAbuse.filters.status')}>
            <NativeSelect
              value={props.search.status}
              onChange={(event) =>
                props.onSearchChange(
                  updateGPTAbuseSearchForFilterChange(props.search, {
                    status: event.target.value as GPTAbuseUserListSearch['status'],
                  })
                )
              }
              className='w-full'
            >
              {GPT_ABUSE_STATUS_OPTIONS.map((option) => (
                <NativeSelectOption key={option} value={option}>
                  {t(gptAbuseStatusLabelKey(option))}
                </NativeSelectOption>
              ))}
            </NativeSelect>
          </FilterField>
          <FilterField label={t('gptAbuse.filters.kind')}>
            <NativeSelect
              value={props.search.kind}
              onChange={(event) =>
                props.onSearchChange(
                  updateGPTAbuseSearchForFilterChange(props.search, {
                    kind: event.target.value,
                  })
                )
              }
              className='w-full'
            >
              {GPT_ABUSE_KIND_OPTIONS.map((option) => (
                <NativeSelectOption key={option || 'all'} value={option}>
                  {t(gptAbuseKindLabelKey(option))}
                </NativeSelectOption>
              ))}
            </NativeSelect>
          </FilterField>
          <FilterField label={t('gptAbuse.filters.severity')}>
            <NativeSelect
              value={props.search.severity}
              onChange={(event) =>
                props.onSearchChange(
                  updateGPTAbuseSearchForFilterChange(props.search, {
                    severity: event.target.value,
                  })
                )
              }
              className='w-full'
            >
              {GPT_ABUSE_SEVERITY_OPTIONS.map((option) => (
                <NativeSelectOption key={option || 'all'} value={option}>
                  {t(gptAbuseSeverityLabelKey(option))}
                </NativeSelectOption>
              ))}
            </NativeSelect>
          </FilterField>
          <FilterField label={t('gptAbuse.filters.source')}>
            <NativeSelect
              value={props.search.source}
              onChange={(event) =>
                props.onSearchChange(
                  updateGPTAbuseSearchForFilterChange(props.search, {
                    source: event.target.value,
                  })
                )
              }
              className='w-full'
            >
              {GPT_ABUSE_SOURCE_OPTIONS.map((option) => (
                <NativeSelectOption key={option || 'all'} value={option}>
                  {t(gptAbuseSourceLabelKey(option))}
                </NativeSelectOption>
              ))}
            </NativeSelect>
          </FilterField>
          <FilterField label={t('gptAbuse.filters.sortBy')}>
            <NativeSelect
              value={props.search.sort_by}
              onChange={(event) =>
                props.onSearchChange(
                  updateGPTAbuseSearchForSorting(props.search, {
                    sort_by: event.target.value as GPTAbuseUserListSearch['sort_by'],
                  })
                )
              }
              className='w-full'
            >
              {GPT_ABUSE_SORT_BY_OPTIONS.map((option) => (
                <NativeSelectOption key={option} value={option}>
                  {t(gptAbuseSortByLabelKey(option))}
                </NativeSelectOption>
              ))}
            </NativeSelect>
          </FilterField>
          <FilterField label={t('gptAbuse.filters.sortOrder')}>
            <NativeSelect
              value={props.search.sort_order}
              onChange={(event) =>
                props.onSearchChange(
                  updateGPTAbuseSearchForSorting(props.search, {
                    sort_order: event.target.value as GPTAbuseUserListSearch['sort_order'],
                  })
                )
              }
              className='w-full'
            >
              {GPT_ABUSE_SORT_ORDER_OPTIONS.map((option) => (
                <NativeSelectOption key={option} value={option}>
                  {t(gptAbuseSortOrderLabelKey(option))}
                </NativeSelectOption>
              ))}
            </NativeSelect>
          </FilterField>
          <FilterField label={t('gptAbuse.filters.limit')}>
            <NativeSelect
              value={String(props.search.limit)}
              onChange={(event) =>
                props.onSearchChange(
                  updateGPTAbuseSearchForPagination(props.search, {
                    limit: Number(event.target.value),
                    offset: 0,
                  })
                )
              }
              className='w-full'
            >
              {[10, 20, 50, 100].map((option) => (
                <NativeSelectOption key={option} value={option}>
                  {t('gptAbuse.pagination.perPage', { count: option })}
                </NativeSelectOption>
              ))}
            </NativeSelect>
          </FilterField>
        </div>
      </CardContent>
    </Card>
  )
}

function FilterField(props: { label: string; children: JSX.Element }): JSX.Element {
  return (
    <div className='space-y-2'>
      <Label>{props.label}</Label>
      {props.children}
    </div>
  )
}
