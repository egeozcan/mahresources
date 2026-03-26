# Bug Report - Cycle 3.2 (Deep Targeted Testing)

**Date**: 2026-03-26
**Tester**: Claude QA
**App URL**: http://localhost:8181

## Test Areas
1. Plugin system
2. Note blocks system
3. Resource categories
4. Meta field complex operations
5. Shared notes
6. Settings panel
7. Gallery/lightbox
8. Custom header/sidebar/footer

---

## Bugs Found

### BUG 1: Meta filter on notes list pages does not submit when only field name is filled
**Severity**: Low
**Page**: `/notes` (and any list page with meta filter)
**Steps to reproduce**:
1. Navigate to `/notes`
2. In the sidebar filter panel, find the "Meta" section
3. Click "+ Add Field"
4. Enter only a field name (e.g. "nested") but leave the value empty
5. Click "Apply Filters"
**Expected**: URL should include meta filter params and results should be filtered to notes that have the specified meta key (regardless of value)
**Actual**: The URL after submission contains no meta filter parameters. The meta field row is lost. All notes are returned unfiltered.
**Root cause**: In `templates/partials/form/freeFields.tpl` line 28-29, the hidden input is only generated when `field.name && field.value && !jsonOutput`. This means filtering by key-existence-only (without specifying a value) is impossible via the UI.

### BUG 2: Notes meta filter causes 500 Internal Server Error ("ambiguous column name: meta")
**Severity**: High
**Page**: `/notes` (HTML template path only; JSON API `/v1/notes` works fine)
**Steps to reproduce**:
1. Navigate to `/notes`
2. In the sidebar, click "+ Add Field" under Meta
3. Enter a field name (e.g. "color") and value (e.g. "blue")
4. Select any comparison operator (defaults to "=")
5. Click "Apply Filters"
**Expected**: Notes should be filtered to only those matching the meta key/value
**Actual**: Server returns HTTP 500 with error message "ambiguous column name: meta" displayed on the page in an alert box
**URL**: `/notes?MetaQuery.0=color%3AEQ%3A%22blue%22`
**Root cause**: In `models/database_scopes/note_scope.go` line 105, the meta query uses an unqualified `meta` column reference:
```go
dbQuery = dbQuery.Where(types.JSONQuery("meta").Operation(...))
```
When the template context provider calls `GetPopularNoteTags()` (in `application_context/note_context.go` lines 234-237), which JOINs with the `tags` table (`INNER JOIN note_tags ... INNER JOIN tags t ...`), SQLite cannot disambiguate which table's `meta` column is being referenced since both `notes` and `tags` have a `meta` column.

The fix is to qualify the column as `notes.meta`:
```go
dbQuery = dbQuery.Where(types.JSONQuery("notes.meta").Operation(...))
```
This matches the pattern already used in:
- `group_scope.go` line 209: `types.JSONQuery("groups.meta")`
- `resource_scope.go` line 150: `types.JSONQuery("resources.meta")`

This bug only affects the HTML rendering path (not the JSON API at `/v1/notes`) because only the template handler calls `GetPopularNoteTags()` which introduces the ambiguous JOIN.
**Verified**: Resources (`/resources`) and Groups (`/groups`) meta filters work correctly because they use properly qualified column names.

---

## Test Log

### 1. Plugin System
- **Status**: PASS
- Navigated to `/plugins/manage` - 3 plugins displayed: example-blocks, example-plugin, fal-ai
- Enabled example-plugin: footer updated with greeting message, button changed to "Disable"
- Enabled example-blocks: status toggled correctly
- Changed example-plugin greeting from "Hello from Example Plugin!" to "Test Greeting Changed!" and clicked Save Settings - "Saved!" confirmation shown
- After page reload, new greeting persists in footer and settings field
- Purge Data on disabled plugin: confirmation dialog shown, accepted successfully
- No console errors throughout plugin testing

### 2. Note Blocks System
- **Status**: PASS
- Created note "Test Note for Blocks" (ID 3)
- Clicked "Edit Blocks" - empty state shows "No blocks yet." with "Click Add Block below to get started."
- Block type picker shows 9 types: Heading, Text, Counter, References, Table, Todos, Calendar, Divider, Gallery
- Added Text block: typed content "This is my first text block content"
- Added Heading block: typed "My Test Heading", level selector shows H1/H2/H3 options
- Reordering: Move up/down buttons correctly enabled/disabled at boundaries; moving heading above text worked correctly
- Added Divider block: separator renders properly
- Deleted heading block: confirmation dialog shown, block removed
- Added Todos block: "+ Add item" works, items have text input and delete button, checkboxes persist state after page reload
- Read mode rendering: blocks display correctly (text as paragraph, heading as h2, divider as separator, todos as checkbox list)

### 3. Resource Categories
- **Status**: PASS
- Created "Test Images Category" at `/resourceCategory/new` with name and description
- Category detail page shows correctly with edit name, description, and resources section
- Edit form includes: Name, Description, Custom Header, Custom Sidebar, Custom Summary, Custom Avatar, Meta JSON Schema

### 4. Meta Field Complex Operations
- **Status**: PARTIAL PASS (see BUG 1, BUG 2)
- Added nested JSON meta `{"key": [1,2,3]}` to note via edit form - saved successfully
- Meta renders in sidebar with expandable tree view: nested > key > [1, 2, 3]
- Fullscreen button available for meta table
- Meta filter form has comparison operators: =, LIKE, <>, NOT LIKE, >, >=, <, <=
- **FAIL**: Meta filter with name-only (no value) does not submit (BUG 1)
- **FAIL**: Meta filter with both name and value causes 500 error on `/notes` (BUG 2)
- Meta filter works correctly on `/resources` and `/groups` (properly qualified column names)

### 5. Shared Notes
- **Status**: PASS
- "Share Note" button on note detail page sidebar
- Clicking Share: status changes to "Shared", share URL displayed in text input with Copy URL button
- Share URL uses separate share server (port 8383) - by design
- "Unshare" button appears and works: reverts to "Share Note" button
- "Shared Only" filter on notes list: correctly filters to only shared notes

### 6. Settings Panel
- **Status**: PASS
- Gear icon opens settings dropdown with "Show Descriptions" checkbox
- Unchecking removes descriptions from list view cards
- Setting persists across page navigation (stored client-side)
- Re-checking re-enables description display

### 7. Gallery/Lightbox
- **Status**: PASS
- Uploaded 3 test PNG images (Red, Green, Blue)
- Resources list shows thumbnails in all views: Thumbnails, Details (table), Simple
- Clicking thumbnail opens lightbox dialog with: image, Previous/Next buttons, Close button, image counter (1/3), dimensions (50x50), 100%/fullscreen buttons, Edit Tags button, Edit button
- Navigation: Next/Previous buttons work, properly disabled at boundaries
- Only image resources included (text file excluded from count: "3" images shown, not 4)
- Keyboard: ArrowRight navigates to next, Escape closes dialog
- Close button works

### 8. Custom Header/Sidebar/Footer
- **Status**: PASS
- Resource category edit form has Custom Header, Custom Sidebar, Custom Summary, Custom Avatar fields
- Plugin system example-plugin injects custom footer content (greeting message)
- Custom HTML injection is intentionally allowed (documented in CLAUDE.md)

### Additional Testing

#### Admin Overview
- Server Health: uptime, memory, GC, goroutines, Go version, DB info
- Configuration: bind address, storage, DB type, FFmpeg, FTS, hash workers, etc.
- Data Overview: entity counts with weekly change indicators
- Detailed Statistics: storage by content type, top tags, top categories, orphaned resources, similarity detection, log statistics

#### Timeline Views
- Notes timeline: bar chart with monthly activity, Created/Updated toggle, Year/Month/Week granularity, time navigation

#### Global Search (Cmd+K)
- Cross-entity search: finds notes, queries, resource categories, resources
- Matching terms highlighted with `<mark>` tags
- Keyboard hints shown: arrows navigate, Enter selects, Esc closes

#### Group Tree View
- Tree view shows root groups as starting points
- Expanding a root shows its hierarchical children

#### Query System
- Query detail shows SQL code and Run button
- Running `SELECT 1` returns result table with column "1" and value "1"

#### Bulk Operations
- Selecting notes reveals bulk action bar: Deselect All, Select All, Add Tag, Remove Tag, + Add Field, Add Groups, Delete

#### Error Handling
- Non-existent note (id=99999): returns 404 with "record not found" error page
- Invalid version hash on resource view: 302 redirect to actual resource (caching mechanism)
