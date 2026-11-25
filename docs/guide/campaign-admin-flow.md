# Admin campaign flow

`business_flow/campaign_admin_flow.go` supplies administrator campaign queries, decisions, scheduling and page pricing. `AdminCampaignFlow` is the interface; `AdminCampaignFlowImpl` coordinates repositories, database transactions and notifications. `NewAdminCampaignFlow` wires its dependencies. The campaign and wallet rows, rather than an HTTP request alone, determine whether a transition succeeds.

## Reads and display helpers

`ListCampaigns` applies admin filters, database ordering and pagination, and enriches rows with statistics, click data and pricing. `GetCampaign` builds the detailed admin view. `resolvePlatformBasePriceForAdminList` and `readCampaignPriceFactorsFromMetadata` prefer captured financial metadata where available; `parseMetadataFloat` handles a factor value. `formatCampaignPartyFullName` and `formatCampaignAgencyFullName` construct display names. These are display projections and must not be confused with recomputing a campaign's historical charge using today's price.

## Approval

`ApproveCampaign` requires a campaign ID and executes its critical work in a transaction. It locks and reloads the campaign, requires waiting-for-approval status and a schedule that is not in the past. Non-Excel campaigns lock the bundle because they use its audience ledger. Smart-targeting test approval verifies the current or permitted frozen sampling intent and the complete active test reservation; `currentOrFrozenSmartTargetingTestSamplingIntent` shares the campaign flow's rules. `validateApprovalPlatformSettings` verifies the selected platform configuration, including its required metadata; `parsePositiveIntMetadata` and `parseStringMetadata` validate metadata values.

The flow requires exactly one completed finalization freeze transaction, locks the customer's wallet, reads its latest snapshot and verifies the frozen balance. It creates a new snapshot moving the freeze amount to campaign spending, records a fee transaction and updates campaign status and comment, all in the same transaction. Customer notification is best effort after commit; audit logging records the result. A notification failure therefore does not roll back approval.

## Other transitions

`RejectCampaign` operates on a waiting campaign, records the decision and returns its frozen reservation to the appropriate balance under transaction and wallet controls. `CancelCampaign` handles approved campaigns or missed waiting-for-approval campaigns, with status-dependent financial reversal and reservation cleanup. Review the branch and its original transaction metadata before changing refund behavior: a frozen reservation and an already-approved charge are different states. `RescheduleCampaign` validates the campaign state and time, preserving lead-time rules for active schedules while allowing the explicitly supported missed-approval recovery path. `isAdminReschedulable`, `isAdminMissedPendingApprovalCampaign`, `isUTCInstant`, `tehranLocation` and `isWithinRescheduleWindow` implement those checks.

`UpdatePagePrice` records an administrator price change; `GetPagePrices` reads the current page prices. `normalizeIranMobile` supports notification delivery. Side effects outside the transaction, especially SMS, are best effort; persisted wallet and campaign transitions are the authoritative result.

## Complete function index

Follow the links to the implementation. Signatures show inputs, outputs and receiver ownership; source comments are included where available.

- [`currentOrFrozenSmartTargetingTestSamplingIntent`](../../business_flow/campaign_admin_flow.go#L61): `func (s *AdminCampaignFlowImpl) currentOrFrozenSmartTargetingTestSamplingIntent( ctx context.Context, campaign *models.Campaign, requireSatisfied bool, ) (*smartTargetingTestSamplingIntent, error)` — currentOrFrozenSmartTargetingTestSamplingIntent shares the campaign flow's immutable Test-selection validation with the approval flow without widening the admin flow's dependency surface.
- [`NewAdminCampaignFlow`](../../business_flow/campaign_admin_flow.go#L83): `func NewAdminCampaignFlow( campaignRepo repository.CampaignRepository, customerRepo repository.CustomerRepository, walletRepo repository.WalletRepository, balanceSnapshotRepo repository.BalanceSnapshotRepository, transactionRepo repository.TransactionRepository, auditRepo repository.AuditLogRepository, platformSettingsRepo repository.PlatformSettingsRepository, platformBaseRepo repository.PlatformBasePriceRepository, lineNumberRepo repository.LineNumberRepository, segmentPriceRepo repository.SegmentPriceFactorRepository, pagePriceRepo repository.PagePriceRepository, selectedTagRepo repository.CampaignSelectedTagRepository, db *gorm.DB, rc *redis.Client, notifier services.NotificationService, adminConfig config.AdminConfig, messageConfig config.MessageConfig, cacheConfig config.CacheConfig, ) AdminCampaignFlow` — NewAdminCampaignFlow creates a new campaign flow instance
- [`ListCampaigns`](../../business_flow/campaign_admin_flow.go#L126): `func (s *AdminCampaignFlowImpl) ListCampaigns( ctx context.Context, filter dto.AdminListCampaignsFilter, ) (*dto.AdminListCampaignsResponse, error)` — ListCampaigns retrieves campaigns for admin using optional filters.
- [`GetCampaign`](../../business_flow/campaign_admin_flow.go#L301): `func (s *AdminCampaignFlowImpl) GetCampaign(ctx context.Context, id uint) (*dto.AdminGetCampaignResponse, error)` — GetCampaign retrieves a single campaign by ID for admin
- [`resolvePlatformBasePriceForAdminList`](../../business_flow/campaign_admin_flow.go#L419): `func (s *AdminCampaignFlowImpl) resolvePlatformBasePriceForAdminList( ctx context.Context, platform string, cache map[string]*uint64, ) (*uint64, error)`
- [`formatCampaignPartyFullName`](../../business_flow/campaign_admin_flow.go#L441): `func formatCampaignPartyFullName(customer *models.Customer) *string`
- [`formatCampaignAgencyFullName`](../../business_flow/campaign_admin_flow.go#L465): `func formatCampaignAgencyFullName(customer *models.Customer) *string`
- [`readCampaignPriceFactorsFromMetadata`](../../business_flow/campaign_admin_flow.go#L472): `func (s *AdminCampaignFlowImpl) readCampaignPriceFactorsFromMetadata( ctx context.Context, campaignID uint, ) (*uint64, *float64, *float64, error)`
- [`parseMetadataFloat`](../../business_flow/campaign_admin_flow.go#L504): `func parseMetadataFloat(value any) *float64`
- [`ApproveCampaign`](../../business_flow/campaign_admin_flow.go#L520): `func (s *AdminCampaignFlowImpl) ApproveCampaign( ctx context.Context, req *dto.AdminApproveCampaignRequest, ) (*dto.AdminApproveCampaignResponse, error)` — ApproveCampaign approves a campaign: ensure schedule_at > now, change status to approved, and reduce frozen to locked spend
- [`validateApprovalPlatformSettings`](../../business_flow/campaign_admin_flow.go#L721): `func (s *AdminCampaignFlowImpl) validateApprovalPlatformSettings(ctx context.Context, campaign *models.Campaign) error`
- [`parsePositiveIntMetadata`](../../business_flow/campaign_admin_flow.go#L766): `func parsePositiveIntMetadata(metadata map[string]any, key string) (int64, error)`
- [`parseStringMetadata`](../../business_flow/campaign_admin_flow.go#L812): `func parseStringMetadata(metadata map[string]any, key string) (string, error)`
- [`RejectCampaign`](../../business_flow/campaign_admin_flow.go#L858): `func (s *AdminCampaignFlowImpl) RejectCampaign( ctx context.Context, req *dto.AdminRejectCampaignRequest, ) (*dto.AdminRejectCampaignResponse, error)` — RejectCampaign rejects a campaign: change status to rejected and refund frozen to free
- [`RescheduleCampaign`](../../business_flow/campaign_admin_flow.go#L1052): `func (s *AdminCampaignFlowImpl) RescheduleCampaign( ctx context.Context, req *dto.AdminRescheduleCampaignRequest, ) (*dto.AdminRescheduleCampaignResponse, error)` — RescheduleCampaign updates the scheduled time for eligible campaigns (admin action).
- [`CancelCampaign`](../../business_flow/campaign_admin_flow.go#L1146): `func (s *AdminCampaignFlowImpl) CancelCampaign( ctx context.Context, req *dto.AdminCancelCampaignRequest, ) (*dto.AdminCancelCampaignResponse, error)` — CancelCampaign cancels an approved campaign or a missed waiting-for-approval campaign.
- [`UpdatePagePrice`](../../business_flow/campaign_admin_flow.go#L1429): `func (s *AdminCampaignFlowImpl) UpdatePagePrice( ctx context.Context, req *dto.AdminUpdatePagePriceRequest, ) (*dto.AdminUpdatePagePriceResponse, error)`
- [`GetPagePrices`](../../business_flow/campaign_admin_flow.go#L1473): `func (s *AdminCampaignFlowImpl) GetPagePrices(ctx context.Context) (*dto.AdminGetPagePricesResponse, error)`
- [`isAdminReschedulable`](../../business_flow/campaign_admin_flow.go#L1502): `func isAdminReschedulable(status models.CampaignStatus) bool`
- [`isAdminMissedPendingApprovalCampaign`](../../business_flow/campaign_admin_flow.go#L1514): `func isAdminMissedPendingApprovalCampaign(campaign *models.Campaign, nowUTC time.Time) bool`
- [`isUTCInstant`](../../business_flow/campaign_admin_flow.go#L1524): `func isUTCInstant(t time.Time) bool`
- [`tehranLocation`](../../business_flow/campaign_admin_flow.go#L1529): `func tehranLocation() *time.Location`
- [`isWithinRescheduleWindow`](../../business_flow/campaign_admin_flow.go#L1540): `func isWithinRescheduleWindow(tehranTime time.Time) bool`
- [`normalizeIranMobile`](../../business_flow/campaign_admin_flow.go#L1554): `func normalizeIranMobile(m string) string`
