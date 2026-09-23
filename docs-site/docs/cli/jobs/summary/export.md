---
title: mr jobs summary export
description: Queue a CSV or JSON summary export longer than 90 days
sidebar_label: export
---

# mr jobs summary export

Queue a filtered summary export for an explicit RFC3339 range longer than 90
days. The server applies the same visibility rules and filters as interactive
summary, then publishes a typed artifact on the accepted Job. The artifact
expires according to the server's export-retention setting. Read the Job with
`jobs get` to inspect its output and commands.

The accepted Job is owned by the submitting account. Filtering by another
owner or actor does not grant access to that person's Jobs.

## Usage

```bash
mr jobs summary export
```

## Examples

**Queue a CSV export for a year of remote downloads**

```bash
mr jobs summary export --from 2025-01-01T00:00:00Z --to 2026-01-01T00:00:00Z --kind remote-download --format csv
```

**Queue a JSON export with an owner filter**

```bash
mr jobs summary export --from 2024-01-01T00:00:00Z --to 2026-01-01T00:00:00Z --format json --owner-id 7
```


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--state` | stringSlice | `[]` | Filter by Job state (repeatable) |
| `--kind` | stringSlice | `[]` | Filter by Job kind (repeatable) |
| `--origin` | stringSlice | `[]` | Filter by submission origin (repeatable) |
| `--owner-id` | uint | `0` | Filter by owner user ID |
| `--actor-id` | uint | `0` | Filter by acting user ID |
| `--accepted-after` | string | `` | Include Jobs accepted at or after RFC3339 time |
| `--accepted-before` | string | `` | Include Jobs accepted at or before RFC3339 time |
| `--relationship` | string | `` | Filter by visible lineage relationship |
| `--search` | string | `` | Search visible Job text and output labels |
| `--command` | string | `` | Filter Jobs currently advertising this command key |
| `--pinned` | string | `` | Filter this viewer's pin preference (true or false) |
| `--dismissed` | string | `` | Filter this viewer's dismissal preference (true or false) |
| `--from` | string | `` | Inclusive start time in RFC3339 form (required) **(required)** |
| `--to` | string | `` | Inclusive end time in RFC3339 form (required) **(required)** |
| `--format` | string | `json` | Artifact format: csv or json |
### Inherited global flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | bool | `false` | Output raw JSON |
| `--no-header` | bool | `false` | Omit table headers |
| `--page` | int | `1` | Page number for list commands (default page size: 50) |
| `--quiet` | bool | `false` | Only output IDs |
| `--server` | string | `http://localhost:8181` | mahresources server URL (env: MAHRESOURCES_URL) |
## Output

Accepted summary-export Job snapshot

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr jobs summary`](./index.md)
- [`mr jobs get`](../get.md)
- [`mr jobs timeline`](../timeline.md)
