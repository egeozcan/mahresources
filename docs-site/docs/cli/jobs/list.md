---
title: mr jobs list
description: List the download queue
sidebar_label: list
---

# mr jobs list

Return a snapshot of every job the server is currently tracking,
including pending, running, paused, finished, and failed ones. The
response is a single object whose `jobs` key is an array ordered by
submission time. Each entry exposes enough detail to drive CLI
dashboards, pause/resume decisions, or cleanup scripts.

The queue lives in server memory; a restart empties it. Pagination is
not supported — the full list is returned in one response.

## Usage

```bash
mr jobs list
```

## Examples

**Show every job (human-readable)**

```bash
mr jobs list
```

**Filter to still-running jobs and pull just their URLs**

```bash
mr jobs list --json | jq -r '.jobs[] | select(.status == "downloading" or .status == "pending") | .url'
```


## Flags

This command has no local flags.
### Inherited global flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | bool | `false` | Output raw JSON |
| `--no-header` | bool | `false` | Omit table headers |
| `--page` | int | `1` | Page number for list commands (default page size: 50) |
| `--quiet` | bool | `false` | Only output IDs |
| `--server` | string | `http://localhost:8181` | mahresources server URL (env: MAHRESOURCES_URL) |
## Output

Object with a jobs array; each entry has id, url, status, progress, totalSize, progressPercent, createdAt, and optional error, startedAt, completedAt, resourceId, source

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr job submit`](../job/submit.md)
- [`mr job cancel`](../job/cancel.md)
- [`mr job retry`](../job/retry.md)
