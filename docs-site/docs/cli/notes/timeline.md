---
title: mr notes timeline
description: Display a timeline of note activity
sidebar_label: timeline
---

# mr notes timeline

Display a timeline of Note activity as an ASCII bar chart. Each bucket
(yearly, monthly, or weekly, controlled by `--granularity`) prints two
bars: a solid bar for the Notes created in it and a shaded bar for the
Notes updated in it, followed by a legend.

The chart is anchored at the `--anchor` date (default: today) and
shows `--columns` buckets backward from the anchor (default 15; a value
outside 1 to 60 is ignored and 15 is used). The `--name`,
`--description`, `--tags`, `--groups`, `--owner-id`, `--note-type-id`,
`--created-before` and `--created-after` filter flags apply the same way
to the timeline aggregation; `--mrql` is not available here. Pass the
global `--json` flag to get the raw bucket data for scripting.

## Usage

```bash
mr notes timeline
```

## Examples

**Monthly timeline anchored at today (default)**

```bash
mr notes timeline
```

**Weekly granularity**

```bash
mr notes timeline --granularity weekly --columns 12
```

**Yearly timeline filtered by tag**

```bash
mr notes timeline --granularity yearly --tags 5 --json
```


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--granularity` | string | `monthly` | Bucket granularity: yearly, monthly, or weekly |
| `--anchor` | string | `` | Anchor date (YYYY-MM-DD); defaults to today |
| `--columns` | int | `15` | Number of timeline buckets (max 60) |
| `--name` | string | `` | Filter by name |
| `--description` | string | `` | Filter by description |
| `--tags` | string | `` | Comma-separated tag IDs to filter by |
| `--groups` | string | `` | Comma-separated group IDs to filter by |
| `--owner-id` | uint | `0` | Filter by owner group ID |
| `--note-type-id` | uint | `0` | Filter by note type ID |
| `--created-before` | string | `` | Filter by creation date (before) |
| `--created-after` | string | `` | Filter by creation date (after) |
### Inherited global flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | bool | `false` | Output raw JSON |
| `--no-header` | bool | `false` | Omit table headers |
| `--page` | int | `1` | Page number for list commands (default page size: 50) |
| `--quiet` | bool | `false` | Only output IDs |
| `--server` | string | `http://localhost:8181` | mahresources server URL (env: MAHRESOURCES_URL) |
## Output

Object with buckets (array of &#123;label, start, end, created, updated&#125;) and hasMore (&#123;left, right&#125;)

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr notes list`](./list.md)
- [`mr resources timeline`](../resources/timeline.md)
- [`mr groups timeline`](../groups/timeline.md)
