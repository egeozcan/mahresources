# Group Export / Import

**Status:** Design
**Date:** 2026-04-11
**Schema version:** 1

## 1. Goal

Provide a first-class way to move Groups (and their surrounding world — subgroups, owned resources, owned notes, tags, categories, note types, typed group relations, optional file bytes) between mahresources instances, or back onto the same instance as a restore. The output is a single self-describing tar file. Import is a review-and-apply flow with name-based mapping for referenced entities.

Both same-instance backup/restore and cross-instance migration are first-class use cases. Granularity is configurable on both the export and the import sides.

## 2. Surfaces

1. **`/admin/export`** — configure and launch export jobs. Pre-fillable from the groups list page via a query string seed.
2. **`/admin/import`** — upload tar, review/map, apply.
3. **Groups list page** — new "Export selected" bulk action that redirects to `/admin/export` with group IDs pre-selected. Reuses the existing `bulkSelection` Alpine component.
4. **`mr` CLI** — `mr groups export` and `mr groups import` commands driving the same HTTP APIs.

All long-running work rides the existing `download_queue` job infrastructure (jobs, SSE progress via `downloadCockpit.js`, retention cleanup). Three new job types: `group-export`, `group-import-parse`, `group-import-apply`. Jobs always run in the background; the UI offers a fast-path where completed exports auto-trigger a browser download without an extra click.

## 3. Out of scope

- Queries (`Query` entity) — not exported.
- Note `ShareToken` — not exported.
- Resource de-duplication beyond the single "skip on hash match" policy during import.
- Merging groups that share a name with an existing group — always create new.
- Any new authentication or permission layer.
- Preserving database IDs across import — all IDs are reassigned.
- Rollback of a partially applied import.

## 4. Data scope and toggles

The export page and CLI expose three toggle groups. The same toggles exist in the CLI as explicit flags. The import review screen can prune individual items from a tar even if the tar contains more.

### 4.1 Scope

- **S1 Subtree** — include all descendant subgroups recursively.
- **S2 Owned Resources** — include all Resources whose `OwnerId` is in scope.
- **S3 Owned Notes** — include all Notes whose `OwnerId` is in scope.
- **S4 Related entities** — include many-to-many `RelatedGroups` / `RelatedResources` / `RelatedNotes`. Entities reached only through a relation live outside the owner subtree; see dangling references (§7).
- **S5 GroupRelations** — include typed custom `GroupRelation` rows between groups in scope, plus (per §7) stubs for out-of-scope endpoints.

### 4.2 Fidelity

- **F1 Resource file bytes** — if on, raw file bytes are packed into the tar under `blobs/`. If off, only metadata and hashes are exported (manifest-only mode).
- **F2 Resource version history** — if on, all historical `ResourceVersion` rows and their blobs are included. If off, only the current version.
- **F3 Resource previews** — if on, generated previews/thumbnails are packed under `previews/`.
- **F4 Resource Series** — if on, Series membership is preserved; cross-subtree series siblings become dangling references.

### 4.3 Schema defs

- **D1 Categories / NoteTypes / ResourceCategories** — definitions with `MetaSchema`, all Custom HTML (`CustomHeader`, `CustomSidebar`, `CustomSummary`, `CustomAvatar`, `CustomMRQLResult`), and `SectionConfig`.
- **D2 Tags** — definitions with `Meta`.
- **D3 GroupRelationTypes** — definitions with `FromCategory`, `ToCategory`, `BackRelation`.

### 4.4 Always preserved

- `Meta` and `OwnMeta` JSON columns on every entity are always exported as raw JSON, regardless of toggles.
- Timestamps (`CreatedAt`, `UpdatedAt`) are preserved on export and reassigned by GORM on import.
- `NoteBlock` rows are always packed inline inside the note JSON (ordered by `Position`, with `Type`, `Content`, `State`).

## 5. Architecture

New code, grouped by layer.

### 5.1 `archive/` — serialization core

New top-level package. Framework-free (no GORM, no HTTP) so it can be unit-tested in isolation and reused by the CLI for local dry-runs.

- `archive/manifest.go` — manifest structs, schema version constant, marshal/unmarshal helpers.
- `archive/writer.go` — `Writer` type wrapping `archive/tar` + optional `compress/gzip`. Streaming. Methods: `WriteManifest`, `WriteSchemaDefs`, `WriteGroup`, `WriteNote`, `WriteResource`, `WriteBlob`, `WritePreview`, `Close`. Accepts plain DTOs, not GORM models.
- `archive/reader.go` — streaming reader. Parses manifest first, then yields entries on demand. Methods: `ReadManifest`, `IterateGroups`, `IterateNotes`, `IterateResources`, `ReadBlob`. Tolerates unknown manifest fields for forward compatibility.
- `archive/version.go` — `SchemaVersion = 1`. Readers reject unknown major versions with a clear error.

### 5.2 `application_context/export_context.go`

Orchestrator for export jobs. Takes an `ExportRequest` (root group IDs + toggles), walks the DB using existing helpers (`resource_context`, `note_context`, `group_tree_context.GetGroupTreeDown`), streams entries into an `archive.Writer`. Computes cross-subtree stubs and writes them into the manifest's `dangling_references` section. Reports progress through the job interface.

Key responsibilities:
- Build an export plan (set of group IDs in scope, resources, notes, schema def IDs referenced).
- Assign synthetic `export_id` values (e.g. `g0001`, `r0042`) stable within the archive.
- Rewrite all foreign keys in entity payloads from DB IDs to export IDs.
- De-duplicate blobs by `sha1` hash (one blob per unique hash, multiple resources reference it).
- Stream batches so even 50 GB exports fit in bounded memory.

### 5.3 `application_context/import_context.go`

Orchestrator for import jobs. Two entry points:

- `ParseImport(tarPath)` — reads the manifest, builds an `ImportPlan` (item tree, name-based mapping suggestions, conflict summary), persists the plan to `FILE_SAVE_PATH/_imports/<jobId>.plan.json`, returns the plan ID.
- `ApplyImport(planID, userDecisions)` — walks the plan with the user's decisions, creates or reuses entities inside a per-batch transaction.

### 5.4 `download_queue/` extensions

Generalize the existing manager to host non-download jobs. Either rename the internal `Job` interface to a neutral name or add a second parallel interface; keep backward compatibility for the existing remote-download jobs. Add new job type constants (`group-export`, `group-import-parse`, `group-import-apply`). The SSE stream gains a `type` discriminator so clients can filter.

No new schema. No new frontend event infrastructure — `downloadCockpit.js` becomes a generic job cockpit used by the existing download queue, export pages, and import pages.

### 5.5 HTTP layer (`server/api_handlers/export_api_handlers.go`)

- `POST /v1/groups/export/estimate` — returns counts (groups, notes, resources, unique blobs, bytes, dangling references). Query-only pass, no job.
- `POST /v1/groups/export` — enqueues a `group-export` job, returns `{job_id}`.
- `GET /v1/exports/{jobId}/download` — streams the finished tar file, sets `Content-Type: application/x-tar` and `Content-Disposition: attachment`.
- `POST /v1/groups/import/parse` — multipart upload, stages the tar under `FILE_SAVE_PATH/_imports/<jobId>.tar`, enqueues a `group-import-parse` job, returns `{job_id}`.
- `GET /v1/imports/{jobId}/plan` — returns the parsed `ImportPlan` JSON.
- `POST /v1/imports/{jobId}/apply` — accepts `ImportDecisions`, enqueues a `group-import-apply` job, returns `{job_id}`.
- `DELETE /v1/imports/{jobId}` — cancels and cleans up the staged tar and plan JSON.

All routes are registered in `server/routes_openapi.go` under the appropriate section (`registerGroupRoutes`, `registerAdminRoutes`) and follow the existing `openapi.RouteInfo` convention.

### 5.6 Template and frontend layer

- `server/template_handlers/admin_export_template_context.go` and `admin_import_template_context.go` — page handlers mirroring `admin_overview_template_context.go`. Mostly shells.
- `templates/adminExport.tpl` and `templates/adminImport.tpl` — Pongo2 templates using the same layout shell as `adminOverview.tpl`.
- `src/components/adminExport.js` and `src/components/adminImport.js` — Alpine.js components.

### 5.7 CLI (`cmd/mr/commands/groups.go`)

Add `newGroupExportCmd(c, opts)` and `newGroupImportCmd(c, opts)` to the existing group command subtree. Both drive the HTTP APIs and poll job status until completion.

### 5.8 Storage layout on disk

- `FILE_SAVE_PATH/_exports/<jobId>.tar` — completed export tar files. Retained until job retention expiry, then deleted.
- `FILE_SAVE_PATH/_imports/<jobId>.tar` — staged upload tar files.
- `FILE_SAVE_PATH/_imports/<jobId>.plan.json` — parsed import plan.

Leading underscore names keep `resources/` listings from scanning them. In `-memory-fs` mode, these live in memory and vanish on restart — acceptable for that deployment style.

### 5.9 Database migrations

None. This feature reads and writes existing tables; no new schema.

## 6. Tar layout and manifest schema

### 6.1 Tar layout

Uncompressed tar by default. Gzip is an opt-in option on export. Most binary blobs inside (images, video, already-compressed files) don't gain meaningfully from gzip.

```
manifest.json                       # always first in tar
schemas/
  categories.json                   # if D1 enabled
  note_types.json                   # if D1 enabled
  resource_categories.json          # if D1 enabled
  tags.json                         # if D2 enabled
  group_relation_types.json         # if D3 enabled
groups/
  <export_id>.json                  # one file per group
notes/
  <export_id>.json                  # inline NoteBlocks
resources/
  <export_id>.json                  # metadata, refs blobs/previews
blobs/
  <sha1>                            # raw file bytes, content-addressed
  versions/
    <sha1>                          # historical version bytes (F2)
previews/
  <resource_export_id>/<name>       # preview bytes (F3)
```

Properties:

- `manifest.json` is always the first tar entry. Readers stream-parse it without reading the rest.
- Blobs are content-addressed by SHA1 hash. Duplicate file bytes produce one blob; resources reference it via `blob_ref`.
- One file per entity (not a single giant array). Streaming-friendly and GORM batch writes can insert them without loading the whole tar.
- Export IDs (`g0001`, `r0042`) are synthetic identifiers stable within the archive. They are not real DB IDs and do not appear outside the archive format.

### 6.2 Manifest schema (`manifest.json`, version 1)

```json
{
  "schema_version": 1,
  "created_at": "2026-04-11T14:02:00Z",
  "created_by": "mahresources",
  "source_instance_id": "optional-identifier",
  "export_options": {
    "scope": {
      "subtree": true,
      "owned_resources": true,
      "owned_notes": true,
      "related_m2m": true,
      "group_relations": true
    },
    "fidelity": {
      "resource_blobs": true,
      "resource_versions": false,
      "resource_previews": true,
      "resource_series": true
    },
    "schema_defs": {
      "categories_and_types": true,
      "tags": true,
      "group_relation_types": true
    }
  },
  "roots": ["g0001", "g0002"],
  "counts": {
    "groups": 42,
    "notes": 180,
    "resources": 900,
    "blobs": 840,
    "previews": 1680,
    "versions": 0
  },
  "entries": {
    "groups":    [{"export_id": "g0001", "name": "Books", "source_id": 17, "path": "groups/g0001.json"}],
    "notes":     [{"export_id": "n0001", "name": "Review", "source_id": 54, "owner": "g0001", "path": "notes/n0001.json"}],
    "resources": [{"export_id": "r0001", "name": "cover.jpg", "source_id": 9001, "owner": "g0001", "hash": "abcd...", "path": "resources/r0001.json"}]
  },
  "schema_defs": {
    "categories":           [{"export_id": "c0001", "name": "Books", "source_id": 3, "path": "schemas/categories.json"}],
    "note_types":           [],
    "resource_categories":  [],
    "tags":                 [],
    "group_relation_types": []
  },
  "dangling_references": [
    {
      "id": "dr0001",
      "kind": "related_group",
      "from": "g0001",
      "to_stub": {"source_id": 88, "name": "Archive", "reason": "out_of_scope"}
    },
    {
      "id": "dr0002",
      "kind": "group_relation",
      "from": "g0003",
      "relation_type_name": "DerivedFrom",
      "to_stub": {"source_id": 102, "name": "Reference", "reason": "out_of_scope"}
    },
    {
      "id": "dr0003",
      "kind": "resource_series_sibling",
      "from": "r0017",
      "to_stub": {"source_id": 15234, "name": "Volume 4", "reason": "out_of_scope"}
    }
  ],
  "warnings": []
}
```

### 6.3 Per-entity payloads

Each `<export_id>.json` file contains the full entity state with foreign keys rewritten as export IDs.

**Group (`groups/g0001.json`)**:
```json
{
  "export_id": "g0001",
  "source_id": 17,
  "name": "Books",
  "description": "...",
  "url": "",
  "owner_ref": "g0004",
  "category_ref": "c0001",
  "tags": ["t0001", "t0002"],
  "related_groups": ["g0002"],
  "related_resources": ["r0017"],
  "related_notes": ["n0004"],
  "relationships": [
    {"type_ref": "grt0001", "to_ref": "g0002", "name": "", "description": ""},
    {"type_ref": "grt0001", "dangling_ref": "dr0001"}
  ],
  "meta": {},
  "created_at": "...",
  "updated_at": "..."
}
```

**Note (`notes/n0001.json`)**: Note fields, `owner_ref`, `note_type_ref`, `tags`, `resources`, `groups` (m2m), inline `blocks` array with `{type, position, content, state}`.

**Resource (`resources/r0001.json`)**: Resource fields, `owner_ref`, `resource_category_ref`, `tags`, `groups` (m2m), `notes` (m2m), `blob_ref: "<sha1>"` (or `null`), `versions: [{version_number, blob_ref, comment, created_at}, ...]`, `previews: ["<name1>", "<name2>"]`, `series_ref: "s0001"` (or `null`), `meta`, `own_meta`.

**Schema def files** (`schemas/categories.json` etc.) are arrays of full definitions with `export_id`, `source_id`, name, description, all Custom HTML templates, MetaSchema, SectionConfig.

### 6.4 Compatibility rules

- Readers reject unknown major `schema_version` values with a clear error listing supported versions.
- Unknown top-level keys in the manifest are silently ignored (forward compatibility).
- `source_instance_id` is informational — shown in the import review UI so users can see where the tar came from.
- The manifest contract is stable. Breaking changes bump the schema version.

## 7. Cross-subtree references (dangling refs)

Exporting the subtree rooted at Group X, some references may point outside that subtree:

- Group X has a `RelatedGroup` Y (m2m) where Y is not a descendant of X.
- A resource owned by X is in `RelatedResources` of a group outside scope.
- Group X has a typed `GroupRelation` to Y, Y out of scope.
- A resource owned by X has a `Series` where some siblings are owned by a different group.

**Policy:** Record a stub entry in `manifest.dangling_references` for each out-of-scope reference. The stub carries the source ID and name of the missing entity plus a `reason`. At import time, each stub becomes a mapping target: the user can "Map to existing destination entity" (autocomplete over the right type) or "Drop relation". The export UI shows a pre-export warning summarizing how many dangling references will be stubbed, grouped by kind.

For bulk multi-root exports, references crossing *between* selected roots are in scope (both endpoints are in the archive). Dangling detection applies only to references leaving the union of selected subtrees.

## 8. Export flow

### 8.1 UI

User enters `/admin/export` either directly or via the "Export selected" bulk action on the groups list page. The bulk action is a simple client-side redirect with `?groups=17,42,88` — no POST from the list page.

The `adminExport()` Alpine component renders:

1. **Group picker.** Pre-seeded from the query string. Chips with remove buttons. An autocomplete adds more groups, backed by the existing group search endpoint.
2. **Toggle panel.** Three toggle groups (§4) as labeled checkboxes. Defaults: everything on except `resource_versions` and `resource_previews`.
3. **Estimate button.** Hits `POST /v1/groups/export/estimate` and displays counts plus predicted tar size and the number of dangling references grouped by kind.
4. **Submit button.** POSTs to `/v1/groups/export`. Server returns a job ID.
5. **Progress panel.** Subscribes to the SSE stream filtered by job ID. Shows current phase, counts processed, bytes written. Cancel button.
6. **Fast-path completion.** On `completed`, the component programmatically fires a `<a href="/v1/exports/{jobId}/download" download>` click to start the browser download automatically. If the user navigated away before completion, the job stays in the job list and the tar can be downloaded later.

### 8.2 Export job pseudocode

```
// phase 1 — build plan
plan := exportContext.BuildPlan(request)  // walks group tree, collects IDs, computes stubs
job.reportProgress("plan built", plan.counts)

// phase 2 — create tar file in staging
tarPath := fileSavePath + "/_exports/" + job.ID + ".tar"
w := archive.NewWriter(tarPath, request.Gzip)
defer w.Close()

// phase 3 — write manifest (counts finalized)
w.WriteManifest(plan.toManifest())

// phase 4 — schema defs
if request.SchemaDefs.CategoriesAndTypes {
    w.WriteSchemaDefs(plan.categories, plan.noteTypes, plan.resourceCategories)
}
if request.SchemaDefs.Tags             { w.WriteTags(plan.tags) }
if request.SchemaDefs.GroupRelationTypes { w.WriteGroupRelationTypes(plan.grts) }

// phase 5 — groups, notes, resources (streamed via FindInBatches)
for group := range plan.GroupsBatched() {
    w.WriteGroup(group.toExportJSON(plan.idMap))
    job.reportProgress("groups", groupCount++)
}
for note := range plan.NotesBatched() {
    w.WriteNote(note.toExportJSON(plan.idMap))
    job.reportProgress("notes", noteCount++)
}
for resource := range plan.ResourcesBatched() {
    w.WriteResource(resource.toExportJSON(plan.idMap))
    if request.Fidelity.ResourceBlobs && !plan.blobWritten[resource.Hash] {
        fileReader := fileSystem.Open(resource.Location)
        w.WriteBlob(resource.Hash, fileReader)
        plan.blobWritten[resource.Hash] = true
    }
    if request.Fidelity.ResourceVersions {
        for version := range resource.Versions {
            if !plan.versionBlobWritten[version.Hash] {
                w.WriteVersionBlob(version.Hash, fileSystem.Open(version.Location))
                plan.versionBlobWritten[version.Hash] = true
            }
        }
    }
    if request.Fidelity.ResourcePreviews {
        for preview := range resource.Previews {
            w.WritePreview(resource.ExportID, preview.Name, fileSystem.Open(preview.Path))
        }
    }
    job.reportProgress("resources", resourceCount++)
}

// phase 6 — finalize
w.Close()
job.setResult({tarPath, warnings: plan.warnings})
job.setStatus(completed)
```

### 8.3 Properties

- **Streaming end-to-end.** Entities are fetched in batches via GORM's `FindInBatches`. Blobs stream from filesystem to tar without fully loading. Memory stays bounded even for multi-terabyte exports.
- **Blob de-duplication by hash.** Each unique SHA1 produces one blob. Resources reference blobs by hash.
- **Alt-fs support.** `fileSystem.Open` uses the existing Afero abstraction, so resources in alternate file systems are read correctly.
- **Missing blob handling.** If a referenced file is gone from the filesystem, the entry's `blob_ref` is set to `null`, a `blob_missing: true` flag is recorded, and the job's `warnings` array captures the list. The export does not fail.
- **Cancellation.** The job honors context cancellation. Partial tar files are deleted on cleanup.
- **Retention.** Completed jobs expire after the standard download_queue retention window. Expired tar files are deleted.

### 8.4 Download endpoint

`GET /v1/exports/{jobId}/download` looks up the job, confirms it's in `completed` status, opens the tar file, sets `Content-Type: application/x-tar` and `Content-Disposition: attachment; filename="mahresources-export-<timestamp>.tar"`, streams the file body. Does not delete the file on download — it lives until retention expiry so re-downloads are possible.

## 9. Import flow

Four phases: **upload → parse → review → apply**. Parse and apply are background jobs.

### 9.1 Phase 1 — Upload

User navigates to `/admin/import` and uploads a tar via the file input. Multipart POST to `/v1/groups/import/parse`. The server streams the upload to `FILE_SAVE_PATH/_imports/<jobId>.tar`, enqueues a `group-import-parse` job, returns `{job_id}`. The UI immediately shows a progress panel.

Upload size is capped at a configurable maximum (`-max-import-size`, default 10 GB). Larger requires a config override.

### 9.2 Phase 2 — Parse (job)

```
r := archive.NewReader(stagingPath)
manifest := r.ReadManifest()         // rejects unknown schema_version
plan := ImportPlan{
    job_id: ...,
    manifest: manifest,
    source_instance_id: manifest.SourceInstanceID,
    items: buildItemTree(manifest),        // hierarchical: roots → subgroups → owned counts
    mappings: buildMappings(manifest, db), // name-based suggestions for schema defs
    conflicts: detectConflicts(manifest, db),
}
persistPlan(plan)                          // → _imports/<jobId>.plan.json
job.setStatus(completed)
```

**Plan components:**
- **`items`** — hierarchical tree mirroring the manifest's group forest, with descendant counts on each node. The UI uses this to render the review tree with checkboxes.
- **`mappings`** — one entry per Category, NoteType, ResourceCategory, Tag, GroupRelationType present in the manifest. Each entry has `{source_name, source_export_id, suggestion, destination_id, alternatives}`. Matching is exact case-sensitive on `name`. Exact match → `suggestion: "map"` with `destination_id` set. No exact match → `suggestion: "create"`. Case-insensitive near-matches appear in `alternatives` flagged as "close_match".
- **`conflicts`** — summary counts: resources in the tar whose hashes exist in the destination, groups whose names exist under the chosen parent.

The parse job writes the plan JSON to disk and does not hold DB state open. The plan is persistent and reloadable by job ID.

### 9.3 Phase 3 — Review (UI)

`adminImport()` Alpine component fetches `GET /v1/imports/{jobId}/plan` and renders:

1. **Header.** Source instance, created_at, schema version, counts, warnings.
2. **Global options.**
   - **Parent group picker** — autocomplete; default empty, imported roots land as top-level groups.
   - **Resource collision policy** — single dropdown: `Skip (use existing)` (default) or `Create duplicate row`. Applied to the entire import.
3. **Mapping panel** (collapsible sections per entity type, one table each for Categories / NoteTypes / ResourceCategories / Tags / GroupRelationTypes).
   - Columns: `[✓] include`, `Source name`, `Action`, `Destination`.
   - Action dropdown: "Map to existing" or "Create new".
   - Exact-name matches pre-filled as "Map to existing" with the destination pointed to the matched row. User can flip to "Create new" with one click.
   - Near-matches pre-filled as "Create new" but with the close match at the top of the destination autocomplete.
4. **Dangling references panel.** One row per stub in `manifest.dangling_references`: `[kind] from <export_id> → stub "<stub name>"`. Dropdown to "Map to destination entity" (autocomplete over existing entities of the right type) or "Drop relation".
5. **Item tree.** Hierarchical tree with checkboxes. Unchecking a group unchecks its descendants. Counts of owned resources and notes roll up.
6. **Apply button.** Collects all decisions into an `ImportDecisions` payload, POSTs to `/v1/imports/{jobId}/apply`, enqueues the apply job. The UI subscribes to SSE and shows progress.

### 9.4 Phase 4 — Apply (job)

```
decisions := loadDecisions(request)
plan := loadPlan(jobId)
r := archive.NewReader(stagingPath)
idMap := {}  // export_id → destination_id

// step 1 — resolve schema defs first
for def in plan.mappings.categories {
    if decisions.categoryActions[def.export_id] == "map" {
        idMap[def.export_id] = decisions.categoryActions[def.export_id].destID
    } else {
        idMap[def.export_id] = createCategory(r.ReadCategory(def))
    }
}
// ... same for note_types, resource_categories, tags, group_relation_types

// step 2 — groups in topological order (roots first)
for group in plan.items.walkSelected() {
    if group.skippedByUser { continue }
    g := r.ReadGroup(group.export_id)
    destID := createGroup(g, idMap, decisions)
    idMap[group.export_id] = destID
    job.reportProgress("groups", groupCount++)
}

// step 3 — resources, batched
for batch in plan.items.SelectedResourceBatches(size=500) {
    tx := db.Begin()
    for r in batch {
        res := archive.ReadResource(r.export_id)
        if existing := findByHash(res.hash); existing != nil && decisions.resourceCollision == "skip" {
            idMap[r.export_id] = existing.ID
            continue
        }
        if res.blob_ref {
            blobBytes := archive.ReadBlob(res.blob_ref)
            storeBlob(blobBytes, destinationLocation(res))
        }
        dest := createResource(res, idMap)
        idMap[r.export_id] = dest.ID
    }
    tx.Commit()
    job.reportProgress("resources", resourceCount += len(batch))
}

// step 4 — notes (with inline blocks), batched
...

// step 5 — apply m2m relationships
for link in plan.m2mLinks {
    if link.target not in idMap { continue }  // pruned — drop link
    applyLink(link)
}

// step 6 — apply dangling reference decisions
for dangling in plan.danglingRefs {
    decision := decisions.danglingActions[dangling.id]
    if decision == "drop" { continue }
    applyDanglingMapping(dangling, decision.destID)
}

job.setResult({created_groups, created_resources, skipped_by_hash, ...})
job.setStatus(completed)
```

### 9.5 Properties

- **Batched transactions.** Each batch of 500 resources (or 500 notes) is a single transaction. Not one giant transaction over the whole import — that's pathological on SQLite and flaky on Postgres under load.
- **Topological order.** Groups are walked roots-first so `owner_ref` always resolves to an already-created destination ID. The manifest guarantees a forest structure (descendants point up; no cycles).
- **Blob restoration.** Blobs land in the default save path via the existing Afero file system. Source `location` paths are not preserved — they're meaningless across instances.
- **No idempotency.** Re-running an apply with the same decisions creates duplicates. Each apply is single-shot: on completion or failure, the user cannot re-apply without re-uploading.
- **Partial-apply on failure.** If a batch fails mid-apply, already-committed batches are left in place. The job transitions to `failed`. The job result lists created IDs per phase so the user can manually clean up if they want. Full rollback is out of scope — rolling back a partially-applied 10 GB import is not a good experience.
- **Cleanup.** After retention expiry, the staged tar and plan JSON are deleted. If the user cancels an import before apply, stage and plan are deleted immediately.

## 10. Error handling

### 10.1 Export-side

- **Missing blob file.** Resource metadata refers to a file absent from the filesystem. Log a warning, set `blob_ref: null` and `blob_missing: true` on the entry, append to `manifest.warnings`, continue. The user sees the warnings on the job result screen.
- **Circular parent references.** Shouldn't exist. Defensive check during tree walk; if found, log, break the cycle, continue.
- **Zero-scope export.** No groups selected, or all toggles off. The estimate endpoint catches this and the UI disables the export button with a tooltip.
- **Huge meta JSON.** No size cap. Exported as-is.
- **Concurrent modifications during export.** A resource could be deleted mid-walk. Each batch reads in its own read transaction; a row deleted between batches simply disappears from the walk. Minor divergence from live DB at the end is acceptable and documented.

### 10.2 Import-side

- **Unknown schema version.** Parse job fails immediately with a clear error listing supported versions.
- **Corrupt tar or missing manifest.** Parse job fails. Staged file is deleted.
- **Reference to an export_id not in the tar.** Treated as a dangling reference; surfaced on the review screen. Defends against hand-edited tars.
- **Apply-time conflicts the plan didn't predict.** E.g. a destination category deleted between parse and apply. The apply job fails with a clear error, no silent fallback. The user re-parses and re-decides.
- **Resource blob restore fails.** Disk full, permissions, etc. The batch transaction rolls back, the job fails with the failing batch identified.
- **Disk pressure during upload.** Upload size is capped at `-max-import-size` (default 10 GB).
- **Missing alt-fs.** Resources written during import always go to the default save path. Alt-fs configurations don't need to match between instances.

## 11. CLI

### 11.1 Export

```
mr groups export <id> [<id>...] [flags]
  --include-subtree / --no-subtree                 default on
  --include-resources / --no-resources             default on
  --include-notes / --no-notes                     default on
  --include-related / --no-related                 default on
  --include-group-relations / --no-group-relations default on
  --include-blobs / --no-blobs                     default on
  --include-versions / --no-versions               default off
  --include-previews / --no-previews               default off
  --include-series / --no-series                   default on
  --schema-defs none|all|selected                  default all
  --gzip                                            default off
  -o, --output <file>                               default: stdout
  --wait / --no-wait                                default wait
```

CLI posts to the export endpoint, polls the job, downloads the tar to the output file or stdout. Non-interactive; scripts work.

### 11.2 Import

```
mr groups import <tarfile> [flags]
  --parent-group <id>                               optional
  --on-resource-conflict skip|duplicate             default skip
  --auto-map                                         default on in non-interactive mode
  --dry-run                                           parse, print plan, don't apply
  --plan-output <file>                                write plan JSON
  --decisions <file>                                  read a decisions JSON
  --wait / --no-wait                                  default wait
```

Two common workflows:
- **Interactive.** `mr groups import foo.tar --auto-map` applies with exact-name auto-mapping and default conflict policy.
- **Scripted.** `mr groups import foo.tar --dry-run --plan-output plan.json`, user generates a decisions file externally, then `mr groups import foo.tar --decisions decisions.json`.

### 11.3 Client library

`cmd/mr/client/client.go` already supports multipart uploads. No new client primitives needed beyond wrapping the new endpoints.

## 12. Testing strategy

### 12.1 Unit tests — `archive/` package

Pure Go tests, no DB. Round-trip properties:

- Any valid manifest/entity payload written by `Writer` is parsed back identically by `Reader`.
- Empty manifest.
- Manifest with schema version higher than supported (reject with clear error).
- Manifest with unknown top-level keys (accept, ignore).
- Blob de-duplication: two resources sharing a hash produce one blob in the tar.
- Content-addressed blob retrieval.

### 12.2 Integration tests — `application_context/` (Go)

Using the existing ephemeral-DB + memory-fs harness:

- `TestExportImport_RoundTrip_FullFidelity` — create a group tree with subgroups, resources, notes, tags, categories, relations; export with all toggles on; import into a fresh DB; assert deep equality (adjusting for reassigned IDs).
- `TestExport_ToggleCombinations` — table-driven, each toggle combination produces the expected manifest shape.
- `TestImport_NameBasedMapping` — seed destination DB with matching names; parse a tar; assert mapping suggestions point at the right destination IDs.
- `TestImport_ResourceCollisionSkip` — destination already has a resource with a matching hash; import reuses existing, doesn't write a duplicate blob.
- `TestImport_DanglingReferenceStubs` — export a subtree whose groups have out-of-scope references; import elsewhere; assert the dangling panel shows the right stubs and user decisions are honored.
- `TestImport_PartialApplyFailureSurfacesProgress` — inject a failure mid-apply; assert the job result reports what was committed.
- `TestExport_BlobMissing` — delete a file on disk before packing; export completes with `blob_missing: true` and a warning in the manifest.
- `TestExportImport_RoundTrip_ManifestOnly` — export with `resource_blobs: false`; import reuses existing resources by hash; no blobs in the tar.
- `TestExportImport_Series` — export a subtree containing a Series with in-scope and out-of-scope members; assert in-scope members preserve Series membership and out-of-scope members become dangling references.

All integration tests run under both SQLite and Postgres (gated by the existing `postgres` build tag).

### 12.3 E2E browser tests — `e2e/tests/admin-export-import/`

Playwright:
- `export.spec.ts` — seed groups, navigate to `/admin/export`, toggle options, run export, wait for job, assert the browser triggers a download.
- `import.spec.ts` — upload a known fixture tar, review plan, flip one mapping, apply, assert the resulting DB state.
- `bulk-selection-redirect.spec.ts` — select groups on the list page, click "Export selected", assert the redirect pre-fills the group picker.
- `import-mapping-close-match.spec.ts` — case-insensitive name match shown as a close-match alternative.
- `accessibility.spec.ts` — axe-core runs on both new admin pages and the review screen.

### 12.4 CLI E2E tests — `e2e/tests/cli/`

Against an ephemeral server via the existing CLI harness:
- `groups-export.spec.ts` — round-trip: export from one server, import into a second, assert counts and key fields.
- `groups-import-dry-run.spec.ts` — dry-run prints the plan without writing anything; destination DB unchanged.
- `groups-import-decisions-file.spec.ts` — scripted import using `--dry-run --plan-output` then `--decisions`.
- `groups-export-flags.spec.ts` — toggle combinations as flags produce the expected manifest shapes.

Fixtures are generated at test-setup time from a Go seed script.

## 13. Documentation

### 13.1 New docs-site pages

- **`features/export-import.md`** — feature overview, walkthrough with screenshots, worked example (export a group tree from instance A, import into instance B).
- **`reference/manifest-schema.md`** — full manifest schema v1, field-by-field, examples of each entry payload, compatibility rules, dangling reference kinds.
- **`reference/cli-groups-export.md`** and **`reference/cli-groups-import.md`** — CLI command references with every flag documented, including the scripted-decisions workflow.
- **`guides/backup-and-restore.md`** — using export/import as a backup-and-restore tool on a single instance: recommended options, frequency, retention.
- **`guides/moving-data-between-instances.md`** — using export/import to move data between instances: mapping strategy, handling of alt-fs, resource collisions.

Screenshots captured via the existing `retake-screenshots` workflow after the feature is built and seeded.

### 13.2 Updates to existing docs

- `README.md` — short mention of export/import in the feature list.
- `CLAUDE.md` — note the manifest contract (schema version 1, forward-compat rules, stable public format) so future work doesn't break it by accident.
- OpenAPI spec regenerated via `cmd/openapi-gen` to expose the new endpoints.

## 14. Implementation note — suggested sub-plan boundaries

This spec is intentionally single-document because the pieces are tightly coupled. A reasonable way to slice it into separable implementation plans:

1. **Plan A — Archive core and export.** `archive/` package, `export_context.go`, `group-export` job type, `POST /v1/groups/export`, `POST /v1/groups/export/estimate`, `GET /v1/exports/{jobId}/download`, admin export page, CLI `mr groups export`. Ends with: fully working export, no import.
2. **Plan B — Import parse and review.** `import_context.go` parse path, `group-import-parse` job type, `POST /v1/groups/import/parse`, `GET /v1/imports/{jobId}/plan`, admin import page (upload + review UI), CLI `mr groups import --dry-run`. Ends with: upload and inspect, no apply.
3. **Plan C — Import apply.** `import_context.go` apply path, `group-import-apply` job type, `POST /v1/imports/{jobId}/apply`, mapping enforcement, admin import page (apply + progress), CLI `mr groups import` full. Ends with: full round-trip.
4. **Plan D — Docs and edge-case hardening.** Docs-site pages, screenshot capture, README and CLAUDE.md updates, remaining edge-case tests, bulk-selection-redirect integration on the groups list page.

Plans A, B, C, D are sequential; later plans depend on earlier ones. The split is a hint, not a commitment — the writing-plans skill gets the final call on how to decompose.
