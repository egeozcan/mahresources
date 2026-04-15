---
title: mr job submit
description: Submit URLs for download
sidebar_label: submit
---

# mr job submit



## Usage

    mr job submit

## Examples


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--urls` | string | `` | Comma-separated URLs to download (required) **(required)** |
| `--tags` | string | `` | Comma-separated tag IDs |
| `--groups` | string | `` | Comma-separated group IDs |
| `--name` | string | `` | Job name |
| `--owner-id` | uint | `0` | Owner group ID |
### Inherited global flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | bool | `false` | Output raw JSON |
| `--no-header` | bool | `false` | Omit table headers |
| `--page` | int | `1` | Page number for list commands (default page size: 50) |
| `--quiet` | bool | `false` | Only output IDs |
| `--server` | string | `http://localhost:8181` | mahresources server URL (env: MAHRESOURCES_URL) |
## Exit Codes

