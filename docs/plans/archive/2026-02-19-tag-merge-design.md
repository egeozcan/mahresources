# Tag Merge Design

## Summary

Add the ability to merge tags, transferring all associations (resources, notes, groups) from loser tags to a winner tag, then deleting the losers. Follows the established merge pattern used by groups and resources.

## Data Model

Add a `Meta` (datatypes.JSON) field to the `Tag` model. Nullable, auto-migrated by GORM. Stores merge backup data in `meta.backups` keyed by `tag_{id}`.

## Business Logic

New file: `application_context/tag_bulk_context.go`

`MergeTags(winnerId uint, loserIds []uint) error`:

1. Validate winner not in losers, all IDs non-zero
2. In a single DB transaction:
   - Transfer `resource_tags` from losers to winner (skip duplicates via ON CONFLICT)
   - Transfer `note_tags` from losers to winner (skip duplicates)
   - Transfer `group_tags` from losers to winner (skip duplicates)
   - Serialize loser tags to JSON, store in `winner.Meta.backups`
   - Log the merge operation
   - Delete loser tags (cascade removes stale join entries)
3. Invalidate search cache

Winner keeps its own name and description unchanged.

## API

- New interface: `TagMerger` in `server/interfaces/tag_interfaces.go`
- New route: `POST /v1/tags/merge`
- Handler: `GetMergeTagsHandler` in `server/api_handlers/tag_api_handlers.go`
- Uses existing `MergeQuery` (winner + losers) from `query_models`

## UI

Two entry points:

### Tag Detail Page (`displayTag.tpl`)

Add merge form in sidebar:
- Winner pre-set as current tag (hidden input)
- Autocompleter to search/select loser tags
- Confirmation dialog before merge
- Follows the same pattern as group detail page merge form

### Tag List Page (`listTags.tpl`)

Add bulk selection support:
- New `partials/bulkEditorTag.tpl` following group/resource pattern
- Checkbox on each tag card for selection
- Bulk action bar with merge form (pick winner via autocompleter, selected tags become losers)
- Bulk delete for consistency

## Testing

- E2E test: create tags, assign to entities, merge, verify associations transferred and losers deleted
- E2E test: verify merge from tag detail page
- Unit test: MergeTags logic (validation, association transfer, backup storage)
