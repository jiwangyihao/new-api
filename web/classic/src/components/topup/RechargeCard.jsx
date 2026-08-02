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

import React from 'react';
import { Card, Space } from '@douyinfe/semi-ui';
import SubscriptionPlansCard from './SubscriptionPlansCard';

const RechargeCard = ({
  t,
  enableOnlineTopUp,
  enableStripeTopUp,
  enableCreemTopUp,
  enableKyrenSubscription,
  payMethods,
  activeSubscriptions = [],
  allSubscriptions = [],
  lastSubscriptionPurchaseMode,
  creditBalancePurchaseEnabled = false,
  reloadSubscriptionSelf,
  subscriptionLoading = false,
  subscriptionPlans = [],
}) => {
  const shouldShowSubscription =
    !subscriptionLoading && subscriptionPlans.length > 0;

  const subscriptionContent = (
    <SubscriptionPlansCard
      t={t}
      loading={subscriptionLoading}
      plans={subscriptionPlans}
      payMethods={payMethods}
      enableOnlineTopUp={enableOnlineTopUp}
      enableStripeTopUp={enableStripeTopUp}
      enableCreemTopUp={enableCreemTopUp}
      enableKyrenSubscription={enableKyrenSubscription}
      activeSubscriptions={activeSubscriptions}
      allSubscriptions={allSubscriptions}
      reloadSubscriptionSelf={reloadSubscriptionSelf}
      lastSubscriptionPurchaseMode={lastSubscriptionPurchaseMode}
      creditBalancePurchaseEnabled={creditBalancePurchaseEnabled}
      withCard={false}
    />
  );

  return (
    <Card className='!rounded-2xl shadow-sm border-0'>
      {shouldShowSubscription ? (
        <Space vertical style={{ width: '100%' }}>
          {subscriptionContent}
        </Space>
      ) : (
        subscriptionContent
      )}
    </Card>
  );
};

export default RechargeCard;
