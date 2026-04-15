---
title: mr resources timeline
description: Display a timeline of resource activity
sidebar_label: timeline
---

# mr resources timeline

Display a timeline of Resource activity as an ASCII bar chart. Each
bar represents a time bucket (yearly, monthly, or weekly, controlled by
`--granularity`), and the bar height reflects the count of Resources
created in that bucket.

The chart is anchored at the `--anchor` date (default: today) and shows
`--columns` buckets backward from the anchor (default 15, max 60). All
resource-list filter flags (`--name`, `--tags`, `--groups`, etc.) apply
the same way to the timeline aggregation. Pass the global `--json` flag
to get the raw bucket data for scripting.

## Usage

    mr resources timeline

## Examples

**Monthly timeline anchored at today (default)**

    mr resources timeline

**Weekly granularity**

    mr resources timeline --granularity weekly --columns 12

**Yearly timeline filtered by tag**

    mr resources timeline --granularity yearly --tags 5 --json


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--granularity` | string | `monthly` | Bucket granularity: yearly, monthly, or weekly |
| `--anchor` | string | `` | Anchor date (YYYY-MM-DD); defaults to today |
| `--columns` | int | `15` | Number of timeline buckets (max 60) |
| `--name` | string | `` | Filter by name |
| `--description` | string | `` | Filter by description |
| `--content-type` | string | `` | Filter by content type |
| `--owner-id` | uint | `0` | Filter by owner group ID |
| `--tags` | string | `` | Comma-separated tag IDs to filter by |
| `--groups` | string | `` | Comma-separated group IDs to filter by |
| `--notes` | string | `` | Comma-separated note IDs to filter by |
| `--resource-category-id` | uint | `0` | Filter by resource category ID |
| `--created-before` | string | `` | Filter by creation date (before) |
| `--created-after` | string | `` | Filter by creation date (after) |
| `--min-width` | uint | `0` | Minimum width |
| `--min-height` | uint | `0` | Minimum height |
| `--max-width` | uint | `0` | Maximum width |
| `--max-height` | uint | `0` | Maximum height |
| `--hash` | string | `` | Filter by hash |
| `--original-name` | string | `` | Filter by original name |
### Inherited global flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | bool | `false` | Output raw JSON |
| `--no-header` | bool | `false` | Omit table headers |
| `--page` | int | `1` | Page number for list commands (default page size: 50) |
| `--quiet` | bool | `false` | Only output IDs |
| `--server` | string | `http://localhost:8181` | mahresources server URL (env: MAHRESOURCES_URL) |
## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr resources list`](./list.md)
- [`mr groups timeline`](../groups/timeline.md)
