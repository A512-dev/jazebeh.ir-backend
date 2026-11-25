# `campaign_targeting_capacity_repository.go`

This repository stores asynchronous smart-targeting capacity calculations. It supports saving a calculation, retrieving by ID/campaign/input, finding the current result for a phase, and worker lifecycle operations.

`ClaimPending` leases due work to a worker. `Complete` and `Fail` update only the expected calculation state/lease; if that state changed concurrently, the repository reports `ErrCampaignTargetingCapacityStateConflict`. `Supersede` marks obsolete calculations, while `ActiveByCampaignID`, `LatestByCampaignID`, `LatestByInput`, and `LatestCalculatedByInput` let flows distinguish current work from usable completed output.

The repository owns persistence and state transitions. The scheduler/flow decides when to enqueue or consume a calculation.
