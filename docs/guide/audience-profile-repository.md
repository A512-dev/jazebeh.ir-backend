# `audience_profile_repository.go`

`AudienceProfileRepositoryImpl` provides audience-profile lookup and campaign candidate selection. It embeds `BaseRepository` and builds its own model-specific filters.

## Direct lookups

- `ByID` and `ByUID` return `(nil, nil)` when no profile matches.
- `ByIDs` and `ByUIDs` return only `id`, `uid`, and `phone_number`. Empty input returns nil without querying. `ByIDs` restores the caller's input order; `ByUIDs` queries with PostgreSQL's typed array operator and does not promise input order.

## Campaign candidates

`SelectCampaignCandidates` returns no rows when `limit <= 0`. Otherwise it selects only the columns schedulers need, excludes profiles without a nonblank phone number, applies the supplied profile filters, and orders by descending ID before applying the limit.

It can exclude explicit profile IDs using one PostgreSQL bigint-array parameter. When `ExcludeBundleID` is set, it also excludes audience members already selected or reserved for that bundle, including active test-sample/execution reservations and members of committed calculations. These exclusions happen in PostgreSQL before the limit, avoiding loading a large candidate set into application memory.

## General filtering

`applyFilter` supports ID/IDs, UID, phone number, overlapping tags, color, created-at bounds, and normalized-score range constraints. `ByFilter` defaults to a deterministic hash-based ordering (`md5(COALESCE(uid, id::text))`) if no order is provided; `Count` and `Exists` apply the same filters.
## Types and methods

`AudienceProfileRepositoryImpl` embeds `BaseRepository[models.AudienceProfile, models.AudienceProfileFilter]`; `NewAudienceProfileRepository` constructs it.

- `ByID` and `ByUID` return `(nil, nil)` for no match. `ByIDs` returns only ID, UID and phone, restoring input order and omitting missing IDs; duplicate input IDs can appear more than once. `ByUIDs` also selects those three columns but does not promise input order. Both return nil for empty input; `audienceProfilesByUIDsQuery` builds the typed-array query.
- `SelectCampaignCandidates` returns reachable profiles in descending ID order, limited after exclusions. `campaignCandidatesQuery` uses one typed array for explicit excluded IDs and, when a bundle is supplied, excludes its materialized selections, active test and execution reservations, and committed execution calculation members. Nonpositive limit returns nil without a query.
- `applyFilter` handles IDs, UID, phone, overlapping tags, color, creation range and normalized-score conditions. `ByFilter` defaults to deterministic MD5 ordering and supports pagination; `Count` and `Exists` share the filters. The bundle-consumption exclusions above are specific to the candidate query, not the general filter. Caller-provided ordering should be trusted.
