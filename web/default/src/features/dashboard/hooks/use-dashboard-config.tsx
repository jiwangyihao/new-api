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
import {
  Hash,
  Layers,
  Gauge,
  Zap,
  Flame,
  TrendingUp,
  Activity,
  type LucideIcon,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { safeDivide } from '@/features/dashboard/lib'

interface StatCardConfig {
  key: string
  title: string
  description: string
  icon: LucideIcon
  getValue: (stat: Record<string, number>, days?: number) => number
}

export function useModelStatCardsConfig(): StatCardConfig[] {
  const { t } = useTranslation()

  return [
    {
      key: 'count',
      title: t('Total Count'),
      description: t('Statistical count'),
      icon: Hash,
      getValue: (stat) => stat?.rpm ?? 0,
    },
    {
      key: 'tokens',
      title: t('Total Tokens Used'),
      description: t('Tokens used in selected range'),
      icon: Layers,
      getValue: (stat) => stat?.tpm ?? 0,
    },
    {
      key: 'avgRpm',
      title: t('Average RPM'),
      description: t('Requests per minute'),
      icon: Gauge,
      getValue: (stat, timeRangeMinutes = 1) =>
        safeDivide(stat?.rpm ?? 0, timeRangeMinutes),
    },
    {
      key: 'avgTpm',
      title: t('Average TPM'),
      description: t('Tokens per minute'),
      icon: Zap,
      getValue: (stat, timeRangeMinutes = 1) =>
        safeDivide(stat?.tpm ?? 0, timeRangeMinutes),
    },
  ]
}

export function useSummaryCardsConfig(totals: {
  remainingTokensDisplay: string
  cycleTokensDisplay: string
  recentTokensDisplay: string
  requestCountDisplay: string
}) {
  const { t } = useTranslation()

  return [
    {
      key: 'remainingTokens',
      title: t('Subscription tokens remaining'),
      value: totals.remainingTokensDisplay,
      description: t('Tokens available in the current plan cycle'),
      icon: Zap,
    },
    {
      key: 'cycleTokens',
      title: t('Current cycle tokens used'),
      value: totals.cycleTokensDisplay,
      description: t('Tokens used by the active subscription cycle'),
      icon: TrendingUp,
    },
    {
      key: 'recentTokens',
      title: t('Last 24h token usage'),
      value: totals.recentTokensDisplay,
      description: t('Tokens used in the last 24 hours'),
      icon: Flame,
    },
    {
      key: 'requests',
      title: t('Request Count'),
      value: totals.requestCountDisplay,
      description: t('Total requests made'),
      icon: Activity,
    },
  ]
}
