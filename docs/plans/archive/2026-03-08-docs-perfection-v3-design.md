# Documentation Perfection v3: Agent Team Design

## Goal

Bring all docs under `docs-site/` up to date with the current codebase. Fix accuracy, fill coverage gaps for new features (especially plugins), capture new screenshots with reproducible seeding metadata, and enforce zero AI-slop. Subagent pipeline with manual output passing — conductor stays in control.

## Context

Two prior attempts exist:
- **Mar 4** (`docs-team-plan.md`): 8-agent team using TeamCreate + SendMessage. Not fully executed.
- **Mar 7** (`docs-perfection-design.md` + `impl.md`): 7-agent team using subagents + Playwright. Partially executed — screenshots added, some de-slopping done.

Since then, significant features have been added (plugin block types, plugin API endpoints, plugin KV store, plugin entity CRUD, plugin settings/management, paste upload, quick tag panel). The docs have not kept up.

### Existing Artifacts (Reused)

| Artifact | Path | Status |
|----------|------|--------|
| Style guide | `tasks/doc-review/style-guide.md` | Current — reuse as-is |
| Gap analysis | `tasks/doc-review/gaps.md` | Partially stale — used as cross-check only |
| Existing screenshots | `docs-site/static/img/*.png` (19 files) | Trusted — not re-captured |
| Doc inventories | `tasks/doc-review/inventory-*.md` | Reference only |

## Agent Team (6 + Conductor)

| Agent | Type | Phase | Parallel With | Edits Docs? |
|-------|------|-------|---------------|-------------|
| Summarizer A | Explore (thorough) | 1 | Summarizer B | No — report only |
| Summarizer B | Explore (thorough) | 1 | Summarizer A | No — report only |
| Checker A | General-purpose | 2 | Checker B | No — report only |
| Checker B | General-purpose | 2 | Checker A | No — report only |
| Writing Coach | General-purpose | 3 | Screenshot Agent | Yes — text only |
| Screenshot Agent | General-purpose | 3 | Writing Coach | Yes — images + manifest |
| Conductor (me) | — | 4 | — | Yes — final pass |

## Execution Phases

### Phase 1 — Ground Truth Extraction (2 Summarizers in Parallel)

**Summarizer A: Entities & CRUD**

Scope:
- Resources, Notes, Groups, Tags, Categories, ResourceCategories, NoteTypes, Series, Relations/RelationTypes, Queries, NoteBlocks
- For each entity: model fields, all API endpoints (method/path/params/response), template pages, query/filter parameters, bulk operations, relationships to other entities

Key files to read:
- `models/*_model.go` — entity definitions
- `application_context/*_context.go`, `*_crud_context.go` — business logic
- `server/routes.go`, `server/routes_openapi.go` — all route registrations
- `server/api_handlers/` — API handler implementations
- `server/template_handlers/` — template page handlers
- `models/query_models/` — filter/query DTOs
- `models/database_scopes/` — GORM query scopes

Output: `tasks/doc-review/ground-truth-entities.md`

**Summarizer B: Features, Plugins & Config**

Scope:
- Plugin system: Lua API (all `mah.*` functions), hooks, actions, pages, blocks, KV store, JSON API endpoints, settings, management, entity CRUD via plugins
- Resource versioning (version CRUD, restore, compare, cleanup, deduplication)
- Image similarity (perceptual hashing: DHash, Hamming distance, background worker, LRU cache)
- Note block system (block types, block API, reordering, calendar/table blocks)
- Search/FTS (global search, type filtering, caching)
- Download queue / job system (states, SSE events, pause/resume/cancel/retry)
- Note sharing (share tokens, share server, interactive features on shared notes)
- Thumbnail generation (image, video via ffmpeg, office docs via LibreOffice, background worker)
- Custom templates (pongo2, custom headers/sidebars/summaries/avatars)
- Meta schemas (JSON Schema validation for metadata)
- Entity picker component
- Activity log (what gets logged, log model, endpoints)
- Paste upload (global paste interception, modal, batch upload)
- Quick tag panel (lightbox side panel, 1-9 key slots, localStorage persistence)
- All keyboard shortcuts across all components
- All configuration flags and environment variables with defaults
- Background workers (hash worker, thumbnail worker, download queue)
- Dashboard (what data it shows)

Key files to read:
- `plugin_system/` — all plugin infrastructure
- `src/components/` — all frontend components
- `main.go` — config flag definitions
- `application_context/version_context.go` — versioning
- `application_context/hash_*`, `application_context/download_*` — workers
- `server/routes.go`, `server/routes_openapi.go` — route registrations
- `server/share_server.go` — share server

Output: `tasks/doc-review/ground-truth-features.md`

### Phase 2 — Doc Checking (2 Checkers in Parallel)

Both checkers read:
- Both ground truth reports from Phase 1
- The existing style guide at `tasks/doc-review/style-guide.md`

**Checker A: Concepts, Getting Started, User Guide (18 docs)**

Files to check:
- `docs-site/docs/intro.md`
- `docs-site/docs/concepts/*.md` (8 files)
- `docs-site/docs/getting-started/*.md` (3 files)
- `docs-site/docs/user-guide/*.md` (6 files)

**Checker B: Features, API, Config, Deployment (30 docs)**

Files to check:
- `docs-site/docs/features/*.md` (16 files)
- `docs-site/docs/api/*.md` (6 files)
- `docs-site/docs/configuration/*.md` (4 files)
- `docs-site/docs/deployment/*.md` (5 files — note: `_category_.json` excluded)
- `docs-site/docs/troubleshooting.md`

**Report format (same for both checkers):**

For each doc file:
1. **INACCURATE** — claims that don't match ground truth (quote the text, cite the correct info)
2. **MISSING** — features from ground truth that should be in this doc but aren't
3. **AI-SLOP** — phrases violating the style guide's banned list (quote the line, categorize the violation)
4. **OUTDATED** — descriptions of features that have changed since docs were written
5. **PRIORITY** — KEEP (no changes needed) / EDIT (minor fixes) / REWRITE (major issues)

Output: `tasks/doc-review/checker-report-a.md` and `tasks/doc-review/checker-report-b.md`

### Phase 3 — Fixes + Screenshots (Parallel)

**Writing Coach**

Inputs: both checker reports + both ground truth reports + style guide

Rules:
- Fix all inaccurate descriptions to match ground truth
- Fill gaps for missing features (add sections to existing docs)
- Create new doc pages only if feature has its own UI page or 3+ API endpoints
- Remove all AI-slop phrases per the style guide's banned list
- Use terminology canon from the style guide
- Follow page templates (concept, how-to, API reference) from the style guide
- Do NOT restructure docs that are rated KEEP
- Do NOT add enthusiasm, exclamation marks, or boilerplate admonitions
- Do NOT touch screenshot references — conductor handles those in Phase 4
- For new doc pages: include Docusaurus frontmatter (`sidebar_position`, `title`, `slug`)

Output: edited files in `docs-site/docs/`, plus a summary listing all files changed/created

**Screenshot Agent**

Three sub-steps:

1. **Screenshot Analyzer** — Read all existing PNG files in `docs-site/static/img/` as images. Produce a draft manifest describing what each screenshot shows, what page it's from, and what data appears to be in it.

2. **Screenshot Planner** — Using the checker reports (to know which new features need screenshots) + the analyzer's manifest (to know what exists), decide:
   - Which new screenshots to capture
   - What data each screenshot needs seeded in the system
   - The full seeding sequence (categories before groups, tags before resources, etc.)
   - Any interactions needed (keyboard shortcuts, checkbox clicks, modal opens)

3. **Seed + Capture** — Build the app (`npm run build`), start ephemeral server (`./mahresources -ephemeral -bind-address=:8282 -max-db-connections=2 -hash-worker-disabled`), seed data via API, navigate pages with Playwright, capture screenshots.

Screenshot specs:
- 1200px viewport width, 800px height
- PNG format, light mode
- Save to `docs-site/static/img/`
- Use existing `e2e/` Playwright setup (browsers already installed)

Output: new PNG files + `docs-site/static/img/screenshot-manifest.json`

### Phase 4 — Conductor Final Review

1. **Cross-check** — Compare checker reports against known gap list (`tasks/doc-review/gaps.md`) to catch anything summarizers missed
2. **Reconcile screenshots** — Add image references to docs for new screenshots, using filenames from the manifest
3. **Update sidebars.ts** — Add entries for any new doc pages created by the writing coach
4. **Slop scan** — Grep all edited docs for banned phrases from the style guide
5. **Build verification** — Run `cd docs-site && npm run build` to catch broken links or missing files
6. **Commit** — Stage all changes and commit

## Screenshot Manifest

Persistent metadata file at `docs-site/static/img/screenshot-manifest.json`.

- Checked into git (metadata, not docs content)
- Not rendered by Docusaurus (JSON in `static/`)
- Discoverable by future documentation runs

Format:

```json
{
  "version": 1,
  "screenshots": {
    "dashboard.png": {
      "page": "/dashboard",
      "description": "Dashboard with populated data showing group/resource/note counts",
      "seedDependencies": ["categories", "groups", "resources", "notes", "tags"],
      "seedDetails": "2+ categories, 4+ groups with hierarchy, 8+ resources with images, 3+ notes with blocks, 6+ tags assigned to entities",
      "viewport": { "width": 1200, "height": 800 },
      "interactions": [],
      "capturedDate": "2026-03-07"
    },
    "global-search.png": {
      "page": "/resources",
      "description": "Global search modal open with results",
      "seedDependencies": ["resources", "notes", "groups"],
      "seedDetails": "Searchable entities with varied names",
      "viewport": { "width": 1200, "height": 800 },
      "interactions": ["press Cmd+K", "type 'Sample'", "wait for results"],
      "capturedDate": "2026-03-07"
    }
  }
}
```

Future runs can read this to know what data to seed and whether screenshots need re-capturing.

## Conflict Resolution

- **Writing Coach** owns all `.md` text edits
- **Screenshot Agent** owns all `.png` files and `screenshot-manifest.json`
- **Conductor** is the only agent that touches both text and images (adding image references in Phase 4)
- If writing coach creates new doc pages, conductor updates `sidebars.ts`
- If writing coach and screenshot agent produce conflicting assumptions about filenames, conductor reconciles

## Conductor Cross-Checks

Before dispatching the writing coach, the conductor verifies:
1. Both ground truth reports cover all entities and features listed in the summarizer prompts
2. Both checker reports address all 48 doc files
3. Known gaps from `tasks/doc-review/gaps.md` appear in the checker reports (if not, conductor adds them manually)

After writing coach completes, the conductor verifies:
1. No new AI-slop introduced (grep for banned phrases)
2. All EDIT/REWRITE files from checker reports were actually touched
3. New doc pages have proper frontmatter
4. Cross-references between docs still work

## Artifacts Summary

| File | Purpose | Persists? |
|------|---------|-----------|
| `tasks/doc-review/ground-truth-entities.md` | Source of truth for entities | Yes |
| `tasks/doc-review/ground-truth-features.md` | Source of truth for features/plugins | Yes |
| `tasks/doc-review/checker-report-a.md` | Issues in concepts/user-guide/getting-started | Yes |
| `tasks/doc-review/checker-report-b.md` | Issues in features/API/config/deployment | Yes |
| `tasks/doc-review/style-guide.md` | Writing rules (already exists, reused) | Yes |
| `tasks/doc-review/gaps.md` | Known gaps (already exists, cross-check only) | Yes |
| `docs-site/static/img/screenshot-manifest.json` | Screenshot reproduction metadata | Yes |
| `docs-site/docs/**/*.md` | The documentation | Yes |
| `docs-site/static/img/*.png` | Screenshots | Yes |
