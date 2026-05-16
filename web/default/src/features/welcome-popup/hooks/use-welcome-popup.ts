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
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useAuthStore } from '@/stores/auth-store'
import { getWelcomePopup } from '../api'
import { buildClosedState, shouldShowWelcomePopup } from '../lib/welcome-popup-state'
import {
  readWelcomePopupClosedState,
  writeWelcomePopupClosedState,
} from '../lib/welcome-popup-storage'

type UseWelcomePopupResult = {
  open: boolean
  content: string
  close: () => void
}

export function useWelcomePopup(): UseWelcomePopupResult {
  const userId = useAuthStore((state) => state.auth.user?.id)
  const everySessionShownRef = useRef(false)
  const [open, setOpen] = useState(false)

  const query = useQuery({
    queryKey: ['welcome-popup', userId],
    queryFn: getWelcomePopup,
    enabled: Boolean(userId),
    staleTime: 5 * 60 * 1000,
  })

  const config = query.data
  const content = config?.content ?? ''

  useEffect(() => {
    if (!userId || !config) return

    const closedState = readWelcomePopupClosedState(userId)
    const shouldShow = shouldShowWelcomePopup({
      userId,
      enabled: config.enabled,
      content: config.content,
      frequency: config.frequency,
      closedState,
      shownThisSession: everySessionShownRef.current,
      now: new Date(),
    })

    if (shouldShow) {
      if (config.frequency === 'every_session') {
        everySessionShownRef.current = true
      }
      setOpen(true)
    }
  }, [config, userId])

  const close = useCallback(() => {
    if (userId && config?.content) {
      if (config.frequency !== 'every_session') {
        writeWelcomePopupClosedState(
          userId,
          buildClosedState({ content: config.content, now: new Date() })
        )
      }
    }

    everySessionShownRef.current = true
    setOpen(false)
  }, [config?.content, config?.frequency, userId])

  return useMemo(
    () => ({
      open,
      content,
      close,
    }),
    [close, content, open]
  )
}
