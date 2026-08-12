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
import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { existsSync, readFileSync } from 'node:fs'
import { resolve } from 'node:path'

const repoRoot = resolve(process.cwd(), '../..')
const read = (path: string) => readFileSync(resolve(repoRoot, path), 'utf8')
const readIfExists = (path: string) =>
  existsSync(resolve(repoRoot, path)) ? read(path) : ''

const optionalFiles = new Set([
  'web/classic/src/pages/Setting/Ratio/GroupRatioSettings.jsx',
  'web/classic/src/components/table/model-pricing/filter/PricingGroups.jsx',
])

const forbidden = [
  'cross_group_retry',
  'default_use_auto_group',
  'upgrade_group',
  'GroupRatio',
  'UserUsableGroups',
  'TopupGroupRatio',
  'AutoGroups',
  'ModelRequestRateLimitGroup',
  'include_using_group',
  'using_group',
  'enable_groups',
  '/api/group',
  '/api/user/self/groups',
  'group_ratio',
  'user_group_ratio',
  "value: 'group'",
  'PricingGroups',
  '分组倍率',
  '升级分组',
  '用户组',
]

describe('business group removal in classic frontend', () => {
  it('removes classic business group UI contracts', () => {
    for (const path of [
      'web/classic/src/components/playground/SettingsPanel.jsx',
      'web/classic/src/components/playground/OptimizedComponents.js',
      'web/classic/src/components/table/tokens/modals/EditTokenModal.jsx',
      'web/classic/src/components/table/tokens/TokensColumnDefs.jsx',
      'web/classic/src/components/table/users/modals/EditUserModal.jsx',
      'web/classic/src/components/settings/personal/components/UserInfoHeader.jsx',
      'web/classic/src/components/table/users/UsersColumnDefs.jsx',
      'web/classic/src/components/table/users/UsersFilters.jsx',
      'web/classic/src/components/table/channels/modals/EditChannelModal.jsx',
      'web/classic/src/components/table/channels/modals/EditTagModal.jsx',
      'web/classic/src/components/table/channels/ChannelsColumnDefs.jsx',
      'web/classic/src/components/table/channels/ChannelsFilters.jsx',
      'web/classic/src/hooks/channels/useChannelsData.jsx',
      'web/classic/src/components/table/subscriptions/modals/AddEditSubscriptionModal.jsx',
      'web/classic/src/components/table/subscriptions/SubscriptionsColumnDefs.jsx',
      'web/classic/src/components/topup/SubscriptionPlansCard.jsx',
      'web/classic/src/components/table/model-pricing/filter/PricingGroups.jsx',
      'web/classic/src/hooks/model-pricing/useModelPricingData.jsx',
      'web/classic/src/hooks/model-pricing/usePricingFilterCounts.js',
      'web/classic/src/helpers/render.jsx',
      'web/classic/src/helpers/utils.jsx',
      'web/classic/src/components/table/model-pricing/layout/PricingSidebar.jsx',
      'web/classic/src/components/table/model-pricing/layout/PricingPage.jsx',
      'web/classic/src/components/table/model-pricing/view/card/PricingCardView.jsx',
      'web/classic/src/components/table/model-pricing/view/table/PricingTableColumns.jsx',
      'web/classic/src/components/table/model-pricing/modal/components/ModelPricingTable.jsx',
      'web/classic/src/components/table/model-pricing/layout/content/PricingContent.jsx',
      'web/classic/src/components/table/model-pricing/modal/components/DynamicPricingBreakdown.jsx',
      'web/classic/src/components/table/model-pricing/modal/components/FilterModalContent.jsx',
      'web/classic/src/components/table/model-pricing/modal/ModelDetailSideSheet.jsx',
      'web/classic/src/components/table/model-pricing/modal/PricingFilterModal.jsx',
      'web/classic/src/components/table/usage-logs/UsageLogsFilters.jsx',
      'web/classic/src/components/table/usage-logs/UsageLogsColumnDefs.jsx',
      'web/classic/src/components/table/usage-logs/modals/ChannelAffinityUsageCacheModal.jsx',
      'web/classic/src/components/table/usage-logs/modals/UserInfoModal.jsx',
      'web/classic/src/hooks/usage-logs/useUsageLogsData.jsx',
      'web/classic/src/components/topup/modals/SubscriptionPurchaseModal.jsx',
      'web/classic/src/pages/Setting/Ratio/GroupRatioSettings.jsx',
      'web/classic/src/components/settings/RatioSetting.jsx',
      'web/classic/src/components/settings/RateLimitSetting.jsx',
      'web/classic/src/pages/Setting/RateLimit/SettingsRequestRateLimit.jsx',
      'web/classic/src/pages/Setting/Operation/SettingsChannelAffinity.jsx',
      'web/classic/src/pages/Setting/Payment/SettingsGeneralPayment.jsx',
      'web/classic/src/components/settings/PaymentSetting.jsx',
      'web/classic/src/components/settings/SystemSetting.jsx',
      'web/classic/src/components/table/models/ModelsColumnDefs.jsx',
      'web/classic/src/constants/playground.constants.js',
      'web/classic/src/hooks/playground/useDataLoader.js',
      'web/classic/src/constants/channel-affinity-template.constants.js',
      'web/classic/src/hooks/tokens/useTokensData.jsx',
      'web/classic/src/hooks/users/useUsersData.jsx',
      'web/classic/src/pages/Pricing/index.jsx',
    ]) {
      if (!optionalFiles.has(path)) {
        assert.equal(
          existsSync(resolve(repoRoot, path)),
          true,
          `${path} must exist and be scanned`
        )
      }
      const source = optionalFiles.has(path) ? readIfExists(path) : read(path)
      for (const term of forbidden) {
        assert.equal(source.includes(term), false, `${path} still contains ${term}`)
      }
    }
  })
})
