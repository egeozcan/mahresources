# Tag Merge Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add the ability to merge tags — transferring all associations from loser tags to a winner, storing backups in meta, then deleting losers.

**Architecture:** Follows the exact merge pattern established by `MergeGroups` in `application_context/group_bulk_context.go`. Adds a `Meta` JSON field to the Tag model, a new `MergeTags` business logic function, a `POST /v1/tags/merge` API endpoint, and two UI entry points (tag detail page sidebar + tag list bulk editor).

**Tech Stack:** Go/GORM (backend), Pongo2 templates + Alpine.js (frontend), Playwright (E2E tests)

---

### Task 1: Add Meta field to Tag model

**Files:**
- Modify: `models/tag_model.go`

**Step 1: Add the Meta field**

In `models/tag_model.go`, add the `Meta` field to the `Tag` struct. Import `mahresources/models/types`.

```go
import (
	"mahresources/models/types"
	"time"
)

type Tag struct {
	ID          uint        `gorm:"primarykey"`
	CreatedAt   time.Time   `gorm:"index"`
	UpdatedAt   time.Time   `gorm:"index"`
	Name        string      `gorm:"uniqueIndex:unique_tag_name"`
	Description string      `gorm:"index"`
	Meta        types.JSON
	Resources   []*Resource `gorm:"many2many:resource_tags;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	Notes       []*Note     `gorm:"many2many:note_tags;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	Groups      []*Group    `gorm:"many2many:group_tags;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
}
```

**Step 2: Verify it compiles**

Run: `go build --tags 'json1 fts5'`
Expected: Compiles with no errors. GORM auto-migrates the new column.

**Step 3: Run existing tests to confirm no regressions**

Run: `go test ./... --tags 'json1 fts5'`
Expected: All existing tests pass.

**Step 4: Commit**

```bash
git add models/tag_model.go
git commit -m "feat(tags): add Meta JSON field to Tag model for merge backups"
```

---

### Task 2: Implement MergeTags business logic

**Files:**
- Create: `application_context/tag_bulk_context.go`

**Step 1: Create the merge function**

Create `application_context/tag_bulk_context.go` following the `MergeGroups` pattern from `application_context/group_bulk_context.go`:

```go
package application_context

import (
	"encoding/json"
	"errors"
	"fmt"

	"mahresources/constants"
	"mahresources/models"
	"mahresources/models/types"
)

func (ctx *MahresourcesContext) MergeTags(winnerId uint, loserIds []uint) error {
	if len(loserIds) == 0 {
		return errors.New("one or more losers required")
	}

	for _, id := range loserIds {
		if id == 0 {
			return errors.New("invalid tag ID")
		}
		if id == winnerId {
			return errors.New("winner cannot also be the loser")
		}
	}

	if winnerId == 0 {
		return errors.New("invalid winner ID")
	}

	return ctx.WithTransaction(func(altCtx *MahresourcesContext) error {
		// Load losers without associations — only need basic fields for backup
		var losers []*models.Tag
		if err := altCtx.db.Find(&losers, &loserIds).Error; err != nil {
			return err
		}

		// Load winner without associations
		var winner models.Tag
		if err := altCtx.db.First(&winner, winnerId).Error; err != nil {
			return err
		}

		// Batch SQL transfers — resource_tags
		if err := altCtx.db.Exec(
			"INSERT INTO resource_tags (resource_id, tag_id) SELECT resource_id, ? FROM resource_tags WHERE tag_id IN ? ON CONFLICT DO NOTHING",
			winnerId, loserIds,
		).Error; err != nil {
			return err
		}

		// Batch SQL transfers — note_tags
		if err := altCtx.db.Exec(
			"INSERT INTO note_tags (note_id, tag_id) SELECT note_id, ? FROM note_tags WHERE tag_id IN ? ON CONFLICT DO NOTHING",
			winnerId, loserIds,
		).Error; err != nil {
			return err
		}

		// Batch SQL transfers — group_tags
		if err := altCtx.db.Exec(
			"INSERT INTO group_tags (group_id, tag_id) SELECT group_id, ? FROM group_tags WHERE tag_id IN ? ON CONFLICT DO NOTHING",
			winnerId, loserIds,
		).Error; err != nil {
			return err
		}

		// Build backup data from losers
		backups := make(map[string]types.JSON)
		for _, loser := range losers {
			backupData, err := json.Marshal(loser)
			if err != nil {
				return err
			}
			backups[fmt.Sprintf("tag_%v", loser.ID)] = backupData
		}

		// Save backups to winner's meta
		backupObj := map[string]any{"backups": backups}
		backupsBytes, err := json.Marshal(&backupObj)
		if err != nil {
			return err
		}

		switch altCtx.Config.DbType {
		case constants.DbTypePosgres:
			if err := altCtx.db.Exec(
				"UPDATE tags SET meta = COALESCE(meta, '{}'::jsonb) || ? WHERE id = ?",
				backupsBytes, winner.ID,
			).Error; err != nil {
				return err
			}
		case constants.DbTypeSqlite:
			if err := altCtx.db.Exec(
				"UPDATE tags SET meta = json_patch(COALESCE(meta, '{}'), ?) WHERE id = ?",
				string(backupsBytes), winner.ID,
			).Error; err != nil {
				return err
			}
		default:
			return errors.New("db doesn't support merging meta")
		}

		// Delete losers (cascade removes stale join entries)
		for _, loser := range losers {
			if err := altCtx.DeleteTag(loser.ID); err != nil {
				return err
			}
		}

		return nil
	})
}
```

**Key differences from MergeGroups:**
- No ownership transfers (tags have no `owner_id`)
- No meta merging between winner/losers (winner keeps its description)
- No self-reference cleanup needed
- Three join tables instead of six

**Step 2: Verify it compiles**

Run: `go build --tags 'json1 fts5'`
Expected: Compiles with no errors.

**Step 3: Commit**

```bash
git add application_context/tag_bulk_context.go
git commit -m "feat(tags): implement MergeTags business logic"
```

---

### Task 3: Add TagMerger interface and API handler

**Files:**
- Modify: `server/interfaces/tag_interfaces.go`
- Modify: `server/api_handlers/tag_api_handlers.go`

**Step 1: Add TagMerger interface**

In `server/interfaces/tag_interfaces.go`, add the `TagMerger` interface following the `GroupMerger` pattern from `server/interfaces/group_interfaces.go`:

```go
// TagMerger handles tag merging operations
type TagMerger interface {
	MergeTags(winnerId uint, loserIds []uint) error
}
```

**Step 2: Add the merge handler**

In `server/api_handlers/tag_api_handlers.go`, add `GetMergeTagsHandler` following the exact pattern from `GetMergeGroupsHandler` in `server/api_handlers/group_api_handlers.go:226`:

```go
func GetMergeTagsHandler(ctx interfaces.TagMerger) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		effectiveCtx := withRequestContext(ctx, request).(interfaces.TagMerger)

		var editor = query_models.MergeQuery{}
		var err error

		if err = tryFillStructValuesFromRequest(&editor, request); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		err = effectiveCtx.MergeTags(editor.Winner, editor.Losers)

		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		http_utils.RedirectIfHTMLAccepted(writer, request, "/tags")
	}
}
```

Note: This requires importing `query_models` which is already imported in this file.

**Step 3: Verify it compiles**

Run: `go build --tags 'json1 fts5'`
Expected: Compiles with no errors.

**Step 4: Commit**

```bash
git add server/interfaces/tag_interfaces.go server/api_handlers/tag_api_handlers.go
git commit -m "feat(tags): add TagMerger interface and merge API handler"
```

---

### Task 4: Register merge route and OpenAPI spec

**Files:**
- Modify: `server/routes.go` (around line 231)
- Modify: `server/routes_openapi.go` (in `registerTagRoutes` function, around line 1070)

**Step 1: Add the route**

In `server/routes.go`, after line 231 (the `tag/editDescription` line), add:

```go
	router.Methods(http.MethodPost).Path("/v1/tags/merge").HandlerFunc(api_handlers.GetMergeTagsHandler(appContext))
```

**Step 2: Add the OpenAPI registration**

In `server/routes_openapi.go`, inside `registerTagRoutes` (around line 1070, before the closing `}`), add:

```go
	mergeQueryType := reflect.TypeOf(query_models.MergeQuery{})

	r.Register(openapi.RouteInfo{
		Method:              http.MethodPost,
		Path:                "/v1/tags/merge",
		OperationID:         "mergeTags",
		Summary:             "Merge tags",
		Tags:                []string{"tags"},
		RequestType:         mergeQueryType,
		RequestContentTypes: []openapi.ContentType{openapi.ContentTypeJSON, openapi.ContentTypeForm},
	})
```

Note: `query_models` and `reflect` are already imported in this file. Check the imports at the top of `registerTagRoutes` — `query_models.MergeQuery` is used elsewhere in the file so it should be available.

**Step 3: Verify it compiles**

Run: `go build --tags 'json1 fts5'`
Expected: Compiles with no errors.

**Step 4: Commit**

```bash
git add server/routes.go server/routes_openapi.go
git commit -m "feat(tags): register /v1/tags/merge route and OpenAPI spec"
```

---

### Task 5: Add merge form to tag detail page sidebar

**Files:**
- Modify: `templates/displayTag.tpl`

**Step 1: Add the merge form**

Replace the empty `{% block sidebar %}` block in `templates/displayTag.tpl` with a merge form. Follow the pattern from `templates/displayGroup.tpl:56-67`:

```django
{% block sidebar %}
    {% include "/partials/sideTitle.tpl" with title="Meta Data" %}
    {% include "/partials/json.tpl" with jsonData=tag.Meta %}

    <form
        x-data="confirmAction({ message: `Selected tags will be deleted and merged to {{ tag.Name|json }}. Are you sure?` })"
        action="/v1/tags/merge"
        :action="'/v1/tags/merge?redirect=' + encodeURIComponent(window.location)"
        method="post"
        x-bind="events"
    >
        <input type="hidden" name="winner" value="{{ tag.ID }}">
        <p>Merge others with this tag?</p>
        {% include "/partials/form/autocompleter.tpl" with url='/v1/tags' elName='losers' title='Tags To Merge' id=getNextId("autocompleter") %}
        <div class="mt-2">{% include "/partials/form/searchButton.tpl" with text="Merge" %}</div>
    </form>
{% endblock %}
```

**Step 2: Verify by building and manual check**

Run: `npm run build` (to rebuild CSS/JS if needed, though templates are served dynamically)
Run: `go build --tags 'json1 fts5'`

**Step 3: Commit**

```bash
git add templates/displayTag.tpl
git commit -m "feat(tags): add merge form to tag detail page sidebar"
```

---

### Task 6: Add bulk selection to tag list page

**Files:**
- Create: `templates/partials/bulkEditorTag.tpl`
- Modify: `templates/listTags.tpl`

**Step 1: Create the bulk editor partial**

Create `templates/partials/bulkEditorTag.tpl` following the pattern from `templates/partials/bulkEditorGroup.tpl`:

```django
<div class="pb-3" x-data x-show="[...$store.bulkSelection.selectedIds].length === 0" x-collapse>
    {% include "/partials/form/formParts/connected/selectAllButton.tpl" %}
</div>
<div x-cloak class="sticky top-0 z-50 flex pl-4 pb-2 lg:gap-4 gap-1 flex-wrap bulk-editors items-center" x-show="[...$store.bulkSelection.selectedIds].length > 0" x-collapse x-data="bulkSelectionForms">
    {% include "/partials/form/formParts/connected/deselectButton.tpl" %}
    {% include "/partials/form/formParts/connected/selectAllButton.tpl" %}
    <form
        class="px-4"
        method="post"
        :action="'/v1/tags/merge?redirect=' + encodeURIComponent(window.location)"
        x-data="confirmAction('Selected tags will be merged. Are you sure?')"
        x-bind="events"
    >
        {% include "/partials/form/formParts/connected/selectedIds.tpl" %}
        <div class="flex gap-2 items-start">
            {% include "/partials/form/autocompleter.tpl" with url='/v1/tags' max=1 elName='winner' title='Merge Winner' id=getNextId("tag_autocompleter") %}
            <div class="mt-7">{% include "/partials/form/searchButton.tpl" with text="Merge" %}</div>
        </div>
    </form>
    <form
        class="px-4 no-ajax"
        method="post"
        :action="'/v1/tags/delete?redirect=' + encodeURIComponent(window.location)"
        x-data="confirmAction('Are you sure you want to delete the selected tags?')"
        x-bind="events"
    >
        {% include "/partials/form/formParts/connected/selectedIds.tpl" %}
        <div class="flex flex-col">
            <span class="block text-sm font-medium text-gray-700 mt-3">Delete Selected</span>
            {% include "/partials/form/searchButton.tpl" with text="Delete" danger=true %}
        </div>
    </form>
</div>
```

Note about the merge form: The `selectedIds` partial injects hidden `id` fields for all selected items. The bulk merge endpoint receives these as `losers`. The user picks the `winner` via the autocompleter. The selected items become the losers. This is slightly different from the detail page pattern (where winner is pre-set), but uses the same API endpoint.

**Important:** The bulk merge form sends selected IDs as `losers` (via the `selectedIds.tpl` which uses the field name from the form context). Check how `selectedIds.tpl` works — it likely uses `id` as the field name. The `MergeQuery` expects `Losers` field. We need to verify the field name mapping. Read `templates/partials/form/formParts/connected/selectedIds.tpl` to understand how it generates hidden inputs.

If `selectedIds.tpl` generates inputs named `id`, the merge handler won't map them to `Losers`. In that case, use a different approach: have the JS fill losers from the bulk selection. Alternatively, use the pattern where the form submits to a custom handler. **Check and adjust as needed during implementation.**

**Step 2: Update the tag list template**

Modify `templates/listTags.tpl` to include the bulk editor and make tag cards selectable. The key changes:

1. Add `{% block prebody %}` with the bulk editor include
2. Change the tag card rendering to use `selectable=true`

Since tags don't have a dedicated partial like `partials/group.tpl`, we need to add selectable support directly in the list template. Make the tag articles selectable by adding `x-data="selectableItem({ itemId: {{ tag.ID }} })"` and a checkbox, following the `partials/group.tpl` pattern:

```django
{% extends "/layouts/base.tpl" %}

{% block prebody %}
    {% include "/partials/bulkEditorTag.tpl" %}
{% endblock %}

{% block body %}
    <div class="list-container">
        {% for tag in tags %}
            <article class="card tag-card card--selectable" x-data="selectableItem({ itemId: {{ tag.ID }} })">
                <input type="checkbox" :checked="selected() ? 'checked' : null" x-bind="events" aria-label="Select {{ tag.Name }}" class="card-checkbox focus:ring-indigo-500 h-6 w-6 text-indigo-600 border-gray-300 rounded">
                <h3 class="card-title card-title--simple">
                    <a href="/tag?id={{ tag.ID }}">{{ tag.Name }}</a>
                </h3>
                {% if tag.Description %}
                <div class="card-description">
                    {% include "/partials/description.tpl" with description=tag.Description preview=true %}
                </div>
                {% endif %}
            </article>
        {% empty %}
            <p class="text-gray-500 text-sm py-4">No tags found. <a href="/createTag" class="text-indigo-600 hover:text-indigo-800 underline">Create one</a>.</p>
        {% endfor %}
    </div>
{% endblock %}

{% block sidebar %}
    {% include "/partials/sideTitle.tpl" with title="Filter" %}
    <form class="flex gap-2 items-start flex-col" aria-label="Filter tags">
        {% include "/partials/form/textInput.tpl" with name='Name' label='Name' value=queryValues.Name.0 %}
        {% include "/partials/form/textInput.tpl" with name='Description' label='Description' value=queryValues.Description.0 %}
        {% include "/partials/form/dateInput.tpl" with name='CreatedBefore' label='Created Before' value=queryValues.CreatedBefore.0 %}
        {% include "/partials/form/dateInput.tpl" with name='CreatedAfter' label='Created After' value=queryValues.CreatedAfter.0 %}
        {% include "/partials/form/searchButton.tpl" %}
    </form>
{% endblock %}
```

**Step 3: Check how selectedIds.tpl maps field names**

Read `templates/partials/form/formParts/connected/selectedIds.tpl` to understand what input name it generates. If it uses `id` we may need to adjust the bulk merge approach.

**Step 4: Verify by building**

Run: `go build --tags 'json1 fts5'`

**Step 5: Commit**

```bash
git add templates/partials/bulkEditorTag.tpl templates/listTags.tpl
git commit -m "feat(tags): add bulk selection and merge to tag list page"
```

---

### Task 7: Add bulk delete API endpoint for tags

**Files:**
- Modify: `application_context/tags_context.go` (or create `application_context/tag_bulk_context.go` if not already done — it was created in Task 2, so add to it)
- Modify: `server/api_handlers/tag_api_handlers.go`
- Modify: `server/interfaces/tag_interfaces.go`
- Modify: `server/routes.go`
- Modify: `server/routes_openapi.go`

The bulk editor has a "Delete Selected" form that submits to `/v1/tags/delete`. We need to ensure this endpoint handles bulk deletion (multiple IDs). Check if the existing `tag/delete` handler supports it. If not, add a `POST /v1/tags/delete` bulk endpoint.

**Step 1: Add BulkDeleteTags to the context**

Add to `application_context/tag_bulk_context.go`:

```go
func (ctx *MahresourcesContext) BulkDeleteTags(query *query_models.BulkQuery) error {
	return ctx.WithTransaction(func(altCtx *MahresourcesContext) error {
		for _, id := range query.ID {
			if err := altCtx.DeleteTag(id); err != nil {
				return err
			}
		}
		return nil
	})
}
```

**Step 2: Add BulkTagDeleter interface**

In `server/interfaces/tag_interfaces.go`:

```go
// BulkTagDeleter handles bulk tag deletion
type BulkTagDeleter interface {
	BulkDeleteTags(query *query_models.BulkQuery) error
}
```

**Step 3: Add the handler**

In `server/api_handlers/tag_api_handlers.go`, add `GetBulkDeleteTagsHandler` following the `GetBulkDeleteGroupsHandler` pattern:

```go
func GetBulkDeleteTagsHandler(ctx interfaces.BulkTagDeleter) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		effectiveCtx := withRequestContext(ctx, request).(interfaces.BulkTagDeleter)

		var editor = query_models.BulkQuery{}
		var err error

		if err = tryFillStructValuesFromRequest(&editor, request); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		err = effectiveCtx.BulkDeleteTags(&editor)

		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		http_utils.RedirectIfHTMLAccepted(writer, request, "/tags")
	}
}
```

**Step 4: Register the route**

In `server/routes.go`, after the merge route:

```go
	router.Methods(http.MethodPost).Path("/v1/tags/delete").HandlerFunc(api_handlers.GetBulkDeleteTagsHandler(appContext))
```

In `server/routes_openapi.go`, inside `registerTagRoutes`:

```go
	bulkQueryType := reflect.TypeOf(query_models.BulkQuery{})

	r.Register(openapi.RouteInfo{
		Method:              http.MethodPost,
		Path:                "/v1/tags/delete",
		OperationID:         "bulkDeleteTags",
		Summary:             "Bulk delete tags",
		Tags:                []string{"tags"},
		RequestType:         bulkQueryType,
		RequestContentTypes: []openapi.ContentType{openapi.ContentTypeJSON, openapi.ContentTypeForm},
	})
```

**Step 5: Verify it compiles**

Run: `go build --tags 'json1 fts5'`

**Step 6: Commit**

```bash
git add application_context/tag_bulk_context.go server/api_handlers/tag_api_handlers.go server/interfaces/tag_interfaces.go server/routes.go server/routes_openapi.go
git commit -m "feat(tags): add bulk delete endpoint for tags"
```

---

### Task 8: Add mergeTags method to E2E API client

**Files:**
- Modify: `e2e/helpers/api-client.ts`

**Step 1: Add the mergeTags method**

In `e2e/helpers/api-client.ts`, after the `deleteTag` method (around line 185), add:

```typescript
  async mergeTags(winnerId: number, loserIds: number[]): Promise<void> {
    const formData = new URLSearchParams();
    formData.append('Winner', winnerId.toString());
    for (const id of loserIds) {
      formData.append('Losers', id.toString());
    }
    return this.postVoidRetry(`${this.baseUrl}/v1/tags/merge`, {
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      data: formData.toString(),
    });
  }

  async bulkDeleteTags(ids: number[]): Promise<void> {
    const formData = new URLSearchParams();
    for (const id of ids) {
      formData.append('id', id.toString());
    }
    return this.postVoidRetry(`${this.baseUrl}/v1/tags/delete`, {
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      data: formData.toString(),
    });
  }
```

**Step 2: Commit**

```bash
git add e2e/helpers/api-client.ts
git commit -m "feat(tags): add mergeTags and bulkDeleteTags to E2E API client"
```

---

### Task 9: Write E2E test for tag merging

**Files:**
- Create: `e2e/tests/25-tag-merge.spec.ts`

**Step 1: Write the E2E test**

Create `e2e/tests/25-tag-merge.spec.ts`:

```typescript
import { test, expect } from '../fixtures/base.fixture';

test.describe.serial('Tag Merge Operations', () => {
  let winnerTagId: number;
  let loserTag1Id: number;
  let loserTag2Id: number;
  let groupId: number;
  let categoryId: number;
  let testRunId: string;

  test.beforeAll(async ({ apiClient }) => {
    testRunId = `${Date.now()}-${Math.random().toString(36).substring(2, 8)}`;

    // Create a category for the group
    const category = await apiClient.createCategory(
      `Merge Test Category ${testRunId}`,
      'Category for tag merge tests'
    );
    categoryId = category.ID;

    // Create tags
    const winner = await apiClient.createTag(`Winner Tag ${testRunId}`, 'The winner');
    winnerTagId = winner.ID;

    const loser1 = await apiClient.createTag(`Loser Tag 1 ${testRunId}`, 'First loser');
    loserTag1Id = loser1.ID;

    const loser2 = await apiClient.createTag(`Loser Tag 2 ${testRunId}`, 'Second loser');
    loserTag2Id = loser2.ID;

    // Create a group and assign loser tags to it
    const group = await apiClient.createGroup({
      name: `Merge Test Group ${testRunId}`,
      categoryId: categoryId,
      tags: [loserTag1Id, loserTag2Id],
    });
    groupId = group.ID;
  });

  test('should merge tags and transfer associations', async ({ apiClient, tagPage, groupPage, page }) => {
    // Merge loser tags into winner
    await apiClient.mergeTags(winnerTagId, [loserTag1Id, loserTag2Id]);

    // Verify loser tags are deleted
    await tagPage.verifyTagNotInList(`Loser Tag 1 ${testRunId}`);
    await tagPage.verifyTagNotInList(`Loser Tag 2 ${testRunId}`);

    // Verify winner tag still exists
    await tagPage.verifyTagInList(`Winner Tag ${testRunId}`);

    // Verify the group now has the winner tag (associations were transferred)
    await groupPage.gotoDisplay(groupId);
    await expect(page.locator(`a:has-text("Winner Tag ${testRunId}")`).first()).toBeVisible();
  });

  test('should show merge form on tag detail page', async ({ tagPage, page }) => {
    await tagPage.gotoDisplay(winnerTagId);

    // Verify the merge form is present in sidebar
    await expect(page.locator('text=Merge others with this tag?')).toBeVisible();
    await expect(page.locator('text=Tags To Merge')).toBeVisible();
  });

  test('should show meta with merge backups', async ({ tagPage, page }) => {
    await tagPage.gotoDisplay(winnerTagId);

    // Verify meta section shows backup data
    await expect(page.locator('text=backups')).toBeVisible();
  });

  test.afterAll(async ({ apiClient }) => {
    try { await apiClient.deleteTag(winnerTagId); } catch {}
    try { await apiClient.deleteGroup(groupId); } catch {}
    try { await apiClient.deleteCategory(categoryId); } catch {}
    // Losers are already deleted by the merge
  });
});

test.describe('Tag List Bulk Selection', () => {
  let tag1Id: number;
  let tag2Id: number;
  let testRunId: string;

  test.beforeAll(async ({ apiClient }) => {
    testRunId = `${Date.now()}-${Math.random().toString(36).substring(2, 8)}`;

    const tag1 = await apiClient.createTag(`Bulk Tag A ${testRunId}`, 'First bulk tag');
    tag1Id = tag1.ID;

    const tag2 = await apiClient.createTag(`Bulk Tag B ${testRunId}`, 'Second bulk tag');
    tag2Id = tag2.ID;
  });

  test('should show bulk editor when tag selected', async ({ tagPage, page }) => {
    await tagPage.gotoList();

    // Select a tag checkbox
    await page.locator(`[x-data*="itemId: ${tag1Id}"] input[type="checkbox"]`).check();

    // Bulk editor should appear
    await expect(page.locator('button:has-text("Deselect All"), button:has-text("Deselect")')).toBeVisible();
  });

  test('should bulk delete tags via API', async ({ apiClient, tagPage }) => {
    await apiClient.bulkDeleteTags([tag1Id, tag2Id]);
    await tagPage.verifyTagNotInList(`Bulk Tag A ${testRunId}`);
    await tagPage.verifyTagNotInList(`Bulk Tag B ${testRunId}`);
  });

  test.afterAll(async ({ apiClient }) => {
    try { await apiClient.deleteTag(tag1Id); } catch {}
    try { await apiClient.deleteTag(tag2Id); } catch {}
  });
});
```

**Step 2: Run the E2E tests**

Run: `cd e2e && npm run test:with-server -- --grep "Tag Merge|Tag List Bulk"`
Expected: All tests pass.

**Step 3: Run the full E2E suite to check for regressions**

Run: `cd e2e && npm run test:with-server`
Expected: All existing tests still pass.

**Step 4: Commit**

```bash
git add e2e/tests/25-tag-merge.spec.ts
git commit -m "test(tags): add E2E tests for tag merge and bulk selection"
```

---

### Task 10: Build, run full test suite, and verify

**Step 1: Build everything**

Run: `npm run build`
Expected: JS + CSS + Go binary all build successfully.

**Step 2: Run Go unit tests**

Run: `go test ./... --tags 'json1 fts5'`
Expected: All pass.

**Step 3: Run full E2E suite**

Run: `cd e2e && npm run test:with-server`
Expected: All tests pass including the new tag merge tests.

**Step 4: Regenerate OpenAPI spec**

Run: `go run ./cmd/openapi-gen`
Expected: Spec regenerated with the new `/v1/tags/merge` and `/v1/tags/delete` endpoints.

**Step 5: Commit OpenAPI spec if changed**

```bash
git add openapi.yaml
git commit -m "docs: regenerate OpenAPI spec with tag merge endpoints"
```
