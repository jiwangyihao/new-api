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
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { useSystemConfig } from '@/hooks/use-system-config'
import { cn } from '@/lib/utils'
import { Skeleton } from '@/components/ui/skeleton'

type AuthLayoutProps = {
  children: React.ReactNode
  backgroundImageSrc?: string
}

export function AuthLayout(props: AuthLayoutProps) {
  const { t } = useTranslation()
  const { systemName, logo, loading } = useSystemConfig()
  const hasBackground = Boolean(props.backgroundImageSrc)

  return (
    <div className='relative grid min-h-svh max-w-none overflow-hidden'>
      {hasBackground && (
        <div className='absolute inset-0' aria-hidden='true'>
          <img
            src={props.backgroundImageSrc}
            alt=''
            aria-hidden='true'
            className='absolute inset-0 h-full w-full object-cover object-center'
          />
          <div className='bg-background/80 absolute inset-0 backdrop-blur-[1px] sm:bg-background/70 lg:bg-background/45' />
          <div className='absolute inset-0 bg-[radial-gradient(circle_at_72%_38%,transparent_0,transparent_28%,var(--background)_78%)] opacity-95' />
        </div>
      )}
      <Link
        to='/'
        className='absolute top-4 left-4 z-20 flex items-center gap-2 transition-opacity hover:opacity-80 sm:top-8 sm:left-8'
      >
        <div className='relative h-8 w-8'>
          {loading ? (
            <Skeleton className='absolute inset-0 rounded-full' />
          ) : (
            <img
              src={logo}
              alt={t('Logo')}
              className='h-8 w-8 rounded-full object-cover'
            />
          )}
        </div>
        {loading ? (
          <Skeleton className='h-6 w-24' />
        ) : (
          <h1 className='text-xl font-medium'>{systemName}</h1>
        )}
      </Link>
      <div
        className={cn(
          'container relative z-10 flex',
          hasBackground
            ? 'items-start py-24 sm:items-center sm:py-0'
            : 'items-center pt-16 sm:pt-0'
        )}
      >
        <div
          className={cn(
            'mx-auto flex w-full flex-col justify-center space-y-2 px-4 py-8 sm:w-[480px] sm:p-8',
            hasBackground
              ? 'rounded-3xl border bg-background/90 shadow-2xl backdrop-blur-xl'
              : ''
          )}
        >
          {props.children}
        </div>
      </div>
    </div>
  )
}
