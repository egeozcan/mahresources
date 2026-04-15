---
title: mr groups add-meta
description: Add metadata to multiple groups
sidebar_label: add-meta
---

# mr groups add-meta

Merge a Meta JSON object onto multiple Groups at once. Both arguments
are required: `--ids` selects the target Groups (comma-separated) and
`--meta` is a JSON object string that is deep-merged onto each target's
existing Meta. Existing keys are overwritten by the incoming value;
keys not present in `--meta` are preserved.

To edit a single path on a single group, prefer `group edit-meta` which
takes a dotted path + JSON literal. This bulk variant is best for
stamping the same set of keys across many Groups.

## Usage

```bash
mr groups add-meta
```

## Examples

**Stamp one Meta key across three groups**

```bash
mr groups add-meta --ids 10,11,12 --meta '{"reviewed":true}'
```

**Merge multiple keys**

```bash
mr groups add-meta --ids 10 --meta '{"season":"winter","owner":"alice"}'
```


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--ids` | string | `` | Comma-separated group IDs (required) **(required)** |
| `--meta` | string | `` | Meta JSON string (required) **(required)** |
### Inherited global flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | bool | `false` | Output raw JSON |
| `--no-header` | bool | `false` | Omit table headers |
| `--page` | int | `1` | Page number for list commands (default page size: 50) |
| `--quiet` | bool | `false` | Only output IDs |
| `--server` | string | `http://localhost:8181` | mahresources server URL (env: MAHRESOURCES_URL) |
## Output

Status object with ok (bool)

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr group edit-meta`](../group/edit-meta.md)
- [`mr groups meta-keys`](./meta-keys.md)
- [`mr group get`](../group/get.md)
