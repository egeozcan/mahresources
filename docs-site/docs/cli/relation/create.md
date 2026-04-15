---
title: mr relation create
description: Create a new group relation
sidebar_label: create
---

# mr relation create



## Usage

    mr relation create

## Examples


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--from-group-id` | uint | `0` | Source group ID (required) **(required)** |
| `--to-group-id` | uint | `0` | Target group ID (required) **(required)** |
| `--relation-type-id` | uint | `0` | Relation type ID (required) **(required)** |
| `--name` | string | `` | Relation name |
| `--description` | string | `` | Relation description |
### Inherited global flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | bool | `false` | Output raw JSON |
| `--no-header` | bool | `false` | Omit table headers |
| `--page` | int | `1` | Page number for list commands (default page size: 50) |
| `--quiet` | bool | `false` | Only output IDs |
| `--server` | string | `http://localhost:8181` | mahresources server URL (env: MAHRESOURCES_URL) |
## Exit Codes

