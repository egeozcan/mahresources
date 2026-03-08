---
sidebar_position: 6
---

# Bulk Operations

Bulk operations let you modify multiple resources, notes, or groups at once: add/remove tags, update metadata, assign groups, or delete.

## Selecting Items

![Bulk selection toolbar with tag and group operations](/img/bulk-selection.png)

Click the checkbox next to any item in a list view to select it.

| Action | Method |
|--------|--------|
| Toggle selection | Click checkbox or press Space |
| Range select | Shift+Click or Right-click |
| Select text range then toggle | Select text with mouse, press Space |

### Range Selection

Select multiple consecutive items:

1. Click the first item's checkbox
2. Hold Shift and click the last item's checkbox
3. All items between are selected

Alternatively, right-click any item to select from the last-clicked item to the right-clicked item.

### Select All

Click the **Select All** button above the list to select all visible items on the current page.

### Deselect All

Click **Deselect All** or clear individual checkboxes to deselect items.

## The Bulk Editor

When you select one or more items, the bulk editor appears inline above the list. It shows all available operations simultaneously (Add Tag, Remove Tag, Add Metadata, etc.) along with Deselect and Select All buttons.

### Available Operations by Entity Type

| Operation | Resources | Notes | Groups | Tags |
|-----------|:---------:|:-----:|:------:|:----:|
| Add Tags | Yes | Yes | Yes | - |
| Remove Tags | Yes | Yes | Yes | - |
| Replace Tags | Yes | - | - | - |
| Add Metadata | Yes | Yes | Yes | - |
| Add Groups | Yes | Yes | - | - |
| Update Dimensions | Yes | - | - | - |
| Compare | Yes (2 only) | - | - | - |
| Merge | Yes | - | - | Yes |
| Delete | Yes | Yes | Yes | Yes |

## Adding Tags

1. Select items in the list
2. In the **Add Tag** form in the bulk editor, search for tags using autocomplete
3. Click **Add**

Tags are added immediately. Existing tags on items are preserved. If a tag doesn't exist yet, type the name and click **+** to create it.

## Removing Tags

1. Select items in the list
2. In the **Remove Tag** form, search for and select tags to remove
3. Click **Remove**

Only the specified Tags are removed. Other Tags remain.

## Replacing Tags

Replace all Tags on selected Resources with a new set:

1. Select Resources in the list
2. In the **Replace Tags** form, search for and select the new Tags
3. Click **Replace**

All existing Tags on the selected Resources are removed and replaced with the specified set. Available for Resources only.

## Adding Metadata

1. Select items in the list
2. In the **Add Metadata** form, enter a key and value
3. Add more key-value pairs if needed
4. Click **Add**

New keys are added to items. Existing keys are overwritten with the new value. Other metadata keys are preserved.

## Adding to Groups

1. Select resources in the list
2. In the **Add Groups** form, search for and select target groups
3. Click **Add**

Resources are added as **related** to the selected groups (not owned). This operation is available for resources only.

## Updating Dimensions

Select image resources and click **Update Dimensions** to re-read each file and update stored width/height values.

## Comparing Resources

Compare two resources side-by-side:

1. Select exactly 2 resources
2. The **Compare** button appears
3. Click **Compare**
4. A comparison view opens

The comparison view shows metadata differences between the two Resources: size delta, whether they share the same hash, content type match, and dimension differences.

For image resources, four visual comparison modes are available: side-by-side, slider, overlay, and toggle. For text-based resources, a unified or split diff view highlights line-level changes. Cross-resource version comparison is also supported.

## Bulk Deletion

Delete multiple items at once:

1. Select items to delete
2. Click **Delete Selected** in the bulk editor
3. Confirm the deletion in the popup

:::warning

Bulk deletion removes database records. For Resources, files are backed up to the `/deleted/` directory before removal. Files are only physically deleted if no other Resources or versions reference the same hash.

:::

## Merging Resources

Merge resources from two locations:

**From the similar resources section** on a resource detail page:

1. The similar resources section shows visually similar images
2. Click **Merge Others To This**
3. Confirm the merge

**From bulk selection** in the resource list:

1. Select the resources to merge
2. Use the merge operation
3. Select the winner resource (the one to keep)

The merge operation:
- Keeps the winner resource
- Transfers metadata from merged resources
- Deletes the merged (loser) resources

## Merging Tags

Combine duplicate tags into one:

1. Select tags in the tag list
2. Use the merge operation
3. Select the winner tag

All entity associations from the loser tags transfer to the winner. The loser tags are deleted.

## Merging Groups

Combine multiple groups into one:

1. Navigate to the "winner" group
2. In the sidebar, use the merge autocomplete
3. Select groups to merge
4. Click **Merge**

The merge operation:
- Moves all owned content to the winner
- Updates relations to point to the winner
- Deletes the merged groups

## Limitations

- Operations apply only to items on the current page. For large sets, work in batches.
- Bulk operations cannot be undone. Restore from backups if needed.
- Partial failures are possible: some items may already be modified if an operation fails midway. Check results and retry.
- Deleting many resources may be slow because files must be removed from disk.

## Keyboard Shortcuts Summary

| Shortcut | Action |
|----------|--------|
| Click checkbox | Toggle single selection |
| Shift + Click | Select range |
| Right-click | Select range (alternative) |
| Space (with text selected) | Toggle selected checkboxes |
