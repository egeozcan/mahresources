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

Content is deduplicated by hash, so re-fetching bytes the server already holds
never produces a second resource. With no `--owner-id` given, the request is
refused with HTTP 400 naming the existing one. The doctest below fetches the
same asset on every run, so its cleanup has to run on *every* exit path -- a run
that dies after the create leaves the row behind, and every later run of the
block then fails on that duplicate however unique its `--name` is. Hence the
`trap` rather than a trailing `delete`, which `bash -e` would skip.

The trap is armed *before* the create and resolves the resource by its unique
name at exit time, never from an id the create handed back. It looks the name up
through `--mrql` rather than `--name`: the latter is a LIKE filter, so it matches
any name *containing* the generated one and returns at most a page of them, and
deleting `.[0]` of that could remove a different row while leaking its own. MRQL
`name = "..."` is an equality, which is what "resolves by its unique name"
requires to be true. Arming it after
an `ID=$(... | jq ...)` capture would leave the one window that matters: the
create has committed on the server, the pipeline that reads its id fails, and
`bash -e` exits with no trap installed. The name is generated before the create
and is all the trap needs, so nothing about the create's output can decide
whether the row is cleaned up.

## Usage

```bash
mr resource from-url
```

## Examples

**Create from a URL**

```bash
mr resource from-url --url https://example.com/photo.jpg
```

**With metadata and groups**

```bash
mr resource from-url --url https://example.com/doc.pdf --name "Paper" --meta '{"source":"arxiv"}' --groups 5
```


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
