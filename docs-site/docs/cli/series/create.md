---
title: mr series create
description: Create a new series
sidebar_label: create
---

# mr series create

Create a new series. `--name` is required. The server derives the slug
from the name at creation time; the slug never changes when the name is
later edited, so pick a name with care. On success prints a confirmation
line with the new ID; pass the global `--json` flag to emit the full
record for scripting (e.g., piping the new ID into follow-up commands).

## Usage

```bash
mr series create
```

## Examples

**Create a series with just a name**

```bash
mr series create --name "spring-2026-photos"
```

**Create and capture the new ID via jq**

```bash
ID=$(mr series create --name "volume-1" --json | jq -r .ID)
```


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--name` | string | `` | Series name (required) **(required)** |
### Inherited global flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | bool | `false` | Output raw JSON |
| `--no-header` | bool | `false` | Omit table headers |
| `--page` | int | `1` | Page number for list commands (default page size: 50) |
| `--quiet` | bool | `false` | Only output IDs |
| `--server` | string | `http://localhost:8181` | mahresources server URL (env: MAHRESOURCES_URL) |
## Output

Created Series object with ID (uint), Name (string), Slug (string), Meta (object), CreatedAt, UpdatedAt

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr series get`](./get.md)
- [`mr series edit-name`](./edit-name.md)
- [`mr series list`](./list.md)
