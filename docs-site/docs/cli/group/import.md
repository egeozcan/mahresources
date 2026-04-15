---
title: mr group import
description: Import a group export tar into this instance
sidebar_label: import
---

# mr group import

Upload a group export tar, parse it into an import plan, and optionally
apply it. Takes the path to a tar file (produced by `mr group export`
or the `/v1/groups/export` API) as its single positional argument.

The command runs a two-phase job pipeline: first a `parse` job uploads
the tar, validates the manifest schema version, and produces an
`ImportPlan` (counts, mappings, conflicts, dangling refs). Then — unless
`--dry-run` is set — an `apply` job actually creates the groups and
related entities.

Use `--dry-run` to inspect the plan without mutating state. Use
`--plan-output <file>` to save the parsed plan JSON. Use
`--parent-group <id>` to graft imported top-level groups under an
existing parent. Use `--on-resource-conflict=skip|duplicate` and
`--guid-collision-policy=merge|skip|replace` to steer conflict
resolution. For full manual control over every mapping/dangling/shell
decision, pass `--decisions <json-file>` produced from a prior dry-run.

When the server plan reports resources without bytes in the tar,
`--acknowledge-missing-hashes` is required to proceed.

## Usage

    mr group import <tarfile>

Positional arguments:

- `<tarfile>`


## Examples

**Dry-run an import and print the plan**

    mr group import /tmp/trip-2026.tar --dry-run

**Import**

    mr group import /tmp/trip-2026.tar --parent-group 17

**Dry-run to JSON file for review**

    mr group import /tmp/trip-2026.tar --dry-run --plan-output /tmp/plan.json


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
## Output

ImportPlan (dry-run) or ImportApplyResult object with CreatedGroups, CreatedResources, SkippedByHash, CreatedNotes, CreatedGroupIDs arrays, etc.

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr group export`](./export.md)
- [`mr group create`](./create.md)
- [`mr groups list`](../groups/list.md)
