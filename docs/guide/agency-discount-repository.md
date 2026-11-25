# `agency_discount_repository.go`

`AgencyDiscountRepositoryImpl` handles agency-to-customer discount records. It embeds `BaseRepository` and uses the context-aware DB helpers.

## Finding discounts

- `ByID` and `ByUUID` return `(nil, nil)` when absent. `ByUUID` parses the UUID first and returns a parse error for invalid input.
- `ByAgencyAndCustomer` is a convenience wrapper around `ByFilter`.
- `GetActiveDiscount` finds the latest-ID discount for the agency/customer pair where expiry is null or later than the current UTC time.
- `ByFilter` and `Count` support identity, agency/customer, rate, active status, expiry bounds, and creation bounds. An `IsActive` value only adds a condition when it is true; false does not mean “expired only.”

## Joined listing and expiry

`ListActiveDiscountsWithCustomer` joins customers and returns a projection containing discount and customer display fields. Its optional `nameLike` searches representative first name, last name, and company name case-insensitively. Sort options are mapped through a whitelist; unknown or empty values default to newest first.

`ExpireActiveByAgencyAndCustomer` sets the expiry time on currently unexpired records for that pair. It uses `getDBForWrite`, so it reuses an outer transaction or starts its own.
## Types and methods

`AgencyDiscountRepositoryImpl` embeds `BaseRepository[models.AgencyDiscount, models.AgencyDiscountFilter]`; `NewAgencyDiscountRepository` initializes it. `AgencyDiscountWithCustomer` is a joined projection containing discount IDs, rate and dates alongside customer identity and representative/company names.

- `ByID` and `ByUUID` return `(nil, nil)` when absent; the latter parses the UUID and returns parsing errors.
- `ByAgencyAndCustomer` lists the pair's history through `ByFilter`. `GetActiveDiscount` looks for an unexpired row for the pair; a missing row returns nil. Active means expiry is null or later than the current UTC time.
- `ExpireActiveByAgencyAndCustomer` updates the expiry of currently active rows using the write transaction helper. `ListActiveDiscountsWithCustomer` joins customers, optionally searches trimmed names/company case-insensitively, and maps a small allowlist of sort names to SQL; unknown sorts default to newest creation time.
- `ByFilter` supports identity, agency, customer, rate, active state and creation/expiry ranges, with optional ordering and pagination. `Count` repeats those predicates and `Exists` delegates to it. `IsActive=false` does not select inactive rows: only true adds an active predicate. Unlike the joined-list sort, `ByFilter` passes its `orderBy` to GORM, so callers should use trusted ordering values.
