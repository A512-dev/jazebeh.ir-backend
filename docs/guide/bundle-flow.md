# Bundle flow: lifecycle and statistics

This guide follows `business_flow/bundle_flow.go`. A bundle is the customer-owned container that groups campaigns and supplies shared targeting context. This flow creates, edits and reads bundles; campaign execution and AI evaluation are separate flows. The repository guide is [bundle repository](bundle-repository.md).

## Dependencies and contracts

`BundleFlow` declares `CreateBundle`, `UpdateBundle`, `GetBundle` and `ListBundles`. `BundleFlowImpl` holds bundle, campaign, customer, audit and evaluation-read repositories plus a GORM database. `NewBundleFlow` wires them. Customer IDs are supplied in request DTOs; each operation verifies the customer, and reads/updates check bundle ownership. Failures are wrapped in business errors with operation-specific codes. A nil request is invalid.

## Create and update

`CreateBundle` loads the customer and runs `validateBundleInput` before opening a transaction. The validated `bundleInput.newBundle` initializes empty JSON metadata and statistics and timestamps. The flow saves the row and its success audit in one `WithTransaction` callback: either both commit or neither does. On transaction failure it makes a best-effort failure audit outside the transaction and returns a creation error. A successful response contains the new ID and creation time.

`UpdateBundle` applies the same full input validation, loads the bundle, distinguishes missing from access denied, applies all validated values and saves it with a success audit in one transaction. Its failure audit is also best effort outside the failed transaction. Since `bundleInput.applyTo` assigns every supported field, this is not a sparse patch: optional fields that validate to nil are cleared.

`validateBundleInput` trims and checks title (required, at most 255 runes), objective and target persona (each required, at most 1023 runes). It delegates domain and category/job rules to shared campaign sanitizers. Description and ad link allow up to 2047 runes; target customer name allows 255. `validateRequiredBundleField` rejects blank input; `sanitizeOptionalBundleField` turns nil or whitespace-only input into nil and otherwise checks rune length. `bundleInput` holds the normalized values, `newBundle` builds a model, and `applyTo` copies them to an existing model.

## Read paths

`GetBundle` validates customer and ownership, maps the row with `bundleItemFromModel`, then enriches it with campaign statistics and evaluation status. `ListBundles` filters by customer and optional trimmed title/customer-name, defaults page to at least 1 and limit to 10, caps limit at 100, counts and fetches rows ordered by update time and ID descending. It enriches the page in batches and returns total pages. `bundleItemsToPointers` lets the enrichment mutate elements of the result slice.

`bundleItemFromModel` copies bundle fields and attempts to decode stored statistics JSON. Invalid or absent JSON does not itself fail the request. `injectBundleStatistics` loads this customer's campaigns for all bundle IDs on the page and applies `aggregateBundleStatistics` results. That helper counts campaigns overall and by test/execution phase and sums `aggregatedTotalRecords` and `aggregatedTotalSent` from each campaign's statistics. `unmarshalStatisticsMap` tolerates invalid JSON; `parseUint64Stat` handles supported numeric types and treats missing, invalid or negative values as zero. Live click aggregation is intentionally disabled on these paths to avoid an unbounded query; any existing persisted bundle click value is left intact, not recomputed.

`injectBundleEvaluationStatus` batch-loads current evaluation read models. A bundle without a row gets `not evaluated`; a row supplies status and latest completion time. If the read repository is nil, enrichment is skipped. These are presentation-time fields, not mutations of the bundle row.

## Trace and review

For creation, follow DTO → `validateBundleInput` → `newBundle` → transactional save and audit. For detail/list, follow `bundleItemFromModel` → statistics → evaluation status. When adding a field, update validation, `bundleInput`, `applyTo`, mapping and DTO together; otherwise create, update and read behavior can diverge.

## Complete function index

Follow the links to the implementation. Signatures show inputs, outputs and receiver ownership; source comments are included where available.

- [`NewBundleFlow`](../../business_flow/bundle_flow.go#L33): `func NewBundleFlow( bundleRepo repository.BundleRepository, campaignRepo repository.CampaignRepository, customerRepo repository.CustomerRepository, auditRepo repository.AuditLogRepository, evaluationReadRepo repository.BundleTagEvaluationReadRepository, db *gorm.DB, ) BundleFlow`
- [`CreateBundle`](../../business_flow/bundle_flow.go#L51): `func (f *BundleFlowImpl) CreateBundle( ctx context.Context, req *dto.CreateBundleRequest, metadata *ClientMetadata, ) (*dto.CreateBundleResponse, error)`
- [`UpdateBundle`](../../business_flow/bundle_flow.go#L109): `func (f *BundleFlowImpl) UpdateBundle( ctx context.Context, req *dto.UpdateBundleRequest, metadata *ClientMetadata, ) (*dto.UpdateBundleResponse, error)`
- [`GetBundle`](../../business_flow/bundle_flow.go#L182): `func (f *BundleFlowImpl) GetBundle( ctx context.Context, req *dto.GetBundleRequest, metadata *ClientMetadata, ) (*dto.GetBundleResponse, error)`
- [`ListBundles`](../../business_flow/bundle_flow.go#L220): `func (f *BundleFlowImpl) ListBundles( ctx context.Context, req *dto.ListBundlesRequest, metadata *ClientMetadata, ) (*dto.ListBundlesResponse, error)`
- [`bundleItemFromModel`](../../business_flow/bundle_flow.go#L296): `func bundleItemFromModel(bundle *models.Bundle) *dto.BundleItem`
- [`validateBundleInput`](../../business_flow/bundle_flow.go#L330): `func validateBundleInput( customer models.Customer, title, objective, targetAudiencePersona string, shortLinkDomain, description, adLink, targetCustomerName, category, job *string, ) (bundleInput, error)`
- [`newBundle`](../../business_flow/bundle_flow.go#L381): `func (input bundleInput) newBundle(customerID uint, now time.Time) *models.Bundle`
- [`applyTo`](../../business_flow/bundle_flow.go#L393): `func (input bundleInput) applyTo(bundle *models.Bundle)`
- [`validateRequiredBundleField`](../../business_flow/bundle_flow.go#L405): `func validateRequiredBundleField(value string, maxLen int, fieldName string) (string, error)`
- [`sanitizeOptionalBundleField`](../../business_flow/bundle_flow.go#L416): `func sanitizeOptionalBundleField(value *string, maxLen int, fieldName string) (*string, error)`
- [`bundleItemsToPointers`](../../business_flow/bundle_flow.go#L439): `func bundleItemsToPointers(items []dto.BundleItem) []*dto.BundleItem`
- [`injectBundleStatistics`](../../business_flow/bundle_flow.go#L447): `func (f *BundleFlowImpl) injectBundleStatistics(ctx context.Context, customerID uint, items []*dto.BundleItem) error`
- [`aggregateBundleStatistics`](../../business_flow/bundle_flow.go#L497): `func (f *BundleFlowImpl) aggregateBundleStatistics( ctx context.Context, campaigns []*models.Campaign, ) (map[uint]bundleStatisticsAggregate, error)`
- [`injectBundleEvaluationStatus`](../../business_flow/bundle_flow.go#L548): `func (f *BundleFlowImpl) injectBundleEvaluationStatus(ctx context.Context, items []*dto.BundleItem) error`
- [`unmarshalStatisticsMap`](../../business_flow/bundle_flow.go#L591): `func unmarshalStatisticsMap(raw json.RawMessage) map[string]any`
- [`parseUint64Stat`](../../business_flow/bundle_flow.go#L600): `func parseUint64Stat(statsMap map[string]any, key string) uint64`
