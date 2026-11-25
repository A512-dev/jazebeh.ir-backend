# `campaign_selected_tag_repository.go`

`CampaignSelectedTagRepositoryImpl` reads and replaces the smart tags selected for a campaign. It also builds the available-tag list and its performance/action read models.

Available tags are scoped to the bundle's targeting source, searchable, sortable through validated sort choices, and enriched with campaign test performance and bundle action metrics. `ListAvailableTagIDs` provides the ID-only form. `ListSelected` preserves selection order; `Summary` returns counts/selection state; `Validate` checks that persisted selections remain available for the campaign's bundle.

`Replace` validates tag availability and campaign editability, then replaces the ordered selection atomically. Invalid source tags return `ErrInvalidCampaignSelectedTags`; attempts to edit a campaign in a disallowed state return `ErrCampaignSelectedTagsNotEditable`. Replacing tags invalidates the campaign's smart-targeting test preview so stale preview results are not reused. `Clear` removes the campaign selection.
