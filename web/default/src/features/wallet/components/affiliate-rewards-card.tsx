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
import { CalendarCheck, Share2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Card, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { CopyButton } from '@/components/copy-button'
import { formatAffiliateEntitlementEndTime } from '../lib'
import type { InvitationEntitlement } from '../types'

interface AffiliateRewardsCardProps {
  affiliateLink: string
  entitlement?: InvitationEntitlement | null
  loading?: boolean
}

export function AffiliateRewardsCard({
  affiliateLink,
  entitlement,
  loading,
}: AffiliateRewardsCardProps) {
  const { t } = useTranslation()
  if (loading) {
    return (
      <Card className='bg-muted/20 py-0'>
        <CardContent className='grid gap-4 p-3 sm:p-4 lg:grid-cols-[minmax(220px,1fr)_minmax(220px,0.72fr)_minmax(320px,1.15fr)] lg:items-center'>
          <div>
            <Skeleton className='h-5 w-32' />
            <Skeleton className='mt-2 h-4 w-48' />
          </div>
          <Skeleton className='h-14 rounded-lg' />
          <Skeleton className='h-10 rounded-lg' />
        </CardContent>
      </Card>
    )
  }

  const referralCopy = t(
    '赔钱GPT超低价稳定GPT服务，用我邀请链接注册可免费试用一天无限token：'
  )
  const currentRewardTitle = entitlement?.entitled
    ? entitlement.reward_plan_title || t('Granted')
    : t('Not granted')
  const hasDowngradeReward =
    (entitlement?.downgrade_reward_plan_id ?? 0) > 0 &&
    (entitlement?.downgrade_entitlement_end_time ?? 0) >
      (entitlement?.entitlement_end_time ?? 0)

  return (
    <Card className='bg-muted/20 py-0'>
      <CardContent className='grid gap-3 p-3 sm:gap-4 sm:p-4 lg:grid-cols-[minmax(200px,1fr)_minmax(180px,0.65fr)_minmax(280px,1fr)] lg:items-center'>
        <div className='flex min-w-0 items-center gap-2.5'>
          <div className='bg-background flex size-8 shrink-0 items-center justify-center rounded-lg border'>
            <Share2 className='text-muted-foreground size-4' />
          </div>
          <div className='min-w-0'>
            <h3 className='truncate text-sm font-semibold'>
              {t('Referral Program')}
            </h3>
            <p className='text-muted-foreground line-clamp-1 text-xs'>
              {t(
                'Earn rewards when your referrals add funds. Transfer accumulated rewards to your balance anytime.'
              )}
            </p>
          </div>
        </div>

        <div className='text-muted-foreground text-xs lg:text-right'>
          {referralCopy}
        </div>

        <div className='grid grid-cols-2 gap-2 text-xs lg:col-span-3 lg:grid-cols-6'>
          <div className='bg-background/60 rounded-lg border p-2'>
            <div className='text-muted-foreground'>{t('Direct invites')}</div>
            <div className='mt-1 font-semibold tabular-nums'>
              {entitlement?.direct_invite_count ?? 0}
            </div>
          </div>
          <div className='bg-background/60 rounded-lg border p-2'>
            <div className='text-muted-foreground'>
              {t('Qualified paid invites')}
            </div>
            <div className='mt-1 font-semibold tabular-nums'>
              {entitlement?.qualified_active_count ?? 0}
            </div>
          </div>
          <div className='bg-background/60 rounded-lg border p-2'>
            <div className='text-muted-foreground'>{t('Monthly reward')}</div>
            <div className='mt-1 flex items-center gap-1 font-semibold'>
              <CalendarCheck className='size-3.5' />
              {currentRewardTitle}
            </div>
          </div>
          <div className='bg-background/60 rounded-lg border p-2'>
            <div className='text-muted-foreground'>{t('Reward month')}</div>
            <div className='mt-1 font-semibold tabular-nums'>
              {entitlement?.reward_month || '-'}
            </div>
          </div>
          <div className='bg-background/60 rounded-lg border p-2'>
            <div className='text-muted-foreground'>
              {t('Reward valid until')}
            </div>
            <div className='mt-1 font-semibold tabular-nums'>
              {formatAffiliateEntitlementEndTime(
                entitlement?.entitlement_end_time ?? 0
              )}
            </div>
          </div>
          {hasDowngradeReward && (
            <div className='bg-background/60 rounded-lg border p-2'>
              <div className='text-muted-foreground'>{t('Downgrades to')}</div>
              <div className='mt-1 font-semibold'>
                {entitlement?.downgrade_reward_plan_title}
              </div>
              <div className='text-muted-foreground mt-0.5 tabular-nums'>
                {formatAffiliateEntitlementEndTime(
                  entitlement?.downgrade_entitlement_end_time ?? 0
                )}
              </div>
            </div>
          )}
        </div>

        <div className='flex flex-col gap-2 lg:col-span-3'>
          <div className='text-muted-foreground text-xs sm:hidden'>
            {referralCopy}
          </div>
          <div className='flex items-center gap-2'>
            <Input
              value={affiliateLink}
              readOnly
              className='border-muted bg-background/70 h-9 min-w-0 flex-1 font-mono text-xs'
            />
            <CopyButton
              value={`${referralCopy}${affiliateLink}`}
              variant='outline'
              className='bg-background size-9 shrink-0'
              iconClassName='size-4'
              tooltip={t('Copy referral link')}
              aria-label={t('Copy referral link')}
            />
          </div>
        </div>

        <div className='bg-background/60 rounded-lg border p-3 text-xs lg:col-span-3'>
          <h4 className='text-foreground font-semibold'>
            {t('Invitation reward rules')}
          </h4>
          <ul className='text-muted-foreground mt-2 list-disc space-y-1 pl-4'>
            <li>
              {t(
                'Invite at least two direct users with active paid subscriptions to receive the highest qualified matching reward plan.'
              )}
            </li>
            <li>
              {t(
                'The reward is valid until the overlap end time of your two longest valid paid referrals.'
              )}
            </li>
            <li>
              {t(
                'When the reward is the same tier as your paid plan, reward time is consumed first and paid time is preserved.'
              )}
            </li>
            <li>
              {t(
                'When tiers differ, choose the active plan in Wallet. Reward usage does not consume paid plan time; paid plan usage lets both natural validity windows elapse.'
              )}
            </li>
            <li>
              {t(
                'Quota reset consumes one month from a paid plan and cannot be paid by invitation rewards.'
              )}
            </li>
          </ul>
        </div>
      </CardContent>
    </Card>
  )
}
