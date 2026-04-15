---
title: mr groups timeline
description: Display a timeline of group activity
sidebar_label: timeline
---

# mr groups timeline

Display a timeline of Group creation and update activity as an ASCII
bar chart. Each bar represents a time bucket (yearly, monthly, or
weekly, controlled by `--granularity`), and the bar height reflects
the count of Groups created in that bucket.

The chart is anchored at the `--anchor` date (default: today) and shows
`--columns` buckets backward from the anchor. All group-list filter
flags (`--name`, `--tags`, `--groups`, `--owner-id`, etc.) apply the
same way to the timeline aggregation. Pass the global `--json` flag to
get the raw bucket data for scripting — the top-level response has a
`buckets` array and a `hasMore` flag.

## Usage

    mr groups timeline

## Examples

**Monthly timeline anchored at today (default)**

    mr groups timeline

**Weekly granularity**

    mr groups timeline --granularity weekly --columns 20

**Yearly timeline anchored at 2020**

    mr groups timeline --granularity yearly --anchor 2020-01-01


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
| `--category-id` | uint | `0` | Filter by category ID |
| `--url` | string | `` | Filter by URL |
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

Object with buckets (array of {label, start, end, created, updated}) and hasMore (bool)

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr groups list`](./list.md)
- [`mr resources timeline`](../resources/timeline.md)
