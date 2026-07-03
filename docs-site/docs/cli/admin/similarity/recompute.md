---
title: mr admin similarity recompute
description: Rebuild all v2 similarity pairs from stored hashes
sidebar_label: recompute
---

# mr admin similarity recompute

Submit a background job that deletes every similarity pair whose both endpoints are v2 rows and rebuilds them from the stored perceptual hashes. This performs no image decoding (it reads hashes from the database), so it is cheap enough to run after an algorithm or threshold change. Only one recompute may run at a time; a second request while one is active returns HTTP 409.

Progress is visible in the background jobs list and on the admin overview page.

## Usage

```bash
mr admin similarity recompute
```

## Examples

**Start a recompute**

```bash
mr admin similarity recompute
```

**Start a recompute and print the raw job JSON**

```bash
mr admin similarity recompute --json
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

Object with jobId (the background job identifier)

## Exit Codes

0 on success; 1 on error; the API returns 409 if a recompute is already running

## See Also

- [`mr admin similarity retry-failed`](./retry-failed.md)
- [`mr admin stats`](../stats.md)
