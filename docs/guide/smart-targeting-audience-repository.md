# `smart_targeting_audience_repository.go`

This repository is the data foundation for smart-targeting audience queries and score-based selection. It centralizes audience eligibility, filter application, and score ordering so capacity calculations, test sampling, and execution selection use consistent SQL rules.

The query path applies targeting criteria and exclusions in the database, then selects only the identifiers and scoring fields needed by downstream flows. Score selection is deterministic at the database level and avoids fetching a broad audience into memory before filtering. Treat this repository as the source for how candidate audience rows are assembled; the business flows decide the requested count and how selected IDs are persisted.
