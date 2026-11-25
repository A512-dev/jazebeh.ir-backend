# `campaign_selected_tag_repository.go`

`CampaignSelectedTagRepositoryImpl` reads and replaces the smart tags selected for a campaign. It also builds the available-tag list and its performance/action read models.

Available tags are scoped to the bundle's targeting source, searchable, sortable through validated sort choices, and enriched with campaign test performance and bundle action metrics. `ListAvailableTagIDs` provides the ID-only form. `ListSelected` preserves selection order; `Summary` returns counts/selection state; `Validate` checks that persisted selections remain available for the campaign's bundle.

`Replace` validates tag availability and campaign editability, then replaces the ordered selection atomically. Invalid source tags return `ErrInvalidCampaignSelectedTags`; attempts to edit a campaign in a disallowed state return `ErrCampaignSelectedTagsNotEditable`. Replacing tags invalidates the campaign's smart-targeting test preview so stale preview results are not reused. `Clear` removes the campaign selection.
## Types and methods

`CampaignSelectedTagRepositoryImpl` stores the database; `NewCampaignSelectedTagRepository` constructs it and `getDB` honors a context transaction. `campaignSelectedTagSnapshot` is an internal projection of tag details and performance metrics. `ErrInvalidCampaignSelectedTags` signals unavailable or mismatched tags; `ErrCampaignSelectedTagsNotEditable` signals a campaign that is not editable smart targeting.

- `ListAvailable` returns the available tag projection with filtering, sorting and pagination; `ListAvailableTagIDs` returns matching IDs. `baseAvailableQuery`, `availableSmartTagsQuery` and `listAvailableRowsQuery` build the source and projection. `escapeLike` and `applySmartTagSearch` build escaped searches; `smartTagOrder` validates sort choices. `applyCampaignTestPerformance` and `applyActionTagMetrics` join derived performance data.
- `ListSelected` uses `orderedCampaignSelectedTagsQuery` to preserve selection order. `Summary` counts selected tags and sums their snapshotted raw capacity. `Validate` checks that every selected tag still belongs to the bundle's available targeting source.
- `Replace` locks the campaign, checks bundle and editability, validates tags and snapshots title, audience count, fit score and CTR/ATR values. An identical ordered selection is a no-op. Otherwise it invalidates the test preview, deletes old selections and inserts new rows in caller order. `sameOrderedCampaignSelectedTags` detects the no-op and `buildCampaignSelectedTagRows` restores request order after unordered database reads. The caller should supply a transaction for the lock and replacement to be atomic.
- `Clear` deletes the campaign's selected tags and calls `invalidateCampaignSmartTargetingTestPreview`; it likewise needs a surrounding transaction when atomicity is required.
