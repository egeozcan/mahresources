---
sidebar_position: 1
title: Export / Import Guide
description: Practical recipes for exporting, importing, and migrating group data
---

# Export / Import Guide

This guide covers common export/import workflows: backing up a group tree, restoring it on the same instance, moving data between instances, and handling conflicts.

## Prerequisites

- The `mr` CLI binary (see [CLI docs](../features/cli.md))
- A running Mahresources server that the CLI can reach

## 1. Backup and Restore (Same Instance)

### Export a group tree

Export group ID 42 and all its subgroups, resources, and notes to a tar file:

```bash
mr group export 42 -o backup-42.tar
```

This includes subgroups, owned resources (with file bytes), owned notes, m2m relations, group relations, series membership, and all schema definitions (categories, tags, group relation types) by default.

To export multiple root groups in a single tar:

```bash
mr group export 42 85 -o backup-multi.tar
```

#### Web UI

You can also start an export from the group detail page in the web UI. The Admin section provides export controls with scope and fidelity options. The resulting tar is downloadable from the job status page.

### Full fidelity export (versions + previews)

By default, resource version history and preview images are excluded to keep the tar small. Include them for a complete backup:

```bash
mr group export 42 \
  --include-versions \
  --include-previews \
  -o backup-42-full.tar
```

### Compressed export

Add `--gzip` to compress the tar:

```bash
mr group export 42 --gzip -o backup-42.tar.gz
```

### Restore on the same instance

Import the tar with auto-mapping enabled (the default). Resources that already exist (matching hash) are skipped automatically:

```bash
mr group import backup-42.tar
```

The import process:
1. Uploads and parses the tar
2. Builds a plan (schema mappings, conflict detection)
3. Applies the plan with `--auto-map` (maps categories, tags, etc. to existing definitions by name)

Resources with identical hashes on the destination are skipped, so re-importing is safe.

### Manifest-only backup (metadata only)

Export only the metadata without file bytes. This produces a lightweight tar suitable for auditing or metadata-only migration:

```bash
mr group export 42 --no-blobs -o manifest-42.tar
```

When importing a manifest-only tar, resources without bytes are skipped unless you explicitly acknowledge the missing data:

```bash
mr group import manifest-42.tar --acknowledge-missing-hashes
```

:::warning
A manifest-only import creates resource records without files. The resources will appear in the UI but their files will be missing. Use this only when you intend to supply the files through another mechanism.
:::

## 2. Moving Data Between Instances

### Export from the source

On the source instance, export the group tree:

```bash
mr --server http://source:8181 group export 42 -o transfer.tar
```

### Import to the destination

On the destination instance, import into a specific parent group:

```bash
mr --server http://destination:8181 group import transfer.tar --parent-group 10
```

The `--parent-group` flag places the imported top-level groups under group ID 10 on the destination.

### Scripted migration (dry-run, plan, decisions, apply)

For production migrations, use a multi-step workflow that lets you review and adjust before committing.

**Step 1: Dry-run to inspect the plan**

```bash
mr group import transfer.tar --dry-run --plan-output plan.json
```

This uploads and parses the tar, prints a summary, saves the full plan to `plan.json`, and then cleans up without applying anything.

**Step 2: Review the plan**

Inspect `plan.json` to see what will be created, which schema definitions need mapping, and whether there are conflicts or dangling references.

```bash
cat plan.json | python3 -m json.tool | less
```

Key fields to check:
- `counts` -- groups, resources, notes, blobs to be processed
- `mappings` -- categories, tags, note types, resource categories, group relation types and their suggested actions (`create`, `map`, or ambiguous)
- `conflicts.resource_hash_matches` -- resources already present on destination
- `dangling_refs` -- references to entities outside the export scope

**Step 3: Create a decisions file**

If the auto-mapped suggestions are not correct (e.g., ambiguous tag names, or you want to map a source category to a different destination category), create a decisions JSON file:

```json
{
  "parent_group_id": 10,
  "resource_collision_policy": "skip",
  "mapping_actions": {
    "category:Photography": {
      "include": true,
      "action": "map",
      "destination_id": 5
    },
    "tag:landscape": {
      "include": true,
      "action": "create"
    }
  },
  "dangling_actions": {
    "dangling-ref-id-1": {
      "action": "drop"
    }
  }
}
```

**Step 4: Apply with decisions**

```bash
mr group import transfer.tar --decisions decisions.json
```

This re-uploads the tar, parses it, and applies using your explicit decisions. The `--decisions` flag overrides `--auto-map` and all other mapping flags.

## 3. Handling Conflicts

### Resource hash collisions

When a resource in the tar has the same hash as one already on the destination, the default policy is `skip` -- the existing resource is kept and the import moves on.

To create duplicate records instead (separate resource entries pointing to the same content):

```bash
mr group import transfer.tar --on-resource-conflict duplicate
```

### Schema definition mismatches

During parsing, the import engine matches source schema definitions (categories, tags, note types, resource categories, group relation types) to destination definitions by name.

- **Exact match**: auto-mapped to the existing definition
- **No match**: suggested action is `create` (a new definition will be created)
- **Ambiguous**: multiple destination candidates match; requires explicit resolution

To disable auto-mapping and force manual resolution of all schema definitions:

```bash
mr group import transfer.tar --auto-map=false --decisions decisions.json
```

:::warning
`--auto-map=false` requires a `--decisions` file. Without it, the import will refuse to proceed because all mappings are unresolved.
:::

### Dangling references

When the export includes a group relation that points to a group outside the export scope, it appears as a dangling reference in the plan. By default, the CLI drops all dangling references.

To handle them explicitly, use a decisions file with `dangling_actions` entries. Each dangling reference can be:
- `"drop"` -- remove the reference
- Mapped to a destination entity by providing a `destination_id`

## 4. Retention and Cleanup

### Export tar retention

Completed export tars are stored on the server's disk and cleaned up automatically. The retention period defaults to 24 hours and is configurable:

```bash
# Server flag
./mahresources -export-retention 48h

# Environment variable
EXPORT_RETENTION=48h
```

After the retention period, the tar file is deleted from disk. Download your exports before they expire.

### Import cleanup

Import staging files (the uploaded tar and parsed plan) remain on disk until explicitly deleted. They are cleaned up automatically after a successful apply.

To delete import staging files manually (e.g., after a dry-run or if you decide not to proceed):

```bash
# Via CLI (happens automatically on dry-run)
# The dry-run flag cleans up after printing the plan

# Via API
curl -X DELETE http://localhost:8181/v1/imports/{jobId}
```

This cancels any active parse job and removes the staged tar and plan files.

### Server-side size limit

The maximum upload size for import tars defaults to 10 GB. Adjust with:

```bash
# Server flag
./mahresources -max-import-size 21474836480

# Environment variable
MAX_IMPORT_SIZE=21474836480
```

## Quick Reference

| Task | Command |
|------|---------|
| Export a group | `mr group export 42 -o backup.tar` |
| Export with versions and previews | `mr group export 42 --include-versions --include-previews -o full.tar` |
| Export compressed | `mr group export 42 --gzip -o backup.tar.gz` |
| Export metadata only | `mr group export 42 --no-blobs -o manifest.tar` |
| Exclude schema definitions | `mr group export 42 --schema-defs none -o minimal.tar` |
| Import with defaults | `mr group import backup.tar` |
| Import into a parent group | `mr group import backup.tar --parent-group 10` |
| Dry-run import | `mr group import backup.tar --dry-run --plan-output plan.json` |
| Import with decisions file | `mr group import backup.tar --decisions decisions.json` |
| Import manifest-only tar | `mr group import manifest.tar --acknowledge-missing-hashes` |
| Skip hash-matching resources | `mr group import backup.tar --on-resource-conflict skip` |
| Create duplicates on collision | `mr group import backup.tar --on-resource-conflict duplicate` |
