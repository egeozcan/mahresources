---
title: mr group export
description: Export one or more groups (and their reachable entities) to a tar file
sidebar_label: export
---

# mr group export



## Usage

    mr group export <id> [<id>...]

Positional arguments:

- `<id>` (variadic; one or more)


## Examples


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
## Exit Codes

