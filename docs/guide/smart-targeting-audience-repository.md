# `smart_targeting_audience_repository.go`

This repository is the data foundation for smart-targeting audience queries and score-based selection. It centralizes audience eligibility, filter application, and score ordering so capacity calculations, test sampling, and execution selection use consistent SQL rules.

The query path applies targeting criteria and exclusions in the database, then selects only the identifiers and scoring fields needed by downstream flows. Score selection is deterministic at the database level and avoids fetching a broad audience into memory before filtering. Treat this repository as the source for how candidate audience rows are assembled; the business flows decide the requested count and how selected IDs are persisted.
## Types and eligibility

`SmartTargetingAudienceRepository` is implemented by `smartTargetingAudienceRepository`; `NewSmartTargetingAudienceRepository` constructs it and `getDB` honors a context transaction. `SmartTargetingSelectionPhase` explicitly distinguishes test and execution. `SmartTargetingAudienceQuery` carries the bundle, phase, selected tag union, score classes, allowed colors and optional campaign IDs whose own reservations should remain eligible. Build it from a consistent campaign snapshot.

`SmartTargetingAudienceCapacity` holds raw and eligible unique counts. `SmartTargetingScoreBounds` holds the union-wide p33/p66 thresholds. Classes are C at or below p33, B above p33 through p66, and A above p66. Selecting all classes skips score filtering and includes unscored profiles; nil bounds make restricted classes empty.

## Methods and query builders

- `CalculateCapacity` counts the selected-tag union before and after eligibility and score filtering. `SelectCandidates` returns deterministic score-ranked execution candidates up to the requested limit.
- `CalculateScoreBounds` computes one pair of percentile bounds for the union so all test-tag selections use the same classification. `SelectIDsForTag` returns IDs; `SelectForTag` returns fields needed for a test preview, excluding IDs already selected for earlier tags. Test selection imposes no order: its limited results depend on PostgreSQL's plan and are neither guaranteed random nor reproducible.
- The shared population CTEs enforce reachability and bundle consumption, reservation and exclusion rules; the selectable variant includes fields needed for persistence. `smartTargetingEligiblePopulationArgs` supplies their ordered parameters. `valid` checks the query shape, and `smartTargetingSelectsAllScoreClasses` recognizes the canonical all-class form.
- `smartTargetingScoreBoundsQuery`, `buildSmartTargetingTestTagSelectionQuery` and `buildSmartTargetingScoreClassPredicate` construct the specialized percentile, per-tag and class-filter SQL. Their eligibility predicates must remain consistent with the shared population CTEs.
