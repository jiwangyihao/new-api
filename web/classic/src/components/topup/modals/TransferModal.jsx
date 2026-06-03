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
import { Modal, Typography, Input, InputNumber } from '@douyinfe/semi-ui';
import { CreditCard } from 'lucide-react';
import {
  accountBalanceCentsToCnyAmount,
  accountBalanceCnyToCents,
  formatAccountBalance,
} from '../../../helpers/account-balance.js';

const MIN_TRANSFER_AMOUNT_CNY = 0.01;

const TransferModal = ({
  t,
  openTransfer,
  transfer,
  handleTransferCancel,
  userState,
  transferAmount,
  setTransferAmount,
}) => {
  const availableCents = Number(userState?.user?.aff_quota || 0);
  const maxAmount = accountBalanceCentsToCnyAmount(availableCents);
  const minimumTransferDisplay = formatAccountBalance(
    accountBalanceCnyToCents(MIN_TRANSFER_AMOUNT_CNY),
  );
  return (
    <Modal
      title={
        <div className='flex items-center'>
          <CreditCard className='mr-2' size={18} />
          {t('划转邀请额度')}
        </div>
      }
      visible={openTransfer}
      onOk={() => transfer(accountBalanceCnyToCents(transferAmount))}
      onCancel={handleTransferCancel}
      maskClosable={false}
      centered
    >
      <div className='space-y-4'>
        <div>
          <Typography.Text strong className='block mb-2'>
            {t('可用邀请额度')}
          </Typography.Text>
          <Input
            value={formatAccountBalance(availableCents)}
            disabled
            className='!rounded-lg'
          />
        </div>
        <div>
          <Typography.Text strong className='block mb-2'>
            {t('划转额度')} · {t('最低') + minimumTransferDisplay}
          </Typography.Text>
          <InputNumber
            min={MIN_TRANSFER_AMOUNT_CNY}
            max={maxAmount}
            value={transferAmount}
            onChange={(value) => setTransferAmount(Number(value || 0))}
            step={MIN_TRANSFER_AMOUNT_CNY}
            precision={2}
            className='w-full !rounded-lg'
          />
        </div>
      </div>
    </Modal>
  );
};

export default TransferModal;
