---
title: mr token list
description: List your API tokens
sidebar_label: list
---

# mr token list

Show the API tokens for the authenticated account, including their id, label, and display prefix. The secret value itself is never shown after creation.

## Usage

```bash
mr token list
```

## Examples

**List your tokens**

```bash
mr token list
```

**As raw JSON**

```bash
mr token list --json
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
## Exit Codes

0 success; 1 error (not authenticated, network error, or token not found)