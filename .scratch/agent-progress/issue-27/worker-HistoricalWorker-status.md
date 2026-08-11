# HistoricalWorker 状态

## 当前状态

Timed order recovery now fails closed unless an authoritative order-linked event window is present. `timedHistoricalOrderSource` no longer falls back to enclosing subscription `StartTime/EndTime`; successful orders without a uniquely persisted event window report `timed_event_window_missing` and create no grant. This prevents renewal orders from being incorrectly represented by one merged subscription window.

Credit historical backfill remains restricted to explicit `entitlement_type=credit_balance`, stable ledger identity deduplication, integer micros, conservative estimated/unknown outcomes, SQLite-safe locking, dry-run zero writes, atomic apply, and non-overwrite of existing states.

Timed redemption recovery may use event_start/end carried in its fulfillment snapshot; missing or invalid windows remain unknown/ambiguous. Existing grants remain immutable and are not rewritten.

## Files changed

- `model/credit_valuation_backfill.go`
- `model/credit_valuation_backfill_test.go`
- `model/timed_subscription_valuation_backfill.go`
- `model/timed_subscription_valuation_backfill_test.go`
- `.scratch/agent-progress/issue-27/worker-HistoricalWorker-status.md`

## Verification and risks

- LSP diagnostics for the timed backfill were rechecked after the fail-closed edit and returned OK.
- Per assignment, formatter, lint, build, tests, and project-level commands were not run.
- The repository has no verified order-linked event-window field on `SubscriptionOrder`; orders lacking such a record now intentionally downgrade to unknown rather than guessing. Coordinator should adjust fixtures to include an authoritative window source before acceptance.
