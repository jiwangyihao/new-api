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
import { useEffect, useState, type KeyboardEvent } from 'react'
import { X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { formatTimestampForInput, parseTimestampFromInput } from '@/lib/format'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { TagInput } from '@/components/tag-input'
import {
  USAGE_ANALYTICS_GRANULARITY_OPTIONS,
  USAGE_ANALYTICS_GROUP_BY_OPTIONS,
  USAGE_ANALYTICS_MAX_LIMIT,
  USAGE_ANALYTICS_METRIC_OPTIONS,
  USAGE_ANALYTICS_SORT_BY_OPTIONS,
  USAGE_ANALYTICS_SORT_ORDER_OPTIONS,
} from '../constants'
import { buildDefaultUsageAnalyticsFilters } from '../lib/page-contract'
import type {
  UsageAnalyticsCanonicalFilters,
  UsageAnalyticsGranularity,
  UsageAnalyticsMetric,
  UsageAnalyticsSortOrder,
  UsageAnalyticsStatus,
  UsageAnalyticsStream,
} from '../types'

interface UsageAnalyticsFilterBarProps {
  value: UsageAnalyticsCanonicalFilters
  onApply: (next: UsageAnalyticsCanonicalFilters) => void
}

const streamOptions: Array<{ value: UsageAnalyticsStream; labelKey: string }> = [
  { value: 'true', labelKey: 'Streaming' },
  { value: 'false', labelKey: 'Non-streaming' },
]

const statusOptions: Array<{ value: UsageAnalyticsStatus; labelKey: string }> = [
  { value: 'success', labelKey: 'Success' },
  { value: 'error', labelKey: 'Error' },
]

function normalizeLimit(value: number): number {
  if (!Number.isFinite(value)) return 1
  return Math.min(Math.max(Math.trunc(value), 1), USAGE_ANALYTICS_MAX_LIMIT)
}

function sortedTokenIds(ids: number[]): number[] {
  return Array.from(
    new Set(ids.filter((id) => Number.isSafeInteger(id) && id > 0))
  ).sort((first, second) => first - second)
}

function addTokenId(ids: number[], rawValue: string): number[] {
  const parsed = Number(rawValue.trim())
  if (!Number.isSafeInteger(parsed) || parsed <= 0) return ids
  return sortedTokenIds([...ids, parsed])
}

function removeTokenId(ids: number[], id: number): number[] {
  return ids.filter((item) => item !== id)
}

function toggleStream(
  values: UsageAnalyticsStream[],
  value: UsageAnalyticsStream
): UsageAnalyticsStream[] {
  const next = values.includes(value)
    ? values.filter((item) => item !== value)
    : [...values, value]
  return streamOptions
    .map((option) => option.value)
    .filter((item) => next.includes(item))
}

function toggleStatus(
  values: UsageAnalyticsStatus[],
  value: UsageAnalyticsStatus
): UsageAnalyticsStatus[] {
  const next = values.includes(value)
    ? values.filter((item) => item !== value)
    : [...values, value]
  return statusOptions
    .map((option) => option.value)
    .filter((item) => next.includes(item))
}

function buildAppliedFilters(
  draft: UsageAnalyticsCanonicalFilters,
  startInput: string,
  endInput: string
): UsageAnalyticsCanonicalFilters {
  const startTimestamp = parseTimestampFromInput(startInput)
  const endTimestamp = parseTimestampFromInput(endInput)
  return {
    ...draft,
    start_timestamp:
      startTimestamp > 0 ? startTimestamp : draft.start_timestamp,
    end_timestamp: endTimestamp > 0 ? endTimestamp : draft.end_timestamp,
    token_ids: sortedTokenIds(draft.token_ids),
    model_names: Array.from(
      new Set(draft.model_names.map((item) => item.trim()).filter(Boolean))
    ).sort(),
    limit: normalizeLimit(draft.limit),
  }
}

export function UsageAnalyticsFilterBar(props: UsageAnalyticsFilterBarProps) {
  const { t } = useTranslation()
  const [draft, setDraft] = useState<UsageAnalyticsCanonicalFilters>(
    props.value
  )
  const [startInput, setStartInput] = useState(
    formatTimestampForInput(props.value.start_timestamp)
  )
  const [endInput, setEndInput] = useState(
    formatTimestampForInput(props.value.end_timestamp)
  )
  const [tokenInput, setTokenInput] = useState('')

  useEffect(() => {
    setDraft(props.value)
    setStartInput(formatTimestampForInput(props.value.start_timestamp))
    setEndInput(formatTimestampForInput(props.value.end_timestamp))
  }, [props.value])

  const addCurrentTokenId = () => {
    setDraft((previous) => ({
      ...previous,
      token_ids: addTokenId(previous.token_ids, tokenInput),
    }))
    setTokenInput('')
  }

  const handleTokenKeyDown = (event: KeyboardEvent<HTMLInputElement>) => {
    if (event.key !== 'Enter') return
    event.preventDefault()
    addCurrentTokenId()
  }

  const handleApply = () => {
    props.onApply(buildAppliedFilters(draft, startInput, endInput))
  }

  const handleReset = () => {
    const next = buildDefaultUsageAnalyticsFilters(Math.floor(Date.now() / 1000))
    setDraft(next)
    setStartInput(formatTimestampForInput(next.start_timestamp))
    setEndInput(formatTimestampForInput(next.end_timestamp))
    setTokenInput('')
    props.onApply(next)
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('Filters')}</CardTitle>
      </CardHeader>
      <CardContent>
        <div className='grid gap-4'>
          <div className='grid gap-3 sm:grid-cols-2 lg:grid-cols-4'>
            <div className='grid gap-2'>
              <Label htmlFor='usage-analytics-start-time'>
                {t('Start Time')}
              </Label>
              <Input
                id='usage-analytics-start-time'
                type='datetime-local'
                value={startInput}
                onChange={(event) => setStartInput(event.target.value)}
              />
            </div>
            <div className='grid gap-2'>
              <Label htmlFor='usage-analytics-end-time'>{t('End Time')}</Label>
              <Input
                id='usage-analytics-end-time'
                type='datetime-local'
                value={endInput}
                onChange={(event) => setEndInput(event.target.value)}
              />
            </div>
            <Select
              value={draft.group_by}
              items={USAGE_ANALYTICS_GROUP_BY_OPTIONS.map((option) => ({
                value: option.value,
                label: t(option.labelKey),
              }))}
              onValueChange={(value) => {
                if (value === null) return
                setDraft((previous) => ({
                  ...previous,
                  group_by: value as UsageAnalyticsCanonicalFilters['group_by'],
                }))
              }}
            >
              <div className='grid gap-2'>
                <Label>{t('Group by')}</Label>
                <SelectTrigger className='w-full'>
                  <SelectValue placeholder={t('Group by')} />
                </SelectTrigger>
              </div>
              <SelectContent alignItemWithTrigger={false}>
                <SelectGroup>
                  {USAGE_ANALYTICS_GROUP_BY_OPTIONS.map((option) => (
                    <SelectItem key={option.value} value={option.value}>
                      {t(option.labelKey)}
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>
            <Select
              value={draft.metric}
              items={USAGE_ANALYTICS_METRIC_OPTIONS.map((option) => ({
                value: option.value,
                label: t(option.labelKey),
              }))}
              onValueChange={(value) => {
                if (value === null) return
                setDraft((previous) => ({
                  ...previous,
                  metric: value as UsageAnalyticsMetric,
                  sort_by:
                    previous.sort_by === previous.metric ? value : previous.sort_by,
                }))
              }}
            >
              <div className='grid gap-2'>
                <Label>{t('Metric')}</Label>
                <SelectTrigger className='w-full'>
                  <SelectValue placeholder={t('Metric')} />
                </SelectTrigger>
              </div>
              <SelectContent alignItemWithTrigger={false}>
                <SelectGroup>
                  {USAGE_ANALYTICS_METRIC_OPTIONS.map((option) => (
                    <SelectItem key={option.value} value={option.value}>
                      {t(option.labelKey)}
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>
          </div>

          <div className='grid gap-3 sm:grid-cols-2 lg:grid-cols-4'>
            <Select
              value={draft.granularity}
              items={USAGE_ANALYTICS_GRANULARITY_OPTIONS.map((option) => ({
                value: option.value,
                label: t(option.labelKey),
              }))}
              onValueChange={(value) => {
                if (value === null) return
                setDraft((previous) => ({
                  ...previous,
                  granularity: value as UsageAnalyticsGranularity,
                }))
              }}
            >
              <div className='grid gap-2'>
                <Label>{t('Granularity')}</Label>
                <SelectTrigger className='w-full'>
                  <SelectValue placeholder={t('Granularity')} />
                </SelectTrigger>
              </div>
              <SelectContent alignItemWithTrigger={false}>
                <SelectGroup>
                  {USAGE_ANALYTICS_GRANULARITY_OPTIONS.map((option) => (
                    <SelectItem key={option.value} value={option.value}>
                      {t(option.labelKey)}
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>
            <div className='grid gap-2'>
              <Label htmlFor='usage-analytics-limit'>{t('Top N')}</Label>
              <Input
                id='usage-analytics-limit'
                type='number'
                min={1}
                max={USAGE_ANALYTICS_MAX_LIMIT}
                value={draft.limit}
                onChange={(event) => {
                  const parsed = Number(event.target.value)
                  setDraft((previous) => ({
                    ...previous,
                    limit: normalizeLimit(parsed),
                  }))
                }}
              />
            </div>
            <Select
              value={draft.sort_by}
              items={USAGE_ANALYTICS_SORT_BY_OPTIONS.map((option) => ({
                value: option.value,
                label: t(option.labelKey),
              }))}
              onValueChange={(value) => {
                if (value === null) return
                setDraft((previous) => ({ ...previous, sort_by: value }))
              }}
            >
              <div className='grid gap-2'>
                <Label>{t('Sort by')}</Label>
                <SelectTrigger className='w-full'>
                  <SelectValue placeholder={t('Sort by')} />
                </SelectTrigger>
              </div>
              <SelectContent alignItemWithTrigger={false}>
                <SelectGroup>
                  {USAGE_ANALYTICS_SORT_BY_OPTIONS.map((option) => (
                    <SelectItem key={option.value} value={option.value}>
                      {t(option.labelKey)}
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>
            <Select
              value={draft.sort_order}
              items={USAGE_ANALYTICS_SORT_ORDER_OPTIONS.map((option) => ({
                value: option.value,
                label: t(option.labelKey),
              }))}
              onValueChange={(value) => {
                if (value === null) return
                setDraft((previous) => ({
                  ...previous,
                  sort_order: value as UsageAnalyticsSortOrder,
                }))
              }}
            >
              <div className='grid gap-2'>
                <Label>{t('Sort order')}</Label>
                <SelectTrigger className='w-full'>
                  <SelectValue placeholder={t('Sort order')} />
                </SelectTrigger>
              </div>
              <SelectContent alignItemWithTrigger={false}>
                <SelectGroup>
                  {USAGE_ANALYTICS_SORT_ORDER_OPTIONS.map((option) => (
                    <SelectItem key={option.value} value={option.value}>
                      {t(option.labelKey)}
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>
          </div>

          <div className='grid gap-3 lg:grid-cols-3'>
            <div className='grid gap-2'>
              <Label htmlFor='usage-analytics-token-id'>{t('API Key IDs')}</Label>
              <div className='flex gap-2'>
                <Input
                  id='usage-analytics-token-id'
                  type='number'
                  min={1}
                  value={tokenInput}
                  onChange={(event) => setTokenInput(event.target.value)}
                  onKeyDown={handleTokenKeyDown}
                  placeholder={t('Enter API Key ID')}
                />
                <Button type='button' variant='outline' onClick={addCurrentTokenId}>
                  {t('Add')}
                </Button>
              </div>
              {draft.token_ids.length > 0 && (
                <div className='flex flex-wrap gap-1.5'>
                  {draft.token_ids.map((id) => (
                    <Badge key={id} variant='secondary' className='gap-1 pr-1'>
                      {id}
                      <Button
                        type='button'
                        variant='ghost'
                        size='icon-sm'
                        aria-label={t('Remove')}
                        onClick={() =>
                          setDraft((previous) => ({
                            ...previous,
                            token_ids: removeTokenId(previous.token_ids, id),
                          }))
                        }
                        className='hover:bg-secondary-foreground/20 size-auto rounded-sm p-0'
                      >
                        <X className='h-3 w-3' aria-hidden='true' />
                      </Button>
                    </Badge>
                  ))}
                </div>
              )}
            </div>
            <div className='grid gap-2'>
              <Label>{t('Models')}</Label>
              <TagInput
                value={draft.model_names}
                onChange={(modelNames) =>
                  setDraft((previous) => ({
                    ...previous,
                    model_names: modelNames,
                  }))
                }
                placeholder={t('Add model name')}
              />
            </div>
          </div>

          <div className='grid gap-3 lg:grid-cols-2'>
            <div className='grid gap-2'>
              <Label>{t('Stream status')}</Label>
              <div className='flex flex-wrap gap-2'>
                {streamOptions.map((option) => (
                  <Button
                    key={option.value}
                    type='button'
                    variant={
                      draft.streams.includes(option.value) ? 'secondary' : 'outline'
                    }
                    onClick={() =>
                      setDraft((previous) => ({
                        ...previous,
                        streams: toggleStream(previous.streams, option.value),
                      }))
                    }
                  >
                    {t(option.labelKey)}
                  </Button>
                ))}
              </div>
            </div>
            <div className='grid gap-2'>
              <Label>{t('Result status')}</Label>
              <div className='flex flex-wrap gap-2'>
                {statusOptions.map((option) => (
                  <Button
                    key={option.value}
                    type='button'
                    variant={
                      draft.statuses.includes(option.value) ? 'secondary' : 'outline'
                    }
                    onClick={() =>
                      setDraft((previous) => ({
                        ...previous,
                        statuses: toggleStatus(previous.statuses, option.value),
                      }))
                    }
                  >
                    {t(option.labelKey)}
                  </Button>
                ))}
              </div>
            </div>
          </div>

          <div className='flex flex-col-reverse gap-2 sm:flex-row sm:justify-end'>
            <Button type='button' variant='outline' onClick={handleReset}>
              {t('Reset filters')}
            </Button>
            <Button type='button' onClick={handleApply}>
              {t('Apply filters')}
            </Button>
          </div>
        </div>
      </CardContent>
    </Card>
  )
}
