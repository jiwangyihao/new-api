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
import { requireAdminInvitationCommissionWithdrawalsAccess } from '@/routes/_authenticated/invitation-commission/withdrawals'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import { useAuthStore, type AuthUser } from '@/stores/auth-store'
import { ROLE } from '@/lib/roles'

function setRouteUser(role: number | undefined) {
  const auth = useAuthStore.getState().auth
  const user: AuthUser | null = role
    ? {
        id: role,
        username: `role-${role}`,
        role,
      }
    : null

  useAuthStore.setState({
    auth: {
      ...auth,
      user,
    },
  })
}

function assertRedirectsTo403(action: () => void) {
  try {
    action()
  } catch (error) {
    if (error instanceof Response) {
      const redirectOptions = (
        error as Response & { options?: { to?: string } }
      ).options
      assert.equal(redirectOptions?.to, '/403')
      return
    }
    assert.match(String(error), /\/403/)
    return
  }

  assert.fail('expected route guard to redirect ordinary users to /403')
}

test('admin withdrawals page uses fixed route guard and refreshes withdrawals plus task summary', () => {
  const route = readFileSync(
    'src/routes/_authenticated/invitation-commission/withdrawals/index.tsx',
    'utf8'
  )
  const page = readFileSync(
    'src/features/invitation-commission/admin-withdrawals.tsx',
    'utf8'
  )
  const api = readFileSync('src/features/invitation-commission/api.ts', 'utf8')

  assert.match(route, /\/_authenticated\/invitation-commission\/withdrawals\//)
  assert.match(route, /beforeLoad/)
  assert.match(route, /ROLE\.ADMIN/)
  assert.match(route, /redirect\(\{\s*to:\s*'\/403'/)
  assert.match(page, /Mark manual cashback as completed/)
  assert.match(page, /\['admin', 'invitation-commission', 'withdrawals'/)
  assert.match(page, /\['admin', 'tasks', 'summary'\]/)
  assert.match(page, /user_remark/)
  assert.doesNotMatch(page, /\.remark\b/)
  assert.match(api, /\/api\/admin\/invitation-commission\/withdrawals/)
  assert.match(api, /\/api\/admin\/tasks\/summary/)
})

test('admin withdrawals route guard allows only administrators', () => {
  setRouteUser(undefined)
  assertRedirectsTo403(requireAdminInvitationCommissionWithdrawalsAccess)

  setRouteUser(ROLE.USER)
  assertRedirectsTo403(requireAdminInvitationCommissionWithdrawalsAccess)

  setRouteUser(ROLE.ADMIN)
  assert.doesNotThrow(requireAdminInvitationCommissionWithdrawalsAccess)

  setRouteUser(ROLE.SUPER_ADMIN)
  assert.doesNotThrow(requireAdminInvitationCommissionWithdrawalsAccess)
})

test('admin withdrawal API helpers unwrap payload data contract', () => {
  const api = readFileSync('src/features/invitation-commission/api.ts', 'utf8')
  const page = readFileSync(
    'src/features/invitation-commission/admin-withdrawals.tsx',
    'utf8'
  )

  assert.match(api, /return unwrapAdminPayload\(res\.data\)/)
  assert.match(
    api,
    /Promise<PageEnvelope<AdminInvitationCommissionWithdrawal>>/
  )
  assert.match(api, /Promise<AdminTasksSummary>/)
  assert.match(api, /pending_commission_withdrawals/)
  assert.match(api, /unwrapAdminPayload/)
  assert.match(api, /if \(!payload\.success\)/)
  assert.match(api, /throw new Error\(payload\.message \|\| 'Request failed'\)/)
  assert.match(page, /\['admin', 'tasks', 'summary'\]/)
})
