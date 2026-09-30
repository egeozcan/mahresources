# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Development Commands

```bash
# Build the application (compiles CSS + JS bundle + Go binary)
npm run build

# Development mode with hot reload
npm run watch

# Build CSS only
npm run build-css

# Check the stylesheet's source scan: prose stays out of public/tailwind.css, no
# class the app authors or the docs hand out was dropped by an exclusion in
# index.css, and every explicit @source reaches the whole tree it names
./scripts/css-scan-test.sh

# Build JS bundle only (Vite)
npm run build-js

# Watch mode for JS development
npm run dev

# Run Go unit tests (json1 and fts5 tags required for full coverage)
go test --tags 'json1 fts5' ./...

# Run specific test file
go test ./server/api_tests/...

# Run E2E tests (recommended: automatic server management)
cd e2e && npm run test:with-server

# Run accessibility tests only
cd e2e && npm run test:with-server:a11y

# Run E2E tests with browser visible
cd e2e && npm run test:with-server:headed

# Build Go binary directly (requires json1 for SQLite JSON, fts5 for full-text search)
go build --tags 'json1 fts5'

# Run the server (default port 8181)
./mahresources

# Generate OpenAPI spec from code
go run ./cmd/openapi-gen

# Generate OpenAPI spec with custom output
go run ./cmd/openapi-gen -output api-spec.yaml
go run ./cmd/openapi-gen -output api-spec.json -format json

# Validate a generated OpenAPI spec
go run ./cmd/openapi-gen/validate.go openapi.yaml
```

## Architecture Overview

Mahresources is a CRUD application for personal information management written in Go. It manages Resources (files), Notes, Groups, Tags, Categories, Queries, and their relationships.

### Core Layers

**application_context/** - Business logic and data access layer. Each entity has a dedicated context file (e.g., `resource_context.go`, `note_context.go`) that implements CRUD operations. The main `context.go` initializes DB, filesystem, and configuration.

**models/** - GORM models and database layer. Entity models are in `*_model.go` files. Query DTOs are in `query_models/`. GORM query scopes are in `database_scopes/`.

**contracts/** - The boundary between the two layers above: the Reader/Writer/Deleter interfaces that `application_context` implements and `server` consumes, plus the few shared response types they exchange (`CalendarEventsResponse`, `MetaKey`, `SuggestedTag`). It depends only on `models/` and `constants/`, so both layers can point at it without pointing at each other.

**groupio/** - Group export and import: export planning, tar streaming, import parsing and apply. Extracted from `application_context`, which now reaches it through a facade (`application_context/groupio_facade.go`) so no caller outside the seam changed.

**search/** - Global search: the cross-entity query, the LIKE and FTS backends, and the process-wide result cache. Extracted likewise, behind `application_context/search_facade.go`.

**server/** - HTTP layer with Gorilla Mux routing.
- `api_handlers/` - JSON API endpoints
- `template_handlers/` - HTML template rendering
- `openapi/` - OpenAPI 3.0 spec generation from code
- `routes_openapi.go` - API route definitions with OpenAPI metadata

**templates/** - Pongo2 templates (Django-like syntax). Each entity has create, display, and list templates.

**src/** - Frontend JavaScript source files, bundled with Vite.
- `main.js` - Entry point that imports all modules and initializes Alpine.js
- `index.js` - Utility functions (abortableFetch, clipboard, etc.)
- `components/` - Alpine.js data components (globalSearch, bulkSelection, etc.)
- `selector/` - Headless selector core, sources, and entity profiles. Every entity picker is built
  from it; see `docs/architecture/selector-architecture.md` before adding or changing one.
- `webcomponents/` - Custom elements (expandable-text, inline-edit)
- `tableMaker.js` - JSON table rendering

**public/** - Static assets served by the Go server.
- `dist/` - Vite build output: `main.js` plus the lazy-loaded chunks in `assets/`. These are **committed**, not gitignored, so `npm run build-js` produces a diff that must be committed alongside the source change that caused it. Two branches that both rebuild will conflict on minified output; resolve by taking either side wholesale and rebuilding once.
- `tailwind.css` - Generated Tailwind CSS
- `index.css`, `jsonTable.css` - Custom styles
- `favicon/` - Favicon files

### Key Design Patterns

**Dual Response Format**: Routes support both HTML and JSON responses. Add `.json` suffix or use `Accept: application/json` header to get JSON.

**Generic Entity Writers**: `EntityWriter[T]` generic type handles common CRUD operations across entities.

**Interface-based DI**: Handlers receive specific interfaces (e.g., `contracts.ResourceReader`, `contracts.GroupWriter`) rather than concrete implementations. `application_context/contract_checks.go` asserts at compile time that `MahresourcesContext` satisfies each one.

**Enforced layering**: the dependency direction is `server/` → `application_context/` → {`groupio/`, `search/`, `contracts/`} → `models/` → `constants/`, and it is checked, not merely documented. `internal/arch/layering_test.go` fails the build if anything below `server/` imports it (only `main`, `cmd/...`, and `internal/...` may), if `models/` reaches outside itself, if `contracts/` depends on either layer that uses it, or if `groupio/` or `search/` reaches back up into `application_context/`, `server/` or `contracts/`. When you need to hand a type across the `application_context`/`server` boundary, put it in `contracts/`.

**Extracted services take the db handle per call, never at construction.** Scope, actor identity, and transaction membership all ride inside the `*gorm.DB` handle: `WithPrincipal` swaps in a handle whose context carries the subtree allow-list and acting user, `WithTransaction` swaps in the transaction's handle, and the GORM callbacks read both off `db.Statement.Context`. A service that captured `db` when it was built would run outside the caller's transaction and outside their subtree — silently, and with no test failing. `groupio` and `search` therefore hold only process-lifetime state (filesystems, the search cache, the FTS provider) and receive `Deps{DB, Scope}` on every entry point; the facades rebuild `Deps` per call. Follow this for any further extraction. See `docs/plans/2026-07-28-application-context-decomposition.md` §2.

**SQLite transactions take the writer lock at BEGIN.** Every production SQLite handle opens with `models.SQLiteDriverName`, which begins a transaction `BEGIN IMMEDIATE` unless it is declared read-only. Two rules follow, and `docs/architecture/sqlite-transactions.md` has the reasons:

- A transaction that only reads must pass `models.ReadOnlyTxOptions(db)...` to `Transaction`. Unmarked, it holds the writer lock for its whole length and every writer in the process waits on it.
- Nothing inside a transaction may write through a second connection (`ctx.db` instead of the transaction's handle, or a logger built from the outer context). That write waits `busy_timeout` for the lock its own transaction holds, then fails.

**Scope is a query callback, so the query method matters.** `Find`, `Pluck` and `Count` run the callback that appends the subtree predicate. `Scan` does not, so a `Scan` hands a group-limited principal rows outside its subtree.

**Postgres check-then-act needs a row lock.** A read inside a READ COMMITTED transaction does not stop a concurrent delete, so a validation that a later write depends on takes `SELECT ... FOR UPDATE` (`ValidateAndLockAssociationIDs`). SQLite serializes writers and cannot reproduce these races, so they need a Postgres test.

### Entity Relationships

- **Resource**: Files with metadata, thumbnails, perceptual hashes. Many-to-many with Tags, Notes, Groups.
- **Note**: Text content with NoteType. Many-to-many with Resources, Tags, Groups.
- **Group**: Hierarchical collections. Can own other Groups, Resources, Notes.
- **GroupRelation**: Custom typed relationships between groups.
- **Tag/Category**: Labels for organization.
- **Query**: Saved searches.
- **DownloadHistoryEntry**: A legacy download-history record used by the compatibility routes and `/downloads` view. Canonical Job lifecycle and ownership live in the Job tables.

### Subsystem notes

Design notes for individual subsystems are in `docs/architecture/`. Each records invariants, rejected alternatives and the tests that pin them. Read the matching file before you change that area, and update it when the design changes.

- `background-jobs.md`: the Job Service (the durable record of every background Job) and the in-memory `download_queue` with its legacy download routes; retry, shutdown, failure codes, progress metrics and operator rollout checks.
- `sqlite-transactions.md`: why transactions begin `IMMEDIATE`, and what that means for tests and older write-first code.
- `bulk-resource-uploads.md`: the per-file upload widget, `AddResource`'s three phases, hash deduplication and its scope rules, and file reference counting.
- `mass-edit.md`: `POST /v1/{resources,notes,groups}/massEdit`, with filter-mode targets, `ExpectedCount`, lock ordering, scoped replace and bind budgets.
- `series-metadata.md`: keeping `Meta` and `OwnMeta` consistent across a Series, and the Postgres lock order that requires.
- `hls-ingest.md`: assembling HLS playlists into one MP4 in `hls/`, with the fetch policy applied to every playlist, key and segment.
- `host-fetch.md`: the User-Agent and per-download headers on the server's own fetches (`hostfetch/`).
- `plugin-downloads-and-media.md`: `mah.download.submit` and `mah.media`, and the capability, egress, scope and role checks on each.
- `plugin-download-pacing.md`: manifest `download_limits` and deferred (`start_at` / `delay`) plugin downloads.
- `job-lifecycle-events.md`: the `after_job_*` hooks and their asynchronous dispatcher.
- `plugin-command-staging.md`: plugin-declared host commands, the staging root, quotas and recovery of abandoned runs.
- `plugin-static-assets.md`: serving a plugin's `public/` directory at `/plugins/<name>/public/*`.
- `plugin-work-admission.md`: the order in which plugin work takes its lane, a job slot, the VM and the host's admission.
- `plugin-handler-stops.md`: cancel, disable and shutdown of running handlers, outcome classification and secret redaction.
- `plugin-schedules.md`: `mah.schedule`, with durable rows, the derived claim TTL, overlap policies and enable/disable reconciliation.
- `authentication-and-roles.md`: roles, subtree scoping, per-plugin access for scoped accounts, role-capability guards, login and CSRF.
- `root-admin-and-attribution.md`: the root admin invariant, the last-admin guard and `CreatedByUserId` stamping.
- `configuration.md`: every flag and environment variable, alternative file systems and example invocations.
- `selector-architecture.md`: the headless selector every entity picker is built from.

### Configuration

Every setting is a command-line flag or an environment variable (in `.env`), and a flag wins over its variable. The full table, the alternative file system syntax and example invocations are in `docs/architecture/configuration.md`. Add a new flag's row there.

### Authentication & roles

Auth is opt-in (`-auth`). With it off, every request runs as an implicit administrator built from the root admin. With it on, a request authenticates with a session cookie or `Authorization: Bearer <token>`, and there are four roles:

- **admin**: everything, including system settings, plugin management, categories and user administration.
- **editor**: CRUD on entities except creating or editing Categories and Resource Categories; no system settings.
- **user**: CRUD on resources and notes (plus subgroups, tagging, note sharing, group import/export and plugin actions), optionally confined to one Group's subtree.
- **guest**: read-only, always confined to one Group's subtree.

Rules that are easy to break:

- Confinement to a subtree fails closed, and it covers lists, single reads, search, MRQL, file serving, group export and writes.
- A new admin page must be listed in `isSystemPath` (`server/authz_policy.go`). Otherwise it falls through to `capRead` and every authenticated role can read it.
- Role checks go on *operations* (`requireTaxonomyRole`, `requireEditorRole`), never on tables. `internal/arch/role_capability_gate_test.go` fails the build when a new operation of that shape has no guard.
- Plugin hooks fire from ordinary writes, so the URL-based plugin deny never sees them. That is why `mah.db` is bound to the acting principal.
- A root admin always exists, and the last enabled admin cannot be deleted, demoted or disabled (`ErrLastAdmin`, HTTP 409).
- `CreatedByUserId` is stamped from the acting user by a global GORM create callback. It is never on a request DTO.

The details are in `docs/architecture/authentication-and-roles.md` and `docs/architecture/root-admin-and-attribution.md`.

### API Structure

Base path: `/v1`

Endpoints follow pattern: `GET/POST/DELETE /v1/{entities}` for lists, `/v1/{entity}` for single items.

Bulk operations available: `addTags`, `removeTags`, `addMeta`, `delete`, `merge`.

### Frontend Stack

- **Vite** - Bundler for JavaScript modules
- **Alpine.js** - Lightweight reactive framework for UI components
- **Tailwind CSS** - Utility-first CSS framework
- Keep the two corners touching a thick colored accent border square. Other corners may remain rounded.
- Hide a focus outline with `focus:outline-hidden` (or `outline: 2px solid transparent` in CSS), never with `outline-none` / `outline: none`. In Tailwind v4 `outline-none` is `outline-style: none`, and forced colors (Windows High Contrast) drops the `ring` box-shadow and repaints focus colours, so a control relying on a ring shows no focus there; the transparent outline is the one indicator forced colors paints. Only a `:focus:not(:focus-visible)` rule whose `:focus-visible` draws a real outline may remove it. `internal/arch/forced_colors_focus_test.go` enforces this.
- **baguetteBox.js** - Image gallery lightbox
- **Web Components** - Custom elements for expandable text and inline editing

Global search is accessible via `Cmd/Ctrl+K` shortcut.

## Testing

### Go Unit Tests
```bash
go test ./...
```

### E2E Tests (Playwright)

**IMPORTANT: Always run E2E tests against an ephemeral instance** to ensure test isolation and avoid polluting real data.

```bash
# Easiest way: automatic server management (recommended)
cd e2e && npm run test:with-server

# Other automatic server commands:
npm run test:with-server:headed  # Run with browser visible
npm run test:with-server:debug   # Run in debug mode
npm run test:with-server:a11y    # Run accessibility tests only

# CLI E2E tests (tests the `mr` CLI binary against an ephemeral server)
npm run test:with-server:cli
```

**After any significant change, run both browser and CLI E2E tests in parallel:**
```bash
cd e2e && npm run test:with-server:all
```
This launches two separate ephemeral servers and runs browser + CLI tests simultaneously.

The `test:with-server` scripts automatically find an available port, start an ephemeral server with `-max-db-connections=2`, run tests in parallel, and clean up.

### Postgres Tests (requires Docker)

```bash
# Run Go tests against Postgres (MRQL + API)
go test --tags 'json1 fts5 postgres' ./mrql/... ./server/api_tests/... -count=1

# Run E2E tests against Postgres
cd e2e && npm run test:with-server:postgres

# Run all Postgres tests (Go + E2E)
go test --tags 'json1 fts5 postgres' ./mrql/... ./server/api_tests/... -count=1 && cd e2e && npm run test:with-server:postgres
```

**Note:** Postgres tests should be run when finishing features or bugfixes, alongside regular SQLite tests. They require Docker to be running.

**Manual server management** (if you need more control):

```bash
# 1. Build the application first
npm run build

# 2. Start server in ephemeral mode (separate terminal)
# Use -max-db-connections=2 to reduce SQLite lock contention with parallel tests
./mahresources -ephemeral -bind-address=:8181 -max-db-connections=2

# 3. Run all E2E tests
cd e2e && npm test

# Other test commands:
npm run test:headed    # Run with browser visible
npm run test:debug     # Run in debug mode
npm run test:ui        # Run with Playwright UI
npm run test:a11y      # Run accessibility tests only
npm run report         # View HTML test report
```

### E2E Test Structure

**e2e/** - Playwright test suite
- `fixtures/` - Test fixtures (base.fixture.ts, a11y.fixture.ts)
- `helpers/` - API client and accessibility helpers
- `pages/` - Page Object Models for each entity type
- `tests/` - Test specs organized by feature
- `tests/accessibility/` - axe-core accessibility tests (WCAG compliance)
- `tests/cli/` - CLI E2E tests (20 spec files, ~229 tests for the `mr` binary)
- `fixtures/cli.fixture.ts` - CLI test fixture (`CliRunner` helper)
- `helpers/cli-runner.ts` - CLI binary executor with retry logic for SQLite contention

## Important Notes

- Authentication/authorization is **opt-in** (`-auth`). Off by default — designed for private networks — but when enabled it adds user accounts + four RBAC roles (admin/editor/user/guest) with group-subtree scoping. See the "Authentication & roles" section above.
- Fully aware that we can inject all kinds of content via unescaped via CustomHeader, CustomSidebar, etc. and that's okay.
- A11y is important. Very important.
- The group export/import archive format (manifest schema version 1) is a stable public contract. `archive/manifest.go` defines the schema. Rules: readers reject unknown major `schema_version` values with a clear error; unknown top-level keys in the manifest are silently ignored (forward compatibility). Breaking changes require bumping `schema_version`. Do not change field names, remove fields, or alter semantics without a version bump.
- SQLite requires `--tags json1` build flag for JSON query support
- Image processing uses disintegration/imaging (thumbnail resizing, Lanczos) and anthonynsimon/bild (rotation and other transforms)
- File system abstraction via Afero supports multiple storage locations
- Run `npm run build-js` after modifying files in `src/` to rebuild the bundle
- Keep in mind that some deployments of this software deal with millions of resources
- Tests need to be fixed, regardless of what broke it. 
  - It may be a good idea to run tests before you start to see if there are any failing and fix them beforehand.

## CLI Documentation

When you add or change a command or flag in `cmd/mr/commands/`, update the corresponding `<group>_help/*.md` file. CI runs `./mr docs lint` (the `cli-docs-fresh` job) and `./mr docs check-examples` (the `cli-doctest` job) on every PR. Reference pattern: `cmd/mr/commands/resources_help/resource_get.md`.

## Agent skill (`skills/`)

`skills/mahresources-mrql/` is an installable [open agent skill](https://github.com/vercel-labs/skills) that teaches an agent to drive MRQL through the `mr` CLI. It is not a fourth place to write MRQL documentation:

- `references/language.md` is **generated** (`npm run skills-gen`, `cmd/skills-gen`) from `docs-site/docs/features/mrql-reference.md` plus the live Cobra tree. Never edit it; edit the docs-site page. The `cli-docs-fresh` job regenerates and diffs `skills/`.
- That page is checked against the code: `mrql/reference_docs_test.go` (fields, parser guardrails) and `application_context/mrql_reference_docs_test.go` (execution limits) fail when a field is added to `mrql/fields.go` or a constant changes without the page following.
- `SKILL.md` and `references/recipes.md` are hand-authored, and every fenced `bash` block in them runs against an ephemeral server in the `cli-doctest` job via `mr docs check-examples --files`. A block that cannot run standalone opts out with `# mr-doctest: skip, <reason>` as its first line.

## Agent skills

Configuration for the mattpocock engineering skills (`/grill-with-docs`, `/to-spec`,
`/to-tickets`, `/implement`, `/triage`, `/wayfinder`). Unrelated to the installable
MRQL agent skill documented in the section above.

### Issue tracker

Issues live as GitHub issues in `egeozcan/mahresources`, driven by the `gh` CLI.
See `docs/agents/issue-tracker.md`.

### Triage labels

The five canonical triage roles, each label string equal to its name. See
`docs/agents/triage-labels.md`.

### Domain docs

Single-context: `CONTEXT.md` and `docs/adr/` at the repo root. See
`docs/agents/domain.md`.

## Workflow Orchestration

### 1. Plan Node Default
- Enter plan mode for ANY non-trivial task (3+ steps or architectural decisions)
- If something goes sideways, STOP and re-plan immediately - don't keep pushing
- Use plan mode for verification steps, not just building
- Write detailed specs upfront to reduce ambiguity

### 2. Subagent Strategy
- Use subagents liberally to keep main context window clean
- Offload research, exploration, and parallel analysis to subagents
- For complex problems, throw more compute at it via subagents
- One tack per subagent for focused execution

### 3. Self-Improvement Loop
- After ANY correction from the user: update `docs/lessons.md` with the pattern
- Write rules for yourself that prevent the same mistake
- Ruthlessly iterate on these lessons until mistake rate drops
- Review lessons at session start for relevant project

### 4. Verification Before Done
- Never mark a task complete without proving it works
- Diff behavior between main and your changes when relevant
- Ask yourself: "Would a staff engineer approve this?"
- Run tests, check logs, demonstrate correctness

### 5. Demand Elegance (Balanced)
- For non-trivial changes: pause and ask "is there a more elegant way?"
- If a fix feels hacky: "Knowing everything I know now, implement the elegant solution"
- Skip this for simple, obvious fixes - don't over-engineer
- Challenge your own work before presenting it

### 6. Autonomous Bug Fixing
- When given a bug report: just fix it. Don't ask for hand-holding
- Point at logs, errors, failing tests - then resolve them
- Zero context switching required from the user
- Go fix failing CI tests without being told how

## Task Management

1. **Plan First**: Write plan to `docs/todo.md` with checkable items
2. **Verify Plan**: Check in before starting implementation
3. **Track Progress**: Mark items complete as you go
4. **Explain Changes**: High-level summary at each step
5. **Document Results**: Add review section to `docs/todo.md`
6. **Capture Lessons**: Update `docs/lessons.md` after corrections

## Core Principles

- **Simplicity First**: Make every change as simple as possible. Impact minimal code.
- **No Laziness**: Find root causes. No temporary fixes. Senior developer standards.
- **Minimal Impact**: Changes should only touch what's necessary. Avoid introducing bugs.

## Methodology

Use TDD (red/green/refactor) as much as it makes sense. Adding integration tests and running them before starting and after the work is complete is very important.


### Project Management integration rules

- Keep host enablers generic: no PM taxonomy names in server or application context.
- Prefer `[meta editable=true]` for schema-backed fields. PM controls own only status/order, dates and ownership semantics.
- Use rollup metadata for PM card summaries, never per-card MRQL. The reconciliation must cover missed hooks, mass edits, deletion and block state writes.
- List action filtering treats unknown dimensions as unknown; a category-filtered resource list must retain content-type-filtered actions. Unfiltered lists still offer all actions for their entity kind.
- Mirror bundled PM source and assets into `e2e/test-plugins/project-management`; the test guards byte equality.
