---
title: mr queries
description: List saved queries
sidebar_label: queries
---

# mr queries

Discover and summarize saved Queries. The `queries` subcommands
operate on the collection: `list` returns queries (paged via the
global `--page` flag, optionally filtered by `--name`), and `timeline`
aggregates query creation and update activity into an ASCII bar chart.

To execute a query, use `query run <id>` or `query run-by-name --name
<name>` from the singular `query` subtree.

## Usage

    mr queries

## Flags

This command has no local flags.
### Inherited global flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | bool | `false` | Output raw JSON |
| `--no-header` | bool | `false` | Omit table headers |
| `--page` | int | `1` | Page number for list commands (default page size: 50) |
| `--quiet` | bool | `false` | Only output IDs |
| `--server` | string | `http://localhost:8181` | mahresources server URL (env: MAHRESOURCES_URL) |
## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr query get`](../query/get.md)
- [`mr query run`](../query/run.md)
- [`mr mrql`](../mrql/index.md)
