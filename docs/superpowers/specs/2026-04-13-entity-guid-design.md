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
- **Existing entities**: Lazy — generated and persisted on first export. No background worker, no blocking migration.
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

In `export_context.go`, when building payloads: if `entity.GUID == ""`, generate UUID v7, persist to DB (`UPDATE ... SET guid = ? WHERE id = ?`), then write to the payload. This is the lazy backfill path for pre-existing entities.

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

| Policy | Scalars | Meta | M2M Relationships | Timestamps |
|--------|---------|------|-------------------|------------|
| **merge** | Incoming wins (overwrite) | Deep merge (incoming keys overwrite, existing-only keys preserved, recursive) | Union (add incoming, keep existing) | Preserve `CreatedAt`, set `UpdatedAt` = now |
| **skip** | No change | No change | No change | No change |
| **replace** | Incoming overwrites all | Incoming replaces entirely | Clear all existing links, then set exactly the incoming links | Preserve `CreatedAt`, set `UpdatedAt` = now |

For all three policies, the existing entity's DB ID is mapped into `idMap` so downstream relationship wiring (M2M, owner refs) resolves correctly.

### Schema Def Resolution Order

1. GUID match (exact, takes priority)
2. Name match (existing behavior, fallback)
3. User decision (create new / map to existing)

## MRQL Integration

Add `guid` to the MRQL translator's field whitelist for all 8 entity types. Standard string field — supports `=`, `!=`, `IN`, `LIKE`, etc.

Example queries:
- `guid = "0193a7f1-7b1a-7000-8abc-def012345678"`
- `guid != ""`

## UI

### Detail Pages

For Group, Note, Resource detail pages: show GUID as a small, muted metadata line (e.g., in sidebar or metadata section). Include click-to-copy using existing clipboard utilities from `src/index.js`.

Schema def entities (Category, NoteType, etc.) — no UI change. GUIDs accessible via API and MRQL.

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
- E2E round-trip: export with GUIDs, re-import with each policy (merge/skip/replace), verify outcomes.
- E2E: import archive without GUIDs (old format) still works — backward compatibility.
- MRQL tests for `guid` field queries.
