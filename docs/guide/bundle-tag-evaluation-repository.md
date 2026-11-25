# `bundle_tag_evaluation_repository.go`

This repository manages bundle/tag evaluation state and the specialized read models used to decide whether tags have enough reliable test performance for targeting. It combines persisted evaluation lifecycle data with derived campaign/tag performance and bundle-level summaries.

The repository is responsible for loading evaluation state, claiming or transitioning evaluation work, and returning evaluation data scoped to a bundle and its tags. Read models are designed for downstream selection flows and admin views; they should not be interpreted as raw delivery facts. Tag performance is derived by `tag_test_performance_repository.go`, while this repository assembles/evaluates that evidence at bundle/tag scope.

When changing these queries, preserve bundle and tag scoping and the distinction between missing evaluation, pending evaluation, and a completed evaluation with insufficient evidence. Those states have different meanings to callers.
## Repository types and methods

The file contains several focused repository implementations, each constructed by its corresponding `New...Repository` function. Run, event, persona-attempt, batch, batch-attempt and score repositories embed the generic base repository; `BundleTagEvaluationReadRepositoryImpl` instead stores a GORM database and uses `getDB` to select a context transaction or the base connection.

- `BundleTagEvaluationRunRepositoryImpl`: `ByID` looks up a run; `ListByBundleID` lists its bundle's runs; `CountByCustomerIDCreatedBetween` counts runs within a customer/time window.
- `BundleTagEvaluationEventRepositoryImpl`: `LatestByRunID` and `ListByRunID` read run events; `ExistsByRunIDAndType` and `ExistsByBatchIDAndType` check for event types.
- `BundleTagPersonaAnalysisAttemptRepositoryImpl`: `LatestByRunID` and `ListByRunID` read persona attempts. `BundleTagEvaluationBatchRepositoryImpl.ListByRunID` reads batches. `BundleTagEvaluationBatchAttemptRepositoryImpl.LatestByBatchID` and `ListByBatchID` read attempts for a batch.
- `BundleTagScoreRepositoryImpl`: `CountByRunID`, `CountByRunIDAndBatchID` and `ListByRunID` read stored scores.
- `BundleTagEvaluationReadRepositoryImpl`: `ByBundleID` and `ByRunID` return the corresponding derived status or nil when absent; `ListByBundleIDs` returns an empty slice for empty input. `ListPendingRuns` reads derived `created` and `evaluating` states in creation order, with an optional limit. `ListCurrentScoresByBundleID` and `CountCurrentScoresByBundleID` include only active tags; the list orders by fit score descending and tag ID ascending and accepts pagination.

The read models summarize the event-driven evaluation state; the underlying run, event, attempt, batch and score records remain available for detailed history. Consult each query's order when relying on a particular list ordering.
