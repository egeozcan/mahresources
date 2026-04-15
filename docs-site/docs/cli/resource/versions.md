---
title: mr resource versions
description: List versions of a resource
sidebar_label: versions
---

# mr resource versions

List every stored version of a Resource, newest first. Columns are the
version ID, version number, size in bytes, content type, an optional
author comment, and the creation timestamp. Pass the global `--json`
flag to get the full records for scripting.

## Usage

    mr resource versions <resource-id>

Positional arguments:

- `<resource-id>`


## Examples

**List versions (table)**

    mr resource versions 42

**Get the newest version's ID via jq**

    mr resource versions 42 --json | jq -r '.[0].id'

**upload a resource**

    GRP=$(mr group create --name "doctest-versions-$$-$RANDOM" --json | jq -r '.ID')
    ID=$(mr resource upload ./testdata/sample.jpg --owner-id=$GRP --name "versions-test-$$" --json | jq -r '.[0].ID')
    mr resource version-upload $ID ./testdata/sample.png
    mr resource versions $ID --json | jq -e 'length >= 2'


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

Array of version objects with id, number, size, type, comment, created

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr resource version`](./version.md)
- [`mr resource version-upload`](./version-upload.md)
- [`mr resource versions-compare`](./versions-compare.md)
