# Entity GUID Design

Stable, portable identity for exported entities via UUID v7 GUIDs, enabling idempotent import with merge/skip/replace semantics.

## Scope

Eight entity types receive a `GUID` column:

- **Core content**: Group, Note, Resource
- **Schema defs**: Category, NoteType, ResourceCategory, Tag, GroupRelationType

Series, Preview, ResourceVersion, NoteBlock, and other derivative entities are out of scope.

## Data Model

Each of the 8 models gets:

```go
GUID string `gorm:"uniqueIndex;size:36" json:"guid,omitempty"`
```

- Column is nullable. GORM `AutoMigrate` adds it; existing rows get the zero value.
- SQLite and Postgres both allow multiple NULLs/empty strings in a unique index.
- UUID v7 (RFC 9562): time-sortable, 128-bit, standard `xxxxxxxx-xxxx-7xxx-yxxx-xxxxxxxxxxxx` format.

### Generation

- **New entities**: GORM `BeforeCreate` hook generates UUID v7 if `GUID` is empty.
- **Existing entities**: Lazy — generated and persisted on first export. No background worker, no blocking migration. See Export Logic for concurrency handling.
- **Helper**: Small shared function in `models/types/` producing UUID v7 from `crypto/rand` + `time`. No external dependency.

## Export Changes

### Archive Format

Add `guid` field (as `json:"guid,omitempty"`) to:

- `GroupPayload`, `NotePayload`, `ResourcePayload`
- `CategoryDef` (inherited by `NoteTypeDef`), `ResourceCategoryDef`, `TagDef`, `GroupRelationTypeDef`
- `GroupEntry`, `NoteEntry`, `ResourceEntry` (manifest entries, for quick lookup)
- `SchemaDefEntry` (manifest schema def entries)

This is additive with `omitempty`. Old readers silently ignore unknown fields per the archive contract. No `schema_version` bump required.

### Export Logic

In `export_context.go`, when building payloads: if `entity.GUID == ""`, generate UUID v7 and persist to DB using an atomic conditional update:

```sql
UPDATE <table> SET guid = ? WHERE id = ? AND (guid IS NULL OR guid = '')
```

If the update affects 0 rows, another concurrent export already assigned a GUID — re-read the row to get the winning value. This makes the backfill safe under concurrent exports: exactly one writer wins, and all exports for the same entity converge on the same GUID.

The generated (or re-read) GUID is then written to the archive payload.

## Import Changes

### GUID Matching

During `ParseImport`, for each incoming entity with a non-empty GUID, query the local DB (`WHERE guid = ?`). If found, record it as a GUID match in the import plan.

GUID matching takes priority over name-based matching for schema defs. If no GUID match exists, fall back to existing name-based resolution.

### New Policy Field

`ImportDecisions` gains:

```go
GUIDCollisionPolicy string `json:"guid_collision_policy"` // "merge" (default), "skip", "replace"
```

This is a global per-import policy, alongside the existing `ResourceCollisionPolicy`.

### Import Plan Enrichment

`ImportPlanItem` gains:

- `GUIDMatch bool` `json:"guid_match,omitempty"` — entity's GUID exists locally
- `GUIDMatchID uint` `json:"guid_match_id,omitempty"` — local entity's DB ID
- `GUIDMatchName string` `json:"guid_match_name,omitempty"` — local entity's name

`ConflictSummary` gains:

- `GUIDMatches int` `json:"guid_matches"`

### Apply Behavior

This table applies to Group, Note, and schema def entities. Resource has additional rules — see Resource-Specific Behavior below.

| Policy | Scalars | Meta | M2M Relationships | Timestamps |
|--------|---------|------|-------------------|------------|
| **merge** | Incoming wins (overwrite) | Deep merge (incoming keys overwrite, existing-only keys preserved, recursive) | Union (add incoming, keep existing) | Preserve `CreatedAt`, set `UpdatedAt` = now |
| **skip** | No change | No change | No change | No change |
| **replace** | Incoming overwrites all | Incoming replaces entirely | Clear all existing links, then set exactly the incoming links | Preserve `CreatedAt`, set `UpdatedAt` = now |

For all three policies, the existing entity's DB ID is mapped into `idMap` so downstream relationship wiring (M2M, owner refs) resolves correctly.

### Resource-Specific Behavior

Resources have additional state beyond scalars and M2M: blob bytes, version history, previews, and content-addressed hash identity. GUID matching and hash matching can interact, so the precedence is:

**Collision detection order**: GUID match is checked first. If a GUID match is found, the GUID collision policy applies and hash matching is skipped entirely for that resource. If no GUID match, fall through to the existing `ResourceCollisionPolicy` hash-based logic.

**Resource fields are split into two groups**:

- **Non-blob scalars**: Name, OriginalName, OriginalLocation, Description, Category, ResourceCategoryId, OwnMeta. These follow the generic scalar rules (merge = incoming wins, replace = incoming overwrites).
- **Blob-derived metadata**: ContentType, ContentCategory, Width, Height, FileSize. Under merge, these are kept in sync with the kept blob (i.e., not overwritten by incoming values, since they describe the existing file). Under replace, they are overwritten along with the blob.
- **Blob-coupled fields**: Hash, HashType, Location, StorageLocation, blob bytes, versions, previews. These are treated as a unit because they are physically tied to the file on disk.

**Per-policy blob-coupled behavior**:

| Policy | Blob bytes on disk | Versions | Previews | Hash / HashType / Location |
|--------|-------------------|----------|----------|---------------------------|
| **merge** | Keep existing. If incoming hash differs, add a warning to the result. | Keep existing. Do not import incoming versions. | Keep existing. | Keep existing unchanged. |
| **skip** | No change. | No change. | No change. | No change. |
| **replace** | Replace with incoming blob if present in archive; if archive has no blob bytes, keep existing and add a warning (see below). | Delete existing versions, import incoming versions. | Delete existing previews, import incoming previews. | Overwrite with incoming values. |

**Replace with missing blob bytes**: If the archive was exported without `resource_blobs` fidelity, the incoming resource has metadata but no file. In this case, replace updates non-blob scalars and M2M only, keeps the existing blob/hash/versions/previews intact, and adds a warning: "Resource <name>: blob not present in archive, keeping existing file."

**Rationale for merge not touching blobs**: Merge is the conservative default. Silently replacing a file's bytes is destructive and not reversible. If the user wants to update the actual file content, they should use `replace`.

### Schema Def Resolution Order

1. GUID match (exact, takes priority)
2. Name match (existing behavior, fallback)
3. User decision (create new / map to existing)

### Schema Def Unique-Name Conflicts

Schema defs have unique-name constraints (e.g., `unique_tag_name`, `unique_category_name`). GUID matching can conflict with these:

**Scenario**: Incoming tag has GUID=X, Name="Foo". Local DB has tag with GUID=X, Name="Bar" (renamed since last export). Local DB also has a different tag with Name="Foo" (name collision).

**Resolution**: When a GUID match is found, the merge/replace applies to the GUID-matched entity. If the incoming name would violate a unique constraint against a *different* local entity, the import plan flags this as a conflict requiring user decision:

- **Option A**: Rename — apply the GUID match but use a user-provided name instead of the incoming name
- **Option B**: Map to the name-colliding entity instead (discard the GUID match)
- **Option C**: Skip this entity

The import plan's `MappingEntry` already supports `Ambiguous` + `Alternatives` — GUID-vs-name conflicts surface through this same mechanism. New fields on `MappingEntry`:

- `GUIDConflict bool` `json:"guid_conflict,omitempty"` — distinguishes GUID-vs-name conflicts from pure name ambiguities
- `GUIDMatchID uint` `json:"guid_match_id,omitempty"` — the local entity matched by GUID
- `GUIDMatchName string` `json:"guid_match_name,omitempty"` — its current name

New field on `MappingAction` to carry a user-provided rename:

- `RenameTo string` `json:"rename_to,omitempty"` — the user-provided name to use instead of the incoming name. Only used for GUID conflict resolution (Option A).

New `MappingAction.Action` value for this case:

- `Action: "guid_rename"` — update the GUID-matched entity using `RenameTo` as the name. Distinct from `"create"` (which creates a new entity) and `"map"` (which maps to an existing entity by ID).

**GroupRelationType composite uniqueness**: GroupRelationTypes are identified by (Name, FromCategory, ToCategory). GUID match takes priority; if the incoming payload's composite key collides with a different local GRT, the same conflict resolution mechanism applies.

## MRQL Integration

MRQL currently supports three entity types: `resource`, `note`, `group`. Add `guid` as a `FieldString` entry to `commonFields` in `mrql/fields.go`:

```go
{Name: "guid", Type: FieldString, Column: "guid"},
```

This makes `guid` queryable on all three supported entity types. The other 5 entity types (Category, NoteType, ResourceCategory, Tag, GroupRelationType) do not have MRQL support and are not in scope for MRQL changes.

Example queries:
- `type = "group" AND guid = "0193a7f1-7b1a-7000-8abc-def012345678"`
- `type = "resource" AND guid != ""`

## UI

### Detail Pages

For Group, Note, Resource detail pages: show GUID as a small, muted metadata line (e.g., in sidebar or metadata section). Include click-to-copy using existing clipboard utilities from `src/index.js`.

Schema def entities (Category, NoteType, etc.) — no UI change. GUIDs accessible via API JSON responses.

### Import Review

- Summary line showing GUID match count (e.g., "12 entities match by GUID")
- Global GUID collision policy selector: dropdown with merge/skip/replace, defaulting to merge. Positioned alongside the existing resource collision policy control.

## API Surface

- GUID is included in JSON responses automatically (GORM default serialization).
- GUID is **read-only**: not accepted in create/update request bodies (silently ignored if present).
- Import apply endpoint (`POST /v1/imports/{jobId}/apply`) accepts `guid_collision_policy` in the decisions payload.

## Testing

- Unit tests for UUID v7 generation helper.
- Unit tests for deep merge utility.
- Unit test for concurrent lazy backfill (two goroutines exporting the same entity converge on one GUID).
- E2E round-trip: export with GUIDs, re-import with each policy (merge/skip/replace), verify outcomes.
- E2E: import archive without GUIDs (old format) still works — backward compatibility.
- E2E: resource GUID match with different hash — verify merge keeps existing blob, replace swaps it.
- E2E: schema def GUID-vs-name conflict — verify conflict surfaces in import plan.
- MRQL tests for `guid` field queries on resource, note, group.
