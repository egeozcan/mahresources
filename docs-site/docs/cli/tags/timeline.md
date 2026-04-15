---
title: mr tags timeline
description: Display a timeline of tag activity
sidebar_label: timeline
---

# mr tags timeline

Display a timeline of Tag activity as an ASCII bar chart. Each bar
represents a time bucket (yearly, monthly, or weekly, controlled by
`--granularity`), and the bar height reflects the count of Tags created
in that bucket.

The chart is anchored at the `--anchor` date (default: today) and shows
`--columns` buckets backward from the anchor (default 15, max 60). The
`--name` and `--description` filter flags apply the same substring
matching as `tags list`. Pass the global `--json` flag to get the raw
bucket data for scripting.

## Usage

```bash
mr tags timeline
```

## Examples

**Monthly timeline anchored at today (default)**

```bash
mr tags timeline
```

**Weekly granularity**

```bash
mr tags timeline --granularity weekly --columns 12
```

**Yearly timeline anchored at a specific date**

```bash
mr tags timeline --granularity yearly --anchor 2020-01-01 --json
```


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--granularity` | string | `monthly` | Bucket granularity: yearly, monthly, or weekly |
| `--anchor` | string | `` | Anchor date (YYYY-MM-DD); defaults to today |
| `--columns` | int | `15` | Number of timeline buckets (max 60) |
| `--name` | string | `` | Filter by name |
| `--description` | string | `` | Filter by description |
### Inherited global flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | bool | `false` | Output raw JSON |
| `--no-header` | bool | `false` | Omit table headers |
| `--page` | int | `1` | Page number for list commands (default page size: 50) |
| `--quiet` | bool | `false` | Only output IDs |
| `--server` | string | `http://localhost:8181` | mahresources server URL (env: MAHRESOURCES_URL) |
## Output

Object with buckets ([]&#123;label, start, end, created, updated&#125;)

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr tags list`](./list.md)
- [`mr resources timeline`](../resources/timeline.md)
