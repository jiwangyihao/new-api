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
import { ExternalLink, Gift } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Spinner } from '@/components/ui/spinner'
import { TitledCard } from '@/components/ui/titled-card'

interface RedemptionCodeCardProps {
  code: string
  redeeming: boolean
  topupLink?: string
  onCodeChange: (code: string) => void
  onRedeem: () => Promise<void> | void
}

export function RedemptionCodeCard(props: RedemptionCodeCardProps) {
  const { t } = useTranslation()

  return (
    <TitledCard
      title={t('Redemption Code')}
      description={t('Enter your redemption code')}
      icon={<Gift className='h-4 w-4' aria-hidden='true' />}
      contentClassName='space-y-3'
    >
      <form
        className='space-y-3'
        onSubmit={(event) => {
          event.preventDefault()
          void props.onRedeem()
        }}
      >
        <div className='space-y-2'>
          <Label htmlFor='subscription-redemption-code'>
            {t('Redemption Code')}
          </Label>
          <div className='flex flex-col gap-2 sm:flex-row'>
            <Input
              id='subscription-redemption-code'
              value={props.code}
              onChange={(event) => props.onCodeChange(event.target.value)}
              placeholder={t('Enter your redemption code')}
              autoComplete='off'
              spellCheck={false}
              disabled={props.redeeming}
            />
            <Button
              type='submit'
              className='sm:shrink-0'
              disabled={props.redeeming || props.code.trim() === ''}
            >
              {props.redeeming && <Spinner data-icon='inline-start' />}
              {t('Redeem')}
            </Button>
          </div>
        </div>

        {props.topupLink && (
          <p className='text-muted-foreground text-xs'>
            {t('Need a redemption code?')}{' '}
            <a
              href={props.topupLink}
              target='_blank'
              rel='noopener noreferrer'
              className='inline-flex items-center gap-1 underline-offset-4 hover:underline'
            >
              {t('Get one here')}
              <ExternalLink className='h-3 w-3' aria-hidden='true' />
            </a>
          </p>
        )}
      </form>
    </TitledCard>
  )
}
