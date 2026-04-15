---
title: mr note create
description: Create a new note
sidebar_label: create
---

# mr note create



## Usage

    mr note create

## Examples


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--name` | string | `` | Note name (required) **(required)** |
| `--description` | string | `` | Note description |
| `--tags` | string | `` | Comma-separated tag IDs |
| `--groups` | string | `` | Comma-separated group IDs |
| `--resources` | string | `` | Comma-separated resource IDs |
| `--meta` | string | `` | Meta JSON string |
| `--owner-id` | uint | `0` | Owner group ID |
| `--note-type-id` | uint | `0` | Note type ID |
### Inherited global flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | bool | `false` | Output raw JSON |
| `--no-header` | bool | `false` | Omit table headers |
| `--page` | int | `1` | Page number for list commands (default page size: 50) |
| `--quiet` | bool | `false` | Only output IDs |
| `--server` | string | `http://localhost:8181` | mahresources server URL (env: MAHRESOURCES_URL) |
## Exit Codes

