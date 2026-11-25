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
