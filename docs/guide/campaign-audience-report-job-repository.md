# `campaign_audience_report_job_repository.go`

This repository manages asynchronous campaign audience-report jobs and their generated output references.

`CreateWithinCustomerLimit` creates a job while enforcing the per-customer active-job limit. `ByIDForCustomer` scopes retrieval to the owning customer. `ClaimDue` leases due jobs for workers, and `UpdateClaimed` only updates a job when the claim is still valid. Output cleanup helpers list expired paths, mark output as expired, find still-referenced paths, and purge terminal job records before a cutoff.

The repository coordinates durable job and file-reference state; report generation, storage deletion, and scheduling are performed by their respective workers/services.
