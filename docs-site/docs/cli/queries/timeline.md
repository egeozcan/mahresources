---
title: mr queries timeline
description: Display a timeline of query activity
sidebar_label: timeline
---

# mr queries timeline

Display a timeline of query creation and update activity as an ASCII bar chart.

Examples:
  mr queries timeline
  mr queries timeline --granularity=weekly --columns=20
  mr queries timeline --granularity=yearly --anchor=2020-01-01
  mr queries timeline --json

## Usage

    mr queries timeline

## Examples


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
## Exit Codes

