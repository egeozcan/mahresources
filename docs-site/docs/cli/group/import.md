---
title: mr group import
description: Import a group export tar into this instance
sidebar_label: import
---

# mr group import

Upload an export tar, parse it, and optionally apply it.

Use --dry-run to parse and print the plan without applying.
Use --plan-output to save the plan JSON to a file.
Use --decisions to supply a decisions JSON file for full control.

## Usage

    mr group import <tarfile>

Positional arguments:

- `<tarfile>`


## Examples


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | `false` | Parse and print the plan without applying |
| `--plan-output` | string | `` | Write the plan JSON to a file |
| `--poll-interval` | duration | `1s` | Polling interval |
| `--timeout` | duration | `30m0s` | Max total wait time |
| `--parent-group` | uint | `0` | Parent group ID for imported top-level groups |
| `--on-resource-conflict` | string | `skip` | Resource collision policy: "skip" or "duplicate" |
| `--guid-collision-policy` | string | `` | GUID collision policy: "merge", "skip", or "replace" (default: server default = "merge") |
| `--auto-map` | bool | `true` | Automatically accept plan mapping suggestions |
| `--acknowledge-missing-hashes` | bool | `false` | Proceed even when some resources have no bytes |
| `--decisions` | string | `` | Path to a decisions JSON file (overrides other flags) |
### Inherited global flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | bool | `false` | Output raw JSON |
| `--no-header` | bool | `false` | Omit table headers |
| `--page` | int | `1` | Page number for list commands (default page size: 50) |
| `--quiet` | bool | `false` | Only output IDs |
| `--server` | string | `http://localhost:8181` | mahresources server URL (env: MAHRESOURCES_URL) |
## Exit Codes

