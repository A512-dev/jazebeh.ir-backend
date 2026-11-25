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
