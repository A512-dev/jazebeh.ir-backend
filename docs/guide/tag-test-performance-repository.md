# `tag_test_performance_repository.go`

This repository derives tag performance from completed test campaigns and materializes campaign-, phase-, and bundle-level read models. It discovers campaigns needing work, claims them for a worker, recomputes campaign performance under a lease, and records failures.

Recomputation attributes completed test-campaign delivery outcomes to tags, refreshes derived summaries, and removes stale campaign/tag performance rows that are no longer supported by current source data. Lease checks protect the derived state from stale workers. The resulting metrics are consumed by campaign tag selection and bundle evaluation; this is a derived projection, not the authoritative campaign delivery record.
