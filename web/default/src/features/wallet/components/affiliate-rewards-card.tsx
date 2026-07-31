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
import { useState } from 'react'
import { CalendarCheck, Share2, WalletCards } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { CopyButton } from '@/components/copy-button'
import { formatAccountBalanceForPlanPurchase } from '@/features/subscriptions/lib'
import {
  useInvitationCommissionRecords,
  useInvitationCommissionSummary,
  useInvitationCommissionWithdrawals,
  useRequestInvitationCommissionWithdrawal,
  useTransferInvitationCommission,
} from '../hooks/use-invitation-commission'
import { formatAffiliateEntitlementEndTime } from '../lib'
import type {
  InvitationCommissionRecord,
  InvitationCommissionSummary,
  InvitationCommissionWithdrawal,
  InvitationCommissionWithdrawalPayload,
  InvitationEntitlement,
} from '../types'
import { CommissionTransferDialog } from './dialogs/commission-transfer-dialog'
import { CommissionWithdrawalDialog } from './dialogs/commission-withdrawal-dialog'

interface AffiliateRewardsCardProps {
  affiliateLink: string
  entitlement?: InvitationEntitlement | null
  commissionSummary?: InvitationCommissionSummary | null
  loading?: boolean
  onCommissionTransferSuccess?: () => Promise<void> | void
}

function formatRateBps(rateBps: number): string {
  return `${(rateBps / 100).toFixed(2)}%`
}

function formatCommissionTime(seconds: number): string {
  if (!seconds) return '-'
  return new Date(seconds * 1000).toLocaleDateString()
}

export function commissionRecordStatusLabel(
  status: InvitationCommissionRecord['status']
): string {
  if (status === 'available') return 'Available'
  if (status === 'skipped') return 'Skipped'
  if (status === 'unrecoverable') return 'Unrecoverable commission'
  return 'Cancelled'
}

function commissionWithdrawalStatusLabel(
  status: InvitationCommissionWithdrawal['status']
): string {
  if (status === 'pending') return 'Pending'
  if (status === 'completed') return 'Completed'
  return 'Rejected'
}

export function AffiliateRewardsCard(props: AffiliateRewardsCardProps) {
  const { t } = useTranslation()
  const [commissionTransferDialogOpen, setCommissionTransferDialogOpen] =
    useState(false)
  const [commissionWithdrawalDialogOpen, setCommissionWithdrawalDialogOpen] =
    useState(false)
  const commissionSummaryQuery = useInvitationCommissionSummary()
  const commissionRecordsQuery = useInvitationCommissionRecords({
    page: 1,
    page_size: 3,
  })
  const commissionWithdrawalsQuery = useInvitationCommissionWithdrawals({
    page: 1,
    page_size: 3,
  })
  const transferCommissionMutation = useTransferInvitationCommission()
  const requestCommissionWithdrawalMutation =
    useRequestInvitationCommissionWithdrawal()

  if (props.loading) {
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
  const referralShareText = `${referralCopy}${props.affiliateLink}`
  const currentRewardTitle = props.entitlement?.entitled
    ? props.entitlement.reward_plan_title || t('Granted')
    : t('Not granted')
  const hasDowngradeReward =
    (props.entitlement?.downgrade_reward_plan_id ?? 0) > 0 &&
    (props.entitlement?.downgrade_entitlement_end_time ?? 0) >
      (props.entitlement?.entitlement_end_time ?? 0)
  const commissionSummary =
    props.commissionSummary ?? commissionSummaryQuery.data
  const showCommissionRewards = Boolean(
    commissionSummary &&
    (commissionSummary.reward_mode === 'commission' ||
      commissionSummary.has_commission_account)
  )
  const directInviteCount =
    commissionSummary?.direct_invite_count ??
    props.entitlement?.direct_invite_count ??
    0
  const qualifiedPaidInviteCount =
    commissionSummary?.qualified_paid_invite_count ??
    props.entitlement?.qualified_active_count ??
    0
  const showHistoricalCommissionNotice = Boolean(
    commissionSummary?.reward_mode === 'subscription' &&
    commissionSummary.has_commission_account
  )
  const recentCommissionRecords = commissionRecordsQuery.data?.items ?? []
  const recentCommissionWithdrawals =
    commissionWithdrawalsQuery.data?.items ?? []

  const handleCommissionTransfer = async (amountCents: number) => {
    try {
      await transferCommissionMutation.mutateAsync(amountCents)
      await props.onCommissionTransferSuccess?.()
      return true
    } catch {
      return false
    }
  }

  const handleCommissionWithdrawal = async (
    payload: InvitationCommissionWithdrawalPayload
  ) => {
    try {
      await requestCommissionWithdrawalMutation.mutateAsync(payload)
      return true
    } catch {
      return false
    }
  }

  return (
    <>
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
                {showCommissionRewards
                  ? t(
                      'Earn cashback when your referrals add funds. Transfer available commission to your balance or request manual cashback.'
                    )
                  : t(
                      'Earn rewards when your referrals add funds. Transfer accumulated rewards to your balance anytime.'
                    )}
              </p>
            </div>
          </div>

          {showHistoricalCommissionNotice && (
            <div className='bg-background/60 rounded-lg border p-3 text-xs lg:col-span-3'>
              {t(
                'New paid invitations will receive reward packages. Historical commission balance can still be handled.'
              )}
            </div>
          )}

          <div className='grid grid-cols-2 gap-2 text-xs lg:col-span-3 lg:grid-cols-6'>
            <div className='bg-background/60 rounded-lg border p-2'>
              <div className='text-muted-foreground'>{t('Direct invites')}</div>
              <div className='mt-1 font-semibold tabular-nums'>
                {directInviteCount}
              </div>
            </div>
            <div className='bg-background/60 rounded-lg border p-2'>
              <div className='text-muted-foreground'>
                {t('Qualified paid invites')}
              </div>
              <div className='mt-1 font-semibold tabular-nums'>
                {qualifiedPaidInviteCount}
              </div>
            </div>

            {showCommissionRewards && commissionSummary ? (
              <>
                <div className='bg-background/60 rounded-lg border p-2'>
                  <div className='text-muted-foreground'>
                    {t('Available commission balance')}
                  </div>
                  <div className='mt-1 font-semibold tabular-nums'>
                    {formatAccountBalanceForPlanPurchase(
                      commissionSummary.account.available_cents
                    )}
                  </div>
                </div>
                <div className='bg-background/60 rounded-lg border p-2'>
                  <div className='text-muted-foreground'>
                    {t('Pending cashback amount')}
                  </div>
                  <div className='mt-1 font-semibold tabular-nums'>
                    {formatAccountBalanceForPlanPurchase(
                      commissionSummary.account.pending_cents
                    )}
                  </div>
                </div>
                <div className='bg-background/60 rounded-lg border p-2'>
                  <div className='text-muted-foreground'>
                    {t('Commission rate')}
                  </div>
                  <div className='mt-1 font-semibold tabular-nums'>
                    {formatRateBps(commissionSummary.setting.rate_bps)}
                  </div>
                </div>
                <div className='bg-background/60 rounded-lg border p-2'>
                  <div className='text-muted-foreground'>{t('Mode')}</div>
                  <div className='mt-1'>
                    <Badge variant='secondary'>
                      {t(
                        commissionSummary.reward_mode === 'commission'
                          ? 'Commission'
                          : 'Reward package'
                      )}
                    </Badge>
                  </div>
                </div>
              </>
            ) : (
              <>
                <div className='bg-background/60 rounded-lg border p-2'>
                  <div className='text-muted-foreground'>
                    {t('Monthly reward')}
                  </div>
                  <div className='mt-1 flex items-center gap-1 font-semibold'>
                    <CalendarCheck className='size-3.5' />
                    {currentRewardTitle}
                  </div>
                </div>
                <div className='bg-background/60 rounded-lg border p-2'>
                  <div className='text-muted-foreground'>
                    {t('Reward month')}
                  </div>
                  <div className='mt-1 font-semibold tabular-nums'>
                    {props.entitlement?.reward_month || '-'}
                  </div>
                </div>
                <div className='bg-background/60 rounded-lg border p-2'>
                  <div className='text-muted-foreground'>
                    {t('Reward valid until')}
                  </div>
                  <div className='mt-1 font-semibold tabular-nums'>
                    {formatAffiliateEntitlementEndTime(
                      props.entitlement?.entitlement_end_time ?? 0
                    )}
                  </div>
                </div>
                {hasDowngradeReward && (
                  <div className='bg-background/60 rounded-lg border p-2'>
                    <div className='text-muted-foreground'>
                      {t('Downgrades to')}
                    </div>
                    <div className='mt-1 font-semibold'>
                      {props.entitlement?.downgrade_reward_plan_title}
                    </div>
                    <div className='text-muted-foreground mt-0.5 tabular-nums'>
                      {formatAffiliateEntitlementEndTime(
                        props.entitlement?.downgrade_entitlement_end_time ?? 0
                      )}
                    </div>
                  </div>
                )}
              </>
            )}
          </div>

          <div className='flex flex-col gap-2 lg:col-span-3'>
            <div className='flex items-center gap-2'>
              <Input
                value={referralShareText}
                readOnly
                className='border-muted bg-background/70 h-9 min-w-0 flex-1 font-mono text-xs'
              />
              <CopyButton
                value={referralShareText}
                variant='outline'
                className='bg-background size-9 shrink-0'
                iconClassName='size-4'
                tooltip={t('Copy referral link')}
                aria-label={t('Copy referral link')}
              />
            </div>
          </div>

          {showCommissionRewards && commissionSummary ? (
            <div className='bg-background/60 space-y-3 rounded-lg border p-3 text-xs lg:col-span-3'>
              <div className='flex flex-wrap items-center justify-between gap-2'>
                <div>
                  <h4 className='text-foreground font-semibold'>
                    {t('Commission earnings')}
                  </h4>
                  <p className='text-muted-foreground mt-1'>
                    {t('This is not an automatic payout.')}
                  </p>
                </div>
                <div className='flex flex-wrap gap-2'>
                  <Button
                    type='button'
                    size='sm'
                    disabled={!commissionSummary.can_transfer}
                    onClick={() => setCommissionTransferDialogOpen(true)}
                  >
                    <WalletCards className='size-3.5' />
                    {t('Transfer to balance')}
                  </Button>
                  <Button
                    type='button'
                    size='sm'
                    variant='outline'
                    disabled={!commissionSummary.can_request_withdrawal}
                    onClick={() => setCommissionWithdrawalDialogOpen(true)}
                  >
                    {t('Request manual cashback')}
                  </Button>
                </div>
              </div>
              <div className='text-muted-foreground grid gap-1 sm:grid-cols-2'>
                <div>
                  {t('Minimum transfer:')}{' '}
                  {formatAccountBalanceForPlanPurchase(
                    commissionSummary.setting.minimum_transfer_cents
                  )}
                </div>
                <div>
                  {t('Minimum manual cashback:')}{' '}
                  {formatAccountBalanceForPlanPurchase(
                    commissionSummary.setting.minimum_withdraw_cents
                  )}
                </div>
                <div>
                  {t('Transferred:')}{' '}
                  {formatAccountBalanceForPlanPurchase(
                    commissionSummary.account.transferred_cents
                  )}
                </div>
                <div>
                  {t('Manual cashback paid:')}{' '}
                  {formatAccountBalanceForPlanPurchase(
                    commissionSummary.account.withdrawn_cents
                  )}
                </div>
              </div>
              <div className='grid gap-3 sm:grid-cols-2'>
                <div className='space-y-2'>
                  <div className='text-foreground font-medium'>
                    {t('Recent commission records')}
                  </div>
                  <div className='space-y-1'>
                    {recentCommissionRecords.length > 0 ? (
                      recentCommissionRecords.map((record) => (
                        <div
                          key={record.id}
                          className='bg-background/70 flex items-center justify-between gap-2 rounded border px-2 py-1.5'
                        >
                          <div className='min-w-0'>
                            <div className='truncate font-medium'>
                              {formatAccountBalanceForPlanPurchase(
                                record.commission_cents
                              )}
                            </div>
                            <div className='text-muted-foreground truncate'>
                              {record.source_type} ·{' '}
                              {formatCommissionTime(record.created_at)}
                            </div>
                          </div>
                          <Badge variant='secondary'>
                            {t(commissionRecordStatusLabel(record.status))}
                          </Badge>
                        </div>
                      ))
                    ) : (
                      <div className='text-muted-foreground'>
                        {t('No commission records yet')}
                      </div>
                    )}
                  </div>
                </div>
                <div className='space-y-2'>
                  <div className='text-foreground font-medium'>
                    {t('Recent manual cashback requests')}
                  </div>
                  <div className='space-y-1'>
                    {recentCommissionWithdrawals.length > 0 ? (
                      recentCommissionWithdrawals.map((withdrawal) => (
                        <div
                          key={withdrawal.id}
                          className='bg-background/70 flex items-center justify-between gap-2 rounded border px-2 py-1.5'
                        >
                          <div className='min-w-0'>
                            <div className='truncate font-medium'>
                              {formatAccountBalanceForPlanPurchase(
                                withdrawal.amount_cents
                              )}
                            </div>
                            <div className='text-muted-foreground truncate'>
                              {withdrawal.contact.type} ·{' '}
                              {withdrawal.contact.value} ·{' '}
                              {formatCommissionTime(withdrawal.created_at)}
                            </div>
                          </div>
                          <Badge variant='outline'>
                            {t(
                              commissionWithdrawalStatusLabel(withdrawal.status)
                            )}
                          </Badge>
                        </div>
                      ))
                    ) : (
                      <div className='text-muted-foreground'>
                        {t('No manual cashback requests yet')}
                      </div>
                    )}
                  </div>
                </div>
              </div>
            </div>
          ) : (
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
                    'Credit reset consumes one month from a paid plan and cannot be paid by invitation rewards.'
                  )}
                </li>
              </ul>
            </div>
          )}
        </CardContent>
      </Card>

      <CommissionTransferDialog
        open={commissionTransferDialogOpen}
        onOpenChange={setCommissionTransferDialogOpen}
        availableCents={commissionSummary?.account.available_cents ?? 0}
        minimumTransferCents={
          commissionSummary?.setting.minimum_transfer_cents ?? 0
        }
        transferring={transferCommissionMutation.isPending}
        onConfirm={handleCommissionTransfer}
      />
      <CommissionWithdrawalDialog
        open={commissionWithdrawalDialogOpen}
        onOpenChange={setCommissionWithdrawalDialogOpen}
        availableCents={commissionSummary?.account.available_cents ?? 0}
        minimumWithdrawalCents={
          commissionSummary?.setting.minimum_withdraw_cents ?? 0
        }
        submitting={requestCommissionWithdrawalMutation.isPending}
        onConfirm={handleCommissionWithdrawal}
      />
    </>
  )
}
