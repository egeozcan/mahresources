---
title: mr resource from-url
description: Create a resource from a remote URL
sidebar_label: from-url
---

# mr resource from-url

Create a Resource by having the server fetch a remote URL. Useful when
you have a public asset that shouldn't be proxied through your local
machine. The `--url` flag is required; the server downloads, stores, and
indexes the file. Optional `--tags` / `--groups` attach relationships at
creation.

## Usage

    mr resource from-url

## Examples

**Create from a URL**

    mr resource from-url --url https://example.com/photo.jpg

**With metadata and groups**

    mr resource from-url --url https://example.com/doc.pdf --name "Paper" --meta '{"source":"arxiv"}' --groups 5

**ephemeral server has no outbound access**

    mr resource from-url --url https://example.com/tiny.jpg --name "from-url-test"


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--url` | string | `` | Remote URL (required) **(required)** |
| `--name` | string | `` | Resource name |
| `--description` | string | `` | Resource description |
| `--tags` | string | `` | Comma-separated tag IDs |
| `--groups` | string | `` | Comma-separated group IDs |
| `--owner-id` | uint | `0` | Owner group ID |
| `--meta` | string | `` | Meta JSON string |
| `--file-name` | string | `` | Override file name |
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
- [`mr resource from-local`](./from-local.md)
