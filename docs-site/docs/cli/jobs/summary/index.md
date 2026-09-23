---
title: mr jobs summary
description: Aggregate visible Jobs over at most 90 days
sidebar_label: summary
---

# mr jobs summary

Aggregate visible Jobs over the last 90 days or less. Apply the same state,
kind, origin, owner, actor, relationship, search, and viewer-preference filters
as `jobs list`. Use `--window` to choose a shorter interval. For an explicit
range longer than 90 days, use `jobs summary export` to create a durable Job
with a CSV or JSON artifact.

## Usage

```bash
mr jobs summary
```

## Examples

**Count a month of remote downloads**

```bash
mr jobs summary --window 30d --kind remote-download --json
```

**Queue a longer explicit range as a CSV export**

```bash
mr jobs summary export --from 2025-01-01T00:00:00Z --to 2026-01-01T00:00:00Z --format csv
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
| `--window` | string | `` | Aggregate window such as 7d or 12h (maximum: 90d) |
### Inherited global flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | bool | `false` | Output raw JSON |
| `--no-header` | bool | `false` | Omit table headers |
| `--page` | int | `1` | Page number for list commands (default page size: 50) |
| `--quiet` | bool | `false` | Only output IDs |
| `--server` | string | `http://localhost:8181` | mahresources server URL (env: MAHRESOURCES_URL) |
## Output

Aggregate counts and duration statistics for visible Jobs

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr jobs list`](../list.md)
- [`mr jobs summary export`](./export.md)
