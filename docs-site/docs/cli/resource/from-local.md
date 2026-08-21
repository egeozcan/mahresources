---
title: mr resource from-local
description: Create a resource from a local server path
sidebar_label: from-local
---

# mr resource from-local

Create a Resource from a file already present on the server's filesystem.
Differs from `upload` (which streams bytes over HTTP) in that the server
reads the file in place. Useful for bulk-importing existing files or
deploying pre-staged assets.

`--path` is required and is resolved inside the server's storage root, not
on the host: a file staged at `$FILE_SAVE_PATH/incoming/photo.jpg` is
`--path /incoming/photo.jpg` here. The name defaults to the file's base
name.

## Usage

```bash
mr resource from-local
```

## Examples

**Create from a path under the server's storage root**

```bash
mr resource from-local --path /incoming/photo.jpg
```

**With metadata**

```bash
mr resource from-local --path /imports/doc.pdf --name "Doc" --tags 3,7
```


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
