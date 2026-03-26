# Plan: Group B -- Error Message Quality Bugs

## Summary

Three bugs where raw internal error strings leak to users instead of friendly messages:

1. **BUG-1-02**: Non-numeric entity ID (`/note?id=abc`) shows raw `schema: error converting value for "id"`.
2. **BUG-1-03**: Duplicate tag creation via API returns raw `UNIQUE constraint failed: tags.name`.
3. **BUG-2-01**: Template `.json` route errors leak internal template context (`adminMenu`, `menu`, `assetVersion`, etc.) in the JSON response body.

---

## BUG-1-02: Non-numeric Entity ID Shows Raw Schema Error

### Root Cause

In `server/template_handlers/template_context_providers/template_context_providers.go`, the `addErrContext` function detects schema conversion errors but still passes the **raw error message** verbatim as `errorMessage`:

```go
} else if strings.Contains(errMsg, "schema: error converting value") ||
    strings.Contains(errMsg, "schema: invalid path") {
    statusCode = http.StatusBadRequest
}
// errMsg is still the raw "schema: error converting value for \"id\"" string
```

The status code is correctly set to 400, but the error message is not rewritten to something user-friendly. Compare with the `no such column` branch just below, which does replace the message with `"invalid sort column"`.

Similarly, in the API layer (`server/api_handlers/`), handlers call `tryFillStructValuesFromRequest` and pass the raw gorilla/schema error directly to `http_utils.HandleError`, which serializes it as-is into the JSON `{"error": "..."}` response.

### Files Involved

- `server/template_handlers/template_context_providers/template_context_providers.go` -- `addErrContext()` function
- `server/http_utils/http_helpers.go` -- `HandleError()` function (API JSON responses)
- `server/api_handlers/api_handlers.go` -- `tryFillStructValuesFromRequest()` is where schema errors originate

### RED: Failing Tests

#### Go Unit Test (new file: `server/api_tests/schema_error_friendly_message_test.go`)

```
TestNonNumericIdReturnsUserFriendlyError
  - Setup: SetupTestEnv(t)
  - Send GET /note.json?id=abc (with Accept: application/json header)
  - Assert: status code == 400
  - Decode JSON body into map[string]string
  - Assert: response["errorMessage"] does NOT contain "schema: error converting"
  - Assert: response["errorMessage"] contains a user-friendly phrase like "invalid value" or "must be a number"

TestNonNumericIdApiReturnsUserFriendlyError
  - Setup: SetupTestEnv(t)
  - Send GET /v1/tags?id=abc (API route, no Accept header)
  - Assert: status code == 400
  - Decode JSON body into map[string]string
  - Assert: response["error"] does NOT contain "schema: error converting"
  - Assert: response["error"] contains a user-friendly phrase like "invalid value"

TestNonNumericFilterParamReturnsFriendlyError
  - Setup: SetupTestEnv(t)
  - Send GET /groups.json?Tags=abc
  - Assert: status code == 400
  - Decode JSON body
  - Assert: response does NOT contain "schema: error converting"
```

#### E2E Test (new file: `e2e/tests/94-friendly-schema-error-messages.spec.ts`)

```
test('non-numeric id on detail page shows friendly error, not raw schema error')
  - Navigate to /note?id=abc
  - Assert status == 400
  - Assert page does NOT contain text "schema: error converting"
  - Assert page contains text matching /invalid|must be a number/i

test('non-numeric id on .json route shows friendly error')
  - Fetch /note.json?id=abc
  - Assert status == 400
  - Parse JSON body
  - Assert errorMessage does NOT contain "schema: error converting"

test('non-numeric tag filter on list page shows friendly error')
  - Navigate to /groups?Tags=abc
  - Assert status == 400
  - Assert page does NOT contain text "schema: error converting"
```

### GREEN: Minimal Fix

**File: `server/template_handlers/template_context_providers/template_context_providers.go`**

In `addErrContext`, modify the `schema: error converting value` branch to replace the raw error message with a user-friendly one:

```go
} else if strings.Contains(errMsg, "schema: error converting value") ||
    strings.Contains(errMsg, "schema: invalid path") {
    statusCode = http.StatusBadRequest
    errMsg = friendlySchemaError(errMsg)  // NEW
}
```

Add a helper function `friendlySchemaError(raw string) string` that parses the field name from the raw error (e.g., extracts `"id"` from `schema: error converting value for "id"`) and returns something like: `"Invalid value for 'id': must be a valid number"`. If the field name cannot be extracted, fall back to `"Invalid parameter value: check that numeric fields contain only numbers"`.

**File: `server/http_utils/http_helpers.go`**

Add a new function `SanitizeSchemaError(err error) error` that:
1. Checks if `err.Error()` contains `"schema: error converting value"`
2. If so, returns a new error with a friendly message (same logic as above)
3. Otherwise returns the original error

**File: `server/api_handlers/api_handlers.go`**

In `tryFillStructValuesFromRequest`, wrap the returned error:

```go
// After decoder.Decode(dst, ...) calls, wrap the error:
if err != nil {
    return http_utils.SanitizeSchemaError(err)
}
```

Alternatively, each handler already calls `tryFillStructValuesFromRequest` and passes the error to `HandleError`. We could instead make `HandleError` itself sanitize schema errors. This is cleaner because it's a single choke point. Add to `HandleError`:

```go
func HandleError(err error, writer http.ResponseWriter, request *http.Request, responseCode int) {
    err = SanitizeSchemaError(err)  // NEW: sanitize before displaying
    fmt.Printf("\n[ERROR]: %v\n", err)
    // ... rest unchanged
}
```

Decision: Apply the sanitization in **both** `addErrContext` (template layer) and `HandleError` (API layer) for full coverage.

### REFACTOR

- Extract the `friendlySchemaError` helper into `http_utils` so both template_context_providers and HandleError can share it.
- Consider a more general `SanitizeUserError` function that handles both schema errors and column errors (currently handled separately in `addErrContext`).

---

## BUG-1-03: Duplicate Tag Creation Returns Raw UNIQUE Constraint Error

### Root Cause

In `application_context/tags_context.go`, the `CreateTag` method calls `ctx.db.Create(&tag)` and returns the raw GORM error on failure. When SQLite encounters a duplicate name, GORM returns `UNIQUE constraint failed: tags.name`. This raw DB error propagates all the way to `CreateTagHandler` in `handler_factory.go`, which passes it to `http_utils.HandleError`, which serializes it as `{"error": "UNIQUE constraint failed: tags.name"}`.

There is no layer that intercepts unique constraint violations and translates them to user-friendly messages.

### Files Involved

- `application_context/tags_context.go` -- `CreateTag()` method (line 81)
- `server/api_handlers/handler_factory.go` -- `CreateTagHandler()` (line 257)
- `server/http_utils/http_helpers.go` -- could add general unique constraint detection here

### RED: Failing Tests

#### Go Unit Test (new file: `server/api_tests/duplicate_tag_friendly_error_test.go`)

```
TestDuplicateTagCreationReturnsFriendlyError
  - Setup: SetupTestEnv(t)
  - Create tag via API: POST /v1/tag with {"Name": "unique-test-tag"}
  - Assert: status 200 (first creation succeeds)
  - Create same tag again: POST /v1/tag with {"Name": "unique-test-tag"}
  - Assert: status 400 (or 409)
  - Decode JSON body into map[string]string
  - Assert: response["error"] does NOT contain "UNIQUE constraint failed"
  - Assert: response["error"] does NOT contain "tags.name"
  - Assert: response["error"] contains a user-friendly phrase like "already exists" or "duplicate"

TestDuplicateTagCreationViaFormReturnsFriendlyError
  - Setup: SetupTestEnv(t)
  - Create first tag: POST /v1/tag via MakeFormRequest with Name=form-dup-tag
  - Assert success
  - Create duplicate: POST /v1/tag via MakeFormRequest with Name=form-dup-tag
  - Assert: status >= 400
  - Assert: body does NOT contain "UNIQUE constraint failed"
```

#### E2E Test (new file: `e2e/tests/95-duplicate-tag-friendly-error.spec.ts`)

```
test('creating a duplicate tag via API returns a friendly error message')
  - Create tag "e2e-dup-tag" via API POST /v1/tag
  - Create same tag again via API POST /v1/tag
  - Assert status >= 400
  - Assert response body does NOT contain "UNIQUE constraint"
  - Assert response body contains /already exists|duplicate/i
```

### GREEN: Minimal Fix

**Option A (Application Layer -- Preferred):**

In `application_context/tags_context.go`, in `CreateTag()`, wrap the `ctx.db.Create(&tag)` error:

```go
if err := ctx.db.Create(&tag).Error; err != nil {
    if isUniqueConstraintError(err) {
        return nil, fmt.Errorf("a tag named %q already exists", tagQuery.Name)
    }
    return nil, err
}
```

Add a helper function `isUniqueConstraintError(err error) bool` in a shared location (e.g., `application_context/` or `models/`) that checks for:
- SQLite: `strings.Contains(msg, "UNIQUE constraint failed")`
- PostgreSQL: `strings.Contains(msg, "duplicate key value violates unique constraint")`

This approach is better because it gives entity-specific error messages ("a tag named X already exists") which are more helpful than generic ones.

**Option B (HTTP Layer -- Fallback):**

In `http_utils/http_helpers.go`, add sanitization to `HandleError` that catches unique constraint errors and rewrites them:

```go
func HandleError(err error, ...) {
    // ... after SanitizeSchemaError
    if isUniqueConstraintError(err) {
        err = errors.New("a record with this name already exists")
    }
    // ...
}
```

This is less ideal because the message is generic. Option A is preferred.

**Chosen approach: Option A** -- Wrap the error in `CreateTag()` at the application layer. Also apply the same pattern to `UpdateTag()` since renaming a tag could also hit the unique constraint.

Also check: does `CreateCategory`, `CreateNoteType`, etc. have similar unique constraints? If so, document them for a follow-up but do not fix in this group (scope control).

### REFACTOR

- Extract `isUniqueConstraintError(err) bool` into a shared utility (e.g., `models/errors.go` or a new `application_context/db_errors.go`) since other entities (Category, NoteType) may need it later.
- Consider whether `UpdateTag` also needs the same treatment (it does, since renaming to a duplicate name would trigger the same constraint).

---

## BUG-2-01: Template `.json` Route Errors Leak Internal Context

### Root Cause

In `server/template_handlers/render_template.go`, when a `.json` route is requested, the `RenderTemplate` function serializes the **entire `pongo2.Context` map** to JSON, minus a hardcoded set of fields in `discardFields`. However, the discard list is incomplete -- it does not remove:

- `menu` -- array of main navigation entries (Entry structs with Name/Url)
- `adminMenu` -- array of admin navigation entries
- `title` -- application title string ("mahresources")
- `assetVersion` -- hash string for cache busting
- `queryValues` -- the raw URL query params
- `url` -- the full request URL
- `hasPluginManager` -- boolean
- `pluginDetailActions`, `pluginCardActions`, `pluginBulkActions` -- plugin action lists

These are all template-rendering concerns that should not appear in a JSON API response. When an error occurs, the JSON response includes both the error info and all this internal context, e.g.:

```json
{
  "errorMessage": "record not found",
  "_statusCode": 404,
  "menu": [...],
  "adminMenu": [...],
  "title": "mahresources",
  "assetVersion": "abc123...",
  ...
}
```

### Files Involved

- `server/template_handlers/render_template.go` -- `RenderTemplate()`, specifically the `discardFields` map and the `.json` branch
- `server/template_handlers/template_context_providers/static_template_context.go` -- `baseTemplateContext` and `staticTemplateCtx` define the leaked fields
- `server/routes.go` -- `wrapContextWithPlugins` adds `_pluginManager`, `currentPath`, `pluginMenuItems`, `hasPluginManager`, `pluginDetailActions`, etc.

### RED: Failing Tests

#### Go Unit Test (new file: `server/api_tests/json_route_no_internal_context_test.go`)

```
TestJsonRouteErrorDoesNotLeakAdminMenu
  - Setup: SetupTestEnv(t)
  - Send GET /note.json?id=99999 (nonexistent entity, triggers error)
  - Assert: status == 404
  - Decode JSON body into map[string]any
  - Assert: map does NOT contain key "adminMenu"
  - Assert: map does NOT contain key "menu"
  - Assert: map does NOT contain key "assetVersion"
  - Assert: map does NOT contain key "title"
  - Assert: map does NOT contain key "queryValues"
  - Assert: map does NOT contain key "url"
  - Assert: map DOES contain key "errorMessage"

TestJsonRouteSuccessDoesNotLeakInternalFields
  - Setup: SetupTestEnv(t)
  - Create a tag via DB directly
  - Send GET /tag.json?id=<created_tag_id>
  - Assert: status == 200
  - Decode JSON body into map[string]any
  - Assert: map does NOT contain key "adminMenu"
  - Assert: map does NOT contain key "menu"
  - Assert: map does NOT contain key "assetVersion"
  - Assert: map does NOT contain key "queryValues"
  - Assert: map DOES contain key "tag" (the actual entity data)

TestJsonRouteDoesNotLeakPluginFields
  - Setup: SetupTestEnv(t)
  - Send GET /notes.json
  - Assert: status == 200
  - Decode JSON body into map[string]any
  - Assert: map does NOT contain key "hasPluginManager"
```

#### E2E Test (new file: `e2e/tests/96-json-route-no-internal-context.spec.ts`)

```
test('.json route for error does not leak adminMenu')
  - Fetch /note.json?id=99999
  - Assert status == 404
  - Parse JSON body
  - Assert body does NOT have key "adminMenu"
  - Assert body does NOT have key "menu"
  - Assert body does NOT have key "assetVersion"
  - Assert body does NOT have key "title"
  - Assert body DOES have key "errorMessage"

test('.json route for success does not leak internal context')
  - Create a tag via API
  - Fetch /tag.json?id=<id>
  - Parse JSON body
  - Assert body does NOT have key "adminMenu"
  - Assert body does NOT have key "menu"

test('.json route for list does not leak internal context')
  - Fetch /tags.json
  - Parse JSON body
  - Assert body does NOT have key "adminMenu"
  - Assert body does NOT have key "assetVersion"
```

### GREEN: Minimal Fix

**File: `server/template_handlers/render_template.go`**

Approach: Instead of maintaining an incomplete denylist of fields to discard, switch to a **combined approach**: keep the existing denylist (for function-valued fields that can't be serialized) and add all the "internal/rendering" fields that should never appear in JSON output.

Add the following keys to the `discardFields` map:

```go
"menu":                true,
"adminMenu":           true,
"title":               true,
"assetVersion":        true,
"queryValues":         true,
"url":                 true,
"hasPluginManager":    true,
"pluginDetailActions": true,
"pluginCardActions":   true,
"pluginBulkActions":   true,
```

The complete updated `discardFields` map will be:

```go
discardFields(map[string]bool{
    // Function-valued fields (can't serialize to JSON)
    "partial":     true,
    "withQuery":   true,
    "hasQuery":    true,
    "stringId":    true,
    "getNextId":   true,
    "dereference": true,
    // Internal/rendering fields (should not leak to JSON consumers)
    "path":               true,
    "menu":               true,
    "adminMenu":          true,
    "title":              true,
    "assetVersion":       true,
    "queryValues":        true,
    "url":                true,
    "_pluginManager":     true,
    "_statusCode":        true,
    "currentPath":        true,
    "pluginMenuItems":    true,
    "hasPluginManager":   true,
    "pluginDetailActions": true,
    "pluginCardActions":  true,
    "pluginBulkActions":  true,
}, context)
```

### REFACTOR

- Consider refactoring to an allowlist approach instead of a denylist. With a denylist, every new field added to the template context must also be added to `discardFields` or it leaks. An allowlist would be safer: only explicitly approved fields get serialized.
- However, an allowlist is harder to maintain because each template route returns different entity-specific fields (e.g., `note`, `tag`, `groups`, `pagination`, etc.). The denylist approach is simpler for now.
- Alternative: mark internal-only fields with a prefix convention (e.g., `_` prefix) and have the discard logic strip all `_`-prefixed fields automatically. The `_statusCode` and `_pluginManager` fields already use this convention. Extend it by renaming `menu` -> `_menu`, `adminMenu` -> `_adminMenu`, etc. in `baseTemplateContext` and all templates. This is a larger refactor but prevents future leaks.
- For now, stick with the explicit denylist expansion since it is the minimal fix.

---

## Implementation Order

1. **BUG-2-01** first (simplest -- just add keys to a map, no logic changes)
2. **BUG-1-02** second (needs a string parsing helper but straightforward)
3. **BUG-1-03** third (needs a DB error detection helper, slightly more involved)

## Files to Create

| File | Purpose |
|------|---------|
| `server/api_tests/schema_error_friendly_message_test.go` | Go tests for BUG-1-02 |
| `server/api_tests/duplicate_tag_friendly_error_test.go` | Go tests for BUG-1-03 |
| `server/api_tests/json_route_no_internal_context_test.go` | Go tests for BUG-2-01 |
| `e2e/tests/94-friendly-schema-error-messages.spec.ts` | E2E tests for BUG-1-02 |
| `e2e/tests/95-duplicate-tag-friendly-error.spec.ts` | E2E tests for BUG-1-03 |
| `e2e/tests/96-json-route-no-internal-context.spec.ts` | E2E tests for BUG-2-01 |

## Files to Modify

| File | Change |
|------|--------|
| `server/template_handlers/render_template.go` | Expand `discardFields` map (BUG-2-01) |
| `server/template_handlers/template_context_providers/template_context_providers.go` | Replace raw schema error message in `addErrContext` (BUG-1-02) |
| `server/http_utils/http_helpers.go` | Add `SanitizeSchemaError()` helper; apply in `HandleError` (BUG-1-02) |
| `application_context/tags_context.go` | Wrap unique constraint error in `CreateTag()` and `UpdateTag()` (BUG-1-03) |
