# Smart Targeting campaign-lifecycle integrations

This guide documents the Smart Targeting behavior implemented outside the
dedicated configuration/calculation flows. It covers the handoffs from sender
configuration through customer finalization, administrator approval, bot
readiness, and the repository projections/allocation inputs that preserve a
frozen audience decision.

Read [SMART_TARGETING_CONFIGURATION_AND_CALCULATIONS_README.md](SMART_TARGETING_CONFIGURATION_AND_CALCULATIONS_README.md)
first for tag selection, capacity generations, Test sampling, Execution
calculations, and reservation repositories. Read
[CUSTOMER_CAMPAIGN_LIFECYCLE_README.md](CUSTOMER_CAMPAIGN_LIFECYCLE_README.md)
for the broader campaign, wallet, delivery, and refund lifecycle.

## Boundary map

```text
sender line/provider ──> allowed colours ──> calculation input/hash
                                                 |
customer create/update ─> ordered tag selection  |
                         └> preview invalidation  v
                                      capacity / Test preview / Execution calculation
                                                           |
customer finalization ─> frozen Test or Execution reservation
                                                           |
admin approval ────────> validates reservation + consumes frozen budget
                                                           |
bot readiness / scheduler ─> exposes and materializes frozen audience
```

The critical rule is that a campaign may change editable intent only before
finalization. After finalization, dispatch must use the persisted Test sample
or Execution reservation; it must not select a replacement audience from live
profile data.

## 1. Sender configuration is calculation input

Source: [smart_targeting_delivery_eligibility.go](smart_targeting_delivery_eligibility.go).

`smartTargetingAllowedColorsForCampaign` derives Smart Targeting audience
colours from the effective sender rather than trusting a caller-supplied value:

1. It starts with PayamSMS-compatible behavior.
2. If the campaign specifies a line number, it loads that line through
   `LineNumberRepository.ByValue`.
3. A configured Candoo SMS line changes the provider rule to black-only;
   missing, retired, or non-Candoo lines retain the historical behavior.
4. `models.SmartTargetingAllowedColors` also considers the campaign platform.

Capacity, Test sampling, and Execution calculations persist allowed colours in
their input snapshots/hashes. Updating a line number or changing its provider
can therefore make an earlier capacity or preview stale; it is not merely a
delivery-time preference.

## 2. Customer creation, edits, cost, and finalization

Source: [campaign_flow.go](campaign_flow.go).

`CampaignFlowImpl` is the customer-facing integration point for Smart
Targeting. The specialized flows calculate durable results, but this flow
decides when they are required and turns them into campaign commitments.

| Lifecycle point | Smart Targeting behavior |
| --- | --- |
| Create/update | Resolves the targeting method, validates bundle/phase/sample-size rules, and persists ordered selected tags through `CampaignSelectedTagRepository.Replace`. A meaningful input change invalidates the Test preview and supersedes obsolete sampling work. |
| Capacity and price | Uses `EnsureCurrentSmartTargetingCapacity`; an absent or stale exact capacity produces a typed pending/required result instead of using an estimate. Test price/count derives from the satisfied immutable preview; Execution count is budget-derived and cannot exceed current exact capacity. |
| Test finalization | Requires a current, satisfied preview and frozen intent. Under the campaign and Bundle locks, it calls `CampaignTargetingTestSampleSelectionRepository.ReserveForCampaign`; the exact preview members become an active reservation before money/status publication. |
| Execution finalization | Requires the caller-named ready calculation. It locks and verifies the calculation and members, compares the allocation fingerprint, releases any Test reservation, reserves the Execution audience, marks the calculation committed, and sets the reservation schema version on the campaign. |
| Failure/terminal paths | Maps reservation conflicts/staleness to stable business errors and records Execution-finalization telemetry. Cancellation, rejection, expiry, or editable reconfiguration releases/invalidates the relevant reservations rather than leaving audience holds behind. |

Finalization deliberately separates expensive input/cost calculation from the
short money/commit transaction. The commit transaction re-locks and rechecks
the campaign and Bundle so a stale calculation cannot reserve recipients after
the user has changed the underlying intent.

## 3. Approval revalidates a frozen Test audience

Source: [campaign_admin_flow.go](campaign_admin_flow.go).

`ApproveCampaign` does not treat a `waiting-for-approval` Test campaign as
sufficient proof that it has a sendable Smart Targeting audience. In its
transaction it:

1. locks and rereads the campaign, then locks the Bundle audience ledger for
   non-Excel targeting;
2. resolves the current-or-frozen Test sampling intent and requires an active
   Test selection ID;
3. loads `ActiveReservedForCampaign` and requires its member count to equal
   the frozen preview’s effective audience count; and
4. only then consumes the frozen budget and makes the campaign approved.

This catches a preview/reservation that was removed or changed while the
campaign waited for approval. Administrative rejection/cancellation paths are
also lifecycle owners for releasing Smart Targeting Test and Execution holds.

## 4. Bot readiness exposes frozen, not inferred, intent

Source: [campaign_bot_flow.go](campaign_bot_flow.go).

`ListReadyCampaigns` is the scheduler-facing handoff. For every Smart
Targeting campaign it validates the persisted selected-tag set. For Test
campaigns it additionally resolves the current-or-frozen sampling intent and
returns:

- the satisfied tag IDs;
- the active immutable Test selection ID; and
- the effective preview audience count, rather than a mutable campaign default.

This prevents a delivery worker from treating an incomplete or invalid current
selection as ready. The same flow carries the Execution reservation schema
version so the scheduler can choose the reservation-aware, fail-closed
materialization path instead of the explicitly limited legacy fallback.

## 5. Repository contracts that preserve the decision

### Campaign projection

Source: [campaign_repository.go](../repository/campaign_repository.go).

The standard campaign projection includes Test-preview fields, the active Test
selection ID, and `smart_targeting_execution_reservation_version`. Flows often
load a campaign and later write status or other fields. Omitting these columns
from that projection would cause a full-row GORM update to write zero values,
disconnecting a valid frozen sample or Execution reservation. Any new campaign
projection used in a mutating flow must preserve these fields.

### Shared Bundle allocation accounting

Source: [campaign_execution_repository.go](../repository/campaign_execution_repository.go).

`bundleCampaignAllocationsQuery` derives demand for active Bundle campaigns.
It normalizes the targeting method from campaign JSON, recognizes `smart`, and
excludes Excel-targeted campaigns from this shared live-audience ledger.
Capacity/fingerprint logic consumes the result together with materialized
selections and active reservations. This is why Smart Targeting calculations
cannot reason about their candidate population in isolation: approved/running
Bundle work also changes availability.

### Test-performance input to tag choice

Source: [tag_test_performance_repository.go](../repository/tag_test_performance_repository.go).

This repository discovers and recomputes Test-performance reports only for
attributable Smart Targeting Test/Execution campaigns. It uses durable
lease-guarded work and stores metrics that
`CampaignSelectedTagRepository.ListAvailable` joins into the tag catalog.
Consequently, Test performance can influence tag ordering/selection, but it is
not a substitute for the current capacity, preview, or reservation checks made
at finalization.

## 6. Reservation-finalization observability

Source: [smart_targeting_execution_reservation_metrics.go](smart_targeting_execution_reservation_metrics.go).

`recordSmartTargetingExecutionReservationFinalizationFailure` increments
`smart_targeting_execution_reservation_finalization_failures_total` only with a
bounded reason label. It classifies repository errors as insufficient capacity,
conflict, concurrency, stale, corrupt, missing, unavailable, or `other`.

Use this metric with calculation queue age, lease-reclaim counts, and Bundle
lock wait time. It distinguishes normal availability races from corrupt or
missing durable reservation state without placing campaign IDs or raw error
messages into metric labels.

## Review checklist

- Does every changed campaign input invalidate the exact preview/calculation it
  makes obsolete?
- Does finalization lock and revalidate both the campaign and Bundle before
  freezing money and reserving recipients?
- Does approval require the persisted Test reservation, not just a preview
  status field?
- Does the bot/scheduler receive the immutable selection/reservation identity?
- Do campaign read projections preserve Smart Targeting state before any
  full-row update?
- Are allocation-fingerprint and reservation failures visible through bounded
  metrics and returned as retryable/stable business errors where appropriate?

## Related environment variables

- `SMART_TARGETING_CAPACITY_SCHEDULER_ENABLED`
- `SMART_TARGETING_TEST_SAMPLING_SCHEDULER_ENABLED`, `SMART_TARGETING_TEST_SAMPLING_MAX_PARALLEL_RUNS`
- `SMART_TARGETING_EXECUTION_CALCULATION_SCHEDULER_ENABLED`, `SMART_TARGETING_EXECUTION_CALCULATION_MAX_PARALLEL_RUNS`
- `CAMPAIGN_EXECUTION_ENABLED`, `CAMPAIGN_EXECUTION_INTERVAL`, `CAMPAIGN_EXECUTION_MAX_PARALLEL_RUNS`
