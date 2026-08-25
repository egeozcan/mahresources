---
title: mr resources add-meta
description: Add metadata to multiple resources
sidebar_label: add-meta
---

# mr resources add-meta

Add metadata keys to every Resource listed in `--ids` by passing a JSON
string via `--meta`. The given keys are merged over the existing `meta`
and `own_meta`: a key of the same name is overwritten, keys not named
are left alone. A blank `--meta` is a no-op, the JSON is validated
first, and the call fails if any ID in `--ids` does not exist. For
single-resource single-key edits, use `resource edit-meta` (dot-path
syntax).

## Usage

```bash
mr resources add-meta
```

## Examples

**Set a single key on multiple resources**

```bash
mr resources add-meta --ids 1,2,3 --meta '{"status":"reviewed"}'
```

**Set multiple keys at once (JSON object)**

```bash
mr resources add-meta --ids 1,2 --meta '{"priority":5,"owner":"alice"}'
```


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--ids` | string | `` | Comma-separated resource IDs (required) **(required)** |
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

- [`mr resource edit-meta`](../resource/edit-meta.md)
- [`mr resources meta-keys`](./meta-keys.md)
- [`mr resources add-tags`](./add-tags.md)
