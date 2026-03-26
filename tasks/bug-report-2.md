# Bug Report 2 - QA Bug Hunt

**Date**: 2026-03-26
**Target**: http://localhost:8181
**Tester**: Claude Code QA

---

## 1. JSON API Responses

### BUG-2-01: Template .json error responses leak internal context data
- **Severity**: minor
- **URL**: /note.json?id=999999 (any template .json route with nonexistent entity)
- **Steps**:
  1. Request `/note.json?id=999999`
  2. Observe the response body
- **Expected**: A clean JSON error response like `{"error": "record not found"}` (matching the `/v1/note?id=999999` API behavior)
- **Actual**: Returns 404 with a JSON object containing internal template context: `adminMenu` (full menu structure with 7 items), `assetVersion`, `hasPluginManager`, `menu`, `queryValues`, `url`, and `errorMessage`. This leaks internal application structure. The API route (`/v1/note?id=999999` with Accept header) correctly returns `{"error":"record not found"}`.

### Observation: API error handling is consistent and correct
- POST `/v1/note` with empty body returns `400 {"error":"note name needed"}`
- POST `/v1/group` with empty body returns `400 {"error":"group name is required"}`
- POST `/v1/tag` with empty body returns `400 {"error":"tag name must be non-empty"}`
- GET `/v1/note?id=999999` returns `404 {"error":"record not found"}`
- All list endpoints (`/v1/notes`, `/v1/tags`, `/v1/groups`, `/v1/resources`, `/v1/categories`) return JSON arrays with 200 status

---

## 2. Meta Field Handling

### BUG-2-02: addMeta returns 200 OK when no IDs are provided
- **Severity**: major
- **URL**: POST /v1/notes/addMeta, /v1/groups/addMeta, /v1/resources/addMeta
- **Steps**:
  1. POST to `/v1/notes/addMeta` with body `Meta={"key":"val"}` but NO `ID` field
  2. Observe response
- **Expected**: 400 error indicating that at least one ID is required
- **Actual**: Returns `200 {"ok":true}` silently succeeding with no effect. Reproduces on all three entity types (notes, groups, resources). This is misleading - the client believes the operation succeeded when nothing was actually modified.

### BUG-2-03: addMeta returns 200 OK for nonexistent entity IDs
- **Severity**: major
- **URL**: POST /v1/notes/addMeta (and likely groups/resources too)
- **Steps**:
  1. POST to `/v1/notes/addMeta` with `ID=999999&Meta={"key":"val"}`
  2. Observe response
- **Expected**: 404 error or at minimum an indication that no records were updated
- **Actual**: Returns `200 {"ok":true}` even though note ID 999999 does not exist. The client believes the operation succeeded.

### Observation: Meta validation works correctly
- Invalid JSON meta (`{broken`) correctly returns `400 {"error":"invalid json"}`
- Empty meta field correctly returns `400 {"error":"invalid json"}`
- Valid JSON meta via API correctly adds to the note and shows in the edit form

---

## 3. Bulk Operations

### BUG-2-04: Bulk operations return 200 OK when no IDs are provided
- **Severity**: major
- **URL**: All bulk endpoints across all entity types
- **Steps**:
  1. POST to any bulk endpoint (addTags, removeTags, addMeta, delete) with no `ID` field
  2. Observe response
- **Expected**: 400 error indicating that at least one ID is required
- **Actual**: Returns `200 {"ok":true}` on ALL of the following endpoints:
  - `POST /v1/notes/addTags` (no IDs) -> 200 OK
  - `POST /v1/notes/removeTags` (no IDs) -> 200 OK
  - `POST /v1/notes/addMeta` (no IDs) -> 200 OK
  - `POST /v1/notes/delete` (no IDs) -> 200 OK
  - `POST /v1/groups/addTags` (no IDs) -> 200 OK
  - `POST /v1/groups/delete` (no IDs) -> 200 OK
  - `POST /v1/resources/addTags` (no IDs) -> 200 OK
  - `POST /v1/resources/delete` (no IDs) -> 200 OK

### BUG-2-05: Bulk addTags/removeTags return 200 OK when no TagID is provided
- **Severity**: major
- **URL**: POST /v1/notes/addTags, /v1/notes/removeTags (and groups/resources equivalents)
- **Steps**:
  1. POST to `/v1/notes/addTags` with `ID=7` but no `TagID` field
  2. Observe response
- **Expected**: 400 error indicating that a TagID is required
- **Actual**: Returns `200 {"ok":true}`. Same behavior for removeTags. The operation silently does nothing.

### BUG-2-06: Bulk addTags returns 200 OK for nonexistent TagID
- **Severity**: minor
- **URL**: POST /v1/notes/addTags
- **Steps**:
  1. POST to `/v1/notes/addTags` with `ID=7&TagID=999999`
  2. Observe response
- **Expected**: 404 error or indication that the tag doesn't exist
- **Actual**: Returns `200 {"ok":true}` even though tag ID 999999 doesn't exist.

### Observation: Bulk delete with nonexistent IDs correctly returns 404
- `POST /v1/notes/delete` with `ID=999999` correctly returns `404 {"error":"record not found"}`
- This is inconsistent with the other bulk operations (addTags, addMeta) which silently succeed for nonexistent IDs.

---

## 4. Search

### Observation: Search handles all edge cases safely
- Global search (Cmd+K) works correctly for existing entities, showing results with type labels and highlighted matches
- Nonexistent terms show a clean "No results found" message with a helpful suggestion
- XSS payloads (`<script>alert(1)</script>`) are properly escaped - displayed as text, not executed
- SQL injection attempts (`' OR 1=1 --`) are safely handled, returning no results
- Null bytes (`%00`) are handled without errors
- Path traversal (`../../etc/passwd`) causes no issues
- Very long queries (10,000 chars) are processed without error (though the full query is echoed back in the response, which makes the response unnecessarily large)
- Empty and whitespace-only queries return 0 results cleanly
- Search API uses `q` parameter (not `query`)

---

## 5. Sort Functionality

### Observation: Sort works correctly via URL parameters
- Sort uses `SortBy` parameter with format `column direction` (e.g., `SortBy=name+asc`)
- The sort dropdown correctly initializes from URL parameters when using the proper `SortBy` param
- Sort controls (column selector, direction toggle) are part of the sidebar filter form and require form submission to apply
- SQL injection attempts via sort parameter do not cause server errors or affect data integrity (GORM parameterizes queries)
- Invalid sort values are silently ignored and default ordering is used

---

## Summary

| Bug ID | Severity | Category | Description |
|--------|----------|----------|-------------|
| BUG-2-01 | minor | JSON API | Template .json error responses leak internal context data |
| BUG-2-02 | major | Meta/API | addMeta returns 200 OK when no IDs provided (all entity types) |
| BUG-2-03 | major | Meta/API | addMeta returns 200 OK for nonexistent entity IDs |
| BUG-2-04 | major | Bulk Ops | All bulk operations return 200 OK when no IDs provided |
| BUG-2-05 | major | Bulk Ops | addTags/removeTags return 200 OK when no TagID provided |
| BUG-2-06 | minor | Bulk Ops | addTags returns 200 OK for nonexistent TagID |

**Total bugs found: 6** (4 major, 2 minor)

**Common root cause for BUG-2-02 through BUG-2-06**: Bulk operation endpoints lack input validation for required parameters. They execute the operation on an empty set of IDs or with missing required fields and report success, when they should return a 400 error. This is a systemic pattern across all entity types (notes, groups, resources) and all bulk operation types (addTags, removeTags, addMeta, delete).
