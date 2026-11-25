# `bundle_audience_selection_repository.go`

`BundleAudienceSelectionRepositoryImpl` persists the audience allocation associated with a campaign. It stores a selection record and normalized member rows, while exposing the selected audience IDs on the returned model.

## Reading an allocation

`ByCampaignID` loads the selection for a campaign and then reads its member audience IDs ordered by `selection_order`. If no selection exists, it returns `(nil, nil)`.

## Creating an allocation

`InsertForCampaign` appends an immutable selection for the supplied customer, bundle, campaign, correlation ID, and audience IDs.

1. It locks the bundle row (`FOR UPDATE`) to serialize allocations for that bundle.
2. If a selection for the campaign already exists, it returns that original allocation unchanged after checking the customer, bundle, and stored count are consistent. This makes retries idempotent; a retry does not replace the original ID list.
3. For a new allocation, it rejects non-positive or duplicate audience IDs.
4. It inserts the selection and ordered member rows, using batches of up to 1,000 members.

When the context already contains a transaction, the method uses it and leaves transaction completion to the caller. Otherwise it wraps the persistence operation in its own GORM transaction. This allows the caller to make allocation and related bundle changes atomic together.
# Types and function reference

`BundleAudienceSelectionRepository` exposes lookup and insertion; `BundleAudienceSelectionRepositoryImpl` holds `DB`. `NewBundleAudienceSelectionRepository` constructs it. `getDB` uses the transaction stored under `TxContextKey` when available, otherwise the repository database.

- `ByCampaignID(ctx, campaignID)` delegates to `loadBundleAudienceSelectionByCampaign`. The helper loads the selection, then its normalized members ordered by `selection_order`, and fills `SelectedAudienceIDs`. A missing selection returns `(nil, nil)`; database failures are returned.
- `InsertForCampaign(ctx, customerID, bundleID, campaignID, correlationID, ids)` persists an immutable selection and its ordered members. It locks the bundle row and first checks for an existing campaign selection. A retry returns that original selection unchanged, after checking customer, bundle and stored member count; it does not replace the allocation with the supplied IDs. For a new selection it rejects nonpositive or duplicate IDs, creates the selection and members transactionally, and preserves input order. It participates in a context transaction when supplied; otherwise it starts its own transaction. Database constraints still enforce conflicts with other bundle allocations.
