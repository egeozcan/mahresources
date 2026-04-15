---
title: mr logs list
description: List log entries
sidebar_label: list
---

# mr logs list



## Usage

    mr logs list

## Examples


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--level` | string | `` | Filter by level (info/warning/error) |
| `--action` | string | `` | Filter by action (create/update/delete/system) |
| `--entity-type` | string | `` | Filter by entity type |
| `--entity-id` | uint | `0` | Filter by entity ID |
| `--message` | string | `` | Filter by message |
| `--created-before` | string | `` | Filter by created before (RFC3339) |
| `--created-after` | string | `` | Filter by created after (RFC3339) |
### Inherited global flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | bool | `false` | Output raw JSON |
| `--no-header` | bool | `false` | Omit table headers |
| `--page` | int | `1` | Page number for list commands (default page size: 50) |
| `--quiet` | bool | `false` | Only output IDs |
| `--server` | string | `http://localhost:8181` | mahresources server URL (env: MAHRESOURCES_URL) |
## Exit Codes

