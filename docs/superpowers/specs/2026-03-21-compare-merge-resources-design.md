# Compare & Merge for Similar Resources

## Problem

The similar resources section on a resource detail page only offers a "Merge Others To This" button that merges ALL similar resources at once. There is no way to:

1. Compare a specific similar resource side-by-side before deciding what to do
2. Merge a single specific resource rather than all of them
3. Choose merge direction (which resource wins)
4. Preserve the loser as an older version of the winner

## Design

### 1. Compare Link on Similar Resources

**Location:** `templates/displayResource.tpl` (similar resources section, lines 185-200)

Add a "Compare" link for each similar resource in the list. The link opens the existing compare page:

```
/resource/compare?r1={currentResourceID}&r2={similarResourceID}
```

The existing "Merge Others To This" bulk button remains unchanged.

**Implementation:** Modify the similar resources loop in `displayResource.tpl` to include a compare link per resource. Since `seeAll.tpl` renders resource cards via `partial("resource")`, the compare link should be added as part of the similar resources section rather than modifying the shared resource card partial (which is used in many contexts). A new partial or inline template block iterating over `similarResources` with the compare link appended after each card is the cleanest approach.

### 2. Left/Right Labels for Cross-Resource Comparison

**Location:** `templates/compare.tpl` and `templates/partials/compareImage.tpl`

When `crossResource` is true (already available in template context), all "OLD"/"NEW" labels change to "Left"/"Right":

- Picker toolbar labels (lines 15, 52 of `compare.tpl`)
- Side-by-side panel headers (lines 38, 42 of `compareImage.tpl`)
- Slider overlay labels (lines 62-63 of `compareImage.tpl`)
- Onion skin slider labels (lines 75-77 of `compareImage.tpl`)
- Toggle mode labels (lines 84-85 of `compareImage.tpl`)
- Empty state hint (lines 232-234 of `compare.tpl`)

**No backend changes needed** — `crossResource` is already computed and passed to the template by `CompareContextProvider`.

The CSS classes `compare-side-label--old` and `compare-side-label--new` can remain as-is (they only control styling); only the display text changes.

### 3. Merge Panel on Compare View

**Location:** Bottom of `templates/compare.tpl`, after the content comparison area (after line 219)

A collapsible `<details>` element shown only when:
- `crossResource` is true
- Both sides are showing the current/top version of their respective resource

**Definition of "current/top version":** A side is showing the current version when `query.VersionN == versions[0].VersionNumber` (i.e., the selected version number equals the highest version number, since versions are ordered newest-first). This is unambiguous regardless of what `resource.CurrentVersionID` points to.

**Template context additions:** `CompareContextProvider` computes and passes:
- `canMerge` (bool): true when `crossResource && query.Version1 == versions1[0].VersionNumber && query.Version2 == versions2[0].VersionNumber`

**UI contents:**
- `<details>` with `<summary>Merge</summary>`
- Two form buttons on opposite ends: "Left Wins" (left-aligned), "Right Wins" (right-aligned)
- Each button submits a form to `POST /v1/resources/merge` with the appropriate winner/loser
- An unchecked checkbox label: "Keep loser as older version"
- Alpine.js `confirmAction` for confirmation before submitting (same pattern as existing merge button)

**Merge form fields:**
- `winner`: the winning resource ID
- `losers`: the losing resource ID (single value)
- `keepAsVersion`: boolean, from the checkbox
- `redirect`: current compare page URL or winner resource page

### 4. Backend: Keep-as-Version Merge Option

**Location:** `application_context/resource_bulk_context.go` (`MergeResources` function) and `models/query_models/entity_query.go` (`MergeQuery` struct)

**MergeQuery change:**
```go
type MergeQuery struct {
    Winner        uint
    Losers        []uint
    KeepAsVersion bool
}
```

(No schema tags — matches existing convention; gorilla/schema matches by field name.)

**MergeResources signature change:**

The current signature is `MergeResources(winnerId uint, loserIds []uint) error`. This must change to `MergeResources(winnerId uint, loserIds []uint, keepAsVersion bool) error`.

The `ResourceMerger` interface in `server/interfaces/resource_interfaces.go` must also be updated to match.

**Why `keepAsVersion` is not redundant:**

The existing merge transfers all `ResourceVersion` records from losers to the winner. However, a resource's *own* file (its `Hash`, `Location`, file metadata) is distinct from its `ResourceVersion` records — it represents the resource's identity/current state. This resource-level file is NOT automatically preserved as a version entry. When `keepAsVersion` is true, this file must be explicitly captured.

**MergeResources behavior when `keepAsVersion` is true:**

Before deleting each loser resource:

1. Create a new `ResourceVersion` on the winner using the loser's resource-level file (hash, content type, dimensions, file size, location)
2. The new version gets the next sequential version number after all existing winner versions
3. Set the version's comment to indicate provenance (e.g., "Merged from: {loser resource name}")
4. Then proceed with normal merge (transfer associations, transfer loser's existing versions, metadata backup, delete loser)

The loser's resource-level file version should be numbered *before* the transferred versions in the sequence, so it appears as an "older" entry. Ordering: winner's existing versions (oldest) → loser's resource-level file version → loser's transferred versions → winner's current version (newest).

### 5. API Handler Change

**Location:** `server/api_handlers/resource_api_handlers.go` (`GetMergeResourcesHandler`)

Parse the new `keepAsVersion` field from the form/query parameters and pass it through to `MergeResources`. The redirect behavior remains the same.

## Files Changed

| File | Change |
|------|--------|
| `templates/displayResource.tpl` | Add compare links in similar resources section |
| `templates/compare.tpl` | Conditional Left/Right labels; merge panel at bottom |
| `templates/partials/compareImage.tpl` | Conditional Left/Right labels |
| `templates/partials/compareText.tpl` | Conditional Left/Right labels (if applicable) |
| `templates/partials/comparePdf.tpl` | Conditional Left/Right labels (if applicable) |
| `templates/partials/compareBinary.tpl` | Conditional Left/Right labels (if applicable) |
| `server/template_handlers/template_context_providers/compare_template_context.go` | Add `canMerge` context variable |
| `models/query_models/entity_query.go` | Add `KeepAsVersion` to `MergeQuery` |
| `application_context/resource_bulk_context.go` | Handle `keepAsVersion` in `MergeResources` |
| `server/api_handlers/resource_api_handlers.go` | Parse `keepAsVersion` parameter |
| `server/interfaces/resource_interfaces.go` | Update `ResourceMerger` interface signature |

## Out of Scope

- Checkbox-based selective merge from the similar resources list (user chose compare-first workflow instead)
- Inline comparison within the resource detail page
- New API endpoints (reuses existing merge endpoint with a new parameter)
