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
