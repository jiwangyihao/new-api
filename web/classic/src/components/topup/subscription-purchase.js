/*
Copyright (C) 2025 QuantumNous

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

export const SUBSCRIPTION_PURCHASE_MODE_TIMED = 'timed';
export const SUBSCRIPTION_PURCHASE_MODE_CREDIT_BALANCE = 'credit_balance';

const subscriptionPurchaseModes = new Set([
  SUBSCRIPTION_PURCHASE_MODE_TIMED,
  SUBSCRIPTION_PURCHASE_MODE_CREDIT_BALANCE,
]);

export function isCreditBalancePurchaseAvailable(plan, globallyEnabled) {
  return !!(
    globallyEnabled &&
    plan?.enabled &&
    plan.public_visible &&
    plan.unlimited_purchase_enabled &&
    plan.duration_unit === 'month' &&
    Number(plan.duration_value) === 1 &&
    plan.quota_reset_period === 'monthly' &&
    Number(plan.monthly_token_limit || 0) > 0 &&
    !plan.is_trial &&
    !plan.invite_trial
  );
}

export function initialSubscriptionPurchaseMode(preference, creditAvailable) {
  if (
    preference === SUBSCRIPTION_PURCHASE_MODE_CREDIT_BALANCE &&
    creditAvailable
  ) {
    return SUBSCRIPTION_PURCHASE_MODE_CREDIT_BALANCE;
  }
  return preference === SUBSCRIPTION_PURCHASE_MODE_TIMED
    ? SUBSCRIPTION_PURCHASE_MODE_TIMED
    : undefined;
}

export function isKyrenSubscriptionAvailable(plan, globallyEnabled) {
  return !!(
    globallyEnabled &&
    plan?.kyren_product_id?.trim() &&
    String(plan.currency || '').toUpperCase() === 'CNY' &&
    Number(plan.price_amount || 0) >= 0.01 &&
    plan.enabled !== false &&
    plan.public_visible !== false &&
    !plan.is_trial
  );
}

export function buildSubscriptionPaymentRequest(
  planId,
  purchaseMode,
  paymentMethod,
) {
  const normalizedPlanId = Number(planId);
  if (!Number.isInteger(normalizedPlanId) || normalizedPlanId <= 0) {
    throw new Error('Invalid subscription plan');
  }
  if (!subscriptionPurchaseModes.has(purchaseMode)) {
    throw new Error('Select a subscription purchase mode');
  }

  const request = {
    plan_id: normalizedPlanId,
    purchase_mode: purchaseMode,
  };
  if (paymentMethod) {
    request.payment_method = paymentMethod;
  }
  return request;
}

export async function processKyrenSubscriptionPayment({
  planId,
  purchaseMode,
  requestPayment,
  openCheckout,
}) {
  const request = buildSubscriptionPaymentRequest(planId, purchaseMode);
  const response = await requestPayment('/api/subscription/kyren/pay', request);
  const payload =
    response?.data?.success === undefined ? response : response.data;
  const checkoutUrl =
    payload?.data?.checkout_url ||
    payload?.data?.url ||
    payload?.checkout_url ||
    payload?.url;

  if (!(payload?.success || payload?.message === 'success') || !checkoutUrl) {
    const message =
      typeof payload?.data === 'string'
        ? payload.data
        : payload?.message || 'Kyren checkout creation failed';
    throw new Error(message);
  }

  openCheckout(checkoutUrl);
  return checkoutUrl;
}
