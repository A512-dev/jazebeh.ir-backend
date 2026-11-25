# `campaign_audience_report_job_repository.go`

This repository manages asynchronous campaign audience-report jobs and their generated output references.

`CreateWithinCustomerLimit` creates a job while enforcing the per-customer active-job limit. `ByIDForCustomer` scopes retrieval to the owning customer. `ClaimDue` leases due jobs for workers, and `UpdateClaimed` only updates a job when the claim is still valid. Output cleanup helpers list expired paths, mark output as expired, find still-referenced paths, and purge terminal job records before a cutoff.

The repository coordinates durable job and file-reference state; report generation, storage deletion, and scheduling are performed by their respective workers/services.
## Types and methods

`CampaignAudienceReportJobRepository` is implemented by `campaignAudienceReportJobRepository`; `NewCampaignAudienceReportJobRepository` supplies its database. These methods use `db.WithContext` directly.

- `CreateWithinCustomerLimit(ctx, job, limit)` requires a positive limit. A per-customer PostgreSQL advisory transaction lock serializes the count and insert. Pending and processing jobs, plus completed jobs not yet expired, count toward the limit. It returns `false, nil` when full.
- `ByIDForCustomer` scopes the lookup to the customer and returns `(nil, nil)` if absent.
- `ClaimDue(ctx, now, lease)` locks the oldest pending job or expired processing lease with `SKIP LOCKED`, increments attempts and installs a fresh UUID lease token and expiry. The boolean reports whether work was found.
- `UpdateClaimed` updates output, counts, status and errors only for the supplied lease token, then clears the token and expiry. It returns false for a missing or changed token, protecting a newer worker.
- `ExpiredOutputPaths` lists completed expired files. After deletion, `MarkExpiredOutput` marks matching completed, expired paths as expired and clears the path. `ReferencedOutputPaths` lists paths retained by completed jobs for orphan cleanup. `PurgeTerminalBefore` deletes only old failed or expired job metadata; completed output must first be expired.
