---
sidebar_position: 1
title: Group Export / Import
---

# Group Export / Import

Export and import move groups and their associated entities between mahresources instances as self-contained tar archives. A single export captures groups, resources (with file bytes), notes, tags, categories, series, and typed group relations into a portable archive. The feature is available through the web UI (group detail page), the REST API, and the `mr` CLI.

## Export

An export starts from one or more root groups and walks outward according to three sets of toggles: scope, fidelity, and schema definitions.

### Scope Toggles

Control which entities the export collects.

| Toggle | Field | Default | Description |
|--------|-------|---------|-------------|
| Subtree | `subtree` | on | Include all descendant subgroups |
| Owned Resources | `owned_resources` | on | Include resources owned by in-scope groups |
| Owned Notes | `owned_notes` | on | Include notes owned by in-scope groups |
| Related M2M | `related_m2m` | on | Include many-to-many related entities (resources, notes, groups linked via join tables) |
| Group Relations | `group_relations` | on | Include typed group-to-group relations |

### Fidelity Toggles

Control how much data each resource carries.

| Toggle | Field | Default | Description |
|--------|-------|---------|-------------|
| Resource Blobs | `resource_blobs` | on | Include actual file bytes for resources |
| Resource Versions | `resource_versions` | off | Include version history files |
| Resource Previews | `resource_previews` | off | Include generated preview/thumbnail files |
| Resource Series | `resource_series` | on | Preserve Series membership |

### Schema Definition Toggles

Control whether the archive embeds full definitions for schema objects referenced by exported entities.

| Toggle | Field | Default | Description |
|--------|-------|---------|-------------|
| Categories and Types | `categories_and_types` | on | Include Category, NoteType, and ResourceCategory definitions |
| Tags | `tags` | on | Include Tag definitions with description and meta |
| Group Relation Types | `group_relation_types` | on | Include GroupRelationType definitions |

When a schema definition toggle is off, entities still carry the referenced name (e.g. `category_name`), so the importer can match by name. Including the full definition allows the importer to create an identical copy when no match exists on the destination.

### Dangling References

When an entity inside the export scope references an entity outside the scope (e.g. a group relation pointing to a group that was not selected for export), the export records a **dangling reference** in the manifest. Dangling references carry the original source ID, name, kind, and reason. During import, the user can choose to drop each dangling reference or map it to a local entity.

Five kinds of dangling references exist:

| Kind | Description |
|------|-------------|
| `related_group` | M2M group link pointing outside the export |
| `related_resource` | M2M resource link pointing outside |
| `related_note` | M2M note link pointing outside |
| `group_relation` | Typed group relation where the target group is outside |
| `resource_series_sibling` | Series contains resources not in the export |

### Estimate

Before committing to a full export, you can request an estimate. The estimate walks the same scope as a real export but reads no file bytes and writes no tar. It returns:

- Entity counts (groups, resources, notes, series)
- Unique blob count and estimated byte total
- Dangling reference counts by kind

This is useful for large exports where you want to verify scope before waiting for the archive to build.

### CLI Export Examples

```bash
# Export a single group (waits for completion, writes to stdout)
mr group export 42 -o backup.tar

# Export multiple groups with gzip
mr group export 1 2 3 --gzip -o groups.tar.gz

# Metadata-only export (no file bytes)
mr group export 42 --no-blobs -o metadata.tar

# Include version history and previews
mr group export 42 --include-versions --include-previews -o full.tar

# Skip schema definitions
mr group export 42 --schema-defs=none -o slim.tar

# Fire-and-forget (returns job ID immediately)
mr group export 42 --no-wait

# Custom polling and timeout
mr group export 42 --poll-interval 5s --timeout 1h -o large.tar
```

All scope and fidelity toggles support `--include-X` and `--no-X` flag pairs:

```bash
# Export without notes or group relations
mr group export 42 --no-notes --no-group-relations -o partial.tar

# Export with only categories (skip tags and relation type defs)
mr group export 42 --no-tag-defs --no-group-relation-type-defs -o cats-only.tar
```

## Import

Import is a multi-step workflow: upload the tar, review the parsed plan, supply decisions, then apply.

### Workflow

1. **Upload and parse** -- Upload the tar file to the server. A background job extracts the manifest and entity JSON, identifies hash collisions with existing resources, builds schema definition mappings, and produces a plan.
2. **Review** -- The plan lists every entity that will be created, highlights resource hash conflicts, shows schema definition mapping suggestions, and flags dangling references. The web UI renders this as an interactive review screen; the CLI prints a summary.
3. **Decide** -- Supply decisions: resource collision policy, schema definition mappings, dangling reference handling, optional parent group, and any items to exclude.
4. **Apply** -- A background job creates entities in dependency order (schema defs, then groups top-down, then resources, then notes), wiring up all cross-references by export ID.

### Resource Collision Policy

When a resource in the archive has the same SHA1 hash as an existing resource on the destination:

| Policy | Behavior |
|--------|----------|
| `skip` (default) | Do not create a new resource; reuse the existing one for relationship wiring |
| `duplicate` | Create a new resource even though the file content already exists |

### Schema Definition Mapping

For each schema definition in the archive (categories, note types, resource categories, tags, group relation types), the parser compares against local definitions:

| Scenario | Suggestion | Description |
|----------|------------|-------------|
| Exact name match (unique) | `map` | Map to the existing local definition |
| No match | `create` | Create a new definition from the archive payload |
| Multiple name matches | ambiguous | User must choose which local definition to map to, or create a new one |

The `--auto-map` CLI flag (on by default) accepts all unambiguous suggestions automatically. Set `--auto-map=false` to require a `--decisions` file with explicit choices for every mapping.

### Manifest-Only Imports

When an archive was exported with `resource_blobs` off, resources have metadata but no file bytes. The plan flags these as "missing hashes." To proceed, you must explicitly acknowledge this:

- Web UI: a checkbox in the review screen
- CLI: `--acknowledge-missing-hashes`
- API: `acknowledge_missing_hashes: true` in the decisions JSON

Resources with missing bytes are created as metadata-only records (no file on disk).

### Series Reconciliation

Series are matched by slug. If a series with the same slug exists on the destination, the importer reuses it. Otherwise a new series is created. The plan's `series_info` array shows the action (`reuse_existing` or `create`) for each series.

### CLI Import Examples

```bash
# Full import (parse, auto-map, apply)
mr group import backup.tar

# Dry run (parse and print plan, do not apply)
mr group import backup.tar --dry-run

# Save plan to a file for inspection
mr group import backup.tar --dry-run --plan-output plan.json

# Import into a specific parent group
mr group import backup.tar --parent-group 10

# Duplicate resources instead of skipping hash matches
mr group import backup.tar --on-resource-conflict duplicate

# Supply explicit decisions from a file
mr group import backup.tar --decisions decisions.json

# Disable auto-mapping (requires --decisions)
mr group import backup.tar --auto-map=false --decisions decisions.json

# Acknowledge missing bytes in a manifest-only archive
mr group import metadata.tar --acknowledge-missing-hashes

# JSON output for scripting
mr group import backup.tar --json
```

## API Endpoints

### Export

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/groups/export/estimate` | Estimate the size and shape of a proposed export |
| `POST` | `/v1/groups/export` | Enqueue a group export job; returns a job ID |
| `GET` | `/v1/exports/{jobId}/download` | Download the completed export tar (409 if not ready, 410 if expired) |

### Import

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/groups/import/parse` | Upload an import tar (multipart) and start the parse job |
| `GET` | `/v1/imports/{jobId}/plan` | Get the parsed import plan |
| `POST` | `/v1/imports/{jobId}/apply` | Submit decisions and start the apply job |
| `GET` | `/v1/imports/{jobId}/result` | Get the apply result (created counts, warnings) |
| `DELETE` | `/v1/imports/{jobId}` | Cancel and clean up a pending import (deletes staged tar and plan) |

Export and import jobs run through the shared [job system](./job-system). Poll `/v1/jobs/events` for real-time progress, or use the CLI's built-in polling (`--poll-interval`, `--timeout`).

## Archive Format

The export produces a standard tar (optionally gzipped). The first entry is always `manifest.json`.

### Tar Layout

```
manifest.json
schema_defs/
  categories/cat_0001.json
  note_types/nt_0001.json
  resource_categories/rc_0001.json
  tags/tag_0001.json
  group_relation_types/grt_0001.json
groups/
  g0001.json
  g0002.json
notes/
  n0001.json
resources/
  r0001.json
  r0002.json
series/
  s0001.json
blobs/
  <sha1_hash>
versions/
  <sha1_hash>
previews/
  <export_id>_<width>x<height>
```

Entity JSON files use export IDs (e.g. `g0001`, `r0042`) for all internal cross-references instead of database IDs. This makes the archive portable across instances.

### Manifest Contract

The manifest (`manifest.json`) is schema version 1. It contains:

- **Export options** -- The exact scope, fidelity, and schema-def toggles used
- **Roots** -- The export IDs of the root groups
- **Counts** -- Totals for groups, notes, resources, series, blobs, previews, versions
- **Entries** -- An index of every entity with its export ID, name, source ID, and path in the tar
- **Schema defs** -- An index of every schema definition with its export ID, name, and path
- **Dangling references** -- References that point outside the export scope
- **Warnings** -- Non-fatal issues encountered during export

Forward compatibility rules:

- Importers must ignore unknown top-level keys in `manifest.json`
- Importers must ignore unknown keys in entity payloads
- Importers must reject archives with a `schema_version` higher than they support
- The `schema_version` increments only for breaking changes to existing fields
