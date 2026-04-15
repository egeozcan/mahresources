---
title: mr relation-type create
description: Create a new relation type
sidebar_label: create
---

# mr relation-type create



## Usage

    mr relation-type create

## Examples


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--name` | string | `` | Relation type name (required) **(required)** |
| `--description` | string | `` | Relation type description |
| `--reverse-name` | string | `` | Reverse relation name |
| `--from-category` | uint | `0` | From category ID |
| `--to-category` | uint | `0` | To category ID |
### Inherited global flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | bool | `false` | Output raw JSON |
| `--no-header` | bool | `false` | Omit table headers |
| `--page` | int | `1` | Page number for list commands (default page size: 50) |
| `--quiet` | bool | `false` | Only output IDs |
| `--server` | string | `http://localhost:8181` | mahresources server URL (env: MAHRESOURCES_URL) |
## Exit Codes

