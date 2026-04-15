---
title: mr categories timeline
description: Display a timeline of category activity
sidebar_label: timeline
---

# mr categories timeline

Display a timeline of category creation and update activity as an ASCII bar chart.

Examples:
  mr categories timeline
  mr categories timeline --granularity=weekly --columns=20
  mr categories timeline --granularity=yearly --anchor=2020-01-01
  mr categories timeline --json

## Usage

    mr categories timeline

## Examples


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
## Exit Codes

