# `campaign_status_job_repository.go`

`CampaignStatusJobRepositoryImpl` stores and queries campaign delivery-status jobs. `ListDue` fetches jobs ready for processing; `ListDueInterleavedByCampaign` spreads a batch across campaigns to avoid letting one campaign dominate a worker batch. `SaveBatch` persists jobs and `Update` records job changes.

`DeliveryStatusFinal` checks whether status collection is complete for a processed campaign. `ByID`, `ByFilter`, `Count`, and `Exists` provide lookup/listing support. The repository handles persistence and due-job ordering; the scheduler decides how frequently to poll and how to handle provider status responses.
