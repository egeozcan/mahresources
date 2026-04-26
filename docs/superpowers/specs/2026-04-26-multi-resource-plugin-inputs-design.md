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

`ValidateActionParams` gains an `entity_ref` arm that handles the structural cases (type assertion to `float64` / `[]any`, ID > 0, count vs `Min`/`Max`). A new helper, `validateEntityRefIDs(ctx, entity, ids, effectiveFilter)`, runs after structural validation and:

1. Resolves the effective filter: per-param `Filters` if set, else inherits `action.Filters`.
2. Looks up all referenced entities in one batched query (using existing `query_resources` / `query_notes` / `query_groups` plumbing — keys: `id IN (?)`) to confirm existence.
3. For each entity, checks the filter using the same comparison logic as `actionMatchesFilters` (`actions.go:274`): content_type for resources, category_id for groups, note_type_id for notes.
4. Returns a `ValidationError` per missing or filter-rejected ID.

This runs before the Lua handler is invoked, so handlers can trust every ID in `ctx.params.<name>` exists and matches the filter.

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
Alpine.store('entityPicker').open({
    entityType: param.entity,
    existingIds: formValues[param.name] || [],
    lockedFilters: effectiveFilters,    // new option (see Picker extension below)
    onConfirm: ids => formValues[param.name] = [...formValues[param.name], ...ids],
});
```

**Validation** (client-side, before submit): required, min, max enforced the same way other params are. The existing `visibleParams` pruning at `pluginActionModal.js:77-87` automatically strips `entity_ref` fields hidden by `show_when` — no changes needed.

### Picker Extension (`src/components/picker/entityPicker.js` + `entityConfigs.js`)

Two changes:

1. **`open()` accepts a new `lockedFilters` option** (separate from user-tunable `filters`). Locked filters are not exposed in the filter UI and are appended to the search URL via the entity config's `searchParams` builder. The searchParams signature becomes `(query, filters, lockedFilters, maxResults)`.

   - `resource` config: extend `searchParams` to translate `lockedFilters.content_types` into the resource list endpoint's MIME filter param (the exact param name — `Mime` or similar — to be confirmed during implementation by inspecting `server/api_handlers/resource_handlers.go`).
   - `group` config: translate `lockedFilters.category_ids` into repeated `categoryId` params.
   - `note` config (new): handles `lockedFilters.note_type_ids`.

2. **New `note` entity config** in `entityConfigs.js`:
   - `entityType: 'note'`, `searchEndpoint: '/v1/notes'`
   - Search by name; user-tunable `tags` filter (matching the resource config pattern)
   - No tabs (notes don't have a "this note's notes" relationship)
   - Card-style render

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
  - `parseActionTable`: valid `entity_ref` with each entity type; invalid `entity` value; missing `entity`; `multi` parsing; `filters` override parsing.
  - `ValidateActionParams`: `multi=false` rejects array; `multi=true` accepts empty array when `min=0`; min/max violations; required-when-empty; rejection of non-existent IDs; rejection of IDs that fail the filter.
- **Go API test** in `server/api_tests/plugin_api_test.go`: POST `/v1/jobs/action/run` with `entity_ref` params, including content-type filter rejection, missing-entity rejection, and successful multi-resource flow.
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

- Exact name of the resource list endpoint's MIME filter param (`Mime` vs `ContentType` vs other) — confirm by reading `server/api_handlers/resource_handlers.go` during implementation.
- Whether fal.ai HTTP can be mocked at the transport layer for E2E tests, or whether E2E for fal.ai-specific flows must be gated behind a real API key.
- Whether the `entityPicker` chip render needs a thumbnail-loading helper or whether the existing entity config provides enough metadata in search results to render chips without an extra fetch per chip.

## Out of Scope

- Tag/category as `entity` types.
- MRQL query as input source.
- Drag-and-drop selection.
- Per-model max-image enforcement at the framework level.
- Auto-hydration of entity metadata on the wire (deferred to "add later if needed" per Q5).
