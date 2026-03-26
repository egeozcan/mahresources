# Bug Report - Cycle 2, Session 1

**Date**: 2026-03-26
**Tester**: Claude (automated)
**App URL**: http://localhost:8181

---

## Bugs Found

### BUG-C2-01: Query run API returns raw internal error with form-encoded or no Content-Type
- **Severity**: major
- **URL**: POST `/v1/query/run?id=1`
- **Steps**:
  1. Create a query via API (e.g., `SELECT id, name FROM notes`)
  2. Run the query via POST `/v1/query/run?id=1` with `Content-Type: application/x-www-form-urlencoded` or no Content-Type header at all
- **Expected**: The query runs successfully or returns a user-friendly error indicating JSON Content-Type is required
- **Actual**: Returns `{"error":"schema: interface must be a pointer to struct"}` -- a raw internal Go/gorilla schema error message exposed to the user
- **Root cause**: In `server/api_handlers/query_api_handlers.go` line 86, `tryFillStructValuesFromRequest(&values, request)` is called where `values` is `query_models.QueryParameters` (which is `map[string]any`). The gorilla/schema decoder path (used for form-encoded and no-content-type requests) requires a struct pointer, not a map pointer. Only the JSON decoder path handles `map[string]any` correctly.
- **Note**: The UI works because it sends `Content-Type: application/json`. This bug affects API clients using form-encoded data or curl without explicit Content-Type.

---

## Test Results Summary

### 1. Group Relations
**Status: PASS** -- No bugs found.
- Created 2 categories (TestCatA, TestCatB)
- Created a relation type (TestRelation) with reverse relation (ReverseTestRelation)
- Created 2 groups (GroupInCatA in TestCatA, GroupInCatB in TestCatB)
- Created a relation from GroupInCatA to GroupInCatB
- Verified both forward and back relations display correctly on both group detail pages
- Verified relation detail page shows correct From/To groups with category links
- Verified relation type detail page shows category flow and reverse relation link

### 2. Resource Upload & Management
**Status: PASS** -- No bugs found.
- Uploaded a text file (42 B) and a PNG image (200x150, 533 B)
- Verified dimensions are correctly detected for the image
- Verified file previews work (both thumbnail and full view)
- Verified resource detail pages display correctly (name, original name, size, dates, metadata)
- Verified download links work (302 redirect to file, correct content type)
- Verified resources list in Thumbnails, Details, and Simple views all work correctly

### 3. Note-Resource Associations
**Status: PASS** -- No bugs found.
- Created a note (TestNote1) and linked resource (TestImageFile) via edit API
- Verified the note detail page shows the linked resource with preview, name, and size
- Verified the resource detail page shows the linked note with name and description
- Both directions of the association display correctly

### 4. Group Hierarchy
**Status: PASS** -- No bugs found.
- Created parent group (ParentGroup) and child group (ChildGroup) with ownerId set
- Verified parent group shows "Sub-Groups" section with child link
- Verified child group shows breadcrumb (Groups > ParentGroup > ChildGroup)
- Verified child group shows "Owner: ParentGroup" in sidebar
- Verified group tree view renders correctly with expandable nodes

### 5. Merge Operations
**Status: PASS** -- No bugs found.
- Created two groups with different tags (MergeTagA on group 2, MergeTagB on group 4)
- Merged group 4 (ParentGroup) into group 2 (GroupInCatA)
- Verified winner (GroupInCatA) inherited: tags, sub-groups (ChildGroup), and meta data
- Verified loser (ParentGroup) was deleted (returns 404)

### 6. Timeline Views
**Status: PASS** -- No bugs found.
- Tested `/notes/timeline`, `/groups/timeline`, `/tags/timeline`, `/resources/timeline`
- All show bar charts with monthly granularity, correct counts, and interactive buttons
- Time range navigation (Prev/Next), granularity switching (Year/Month/Week) buttons are present
- Activity type switching (Created/Updated) works

### 7. Admin Pages
**Status: PASS** -- No bugs found.
- `/admin/overview` shows comprehensive server health, configuration, data stats, storage breakdown, top tags/categories, orphaned resources, similarity detection, and log statistics
- `/logs` page works with Level/Action/Entity Type/Entity ID/Message filters
- `/relationTypes`, `/relations`, `/noteTypes`, `/resourceCategories` all return 200
- Admin dropdown in nav shows all expected links

### 8. Query Execution
**Status: PASS (UI) / FAIL (API)** -- See BUG-C2-01.
- Created a query with SQL `SELECT id, name FROM notes`
- Running via UI ("Run" button) works correctly, renders results in a table
- Results show all 4 notes with correct IDs and names
- Running via API with JSON Content-Type works correctly
- Running via API with form-encoded or no Content-Type fails (BUG-C2-01)

### Additional Testing
- **404 handling**: All entity types correctly return 404 for non-existent IDs
- **Invalid ID handling**: Returns 400 for invalid IDs (abc, -1), 404 for zero/empty
- **Inline editing**: Edit name button and double-click description editing work correctly
- **Note sharing**: Share/Unshare feature works, generates share URL with token
- **XSS protection**: Script tags in entity names are properly escaped (displayed as text)
- **Dashboard**: Shows recent resources/notes/groups/tags and activity log correctly with relative timestamps
- **Console errors**: No console errors on dashboard page
- **Global search**: Cmd+K dialog works, search returns relevant results with proper categorization
- **Group clone**: Clone via API works correctly, preserving associations and metadata
- **Group tree view**: Tree renders correctly with expandable nodes and correct hierarchy
- **Note wide display**: `/note/text?id=N` shows clean text view with back link
- **Resource views**: Thumbnails, Details, Simple, and Timeline views all render correctly
- **Groups views**: List, Text, Tree, and Timeline views all render correctly
- **Plugins page**: Shows available plugins with settings and enable/disable buttons
- **Category deletion cascade**: Deleting a category correctly nullifies group categoryId references, removes relation types tied to deleted categories, and cascades cleanup of orphaned relations
- **Empty name validation**: Creating a note with empty name returns friendly "note name needed" error
- **Duplicate resource detection**: Uploading same file again returns informative error with existing resource ID
- **SQL injection in queries**: SQL injection-like text in query text is stored safely; queries run through read-only connections
