---
title: mr resource upload
description: Upload a file as a new resource
sidebar_label: upload
---

# mr resource upload

Upload a local file as a new Resource. Sends the file via multipart form
to `POST /v1/resource`. The Resource's name defaults to the source
filename if `--name` is not set. Use `--meta` for a JSON blob of custom
metadata that is merged into the new record.

## Usage

    mr resource upload <file>

Positional arguments:

- `<file>`


## Examples

**Basic upload (name defaults to the filename)**

    mr resource upload ./photo.jpg

**Upload with ownership and meta JSON**

    mr resource upload ./photo.jpg --owner-id 3 --meta '{"camera":"Pixel"}'

**upload a fixture and verify the returned id**

    GRP=$(mr group create --name "doctest-upload-$$-$RANDOM" --json | jq -r '.ID')
    ID=$(mr resource upload ./testdata/sample.jpg --owner-id=$GRP --name "upload-test-$$" --json | jq -r '.[0].ID')
    mr resource get $ID --json | jq -e '.ID > 0'


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--name` | string | `` | Resource name |
| `--description` | string | `` | Resource description |
| `--owner-id` | uint | `0` | Owner group ID |
| `--meta` | string | `` | Meta JSON string |
| `--category` | string | `` | Category |
| `--content-category` | string | `` | Content category |
| `--resource-category-id` | uint | `0` | Resource category ID |
| `--original-name` | string | `` | Original file name |
### Inherited global flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | bool | `false` | Output raw JSON |
| `--no-header` | bool | `false` | Omit table headers |
| `--page` | int | `1` | Page number for list commands (default page size: 50) |
| `--quiet` | bool | `false` | Only output IDs |
| `--server` | string | `http://localhost:8181` | mahresources server URL (env: MAHRESOURCES_URL) |
## Output

Resource object with id, name

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr resource edit`](./edit.md)
- [`mr resource from-url`](./from-url.md)
- [`mr resource from-local`](./from-local.md)
- [`mr resources list`](../resources/list.md)
