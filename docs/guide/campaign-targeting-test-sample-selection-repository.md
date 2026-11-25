# `campaign_targeting_test_sample_selection_repository.go`

This repository stores the selected audience sample for a campaign targeting test and manages bundle-scoped reservations that prevent the same audience from being concurrently allocated to conflicting work.

It can load a selection and its ordered members, materialize a calculation's chosen IDs, reserve/release campaign sample IDs, and query active reservations. Reservation reads and updates are transaction-aware. Materialization checks the expected reservation rows and is intended to be idempotent/guarded against state drift; callers should treat a reservation conflict as a signal to recalculate or retry through the flow, not bypass it.

The repository provides persistence and concurrency protection; the sampling flow determines the sample size and which candidates are selected.
