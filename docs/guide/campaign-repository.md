# `campaign_repository.go`

`CampaignRepositoryImpl` is the main persistence/query adapter for campaigns. It provides ID/customer lookups, update and status operations, visibility changes, pending/scheduled lists, aggregates, and model-specific filtering with pagination.

The repository also contains click and sent-volume aggregation helpers. Click queries exclude automated traffic through shared SQL logic, so report counts can differ from raw click rows. Ordering supports campaign fields and click-rate ordering; callers should use the repository's supported order keys rather than constructing SQL order clauses themselves.

Writes use context-aware GORM operations and transaction helpers where multiple updates must remain consistent. `ByFilter`, `Count`, and `Exists` keep campaign-specific conditions in this repository rather than the generic base repository.
## Types and core operations

`CampaignRepositoryImpl` embeds `BaseRepository[models.Campaign, models.CampaignFilter]`; `NewCampaignRepository` initializes it. `statisticsWithoutTrackingResults` is a read projection that excludes the potentially large tracking-results array.

- `ByID`, `ByUUID`, `ByCustomerID`, `ByCustomerIDAndIDs`, `ByCustomerIDAndBundleIDs` and `ByStatus` provide scoped lookups. UUID parsing can fail; the bundle-ID query selects fields needed for bundle aggregation. `GetPendingApproval` and `GetScheduledCampaigns` provide specialized lists.
- `Update` saves campaign changes. `UpdateStatistics` replaces statistics, whereas `MergeStatistics` merges supplied JSON keys so a concurrent writer need not replace the entire object. `AppendTrackingResults` appends to the JSON array in the database. `UpdateStatus` targets the status field. These have different concurrency implications: do not use whole-object replacement when an independent partial update is intended.
- `MarkHidden` and `MarkVisible` change visibility for customer-scoped campaign IDs and return affected counts. `CountByCustomerID` and `CountByStatus` count their respective scopes.
- `AggregateClickCountsByCampaignIDs`, `AggregateClickCountsByCustomerIDs` and `AggregateTotalSentByCustomerIDs` build grouped reporting maps. `nonAutomatedClickTrafficSQL`, `excludeAutomatedClickTraffic` and exported `ExcludeAutomatedClickTraffic` centralize the reporting filter for test and automated click traffic.
- `ByFilter`, `Count` and `Exists` use `applyFilter`; `applyOrder` handles supported ordering and `withClickRateOrdering` adds the query needed for click-rate sorting. Check the filtering and ordering helpers when adding an API filter so list and count semantics stay aligned.
