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
import { api } from '@/lib/api'
import type { WelcomePopupConfig, WelcomePopupFrequency } from './types'

type WelcomePopupResponseData = {
  enabled?: unknown
  content?: unknown
  frequency?: unknown
}

function normalizeWelcomePopupFrequency(
  frequency: unknown
): WelcomePopupFrequency {
  if (frequency === 'once_per_day' || frequency === 'every_session') {
    return frequency
  }

  return 'once_per_version'
}

export async function getWelcomePopup(): Promise<WelcomePopupConfig> {
  const res = await api.get('/api/user/welcome-popup')
  const data = (res.data?.data ?? {}) as WelcomePopupResponseData

  return {
    enabled: data.enabled === true,
    content: typeof data.content === 'string' ? data.content : '',
    frequency: normalizeWelcomePopupFrequency(data.frequency),
  }
}
