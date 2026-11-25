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
