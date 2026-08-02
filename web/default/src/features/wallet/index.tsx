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
import { SectionPageLayout } from '@/components/layout'
import { TimedSubscriptionConversionQuotesCard } from '@/features/subscription-conversion/components/timed-subscription-conversion-quotes-card'
import { AffiliateRewardsCard } from './components/affiliate-rewards-card'
import { SubscriptionPlansCard } from './components/subscription-plans-card'
import { useAffiliate, useTopupInfo } from './hooks'

export function Wallet() {
  const { t } = useTranslation()
  const { topupInfo } = useTopupInfo()
  const affiliate = useAffiliate()
  return (
    <>
      <SectionPageLayout>
        <SectionPageLayout.Title>{t('Subscriptions')}</SectionPageLayout.Title>
        <SectionPageLayout.Description>
          {t('Subscription Plans')}
        </SectionPageLayout.Description>
        <SectionPageLayout.Content>
          <div className='mx-auto flex w-full max-w-7xl flex-col gap-4 sm:gap-5'>
            <SubscriptionPlansCard topupInfo={topupInfo} />

            <TimedSubscriptionConversionQuotesCard />
            <AffiliateRewardsCard
              affiliateLink={affiliate.affiliateLink}
              entitlement={affiliate.entitlement}
              loading={affiliate.loading}
            />
          </div>
        </SectionPageLayout.Content>
      </SectionPageLayout>
    </>
  )
}
