# `segment_price_factor_repository.go`

`SegmentPriceFactorRepositoryImpl` stores and queries `models.SegmentPriceFactor`, a platform-scoped factor keyed by segment level 3. It embeds `BaseRepository` and has concrete filtering for platform and level 3.

## Latest factors

- `ListLatestByLevel3` is the SMS-default convenience method.
- `ListLatestByLevel3ForPlatform` returns the latest row per `level3` for one platform using PostgreSQL `DISTINCT ON`.
- `LatestByLevel3s` is the SMS-default map lookup.
- `LatestByLevel3sForPlatform` returns `map[level3]priceFactor` for requested values. Empty input returns an empty map without querying.

The latest-row queries order by `created_at DESC`. If two records for a level have the same timestamp, this SQL does not add an ID tie-breaker.

## Other queries

`ByID` returns `(nil, nil)` for a missing row. `ByFilter` supports platform and level3, defaults to `created_at DESC`, and supports ordering and pagination. `Count` and `Exists` use the same filter conditions.
## Types and methods

`SegmentPriceFactorRepositoryImpl` embeds `BaseRepository[models.SegmentPriceFactor, models.SegmentPriceFactorFilter]`; `NewSegmentPriceFactorRepository` constructs it. `ByID` returns `(nil, nil)` when absent.

`ListLatestByLevel3` and `LatestByLevel3s` use the SMS platform; their `ForPlatform` counterparts accept a platform explicitly. The list returns one row per level-3 segment; the map lookup returns only requested segments found, and returns an empty map without querying for empty input. Both use PostgreSQL `DISTINCT ON` ordered by creation time descending; equal timestamps have no explicit ID tie-breaker.

`applyFilter` handles platform and level 3. `ByFilter` defaults to creation time descending and accepts positive pagination; `Count` and `Exists` use the same filter. Caller-provided order values should be trusted.
