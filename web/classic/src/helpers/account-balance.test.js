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
import { readFileSync } from 'node:fs';
import { describe, test } from 'node:test';
import {
  accountBalanceCentsToCnyAmount,
  accountBalanceCnyToCents,
  formatAccountBalance,
} from './account-balance.js';

describe('classic account balance helper', () => {
  test('uses cents for account balance amounts', () => {
    assert.equal(accountBalanceCentsToCnyAmount(4000), 40);
    assert.equal(accountBalanceCentsToCnyAmount(3990), 39.9);
    assert.equal(accountBalanceCentsToCnyAmount(-250), -2.5);
    assert.equal(accountBalanceCnyToCents(40), 4000);
    assert.equal(accountBalanceCnyToCents(39.9), 3990);
    assert.equal(accountBalanceCnyToCents(-2.5), -250);
    assert.equal(accountBalanceCnyToCents(Number.NaN), 0);
    assert.match(formatAccountBalance(4000), /40\.00/);
    assert.match(formatAccountBalance(3990), /39\.90/);
    assert.match(formatAccountBalance(-250), /-2\.50/);
  });

  test('top-up balance UI uses credited CNY balance instead of legacy quota conversion', () => {
    const index = readFileSync('src/components/topup/index.jsx', 'utf8');
    const recharge = readFileSync('src/components/topup/RechargeCard.jsx', 'utf8');
    const invitation = readFileSync('src/components/topup/InvitationCard.jsx', 'utf8');
    const transfer = readFileSync('src/components/topup/modals/TransferModal.jsx', 'utf8');
    const confirm = readFileSync('src/components/topup/modals/PaymentConfirmModal.jsx', 'utf8');
    const creemAndTransferSource = index + recharge + invitation + transfer + confirm;

    assert.match(creemAndTransferSource, /formatAccountBalance/);
    assert.match(transfer, /accountBalanceCnyToCents/);
    assert.match(confirm, /到账余额|Top-up credit|Credited balance|Account Balance/);
    assert.doesNotMatch(creemAndTransferSource, /renderQuotaWithAmount\(topUpCount\)/);
    assert.doesNotMatch(transfer, /getQuotaPerUnit\(\)/);
  });

  test('classic subscription balance purchase calls balance API with cents comparison', () => {
    const card = readFileSync('src/components/topup/SubscriptionPlansCard.jsx', 'utf8');
    const modal = readFileSync('src/components/topup/modals/SubscriptionPurchaseModal.jsx', 'utf8');

    assert.match(card, /\/api\/subscription\/balance\/pay/);
    assert.match(card, /idempotency_key/);
    assert.match(modal, /Pay with Account Balance|账户余额支付|余额支付/);
    assert.match(modal, /Math\.round\([^)]*price_amount[^)]*\*\s*100\)|balanceCents\s*>=/);
    assert.doesNotMatch(modal, /renderQuota\([^)]*price_amount/);
  });

  test('classic payment gateway labels use credited CNY balance and channel unit price', () => {
    const epay = readFileSync('src/pages/Setting/Payment/SettingsPaymentGateway.jsx', 'utf8');
    const stripe = readFileSync('src/pages/Setting/Payment/SettingsPaymentGatewayStripe.jsx', 'utf8');
    const waffo = readFileSync('src/pages/Setting/Payment/SettingsPaymentGatewayWaffo.jsx', 'utf8');
    const pancake = readFileSync('src/pages/Setting/Payment/SettingsPaymentGatewayWaffoPancake.jsx', 'utf8');
    const source = epay + stripe + waffo + pancake;

    assert.doesNotMatch(source, /最低充值美元数量|充值价格（x元\/美金）|getQuotaPerUnit|Minimum top-up \(USD\)|Minimum top-up quantity/);
    assert.match(source, /到账余额|实付单价|credited CNY balance|channel unit price/i);
  });

  test('classic redemption paths round trip wallet amounts as cents', () => {
    const columns = readFileSync('src/components/table/redemptions/RedemptionsColumnDefs.jsx', 'utf8');
    const modal = readFileSync('src/components/table/redemptions/modals/EditRedemptionModal.jsx', 'utf8');
    const hook = readFileSync('src/hooks/redemptions/useRedemptionsData.jsx', 'utf8');
    const table = readFileSync('src/components/table/redemptions/RedemptionsTable.jsx', 'utf8');

    assert.match(columns, /formatAccountBalance/);
    assert.match(modal, /accountBalanceCentsToCnyAmount/);
    assert.match(modal, /accountBalanceCnyToCents/);
    assert.doesNotMatch(hook + table, /quotaToDisplayAmount|displayAmountToQuota/);
  });

  test('classic check-in and reward settings use CNY yuan inputs for account balance cents', () => {
    const checkinCard = readFileSync('src/components/settings/personal/cards/CheckinCalendar.jsx', 'utf8');
    const checkinSettings = readFileSync('src/pages/Setting/Operation/SettingsCheckin.jsx', 'utf8');
    const creditLimit = readFileSync('src/pages/Setting/Operation/SettingsCreditLimit.jsx', 'utf8');

    assert.match(checkinCard, /formatAccountBalance/);
    assert.match(checkinSettings, /accountBalanceCentsToCnyAmount/);
    assert.match(checkinSettings, /accountBalanceCnyToCents/);
    assert.match(creditLimit, /accountBalanceCnyToCents/);
    assert.match(creditLimit, /PreConsumedQuota/);
  });

  test('classic user views format account balance separately from used quota', () => {
    const header = readFileSync('src/components/settings/personal/components/UserInfoHeader.jsx', 'utf8');
    const userColumns = readFileSync('src/components/table/users/UsersColumnDefs.jsx', 'utf8');
    const usersTable = readFileSync('src/components/table/users/UsersTable.jsx', 'utf8');
    const editUser = readFileSync('src/components/table/users/modals/EditUserModal.jsx', 'utf8');
    const usageUser = readFileSync('src/components/table/usage-logs/modals/UserInfoModal.jsx', 'utf8');
    const source = header + userColumns + usersTable + editUser + usageUser;

    assert.match(source, /formatAccountBalance/);
    assert.match(editUser, /accountBalanceCnyToCents/);
    assert.doesNotMatch(source, /used_quota\s*\+\s*quota|quota\s*\+\s*used_quota/);
  });

  test('classic top-up history consumes credited balance DTO fields', () => {
    const history = readFileSync('src/components/topup/modals/TopupHistoryModal.jsx', 'utf8');

    assert.match(history, /credited_balance_display/);
    assert.match(history, /credited_balance_cents/);
    assert.match(history, /Number\.isFinite\(cents\)/);
    assert.doesNotMatch(history, /cents > 0/);
    assert.doesNotMatch(history, /<Text>\{amount\}<\/Text>/);
  });
});
