# Maintainability Cleanup Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Fix correctness bugs, error-handling defects, content negotiation, lint findings, and code duplication across the mahresources codebase.

**Architecture:** Sequential behavior-preserving fixes. Items 1-5 are quick isolated fixes. Items 6-8 are medium-effort cleanup. Items 9-10 are frontend/CI. Each item is one commit.

**Tech Stack:** Go 1.22+, Gorilla Mux, Pongo2 templates, Alpine.js, Vite, Playwright (E2E)

---

### Task 1: Fix silent error shadowing in create providers

**Files:**
- Modify: `server/template_handlers/template_context_providers/category_template_context.go:65-85`
- Modify: `server/template_handlers/template_context_providers/tag_template_context.go:65-85`
- Modify: `server/template_handlers/template_context_providers/query_template_context.go:65-86`
- Modify: `server/template_handlers/template_context_providers/resource_category_template_context.go:61-81`

**Step 1: Fix `CategoryCreateContextProvider`**

In `category_template_context.go`, replace lines 71-78:

```go
		var query query_models.EntityIdQuery
		err := decoder.Decode(&query, request.URL.Query())

		category, err := context.GetCategory(query.ID)

		if err != nil {
			return tplContext
		}
```

With:

```go
		var query query_models.EntityIdQuery
		if err := decoder.Decode(&query, request.URL.Query()); err != nil {
			return addErrContext(err, tplContext)
		}

		if query.ID == 0 {
			return tplContext
		}

		category, err := context.GetCategory(query.ID)

		if err != nil {
			return tplContext
		}
```

**Step 2: Fix `TagCreateContextProvider`**

In `tag_template_context.go`, apply the identical pattern: replace lines 71-78 the same way — check decode error first, check `query.ID == 0`, then fetch tag.

**Step 3: Fix `QueryCreateContextProvider`**

In `query_template_context.go`, replace lines 72-78:

```go
		var entityId query_models.EntityIdQuery
		err := decoder.Decode(&entityId, request.URL.Query())

		query, err := context.GetQuery(entityId.ID)

		if err != nil {
			return tplContext
		}
```

With:

```go
		var entityId query_models.EntityIdQuery
		if err := decoder.Decode(&entityId, request.URL.Query()); err != nil {
			return addErrContext(err, tplContext)
		}

		if entityId.ID == 0 {
			return tplContext
		}

		query, err := context.GetQuery(entityId.ID)

		if err != nil {
			return tplContext
		}
```

**Step 4: Fix `ResourceCategoryCreateContextProvider`**

In `resource_category_template_context.go`, replace lines 67-73:

```go
		var query query_models.EntityIdQuery
		_ = decoder.Decode(&query, request.URL.Query())

		resourceCategory, err := context.GetResourceCategory(query.ID)

		if err != nil {
			return tplContext
		}
```

With:

```go
		var query query_models.EntityIdQuery
		if err := decoder.Decode(&query, request.URL.Query()); err != nil {
			return addErrContext(err, tplContext)
		}

		if query.ID == 0 {
			return tplContext
		}

		resourceCategory, err := context.GetResourceCategory(query.ID)

		if err != nil {
			return tplContext
		}
```

**Step 5: Run tests to verify**

Run: `go test --tags 'json1 fts5' ./server/template_handlers/...`
Expected: PASS

Run: `go test --tags 'json1 fts5' ./...`
Expected: PASS

**Step 6: Commit**

```bash
git add server/template_handlers/template_context_providers/category_template_context.go \
       server/template_handlers/template_context_providers/tag_template_context.go \
       server/template_handlers/template_context_providers/query_template_context.go \
       server/template_handlers/template_context_providers/resource_category_template_context.go
git commit -m "fix: stop shadowing decode errors in create context providers"
```

---

### Task 2: Fix copy/paste correctness bugs

**Files:**
- Modify: `server/template_handlers/template_context_providers/resource_template_context.go:278`
- Modify: `server/template_handlers/template_context_providers/group_template_context.go:62-66`

**Step 1: Fix typo in resource breadcrumb**

In `resource_template_context.go` line 278, change:

```go
Url:  fmt.Sprintf("/resouce?id=%v", resource.ID),
```

To:

```go
Url:  fmt.Sprintf("/resource?id=%v", resource.ID),
```

**Step 2: Remove duplicate dead `if err != nil` block**

In `group_template_context.go`, remove lines 62-66 (the second duplicate `if err != nil` block after the `GetTagsWithIds` call):

```go
		if err != nil {
			fmt.Println(err)

			return addErrContext(err, baseContext)
		}
```

This block is unreachable because the identical block on lines 57-61 handles the same `err`.

**Step 3: Run tests**

Run: `go test --tags 'json1 fts5' ./server/template_handlers/...`
Expected: PASS

**Step 4: Commit**

```bash
git add server/template_handlers/template_context_providers/resource_template_context.go \
       server/template_handlers/template_context_providers/group_template_context.go
git commit -m "fix: correct resource breadcrumb URL typo and remove dead code block"
```

---

### Task 3: Fix content negotiation to use Accept header

**Files:**
- Modify: `server/template_handlers/render_template.go:49`

**Step 1: Change header check**

In `render_template.go` line 49, change:

```go
		if contentType := request.Header.Get("Content-type"); contentType == constants.JSON || strings.HasSuffix(request.URL.Path, ".json") {
```

To:

```go
		if accept := request.Header.Get("Accept"); strings.Contains(accept, constants.JSON) || strings.HasSuffix(request.URL.Path, ".json") {
```

Note: use `strings.Contains` rather than `==` because Accept headers can contain multiple types (e.g. `application/json, text/plain`). The `strings` import is already present.

**Step 2: Run tests**

Run: `go test --tags 'json1 fts5' ./server/...`
Expected: PASS

**Step 3: Commit**

```bash
git add server/template_handlers/render_template.go
git commit -m "fix: use Accept header for content negotiation instead of Content-Type"
```

---

### Task 4: Make JSON request parsing robust for charset headers

**Files:**
- Modify: `server/api_handlers/api_handlers.go:41`

**Step 1: Change strict equality to prefix check**

In `api_handlers.go` line 41, change:

```go
	if contentTypeHeader == constants.JSON {
```

To:

```go
	if strings.HasPrefix(contentTypeHeader, constants.JSON) {
```

The `strings` import is already present in this file.

**Step 2: Run tests**

Run: `go test --tags 'json1 fts5' ./server/...`
Expected: PASS

**Step 3: Commit**

```bash
git add server/api_handlers/api_handlers.go
git commit -m "fix: accept application/json with charset parameter in Content-Type"
```

---

### Task 5: Fix mock and test quality

**Files:**
- Modify: `application_context/mock_context/mock_group_context.go`
- Modify: `server/template_handlers/template_context_providers/group_template_context_test.go`

**Step 1: Replace panics in mock with stub returns**

Replace the entire content of `mock_group_context.go` with proper stubs. Write methods return `errors.New("mock: not implemented")` instead of panicking. Read methods like `GetGroups` return empty results. Keep the existing working implementations for `GetGroup`, `FindParentsOfGroup`, and `NewMockGroupContext`.

```go
package mock_context

import (
	"errors"
	"mahresources/models"
	"mahresources/models/query_models"
	"time"
)

var errNotImplemented = errors.New("mock: not implemented")

type MockGroupContext struct{}

func NewMockGroupContext() *MockGroupContext {
	return &MockGroupContext{}
}

func (r MockGroupContext) CreateGroup(g *query_models.GroupCreator) (*models.Group, error) {
	return nil, errNotImplemented
}

func (r MockGroupContext) UpdateGroup(g *query_models.GroupEditor) (*models.Group, error) {
	return nil, errNotImplemented
}

func (r MockGroupContext) BulkAddTagsToGroups(query *query_models.BulkEditQuery) error {
	return errNotImplemented
}

func (r MockGroupContext) BulkRemoveTagsFromGroups(query *query_models.BulkEditQuery) error {
	return errNotImplemented
}

func (r MockGroupContext) BulkAddMetaToGroups(query *query_models.BulkEditMetaQuery) error {
	return errNotImplemented
}

func (r MockGroupContext) MergeGroups(winnerId uint, loserIds []uint) error {
	return errNotImplemented
}

func (r MockGroupContext) DuplicateGroup(id uint) (*models.Group, error) {
	return nil, errNotImplemented
}

func (r MockGroupContext) DeleteGroup(groupId uint) error {
	return errNotImplemented
}

func (r MockGroupContext) BulkDeleteGroups(query *query_models.BulkQuery) error {
	return errNotImplemented
}

func (r MockGroupContext) UpdateGroupName(id uint, name string) error {
	return errNotImplemented
}

func (r MockGroupContext) UpdateGroupDescription(id uint, description string) error {
	return errNotImplemented
}

func (MockGroupContext) GetGroups(offset, maxResults int, query *query_models.GroupQuery) ([]models.Group, error) {
	return []models.Group{}, nil
}

func (MockGroupContext) GetGroup(id uint) (*models.Group, error) {
	return &models.Group{
		ID:        0,
		CreatedAt: time.Time{},
		UpdatedAt: time.Time{},
	}, nil
}

func (r MockGroupContext) FindParentsOfGroup(id uint) ([]models.Group, error) {
	return []models.Group{}, nil
}
```

**Step 2: Rewrite test with assertions**

Replace `group_template_context_test.go`:

```go
package template_context_providers

import (
	"mahresources/application_context/mock_context"
	"net/http/httptest"
	"testing"
)

func TestGroupContextProviderImpl(t *testing.T) {
	reader := mock_context.NewMockGroupContext()
	provider := groupContextProviderImpl(reader)

	req := httptest.NewRequest("GET", "http://example.com/group?id=1", nil)
	ctx := provider(req)

	if ctx["pageTitle"] == nil {
		t.Error("expected pageTitle in context, got nil")
	}

	if ctx["group"] == nil {
		t.Error("expected group in context, got nil")
	}

	if ctx["breadcrumb"] == nil {
		t.Error("expected breadcrumb in context, got nil")
	}
}

func TestGroupContextProviderImpl_NoID(t *testing.T) {
	reader := mock_context.NewMockGroupContext()
	provider := groupContextProviderImpl(reader)

	req := httptest.NewRequest("GET", "http://example.com/group", nil)
	ctx := provider(req)

	if ctx["errorMessage"] == nil {
		t.Error("expected errorMessage in context when no ID provided")
	}
}
```

**Step 3: Run tests**

Run: `go test --tags 'json1 fts5' ./server/template_handlers/template_context_providers/...`
Expected: PASS

Run: `go test --tags 'json1 fts5' ./...`
Expected: PASS

**Step 4: Commit**

```bash
git add application_context/mock_context/mock_group_context.go \
       server/template_handlers/template_context_providers/group_template_context_test.go
git commit -m "fix: replace panic-based mocks with stubs and add real test assertions"
```

---

### Task 6: Remove fmt.Println error noise from template providers

**Files:**
- Modify: `server/template_handlers/template_context_providers/category_template_context.go`
- Modify: `server/template_handlers/template_context_providers/tag_template_context.go`
- Modify: `server/template_handlers/template_context_providers/query_template_context.go`
- Modify: `server/template_handlers/template_context_providers/resource_category_template_context.go`
- Modify: `server/template_handlers/template_context_providers/group_template_context.go`
- Modify: `server/template_handlers/template_context_providers/note_template_context.go`
- Modify: `server/template_handlers/template_context_providers/resource_template_context.go`
- Modify: `server/template_handlers/template_context_providers/series_template_context.go`
- Modify: `server/template_handlers/template_context_providers/relation_template_context.go`
- Modify: `server/template_handlers/template_context_providers/compare_template_context.go`
- Modify: `server/template_handlers/template_context_providers/log_template_context.go`

**Step 1: Remove all `fmt.Println(err)` lines before `addErrContext` calls**

In every template context provider file, remove all lines matching the pattern:

```go
			fmt.Println(err)
```

that appear immediately before `return addErrContext(err, ...)`. The `addErrContext` helper already puts the error message into the template context for display. The `fmt.Println` is noise that goes to stdout with no context.

After removing these, also remove the `"fmt"` import from any file that no longer uses `fmt`. Files that still use `fmt.Sprintf` (like for URL construction) should keep the import.

**Step 2: Run tests**

Run: `go test --tags 'json1 fts5' ./server/template_handlers/...`
Expected: PASS

Run: `go test --tags 'json1 fts5' ./...`
Expected: PASS (verify no compile errors from unused imports)

**Step 3: Commit**

```bash
git add server/template_handlers/template_context_providers/
git commit -m "refactor: remove fmt.Println noise from template context providers"
```

---

### Task 7: De-duplicate handler factory create-or-update handlers

**Files:**
- Modify: `server/api_handlers/handler_factory.go:193-343`

**Step 1: Extract generic `createOrUpdateHandler`**

Add this helper function before the four specific handlers:

```go
// createOrUpdateHandler creates an HTTP handler for create-or-update operations.
// getID extracts the ID from the decoded request struct.
// create and update are the respective operations.
// entityName is used for redirect URL construction (e.g., "tag", "category").
func createOrUpdateHandler[T any](
	entityName string,
	getID func(*T) uint,
	create func(*T) (interface{}, error),
	update func(*T) (interface{}, error),
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var editor T

		if err := tryFillStructValuesFromRequest(&editor, r); err != nil {
			http_utils.HandleError(err, w, r, http.StatusBadRequest)
			return
		}

		var result interface{}
		var err error

		if getID(&editor) != 0 {
			result, err = update(&editor)
		} else {
			result, err = create(&editor)
		}

		if err != nil {
			http_utils.HandleError(err, w, r, http.StatusBadRequest)
			return
		}

		type hasID interface{ GetId() uint }
		if entity, ok := result.(hasID); ok {
			redirectURL := "/" + entityName + "?id=" + strconv.Itoa(int(entity.GetId()))
			if http_utils.RedirectIfHTMLAccepted(w, r, redirectURL) {
				return
			}
		}

		w.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(w).Encode(result)
	}
}
```

**Step 2: Replace the four handlers with thin wrappers**

```go
func CreateTagHandler(reader interfaces.TagsReader, writer interfaces.TagsWriter) http.HandlerFunc {
	return createOrUpdateHandler(
		"tag",
		func(c *query_models.TagCreator) uint { return c.ID },
		func(c *query_models.TagCreator) (interface{}, error) { return writer.CreateTag(c) },
		func(c *query_models.TagCreator) (interface{}, error) { return writer.UpdateTag(c) },
	)
}

func CreateCategoryHandler(writer interfaces.CategoryWriter) http.HandlerFunc {
	return createOrUpdateHandler(
		"category",
		func(e *query_models.CategoryEditor) uint { return e.ID },
		func(e *query_models.CategoryEditor) (interface{}, error) { return writer.CreateCategory(&e.CategoryCreator) },
		func(e *query_models.CategoryEditor) (interface{}, error) { return writer.UpdateCategory(e) },
	)
}

func CreateResourceCategoryHandler(writer interfaces.ResourceCategoryWriter) http.HandlerFunc {
	return createOrUpdateHandler(
		"resourceCategory",
		func(e *query_models.ResourceCategoryEditor) uint { return e.ID },
		func(e *query_models.ResourceCategoryEditor) (interface{}, error) { return writer.CreateResourceCategory(&e.ResourceCategoryCreator) },
		func(e *query_models.ResourceCategoryEditor) (interface{}, error) { return writer.UpdateResourceCategory(e) },
	)
}

func CreateQueryHandler(writer interfaces.QueryWriter) http.HandlerFunc {
	return createOrUpdateHandler(
		"query",
		func(e *query_models.QueryEditor) uint { return e.ID },
		func(e *query_models.QueryEditor) (interface{}, error) { return writer.CreateQuery(&e.QueryCreator) },
		func(e *query_models.QueryEditor) (interface{}, error) { return writer.UpdateQuery(e) },
	)
}
```

Note: `CreateTagHandler` signature keeps the `reader` param even though it's unused in the body — check if routes.go passes it. If so, keep the signature stable. If `reader` is not used anywhere in the function, remove it from the signature and update the call site.

**Step 3: Remove unused `reader` parameter from `CreateTagHandler` if applicable**

Check `server/routes.go` for how `CreateTagHandler` is called. If `reader` is passed but unused, remove it from both the handler signature and the call site.

**Step 4: Run tests**

Run: `go test --tags 'json1 fts5' ./server/...`
Expected: PASS

Run: `go test --tags 'json1 fts5' ./...`
Expected: PASS

**Step 5: Commit**

```bash
git add server/api_handlers/handler_factory.go
# Also add routes.go if CreateTagHandler signature changed
git commit -m "refactor: extract generic createOrUpdateHandler to de-duplicate handler factory"
```

---

### Task 8: Clear staticcheck backlog

**Files (all modifications):**
- `application_context/resource_media_context.go:77` — SA4006: unused `fileBytes`
- `application_context/resource_bulk_context.go:304` — S1028: `errors.New(fmt.Sprintf(...))` → `fmt.Errorf(...)`
- `application_context/resource_media_context.go:1095` — S1028: same
- `application_context/resource_upload_context.go:424,430` — S1028: same
- `server/api_handlers/resource_api_handlers.go:169` — S1028: same
- `application_context/group_bulk_context.go:209` — S1002: `== false` → `!`
- `hash_worker/worker.go:83` — U1000: unused `logInfo` method
- `models/block_types/table.go:9` — U1000: unused `tableColumn` type
- `models/group_model.go:49` — U1000: unused `initials` method
- `models/tag_model.go:18` — U1000/SA4005: unused `setId` with ineffective assignment
- `models/types/json.go:19` — SA9004: missing explicit type on const group
- `server/api_handlers/query_api_handlers.go:28` — S1005: unnecessary blank identifier
- `server/openapi/registry.go:199` — SA1019: deprecated `openapi3.BoolPtr` → `openapi3.Ptr`
- `server/openapi/schema.go:103` — SA1019: same
- `server/template_handlers/loaders/template_loader.go:7` — SA1019: deprecated `ioutil` → `os`
- `models/database_scopes/group_scope.go:41,45,68,88,108` — S1009: nil check before len
- `models/database_scopes/note_scope.go:18,26,30` — S1009: same
- `models/database_scopes/resource_scope.go:18,22,36,54` — S1009: same
- `server/template_handlers/template_filters/base64_filter.go:18` — S1009: nil check before len
- `server/template_handlers/template_context_providers/static_template_context.go:211` — S1034: type switch variable
- `server/template_handlers/template_filters/markdown_filter.go:16` — S1034: same

**Step 1: Fix SA4006 — unused variable assignment**

In `resource_media_context.go:77`, the variable `fileBytes` from `getOrCreateNullThumbnail` is assigned but never used. Change:

```go
	nullThumbnail, fileBytes, err := ctx.getOrCreateNullThumbnail(resource, fs, httpContext)
```

To:

```go
	nullThumbnail, _, err := ctx.getOrCreateNullThumbnail(resource, fs, httpContext)
```

Verify that `fileBytes` is truly unused in the rest of the function before making this change.

**Step 2: Fix S1028 — use fmt.Errorf instead of errors.New(fmt.Sprintf(...))**

In each of these files, replace the pattern `errors.New(fmt.Sprintf("...", args))` with `fmt.Errorf("...", args)`:

- `resource_bulk_context.go:304`: `errors.New(fmt.Sprintf("loser number %v has 0 id", i+1))` → `fmt.Errorf("loser number %v has 0 id", i+1)`
- `resource_media_context.go:1095`: same pattern
- `resource_upload_context.go:424,430`: same pattern
- `resource_api_handlers.go:169`: same pattern

**Step 3: Fix S1002 — simplify bool comparison**

In `group_bulk_context.go:209`, change:

```go
	if json.Valid([]byte(query.Meta)) == false {
```

To:

```go
	if !json.Valid([]byte(query.Meta)) {
```

**Step 4: Fix U1000 — remove unused code**

- Delete `tableColumn` struct from `models/block_types/table.go:8-12` (type and its comment)
- Delete `initials()` method from `models/group_model.go:49-58`
- Delete `setId()` method from `models/tag_model.go:18-20` (also fixes SA4005)
- Delete `logInfo()` method from `hash_worker/worker.go:82-85` (and its comment)

**Step 5: Fix SA1019 — deprecated APIs**

- In `server/openapi/registry.go:199` and `server/openapi/schema.go:103`: change `openapi3.BoolPtr(true)` to `openapi3.Ptr(true)`
- In `server/template_handlers/loaders/template_loader.go:78`: change `ioutil.ReadFile(path)` to `os.ReadFile(path)` and remove `"io/ioutil"` from imports

**Step 6: Fix SA9004 — explicit type on const group**

In `models/types/json.go:19-25`, add explicit type to subsequent constants:

```go
const (
	OperatorEquals              JsonOperation = "="
	OperatorLike                JsonOperation = "LIKE"
	OperatorNotEquals           JsonOperation = "<>"
	OperatorNotLike             JsonOperation = "NOT LIKE"
	OperatorGreaterThan         JsonOperation = ">"
	OperatorGreaterThanOrEquals JsonOperation = ">="
	OperatorLessThan            JsonOperation = "<"
```

Continue for all constants in the block.

**Step 7: Fix S1005 — unnecessary blank identifier**

In `query_api_handlers.go:28`, change:

```go
		for i, _ := range columns {
```

To:

```go
		for i := range columns {
```

**Step 8: Fix S1009 — nil check before len**

In all database scope files (`group_scope.go`, `note_scope.go`, `resource_scope.go`) and `base64_filter.go`, change patterns like:

```go
if query.Ids != nil && len(query.Ids) > 0 {
```

To:

```go
if len(query.Ids) > 0 {
```

Because `len(nil)` returns 0 in Go.

For `base64_filter.go:18`, change:

```go
	if input == nil || len(input) == 0 {
```

To:

```go
	if len(input) == 0 {
```

**Step 9: Fix S1034 — type switch variable**

In `static_template_context.go:210-221`, change:

```go
func dereference(v interface{}) interface{} {
	switch v.(type) {
	case *uint:
		return *v.(*uint)
	case *string:
		return *v.(*string)
	case *time.Time:
		return *v.(*time.Time)
	default:
		return v
	}
}
```

To:

```go
func dereference(v interface{}) interface{} {
	switch v := v.(type) {
	case *uint:
		return *v
	case *string:
		return *v
	case *time.Time:
		return *v
	default:
		return v
	}
}
```

In `markdown_filter.go:16-21`, change:

```go
	switch interfaceVal.(type) {
	case string:
		md = interfaceVal.(string)
	case *string:
		md = *interfaceVal.(*string)
	}
```

To:

```go
	switch v := interfaceVal.(type) {
	case string:
		md = v
	case *string:
		md = *v
	}
```

**Step 10: Run staticcheck**

Run: `staticcheck ./...`
Expected: Zero findings (or only findings unrelated to the categories above)

Run: `go test --tags 'json1 fts5' ./...`
Expected: PASS

**Step 11: Commit**

```bash
git add -A
git commit -m "fix: resolve all staticcheck SA4006/U1000/SA1019/S1028/S1002/S1009/S1034 findings"
```

---

### Task 9: Frontend hotspots — batch calendar operations

**Files:**
- Modify: `src/components/blocks/blockCalendar.js`

**Step 1: Batch multi-select calendar additions**

In `blockCalendar.js`, find the resource picker handler (around line 365-389) that calls `this.addCalendarFromResource` in a loop. Each call to `addCalendarFromResource` triggers `saveContent()` + `fetchEvents(true)` individually.

Refactor so the resource picker handler:
1. Collects all resources to add
2. Adds them to `this.calendars` and `this.calendarMeta` directly (without calling `addCalendarFromResource`)
3. Calls `this.saveContent()` once
4. Calls `this.fetchEvents(true)` once

The `addCalendarFromResource` method stays for single-add use cases (e.g., URL add). Only the multi-select loop changes.

**Step 2: Build and verify**

Run: `npm run build-js`
Expected: Build succeeds

**Step 3: Commit**

```bash
git add src/components/blocks/blockCalendar.js
git commit -m "perf: batch calendar multi-select to single save+fetch"
```

**Note:** The schemaForm.js split (item 9 second part) is deferred from this cleanup pass — it requires more careful extraction and is lower priority than the calendar batching fix.

---

### Task 10: Add CI workflow and .dockerignore

**Files:**
- Create: `.github/workflows/ci.yml`
- Create: `.dockerignore`

**Step 1: Create CI workflow**

Create `.github/workflows/ci.yml`:

```yaml
name: CI

on:
  push:
    branches: [master]
  pull_request:
    branches: [master]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod

      - name: Install build dependencies
        run: sudo apt-get update && sudo apt-get install -y gcc libsqlite3-dev

      - name: Run tests
        run: go test --tags 'json1 fts5' ./...

      - name: Install staticcheck
        run: go install honnef.co/go/tools/cmd/staticcheck@latest

      - name: Run staticcheck
        run: staticcheck ./...
```

**Step 2: Create .dockerignore**

Create `.dockerignore`:

```
.git
node_modules
e2e
*.db
.env
docs
docs-site
.github
.husky
.claude
```

**Step 3: Verify Docker build still works (optional)**

If Docker is available: `docker build -t mahresources-test .`
Expected: Build succeeds and context transfer is faster

**Step 4: Commit**

```bash
git add .github/workflows/ci.yml .dockerignore
git commit -m "ci: add Go test and staticcheck CI workflow, add .dockerignore"
```

---

### Task 11: Final verification

**Step 1: Run full test suite**

Run: `go test --tags 'json1 fts5' ./...`
Expected: PASS

**Step 2: Run staticcheck**

Run: `staticcheck ./...`
Expected: Zero high-signal findings (SA4006, U1000, SA1019, S1028)

**Step 3: Build frontend**

Run: `npm run build`
Expected: Build succeeds

**Step 4: Run E2E tests**

Run: `cd e2e && npm run test:with-server`
Expected: PASS
