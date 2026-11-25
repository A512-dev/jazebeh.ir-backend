# `bundle_action_repository.go`

`BundleActionRepositoryImpl` manages uploaded UID action files for bundles, their processing leases, per-UID validation results, and materialized action metrics. It uses its own context-aware DB helper and PostgreSQL-specific SQL. The processing methods are designed for workers that may retry or run concurrently.

## File lifecycle and worker leases

- `Create` stores a file record; `ByID` retrieves one; `List` lists bundle files.
- `RequestDelete`, `PromotePending`, and `DiscardStaged` handle deletion requests and staged uploads.
- `ClaimPending` claims work in batches and marks files as processing with a timestamp lease. Stale processing work can be reclaimed according to the supplied stale threshold.
- `Fail` records a bounded error code/message only if the file still has the expected processing status and lease timestamp.
- `Complete` checks the lease under a row lock, transitions the file to processed (or completes a pending deletion), and refreshes bundle metrics in the same transaction.

If a worker's status or lease timestamp no longer matches, mutating operations return `ErrBundleActionFileLeaseLost`. This prevents an old worker from overwriting newer work.

## Persisting parsed UIDs

`StoreUIDsAndResult` verifies and locks the exact processing lease, replaces the file's UID snapshot, and inserts UID rows in chunks of 1,000. It classifies each UID based on whether it belongs to the bundle, has an assigned campaign tag, and has a fully delivered message in one of the supported channels (SMS, Bale, Rubika, or Splus). It also stores parser totals and counts for duplicates, new UIDs, and validation outcomes.

This persistence is transactional: the lease check, UID replacement/inserts, classification, and summary update either complete together or roll back. A mismatch returns `ErrBundleActionFileLeaseLost`.

## Materialized metrics and reads

On completion, `refreshBundleActionMetrics` rebuilds campaign and tag metrics from processed action files and eligible delivered audience. It serializes completion on the bundle row so concurrent file completions cannot publish a stale partial snapshot. The derived audience uses the immutable bundle selection and campaign tag-attribution records, plus delivery status; it does not rely on mutable profile tags.

`Summary`, `CampaignMetric`, and `TagMetrics` read the materialized views. Missing summary/campaign metrics return zero-value models keyed to the requested bundle/campaign. `ActiveUIDSet` returns a set of requested UIDs found in processed files; empty input returns an empty set without querying.
# Types and function reference

`BundleActionRepository` owns action-file state and bundle-scoped action-rate read models. `BundleActionRepositoryImpl` stores the GORM database; `NewBundleActionRepository` constructs it and `getDB` honors a transaction in `TxContextKey`. Parsing uploads is performed outside this repository.

## File lifecycle

- `Create` inserts a file row. `ByID` returns the GORM error, including record-not-found. `List(ctx, bundleID, limit, offset)` returns a total count and a page ordered by creation time and ID descending.
- `PromotePending(ctx, id, storagePath, at)` atomically moves a staging file to pending and records its durable path; a non-staging row returns `ErrBundleActionFileNotStaged`. `DiscardStaged` deletes only a staging row and returns whether it was deleted, allowing the caller to decide whether to remove its stored file.
- `RequestDelete(ctx, id, customerID, at)` moves a processed file to delete-pending and records who requested it. It returns the database error but does not check whether a row matched.
- `ClaimPending(ctx, limit, stale, at)` uses a transaction and `FOR UPDATE SKIP LOCKED` to claim pending, delete-pending, or stale processing files. Its candidate query chooses at most one file per bundle in each claim batch; a nonpositive limit becomes one. It sets `started_at` as the lease token, clears errors and leaves delete-pending files in that state.
- `StoreUIDsAndResult(ctx, fileID, bundleID, leaseStartedAt, uids, total, duplicates, invalid, at)` locks and verifies the processing lease, replaces its UID rows in batches of 1000, resolves each UID within the bundle and records parser and validation counts in one transaction. Validation is ordered: outside bundle, unassigned tag, missing delivery, then eligible. Delivery requires a current processed campaign, the selection and phone match, and all positive message parts delivered on at least one of SMS, Bale, Rubika or SPlus. New/duplicate-across-files counts compare against other processed files and are independent of eligibility.
- `Fail(ctx, id, leaseStartedAt, at, code, message)` marks a matching processing lease failed, truncating code to 64 bytes and message to 255 bytes. `Complete(ctx, id, leaseStartedAt, at)` locks the matching processing or delete-pending file and its bundle, marks it processed or deleted, refreshes metrics and updates the bundle timestamp in one transaction. A lost lease returns `ErrBundleActionFileLeaseLost`.

## Metrics and reads

`refreshBundleActionMetrics` replaces campaign and tag metrics and the bundle summary. It uses `bundleActionEligibleAudienceSQL` to join immutable selections and tag attribution to fully delivered audiences, and checks distinct UIDs in processed action files. It does not use mutable profile tags. A transaction-local temporary table avoids repeating the expensive delivery join. If no processed files remain, metrics are removed and the summary is reset. The bundle lock in `Complete` serializes concurrent refreshes.

`Summary` and `CampaignMetric` return zero-valued models with the requested IDs when no row exists. `TagMetrics` joins display titles and orders by tag ID. `ActiveUIDSet` returns a map containing requested UIDs found in processed files; an empty input returns an empty map without querying.
