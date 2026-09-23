---
title: mr jobs timeline
description: Read a Job event timeline
sidebar_label: timeline
---

# mr jobs timeline

Read the durable event timeline for a visible Job. Events have a sequence
number scoped to the Job. Use `--after-sequence` to continue from a saved
cursor and `--limit` to bound a page.

## Usage

```bash
mr jobs timeline <job-id>
```

Positional arguments:

- `<job-id>`


## Examples

**Read the newest events**

```bash
mr jobs timeline 018f4db1-9b40-7f54-8f16-37a449bcf01d --limit 100 --json
```

**Continue after sequence 100**

```bash
mr jobs timeline 018f4db1-9b40-7f54-8f16-37a449bcf01d --after-sequence 100
```


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--after-sequence` | uint64 | `0` | Return events after this per-Job sequence |
| `--limit` | int | `0` | Events per page (server maximum: 500) |
### Inherited global flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | bool | `false` | Output raw JSON |
| `--no-header` | bool | `false` | Omit table headers |
| `--page` | int | `1` | Page number for list commands (default page size: 50) |
| `--quiet` | bool | `false` | Only output IDs |
| `--server` | string | `http://localhost:8181` | mahresources server URL (env: MAHRESOURCES_URL) |
## Output

A page of ordered Job events

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr jobs get`](./get.md)
- [`mr jobs list`](./list.md)
