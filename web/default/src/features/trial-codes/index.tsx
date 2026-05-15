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
import { useTranslation } from 'react-i18next'
import { SectionPageLayout } from '@/components/layout'
import { TrialCodesDialogs } from './components/trial-codes-dialogs'
import { TrialCodesPrimaryButtons } from './components/trial-codes-primary-buttons'
import { TrialCodesProvider } from './components/trial-codes-provider'
import { TrialCodesTable } from './components/trial-codes-table'

export function TrialCodes() {
  const { t } = useTranslation()

  return (
    <TrialCodesProvider>
      <SectionPageLayout>
        <SectionPageLayout.Title>{t('Trial Codes')}</SectionPageLayout.Title>
        <SectionPageLayout.Description>
          {t('Manage manual trial codes for registration and OAuth account setup')}
        </SectionPageLayout.Description>
        <SectionPageLayout.Actions>
          <TrialCodesPrimaryButtons />
        </SectionPageLayout.Actions>
        <SectionPageLayout.Content>
          <TrialCodesTable />
        </SectionPageLayout.Content>
      </SectionPageLayout>

      <TrialCodesDialogs />
    </TrialCodesProvider>
  )
}
