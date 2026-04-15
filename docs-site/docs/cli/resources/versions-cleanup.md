---
title: mr resources versions-cleanup
description: Clean up old versions across resources
sidebar_label: versions-cleanup
---

# mr resources versions-cleanup

Bulk-clean old Resource versions across the entire corpus. Applies the
same retention rules as the singular `resource versions-cleanup`:
`--keep N` retains the N most recent versions per resource;
`--older-than-days N` removes versions older than N days. Both filters
may be combined. Scope the operation to a single owner group with
`--owner-id`. Pass `--dry-run` to preview the count of versions that
would be removed without committing any deletes.

## Usage

    mr resources versions-cleanup

## Examples

**Keep last 3 versions across all resources**

    mr resources versions-cleanup --keep 3

**Preview cleanup of versions older than 90 days**

    mr resources versions-cleanup --older-than-days 90 --owner-id 5 --dry-run

**Remove all but the latest version across the entire corpus**

    mr resources versions-cleanup --keep 1

**upload 2 resources**

    GRP=$(mr group create --name "doctest-vcsbulk-$$-$RANDOM" --json | jq -r '.ID')
    ID1=$(mr resource upload ./testdata/sample.jpg --owner-id=$GRP --name "vcu-a-$$" --json | jq -r '.[0].ID')
    ID2=$(mr resource upload ./testdata/sample.png --owner-id=$GRP --name "vcu-b-$$" --json | jq -r '.[0].ID')
    mr resource version-upload $ID1 ./testdata/sample.png
    mr resource version-upload $ID2 ./testdata/sample.jpg
    mr resources versions-cleanup --keep 1
    mr resource versions $ID1 --json | jq -e 'length == 1'
    mr resource versions $ID2 --json | jq -e 'length == 1'


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--keep` | uint | `0` | Number of versions to keep |
| `--older-than-days` | uint | `0` | Delete versions older than N days |
| `--owner-id` | uint | `0` | Filter by owner group ID |
| `--dry-run` | bool | `false` | Preview without deleting |
### Inherited global flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | bool | `false` | Output raw JSON |
| `--no-header` | bool | `false` | Omit table headers |
| `--page` | int | `1` | Page number for list commands (default page size: 50) |
| `--quiet` | bool | `false` | Only output IDs |
| `--server` | string | `http://localhost:8181` | mahresources server URL (env: MAHRESOURCES_URL) |
## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr resource versions-cleanup`](../resource/versions-cleanup.md)
- [`mr resource versions`](../resource/versions.md)
- [`mr resources list`](./list.md)
