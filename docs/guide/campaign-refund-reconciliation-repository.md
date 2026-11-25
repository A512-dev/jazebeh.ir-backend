# `campaign_refund_reconciliation_repository.go`

This repository manages reconciliation jobs for campaign refunds. `DiscoverPending` finds campaigns that need reconciliation and ensures work is represented durably; `ClaimPending` leases a batch for a worker.

Workers finish through `Complete`, schedule another attempt through `Retry`, or move a case to `ManualReview`. These transitions are tied to the claim timestamp so a stale worker cannot overwrite a newer lease. Error messages are truncated before persistence. The repository does not itself issue refunds; it coordinates which campaigns need reconciliation and records their processing outcome.
# Detailed API and queue behavior

`CampaignRefundReconciliationRepository` defines queue operations; `CampaignRefundReconciliationRepositoryImpl` stores the GORM database. `NewCampaignRefundReconciliationRepository` constructs it and `getDB` honors a transaction in `TxContextKey`. The actual accounting decision and refund occur outside this repository.

- `DiscoverPending(ctx, at, batchSize)` performs a bounded historical backfill of executed campaigns, defaulting to 250 for a nonpositive size. It locks the singleton cursor, inserts missing jobs and advances the cursor in one transaction. A database trigger enqueues future executions. An invalid or missing `schedule_at` becomes a manual-review job instead of blocking the scan; existing jobs are unchanged.
- `ClaimPending(ctx, limit, staleBefore, eligibleBefore, at, leaseExpiresAt)` atomically claims eligible pending/retry jobs whose next attempt is due, or processing jobs whose lease expired before `staleBefore`. It orders by eligibility and campaign ID and uses `FOR UPDATE SKIP LOCKED`. It increments attempts and sets the lease times. Nonpositive limit becomes one.
- `Complete(ctx, campaignID, startedAt, at)` marks a matching processing lease completed. `Retry` sets it to retry with a next attempt and error details. `ManualReview` records an error and stops automatic retries. All clear the lease expiry.

These transitions filter on processing state and exact `startedAt`, protecting a newer lease from an old worker. They return database errors but do **not** check affected-row count, so a lost lease is a silent no-op. `truncateCampaignRefundReconciliationError` trims whitespace and limits the message to 1024 bytes without splitting a UTF-8 rune.

`ErrCampaignRefundReconciliationNoop` identifies a terminal case with no refund due; `ErrCampaignRefundReconciliationManualReview` signals inconsistent accounting state that should not be retried automatically. They communicate decisions made by callers.
