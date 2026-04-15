---
title: mr resource versions-compare
description: Compare two versions of a resource
sidebar_label: versions-compare
---

# mr resource versions-compare

Compare two versions of a Resource and report the size delta, whether
the content hashes match, whether the content types match, and the
dimension differences. Both `--v1` and `--v2` are required and must be
version IDs of the same Resource.

## Usage

    mr resource versions-compare <resource-id>

Positional arguments:

- `<resource-id>`


## Examples

**Compare two versions (table)**

    mr resource versions-compare 42 --v1 17 --v2 21

**Extract sameHash via jq**

    mr resource versions-compare 42 --v1 17 --v2 21 --json | jq -r .sameHash

**upload same file twice**

    GRP=$(mr group create --name "doctest-vcompare-$$-$RANDOM" --json | jq -r '.ID')
    ID=$(mr resource upload ./testdata/sample.jpg --owner-id=$GRP --name "compare-test-$$" --json | jq -r '.[0].ID')
    V1=$(mr resource versions $ID --json | jq -r '.[0].id')
    mr resource version-upload $ID ./testdata/sample.jpg
    V2=$(mr resource versions $ID --json | jq -r '.[0].id')
    mr resource versions-compare $ID --v1 $V1 --v2 $V2 --json | jq -e '.sameHash == true'


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--v1` | uint | `0` | First version ID (required) **(required)** |
| `--v2` | uint | `0` | Second version ID (required) **(required)** |
### Inherited global flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | bool | `false` | Output raw JSON |
| `--no-header` | bool | `false` | Omit table headers |
| `--page` | int | `1` | Page number for list commands (default page size: 50) |
| `--quiet` | bool | `false` | Only output IDs |
| `--server` | string | `http://localhost:8181` | mahresources server URL (env: MAHRESOURCES_URL) |
## Output

Comparison object with sizeDelta, sameHash, sameType, dimensionsDiff

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr resource versions`](./versions.md)
- [`mr resource version`](./version.md)
- [`mr resource versions-cleanup`](./versions-cleanup.md)
