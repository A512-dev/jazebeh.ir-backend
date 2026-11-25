# Asynchronous campaign audience report flow

`business_flow/campaign_audience_report_job_flow.go` exports audience rows for owned campaigns without building an unbounded workbook in a request. It uses the [report job repository](campaign-audience-report-job-repository.md) for customer limits, leasing and retention. `CampaignAudienceReportJobFlow` is implemented by `CampaignAudienceReportJobFlowImpl`, constructed by `NewCampaignAudienceReportJobFlow`; the implementation holds repositories, DB, storage root and configurable row, file-size and job limits.

## Request and access

`reportCustomerID` extracts the authenticated customer. `validateReportCampaignIDs` checks the requested IDs; `Create` verifies all requested campaigns belong to the customer, creates a pending UUID job through the repository's serialized customer limit, and writes a best-effort audit event. `mapCampaignAudienceReportJob` maps persistent job state to a response. `Get` validates the job UUID, scopes lookup to the customer and calls `ensureJobCampaignsAreStillOwned` again. `RecordDownload` records an audit event; it does not generate the workbook.

## Worker pipeline

1. `ExecuteNext` claims one due job with an attempt-specific UUID lease token. It cleans old-token staging rows and defers cleanup of its own staging rows. A claim with no work returns normally.
2. `stage` rechecks ownership and visits each campaign's durable audience UID/code file. `campaignAudienceReportStageRow` represents a staging table row and supplies its table name. `appendStageRow` coalesces duplicate UIDs within a batch; `upsertStageBatch` inserts or updates rows for job, lease, campaign and UID and counts newly inserted rows. `stage` enforces the maximum unique row count and errors when a campaign has no audience report data.
3. `write` keyset-pages staging rows by campaign and UID, obtains live click/action states with `liveStates`, and streams an XLSX, splitting sheets at the Excel row limit. Each row includes campaign ID/UUID, audience UID, status, clicked and action. Status is `unknown`: the durable UID file is not delivery-status evidence. Clicks use the canonical non-automated filter; actions come from processed bundle action files. Values passed through `excelSafeString` are protected from spreadsheet formula interpretation.
4. `limitedWriter` enforces the output byte limit. `reportOutputPath` includes both job ID and lease token. The worker writes a partial file and renames it after successful output. `complete` marks the matching lease completed and gives the output a 24-hour expiry. If publication loses its lease or fails, it removes its own attempt-specific output.

`campaignAudienceReportFailureCode` distinguishes row and output-size limits from other errors. `fail` removes a partial file, records a sanitized error under the lease using a short independent context, audits failure and returns the underlying cause. A changed lease prevents a stale worker from updating the job. `reportJobPageRow` is the join projection used during writing.

## Retention

`Cleanup` first marks expired completed outputs unavailable, then deletes files, checks retained paths, removes old unreferenced artifacts and purges sufficiently old failed/expired metadata. `removeUnreferencedOutputFiles` uses an age guard to avoid racing a live writer; `isCampaignAudienceReportOutputName` restricts cleanup to the expected UUID-based final/partial names. A crash can leave an orphan, but the cleanup path is designed to reap it later.

## Consistency example

If a lease expires while worker A is generating a workbook, worker B receives a different token. Their staging keys and filenames include their respective tokens. A's final `UpdateClaimed` cannot complete B's lease; A removes only its own output. The report combines an attempt's staged audience snapshot with live click and action lookups while pages are written, so it should not be interpreted as a single database-wide point-in-time snapshot of all external events.

## Complete function index

Follow the links to the implementation. Signatures show inputs, outputs and receiver ownership; source comments are included where available.

- [`auditEvent`](../../business_flow/campaign_audience_report_job_flow.go#L46): `func (f *CampaignAudienceReportJobFlowImpl) auditEvent( ctx context.Context, customerID uint, action, description string, success bool, errorMessage *string, metadata *ClientMetadata, )`
- [`NewCampaignAudienceReportJobFlow`](../../business_flow/campaign_audience_report_job_flow.go#L87): `func NewCampaignAudienceReportJobFlow( j repository.CampaignAudienceReportJobRepository, c repository.CampaignRepository, customers repository.CustomerRepository, audit repository.AuditLogRepository, db *gorm.DB, root string, maxRows, maxBytes, maxJobsPerCustomer int64, ) *CampaignAudienceReportJobFlowImpl`
- [`mapCampaignAudienceReportJob`](../../business_flow/campaign_audience_report_job_flow.go#L122): `func mapCampaignAudienceReportJob(j *models.CampaignAudienceReportJob) *dto.CampaignAudienceReportJobResponse`
- [`reportCustomerID`](../../business_flow/campaign_audience_report_job_flow.go#L142): `func reportCustomerID(ctx context.Context) (uint, error)`
- [`validateReportCampaignIDs`](../../business_flow/campaign_audience_report_job_flow.go#L149): `func validateReportCampaignIDs(ids []uint) error`
- [`ensureJobCampaignsAreStillOwned`](../../business_flow/campaign_audience_report_job_flow.go#L171): `func (f *CampaignAudienceReportJobFlowImpl) ensureJobCampaignsAreStillOwned( ctx context.Context, j *models.CampaignAudienceReportJob, ) error` — ensureJobCampaignsAreStillOwned prevents a queued job from exporting a campaign that was transferred after the job was created.
- [`Create`](../../business_flow/campaign_audience_report_job_flow.go#L192): `func (f *CampaignAudienceReportJobFlowImpl) Create( ctx context.Context, ids []uint, metadata *ClientMetadata, ) (*dto.CampaignAudienceReportJobResponse, error)`
- [`Get`](../../business_flow/campaign_audience_report_job_flow.go#L244): `func (f *CampaignAudienceReportJobFlowImpl) Get( ctx context.Context, id string, ) (*dto.CampaignAudienceReportJobResponse, *models.CampaignAudienceReportJob, error)`
- [`RecordDownload`](../../business_flow/campaign_audience_report_job_flow.go#L268): `func (f *CampaignAudienceReportJobFlowImpl) RecordDownload( ctx context.Context, job *models.CampaignAudienceReportJob, metadata *ClientMetadata, )`
- [`TableName`](../../business_flow/campaign_audience_report_job_flow.go#L302): `func (campaignAudienceReportStageRow) TableName() string`
- [`appendStageRow`](../../business_flow/campaign_audience_report_job_flow.go#L308): `func appendStageRow( batch []campaignAudienceReportStageRow, indexes map[string]int, row campaignAudienceReportStageRow, ) []campaignAudienceReportStageRow` — appendStageRow keeps one row per UID in an INSERT batch. PostgreSQL rejects an ON CONFLICT DO UPDATE statement when the same conflict key occurs twice in that statement, while the durable JSONL input deliberately permits later records to replace an earlier UID-to-code mapping.
- [`upsertStageBatch`](../../business_flow/campaign_audience_report_job_flow.go#L325): `func (f *CampaignAudienceReportJobFlowImpl) upsertStageBatch(ctx context.Context, batch []campaignAudienceReportStageRow) (int64, error)` — upsertStageBatch returns only newly-created staging rows. PostgreSQL's xmax is zero for INSERT results and nonzero for rows reached through ON CONFLICT DO UPDATE, letting the caller keep an exact row count without repeatedly scanning the complete staging table.
- [`stage`](../../business_flow/campaign_audience_report_job_flow.go#L361): `func (f *CampaignAudienceReportJobFlowImpl) stage(ctx context.Context, j *models.CampaignAudienceReportJob) error`
- [`Write`](../../business_flow/campaign_audience_report_job_flow.go#L429): `func (w *limitedWriter) Write(p []byte) (int, error)`
- [`reportOutputPath`](../../business_flow/campaign_audience_report_job_flow.go#L437): `func reportOutputPath(root, id, leaseToken string) string`
- [`write`](../../business_flow/campaign_audience_report_job_flow.go#L441): `func (f *CampaignAudienceReportJobFlowImpl) write( ctx context.Context, j *models.CampaignAudienceReportJob, ) (int64, int, int64, string, error)`
- [`liveStates`](../../business_flow/campaign_audience_report_job_flow.go#L550): `func (f *CampaignAudienceReportJobFlowImpl) liveStates( ctx context.Context, rows []reportJobPageRow, ) (map[uint]map[string]bool, map[uint]map[string]bool, error)`
- [`ExecuteNext`](../../business_flow/campaign_audience_report_job_flow.go#L588): `func (f *CampaignAudienceReportJobFlowImpl) ExecuteNext(ctx context.Context, lease time.Duration) error`
- [`complete`](../../business_flow/campaign_audience_report_job_flow.go#L612): `func (f *CampaignAudienceReportJobFlowImpl) complete( ctx context.Context, job *models.CampaignAudienceReportJob, rows int64, sheets int, bytes int64, path string, ) error`
- [`removeCampaignAudienceReportOutput`](../../business_flow/campaign_audience_report_job_flow.go#L648): `func removeCampaignAudienceReportOutput(path string) error`
- [`campaignAudienceReportFailureCode`](../../business_flow/campaign_audience_report_job_flow.go#L655): `func campaignAudienceReportFailureCode(cause error, fallback string) string`
- [`fail`](../../business_flow/campaign_audience_report_job_flow.go#L666): `func (f *CampaignAudienceReportJobFlowImpl) fail( ctx context.Context, job *models.CampaignAudienceReportJob, code string, cause error, ) error`
- [`Cleanup`](../../business_flow/campaign_audience_report_job_flow.go#L706): `func (f *CampaignAudienceReportJobFlowImpl) Cleanup(ctx context.Context) error`
- [`removeUnreferencedOutputFiles`](../../business_flow/campaign_audience_report_job_flow.go#L742): `func (f *CampaignAudienceReportJobFlowImpl) removeUnreferencedOutputFiles(referencedPaths []string, before time.Time) error` — removeUnreferencedOutputFiles reaps old final and partial artifacts from workers that crashed or lost database connectivity after writing a file. The age guard avoids racing a worker that has renamed its output but has not yet committed the completed state. Only files matching this report's UUID-based naming scheme inside its dedicated root are eligible; unrelated files in the directory are left untouched.
- [`isCampaignAudienceReportOutputName`](../../business_flow/campaign_audience_report_job_flow.go#L782): `func isCampaignAudienceReportOutputName(name string) bool`
