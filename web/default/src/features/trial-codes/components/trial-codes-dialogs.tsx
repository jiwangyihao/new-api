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
import { TrialCodeDeleteDialog } from './trial-code-delete-dialog'
import { TrialCodeMutateDrawer } from './trial-code-mutate-drawer'
import { useTrialCodes } from './trial-codes-provider'

export function TrialCodesDialogs() {
  const { open, setOpen, currentRow } = useTrialCodes()
  const isUpdate = open === 'update'

  return (
    <>
      <TrialCodeMutateDrawer
        open={open === 'create' || isUpdate}
        onOpenChange={(isOpen) => !isOpen && setOpen(null)}
        currentRow={isUpdate ? currentRow || undefined : undefined}
      />
      <TrialCodeDeleteDialog />
    </>
  )
}
