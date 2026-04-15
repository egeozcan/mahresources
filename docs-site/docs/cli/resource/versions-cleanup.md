---
title: mr resource versions-cleanup
description: Clean up old versions of a resource
sidebar_label: versions-cleanup
---

# mr resource versions-cleanup

Bulk-delete old versions of a single Resource. Retains either the N most
recent versions (`--keep`) or deletes versions older than N days
(`--older-than-days`). Pass `--dry-run` to preview without deleting.

## Usage

    mr resource versions-cleanup <resource-id>

Positional arguments:

- `<resource-id>`


## Examples

**Keep only the last 3 versions**

    mr resource versions-cleanup 42 --keep 3

**Delete versions older than 90 days (preview)**

    mr resource versions-cleanup 42 --older-than-days 90 --dry-run


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
