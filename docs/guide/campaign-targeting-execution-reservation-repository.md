# `campaign_targeting_execution_reservation_repository.go`

This repository manages audience reservations for smart-targeting execution calculations. Reservations keep candidate IDs from being consumed by overlapping calculations while work is pending and help make capacity checks consistent.

Its operations create or materialize reservations for a calculation, inspect active reservations, and release them when work is completed, superseded, or abandoned. State changes are guarded and database-backed so concurrent schedulers cannot silently treat the same reserved audience as free. The execution/capacity flow owns policy decisions; this repository enforces persistence and reservation lifecycle.
