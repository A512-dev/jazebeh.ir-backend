# Bundle action flow: uploads, workers and metrics

`business_flow/bundle_action_flow.go` coordinates customer action-file uploads and worker processing. The file supplies UIDs representing actions; the [bundle action repository](bundle-action-repository.md) resolves them against historical attribution and delivered audiences and maintains the metric projections. `BundleActionFlow` declares the API and worker method. `BundleActionFlowImpl` holds action, bundle and campaign repositories, a database and storage root. `NewBundleActionFlow` uses `BUNDLE_ACTION_FILE_STORAGE_ROOT` or `data/uploads/bundle-action-files`.

## Ownership and upload

`getCustomerBundle` loads a bundle and checks its customer. `Upload` first checks ownership, requires an `.xlsx` extension case-insensitively, reads at most 50 MiB plus one byte, and rejects empty or oversized content. It does not parse the workbook synchronously. It computes SHA-256 and stores the basename, action level, uploader and timestamps in a non-claimable `staging` row.

The file publication sequence matters:

1. `actionFileStagingPath` creates a temporary file under a staging directory inside the storage root, with restricted permissions. `writeStagedActionFile` writes and syncs the bytes.
2. After creating the staging database row, `actionFileStoragePath` derives `<root>/<bundle ID>/<file ID>.xlsx`. `publishStagedActionFile` creates the destination directory, renames the staged file within the same root and syncs the destination directory.
3. `PromotePending` atomically installs the durable path and moves the row to pending, making it claimable. The returned object is updated accordingly.

If publication fails, `cleanupStagedActionFile` uses a short independent context to discard the row only if it is still staging. It removes bytes only when that discard succeeds; a potentially promoted row must retain its file. This protects a worker from a pending row whose file was deleted during uncertain promotion.

## Customer methods

`Get` checks both bundle ownership and the file's bundle. `List` checks ownership and passes limit and `(page-1)*limit` to the repository; callers must supply suitable pagination. `Delete` accepts only processed files and requests asynchronous deletion; it does not immediately recompute metrics itself. `Summary` and `TagMetrics` check bundle ownership before returning projections. `CampaignMetric` resolves an owned campaign by UUID; a campaign without a bundle receives a zero-valued metric for its ID.

## Worker and parser

`ExecuteBundleActionFile` loads the file and compares its `StartedAt` with the worker's lease. A mismatch returns `ErrBundleActionFileLeaseLost`. Delete-pending work is completed directly. For processing work, it parses the stored workbook, calls `StoreUIDsAndResult` with parser counts and then `Complete`, both fenced by the lease. A parse failure attempts to persist a sanitized failure using a background context and returns the original error. Other statuses are a no-op after the lease check.

`parseBundleActionXLSX` reads raw cells from the first worksheet. A case-insensitive trimmed `uid` header selects that column; duplicate UID headers fail. Without a header, column one is data including the first row. Header-only, empty-sheet and no-valid-UID files fail. It trims UIDs, counts blanks or values over 255 bytes as invalid, counts subsequent duplicates separately, and returns sorted unique UIDs. It limits data rows to 500,000, uncompressed workbook size to 256 MiB and in-memory worksheet XML to 8 MiB. Iterator and workbook errors propagate.

The repository classifies each UID as outside the bundle, without an assigned tag, missing a fully delivered message, or eligible. Completion serializes and rebuilds bundle metrics. Thus upload success means queued, not that parsing, validation or aggregation has succeeded.

## Example and failure boundaries

Suppose the first sheet contains `UID`, then `a`, `a`, a blank cell and `b`. The header is excluded from the data-row count; the parser returns sorted UIDs `a` and `b`, total 4, duplicate 1 and invalid 1. Repository validation then classifies `a` and `b` independently of those parser counts. A unique uploaded UID is not necessarily an eligible action: it may not belong to the bundle, may lack attribution, or may lack a fully delivered message.

The staging row and rename sequence are not a distributed filesystem/database transaction. The code handles uncertain failure conservatively by keeping files if it cannot prove the row is still discardable. Similarly, parsing and storing the result precede `Complete`: callers should inspect file status and lease when a job has not reached processed. The current `getCustomerBundle` dereferences the returned bundle without an explicit nil check, so its behavior for a missing row depends on the repository contract; do not assume it maps every missing bundle to a business error.

## Complete function index

Follow the links to the implementation. Signatures show inputs, outputs and receiver ownership; source comments are included where available.

- [`NewBundleActionFlow`](../../business_flow/bundle_action_flow.go#L61): `func NewBundleActionFlow( actionRepository repository.BundleActionRepository, bundleRepository repository.BundleRepository, campaignRepository repository.CampaignRepository, db *gorm.DB, ) *BundleActionFlowImpl` — NewBundleActionFlow creates a flow for managing bundle action files.
- [`getCustomerBundle`](../../business_flow/bundle_action_flow.go#L81): `func (flow *BundleActionFlowImpl) getCustomerBundle(ctx context.Context, customerID, bundleID uint) (*models.Bundle, error)` — getCustomerBundle returns a bundle after confirming that it belongs to the customer.
- [`actionFileStoragePath`](../../business_flow/bundle_action_flow.go#L93): `func actionFileStoragePath(storageRoot string, bundleID uint, actionFileID int64) string` — actionFileStoragePath returns the destination path for a bundle action file.
- [`actionFileStagingPath`](../../business_flow/bundle_action_flow.go#L97): `func actionFileStagingPath(storageRoot string) (string, error)`
- [`writeStagedActionFile`](../../business_flow/bundle_action_flow.go#L124): `func writeStagedActionFile(stagedPath string, fileData []byte) error`
- [`publishStagedActionFile`](../../business_flow/bundle_action_flow.go#L139): `func publishStagedActionFile(stagedPath, storagePath string) error`
- [`Upload`](../../business_flow/bundle_action_flow.go#L162): `func (flow *BundleActionFlowImpl) Upload( ctx context.Context, customerID, bundleID uint, fileName, actionLevel string, source io.Reader, ) (*models.BundleActionFile, error)` — Upload validates, stores, and queues an XLSX action file for a customer bundle.
- [`cleanupStagedActionFile`](../../business_flow/bundle_action_flow.go#L227): `func (flow *BundleActionFlowImpl) cleanupStagedActionFile(actionFileID int64, stagedPath, storagePath string)` — cleanupStagedActionFile removes bytes only after its non-claimable row was removed. If promotion might have succeeded, retaining the final file is safer than leaving a pending row without storage.
- [`Get`](../../business_flow/bundle_action_flow.go#L239): `func (flow *BundleActionFlowImpl) Get( ctx context.Context, customerID, bundleID uint, actionFileID int64, ) (*models.BundleActionFile, error)` — Get returns an action file when it belongs to the customer's bundle.
- [`List`](../../business_flow/bundle_action_flow.go#L255): `func (flow *BundleActionFlowImpl) List( ctx context.Context, customerID, bundleID uint, page, limit int, ) ([]*models.BundleActionFile, int64, error)` — List returns a page of action files for a customer's bundle.
- [`Delete`](../../business_flow/bundle_action_flow.go#L267): `func (flow *BundleActionFlowImpl) Delete(ctx context.Context, customerID, bundleID uint, actionFileID int64) error` — Delete requests deletion of a processed action file owned by the customer's bundle.
- [`Summary`](../../business_flow/bundle_action_flow.go#L285): `func (flow *BundleActionFlowImpl) Summary(ctx context.Context, customerID, bundleID uint) (*models.BundleActionSummary, error)` — Summary returns aggregate action metrics for a customer's bundle.
- [`TagMetrics`](../../business_flow/bundle_action_flow.go#L293): `func (flow *BundleActionFlowImpl) TagMetrics(ctx context.Context, customerID, bundleID uint) ([]*models.BundleActionTagMetric, error)` — TagMetrics returns action metrics grouped by tag for a customer's bundle.
- [`CampaignMetric`](../../business_flow/bundle_action_flow.go#L301): `func (flow *BundleActionFlowImpl) CampaignMetric( ctx context.Context, customerID uint, campaignUUID string, ) (*models.BundleActionCampaignMetric, error)` — CampaignMetric returns action metrics for a campaign, if the campaign belongs to a bundle.
- [`ExecuteBundleActionFile`](../../business_flow/bundle_action_flow.go#L319): `func (flow *BundleActionFlowImpl) ExecuteBundleActionFile(ctx context.Context, actionFileID int64, leaseStartedAt time.Time) error` — ExecuteBundleActionFile processes a claimed action file or completes its pending deletion. Every durable mutation is fenced by leaseStartedAt so a reclaimed worker cannot publish after a newer worker has claimed the file.
- [`parseBundleActionXLSX`](../../business_flow/bundle_action_flow.go#L353): `func parseBundleActionXLSX(filePath string) (uids []string, totalRows, duplicateUIDs, invalidUIDs int64, err error)` — parseBundleActionXLSX extracts sorted, unique valid UIDs and row-count metrics from an XLSX file.
