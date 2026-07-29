import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import { subscriptionQueryKeys } from '@/features/subscriptions/query-keys'
import type {
  PlanChannelTokenEquivalent,
  PublicPlanRecord,
} from '@/features/subscriptions/types'
import {
  getHomePublicPlansQueryKey,
  renderHomePlanChannelEquivalentLabels,
} from './plans-preview'

type PublicPlanRecordForTest = PublicPlanRecord
type PlansPreviewModule = {
  HOME_PLANS_PREVIEW_LIMIT?: number
  selectHomePlanRecords?: (
    records?: readonly unknown[]
  ) => PublicPlanRecordForTest[]
  hasMoreHomePlans?: (records?: readonly unknown[]) => boolean
}

async function loadPlansPreviewModule(): Promise<PlansPreviewModule> {
  try {
    return (await import('./plans-preview')) as unknown as PlansPreviewModule
  } catch {
    return {}
  }
}

function record(id: number, publicVisible = true): PublicPlanRecordForTest {
  return {
    plan: {
      id,
      title: `Plan ${id}`,
      subtitle: '',
      price_amount: id,
      currency: 'CNY',
      duration_unit: 'month',
      duration_value: 1,
      custom_seconds: 0,
      monthly_token_limit: 1000,
      concurrency_limit: 1,
      queue_capacity: 0,
      gpt_abuse_warning_limit: 0,
      public_visible: publicVisible,
    },
  }
}

describe('home subscription query key', () => {
  test('uses a public plans cache key distinct from wallet plans', () => {
    assert.deepEqual(subscriptionQueryKeys.homePublicPlans, [
      'home',
      'subscription-public-plans',
    ])
    assert.notDeepEqual(
      subscriptionQueryKeys.homePublicPlans,
      subscriptionQueryKeys.walletPlans
    )
  })

  test('exports the public plans query key used by the home preview', () => {
    assert.deepEqual(
      getHomePublicPlansQueryKey(),
      subscriptionQueryKeys.homePublicPlans
    )
  })
})

describe('home plan channel equivalent preview', () => {
  const t = (key: string, values?: Record<string, unknown>) => {
    const translated = key === 'about' ? 'about' : key
    return translated.replace(/{{(\w+)}}/g, (_match, name: string) =>
      String(values?.[name] ?? '')
    )
  }

  test('renders at most two channel equivalents with overflow', () => {
    const equivalents: PlanChannelTokenEquivalent[] = [
      {
        kind: 'single',
        channel_type: 1,
        channel_type_name: 'OpenAI',
        variant_count: 1,
        multiplier: 2,
        equivalent_token_limit: 500_000,
      },
      {
        kind: 'range',
        channel_type: 14,
        channel_type_name: 'Claude',
        variant_count: 2,
        min_multiplier: 1.5,
        max_multiplier: 2,
        equivalent_token_limit_min: 500_000,
        equivalent_token_limit_max: 666_666,
      },
      {
        kind: 'unlimited',
        channel_type: 24,
        channel_type_name: 'Gemini',
        variant_count: 1,
        token_unlimited: true,
      },
    ]
    const plan: PublicPlanRecordForTest = {
      plan: { ...record(1).plan, channel_token_equivalents: equivalents },
    }

    assert.deepEqual(renderHomePlanChannelEquivalentLabels(plan, t), [
      'OpenAI: about 500K tokens',
      'Claude: about 500K tokens - 666.67K tokens',
      '+1 more',
    ])
  })

  test('suppresses all 1.0 channel equivalents', () => {
    const equivalents: PlanChannelTokenEquivalent[] = [
      {
        kind: 'single',
        channel_type: 1,
        channel_type_name: 'OpenAI',
        variant_count: 1,
        multiplier: 1,
        equivalent_token_limit: 1_000_000,
      },
    ]
    const plan: PublicPlanRecordForTest = {
      plan: { ...record(1).plan, channel_token_equivalents: equivalents },
    }

    assert.deepEqual(renderHomePlanChannelEquivalentLabels(plan, t), [])
  })
})

describe('home plans preview selection', () => {
  test('keeps backend order and limits to three visible plans', async () => {
    const mod = await loadPlansPreviewModule()

    assert.equal(mod.HOME_PLANS_PREVIEW_LIMIT, 3)
    assert.equal(typeof mod.selectHomePlanRecords, 'function')

    const selected = mod.selectHomePlanRecords?.([
      record(5),
      record(4),
      record(3),
      record(2),
    ])

    assert.deepEqual(
      selected?.map((item) => item.plan.id),
      [5, 4, 3]
    )
  })

  test('filters hidden and malformed records without type assertions', async () => {
    const mod = await loadPlansPreviewModule()

    assert.equal(typeof mod.selectHomePlanRecords, 'function')

    const selected = mod.selectHomePlanRecords?.([
      record(3, false),
      null,
      undefined,
      {},
      record(2),
    ])

    assert.deepEqual(
      selected?.map((item) => item.plan.id),
      [2]
    )
  })

  test('detects when more visible plans are available after filtering', async () => {
    const mod = await loadPlansPreviewModule()

    assert.equal(typeof mod.hasMoreHomePlans, 'function')

    assert.equal(
      mod.hasMoreHomePlans?.([record(4), record(3), record(2), record(1)]),
      true
    )
    assert.equal(
      mod.hasMoreHomePlans?.([
        record(4),
        record(3),
        record(2),
        record(1, false),
      ]),
      false
    )
  })
})
