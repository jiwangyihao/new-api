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
import { useEffect } from 'react'
import { useQuery } from '@tanstack/react-query'
import { getSubscriptionConversionQuotes } from '../api'
import type { SubscriptionConversionQuoteList } from '../types'

export const CONVERSION_QUOTE_REFETCH_MS = 5000
export const subscriptionConversionQuoteQueryKey = [
  'subscriptions',
  'self',
  'conversion-quotes',
] as const

export function useSubscriptionConversionQuotes(
  queryFn: () => Promise<SubscriptionConversionQuoteList> = getSubscriptionConversionQuotes
) {
  const query = useQuery({
    queryKey: subscriptionConversionQuoteQueryKey,
    queryFn,
    refetchInterval: CONVERSION_QUOTE_REFETCH_MS,
    refetchIntervalInBackground: false,
    refetchOnWindowFocus: false,
  })
  const { refetch } = query

  useEffect(() => {
    const refreshOnFocus = () => {
      void refetch()
    }
    window.addEventListener('focus', refreshOnFocus)
    return () => window.removeEventListener('focus', refreshOnFocus)
  }, [refetch])

  return query
}
