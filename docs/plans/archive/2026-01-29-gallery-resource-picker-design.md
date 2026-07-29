# Gallery Block Resource Picker Design

## Overview

Replace the comma-separated ID input in the gallery block with a visual resource picker modal. Users can select resources from the current note's attachments or search/filter all resources.

## Requirements

- Visual thumbnail-based selection (galleries are visual, users need to see images)
- Two sources: note's attached resources (quick access) + all resources (search)
- Tag and group filters via existing autocomplete component
- Multi-select capability
- Reuse existing patterns (globalSearch modal, autocompleter, bulkSelection)

## Modal Structure

```
┌─────────────────────────────────────────────────────┐
│  Select Resources                              [X]  │
├─────────────────────────────────────────────────────┤
│  [Note's Resources]  [All Resources]                │  ← Tab buttons
├─────────────────────────────────────────────────────┤
│  🔍 [Search by name...]                             │
│  Tag: [autocomplete dropdown]                       │  ← Filters (All Resources tab only)
│  Group: [autocomplete dropdown]                     │
├─────────────────────────────────────────────────────┤
│  ┌─────┐ ┌─────┐ ┌─────┐ ┌─────┐                   │
│  │ ☑️  │ │     │ │ ☑️  │ │     │   ...             │  ← Thumbnail grid
│  │ img │ │ img │ │ img │ │ img │                   │
│  └─────┘ └─────┘ └─────┘ └─────┘                   │
│                                                     │
├─────────────────────────────────────────────────────┤
│  3 selected                    [Cancel]  [Confirm]  │
└─────────────────────────────────────────────────────┘
```

## Component Architecture

### New Files

- `src/components/blocks/resourcePicker.js` - Modal component (Alpine.js)

### Modified Files

- `src/components/blocks/blockGallery.js` - Add method to open picker
- `templates/partials/blockEditor.tpl` - Add picker modal HTML and button
- `src/main.js` - Import and register component

### Integration with Existing Autocompleter

Tag/group filters use the existing `autocompleter` component in standalone mode:

```javascript
x-data="autocompleter({
    selectedResults: [],
    max: 1,
    url: '/v1/tags',
    standalone: true,
    onSelect: (tag) => $dispatch('filter-changed', { tags: [tag.ID] })
})"
```

### Data Flow

1. Gallery block calls `openResourcePicker(noteId)`
2. Picker fetches note's resources and opens modal
3. User filters/searches and selects resources
4. User clicks "Confirm"
5. Picker calls callback with selected IDs
6. Gallery's existing `addResources(ids)` method receives them

## API Interactions

### Fetching Note's Resources

```
GET /v1/resources?ownerId={noteId}
```

### Fetching All Resources (with filters)

```
GET /v1/resources?name={search}&Tags={tagId}&Groups={groupId}&MaxResults=50
```

Parameters:
- `name` - text search
- `Tags` - filter by tag ID
- `Groups` - filter by group ID
- `MaxResults` - pagination limit

### Existing Endpoints (no changes needed)

- `/v1/tags?name={search}` - tag autocomplete
- `/v1/groups?name={search}` - group autocomplete
- `/v1/resource/preview?id={id}` - thumbnails

## UI/UX Behavior

### Modal

- Opens via "Select Resources" button in gallery edit mode
- Closes via X button, Cancel, Escape key, or backdrop click
- Focus trapped inside modal while open

### Tabs

- "Note's Resources" is default if note has resources
- Switching tabs preserves selections
- Filters only visible on "All Resources" tab

### Selection

- Click thumbnail to toggle selection
- Checkmark overlay + border highlight on selected
- Selection count shown at bottom
- Already-in-gallery resources show "Already added" badge

### Search & Filters

- Search debounced (200ms)
- Tag/group filters are single-select with clear button
- Filters combine with AND logic

### Confirm

- Disabled if nothing selected
- Merges selected IDs into gallery (duplicates prevented)
- Closes modal

### Empty States

- Note's Resources: "No resources attached to this note"
- All Resources: "No resources found"

## Accessibility

- `role="dialog"` and `aria-modal="true"` on modal
- Focus to search input on open, back to trigger on close
- Tab list: `role="tablist"`, tabs: `role="tab"` with `aria-selected`
- Grid: `role="listbox"`, items: `role="option"` with `aria-selected`
- Keyboard: Space/Enter toggles selection, Escape closes
- ARIA live region announces selection changes

## Error Handling

- Network errors: inline message with retry button
- Loading state: skeleton/spinner in grid
- Autocompleter errors: handled by existing component
- Invalid IDs on confirm: API ignores gracefully

## Edge Cases

- New unsaved note: disable "Note's Resources" tab with message
- Large result sets: `MaxResults=50` with "Load more"
- Deleted resources: handled gracefully on confirm
