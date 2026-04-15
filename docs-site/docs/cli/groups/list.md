---
title: mr groups list
description: List groups
sidebar_label: list
---

# mr groups list



## Usage

    mr groups list

## Examples


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
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

