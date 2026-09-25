---
title: mr jobs list
description: List visible Jobs
sidebar_label: list
---

# mr jobs list

List the durable Jobs visible to the current account. The server orders results
newest first and returns an opaque `nextCursor` when another page is available.
Pass that value to `--cursor` to continue. Use the repeatable state, kind, and
origin filters, or narrow by owner, actor, accepted time, relationship, text,
advertised command, or your pin and dismissal preferences. Besides the
lifecycle states, `--state` accepts `partial`: succeeded Jobs whose Kind
recorded that the work stopped short of finished. `--inbound-relationship`
matches Jobs another visible Job links to (`retry-of`: retried or continued,
`repeat-of`: repeated, `parent-child`: a child stage), and
`--no-inbound-relationship` matches Jobs no visible Job links to that way.

The canonical list endpoint is controlled by the server's Job Center release
gate. While that endpoint is unavailable, an unfiltered `jobs list` request
falls back to the legacy download queue. Use `jobs queue` when a script needs
the legacy response explicitly.

## Usage

```bash
mr jobs list
```

## Examples

**Find failed remote downloads**

```bash
mr jobs list --state failed --kind remote-download --limit 50
```

**Find work that stopped short of finished**

```bash
mr jobs list --state partial --state failed
```

**Failed Jobs nobody has retried yet**

```bash
mr jobs list --state failed --no-inbound-relationship retry-of
```

**Continue from an opaque cursor on the next page**

```bash
mr jobs list --accepted-after 2026-01-01T00:00:00Z --cursor 'opaque-value'
```


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--state` | stringSlice | `[]` | Filter by Job state (repeatable) |
| `--kind` | stringSlice | `[]` | Filter by Job kind (repeatable) |
| `--origin` | stringSlice | `[]` | Filter by submission origin (repeatable) |
| `--owner-id` | uint | `0` | Filter by owner user ID |
| `--actor-id` | uint | `0` | Filter by acting user ID |
| `--accepted-after` | string | `` | Include Jobs accepted at or after RFC3339 time |
| `--accepted-before` | string | `` | Include Jobs accepted at or before RFC3339 time |
| `--relationship` | string | `` | Filter by visible lineage relationship |
| `--inbound-relationship` | string | `` | Filter Jobs a visible Job links to with this relationship (retried, repeated, or child stage) |
| `--no-inbound-relationship` | string | `` | Filter Jobs no visible Job links to with this relationship (for example, not yet retried) |
| `--search` | string | `` | Search visible Job text and output labels |
| `--command` | string | `` | Filter Jobs currently advertising this command key |
| `--pinned` | string | `` | Filter this viewer's pin preference (true or false) |
| `--dismissed` | string | `` | Filter this viewer's dismissal preference (true or false) |
| `--cursor` | string | `` | Opaque cursor returned by the previous page |
| `--limit` | int | `0` | Jobs per page (server maximum: 200) |
### Inherited global flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | bool | `false` | Output raw JSON |
| `--no-header` | bool | `false` | Omit table headers |
| `--page` | int | `1` | Page number for list commands (default page size: 50) |
| `--quiet` | bool | `false` | Only output IDs |
| `--server` | string | `http://localhost:8181` | mahresources server URL (env: MAHRESOURCES_URL) |
## Output

Canonical page with visible Jobs and an optional nextCursor

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr jobs get`](./get.md)
- [`mr jobs timeline`](./timeline.md)
- [`mr jobs summary`](./summary/index.md)
- [`mr job submit`](../job/submit.md)
