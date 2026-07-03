# MRQL v3 Package 5: Adoption Surfaces — Design / Plan

Implements `v3-packages.md` §5a (query bar on the list pages, MRQL in Cmd+K),
plus two small adjacent adoption items that fell out of the analysis: saved
MRQL queries surfaced in global search, and an `--mrql` flag on the CLI list
commands. No language changes, no schema changes. The theme of the package:
MRQL exists and is capable (packages 1 to 4), but it lives on one page;
this package puts it where browsing already happens.

## 5a. List-page MRQL filter bar

### Surface

A single-line MRQL input at the top of the main column on `/resources` (all
four display variants), `/notes`, and `/groups`. Auto-scoped: the user types a
filter expression, `type = <entity>` is implied by the page.

```
tags = "vacation" AND created > -30d
notes IS EMPTY AND fileSize > 10mb
SIMILAR TO resource(1234) AND tags != "reviewed"
descendants.category = "Archive"
```

Submitting navigates to the same list URL with `?mrql=<expr>` set. Results
stay fully server-rendered: the existing list pipeline (partials, pagination,
sort, display options, bulk selection, plugin slots) is untouched; MRQL just
contributes one more predicate.

### Semantics (decisions)

- **Filter expressions, not full queries.** A new parser entry point
  `mrql.ParseFilter(entity EntityType, input string) (*Query, error)` parses
  a bare boolean expression (the internal `parseExpression` at
  `mrql/parser.go:145` already exists) and returns a `Query` with the given
  entity type set. Rejected with positioned errors that match the user's
  input exactly:
  - clause keywords: `ORDER BY`, `LIMIT`, `OFFSET`, `GROUP BY`, `HAVING`,
    `SCOPE` (the list page owns sort and pagination; subtree filtering is
    expressible in the expression itself via package 2's `descendants.` /
    `ancestors.` traversal);
  - the `type` field (implied by the page; a second `type` would either be
    redundant or contradict the page);
  - `$name` parameter placeholders (there are no param inputs on list pages
    in v1; a bar query must be self-contained).
  - Everything else in the expression grammar is allowed, including
    `SIMILAR TO resource(N)` (predicate form only; `ORDER BY distance` is not
    available in the bar, noted under out-of-scope).
- **Applied as an ID membership predicate.** The application context builds
  the filter query via `ParseFilter` + `Validate` + `TranslateWithOptions`
  (with `ctx.mrqlTranslateOptions()`, `application_context/mrql_context.go:169`,
  so similarity thresholds keep working), selects only the entity's `id`
  column, and composes it onto the list query as
  `resources.id IN (?)` (analogous for notes/groups). This ANDs with all
  existing sidebar filters, sort, and pagination by construction.
- **No LIMIT ever lands in the subquery.** The translator only emits LIMIT
  when the AST carries one (`mrql/translator.go:102`); `ParseFilter` output
  never does (LIMIT is rejected), and the filter path translates the AST
  directly instead of going through `ExecuteMRQL`, so the MRQL default-limit
  logic does not apply. A predicate must match all rows; the list's own
  pagination bounds the output.
- **One helper, three call sites per entity.** A context-layer helper
  (e.g. `ctx.applyMRQLFilter(db, entity, expr) (*gorm.DB, error)`) applied in
  list, count, and popular-tags call sites (`GetResources` /
  `GetResourceCount` at `application_context/resource_crud_context.go:177,184`,
  `GetPopularResourceTags` at `application_context/resource_bulk_context.go:591`;
  analogous for notes and groups). Count and popular tags must see the same
  predicate as the list or pagination and the tag sidebar silently lie.
  Applied at the context layer, not inside `database_scopes`, because scopes
  are `func(*gorm.DB) *gorm.DB` with no error path and no access to translate
  options.
- **Fail closed.** An invalid bar expression renders the list page with an
  error banner and **zero results**, never the unfiltered list. Rationale:
  bulk selection lives on these pages; silently dropping a broken filter and
  then bulk-tagging or bulk-deleting "everything that matched" would act on
  the wrong set.
- **DTO field.** `query_models.ResourceSearchQuery`, `NoteQuery`, and
  `GroupQuery` gain `MRQL string` (decoded from the `mrql` query/form
  parameter). Because the JSON API list handlers decode the same DTOs,
  `/v1/resources`, `/v1/notes`, and `/v1/groups` accept `mrql=` for free;
  on those endpoints an invalid expression is a 400 with the positioned
  error (the API equivalent of fail-closed).
- **RBAC.** The outer list query already carries the forced scope for
  group-limited principals; the MRQL subquery intersects with it, so a
  confined user cannot widen their scope through the bar. Same argument as
  MRQL execution, which is already scope-forced. Covered by a dedicated test.
- **URL contract.** `?mrql=<expr>`. Pagination links and display-option links
  are generated from the request URL
  (`template_entities.GeneratePagination`, `getPathExtensionOptions`) and
  preserve it automatically. The sidebar search forms gain a hidden `mrql`
  input carrying the current value, so refining sidebar filters keeps the bar
  filter, and the bar submit carries the current sidebar params (with `page`
  reset to 1). Clearing the bar and submitting removes the parameter.

### API changes

- `/v1/resources`, `/v1/notes`, `/v1/groups` (list + count variants): accept
  `mrql` (free via the DTO, see above). OpenAPI metadata + regenerated spec.
- `POST /v1/mrql/validate` — request gains optional `entityType`
  (`"resource" | "note" | "group"`) and `filter: true`. In filter mode the
  handler runs `ParseFilter` instead of `Parse`, so error positions match the
  bar input 1:1. Response shape unchanged.
- `POST /v1/mrql/complete` — same two request fields. In filter mode the
  handler prepends `type = <entity> AND ` internally and shifts the cursor
  before calling `Complete` (`mrql/completer.go:175`); suggestions carry no
  positions (the client computes `from` itself, `mrqlEditor.js:200`), so no
  reverse offset math is needed. Filter mode also suppresses clause-keyword
  suggestions (`ORDER`, `LIMIT`, `GROUP`, ...) that would be rejected anyway.

### Web UI

- New `src/components/mrqlBar.js` Alpine component: a plain `<input>`, not
  CodeMirror. Rationale: the list pages are the hottest pages in the app and
  should not pull the CodeMirror chunks; the editor experience stays on
  `/mrql`. The bar provides:
  - autocomplete via `/v1/mrql/complete` in filter mode, rendered as an ARIA
    combobox (input with `role="combobox"`, `aria-expanded`,
    `aria-activedescendant`; suggestion popup as `role="listbox"` with
    `role="option"` children; Up/Down/Enter/Escape keyboard handling);
  - debounced validation via `/v1/mrql/validate` in filter mode (500ms, same
    cadence as the editor), inline error text tied to the input with
    `aria-describedby`;
  - Enter (outside an open suggestion popup) submits the surrounding GET
    form: plain navigation, no client-side result swap;
  - an "Edit in MRQL editor" link that opens
    `/mrql?q=type = <entity> AND (<expr>)` (URL-encoded) for graduation to
    the full editor;
  - server error state: when the page rendered fail-closed, the banner and
    the offending expression (prefilled from `parsedQuery.MRQL`) are shown.
- New shared partial `templates/partials/mrqlBar.tpl` (takes the entity type
  and current value), included from `listResources.tpl`,
  `listResourcesDetails.tpl`, `listResourcesSimple.tpl`,
  `listResourcesTimeline.tpl`, `listNotes.tpl`, `listNotesTimeline.tpl`,
  `listGroups.tpl`, `listGroupsText.tpl`, `listGroupsTimeline.tpl`.
- Hidden `mrql` inputs in `templates/partials/form/searchFormResource.tpl`
  and the inline sidebar forms in `listNotes.tpl` / `listGroups.tpl`.

### CLI

`mr resources list --mrql "<expr>"`, `mr notes list --mrql`,
`mr groups list --mrql`: passes the value as the `mrql` query parameter,
composing with the existing list flags. Update the corresponding
`<group>_help/*.md` files; CI runs `./mr docs lint` and
`./mr docs check-examples`.

## 5b. MRQL in Cmd+K

### Surface

Typing a valid MRQL query into the global search modal surfaces a pinned
action row above the search results: **"Run MRQL query"** with the query
text. Selecting it (click or Enter) navigates to `/mrql?q=<encoded>`, which
already auto-executes queries from the URL (`mrqlEditor.js:293`).

### Decisions

- **Heuristic gate before any network call.** The MRQL interpretation is
  attempted only when the input plausibly is a query: matches
  `/[=~<>]/` or `/\b(IS|IN|EMPTY|SIMILAR)\b/i` or starts with `type `.
  Ordinary search terms never trigger extra requests.
- **Validation decides, full grammar.** When gated in, a debounced
  `POST /v1/mrql/validate` (the normal mode, clauses allowed, `type =`
  optional since cross-entity queries are legal) runs alongside the regular
  `/v1/search` request. `valid: true` pins the action row; anything else
  shows nothing (no error noise in the search modal).
- The row is an option in the existing listbox: participates in arrow-key
  navigation and the `aria-live` announcements (`globalSearch.js` `announce()`),
  announced as "Run MRQL query".
- The `/v1/search` client cache (`globalSearch.js:5`) is untouched; the MRQL
  row derives from validation state, not cached search results.
- A query containing `$name` placeholders still navigates; on `/mrql` the
  param inputs appear via the validate response and execution reports the
  missing values (existing package 4 behavior). No special casing.

## 5c. Saved MRQL queries in global search

Small, rounds out the package: a report saved on `/mrql` is findable from
anywhere.

- `application_context/search_context.go`: add a `mrqlQuery` entity type
  searching `saved_mrql_queries` by name/description through the existing
  `searchEntitiesLike` generic (saved queries are not FTS-indexed; LIKE is
  fine at saved-query cardinality). Result URL: `/mrql?saved=<id>`.
- `getTypesToSearch` gains the new type; `globalSearch.js` `typeIcons` /
  `typeLabels` gain `mrqlQuery`.
- `mrqlEditor.js` `init()`: handle `?saved=<id>` by fetching
  `/v1/mrql/saved?id=` and calling `loadSavedQuery`, which already handles
  parameterized queries correctly (focuses the first empty param input
  instead of auto-running, `mrqlEditor.js:618`).
- Cache invalidation: saved-query create/update/delete calls
  `InvalidateSearchCacheByType("mrqlQuery")`
  (`application_context/search_context.go:30`).

## Touch points

- `mrql/parser.go` — `ParseFilter` entry point; export path for expression
  parsing.
- `mrql/validator.go` — filter-mode rejections (clauses, `type`, `ParamRef`)
  with positioned errors.
- `mrql/completer.go` — clause-keyword suppression hook for filter mode.
- `models/query_models/resource_query.go`, `note_query.go`,
  `group_query.go` — `MRQL string` field.
- `application_context/mrql_context.go` — `applyMRQLFilter` helper (parse,
  validate, translate to ID subquery).
- `application_context/resource_crud_context.go`,
  `resource_bulk_context.go`, `note_context.go`, `group_crud_context.go` —
  apply the helper at list/count/popular-tags call sites.
- `application_context/search_context.go` — `mrqlQuery` search type +
  invalidation calls in the saved-query CRUD paths.
- `server/api_handlers/mrql_api_handlers.go` — filter mode on validate and
  complete.
- `server/routes_openapi.go` — `mrql` parameter on the three list endpoints;
  regenerate the spec (`go run ./cmd/openapi-gen`).
- `server/template_handlers/template_context_providers/
  resource_template_context.go`, `note_template_context.go`,
  `group_template_context.go` — fail-closed error context + `parsedQuery`
  already carries the field.
- `templates/partials/mrqlBar.tpl` (new), the nine list templates,
  `templates/partials/form/searchFormResource.tpl`, sidebar forms in
  `listNotes.tpl` / `listGroups.tpl`.
- `src/components/mrqlBar.js` (new), `src/components/globalSearch.js`,
  `src/components/mrqlEditor.js` (`?saved=` loading), `src/main.js`
  (register component). `npm run build-js` + rebuild `./mahresources` before
  e2e (stale-binary trap).
- `cmd/mr/commands/resources.go`, `notes.go`, `groups.go` + their
  `*_help/*.md` files.
- Docs: `docs-site/docs/features/mrql.md`, `mrql-reference.md`,
  `.claude/skills/mahresources-cli/references/mrql.md`, regenerated
  `docs-site/docs/cli`.

## Implementation order (each step red → green)

1. **mrql package: `ParseFilter`.** Entry point + filter-mode rejections.
   Unit tests: valid expressions per entity, positioned errors for each
   rejected construct (`ORDER BY`, `LIMIT`, `SCOPE`, `type =`, `$param`),
   `SIMILAR TO` accepted, translated subquery carries no LIMIT.
2. **Context layer + JSON API.** DTO fields, `applyMRQLFilter`, application
   at list/count/popular-tags sites. `server/api_tests`: `mrql=` narrows
   `/v1/resources`, count agrees with list, invalid expression is a 400 with
   position, group-limited principal cannot escape scope via `mrql=`,
   SQLite + Postgres (`--tags 'json1 fts5 postgres'`).
3. **Validate/complete filter mode.** Handler changes + unit tests (error
   positions unshifted; clause suggestions suppressed).
4. **List-page bar UI.** `mrqlBar.js`, partial, hidden inputs, fail-closed
   banner. `e2e/tests/mrql-list-bar.spec.ts`: filter narrows the resource
   list, pagination link preserves `mrql`, sidebar refinement preserves
   `mrql`, invalid expression shows error and zero results, autocomplete
   popup appears and applies, "Edit in MRQL editor" link round-trips, a11y
   scan of the combobox.
5. **Cmd+K.** `globalSearch.js` gate + validate + pinned row.
   `e2e/tests/mrql-cmdk.spec.ts`: plain term shows no MRQL row, valid query
   shows it, Enter lands on `/mrql` with results; a11y announcement checked.
6. **Saved queries in global search.** Backend type + cache invalidation +
   `?saved=` loading in the editor. E2E: save on `/mrql`, find via Cmd+K,
   select, editor loads it (parameterized query focuses param input).
7. **CLI `--mrql`.** Flags + help docs (`docs lint`, `check-examples`) + CLI
   e2e specs in `e2e/tests/cli/`.
8. **Docs + full verification.** All docs above; Go unit
   (`--tags 'json1 fts5'`), `cd e2e && npm run test:with-server:all`, and the
   Postgres suites (Go `--tags 'json1 fts5 postgres'` for `./mrql/...` +
   `./server/api_tests/...`, plus `test:with-server:postgres`).

## Explicitly out of scope (v1)

- `ORDER BY` (including `ORDER BY distance`), `LIMIT`/`OFFSET`, `GROUP BY`,
  `SCOPE`, and `$name` params in the list bar. The "Edit in MRQL editor"
  link is the escape hatch for all of them.
- Inline MRQL execution or result rendering inside the Cmd+K modal
  (navigation to `/mrql` only; a candidate v2 once the bar proves demand).
- CodeMirror on list pages.
- Saved-query pills / pinned filters on list pages.
- MRQL bars on the tag/category/series/relation list pages (MRQL only
  queries resources, notes, and groups; `mrql/ast.go:195`).
- Persisting bar history (the `/mrql` editor keeps its server-backed
  history; the bar is stateless beyond the URL).
