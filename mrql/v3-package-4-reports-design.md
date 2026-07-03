# MRQL v3 Package 4: Saved Queries as Reports — Design / Plan

Implements `v3-packages.md` §4a (parameterized saved queries) and §4b (EXPLAIN
endpoint and result export), plus a web UI surface for `/v1/mrql/explain` on the
`/mrql` page (user-requested addition). No schema changes; `SavedMRQLQuery`
stays `{Name, Query, Description}` — parameters are derived from the query text.

## 4a. Parameterized queries

### Surface

```
type = resource AND tags = $tag AND created > $since
type = note AND name ~ $needle LIMIT 50
type = resource GROUP BY contentType COUNT() HAVING COUNT() > $min
tags IN ($a, $b)
```

- Placeholder syntax: `$name`, where name is `[a-zA-Z_][a-zA-Z0-9_]*`.
  `$` followed by anything else is a lex error. `$name` inside a quoted string
  stays literal text (the lexer only tokenizes outside strings).
- Placeholders are valid **only in value positions** — everywhere `parseValue`
  is used today: comparison RHS, `IN (...)` list items, HAVING comparison RHS.
  Not in field names, `LIMIT`/`OFFSET`, `SCOPE`, `WITHIN`, or GROUP BY keys (v1).
- The same placeholder may appear multiple times; one supplied value binds all
  occurrences.

### Binding semantics (decisions)

- **Substitution is at the AST value level, never string interpolation.** A new
  `ParamRef{Token, Name}` AST node; `mrql.BindParams(q *Query, params map[string]any)
  error` walks the AST and replaces each `ParamRef` with a literal node. Bound
  literals translate to GORM bind placeholders exactly like typed literals —
  injection-safe by construction.
- **Value coercion mirrors the MRQL lexer.** A supplied value behaves as if it
  had been typed at that value position:
  - JSON number → `NumberLiteral` directly.
  - JSON string → lex the string; if it is **exactly one** value token + EOF
    (number with optional unit `10mb`, relative date `-7d`, date function
    `NOW()`, or a `"quoted string"` which unwraps), that node is used.
    Anything else — including strings with spaces, operators, or multiple
    tokens — becomes a plain `StringLiteral` of the raw input. So
    `since=-7d` works, and `tag=x" OR 1=1` is just a weird tag string.
  - Force-string escape hatch: wrap in quotes (`--param 'n="42"'`).
- **Strict param checking at run time.** Missing params → 400 listing every
  missing name. Unknown/extra params → 400 (typo protection). Param names are
  case-sensitive.
- **Save/validate-time: unbound params are fine.** The validator accepts
  `ParamRef` as a comparison value against any field type; type compatibility
  is re-checked after binding (bind → re-run `Validate` → translate). So
  `/v1/mrql/validate` returns `valid: true` for a parameterized query, and
  saving works. Direct execution of an unbound query (e.g. Run pressed with an
  empty param input) → 400 "missing parameter $tag".
- **Params list is derived, not stored.** New `mrql.ListParams(q *Query) []string`
  (sorted, deduped, in first-appearance order) walks the AST.
- NL generation must not emit placeholders: `LintGeneratedQuery` gains a
  `ParamRef` rejection (a generated query with unbound params can never run).

### API changes

- `POST /v1/mrql` — request gains `params` (JSON object). Bind before validate.
- `POST /v1/mrql/validate` — response gains `params: ["tag", "since"]`
  (empty/omitted when none). Drives the UI param inputs.
- `GET /v1/mrql/saved` (list and `?id=` single) — each item gains a derived
  `params` array (computed by parsing `Query`; parse failure → omitted).
- `POST /v1/mrql/saved/run` — accepts params two ways (both lenient-coerced
  strings): query parameters `param.<name>=<value>` (CLI/curl-friendly — the
  CLI posts `url.Values` today) and, when the body is JSON, a `params` object.
- Flow in run handlers becomes: parse → **bind** → validate → (grouped or flat)
  execute. The existing "saved query is no longer valid" error path covers
  post-bind validation failures too.

### Shortcodes and plugins

(§4a explicitly targets `CustomMRQLResult` templates and plugins.)

- `[mrql saved="report" param-tag="x" param-since="-7d"]` — attrs with the
  `param-` prefix collect into a params map. `shortcodes.QueryExecutor` gains a
  `params map[string]string` argument (signature change ripples through
  `mrql_handler.go`, `conditional_handler.go`, `shortcode_query_executor.go`,
  `custom_css_tag.go`, `shortcode_tag.go`, and the mrql API handlers that build
  executors).
- `plugin_system.MRQLExecOptions` gains `Params map[string]string`; the
  `pluginMRQLAdapter` binds them. Document in the plugin API docs.

### CLI

- `mr mrql "<query>" --param k=v` (root command) and
  `mr mrql run <name-or-id> --param k=v` — repeatable `--param` (StringArray,
  split on first `=`), sent as `param.<k>` query parameters.
- Update `mrql_help/mrql.md` and `mrql_help/mrql_run.md`; CI runs
  `./mr docs lint` and `./mr docs check-examples`.

### Web UI (`/mrql` page)

- `mrqlEditor.js` already debounce-validates on every edit; use the new
  `params` array from the validate response to render one labeled text input
  per placeholder above the Run button (Alpine `x-for`, proper `<label>`s —
  a11y). Values are kept in component state (`paramValues: {}`), sent with
  execute/explain/export, and preserved while the placeholder set is unchanged.
- Loading a saved query with params focuses the first empty param input
  instead of auto-executing (running unbound would just 400).

## 4b. EXPLAIN endpoint

### Surface

`POST /v1/mrql/explain` — body `{query, params}` or `{id}`/`{name}` for a saved
query. Response:

```json
{
  "entityType": "resource",
  "statements": [
    {"label": "resources", "sql": "SELECT ... WHERE ... LIMIT ?", "vars": [...], "interpolated": "SELECT ... LIMIT 500"}
  ],
  "warnings": []
}
```

- **Flat single-entity**: one statement — trivial, `Translate` already returns
  a `*gorm.DB`; run it through a `Session(&gorm.Session{DryRun: true})` `Find`
  into the right model slice, read `Statement.SQL`/`Vars`, and build
  `interpolated` with `db.Dialector.Explain(sql, vars...)` (display only, the
  same interpolation the GORM logger uses).
- **Cross-entity**: three labeled statements (resources/notes/groups).
- **Aggregated GROUP BY**: one statement. `translateAggregatedGroupBy`
  currently builds *and executes*; small refactor to separate build (returns
  the composed `*gorm.DB`) from execute, so explain reuses the build step.
  Same refactor shape for `TranslateGroupByKeys`.
- **Bucketed GROUP BY**: the bucket-keys statement (labeled `bucket keys`)
  plus one representative per-bucket statement via `TranslateGroupByBucket`
  with the first key — or, when keys can't be known without executing, the
  keys statement alone plus a warning ("per-bucket statements repeat per key").
  Decide at implementation; never execute the real query.
- Semantics parity with execution: default LIMIT applied (and reported via the
  standard `default_limit_applied`/`applied_limit` fields), `SCOPE` resolved,
  **RBAC forced scope included** (a group-limited principal sees the scoped SQL
  that would actually run — honest and non-leaking), params required and bound
  first.
- Access policy = execute policy: add `/v1/mrql/explain` to `isReadViaPost`
  (`server/authz_policy.go:111`) so read-only principals may use it and the
  CSRF middleware exempts it.
- Out of scope: DB-level `EXPLAIN QUERY PLAN` / `EXPLAIN ANALYZE` passthrough
  (dialect-specific; a natural follow-up flag once the SQL surface exists).

### Web UI for explain (user-requested)

On `/mrql`, an **Explain** button next to Run:

- Calls `/v1/mrql/explain` with the editor text + param values; renders a
  collapsible panel above the results area with one `<pre><code>` block per
  statement (label as heading, `interpolated` shown, raw SQL + vars behind a
  toggle), a copy-to-clipboard button per statement, and the default-limit
  banner when applicable. Panel is `aria-live="polite"`; errors reuse the
  existing error strip.
- Keyboard: `Mod-Shift-Enter` triggers explain (Run stays `Mod-Enter`).
- No new bundle deps — plain `<pre>` styling, no SQL highlighter.

### CLI

`mr mrql explain "<query>"` and `mr mrql explain --saved <name-or-id>`, with
`--param`. Prints each statement (label header + interpolated SQL); `--json`
emits the raw response. New `mrql_help/mrql_explain.md`.

## 4b. Result export (CSV / JSON)

### Surface

`GET|POST /v1/mrql/export` — same inputs as execute (`query` or `id`/`name`,
`params` / `param.<name>`, `limit`, `page`, `buckets`, `offset`) plus
`format=csv|json` (default `csv`). Streams a download:

- `Content-Disposition: attachment; filename="<saved-name-or-mrql-export>-<date>.<ext>"`
- `format=json`: the exact `/v1/mrql` response body as a download (no `render`).
- `format=csv` (stdlib `encoding/csv`), shape by result mode:
  - **aggregated**: header = GROUP BY keys then aggregate aliases, in query
    order (derived from the AST, since rows are maps); one row per result row.
  - **flat**: fixed scalar column set per entity from the model (resource:
    `id,name,description,content_type,file_size,width,height,created_at,
    updated_at,owner_id,category_id,meta`; analogous for note/group), `meta` as
    a JSON string. Relations aren't loaded by the execute path and stay out.
  - **bucketed**: bucket-key columns prepended to the flat item columns.
- Limits: identical to execution — explicit `LIMIT` respected, default limit
  applied otherwise, signaled via an `X-MRQL-Default-Limit-Applied: <n>`
  response header (a CSV body can't carry the JSON flag). Full-library
  streaming export is out of scope (v1).
- Access policy = execute policy: add `/v1/mrql/export` to `isReadViaPost`.

### Web UI

**Export CSV** / **Export JSON** buttons in the results header, enabled when a
result is present; re-submit the current query + params to `/v1/mrql/export`
via fetch → blob → anchor download (the fetch wrapper carries the CSRF header,
though the endpoint is exempt as a read).

### CLI

`mr mrql export "<query>"` / `mr mrql export --saved <name-or-id>`, flags
`--format csv|json` (default csv), `--param`, `--output <file>` (default
stdout), plus the run pagination flags. New `mrql_help/mrql_export.md`.

## Touch points

- `mrql/token.go`, `lexer.go` — `TokenParam` (`$` branch in `next()`).
- `mrql/ast.go` — `ParamRef` node.
- `mrql/parser.go` — `parseValue` case.
- `mrql/validator.go` — accept `ParamRef` at value positions (defer type
  checks); reject in unsupported positions with positioned errors.
- `mrql/params.go` (new) — `ListParams`, `BindParams`, lenient literal
  coercion.
- `mrql/explain.go` (new) — statement extraction via DryRun sessions;
  build/execute split refactors in `translator.go` / `translator_groupby.go`.
- `mrql/generation_lint.go` — reject `ParamRef` in generated queries.
- `mrql/completer.go` — don't break on `$`; (optional) suggest already-used
  params at value positions.
- `application_context/mrql_context.go` — bind step in `ExecuteMRQL`/grouped
  paths (accept a params argument); `ExplainMRQL`; export result assembly.
- `server/api_handlers/mrql_api_handlers.go` — params plumbing on execute/
  validate/saved/run; new explain + export handlers; derived `params` on saved
  responses.
- `server/routes.go`, `server/routes_openapi.go` — two new routes + OpenAPI
  metadata (regenerate spec).
- `server/authz_policy.go` — `isReadViaPost` additions (covers RBAC + CSRF).
- `shortcodes/processor.go`, `mrql_handler.go`, `conditional_handler.go`;
  `server/template_handlers/template_filters/shortcode_query_executor.go` —
  QueryExecutor params.
- `plugin_system/db_api.go`, `application_context/plugin_mrql_adapter.go` —
  `MRQLExecOptions.Params`.
- `templates/mrql.tpl`, `src/components/mrqlEditor.js` — param inputs, Explain
  button/panel, export buttons (`npm run build-js` + rebuild `./mahresources`
  before e2e).
- `cmd/mr/commands/mrql.go` + `mrql_help/*.md` — `--param`, `explain`,
  `export`.
- Docs: `docs-site/docs/features/mrql.md`, `mrql-reference.md`,
  `.claude/skills/mahresources-cli/references/mrql.md`, plugin API docs,
  regenerated `docs-site/docs/cli`.

## Implementation order (each step red → green)

1. **mrql package: params core.** Lexer/AST/parser/validator + `ListParams` /
   `BindParams` + coercion rules. Unit tests: `mrql/params_test.go` (lex/parse
   positions, coercion table incl. injection-shaped strings, missing/extra
   params, duplicate placeholders, re-validate after bind, `$` in strings).
2. **API params.** Execute/validate/saved-run plumbing, derived `params` in
   saved responses. Tests in `server/api_tests`.
3. **Explain.** Build/execute split refactor, `mrql/explain.go`, endpoint,
   authz/CSRF exemption, OpenAPI. Unit tests assert SQL shape per mode on
   SQLite + PG (`explain_pg_test.go`) and that forced scope appears.
4. **Export.** Endpoint + CSV shapes + headers. Tests: CSV golden outputs per
   mode, JSON body equality with `/v1/mrql`, limit semantics.
5. **Web UI.** Param inputs, Explain panel, Export buttons.
   `e2e/tests/mrql-reports.spec.ts`: save a `$tag` query, inputs appear, run
   with value, missing-value 400 surfaced, explain panel shows SQL, CSV
   download has expected header row. A11y checks on the new controls.
6. **CLI.** `--param`, `explain`, `export` + help docs (`docs lint`,
   `check-examples`). CLI e2e specs in `e2e/tests/cli/`.
7. **Shortcode + plugin params** with tests (shortcode attr parsing, plugin
   adapter).
8. **Docs + full verification.** All docs above; run Go unit
   (`--tags 'json1 fts5'`), `cd e2e && npm run test:with-server:all`, and
   Postgres suites (Go `--tags 'json1 fts5 postgres'` for `./mrql/...` +
   `./server/api_tests/...`, plus `test:with-server:postgres`).

## Explicitly out of scope (v1)

- Param defaults (`$tag:default`), typed param declarations, array params
  expanding `IN` lists.
- Params in `LIMIT`/`OFFSET`/`SCOPE`/`WITHIN`/GROUP BY keys.
- DB-level `EXPLAIN QUERY PLAN` / `EXPLAIN ANALYZE` passthrough.
- Unbounded/streaming full-library export; export of `render=1` HTML.
- Scheduled/emailed reports (this package only makes queries reusable).
