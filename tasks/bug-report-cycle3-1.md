# Bug Report - Cycle 3, Session 1

**Date**: 2026-03-26
**Tester**: Claude (QA Bug Hunter)
**App URL**: http://localhost:8181
**Focus**: Race conditions, cross-entity integrity, version/hash endpoints, OpenAPI spec, pagination+filters, large ID values

## Bugs Found

### BUG 1: DELETE block returns 500 for non-existent block (Medium)

**Severity**: Medium
**Type**: Wrong HTTP status code (500 instead of 404)
**Endpoints**: `DELETE /v1/note/block?id=X`, `POST /v1/note/block/delete?id=X`

**Description**: Deleting a block that does not exist returns HTTP 500 (Internal Server Error) instead of 404 (Not Found). A 500 response incorrectly signals a server-side failure when the issue is simply a missing resource.

**Steps to reproduce**:
```
DELETE /v1/note/block?id=99999  -> 500 {"error":"record not found"}
POST /v1/note/block/delete?id=99999  -> 500 {"error":"record not found"}
```

**Expected**: HTTP 404 with `{"error":"record not found"}`

**Root cause**: `server/api_handlers/block_api_handlers.go:181` - `ctx.DeleteBlock(id)` errors are always returned with `http.StatusInternalServerError`, even for "record not found" errors. Should check for not-found errors and return 404.

---

### BUG 2: Resource edit returns 500 for validation errors like invalid Meta JSON (Medium)

**Severity**: Medium
**Type**: Wrong HTTP status code (500 instead of 400)
**Endpoint**: `POST /v1/resource/edit`

**Description**: When editing a resource with invalid JSON in the Meta field, the server returns HTTP 500 (Internal Server Error) instead of 400 (Bad Request). The `errorStatusCode()` function only recognizes "record not found" for 404 and defaults everything else to 500.

**Steps to reproduce**:
```
POST /v1/resource/edit  body: id=1&name=Test&meta=invalid_json
-> 500 {"error":"invalid JSON in Meta field"}
```

**Expected**: HTTP 400 with the same error message.

**Root cause**: `server/api_handlers/middleware.go:21-26` - The `errorStatusCode()` function returns 500 for any error that doesn't contain "record not found". It should check for validation-type errors and return 400.

---

### BUG 3: Raw "FOREIGN KEY constraint failed" error leaks to API clients (Medium)

**Severity**: Medium
**Type**: Information leak / unsanitized DB error
**Endpoint**: `POST /v1/relationType` (and potentially others)

**Description**: Creating a relation type without required `FromCategory`/`ToCategory` parameters exposes the raw SQLite error "FOREIGN KEY constraint failed" to the client. This leaks internal database schema details.

**Steps to reproduce**:
```
POST /v1/relationType  body: name=Test
-> 400 {"error":"FOREIGN KEY constraint failed"}
```

**Expected**: A user-friendly error like `{"error":"fromCategory and toCategory are required"}` or a sanitized version.

**Root cause**: `server/api_handlers/relation_api_handlers.go:32` passes the raw DB error to the client. The `HandleError` function's `SanitizeSchemaError` only handles gorilla/schema errors, not database constraint errors. There's an `isUniqueConstraintError` utility in `application_context/db_errors.go`, but no equivalent for foreign key errors, and it's not used at the HTTP handler level.

---

### BUG 4: Raw "UNIQUE constraint failed" error leaks for Category, ResourceCategory, and Query (Medium)

**Severity**: Medium
**Type**: Information leak / unsanitized DB error
**Endpoints**: `POST /v1/category`, `POST /v1/resourceCategory`, `POST /v1/query`

**Description**: Creating a duplicate Category, ResourceCategory, or Query exposes the raw SQLite error. Tag creation correctly sanitizes this to a user-friendly message, but other entities do not.

**Steps to reproduce**:
```
POST /v1/category  body: {"name":"DupCat"}  (when "DupCat" already exists)
-> 400 {"error":"UNIQUE constraint failed: categories.name"}

POST /v1/resourceCategory  body: {"name":"DupResCat"}  (when "DupResCat" already exists)
-> 400 {"error":"UNIQUE constraint failed: resource_categories.name"}

POST /v1/query  body: {"name":"DupQuery","Text":"SELECT 1"}  (when "DupQuery" already exists)
-> 400 {"error":"UNIQUE constraint failed: queries.name"}
```

**Expected**: User-friendly error like `{"error":"a category named \"DupCat\" already exists"}` (matching the tag behavior at `application_context/tags_context.go:84`).

**Root cause**: The tag context at `application_context/tags_context.go:84` uses `isUniqueConstraintError()` to produce friendly messages, but the Category, ResourceCategory, and Query creation paths (which use the factory pattern) do not perform similar sanitization.

---

### BUG 5: Query run error exposes internal database details and uses wrong status code (Medium)

**Severity**: Medium
**Type**: Information leak + wrong HTTP status code
**Endpoint**: `POST /v1/query/run`

**Description**: When a saved query contains a write statement (DROP, DELETE, INSERT, etc.) and runs against the read-only connection, the raw SQLite error message "attempt to write a readonly database" is exposed. Additionally, these errors return HTTP 404 (Not Found) instead of 400 or 403.

**Steps to reproduce**:
1. Create a query: `POST /v1/query` with `{"name":"Drop","Text":"DROP TABLE notes"}`
2. Run it: `POST /v1/query/run?id=<id>`
3. Response: `{"error":"row iteration error: attempt to write a readonly database"}` with HTTP 404

**Expected**:
- Error message should be sanitized, e.g. "query execution failed: write operations are not permitted"
- Status code should be 400 (Bad Request) or 403 (Forbidden), not 404

**Root cause**: `server/api_handlers/query_api_handlers.go:148` passes raw DB errors to the client with 404 status.

---

### BUG 6: Download job Pause/Resume/Retry return 400 for "not found" instead of 404 (Low)

**Severity**: Low
**Type**: Inconsistent HTTP status codes
**Endpoints**: `POST /v1/download/pause`, `/resume`, `/retry` vs `POST /v1/download/cancel`

**Description**: The Cancel handler correctly returns 404 when a job is not found, but Pause, Resume, and Retry all return 400 for the same "not found" error.

**Steps to reproduce**:
```
POST /v1/download/cancel  body: id=nonexistent  -> 404 "job nonexistent not found" (correct)
POST /v1/download/pause   body: id=nonexistent  -> 400 "job nonexistent not found" (should be 404)
POST /v1/download/resume  body: id=nonexistent  -> 400 "job nonexistent not found" (should be 404)
POST /v1/download/retry   body: id=nonexistent  -> 400 "job nonexistent not found" (should be 404)
```

**Root cause**:
- Cancel at `download_queue_handlers.go:84` uses `http.StatusNotFound` (correct)
- Pause at `:108`, Resume at `:132`, Retry at `:156` all use `http.StatusBadRequest` (incorrect)

---

### BUG 7: editName/editDescription returns 400 instead of 404 for non-existent entities (Low)

**Severity**: Low
**Type**: Wrong HTTP status code
**Endpoints**: All `POST /v1/{entity}/editName`, `POST /v1/{entity}/editDescription`

**Description**: When calling editName or editDescription for a non-existent entity ID, the error "record not found" is returned with HTTP 400 (Bad Request) instead of 404 (Not Found).

**Steps to reproduce**:
```
POST /v1/note/editName?id=99999  body: Name=test  -> 400 "record not found"  (should be 404)
POST /v1/note/editDescription?id=99999  body: Description=test  -> 400 "record not found"  (should be 404)
```

**Root cause**: `server/api_handlers/generic_api_handlers.go:31` and `:62` - errors from `ctx.UpdateName()` / `ctx.UpdateDescription()` always use `http.StatusBadRequest`, regardless of error type.

**Affected entities**: All entity types (notes, groups, resources, tags, categories, queries, relations, relation types, note types, series).

---

### BUG 8: Inconsistent ID validation across entity GET endpoints (Low)

**Severity**: Low
**Type**: Inconsistency / misleading error
**Endpoints**: `GET /v1/note`, `GET /v1/group` vs `GET /v1/resource`

**Description**: Note and Group GET handlers silently convert non-numeric IDs to 0, resulting in a misleading "record not found" (404). The Resource GET handler properly validates and returns a clear 400 error.

**Steps to reproduce**:
```
GET /v1/note?id=abc    -> 404 "record not found"  (misleading: should be 400 "invalid id")
GET /v1/group?id=abc   -> 404 "record not found"  (misleading: should be 400 "invalid id")
GET /v1/resource?id=abc -> 400 "invalid value for \"id\": must be a valid number" (correct)
```

**Root cause**: `note_api_handlers.go:44` and `group_api_handlers.go:47` use `GetIntQueryParameter(request, "id", 0)` which silently defaults invalid input to 0. The resource handler uses `tryFillStructValuesFromRequest` with proper struct validation.

---

## Test Log

### Test 1: Race Conditions via Rapid API Calls
- Sent 3 parallel POST /v1/note updates to the same note
- Result: 1-2 succeeded, others returned "database is locked" with 400
- This is expected SQLite behavior under concurrent writes, not a bug
- No data corruption, no 500 errors observed
- **PASS** (expected behavior for SQLite)

### Test 2: Cross-Entity Integrity
- **Tag deletion**: Deleted tag attached to note and group. Join tables cleaned properly. PASS.
- **Note type deletion**: Deleted note type assigned to note. NoteTypeId nullified correctly. PASS.
- **Parent group deletion**: Deleted parent, child survived, parent link cleaned. PASS.
- **Category deletion**: Deleted category, group's CategoryId nullified correctly. PASS.
- **Tag merge**: Self-merge rejected, non-existent loser rejected. PASS.
- **Relation type without categories**: Raw FK error leaked (BUG 3).

### Test 3: Version/Hash Endpoints
- Version list, upload, restore, delete, compare all handle errors properly
- Version upload requires `resourceId` in query string (not form body) - consistent with handler design
- Version compare works correctly with `v1`/`v2` params
- **PASS** (no version-specific bugs)

### Test 4: OpenAPI Spec
- OpenAPI spec is generated via CLI tool (`go run ./cmd/openapi-gen`), not served at runtime
- **N/A** (not a runtime endpoint)

### Test 5: Pagination with Filters
- API uses fixed `MaxResultsPerPage = 50` (by design)
- Page parameter works correctly
- Sort with invalid columns silently ignored (safe)
- SQL injection in sort blocked by `SortColumnMatcher` regex
- **PASS**

### Test 6: Large/Invalid ID Values
- Large IDs (int32 max, beyond int32): Handled correctly, no overflow
- Negative IDs: Handled (inconsistently per BUG 8)
- Non-numeric IDs: Handled inconsistently (BUG 8)

### Test 7: Search Endpoint
- Empty, very long (10K chars), SQL injection, unicode: All handled gracefully
- **PASS**

### Test 8: Block API
- Create, update, delete with edge cases tested
- Delete non-existent block returns 500 (BUG 1)
- Update non-existent block returns 400 instead of 404 (same pattern as BUG 7)
- Invalid block type properly rejected
- **PARTIAL PASS** (BUG 1)

### Test 9: Download Queue
- Status code inconsistency between cancel vs pause/resume/retry (BUG 6)
- Otherwise handles edge cases well

### Test 10: Duplicate Entity Creation
- Tags: Properly sanitized error message
- Categories, ResourceCategories, Queries: Raw UNIQUE constraint leaked (BUG 4)
- Note types: Allow duplicates (by design, no unique constraint)

### Test 11: Resource Edit
- Invalid meta returns 500 instead of 400 (BUG 2)
- Owner validation works correctly
- Non-existent resource returns 404 (correct)

### Test 12: Group Tree
- Root nodes returned correctly when no parentId
- Children of non-existent parent returns `null` instead of `[]` (minor inconsistency, not filed as bug)

### Test 13: Note Sharing
- Share/unshare works correctly
- Non-existent note returns 404 (correct)

### Test 14: Admin Stats
- All three endpoints (server-stats, data-stats, expensive) return correctly
- **PASS**
