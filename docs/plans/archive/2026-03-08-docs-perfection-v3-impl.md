# Documentation Perfection v3 Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Bring all 50 docs under `docs-site/` to full accuracy and coverage using a 6-agent subagent pipeline, capturing new screenshots with reproducible seeding metadata.

**Architecture:** 4-phase pipeline — two summarizers extract ground truth from code in parallel, two checkers compare all docs against ground truth in parallel, writing coach + screenshot agent fix docs and capture images in parallel, conductor does final review. All output passing is manual (conductor reads reports and passes to next phase).

**Tech Stack:** Subagents (Explore + general-purpose), Playwright for screenshots, ephemeral server for screenshot seeding, existing style guide at `tasks/doc-review/style-guide.md`.

---

## Shared Context

**Codebase root:** `/Users/egecan/Code/mahresources`
**Docs directory:** `/Users/egecan/Code/mahresources/docs-site/docs` (50 .md files)
**Screenshots:** `/Users/egecan/Code/mahresources/docs-site/static/img/` (20 PNGs — 19 app screenshots + docusaurus.png)
**Style guide:** `/Users/egecan/Code/mahresources/tasks/doc-review/style-guide.md` (reuse as-is)
**Known gaps:** `/Users/egecan/Code/mahresources/tasks/doc-review/gaps.md` (cross-check reference)
**Sidebar:** `/Users/egecan/Code/mahresources/docs-site/sidebars.ts`

**Doc file inventory (50 files, by section):**

| Section | Count | Files |
|---------|-------|-------|
| Root | 1 | `intro.md` |
| Getting Started | 3 | `installation.md`, `quick-start.md`, `first-steps.md` |
| Core Concepts | 8 | `overview.md`, `resources.md`, `notes.md`, `note-blocks.md`, `groups.md`, `tags-categories.md`, `relationships.md`, `series.md` |
| User Guide | 6 | `navigation.md`, `managing-resources.md`, `managing-notes.md`, `organizing-with-groups.md`, `search.md`, `bulk-operations.md` |
| Configuration | 4 | `overview.md`, `database.md`, `storage.md`, `advanced.md` |
| Advanced Features | 16 | `versioning.md`, `image-similarity.md`, `saved-queries.md`, `custom-templates.md`, `meta-schemas.md`, `note-sharing.md`, `download-queue.md`, `job-system.md`, `activity-log.md`, `thumbnail-generation.md`, `custom-block-types.md`, `entity-picker.md`, `plugin-system.md`, `plugin-actions.md`, `plugin-hooks.md`, `plugin-lua-api.md` |
| API Reference | 6 | `overview.md`, `resources.md`, `notes.md`, `groups.md`, `plugins.md`, `other-endpoints.md` |
| Deployment | 5 | `docker.md`, `systemd.md`, `reverse-proxy.md`, `public-sharing.md`, `backups.md` |
| Other | 1 | `troubleshooting.md` |

**Existing screenshots (19 app screenshots):**
`dashboard.png`, `grid-view.png`, `resource-detail.png`, `upload-form.png`, `note-blocks.png`, `group-tree.png`, `group-detail.png`, `search-results.png`, `query-editor.png`, `activity-log.png`, `tag-list.png`, `note-list.png`, `group-list.png`, `resource-detail-view.png`, `note-edit.png`, `group-edit.png`, `category-list.png`, `global-search.png`, `bulk-selection.png`

---

### Task 1: Phase 1 — Dispatch Summarizer Agents (Parallel)

Dispatch two Explore subagents simultaneously. They read code only — no doc reads, no edits.

**Step 1: Dispatch both summarizers in a single message with two Agent tool calls**

**Summarizer A prompt (Explore agent, "very thorough"):**

```
You are Technical Summarizer A. Produce a GROUND TRUTH REPORT of all entity-related features in mahresources by reading the actual source code. Do NOT read or modify any docs under docs-site/.

ENTITIES TO COVER:
- Resources
- Notes
- NoteBlocks
- Groups
- Tags
- Categories
- ResourceCategories
- NoteTypes
- Series
- Relations / RelationTypes
- Queries
- LogEntries

FOR EACH ENTITY, DOCUMENT:
1. All model fields with Go types (from models/*_model.go)
2. All CRUD operations and what they do (from application_context/*_context.go, *_crud_context.go)
3. All API endpoints — exact method (GET/POST/DELETE), exact path, all query params, request body format, response shape (from server/routes.go, server/routes_openapi.go, server/api_handlers/)
4. All template pages — URL path and what they render (from server/template_handlers/)
5. All query/filter parameters with types (from models/query_models/)
6. All bulk operations available and their endpoints
7. All many-to-many relationships and ownership rules
8. Deletion behavior — what cascades, what gets orphaned

KEY FILES TO READ:
- server/routes.go and server/routes_openapi.go — ALL route registrations (read these FIRST to get the complete endpoint list)
- models/ — all *_model.go files
- application_context/ — all *_context.go and *_crud_context.go files
- server/api_handlers/ — handler implementations for request/response details
- server/template_handlers/ — template page registrations
- models/query_models/ — filter/query DTOs with field names and types
- models/database_scopes/ — GORM query scopes (filtering logic)

OUTPUT: Write your complete report to /Users/egecan/Code/mahresources/tasks/doc-review/ground-truth-entities.md

Format: One ## section per entity. Include exact field names, exact endpoint paths, exact parameter names. Use tables for fields and endpoints. This report will be compared word-for-word against documentation to find inaccuracies.
```

**Summarizer B prompt (Explore agent, "very thorough"):**

```
You are Technical Summarizer B. Produce a GROUND TRUTH REPORT of all advanced features, plugins, frontend components, and configuration in mahresources by reading the actual source code. Do NOT read or modify any docs under docs-site/.

FEATURES TO COVER:

1. PLUGIN SYSTEM (highest priority — most gaps expected here)
   - Plugin discovery and lifecycle (plugin_system/manager.go)
   - Lua API — ALL mah.* functions: mah.db.* (CRUD for every entity type, relationship operations), mah.kv.* (key-value store), mah.http.* (outbound HTTP), mah.log(), mah.start_job(), mah.job_progress/complete/fail
   - Plugin actions: registration, form parameters, filters, placement, sync vs async execution
   - Plugin hooks: lifecycle events, template injections
   - Plugin pages: custom page serving
   - Plugin block types: custom note block types via plugins
   - Plugin JSON API: custom endpoints at /v1/plugins/{name}/*
   - Plugin settings and management: enable/disable/configure/purge-data
   - Plugin menu items
   Read: plugin_system/*.go, server/api_handlers/plugin_handlers.go, server/routes*.go

2. RESOURCE VERSIONING
   - Version CRUD, restore, compare, cleanup, deduplication
   Read: application_context/version_context.go, models/resource_version_model.go

3. IMAGE SIMILARITY
   - Perceptual hashing (DHash), Hamming distance, background hash worker, LRU cache, similarity threshold
   Read: application_context/hash_*.go, models/image_hash_model.go

4. NOTE BLOCK SYSTEM
   - Built-in block types, block API (CRUD, reorder, rebalance), block state, calendar blocks, table blocks
   Read: application_context/block_context.go, models/note_block_model.go

5. SEARCH / FTS
   - Global search endpoint, FTS5 setup, type filtering, search caching
   Read: application_context/search_context.go, server/api_handlers/search_handlers.go

6. DOWNLOAD QUEUE / JOB SYSTEM
   - Download manager, job states, SSE events, pause/resume/cancel/retry, unified job system for downloads + plugin actions
   Read: application_context/download_*.go, application_context/job_*.go

7. NOTE SHARING
   - Share tokens, share server endpoints, interactive features (todo toggling, calendar events on shared notes)
   Read: server/share_server.go, application_context/share_context.go

8. THUMBNAIL GENERATION
   - Image thumbnails, video thumbnails via ffmpeg, office doc thumbnails via LibreOffice, background thumbnail worker
   Read: application_context/thumbnail_*.go

9. CUSTOM TEMPLATES
   - Pongo2 template system, custom headers/sidebars/summaries/avatars via entity fields
   Read: templates/, server/template_handlers/

10. META SCHEMAS
    - JSON Schema validation for metadata fields
    Read: application_context/meta_schema_context.go

11. ACTIVITY LOG
    - What gets logged, log model, log endpoints, cleanup
    Read: application_context/log_context.go, models/log_entry_model.go

12. FRONTEND COMPONENTS (read src/components/ directory)
    - Paste upload (pasteUpload.js) — global paste interception, modal, batch upload, duplicate detection
    - Quick tag panel (lightbox/quickTagPanel.js) — lightbox side panel, 1-9 key slots, localStorage
    - Entity picker (picker/entityPicker.js) — modal with search, tabs, filters, multi-select
    - Code editor (codeEditor.js) — CodeMirror 6 for SQL/HTML, schema autocompletion
    - Multi-sort (multiSort.js) — multi-column sort criteria builder
    - Confirm action (confirmAction.js) — Shift-to-bypass confirmation
    - Free fields (freeFields.js) — dynamic metadata key-value fields
    - Image compare (imageCompare.js) — side-by-side, slider, overlay, toggle modes
    - Text diff (textDiff.js) — unified/split diff
    - Download cockpit (downloadCockpit.js) — floating download status UI
    - Bulk selection (bulkSelection.js) — checkbox selection with range select
    - ALL keyboard shortcuts across ALL components

13. CONFIGURATION
    - ALL flags and env vars with defaults and descriptions
    Read: main.go (flag definitions)

14. DASHBOARD
    - What data the dashboard shows
    Read: server/template_handlers/, templates/dashboard*

OUTPUT: Write your complete report to /Users/egecan/Code/mahresources/tasks/doc-review/ground-truth-features.md

Format: One ## section per feature. Include exact function names, exact config flag names, exact endpoint paths, exact parameter names. Use tables for config flags, API endpoints, and Lua API functions. This report will be compared word-for-word against documentation.
```

**Step 2: Wait for both summarizers to complete**

Expected: Two report files written:
- `tasks/doc-review/ground-truth-entities.md`
- `tasks/doc-review/ground-truth-features.md`

**Step 3: Verify reports were written and have substance**

```bash
wc -l tasks/doc-review/ground-truth-entities.md tasks/doc-review/ground-truth-features.md
```

Expected: Each report should be 500+ lines. If either is significantly shorter, resume that summarizer agent to fill gaps.

**Step 4: Quick sanity check — verify key items are covered**

Spot-check that reports include:
- Entities report: all 12 entity types listed, NoteBlock block types enumerated, bulk operation endpoints listed
- Features report: all `mah.kv.*` functions listed, all `mah.db.create_*` functions listed, paste upload described, quick tag panel described, all config flags from main.go listed

If major items are missing, resume the relevant summarizer agent with specific instructions to cover the gap.

---

### Task 2: Phase 1b — Conductor Cross-Check (Before Dispatching Checkers)

Before dispatching checkers, verify the ground truth reports are complete enough.

**Step 1: Read both ground truth reports**

Read:
- `tasks/doc-review/ground-truth-entities.md`
- `tasks/doc-review/ground-truth-features.md`

**Step 2: Cross-check against known gap list**

Read `tasks/doc-review/gaps.md` and verify these specific items appear in the ground truth reports:

| Gap # | Item | Should appear in |
|-------|------|-----------------|
| 1 | Plugin CRUD (mah.db.create_group, etc.) | ground-truth-features.md |
| 2 | Plugin relationship ops (mah.db.add_tags, etc.) | ground-truth-features.md |
| 3 | Plugin KV store (mah.kv.*) | ground-truth-features.md |
| 4 | Plugin logging (mah.log) | ground-truth-features.md |
| 5 | Plugin mah.start_job | ground-truth-features.md |
| 6 | Plugin purge-data endpoint | ground-truth-features.md |
| 7 | Paste upload feature | ground-truth-features.md |
| 8 | Quick tag panel | ground-truth-features.md |
| 9 | OpenAPI validator | ground-truth-features.md |
| 10 | Code editor (SQL autocompletion) | ground-truth-features.md |
| 15 | Interactive shared note features | ground-truth-features.md |
| 17 | Keyboard shortcuts | ground-truth-features.md |

If any are missing, note them — they'll be added to the checker reports manually in Task 3.

**Step 3: Note any gaps found for manual addition to checker reports**

Write a brief list of anything the summarizers missed. This list will be appended to the checker prompts in Task 3.

---

### Task 3: Phase 2 — Dispatch Doc Checker Agents (Parallel)

Dispatch two general-purpose subagents simultaneously. They read ground truth + style guide + docs — no edits.

**Step 1: Dispatch both checkers in a single message with two Agent tool calls**

**Checker A prompt (general-purpose agent):**

```
You are Doc Checker A. Compare documentation against ground truth reports and the style guide, then produce an ISSUE REPORT. Do NOT edit any files.

READ THESE FIRST:
1. /Users/egecan/Code/mahresources/tasks/doc-review/ground-truth-entities.md
2. /Users/egecan/Code/mahresources/tasks/doc-review/ground-truth-features.md
3. /Users/egecan/Code/mahresources/tasks/doc-review/style-guide.md

THEN CHECK EVERY ONE OF THESE 18 DOC FILES:
- /Users/egecan/Code/mahresources/docs-site/docs/intro.md
- /Users/egecan/Code/mahresources/docs-site/docs/concepts/overview.md
- /Users/egecan/Code/mahresources/docs-site/docs/concepts/resources.md
- /Users/egecan/Code/mahresources/docs-site/docs/concepts/notes.md
- /Users/egecan/Code/mahresources/docs-site/docs/concepts/note-blocks.md
- /Users/egecan/Code/mahresources/docs-site/docs/concepts/groups.md
- /Users/egecan/Code/mahresources/docs-site/docs/concepts/tags-categories.md
- /Users/egecan/Code/mahresources/docs-site/docs/concepts/relationships.md
- /Users/egecan/Code/mahresources/docs-site/docs/concepts/series.md
- /Users/egecan/Code/mahresources/docs-site/docs/getting-started/installation.md
- /Users/egecan/Code/mahresources/docs-site/docs/getting-started/quick-start.md
- /Users/egecan/Code/mahresources/docs-site/docs/getting-started/first-steps.md
- /Users/egecan/Code/mahresources/docs-site/docs/user-guide/navigation.md
- /Users/egecan/Code/mahresources/docs-site/docs/user-guide/managing-resources.md
- /Users/egecan/Code/mahresources/docs-site/docs/user-guide/managing-notes.md
- /Users/egecan/Code/mahresources/docs-site/docs/user-guide/organizing-with-groups.md
- /Users/egecan/Code/mahresources/docs-site/docs/user-guide/search.md
- /Users/egecan/Code/mahresources/docs-site/docs/user-guide/bulk-operations.md

FOR EACH DOC FILE, report under these categories:

1. INACCURATE — Claims that don't match the ground truth reports. Quote the exact text from the doc. State what the ground truth says instead. Include line numbers.

2. MISSING — Features from the ground truth that SHOULD be mentioned in this doc but aren't. Be specific: "The paste upload feature (from ground-truth-features.md § Frontend Components) should be documented in managing-resources.md"

3. AI-SLOP — Lines that violate the style guide's banned phrase list (Section 2 of style-guide.md). Quote the exact line. Name the specific banned phrase. Suggest the fix per the style guide's replacement column.

4. OUTDATED — Descriptions of features that have changed since docs were written. Compare the doc's description against the ground truth and flag differences.

5. PRIORITY — Rate the doc: KEEP (no changes needed), EDIT (minor fixes — fewer than 5 issues), REWRITE (major issues — 5+ issues or factual errors throughout)

If a doc has NO issues in a category, write "None" for that category. Do NOT skip any doc file.

IMPORTANT: Also check these specific known gaps that should appear in these docs:
- intro.md: Should mention plugin KV store, plugin entity CRUD, paste upload
- user-guide/navigation.md: Should have Quick Tag Panel section, consolidated keyboard shortcuts table, multi-sort UI description, Shift-to-bypass confirmation mention
- user-guide/managing-resources.md: Should have Paste Upload section
- concepts/note-blocks.md: Should mention plugin-defined custom block types

OUTPUT: Write your complete report to /Users/egecan/Code/mahresources/tasks/doc-review/checker-report-a.md

End the report with a SUMMARY table:
| File | Priority | Issue Count | Biggest Issue |
```

**Checker B prompt (general-purpose agent):**

```
You are Doc Checker B. Compare documentation against ground truth reports and the style guide, then produce an ISSUE REPORT. Do NOT edit any files.

READ THESE FIRST:
1. /Users/egecan/Code/mahresources/tasks/doc-review/ground-truth-entities.md
2. /Users/egecan/Code/mahresources/tasks/doc-review/ground-truth-features.md
3. /Users/egecan/Code/mahresources/tasks/doc-review/style-guide.md

THEN CHECK EVERY ONE OF THESE 32 DOC FILES:
- /Users/egecan/Code/mahresources/docs-site/docs/features/versioning.md
- /Users/egecan/Code/mahresources/docs-site/docs/features/image-similarity.md
- /Users/egecan/Code/mahresources/docs-site/docs/features/saved-queries.md
- /Users/egecan/Code/mahresources/docs-site/docs/features/custom-templates.md
- /Users/egecan/Code/mahresources/docs-site/docs/features/meta-schemas.md
- /Users/egecan/Code/mahresources/docs-site/docs/features/note-sharing.md
- /Users/egecan/Code/mahresources/docs-site/docs/features/download-queue.md
- /Users/egecan/Code/mahresources/docs-site/docs/features/job-system.md
- /Users/egecan/Code/mahresources/docs-site/docs/features/activity-log.md
- /Users/egecan/Code/mahresources/docs-site/docs/features/thumbnail-generation.md
- /Users/egecan/Code/mahresources/docs-site/docs/features/custom-block-types.md
- /Users/egecan/Code/mahresources/docs-site/docs/features/entity-picker.md
- /Users/egecan/Code/mahresources/docs-site/docs/features/plugin-system.md
- /Users/egecan/Code/mahresources/docs-site/docs/features/plugin-actions.md
- /Users/egecan/Code/mahresources/docs-site/docs/features/plugin-hooks.md
- /Users/egecan/Code/mahresources/docs-site/docs/features/plugin-lua-api.md
- /Users/egecan/Code/mahresources/docs-site/docs/api/overview.md
- /Users/egecan/Code/mahresources/docs-site/docs/api/resources.md
- /Users/egecan/Code/mahresources/docs-site/docs/api/notes.md
- /Users/egecan/Code/mahresources/docs-site/docs/api/groups.md
- /Users/egecan/Code/mahresources/docs-site/docs/api/plugins.md
- /Users/egecan/Code/mahresources/docs-site/docs/api/other-endpoints.md
- /Users/egecan/Code/mahresources/docs-site/docs/configuration/overview.md
- /Users/egecan/Code/mahresources/docs-site/docs/configuration/database.md
- /Users/egecan/Code/mahresources/docs-site/docs/configuration/storage.md
- /Users/egecan/Code/mahresources/docs-site/docs/configuration/advanced.md
- /Users/egecan/Code/mahresources/docs-site/docs/deployment/docker.md
- /Users/egecan/Code/mahresources/docs-site/docs/deployment/systemd.md
- /Users/egecan/Code/mahresources/docs-site/docs/deployment/reverse-proxy.md
- /Users/egecan/Code/mahresources/docs-site/docs/deployment/public-sharing.md
- /Users/egecan/Code/mahresources/docs-site/docs/deployment/backups.md
- /Users/egecan/Code/mahresources/docs-site/docs/troubleshooting.md

FOR EACH DOC FILE, report under these categories:

1. INACCURATE — Claims that don't match the ground truth reports. Quote the exact text from the doc. State what the ground truth says instead. Include line numbers.

2. MISSING — Features from the ground truth that SHOULD be mentioned in this doc but aren't. Be specific about which ground truth section the missing info comes from.

3. AI-SLOP — Lines that violate the style guide's banned phrase list (Section 2 of style-guide.md). Quote the exact line. Name the specific banned phrase. Suggest the fix per the style guide's replacement column.

4. OUTDATED — Descriptions of features that have changed since docs were written.

5. PRIORITY — Rate the doc: KEEP (no changes needed), EDIT (minor fixes — fewer than 5 issues), REWRITE (major issues — 5+ issues or factual errors throughout)

If a doc has NO issues in a category, write "None" for that category. Do NOT skip any doc file.

IMPORTANT: Also check these specific known gaps that MUST appear in the report:
- features/plugin-lua-api.md: MUST flag missing mah.db.create_*/update_*/patch_*/delete_* CRUD functions, missing mah.db.add_tags/remove_tags/add_groups/remove_groups relationship functions, missing mah.kv.* section, missing mah.log(), missing mah.start_job(). The page header "Read access to all entity types and write access for Resource creation" is FACTUALLY WRONG — full CRUD is available.
- features/plugin-system.md: MUST flag missing purge-data endpoint in management API table
- api/plugins.md: MUST flag missing purge-data endpoint
- features/note-sharing.md: MUST flag missing interactive shared note features (todo toggling, calendar events)
- features/saved-queries.md: MUST flag missing SQL editor capabilities (CodeMirror, autocompletion)
- api/overview.md: MUST flag missing OpenAPI validator command
- configuration/advanced.md: Check all config flags against ground truth — any missing flags?

OUTPUT: Write your complete report to /Users/egecan/Code/mahresources/tasks/doc-review/checker-report-b.md

End the report with a SUMMARY table:
| File | Priority | Issue Count | Biggest Issue |
```

**Step 2: Wait for both checkers to complete**

Expected: Two report files written:
- `tasks/doc-review/checker-report-a.md`
- `tasks/doc-review/checker-report-b.md`

**Step 3: Verify reports cover all doc files**

```bash
grep -c "^## " tasks/doc-review/checker-report-a.md
grep -c "^## " tasks/doc-review/checker-report-b.md
```

Expected: Checker A should have ~18 sections, Checker B should have ~32 sections. If significantly fewer, resume the agent to complete missing files.

**Step 4: Review summary tables**

Read the SUMMARY tables at the end of each report. Note how many docs are KEEP vs EDIT vs REWRITE. This determines the scope of work for the writing coach.

---

### Task 4: Phase 3 — Dispatch Writing Coach + Screenshot Agent (Parallel)

Dispatch both agents simultaneously.

**Step 1: Dispatch both agents in a single message with two Agent tool calls**

**Writing Coach prompt (general-purpose agent, mode: "auto" so it can edit files):**

```
You are the Writing Coach. Fix all documentation issues found by the doc checkers. You CAN and SHOULD edit files.

READ THESE FILES FIRST (in this order):
1. Style guide: /Users/egecan/Code/mahresources/tasks/doc-review/style-guide.md
2. Checker report A: /Users/egecan/Code/mahresources/tasks/doc-review/checker-report-a.md
3. Checker report B: /Users/egecan/Code/mahresources/tasks/doc-review/checker-report-b.md
4. Ground truth (entities): /Users/egecan/Code/mahresources/tasks/doc-review/ground-truth-entities.md
5. Ground truth (features): /Users/egecan/Code/mahresources/tasks/doc-review/ground-truth-features.md

YOUR RULES:

WHAT TO FIX:
- Every INACCURATE item from both checker reports — correct the text to match ground truth
- Every MISSING item from both checker reports — add the content to the appropriate doc
- Every AI-SLOP item from both checker reports — rewrite per the style guide's replacement rules
- Every OUTDATED item — update to match current ground truth

WHAT NOT TO DO:
- Do NOT touch docs rated KEEP unless they have specific issues listed
- Do NOT restructure docs that are working — only fix the specific issues
- Do NOT add screenshot references — the conductor handles that in Phase 4
- Do NOT add enthusiasm, exclamation marks, or unnecessary admonitions
- Do NOT use any phrase from the style guide's banned list (Section 2)
- Do NOT use placeholder values — use realistic Mahresources defaults (port 8181, SQLite, ./files, etc.)

STYLE RULES (from style guide):
- Second person ("you"), present tense
- Bare imperatives for instructions
- Every claim backed by example, code block, or table
- No hedging unless genuinely uncertain
- Use terminology canon from Section 5 of style guide exactly
- Page opener: 1 sentence maximum
- Max paragraph: 4 sentences
- Tables for 4+ parallel items
- Code blocks always have language tags
- Every config option: show flag, env var, and example
- Every API endpoint: show curl request and JSON response

NEW DOC PAGES:
If a feature has its own UI page or 3+ API endpoints and is not documented anywhere, create a new doc page. Include Docusaurus frontmatter:
```yaml
---
sidebar_position: N
title: Page Title
---
```

WORKFLOW:
1. Process all REWRITE-priority docs first (most work, highest impact)
2. Then all EDIT-priority docs
3. For each doc: read it, read the relevant checker report issues, read the relevant ground truth section, make ALL fixes
4. Track every file you change and every new file you create

WHEN DONE:
Write a summary to /Users/egecan/Code/mahresources/tasks/doc-review/writing-coach-summary.md listing:
- Every file edited and what changed (1-2 sentences per file)
- Every new file created and what it covers
- Any issues from the checker reports you could NOT resolve (with explanation)
```

**Screenshot Agent prompt (general-purpose agent, mode: "auto" so it can run commands and write files):**

```
You are the Screenshot Agent. Your job is to analyze existing screenshots, plan new ones, seed an ephemeral server with realistic data, and capture new screenshots via Playwright.

PHASE A: ANALYZE EXISTING SCREENSHOTS

Read every PNG file in /Users/egecan/Code/mahresources/docs-site/static/img/ as images (use the Read tool — it can read images). For each screenshot, note:
- What page it shows
- What data appears in it
- Whether it looks populated and useful

The existing screenshots are:
dashboard.png, grid-view.png, resource-detail.png, upload-form.png, note-blocks.png, group-tree.png, group-detail.png, search-results.png, query-editor.png, activity-log.png, tag-list.png, note-list.png, group-list.png, resource-detail-view.png, note-edit.png, group-edit.png, category-list.png, global-search.png, bulk-selection.png

PHASE B: PLAN NEW SCREENSHOTS

Read the checker reports to understand which new features need screenshots:
- /Users/egecan/Code/mahresources/tasks/doc-review/checker-report-a.md
- /Users/egecan/Code/mahresources/tasks/doc-review/checker-report-b.md

Determine which NEW screenshots are needed. Likely candidates (verify against checker reports):
- plugin-management.png: Plugin list/management page (/plugins/manage)
- paste-upload.png: Paste upload modal (if possible to trigger via Playwright)
- download-cockpit.png: Download queue UI (/dashboard with downloads)
- version-compare.png: Side-by-side version comparison (/resource/compare)
- quick-tag-panel.png: Lightbox with quick tag panel open

For each new screenshot, plan:
- What page URL to navigate to
- What data needs to be seeded first
- What interactions are needed (clicks, key presses)
- The filename

PHASE C: BUILD AND SEED

Step 1: Build the application
```bash
cd /Users/egecan/Code/mahresources && npm run build
```

Step 2: Start ephemeral server (use a port that's likely free)
```bash
cd /Users/egecan/Code/mahresources && ./mahresources -ephemeral -bind-address=:8282 -max-db-connections=2 -hash-worker-disabled -plugins-disabled &
```

Step 3: Wait for server to respond
```bash
curl -s -o /dev/null -w "%{http_code}" http://localhost:8282/
```

Step 4: Seed data via API. Order matters:
1. Categories (POST /v1/category)
2. Resource categories (POST /v1/resourceCategory)
3. Tags (POST /v1/tag)
4. Note types (POST /v1/note/noteType)
5. Groups with hierarchy (POST /v1/group, use ownerId for parent-child)
6. Relation types (POST /v1/relationType)
7. Relations (POST /v1/relation)
8. Resources — upload actual image files from e2e/test-assets/ (POST /v1/resource with multipart form)
9. Add tags to resources (POST /v1/resources/addTags)
10. Add resources to groups (POST /v1/resources/addGroups)
11. Notes with groups and tags (POST /v1/note)
12. Note blocks — heading, text, todos (POST /v1/note/block with JSON body)
13. Saved query (POST /v1/query)

Create enough data to make screenshots meaningful:
- 2+ categories, 2+ resource categories
- 6+ tags with descriptive names
- 2+ note types
- 4+ groups with hierarchy (at least one parent with 2 children)
- 8+ resources (use sample images from e2e/test-assets/)
- 3+ notes with blocks (heading, text, todos)
- 1 saved query with working SQL
- 1 relation type and 1 relation

Verify seeding:
```bash
curl -s http://localhost:8282/v1/resources.json | python3 -c "import sys,json; d=json.load(sys.stdin); print(f'{len(d)} resources')"
```

PHASE D: CAPTURE SCREENSHOTS

Use Playwright to capture screenshots. The e2e/ directory has Playwright set up.

Create a Node.js script at /Users/egecan/Code/mahresources/e2e/scripts/capture-new-screenshots.js:

```javascript
const { chromium } = require('playwright');
const path = require('path');
const fs = require('fs');

const BASE_URL = process.env.BASE_URL || 'http://localhost:8282';
const OUTPUT_DIR = path.resolve(__dirname, '../../docs-site/static/img');

async function main() {
  fs.mkdirSync(OUTPUT_DIR, { recursive: true });

  const browser = await chromium.launch();
  const context = await browser.newContext({
    viewport: { width: 1200, height: 800 },
    colorScheme: 'light',
  });
  const page = await context.newPage();

  // Add your planned screenshots here. Example:
  const screenshots = [
    // Add entries based on your Phase B plan
  ];

  for (const shot of screenshots) {
    try {
      console.log(`Capturing ${shot.name}: ${shot.url}`);
      await page.goto(`${BASE_URL}${shot.url}`, { waitUntil: 'networkidle' });
      await page.waitForTimeout(500);
      if (shot.waitFor) {
        try { await page.waitForSelector(shot.waitFor, { timeout: 5000 }); } catch {}
      }
      if (shot.interactions) {
        for (const action of shot.interactions) {
          await action(page);
        }
        await page.waitForTimeout(300);
      }
      await page.screenshot({ path: path.join(OUTPUT_DIR, shot.name), fullPage: false });
      console.log(`  Saved ${shot.name}`);
    } catch (err) {
      console.error(`  Error: ${err.message}`);
    }
  }

  await browser.close();
}

main().catch(console.error);
```

Run it:
```bash
cd /Users/egecan/Code/mahresources && node e2e/scripts/capture-new-screenshots.js
```

PHASE E: CREATE SCREENSHOT MANIFEST

Write a complete manifest to /Users/egecan/Code/mahresources/docs-site/static/img/screenshot-manifest.json covering ALL screenshots (existing + new). Format:

```json
{
  "version": 1,
  "screenshots": {
    "filename.png": {
      "page": "/url-path",
      "description": "What the screenshot shows",
      "seedDependencies": ["categories", "groups", "resources"],
      "seedDetails": "Specific data needed for this screenshot",
      "viewport": { "width": 1200, "height": 800 },
      "interactions": ["list of user actions needed"],
      "capturedDate": "2026-03-08"
    }
  }
}
```

For existing screenshots, set capturedDate to "2026-03-07" and describe what you observed in Phase A.
For new screenshots, set capturedDate to "2026-03-08".

PHASE F: CLEANUP

Stop the ephemeral server:
```bash
kill $(lsof -ti:8282) 2>/dev/null || true
```

Delete the temporary capture script:
```bash
rm -f /Users/egecan/Code/mahresources/e2e/scripts/capture-new-screenshots.js
```

WHEN DONE:
Write a summary listing all new screenshots captured and their filenames.
```

**Step 2: Wait for both agents to complete**

Expected:
- Writing coach: edited doc files + `tasks/doc-review/writing-coach-summary.md`
- Screenshot agent: new PNGs in `docs-site/static/img/` + `docs-site/static/img/screenshot-manifest.json`

---

### Task 5: Phase 4 — Conductor Final Review

**Step 1: Read the writing coach summary**

Read: `tasks/doc-review/writing-coach-summary.md`

Note: files changed, files created, unresolved issues.

**Step 2: Read the screenshot manifest**

Read: `docs-site/static/img/screenshot-manifest.json`

Note: new screenshots and their filenames.

**Step 3: Add screenshot references to docs**

For each NEW screenshot, add an image reference in the appropriate doc file. Use Docusaurus format:

```markdown
![Description](/img/filename.png)
```

Place screenshots near the text they illustrate. Mapping:

| Screenshot | Doc file | Placement |
|------------|----------|-----------|
| plugin-management.png | features/plugin-system.md | Near "Managing Plugins" section |
| download-cockpit.png | features/download-queue.md | Near UI description |
| version-compare.png | features/versioning.md | Near comparison section |
| (others based on manifest) | (determined by content) | (near relevant text) |

**Step 4: Update sidebars.ts if new doc pages were created**

Read the writing coach summary. If new pages were created, add them to `docs-site/sidebars.ts` in the appropriate category.

**Step 5: AI-slop scan**

Grep all docs for banned phrases from the style guide:

```bash
cd /Users/egecan/Code/mahresources
grep -rni "seamlessly\|leverages\|robust\|streamlined\|effortlessly\|powerful\|comprehensive\|extensive\|under the hood\|out of the box\|best practices\|and much more\|feel free\|please note\|it should be noted\|designed to\|provides a\|enables you to\|makes it easy\|take advantage\|you can easily\|straightforward\|worth noting\|worth mentioning\|importantly\|in this section\|in this page\|let's\|we'll" docs-site/docs/
```

Fix any hits found. These are the writing coach's misses.

**Step 6: Verify docs site builds**

```bash
cd /Users/egecan/Code/mahresources/docs-site && npm run build
```

Expected: Build succeeds. If it fails, fix broken links or missing references.

**Step 7: Verify new screenshots exist and have reasonable size**

```bash
ls -la docs-site/static/img/*.png | awk '{print $5, $9}' | sort -rn
```

Expected: All PNGs > 10KB. Any tiny files may indicate capture failures.

**Step 8: Spot-check 5 edited docs**

Pick 5 docs that were rated REWRITE by the checkers. For each:
1. Read the doc
2. Check for any remaining AI-slop
3. Verify technical claims against the ground truth report
4. Verify terminology matches the style guide's canon

**Step 9: Commit**

```bash
cd /Users/egecan/Code/mahresources
git add docs-site/docs/ docs-site/static/img/ docs-site/sidebars.ts tasks/doc-review/
git commit -m "docs: comprehensive docs update v3 — accuracy, coverage, screenshots, de-slop

- Cross-referenced all 50 docs against codebase ground truth
- Fixed accuracy issues (wrong endpoints, missing params, outdated descriptions)
- Filled coverage gaps for plugin CRUD, KV store, paste upload, quick tag panel
- Captured new screenshots for undocumented features
- Created screenshot-manifest.json for reproducible future captures
- Removed AI-slop phrases per style guide banned list
- Updated ground truth reports and checker reports for future reference

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

## Execution Summary

| Phase | Task | Agents | Parallelism | Depends On |
|-------|------|--------|-------------|------------|
| 1 | Task 1 | Summarizer A + Summarizer B | Parallel | — |
| 1b | Task 2 | Conductor (manual) | Sequential | Task 1 |
| 2 | Task 3 | Checker A + Checker B | Parallel | Task 2 |
| 3 | Task 4 | Writing Coach + Screenshot Agent | Parallel | Task 3 |
| 4 | Task 5 | Conductor (manual) | Sequential | Task 4 |

Total: 5 tasks, 6 subagents, 2 conductor review steps.

## Rollback Plan

If results are unsatisfactory after Phase 4:

```bash
git diff HEAD~1 --stat  # See what changed
git stash               # Stash all changes
```

Individual agent outputs are preserved in `tasks/doc-review/` for selective re-application.
