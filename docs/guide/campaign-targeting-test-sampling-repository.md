# `campaign_targeting_test_sampling_repository.go`

This repository persists smart-targeting test-sampling calculations, including their input snapshot and worker status. It provides lookup by ID/campaign/input, latest and active calculation queries, and pending-work claiming.

Workers complete, fail, or supersede calculations through guarded state transitions. `Complete` checks that the calculation is still in the expected state; a concurrent change returns `ErrCampaignTargetingTestSamplingStateConflict`. The `LatestCalculatedByInput` query is useful when callers need completed output matching the exact input rather than merely the newest row.

Sampling policy and candidate scoring are handled by the business flow and audience-selection repository; this repository stores and coordinates the durable calculation lifecycle.
