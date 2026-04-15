---
title: mr job submit
description: Submit URLs for download
sidebar_label: submit
---

# mr job submit

Submit one or more URLs to the download queue. The server creates one
job per URL and immediately begins fetching in the background; this
command returns as soon as the jobs are queued, not when downloads
finish. Use `--urls` with a comma-separated list; attach tags, groups,
an owner, or a custom name with the remaining flags.

Downloaded content becomes a new Resource once the fetch succeeds. Watch
progress with `jobs list` or the `/v1/download/events` SSE stream.

## Usage

```bash
mr job submit
```

## Examples

**Queue a single download**

```bash
mr job submit --urls https://example.com/photo.jpg
```

**Queue multiple URLs with tags and an owner group**

```bash
mr job submit --urls https://a.example.com/a.jpg,https://b.example.com/b.jpg --tags 5,7 --owner-id 3
```


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
## Output

Object with queued=true and a jobs array containing each created job's id, url, and initial status

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr jobs list`](../jobs/list.md)
- [`mr job cancel`](./cancel.md)
- [`mr job retry`](./retry.md)
