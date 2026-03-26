# Plan: Group A - Bulk Operation Input Validation

## Problem Summary

All bulk endpoints (`addTags`, `removeTags`, `addMeta`, `delete`, `addGroups`) silently return `200 {"ok":true}` when required parameters are missing or reference nonexistent entities. The root cause is in the `application_context/*_bulk_context.go` files: the business logic functions return `nil` (no error) for empty inputs instead of returning a validation error.

## Root Cause Analysis

### Bug 1 & 3: addMeta/addTags/removeTags/addGroups/delete with no IDs

- **addTags/removeTags/addGroups** (`BulkAddTagsToNotes`, `BulkRemoveTagsFromNotes`, etc.): The guard clause `if len(query.ID) == 0 || len(query.EditedId) == 0 { return nil }` silently succeeds.
- **addMeta** (`BulkAddMetaToNotes`, etc.): No guard for empty `query.ID` at all -- it runs `WHERE id IN ?` with an empty slice, which matches zero rows and GORM returns no error.
- **delete** (`BulkDeleteNotes`, etc.): Iterates over `query.ID` -- if empty, loop body never executes, returns nil.

### Bug 2: addMeta with nonexistent IDs

- `BulkAddMetaToNotes`/`BulkAddMetaToGroups`/`BulkAddMetaToResources`: Runs `UPDATE ... WHERE id IN ?` with nonexistent IDs. GORM updates zero rows and returns no error. No `RowsAffected` check exists.

### Bug 4: addTags/removeTags with no TagID (EditedId)

- Same as Bug 1/3: the `len(query.EditedId) == 0` guard returns `nil` instead of an error.

### Bug 5: addTags with nonexistent TagID

- This one is actually **already fixed** in the current code. `BulkAddTagsToNotes`, `BulkAddTagsToGroups`, and `BulkAddTagsToResources` all validate tag existence with a COUNT query and return `fmt.Errorf("one or more tags not found")` if the count doesn't match. However, if the entity IDs (the `query.ID` side) are nonexistent, the `INSERT ... SELECT id, ? FROM notes WHERE id IN ?` silently inserts zero rows with no error.

## Affected Functions and Files

| File | Function | Missing Validation |
|------|----------|--------------------|
| `application_context/note_bulk_context.go` | `BulkAddTagsToNotes` | Empty IDs return nil |
| `application_context/note_bulk_context.go` | `BulkRemoveTagsFromNotes` | Empty IDs return nil |
| `application_context/note_bulk_context.go` | `BulkAddGroupsToNotes` | Empty IDs return nil |
| `application_context/note_bulk_context.go` | `BulkAddMetaToNotes` | Empty IDs, nonexistent IDs |
| `application_context/note_bulk_context.go` | `BulkDeleteNotes` | Empty IDs |
| `application_context/group_bulk_context.go` | `BulkAddTagsToGroups` | Empty IDs return nil |
| `application_context/group_bulk_context.go` | `BulkRemoveTagsFromGroups` | Empty IDs return nil |
| `application_context/group_bulk_context.go` | `BulkAddMetaToGroups` | Empty IDs, nonexistent IDs |
| `application_context/group_bulk_context.go` | `BulkDeleteGroups` | Empty IDs |
| `application_context/resource_bulk_context.go` | `BulkAddTagsToResources` | Empty IDs return nil |
| `application_context/resource_bulk_context.go` | `BulkRemoveTagsFromResources` | Empty IDs return nil |
| `application_context/resource_bulk_context.go` | `BulkReplaceTagsFromResources` | Empty IDs return nil |
| `application_context/resource_bulk_context.go` | `BulkAddGroupsToResources` | Empty IDs return nil |
| `application_context/resource_bulk_context.go` | `BulkAddMetaToResources` | Empty IDs, nonexistent IDs |
| `application_context/resource_bulk_context.go` | `BulkDeleteResources` | Empty IDs |
| `application_context/tag_bulk_context.go` | `BulkDeleteTags` | Empty IDs |

---

## Phase 1: RED (Write Failing Tests)

### Test File

Create: `/Users/egecan/Code/mahresources/server/api_tests/bulk_validation_test.go`

Use the existing `SetupTestEnv` / `MakeFormRequest` / `MakeRequest` test helpers from `api_test_utils.go`. Use `url.Values` form encoding (matches how the frontend sends bulk ops).

### Test Cases

Each test posts to the bulk endpoint and asserts `400 Bad Request` instead of the current `200 OK`.

#### A. Empty IDs validation (all entity types x all bulk ops)

```
TestBulkAddTagsToNotes_NoIDs
  POST /v1/notes/addTags with form: EditedId=<valid tag ID>  (no ID param)
  Assert: 400

TestBulkAddTagsToNotes_NoTagID
  POST /v1/notes/addTags with form: ID=<valid note ID>  (no EditedId param)
  Assert: 400

TestBulkRemoveTagsFromNotes_NoIDs
  POST /v1/notes/removeTags with form: EditedId=<valid tag ID>  (no ID param)
  Assert: 400

TestBulkRemoveTagsFromNotes_NoTagID
  POST /v1/notes/removeTags with form: ID=<valid note ID>  (no EditedId param)
  Assert: 400

TestBulkAddGroupsToNotes_NoIDs
  POST /v1/notes/addGroups with form: EditedId=<valid group ID>  (no ID param)
  Assert: 400

TestBulkAddGroupsToNotes_NoGroupID
  POST /v1/notes/addGroups with form: ID=<valid note ID>  (no EditedId param)
  Assert: 400

TestBulkAddMetaToNotes_NoIDs
  POST /v1/notes/addMeta with JSON: {"Meta": "{\"key\":\"val\"}"}  (no ID param)
  Assert: 400

TestBulkDeleteNotes_NoIDs
  POST /v1/notes/delete with form: (empty body)
  Assert: 400

TestBulkAddTagsToGroups_NoIDs
  POST /v1/groups/addTags with form: EditedId=<valid tag ID>
  Assert: 400

TestBulkAddTagsToGroups_NoTagID
  POST /v1/groups/addTags with form: ID=<valid group ID>
  Assert: 400

TestBulkRemoveTagsFromGroups_NoIDs
  POST /v1/groups/removeTags with form: EditedId=<valid tag ID>
  Assert: 400

TestBulkRemoveTagsFromGroups_NoTagID
  POST /v1/groups/removeTags with form: ID=<valid group ID>
  Assert: 400

TestBulkAddMetaToGroups_NoIDs
  POST /v1/groups/addMeta with JSON: {"Meta": "{\"key\":\"val\"}"}
  Assert: 400

TestBulkDeleteGroups_NoIDs
  POST /v1/groups/delete with form: (empty body)
  Assert: 400

TestBulkAddTagsToResources_NoIDs
  POST /v1/resources/addTags with form: EditedId=<valid tag ID>
  Assert: 400

TestBulkAddTagsToResources_NoTagID
  POST /v1/resources/addTags with form: ID=<valid resource ID>
  Assert: 400

TestBulkRemoveTagsFromResources_NoIDs
  POST /v1/resources/removeTags with form: EditedId=<valid tag ID>
  Assert: 400

TestBulkRemoveTagsFromResources_NoTagID
  POST /v1/resources/removeTags with form: ID=<valid resource ID>
  Assert: 400

TestBulkReplaceTagsOfResources_NoIDs
  POST /v1/resources/replaceTags with form: (empty body -- no ID)
  Assert: 400

TestBulkAddGroupsToResources_NoIDs
  POST /v1/resources/addGroups with form: EditedId=<valid group ID>
  Assert: 400

TestBulkAddGroupsToResources_NoGroupID
  POST /v1/resources/addGroups with form: ID=<valid resource ID>
  Assert: 400

TestBulkAddMetaToResources_NoIDs
  POST /v1/resources/addMeta with JSON: {"Meta": "{\"key\":\"val\"}"}
  Assert: 400

TestBulkDeleteResources_NoIDs
  POST /v1/resources/delete with form: (empty body)
  Assert: 400

TestBulkDeleteTags_NoIDs
  POST /v1/tags/delete with form: (empty body)
  Assert: 400
```

#### B. Nonexistent entity IDs (addMeta only, since addTags already validates tag existence)

```
TestBulkAddMetaToNotes_NonexistentIDs
  Create no notes. POST /v1/notes/addMeta with JSON: {"ID": [999999], "Meta": "{\"key\":\"val\"}"}
  Assert: 400

TestBulkAddMetaToGroups_NonexistentIDs
  Create no groups. POST /v1/groups/addMeta with JSON: {"ID": [999999], "Meta": "{\"key\":\"val\"}"}
  Assert: 400

TestBulkAddMetaToResources_NonexistentIDs
  Create no resources. POST /v1/resources/addMeta with JSON: {"ID": [999999], "Meta": "{\"key\":\"val\"}"}
  Assert: 400
```

#### C. Nonexistent entity IDs on addTags (entity side, not tag side)

The `INSERT ... SELECT id, ? FROM notes WHERE id IN ?` pattern silently matches zero rows if note IDs don't exist. This is arguably acceptable (tags are validated; nonexistent notes just produce zero inserts). However, to be consistent, we should validate entity existence too.

```
TestBulkAddTagsToNotes_NonexistentNoteIDs
  Create a tag. POST /v1/notes/addTags with form: ID=999999&EditedId=<valid tag ID>
  Assert: 400

TestBulkAddTagsToGroups_NonexistentGroupIDs
  Create a tag. POST /v1/groups/addTags with form: ID=999999&EditedId=<valid tag ID>
  Assert: 400

TestBulkAddTagsToResources_NonexistentResourceIDs
  Create a tag. POST /v1/resources/addTags with form: ID=999999&EditedId=<valid tag ID>
  Assert: 400
```

### Test Structure

Use a single top-level test function per entity type with `t.Run` subtests, following the codebase convention. For example:

```go
func TestBulkNoteValidation(t *testing.T) {
    tc := SetupTestEnv(t)
    tag := &models.Tag{Name: "Bulk Test Tag"}
    tc.DB.Create(tag)
    note := tc.CreateDummyNote("Bulk Test Note")
    group := tc.CreateDummyGroup("Bulk Test Group")

    t.Run("addTags with no IDs returns 400", func(t *testing.T) { ... })
    t.Run("addTags with no TagID returns 400", func(t *testing.T) { ... })
    // ... etc
}

func TestBulkGroupValidation(t *testing.T) { ... }
func TestBulkResourceValidation(t *testing.T) { ... }
func TestBulkTagValidation(t *testing.T) { ... }
```

### Expected Behavior

For form-encoded requests, use `tc.MakeFormRequest`. For JSON requests (addMeta), use `tc.MakeRequest`. All should assert:
- `resp.Code == http.StatusBadRequest` (400)
- Response body contains an `"error"` key (from `http_utils.HandleError`)

---

## Phase 2: GREEN (Minimal Code Changes)

### Strategy: Validate in the context layer (business logic)

The validation belongs in the `application_context/*_bulk_context.go` functions, not the HTTP handlers. This ensures:
1. Any caller (API handler, CLI, plugin) gets the same validation
2. The HTTP handlers already propagate errors as 400 responses (they call `http_utils.HandleError(err, writer, request, http.StatusBadRequest)`)

### Change 1: Fix empty-ID guards to return errors

In every bulk function that currently has `return nil` for empty inputs, change to `return errors.New(...)`.

**File: `application_context/note_bulk_context.go`**

- `BulkAddTagsToNotes`: Change `if len(query.ID) == 0 || len(query.EditedId) == 0 { return nil }` to return an error. Use a descriptive message like `"at least one note ID is required"` or `"at least one tag ID is required"` depending on which is empty.
- `BulkRemoveTagsFromNotes`: Same pattern.
- `BulkAddGroupsToNotes`: Same pattern. Return `"at least one note ID is required"` / `"at least one group ID is required"`.
- `BulkAddMetaToNotes`: Add guard: `if len(query.ID) == 0 { return errors.New("at least one note ID is required") }`.
- `BulkDeleteNotes`: Add guard: `if len(query.ID) == 0 { return errors.New("at least one note ID is required") }`.

**File: `application_context/group_bulk_context.go`**

- `BulkAddTagsToGroups`: Same pattern as notes.
- `BulkRemoveTagsFromGroups`: Same pattern.
- `BulkAddMetaToGroups`: Add empty ID guard.
- `BulkDeleteGroups`: Add empty ID guard.

**File: `application_context/resource_bulk_context.go`**

- `BulkAddTagsToResources`: Same pattern.
- `BulkRemoveTagsFromResources`: Same pattern.
- `BulkReplaceTagsFromResources`: Change `if len(query.ID) == 0 { return nil }` to return error.
- `BulkAddGroupsToResources`: Same pattern.
- `BulkAddMetaToResources`: Add empty ID guard.
- `BulkDeleteResources`: Add empty ID guard.

**File: `application_context/tag_bulk_context.go`**

- `BulkDeleteTags`: Add `if len(query.ID) == 0 { return errors.New("at least one tag ID is required") }`.

### Change 2: Validate entity existence for addMeta

For `BulkAddMetaToNotes`, `BulkAddMetaToGroups`, `BulkAddMetaToResources`: after the JSON validation and before the UPDATE, add a COUNT query to verify all IDs exist:

```go
// In BulkAddMetaToNotes:
var count int64
if err := ctx.db.Model(&models.Note{}).Where("id IN ?", query.ID).Count(&count).Error; err != nil {
    return err
}
if int(count) != len(deduplicateUints(query.ID)) {
    return fmt.Errorf("one or more notes not found")
}
```

Same pattern for groups and resources.

### Change 3: Validate entity existence for addTags (entity side)

In `BulkAddTagsToNotes`, `BulkAddTagsToGroups`, `BulkAddTagsToResources`: inside the transaction, after the tag existence check, add a check for the entity IDs:

```go
// In BulkAddTagsToNotes (inside the transaction):
var noteCount int64
if err := tx.Model(&models.Note{}).Where("id IN ?", query.ID).Count(&noteCount).Error; err != nil {
    return err
}
uniqueEntityIds := deduplicateUints(query.ID)
if int(noteCount) != len(uniqueEntityIds) {
    return fmt.Errorf("one or more notes not found")
}
```

Same for groups and resources.

### Change 4: Split the OR guard into separate checks

For `BulkAddTagsToNotes` and similar functions, the current guard is:
```go
if len(query.ID) == 0 || len(query.EditedId) == 0 {
    return nil
}
```

This needs to become two separate checks with different error messages:
```go
if len(query.ID) == 0 {
    return errors.New("at least one note ID is required")
}
if len(query.EditedId) == 0 {
    return errors.New("at least one tag ID is required")
}
```

The `removeTags` variants need the same split. For `addGroups` variants, the second message should say `"at least one group ID is required"`.

---

## Phase 3: REFACTOR (Reduce Duplication)

After all tests pass, consider extracting common validation into helper functions.

### Option A: Validation helper functions in `application_context/`

Create helpers in `application_context/associations.go` (or a new `bulk_validation.go`):

```go
// requireIDs returns an error if the ID slice is empty.
func requireIDs(ids []uint, entityName string) error {
    if len(ids) == 0 {
        return fmt.Errorf("at least one %s ID is required", entityName)
    }
    return nil
}

// requireEditedIDs returns an error if the EditedId slice is empty.
func requireEditedIDs(ids []uint, entityName string) error {
    if len(ids) == 0 {
        return fmt.Errorf("at least one %s ID is required", entityName)
    }
    return nil
}

// validateBulkEditQuery validates both ID and EditedId slices are non-empty.
func validateBulkEditQuery(query *query_models.BulkEditQuery, entityName, editedEntityName string) error {
    if err := requireIDs(query.ID, entityName); err != nil {
        return err
    }
    return requireEditedIDs(query.EditedId, editedEntityName)
}

// validateEntitiesExist checks that all IDs in the slice exist in the given model table.
func validateEntitiesExist[T any](tx *gorm.DB, ids []uint) error {
    unique := deduplicateUints(ids)
    var count int64
    if err := tx.Model(new(T)).Where("id IN ?", unique).Count(&count).Error; err != nil {
        return err
    }
    if int(count) != len(unique) {
        return fmt.Errorf("one or more entities not found")
    }
    return nil
}
```

Then each bulk function becomes concise:

```go
func (ctx *MahresourcesContext) BulkAddTagsToNotes(query *query_models.BulkEditQuery) error {
    if err := validateBulkEditQuery(query, "note", "tag"); err != nil {
        return err
    }
    // ... rest of logic
}
```

### Option B: Keep validation inline but consistent

If the team prefers keeping validation inline (avoiding abstraction), just ensure all functions use the same error message patterns and the same ordering (check IDs first, then EditedId, then entity existence).

### Recommendation

Option A is preferred because there are 16 functions with the same validation pattern. The helper functions are simple, well-named, and testable. A single `bulk_validation.go` file with ~30 lines of helpers eliminates duplicated code across 4 files.

---

## Execution Order

1. **Create test file** `server/api_tests/bulk_validation_test.go` with all RED tests
2. **Run tests** to confirm they all FAIL with `200` instead of expected `400`
3. **Add validation** to each `*_bulk_context.go` function (Changes 1-4)
4. **Run tests** to confirm they all PASS
5. **Refactor** -- extract shared validation helpers, update all functions to use them
6. **Run tests** again to confirm nothing broke
7. **Run full test suite** (`go test --tags 'json1 fts5' ./...`) to check for regressions

### Regression Risk

The empty-ID guard change (`return nil` -> `return error`) could break frontend callers that accidentally submit empty bulk operations. However:
- The frontend bulk selection UI only enables the bulk action buttons when items are selected
- Returning an error for empty input is the correct behavior (it prevents silent no-ops that confuse users)
- The HTTP status changes from 200 to 400, which the frontend `abortableFetch` helper already handles by showing an error toast

No regressions are expected from the entity existence checks since they only add validation before operations that would be no-ops anyway.

---

## Files to Create/Modify

| Action | File |
|--------|------|
| CREATE | `server/api_tests/bulk_validation_test.go` |
| MODIFY | `application_context/note_bulk_context.go` |
| MODIFY | `application_context/group_bulk_context.go` |
| MODIFY | `application_context/resource_bulk_context.go` |
| MODIFY | `application_context/tag_bulk_context.go` |
| CREATE (refactor) | `application_context/bulk_validation.go` (optional, Phase 3) |
