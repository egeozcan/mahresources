---
title: mr mrql explain
description: Show the SQL an MRQL query would run, without executing it
sidebar_label: explain
---

# mr mrql explain

Show the SQL statement(s) an MRQL query would run, without executing it.
Accepts an inline query (positional argument, `-f <file>`, or stdin `-`),
or a saved query via `--saved <name-or-id>`. Bind any `$name` parameter
placeholders with repeatable `--param name=value` flags.

The reported SQL reflects what would actually run: the default `LIMIT`
is applied (and noted on stderr), `SCOPE` is resolved, and RBAC forced
scoping for group-limited users is included. Flat single-entity queries
produce one statement; cross-entity queries produce one per entity table
(resources/notes/groups); aggregated `GROUP BY` produces one statement;
bucketed `GROUP BY` shows the key-discovery query with a note that the
per-bucket item query repeats once per group key.

By default the interpolated SQL is printed under a `-- <label> --`
header for each statement. Pass `--json` (or the global `--json`) to emit
the raw response, which additionally carries the parameterized `sql` and
its `vars` per statement.

## Usage

```bash
mr mrql explain [query]
```

Positional arguments:

- `<query>` (optional)


## Examples

**Explain an inline query**

```bash
mr mrql explain 'type = resource AND fileSize > 1mb'
```

**Explain a parameterized query**

```bash
mr mrql explain 'type = note AND name ~ $needle' --param needle=meeting
```

**Explain a saved query as JSON**

```bash
mr mrql explain --saved my-report --json
```


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--saved` | string | `` | Explain a saved query by name or ID instead of an inline query |
| `--file` | string | `` | Read query from file |
| `--param` | stringArray | `[]` | Bind a query parameter placeholder, repeatable: --param name=value |
### Inherited global flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--no-header` | bool | `false` | Omit table headers |
| `--page` | int | `1` | Page number for list commands (default page size: 50) |
| `--quiet` | bool | `false` | Only output IDs |
| `--server` | string | `http://localhost:8181` | mahresources server URL (env: MAHRESOURCES_URL) |
## Output

Human-readable label headers plus interpolated SQL, or with --json the raw explain response &#123;entityType, statements[], warnings, default_limit_applied, applied_limit&#125;

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr mrql`](./index.md)
- [`mr mrql run`](./run.md)
- [`mr mrql export`](./export.md)
