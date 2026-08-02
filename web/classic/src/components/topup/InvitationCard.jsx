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
import { Avatar, Typography, Card, Button, Input } from '@douyinfe/semi-ui';
import { Copy, Users, Gift } from 'lucide-react';

const InvitationCard = ({
  t,
  userState,
  affLink,
  handleAffLinkClick,
}) => {
  return (
    <Card className='!rounded-2xl shadow-sm border-0'>
      <div className='flex items-center mb-4'>
        <Avatar size='small' color='green' className='mr-3 shadow-md'>
          <Gift size={16} />
        </Avatar>
        <div>
          <Typography.Text className='text-lg font-medium'>
            {t('邀请奖励')}
          </Typography.Text>
          <div className='text-xs'>{t('邀请好友获得额外奖励')}</div>
        </div>
      </div>

      <div className='grid gap-3 sm:grid-cols-[140px_minmax(0,1fr)] sm:items-end'>
        <div className='rounded-xl bg-slate-50 p-3 text-center dark:bg-slate-800'>
          <div className='flex items-center justify-center gap-2 text-2xl font-bold'>
            <Users size={18} />
            {userState?.user?.aff_count || 0}
          </div>
          <Typography.Text type='tertiary' size='small'>
            {t('邀请人数')}
          </Typography.Text>
        </div>
        <Input
          value={affLink}
          readonly
          className='!rounded-lg'
          prefix={t('邀请链接')}
          suffix={
            <Button
              type='primary'
              theme='solid'
              onClick={handleAffLinkClick}
              icon={<Copy size={14} />}
              className='!rounded-lg'
            >
              {t('复制')}
            </Button>
          }
        />
      </div>
    </Card>
  );
};

export default InvitationCard;
