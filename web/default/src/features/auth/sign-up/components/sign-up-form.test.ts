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
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { describe, test } from 'node:test'

function readSource(file: string): string {
  return readFileSync(file, 'utf8')
}

describe('sign-up invitation code field', () => {
  test('prefills the shared trial code field from stored affiliate code', () => {
    const source = readSource(
      'src/features/auth/sign-up/components/sign-up-form.tsx'
    )

    assert.match(
      source,
      /defaultValues:\s*{[\s\S]*trial_code:\s*getAffiliateCode\(\)/,
      'invite links should appear as the value of the existing trial_code field'
    )
    assert.equal(
      source.includes(`name='aff'`),
      false,
      'registration should not add a separate invitation code form field'
    )
    assert.ok(
      source.includes(`name='trial_code'`),
      'registration should keep using the shared trial_code field'
    )
  })

  test('sign-up route saves link affiliate before the form initializes', () => {
    const routeSource = readSource('src/routes/(auth)/sign-up.tsx')

    assert.ok(
      routeSource.includes('saveAffiliateCode'),
      'route should persist search aff before rendering the sign-up form'
    )
    assert.match(
      routeSource,
      /beforeLoad:\s*\(\{\s*search\s*\}\)\s*=>\s*\{[\s\S]*saveAffiliateCode\(search\.aff\)/,
      'beforeLoad should save search.aff synchronously before component render'
    )
  })
})
