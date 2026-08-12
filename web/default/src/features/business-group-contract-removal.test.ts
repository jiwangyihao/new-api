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
import { readFileSync } from 'node:fs'
import { inspect } from 'node:util'
import { resolve } from 'node:path'

const repoRoot = resolve(process.cwd(), '../..')
const read = (path: string) => readFileSync(resolve(repoRoot, path), 'utf8')

const openApiForbidden = [
  'cross_group_retry',
  'upgrade_group',
  'enable_groups',
  'group_ratio',
  'user_group_ratio',
  'default_use_auto_group',
  'include_using_group',
  'using_group',
  '/api/user/self/groups',
  '/api/user/groups',
  '/api/group',
]

const openApiBusinessGroupParameterNames = [
  'group',
  'groups',
  'user_group',
  'request_group',
  'upgrade_group',
  'enable_groups',
  'cross_group_retry',
]

const openApiBusinessGroupPropertyNames = [
  ...openApiBusinessGroupParameterNames,
  'group_ratio',
  'user_group_ratio',
]

const i18nAndDocsForbidden = [
  'GroupRatio',
  'GroupGroupRatio',
  'TopupGroupRatio',
  'DefaultUseAutoGroup',
  'UserUsableGroups',
  'AutoGroups',
  'ModelRequestRateLimitGroup',
  'cross_group_retry',
  'upgrade_group',
  'enable_groups',
  'using_group',
  'include_using_group',
  'MsgGroup',
  'group.name_type_empty',
  'group.name_exists',
  'group.id_missing',
  'group_access_denied',
  'group_not_exists',
  'under group',
  '当前分组',
  '分组 {{.Group}}',
  '分组倍率',
  '自动分组',
  '升级分组',
  '令牌分组',
  '用户分组',
  '用户组',
  'system group rate',
  'token group',
  'Token group',
  'user group',
  'User group',
  'Group Ratio',
  'group ratio',
  'Default to auto groups',
  'トークングループ化',
  'Regroupement de jetons',
  'Add auto group',
  'Auto Group Chain',
  'Auto group behavior',
  'Default to auto group',
  'default auto group',
  'specific models, groups, IPs',
  '分组与模型定价设置',
  '用户可选分组',
  '创建令牌默认选择auto分组',
  '默认使用auto分组',
  'Groups are a core concept',
  'user-selectable groups',
  'Edit group rate limit',
  'Group-based rate limits',
  'Base multipliers applied when users select specific groups',
  'Create your first group to reuse model',
  'No group-based rate limits',
  'Pricing by Group',
  'Cross-group retry',
  'Group Pricing',
  'Pricing groups',
  'Selectable groups',
  'default billing group',
  'current group fail',
  '分组设置使用说明',
  '分组速率限制',
  '跨分组特殊倍率',
  '分组价格调整',
  'Configure the ratio for this group',
  'Determines how this group is applied elsewhere',
  'Give the group a recognizable name',
  'Give this group a recognizable name',
  'Group name cannot be changed when editing',
  'Group prices cannot be expanded',
  'Optional notes about when to use this group',
  'Unique identifier for this group',
  'Make it easier for teammates to pick the right group',
  'Group Type',
  'Select a group type',
  'Model Group',
  'Tag Group',
  'Endpoint Group',
  'Group name is required',
  '目标分组',
  '确认删除该分组',
  '该分组的所有规则',
  '令牌的计费倍率由令牌所选的分组决定',
  '用户所在分组',
  '通过分组可以实现不同用户等级',
  '根据用户所在分组',
  '管理员在此创建分组并设置倍率',
  '由管理员分配，决定用户身份等级',
  '管理员在用户管理中将用户分配到对应分组',
  'auto分组调用链路',
  'default 和 vip 只能由管理员',
  'Admin creates groups and sets ratios',
  'Assigned by admin, determines user tier',
  'auto group call chain',
]

const i18nAndDocsFiles = [
  'i18n/locales/en.yaml',
  'i18n/keys.go',
  'i18n/locales/zh-CN.yaml',
  'i18n/locales/zh-TW.yaml',
  'web/default/src/i18n/static-keys.ts',
  'web/default/src/i18n/locales/en.json',
  'web/default/src/i18n/locales/zh.json',
  'web/default/src/i18n/locales/fr.json',
  'web/default/src/i18n/locales/ja.json',
  'web/default/src/i18n/locales/ru.json',
  'web/default/src/i18n/locales/vi.json',
  'web/classic/src/i18n/locales/en.json',
  'web/classic/src/i18n/locales/zh-CN.json',
  'web/classic/src/i18n/locales/zh.json',
  'web/classic/src/i18n/locales/zh-TW.json',
  'web/classic/src/i18n/locales/fr.json',
  'web/classic/src/i18n/locales/ja.json',
  'web/classic/src/i18n/locales/ru.json',
  'web/classic/src/i18n/locales/vi.json',
  'README.md',
  'README.en.md',
  'README.zh_CN.md',
  'README.zh_TW.md',
  'README.ja.md',
  'README.fr.md',
  'docs/channel/other_setting.md',
  'web/default/src/i18n/locales/_reports/_sync-report.json',
  'web/default/src/i18n/locales/_reports/fr.untranslated.json',
  'web/default/src/i18n/locales/_reports/ja.untranslated.json',
  'web/default/src/i18n/locales/_reports/ru.untranslated.json',
  'web/default/src/i18n/locales/_reports/vi.untranslated.json',
  'web/default/src/i18n/locales/_reports/zh.untranslated.json',
]

describe('business group public contracts are removed', () => {
  it('removes business group endpoints and schemas from OpenAPI', () => {
    const api = read('docs/openapi/api.json')
    for (const term of openApiForbidden) {
      assert.equal(api.includes(term), false, `docs/openapi/api.json still contains ${term}`)
    }
    assert.equal(api.includes('/api/prefill_group/'), true)
    const apiDocument = JSON.parse(api)
    for (const [path, methods] of Object.entries(apiDocument.paths ?? {})) {
      for (const [method, operation] of Object.entries(methods as Record<string, unknown>)) {
        if (!operation || typeof operation !== 'object') continue
        const parameters = (operation as { parameters?: unknown[] }).parameters ?? []
        for (const parameter of parameters) {
          if (!parameter || typeof parameter !== 'object') continue
          const name = (parameter as { name?: unknown }).name
          assert.equal(
            typeof name === 'string' && openApiBusinessGroupParameterNames.includes(name),
            false,
            `docs/openapi/api.json still exposes ${String(name)} parameter at ${method.toUpperCase()} ${path}`
          )
        }
        const tags = (operation as { tags?: unknown[] }).tags ?? []
        assert.equal(
          tags.includes('分组'),
          false,
          `docs/openapi/api.json still exposes legacy business group operation tag at ${method.toUpperCase()} ${path}`
        )
        if (path.startsWith('/api/prefill_group')) {
          assert.equal(
            tags.includes('预填组'),
            true,
            `docs/openapi/api.json should tag ${method.toUpperCase()} ${path} as prefill groups`
          )
        }
      }
    }

    for (const [schemaName, schema] of Object.entries(apiDocument.components?.schemas ?? {})) {
      if (!schema || typeof schema !== 'object') continue
      const properties = (schema as { properties?: Record<string, unknown> }).properties ?? {}
      for (const propertyName of Object.keys(properties)) {
        assert.equal(
          openApiBusinessGroupPropertyNames.includes(propertyName),
          false,
          `docs/openapi/api.json still exposes ${propertyName} on schema ${schemaName}: ${inspect(properties[propertyName])}`
        )
      }
    }

    for (const tag of apiDocument.tags ?? []) {
      if (!tag || typeof tag !== 'object') continue
      assert.notEqual(
        (tag as { name?: unknown }).name,
        '分组',
        'docs/openapi/api.json still exposes legacy business group tag'
      )
    }
  })

  it('removes business group phrases from i18n and documentation contracts', () => {
    for (const path of i18nAndDocsFiles) {
      const source = read(path)
      for (const term of i18nAndDocsForbidden) {
        assert.equal(source.includes(term), false, `${path} still contains ${term}`)
      }
    }
  })
})
