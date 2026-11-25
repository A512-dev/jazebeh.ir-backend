# `bundle_tag_evaluation_repository.go`

This repository manages bundle/tag evaluation state and the specialized read models used to decide whether tags have enough reliable test performance for targeting. It combines persisted evaluation lifecycle data with derived campaign/tag performance and bundle-level summaries.

The repository is responsible for loading evaluation state, claiming or transitioning evaluation work, and returning evaluation data scoped to a bundle and its tags. Read models are designed for downstream selection flows and admin views; they should not be interpreted as raw delivery facts. Tag performance is derived by `tag_test_performance_repository.go`, while this repository assembles/evaluates that evidence at bundle/tag scope.

When changing these queries, preserve bundle and tag scoping and the distinction between missing evaluation, pending evaluation, and a completed evaluation with insufficient evidence. Those states have different meanings to callers.
