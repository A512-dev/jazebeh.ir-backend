# `campaign_targeting_execution_reservation_repository.go`

This repository manages audience reservations for smart-targeting execution calculations. Reservations keep candidate IDs from being consumed by overlapping calculations while work is pending and help make capacity checks consistent.

Its operations create or materialize reservations for a calculation, inspect active reservations, and release them when work is completed, superseded, or abandoned. State changes are guarded and database-backed so concurrent schedulers cannot silently treat the same reserved audience as free. The execution/capacity flow owns policy decisions; this repository enforces persistence and reservation lifecycle.
# Detailed types and operations

`CampaignTargetingExecutionReservationRepository` defines the reservation API; `CampaignTargetingExecutionReservationRepositoryImpl` holds a GORM database. `NewCampaignTargetingExecutionReservationRepository` constructs it, and `getDB` honors a transaction in `TxContextKey`.

`ExecutionReservationRequestSnapshot` persists selected tag IDs, score classes, allowed colors and platform so recovery does not depend on mutable campaign JSON. `CampaignTargetingExecutionReservationSnapshot` combines a header and ordered members. Errors distinguish unavailable input, missing reservation, conflicting availability, insufficient capacity, stale state, corrupt persisted data and concurrent change. The insufficient-capacity sentinel is declared here for callers; this file does not compute capacity.

- `ReserveForCampaign(ctx, campaign, header, rows)` is intended for the short finalization transaction under the bundle lock. It validates scope, versions, count, sequential order and unique positive audience IDs. An existing active reservation is accepted only if metadata and ordered audience/tag pairs match; otherwise it reports stale state or concurrency. For a new reservation it locks profile rows in ID order, checks their scores, and rechecks phone, selected and assigned tags, allowed colors, bundle selections, exclusions and active test/execution reservations before inserting the header and members in batches. PostgreSQL uniqueness violations become concurrency or conflict errors.
- `ActiveReservedForCampaign(ctx, campaignID, expected)` requires a transaction, locks the active header and members, and validates versions, counts, order and uniqueness. If no legacy header exists, `committedCalculationSnapshot` adapts the latest committed execution calculation and immutable members into the same shape, without profile lookups.
- `ValidateMaterializedForCampaign(ctx, campaignID, bundleID, expected, audienceIDs)` requires a transaction and verifies that an existing selection exactly matches the ordered materialized reservation. It also supports the committed-calculation representation. Mismatches are corrupt data.
- `HasActiveRowsForCampaign` checks for active legacy members or a committed calculation. `Materialize` validates the active snapshot and transitions legacy header and members to materialized, checking affected counts. For a committed calculation it is a no-op because the permanent selection owns the claim.
- `ReleaseForCampaign` and `InvalidateForCampaign` call `transitionForCampaign`: it locks the campaign and relevant bundles, marks committed calculations stale, releases active legacy members and marks active headers released or stale respectively. A zero campaign ID is a no-op.

## Internal validation

`validExecutionReservationHeader`, `validateExecutionReservationRows`, and `decodeExecutionReservationSnapshot` check header metadata, member shape and snapshot JSON. `activeSnapshot` locks and validates stored members. `lockAndValidateExecutionReservationScores` compares the selected score snapshot against locked profiles; `sameExecutionReservationScore` compares float bits, including nil handling. `executionReservationInsertError` translates uniqueness errors. `executionReservationAvailabilityQuery` checks the exact assigned tag for each member, not merely any selected tag.

The score check has a documented limitation: changes to other profiles can shift percentile boundaries and reclassify a selected member even if that member's own score is unchanged. The current check does not recompute those boundaries.
