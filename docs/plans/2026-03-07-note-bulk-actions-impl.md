# Note Bulk Actions Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add bulk actions (add tags, remove tags, add groups, add meta, delete) for notes, mirroring the existing resource/group patterns.

**Architecture:** New interfaces → new context methods → new API handlers → new routes → new template partial + template updates. No JS changes needed (Alpine `bulkSelection` store is entity-agnostic).

**Tech Stack:** Go (GORM, Gorilla Mux), Pongo2 templates, Alpine.js, Playwright (E2E tests)

---

### Task 1: Add Bulk Note Interfaces

**Files:**
- Modify: `server/interfaces/note_interfaces.go`

**Step 1: Add bulk interfaces and update NoteWriter composite**

After the existing `NoteSharer` interface (line 44), add:

```go
// BulkNoteTagEditor handles bulk tag operations on notes
type BulkNoteTagEditor interface {
	BulkAddTagsToNotes(query *query_models.BulkEditQuery) error
	BulkRemoveTagsFromNotes(query *query_models.BulkEditQuery) error
}

// BulkNoteGroupEditor handles bulk group operations on notes
type BulkNoteGroupEditor interface {
	BulkAddGroupsToNotes(query *query_models.BulkEditQuery) error
}

// BulkNoteMetaEditor handles bulk meta operations on notes
type BulkNoteMetaEditor interface {
	BulkAddMetaToNotes(query *query_models.BulkEditMetaQuery) error
}

// BulkNoteDeleter handles bulk note deletion
type BulkNoteDeleter interface {
	BulkDeleteNotes(query *query_models.BulkQuery) error
}
```

Update existing `NoteDeleter` to include `BulkDeleteNotes`:

```go
type NoteDeleter interface {
	DeleteNote(noteId uint) error
	BulkDeleteNotes(query *query_models.BulkQuery) error
}
```

Update existing `NoteWriter` to compose bulk interfaces:

```go
type NoteWriter interface {
	CreateOrUpdateNote(noteQuery *query_models.NoteEditor) (*models.Note, error)
	BulkNoteTagEditor
	BulkNoteGroupEditor
	BulkNoteMetaEditor
}
```

**Step 2: Verify compilation**

Run: `go build --tags 'json1 fts5'`
Expected: BUILD FAILURE — `MahresourcesContext` doesn't implement new interfaces yet. That's expected.

**Step 3: Commit**

```bash
git add server/interfaces/note_interfaces.go
git commit -m "feat(notes): add bulk operation interfaces for notes"
```

---

### Task 2: Add Bulk Note Context Methods

**Files:**
- Create: `application_context/note_bulk_context.go`

**Step 1: Create note_bulk_context.go with all 5 bulk methods**

Follow the exact patterns from `application_context/group_bulk_context.go` lines 147-217.

```go
package application_context

import (
	"encoding/json"
	"errors"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"mahresources/constants"
	"mahresources/models"
	"mahresources/models/query_models"
)

func (ctx *MahresourcesContext) BulkAddTagsToNotes(query *query_models.BulkEditQuery) error {
	if len(query.ID) == 0 || len(query.EditedId) == 0 {
		return nil
	}

	uniqueEditedIds := deduplicateUints(query.EditedId)

	return ctx.db.Transaction(func(tx *gorm.DB) error {
		var tagCount int64
		if err := tx.Model(&models.Tag{}).Where("id IN ?", uniqueEditedIds).Count(&tagCount).Error; err != nil {
			return err
		}
		if int(tagCount) != len(uniqueEditedIds) {
			return fmt.Errorf("one or more tags not found")
		}

		for _, tagID := range uniqueEditedIds {
			if err := tx.Exec(
				"INSERT INTO note_tags (note_id, tag_id) SELECT id, ? FROM notes WHERE id IN ? ON CONFLICT DO NOTHING",
				tagID, query.ID,
			).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (ctx *MahresourcesContext) BulkRemoveTagsFromNotes(query *query_models.BulkEditQuery) error {
	if len(query.ID) == 0 || len(query.EditedId) == 0 {
		return nil
	}

	return ctx.db.Transaction(func(tx *gorm.DB) error {
		return tx.Exec(
			"DELETE FROM note_tags WHERE note_id IN ? AND tag_id IN ?",
			query.ID, query.EditedId,
		).Error
	})
}

func (ctx *MahresourcesContext) BulkAddGroupsToNotes(query *query_models.BulkEditQuery) error {
	if len(query.ID) == 0 || len(query.EditedId) == 0 {
		return nil
	}

	uniqueEditedIds := deduplicateUints(query.EditedId)

	return ctx.db.Transaction(func(tx *gorm.DB) error {
		var groupCount int64
		if err := tx.Model(&models.Group{}).Where("id IN ?", uniqueEditedIds).Count(&groupCount).Error; err != nil {
			return err
		}
		if int(groupCount) != len(uniqueEditedIds) {
			return fmt.Errorf("one or more groups not found")
		}

		for _, groupID := range uniqueEditedIds {
			if err := tx.Exec(
				"INSERT INTO groups_related_notes (note_id, group_id) SELECT id, ? FROM notes WHERE id IN ? ON CONFLICT DO NOTHING",
				groupID, query.ID,
			).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (ctx *MahresourcesContext) BulkAddMetaToNotes(query *query_models.BulkEditMetaQuery) error {
	if !json.Valid([]byte(query.Meta)) {
		return errors.New("invalid json")
	}

	var note models.Note
	var expr clause.Expr

	if ctx.Config.DbType == constants.DbTypePosgres {
		expr = gorm.Expr("meta || ?", query.Meta)
	} else {
		expr = gorm.Expr("json_patch(meta, ?)", query.Meta)
	}

	return ctx.db.
		Model(&note).
		Where("id in ?", query.ID).
		Update("Meta", expr).Error
}

func (ctx *MahresourcesContext) BulkDeleteNotes(query *query_models.BulkQuery) error {
	return ctx.WithTransaction(func(altCtx *MahresourcesContext) error {
		for _, id := range query.ID {
			if err := altCtx.DeleteNote(id); err != nil {
				return err
			}
		}
		return nil
	})
}
```

**Step 2: Verify compilation**

Run: `go build --tags 'json1 fts5'`
Expected: SUCCESS — `MahresourcesContext` now satisfies all bulk note interfaces.

**Step 3: Commit**

```bash
git add application_context/note_bulk_context.go
git commit -m "feat(notes): add bulk context methods for notes"
```

---

### Task 3: Add Bulk Note API Handlers

**Files:**
- Modify: `server/api_handlers/note_api_handlers.go`

**Step 1: Add 5 bulk handler functions**

Append to `note_api_handlers.go`. Follow the exact pattern from `group_api_handlers.go` lines 130-215, but use note interfaces and redirect to `/notes`.

```go
func GetAddTagsToNotesHandler(ctx interfaces.BulkNoteTagEditor) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		var editor = query_models.BulkEditQuery{}
		var err error

		if err = tryFillStructValuesFromRequest(&editor, request); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		err = ctx.BulkAddTagsToNotes(&editor)

		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		http_utils.RedirectIfHTMLAccepted(writer, request, "/notes")
	}
}

func GetRemoveTagsFromNotesHandler(ctx interfaces.BulkNoteTagEditor) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		var editor = query_models.BulkEditQuery{}
		var err error

		if err = tryFillStructValuesFromRequest(&editor, request); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		err = ctx.BulkRemoveTagsFromNotes(&editor)

		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		http_utils.RedirectIfHTMLAccepted(writer, request, "/notes")
	}
}

func GetAddGroupsToNotesHandler(ctx interfaces.BulkNoteGroupEditor) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		var editor = query_models.BulkEditQuery{}
		var err error

		if err = tryFillStructValuesFromRequest(&editor, request); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		err = ctx.BulkAddGroupsToNotes(&editor)

		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		http_utils.RedirectIfHTMLAccepted(writer, request, "/notes")
	}
}

func GetAddMetaToNotesHandler(ctx interfaces.BulkNoteMetaEditor) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		var editor = query_models.BulkEditMetaQuery{}
		var err error

		if err = tryFillStructValuesFromRequest(&editor, request); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		err = ctx.BulkAddMetaToNotes(&editor)

		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		http_utils.RedirectIfHTMLAccepted(writer, request, "/notes")
	}
}

func GetBulkDeleteNotesHandler(ctx interfaces.BulkNoteDeleter) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		effectiveCtx := withRequestContext(ctx, request).(interfaces.BulkNoteDeleter)

		var editor = query_models.BulkQuery{}
		var err error

		if err = tryFillStructValuesFromRequest(&editor, request); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		err = effectiveCtx.BulkDeleteNotes(&editor)

		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		http_utils.RedirectIfHTMLAccepted(writer, request, "/notes")
	}
}
```

Note: Only `GetBulkDeleteNotesHandler` uses `withRequestContext` (for audit logging), matching the pattern from `GetBulkDeleteGroupsHandler` (line 172-194 of group_api_handlers.go). The tag/meta handlers don't use it, matching the group pattern.

**Step 2: Verify compilation**

Run: `go build --tags 'json1 fts5'`
Expected: SUCCESS

**Step 3: Commit**

```bash
git add server/api_handlers/note_api_handlers.go
git commit -m "feat(notes): add bulk API handlers for notes"
```

---

### Task 4: Add Routes

**Files:**
- Modify: `server/routes.go` (after line 193, before the block API routes at line 195)
- Modify: `server/routes_openapi.go` (in `registerNoteRoutes`, before the closing `}` at line 503)

**Step 1: Add routes to routes.go**

Insert after line 193 (`router.Methods(http.MethodDelete).Path("/v1/note/share")...`):

```go
	// Note bulk operations
	router.Methods(http.MethodPost).Path("/v1/notes/addTags").HandlerFunc(api_handlers.GetAddTagsToNotesHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/notes/removeTags").HandlerFunc(api_handlers.GetRemoveTagsFromNotesHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/notes/addGroups").HandlerFunc(api_handlers.GetAddGroupsToNotesHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/notes/addMeta").HandlerFunc(api_handlers.GetAddMetaToNotesHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/notes/delete").HandlerFunc(api_handlers.GetBulkDeleteNotesHandler(appContext))
```

**Step 2: Add OpenAPI registrations to routes_openapi.go**

Insert before the closing `}` of `registerNoteRoutes` (line 503). The function already defines `noteType`, `noteQueryType`, `noteEditorType`. We need to add bulk query types:

```go
	// Bulk note operations
	bulkQueryType := reflect.TypeOf(query_models.BulkQuery{})
	bulkEditQueryType := reflect.TypeOf(query_models.BulkEditQuery{})
	bulkEditMetaQueryType := reflect.TypeOf(query_models.BulkEditMetaQuery{})

	r.Register(openapi.RouteInfo{
		Method:              http.MethodPost,
		Path:                "/v1/notes/addTags",
		OperationID:         "bulkAddTagsToNotes",
		Summary:             "Bulk add tags to notes",
		Tags:                []string{"notes"},
		RequestType:         bulkEditQueryType,
		RequestContentTypes: []openapi.ContentType{openapi.ContentTypeJSON, openapi.ContentTypeForm},
	})

	r.Register(openapi.RouteInfo{
		Method:              http.MethodPost,
		Path:                "/v1/notes/removeTags",
		OperationID:         "bulkRemoveTagsFromNotes",
		Summary:             "Bulk remove tags from notes",
		Tags:                []string{"notes"},
		RequestType:         bulkEditQueryType,
		RequestContentTypes: []openapi.ContentType{openapi.ContentTypeJSON, openapi.ContentTypeForm},
	})

	r.Register(openapi.RouteInfo{
		Method:              http.MethodPost,
		Path:                "/v1/notes/addGroups",
		OperationID:         "bulkAddGroupsToNotes",
		Summary:             "Bulk add groups to notes",
		Tags:                []string{"notes"},
		RequestType:         bulkEditQueryType,
		RequestContentTypes: []openapi.ContentType{openapi.ContentTypeJSON, openapi.ContentTypeForm},
	})

	r.Register(openapi.RouteInfo{
		Method:              http.MethodPost,
		Path:                "/v1/notes/addMeta",
		OperationID:         "bulkAddMetaToNotes",
		Summary:             "Bulk add/merge meta to notes",
		Tags:                []string{"notes"},
		RequestType:         bulkEditMetaQueryType,
		RequestContentTypes: []openapi.ContentType{openapi.ContentTypeJSON, openapi.ContentTypeForm},
	})

	r.Register(openapi.RouteInfo{
		Method:              http.MethodPost,
		Path:                "/v1/notes/delete",
		OperationID:         "bulkDeleteNotes",
		Summary:             "Bulk delete notes",
		Tags:                []string{"notes"},
		RequestType:         bulkQueryType,
		RequestContentTypes: []openapi.ContentType{openapi.ContentTypeJSON, openapi.ContentTypeForm},
	})
```

**Step 3: Verify compilation**

Run: `go build --tags 'json1 fts5'`
Expected: SUCCESS

**Step 4: Commit**

```bash
git add server/routes.go server/routes_openapi.go
git commit -m "feat(notes): add bulk note routes and OpenAPI registrations"
```

---

### Task 5: Add Bulk Editor Note Template

**Files:**
- Create: `templates/partials/bulkEditorNote.tpl`

**Step 1: Create bulkEditorNote.tpl**

Model after `templates/partials/bulkEditorGroup.tpl`, adding the "Add Groups" form (from `bulkEditorResource.tpl` line 28-34):

```html
<div class="pb-3" x-data x-show="[...$store.bulkSelection.selectedIds].length === 0" x-collapse>
    {% include "/partials/form/formParts/connected/selectAllButton.tpl" %}
</div>
<div x-cloak class="sticky top-0 z-50 flex pl-4 pb-2 lg:gap-4 gap-1 flex-wrap bulk-editors items-center" x-show="[...$store.bulkSelection.selectedIds].length > 0" x-collapse x-data="bulkSelectionForms">
    {% include "/partials/form/formParts/connected/deselectButton.tpl" %}
    {% include "/partials/form/formParts/connected/selectAllButton.tpl" %}
    <form class="px-4" method="post" :action="'/v1/notes/addTags?redirect=' + encodeURIComponent(window.location)">
        {% include "/partials/form/formParts/connected/selectedIds.tpl" %}
        <div class="flex gap-2 items-start">
            {% include "/partials/form/autocompleter.tpl" with url='/v1/tags' addUrl='/v1/tag' elName='editedId' title='Add Tag' id=getNextId("tag_autocompleter") %}
            <div class="mt-7">{% include "/partials/form/searchButton.tpl" with text="Add" %}</div>
        </div>
    </form>
    <form class="px-4" method="post" :action="'/v1/notes/removeTags?redirect=' + encodeURIComponent(window.location)">
        {% include "/partials/form/formParts/connected/selectedIds.tpl" %}
        <div class="flex gap-2 items-start">
            {% include "/partials/form/autocompleter.tpl" with url='/v1/tags' elName='editedId' title='Remove Tag' id=getNextId("tag_autocompleter") %}
            <div class="mt-7">{% include "/partials/form/searchButton.tpl" with text="Remove" %}</div>
        </div>
    </form>
    <form class="px-4" method="post" :action="'/v1/notes/addMeta?redirect=' + encodeURIComponent(window.location)">
        {% include "/partials/form/formParts/connected/selectedIds.tpl" %}
        <div class="flex gap-2 items-start">
            {% include "/partials/form/freeFields.tpl" with name="Meta" url='/v1/notes/meta/keys' jsonOutput="true" id=getNextId("freeField") %}
            <div class="mt-7">{% include "/partials/form/searchButton.tpl" with text="Add" %}</div>
        </div>
    </form>
    <form class="px-4" method="post" :action="'/v1/notes/addGroups?redirect=' + encodeURIComponent(window.location)">
        {% include "/partials/form/formParts/connected/selectedIds.tpl" %}
        <div class="flex gap-2 items-start">
            {% include "/partials/form/autocompleter.tpl" with url='/v1/groups' elName='editedId' title='Add Groups' id=getNextId("autocompleter") extraInfo="Category" %}
            <div class="mt-7">{% include "/partials/form/searchButton.tpl" with text="Add" %}</div>
        </div>
    </form>
    <form
            class="px-4 no-ajax"
            method="post"
            :action="'/v1/notes/delete?redirect=' + encodeURIComponent(window.location)"
            x-data="confirmAction('Are you sure you want to delete the selected notes?')"
            x-bind="events"
    >
        {% include "/partials/form/formParts/connected/selectedIds.tpl" %}
        <div class="flex flex-col">
            <span class="block text-sm font-medium text-gray-700 mt-3">Delete Selected</span>
            {% include "/partials/form/searchButton.tpl" with text="Delete" danger=true %}
        </div>
    </form>
    {% include "partials/pluginActionsBulk.tpl" with entityType="note" %}
</div>
```

**Step 2: Commit**

```bash
git add templates/partials/bulkEditorNote.tpl
git commit -m "feat(notes): add bulk editor note template"
```

---

### Task 6: Update Note Partial and List Template

**Files:**
- Modify: `templates/partials/note.tpl` (line 1)
- Modify: `templates/listNotes.tpl` (lines 1-11)

**Step 1: Add selectable support to note.tpl**

Change line 1 from:
```html
<article class="card note-card" x-data='{ "entity": {{ entity|json }} }'>
```
to:
```html
<article class="card note-card{% if selectable %} card--selectable{% endif %}" {% if selectable %}x-data="selectableItem({ itemId: {{ entity.ID }} })"{% else %}x-data='{ "entity": {{ entity|json }} }'{% endif %}>
```

After the opening `<article>` tag (new line 2), add:
```html
    {% if selectable %}
    <input type="checkbox" :checked="selected() ? 'checked' : null" x-bind="events" aria-label="Select {{ entity.Name }}" class="card-checkbox focus:ring-indigo-500 h-6 w-6 text-indigo-600 border-gray-300 rounded">
    {% endif %}
```

**Step 2: Update listNotes.tpl**

The template currently extends `gallery.tpl`. Change it to extend `base.tpl` directly (matching `listGroups.tpl` pattern) to support the `prebody` block for the bulk editor:

```html
{% extends "/layouts/base.tpl" %}

{% block prebody %}
    {% include "/partials/bulkEditorNote.tpl" %}
{% endblock %}

{% block body %}
    {% plugin_slot "note_list_before" %}
    <div style="display:contents"{% if owners && owners|length == 1 %} data-paste-context='{"type":"group","id":{{ owners.0.ID }},"name":"{{ owners.0.Name|escapejs }}"}'{% endif %}>
    {% for entity in notes %}
        {% include "/partials/note.tpl" with selectable=true %}
    {% endfor %}
    </div>
    {% plugin_slot "note_list_after" %}
{% endblock %}

{% block sidebar %}
    <form class="flex gap-2 items-start flex-col" aria-label="Filter notes">
        <div class="tags mt-3 mb-2 gap-1 flex flex-wrap" style="margin-left: -0.5rem">
            {% for tag in popularTags %}
            <a class="no-underline" href='{{ withQuery("tags", stringId(tag.Id), true) }}'>
                {% include "partials/tag.tpl" with name=tag.Name count=tag.Count active=hasQuery("tags", stringId(tag.Id)) %}
            </a>
            {% endfor %}
        </div>
        {% include "/partials/sideTitle.tpl" with title="Sort" %}
        {% include "/partials/form/multiSortInput.tpl" with name='SortBy' values=sortValues %}
        {% include "/partials/sideTitle.tpl" with title="Filter" %}
        {% include "/partials/form/textInput.tpl" with name='Name' label='Name' value=queryValues.Name.0 %}
        {% include "/partials/form/textInput.tpl" with name='Description' label='Text' value=queryValues.Description.0 %}
        {% include "/partials/form/autocompleter.tpl" with url='/v1/tags' elName='tags' title='Tags' selectedItems=tags id=getNextId("autocompleter") %}
        {% include "/partials/form/autocompleter.tpl" with url='/v1/groups' elName='groups' title='Groups' selectedItems=groups id=getNextId("autocompleter") extraInfo="Category" %}
        {% include "/partials/form/autocompleter.tpl" with url='/v1/groups' max=1 elName='ownerId' title='Owner' selectedItems=owners id=getNextId("autocompleter") extraInfo="Category" %}
        {% include "/partials/form/autocompleter.tpl" with url='/v1/note/noteTypes' elName='NoteTypeId' title='Note Type' selectedItems=noteTypes max=1 id=getNextId("autocompleter") %}
        {% include "/partials/form/freeFields.tpl" with name="MetaQuery" url='/v1/notes/meta/keys' fields=parsedQuery.MetaQuery id=getNextId("freeField") %}
        {% include "/partials/form/dateInput.tpl" with name='StartDateBefore' label='Start Date Before' value=queryValues.StartDateBefore.0 %}
        {% include "/partials/form/dateInput.tpl" with name='StartDateAfter' label='Start Date After' value=queryValues.StartDateAfter.0 %}
        {% include "/partials/form/dateInput.tpl" with name='EndDateBefore' label='End Date Before' value=queryValues.EndDateBefore.0 %}
        {% include "/partials/form/dateInput.tpl" with name='EndDateAfter' label='End Date After' value=queryValues.EndDateAfter.0 %}
        {% include "/partials/form/checkboxInput.tpl" with name='Shared' label='Shared Only' value=queryValues.Shared.0 id=getNextId("Shared") %}
        {% include "/partials/form/searchButton.tpl" %}
    </form>
{% endblock %}
```

**Step 3: Build CSS + JS to check for template issues**

Run: `npm run build`
Expected: SUCCESS

**Step 4: Commit**

```bash
git add templates/partials/note.tpl templates/listNotes.tpl
git commit -m "feat(notes): add selectable checkboxes and bulk editor to note list"
```

---

### Task 7: Add E2E Tests

**Files:**
- Modify: `e2e/helpers/api-client.ts` (add note bulk API methods)
- Modify: `e2e/pages/NotePage.ts` (add selectNoteCheckbox method)
- Modify: `e2e/tests/10-bulk-operations.spec.ts` (add note bulk operation tests)

**Step 1: Add API client methods for note bulk operations**

Append to `e2e/helpers/api-client.ts`, after the group bulk methods:

```typescript
  async addTagsToNotes(noteIds: number[], tagIds: number[]): Promise<void> {
    const formData = new URLSearchParams();
    noteIds.forEach(id => formData.append('ID', id.toString()));
    tagIds.forEach(id => formData.append('EditedId', id.toString()));

    return this.postVoidRetry(`${this.baseUrl}/v1/notes/addTags`, {
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      data: formData.toString(),
    });
  }

  async removeTagsFromNotes(noteIds: number[], tagIds: number[]): Promise<void> {
    const formData = new URLSearchParams();
    noteIds.forEach(id => formData.append('ID', id.toString()));
    tagIds.forEach(id => formData.append('EditedId', id.toString()));

    return this.postVoidRetry(`${this.baseUrl}/v1/notes/removeTags`, {
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      data: formData.toString(),
    });
  }

  async addGroupsToNotes(noteIds: number[], groupIds: number[]): Promise<void> {
    const formData = new URLSearchParams();
    noteIds.forEach(id => formData.append('ID', id.toString()));
    groupIds.forEach(id => formData.append('EditedId', id.toString()));

    return this.postVoidRetry(`${this.baseUrl}/v1/notes/addGroups`, {
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      data: formData.toString(),
    });
  }

  async bulkDeleteNotes(noteIds: number[]): Promise<void> {
    const formData = new URLSearchParams();
    noteIds.forEach(id => formData.append('ID', id.toString()));

    return this.postVoidRetry(`${this.baseUrl}/v1/notes/delete`, {
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      data: formData.toString(),
    });
  }
```

**Step 2: Add selectNoteCheckbox to NotePage**

```typescript
  async selectNoteCheckbox(noteId: number) {
    await this.page.locator(`[x-data*="itemId: ${noteId}"] input[type="checkbox"]`).check();
  }
```

**Step 3: Add note bulk operation tests**

Add a new `test.describe('Bulk Operations on Notes', ...)` block to `e2e/tests/10-bulk-operations.spec.ts`. Follow the exact structure of the existing `Bulk Operations on Groups` test block:

```typescript
test.describe('Bulk Operations on Notes', () => {
  let categoryId: number;
  let groupId: number;
  let noteIds: number[] = [];
  let tagId: number;
  let secondTagId: number;
  let testRunId: string;

  test.beforeAll(async ({ apiClient }) => {
    testRunId = `${Date.now()}-${Math.random().toString(36).substring(2, 8)}`;

    const category = await apiClient.createCategory(`Note Bulk Category ${testRunId}`, 'Category for note bulk tests');
    categoryId = category.ID;

    const group = await apiClient.createGroup({
      name: `Note Bulk Group ${testRunId}`,
      categoryId: categoryId,
    });
    groupId = group.ID;

    const tag = await apiClient.createTag(`Note Bulk Tag 1 ${testRunId}`, 'First note bulk tag');
    tagId = tag.ID;

    const secondTag = await apiClient.createTag(`Note Bulk Tag 2 ${testRunId}`, 'Second note bulk tag');
    secondTagId = secondTag.ID;

    noteIds = [];
    for (let i = 1; i <= 5; i++) {
      const note = await apiClient.createNote({
        name: `Bulk Test Note ${i} ${testRunId}`,
        description: `Note ${i} for bulk testing`,
      });
      noteIds.push(note.ID);
    }
  });

  test('should select multiple notes', async ({ notePage, page }) => {
    await notePage.gotoList();

    for (let i = 0; i < 3; i++) {
      await notePage.selectNoteCheckbox(noteIds[i]);
    }

    await expect(page.locator('button:has-text("Deselect All"), button:has-text("Deselect")')).toBeVisible();
  });

  test('should bulk add tags to notes', async ({ notePage, apiClient, page }) => {
    await apiClient.addTagsToNotes([noteIds[0], noteIds[1]], [tagId]);

    await notePage.gotoDisplay(noteIds[0]);
    await expect(page.locator(`a:has-text("Note Bulk Tag 1 ${testRunId}")`).first()).toBeVisible();

    await notePage.gotoDisplay(noteIds[1]);
    await expect(page.locator(`a:has-text("Note Bulk Tag 1 ${testRunId}")`).first()).toBeVisible();
  });

  test('should bulk remove tags from notes', async ({ notePage, apiClient, page }) => {
    await apiClient.addTagsToNotes([noteIds[0], noteIds[1]], [secondTagId]);
    await apiClient.removeTagsFromNotes([noteIds[0], noteIds[1]], [secondTagId]);

    await notePage.gotoDisplay(noteIds[0]);
    await expect(page.locator(`a:has-text("Note Bulk Tag 2 ${testRunId}")`)).not.toBeVisible();
  });

  test('should bulk add groups to notes', async ({ notePage, apiClient, page }) => {
    await apiClient.addGroupsToNotes([noteIds[0], noteIds[1]], [groupId]);

    await notePage.gotoDisplay(noteIds[0]);
    await expect(page.locator(`a:has-text("Note Bulk Group ${testRunId}")`).first()).toBeVisible();
  });

  test('should bulk delete notes', async ({ notePage, apiClient }) => {
    await apiClient.bulkDeleteNotes([noteIds[3], noteIds[4]]);

    await notePage.verifyNoteNotInList(`Bulk Test Note 4 ${testRunId}`);
    await notePage.verifyNoteNotInList(`Bulk Test Note 5 ${testRunId}`);

    noteIds = noteIds.slice(0, 3);
  });

  test.afterAll(async ({ apiClient }) => {
    for (const noteId of noteIds) {
      try {
        await apiClient.deleteNote(noteId);
      } catch (error) {
        console.warn(`Cleanup: Failed to delete note ${noteId}:`, error);
      }
    }
    if (tagId) await apiClient.deleteTag(tagId);
    if (secondTagId) await apiClient.deleteTag(secondTagId);
    if (groupId) await apiClient.deleteGroup(groupId);
    if (categoryId) await apiClient.deleteCategory(categoryId);
  });
});
```

Note: Check `e2e/fixtures/base.fixture.ts` to confirm `notePage` is already a fixture. If not, add it.

**Step 4: Run E2E tests**

Run: `cd e2e && npm run test:with-server`
Expected: All tests pass (existing and new).

**Step 5: Commit**

```bash
git add e2e/helpers/api-client.ts e2e/pages/NotePage.ts e2e/tests/10-bulk-operations.spec.ts
git commit -m "test(notes): add E2E tests for note bulk operations"
```

---

### Task 8: Run Go Unit Tests and Final Verification

**Step 1: Run Go tests**

Run: `go test ./... --tags 'json1 fts5'`
Expected: All pass

**Step 2: Run full E2E suite**

Run: `cd e2e && npm run test:with-server`
Expected: All pass

**Step 3: Manual smoke test**

Run: `npm run build && ./mahresources -ephemeral -bind-address=:8181`

1. Open http://localhost:8181/notes
2. Create a few notes if none exist
3. Verify checkboxes appear next to notes
4. Select 2+ notes — verify bulk editor bar appears with: Add Tag, Remove Tag, Add Meta, Add Groups, Delete
5. Test adding a tag to selected notes
6. Test deleting selected notes

**Step 4: Run accessibility tests**

Run: `cd e2e && npm run test:with-server:a11y`
Expected: All pass (checkbox has `aria-label`)
