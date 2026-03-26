# Bug Report - QA Session 1

**Date**: 2026-03-26
**Tester**: Claude Code (Automated)
**Application**: Mahresources at http://localhost:8181

---

## 1. Entity Creation Tests

All entity creation redirects work correctly:

| Entity | Create URL | Redirect After Save | Status |
|--------|-----------|---------------------|--------|
| Tag | /tag/new | /tag?id=4 | OK |
| Category | /category/new | /category?id=5 | OK |
| Note Type | /noteType/new | /noteType?id=3 | OK |
| Note | /note/new | /note?id=5 | OK |
| Group | /group/new | /group?id=5 | OK |
| Query | /query/new | /query?id=3 | OK |
| Relation Type | /relationType/new | /relationType?id=4 | OK (after selecting required categories) |

No bugs found in entity creation flow.

---

## 2. Entity Edit Tests

All entity edit redirects work correctly:

| Entity | Edit URL | Redirect After Save | Status |
|--------|---------|---------------------|--------|
| Tag | /tag/edit?id=4 | /tag?id=4 | OK |
| Category | /category/edit?id=5 | /category?id=5 | OK |
| Note Type | /noteType/edit?id=3 | /noteType?id=3 | OK |
| Note | /note/edit?id=5 | /note?id=5 | OK |
| Group | /group/edit?id=5 | /group?id=5 | OK |
| Query | /query/edit?id=3 | /query?id=3 | OK |

Inline edit (via "Edit name" pencil icon) also works correctly for notes.

No bugs found in entity edit flow.

---

## 3. Entity Delete Tests

| Entity | Delete From | Redirect After Delete | Status |
|--------|-----------|----------------------|--------|
| Tag | /tag?id=6 | /tags | OK |
| Category | /category?id=6 | /categories | OK |

Delete flows work correctly with confirmation dialog ("Are you sure you want to delete?").

---

## 4. Pagination Edge Cases

| URL | Result | Status |
|-----|--------|--------|
| /notes?page=-1 | Shows page 1 content (clamped) | OK |
| /notes?page=0 | Shows page 1 content (clamped) | OK |
| /notes?page=999999999999 | Shows empty page, no error | OK |
| /v1/notes?page=99999999999999999999 | Returns 200 with empty array | OK |

Pagination handles edge cases gracefully.

---

## 5. Long Text Handling

- **2000-character note name**: Created successfully (note id=10)
- **Detail page**: The full 2000-char title displays in the h1, filling the viewport. Edit and Delete buttons remain accessible.
- **List page**: Title is truncated with "..." ellipsis within the card. Display is acceptable.
- **Page title**: Contains the full 2000-char name in the HTML `<title>` tag. Very long but not technically broken.

No critical bugs found, though a max-length or truncation in the h1 on detail pages could improve UX.

---

## 6. Empty Form Submissions

All empty form submissions are properly handled:

| Entity | Client-side Validation | Server-side Validation (API) |
|--------|----------------------|----------------------------|
| Tag | HTML5 "required" attribute | 400: "tag name must be non-empty" |
| Category | HTML5 "required" attribute | 400: "category name must be non-empty" |
| Note Type | HTML5 "required" attribute | 400: "note type name must be non-empty" |
| Note | HTML5 "required" attribute | 400: "note name needed" |
| Group | HTML5 "required" attribute | 400: "group name is required" |
| Query | HTML5 "required" attribute | 400: "query name must be non-empty" |
| Relation Type | Client-side shows "Please select at least 1 value" for categories | Properly rejected |

---

## Bugs Found

### BUG-1-01: Filter input fields don't preserve values from lowercase URL parameters
- **Severity**: minor
- **URL**: /notes?name=QA, /tags?name=QA, /groups?name=QA (any list page with lowercase param)
- **Steps**:
  1. Navigate to any entity list page with a lowercase filter parameter, e.g., `/tags?name=QA`
  2. Observe the filtered results (filtering works correctly)
  3. Look at the Name filter input in the sidebar
- **Expected**: The Name filter input should show "QA" to indicate the active filter
- **Actual**: The Name filter input is empty, even though filtering is active. This is because the template looks up `queryValues.Name.0` (uppercase key) but the URL parameter `name` is lowercase. When using the form's "Apply Filters" button, it submits as `?Name=QA` (uppercase) which works correctly.
- **Note**: The filter form itself generates uppercase parameters correctly. This only affects direct URL navigation with lowercase params. Go's gorilla/schema decoder handles both cases for filtering, but the template value display is case-sensitive.

### BUG-1-02: Technical error message exposed for invalid entity ID format
- **Severity**: cosmetic
- **URL**: /note?id=abc, /tag?id=abc, /group?id=abc (any entity detail page with non-numeric ID)
- **Steps**:
  1. Navigate to `/note?id=abc`
  2. Observe the error message
- **Expected**: A user-friendly error message like "Invalid ID" or "Note not found"
- **Actual**: Shows `schema: error converting value for "id"` - a technical/internal error message from the Go schema decoder
- **Note**: Returns HTTP 400 status code which is correct. The issue is only the user-facing error text.

### BUG-1-03: Duplicate tag creation returns raw database constraint error
- **Severity**: minor
- **URL**: POST /v1/tag (API)
- **Steps**:
  1. Create a tag with name "MyTag"
  2. Try to create another tag with the same name "MyTag"
- **Expected**: A user-friendly error like "A tag with this name already exists"
- **Actual**: Returns 400 with `{"error":"UNIQUE constraint failed: tags.name"}` - a raw SQLite constraint error message
- **Note**: The uniqueness constraint itself is correct behavior; only the error message is unfriendly.

---

## Additional Observations (Not Bugs)

1. **Dashboard root URL**: `/` correctly redirects to `/dashboard`
2. **Non-existent entity pages**: Return 404 with "record not found" - correct behavior
3. **Global search (Cmd+K)**: Works well across all entity types, instant results
4. **Timeline views**: Render correctly with bar charts
5. **Admin overview**: Shows server health and configuration
6. **Logs page**: Shows comprehensive audit trail of all actions
7. **Bulk operations**: Select All/Deselect All with Add Tag, Remove Tag, Delete options work correctly
8. **Query execution**: "Run" button executes SQL and displays results in a table
9. **Resource detail page**: Shows file preview, metadata, related notes/groups/tags correctly
10. **Inline edit**: Name editing via pencil icon with Enter to confirm works properly
11. **Relation Type creation**: Properly validates required From/To Category fields with user-friendly "Please select at least 1 value" messages
12. **Double-submit protection**: Navigation after first submit prevents double creation
