---
title: mr groups timeline
description: Display a timeline of group activity
sidebar_label: timeline
---

# mr groups timeline

Display a timeline of group creation and update activity as an ASCII bar chart.

Examples:
  mr groups timeline
  mr groups timeline --granularity=weekly --columns=20
  mr groups timeline --granularity=yearly --anchor=2020-01-01
  mr groups timeline --json

## Usage

    mr groups timeline

## Examples


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
## Exit Codes

