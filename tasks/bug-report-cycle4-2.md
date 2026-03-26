# Bug Report - Cycle 4.2 Deep Testing

**Date**: 2026-03-26
**Tester**: Claude QA
**App URL**: http://localhost:8181

## Test Data Created
- 2 categories (IDs 4, 5)
- 10 tags (IDs 1-10)
- 4 groups (2 parent, 2 child)
- 40 notes (5 FTS-specific + 35 pagination)
- 35 resources (text files)

---

## Test Results

### 1. Full-Text Search (FTS)
**Status**: PASS (with minor issues noted as BUG-1, BUG-2)

- Exact phrase matching: PASS -- `q=quick brown fox` correctly finds only the matching note
- Partial word / prefix matching: PASS -- `q=Prog*` matches "Programming" in description
- Multi-word queries: PASS -- `q=elephant magnificent` finds the correct note
- Unicode search: PASS -- searching for `uber cafe` finds the unicode test note
- Search highlighting: PASS -- global search UI wraps matched terms in `<mark>` tags
- Search result ranking: PASS -- results are scored and sorted by relevance
- Type filtering: PASS -- `types=note` correctly restricts results to notes
- Empty/whitespace queries: PASS -- returns empty results, no errors
- SQL injection: PASS -- `q='OR 1=1--` returns 0 results, no error
- Very long query: PASS -- 500-char query works without error
- Limit parameter: FAIL -- see BUG-1 (cliff from 50 to 20 results)
- Fuzzy search: PARTIAL -- works for name column only, see BUG-2

### 2. Template Rendering Edge Cases
**Status**: PASS

- Note detail with tags, sidebar, edit buttons: renders correctly
- Group detail with children (sub-groups), breadcrumbs: renders correctly
- Resource detail with metadata, preview, versions: renders correctly
- Category detail with owned groups: renders correctly
- Tag detail with associated notes count: renders correctly
- Inline name edit (click pencil button): works, updates page title
- Inline description edit (double-click): opens textarea, Escape cancels properly
- `.json` suffix on template routes: returns filtered template context (no internal data leaked)
- XSS test strings: properly escaped in all views (rendered as text, not executed)
- Very long entity names: rendered (may overflow visually but no errors)

### 3. Pagination Boundary Testing
**Status**: PASS

- `pageSize=5`: shows 5 items per page, correct pagination links (9 pages for 43 notes)
- `pageSize=1`: 1 item per page, correct page count
- `pageSize=200`: capped at 200, works correctly
- `pageSize=0`: falls back to default (50)
- `pageSize=-5`: falls back to default (50)
- `page=0` / `page=-1`: treated as page 1 (no error)
- `page=99999`: returns empty result set with 200 OK
- Last page content: correct remainder items
- Pagination headers (X-Page, X-Per-Page): correct
- API `MaxResults` parameter: properly validated (negative values return 400)

### 4. Alternative File Systems
**Status**: N/A

No `/v1/resource/altFileSystems` API endpoint exists. Alt file systems are configured via server flags and displayed in the admin overview. Admin overview shows "Alt Filesystems: some_key" correctly.

### 5. Log Filtering
**Status**: PASS

- Level filter (Error): shows no results (correct, no errors logged)
- Entity Type filter (Resource): shows only resource-related logs
- Combined filters (Action=create, EntityType=resource): correctly intersected
- Filter state preserved in dropdowns after applying
- URL params preserved after filter application
- Log detail page: renders correctly with all fields (level, action, entity link, message)

### 6. Note Text View
**Status**: PASS

- `/note/text?id=1`: renders title, tags sidebar, description as paragraph, "Go back to note" link
- Note without description: shows empty main area with "Go back to note" link, no errors
- Inline edit functionality available from text view

### 7. Group Text View
**Status**: PASS

- `/groups/text`: shows simplified list with names and descriptions only
- Groups without descriptions show just headings
- Groups with descriptions show editable paragraphs with double-click
- Category links preserved in text view
- Group tree view: works correctly, shows child counts, expand/collapse works

### 8. Dashboard Data Accuracy
**Status**: PASS

- Recent Resources: shows newest resources first (correct ordering)
- Recent Notes: shows newest notes first
- Recent Groups: shows newest groups first
- Recent Tags: shows tags
- Recent Activity: shows latest log entries with entity links
- Entity counts in admin overview match actual data (verified via SQL queries)
- Dashboard loads successfully even when notes/groups list pages are broken (because dashboard queries have different result sets)

---

## Bugs Found

### BUG-1: Search limit silently drops from requested value to 20 when limit > 50

**Severity**: Medium
**Location**: `application_context/search_context.go` line 69, `server/api_handlers/search_api_handlers.go` line 17

**Steps to reproduce**:
1. `GET /v1/search?q=Test&limit=50` -- returns 50 results as expected
2. `GET /v1/search?q=Test&limit=51` -- returns only 20 results (silently reset)
3. `GET /v1/search?q=Test&limit=200` -- returns only 20 results

**Root cause**: The HTTP handler caps limit to 200 (`min(limit, 200)`), but `GlobalSearch()` has a stricter check at line 69: `if query.Limit <= 0 || query.Limit > 50 { query.Limit = 20 }`. Any limit above 50 is silently reset to 20, not capped to 50. The handler's 200 cap is misleading because the business logic will never honor anything above 50.

**Expected behavior**: Either (a) cap to 50 instead of resetting to 20, or (b) the handler should cap to 50 instead of 200 so the API surface is consistent. The sudden cliff from 50->20 when requesting 51 is confusing.

### BUG-2: Fuzzy search (~) only matches against name column, not description

**Severity**: Low
**Location**: `fts/sqlite.go` lines 173-201 (`fuzzyFallback` function)

**Steps to reproduce**:
1. Create a note with description "Elephants are magnificent"
2. `GET /v1/search?q=~elphants` -- returns 0 results (should match "Elephants" in description)
3. `GET /v1/search?q=~TestResourc` -- returns 35 results (matches TestResource* in names)

**Root cause**: The `fuzzyFallback` function only generates LIKE conditions against `tableName+".name"`, ignoring the `description` column. Regular (non-fuzzy) FTS searches check both name and description via the FTS5 index.

**Expected behavior**: Fuzzy search should also check the description column (and original_name for resources), matching the behavior of non-fuzzy FTS search.

### BUG-3: Non-object meta values cause list endpoints to fail with 404

**Severity**: High
**Location**: Multiple list handlers, GORM JSONB scanner

**Steps to reproduce**:
1. Have a note or group in the database with a non-object meta value (e.g., `meta=42`, `meta=[1,2,3]`, `meta="hello"`, `meta=true`)
2. `GET /v1/notes` or `GET /v1/groups` with `Accept: application/json`
3. Or navigate to `/notes` or `/groups` in the browser

**Observed behavior**:
- API response: `{"error":"sql: Scan error on column index 5, name \"meta\": Failed to unmarshal JSONB value:42"}`
- HTTP status: **404 Not Found** (incorrect for a deserialization error)
- The entire list endpoint is broken -- no notes/groups can be listed at all
- HTML pages show a red error banner with the raw SQL scan error, completely blank otherwise

**Affected data (from previous test cycles)**:
- Note ID 48 (`num-meta-note`): meta=42 (integer)
- Note ID 45 (`array-meta-note`): meta=[1,2,3] (array)
- Note ID 50 (`str-meta-note`): meta="hello" (string)
- Group ID 7 (`num-meta-group`): meta=42 (integer)
- Group ID 8 (`bool-meta-group`): meta=true (boolean)

**Root cause**: GORM's JSONB scanner (via `datatypes.JSONMap` which maps to `map[string]interface{}`) expects a JSON object but the database contains non-object JSON primitives. When scanning any row with these values, the entire query fails. The error handler in the list endpoint returns 404 instead of 500.

**Impact**:
- **Complete list page outage**: Both `/notes` and `/groups` pages (HTML and API) are entirely broken
- **Error message leak**: Raw SQL scan error with column index and internal JSONB details shown to user
- **Cascading failure**: One bad record prevents all records from being listed
- **Wrong status code**: 404 instead of 500 for an internal data corruption issue

**Expected behavior**:
1. The API status code should be 500 (Internal Server Error) for scan errors, not 404
2. Meta validation should reject non-object JSON values at write time (the Meta field is defined as `datatypes.JSONMap` which is `map[string]interface{}`, so only objects should be allowed)
3. The error message should be sanitized to not expose column indices and internal scan details
4. Ideally, one bad record should not break the entire list endpoint -- consider wrapping the meta value in a more tolerant scanner or skipping malformed rows

---

## Summary

| # | Bug | Severity | Status |
|---|-----|----------|--------|
| 1 | Search limit cliff (50->20 at limit=51) | Medium | New |
| 2 | Fuzzy search only checks name, not description | Low | New |
| 3 | Non-object meta values break entire list endpoints | High | New |

**Total bugs found**: 3 (1 high, 1 medium, 1 low)

**Areas tested without issues**:
- Full-text search (exact, prefix, multi-word, unicode, highlighting)
- Template rendering for all entity types
- Pagination (boundary values, negative inputs, large page numbers)
- Log filtering (level, action, entity type, combined filters)
- Note text view and group text view
- Dashboard data accuracy and ordering
- Group tree view
- Timeline views (notes, resources)
- Inline editing (name and description)
- SQL injection prevention (search and saved queries)
- XSS prevention (script tags rendered as text)
- Read-only query execution (write queries blocked)
