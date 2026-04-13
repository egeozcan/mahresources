# Related Entity Export/Import

**Date:** 2026-04-13
**Status:** Approved

## Problem

The group export/import system currently includes entities **owned by** in-scope groups. M2M relationships (RelatedGroups, RelatedResources, RelatedNotes) and GroupRelations are recorded in payloads, but targets outside the ownership scope become dangling references that must be manually mapped on import. This means a group's full context — the resources, notes, and groups it relates to — is lost unless those entities happen to share the same owner hierarchy.

## Solution

Add a `RelatedDepth` parameter to export options. When > 0, the export plan builder runs a BFS from in-scope groups, following enabled m2m edges to discover and include related entities up to N hops deep. This reduces dangling references and preserves a group's relational context in the archive.

## Design Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Depth model | Configurable integer (0 = off) | Covers one-level and multi-hop use cases |
| Which edges to follow | All m2m types enabled by existing scope flags (`RelatedM2M`, `GroupRelations`) | Reuses existing toggles, no new flags |
| Transitive traversal | Groups only spawn next hop; resources/notes are leaves | Keeps traversal group-centric and predictable |
| Pulled-in group fidelity | Shell only (metadata, tags, category — no owned entities) | Lightweight; user would add as root if they wanted full subtree |
| Pulled-in resource/note ownership | Owning group auto-included as shell group | Owner export ID always resolves; backward-compatible with old readers |
| Schema version | No bump (additive fields only) | Old readers ignore unknown fields; shell groups import as normal groups |

## Manifest & Archive Format

### New/Changed Fields

**`ExportOptions`:**
- `RelatedDepth int` — BFS depth limit. Default 0 (current behavior).

**`GroupEntry`:**
- `Shell bool` — True when the group was pulled in as a relationship target or pulled-in entity owner, not as an owned/root group. Shell groups carry metadata but no owned entities.

**`GroupPayload`:**
- `Shell bool` — Mirrors the entry flag. Shell payloads contain name, category, tags, meta, and their own m2m links, but no owned resources/notes are collected for them.

**`Counts`:**
- `ShellGroups int` — Number of shell groups in the archive, for import plan visibility.

No changes to `ResourceEntry`, `NoteEntry`, `ResourcePayload`, `NotePayload`, or `DanglingRef` structures. No new `DanglingKind` values.

### Backward Compatibility

Old readers (schema version 1) silently ignore `Shell`, `RelatedDepth`, and `ShellGroups` fields. Shell groups are valid group entries — an old importer creates them as normal groups. Pulled-in resources/notes have valid owner export IDs (pointing to shell groups), so old importers resolve them without error.

## Export Pipeline

### BFS Traversal

After `buildExportPlan` completes (owned groups, resources, notes, series), a new phase runs when `RelatedDepth > 0`:

```
frontier = in-scope groups (depth 0)
for level = 1..RelatedDepth:
    for each group G in frontier:
        if RelatedM2M enabled:
            follow G.RelatedGroups → new groups (shell)
            follow G.RelatedResources → new resources
            follow G.RelatedNotes → new notes
        if GroupRelations enabled:
            follow G.GroupRelation targets → new groups (shell)

    for each newly discovered resource/note:
        if owning group not already in scope:
            add owner as shell group

    frontier = newly discovered groups from this level
    if frontier is empty → stop early
```

### Rules

- **Deduplication:** Entities already in scope (from ownership or a previous BFS level) are skipped. No duplicates.
- **Leaves don't spawn hops:** Only newly discovered groups enter the frontier. Resources and notes are terminal — their own m2m links are recorded in payloads but not traversed.
- **Shell groups:** Get `Shell: true`, skip owned-entity collection during payload loading.
- **Full payload for pulled-in resources/notes:** Blob, versions, previews per fidelity flags. They're real entities, just discovered via relationship edges rather than ownership.
- **Tag collection:** Tags referenced by any pulled-in entity are added to the schema defs.
- **M2m links on pulled-in entities:** Recorded in their payloads. Out-of-scope targets become dangling refs (no further traversal from leaves).
- **Export IDs:** Same counter/prefix scheme. Shell groups get `g` prefixes like regular groups.
- **Dangling ref collection:** Runs after BFS completes, so it only records refs genuinely outside the expanded scope.

## Import Pipeline

### ParseImport

- Shell groups appear in the `Items` tree with `Shell: true`. They have no resource/note counts.
- Shell group decision options:
  - `create` (default) — Create a new minimal group with archived metadata.
  - `map_to_existing` — User picks an existing DB group. Resources/notes owned by this shell group get assigned to the mapped group.

### ApplyImport

- Shell groups are created/mapped in the same dependency phase as regular groups, so their DB IDs are in `idMap` before resources/notes need them.
- `map_to_existing`: `idMap` entry points to the existing DB group's ID. Any m2m links recorded in the shell group's payload are wired to the mapped-to group (the `idMap` resolution handles this naturally).
- `create`: New group row with archived name, category, tags, meta. No children to recurse.
- M2m wiring works unchanged — pulled-in entities resolve links through `idMap`.

No changes to `ImportDecisions` structure beyond supporting the `Shell` flag for visual indication in the import plan.

## CLI & API

### Export

**CLI:** New flag `--related-depth N` (default 0).
```
mr export --group 42 --related-depth 2
```

**API (export request body):** New field `"relatedDepth": 2`.

Existing `relatedM2M` and `groupRelations` scope flags still control which edge types are walked. `relatedDepth` controls how far.

### Import

**API (import plan response):** Shell groups appear with `"shell": true` on their entries. No new endpoints.

### Template UI

Numeric input for related depth, defaulting to 0. Shown when RelatedM2M or GroupRelations scope flag is toggled on.

## Testing

### Go Integration Tests

| Test | Verifies |
|------|----------|
| Depth 0 backward compat | Existing tests pass unchanged, no shell groups |
| Depth 1 basic | Group A relates to Resource R (owned by Group B). R in archive, B is shell, R's owner resolves to B |
| Depth 2 chaining | A→B (shell, depth 1), B→Resource S (owned by C). S included, C is shell |
| Early termination | Depth 3 requested, no new groups at depth 2. BFS stops |
| Deduplication | A and B both relate to same Resource R. R appears once |
| Shell group map_to_existing | Round-trip: export depth 1, import mapping shell to existing group, resource gets mapped group as owner |
| Shell group create | Round-trip: export depth 1, import creating shell group, verify minimal group with correct metadata |
| Dangling beyond depth | Depth 1 shell group relates to Group C (depth 2). C is dangling, not included |
| Scope flag interaction | `RelatedM2M: false` + depth > 0: no BFS. `GroupRelations: true` + `RelatedM2M: false`: only typed relation targets followed |

### E2E Tests

| Test | Verifies |
|------|----------|
| CLI round-trip | Export with `--related-depth 1`, re-import, related entities present |
| UI export form | Set related depth, verify plan shows shell groups distinctly |
