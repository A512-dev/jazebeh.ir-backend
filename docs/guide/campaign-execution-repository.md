# `campaign_execution_repository.go`

This repository persists campaign execution work and its lifecycle state. It is used by scheduling/execution flows to create or retrieve execution records, claim due work, and record success or failure.

Claim/update methods use guarded state transitions so stale workers cannot overwrite a newer claim. Where the execution process uses targeting calculations or reservations, coordinate those operations through the business flow and pass the same transaction context when atomicity is required. This repository represents durable execution state; delivery-provider calls and campaign policy live elsewhere.
