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
import { useTranslation } from 'react-i18next'
import { StatusBadge, type StatusVariant } from '@/components/status-badge'
import type { RouteHealthState } from '../types'

const variants: Record<RouteHealthState, StatusVariant> = {
  healthy: 'success',
  unobserved: 'neutral',
  cooling: 'warning',
  recovering: 'info',
  saturated: 'warning',
  manually_disabled: 'neutral',
  automatically_disabled: 'danger',
  no_credentials: 'danger',
  invalid_mapping: 'danger',
  protection_disabled: 'neutral',
}

function useHealthStateLabels(): Record<RouteHealthState, string> {
  const { t } = useTranslation()
  return {
    healthy: t('Healthy'),
    unobserved: t('Unobserved'),
    cooling: t('Cooling down'),
    recovering: t('Recovery probing'),
    saturated: t('Concurrency limit reached'),
    manually_disabled: t('Manually disabled'),
    automatically_disabled: t('Automatically disabled'),
    no_credentials: t('No enabled credentials'),
    invalid_mapping: t('Invalid model mapping'),
    protection_disabled: t('Health protection disabled'),
  }
}

export function HealthStateBadge(props: { state: RouteHealthState }) {
  const labels = useHealthStateLabels()
  return (
    <StatusBadge
      label={labels[props.state]}
      variant={variants[props.state]}
      copyable={false}
    />
  )
}
