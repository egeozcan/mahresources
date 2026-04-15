---
title: mr mrql run
description: Run a saved MRQL query by name or ID
sidebar_label: run
---

# mr mrql run



## Usage

    mr mrql run <name-or-id>

Positional arguments:

- `<name-or-id>`


## Examples


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--limit` | int | `0` | Items per bucket for GROUP BY, or total items for regular queries |
| `--buckets` | int | `0` | Groups per page for bucketed GROUP BY queries |
| `--offset` | int | `0` | Bucket offset for cursor-based GROUP BY pagination |
| `--render` | bool | `false` | Request server-side template rendering via CustomMRQLResult |
### Inherited global flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | bool | `false` | Output raw JSON |
| `--no-header` | bool | `false` | Omit table headers |
| `--page` | int | `1` | Page number for list commands (default page size: 50) |
| `--quiet` | bool | `false` | Only output IDs |
| `--server` | string | `http://localhost:8181` | mahresources server URL (env: MAHRESOURCES_URL) |
## Exit Codes

