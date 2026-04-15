---
title: mr resource get
description: Get a resource by ID
sidebar_label: get
---

# mr resource get

Get a resource by ID and print its metadata. Fetches the full record
including tags, groups, resource category, owner, dimensions, hash,
and any custom meta JSON. Output is a key/value table by default; pass
the global `--json` flag to get the full record for scripting.

## Usage

    mr resource get <id>

Positional arguments:

- `<id>`


## Examples

**Get a resource by ID (table output)**

    mr resource get 42

**Get as JSON and extract a single field with jq**

    mr resource get 42 --json | jq -r .name

**upload a fixture and verify the resource is retrievable**

    GRP=$(mr group create --name "doctest-get-$$-$RANDOM" --json | jq -r '.ID')
    ID=$(mr resource upload ./testdata/sample.jpg --owner-id=$GRP --name "doctest-get-$$" --json | jq -r '.[0].ID')
    mr resource get $ID --json | jq -e '.ID > 0 and (.Name | length) > 0'


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

Resource object with id (uint), name (string), tags ([]Tag), groups ([]Group), meta (object)

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr resource edit`](./edit.md)
- [`mr resource versions`](./versions.md)
- [`mr resource download`](./download.md)
