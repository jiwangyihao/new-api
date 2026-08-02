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

import React, { useCallback, useContext, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { API, copy, showError, showSuccess } from '../../helpers';
import { UserContext } from '../../context/User';
import RechargeCard from './RechargeCard';
import InvitationCard from './InvitationCard';

const TopUp = () => {
  const { t } = useTranslation();
  const [userState] = useContext(UserContext);
  const [payMethods, setPayMethods] = useState([]);
  const [enableOnlineTopUp, setEnableOnlineTopUp] = useState(false);
  const [enableStripeTopUp, setEnableStripeTopUp] = useState(false);
  const [enableCreemTopUp, setEnableCreemTopUp] = useState(false);
  const [enableKyrenSubscription, setEnableKyrenSubscription] = useState(false);
  const [subscriptionPlans, setSubscriptionPlans] = useState([]);
  const [subscriptionLoading, setSubscriptionLoading] = useState(true);
  const [activeSubscriptions, setActiveSubscriptions] = useState([]);
  const [allSubscriptions, setAllSubscriptions] = useState([]);
  const [lastSubscriptionPurchaseMode, setLastSubscriptionPurchaseMode] =
    useState(undefined);
  const [creditBalancePurchaseEnabled, setCreditBalancePurchaseEnabled] =
    useState(false);
  const [affLink, setAffLink] = useState('');

  const getSubscriptionSelf = useCallback(async () => {
    try {
      const response = await API.get('/api/subscription/self');
      if (!response.data?.success) return;

      const data = response.data.data || {};
      setActiveSubscriptions(data.subscriptions || []);
      setAllSubscriptions(data.all_subscriptions || []);
      setLastSubscriptionPurchaseMode(data.last_subscription_purchase_mode);
      setCreditBalancePurchaseEnabled(
        data.credit_balance_purchase_enabled === true,
      );
    } catch (_error) {
      // Keep the plans page available when subscription state cannot be refreshed.
    }
  }, []);

  useEffect(() => {
    let cancelled = false;

    const loadPage = async () => {
      const [plansResult, paymentResult, affiliateResult] =
        await Promise.allSettled([
          API.get('/api/subscription/plans'),
          API.get('/api/user/topup/info'),
          API.get('/api/user/aff'),
          getSubscriptionSelf(),
        ]);

      if (cancelled) return;

      if (
        plansResult.status === 'fulfilled' &&
        plansResult.value.data?.success
      ) {
        setSubscriptionPlans(plansResult.value.data.data || []);
      } else {
        setSubscriptionPlans([]);
      }
      setSubscriptionLoading(false);

      if (
        paymentResult.status === 'fulfilled' &&
        paymentResult.value.data?.success
      ) {
        const data = paymentResult.value.data.data || {};
        let methods = data.pay_methods || [];
        try {
          if (typeof methods === 'string') methods = JSON.parse(methods);
        } catch (_error) {
          methods = [];
        }
        if (!Array.isArray(methods)) methods = [];
        setPayMethods(methods.filter((method) => method?.name && method?.type));
        setEnableOnlineTopUp(data.enable_online_topup === true);
        setEnableStripeTopUp(data.enable_stripe_topup === true);
        setEnableCreemTopUp(data.enable_creem_topup === true);
        setEnableKyrenSubscription(data.enable_kyren_subscription === true);
      }

      if (
        affiliateResult.status === 'fulfilled' &&
        affiliateResult.value.data?.success &&
        affiliateResult.value.data.data
      ) {
        setAffLink(
          `${window.location.origin}/register?aff=${affiliateResult.value.data.data}`,
        );
      }
    };

    void loadPage();
    return () => {
      cancelled = true;
    };
  }, [getSubscriptionSelf]);

  const handleAffLinkClick = async () => {
    if (!affLink) {
      showError(t('邀请链接获取失败'));
      return;
    }
    try {
      await copy(affLink);
      showSuccess(t('邀请链接已复制到剪切板'));
    } catch (_error) {
      showError(t('邀请链接获取失败'));
    }
  };

  return (
    <div className='w-full max-w-7xl mx-auto relative min-h-screen lg:min-h-0 mt-[60px] px-2'>
      <div className='grid grid-cols-1 lg:grid-cols-2 gap-6'>
        <RechargeCard
          t={t}
          enableOnlineTopUp={enableOnlineTopUp}
          enableStripeTopUp={enableStripeTopUp}
          enableCreemTopUp={enableCreemTopUp}
          enableKyrenSubscription={enableKyrenSubscription}
          payMethods={payMethods}
          subscriptionLoading={subscriptionLoading}
          subscriptionPlans={subscriptionPlans}
          activeSubscriptions={activeSubscriptions}
          allSubscriptions={allSubscriptions}
          lastSubscriptionPurchaseMode={lastSubscriptionPurchaseMode}
          creditBalancePurchaseEnabled={creditBalancePurchaseEnabled}
          reloadSubscriptionSelf={getSubscriptionSelf}
        />
        <InvitationCard
          t={t}
          userState={userState}
          affLink={affLink}
          handleAffLinkClick={handleAffLinkClick}
        />
      </div>
    </div>
  );
};

export default TopUp;
