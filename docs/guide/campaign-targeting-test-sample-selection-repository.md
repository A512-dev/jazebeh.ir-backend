# `campaign_targeting_test_sample_selection_repository.go`

This repository stores the selected audience sample for a campaign targeting test and manages bundle-scoped reservations that prevent the same audience from being concurrently allocated to conflicting work.

It can load a selection and its ordered members, materialize a calculation's chosen IDs, reserve/release campaign sample IDs, and query active reservations. Reservation reads and updates are transaction-aware. Materialization checks the expected reservation rows and is intended to be idempotent/guarded against state drift; callers should treat a reservation conflict as a signal to recalculate or retry through the flow, not bypass it.

The repository provides persistence and concurrency protection; the sampling flow determines the sample size and which candidates are selected.
## Types and lifecycle

`CampaignTargetingTestSampleSelectionRepository` defines persistence and reservation operations; `CampaignTargetingTestSampleSelectionRepositoryImpl` stores the database. `NewCampaignTargetingTestSampleSelectionRepository` constructs it and `getDB` honors a context transaction. `ErrSmartTargetingTestSelectionUnavailable` indicates invalid or incomplete snapshot/reservation state; `ErrSmartTargetingTestSelectionConflict` means current audience availability changed.

- `Save` validates campaign, bundle, calculation, generation, member count, unique positive audience IDs, assigned tags and order, then inserts the immutable selection and members in batches. The caller must provide a transaction when these writes must be atomic with surrounding work.
- `ByID` and `ByCalculationID` return nil for invalid IDs or absent rows; `loadMembers` fills members in selection order.
- `ReserveForCampaign` runs during finalization with the caller holding the bundle update lock. It validates the active selection and generation, accepts an existing full active reservation, locks profile rows in stable ID order, then rechecks phone, assigned tag, score, applicable SMS color and bundle consumption/exclusion conflicts. It creates active reservation rows without changing the preview. A conflict requires a fresh sample.
- `ActiveReservedForCampaign` requires a transaction, locks active reservations and verifies their members and count match the selection. `Materialize` also requires a transaction and transitions active rows only if the affected count equals the positive expected count; a partial update must roll back with the enclosing allocation.
- `ReleaseForCampaign` locks the campaign and relevant bundles before releasing active rows; zero campaign ID is a no-op. The caller makes the terminal status decision in the same transaction.

`activeReservationsForUpdateQuery`, `materializeActiveReservationsQuery`, `requireExpectedReservationRows`, `activeTestReservationBundleIDsQuery` and `releaseActiveTestReservationsQuery` build and check those guarded operations. `testSampleSelectionAvailabilityQuery` performs the eligibility recheck. A known limitation is that unchanged member scores do not detect shifts in percentile boundaries caused by other profiles.
