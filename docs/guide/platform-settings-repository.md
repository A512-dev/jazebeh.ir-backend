# `platform_settings_repository.go`

`PlatformSettingsRepositoryImpl` provides persistence and query operations for `models.PlatformSettings`. It embeds `BaseRepository`, so its queries use the context-aware database handle and its write methods can join a transaction carried in the context.

## Lookup and filtering

- `ByID` loads the last row for an ID; a missing record is returned as `(nil, nil)`.
- `ByUUID` parses the supplied UUID, then uses `ByFilter`; malformed UUIDs return a parse error, while no matching row returns `(nil, nil)`.
- `ByFilter` applies only non-nil filter fields: ID, UUID, customer, name, platform, status, multimedia ID, and created-at bounds. It defaults to `id DESC`, supports a caller-supplied order, and applies positive limit/offset values.
- `Count` applies the same filter; `Exists` is `Count > 0`.

The date range predicates are strict: `CreatedAfter` uses `created_at > ?` and `CreatedBefore` uses `created_at < ?`.

## Updates

`UpdateStatus` updates the status and `updated_at` timestamp for a row. `AppendMetadata` updates JSONB metadata by merging one key/value into the existing object, treating null metadata as `{}`. Both use `getDBForWrite`, so they join an outer transaction if present or manage their own write transaction otherwise.
## Types and methods

`PlatformSettingsRepositoryImpl` embeds `BaseRepository[models.PlatformSettings, models.PlatformSettingsFilter]`; `NewPlatformSettingsRepository` initializes it. `ByID` returns nil on absence. `ByUUID` parses the UUID and uses `ByFilter`, returning parsing errors separately.

`applyFilter` handles ID, UUID, customer, name, platform, status, multimedia and exclusive creation-time bounds. `ByFilter` applies it with optional pagination and defaults to descending ID order; `Count` uses the same conditions and `Exists` delegates to it. Caller-supplied ordering should be trusted. `UpdateStatus` writes status and update time; `AppendMetadata` merges one string key/value into the existing JSON object and updates the timestamp. Both use the base write transaction helper and do not check affected-row counts.
