---
title: mr group create
description: Create a new group
sidebar_label: create
---

# mr group create



## Usage

    mr group create

## Examples


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--name` | string | `` | Group name (required) **(required)** |
| `--description` | string | `` | Group description |
| `--tags` | string | `` | Comma-separated tag IDs |
| `--groups` | string | `` | Comma-separated group IDs |
| `--meta` | string | `` | Meta JSON string |
| `--url` | string | `` | URL |
| `--owner-id` | uint | `0` | Owner group ID |
| `--category-id` | uint | `0` | Category ID |
### Inherited global flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | bool | `false` | Output raw JSON |
| `--no-header` | bool | `false` | Omit table headers |
| `--page` | int | `1` | Page number for list commands (default page size: 50) |
| `--quiet` | bool | `false` | Only output IDs |
| `--server` | string | `http://localhost:8181` | mahresources server URL (env: MAHRESOURCES_URL) |
## Exit Codes

