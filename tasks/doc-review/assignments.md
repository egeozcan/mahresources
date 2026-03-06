# Writer Assignments

Based on audit verdicts from Phase 2 and gap analysis.

**Rules for all writers:**
- Read `tasks/doc-review/style-guide.md` before writing anything.
- Read the relevant audit file for your section to see specific issues per file.
- Read the relevant inventory files for source-of-truth data.
- OK files: skip entirely.
- PATCH files: fix only the specific issues listed in the audit.
- REWRITE files: write from scratch using style guide templates + inventory data.
- Do NOT touch `sidebars.ts` — conductor handles that.
- Do NOT modify files outside your assignment.
- Self-check: grep your output for every banned phrase before finishing.
- Preserve existing Docusaurus frontmatter (sidebar_position, title) unless the audit says otherwise.

---

## Writer A: Getting Started + Concepts

**Branch:** docs/writer-a
**Audit file:** `tasks/doc-review/audit-concepts-userguide.md` (concepts section only), `tasks/doc-review/audit-config-deploy-misc.md` (intro only)
**Inventory files:** `inventory-entities.md`, `inventory-features.md`

| File | Action | Key Issues |
|------|--------|------------|
| `docs/intro.md` | PATCH | Add missing features to list (note blocks, download queue, series, activity log, note sharing, custom templates, meta schemas). Fix vague plugin description — name Lua, list capabilities. |
| `docs/getting-started/installation.md` | PATCH | Check Go version in Dockerfile example. Add ImageMagick as optional dependency for SVG thumbnails. |
| `docs/getting-started/quick-start.md` | PATCH | Fix port 8080 → 8181 in all examples. |
| `docs/getting-started/first-steps.md` | OK | Skip. |
| `docs/concepts/overview.md` | PATCH | Add Series and LogEntry to entity table (9 types, not 7). Fix opener to 1 sentence. Add HAS_KEYS as 9th meta operator. Fix search syntax (prefix mode for >=3 chars, add fuzzy/exact modes). Add replaceTags and addGroups to bulk ops. |
| `docs/concepts/resources.md` | PATCH | Fix thumbnail claim — generated on-demand, not at upload. Check all fields against entity inventory. |
| `docs/concepts/notes.md` | OK | Skip. |
| `docs/concepts/note-blocks.md` | PATCH | Fix References schema: uses `{"groupIds": [...]}` not multi-entity items. Fix Todos: uses `"label"` not `"text"`. Fix Table: uses `"queryId"/"queryParams"/"isStatic"/"columns"/"rows"` not `"queryName"/"params"`. |
| `docs/concepts/groups.md` | PATCH | Fix category deletion behavior: CASCADE not SET NULL (confirmed by entity inventory gorm tag). |
| `docs/concepts/tags-categories.md` | PATCH | Per audit — check specific issues. |
| `docs/concepts/relationships.md` | OK | Skip. |
| `docs/concepts/series.md` | OK | Skip. |

---

## Writer B: User Guide + Configuration

**Branch:** docs/writer-b
**Audit files:** `tasks/doc-review/audit-concepts-userguide.md` (user guide section), `tasks/doc-review/audit-config-deploy-misc.md` (configuration section)
**Inventory files:** `inventory-features.md`, `inventory-api.md`
**Gap items assigned:** #7 (paste upload), #8 (quick tag panel), #11 (multi-sort UI), #12 (shift-to-bypass), #17 (keyboard shortcuts), #18 (free-form metadata)

| File | Action | Key Issues |
|------|--------|------------|
| `docs/user-guide/navigation.md` | PATCH | Fix search result limit (200 not 50). Add consolidated keyboard shortcuts table (gap #17). Add Quick Tag Panel section for lightbox (gap #8). Add multi-sort UI description (gap #11). Add shift-to-bypass-confirmation (gap #12). |
| `docs/user-guide/managing-resources.md` | PATCH | Fix thumbnail timing (on-demand, not at upload). Add paste upload section (gap #7). |
| `docs/user-guide/managing-notes.md` | PATCH | Per audit — check specific issues. |
| `docs/user-guide/organizing-with-groups.md` | OK | Skip. |
| `docs/user-guide/search.md` | PATCH | Fix search result limit (200 not 50). Check search syntax against inventory. |
| `docs/user-guide/bulk-operations.md` | PATCH | Per audit — check specific issues. |
| `docs/configuration/overview.md` | PATCH | Remove banned phrase "This allows you to". |
| `docs/configuration/database.md` | OK | Skip. |
| `docs/configuration/storage.md` | OK | Skip. |
| `docs/configuration/advanced.md` | OK | Skip. |

---

## Writer C: Features

**Branch:** docs/writer-c
**Audit file:** `tasks/doc-review/audit-features-api.md` (features section)
**Inventory files:** `inventory-features.md`, `inventory-entities.md`, `inventory-api.md`
**Gap items assigned:** #1-5 (plugin Lua API additions), #6 (purge endpoint in plugin-system.md), #10 (code editor), #15 (interactive shared notes)

| File | Action | Key Issues |
|------|--------|------------|
| `docs/features/versioning.md` | PATCH | Fix endpoint paths (resourceId not id, /resources/versions/cleanup not /versions/bulk-cleanup). Add missing endpoints (version by ID, version file download, POST delete alias). Add env var for -skip-version-migration. |
| `docs/features/image-similarity.md` | PATCH | Per audit — check specific issues. |
| `docs/features/saved-queries.md` | PATCH | Add code editor section describing CodeMirror SQL autocompletion (gap #10). |
| `docs/features/custom-templates.md` | PATCH | Per audit — check specific issues. |
| `docs/features/meta-schemas.md` | PATCH | Per audit — check specific issues. Add free-form metadata field UI description (gap #18). |
| `docs/features/note-sharing.md` | PATCH | Add interactive shared note features — todo toggling, calendar events on shared views (gap #15). |
| `docs/features/download-queue.md` | PATCH | Per audit — check specific issues. |
| `docs/features/job-system.md` | PATCH | Per audit — check specific issues. |
| `docs/features/activity-log.md` | PATCH | Per audit — check specific issues. |
| `docs/features/thumbnail-generation.md` | PATCH | Per audit — check specific issues. |
| `docs/features/custom-block-types.md` | PATCH | Fix "Validation Best Practices" heading (banned phrase). Per audit — check other issues. |
| `docs/features/entity-picker.md` | OK | Skip. |
| `docs/features/plugin-system.md` | PATCH | Add purge-data endpoint to management API table (gap #6). Per audit — check other issues. |
| `docs/features/plugin-actions.md` | PATCH | Per audit — check specific issues. |
| `docs/features/plugin-hooks.md` | PATCH | Per audit — check specific issues. |
| `docs/features/plugin-lua-api.md` | REWRITE | Massively outdated. Add ~30 mah.db CRUD functions (gap #1), relationship management functions (gap #2), mah.kv module (gap #3), mah.log (gap #4), mah.start_job (gap #5). Fix header claim about "read access only". |

---

## Writer D: API + Deployment + Troubleshooting

**Branch:** docs/writer-d
**Audit file:** `tasks/doc-review/audit-features-api.md` (API section), `tasks/doc-review/audit-config-deploy-misc.md` (deployment + troubleshooting)
**Inventory files:** `inventory-api.md`, `inventory-features.md`
**Gap items assigned:** #6 (purge endpoint in api/plugins.md), #9 (OpenAPI validator)

| File | Action | Key Issues |
|------|--------|------------|
| `docs/api/overview.md` | PATCH | Add OpenAPI validator command (gap #9). Per audit — check other issues. |
| `docs/api/resources.md` | PATCH | Fix resource/view — returns 302 redirect, not streaming content. Fix query parameter names per audit. |
| `docs/api/notes.md` | PATCH | Fix query parameter names (blockId not id, etc.). Per audit — check specific issues. |
| `docs/api/groups.md` | PATCH | Per audit — check specific issues. |
| `docs/api/plugins.md` | PATCH | Add purge-data endpoint (gap #6). Per audit — check other issues. |
| `docs/api/other-endpoints.md` | PATCH | Fix endpoint paths per audit. Check series endpoints (seriesList not series/list). |
| `docs/deployment/docker.md` | PATCH | Check Go version in Dockerfile template. |
| `docs/deployment/systemd.md` | OK | Skip. |
| `docs/deployment/reverse-proxy.md` | PATCH | Add SSE proxy configuration guidance. Fix minor style issue. |
| `docs/deployment/public-sharing.md` | OK | Skip. |
| `docs/deployment/backups.md` | PATCH | Add plugin data and alt-fs backup guidance. |
| `docs/troubleshooting.md` | PATCH | Replace placeholder "your-database.db" with realistic value. Add thumbnail worker and video thumb timeout flags. |
