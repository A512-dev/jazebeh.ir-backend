# `campaign_execution_repository.go`

This repository persists campaign execution work and its lifecycle state. It is used by scheduling/execution flows to create or retrieve execution records, claim due work, and record success or failure.

Claim/update methods use guarded state transitions so stale workers cannot overwrite a newer claim. Where the execution process uses targeting calculations or reservations, coordinate those operations through the business flow and pass the same transaction context when atomicity is required. This repository represents durable execution state; delivery-provider calls and campaign policy live elsewhere.
## Locking and recovery functions

`transactionForLock` obtains the transaction from context and rejects calls without one. `LockCampaignForUpdate`, `LockBundleForUpdate`, `LockBundleForShare` and `LockWalletForUpdate` acquire the corresponding row locks within that transaction. Wallet locking matters when separate campaigns mutate the same balance.

`ReleaseUnpreparedCampaign` settles one failed scheduler run. `RecoverStaleCampaignRuns` processes stale running claims and returns `CampaignExecutionRecoveryResult`, distinguishing requeued and interrupted campaigns. Their shared `recoverUndeliveredCampaigns` transaction retires checkpoints and uses `campaignHasDeliveryRecordSQL` to detect durable send intent across attempts. With no intent the campaign can be requeued; with intent it is interrupted to avoid replaying a potentially successful provider call. `TouchRunningCampaign` updates the running heartbeat and returns `ErrCampaignExecutionClaimLost` when the claim is no longer running.

`BundleCampaignAllocation` and `BundleActiveTestReservation` are compact ordered records for capacity deductions and fingerprints. `ListBundleCampaignAllocations` reads bundle allocations, excluding explicitly reusable Excel targeting. `ListBundleActiveTestReservations` and `ListBundleActiveExecutionReservations` expose active reservations of other campaigns for snapshot invalidation. `bundleCampaignAllocationsQuery` and `bundleActiveTestReservationsQuery` construct their shared queries.
