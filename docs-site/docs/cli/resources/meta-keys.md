---
title: mr resources meta-keys
description: List all unique metadata keys used across resources
sidebar_label: meta-keys
---

# mr resources meta-keys

List every distinct `meta` key observed across the entire Resource
corpus. Useful for discovering the vocabulary of an evolving meta
schema. The command has no filter flags in the current CLI; pair it
with client-side `jq` filtering if you only want a subset of keys.

## Usage

```bash
mr resources meta-keys
```

## Examples

**List all meta keys**

```bash
mr resources meta-keys
```

**Filter client-side with jq**

```bash
mr resources meta-keys --json | jq '.[] | select(startswith("image_"))'
```


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

Array of distinct meta key strings across the entire resource corpus

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr resource edit-meta`](../resource/edit-meta.md)
- [`mr resources add-meta`](./add-meta.md)
