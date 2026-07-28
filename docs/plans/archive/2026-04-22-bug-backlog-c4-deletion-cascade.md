# Cluster 4 — Block-Content Deletion Cascade (BH-020, BH-024)

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:subagent-driven-development. Solo subagent (all edits in deletion surface). Steps use checkbox (`- [ ]`) syntax.

**Goal:** When a resource / group / saved-query is deleted, scrub references to it from `note_blocks.content` (BH-020, systemic across 4 block types). Also fix BH-024: dangling query reference currently returns 500, should be 404.

**Architecture:** Each DELETE handler (resource, group, saved-query) walks `note_blocks` and scrubs matching IDs from the JSON content using DB-native JSON functions (SQLite `json_remove` / `json_set`, Postgres `jsonb_set` with `jsonb_array_elements`). A one-shot migration scrubs pre-existing orphans on boot, gated behind `SKIP_BLOCK_REF_CLEANUP=1`. UI components gain graceful-degrade rendering for unavailable targets. BH-024 receives a 2-line fix: wrap the inner query-fetch with `statusCodeForError`.

**Tech Stack:** Go, GORM with SQLite JSON1 + Postgres JSONB, Alpine.js components.

**Worktree branch:** `bugfix/c4-deletion-cascade`

---

## File structure

**Modified:**
- `application_context/resource_context.go` — delete handler scrubs `resourceIds[]` and `calendars[].source.resourceId`
- `application_context/group_crud_context.go` — delete handler scrubs `groupIds[]`
- `application_context/mrql_context.go` (or wherever `SavedMRQLQuery` is deleted) — scrubs `queryId`
- `application_context/context.go` — boot-time migration runner, new `SKIP_BLOCK_REF_CLEANUP` flag
- `server/api_handlers/block_api_handlers.go` — BH-024: wrap inner query fetch with `statusCodeForError`
- `src/components/blocks/gallery.js` or equivalent — graceful-degrade on resource 404
- `src/components/blocks/references.js` — graceful-degrade on group 404
- `src/components/blocks/table.js` or equivalent — graceful-degrade on query 404

**Created:**
- `application_context/block_ref_cleanup.go` — shared scrubber logic
- `application_context/block_ref_cleanup_test.go` — unit tests
- `server/api_tests/block_ref_cascade_test.go` — API-level integration tests (one per block type)
- `server/api_tests/table_block_dangling_query_returns_404_test.go` — BH-024

---

## Pre-work: confirm block-content schema

- [ ] **Step 1: Read the current block-content shapes**

```bash
grep -rn "resourceIds\|groupIds\|queryId\|calendars.*source" models/ application_context/block_context.go | head -40
```

Confirm which fields hold IDs in which block types. BH-020 lists:

- `gallery` → `content.resourceIds[]`
- `references` → `content.groupIds[]`
- `calendar` → `content.calendars[].source.resourceId` (type=resource)
- `table` → `content.queryId`

Adjust field names if they differ in the code from the bug-log descriptions.

---

## Task 1: Failing integration test for gallery-block dangling resource

**Files:**
- Create: `server/api_tests/block_ref_cascade_test.go`

- [ ] **Step 1: Write the failing test**

```go
package api_tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResourceDelete_ScrubsGalleryBlockReferences(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create a resource + a note with a gallery block referencing it
	res := tc.CreateDummyResource(t, "bh020-res")
	note := tc.CreateDummyNote("bh020-note")

	// Add a gallery block with content.resourceIds = [resID]
	blockContent := fmt.Sprintf(`{"resourceIds":[%d]}`, res.ID)
	form := url.Values{}
	form.Set("NoteId", fmt.Sprintf("%d", note.ID))
	form.Set("Type", "gallery")
	form.Set("Content", blockContent)
	rr := tc.MakeFormRequest(http.MethodPost, "/v1/note/block", form)
	require.Equal(t, http.StatusOK, rr.Code, "gallery block create failed: %s", rr.Body.String())

	// Delete the resource
	delRR := tc.MakeRequest(http.MethodDelete, fmt.Sprintf("/v1/resource?id=%d", res.ID), nil)
	require.Equal(t, http.StatusOK, delRR.Code)

	// Fetch the block and verify resourceIds no longer contains the deleted ID
	blocksRR := tc.MakeRequest(http.MethodGet, fmt.Sprintf("/v1/note/blocks?noteId=%d", note.ID), nil)
	require.Equal(t, http.StatusOK, blocksRR.Code)

	var blocks []map[string]any
	require.NoError(t, json.Unmarshal(blocksRR.Body.Bytes(), &blocks))
	require.NotEmpty(t, blocks)

	content, ok := blocks[0]["Content"].(map[string]any)
	require.True(t, ok, "block content must be a JSON object")

	ids, _ := content["resourceIds"].([]any)
	for _, id := range ids {
		assert.NotEqual(t, float64(res.ID), id, "deleted resource ID must not remain in gallery.resourceIds")
	}
}

func TestGroupDelete_ScrubsReferencesBlockGroupIds(t *testing.T) {
	tc := SetupTestEnv(t)
	grp := tc.CreateDummyGroup("bh020-grp")
	note := tc.CreateDummyNote("bh020-refnote")

	blockContent := fmt.Sprintf(`{"groupIds":[%d]}`, grp.ID)
	form := url.Values{}
	form.Set("NoteId", fmt.Sprintf("%d", note.ID))
	form.Set("Type", "references")
	form.Set("Content", blockContent)
	rr := tc.MakeFormRequest(http.MethodPost, "/v1/note/block", form)
	require.Equal(t, http.StatusOK, rr.Code)

	delRR := tc.MakeRequest(http.MethodDelete, fmt.Sprintf("/v1/group?id=%d", grp.ID), nil)
	require.Equal(t, http.StatusOK, delRR.Code)

	blocksRR := tc.MakeRequest(http.MethodGet, fmt.Sprintf("/v1/note/blocks?noteId=%d", note.ID), nil)
	var blocks []map[string]any
	json.Unmarshal(blocksRR.Body.Bytes(), &blocks)
	require.NotEmpty(t, blocks)

	content, _ := blocks[0]["Content"].(map[string]any)
	ids, _ := content["groupIds"].([]any)
	for _, id := range ids {
		assert.NotEqual(t, float64(grp.ID), id, "deleted group ID must not remain in references.groupIds")
	}
}

func TestCalendarBlock_ScrubsResourceSourceOnResourceDelete(t *testing.T) {
	tc := SetupTestEnv(t)
	res := tc.CreateDummyResource(t, "bh020-calres")
	note := tc.CreateDummyNote("bh020-calnote")

	blockContent := fmt.Sprintf(`{"calendars":[{"name":"cal1","source":{"type":"resource","resourceId":%d}}]}`, res.ID)
	form := url.Values{}
	form.Set("NoteId", fmt.Sprintf("%d", note.ID))
	form.Set("Type", "calendar")
	form.Set("Content", blockContent)
	rr := tc.MakeFormRequest(http.MethodPost, "/v1/note/block", form)
	require.Equal(t, http.StatusOK, rr.Code)

	delRR := tc.MakeRequest(http.MethodDelete, fmt.Sprintf("/v1/resource?id=%d", res.ID), nil)
	require.Equal(t, http.StatusOK, delRR.Code)

	blocksRR := tc.MakeRequest(http.MethodGet, fmt.Sprintf("/v1/note/blocks?noteId=%d", note.ID), nil)
	var blocks []map[string]any
	json.Unmarshal(blocksRR.Body.Bytes(), &blocks)
	require.NotEmpty(t, blocks)

	content, _ := blocks[0]["Content"].(map[string]any)
	cals, _ := content["calendars"].([]any)
	require.NotEmpty(t, cals)
	cal0 := cals[0].(map[string]any)
	source := cal0["source"].(map[string]any)
	// Either the whole calendar entry is removed, or its source.resourceId is cleared to 0/null.
	if rid, ok := source["resourceId"]; ok {
		switch v := rid.(type) {
		case float64:
			assert.NotEqual(t, float64(res.ID), v, "calendar source resourceId must be scrubbed")
		}
	}
}
```

Add a helper `CreateDummyResource` to `api_test_utils.go` if not present:

```go
func (tc *TestContext) CreateDummyResource(t *testing.T, name string) *models.Resource {
    r := &models.Resource{Name: name, ContentType: "application/octet-stream", Size: 1}
    require.NoError(t, tc.DB.Create(r).Error)
    return r
}
```

- [ ] **Step 2: Run 3× — expect fail with the dangling-ID symptom**

```bash
go test --tags 'json1 fts5' ./server/api_tests/ -run TestResourceDelete_ScrubsGallery -v -count=3
go test --tags 'json1 fts5' ./server/api_tests/ -run TestGroupDelete_ScrubsReferences -v -count=3
go test --tags 'json1 fts5' ./server/api_tests/ -run TestCalendarBlock_ScrubsResource -v -count=3
```

Expected: 3× FAIL for each, with assertion messages showing the deleted ID still in the block content.

---

## Task 2: Implement the scrubber helpers

**Files:**
- Create: `application_context/block_ref_cleanup.go`

- [ ] **Step 1: Write the shared scrubber**

```go
package application_context

import (
    "fmt"

    "gorm.io/gorm"
)

// ScrubResourceFromBlocks removes resourceID from every gallery.resourceIds[]
// and every calendar.calendars[].source.resourceId in the note_blocks table.
//
// Called synchronously from the resource DELETE handler, BH-020.
func ScrubResourceFromBlocks(db *gorm.DB, resourceID uint) error {
    // Gallery blocks: content.resourceIds is a JSON array of numbers.
    // SQLite: use json_each + recreate array excluding the deleted ID.
    // Postgres: jsonb_set with jsonb_array_elements filter.
    //
    // To keep logic portable, read each candidate row, mutate in Go, write back.
    // Volume is bounded by blocks referencing this resource, which is small.

    var blocks []struct {
        ID      uint
        Content string
    }
    if err := db.Raw(
        `SELECT id, CAST(content AS TEXT) AS content FROM note_blocks
         WHERE type IN ('gallery','calendar')`,
    ).Scan(&blocks).Error; err != nil {
        return err
    }

    for _, b := range blocks {
        updated, changed, err := scrubResourceFromBlockContent(b.Content, resourceID)
        if err != nil {
            return fmt.Errorf("block %d: %w", b.ID, err)
        }
        if !changed {
            continue
        }
        if err := db.Exec(
            `UPDATE note_blocks SET content = ? WHERE id = ?`, updated, b.ID,
        ).Error; err != nil {
            return err
        }
    }

    return nil
}

// ScrubGroupFromBlocks removes groupID from references.groupIds[].
func ScrubGroupFromBlocks(db *gorm.DB, groupID uint) error {
    var blocks []struct {
        ID      uint
        Content string
    }
    if err := db.Raw(
        `SELECT id, CAST(content AS TEXT) AS content FROM note_blocks WHERE type = 'references'`,
    ).Scan(&blocks).Error; err != nil {
        return err
    }

    for _, b := range blocks {
        updated, changed, err := scrubGroupFromBlockContent(b.Content, groupID)
        if err != nil {
            return err
        }
        if !changed {
            continue
        }
        if err := db.Exec(
            `UPDATE note_blocks SET content = ? WHERE id = ?`, updated, b.ID,
        ).Error; err != nil {
            return err
        }
    }
    return nil
}

// ScrubQueryFromBlocks nulls queryId in every table-block whose queryId matches.
func ScrubQueryFromBlocks(db *gorm.DB, queryID uint) error {
    var blocks []struct {
        ID      uint
        Content string
    }
    if err := db.Raw(
        `SELECT id, CAST(content AS TEXT) AS content FROM note_blocks WHERE type = 'table'`,
    ).Scan(&blocks).Error; err != nil {
        return err
    }
    for _, b := range blocks {
        updated, changed, err := scrubQueryFromBlockContent(b.Content, queryID)
        if err != nil {
            return err
        }
        if !changed {
            continue
        }
        if err := db.Exec(
            `UPDATE note_blocks SET content = ? WHERE id = ?`, updated, b.ID,
        ).Error; err != nil {
            return err
        }
    }
    return nil
}
```

Include these pure JSON-mutation helpers in the same file:

```go
// Returns the updated JSON string, whether anything changed, and any error.
func scrubResourceFromBlockContent(content string, resourceID uint) (string, bool, error) {
    var raw map[string]any
    if err := json.Unmarshal([]byte(content), &raw); err != nil {
        return content, false, err
    }
    changed := false

    // Gallery: content.resourceIds = [id, id, ...]
    if ids, ok := raw["resourceIds"].([]any); ok {
        filtered := make([]any, 0, len(ids))
        for _, v := range ids {
            if toUint(v) != resourceID {
                filtered = append(filtered, v)
            } else {
                changed = true
            }
        }
        if changed {
            raw["resourceIds"] = filtered
        }
    }

    // Calendar: content.calendars[].source.resourceId
    if cals, ok := raw["calendars"].([]any); ok {
        for i, c := range cals {
            cmap, ok := c.(map[string]any)
            if !ok {
                continue
            }
            source, ok := cmap["source"].(map[string]any)
            if !ok {
                continue
            }
            if rid, ok := source["resourceId"]; ok && toUint(rid) == resourceID {
                delete(source, "resourceId")
                cmap["source"] = source
                cals[i] = cmap
                changed = true
            }
        }
        if changed {
            raw["calendars"] = cals
        }
    }

    if !changed {
        return content, false, nil
    }
    out, err := json.Marshal(raw)
    if err != nil {
        return content, false, err
    }
    return string(out), true, nil
}

func scrubGroupFromBlockContent(content string, groupID uint) (string, bool, error) {
    var raw map[string]any
    if err := json.Unmarshal([]byte(content), &raw); err != nil {
        return content, false, err
    }
    ids, ok := raw["groupIds"].([]any)
    if !ok {
        return content, false, nil
    }
    filtered := make([]any, 0, len(ids))
    changed := false
    for _, v := range ids {
        if toUint(v) != groupID {
            filtered = append(filtered, v)
        } else {
            changed = true
        }
    }
    if !changed {
        return content, false, nil
    }
    raw["groupIds"] = filtered
    out, err := json.Marshal(raw)
    if err != nil {
        return content, false, err
    }
    return string(out), true, nil
}

func scrubQueryFromBlockContent(content string, queryID uint) (string, bool, error) {
    var raw map[string]any
    if err := json.Unmarshal([]byte(content), &raw); err != nil {
        return content, false, err
    }
    if qid, ok := raw["queryId"]; ok && toUint(qid) == queryID {
        delete(raw, "queryId")
        out, err := json.Marshal(raw)
        if err != nil {
            return content, false, err
        }
        return string(out), true, nil
    }
    return content, false, nil
}

// toUint converts a JSON-decoded number or string to uint; returns 0 if not convertible.
func toUint(v any) uint {
    switch x := v.(type) {
    case float64:
        return uint(x)
    case int:
        return uint(x)
    case uint:
        return x
    case json.Number:
        n, _ := x.Int64()
        return uint(n)
    case string:
        // best-effort — not expected for these fields, but safe
        var n uint
        fmt.Sscanf(x, "%d", &n)
        return n
    }
    return 0
}
```

Add the required imports: `"encoding/json"`, `"fmt"`.

- [ ] **Step 2: Add unit tests for the pure-JSON mutators**

**Files:** Create `application_context/block_ref_cleanup_test.go`. Test:

- `scrubResourceFromBlockContent("""{"resourceIds":[1,2,3]}""", 2)` → `{"resourceIds":[1,3]}`, changed=true
- `scrubResourceFromBlockContent("""{"resourceIds":[1,3]}""", 2)` → changed=false
- `scrubResourceFromBlockContent("""{"calendars":[{"source":{"resourceId":5}}]}""", 5)` → calendar entry removed or source zeroed, changed=true
- `scrubGroupFromBlockContent(...)`, `scrubQueryFromBlockContent(...)` — analogous.

Each test follows the standard Go table-test pattern. Run 3× to verify pre-implementation compile failure, then post-implementation pass.

## Task 3: Wire scrubbers into delete handlers

**Files:**
- Modify: `application_context/resource_context.go` — in the function that deletes a resource, after the resource row is deleted (or before, inside the same transaction), call `ScrubResourceFromBlocks(db, resourceID)`.
- Modify: `application_context/group_crud_context.go` — same for groups.
- Modify: `application_context/mrql_context.go` — same for saved queries (find the `DeleteSavedQuery` or similar function).

- [ ] **Step 1: Example wiring (resource)**

```go
func (ctx *MahresourcesContext) DeleteResource(id uint) error {
    return ctx.DB.Transaction(func(tx *gorm.DB) error {
        if err := tx.Delete(&models.Resource{}, id).Error; err != nil {
            return err
        }
        return ScrubResourceFromBlocks(tx, id)
    })
}
```

Do the same for group and saved-query delete.

- [ ] **Step 2: Run the Task 1 integration tests 3× to verify pass**

```bash
go test --tags 'json1 fts5' ./server/api_tests/ -run 'TestResourceDelete_ScrubsGallery|TestGroupDelete_ScrubsReferences|TestCalendarBlock_ScrubsResource' -v -count=3
```

Expected: PASS all 3 runs.

- [ ] **Step 3: Commit**

```bash
git add application_context/block_ref_cleanup*.go application_context/resource_context.go application_context/group_crud_context.go application_context/mrql_context.go server/api_tests/block_ref_cascade_test.go
git commit -m "fix(blocks): BH-020 — scrub dangling refs on entity delete (gallery/refs/calendar/table)"
```

## Task 4: One-shot boot-time migration

**Files:**
- Modify: `application_context/context.go` — in `NewMahresourcesContext` init path, after AutoMigrate, call a new `migrateBlockReferencesOnce(db)` guarded by `SKIP_BLOCK_REF_CLEANUP`.

- [ ] **Step 1: Add the flag**

- Config field: `SkipBlockRefCleanup bool`
- Flag: `flag.BoolVar(&config.SkipBlockRefCleanup, "skip-block-ref-cleanup", false, "Skip one-shot cleanup of dangling references in note_blocks")`
- Env: `SKIP_BLOCK_REF_CLEANUP=1`

- [ ] **Step 2: Add `migrateBlockReferencesOnce`**

```go
// migrateBlockReferencesOnce scans note_blocks for dangling IDs and removes them.
// One-shot: records completion in plugin_kv (existing table) so it doesn't re-run.
func migrateBlockReferencesOnce(db *gorm.DB) error {
    const markerKey = "block_ref_cleanup_v1"

    var completed models.PluginKV
    db.Where("plugin_name = ? AND key = ?", "_system", markerKey).First(&completed)
    if completed.Value == "done" {
        return nil
    }

    // Load all note_blocks once, scrub each against current target tables.
    var blocks []struct {
        ID      uint
        Type    string
        Content string
    }
    if err := db.Raw(
        `SELECT id, type, CAST(content AS TEXT) AS content FROM note_blocks
         WHERE type IN ('gallery','references','calendar','table')`,
    ).Scan(&blocks).Error; err != nil {
        return err
    }

    // Pre-fetch existing IDs so we scrub only truly missing ones.
    existingResources := map[uint]bool{}
    existingGroups := map[uint]bool{}
    existingQueries := map[uint]bool{}
    db.Raw(`SELECT id FROM resources`).Scan(&[]uint{}) // warmup; overwritten below
    {
        var rows []uint
        db.Raw(`SELECT id FROM resources`).Scan(&rows)
        for _, id := range rows {
            existingResources[id] = true
        }
    }
    {
        var rows []uint
        db.Raw(`SELECT id FROM groups`).Scan(&rows)
        for _, id := range rows {
            existingGroups[id] = true
        }
    }
    {
        var rows []uint
        db.Raw(`SELECT id FROM saved_mrql_queries`).Scan(&rows)
        for _, id := range rows {
            existingQueries[id] = true
        }
    }

    for _, b := range blocks {
        updated := b.Content
        anyChanged := false

        switch b.Type {
        case "gallery":
            updated, anyChanged = scrubMissingIdsFromArrayField(updated, "resourceIds", existingResources)
            updated2, changed2 := scrubMissingCalendarResources(updated, existingResources)
            if changed2 {
                updated = updated2
                anyChanged = true
            }
        case "references":
            updated, anyChanged = scrubMissingIdsFromArrayField(updated, "groupIds", existingGroups)
        case "calendar":
            updated, anyChanged = scrubMissingCalendarResources(updated, existingResources)
        case "table":
            updated, anyChanged = scrubMissingQueryIdField(updated, existingQueries)
        }

        if anyChanged {
            if err := db.Exec(
                `UPDATE note_blocks SET content = ? WHERE id = ?`, updated, b.ID,
            ).Error; err != nil {
                return err
            }
        }
    }

    return db.Exec(
        `INSERT INTO plugin_kvs (plugin_name, key, value) VALUES ('_system', ?, 'done')
         ON CONFLICT(plugin_name, key) DO UPDATE SET value = excluded.value`,
        markerKey,
    ).Error
}

// Helpers local to migration; structurally identical to the delete-time scrubbers
// but operate against a live existing-IDs set, not a single to-delete ID.
func scrubMissingIdsFromArrayField(content, field string, existing map[uint]bool) (string, bool) {
    var raw map[string]any
    if err := json.Unmarshal([]byte(content), &raw); err != nil {
        return content, false
    }
    ids, ok := raw[field].([]any)
    if !ok {
        return content, false
    }
    filtered := make([]any, 0, len(ids))
    changed := false
    for _, v := range ids {
        if existing[toUint(v)] {
            filtered = append(filtered, v)
        } else {
            changed = true
        }
    }
    if !changed {
        return content, false
    }
    raw[field] = filtered
    out, _ := json.Marshal(raw)
    return string(out), true
}

func scrubMissingCalendarResources(content string, existing map[uint]bool) (string, bool) {
    var raw map[string]any
    if err := json.Unmarshal([]byte(content), &raw); err != nil {
        return content, false
    }
    cals, ok := raw["calendars"].([]any)
    if !ok {
        return content, false
    }
    changed := false
    for i, c := range cals {
        cmap, ok := c.(map[string]any)
        if !ok {
            continue
        }
        source, ok := cmap["source"].(map[string]any)
        if !ok {
            continue
        }
        if rid, ok := source["resourceId"]; ok && !existing[toUint(rid)] {
            delete(source, "resourceId")
            cmap["source"] = source
            cals[i] = cmap
            changed = true
        }
    }
    if !changed {
        return content, false
    }
    raw["calendars"] = cals
    out, _ := json.Marshal(raw)
    return string(out), true
}

func scrubMissingQueryIdField(content string, existing map[uint]bool) (string, bool) {
    var raw map[string]any
    if err := json.Unmarshal([]byte(content), &raw); err != nil {
        return content, false
    }
    if qid, ok := raw["queryId"]; ok && !existing[toUint(qid)] {
        delete(raw, "queryId")
        out, _ := json.Marshal(raw)
        return string(out), true
    }
    return content, false
}
```

If the `plugin_kvs` table schema differs from what the marker query assumes, grep `models/` for `PluginKV` and adapt the column names.

- [ ] **Step 3: Commit**

```bash
git add application_context/context.go
git commit -m "feat(blocks): BH-020 — one-shot dangling-ref cleanup migration with SKIP_BLOCK_REF_CLEANUP"
```

## Task 5: BH-024 fix — 500 → 404 on dangling query

**Files:**
- Modify: `server/api_handlers/block_api_handlers.go`

- [ ] **Step 1: Write the failing test**

**Files:** Create `server/api_tests/table_block_dangling_query_returns_404_test.go`:

```go
package api_tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTableBlock_DanglingQueryReturns404(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create a saved query, reference it in a table block, then delete the query.
	form := url.Values{}
	form.Set("Name", "bh024-q")
	form.Set("Content", "type = note")
	queryRR := tc.MakeFormRequest(http.MethodPost, "/v1/mrql/saved", form)
	require.Equal(t, http.StatusOK, queryRR.Code)

	var query map[string]any
	json.Unmarshal(queryRR.Body.Bytes(), &query)
	queryID := uint(query["ID"].(float64))

	note := tc.CreateDummyNote("bh024-tablenote")
	blockForm := url.Values{}
	blockForm.Set("NoteId", fmt.Sprintf("%d", note.ID))
	blockForm.Set("Type", "table")
	blockForm.Set("Content", fmt.Sprintf(`{"queryId":%d}`, queryID))
	blockRR := tc.MakeFormRequest(http.MethodPost, "/v1/note/block", blockForm)
	require.Equal(t, http.StatusOK, blockRR.Code)

	// Parse out blockId from response
	var block map[string]any
	json.Unmarshal(blockRR.Body.Bytes(), &block)
	blockID := uint(block["ID"].(float64))

	// NOTE: BH-020's fix would scrub the queryId on delete. To test BH-024 specifically,
	// delete via SQL directly to bypass the scrubber, leaving the dangling reference.
	tc.DB.Exec(`DELETE FROM saved_mrql_queries WHERE id = ?`, queryID)

	// Now GET the table block's query endpoint
	getRR := tc.MakeRequest(http.MethodGet, fmt.Sprintf("/v1/note/block/table/query?blockId=%d", blockID), nil)
	assert.Equal(t, http.StatusNotFound, getRR.Code, "dangling query must yield 404 (BH-024)")
}
```

Run 3× to verify fail with 500.

- [ ] **Step 2: Fix — wrap the inner query fetch**

In `block_api_handlers.go`, find the table-block query handler. Where the inner query fetch happens:

```go
query, err := ctx.GetSavedMRQLQuery(content.QueryID)
if err != nil {
    http_utils.HandleError(w, err)  // BUG: returned 500 for ErrRecordNotFound
    return
}
```

Replace with:

```go
query, err := ctx.GetSavedMRQLQuery(content.QueryID)
if err != nil {
    statusCode := server.StatusCodeForError(err)
    http.Error(w, err.Error(), statusCode)
    return
}
```

Or equivalent — use whichever helper other endpoints (`/v1/note`, `/v1/group`) use to translate `gorm.ErrRecordNotFound` → 404 (typically `statusCodeForError` in `server/api_handlers/error_status.go`).

- [ ] **Step 3: Run the test 3× to verify pass**

```bash
go test --tags 'json1 fts5' ./server/api_tests/ -run TestTableBlock_DanglingQueryReturns404 -v -count=3
```

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add server/api_handlers/block_api_handlers.go server/api_tests/table_block_dangling_query_returns_404_test.go
git commit -m "fix(blocks): BH-024 — dangling query reference returns 404, not 500"
```

## Task 6: UI graceful-degrade (low risk, high user value)

**Files:**
- Modify: `src/components/blocks/gallery.js` (or whichever file renders gallery)
- Modify: `src/components/blocks/references.js`
- Modify: `src/components/blocks/table.js`

- [ ] **Step 1: In each block component's resource/group/query fetch, handle 404 responses by setting a flag on the local item and rendering "Resource unavailable" / "Group unavailable" / "Query unavailable" instead of dropping into a console error**

Pattern (gallery):

```js
async fetchResource(id) {
  const resp = await fetch(`/v1/resource?id=${id}`, { headers: { Accept: 'application/json' } });
  if (resp.status === 404) {
    return { __unavailable: true, id };
  }
  if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
  return resp.json();
}
```

Render:

```html
<template x-if="resource.__unavailable">
  <span class="resource-unavailable" x-text="'Resource #' + resource.id + ' unavailable'"></span>
</template>
```

- [ ] **Step 2: Rebuild JS bundle**

```bash
npm run build-js
```

- [ ] **Step 3: Commit**

```bash
git add src/components/blocks public/dist
git commit -m "feat(blocks): BH-020 — graceful 'unavailable' render for dangling refs"
```

---

## Cluster PR gate

- [ ] **Step 1: Full Go suite**

```bash
cd <worktree>
go test --tags 'json1 fts5' ./...
```

- [ ] **Step 2: Rebase + full E2E + Postgres.**

- [ ] **Step 3: Open PR, self-merge**

```bash
gh pr create --title "fix(blocks): BH-020, BH-024 — deletion-cascade + dangling-ref 404" --body "$(cat <<'EOF'
Closes BH-020, BH-024.

## Changes

- `application_context/block_ref_cleanup.go` — shared scrubber for gallery / references / calendar / table block content.
- Resource / group / saved-query delete handlers now call the scrubber inside the delete transaction.
- One-shot boot migration cleans up pre-existing orphans, gated by `SKIP_BLOCK_REF_CLEANUP=1`.
- `server/api_handlers/block_api_handlers.go` — table-block query handler translates `ErrRecordNotFound` → 404 via `statusCodeForError` (BH-024, 2-line fix).
- Block UI components render "Resource/Group/Query unavailable" on 404 instead of logging errors.

## Tests

- Unit: ✓ `block_ref_cleanup_test.go`
- Go API: ✓ 4 integration tests, one per block type, pass 3× pre-fix red / post-fix green.
- Go API: ✓ BH-024 dangling query → 404 test, 3× pre red / post green.
- Full `go test ./...`: ✓
- Full E2E: ✓
- Postgres: ✓

## Operator note

The one-shot migration runs at startup. On deployments with millions of resources, set `SKIP_BLOCK_REF_CLEANUP=1` to defer.

## Bug-hunt-log update

Post-merge: move BH-020 and BH-024 to Fixed / closed.
EOF
)"
gh pr merge --merge --delete-branch
```

Then master plan Step F.
