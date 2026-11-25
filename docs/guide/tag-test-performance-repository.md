# `tag_test_performance_repository.go`

This repository derives tag performance from completed test campaigns and materializes campaign-, phase-, and bundle-level read models. It discovers campaigns needing work, claims them for a worker, recomputes campaign performance under a lease, and records failures.

Recomputation attributes completed test-campaign delivery outcomes to tags, refreshes derived summaries, and removes stale campaign/tag performance rows that are no longer supported by current source data. Lease checks protect the derived state from stale workers. The resulting metrics are consumed by campaign tag selection and bundle evaluation; this is a derived projection, not the authoritative campaign delivery record.

The historical name is misleading: the repository handles smart-targeted test and execution phases. It does not lock click or short-link rows.

## Types and setup

`TagTestPerformanceRepository` defines the worker API; `TagTestPerformanceRepositoryImpl` holds the GORM database. `NewTagTestPerformanceRepository` constructs it, and `getDB` uses a transaction in `TxContextKey` if present. `attributableCampaign` carries campaign, bundle, phase and platform from validation. `tagTestPerformanceSourceSQL` holds sent/delivered SQL fragments, with `tagTestPerformanceSources` selecting the appropriate platform tables.

`ErrTagTestPerformanceLeaseLost` means the worker no longer owns the report. `ErrTagTestPerformanceCampaignInvalid` indicates a campaign cannot be attributed or yielded no aggregate rows.

## Worker methods

- `DiscoverPending(ctx, at)` locks scheduler state and, in the same transaction, enqueues affected smart-targeted campaigns and advances click and source-scan cursors. It includes missing reports, even zero-click backfills, and detects new clicks, processed-campaign/status-job changes and version changes. A five-minute overlap helps catch delayed updates. A preparing report stays preparing while its request time is updated.
- `ClaimPending(ctx, limit, staleBefore, at)` uses `FOR UPDATE SKIP LOCKED` to claim current-version unprepared reports, due retryable failures and stale preparing reports. It orders by request time and campaign ID, increments attempts and sets `started_at` as the lease token. A nonpositive limit becomes one.
- `RecomputeCampaign(ctx, campaignID, leaseStartedAt, at)` locks the matching preparing report in a transaction. A changed lease fails; a changed calculation version releases the row without writing old-version metrics. It validates campaign and platform, aggregates complete history, deletes stale campaign/tag rows and takes a per-bundle advisory transaction lock before finishing and refreshing summaries. A request received after the lease started causes another pass. Errors roll back the transaction.
- `Fail(ctx, campaignID, leaseStartedAt, code, message, retryAt, at)` marks only the matching lease failed, truncating code to 64 bytes and message to 255 bytes. Callers must supply sanitized text. A nil retry time prevents claiming until another discovery request.

## Aggregation helpers

`recomputeCampaignTagPerformanceSQLForPlatform` selects constant SQL fragments for SMS, Bale, SPlus or Rubika and rejects unsupported platforms. Attribution supplies one assigned tag per audience. Sent and delivered counts use distinct attributed audiences matched by selection and phone number; delivery requires positive total parts and all parts delivered. Clicks count distinct attributed audiences with a UID and qualifying non-automated traffic. The query upserts counts and tag snapshots. `recomputeCampaignTagPerformanceArgs` supplies its positional arguments; `recomputeCampaignTagPerformanceSQL` is the SMS version retained for SQL-shape tests.

`deleteStaleCampaignTagPerformances` removes unsupported rows. `refreshTagTestPhaseSummary` aggregates test-phase rows per bundle/tag, while `refreshTagOverallPerformanceSummary` aggregates all phases; each also deletes obsolete summaries. The SQL constants define discovery, claiming and these projection updates. Summary refresh is serialized per bundle while campaign aggregation can run concurrently.
