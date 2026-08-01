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
import { useState, type FormEvent, type ReactNode } from 'react'
import {
  useMutation,
  useQuery,
  useQueryClient,
  type QueryClient,
} from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { getCreditBalancePlan } from '../api'
import {
  creditBalancePlanToFormValues,
  submitCreditBalancePlanForm,
  type CreditBalancePlanFormValues,
} from '../lib/credit-balance-plan-form'
import type { SubscriptionPlan } from '../types'

interface EntrySwitchProps {
  id: string
  label: string
  description: string
  checked: boolean
  disabled?: boolean
  onCheckedChange: (checked: boolean) => void
}

function EntrySwitch({
  id,
  label,
  description,
  checked,
  disabled,
  onCheckedChange,
}: EntrySwitchProps) {
  return (
    <div className='flex items-start justify-between gap-4 rounded-lg border p-3'>
      <div className='space-y-1'>
        <Label id={`${id}-label`} htmlFor={id}>
          {label}
        </Label>
        <p className='text-muted-foreground text-xs'>{description}</p>
      </div>
      <Switch
        id={id}
        aria-labelledby={`${id}-label`}
        checked={checked}
        disabled={disabled}
        onCheckedChange={onCheckedChange}
      />
    </div>
  )
}

export function CreditBalancePlanCard() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const queryKey = ['admin-credit-balance-plan'] as const
  const planQuery = useQuery({
    queryKey,
    queryFn: getCreditBalancePlan,
  })
  const plan = planQuery.data?.data

  if (planQuery.isLoading) {
    return <CreditBalancePlanFrame>{t('Loading...')}</CreditBalancePlanFrame>
  }
  if (planQuery.isError || !plan) {
    return (
      <CreditBalancePlanFrame error>
        {t('Request failed')}
      </CreditBalancePlanFrame>
    )
  }

  const editorRevision = JSON.stringify([
    plan.id,
    plan.concurrency_limit,
    plan.queue_capacity,
    plan.business_code,
    plan.credit_balance_configured,
    plan.credit_balance_purchase_enabled,
    plan.credit_balance_redemption_enabled,
    plan.credit_balance_conversion_enabled,
  ])

  return (
    <CreditBalancePlanEditor
      key={editorRevision}
      plan={plan}
      queryKey={queryKey}
      queryClient={queryClient}
    />
  )
}

function CreditBalancePlanFrame({
  children,
  error = false,
}: {
  children: ReactNode
  error?: boolean
}) {
  const { t } = useTranslation()
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('Credit balance plan')}</CardTitle>
        <CardDescription>
          {t(
            'Configure the single non-expiring Credit entitlement. It is not a priced timed plan.'
          )}
        </CardDescription>
      </CardHeader>
      <CardContent>
        <p
          className={
            error ? 'text-destructive text-sm' : 'text-muted-foreground text-sm'
          }
        >
          {children}
        </p>
      </CardContent>
    </Card>
  )
}

function CreditBalancePlanEditor({
  plan,
  queryKey,
  queryClient,
}: {
  plan: SubscriptionPlan
  queryKey: readonly ['admin-credit-balance-plan']
  queryClient: QueryClient
}) {
  const { t } = useTranslation()
  const [values, setValues] = useState<CreditBalancePlanFormValues>(() =>
    creditBalancePlanToFormValues(plan)
  )

  const updateMutation = useMutation({
    mutationFn: (nextValues: CreditBalancePlanFormValues) =>
      submitCreditBalancePlanForm(nextValues),
    onSuccess: (response) => {
      if (!response.success || !response.data) {
        toast.error(response.message || t('Request failed'))
        return
      }
      queryClient.setQueryData(queryKey, response)
      setValues(creditBalancePlanToFormValues(response.data))
      toast.success(t('Credit balance plan saved'))
    },
    onError: () => toast.error(t('Request failed')),
  })

  const setField = <K extends keyof CreditBalancePlanFormValues>(
    key: K,
    value: CreditBalancePlanFormValues[K]
  ) => setValues((current) => ({ ...current, [key]: value }))

  const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    updateMutation.mutate(values)
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('Credit balance plan')}</CardTitle>
        <CardDescription>
          {t(
            'Configure the single non-expiring Credit entitlement. It is not a priced timed plan.'
          )}
        </CardDescription>
      </CardHeader>
      <CardContent className='flex flex-col gap-4'>
        <Alert>
          <AlertTitle>{t('Keep balances distinct')}</AlertTitle>
          <AlertDescription>
            {t('Renminbi account balance')} · {t('Credit balance plan')} ·{' '}
            {t('Timed plans')}
          </AlertDescription>
        </Alert>

        <form className='flex flex-col gap-4' onSubmit={handleSubmit}>
          <div className='grid gap-4 md:grid-cols-2'>
            <div className='space-y-2'>
              <Label htmlFor='credit-balance-concurrency'>
                {t('Concurrency Limit')}
              </Label>
              <Input
                id='credit-balance-concurrency'
                type='number'
                min={0}
                value={values.concurrency_limit}
                onChange={(event) =>
                  setField('concurrency_limit', Number(event.target.value) || 0)
                }
              />
            </div>

            <div className='space-y-2'>
              <Label htmlFor='credit-balance-queue-capacity'>
                {t('Queue Capacity')}
              </Label>
              <Input
                id='credit-balance-queue-capacity'
                type='number'
                min={0}
                value={values.queue_capacity}
                onChange={(event) =>
                  setField('queue_capacity', Number(event.target.value) || 0)
                }
              />
            </div>

            <div className='space-y-2 md:col-span-2'>
              <Label htmlFor='credit-balance-business-code'>
                {t('Business Code')}
              </Label>
              <Input
                id='credit-balance-business-code'
                value={values.business_code}
                placeholder='credit_balance_global'
                onChange={(event) =>
                  setField('business_code', event.target.value)
                }
              />
            </div>
          </div>

          <EntrySwitch
            id='credit-balance-configured'
            label={t('Configuration confirmed')}
            description={t(
              'Confirm only after concurrency, queue capacity, and BusinessCode are reviewed.'
            )}
            checked={values.configured}
            onCheckedChange={(checked) =>
              setValues((current) => ({
                ...current,
                configured: checked,
                purchase_enabled: checked && current.purchase_enabled,
                redemption_enabled: checked && current.redemption_enabled,
                conversion_enabled: checked && current.conversion_enabled,
              }))
            }
          />

          <div className='grid gap-3 md:grid-cols-3'>
            <EntrySwitch
              id='credit-balance-purchase-enabled'
              label={t('New Credit balance purchases')}
              description={t('Controls only new Credit balance purchases.')}
              checked={values.purchase_enabled}
              disabled={!values.configured}
              onCheckedChange={(checked) =>
                setField('purchase_enabled', checked)
              }
            />
            <EntrySwitch
              id='credit-balance-redemption-enabled'
              label={t('New Credit balance redemptions')}
              description={t('Controls only new Credit balance redemptions.')}
              checked={values.redemption_enabled}
              disabled={!values.configured}
              onCheckedChange={(checked) =>
                setField('redemption_enabled', checked)
              }
            />
            <EntrySwitch
              id='credit-balance-conversion-enabled'
              label={t('New timed plan conversions')}
              description={t(
                'Controls only new timed entitlement conversions.'
              )}
              checked={values.conversion_enabled}
              disabled={!values.configured}
              onCheckedChange={(checked) =>
                setField('conversion_enabled', checked)
              }
            />
          </div>

          <div className='flex justify-end'>
            <Button type='submit' disabled={updateMutation.isPending}>
              {updateMutation.isPending
                ? t('Saving...')
                : t('Save configuration')}
            </Button>
          </div>
        </form>
      </CardContent>
    </Card>
  )
}
