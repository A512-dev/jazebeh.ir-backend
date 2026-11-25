# `campaign_targeting_test_sampling_repository.go`

This repository persists smart-targeting test-sampling calculations, including their input snapshot and worker status. It provides lookup by ID/campaign/input, latest and active calculation queries, and pending-work claiming.

Workers complete, fail, or supersede calculations through guarded state transitions. `Complete` checks that the calculation is still in the expected state; a concurrent change returns `ErrCampaignTargetingTestSamplingStateConflict`. The `LatestCalculatedByInput` query is useful when callers need completed output matching the exact input rather than merely the newest row.

Sampling policy and candidate scoring are handled by the business flow and audience-selection repository; this repository stores and coordinates the durable calculation lifecycle.
# Detailed API and queue behavior

`CampaignTargetingTestSamplingRepository` defines the API and `CampaignTargetingTestSamplingRepositoryImpl` stores the database. `NewCampaignTargetingTestSamplingRepository` constructs it; `getDB` uses a context transaction when present. The expensive audience scan and allocation happen in the business flow.

- `Save(ctx, row)` inserts a calculation. `ByID` returns `(nil, nil)` when absent.
- `LatestByCampaignID` finds the newest campaign calculation; `ActiveByCampaignID` restricts it to `calculating`. `LatestByInput` filters by campaign and input hash; `LatestCalculatedByInput` additionally requires `calculated`. The shared `latest` helper orders by creation time and ID descending and returns `(nil, nil)` for no match.
- `Supersede(ctx, id, code, message, at)` changes a calculating row to failed with a reason and finish time. It checks state but does not require a lease token.
- `ClaimPending(ctx, limit, staleBefore, at)` claims calculating rows with no start time or a stale start time. `FOR UPDATE SKIP LOCKED` supports concurrent workers; ordering is by creation time and ID. A nonpositive limit becomes one, and the new `started_at` is the lease token.
- `Complete(ctx, id, leaseStartedAt, tagResults, satisfiedTagCount, effectiveAudienceCount, campaignCost, allocationFingerprint, at)` requires a canonical lowercase 64-character SHA-256 hex digest. It saves results, counts, cost and fingerprint and marks the matching lease calculated. `completeQuery` casts results to `jsonb` and the decimal string form of cost to `numeric`, supporting the full `uint64` range.
- `Fail(ctx, id, leaseStartedAt, code, message, at)` records failure only for the matching calculating lease.

`Supersede`, `Complete` and `Fail` return `ErrCampaignTargetingTestSamplingStateConflict` when the guarded update affects no row. A reclaimed worker therefore cannot publish completion or failure with its old lease. Validation and database errors are returned separately.
