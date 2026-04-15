---
title: mr notes meta-keys
description: List all unique metadata keys used across notes
sidebar_label: meta-keys
---

# mr notes meta-keys

List every distinct `meta` key observed across the entire Note corpus.
Useful for discovering the vocabulary of an evolving meta schema. The
response is a JSON array of objects each shaped `{"key": "..."}`. The
command has no filter flags in the current CLI; pair it with
client-side `jq` filtering if you only want a subset of keys.

## Usage

```bash
mr notes meta-keys
```

## Examples

**List all meta keys**

```bash
mr notes meta-keys
```

**Filter client-side with jq**

```bash
mr notes meta-keys --json | jq '.[] | select(.key | startswith("project_"))'
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

Array of objects with key (string), one per distinct meta key observed across the entire Note corpus

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr note edit-meta`](../note/edit-meta.md)
- [`mr notes add-meta`](./add-meta.md)
