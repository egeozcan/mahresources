---
title: mr docs check-examples
description: Run `# mr-doctest:` example blocks against a live server
sidebar_label: check-examples
---

# mr docs check-examples

Walks the command tree, extracts every example tagged `# mr-doctest:`, and
evaluates each block against the connected server. Per-example metadata on the
label line controls behavior: `expect-exit=N`, `tolerate=/regex/`,
`skip-on=ephemeral`, `timeout=Ns`, and `stdin=<fixture>`.

The runner pipes each block through `bash -e -o pipefail -c`, with cwd set to
`cmd/mr/` so examples can reference `./testdata/*` fixtures. Requires
`MAHRESOURCES_URL`, `bash`, and `jq` on PATH.

## Usage

    mr docs check-examples

## Examples

**Run against a local ephemeral server**

    mr docs check-examples --server http://localhost:8181 --environment=ephemeral

**Inherit server URL from the environment**

    MAHRESOURCES_URL=http://localhost:8181 mr docs check-examples --environment=ephemeral


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--environment` | string | `` | Target environment label used by `skip-on=<env>` metadata. Example: `ephemeral` when targeting a seed-less in-memory server. |
### Inherited global flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | bool | `false` | Output raw JSON |
| `--no-header` | bool | `false` | Omit table headers |
| `--page` | int | `1` | Page number for list commands (default page size: 50) |
| `--quiet` | bool | `false` | Only output IDs |
## Exit Codes

0 if every non-skipped doctest passes its declared expectation; 1 otherwise

## See Also

- [`mr docs lint`](./lint.md)
- [`mr docs dump`](./dump.md)
