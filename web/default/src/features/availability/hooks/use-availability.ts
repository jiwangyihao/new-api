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
import { useState, useSyncExternalStore } from 'react'
import { useQuery } from '@tanstack/react-query'
import { getAvailability } from '../api'
import type { AvailabilityRange, AvailabilityReport } from '../types'

function subscribeVisibility(notify: () => void) {
  document.addEventListener('visibilitychange', notify)
  return () => document.removeEventListener('visibilitychange', notify)
}
function isVisible() {
  return document.visibilityState !== 'hidden'
}

export function useAvailability(range: AvailabilityRange) {
  const visible = useSyncExternalStore(
    subscribeVisibility,
    isVisible,
    () => true
  )
  const [lastReport, setLastReport] = useState<AvailabilityReport>()
  const query = useQuery({
    queryKey: ['availability', range],
    queryFn: ({ signal }) => getAvailability(range, signal),
    enabled: visible,
    refetchInterval: visible ? 60_000 : false,
    refetchIntervalInBackground: false,
    refetchOnWindowFocus: 'always',
    staleTime: 0,
    retry: false,
  })
  if (query.data && query.data !== lastReport) {
    setLastReport(query.data)
  }
  return {
    report: query.data ?? lastReport,
    isError: query.isError,
    isFetching: query.isFetching,
    refetch: query.refetch,
  }
}
