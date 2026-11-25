# `campaign_targeting_capacity_repository.go`

This repository stores asynchronous smart-targeting capacity calculations. It supports saving a calculation, retrieving by ID/campaign/input, finding the current result for a phase, and worker lifecycle operations.

`ClaimPending` leases due work to a worker. `Complete` and `Fail` update only the expected calculation state/lease; if that state changed concurrently, the repository reports `ErrCampaignTargetingCapacityStateConflict`. `Supersede` marks obsolete calculations, while `ActiveByCampaignID`, `LatestByCampaignID`, `LatestByInput`, and `LatestCalculatedByInput` let flows distinguish current work from usable completed output.

The repository owns persistence and state transitions. The scheduler/flow decides when to enqueue or consume a calculation.
## Types and methods

`CampaignTargetingCapacityRepository` defines the durable calculation lifecycle; `CampaignTargetingCapacityRepositoryImpl` holds the database. `NewCampaignTargetingCapacityRepository` constructs it, and `getDB` uses a transaction from `TxContextKey` when supplied. Audience scanning and snapshot validation are handled by the flow.

- `Save` inserts a calculation. `ByID`, `LatestByCampaignID`, `ActiveByCampaignID`, `LatestByInput` and `LatestCalculatedByInput` retrieve the relevant row; absent rows return `(nil, nil)` and latest lookups use descending creation time and ID.
- `CurrentForPhase` accepts only test or execution phase and finds an unexpired calculated row matching campaign, bundle, platform, version and the exact tag, score-class and color arrays. It requires bundle exclusions to have been applied. Invalid phase is an error; no matching snapshot returns nil.
- `ClaimPending` atomically claims calculating rows that have no start time or whose lease is stale, ordered by creation time and ID using `FOR UPDATE SKIP LOCKED`. A nonpositive limit becomes one. `started_at` identifies the lease.
- `Complete` saves raw, eligible, deduction and usable counts and the allocation fingerprint, then marks the matching calculating lease calculated. `Fail` records a failure against the same lease. `Supersede` fails a calculating row by state without a lease token. Each returns `ErrCampaignTargetingCapacityStateConflict` if its guarded update affects no row.
