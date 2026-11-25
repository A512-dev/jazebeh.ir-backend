# `campaign_repository.go`

`CampaignRepositoryImpl` is the main persistence/query adapter for campaigns. It provides ID/customer lookups, update and status operations, visibility changes, pending/scheduled lists, aggregates, and model-specific filtering with pagination.

The repository also contains click and sent-volume aggregation helpers. Click queries exclude automated traffic through shared SQL logic, so report counts can differ from raw click rows. Ordering supports campaign fields and click-rate ordering; callers should use the repository's supported order keys rather than constructing SQL order clauses themselves.

Writes use context-aware GORM operations and transaction helpers where multiple updates must remain consistent. `ByFilter`, `Count`, and `Exists` keep campaign-specific conditions in this repository rather than the generic base repository.
