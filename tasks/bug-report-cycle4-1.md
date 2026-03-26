# Bug Report - Cycle 4.1: Deep Testing

**Date**: 2026-03-26
**Tester**: Claude QA
**App URL**: http://localhost:8181

## Test Plan Coverage
1. HTTP method validation
2. Content-Type edge cases
3. Concurrent entity creation
4. Resource version workflow
5. Note blocks ordering
6. Relation type self-referential
7. Empty/whitespace-only names
8. API response consistency

---

## Confirmed Bugs

### BUG-1 [CRITICAL]: Number Meta value causes 500 errors, breaks entity listings, and creates undeletable records

**Severity**: CRITICAL -- one bad record takes down entire sections of the application
**Affected endpoints**: ALL entity list and detail pages (notes, groups, and potentially resources)

**Steps to reproduce**:
1. Create a note with Meta set to a bare number: `POST /v1/note` with `{"Name":"test","Meta":"42"}`
2. The API accepts this and returns 200
3. Try to view the note: `GET /note?id=<ID>` --> **500 Internal Server Error**
4. Try to list all notes: `GET /notes` --> **500 Internal Server Error** (entire page broken)
5. Try to list notes via API: `GET /v1/notes` --> returns the error instead of notes
6. Try to delete the note: `POST /v1/note/delete` with `id=<ID>` --> **500** (cannot delete!)
7. The note is now **undeletable** and **poisons the entire notes listing**
8. Same behavior confirmed with groups: `POST /v1/group` with `{"Name":"test","CategoryId":1,"Meta":"42"}`

**Error message leaked on pages**: `sql: Scan error on column index 5, name "meta": Failed to unmarshal JSONB value:42`

**Root cause**: The `ValidateMeta()` function in `application_context/context.go:34` correctly rejects non-object JSON (checks that the value starts with `{`). However, it is only called in ONE place: `resource_upload_context.go:349`. All other entity creation/update paths use the weaker `json.Valid()` check instead, which accepts any valid JSON including bare numbers.

**Files that need `ValidateMeta()` instead of `json.Valid()`**:
- `note_context.go:28` -- note creation/update
- `note_bulk_context.go:98` -- bulk note meta addition
- `group_crud_context.go:25` -- group creation (first code path)
- `group_crud_context.go:172` -- group creation (second code path)
- `resource_crud_context.go:215` -- resource edit
- `resource_bulk_context.go:223` -- bulk resource meta addition
- `resource_upload_context.go:669` -- resource upload (second path in same file!)
- `group_bulk_context.go:227` -- bulk group meta addition

**What does NOT crash** (but should still be rejected as non-object):
- String Meta (`"hello"`) -- renders OK but semantically wrong
- Boolean Meta (`true`) -- renders OK but semantically wrong
- Array Meta (`[1,2,3]`) -- renders as table but semantically wrong
- Null Meta (`null`) -- renders as empty

**What DOES crash**:
- Number Meta (`42`, `3.14`) -- 500 error, completely breaks the entity and its list page

**Impact**: A single API call can render entire sections of the application unusable (notes, groups). The broken record cannot be deleted through the API or UI, requiring direct database intervention.

---

### BUG-2 [HIGH]: Bulk delete returns 500 for validation errors

**Affected endpoints**:
- `POST /v1/tags/delete`
- `POST /v1/notes/delete`
- `POST /v1/resources/delete`
- `POST /v1/groups/delete`

**Steps to reproduce**:
1. Send a bulk delete request with empty ID array: `{"ID":[]}`
2. Response: `{"error":"at least one tag ID is required"}` with status **500**

**Expected**: Status 400 (Bad Request) since this is a validation error, not a server error.
**Actual**: Status 500 (Internal Server Error).

**Root cause**: The `errorStatusCode()` function in `server/api_handlers/middleware.go:21` only maps "record not found" to 404, and everything else defaults to 500. Validation errors like "at least one X ID is required" are not distinguished from actual server errors.

---

### BUG-3 [MEDIUM]: Query execution leaks internal error messages for write attempts

**Endpoint**: `POST /v1/query/run?id=<ID>`
**Steps**:
1. Create a query with a write operation: `{"Name":"danger","Text":"DROP TABLE tags"}`
2. Run the query: `POST /v1/query/run?id=<ID>`
3. Response: `{"error":"row iteration error: attempt to write a readonly database"}` with status **500**

**Expected**: Sanitized error like `{"error":"write operations are not allowed in queries"}` with status 400 or 403.
**Actual**: Leaks internal SQLite error details and returns 500.

**Note**: The security is correct -- write operations ARE blocked by the read-only DB connection. The issue is only with the error message and status code.

---

### BUG-4 [LOW]: `405 Method Not Allowed` returns empty body

**Endpoints**: Various (e.g., `PUT /v1/resource`, `PUT /v1/group`, `PATCH /v1/category`)
**Steps**: Send an unsupported HTTP method to an endpoint.
**Expected**: Response should include a JSON error body like `{"error":"method not allowed"}`.
**Actual**: Returns `405` with `Content-Length: 0` and no body. This is Gorilla Mux's default behavior but makes API debugging harder for consumers.

---

### BUG-5 [LOW]: Inconsistent X-Total-Count header across list endpoints

**Endpoints**: All `/v1/*` list endpoints
**Observations**:
- `GET /v1/tags` -- includes `X-Total-Count` header
- `GET /v1/categories` -- includes `X-Total-Count` header
- `GET /v1/notes` -- NO `X-Total-Count` header
- `GET /v1/groups` -- NO `X-Total-Count` header
- `GET /v1/resources` -- NO `X-Total-Count` header

API consumers building pagination UI cannot determine total page count for the majority of endpoints.

---

## Verified Working (No Bugs)

### Test 1: HTTP Method Validation
- Gorilla Mux correctly blocks unsupported methods (returns 404 or 405)
- No endpoint silently accepts PUT/PATCH/DELETE when it shouldn't

### Test 2: Content-Type Edge Cases
- JSON body to form-encoded endpoints: works (server handles both content types)
- Malformed JSON: returns 400 with clear error message
- Empty Content-Type: returns 400 with proper validation error

### Test 3: Concurrent Entity Creation
- Created 10 tags concurrently: all 10 created with unique sequential IDs, no duplicates, no errors

### Test 4: Resource Version Workflow
- Full lifecycle tested: Upload -> New Version -> List -> Restore -> Delete middle version
- Restore creates a new version (v4) with content from v1 (correct)
- Delete of current version returns 409 Conflict (correct)
- Restore of non-existent version returns 404 (correct)
- Compare versions works with `v1`/`v2` parameters

### Test 5: Note Blocks Ordering
- Created 5 blocks: correct fractional indexing positions (n, t, w, y, z)
- Reorder via positions map: works correctly, positions update
- Delete middle block: works, remaining block ordering preserved
- Block ownership validation works: "block does not belong to the specified note" on mismatch
- Invalid block type returns 400: "unknown block type: nonexistent"
- Block on non-existent note returns 400: "note 99999 not found"

### Test 6: Self-Referential Relations
- Self-referential relation type (FromCategory == ToCategory): allowed (correct design choice)
- Self-referential group relation (FromGroup == ToGroup): blocked with 400 "cannot relate to self"
- Relation type creation works with both JSON and form-encoded using `FromCategory`/`ToCategory` field names

### Test 7: Empty/Whitespace-Only Names
- All entity types properly validate: tags, notes, groups, categories, relation types, queries
- Spaces-only, tabs, newlines, empty strings all return 400 with clear error messages
- Unicode names (emoji, CJK, Arabic) accepted correctly
- HTML injection properly escaped by Pongo2 (auto-escaping)
- SQL injection prevented by GORM parameterized queries

### Test 8: API Response Consistency
- All list endpoints return raw JSON arrays (consistent format)
- Pagination headers (X-Page, X-Per-Page) present on all list endpoints
- Logs endpoint uses proper paginated response format with `totalCount`, `page`, `perPage`

### Additional Tests Passed
- **Negative/zero/non-numeric IDs**: All return proper 404 or 400 with clear messages
- **Query execution security**: Write operations (DROP/DELETE/UPDATE) correctly blocked by read-only DB connection
- **Global search**: Handles empty queries, very long queries, SQL injection attempts, special characters
- **Server stats and data stats endpoints**: Return proper structured responses

---

## Summary

| Bug | Severity | Impact |
|-----|----------|--------|
| BUG-1: Number Meta breaks listings | CRITICAL | One API call renders entire sections unusable; undeletable records |
| BUG-2: Bulk delete 500 for validation | HIGH | Incorrect status codes confuse API consumers |
| BUG-3: Query run leaks internal errors | MEDIUM | Information disclosure + wrong status code |
| BUG-4: Empty 405 response body | LOW | Poor API developer experience |
| BUG-5: Inconsistent X-Total-Count | LOW | Some list endpoints lack pagination total |

**Priority recommendation**: Fix BUG-1 immediately -- it is a data-corruption-level vulnerability where a single API call can make entire sections of the application inaccessible. The fix is straightforward: replace `json.Valid()` with `ValidateMeta()` in all 8 identified locations.
