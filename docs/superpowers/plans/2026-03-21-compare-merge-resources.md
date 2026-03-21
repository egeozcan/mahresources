# Compare & Merge for Similar Resources — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add compare links on similar resources, use Left/Right labels for cross-resource comparison, and add a merge panel with bidirectional merge and keep-as-version option to the compare view.

**Architecture:** Three layers of change — (1) template-only changes for compare links and label swapping, (2) a new merge panel in the compare template backed by a `canMerge` context variable, (3) backend extension of `MergeResources` with a `keepAsVersion` parameter that creates a version record from the loser's resource-level file before deletion.

**Tech Stack:** Go, GORM, Pongo2 templates, Alpine.js, Tailwind CSS

**Spec:** `docs/superpowers/specs/2026-03-21-compare-merge-resources-design.md`

---

### Task 1: Add Compare Links on Similar Resources

**Files:**
- Modify: `templates/displayResource.tpl:185-200`

The `seeAll.tpl` partial renders resource cards in a generic loop with no per-item customization hooks. Rather than modifying the shared partial (used across many entity types), replace the `seeAll` include for similar resources with an inline loop that renders each resource card with a compare link.

- [ ] **Step 1: Replace seeAll include with inline loop + compare links**

In `templates/displayResource.tpl`, replace the similar resources section (lines 185-186):

```pongo2
{% include "/partials/seeAll.tpl" with entities=similarResources subtitle="Similar Resources" templateName="resource" %}
```

with:

```pongo2
<div class="detail-panel">
    <div class="detail-panel-header">
        <h3 class="detail-panel-title">Similar Resources</h3>
    </div>
    <div class="detail-panel-body">
        <div class="list-container">
            {% for entity in similarResources %}
                {% include partial("resource") %}
                <a href="/resource/compare?r1={{ resource.ID }}&r2={{ entity.ID }}" class="btn btn-sm btn-outline ml-2">Compare</a>
            {% endfor %}
        </div>
    </div>
</div>
```

Note: the existing `seeAll.tpl` include doesn't pass `formAction`/`formParamName`/`formID` for similar resources, so the "See All" button and lightbox attributes are not active for this section. The inline replacement preserves the same structure.

- [ ] **Step 2: Build and verify visually**

Run: `npm run build`

Start ephemeral server with a seeded DB that has resources with similar images. Navigate to a resource detail page and verify:
- Similar resources still display with thumbnails
- Each has a "Compare" link
- Clicking the link navigates to `/resource/compare?r1=X&r2=Y`
- The existing "Merge Others To This" button still appears below

- [ ] **Step 3: Commit**

```bash
git add templates/displayResource.tpl
git commit -m "feat: add compare links on similar resources"
```

---

### Task 2: Left/Right Labels for Cross-Resource Comparison

**Files:**
- Modify: `templates/compare.tpl:15,52,232-234`
- Modify: `templates/partials/compareImage.tpl:38,42,62-63,75-77,84-85`
- Modify: `templates/partials/compareText.tpl:62,73`
- Modify: `templates/partials/comparePdf.tpl:13,26,44,52`
- Modify: `templates/partials/compareBinary.tpl:9,27`

The `crossResource` variable is already available in the template context. All changes are text-only — CSS classes stay the same.

- [ ] **Step 1: Update compare.tpl picker toolbar labels**

Line 15, change:
```pongo2
<span class="compare-side-label--old" aria-label="Old version">OLD</span>
```
to:
```pongo2
<span class="compare-side-label--old" aria-label="{% if crossResource %}Left resource{% else %}Old version{% endif %}">{% if crossResource %}Left{% else %}OLD{% endif %}</span>
```

Line 52, change:
```pongo2
<span class="compare-side-label--new" aria-label="New version">NEW</span>
```
to:
```pongo2
<span class="compare-side-label--new" aria-label="{% if crossResource %}Right resource{% else %}New version{% endif %}">{% if crossResource %}Right{% else %}NEW{% endif %}</span>
```

- [ ] **Step 2: Update compare.tpl empty state labels**

Lines 232-234, change:
```pongo2
<span class="compare-side-label--old">OLD</span>
<span class="text-stone-400" aria-hidden="true">&harr;</span>
<span class="compare-side-label--new">NEW</span>
```
to:
```pongo2
<span class="compare-side-label--old">{% if crossResource %}Left{% else %}OLD{% endif %}</span>
<span class="text-stone-400" aria-hidden="true">&harr;</span>
<span class="compare-side-label--new">{% if crossResource %}Right{% else %}NEW{% endif %}</span>
```

- [ ] **Step 3: Update compareImage.tpl labels**

Line 38, side-by-side OLD header:
```pongo2
<div class="compare-panel-header--old">OLD — v{{ comparison.Version1.VersionNumber }}</div>
```
to:
```pongo2
<div class="compare-panel-header--old">{% if crossResource %}Left{% else %}OLD{% endif %} — v{{ comparison.Version1.VersionNumber }}</div>
```

Line 42, side-by-side NEW header:
```pongo2
<div class="compare-panel-header--new">NEW — v{{ comparison.Version2.VersionNumber }}</div>
```
to:
```pongo2
<div class="compare-panel-header--new">{% if crossResource %}Right{% else %}NEW{% endif %} — v{{ comparison.Version2.VersionNumber }}</div>
```

Line 62, slider OLD label:
```pongo2
<div class="absolute top-2 left-2"><span class="compare-side-label--old">OLD</span></div>
```
to:
```pongo2
<div class="absolute top-2 left-2"><span class="compare-side-label--old">{% if crossResource %}Left{% else %}OLD{% endif %}</span></div>
```

Line 63, slider NEW label:
```pongo2
<div class="absolute top-2 right-2"><span class="compare-side-label--new">NEW</span></div>
```
to:
```pongo2
<div class="absolute top-2 right-2"><span class="compare-side-label--new">{% if crossResource %}Right{% else %}NEW{% endif %}</span></div>
```

Lines 75-77, onion skin labels:
```pongo2
<span class="compare-side-label--old">OLD</span>
<input type="range" min="0" max="100" x-model="opacity" class="w-48" aria-label="Onion skin opacity">
<span class="compare-side-label--new">NEW</span>
```
to:
```pongo2
<span class="compare-side-label--old">{% if crossResource %}Left{% else %}OLD{% endif %}</span>
<input type="range" min="0" max="100" x-model="opacity" class="w-48" aria-label="Onion skin opacity">
<span class="compare-side-label--new">{% if crossResource %}Right{% else %}NEW{% endif %}</span>
```

Lines 84-85, toggle mode labels:
```pongo2
<span x-show="showLeft" class="compare-side-label--old">OLD — v{{ comparison.Version1.VersionNumber }}</span>
<span x-show="!showLeft" class="compare-side-label--new">NEW — v{{ comparison.Version2.VersionNumber }}</span>
```
to:
```pongo2
<span x-show="showLeft" class="compare-side-label--old">{% if crossResource %}Left{% else %}OLD{% endif %} — v{{ comparison.Version1.VersionNumber }}</span>
<span x-show="!showLeft" class="compare-side-label--new">{% if crossResource %}Right{% else %}NEW{% endif %} — v{{ comparison.Version2.VersionNumber }}</span>
```

- [ ] **Step 4: Update compareText.tpl labels**

Line 62:
```pongo2
<div class="compare-panel-header--old sticky top-0 z-10">OLD — v{{ comparison.Version1.VersionNumber }}</div>
```
to:
```pongo2
<div class="compare-panel-header--old sticky top-0 z-10">{% if crossResource %}Left{% else %}OLD{% endif %} — v{{ comparison.Version1.VersionNumber }}</div>
```

Line 73:
```pongo2
<div class="compare-panel-header--new sticky top-0 z-10">NEW — v{{ comparison.Version2.VersionNumber }}</div>
```
to:
```pongo2
<div class="compare-panel-header--new sticky top-0 z-10">{% if crossResource %}Right{% else %}NEW{% endif %} — v{{ comparison.Version2.VersionNumber }}</div>
```

- [ ] **Step 5: Update comparePdf.tpl labels**

Line 13:
```pongo2
<div class="compare-panel-header--old rounded-t -mx-4 -mt-4 mb-3">OLD — v{{ comparison.Version1.VersionNumber }}</div>
```
to:
```pongo2
<div class="compare-panel-header--old rounded-t -mx-4 -mt-4 mb-3">{% if crossResource %}Left{% else %}OLD{% endif %} — v{{ comparison.Version1.VersionNumber }}</div>
```

Line 26:
```pongo2
<div class="compare-panel-header--new rounded-t -mx-4 -mt-4 mb-3">NEW — v{{ comparison.Version2.VersionNumber }}</div>
```
to:
```pongo2
<div class="compare-panel-header--new rounded-t -mx-4 -mt-4 mb-3">{% if crossResource %}Right{% else %}NEW{% endif %} — v{{ comparison.Version2.VersionNumber }}</div>
```

Line 44 (iframe header):
```pongo2
<span>OLD — v{{ comparison.Version1.VersionNumber }}</span>
```
to:
```pongo2
<span>{% if crossResource %}Left{% else %}OLD{% endif %} — v{{ comparison.Version1.VersionNumber }}</span>
```

Line 52 (iframe header):
```pongo2
<span>NEW — v{{ comparison.Version2.VersionNumber }}</span>
```
to:
```pongo2
<span>{% if crossResource %}Right{% else %}NEW{% endif %} — v{{ comparison.Version2.VersionNumber }}</span>
```

- [ ] **Step 6: Update compareBinary.tpl labels**

Line 9:
```pongo2
<div class="compare-panel-header--old">OLD — v{{ comparison.Version1.VersionNumber }}</div>
```
to:
```pongo2
<div class="compare-panel-header--old">{% if crossResource %}Left{% else %}OLD{% endif %} — v{{ comparison.Version1.VersionNumber }}</div>
```

Line 27:
```pongo2
<div class="compare-panel-header--new">NEW — v{{ comparison.Version2.VersionNumber }}</div>
```
to:
```pongo2
<div class="compare-panel-header--new">{% if crossResource %}Right{% else %}NEW{% endif %} — v{{ comparison.Version2.VersionNumber }}</div>
```

- [ ] **Step 7: Build and verify**

Run: `npm run build`

Navigate to `/resource/compare?r1=X&r2=Y` (cross-resource) and verify all labels say "Left"/"Right". Then navigate to `/resource/compare?r1=X&v1=1&v1=2` (same resource) and verify labels still say "OLD"/"NEW".

- [ ] **Step 8: Commit**

```bash
git add templates/compare.tpl templates/partials/compareImage.tpl templates/partials/compareText.tpl templates/partials/comparePdf.tpl templates/partials/compareBinary.tpl
git commit -m "feat: use Left/Right labels for cross-resource comparison"
```

---

### Task 3: Add `canMerge` Context Variable

**Files:**
- Modify: `server/template_handlers/template_context_providers/compare_template_context.go:102-112`

- [ ] **Step 1: Write the failing test**

Create `server/api_tests/compare_context_canmerge_test.go`:

```go
package api_tests

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCompareContextCanMerge(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create two resources with versions
	r1 := tc.MustCreateResourceWithContent("res1.txt", "content-one")
	r2 := tc.MustCreateResourceWithContent("res2.txt", "content-two")

	// Get the latest version numbers
	versions1, _ := tc.AppCtx.GetVersions(r1.ID)
	versions2, _ := tc.AppCtx.GetVersions(r2.ID)

	v1Num := versions1[0].VersionNumber
	v2Num := versions2[0].VersionNumber

	// Cross-resource, both at latest versions → canMerge should be true
	url := fmt.Sprintf("/resource/compare?r1=%d&v1=%d&r2=%d&v2=%d", r1.ID, v1Num, r2.ID, v2Num)
	req := httptest.NewRequest(http.MethodGet, url, nil)

	ctx := tc.CompareContextProvider(req)
	assert.Equal(t, true, ctx["canMerge"])

	// Same resource → canMerge should be false
	url2 := fmt.Sprintf("/resource/compare?r1=%d&v1=%d&r2=%d&v2=%d", r1.ID, v1Num, r1.ID, v1Num)
	req2 := httptest.NewRequest(http.MethodGet, url2, nil)

	ctx2 := tc.CompareContextProvider(req2)
	assert.Equal(t, false, ctx2["canMerge"])
}
```

Note: the exact test helpers (`MustCreateResourceWithContent`, `CompareContextProvider`) may not exist. Adapt to match the test environment's actual API — e.g., use `tc.AppCtx.AddResource(...)` and call `CompareContextProvider(tc.AppCtx)` directly. Check `server/api_tests/test_helpers.go` or similar for available helpers.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test --tags 'json1 fts5' ./server/api_tests/ -run TestCompareContextCanMerge -v`
Expected: FAIL (function doesn't exist or `canMerge` not in context)

- [ ] **Step 3: Implement canMerge in CompareContextProvider**

In `server/template_handlers/template_context_providers/compare_template_context.go`, modify the return block (around line 102). Before the `return baseContext.Update(...)`:

```go
// Determine if merge is available (cross-resource, both at latest versions)
canMerge := false
if query.Resource1ID != query.Resource2ID && len(versions1) > 0 && len(versions2) > 0 {
    canMerge = query.Version1 == versions1[0].VersionNumber && query.Version2 == versions2[0].VersionNumber
}
```

Then add `"canMerge": canMerge` to the `pongo2.Context` map in the `return` statement.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test --tags 'json1 fts5' ./server/api_tests/ -run TestCompareContextCanMerge -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add server/template_handlers/template_context_providers/compare_template_context.go server/api_tests/compare_context_canmerge_test.go
git commit -m "feat: add canMerge context variable for compare view"
```

---

### Task 4: Add Merge Panel to Compare Template

**Files:**
- Modify: `templates/compare.tpl` (after content comparison area, before `{% else %}`)

- [ ] **Step 1: Add the merge panel**

In `templates/compare.tpl`, after the content comparison includes (after the `{% else %}` / `{% include "/partials/compareBinary.tpl" %}` / `{% endif %}` block for content categories, around line 219), add:

```pongo2
{% if canMerge %}
<details class="mt-6 bg-white shadow rounded-lg" x-data="{ keepAsVersion: false }">
    <summary class="cursor-pointer text-sm font-medium text-stone-600 p-4 select-none font-mono">Merge</summary>
    <div class="p-4 pt-0">
        <div class="mb-4">
            <label class="flex items-center gap-2 text-sm text-stone-600 cursor-pointer">
                <input type="checkbox" x-model="keepAsVersion" class="rounded border-stone-300 text-amber-700 focus:ring-amber-600">
                Keep loser as older version of winner
            </label>
        </div>
        <div class="flex justify-between items-center gap-4">
            <form
                x-data="confirmAction({ message: 'Resource on the right will be merged into the left resource. Are you sure?' })"
                action="/v1/resources/merge"
                method="post"
                :action="'/v1/resources/merge?redirect=' + encodeURIComponent(window.location.pathname + window.location.search)"
                x-bind="events"
            >
                <input type="hidden" name="winner" value="{{ resource1.ID }}">
                <input type="hidden" name="losers" value="{{ resource2.ID }}">
                <input type="hidden" name="KeepAsVersion" :value="keepAsVersion">
                {% include "/partials/form/searchButton.tpl" with text="← Left Wins" %}
            </form>
            <form
                x-data="confirmAction({ message: 'Resource on the left will be merged into the right resource. Are you sure?' })"
                action="/v1/resources/merge"
                method="post"
                :action="'/v1/resources/merge?redirect=' + encodeURIComponent(window.location.pathname + window.location.search)"
                x-bind="events"
            >
                <input type="hidden" name="winner" value="{{ resource2.ID }}">
                <input type="hidden" name="losers" value="{{ resource1.ID }}">
                <input type="hidden" name="KeepAsVersion" :value="keepAsVersion">
                {% include "/partials/form/searchButton.tpl" with text="Right Wins →" %}
            </form>
        </div>
    </div>
</details>
{% endif %}
```

Note: The `x-data="confirmAction(...)"` pattern matches the existing merge button on `displayResource.tpl`. The `KeepAsVersion` hidden input uses Alpine's `:value` binding to reflect the checkbox state. The `redirect` param is passed via query string on the action URL, matching the existing pattern.

- [ ] **Step 2: Build and verify visually**

Run: `npm run build`

Navigate to `/resource/compare?r1=X&r2=Y` with two different resources both at their latest versions. Verify:
- Collapsed "Merge" `<details>` element appears at the bottom
- Expanding it shows "Keep loser as older version" checkbox (unchecked) and two buttons
- "Left Wins" and "Right Wins" are on opposite ends
- The panel does NOT appear for same-resource version comparison

- [ ] **Step 3: Commit**

```bash
git add templates/compare.tpl
git commit -m "feat: add merge panel to compare view with bidirectional merge"
```

---

### Task 5: Backend — Add `KeepAsVersion` to MergeQuery and Update Interface

**Files:**
- Modify: `models/query_models/entity_query.go:26-29`
- Modify: `server/interfaces/resource_interfaces.go:57-60`
- Modify: `server/api_handlers/resource_api_handlers.go:658-680`
- Modify: `application_context/resource_bulk_context.go:530` (signature only for now)

- [ ] **Step 1: Add KeepAsVersion to MergeQuery**

In `models/query_models/entity_query.go`, change:

```go
type MergeQuery struct {
	Winner uint
	Losers []uint
}
```

to:

```go
type MergeQuery struct {
	Winner        uint
	Losers        []uint
	KeepAsVersion bool
}
```

- [ ] **Step 2: Update ResourceMerger interface**

In `server/interfaces/resource_interfaces.go`, change:

```go
type ResourceMerger interface {
	MergeResources(winnerId uint, loserIds []uint) error
}
```

to:

```go
type ResourceMerger interface {
	MergeResources(winnerId uint, loserIds []uint, keepAsVersion bool) error
}
```

- [ ] **Step 3: Update MergeResources signature (no logic yet)**

In `application_context/resource_bulk_context.go`, change line 530:

```go
func (ctx *MahresourcesContext) MergeResources(winnerId uint, loserIds []uint) error {
```

to:

```go
func (ctx *MahresourcesContext) MergeResources(winnerId uint, loserIds []uint, keepAsVersion bool) error {
```

(No behavioral change yet — just the signature.)

- [ ] **Step 4: Update the API handler call site**

In `server/api_handlers/resource_api_handlers.go`, change line 671:

```go
err = effectiveCtx.MergeResources(editor.Winner, editor.Losers)
```

to:

```go
err = effectiveCtx.MergeResources(editor.Winner, editor.Losers, editor.KeepAsVersion)
```

- [ ] **Step 5: Update existing merge form in displayResource.tpl**

The existing "Merge Others To This" form at `templates/displayResource.tpl` calls the same endpoint. It doesn't send `KeepAsVersion`, which means gorilla/schema will default it to `false`. No change needed — but verify the form still works.

- [ ] **Step 6: Fix existing merge tests**

The existing tests in `server/api_tests/resource_merge_test.go` and `resource_merge_multi_loser_test.go` call `tc.AppCtx.MergeResources(winner.ID, []uint{loser.ID})`. These need a third argument:

```go
tc.AppCtx.MergeResources(winner.ID, []uint{loser.ID}, false)
```

Search for all call sites of `MergeResources` across the codebase and add `, false` to each.

- [ ] **Step 7: Run all Go tests to verify compilation and no regressions**

Run: `go test --tags 'json1 fts5' ./...`
Expected: All existing tests pass. No compile errors.

- [ ] **Step 8: Commit**

```bash
git add models/query_models/entity_query.go server/interfaces/resource_interfaces.go application_context/resource_bulk_context.go server/api_handlers/resource_api_handlers.go server/api_tests/
git commit -m "feat: add KeepAsVersion parameter to merge interface (no-op for now)"
```

---

### Task 6: Backend — Implement Keep-as-Version Logic

**Files:**
- Modify: `application_context/resource_bulk_context.go` (inside `MergeResources`, before version transfer)
- Test: `server/api_tests/resource_merge_keep_version_test.go` (new)

The loser's resource-level file (its `Hash`, `Location`, `ContentType`, `Width`, `Height`, `FileSize`, `StorageLocation`) is distinct from its `ResourceVersion` records. When `keepAsVersion` is true, we create a new `ResourceVersion` from the loser's resource-level file data before the standard version transfer, so it appears as an older version of the winner.

- [ ] **Step 1: Write the failing test**

Create `server/api_tests/resource_merge_keep_version_test.go`:

```go
package api_tests

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"mahresources/models"
	"mahresources/models/query_models"
)

func TestMergeResourcesKeepAsVersion(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create winner resource
	file1 := io.NopCloser(bytes.NewReader([]byte("winner-content")))
	winner, err := tc.AppCtx.AddResource(file1, "winner.txt", &query_models.ResourceCreator{
		ResourceQueryBase: query_models.ResourceQueryBase{Name: "Winner Resource"},
	})
	assert.NoError(t, err)

	// Create loser resource with different content
	file2 := io.NopCloser(bytes.NewReader([]byte("loser-content-to-keep")))
	loser, err := tc.AppCtx.AddResource(file2, "loser.txt", &query_models.ResourceCreator{
		ResourceQueryBase: query_models.ResourceQueryBase{Name: "Loser Resource"},
	})
	assert.NoError(t, err)

	// Record loser's resource-level file details
	loserHash := loser.Hash
	loserContentType := loser.ContentType

	// Count winner versions before merge
	var winnerVersionsBefore int64
	tc.DB.Model(&models.ResourceVersion{}).Where("resource_id = ?", winner.ID).Count(&winnerVersionsBefore)

	// Count loser versions before merge
	var loserVersionsBefore int64
	tc.DB.Model(&models.ResourceVersion{}).Where("resource_id = ?", loser.ID).Count(&loserVersionsBefore)

	// Merge with keepAsVersion = true
	err = tc.AppCtx.MergeResources(winner.ID, []uint{loser.ID}, true)
	assert.NoError(t, err)

	// Winner should now have: own versions + 1 (loser's resource-level file) + loser's versions
	var winnerVersionsAfter int64
	tc.DB.Model(&models.ResourceVersion{}).Where("resource_id = ?", winner.ID).Count(&winnerVersionsAfter)
	assert.Equal(t, winnerVersionsBefore+loserVersionsBefore+1, winnerVersionsAfter,
		"winner should have own versions + loser's resource-level file as version + loser's transferred versions")

	// Verify the loser's resource-level file was preserved as a version with provenance comment
	var keptVersion models.ResourceVersion
	err = tc.DB.Where("resource_id = ? AND hash = ? AND comment LIKE ?",
		winner.ID, loserHash, "%Merged from:%").
		First(&keptVersion).Error
	assert.NoError(t, err, "loser's resource-level file should exist as a version on winner")
	assert.Equal(t, loserContentType, keptVersion.ContentType)
	assert.Contains(t, keptVersion.Comment, "Loser Resource")
}

func TestMergeResourcesKeepAsVersionFalse(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create winner and loser
	file1 := io.NopCloser(bytes.NewReader([]byte("winner-content")))
	winner, err := tc.AppCtx.AddResource(file1, "winner.txt", &query_models.ResourceCreator{
		ResourceQueryBase: query_models.ResourceQueryBase{Name: "Winner"},
	})
	assert.NoError(t, err)

	file2 := io.NopCloser(bytes.NewReader([]byte("loser-content")))
	loser, err := tc.AppCtx.AddResource(file2, "loser.txt", &query_models.ResourceCreator{
		ResourceQueryBase: query_models.ResourceQueryBase{Name: "Loser"},
	})
	assert.NoError(t, err)

	var winnerVersionsBefore int64
	tc.DB.Model(&models.ResourceVersion{}).Where("resource_id = ?", winner.ID).Count(&winnerVersionsBefore)
	var loserVersionsBefore int64
	tc.DB.Model(&models.ResourceVersion{}).Where("resource_id = ?", loser.ID).Count(&loserVersionsBefore)

	// Merge with keepAsVersion = false (original behavior)
	err = tc.AppCtx.MergeResources(winner.ID, []uint{loser.ID}, false)
	assert.NoError(t, err)

	// Winner should have own + loser's versions (no extra version created)
	var winnerVersionsAfter int64
	tc.DB.Model(&models.ResourceVersion{}).Where("resource_id = ?", winner.ID).Count(&winnerVersionsAfter)
	assert.Equal(t, winnerVersionsBefore+loserVersionsBefore, winnerVersionsAfter,
		"keepAsVersion=false should not create extra versions")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test --tags 'json1 fts5' ./server/api_tests/ -run TestMergeResourcesKeepAsVersion -v`
Expected: FAIL — `keepAsVersion=true` doesn't create the extra version yet

- [ ] **Step 3: Implement keepAsVersion logic**

In `application_context/resource_bulk_context.go`, inside the `MergeResources` function, within the transaction (after loading `losers` and `winner`, before the version transfer section at line 578), add:

```go
// If keepAsVersion is true, create a version from each loser's resource-level file
if keepAsVersion {
    // Get the current max version number on the winner
    var currentMax int
    if err := tx.Model(&models.ResourceVersion{}).
        Where("resource_id = ?", winnerId).
        Select("COALESCE(MAX(version_number), 0)").
        Scan(&currentMax).Error; err != nil {
        return err
    }

    for i, loser := range losers {
        version := models.ResourceVersion{
            ResourceID:      winnerId,
            VersionNumber:   currentMax + i + 1,
            Hash:            loser.Hash,
            HashType:        loser.HashType,
            FileSize:        loser.FileSize,
            ContentType:     loser.ContentType,
            Width:           loser.Width,
            Height:          loser.Height,
            Location:        loser.Location,
            StorageLocation: loser.StorageLocation,
            Comment:         fmt.Sprintf("Merged from: %s", loser.Name),
        }
        if err := tx.Create(&version).Error; err != nil {
            return err
        }
    }
}
```

This runs BEFORE the version transfer block. The version transfer block (lines 578-607) then re-reads `maxVersion` from the winner and appends loser's existing versions after the newly created ones, maintaining correct ordering.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test --tags 'json1 fts5' ./server/api_tests/ -run TestMergeResourcesKeepAsVersion -v`
Expected: PASS for both test functions

- [ ] **Step 5: Run all Go tests for regressions**

Run: `go test --tags 'json1 fts5' ./...`
Expected: All pass

- [ ] **Step 6: Commit**

```bash
git add application_context/resource_bulk_context.go server/api_tests/resource_merge_keep_version_test.go
git commit -m "feat: implement keepAsVersion for resource merge"
```

---

### Task 7: E2E Tests

**Files:**
- Modify: `e2e/tests/15-version-compare.spec.ts` (add cross-resource label and merge panel tests)

- [ ] **Step 1: Add E2E tests for cross-resource labels and merge panel**

Add new tests to `e2e/tests/15-version-compare.spec.ts`:

```typescript
test('cross-resource compare shows Left/Right labels instead of OLD/NEW', async ({ page, apiClient }) => {
    const cat = await apiClient.createCategory('resource', 'TestCat');
    const group = await apiClient.createGroup('TestGroup', cat.id);

    const r1 = await apiClient.createResource('left-resource.txt', cat.id, group.id, 'left content');
    const r2 = await apiClient.createResource('right-resource.txt', cat.id, group.id, 'right content');

    const versions1 = await apiClient.getVersions(r1.id);
    const versions2 = await apiClient.getVersions(r2.id);

    await page.goto(`/resource/compare?r1=${r1.id}&v1=${versions1[0].versionNumber}&r2=${r2.id}&v2=${versions2[0].versionNumber}`);

    // Should show Left/Right, not OLD/NEW
    await expect(page.locator('.compare-side-label--old')).toContainText('Left');
    await expect(page.locator('.compare-side-label--new')).toContainText('Right');

    // Merge panel should be visible (collapsed)
    await expect(page.locator('details summary')).toContainText('Merge');
});

test('same-resource compare shows OLD/NEW labels and no merge panel', async ({ page, apiClient }) => {
    const cat = await apiClient.createCategory('resource', 'TestCat');
    const group = await apiClient.createGroup('TestGroup', cat.id);

    const r1 = await apiClient.createResource('resource.txt', cat.id, group.id, 'content v1');
    // Upload a second version
    await apiClient.uploadVersion(r1.id, 'content v2', 'resource-v2.txt');

    const versions = await apiClient.getVersions(r1.id);

    await page.goto(`/resource/compare?r1=${r1.id}&v1=${versions[1].versionNumber}&v2=${versions[0].versionNumber}`);

    // Should show OLD/NEW
    await expect(page.locator('.compare-side-label--old').first()).toContainText('OLD');
    await expect(page.locator('.compare-side-label--new').first()).toContainText('NEW');

    // Merge panel should NOT be visible
    await expect(page.locator('details summary')).toHaveCount(0);
});

test('merge panel has Left Wins and Right Wins buttons', async ({ page, apiClient }) => {
    const cat = await apiClient.createCategory('resource', 'TestCat');
    const group = await apiClient.createGroup('TestGroup', cat.id);

    const r1 = await apiClient.createResource('left.txt', cat.id, group.id, 'left');
    const r2 = await apiClient.createResource('right.txt', cat.id, group.id, 'right');

    const versions1 = await apiClient.getVersions(r1.id);
    const versions2 = await apiClient.getVersions(r2.id);

    await page.goto(`/resource/compare?r1=${r1.id}&v1=${versions1[0].versionNumber}&r2=${r2.id}&v2=${versions2[0].versionNumber}`);

    // Expand merge panel
    await page.locator('details summary').click();

    // Verify buttons exist
    await expect(page.getByRole('button', { name: /Left Wins/ })).toBeVisible();
    await expect(page.getByRole('button', { name: /Right Wins/ })).toBeVisible();

    // Verify keepAsVersion checkbox exists and is unchecked
    const checkbox = page.locator('input[type="checkbox"]');
    await expect(checkbox).not.toBeChecked();
});
```

Note: The exact API client methods (`createResource`, `getVersions`, `uploadVersion`) may have different signatures. Adapt to match `e2e/helpers/api-client.ts`. Also, the `details summary` selector for the merge panel may need to be more specific if other `<details>` elements exist (e.g., the Metadata details). Use a more specific selector like `details summary:text("Merge")` or scope to the merge panel area.

- [ ] **Step 2: Run E2E tests**

Run: `cd e2e && npm run test:with-server -- --grep "cross-resource\|OLD/NEW\|merge panel"`
Expected: PASS

- [ ] **Step 3: Run full E2E suite for regressions**

Run: `cd e2e && npm run test:with-server:all`
Expected: All pass

- [ ] **Step 4: Commit**

```bash
git add e2e/tests/15-version-compare.spec.ts
git commit -m "test: add E2E tests for cross-resource labels and merge panel"
```

---

### Task 8: Compare Link in Similar Resources — E2E Test

**Files:**
- Add test to appropriate spec file (e.g., `e2e/tests/15-version-compare.spec.ts` or a new file if compare tests for similar resources warrant it)

- [ ] **Step 1: Add E2E test for compare link on similar resources**

This test needs two resources with matching perceptual hashes. Since the hash worker runs asynchronously, the test may need to either:
- Create resources with identical image content (same file uploaded twice with different names) and wait for hash processing, or
- Directly insert `ResourceSimilarity` records via the DB (if the test fixture exposes DB access)

If the hash worker is disabled in test mode or too slow, consider testing the compare link presence by:
1. Creating a resource
2. Navigating to its page
3. Checking that the similar resources section, when present, contains compare links with the correct URL pattern

Adapt based on what the test infrastructure supports. The key assertion is:

```typescript
// If similar resources are shown, each should have a compare link
const compareLinks = page.locator('a[href*="/resource/compare"]');
// Verify the link points to the right resources
const href = await compareLinks.first().getAttribute('href');
expect(href).toContain(`r1=${resourceId}`);
```

- [ ] **Step 2: Run E2E tests**

Run: `cd e2e && npm run test:with-server`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add e2e/tests/
git commit -m "test: add E2E test for compare link on similar resources"
```

---

### Task 9: Final Verification

- [ ] **Step 1: Run all Go unit tests**

Run: `go test --tags 'json1 fts5' ./...`
Expected: All pass

- [ ] **Step 2: Run all E2E tests (browser + CLI)**

Run: `cd e2e && npm run test:with-server:all`
Expected: All pass

- [ ] **Step 3: Manual smoke test**

Build: `npm run build`

Start server with a test database that has resources with similar images. Walk through the full flow:
1. Navigate to a resource with similar resources
2. Click "Compare" link → opens compare view with Left/Right labels
3. Expand merge panel, check "Keep loser as older version"
4. Click "Left Wins" → confirm → resource merges, loser preserved as version
5. Verify the winner now has the loser's file as an older version
6. Test "Merge Others To This" on the resource detail page still works

- [ ] **Step 4: Final commit if any adjustments needed**
