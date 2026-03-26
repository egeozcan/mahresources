# Bug Report - Cycle 2, Round 2

**Date**: 2026-03-26
**Tester**: Claude (automated QA)
**App URL**: http://localhost:8181

## Test Areas
1. Inline editing
2. Expandable text component
3. Keyboard navigation
4. Multi-sort
5. Browser back/forward
6. Responsive layout
7. Error recovery
8. File download

---

## Bugs Found

### BUG-C2-01: Expandable text component exposes full text to assistive technology even when collapsed
- **Severity**: minor
- **URL**: Any page with `<expandable-text>` in meta data tables (e.g., `/group?id=1` with long meta values)
- **Steps**:
  1. Add a meta data value longer than 30 characters to a group
  2. Navigate to that group's detail page
  3. Inspect the expandable-text component in the accessibility tree
- **Expected**: When collapsed, only the preview text (first 30 chars) should be exposed to assistive technology
- **Actual**: The full text is exposed as the light DOM `textContent` of the `<expandable-text>` element alongside the shadow DOM preview. Unlike `<inline-edit>` (which uses a hidden `<slot>` to suppress light DOM from the a11y tree), `<expandable-text>` in `src/webcomponents/expandabletext.js` has no such mechanism. Screen readers may announce the full text twice -- once from the light DOM and once from the shadow DOM preview.

### BUG-C2-02: Inline description edit disappears when description is cleared to empty
- **Severity**: major
- **URL**: Any entity detail page (e.g., `/group?id=1`, `/note?id=1`)
- **Steps**:
  1. Navigate to a group or note detail page that has a description
  2. Double-click the description to enter edit mode
  3. Clear the textarea completely (empty string)
  4. Click away to save -- page reloads
  5. Try to double-click to re-add a description
- **Expected**: An empty/placeholder description area should remain, allowing the user to double-click and re-add a description
- **Actual**: The description element is conditionally rendered with `{% if description %}` in `templates/partials/description.tpl` (line 1). When description is an empty string, the entire description div (including the double-click-to-edit functionality) is not rendered. The user must navigate to the full "Edit" form to re-add a description. This creates a one-way trap: inline edit lets you clear it but you cannot restore it inline.

### BUG-C2-03: Description inline edit silently fails on server error with no user feedback
- **Severity**: minor
- **URL**: Any entity detail page with description editing
- **Steps**:
  1. Double-click description to enter edit mode
  2. Modify the text
  3. Click away to trigger save
  4. If the server returns a non-2xx status (e.g., 500 error)
- **Expected**: User should see an error notification, or the textarea should stay open indicating the save failed
- **Actual**: In `templates/partials/description.tpl` (lines 19-26), the `click.away` handler does:
  ```js
  fetch(url, {...}).then(r => { if (r.ok && !clickedLink) location.reload(); })
      .catch(e => console.error('Failed to save description:', e));
  ```
  On HTTP errors, `r.ok` is false so the page doesn't reload, but the `.catch()` only handles network errors (fetch rejections), not HTTP errors. The edit mode closes (Alpine sets `editing=false`), the old description text re-appears, and the user has zero feedback that their edit was lost. No toast, no error message, no visual indicator.

### BUG-C2-04: Group tree view shows "1 children" instead of "1 child" (grammar)
- **Severity**: cosmetic
- **URL**: `/group/tree`
- **Steps**:
  1. Navigate to `/group/tree`
  2. Look at a group that has exactly 1 child group
- **Expected**: Should display "1 child"
- **Actual**: Displays "1 children" because `templates/displayGroupTree.tpl` line 246 uses `{{ root.ChildCount }} children` without any pluralization logic. The fix would be to use a conditional: `{{ root.ChildCount }} {% if root.ChildCount == 1 %}child{% else %}children{% endif %}`.

---

## Tested Areas - No Bugs Found

### 1. Inline Editing
- **Escape to cancel (name)**: Clicking the pencil icon opens a textbox; pressing Escape reverts to display mode correctly
- **Escape to cancel (description)**: Double-clicking description opens textarea; pressing Escape cancels without saving
- **Empty name submission**: Server returns 400 ("group name is required"), UI reverts with brief red flash
- **Very long name (600 chars)**: Accepted and saved; heading truncates with CSS `text-overflow: ellipsis`; breadcrumb also truncates
- **XSS via `<script>` tags in name**: Properly escaped in heading, breadcrumb, and page title. No script execution
- **Unicode characters**: Handled correctly throughout
- **Success indicator**: Green flash on successful save (via `displayText.style.backgroundColor = '#d1fae5'`)
- **Page title update**: Document title updates correctly after inline name edit

### 2. Expandable Text Component
- **Read more / Read less toggle**: Correctly shows first 30 chars, "Read more" expands to full text, "Read less" collapses
- **Copy button**: Present alongside Read more button, functional

### 3. Keyboard Navigation
- **Tab order through forms**: Logical sequence -- nav links, then buttons (Admin, Plugins, Search, Settings), then form fields
- **Admin dropdown**: Tab focuses button, Enter opens dropdown (`aria-expanded: true`), Escape closes and returns focus to button
- **Search dialog**: Opens on button click, Escape closes correctly
- **Search results**: Typing returns results across all entity types (groups, resources, notes, tags, categories, queries, relation types)

### 4. Multi-Sort
- **Adding sort columns**: Works correctly; URL contains multiple `SortBy` params (e.g., `SortBy=name+desc&SortBy=created_at+desc`)
- **Sort direction toggle**: Switches between ascending/descending with updated aria-label
- **Removing sort**: Correctly removes selected column, preserves remaining
- **Reordering sorts**: Move up/down buttons swap sort columns correctly
- **Custom Property sort**: Shows "meta key" input when selected
- **State preserved in URL**: All sort settings roundtrip through URL parameters

### 5. Browser Back/Forward
- **Basic navigation**: Back/forward correctly traverses page history (Groups -> Notes -> Resources -> back -> back -> forward)
- **Filter state preservation**: Name filter values and sort columns are preserved on back navigation

### 6. Responsive Layout
- **Groups list at 375px**: Filter sidebar stacks above content, all elements readable
- **Group detail at 375px**: Heading, Edit/Delete buttons, sidebar, content stack properly
- **Resource detail at 375px**: Download button, tags, metadata, version info all visible
- **Dashboard at 375px**: Resource/note/group cards display correctly in single column
- **Mobile hamburger menu**: Opens correctly, shows all nav links organized into Main, Admin, and Plugins sections

### 7. Error Recovery
- **Empty required field**: HTML5 `required` attribute prevents form submission with native browser validation
- **Valid form submission**: Creates entity and redirects to detail page
- **API error messages**: Clean, user-friendly messages ("group name is required", "invalid JSON in Meta field")
- **Delete confirmation**: Shows browser confirm dialog ("Are you sure you want to delete?"); cancelling preserves entity

### 8. File Download
- **Download endpoint**: Returns correct file content with `Content-Disposition: attachment` header
- **View endpoint**: Redirects (302) to file location with correct Content-Type
- **Download filename**: Uses `v{N}_{hash_prefix}` format (by design, not the original filename)

### Other Areas Tested
- **Pagination edge cases**: page=0, page=-1, page=999999 all handled gracefully
- **Clone group**: Properly clones with confirm dialog and redirects to new entity
- **Share note**: Creates share URL, shows "Shared" badge and "Unshare" link
- **Edit Blocks (notes)**: Toggle works, shows "+ Add Block" interface
- **Timeline view**: Renders bar chart with Created/Updated toggle, Year/Month/Week views
- **Tree view**: Shows group hierarchy with child counts and expand/collapse
- **Text view**: Clean list layout for groups and notes
- **Admin Overview**: Shows server health, DB stats, and configuration
- **Logs page**: Renders log table with Time, Level, Action, Entity, Message columns
- **Jobs panel**: Opens side panel showing "No jobs in queue"
- **Settings dropdown**: Opens with "Show Descriptions" toggle
- **SQL injection in filters**: Parameterized queries prevent injection
- **Non-existent entity (404)**: Shows error message "record not found" with proper 404 status
- **Bulk select all**: Selects all groups on page, shows bulk action toolbar
