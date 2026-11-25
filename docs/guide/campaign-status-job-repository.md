# `campaign_status_job_repository.go`

`CampaignStatusJobRepositoryImpl` stores and queries campaign delivery-status jobs. `ListDue` fetches jobs ready for processing; `ListDueInterleavedByCampaign` spreads a batch across campaigns to avoid letting one campaign dominate a worker batch. `SaveBatch` persists jobs and `Update` records job changes.

`DeliveryStatusFinal` checks whether status collection is complete for a processed campaign. `ByID`, `ByFilter`, `Count`, and `Exists` provide lookup/listing support. The repository handles persistence and due-job ordering; the scheduler decides how frequently to poll and how to handle provider status responses.
## Types and methods

`CampaignStatusJobRepositoryImpl` embeds `BaseRepository[models.CampaignStatusJob, any]`; `NewCampaignStatusJobRepository` constructs it. `ByID` returns nil when absent. `SaveBatch` delegates to the base repository, while `Update` saves the supplied job.

`ListDue` selects a platform's unexecuted jobs scheduled by `now` with fewer than three retries, ordered by scheduled time and ID. `ListDueInterleavedByCampaign` applies the same criteria but ranks each processed campaign's jobs so its earliest appears before another job from the same campaign. Both default a nonpositive limit to 100; these are reads, not lease claims.

`DeliveryStatusFinal` is true only if at least one job exists and every job for the processed campaign has executed without an error. A zero processed-campaign ID returns false. This matters when distinguishing zero deliveries from an unfinished status snapshot. `ByFilter` ignores its filter and applies only ordering and pagination; `Count` counts all jobs and `Exists` uses that count.
