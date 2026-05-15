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
import { ExternalLink } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Link } from '@tanstack/react-router'
import { cn } from '@/lib/utils'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { CopyButton } from '@/components/copy-button'
import type { AppGuide } from '../types'

type AppGuideCardProps = {
  guide: AppGuide
  hasApiKey: boolean
}

export function AppGuideCard(props: AppGuideCardProps) {
  const { t } = useTranslation()
  const guide = props.guide
  const Icon = guide.icon
  const importLinks = guide.importLinks ?? []

  return (
    <Card className='h-full'>
      <CardHeader>
        <div className='flex items-start gap-3'>
          <div className='bg-muted flex size-10 shrink-0 items-center justify-center rounded-lg border'>
            <Icon className='text-muted-foreground size-5' />
          </div>
          <div className='min-w-0'>
            <CardTitle>{guide.name}</CardTitle>
            <CardDescription className='mt-1'>
              {guide.description}
            </CardDescription>
          </div>
        </div>
        <CardAction>
          <CopyButton
            value={guide.configValue}
            variant='outline'
            className='size-8'
            iconClassName='size-4'
            tooltip={t('Copy configuration')}
            aria-label={t('Copy configuration')}
          />
        </CardAction>
      </CardHeader>

      <CardContent className='space-y-4'>
        <div className='space-y-2'>
          <div className='text-muted-foreground text-xs font-medium tracking-wide uppercase'>
            {guide.configTitle}
          </div>
          <pre className='bg-muted/50 max-h-56 overflow-auto rounded-lg border p-3 text-xs whitespace-pre-wrap'>
            <code>{guide.configValue}</code>
          </pre>
        </div>

        {importLinks.length > 0 && (
          <div className='flex flex-wrap gap-2'>
            {importLinks.map((link) => {
              const disabled = Boolean(
                link.disabled || (link.requiresKey && !props.hasApiKey)
              )
              return (
                <Button
                  key={link.id ?? link.label}
                  variant='outline'
                  size='sm'
                  className={cn('gap-1.5', disabled && 'pointer-events-none')}
                  disabled={disabled}
                  render={
                    disabled ? undefined : (
                      <a
                        href={link.href}
                        target='_blank'
                        rel='noreferrer noopener'
                      />
                    )
                  }
                >
                  <ExternalLink className='size-3.5' />
                  {link.label}
                </Button>
              )
            })}
          </div>
        )}

        {!props.hasApiKey && guide.configValue.includes('OPENAI_API_KEY=') && (
          <Alert>
            <AlertDescription>
              {t('Create an API key first to enable one-click imports.')}{' '}
              <Link to='/keys' className='underline underline-offset-4'>
                {t('Create API Key')}
              </Link>
            </AlertDescription>
          </Alert>
        )}
      </CardContent>
    </Card>
  )
}
