---
title: mr queries timeline
description: Display a timeline of query activity
sidebar_label: timeline
---

# mr queries timeline

Display a timeline of saved-Query activity as an ASCII bar chart.
Each bar represents a time bucket (yearly, monthly, or weekly,
controlled by `--granularity`), with bar height reflecting the count
of queries created in that bucket.

The chart is anchored at the `--anchor` date (default: today) and
shows `--columns` buckets backward from the anchor (default 15, max
60). Pass `--name` to filter by query name substring. Pass the
global `--json` flag to get the raw bucket data for scripting.

## Usage

```bash
mr queries timeline
```

## Examples

**Monthly timeline anchored at today (default)**

```bash
mr queries timeline
```

**Weekly granularity**

```bash
mr queries timeline --granularity weekly --columns 12
```

**Yearly timeline as JSON**

```bash
mr queries timeline --granularity yearly --json
```


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--granularity` | string | `monthly` | Bucket granularity: yearly, monthly, or weekly |
| `--anchor` | string | `` | Anchor date (YYYY-MM-DD); defaults to today |
| `--columns` | int | `15` | Number of timeline buckets (max 60) |
| `--name` | string | `` | Filter by name |
### Inherited global flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | bool | `false` | Output raw JSON |
| `--no-header` | bool | `false` | Omit table headers |
| `--page` | int | `1` | Page number for list commands (default page size: 50) |
| `--quiet` | bool | `false` | Only output IDs |
| `--server` | string | `http://localhost:8181` | mahresources server URL (env: MAHRESOURCES_URL) |
## Output

Object with buckets array (each bucket has label, start, end, created, updated) and hasMore (left, right)

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr queries list`](./list.md)
- [`mr resources timeline`](../resources/timeline.md)
- [`mr groups timeline`](../groups/timeline.md)
