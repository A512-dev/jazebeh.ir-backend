# `bundle_repository.go`

`BundleRepositoryImpl` handles bundle records and embeds `BaseRepository[models.Bundle, models.BundleFilter]`.

- `ByID` loads the bundle with its `Customer` association and returns `(nil, nil)` if missing.
- `ByCustomerID` delegates to `ByFilter` with a customer filter and newest-ID-first ordering.
- `Update` updates `UpdatedAt` to the current UTC time and calls GORM `Save`. It uses `getDBForWrite` to reuse an outer transaction or own the write transaction.
- `ByFilter` supports ID, customer ID, partial case-insensitive title/persona/customer-name matches, and created/updated time bounds. It defaults to `id DESC`, supports pagination, and preloads `Customer` for every result.
- `Count` and `Exists` apply the same filters without preloading associations.

Text filters are trimmed; blank strings are ignored. The date bounds use inclusive lower bounds and exclusive upper bounds (`>=` for After and `<` for Before).
