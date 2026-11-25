# Bot campaign flow

`business_flow/campaign_bot_flow.go` implements the bot-facing readiness feed, status transitions, audience-file access and delivery reporting. `BotCampaignFlow` defines the operations; `BotCampaignFlowImpl` owns the repositories and dependencies wired by `NewBotCampaignFlow`.

## Readiness and delivery data

`ListReadyCampaigns` lists campaigns ready for the bot, optionally for one platform, and maps campaign, customer, pricing and targeting information. It may perform per-campaign lookups; the source calls out an N+1 concern. Smart-targeting test readiness uses `currentOrFrozenSmartTargetingTestSamplingIntent`, sharing the frozen-preview policy with the customer and admin flows. `selectedTags` resolves tag names for the response. `executionExcelFileUUID` extracts the applicable Excel targeting file reference. `resolvePlatformBasePrice` and `readPlatformBasePriceFromMetadata` support price display, preferring historical metadata where available; `loadPlatformSettingsSpecs` loads relevant settings.

`DownloadTargetAudienceExcelFile` resolves the campaign's configured Excel asset and reads the file for delivery. File UUID, asset and stored-path checks are important here; the returned bytes are not a new audience selection. Other targeting modes use the scheduler and reservation paths rather than assuming an Excel asset exists.

## State transitions and reports

`MoveCampaignToRunning` and `MoveCampaignToExecuted` perform the bot-facing transitions with guarded campaign state. Execution completion is significant for refund reconciliation: the database trigger installed with the refund queue creates durable reconciliation work when a campaign reaches executed status. A bot response should not be treated as a replacement for that queue.

`UpdateCampaignStatistics` merges the reporter's JSON patch rather than replacing the full statistics object, preserving independently written reconciliation fields. `PushCampaignAudienceUIDs` appends audience UID/short-code pairs to the campaign's durable file-backed store in batches; reports depend on these records and retain them. The store is an audience/report input, not evidence that a message was delivered. Delivery status and filtered click records have their own sources.

## Complete function index

Follow the links to the implementation. Signatures show inputs, outputs and receiver ownership; source comments are included where available.

- [`currentOrFrozenSmartTargetingTestSamplingIntent`](../../business_flow/campaign_bot_flow.go#L52): `func (s *BotCampaignFlowImpl) currentOrFrozenSmartTargetingTestSamplingIntent( ctx context.Context, campaign *models.Campaign, requireSatisfied bool, ) (*smartTargetingTestSamplingIntent, error)` — currentOrFrozenSmartTargetingTestSamplingIntent shares the campaign flow's immutable Test-selection validation with the bot readiness feed.
- [`NewBotCampaignFlow`](../../business_flow/campaign_bot_flow.go#L66): `func NewBotCampaignFlow( campaignRepo repository.CampaignRepository, multimediaRepo repository.MultimediaAssetRepository, platformSettingsRepo repository.PlatformSettingsRepository, transactionRepo repository.TransactionRepository, platformBaseRepo repository.PlatformBasePriceRepository, selectedTagRepo repository.CampaignSelectedTagRepository, lineNumberRepo repository.LineNumberRepository, cacheConfig config.CacheConfig, db *gorm.DB, rc *redis.Client, ) BotCampaignFlow`
- [`ListReadyCampaigns`](../../business_flow/campaign_bot_flow.go#L93): `func (s *BotCampaignFlowImpl) ListReadyCampaigns(ctx context.Context, platform *string) (*dto.BotListCampaignsResponse, error)` — ListReadyCampaigns retrieves ready campaigns for bot
- [`selectedTags`](../../business_flow/campaign_bot_flow.go#L210): `func (s *BotCampaignFlowImpl) selectedTags(ctx context.Context, campaign *models.Campaign) ([]string, error)`
- [`executionExcelFileUUID`](../../business_flow/campaign_bot_flow.go#L242): `func executionExcelFileUUID(spec models.CampaignSpec) *string`
- [`resolvePlatformBasePrice`](../../business_flow/campaign_bot_flow.go#L249): `func (s *BotCampaignFlowImpl) resolvePlatformBasePrice(ctx context.Context, campaignID uint, platform string) (*uint64, error)`
- [`readPlatformBasePriceFromMetadata`](../../business_flow/campaign_bot_flow.go#L268): `func (s *BotCampaignFlowImpl) readPlatformBasePriceFromMetadata(ctx context.Context, campaignID uint) (*uint64, error)`
- [`loadPlatformSettingsSpecs`](../../business_flow/campaign_bot_flow.go#L295): `func (s *BotCampaignFlowImpl) loadPlatformSettingsSpecs( ctx context.Context, campaigns []*models.Campaign, ) (map[uint]*dto.BotCampaignPlatformSettingsSpec, error)`
- [`MoveCampaignToRunning`](../../business_flow/campaign_bot_flow.go#L335): `func (s *BotCampaignFlowImpl) MoveCampaignToRunning(ctx context.Context, campaignID uint) error` — MoveCampaignToRunning moves campaign status to running
- [`MoveCampaignToExecuted`](../../business_flow/campaign_bot_flow.go#L365): `func (s *BotCampaignFlowImpl) MoveCampaignToExecuted(ctx context.Context, campaignID uint) error` — MoveCampaignToExecuted moves campaign status to executed
- [`DownloadTargetAudienceExcelFile`](../../business_flow/campaign_bot_flow.go#L398): `func (s *BotCampaignFlowImpl) DownloadTargetAudienceExcelFile(ctx context.Context, campaignID uint) (string, string, []byte, error)` — DownloadTargetAudienceExcelFile downloads campaign target-audience Excel file when available.
- [`UpdateCampaignStatistics`](../../business_flow/campaign_bot_flow.go#L476): `func (s *BotCampaignFlowImpl) UpdateCampaignStatistics( ctx context.Context, campaignID uint, statistics map[string]any, ) (*dto.BotUpdateCampaignStatisticsResponse, error)` — UpdateCampaignStatistics updates the statistics JSON field of a campaign
- [`PushCampaignAudienceUIDs`](../../business_flow/campaign_bot_flow.go#L513): `func (s *BotCampaignFlowImpl) PushCampaignAudienceUIDs(ctx context.Context, campaignID uint, items []dto.BotAudienceUIDItem) error` — PushCampaignAudienceUIDs appends a batch of audience uid/code pairs to the campaign's file-backed store. Called repeatedly for large campaigns (one call per scheduler chunk). These files are durable report inputs and are intentionally retained indefinitely.
