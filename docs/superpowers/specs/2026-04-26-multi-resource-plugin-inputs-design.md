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

**DB-backed — new `(*PluginManager).ValidateActionEntityRefs(action, params) []ValidationError`.** Lives on `PluginManager` because the manager already holds the application context (which exposes resource/note/group readers via `mah.db.*`). For each `entity_ref` param present in `params`:

1. Resolve effective filter: per-param `Filters` if set, else inherit `action.Filters`.
2. Batched existence + filter check using a single `query_*` call per entity type with `Ids` and the appropriate filter set (`ContentTypes` for resources — see "Backend filter additions" below; `CategoryIDs` for groups; `NoteTypeIDs` for notes).
3. Compare returned IDs to the requested set; emit a `ValidationError` per missing or filter-rejected ID.

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

`show_when` is purely client-side per the comment at `actions.go:13-16` ("plumbed verbatim to the frontend, which interprets it identically"). The server doesn't evaluate it — the frontend strips hidden params via `visibleParams` (`pluginActionModal.js:77-87`) and the server treats stripped params as absent (skipped by the `!exists` early-return in `ValidateActionParams`). No Go-side change is required for the array-value extension.

### Frontend Modal (`pluginActionModal.js` + `templates/partials/pluginActionModal.tpl`)

**On modal open**, for each `entity_ref` param:

1. Resolve effective filter (per-param > action-level).
2. Resolve initial value (`default` is authoritative — bulk-selection is consulted only when `default` is `"selection"` or `"both"`):
   - `default = "trigger"` → `[entityIds[0]]` (single-entity placement) or `[...entityIds]` (bulk placement)
   - `default = "selection"` → IDs from the global bulk-selection store, if any, else `[]`
   - `default = "both"` → union of trigger + bulk-selection (deduplicated, trigger first)
   - `default = ""` → `[]`
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

   - `resource` config: translate `lockedFilters.content_types` (a `string[]`) into a repeated `ContentTypes` query param (see "Backend filter additions" below for the new server-side support — the existing scalar `ContentType` LIKE param can't express a multi-value allowlist).
   - `group` config: translate `lockedFilters.category_ids` into repeated `categoryId` params (the endpoint already accepts one; widen to repeated).
   - `note` config (new): handles `lockedFilters.note_type_ids`.

3. **New `note` entity config** in `entityConfigs.js`:
   - `entityType: 'note'`, `searchEndpoint: '/v1/notes'`
   - Search by name; user-tunable `tags` filter (matching the resource config pattern)
   - No tabs (notes don't have a "this note's notes" relationship)
   - Card-style render

### Backend filter additions

Required so `lockedFilters.content_types` actually constrains the picker (today `ResourceSearchQuery.ContentType` is a single string compared with an escaped contains-LIKE in `models/database_scopes/resource_scope.go:106-107` — it cannot express the fal.ai action's multi-value image MIME allowlist):

- **`ResourceSearchQuery`** (`models/query_models/resource_query.go:48`) gains a `ContentTypes []string` field for an exact-match `IN (?)` allowlist. Existing scalar `ContentType` (LIKE) remains, unchanged.
- **`resource_scope.go`** gets a new branch alongside the existing scalar one:
  ```go
  if len(query.ContentTypes) > 0 {
      db = db.Where("content_type IN ?", query.ContentTypes)
  }
  ```
- **Resource list HTTP handler** (`server/api_handlers/resource_handlers.go`): parse repeated `ContentTypes` query params (e.g., `?ContentTypes=image/jpeg&ContentTypes=image/png`) into the new field. Confirm exact handler/binding plumbing during implementation; the existing scalar field uses `schema` struct-tag binding so the addition should be straightforward.
- Equivalent additions for groups (`CategoryIDs []uint` on the search query if not already present) and notes (`NoteTypeIDs []uint` on the search query) so the filter logic in `ValidateActionEntityRefs` and the picker share the same backend mechanism.

These additions are also useful outside this feature — bulk filtering by multiple content types is a common UX request — so they're not "feature-tax" code.

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
  - `parseActionTable`: valid `entity_ref` with each entity type; invalid `entity` value; missing `entity`; `multi` parsing; `filters` override parsing; `default = "both"` with `multi = false` rejected.
  - `ValidateActionParams` (pure-structural arm): `multi=false` rejects array; `multi=true` accepts empty array when `min=0`; min/max violations; required-when-empty; non-positive ID rejected; non-numeric value rejected. (No DB hit.)
  - `ValidateActionEntityRefs` (DB-backed): rejection of non-existent IDs; rejection of IDs that fail the inherited action filter; rejection of IDs that fail a per-param filter override; success path returns no errors. Backed by an in-memory test DB with seeded resources/notes/groups.
- **Go API test** in `server/api_tests/plugin_api_test.go`: POST `/v1/jobs/action/run` with `entity_ref` params, including content-type filter rejection, missing-entity rejection, and successful multi-resource flow. Also assert that bulk fan-out (`entity_ids = [1,2,...,N]`) validates entity refs **once** (not N times) — verifiable by counting query log lines or by injecting a counting wrapper around the entity reader during the test.
- **Backend filter unit tests** in `models/database_scopes` or equivalent: `ResourceSearchQuery.ContentTypes` produces the expected `IN (?)` SQL and matches only listed types; coexists with the existing scalar `ContentType` LIKE filter without conflict.
- **Frontend filter-plumbing test** (Vitest or equivalent if frontend tests exist; otherwise covered by E2E): `cardActionMenu.runAction` dispatches `filters` and `bulk_max` in the event detail; `pluginActionModal.open()` reads `filters` from the detail and exposes it on `this.action`.
- **E2E (Playwright)** in `e2e/tests/plugins/`:
  - Open AI Edit modal on an image resource with `model=flux2`, click "Add resources", pick two from picker, verify chips render and submit succeeds against a mocked fal.ai endpoint (or a fixture endpoint hosted by the test server). If mocking fal.ai is impractical, gate the test behind a `FAL_API_KEY` env var and skip in CI.
  - Verify `entity_ref` field is hidden when `model=clarity` (single-image upscaler).
  - Verify bulk-selection prefill: select 3 cards, click action that uses `entity_ref` with `default = "selection"`, verify chips already show those 3.
  - Verify filter rejection: try to select a non-image resource, picker doesn't show it.
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
