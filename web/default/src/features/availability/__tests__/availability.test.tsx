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
import type { AxiosAdapter } from 'axios'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  cleanup,
  fireEvent,
  render,
  waitFor,
  within,
} from '@testing-library/react/pure'
import userEvent from '@testing-library/user-event'
import { afterEach, test } from 'bun:test'
import { createInstance } from 'i18next'
import assert from 'node:assert/strict'
import { I18nextProvider } from 'react-i18next'
import { api } from '@/lib/api'
import {
  filterNavGroupsByRole,
  filterSidebarNavGroupsForConfig,
  getDefaultSidebarModulesForTest,
} from '@/hooks/use-sidebar-config'
import { useSidebarData } from '@/hooks/use-sidebar-data'
import { TooltipProvider } from '@/components/ui/tooltip'
import { AvailabilityPage } from '..'
import { GroupGrid } from '../components/group-grid'
import { HistoryTimeline } from '../components/history-timeline'
import { MetricDetails } from '../components/metric-details'
import { observedBand, observedState } from '../lib/presentation'
import type {
  AvailabilityGroup,
  AvailabilityMetric,
  AvailabilityModel,
  AvailabilityReport,
} from '../types'

const originalAdapter = api.defaults.adapter
const clients: QueryClient[] = []
afterEach(() => {
  cleanup()
  api.defaults.adapter = originalAdapter
  clients.splice(0).forEach((client) => client.clear())
})
function metric(
  overrides: Partial<AvailabilityMetric> = {}
): AvailabilityMetric {
  return {
    state: 'healthy',
    success_rate: 100,
    first_response_ms: 0,
    low_sample: false,
    coverage: 'complete',
    last_observed_at: 1_700_000_000,
    has_failures: false,
    ...overrides,
  }
}
function model(
  name: string,
  overrides: Partial<AvailabilityModel> = {}
): AvailabilityModel {
  return {
    name,
    active: true,
    current: metric(),
    summary: metric(),
    buckets: [{ start: 1_699_999_000, end: 1_700_000_000, ...metric() }],
    ...overrides,
  }
}
function group(
  id: number,
  overrides: Partial<AvailabilityGroup> = {}
): AvailabilityGroup {
  return {
    id,
    name: `Group ${id}`,
    description: '',
    current: metric(),
    summary: metric({ success_rate: 72.5, first_response_ms: 123.5 }),
    buckets: [],
    models: [model(`model-${id}`)],
    ...overrides,
  }
}
function report(
  overrides: Partial<AvailabilityReport> = {}
): AvailabilityReport {
  return {
    range: '24h',
    window_start: 1_699_913_600,
    window_end: 1_700_000_000,
    current_start: 1_699_999_100,
    generated_at: 1_700_000_000,
    coverage_start: 1_699_900_000,
    refresh_seconds: 60,
    bucket_seconds: 3600,
    groups: [group(1)],
    ...overrides,
  }
}
async function renderFeature(node: React.ReactNode) {
  const i18n = createInstance()
  await i18n.init({
    lng: 'en',
    fallbackLng: 'en',
    resources: { en: { translation: {} } },
    interpolation: { escapeValue: false },
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  })
  clients.push(client)
  return render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={client}>
        <TooltipProvider>{node}</TooltipProvider>
      </QueryClientProvider>
    </I18nextProvider>
  )
}
function serveReports(
  resolve: (range: string) => AvailabilityReport | Promise<AvailabilityReport>
) {
  const adapter: AxiosAdapter = async (config) => ({
    data: {
      success: true,
      message: '',
      data: await resolve(String(config.params.range)),
    },
    status: 200,
    statusText: 'OK',
    headers: {},
    config,
  })
  api.defaults.adapter = adapter
}

test('ordinary users retain an availability entry even without traffic modules', async () => {
  function OrdinarySidebar() {
    const data = useSidebarData()
    const defaults = getDefaultSidebarModulesForTest()
    defaults.console = { enabled: false }
    const visible = filterNavGroupsByRole(
      filterSidebarNavGroupsForConfig(
        data.navGroups,
        defaults,
        { console: { enabled: false } },
        { console: false }
      ),
      1
    )
    return (
      <nav>
        {visible
          .flatMap((item) => item.items)
          .map((item) =>
            'url' in item && item.url ? (
              <a key={item.url} href={item.url}>
                {item.title}
              </a>
            ) : null
          )}
      </nav>
    )
  }
  const view = await renderFeature(<OrdinarySidebar />)
  assert.equal(
    view.getByRole('link', { name: 'Group availability' }).getAttribute('href'),
    '/availability'
  )
  assert.equal(view.queryByRole('link', { name: 'Usage Analytics' }), null)
  serveReports(() => report())
  const page = await renderFeature(<AvailabilityPage />)
  assert.ok(await page.findByRole('button', { name: 'Show models: Group 1' }))
})

test('keyboard expansion keeps one group open and aria controls resolve correctly', async () => {
  const view = await renderFeature(
    <GroupGrid groups={[group(1), group(2), group(3)]} />
  )
  const user = userEvent.setup({ document })
  const first = view.getByRole('button', { name: 'Show models: Group 1' })
  assert.equal(first.getAttribute('aria-expanded'), 'false')
  first.focus()
  await user.keyboard('{Enter}')
  assert.equal(first.getAttribute('aria-expanded'), 'true')
  const panel = document.getElementById(first.getAttribute('aria-controls')!)
  assert.ok(panel)
  assert.equal(panel.hidden, false)
  assert.ok(within(panel).getByText('model-1'))
  const second = view.getByRole('button', { name: 'Show models: Group 2' })
  await user.click(second)
  assert.equal(first.getAttribute('aria-expanded'), 'false')
  assert.equal(panel.hidden, true)
  assert.equal(second.getAttribute('aria-expanded'), 'true')
  second.focus()
  await user.keyboard(' ')
  assert.equal(second.getAttribute('aria-expanded'), 'false')
  assert.equal(view.queryByText('model-2'), null)
})

test('recent status is distinct from range metrics and zero latency remains valid', async () => {
  const view = await renderFeature(
    <MetricDetails
      current={metric({ success_rate: 90 })}
      summary={metric({
        state: 'unhealthy',
        success_rate: 72.5,
        has_failures: true,
      })}
    />
  )
  const recent = within(
    view.getByRole('region', { name: 'Recent status (15 minutes)' })
  )
  assert.ok(recent.getByText('Healthy'))
  assert.equal(recent.queryByText('72.5%'), null)
  const selected = within(view.getByRole('region', { name: 'Selected range' }))
  assert.ok(selected.getByText('72.5%'))
  assert.ok(selected.getByText('0 ms'))
  assert.ok(selected.getByText('Failures observed'))
})

test('historical model failures do not raise active model alerts but remain inspectable', async () => {
  const historical = model('retired-model', {
    active: false,
    current: metric({ state: 'unhealthy', success_rate: 0 }),
  })
  const view = await renderFeature(
    <GroupGrid
      groups={[group(1, { models: [model('current-model'), historical] })]}
    />
  )
  assert.equal(view.queryByText('Current models need attention'), null)
  fireEvent.click(view.getByRole('button', { name: 'Show models: Group 1' }))
  const history = within(
    view.getByRole('region', { name: 'Historical models' })
  )
  assert.ok(history.getByText('retired-model'))
  assert.ok(history.getByText('No longer offered'))
})

test('an active degraded model is not hidden by healthy group aggregation', async () => {
  const view = await renderFeature(
    <GroupGrid
      groups={[
        group(1, {
          models: [
            model('affected-model', {
              current: metric({ state: 'degraded', success_rate: 80 }),
            }),
            model('unobserved', {
              current: metric({ state: 'unknown', success_rate: null }),
            }),
          ],
        }),
      ]}
    />
  )
  assert.ok(view.getByText('Current models need attention'))
  assert.ok(view.getByText('affected-model'))
  assert.equal(view.queryByText('unobserved'), null)
  fireEvent.click(view.getByRole('button', { name: 'Show models: Group 1' }))
  assert.ok(view.getByText('unobserved'))
})

test('unknown and incomplete observations are distinguishable and never filled with 100 percent', async () => {
  const view = await renderFeature(
    <MetricDetails
      current={metric({
        state: 'incomplete',
        coverage: 'incomplete',
        success_rate: null,
      })}
      summary={metric({
        state: 'unknown',
        success_rate: null,
        first_response_ms: null,
        last_observed_at: null,
      })}
    />
  )
  assert.ok(view.getAllByText('Incomplete coverage').length > 0)
  assert.equal(view.queryByText('Healthy'), null)
  assert.equal(view.queryByText('100%'), null)
  const selected = within(view.getByRole('region', { name: 'Selected range' }))
  assert.equal(selected.getAllByText('—').length, 3)
})

test('availability bands distinguish relaxed thresholds without changing request success rates', () => {
  for (const [rate, band, state] of [
    [100, 'excellent', 'healthy'],
    [99, 'excellent', 'healthy'],
    [98.99, 'healthy', 'healthy'],
    [90, 'healthy', 'healthy'],
    [89.99, 'degraded', 'degraded'],
    [80, 'degraded', 'degraded'],
    [79.99, 'unhealthy', 'unhealthy'],
    [50, 'unhealthy', 'unhealthy'],
    [49.99, 'critical', 'unhealthy'],
    [0, 'critical', 'unhealthy'],
  ] as const) {
    const sample = metric({ success_rate: rate })
    assert.equal(observedBand(sample), band)
    assert.equal(observedState(sample), state)
    assert.equal(sample.success_rate, rate)
  }
  assert.equal(observedBand(metric({ success_rate: null })), 'unknown')
  assert.equal(
    observedBand(metric({ success_rate: 100, coverage: 'incomplete' })),
    'incomplete'
  )
})

test('a model meeting the ninety percent target does not raise an active model alert', async () => {
  const view = await renderFeature(
    <GroupGrid
      groups={[
        group(1, {
          models: [
            model('meets-target', {
              current: metric({ state: 'unhealthy', success_rate: 90 }),
            }),
          ],
        }),
      ]}
    />
  )
  assert.equal(view.queryByText('Current models need attention'), null)
})

test('history blocks expose exact intervals and open touch-accessible details', async () => {
  const view = await renderFeature(
    <HistoryTimeline
      buckets={[
        {
          start: 1_699_999_000,
          end: 1_700_000_000,
          ...metric({
            state: 'incomplete',
            coverage: 'incomplete',
            success_rate: null,
            low_sample: true,
          }),
        },
      ]}
    />
  )
  const trigger = view.getByRole('button', {
    name: /Incomplete coverage.*Service success rate: —.*Median first response: 0 ms.*Low sample/,
  })
  assert.match(trigger.getAttribute('aria-label')!, /2023/)
  const user = userEvent.setup({ document })
  await user.click(trigger)
  assert.ok(await view.findByRole('dialog'))
  assert.match(view.getByRole('dialog').textContent!, /Incomplete coverage/)
  await user.keyboard('{Escape}')
  await waitFor(() => assert.equal(view.queryByRole('dialog'), null))
})

test('range selection drives one report request and preserves the server summary', async () => {
  const ranges: string[] = []
  serveReports((range) => {
    ranges.push(range)
    return report({
      range: range === '7d' ? '7d' : '24h',
      groups: [
        group(1, {
          summary: metric({ success_rate: range === '7d' ? 88 : 72.5 }),
        }),
      ],
    })
  })
  const view = await renderFeature(<AvailabilityPage />)
  assert.ok(await view.findByText('72.5%'))
  fireEvent.click(view.getByRole('button', { name: '7 days' }))
  assert.ok(await view.findByText('88%'))
  assert.deepEqual(ranges, ['24h', '7d'])
  assert.equal(view.queryByText('72.5%'), null)
})

test('failed refresh retains the previous report and explicitly marks it old', async () => {
  let fail = false
  serveReports(() => {
    if (fail) throw new Error('Network unavailable')
    return report()
  })
  const view = await renderFeature(<AvailabilityPage />)
  assert.ok(await view.findByText('72.5%'))
  fail = true
  fireEvent.click(view.getByRole('button', { name: 'Refresh' }))
  assert.ok(
    await view.findByText('Refresh failed — showing older observations')
  )
  assert.ok(view.getByText('72.5%'))
  assert.ok(view.getByRole('button', { name: 'Show models: Group 1' }))
  fail = false
  fireEvent.click(view.getByRole('button', { name: 'Refresh' }))
  await waitFor(() =>
    assert.equal(
      view.queryByText('Refresh failed — showing older observations') === null,
      true
    )
  )
})

test('failed range change keeps the previous window explicitly labelled as retained', async () => {
  serveReports((range) => {
    if (range === '30d') throw new Error('Network unavailable')
    return report()
  })
  const view = await renderFeature(<AvailabilityPage />)
  assert.ok(await view.findByText('72.5%'))
  const windowBefore = view.getByText(/^Displayed window:/).textContent
  fireEvent.click(view.getByRole('button', { name: '30 days' }))
  assert.ok(
    await view.findByText('Refresh failed — showing older observations')
  )
  assert.equal(view.getByText(/^Displayed window:/).textContent, windowBefore)
  assert.ok(view.getByText('72.5%'))
})

test('hidden pages defer loading and refresh when visible again', async () => {
  const visibility = Object.getOwnPropertyDescriptor(
    document,
    'visibilityState'
  )
  let state = 'hidden'
  Object.defineProperty(document, 'visibilityState', {
    configurable: true,
    get: () => state,
  })
  const ranges: string[] = []
  serveReports((range) => {
    ranges.push(range)
    return report()
  })
  try {
    const view = await renderFeature(<AvailabilityPage />)
    assert.deepEqual(ranges, [])
    state = 'visible'
    fireEvent(document, new Event('visibilitychange'))
    assert.ok(await view.findByText('72.5%'))
    state = 'hidden'
    fireEvent(document, new Event('visibilitychange'))
    state = 'visible'
    fireEvent(document, new Event('visibilitychange'))
    await waitFor(() => assert.deepEqual(ranges, ['24h', '24h']))
  } finally {
    cleanup()
    if (visibility)
      Object.defineProperty(document, 'visibilityState', visibility)
    else Reflect.deleteProperty(document, 'visibilityState')
  }
})

test('unmount cancels an in-flight report through its query signal', async () => {
  let aborted = false
  let started = false
  api.defaults.adapter = async (config) => {
    started = true
    const pending = Promise.withResolvers<never>()
    config.signal?.addEventListener?.('abort', () => {
      aborted = true
      pending.reject(new Error('Cancelled'))
    })
    return pending.promise
  }
  const view = await renderFeature(<AvailabilityPage />)
  await waitFor(() => assert.equal(started, true))
  view.unmount()
  await waitFor(() => assert.equal(aborted, true))
})
