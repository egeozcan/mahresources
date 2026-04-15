---
title: mr docs
description: Introspect and validate the mr CLI's own documentation
sidebar_label: docs
---

# mr docs

Introspect and validate the mr CLI's own documentation. The `docs` subcommands
walk the command tree to emit machine-readable JSON, generate docs-site
Markdown pages, validate help text against the template rules, and execute
runnable examples.

Use `mr docs` during CLI development to keep help text consistent, and in CI
to guarantee that published documentation stays in sync with the
implementation.

## Usage

    mr docs

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

- [`mr docs dump`](./dump.md)
- [`mr docs lint`](./lint.md)
- [`mr docs check-examples`](./check-examples.md)
