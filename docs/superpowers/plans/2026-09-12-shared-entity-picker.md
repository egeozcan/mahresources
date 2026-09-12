# Shared Entity Picker Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Evolve the existing entity-picker dialog into a paginated property browser available beside every entity selector, with category/type-owned result templates and CSS.

**Architecture:** Preserve the selector core as the authority for field values; add browse metadata and an atomic confirmation bridge. A separate bounded browse service uses existing query scopes, while one dialog session manages nested filter steps and pending additions. Render result content server-side under the requesting principal without making autocomplete load rich cards.

**Tech Stack:** Go, GORM, SQLite/PostgreSQL, Pongo2/shortcodes, TypeScript/JavaScript, Alpine.js, Tailwind, Vitest and Playwright.

**Spec:** `docs/superpowers/specs/2026-09-12-shared-entity-picker-design.md`

## Global Constraints

- “A picker session has exactly one entity type, inherited from its originating field.”
- “A page holds at most 50 results.”
- “Exact total counts are not required.”
- “There is no ‘select every matching entity’ operation.”
- “The picker never writes associations directly.”
- “Confirm replaces the field value atomically.”
- “An empty template uses the default result, never `CustomSummary`.”
- “No new promise of Alpine component initialization inside result markup is introduced.”
- “Client eligibility is not authorization.”
- Preserve all ten catalog types: category, group, note, noteType, query, relationType, resource, resourceCategory, series and tag.
- Add exactly `CustomEntityPickerResult` and `CustomEntityPickerResultCSS` to Category, NoteType and ResourceCategory.
- Keep the existing autocomplete `SelectorSource.search` contract unchanged.
- Use the caller's DB/context per call; never retain an unscoped singleton DB in a browser service.
- No new creation UI, embedded default media player, query-wide selection, CSS sandbox, or mixed-type selection.
- Tests run only against ephemeral data. Generated JS assets are committed with source changes. Run CSS scan separately from template-walk tests.
- One writer per checkout. Reviewers are read-only. Check the checkout before starting; the planning baseline is `e01e6f15` plus this documentation commit.

---

## Execution and file ownership map

Read `CLAUDE.md`, `docs/lessons.md`, the spec and
`docs/architecture/selector-architecture.md` before execution. Follow relevant
lesson links about focus, Alpine teardown and registry notification timing.
Create an isolated execution worktree using the worktree skill; do not assume a
previous scout's clean status still holds.

| Unit | Files and responsibility |
| --- | --- |
| Slot storage/tooling | Existing carrier models/DTOs/writers, archive round trips, template editor/tooling and CLI flags |
| Browse query | New `models/query_models/entity_picker_query.go`, `contracts/entity_picker.go`, `application_context/entity_picker_context.go`: bounded, scoped filter execution, no HTML |
| Result rendering/API | New `server/template_handlers/entity_picker_renderer.go`, `templates/partials/entityPickerResult.tpl`, `server/api_handlers/entity_picker_handlers.go`: content rendering and response envelope |
| Browse transport/types | New `src/selector/entityBrowseTypes.ts`, `httpEntityBrowseSource.ts`: typed requests/results, validation and cancellation |
| Dialog state | New `src/components/picker/pickerSession.ts`: nested steps, pending selections, pagination and request generations; existing `entityPicker.js` becomes an Alpine adapter |
| Field integration | Existing profiles, field adapter, standalone mount and tag editor; new `src/selector/entityBrowseIntegration.ts`: origin validation and one atomic commit |
| Filter UI | New `src/components/picker/entityFilterConfigs.ts` plus `templates/partials/entityPickerFilters.tpl`; existing picker markup owns the dialog shell |
| Tests/docs | Unit, scoped API, template, archive/CLI, browser and accessibility regressions; picker/selector/custom-template documentation |

Tasks 1–3 establish the server contract. Tasks 4–6 establish the client contract.
Task 7 connects the UI. Task 8 covers all existing consumers and regressions.
Task 9 is integration verification and review. Each task ends with its own
red/green evidence and commit; no independent writers mutate these shared files.

## Task 1: Persist and expose the custom result slots

**Files:**
- Modify: `models/category_model.go`, `models/note_type_model.go`, `models/resource_category_model.go`.
- Modify: `models/query_models/category_query.go`, `models/query_models/note_query.go`, `models/query_models/resource_category_query.go`.
- Modify: `application_context/category_context.go`, `application_context/note_context.go`, `application_context/resource_category_context.go`, `application_context/crud_factories.go`, `application_context/plugin_db_adapter.go`.
- Modify: `archive/manifest.go`, `groupio/export_context.go`, `groupio/apply_import.go`.
- Modify: `templates/createCategory.tpl`, `templates/createNoteType.tpl`, `templates/createResourceCategory.tpl`.
- Modify: `application_context/template_generation_prompt.go`, `server/api_handlers/template_generation_api_handlers.go`, `server/api_handlers/handler_factory.go` and `server/api_handlers/note_api_handlers.go` where slots are explicitly copied/enumerated.
- Modify: `cmd/mr/commands/template_slot_flags.go`, associated `categories_help`, `note_types_help`, `resource_categories_help` Markdown, `docs-site/docs/features/custom-templates.md`.
- Create tests: `application_context/entity_picker_slots_test.go`, `groupio/entity_picker_slots_test.go`.
- Extend tests: `server/api_handlers/template_slot_coverage_test.go`, `application_context/template_generation_test.go`.

**Interfaces:** Produces the two string fields on all three carrier models and
request DTOs. Archive fields use `custom_entity_picker_result` and
`custom_entity_picker_result_css`; unknown/missing fields remain compatible with
schema version 1. CLI flags are `--custom-entity-picker-result` and
`--custom-entity-picker-result-css` and inherit existing file-input conventions.

- [x] Add failing persistence and round-trip tests using the existing create/edit context and archive fixtures. Execution used JSON create/edit/reload assertions instead of the proposed reflection-only test, so a dropped mapping fails through observable behavior. Assert actual stored values, not just JSON field presence. Include clearing previously nonempty slots.

```go
func TestEntityPickerSlotsExist(t *testing.T) {
    carriers := []any{models.Category{}, models.NoteType{}, models.ResourceCategory{},
        query_models.CategoryCreator{}, query_models.CategoryEditor{},
        query_models.NoteTypeEditor{}, query_models.ResourceCategoryCreator{},
        query_models.ResourceCategoryEditor{}}
    for _, carrier := range carriers {
        typ := reflect.TypeOf(carrier)
        for _, name := range []string{"CustomEntityPickerResult", "CustomEntityPickerResultCSS"} {
            f, ok := typ.FieldByName(name)
            if !ok || f.Type.Kind() != reflect.String {
                t.Errorf("%s lacks string %s", typ.Name(), name)
            }
        }
    }
}
```

Import `reflect`, `testing`, `mahresources/models` and
`mahresources/models/query_models` in the test. Embedded creator fields remain
visible through `FieldByName`, so this checks both create and edit DTOs.

- [x] Run `go test --tags 'json1 fts5' ./application_context -run TestEntityPickerSlots -count=1`; record the missing-field failure.
- [x] Add the paired fields beside existing `CustomMRQLResult` fields and copy them at every explicit carrier mapping. Use this field shape:

```go
CustomEntityPickerResult    string `gorm:"type:text"`
CustomEntityPickerResultCSS string `gorm:"type:text"`
```

Do not blindly append picker CSS to `TemplateCSS()`: that currently gathers all
slots for ordinary pages. Task 3 emits only shared CSS plus this slot in the
picker; picker-specific CSS should not newly leak onto unrelated list pages.

- [x] Add editor fieldsets using `data-template-cluster="CustomEntityPickerResult"`, the existing generation controls, and two code-editor inputs. Add the slot to server-only template-generation guidance (native links; no Alpine promise), supported-slot checks, preview handling and CLI flag declarations.
- [x] Add archive JSON fields and all three export/import carrier mappings without changing the manifest version. Test an old archive with neither new field, and a new archive round-trip containing both.
- [x] Run `go test --tags 'json1 fts5' ./application_context ./groupio ./server/api_handlers ./cmd/mr/commands -count=1`; run `npm run build-cli && ./mr docs lint`. Fix affected documentation inventories and tests.
- [x] Commit only this slot lifecycle slice: `feat: add entity picker result template slots`.

Execution findings: the CLI category/resource-category get commands serialized
reduced response structs and dropped template fields. They now retain the raw
matching record for JSON output, with 34 passing carrier CLI cases. Additional
slot lifecycle consumers found and updated: `src/components/templateBundle.js`
and `src/components/templatePreview.js`. Preview uses only shared/picker CSS and
the picker wrapper. See `docs/todo.md` for red/green and validation evidence.
Existing custom-slot flags accept literal strings, not a new `@file` syntax;
this feature preserves that existing contract.

## Task 2: Add bounded scoped browsing and confirmation lookup

**Files:**
- Create: `models/query_models/entity_picker_query.go`, `contracts/entity_picker.go`.
- Create: `application_context/entity_picker_context.go`, `application_context/entity_picker_context_test.go`.
- Modify: `application_context/contract_checks.go`.
- Read: `models/query_models/filter_decode.go`, `models/database_scopes/`, existing `GetResources`, `GetGroups`, `GetNotes`, taxonomy readers and `SeriesCRUD`.

**Interfaces:**

```go
// models/query_models/entity_picker_query.go
const EntityPickerPageSize = 50

type EntityPickerQuery struct {
    Entity      string
    Filter      string // encoded ordinary list filters, not SQL
    Constraints string // encoded originating-field filters, intersected
    Page        int    // 1-based; omitted/zero defaults to 1
}

type EntityPickerResolveQuery struct {
    Entity      string
    Constraints string
    IDs         []uint // 1..50 distinct ids per lookup
}

// contracts/entity_picker.go
// Raw is the existing model value, not HTML or a second mutable entity record.
type EntityPickerEntity struct { ID uint; Name string; Raw any }
type EntityPickerPage struct {
    Items []EntityPickerEntity
    Page int
    HasNext bool
}
type EntityPickerReader interface {
    BrowseEntities(*query_models.EntityPickerQuery) (*EntityPickerPage, error)
    ResolvePickerEntities(*query_models.EntityPickerResolveQuery) ([]EntityPickerEntity, error)
}
```

- [x] Add red tests for 51 groups, same-name stable ordering, property filters, scoped visibility, constraints/filter intersection, query `MaxResults` bypass attempts, invalid entity/page/IDs, and resolving selected rows that were deleted or reparented. Use `newPickerTestContext(t)`, an isolated, single-connection fixture, for DB setup. Execution found the older `createTestContext(t)` shares state across unrelated tests. Seed 51 rows with a loop, asserting every Create error.

```go
page, err := ctx.BrowseEntities(&query_models.EntityPickerQuery{Entity: "group", Page: 1})
if err != nil { t.Fatal(err) }
if len(page.Items) != 50 || !page.HasNext { t.Fatalf("bad first page: %#v", page) }
next, err := ctx.BrowseEntities(&query_models.EntityPickerQuery{Entity: "group", Page: 2})
if err != nil { t.Fatal(err) }
if len(next.Items) != 1 || next.HasNext { t.Fatalf("bad next page: %#v", next) }
seen := map[uint]bool{}
for _, row := range page.Items { seen[row.ID] = true }
if seen[next.Items[0].ID] { t.Fatal("page repeated an entity") }
```

- [x] Run `go test --tags 'json1 fts5' ./application_context -run 'TestEntityPicker' -count=1` and retain red evidence.
- [x] Implement a fixed allowlisted entity-to-model/scope switch. Reuse these existing pairs:

| entity | model | scope |
| --- | --- | --- |
| resource | Resource | ResourceQuery + applyMRQLFilter |
| group | Group | GroupQuery + applyMRQLFilter |
| note | Note | NoteQuery + applyMRQLFilter |
| category | Category | CategoryQuery |
| noteType | NoteType | NoteTypeQuery |
| resourceCategory | ResourceCategory | ResourceCategoryQuery |
| tag | Tag | TagQuery |
| query | Query | QueryQuery |
| relationType | GroupRelationType | RelationTypeQuery |
| series | Series | SeriesQuery |

Decode using the real DTOs and gorilla/schema conventions. Preserve
`FillMetaQueryFromValues`; discard request-controlled `MaxResults`, offset and
sort for this first picker version. The browse order is qualified primary-key
ascending, deterministic on both engines. Do not modify ordinary list ordering.
Unknown UI keys follow existing list decoding; malformed known values error.

Use a private `pickerFilteredDB(entity, encodedFilter)` method returning the
scoped `*gorm.DB` for the model, with no presentation ordering/limit. Build both
filter and constraint predicates from `ctx.db`, then intersect independent ID
subqueries rather than concatenating maps or repeating joins with identical
aliases. This is AND, including overlapping category/type constraints. Query
callbacks must still run on the final ORM read; do not replace Find/Pluck with
Scan and lose subtree scoping.

- [x] Fetch 51 bounded identities to determine continuation; hydrate only the first 50 in one batch and restore identity order. Preload only needed tags, owner, category/type and thumbnail source associations. Do not preload all series resources, note blocks, group descendants or call per-result detail endpoints.

```go
hasNext := len(ids) > query_models.EntityPickerPageSize
if hasNext { ids = ids[:query_models.EntityPickerPageSize] }
// Hydrate these ids on the same scoped handle; omit vanished/invisible rows.
```

Calculate `(page-1)*50` with overflow checking. Negative pages, unsupported types
and resolve batches outside 1..50 fail with an explicit typed input error mapped
to HTTP 400. Zero page defaults to 1. Bind IDs and filters; table names only come
from the allowlist.

- [x] Resolve confirmation IDs through the same constraint predicate, independent of editable name/filter/page. Return surviving eligible rows in requested order; missing IDs are not replaced or silently committed by the client. Auth visibility remains server-owned even if Constraints is empty.
- [x] Add the compile-time `contracts.EntityPickerReader` assertion. Test every catalog branch and note association filtering with `Notes`, not `OwnerId`.
- [x] Run the focused tests plus `go test --tags 'json1 fts5' ./internal/arch/... ./models/...`; commit `feat: add scoped paginated entity browsing`.

Execution evidence: `Pluck`→`Scan` mutation makes the scoped pagination test
return zero visible rows instead of one; restored code passes. All picker tests
and the previously polluted tree test pass `-count=3`. Full application-context,
models, contracts and architecture suites pass. Request-time HTML/API integration
is deliberately left to Task 3.

## Task 3: Render bounded picker results and expose the HTTP contract

**Files:**
- Create: `server/template_handlers/entity_picker_renderer.go`, `templates/partials/entityPickerResult.tpl`.
- Create: `server/api_handlers/entity_picker_handlers.go`, `server/api_tests/entity_picker_test.go`.
- Create: `server/template_handlers/entity_picker_renderer_test.go`.
- Modify: `server/routes.go`, `server/routes_openapi.go`.
- Read/reuse: `server/api_handlers/mrql_render_context_test.go`, `mrql_list_render.go`, `template_preview_api_handlers.go`, `server/template_handlers/list_card_renderer.go`.
- Modify: `shortcodes/processor.go`; create `shortcodes/process_diagnostics_test.go` for structured, backward-compatible processing diagnostics.

**Interfaces:** Register authenticated `GET /v1/entity-picker` with query keys
`entity`, `filter`, `constraints`, `page`; register authenticated
`GET /v1/entity-picker/resolve` with `entity`, `constraints` and repeated `id`.
Both are ordinary read capabilities, not public/auth-exempt endpoints. Resolve
returns no rendered HTML or CSS.

```ts
// Wire format consumed by Task 4.
type BrowseValue = { ID: number; Name: string; [key: string]: unknown };
type BrowseResult = { value: BrowseValue; html: string };
type BrowseStyle = { key: string; css: string };
type BrowsePage = {
  items: BrowseResult[];
  page: number;
  hasNext: boolean;
  styles: BrowseStyle[];
  warnings: string[];
};
// resolve response: { items: BrowseValue[] }
```

Go response structs live in this handler file; query/domain types remain in
contracts/models. The handler context embeds `contracts.EntityPickerReader` and
the existing render context interfaces it actually consumes. Mount it with a
principal-bound context in `routes.go`, not a singleton passed through unchecked.

- [x] Write API red tests asserting exact envelope keys, at most 50 results, principal scoping on both routes, invalid-input 400, and inaccessible IDs omitted from resolve. Test with ordinary guest read access as well as admin.
- [x] Write renderer tests for all three carriers, long names/categories, HTML-escaped names, new-tab link attributes, scoped owner/tags, empty slot default, carrier CSS deduplication and custom failure fallback. Default content contains no checkbox: Task 7 owns selection controls outside returned HTML.

```go
// A default result's markup must preserve normal navigation.
if !strings.Contains(html, `target="_blank"`) || !strings.Contains(html, `rel="noopener noreferrer"`) {
    t.Fatal("default result must link safely to details in a new tab")
}
if strings.Contains(html, `type="checkbox"`) { t.Fatal("renderer owns content, not selection") }
```

- [x] Run `go test --tags 'json1 fts5' ./server/api_tests ./server/template_handlers -run EntityPicker -count=1`; retain the missing-handler/renderer failures.
- [x] Build a request-local renderer using `BuildMetaContextForEntity`, existing shortcode processing and the shared render-context builder. Set the same plugin access predicate, request deadline, query cache/budget and render-data cache as current server-rendered cards. Read template definitions from the stored carrier, never from caller-supplied HTML on this read endpoint.
- [x] Cache parsed templates and prepare CSS once per `entity:carrierID`. Emit `CustomCSS` then `CustomEntityPickerResultCSS`, not the carrier's all-slot CSS aggregation. For other types use the default template and verified detail routes from `server/routes.go`. Copy/sanitize model values before serialization as the existing deferred-render path does; avoid serializing full carrier template strings redundantly into every `value`. Include ID, Name and the model fields/association labels needed by existing profile consumers, not executable template source.
- [x] Make failure observable without scraping error-box HTML. If the shortcode processor lacks an error result, add a backward-compatible diagnostic variant, keeping the current `Process` output unchanged for existing callers:

```go
type ProcessResult struct { HTML string; Errors []error }
func ProcessWithDiagnostics(reqCtx context.Context, input string,
    ctx MetaShortcodeContext, renderer PluginRenderer, executor QueryExecutor) ProcessResult
```

Collect real processing errors, including nested processing, through request-local
state; `ErrPluginUnavailable` is neutral, not a rendering error. Keep the existing
Process implementation/output under regression tests. On a result error, log one
bounded warning per carrier/failure kind per request and render the default card.
Do not log full template contents or secrets. Budget exhaustion yields the
existing diagnostic plus a concise response warning. Cancellation does not become
an endless fallback/render retry.

- [x] Register both routes in runtime and OpenAPI metadata. Run the renderer/API/shortcode suites, `go run ./cmd/openapi-gen` and architecture tests. Commit `feat: serve custom entity picker result pages` including generated API artifacts.

## Task 4: Publish browse metadata and add a typed transport

**Files:**
- Create: `src/selector/entityBrowseTypes.ts`, `src/selector/httpEntityBrowseSource.ts`, `src/selector/httpEntityBrowseSource.test.ts`.
- Modify: `src/selector/entityFieldProfiles.ts`, `src/selector/tagEditorProfile.ts`, `src/selector/index.ts`, `src/components/profiledAutocompleter.js`.
- Modify: `src/components/picker/entityConfigs.js`, `templates/partials/entityPicker.tpl` to pass explicit entity metadata to existing dynamic filter profiles before the larger Task 7 UI change.
- Extend: `src/selector/entityFieldProfiles.test.ts`, `src/selector/tagEditorProfile.test.ts`, `src/components/profiledAutocompleter.test.ts`.

**Interfaces:** Reuse the wire types from Task 3; export them from
`entityBrowseTypes.ts`. Export these additional definitions:

```ts
export interface EntityBrowseSource {
  search(input: { entity: EntityProfileName; filter: string; constraints: string; page: number },
    signal: AbortSignal): Promise<BrowsePage>;
  resolve(input: { entity: EntityProfileName; constraints: string; ids: number[] },
    signal: AbortSignal): Promise<BrowseValue[]>;
}
export interface EntityBrowseMetadata {
  entity: EntityProfileName;
  multiple: boolean;
  maximum?: number; // undefined means unlimited; zero means zero
  parameters: () => Readonly<Record<string, SelectorHttpParameter>>;
  excludedKeys: () => readonly (string | number)[];
}
// Add readonly browse: EntityBrowseMetadata to EntityFieldProfile.
export function createHttpEntityBrowseSource(): EntityBrowseSource;
```

Dynamic profiles gain a required `entity: EntityProfileName` at their host call
sites; retain their current searchUrl for autocomplete compatibility. Do not
reverse-engineer entity type from plural endpoint strings. The existing factories
continue translating template `max=0` to unlimited before constructing profiles.

- [x] Write failing tests proving every profile family publishes the right entity/multiplicity/maximum, live parameter/exclusion callbacks and unchanged autocomplete endpoint. Test lean tag suggestions retain their autocomplete path but browse as `tag`.
- [x] Write transport tests for repeated values, distinct encoded filter/constraints parameters, AbortSignal forwarding, invalid JSON/envelope, non-2xx errors and resolve batches. Example request assertion:

```ts
const params = new URL(capturedURL, 'http://localhost').searchParams;
expect(params.get('filter')).toBe('Categories=2');
expect(params.get('constraints')).toBe('Categories=1');
expect(params.get('entity')).toBe('group');
```

Capture the request with this fake; restore globals after every case:

```ts
let capturedURL = '';
vi.stubGlobal('fetch', vi.fn(async (url: string) => {
  capturedURL = String(url);
  return new Response(JSON.stringify({
    items: [], page: 1, hasNext: false, styles: [], warnings: [],
  }), { status: 200, headers: { 'Content-Type': 'application/json' } });
}));
afterEach(() => vi.unstubAllGlobals());
``` A decoder must reject non-array items, invalid page or hasNext, and missing
numeric ID/string Name rather than passing broken state to selection code.

- [x] Run `npm run test:unit -- src/selector/entityFieldProfiles.test.ts src/selector/httpEntityBrowseSource.test.ts` and capture red.
- [x] Implement the metadata as part of profile construction. Implement search/resolve with `URLSearchParams`, preserving repeated inner filter values by encoding the full filter string once as an outer parameter. Use `fetch` so the host wrapper remains in effect; never interpolate HTML into executable Alpine expressions.
- [x] Re-run all selector/profile tests, `npm run build-js`, and commit `feat: expose entity browser profiles and transport` with generated assets.

## Task 5: Implement the headless nested picker session

**Files:** Create `src/components/picker/pickerSession.ts`, `src/components/picker/pickerSession.test.ts`.

**Interfaces:**

```ts
export interface PickerOpenOptions {
  browse: EntityBrowseMetadata;
  existing: readonly BrowseValue[];
  onConfirm: (values: readonly BrowseValue[]) => Promise<boolean> | boolean;
}
export interface PickerStepSnapshot {
  id: number;
  options: PickerOpenOptions;
  filter: string;
  page: number;
  items: readonly BrowseResult[];
  styles: readonly BrowseStyle[];
  warnings: readonly string[];
  pending: readonly BrowseValue[];
  hasNext: boolean;
  status: 'idle' | 'loading' | 'ready' | 'error' | 'confirming';
  error: string | null;
}
export interface PickerSnapshot { steps: readonly PickerStepSnapshot[]; isOpen: boolean }
export interface PickerSession {
  snapshot(): PickerSnapshot;
  subscribe(listener: (state: PickerSnapshot) => void): () => void;
  open(options: PickerOpenOptions): void;
  push(options: PickerOpenOptions): void;
  back(): void;
  cancel(): void;
  setFilter(filter: string, stepId?: number): void; // omitted targets current step
  setPage(page: number): void;
  toggle(value: BrowseValue): void;
  retry(): void;
  confirm(): Promise<boolean>;
  destroy(): void;
}
export function createPickerSession(source: EntityBrowseSource): PickerSession;
```

No DOM, Alpine or database behavior belongs in this unit. Debouncing occurs in
the UI adapter, but state invalidates the previous request immediately when the
filter changes. Use maps keyed by `String(ID)` to avoid numeric/string duplicates.

- [x] Write deterministic red tests with a deferred source that deliberately ignores AbortSignal. Include old responses arriving after a new query, a nested push, Back, Cancel and reopening; a stale finally block must not clear the newer loading state.
- [x] Write selection tests for multi-add, existing IDs, single explicit confirmation, remaining capacity, filtered-away pending rows, next-page persistence and rejection/exception from onConfirm. Include the skeleton below with a test source returning one page:

```ts
const onConfirm = vi.fn(() => true);
const session = createPickerSession(source);
session.open({ browse, existing: [{ ID: 1, Name: 'Existing' }], onConfirm });
session.toggle({ ID: 2, Name: 'New' });
expect(onConfirm).not.toHaveBeenCalled();
await session.confirm();
expect(onConfirm).toHaveBeenCalledWith([{ ID: 2, Name: 'New' }]);
expect(session.snapshot().isOpen).toBe(false);
```

In this test file define `source` as an `EntityBrowseSource` whose `search`
resolves `{items: [], page: 1, hasNext: false, styles: [], warnings: []}` and whose
`resolve` returns requested values. Define `browse` as group/multiple with empty
parameter/exclusion callbacks. Add a separate controlled-promise source for races.

- [x] Run `npm run test:unit -- src/components/picker/pickerSession.test.ts` and retain red.
- [x] Implement immutable published snapshots, an internal stack, independent step IDs and monotonically increasing request generations. Abort active reads on transitions. A successful response applies only if session, step and request generations still match.
- [x] Implement `toggle` with no immediate confirmation; prevent new choices at remaining capacity and keep old values. `confirm` passes only pending additions for multi and the chosen row for single; callbacks return false to retain the dialog and choices. Reject duplicate confirmation while confirming. On nested success pop only the child; on outer success close all steps. A child callback may update its parent filter while it is hidden, so target steps by identity rather than the current top step during that transition.
- [x] Implement Back/Cancel/destroy with deterministic abortion and snapshot publication. Restore a parent's retained page/results on Back; a confirmed filter change resets that parent's page and reloads it. No reopen persistence.
- [x] Re-run session tests plus existing picker tests; build assets if imported by production code in this slice; commit `feat: model nested entity picker sessions`.

## Task 6: Connect browser confirmation to existing selector ownership

**Files:**
- Create: `src/selector/entityBrowseIntegration.ts`, `src/selector/entityBrowseIntegration.test.ts`.
- Modify: `src/components/selectorFieldAdapter.js`, `src/components/profiledAutocompleter.js`, `src/selector/mountEntitySelector.js`.
- Extend: `src/components/selectorFieldAdapter.test.ts`, `src/selector/tagEditorProfile.test.ts`.
- Create: `src/selector/mountEntitySelector.test.ts` for standalone browse/disabled/teardown behavior.

**Interfaces:**

```ts
export interface EntityBrowseOrigin {
  metadata: EntityBrowseMetadata;
  getValues(): readonly BrowseValue[];
  isAvailable(): boolean;
  replace(values: readonly BrowseValue[]): void; // one non-silent core replacement
}
export function createEntityBrowseConfirmation(origin: EntityBrowseOrigin,
  source: EntityBrowseSource): {
    confirm(values: readonly BrowseValue[]): Promise<boolean>;
    destroy(): void;
  };
```

The field adapter exposes `openEntityBrowser()` and closes its autocomplete
popover before launching. Bind `replace` to
`this._replaceSelection(values, { silent: false })`, the existing adapter command
path; do not invent a `selector.replace` method (the core exposes `dispatch`). It keeps a reference to the launch element for Task 7.
The origin uses the profile's core or registry integration method, never assigns
`selectedResults`. Standalone mount disabled state must be visible to
`isAvailable`, not merely a CSS class/pointer-events rule.

- [x] Add red tests that browser multi-add generates one change, single replacement generates one change with removed/added values, reconfirming is a no-op, and silent hydration still emits no event. Prove the autosave observer fires exactly once and ordinary form selection makes no association request.
- [x] Add red tests for destruction while resolving, changing exclusions/parameters, current selection changed externally, a missing resolved ID, resolve HTTP failure, limit reduced or origin disabled. Assert zero commit on any failure.
- [x] Run `npm run test:unit -- src/selector/entityBrowseIntegration.test.ts src/components/selectorFieldAdapter.test.ts` and capture red.
- [x] Implement confirmation: read latest constraints; resolve only pending IDs in batches of at most 50 through Task 4; abort if destroyed. Require every requested ID to return. Recheck availability, serialized constraints, exclusions and capacity after awaiting and before committing. A changed constraint invalidates the attempt and asks the user to refresh rather than guessing. Append to the origin's latest existing values for multi, replace for single.

```ts
const byId = new Map(origin.getValues().map(v => [String(v.ID), v]));
for (const value of resolved) byId.set(String(value.ID), value);
const next = origin.metadata.multiple ? [...byId.values()] : resolved.slice(0, 1);
// Recheck capacity and availability here; then publish once.
origin.replace(next);
```

No new scalar draft is persisted by this bridge. Existing autosave callback and
rollback behavior remain authoritative; successful publication is not a promise
that an asynchronous save has already completed. Do not silently truncate to a
maximum (the core's generic truncation behavior is not the picker policy).

- [x] Hook destroy into adapter/mount teardown and session cancellation. Keep registry notification timing unchanged. Ensure nested filter origins remain alive while hidden; Task 7 must not destroy the parent selector by swapping it through `x-if`.
- [x] Re-run all selector/adapter/mount tests; `npm run build-js`; commit `feat: apply browser selections through selector integration`.

## Task 7: Build the shared dialog, filter coverage and accessible navigation

**Files:**
- Create: `src/components/picker/entityFilterConfigs.ts`, `src/components/picker/entityFilterConfigs.test.ts`, `templates/partials/entityPickerFilters.tpl`, `templates/partials/form/entityBrowseButton.tpl`.
- Modify: `src/components/picker/entityPicker.js`, `src/components/picker/entityConfigs.js`, `src/components/picker/index.js`, `templates/partials/entityPicker.tpl`, `templates/partials/form/autocompleter.tpl`, `templates/partials/lightbox.tpl`.
- Modify: custom selector markup in `templates/groupCompare.tpl`, `templates/partials/blockEditor.tpl`, `templates/partials/pasteUpload.tpl`. Audit `src/main.js` registration and every selector-factory occurrence to avoid missing non-shared markup.
- Extend: `src/components/picker/entityPicker.test.ts`, `src/components/profiledAutocompleter.test.ts`.
- Create: `e2e/tests/selector/entity-browser.spec.ts`.

**Interfaces:** Existing `Alpine.store('entityPicker')` remains the public global
store. `openField(options: PickerOpenOptions, opener: HTMLElement)` opens/pushes
according to whether the opener belongs to a live picker step. The old
`open({entityType, existingIds, lockedFilters, multiSelect, noteId, onConfirm})`
entry point becomes a compatibility adapter in Task 8.

Export `entityFilterConfigs: Record<EntityProfileName, readonly PickerFilter[]>`
and define every descriptor in the same file:

```ts
export type PickerFilter = {
  key: string; label: string; section: 'basic' | 'advanced';
} & (
  | { kind: 'entity'; entity: EntityProfileName; multiple: boolean }
  | { kind: 'text' | 'date' | 'number' | 'checkbox' }
  | { kind: 'metadata' }
);
```

Metadata uses the existing schema/free-meta controls rather than new JSON-path
semantics. Scalar/range/checkbox descriptors use the actual list form field
names. Entity descriptors create explicitly typed dynamic profiles from Task 4.

- [ ] Add red browser tests for the group selector's Browse button, category filtering and duplicate names, new-tab navigation without selection, pending selection across two pages, nested category Browse/Back, and keyboard Escape returning focus.
- [ ] Add a filter-coverage test comparing configured field names against the ordinary list filter controls for all ten types. Read `templates/partials/form/searchFormResource.tpl`, `templates/listGroups.tpl`, `templates/listNotes.tpl`, and other catalog list templates. Exclude only documented presentation controls (sort, page, bulk actions); do not omit metadata, relation-side, dimension, date or boolean filters that those forms expose.
- [ ] Run `cd e2e && npm run test:with-server -- tests/selector/entity-browser.spec.ts` and retain the absent-Browse failure. Run the configuration unit test separately.
- [ ] Make the store a thin adapter over Task 5. Keep each parent step's filter DOM mounted but hidden/inert while its child is active, so callbacks do not refer to destroyed Alpine scopes. One trap/overlay remains active for the stack. Capture refs before teardown; restore focus after trap release. Escape handlers stop propagation and act on the top picker step only.
- [ ] Insert the reusable icon partial beside shared autocomplete and custom tag-editor/filter inputs. Use the field's accessible title for its label, e.g.:

```html
<button type="button" @click="openEntityBrowser()"
        :disabled="browserDisabled" :aria-label="browseLabel" :title="browseLabel">
    <svg aria-hidden="true" focusable="false" viewBox="0 0 24 24"
         fill="none" stroke="currentColor" stroke-width="2" class="h-4 w-4">
        <circle cx="10.5" cy="10.5" r="6.5"></circle>
        <path d="m16 16 4.5 4.5"></path>
    </svg>
</button>
```

`browserDisabled` and `browseLabel` are adapter getters derived from origin
availability and the entity label catalog. Use the button styling of neighboring
controls and a visible keyboard focus ring. No new icon dependency.

- [ ] Render host-owned native checkbox/radio controls beside an inert-to-Alpine content region (`x-ignore` with `x-html` on the content container). Name/author links remain normal anchors. Use stable step/entity IDs for keys and accessible labels outside custom content. Do not put links inside an ARIA option that flattens its interactive descendants.
- [ ] Show basic and More filters, Previous/Next, page indicator, pending count, retry/error/warnings and explicit Confirm for both modes. Keep names/category badges readable. Show already-added and capacity states as text plus controls, not color alone.
- [ ] Install active-step CSS by key using style elements' `textContent`, not executable markup interpolation; remove previous step styles and restore retained parent styles on Back. Remove all picker-owned styles on Cancel. Preserve selected values across filter search failures.
- [ ] Test under an existing plugin-action modal and lightbox: picker owns keyboard/focus only while it is topmost; Escape must not close the parent surface. Re-run unit and focused browser tests, build JS/CSS, run CSS scan, commit `feat: expose accessible entity browsing beside selectors`.

## Task 8: Preserve all consumers and prove custom rendering end-to-end

**Files:**
- Modify: `src/components/blocks/blockReferences.js`, `blockGallery.js`, `blockCalendar.js`, `src/components/pluginActionModal.js` only where compatibility is required.
- Modify: `src/components/picker/entityConfigs.js`, `entityPicker.js` compatibility adapter.
- Extend: `e2e/tests/selector/entity-picker.spec.ts`, `e2e/tests/selector/entity-browser.spec.ts`.
- Create: `e2e/tests/accessibility/entity-browser.spec.ts`, `e2e/tests/cli/entity-picker-templates.spec.ts`.
- Modify docs: `docs/architecture/selector-architecture.md`, `docs-site/docs/features/entity-picker.md`, `docs-site/docs/features/custom-templates.md`.

**Interfaces:** Legacy callers still receive `onConfirm(ids: number[])`; richer
value/session behavior is internal. Map legacy locks explicitly:
resource `content_types` → `ContentTypes`, group `category_ids` → `Categories`,
note `note_type_ids` → `NoteTypeIds`. These are the DTO field spellings in
`resource_query.go`, `group_query.go` and `note_query.go`. Keep this translation centralized.
Resource note-tab constraints are `Notes=<noteID>`, not `OwnerId=<noteID>`.

- [ ] Before adapting callers, add red tests for reference-block group persistence, resource note-tab association correctness, plugin single-entity explicit Confirm, and autosaving lightbox tags. Prove cancellation does not call the old callback.
- [ ] Implement the compatibility mapping and preserve the resource picker note/all tabs as filters over the same paginated source; remove the separate unguarded `loadNoteResources` request. Changing tabs is a guarded filter transition. Do not discard the note tab just because one search/page is empty.
- [ ] Add end-to-end slot tests for group/note/resource carriers via API setup and actual dialog rendering, covering two custom carriers on one page, default fallback, CSS replacement, a logged broken-template fallback, and a custom anchor that does not select.
- [ ] Add CLI tests that create/edit/get the two slot fields for each carrier, including empty-string clearing and file input. Use the existing CLI fixture, not shell interpolation. Verify archive round trips with the Task 1 suite.
- [ ] Add keyboard/axe tests: icon accessible name, nested step labels, focus return, checkbox/radio state, long names, new-tab indication, disabled origins and picker-from-modal nesting. Preserve no-JS native form behavior (icon is inert/hidden without Alpine; underlying field submission is unchanged).
- [ ] Update documentation with custom slot variables, carrier mapping, example HTML/CSS, new-tab links, global-CSS warning, pagination, nested steps and confirmation semantics. Include a minimal author example:

```html
<div class="picker-person">
  <strong>[property path="Name"]</strong>
  <span>[property path="Category.Name"]</span>
</div>
```

Validate the example against the existing property-shortcode grammar and a real
group fixture; CSS uses `.entity-picker-result .picker-person` as the documented
wrapper selector. Explain that template content owns no selection controls and
Alpine initialization is not promised.

- [ ] Run focused browser/CLI/a11y suites against ephemeral servers, all picker unit tests and archive tests. Build JS and commit `test: cover shared entity picker consumers and templates` with docs/assets.

## Task 9: Full verification, review and delivery

**Files:** Generated `public/dist/`, `public/tailwind.css`, OpenAPI artifacts;
`docs/todo.md`; all changed source/tests for fixes accepted by the parent.

**Interfaces:** No new feature interfaces. Evidence consists of exact commands,
exit codes, logs, review findings and the final diff/ref.

- [ ] Run the full unit/build/layering matrix, saving long output outside chat:

```bash
npm run test:unit
npm run build
npm run build-cli
go test --tags 'json1 fts5' ./...
go vet --tags 'json1 fts5' ./...
./scripts/css-scan-test.sh
./mr docs lint
go run ./cmd/openapi-gen
git diff --check
```

Run the CSS scan after Go template-walk tests, not concurrently with them.
Check generated bundles contain current source, and only expected generated
changes exist.

- [ ] Run browser and CLI suites concurrently using the repository's two-server harness:

```bash
cd e2e && npm run test:with-server:all
```

Then run focused auth/a11y coverage if the full harness does not include those
projects. Tests must exercise all ten entity families and at least one origin
outside the shared form partial.

- [ ] With Docker available, add PostgreSQL-specific browse regressions for tied/duplicate names, AND constraint intersection and scope/metadata parity to the existing API PostgreSQL harness. Run:

```bash
go test --tags 'json1 fts5 postgres' ./mrql/... ./server/api_tests/... -count=1
cd e2e && npm run test:with-server:postgres
```

If Docker is unavailable, record that as an unresolved verification blocker;
do not call PostgreSQL coverage passed.

- [ ] Obtain fresh-context read-only Standards and Spec reviews. Provide the approved spec, this plan, the execution baseline and full final diff. Review all-selector coverage, renderer principal/budget binding, stale-response handling, nested focus/teardown, atomic persistence and archive/CLI round trips. The parent adjudicates findings; one writer implements accepted fixes and adds failing regression tests before fixing each.
- [ ] Repeat affected tests and review after fixes. Do not stop at passing unit tests if an end-to-end family remains uncovered. Record residual risks rather than claiming snapshot pagination or CSS isolation.
- [ ] Update `docs/todo.md` with actual validation results and mark only completed tasks. Commit remaining generated assets/docs with their source change. Offer integration options only after evidence supports completion; do not push or merge without authorization.

## Plan self-review and requirement map

| Spec requirement | Tasks |
| --- | --- |
| Existing picker evolution; every profile/catalog type | 4, 6, 7, 8 |
| Basic + advanced property filters, constraints | 2, 4, 7 |
| Explicit bounded pagination, latest response wins | 2, 3, 5, 7 |
| Default identification and new-tab links | 3, 7, 8 |
| Three carrier template/CSS slots and lifecycle | 1, 3, 8 |
| Default/error fallback and shared render budgets | 3, 8 |
| Add-only multi, explicit single, atomic change | 5, 6, 8 |
| Origin persistence ownership, limits and invalidation | 6, 8 |
| Nested steps, fresh reopen, focus/Escape | 5, 7, 8 |
| Scoped data/shortcodes, bounded hydration | 2, 3, 9 |
| Existing consumers, note/resource association correctness | 8 |
| Generated assets, browser/CLI/a11y/PostgreSQL evidence | 1, 7, 8, 9 |

The plan defines each cross-task type at its producer. Raw model transport must
retain the labels/fields required by existing profile observers without shipping
carrier template source redundantly. The implementation must not reinterpret
old max=0 or publish silent programmatic replacement as a user action.

This is a plan, not execution evidence: no code or tests have been implemented by
writing it. The next step is selecting the execution approach.
