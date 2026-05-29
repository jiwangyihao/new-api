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
import { RefreshCw } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Switch } from '@/components/ui/switch'
import type { AdminOpsHealth } from '../types'
import {
  getAdminOpsHealthLabelKey,
  getAdminOpsHealthTone,
} from '../lib/health'

type BadgeVariant = 'default' | 'secondary' | 'destructive' | 'outline'

const toneVariantMap: Record<string, BadgeVariant> = {
  success: 'secondary',
  warning: 'outline',
  destructive: 'destructive',
  muted: 'outline',
}

export function AdminOpsHeader(props: {
  health?: AdminOpsHealth
  generatedAt: number
  refreshing: boolean
  autoRefresh: boolean
  onAutoRefreshChange: (value: boolean) => void
  onRefresh: () => void
}) {
  const { t } = useTranslation()
  const healthStatus = props.health?.status ?? 'unknown'
  const tone = getAdminOpsHealthTone(healthStatus)
  const labelKey =
    healthStatus === 'healthy' ||
    healthStatus === 'degraded' ||
    healthStatus === 'critical'
      ? getAdminOpsHealthLabelKey(healthStatus)
      : 'Unknown'
  const updatedAt =
    props.generatedAt > 0 ? formatTimestamp(props.generatedAt) : '—'

  return (
    <Card>
      <CardContent
        className='flex flex-col gap-4 p-4 sm:flex-row sm:items-center sm:justify-between'
      >
        <div className='space-y-2'>
          <div className='flex flex-wrap items-center gap-2'>
            <Badge variant={toneVariantMap[tone]}>{t(labelKey)}</Badge>
            <span className='text-muted-foreground text-sm'>
              {t('adminOps.health.score')}: {props.health?.score ?? 0}
            </span>
          </div>
          <div className='text-muted-foreground text-sm'>
            {t('adminOps.header.lastUpdated')}: {updatedAt}
          </div>
        </div>
        <div className='flex flex-wrap items-center gap-3'>
          <label className='flex items-center gap-2 text-sm'>
            <Switch
              checked={props.autoRefresh}
              onCheckedChange={props.onAutoRefreshChange}
              aria-label={t('adminOps.header.autoRefresh')}
            />
            <span>{t('adminOps.header.autoRefresh')}</span>
          </label>
          <Button
            variant='outline'
            onClick={props.onRefresh}
            disabled={props.refreshing}
          >
            <RefreshCw
              className={props.refreshing ? 'animate-spin' : undefined}
              aria-hidden='true'
            />
            {props.refreshing
              ? t('adminOps.header.refreshing')
              : t('adminOps.header.manualRefresh')}
          </Button>
        </div>
      </CardContent>
    </Card>
  )
}

function formatTimestamp(seconds: number): string {
  const date = new Date(seconds * 1000)
  if (!Number.isFinite(date.getTime())) return '—'
  return date.toLocaleString()
}
