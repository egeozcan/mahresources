---
title: mr groups meta-keys
description: List all unique metadata keys used across groups
sidebar_label: meta-keys
---

# mr groups meta-keys

List every distinct `Meta` key observed across the entire Group corpus.
Useful for discovering the vocabulary of an evolving meta schema and
for building UI dropdowns of known keys. The command has no filter
flags in the current CLI; pair it with client-side `jq` filtering if
you only want a subset of keys.

The JSON shape is an array of objects with a `key` field
(`[{"key":"status"}, {"key":"owner"}]`), not a flat string array.

## Usage

    mr groups meta-keys

## Examples

**List all meta keys**

    mr groups meta-keys

**Filter client-side with jq**

    mr groups meta-keys --json | jq -r '.[].key | select(startswith("probe_"))'


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
## Output

Array of objects with shape [{"key": string}] — one entry per distinct Meta key across all Groups

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr group edit-meta`](../group/edit-meta.md)
- [`mr groups add-meta`](./add-meta.md)
