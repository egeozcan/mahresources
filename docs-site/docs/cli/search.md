---
title: mr search
description: Search across all entities
sidebar_label: search
---

# mr search



## Usage

    mr search <query>

Positional arguments:

- `<query>`


## Examples


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--types` | string | `` | Comma-separated entity types to search (e.g. resources,notes) |
| `--limit` | int | `20` | Maximum number of results |
### Inherited global flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | bool | `false` | Output raw JSON |
| `--no-header` | bool | `false` | Omit table headers |
| `--page` | int | `1` | Page number for list commands (default page size: 50) |
| `--quiet` | bool | `false` | Only output IDs |
| `--server` | string | `http://localhost:8181` | mahresources server URL (env: MAHRESOURCES_URL) |
## Exit Codes

