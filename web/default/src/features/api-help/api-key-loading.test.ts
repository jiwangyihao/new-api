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

const source = readFileSync(
  new URL('./components/api-usage-help-dialog.tsx', import.meta.url),
  'utf8'
)

describe('api help key loading', () => {
  test('loads more than the first API key page before declaring no enabled keys', () => {
    assert.match(source, /do\s*{[\s\S]*getApiKeys\(\{ p: page, size: API_KEY_PAGE_SIZE \}\)[\s\S]*}\s*while/)
    assert.match(source, /activeItems\.length < API_HELP_KEY_LIMIT/)
    assert.match(source, /page <= totalPages/)
  })

  test('uses backend artifacts instead of frontend OpenCode or OMP renderers', () => {
    assert.doesNotMatch(source, /buildOpenCodeConfig/)
    assert.doesNotMatch(source, /buildOmpModelsConfig/)
    assert.doesNotMatch(source, /buildOmpSettingsConfig/)
    assert.doesNotMatch(source, /buildOmpPluginInstructions/)
    assert.doesNotMatch(source, /buildOmpImageGeneratorConfig/)
    assert.match(source, /fetchAgentConfigArtifact/)
    assert.match(source, /opencode\.json/)
    assert.match(source, /models\.yml/)
    assert.match(source, /config\.yml/)
  })

  test('metadata query is keyed by the explicitly selected API key', () => {
    assert.match(source, /const selectedApiKey = selectedKeyId \? apiKeys\.find/)
    assert.match(source, /buildOpenCodeMetadataQueryKey\(selectedKeyId\)/)
    assert.match(source, /enabled:\s*open\s*&&\s*Boolean\(selectedKeyId\)/)
    assert.match(source, /getOpenCodeOpenAIModels\(selectedApiKey\.id\)/)
  })

  test('dialog uses fixed flex shell with a bounded scroll body', () => {
    assert.match(source, /DialogContent className='flex max-h-\[92vh\][^']*flex-col[^']*overflow-hidden/)
    assert.match(source, /<div className='min-h-0 flex-1 overflow-hidden'>/)
    assert.match(source, /<ScrollArea className='h-full min-h-0'>/)
    assert.match(source, /DialogFooter className='mx-0 mb-0 shrink-0/)
    assert.doesNotMatch(source, /grid-rows-none/)
  })

  test('hides ready config blocks until an API key and backend artifacts are available', () => {
    assert.match(source, /const hasSelectedApiKey = Boolean\(currentSelectedKeyId\) && Boolean\(apiKey\)/)
    assert.match(source, /if \(!hasSelectedApiKey\) return \[\]/)
    assert.match(source, /const opencodeCardState: MetadataState = apiKeySelectionNotice/)
    assert.match(source, /const ompCardState: MetadataState = apiKeySelectionNotice/)
  })

  test('shows an explicit selection hint before metadata readiness checks', () => {
    assert.match(source, /const apiKeySelectionNotice = !hasSelectedApiKey/)
    assert.match(source, /Select an API key to load AI auto-configuration/)
    assert.match(source, /state=\{opencodeCardState\}/)
    assert.match(source, /state=\{ompCardState\}/)
  })

  test('shows Codex Pro header guidance only for verified harness configurations', () => {
    assert.match(source, /buildCodexCliConfig/)
    assert.match(source, /buildClaudeCodeConfig/)
    assert.match(source, /getUnverifiedCodexProHeaderConfigNotice/)
    assert.match(source, /Codex CLI/)
    assert.match(source, /Claude Code/)
    assert.match(source, /Hermes Agent/)
    assert.match(source, /OpenClaw/)
    assert.match(source, /X-NewAPI-Codex-Pro-Intent/)
  })

  test('hides Codex Pro guidance when the global feature switch is enabled', () => {
    assert.match(source, /codexProFeaturesHidden/)
    assert.match(source, /!codexProFeaturesHidden[\s\S]*<TabsTrigger value='codex-pro'>/)
    assert.match(source, /!codexProFeaturesHidden[\s\S]*<TabsContent value='codex-pro'/)
  })
})
