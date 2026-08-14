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
import { useCallback, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { SectionPageLayout } from '@/components/layout'
import { TimedSubscriptionConversionQuotesCard } from '@/features/subscription-conversion/components/timed-subscription-conversion-quotes-card'
import { subscriptionQueryKeys } from '@/features/subscriptions/query-keys'
import { AffiliateRewardsCard } from './components/affiliate-rewards-card'
import { RedemptionCodeCard } from './components/redemption-code-card'
import { RedemptionDialog } from './components/redemption-dialog'
import { SubscriptionPlansCard } from './components/subscription-plans-card'
import {
  submitInitialRedemption,
  useAffiliate,
  useRedemption,
  useTopupInfo,
} from './hooks'
import type { RedemptionRequest, RedemptionResult } from './types'

export function Wallet() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { topupInfo } = useTopupInfo()
  const affiliate = useAffiliate()
  const { redeeming, redeemCode } = useRedemption()
  const [redemptionCode, setRedemptionCode] = useState('')
  const [redemptionDialogOpen, setRedemptionDialogOpen] = useState(false)
  const [showSubscriptionPanel, setShowSubscriptionPanel] = useState(true)

  const refreshSubscriptionBenefits = useCallback(async () => {
    await Promise.all([
      queryClient.invalidateQueries({
        queryKey: subscriptionQueryKeys.selfSummary,
      }),
      queryClient.invalidateQueries({
        queryKey: subscriptionQueryKeys.selfConversionQuotes,
      }),
      queryClient.invalidateQueries({
        queryKey: subscriptionQueryKeys.dashboardSelfSubscriptions,
      }),
    ])
  }, [queryClient])

  const handleRedeem = async () => {
    if (redemptionCode.trim() === '') return

    await submitInitialRedemption({
      key: redemptionCode,
      redeem: redeemCode,
      onRedeemed: async () => {
        setRedemptionCode('')
        await refreshSubscriptionBenefits()
      },
      onModeRequired: () => setRedemptionDialogOpen(true),
      onError: (message) => toast.error(message),
      fallbackError: t('Redemption failed'),
    })
  }

  const handleRedemptionModeSubmit = async (
    request: RedemptionRequest
  ): Promise<RedemptionResult> => {
    const result = await redeemCode(request)
    await refreshSubscriptionBenefits()
    return result
  }

  return (
    <>
      <SectionPageLayout>
        <SectionPageLayout.Title>{t('Subscriptions')}</SectionPageLayout.Title>
        <SectionPageLayout.Description>
          {t('Subscription Plans')}
        </SectionPageLayout.Description>
        <SectionPageLayout.Content>
          <div className='mx-auto flex w-full max-w-7xl flex-col gap-4 sm:gap-5'>
            <div
              className={
                showSubscriptionPanel
                  ? 'grid gap-4 xl:grid-cols-[minmax(0,1.05fr)_minmax(360px,0.95fr)] xl:items-start'
                  : 'grid gap-4'
              }
            >
              <SubscriptionPlansCard
                topupInfo={topupInfo}
                onAvailabilityChange={setShowSubscriptionPanel}
              />

              <div className='flex min-w-0 flex-col gap-4'>
                <RedemptionCodeCard
                  code={redemptionCode}
                  redeeming={redeeming}
                  topupLink={topupInfo?.topup_link}
                  onCodeChange={setRedemptionCode}
                  onRedeem={handleRedeem}
                />
                <TimedSubscriptionConversionQuotesCard />
              </div>
            </div>

            <AffiliateRewardsCard
              affiliateLink={affiliate.affiliateLink}
              entitlement={affiliate.entitlement}
              loading={affiliate.loading}
            />
          </div>
        </SectionPageLayout.Content>
      </SectionPageLayout>

      <RedemptionDialog
        open={redemptionDialogOpen}
        code={redemptionCode}
        redeeming={redeeming}
        onOpenChange={(open) => {
          setRedemptionDialogOpen(open)
          if (!open) setRedemptionCode('')
        }}
        onSubmit={handleRedemptionModeSubmit}
      />
    </>
  )
}
