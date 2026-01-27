---
sidebar_position: 6
---

# Bulk Operations

Bulk operations let you modify multiple items at once. Mahresources supports bulk tag management, metadata updates, group assignments, and deletion across resources, notes, and groups.

## Selecting Items

Before performing bulk operations, you need to select the items you want to modify.

### Individual Selection

Click the checkbox next to any item in a list view to select it.

### Keyboard Selection

With items selected, use keyboard shortcuts:

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

When one or more items are selected, the bulk editor appears above the list with available operations.

### Editor Interface

The bulk editor shows:
- **Deselect** button
- **Select All** button
- Operation buttons (collapsed by default)
- Active operation form (when expanded)

Click an operation button to expand its form. Click again to collapse.

### Available Operations by Entity Type

| Operation | Resources | Notes | Groups |
|-----------|:---------:|:-----:|:------:|
| Add Tags | Yes | Yes | Yes |
| Remove Tags | Yes | Yes | Yes |
| Add Metadata | Yes | Yes | Yes |
| Add Groups | Yes | - | - |
| Update Dimensions | Yes | - | - |
| Compare | Yes (2 only) | - | - |
| Delete | Yes | Yes | Yes |

## Adding Tags

Add one or more tags to all selected items:

1. Select items in the list
2. Click **Add Tag** in the bulk editor
3. Search for tags using the autocomplete
4. Select one or more tags
5. Click **Add**

Tags are added immediately. Existing tags on items are preserved.

### Creating Tags During Bulk Add

If the tag you need doesn't exist:

1. Type the new tag name
2. Click the **+** button or press Enter
3. The tag is created and selected
4. Continue with the add operation

## Removing Tags

Remove tags from all selected items:

1. Select items in the list
2. Click **Remove Tag** in the bulk editor
3. Search for and select tags to remove
4. Click **Remove**

Only the specified tags are removed. Other tags remain.

## Adding Metadata

Add or update metadata across selected items:

1. Select items in the list
2. Click **Add Metadata** in the bulk editor
3. Enter a key name
4. Enter a value
5. Add more key-value pairs if needed
6. Click **Add**

### Metadata Behavior

- New keys are added to items
- Existing keys are updated with the new value
- Other metadata keys are preserved

## Adding to Groups

For resources, add selected items to groups:

1. Select resources in the list
2. Click **Add Groups** in the bulk editor
3. Search for and select target groups
4. Click **Add**

Resources are added as **related** to the selected groups (not owned).

## Updating Dimensions

For image resources, recalculate width and height:

1. Select image resources
2. Click **Update Dimensions** in the bulk editor
3. Click **Update Dimensions**

This re-reads each image file and updates the stored dimension values.

## Comparing Resources

Compare two resources side-by-side:

1. Select exactly 2 resources
2. The **Compare** button appears
3. Click **Compare**
4. A comparison view opens

The comparison view varies by content type:
- **Images** - Side-by-side or overlay comparison
- **Text** - Diff view highlighting changes
- **PDFs** - Page-by-page comparison
- **Other** - Binary file information

## Bulk Deletion

Delete multiple items at once:

1. Select items to delete
2. Click **Delete Selected** in the bulk editor
3. Confirm the deletion in the popup

:::danger

Bulk deletion is permanent and cannot be undone. For resources, files are removed from storage.

:::

## Merging Resources

When viewing similar images on a resource detail page:

1. The similar resources section shows visually similar images
2. Click **Merge Others To This**
3. Confirm the merge

The merge operation:
- Keeps the current resource
- Transfers metadata from merged resources
- Deletes the merged resources

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

## Bulk Operation Tips

### Working with Large Selections

- Operations apply only to items on the current page
- For very large operations, work in batches
- The page refreshes after each operation

### Undo

Bulk operations cannot be undone through the UI. To recover:
- Restore from backups
- Manually reverse the changes

### Performance

- Tag operations are fast (database updates only)
- Metadata operations are fast
- Deletion may take time for many resources (file removal)
- Dimension updates require reading each file

### Error Handling

If an operation fails:
- Partial completion is possible (some items modified)
- Error messages display what went wrong
- Retry the operation after fixing issues

## Keyboard Shortcuts Summary

| Shortcut | Action |
|----------|--------|
| Click checkbox | Toggle single selection |
| Shift + Click | Select range |
| Right-click | Select range (alternative) |
| Space (with text selected) | Toggle selected checkboxes |

## Best Practices

### Before Bulk Operations

1. **Verify selection** - Check the selected count
2. **Consider backups** - Especially before deletion
3. **Test with small batches** - When trying new operations

### Tag Management

- Use bulk add to apply common tags
- Use bulk remove to clean up incorrect tags
- Create consistent tag naming conventions

### Metadata Consistency

- Use bulk metadata to standardize fields
- Autocomplete suggests existing keys
- Apply metadata in batches by type

### Organizing Content

- Bulk add to groups to organize imports
- Use filters first to select related items
- Merge duplicates after identification
