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

No changes to `ResourceEntry`, `NoteEntry`, `ResourcePayload`, `NotePayload`, or `DanglingRef` structures. No new `DanglingKind` values needed — see "M2m links on pulled-in entities" in the Export Pipeline section for why.

### Backward Compatibility

Old readers (schema version 1) silently ignore `Shell`, `RelatedDepth`, and `ShellGroups` fields. Shell groups are valid group entries — an old importer creates them as normal groups. Pulled-in resources/notes have valid owner export IDs (pointing to shell groups), so old importers resolve them without error.

## Export Pipeline

### BFS Traversal

After `buildExportPlan` completes phases A–C (owned groups, resources, notes) but **before** phase D (series collection) and phase E (dangling refs), a new BFS phase runs when `RelatedDepth > 0`:

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

After BFS completes, phase D (series collection) runs over **all** resource IDs — both owned and BFS-discovered — so that `SeriesRef` and series-sibling dangling detection work correctly for pulled-in resources. Phase E (dangling ref collection) then runs over the fully expanded scope.

### Rules

- **Deduplication:** Entities already in scope (from ownership or a previous BFS level) are skipped. No duplicates.
- **Leaves don't spawn hops:** Only newly discovered groups enter the frontier. Resources and notes are terminal — their own m2m links are recorded in payloads but not traversed.
- **Shell groups:** Get `Shell: true`, skip owned-entity collection during payload loading.
- **Full payload for pulled-in resources/notes:** Blob, versions, previews per fidelity flags. They're real entities, just discovered via relationship edges rather than ownership.
- **Tag collection:** Tags referenced by any pulled-in entity are added to the schema defs.
- **M2m links on pulled-in entities:** Recorded in their payloads, but only for targets that are within the expanded scope (i.e., have an export ID). Out-of-scope targets are silently omitted — this is consistent with how owned resources/notes already work today (`export_context.go:1401-1409`: only targets with export IDs are included). No new `DanglingKind` values are needed because resource/note-originated m2m links have never participated in the dangling ref system.
- **Export IDs:** Same counter/prefix scheme. Shell groups get `g` prefixes like regular groups.
- **Dangling ref collection:** Runs after BFS completes, so it only records refs genuinely outside the expanded scope.

## Import Pipeline

### ParseImport

- `ImportPlanItem` gains a `Shell bool` field. Shell groups appear in the `Items` tree with `Shell: true`.
- Shell groups **do** show `ResourceCount`/`NoteCount` when they own pulled-in resources/notes — the tree builder counts by `OwnerRef` as it does today (`import_context.go:594`), and shell groups are valid owners. "Shell" means the group was not discovered via ownership traversal and has no owned-entity scope of its own; it does not mean the archive contains zero entities owned by it.
- Shell group decision options:
  - `create` (default) — Create a new minimal group with archived metadata.
  - `map_to_existing` — User picks an existing DB group. Resources/notes owned by this shell group get assigned to the mapped group.

### ImportDecisions Changes

`ImportDecisions` gains a new field to hold per-shell-group actions:

```go
type ImportDecisions struct {
    // ... existing fields ...
    ShellGroupActions map[string]ShellGroupAction `json:"shell_group_actions"`
}

type ShellGroupAction struct {
    Action        string `json:"action"`         // "create" or "map_to_existing"
    DestinationID *uint  `json:"destination_id,omitempty"` // required when Action = "map_to_existing"
}
```

The map key is the shell group's export ID (e.g., `"g0005"`). Shell groups not present in the map default to `create`. `ValidateForApply` is extended to reject `map_to_existing` entries that have a nil `DestinationID`.

### ApplyImport

- Shell groups are created/mapped in the same dependency phase as regular groups, so their DB IDs are in `idMap` before resources/notes need them.
- `map_to_existing`: `idMap` entry points to the existing DB group's ID. Any m2m links recorded in the shell group's payload are wired to the mapped-to group (the `idMap` resolution handles this naturally). Resources/notes that have this shell as their `OwnerRef` get assigned to the mapped group.
- `create`: New group row with archived name, category, tags, meta. Pulled-in resources/notes owned by this shell are imported with the newly created group as owner.
- M2m wiring works unchanged — pulled-in entities resolve links through `idMap`.

### Merge Semantics for map_to_existing

When a shell group is mapped to an existing destination group, the relation-wiring phase can encounter conflicts: the destination group may already have m2m associations or typed GroupRelation rows that collide with what the archive wants to create.

**GroupRelation rows** have a unique constraint on `(FromGroupId, ToGroupId, RelationTypeId)` (`group_relation_model.go:28-32`). The current apply code does a raw `db.Create` (`apply_import.go:1150`), which would fail on duplicates. When wiring relations for a `map_to_existing` shell, the apply phase must use **conflict-ignore** semantics: attempt the insert, and if the unique constraint fires, skip the row silently. This is the correct behavior because the existing relation already expresses the same intent.

**M2m join-table rows** (RelatedGroups, RelatedResources, RelatedNotes, Tags) are wired via GORM's `Association("...").Append(...)`, which already handles duplicates gracefully on the join table — duplicate inserts are no-ops. No change needed here.

**Owned resources/notes**: when a pulled-in resource's `OwnerRef` resolves to an existing group via `map_to_existing`, the resource is created with that group as owner. No conflict is possible since it's a new resource row with a new ID.

### ImportApplyResult Changes

`ImportApplyResult` gains counters for shell group handling:

```go
CreatedShellGroups int `json:"created_shell_groups"`
MappedShellGroups  int `json:"mapped_shell_groups"`
```

## CLI & API

### Export

**CLI:** New flag `--related-depth N` (default 0). The export command uses positional args for group IDs (`group_export.go:28`), not `--group`:
```
mr group export 42 --related-depth 2
```

`--related-depth` is added to `exportCmdOptions` and registered via `registerExportFlags`. It is a plain `IntVar`, not a triState pair — there is no `--no-related-depth`.

**API (export request body):** New field `"relatedDepth": 2`.

Existing `--include-related` / `--no-related` (maps to `RelatedM2M`) and `--include-group-relations` / `--no-group-relations` scope flags still control which edge types are walked. `relatedDepth` controls how far.

### Import

**API (import plan response):** Shell groups appear with `"shell": true` on their entries. No new endpoints.

**CLI:** `buildCLIDecisions` (`group_import.go:188`) is extended to populate `ShellGroupActions`:
- Default behavior (no flags): all shell groups get `Action: "create"`. This parallels how schema-def mappings default to the plan's suggestion.
- `--decisions <file>`: a JSON file can specify per-shell-group `map_to_existing` entries with destination IDs, overriding the default. This is the only way to use `map_to_existing` from the CLI — same pattern as ambiguous NoteType mappings, which also require `--decisions`.
- No new CLI flag for `map_to_existing` — it's an uncommon, per-group decision that doesn't lend itself to a single flag. The `--decisions` file is the right mechanism.

### Template UI

Numeric input for related depth, defaulting to 0. Shown when RelatedM2M or GroupRelations scope flag is toggled on.

Import review screen: shell groups are visually distinguished in the plan tree. Each shell group gets a dropdown with `create` (default) and `map_to_existing` (with a group picker). The decision is sent as part of `ShellGroupActions` in the apply request.

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
| Series on BFS resources | BFS-discovered resource has a Series. Verify SeriesRef is set and series payload is in archive |
| Shell group with owned entities | Shell group owns pulled-in resources. Import plan shows correct resource counts on the shell item |
| Shell group map validation | `map_to_existing` with nil DestinationID rejected by ValidateForApply |
| Map duplicate GroupRelation | Shell mapped to existing group that already has the same typed relation. Verify conflict-ignore: no error, no duplicate row |
| Map duplicate m2m | Shell mapped to existing group that already has a RelatedResource in common. Verify no error, no duplicate join row |
| CLI default shell create | CLI import with no `--decisions` file. All shell groups get `Action: "create"` |
| CLI decisions file map | CLI import with `--decisions` specifying `map_to_existing` for a shell. Verify mapping applied |

### E2E Tests

| Test | Verifies |
|------|----------|
| CLI round-trip | Export with `--related-depth 1`, re-import, related entities present |
| UI export form | Set related depth, verify plan shows shell groups distinctly |
