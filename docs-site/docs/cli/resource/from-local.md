---
title: mr resource from-local
description: Create a resource from a local server path
sidebar_label: from-local
---

# mr resource from-local

Create a Resource from a file already present on the server's filesystem.
Differs from `upload` (which streams bytes over HTTP) in that the server
reads the file in place. The `--path` flag is required and must resolve
on the target server. Useful for bulk-importing existing files or
deploying pre-staged assets.

## Usage

    mr resource from-local

## Examples

**Create from a server-local path**

    mr resource from-local --path /var/mahresources/incoming/photo.jpg

**With metadata**

    mr resource from-local --path /srv/imports/doc.pdf --name "Doc" --tags 3,7


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--path` | string | `` | Local server path (required) **(required)** |
| `--name` | string | `` | Resource name |
| `--description` | string | `` | Resource description |
| `--tags` | string | `` | Comma-separated tag IDs |
| `--groups` | string | `` | Comma-separated group IDs |
| `--owner-id` | uint | `0` | Owner group ID |
| `--meta` | string | `` | Meta JSON string |
### Inherited global flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | bool | `false` | Output raw JSON |
| `--no-header` | bool | `false` | Omit table headers |
| `--page` | int | `1` | Page number for list commands (default page size: 50) |
| `--quiet` | bool | `false` | Only output IDs |
| `--server` | string | `http://localhost:8181` | mahresources server URL (env: MAHRESOURCES_URL) |
## Output

Resource object with id

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr resource upload`](./upload.md)
- [`mr resource from-url`](./from-url.md)
