# Gap Analysis: Inventory vs Documentation

Compared all three inventories (entities, features, API) against all 50 doc files.

---

## 1. Plugin Lua API -- CRUD Operations for All Entity Types

**What exists in code:** `plugin_system/db_api.go` implements full CRUD for groups, notes, tags, categories, resource categories, note types, group relations, and relation types. Functions: `mah.db.create_group`, `mah.db.update_group`, `mah.db.patch_group`, `mah.db.delete_group`, and the same pattern for notes, tags, categories, resource categories, note types, group relations, and relation types. Also: `mah.db.delete_resource`.

**Current doc coverage:** None. `features/plugin-lua-api.md` documents only `get_*`, `query_*`, `get_resource_data`, `create_resource_from_url`, and `create_resource_from_data`. The page's section header says "Read access to all entity types and write access for Resource creation" -- this is factually wrong. Full CRUD is available.

**Recommendation:** ADD SECTION to `features/plugin-lua-api.md` -- add a "mah.db -- Write Operations" section covering all create/update/patch/delete functions for every entity type.

**Suggested sidebar location:** No change needed (existing page).

---

## 2. Plugin Lua API -- Relationship Management Functions

**What exists in code:** `plugin_system/db_api.go` implements `mah.db.add_tags(entity_type, id, tag_ids)`, `mah.db.remove_tags(entity_type, id, tag_ids)`, `mah.db.add_groups(entity_type, id, group_ids)`, `mah.db.remove_groups(entity_type, id, group_ids)`, `mah.db.add_resources_to_note(note_id, resource_ids)`, `mah.db.remove_resources_from_note(note_id, resource_ids)`.

**Current doc coverage:** None. Not mentioned in any doc file.

**Recommendation:** ADD SECTION to `features/plugin-lua-api.md` -- add a "mah.db -- Relationship Operations" section.

**Suggested sidebar location:** No change needed (existing page).

---

## 3. Plugin Lua API -- Key-Value Store (mah.kv)

**What exists in code:** `plugin_system/kv_api.go` implements `mah.kv.get(key)`, `mah.kv.set(key, value)`, `mah.kv.delete(key)`, `mah.kv.list([prefix])`. Backed by `PluginKV` model in database. Scoped per-plugin.

**Current doc coverage:** None. Not mentioned in any doc file.

**Recommendation:** ADD SECTION to `features/plugin-lua-api.md` -- add a "mah.kv -- Key-Value Store" section.

**Suggested sidebar location:** No change needed (existing page).

---

## 4. Plugin Lua API -- Logging (mah.log)

**What exists in code:** `plugin_system/manager.go` registers `mah.log(level, message, [details])` which delegates to the application logger with plugin name as entity name.

**Current doc coverage:** None. Not mentioned in any doc file.

**Recommendation:** ADD SECTION to `features/plugin-lua-api.md`.

**Suggested sidebar location:** No change needed (existing page).

---

## 5. Plugin Lua API -- Background Jobs (mah.start_job)

**What exists in code:** `plugin_system/action_jobs.go` implements `mah.start_job(label, fn)` which creates an async job and runs fn(job_id) in a background goroutine with semaphore limiting.

**Current doc coverage:** None. `mah.job_progress`, `mah.job_complete`, and `mah.job_fail` are listed in a table in `plugin-lua-api.md` but `mah.start_job` is not.

**Recommendation:** ADD to `features/plugin-lua-api.md` alongside the existing job progress functions.

**Suggested sidebar location:** No change needed (existing page).

---

## 6. Plugin Data Purge Endpoint

**What exists in code:** `POST /v1/plugin/purge-data` handler in `server/api_handlers/plugin_handlers.go`. Deletes all KV data for a plugin. Requires plugin to be disabled first.

**Current doc coverage:** None. The plugin management API table in `features/plugin-system.md` lists only enable, disable, manage, and settings. The `api/plugins.md` page also omits it.

**Recommendation:** ADD to both `features/plugin-system.md` (Management API table) and `api/plugins.md`.

**Suggested sidebar location:** No change needed (existing pages).

---

## 7. Paste Upload Feature

**What exists in code:** `src/components/pasteUpload.js` -- intercepts global paste events, provides modal for uploading pasted images/files/HTML/text as resources. Supports batch uploads, duplicate detection, tag/category/series assignment, and page morphing.

**Current doc coverage:** None. No doc page mentions paste-to-upload functionality.

**Recommendation:** ADD SECTION to `user-guide/managing-resources.md` under a "Paste Upload" heading.

**Suggested sidebar location:** Features section or User Guide > Managing Resources.

---

## 8. Quick Tag Panel (Lightbox)

**What exists in code:** `src/components/lightbox/quickTagPanel.js` -- side panel in the lightbox with 9 configurable tag slots persisted to localStorage. Number keys 1-9 toggle tag slots. One-click tag assignment while browsing images.

**Current doc coverage:** None. The lightbox is documented in `user-guide/navigation.md` but Quick Tag Panel is not mentioned. The Edit Panel is mentioned.

**Recommendation:** ADD SECTION to `user-guide/navigation.md` in the Lightbox section.

**Suggested sidebar location:** No change needed (existing page).

---

## 9. OpenAPI Spec Validator

**What exists in code:** `cmd/openapi-gen/validate.go` -- validates a generated OpenAPI spec file. Invoked as `go run ./cmd/openapi-gen/validate.go <spec-file>`.

**Current doc coverage:** Partial. `api/overview.md` documents generating the spec but not validating it. The CLAUDE.md mentions the validate command.

**Recommendation:** ADD to `api/overview.md` in the OpenAPI Specification section.

**Suggested sidebar location:** No change needed (existing page).

---

## 10. Code Editor Component (SQL/HTML)

**What exists in code:** `src/components/codeEditor.js` -- CodeMirror 6 wrapper for SQL and HTML editing. SQL mode fetches schema from `/v1/query/schema` for autocompletion. Used on query create/edit pages and HTML template fields.

**Current doc coverage:** None explicitly. `features/saved-queries.md` mentions the query editor UI but does not describe the CodeMirror integration, syntax highlighting, or auto-completion features.

**Recommendation:** ADD SECTION to `features/saved-queries.md` describing the SQL editor capabilities.

**Suggested sidebar location:** No change needed (existing page).

---

## 11. Multi-Sort UI Component

**What exists in code:** `src/components/multiSort.js` -- Alpine.js component for building multi-column sort criteria on entity list pages. Supports adding/removing/reordering sort criteria, asc/desc direction, and metadata key sorting.

**Current doc coverage:** None. `api/overview.md` documents the `SortBy` query parameter but no doc describes the UI for building multi-sort criteria.

**Recommendation:** ADD SECTION to `user-guide/navigation.md` or a new section in the relevant user guide page about filtering and sorting.

**Suggested sidebar location:** User Guide section.

---

## 12. Confirm Action Component (Shift-to-bypass)

**What exists in code:** `src/components/confirmAction.js` -- wraps delete forms with confirmation dialog. Holding Shift bypasses confirmation.

**Current doc coverage:** None. No doc mentions the Shift-to-bypass-confirmation keyboard shortcut.

**Recommendation:** ADD to `user-guide/navigation.md` keyboard shortcuts section, or mention in relevant user guide pages where delete actions are described.

**Suggested sidebar location:** No change needed.

---

## 13. Image/Text Comparison Tools (Version Comparison Page)

**What exists in code:** `src/components/imageCompare.js` (side-by-side, slider, overlay, toggle modes), `src/components/textDiff.js` (unified/split diff with line-level diffing), `src/components/compareView.js` (URL state management for comparison page).

**Current doc coverage:** Partial. `features/versioning.md` mentions the comparison page and lists the image comparison modes and text diff. But the actual component capabilities are not fully documented (e.g., slider drag, opacity blend, swap sides).

**Recommendation:** No action needed -- the current coverage in `features/versioning.md` is adequate for user-facing documentation.

---

## 14. Saved Settings Persistence (savedSetting Store)

**What exists in code:** `src/components/storeConfig.js` -- persists UI settings (checkbox states, input values) to localStorage/sessionStorage.

**Current doc coverage:** None. This is an internal UI mechanism, not user-facing.

**Recommendation:** No action needed -- internal implementation detail.

---

## 15. Share Server -- Full Feature Documentation Beyond Deployment

**What exists in code:** `server/share_server.go` -- the share server has 4 endpoints: note view, block state update (todos), calendar events, and resource serving. Shared notes support interactive todo checkboxes and calendar views with custom events.

**Current doc coverage:** Partial. `deployment/public-sharing.md` covers deployment. `features/note-sharing.md` covers the sharing workflow. Neither doc fully describes the interactive features of shared notes (todo checkbox toggling, calendar event creation on shared views).

**Recommendation:** ADD SECTION to `features/note-sharing.md` describing interactive shared note features (todo toggling, calendar event creation).

**Suggested sidebar location:** No change needed (existing page).

---

## 16. PluginKV Entity Model

**What exists in code:** `models/plugin_kv_model.go` -- `PluginKV` model with PluginName, Key, Value fields. Composite unique index on (PluginName, Key).

**Current doc coverage:** None. Not mentioned in any concept or entity doc.

**Recommendation:** Coverage should come via the `mah.kv` documentation in `features/plugin-lua-api.md` (gap #3 above). No separate entity page needed.

---

## 17. Keyboard Shortcuts Summary

**What exists in code:** Multiple keyboard shortcuts across components:
- `Cmd/Ctrl+K`: Global search
- `Cmd/Ctrl+Shift+D`: Download Cockpit toggle
- `1-9`: Quick tag slots in lightbox
- `Shift+Click`: Range select in bulk operations
- `Space`: Toggle selection in bulk operations
- `Shift+Submit`: Bypass delete confirmation
- `Arrow keys`: Lightbox navigation, search result navigation, autocomplete navigation
- `Escape`: Close modals, cancel inline edit
- `Enter`: Select in autocomplete, save inline edit
- `Double-click`: Lightbox zoom to native resolution
- `Ctrl+Scroll`: Lightbox zoom toward cursor

**Current doc coverage:** Scattered. `Cmd/Ctrl+K` is in several places. Lightbox shortcuts are in `user-guide/navigation.md`. But there is no consolidated keyboard shortcuts reference.

**Recommendation:** ADD SECTION to `user-guide/navigation.md` -- a complete keyboard shortcuts table. Or consider a dedicated page if the list grows.

**Suggested sidebar location:** User Guide section, within `navigation.md`.

---

## 18. FreeFields Component (Dynamic Metadata Fields)

**What exists in code:** `src/components/freeFields.js` -- renders dynamic key-value metadata fields with remote field suggestions, type coercion, and JSON output. Exports utility functions for meta query filter generation.

**Current doc coverage:** None explicitly. `features/meta-schemas.md` covers JSON Schema-based metadata but not the dynamic free-form metadata field UI.

**Recommendation:** ADD SECTION to a user guide page covering metadata editing -- explain the key-value field UI, remote field suggestions, and how values are coerced to JSON types.

**Suggested sidebar location:** User Guide or Features section.

---

## 19. Entity Picker Component Details

**What exists in code:** `src/components/picker/entityPicker.js` -- generic modal picker with search, tabs, filters, multi-select. Used by gallery, references, and calendar blocks.

**Current doc coverage:** Partial. `features/entity-picker.md` exists and documents the feature.

**Recommendation:** No action needed -- adequately covered.

---

## 20. CardActionMenu Component (Plugin Actions on Entity Cards)

**What exists in code:** `src/components/cardActionMenu.js` -- dropdown on entity cards for triggering plugin actions. Dispatches events to pluginActionModal.

**Current doc coverage:** Partial. `features/plugin-actions.md` documents the plugin action system and mentions the UI trigger points.

**Recommendation:** No action needed -- adequately covered.

---

## Summary of Actionable Gaps

### Must Fix (factual errors or missing critical content)

| # | Gap | Action | Target File |
|---|-----|--------|------------|
| 1 | Plugin CRUD operations for all entity types | ADD SECTION | `features/plugin-lua-api.md` |
| 2 | Plugin relationship management functions | ADD SECTION | `features/plugin-lua-api.md` |
| 3 | Plugin KV store (mah.kv) | ADD SECTION | `features/plugin-lua-api.md` |
| 4 | Plugin logging (mah.log) | ADD SECTION | `features/plugin-lua-api.md` |
| 5 | Plugin mah.start_job | ADD to existing section | `features/plugin-lua-api.md` |
| 6 | Plugin purge-data endpoint | ADD to API table | `features/plugin-system.md`, `api/plugins.md` |

### Should Fix (undocumented user-facing features)

| # | Gap | Action | Target File |
|---|-----|--------|------------|
| 7 | Paste upload feature | ADD SECTION | `user-guide/managing-resources.md` |
| 8 | Quick Tag Panel in lightbox | ADD SECTION | `user-guide/navigation.md` |
| 9 | OpenAPI validator command | ADD to existing section | `api/overview.md` |
| 10 | Code editor (SQL autocompletion) | ADD SECTION | `features/saved-queries.md` |
| 15 | Interactive shared note features | ADD SECTION | `features/note-sharing.md` |
| 17 | Consolidated keyboard shortcuts | ADD SECTION | `user-guide/navigation.md` |

### Nice to Have

| # | Gap | Action | Target File |
|---|-----|--------|------------|
| 11 | Multi-sort UI component | ADD SECTION | `user-guide/navigation.md` |
| 12 | Shift-to-bypass confirmation | Mention in context | `user-guide/navigation.md` |
| 18 | Free-form metadata field UI | ADD SECTION | User guide or features |
