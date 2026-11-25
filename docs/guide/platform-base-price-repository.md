# `platform_base_price_repository.go`

`platformBasePriceRepository` reads and writes `models.PlatformBasePrice` rows. Unlike many repositories, it does not embed `BaseRepository`; each method applies the context directly to its stored GORM handle.

- `Insert` creates a price-history row.
- `UpdatePriceByPlatform` updates the price for non-deleted rows of the platform. If no row is affected, it returns `gorm.ErrRecordNotFound`.
- `LatestByPlatform` returns the row with the greatest ID for a platform, or `(nil, nil)` when none exists.
- `List` finds the greatest-ID non-deleted row per platform using a grouped subquery, then returns those latest rows ordered by platform ascending.

The update path changes the current non-deleted row in place; `Insert` and `List` support keeping/querying price history. Transaction participation is through the GORM handle/context behavior here, rather than the `TxContextKey` helper used by `BaseRepository`.
## Types and methods

`NewPlatformBasePriceRepository` returns the unexported `platformBasePriceRepository`, which holds a GORM database and calls `db.WithContext` directly. `Insert` creates a row. `UpdatePriceByPlatform` updates the price on matching non-deleted rows and returns `gorm.ErrRecordNotFound` when none match; it is an update rather than an append. `LatestByPlatform` returns the highest-ID row or `(nil, nil)` when absent. `List` joins the maximum non-deleted ID per platform and orders the results by platform.
