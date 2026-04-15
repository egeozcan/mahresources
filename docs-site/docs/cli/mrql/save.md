---
title: mr mrql save
description: Save a MRQL query
sidebar_label: save
---

# mr mrql save

Save a named MRQL query for later reuse. Takes two positional arguments:
`<name>` (a unique label) and `<query>` (the MRQL text). The optional
`--description` flag attaches a human-readable note. The query text is
validated at save time — malformed MRQL returns HTTP 400 with a parse
error pointing at the offending token, and the record is not persisted.

The created record is returned; capture `.id` from JSON output to run
or delete the query in follow-up commands. Saved queries can be executed
by ID or by name via `mrql run`.

## Usage

    mr mrql save <name> <query>

Positional arguments:

- `<name>`
- `<query>`


## Examples

**Save a simple named query**

    mr mrql save "recent-photos" 'type = resource AND tags = "photo"'

**Save with a description**

    mr mrql save "resources-by-type" 'type = resource GROUP BY contentType COUNT()' --description "Resource count per content type"


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--description` | string | `` | Description for the saved query |
### Inherited global flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | bool | `false` | Output raw JSON |
| `--no-header` | bool | `false` | Omit table headers |
| `--page` | int | `1` | Page number for list commands (default page size: 50) |
| `--quiet` | bool | `false` | Only output IDs |
| `--server` | string | `http://localhost:8181` | mahresources server URL (env: MAHRESOURCES_URL) |
## Output

Created saved MRQL query object with id (uint), name (string), query (string), description (string), createdAt, updatedAt

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr mrql list`](./list.md)
- [`mr mrql run`](./run.md)
- [`mr mrql delete`](./delete.md)
