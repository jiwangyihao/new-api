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

import assert from 'node:assert/strict';
import { describe, test } from 'node:test';
import {
  buildSubscriptionPaymentRequest,
  initialSubscriptionPurchaseMode,
  isCreditBalancePurchaseAvailable,
  processKyrenSubscriptionPayment,
} from './subscription-purchase.js';

const eligiblePlan = {
  id: 701,
  enabled: true,
  public_visible: true,
  unlimited_purchase_enabled: true,
  duration_unit: 'month',
  duration_value: 1,
  quota_reset_period: 'monthly',
  monthly_token_limit: 1200,
  is_trial: false,
  invite_trial: false,
};

describe('classic Kyren subscription purchase contract', () => {
  test('submits an explicit timed purchase independently of top-up products', async () => {
    const requests = [];
    const opened = [];

    await processKyrenSubscriptionPayment({
      planId: eligiblePlan.id,
      purchaseMode: 'timed',
      requestPayment: async (url, request) => {
        requests.push({ url, request });
        return {
          data: {
            success: true,
            data: { checkout_url: 'https://checkout.example/timed' },
          },
        };
      },
      openCheckout: (url) => opened.push(url),
    });

    assert.deepEqual(requests, [
      {
        url: '/api/subscription/kyren/pay',
        request: { plan_id: 701, purchase_mode: 'timed' },
      },
    ]);
    assert.deepEqual(opened, ['https://checkout.example/timed']);
    assert.equal('product_id' in requests[0].request, false);
  });

  test('submits Credit mode to the Kyren subscription endpoint without account-balance payment', async () => {
    const requests = [];

    await processKyrenSubscriptionPayment({
      planId: eligiblePlan.id,
      purchaseMode: 'credit_balance',
      requestPayment: async (url, request) => {
        requests.push({ url, request });
        return {
          success: true,
          data: { url: 'https://checkout.example/credit' },
        };
      },
      openCheckout: () => {},
    });

    assert.deepEqual(requests, [
      {
        url: '/api/subscription/kyren/pay',
        request: { plan_id: 701, purchase_mode: 'credit_balance' },
      },
    ]);
    assert.notEqual(requests[0].url, '/api/subscription/balance/pay');
    assert.equal('idempotency_key' in requests[0].request, false);
  });

  test('requires an explicit mode and only restores an available preference', () => {
    assert.equal(initialSubscriptionPurchaseMode(undefined, true), undefined);
    assert.equal(initialSubscriptionPurchaseMode('timed', true), 'timed');
    assert.equal(
      initialSubscriptionPurchaseMode('credit_balance', true),
      'credit_balance',
    );
    assert.equal(
      initialSubscriptionPurchaseMode('credit_balance', false),
      undefined,
    );
    assert.equal(isCreditBalancePurchaseAvailable(eligiblePlan, true), true);
    assert.throws(
      () => buildSubscriptionPaymentRequest(701, undefined),
      /purchase mode/,
    );
  });
});
