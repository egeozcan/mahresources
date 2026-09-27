---
title: mr job submit
description: Submit URLs for download
sidebar_label: submit
---

# mr job submit

Submit one or more URLs to the download queue. The server creates one
job per URL and immediately begins fetching in the background; this
command returns as soon as the jobs are queued, not when downloads
finish. Attach tags, groups, an owner, or a custom name with the
remaining flags.

Name the URLs with `--url` or `--urls`; both can be repeated and
combined. A `--url` value is exactly one URL, taken as written. A
`--urls` value is a list: it is split at newlines and at a comma that
starts another `http://` or `https://` URL. A comma anywhere else is
part of the URL, because commas are legal in URL paths and queries. Use
`--url` for a URL that itself contains a comma followed by `http`.

The server answers each URL separately: it queues the ones it accepts
and names the ones it refuses, with the reason. When any URL is refused
the command prints the answer, with the queued Job ids, and exits 1
with every refusal in the error.

Downloaded content becomes a new Resource once the fetch succeeds.
Each job's `canonicalJobId` is the Job id `jobs list` prints. Watch
progress with `jobs list` or the `/v1/jobs/events?version=2` stream.

## Usage

```bash
mr job submit
```

## Examples

**Queue a single download**

```bash
mr job submit --url https://example.com/photo.jpg
```

**Queue multiple URLs with tags and an owner group**

```bash
mr job submit --urls https://a.example.com/a.jpg,https://b.example.com/b.jpg --tags 5,7 --owner-id 3
```

**A comma inside a URL stays in that URL**

```bash
mr job submit --url "https://img.example.com/w_96,h_64/photo.jpg"
```


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--urls` | stringArray | `[]` | URLs to download, separated by newlines or by a comma that starts another http(s) URL (repeatable) |
| `--url` | stringArray | `[]` | One URL to download, taken as written, commas included (repeatable) |
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

Object with queued=true, a jobs array containing each created job's id, canonicalJobId, url, and initial status, and a refused array naming each URL the server refused

## Exit Codes

0 when every URL was queued; 1 on any error, including a batch in which the server refused a URL

## See Also

- [`mr jobs list`](../jobs/list.md)
- [`mr job cancel`](./cancel.md)
- [`mr job retry`](./retry.md)
