# `page_price_repository.go`

`PagePriceRepositoryImpl` is a small repository for `models.PagePrice`. It embeds `BaseRepository` with `struct{}` as the unused filter type and uses `getDB(ctx)` for all operations.

- `Insert` creates a page-price row.
- `LatestByPlatform` finds the newest row for one platform, ordered by `created_at DESC, id DESC`. A missing row returns `(nil, nil)`.
- `ListLatest` uses PostgreSQL `DISTINCT ON (platform)` to return one latest row per platform. The ordering makes the newest creation time win, with ID as a tie-breaker.

These methods are read/insert operations only; this repository does not define a general filter, count, or update method.
