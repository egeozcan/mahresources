# Multi-Resource Plugin Inputs (`entity_ref` Param Type)

**Date:** 2026-04-26
**Status:** Design — pending review
**Driven by:** fal.ai plugin needs to send 2+ images to Flux 2, Flux 2 Pro, and Nano Banana 2 image-edit models.

## Problem

Plugin actions today take exactly one trigger entity (`ctx.entity_id`) plus a flat map of typed scalars (`text`, `number`, `select`, `boolean`, `textarea`, `hidden`, `info`). There is no way for an action to accept additional entities as input.

The fal.ai plugin's `edit` action wraps four image-edit endpoints, three of which (`flux2`, `flux2pro`, `nanobanana2`) accept an array of image URLs but currently receive a single-element array (`plugins/fal-ai/plugin.lua:267, 283`). Users can't send a second reference image without code changes.

The same gap blocks other plausible plugin work: a "merge two notes" action, a "tag groups by another group's tags" action, a "vary outputs across N source images" generator. We need a standard, reusable mechanism.

## Goals

- Introduce one new `ActionParam` type, `entity_ref`, that can reference one or many resources, notes, or groups.
- Reuse the existing `entityPicker` (`src/components/picker/entityPicker.js`) and existing `ActionFilter` infrastructure (`plugin_system/actions.go`).
- Wire fal.ai's `edit` action to send multi-image payloads to Flux 2 / Flux 2 Pro / Nano Banana 2.
- Keep the wire format simple (IDs only — no auto-hydration).

## Non-Goals

- Tag and category as referenceable entity types (no current consumer).
- MRQL queries as a multi-input source (deferred; can be added as a separate param subtype if needed).
- Drag-and-drop selection.
- Per-fal.ai-model max-image counts enforced at the framework level (the plugin can enforce in its handler if needed).

## Key Design Decisions

| # | Decision | Resolution |
|---|----------|------------|
| 1 | Source of multi-resource selection | **Picker + bulk-selection prefill.** The existing `entityPicker` opens layered over the action modal; if the user has bulk-selected cards before opening the action, those IDs prefill the picker. |
| 2 | Entity-type scope | **Generic `entity_ref` with an `entity` field.** Supports `"resource"`, `"note"`, `"group"` from v1. The picker already abstracts over resource/group; note config is added by this plan. |
| 3 | Filtering | **Inherit action filters by default; per-param `filters` block overrides.** For fal.ai, the `edit` action's existing `filters = { content_types = IMAGE_CONTENT_TYPES }` automatically constrains the picker — zero extra config. |
| 4 | What's prefilled in the picker | **Configurable via `default = "trigger" \| "selection" \| "both" \| ""`, defaulting to `"trigger"`.** `default` is the single source of truth: bulk-selection enters the prefill only when the plugin sets `default = "selection"` or `"both"`. Plugins that want "use bulk if present, otherwise trigger" set `default = "both"`. |
| 5 | Handler payload shape | **IDs only.** `multi=false` → single number; `multi=true` → array of numbers. Plugins call `mah.db.get_resource(id)` if they need metadata, mirroring how `entity_id` already works. |

## Architecture

### New Param Type

`ActionParam.Type` gains the value `"entity_ref"`, with these new optional fields on the struct:

```go
type ActionParam struct {
    // ...existing fields...
    Entity  string        `json:"entity,omitempty"`  // "resource" | "note" | "group" — required when type=="entity_ref"
    Multi   bool          `json:"multi,omitempty"`   // false → single ID; true → array of IDs
    Filters *ActionFilter `json:"filters,omitempty"` // nil = inherit action.Filters; non-nil = override
}
```

`Default`, `Min`, `Max`, `Required`, `ShowWhen`, `Description`, `Label`, `Name` are reused as-is. For `entity_ref`:

- `Default` accepts the strings `"trigger"`, `"selection"`, `"both"`, or `""` (anything else is invalid). Default when omitted: `"trigger"`.
- `Min`, `Max` apply only when `Multi=true`. `Min` defaults to `0`. `Max` of `0` (the zero value) is treated as unlimited.
- `Required` for `Multi=false` means "must select one"; for `Multi=true` it is equivalent to `Min >= 1`.

### Lua Schema

```lua
{
    name        = "extra_images",
    type        = "entity_ref",
    label       = "Additional Images",
    entity      = "resource",
    multi       = true,
    required    = false,
    min         = 0,
    max         = 9,
    default     = "trigger",
    filters     = { content_types = {"image/jpeg", "image/png"} },  -- optional override
    show_when   = { model = {"flux2", "flux2pro", "nanobanana2"} }, -- array form (see show_when extension below)
    description = "Reference images to send alongside the source image.",
}
```

### Wire Format

Request body for POST `/v1/jobs/action/run` extends `actionRunRequest.Params`:

- `Multi=false`: `params.<name>` is a JSON `number` or `null`.
- `Multi=true`: `params.<name>` is a JSON `number[]` (always present as an array, possibly empty).

The 1MB body limit at `server/api_handlers/action_handlers.go:86` is unchanged and ample for ID arrays.

### Server-Side Validation (`plugin_system/action_executor.go`)

Validation splits along a clear axis to avoid mixing pure-structural and DB-backed checks (and to avoid duplicating the DB hit):

**Pure-structural — stays in `ValidateActionParams(action, params) []ValidationError`.** Existing signature unchanged. New `entity_ref` arm here only handles things resolvable without a DB:

- Wire-shape: `multi=false` requires `float64` (or `nil`); `multi=true` requires `[]any` of `float64`. Reject mismatches.
- IDs are positive integers (round-trip through `uint`).
- Count vs `Min`/`Max` (treating `Max==0` as unlimited).
- Required-when-empty.

**DB-backed — new `(*PluginManager).ValidateActionEntityRefs(action, params) []ValidationError`.** Lives on `PluginManager` because the manager already holds the application context. One batched query **per `entity_ref` param** (not one per entity type), because per-param filter overrides mean two resource params with different `Filters` can't share a single resource query without either over-rejecting or over-accepting:

1. Iterate `entity_ref` params present in `params`.
2. For each, resolve effective filter: per-param `Filters` if set, else inherit `action.Filters`.
3. Issue one query for that param using the appropriate search query model with `Ids = <param's IDs>` and the effective filter populated (`ContentTypes` for resources, `Categories` for groups, `NoteTypeIds` for notes — see "Backend filter additions" below).
4. Compare returned IDs to the requested set; emit a `ValidationError` per missing or filter-rejected ID, scoped to that param's `Field` name.

**Important: bypass the Lua-facing plugin DB adapter.** This validation path must NOT go through `application_context/plugin_db_adapter.go`'s `QueryNotes`/`QueryResources`/`QueryGroups`. Three reasons:

1. **Missing filter mappings.** The adapter's `buildResourceQuery` / `buildNoteQuery` / `buildGroupQuery` helpers (lines 170-237) don't map `ids`, `content_types` (plural), `categories` (plural), or `note_type_ids` (plural). They were built for Lua-facing convenience and only expose a subset of the underlying query model fields.
2. **Result cap.** `queryLimit` (line 144-155) defaults to 20 and caps at 100. A plugin author validating, say, 50 resource IDs against a tag filter could silently drop matches and produce false-rejection `ValidationError`s.
3. **Result shape.** The adapter's row maps drop fields we'd need to confirm filter pass (e.g., the resource adapter doesn't return `category_id`).

Instead, `ValidateActionEntityRefs` calls the application context's typed readers directly: `ctx.GetResources(0, len(ids), &ResourceSearchQuery{Ids: ids, ContentTypes: filter.ContentTypes})` and the analogous calls for notes/groups. The query models already support the `Ids` field with `IN (?)` scope wiring (`resource_scope.go:18-19`, `note_scope.go:31-32`, `group_scope.go:19-20`). This path also bypasses the Lua adapter's offset/limit caps.

**Chunking for large ID sets.** SQLite's variable-binding limit (~999 by default; 32766 in newer builds) constrains how many IDs `IN (?)` can take in one query. `ValidateActionEntityRefs` chunks IDs in batches of `500` per query when `len(ids) > 500`, accumulating results across batches. Realistic entity_ref payloads are far below this; the chunk threshold exists as a safety floor, not the steady-state path.

The N of the outer per-param loop is the number of `entity_ref` params on the action, which is almost always 1 and realistically capped at a handful. An optimization to batch by `(entity, effectiveFilter)` tuple is possible but not worth the complexity for v1.

**Where each runs (no duplication):**

- `GetActionRunHandler` (`server/api_handlers/action_handlers.go:77`) calls `ValidateActionParams` first, then `ValidateActionEntityRefs` (only if structural validation passed). Both responses serialize via the existing `errors` JSON shape at line 119-124. This is the single point of DB validation.
- `RunAction` and `RunActionAsync` (the engine entrypoints called from the handler and from internal callers) keep their existing `ValidateActionParams` call as defense-in-depth for structural correctness, but **do not** call `ValidateActionEntityRefs` — they trust their inputs. This avoids per-job DB hits for async fan-out (one bulk `entity_ids=[1,2,...,50]` request validates entity refs once at HTTP entry, not 50 times).
- The `PluginActionRunner` interface in `action_handlers.go:15-17` doesn't need changes; the handler reaches `ValidateActionEntityRefs` via the same `ctx.PluginManager()` it already uses.

**Contract:** when a Lua handler reads `ctx.params.<entity_ref_name>`, every ID in the result is guaranteed to exist and match the filter at the time of the HTTP request. (No re-check at handler call time — TOCTOU between validation and handler execution is accepted, same as for any other param.)

### `parseActionTable` (`plugin_system/actions.go`)

When parsing a Lua param table with `type == "entity_ref"`:

- Require `entity` field; reject if absent or not in `{"resource", "note", "group"}`.
- Parse `multi` as bool; default false.
- Parse `filters` (if present) using the same parser used for the action-level `filters` block, returning an `*ActionFilter`.
- Validate `default` against the four allowed strings (or empty).

Errors surface as Lua load-time errors so plugin authors see them immediately.

### `show_when` Array-Value Extension

The fal.ai use case requires showing the param for any of three model values. `show_when` today (`actions.go:17-29`, `pluginActionModal.js:51-57`) is AND-only over key=value scalar pairs.

We extend the comparison to accept arrays as any-of:

```lua
show_when = { model = {"flux2", "flux2pro", "nanobanana2"} }
-- is true iff formValues.model is in the array
```

Backward-compatible: scalar values use equality (existing path); array values use `includes`. Implementation is one branch in `pluginActionModal.js:51-57` (`isParamVisible`).

`show_when` remains purely client-side per the comment at `actions.go:13-16`. The server doesn't evaluate it. The frontend strips hidden params via `visibleParams` (`pluginActionModal.js:77-87`); for non-required params, the server then skips them via the `!exists` early-return at `action_executor.go:79-82`.

**Constraint: `required = true` cannot be combined with `show_when`.** The required check at `action_executor.go:71-77` runs *before* the absent-skip, so a stripped-because-hidden required param fails server validation as missing. `parseActionTable` enforces this: any param that declares both `required = true` and `show_when` is rejected at plugin load time with a clear error message. The fal.ai `extra_images` param uses `show_when` and is non-required, so it works under this constraint. Lifting the constraint would require teaching the server to evaluate `show_when` against the submitted form values — deferred until a real plugin needs it.

### Frontend Modal (`pluginActionModal.js` + `templates/partials/pluginActionModal.tpl`)

**On modal open**, for each `entity_ref` param:

1. Resolve effective filter (per-param > action-level).
2. Resolve initial value (`default` is authoritative — bulk-selection is consulted only when `default` is `"selection"` or `"both"`):
   - `default = "trigger"` → `[entityIds[0]]` (single-entity placement) or `[...entityIds]` (bulk placement)
   - `default = "selection"` → IDs from the global bulk-selection store, if any, else `[]`
   - `default = "both"` → union of trigger + bulk-selection (deduplicated, trigger first)
   - `default = ""` → `[]`

   **Entity-type compatibility.** Trigger and bulk-selection IDs are only meaningful when their source entity type matches `param.entity`. The trigger source is `action.entityType` (the modal's `entityType` field). The bulk-selection store's entity type is exposed by the existing `bulkSelection` Alpine store (current page entity). When `param.entity !== sourceEntityType`, the contributing source resolves to empty (silent — the picker just opens with fewer or no prefills). Worked example: an action on a resource page declares `entity_ref entity = "group"`. With `default = "trigger"`, the trigger entity (a resource ID) is incompatible, so the picker opens empty. With `default = "both"` and a bulk-selection of group cards on a different page (rare in practice), the selection contributes; otherwise both contribute nothing.

   This is a runtime resolution, not a parse-time error: a plugin author legitimately may want `default = "trigger"` on a `group` entity_ref because the action is also placed on group pages where it works, while gracefully degrading on resource pages.

3. Store the array in `formValues[param.name]`. For `multi=false`, take the first element only and store a single number (or `null`) instead. A plugin author setting `default = "both"` with `multi=false` is a configuration error, rejected at `parseActionTable` time.

**Rendering** (`pluginActionModal.tpl`) gets a new `x-if` arm for `param.type === 'entity_ref'`:

- Chip list of currently-selected entities (each chip shows thumbnail + name fetched lazily via the existing detail endpoint, with a `×` remove button).
- "Add resources" / "Add notes" / "Add groups" button (label depends on `param.entity`).
- Effective-filter badge showing what's being filtered (e.g., "Showing only: image/jpeg, image/png") so users understand why some entities don't appear.
- For `multi=false`, picking a new entity replaces the current selection.

Clicking the add button:

```js
const existing = param.multi
    ? (formValues[param.name] || [])
    : (formValues[param.name] != null ? [formValues[param.name]] : []);

Alpine.store('entityPicker').open({
    entityType: param.entity,
    existingIds: existing,
    lockedFilters: effectiveFilters,        // new option (see Picker extension below)
    multiSelect: param.multi,                // tells the picker whether to allow >1 selection
    onConfirm: ids => {
        if (param.multi) {
            // Append, dedupe, preserve order
            const seen = new Set(formValues[param.name] || []);
            const next = [...(formValues[param.name] || [])];
            for (const id of ids) {
                if (!seen.has(id)) { seen.add(id); next.push(id); }
            }
            formValues[param.name] = next;
        } else {
            // Single-select: replace; store scalar (or null if cleared)
            formValues[param.name] = ids.length > 0 ? ids[0] : null;
        }
    },
});
```

Note: `entityPicker` already enforces single-vs-multi via its own confirm button (today it always allows multi-select; a `multiSelect: false` mode that auto-confirms on click of the first item is a small additional change to `entityPicker.js`).

**Validation** (client-side, before submit): required, min, max enforced the same way other params are. The existing `visibleParams` pruning at `pluginActionModal.js:77-87` automatically strips `entity_ref` fields hidden by `show_when` — no changes needed.

### Picker Extension (`src/components/picker/entityPicker.js` + `entityConfigs.js`)

Three changes:

1. **Make the picker DOM globally available.** Today `templates/partials/entityPicker.tpl` is included only from `templates/partials/blockEditor.tpl:920`, so on resource/group/note pages where the action modal lives, opening the `entityPicker` Alpine store would mutate state with no DOM to render against. Add `{% include "partials/entityPicker.tpl" %}` to the global base layout (`templates/layouts/base.tpl`) next to the existing `pluginActionModal.tpl` include, so any page that renders the action modal also renders the picker overlay. Verify after the change that there's exactly one picker DOM tree per page (no duplication when blockEditor is also present).

2. **`open()` accepts a new `lockedFilters` option** (separate from user-tunable `filters`). Locked filters are not exposed in the filter UI and are appended to the search URL via the entity config's `searchParams` builder. The `searchParams` signature becomes `(query, filters, lockedFilters, maxResults)`.

   - `resource` config: translate `lockedFilters.content_types` (`string[]`) into a repeated `ContentTypes` query param. The existing scalar `ContentType` (LIKE) param stays unused by the picker. See "Backend filter additions" below — the new server-side `ContentTypes` field is required because the existing scalar can't express a multi-value allowlist.
   - `group` config: translate `lockedFilters.category_ids` (`uint[]`) into a repeated `Categories` query param. The `Categories []uint` field already exists on `GroupSearchQuery` (`models/query_models/group_query.go:31`) and is wired in `group_scope.go:174-175` with `IN ?` semantics — no backend change needed.
   - `note` config (new): translate `lockedFilters.note_type_ids` (`uint[]`) into a repeated `NoteTypeIds` query param. Today `NoteSearchQuery` only has the scalar `NoteTypeId` (`models/query_models/note_query.go:38`); see "Backend filter additions" below for the plural addition.

3. **New `note` entity config** in `entityConfigs.js`:
   - `entityType: 'note'`, `searchEndpoint: '/v1/notes'`
   - Search by name; user-tunable `tags` filter (matching the resource config pattern)
   - No tabs (notes don't have a "this note's notes" relationship)
   - `renderItem: 'noteCard'` — a new render branch in `entityPicker.tpl`. Today the template only has `renderItem === 'thumbnail'` (resource grid, line 151) and `renderItem === 'groupCard'` (line 196), and `groupCard` is hard-coded with group-specific labels (`'Unnamed Group'` fallback, `Owner.Name` breadcrumb, `ResourceCount`/`NoteCount` metadata, `Category` badge). A `note` config cannot reuse `groupCard` without showing wrong/empty fields. Two paths:
     - **(a) Add a `noteCard` render branch (chosen for v1).** Mirrors `groupCard`'s structure but with note-specific bits: `'Unnamed Note'` fallback, `NoteType.Name` instead of `Category.Name` in the badge, `ResourceCount` for "N resources" if the note list endpoint returns it, no `NoteCount` line. Smaller, lower-risk change.
     - **(b) Refactor `groupCard` into a generic `entityCard` driven by config-supplied label/metadata accessors.** Cleaner long-term but a refactor of working code; deferred.

   Implementation chooses (a) and leaves (b) as a noted future cleanup.

### Backend filter additions

Each entity-type filter that `ValidateActionEntityRefs` and the picker need has a different starting point in the existing query layer:

- **Resources — needs new field.** `ResourceSearchQuery.ContentType` is a single string compared with an escaped contains-LIKE (`resource_scope.go:106-107`); it can't express a multi-value exact-match allowlist. Add:
  - `ResourceSearchQuery.ContentTypes []string` (`models/query_models/resource_query.go:48`).
  - New scope branch in `resource_scope.go` alongside the existing scalar one:
    ```go
    if len(query.ContentTypes) > 0 {
        db = db.Where("content_type IN ?", query.ContentTypes)
    }
    ```
  - Resource list HTTP handler (`server/api_handlers/resource_handlers.go`): bind repeated `ContentTypes` query params. The existing repeated fields (e.g., `Tags`, `Groups`) on the same handler are the reference for binding shape — confirm exact `gorilla/schema` glue during implementation.

- **Groups — already supported.** `GroupSearchQuery.Categories []uint` exists (`group_query.go:31`) and is wired in `group_scope.go:174-175` with `IN ?`. No new field, no scope change. The picker's `lockedFilters.category_ids` translates to a repeated `Categories` query param (using gorilla/schema's default repeated-field binding). `ValidateActionEntityRefs` populates `GroupSearchQuery.Categories` directly from the effective filter's `category_ids`.

- **Notes — needs new field.** `NoteSearchQuery.NoteTypeId` (`note_query.go:38`) is scalar; multi-value is not expressible. Add:
  - `NoteSearchQuery.NoteTypeIds []uint`.
  - New scope branch in the corresponding note scope file:
    ```go
    if len(query.NoteTypeIds) > 0 {
        db = db.Where("notes.note_type_id IN ?", query.NoteTypeIds)
    }
    ```
  - Note list HTTP handler: bind repeated `NoteTypeIds` query params.

These additions (resource `ContentTypes` and note `NoteTypeIds`) are useful beyond this feature — multi-value bulk filtering is a common UX request — so they're not feature-tax code. The naming matches the action-filter side of the equation: `ActionFilter.ContentTypes`, `ActionFilter.NoteTypeIDs` (preserving the existing `IDs` capitalization on the action side).

### Frontend filter plumbing

For the picker to inherit the action's `filters`, the action filters must reach the modal. Today `pluginActionModal.open()` (`pluginActionModal.js:14-44`) copies a fixed list of fields from the event detail and **omits `filters`**. Two of the three launchers already include filters (the two `.tpl` launchers spread `{{ action|json }}` via `Object.assign`), but `cardActionMenu.runAction()` (`cardActionMenu.js:6-21`) hand-picks fields and drops `filters` and `bulk_max`.

Required changes:

- **`pluginActionModal.open()`**: add `filters: detail.filters` to the copied action object. Without this, even launchers that send filters can't make them available to `entity_ref` rendering.
- **`cardActionMenu.runAction()`**: include `filters: action.filters` (and, while we're touching it, `bulk_max: action.bulk_max`) in the dispatched `detail` object so card-launched actions get the same data as sidebar/bulk-launched ones.
- The two `.tpl` launchers (`pluginActionsSidebar.tpl`, `pluginActionsBulk.tpl`) already pass filters via `{{ action|json }}` and need no changes.

### fal.ai Wiring (`plugins/fal-ai/plugin.lua`)

Refactor: extract the data-URI building from `process_image` into a helper `build_data_uri(resource_id) -> string`.

The `edit` action gains the new param:

```lua
{
    name = "extra_images", type = "entity_ref", entity = "resource",
    label = "Additional Images", multi = true,
    min = 0, max = 9,
    default = "trigger",
    description = "Reference images sent alongside the source. Only Flux 2, Flux 2 Pro, and Nano Banana 2 use these.",
    show_when = { model = {"flux2", "flux2pro", "nanobanana2"} },
},
```

Note: `default = "trigger"` makes the trigger entity an explicit member of `extra_images`, so the handler does NOT prepend it separately. The `flux2`/`flux2pro`/`nanobanana2` branches in `build_request` change from:

```lua
local payload = { image_urls = {data_uri}, prompt = prompt }
```

to:

```lua
local payload = { image_urls = build_image_urls(extra_ids), prompt = prompt }
```

where `build_image_urls(ids)` calls `build_data_uri(id)` for each ID. The handler in `make_handler` reads `params.extra_images` and passes it through.

Single-image models (`flux1dev` and the non-edit actions: colorize, upscale, restore, vectorize) are unchanged — the `entity_ref` param is hidden by `show_when` for them.

### Plugin Documentation

Update `docs-site/docs/features/plugin-actions.md`:

- Add `entity_ref` row to the param-types table.
- New section "Entity Reference Parameters" with the schema, the `default` semantics, and the multi-resource fal.ai example.
- Note the `show_when` array-value extension.

Update fal.ai's own `mah.doc` for the `edit` action with the new param and updated multi-image notes.

## Testing

- **Go unit tests** in `plugin_system/actions_test.go` and `action_executor_test.go`:
  - `parseActionTable`: valid `entity_ref` with each entity type; invalid `entity` value; missing `entity`; `multi` parsing; `filters` override parsing; `default = "both"` with `multi = false` rejected; `required = true` combined with `show_when` rejected (the constraint from the show_when section).
  - `ValidateActionParams` (pure-structural arm): `multi=false` rejects array; `multi=true` accepts empty array when `min=0`; min/max violations; required-when-empty; non-positive ID rejected; non-numeric value rejected. (No DB hit.)
  - `ValidateActionEntityRefs` (DB-backed): rejection of non-existent IDs; rejection of IDs that fail the inherited action filter; rejection of IDs that fail a per-param filter override; **two `entity_ref` params on the same action with different per-param filters validate independently and each enforces its own filter** (regression for the per-param batching decision); success path returns no errors. Backed by an in-memory test DB with seeded resources/notes/groups.
- **Go API test** in `server/api_tests/plugin_api_test.go`: POST `/v1/jobs/action/run` with `entity_ref` params, including content-type filter rejection, missing-entity rejection, and successful multi-resource flow. Also assert that bulk fan-out (`entity_ids = [1,2,...,N]`) validates entity refs **once** (not N times) — verifiable by counting query log lines or by injecting a counting wrapper around the entity reader during the test.
- **Backend filter unit tests** in `models/database_scopes` or equivalent: `ResourceSearchQuery.ContentTypes` produces the expected `IN (?)` SQL and matches only listed types; coexists with the existing scalar `ContentType` LIKE filter without conflict.
- **Frontend filter-plumbing test** (Vitest or equivalent if frontend tests exist; otherwise covered by E2E): `cardActionMenu.runAction` dispatches `filters` and `bulk_max` in the event detail; `pluginActionModal.open()` reads `filters` from the detail and exposes it on `this.action`.
- **E2E (Playwright)** in `e2e/tests/plugins/`:
  - Open AI Edit modal on an image resource with `model=flux2`, click "Add resources", pick two from picker, verify chips render and submit succeeds against a mocked fal.ai endpoint (or a fixture endpoint hosted by the test server). If mocking fal.ai is impractical, gate the test behind a `FAL_API_KEY` env var and skip in CI.
  - Verify `entity_ref` field is hidden when `model=clarity` (single-image upscaler).
  - Verify bulk-selection prefill: select 3 cards, click action that uses `entity_ref` with `default = "selection"`, verify chips already show those 3.
  - Verify filter rejection: try to select a non-image resource, picker doesn't show it.
  - Verify entity-type prefill incompatibility: an action on a resource page with an `entity_ref entity = "group" default = "trigger"` opens with an empty picker (the resource trigger ID is not used as a group ID).
  - Verify chunking: a payload with 600 IDs validates correctly without falling off the SQLite parameter limit (seed enough resources to make this realistic, or test with mocked DB).
- **E2E (CLI)**: the `mr` CLI does not currently expose an action-run command (`cmd/mr/commands/plugins.go` only has enable/disable/settings/purge-data). CLI E2E for `entity_ref` params is therefore out of scope for this design. If a `mr plugin action run` command is added later, its argument parsing should accept entity-ref params via a repeatable flag (e.g., `--ref extra_images=1,2,3`).

Per CLAUDE.md, run the full Go unit suite, browser E2E in parallel with the existing CLI E2E suite (which still has unrelated coverage), and the Postgres test suite before final commit.

## Migration & Compatibility

- **Pure addition.** Existing plugins, schemas, stored data, and stored jobs are unaffected. `entity_ref` only exists in plugins that opt in.
- **Validation arm only triggers** for params declared as `type = "entity_ref"`. Unknown types still fall through (no-op) per the existing switch in `ValidateActionParams`.
- **`show_when` array extension** is backward-compatible: scalar values use the existing equality path; array values activate the new any-of branch.
- **Picker `lockedFilters`** is additive; existing `entityPicker.open()` callers (in `compareView.js`) continue to work without supplying it.

## Open Questions / Investigation During Implementation

- Exact `schema` struct-tag plumbing for the new repeated `ContentTypes` query param on the resource list handler — confirm during implementation by inspecting `server/api_handlers/resource_handlers.go` and the existing `Tags`/`Groups` repeated-field handling for the same handler.
- Whether fal.ai HTTP can be mocked at the transport layer for E2E tests, or whether E2E for fal.ai-specific flows must be gated behind a real API key.
- Whether the `entityPicker` chip render needs a thumbnail-loading helper or whether the existing entity config provides enough metadata in search results to render chips without an extra fetch per chip.
- Whether `entityPicker.js` needs broader refactoring to support `multiSelect: false` (auto-confirm on first click) vs. just adding the option to the existing flow — confirm during implementation.

## Out of Scope

- Tag/category as `entity` types.
- MRQL query as input source.
- Drag-and-drop selection.
- Per-model max-image enforcement at the framework level.
- Auto-hydration of entity metadata on the wire (deferred to "add later if needed" per Q5).
