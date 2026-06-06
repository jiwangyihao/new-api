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
import { useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Loader2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { handleServerError } from '@/lib/handle-server-error'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Textarea } from '@/components/ui/textarea'
import { CopyButton } from '@/components/copy-button'
import { SectionPageLayout } from '@/components/layout'
import { formatAccountBalanceForPlanPurchase } from '@/features/subscriptions/lib'
import {
  completeInvitationCommissionWithdrawal,
  listAdminInvitationCommissionWithdrawals,
  rejectInvitationCommissionWithdrawal,
} from './api'
import type {
  AdminInvitationCommissionWithdrawal,
  AdminInvitationCommissionWithdrawalParams,
  AdminInvitationCommissionWithdrawalStatus,
} from './types'

const DEFAULT_PAGE_SIZE = 20

function buildWithdrawalQueryKey(
  params: AdminInvitationCommissionWithdrawalParams
) {
  return ['admin', 'invitation-commission', 'withdrawals', params] as const
}

function formatUnixSeconds(seconds: number): string {
  if (!seconds) return '-'
  return new Date(seconds * 1000).toLocaleString()
}

function getStatusVariant(
  status: AdminInvitationCommissionWithdrawalStatus
): 'default' | 'secondary' | 'destructive' | 'outline' {
  if (status === 'pending') return 'default'
  if (status === 'completed') return 'secondary'
  if (status === 'rejected') return 'destructive'
  return 'outline'
}

function adminWithdrawalStatusLabel(
  status: AdminInvitationCommissionWithdrawalStatus
): string {
  if (status === 'pending') return 'Pending'
  if (status === 'completed') return 'Completed'
  return 'Rejected'
}

export function AdminInvitationCommissionWithdrawals() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [status, setStatus] =
    useState<AdminInvitationCommissionWithdrawalStatus>('pending')
  const [userIdDraft, setUserIdDraft] = useState('')
  const [userId, setUserId] = useState<number | undefined>()
  const [page, setPage] = useState(1)
  const [actionTarget, setActionTarget] =
    useState<AdminInvitationCommissionWithdrawal | null>(null)
  const [actionType, setActionType] = useState<'complete' | 'reject' | null>(
    null
  )
  const [adminRemark, setAdminRemark] = useState('')

  const params = useMemo<AdminInvitationCommissionWithdrawalParams>(
    () => ({
      page,
      page_size: DEFAULT_PAGE_SIZE,
      status,
      user_id: userId,
    }),
    [page, status, userId]
  )

  const withdrawalsQuery = useQuery({
    queryKey: buildWithdrawalQueryKey(params),
    queryFn: () => listAdminInvitationCommissionWithdrawals(params),
  })

  const completeMutation = useMutation({
    mutationFn: (payload: { id: number; admin_remark: string }) =>
      completeInvitationCommissionWithdrawal(payload.id, payload.admin_remark),
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({
          queryKey: buildWithdrawalQueryKey(params),
        }),
        queryClient.invalidateQueries({
          queryKey: ['admin', 'tasks', 'summary'],
        }),
      ])
      toast.success(t('Manual cashback request updated'))
      closeActionDialog()
    },
    onError: handleServerError,
  })

  const rejectMutation = useMutation({
    mutationFn: (payload: { id: number; admin_remark: string }) =>
      rejectInvitationCommissionWithdrawal(payload.id, payload.admin_remark),
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({
          queryKey: buildWithdrawalQueryKey(params),
        }),
        queryClient.invalidateQueries({
          queryKey: ['admin', 'tasks', 'summary'],
        }),
      ])
      toast.success(t('Manual cashback request updated'))
      closeActionDialog()
    },
    onError: handleServerError,
  })

  const withdrawals = withdrawalsQuery.data?.items ?? []
  const total = withdrawalsQuery.data?.total ?? 0
  const hasPreviousPage = page > 1
  const hasNextPage = page * DEFAULT_PAGE_SIZE < total

  function applyUserFilter() {
    const parsed = Number(userIdDraft)
    setUserId(Number.isInteger(parsed) && parsed > 0 ? parsed : undefined)
    setPage(1)
  }

  function updateStatus(next: AdminInvitationCommissionWithdrawalStatus) {
    setStatus(next)
    setPage(1)
  }

  function openActionDialog(
    withdrawal: AdminInvitationCommissionWithdrawal,
    type: 'complete' | 'reject'
  ) {
    setActionTarget(withdrawal)
    setActionType(type)
    setAdminRemark('')
  }

  function closeActionDialog() {
    setActionTarget(null)
    setActionType(null)
    setAdminRemark('')
  }

  function submitAction() {
    if (!actionTarget || !actionType) return
    const trimmedRemark = adminRemark.trim()
    if (!trimmedRemark) {
      toast.error(t('Admin remark is required'))
      return
    }

    if (actionType === 'complete') {
      completeMutation.mutate({
        id: actionTarget.id,
        admin_remark: trimmedRemark,
      })
      return
    }

    rejectMutation.mutate({
      id: actionTarget.id,
      admin_remark: trimmedRemark,
    })
  }

  const actionPending = completeMutation.isPending || rejectMutation.isPending

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>
        {t('Manual cashback requests')}
      </SectionPageLayout.Title>
      <SectionPageLayout.Description>
        {t(
          'Review manual cashback requests and coordinate offline payout with users.'
        )}
      </SectionPageLayout.Description>
      <SectionPageLayout.Content>
        <Card>
          <CardHeader>
            <CardTitle>{t('Manual cashback requests')}</CardTitle>
            <CardDescription>
              {t('Pending cashback requests')}: {total}
            </CardDescription>
          </CardHeader>
          <CardContent className='space-y-4'>
            <div className='flex flex-wrap items-end gap-3'>
              <div className='space-y-1'>
                <Label>{t('Cashback status')}</Label>
                <NativeSelect
                  value={status}
                  onChange={(event) =>
                    updateStatus(
                      event.target
                        .value as AdminInvitationCommissionWithdrawalStatus
                    )
                  }
                >
                  <NativeSelectOption value='pending'>
                    {t('Pending')}
                  </NativeSelectOption>
                  <NativeSelectOption value='completed'>
                    {t('Completed')}
                  </NativeSelectOption>
                  <NativeSelectOption value='rejected'>
                    {t('Rejected')}
                  </NativeSelectOption>
                </NativeSelect>
              </div>
              <div className='space-y-1'>
                <Label>{t('User ID')}</Label>
                <Input
                  value={userIdDraft}
                  onChange={(event) => setUserIdDraft(event.target.value)}
                  inputMode='numeric'
                  placeholder='123'
                />
              </div>
              <Button onClick={applyUserFilter}>{t('Search')}</Button>
            </div>

            <div className='overflow-x-auto'>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t('User ID')}</TableHead>
                    <TableHead>{t('Username')}</TableHead>
                    <TableHead>{t('Display Name')}</TableHead>
                    <TableHead>{t('Amount')}</TableHead>
                    <TableHead>{t('Cashback status')}</TableHead>
                    <TableHead>{t('Manual cashback contact')}</TableHead>
                    <TableHead>{t('User remark')}</TableHead>
                    <TableHead>{t('Admin remark')}</TableHead>
                    <TableHead>{t('Created At')}</TableHead>
                    <TableHead>{t('Completed At')}</TableHead>
                    <TableHead>{t('Actions')}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {withdrawals.map((withdrawal) => (
                    <TableRow key={withdrawal.id}>
                      <TableCell>{withdrawal.user_id}</TableCell>
                      <TableCell>{withdrawal.username || '-'}</TableCell>
                      <TableCell>{withdrawal.display_name || '-'}</TableCell>
                      <TableCell>
                        {formatAccountBalanceForPlanPurchase(
                          withdrawal.amount_cents
                        )}
                      </TableCell>
                      <TableCell>
                        <Badge variant={getStatusVariant(withdrawal.status)}>
                          {t(adminWithdrawalStatusLabel(withdrawal.status))}
                        </Badge>
                      </TableCell>
                      <TableCell>
                        <div className='flex min-w-48 items-center gap-2'>
                          <span className='text-muted-foreground text-xs uppercase'>
                            {withdrawal.contact.type}
                          </span>
                          <span className='truncate'>
                            {withdrawal.contact.value || '-'}
                          </span>
                          {withdrawal.contact.value && (
                            <CopyButton
                              value={withdrawal.contact.value}
                              tooltip={t('Copy contact')}
                              successTooltip={t('Copied!')}
                            />
                          )}
                        </div>
                      </TableCell>
                      <TableCell className='max-w-60 truncate'>
                        {withdrawal.user_remark || '-'}
                      </TableCell>
                      <TableCell className='max-w-60 truncate'>
                        {withdrawal.admin_remark || '-'}
                      </TableCell>
                      <TableCell>
                        {formatUnixSeconds(withdrawal.created_at)}
                      </TableCell>
                      <TableCell>
                        {formatUnixSeconds(
                          withdrawal.completed_at || withdrawal.reviewed_at
                        )}
                      </TableCell>
                      <TableCell>
                        {withdrawal.status === 'pending' ? (
                          <div className='flex gap-2'>
                            <Button
                              size='sm'
                              onClick={() =>
                                openActionDialog(withdrawal, 'complete')
                              }
                            >
                              {t('Mark manual cashback as completed')}
                            </Button>
                            <Button
                              size='sm'
                              variant='destructive'
                              onClick={() =>
                                openActionDialog(withdrawal, 'reject')
                              }
                            >
                              {t('Reject')}
                            </Button>
                          </div>
                        ) : (
                          '-'
                        )}
                      </TableCell>
                    </TableRow>
                  ))}
                  {withdrawals.length === 0 && (
                    <TableRow>
                      <TableCell colSpan={11} className='text-center'>
                        {withdrawalsQuery.isLoading
                          ? t('Loading...')
                          : t('No data')}
                      </TableCell>
                    </TableRow>
                  )}
                </TableBody>
              </Table>
            </div>

            <div className='flex items-center justify-between gap-3'>
              <div className='text-muted-foreground text-sm'>
                {t('{{count}} records', { count: total })}
              </div>
              <div className='flex gap-2'>
                <Button
                  variant='outline'
                  disabled={!hasPreviousPage}
                  onClick={() => setPage((value) => Math.max(1, value - 1))}
                >
                  {t('Previous')}
                </Button>
                <Button
                  variant='outline'
                  disabled={!hasNextPage}
                  onClick={() => setPage((value) => value + 1)}
                >
                  {t('Next')}
                </Button>
              </div>
            </div>
          </CardContent>
        </Card>
      </SectionPageLayout.Content>

      <Dialog open={Boolean(actionTarget)} onOpenChange={closeActionDialog}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>
              {actionType === 'complete'
                ? t('Mark manual cashback as completed')
                : t('Reject manual cashback request')}
            </DialogTitle>
            <DialogDescription>
              {t('Admin remark is required for audit records.')}
            </DialogDescription>
          </DialogHeader>
          <div className='space-y-2'>
            <Label>{t('Admin remark')}</Label>
            <Textarea
              value={adminRemark}
              maxLength={500}
              onChange={(event) => setAdminRemark(event.target.value)}
            />
          </div>
          <DialogFooter>
            <Button variant='outline' onClick={closeActionDialog}>
              {t('Cancel')}
            </Button>
            <Button onClick={submitAction} disabled={actionPending}>
              {actionPending && <Loader2 className='animate-spin' />}
              {actionType === 'complete'
                ? t('Mark manual cashback as completed')
                : t('Reject')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </SectionPageLayout>
  )
}
