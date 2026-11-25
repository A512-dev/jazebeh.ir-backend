# `campaign_refund_reconciliation_repository.go`

This repository manages reconciliation jobs for campaign refunds. `DiscoverPending` finds campaigns that need reconciliation and ensures work is represented durably; `ClaimPending` leases a batch for a worker.

Workers finish through `Complete`, schedule another attempt through `Retry`, or move a case to `ManualReview`. These transitions are tied to the claim timestamp so a stale worker cannot overwrite a newer lease. Error messages are truncated before persistence. The repository does not itself issue refunds; it coordinates which campaigns need reconciliation and records their processing outcome.
