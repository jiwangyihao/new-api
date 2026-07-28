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
import { z } from 'zod'
import type { TFunction } from 'i18next'
import type { SubscriptionPlan, PlanPayload } from '../types'

export function getPlanFormSchema(t: TFunction) {
  return z.object({
    title: z.string().min(1, t('Please enter plan title')),
    subtitle: z.string().optional(),
    price_amount: z.coerce.number().min(0, t('Please enter amount')),
    duration_unit: z.enum(['year', 'month', 'day', 'hour', 'custom']),
    duration_value: z.coerce.number().min(1),
    custom_seconds: z.coerce.number().min(0).optional(),
    quota_reset_period: z.enum([
      'never',
      'daily',
      'weekly',
      'monthly',
      'custom',
    ]),
    quota_reset_custom_seconds: z.coerce.number().min(0).optional(),
    enabled: z.boolean(),
    sort_order: z.coerce.number(),
    max_purchase_per_user: z.coerce.number().min(0),
    total_amount: z.coerce.number().min(0),
    stripe_price_id: z.string().optional(),
    creem_product_id: z.string().optional(),
    kyren_product_id: z.string().optional(),
    monthly_token_limit: z.coerce.number().min(0),
    concurrency_limit: z.coerce.number().min(0),
    queue_capacity: z.coerce.number().min(0),
    gpt_abuse_warning_limit: z.coerce.number().min(0),
    is_trial: z.boolean(),
    invite_trial: z.boolean(),
    public_visible: z.boolean(),
    trial_duration_hours: z.coerce.number().min(0),
    reward_eligible: z.boolean(),
    business_code: z.string().optional(),
    unlimited_purchase_enabled: z.boolean(),
    timed_conversion_enabled: z.boolean(),
  })
}

export type PlanFormValues = z.infer<ReturnType<typeof getPlanFormSchema>>

export const PLAN_FORM_DEFAULTS: PlanFormValues = {
  title: '',
  subtitle: '',
  price_amount: 0,
  duration_unit: 'month',
  duration_value: 1,
  custom_seconds: 0,
  quota_reset_period: 'never',
  quota_reset_custom_seconds: 0,
  enabled: true,
  sort_order: 0,
  max_purchase_per_user: 0,
  total_amount: 0,
  stripe_price_id: '',
  creem_product_id: '',
  kyren_product_id: '',
  monthly_token_limit: 0,
  concurrency_limit: 0,
  queue_capacity: 0,
  gpt_abuse_warning_limit: 0,
  is_trial: false,
  invite_trial: false,
  public_visible: true,
  trial_duration_hours: 0,
  reward_eligible: true,
  business_code: '',
  unlimited_purchase_enabled: false,
  timed_conversion_enabled: false,
}

export function planToFormValues(plan: SubscriptionPlan): PlanFormValues {
  return {
    title: plan.title || '',
    subtitle: plan.subtitle || '',
    price_amount: Number(plan.price_amount || 0),
    duration_unit: plan.duration_unit || 'month',
    duration_value: Number(plan.duration_value || 1),
    custom_seconds: Number(plan.custom_seconds || 0),
    quota_reset_period: plan.quota_reset_period || 'never',
    quota_reset_custom_seconds: Number(plan.quota_reset_custom_seconds || 0),
    enabled: plan.enabled !== false,
    sort_order: Number(plan.sort_order || 0),
    max_purchase_per_user: Number(plan.max_purchase_per_user || 0),
    total_amount: Number(plan.total_amount || 0),
    stripe_price_id: plan.stripe_price_id || '',
    creem_product_id: plan.creem_product_id || '',
    kyren_product_id: plan.kyren_product_id || '',
    monthly_token_limit: Number(plan.monthly_token_limit || 0),
    concurrency_limit: Number(plan.concurrency_limit || 0),
    queue_capacity: Number(plan.queue_capacity || 0),
    gpt_abuse_warning_limit: Number(plan.gpt_abuse_warning_limit || 0),
    is_trial: plan.is_trial === true,
    invite_trial: plan.invite_trial === true,
    public_visible: plan.public_visible !== false,
    trial_duration_hours: Number(plan.trial_duration_hours || 0),
    reward_eligible: plan.reward_eligible !== false,
    business_code: plan.business_code || '',
    unlimited_purchase_enabled: plan.unlimited_purchase_enabled === true,
    timed_conversion_enabled: plan.timed_conversion_enabled === true,
  }
}

export function formValuesToPlanPayload(values: PlanFormValues): PlanPayload {
  return {
    plan: {
      ...values,
      price_amount: Number(values.price_amount || 0),
      currency: 'CNY',
      duration_value: Number(values.duration_value || 0),
      custom_seconds: Number(values.custom_seconds || 0),
      quota_reset_period: values.quota_reset_period || 'never',
      quota_reset_custom_seconds:
        values.quota_reset_period === 'custom'
          ? Number(values.quota_reset_custom_seconds || 0)
          : 0,
      sort_order: Number(values.sort_order || 0),
      max_purchase_per_user: Number(values.max_purchase_per_user || 0),
      total_amount: Number(values.total_amount || 0),
      kyren_product_id: values.kyren_product_id?.trim() || undefined,
      monthly_token_limit: Number(values.monthly_token_limit || 0),
      concurrency_limit: Number(values.concurrency_limit || 0),
      queue_capacity: Number(values.queue_capacity || 0),
      gpt_abuse_warning_limit: Number(values.gpt_abuse_warning_limit || 0),
      is_trial: values.is_trial,
      invite_trial: values.invite_trial,
      public_visible: values.public_visible,
      trial_duration_hours: Number(values.trial_duration_hours || 0),
      reward_eligible: values.reward_eligible,
      business_code: values.business_code?.trim() || undefined,
      unlimited_purchase_enabled: values.unlimited_purchase_enabled,
      timed_conversion_enabled: values.timed_conversion_enabled,
    },
  }
}
