---
title: mr notes timeline
description: Display a timeline of note activity
sidebar_label: timeline
---

# mr notes timeline

Display a timeline of Note activity as an ASCII bar chart. Each bar
represents a time bucket (yearly, monthly, or weekly, controlled by
`--granularity`), and the bar height reflects the count of Notes
created in that bucket.

The chart is anchored at the `--anchor` date (default: today) and
shows `--columns` buckets backward from the anchor (default 15, max
60). All note-list filter flags (`--name`, `--tags`, `--groups`,
`--owner-id`, `--note-type-id`) apply the same way to the timeline
aggregation. Pass the global `--json` flag to get the raw bucket data
for scripting.

## Usage

    mr notes timeline

## Examples

**Monthly timeline anchored at today (default)**

    mr notes timeline

**Weekly granularity**

    mr notes timeline --granularity weekly --columns 12

**Yearly timeline filtered by tag**

    mr notes timeline --granularity yearly --tags 5 --json


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

Object with buckets (array of {label, start, end, created, updated}) and hasMore ({left, right})

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr notes list`](./list.md)
- [`mr resources timeline`](../resources/timeline.md)
- [`mr groups timeline`](../groups/timeline.md)
