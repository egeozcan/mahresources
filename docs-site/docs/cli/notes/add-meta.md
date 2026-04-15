---
title: mr notes add-meta
description: Add metadata to multiple notes
sidebar_label: add-meta
---

# mr notes add-meta

Add metadata keys to every Note listed in `--ids` by passing a JSON
string via `--meta`. The server-side endpoint at
`POST /v1/notes/addMeta` determines whether this merges on top of
existing meta or replaces it — see the admin interface docs for exact
semantics. For single-note single-key edits, use `note edit-meta`
(dot-path syntax).

## Usage

    mr notes add-meta

## Examples

**Set a single key on multiple notes**

    mr notes add-meta --ids 1,2,3 --meta '{"status":"reviewed"}'

**Set multiple keys at once (JSON object)**

    mr notes add-meta --ids 1,2 --meta '{"priority":5,"owner":"alice"}'


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--ids` | string | `` | Comma-separated note IDs (required) **(required)** |
| `--meta` | string | `` | Meta JSON string (required) **(required)** |
### Inherited global flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | bool | `false` | Output raw JSON |
| `--no-header` | bool | `false` | Omit table headers |
| `--page` | int | `1` | Page number for list commands (default page size: 50) |
| `--quiet` | bool | `false` | Only output IDs |
| `--server` | string | `http://localhost:8181` | mahresources server URL (env: MAHRESOURCES_URL) |
## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr note edit-meta`](../note/edit-meta.md)
- [`mr notes meta-keys`](./meta-keys.md)
- [`mr notes add-tags`](./add-tags.md)
