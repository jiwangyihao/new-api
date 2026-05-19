import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

type PublicPlanRecordForTest = {
  plan: {
    id: number
    title: string
    subtitle: string
    price_amount: number
    currency: string
    duration_unit: string
    duration_value: number
    custom_seconds: number
    monthly_token_limit: number
    concurrency_limit: number
    public_visible: boolean
  }
}

type PlansPreviewModule = {
  HOME_PLANS_PREVIEW_LIMIT?: number
  selectHomePlanRecords?: (records?: readonly unknown[]) => PublicPlanRecordForTest[]
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
      public_visible: publicVisible,
    },
  }
}

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
