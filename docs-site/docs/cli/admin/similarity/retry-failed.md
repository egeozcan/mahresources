---
title: mr admin similarity retry-failed
description: Reset failed hashes so the backfill worker retries them
sidebar_label: retry-failed
---

# mr admin similarity retry-failed

Reset image_hashes rows that were marked failed (undecodable file at hash time) so the background backfill worker attempts them again. Prints how many rows were re-queued. Use this after fixing missing files or storage configuration.

## Usage

```bash
mr admin similarity retry-failed
```

## Examples

**Re-queue all failed hashes**

```bash
mr admin similarity retry-failed
```

**Print the raw JSON result**

```bash
mr admin similarity retry-failed --json
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

Object with reset (number of failed rows re-queued)

## Exit Codes

0 on success; 1 on error

## See Also

- [`mr admin similarity recompute`](./recompute.md)
- [`mr admin stats`](../stats.md)
