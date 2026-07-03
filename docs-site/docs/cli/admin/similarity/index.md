---
title: mr admin similarity
description: Image similarity maintenance jobs
sidebar_label: similarity
---

# mr admin similarity

Image-similarity maintenance actions for the perceptual-hash (v2) engine. Use the subcommands to rebuild similarity pairs from stored hashes or to re-queue images whose hashing previously failed.

## Usage

```bash
mr admin similarity
```

## Examples

**Rebuild all v2 similarity pairs**

```bash
mr admin similarity recompute
```

**Retry images whose hashing failed**

```bash
mr admin similarity retry-failed
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

Subcommand group; run a subcommand

## Exit Codes

0 on success; 1 on error

## See Also

- [`mr admin similarity recompute`](./recompute.md)
- [`mr admin similarity retry-failed`](./retry-failed.md)
- [`mr admin stats`](../stats.md)
