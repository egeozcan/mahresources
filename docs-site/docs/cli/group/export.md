---
title: mr group export
description: Export one or more groups to a tar archive
sidebar_label: export
---

# mr group export

Export one or more Groups and their reachable entities to a portable
tar archive. Sends `POST /v1/groups/export`, polls the resulting job
until completion, then downloads the tar. Takes one or more Group IDs
as positional arguments; each ID becomes a root of the export tree.

The archive format follows the manifest schema v1 (see `archive/manifest.go`)
and is compatible with `mr group import` on any mahresources instance.
Scope and fidelity are controlled by paired `--include-*` / `--no-*`
flags (subtree, resources, notes, related, group-relations, blobs,
versions, previews, series). Schema-definition inclusion (categories,
tag defs, group-relation types) can be toggled individually or via the
`--schema-defs=all|none|selected` shortcut. Use `--gzip` to compress
the output and `--output <path>` (or `-o`) to write to a file rather
than stdout.

By default the command waits for the server-side job to finish before
downloading; pass `--no-wait` to print the job ID and exit immediately
so you can poll and download separately.

## Usage

    mr group export <id> [<id>...]

Positional arguments:

- `<id>` (variadic; one or more)


## Examples

**Export group 42 and its subtree to a tar file**

    mr group export 42 --output /tmp/trip-2026.tar

**Export two roots**

    mr group export 42 43 --gzip --no-blobs --no-related --output /tmp/shell.tar.gz

**Submit the job and print its ID without waiting**

    mr group export 42 --no-wait


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--include-subtree` | bool | `true` | include all descendant subgroups (default on) |
| `--no-subtree` | bool | `false` | disable --include-subtree |
| `--include-resources` | bool | `true` | include owned resources (default on) |
| `--no-resources` | bool | `false` | disable --include-resources |
| `--include-notes` | bool | `true` | include owned notes (default on) |
| `--no-notes` | bool | `false` | disable --include-notes |
| `--include-related` | bool | `true` | include m2m related entities (default on) |
| `--no-related` | bool | `false` | disable --include-related |
| `--include-group-relations` | bool | `true` | include typed group relations (default on) |
| `--no-group-relations` | bool | `false` | disable --include-group-relations |
| `--include-blobs` | bool | `true` | include resource file bytes (default on) |
| `--no-blobs` | bool | `false` | disable --include-blobs |
| `--include-versions` | bool | `true` | include resource version history (default off) |
| `--no-versions` | bool | `false` | disable --include-versions |
| `--include-previews` | bool | `true` | include resource previews (default off) |
| `--no-previews` | bool | `false` | disable --include-previews |
| `--include-series` | bool | `true` | preserve Series membership (default on) |
| `--no-series` | bool | `false` | disable --include-series |
| `--include-categories-and-types` | bool | `true` | include Category/NoteType/ResourceCategory defs (D1, default on) |
| `--no-categories-and-types` | bool | `false` | disable --include-categories-and-types |
| `--include-tag-defs` | bool | `true` | include Tag definitions (D2, default on) |
| `--no-tag-defs` | bool | `false` | disable --include-tag-defs |
| `--include-group-relation-type-defs` | bool | `true` | include GroupRelationType defs (D3, default on) |
| `--no-group-relation-type-defs` | bool | `false` | disable --include-group-relation-type-defs |
| `--schema-defs` | string | `selected` | schema-def shortcut (all|none|selected — selected defers to individual --include-*-defs flags) |
| `--gzip` | bool | `false` | gzip the output tar |
| `--output` | string | `` | output file path (default stdout) |
| `--wait` | bool | `true` | wait for the job to finish before returning |
| `--no-wait` | bool | `false` | return immediately after submitting the job |
| `--poll-interval` | duration | `1s` | polling interval |
| `--timeout` | duration | `30m0s` | max total wait time |
| `--related-depth` | int | `0` | follow m2m relationships up to N hops deep (0 = off) |
### Inherited global flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | bool | `false` | Output raw JSON |
| `--no-header` | bool | `false` | Omit table headers |
| `--page` | int | `1` | Page number for list commands (default page size: 50) |
| `--quiet` | bool | `false` | Only output IDs |
| `--server` | string | `http://localhost:8181` | mahresources server URL (env: MAHRESOURCES_URL) |
## Output

Tar archive written to stdout or --output path; when --no-wait, prints the job ID as plain text

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr group import`](./import.md)
- [`mr group clone`](./clone.md)
- [`mr groups list`](../groups/list.md)
