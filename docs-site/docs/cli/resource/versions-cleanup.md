---
title: mr resource versions-cleanup
description: Clean up old versions of a resource
sidebar_label: versions-cleanup
---

# mr resource versions-cleanup

Bulk-delete old versions of a single Resource. Retains the N most recent
versions (`--keep`), deletes versions older than N days
(`--older-than-days`), or both together: a version must satisfy every
filter given to be deleted. Pass `--dry-run` to preview without
deleting.

The current version is never a candidate, and the last remaining version
of a resource is never deleted, so a cleanup always leaves at least one
version in place.

## Usage

```bash
mr resource versions-cleanup <resource-id>
```

Positional arguments:

- `<resource-id>`


## Examples

**Keep only the last 3 versions**

```bash
mr resource versions-cleanup 42 --keep 3
```

**Delete versions older than 90 days (preview)**

```bash
mr resource versions-cleanup 42 --older-than-days 90 --dry-run
```


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--keep` | uint | `0` | Number of versions to keep |
| `--older-than-days` | uint | `0` | Delete versions older than N days |
| `--dry-run` | bool | `false` | Preview without deleting |
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

- [`mr resource versions`](./versions.md)
- [`mr resource version-delete`](./version-delete.md)
- [`mr resources versions-cleanup`](../resources/versions-cleanup.md)
