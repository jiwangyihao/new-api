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
'use client'

import { type ComponentProps, createContext, useContext } from 'react'
import type { LanguageModelUsage } from 'ai'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import {
  HoverCard,
  HoverCardContent,
  HoverCardTrigger,
} from '@/components/ui/hover-card'
import { Progress } from '@/components/ui/progress'

const PERCENT_MAX = 100
const ICON_RADIUS = 10
const ICON_VIEWBOX = 24
const ICON_CENTER = 12
const ICON_STROKE_WIDTH = 2

type ModelId = string

type ContextSchema = {
  usedTokens: number
  maxTokens: number
  usage?: LanguageModelUsage
  modelId?: ModelId
}

const ContextContext = createContext<ContextSchema | null>(null)

const useContextValue = () => {
  const context = useContext(ContextContext)

  if (!context) {
    throw new Error('Context components must be used within Context')
  }

  return context
}

export type ContextProps = ComponentProps<typeof HoverCard> & ContextSchema

export const Context = ({
  usedTokens,
  maxTokens,
  usage,
  modelId,
  ...props
}: ContextProps) => (
  <ContextContext.Provider
    value={{
      usedTokens,
      maxTokens,
      usage,
      modelId,
    }}
  >
    <HoverCard {...props} />
  </ContextContext.Provider>
)

const ContextIcon = () => {
  const { t } = useTranslation()
  const { usedTokens, maxTokens } = useContextValue()
  const circumference = 2 * Math.PI * ICON_RADIUS
  const usedPercent = usedTokens / maxTokens
  const dashOffset = circumference * (1 - usedPercent)

  return (
    <svg
      aria-label={t('Model context usage')}
      height='20'
      role='img'
      style={{ color: 'currentcolor' }}
      viewBox={`0 0 ${ICON_VIEWBOX} ${ICON_VIEWBOX}`}
      width='20'
    >
      <circle
        cx={ICON_CENTER}
        cy={ICON_CENTER}
        fill='none'
        opacity='0.25'
        r={ICON_RADIUS}
        stroke='currentColor'
        strokeWidth={ICON_STROKE_WIDTH}
      />
      <circle
        cx={ICON_CENTER}
        cy={ICON_CENTER}
        fill='none'
        opacity='0.7'
        r={ICON_RADIUS}
        stroke='currentColor'
        strokeDasharray={`${circumference} ${circumference}`}
        strokeDashoffset={dashOffset}
        strokeLinecap='round'
        strokeWidth={ICON_STROKE_WIDTH}
        style={{ transformOrigin: 'center', transform: 'rotate(-90deg)' }}
      />
    </svg>
  )
}

export type ContextTriggerProps = ComponentProps<typeof Button>

export const ContextTrigger = ({ children, ...props }: ContextTriggerProps) => {
  const { usedTokens, maxTokens } = useContextValue()
  const usedPercent = usedTokens / maxTokens
  const renderedPercent = new Intl.NumberFormat('en-US', {
    style: 'percent',
    maximumFractionDigits: 1,
  }).format(usedPercent)

  return (
    <HoverCardTrigger
      delay={0}
      closeDelay={0}
      render={
        <Button type='button' variant='ghost' {...props}>
          {children ?? (
            <>
              <span className='text-muted-foreground font-medium'>
                {renderedPercent}
              </span>
              <ContextIcon />
            </>
          )}
        </Button>
      }
    />
  )
}

export type ContextContentProps = ComponentProps<typeof HoverCardContent>

export const ContextContent = ({
  className,
  ...props
}: ContextContentProps) => (
  <HoverCardContent
    className={cn('min-w-60 divide-y overflow-hidden p-0', className)}
    {...props}
  />
)

export type ContextContentHeaderProps = ComponentProps<'div'>

export const ContextContentHeader = ({
  children,
  className,
  ...props
}: ContextContentHeaderProps) => {
  const { usedTokens, maxTokens } = useContextValue()
  const usedPercent = usedTokens / maxTokens
  const displayPct = new Intl.NumberFormat('en-US', {
    style: 'percent',
    maximumFractionDigits: 1,
  }).format(usedPercent)
  const used = new Intl.NumberFormat('en-US', {
    notation: 'compact',
  }).format(usedTokens)
  const total = new Intl.NumberFormat('en-US', {
    notation: 'compact',
  }).format(maxTokens)

  return (
    <div className={cn('w-full space-y-2 p-3', className)} {...props}>
      {children ?? (
        <>
          <div className='flex items-center justify-between gap-3 text-xs'>
            <p>{displayPct}</p>
            <p className='text-muted-foreground font-mono'>
              {used} / {total}
            </p>
          </div>
          <div className='space-y-2'>
            <Progress className='bg-muted' value={usedPercent * PERCENT_MAX} />
          </div>
        </>
      )}
    </div>
  )
}

export type ContextContentBodyProps = ComponentProps<'div'>

export const ContextContentBody = ({
  children,
  className,
  ...props
}: ContextContentBodyProps) => (
  <div className={cn('w-full p-3', className)} {...props}>
    {children}
  </div>
)

export type ContextContentFooterProps = ComponentProps<'div'>

export const ContextContentFooter = ({
  children,
  className,
  ...props
}: ContextContentFooterProps) => {
  const { t } = useTranslation()
  const { usage } = useContextValue()
  const totalTokens =
    (usage?.inputTokens ?? 0) +
    (usage?.outputTokens ?? 0) +
    (usage?.reasoningTokens ?? 0) +
    (usage?.cachedInputTokens ?? 0)

  return (
    <div
      className={cn(
        'bg-secondary flex w-full items-center justify-between gap-3 p-3 text-xs',
        className
      )}
      {...props}
    >
      {children ?? (
        <>
          <span className='text-muted-foreground'>{t('Total tokens')}</span>
          <TokenCount tokens={totalTokens} />
        </>
      )}
    </div>
  )
}

export type ContextInputUsageProps = ComponentProps<'div'>

export const ContextInputUsage = ({
  className,
  children,
  ...props
}: ContextInputUsageProps) => {
  const { t } = useTranslation()
  const { usage } = useContextValue()
  const inputTokens = usage?.inputTokens ?? 0

  if (children) {
    return children
  }

  if (!inputTokens) {
    return null
  }


  return (
    <div
      className={cn('flex items-center justify-between text-xs', className)}
      {...props}
    >
      <span className='text-muted-foreground'>{t('Input')}</span>
      <TokenCount tokens={inputTokens} />
    </div>
  )
}

export type ContextOutputUsageProps = ComponentProps<'div'>

export const ContextOutputUsage = ({
  className,
  children,
  ...props
}: ContextOutputUsageProps) => {
  const { t } = useTranslation()
  const { usage } = useContextValue()
  const outputTokens = usage?.outputTokens ?? 0

  if (children) {
    return children
  }

  if (!outputTokens) {
    return null
  }


  return (
    <div
      className={cn('flex items-center justify-between text-xs', className)}
      {...props}
    >
      <span className='text-muted-foreground'>{t('Output')}</span>
      <TokenCount tokens={outputTokens} />
    </div>
  )
}

export type ContextReasoningUsageProps = ComponentProps<'div'>

export const ContextReasoningUsage = ({
  className,
  children,
  ...props
}: ContextReasoningUsageProps) => {
  const { t } = useTranslation()
  const { usage } = useContextValue()
  const reasoningTokens = usage?.reasoningTokens ?? 0

  if (children) {
    return children
  }

  if (!reasoningTokens) {
    return null
  }


  return (
    <div
      className={cn('flex items-center justify-between text-xs', className)}
      {...props}
    >
      <span className='text-muted-foreground'>{t('Reasoning')}</span>
      <TokenCount tokens={reasoningTokens} />
    </div>
  )
}

export type ContextCacheUsageProps = ComponentProps<'div'>

export const ContextCacheUsage = ({
  className,
  children,
  ...props
}: ContextCacheUsageProps) => {
  const { t } = useTranslation()
  const { usage } = useContextValue()
  const cacheTokens = usage?.cachedInputTokens ?? 0

  if (children) {
    return children
  }

  if (!cacheTokens) {
    return null
  }


  return (
    <div
      className={cn('flex items-center justify-between text-xs', className)}
      {...props}
    >
      <span className='text-muted-foreground'>{t('Cache')}</span>
      <TokenCount tokens={cacheTokens} />
    </div>
  )
}

const TokenCount = ({ tokens }: { tokens?: number }) => (
  <span>
    {tokens === undefined
      ? '—'
      : new Intl.NumberFormat('en-US', {
          notation: 'compact',
        }).format(tokens)}
  </span>
)
