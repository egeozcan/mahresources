---
title: mr group edit-meta
description: Edit a single metadata field by JSON path
sidebar_label: edit-meta
---

# mr group edit-meta

Edit a single metadata field using deep-merge-by-path.

The path is a dot-separated JSON path (e.g., "address.city") and the value
is a JSON literal (e.g., '"Berlin"', '42', '{"nested":"obj"}').

Examples:
  mr group edit-meta 5 status '"active"'
  mr group edit-meta 5 address.city '"Berlin"'
  mr group edit-meta 5 scores '[1,2,3]'

## Usage

    mr group edit-meta <id> <path> <value>

Positional arguments:

- `<id>`
- `<path>`
- `<value>`


## Examples


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

